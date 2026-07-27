// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	. "github.com/onsi/gomega"
)

type fakeDiscovery struct {
	resources []*metav1.APIResourceList
}

func (f *fakeDiscovery) ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error) {
	for _, rl := range f.resources {
		if rl.GroupVersion == groupVersion {
			return rl, nil
		}
	}
	return nil, &apierrors.StatusError{
		ErrStatus: metav1.Status{
			Status: metav1.StatusFailure,
			Reason: metav1.StatusReasonNotFound,
		},
	}
}

func TestIsClusterAPIRegistered(t *testing.T) {
	tests := []struct {
		name           string
		resources      []*metav1.APIResourceList
		expectedResult bool
	}{
		{
			name: "When ClusterAPI API is registered it should return true",
			resources: []*metav1.APIResourceList{
				{
					GroupVersion: "operator.openshift.io/v1alpha1",
					APIResources: []metav1.APIResource{
						{Name: "imagecontentsourcepolicies", Kind: "ImageContentSourcePolicy"},
						{Name: "clusterapis", Kind: "ClusterAPI"},
					},
				},
			},
			expectedResult: true,
		},
		{
			name:           "When API group is not registered it should return false",
			resources:      nil,
			expectedResult: false,
		},
		{
			name: "When API group exists but ClusterAPI kind is absent it should return false",
			resources: []*metav1.APIResourceList{
				{
					GroupVersion: "operator.openshift.io/v1alpha1",
					APIResources: []metav1.APIResource{
						{Name: "imagecontentsourcepolicies", Kind: "ImageContentSourcePolicy"},
					},
				},
			},
			expectedResult: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			disco := &fakeDiscovery{resources: tc.resources}

			registered, err := isClusterAPIRegistered(disco)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(registered).To(Equal(tc.expectedResult))
		})
	}
}

func TestIsCAPICRD(t *testing.T) {
	tests := []struct {
		name     string
		crdName  string
		expected bool
	}{
		{"core CAPI CRD", "clusters.cluster.x-k8s.io", true},
		{"infrastructure provider CRD", "metal3clusters.infrastructure.cluster.x-k8s.io", true},
		{"bootstrap provider CRD", "eksconfigs.bootstrap.cluster.x-k8s.io", true},
		{"controlplane provider CRD", "rosacontrolplanes.controlplane.cluster.x-k8s.io", true},
		{"addons CRD", "clusterresourcesets.addons.cluster.x-k8s.io", true},
		{"runtime CRD", "extensionconfigs.runtime.cluster.x-k8s.io", true},
		{"non-CAPI CRD", "nodepools.hypershift.openshift.io", false},
		{"Azure CRD", "resourcegroups.resources.azure.com", false},
		{"Metal3 IPAM CRD", "ippools.ipam.metal3.io", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			g.Expect(isCAPICRD(tc.crdName)).To(Equal(tc.expected))
		})
	}
}

func TestGetCAPICRDNames(t *testing.T) {
	g := NewGomegaWithT(t)

	crds := []*unstructured.Unstructured{
		makeCRD("clusters.cluster.x-k8s.io"),
		makeCRD("machines.cluster.x-k8s.io"),
		makeCRD("nodepools.hypershift.openshift.io"),
		makeCRD("metal3clusters.infrastructure.cluster.x-k8s.io"),
		makeCRD("ippools.ipam.metal3.io"),
		makeCRD("clusters.cluster.x-k8s.io"), // duplicate
	}

	names := getCAPICRDNames(crds)
	g.Expect(names).To(Equal([]string{
		"clusters.cluster.x-k8s.io",
		"machines.cluster.x-k8s.io",
		"metal3clusters.infrastructure.cluster.x-k8s.io",
	}))
}

func TestGetCAPICRDNames_NoCAPICRDs(t *testing.T) {
	g := NewGomegaWithT(t)

	crds := []*unstructured.Unstructured{
		makeCRD("nodepools.hypershift.openshift.io"),
		makeCRD("ippools.ipam.metal3.io"),
	}

	names := getCAPICRDNames(crds)
	g.Expect(names).To(BeEmpty())
}

func TestEnsureUnmanagedCRDs(t *testing.T) {
	gvk := schema.GroupVersionKind{
		Group:   "operator.openshift.io",
		Version: "v1alpha1",
		Kind:    "ClusterAPI",
	}

	ssaInterceptor := interceptor.Funcs{
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			if patch.Type() != types.ApplyPatchType {
				return c.Patch(ctx, obj, patch, opts...)
			}
			existing := obj.DeepCopyObject().(client.Object)
			err := c.Get(ctx, client.ObjectKeyFromObject(obj), existing)
			if apierrors.IsNotFound(err) {
				return c.Create(ctx, obj)
			}
			if err != nil {
				return err
			}
			obj.SetResourceVersion(existing.GetResourceVersion())
			return c.Update(ctx, obj)
		},
	}

	tests := []struct {
		name             string
		existingConfig   *unstructured.Unstructured
		crds             []*unstructured.Unstructured
		expectedCRDNames []string
		expectNoChange   bool
	}{
		{
			name:           "When ClusterAPI config does not exist it should create it with unmanaged CRDs",
			existingConfig: nil,
			crds: []*unstructured.Unstructured{
				makeCRD("clusters.cluster.x-k8s.io"),
				makeCRD("machines.cluster.x-k8s.io"),
				makeCRD("nodepools.hypershift.openshift.io"),
			},
			expectedCRDNames: []string{
				"clusters.cluster.x-k8s.io",
				"machines.cluster.x-k8s.io",
			},
		},
		{
			name: "When ClusterAPI config already exists it should update it",
			existingConfig: makeClusterAPIConfig(gvk, []string{
				"clusters.cluster.x-k8s.io",
			}),
			crds: []*unstructured.Unstructured{
				makeCRD("clusters.cluster.x-k8s.io"),
				makeCRD("machines.cluster.x-k8s.io"),
			},
			expectedCRDNames: []string{
				"clusters.cluster.x-k8s.io",
				"machines.cluster.x-k8s.io",
			},
		},
		{
			name:           "When no CAPI CRDs are present it should not create config",
			existingConfig: nil,
			crds: []*unstructured.Unstructured{
				makeCRD("nodepools.hypershift.openshift.io"),
			},
			expectNoChange: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			s := runtime.NewScheme()
			s.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
			s.AddKnownTypeWithName(
				schema.GroupVersionKind{Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind + "List"},
				&unstructured.UnstructuredList{},
			)

			builder := fake.NewClientBuilder().
				WithScheme(s).
				WithInterceptorFuncs(ssaInterceptor)
			if tc.existingConfig != nil {
				builder = builder.WithObjects(tc.existingConfig)
			}
			cl := builder.Build()

			_, err := ensureUnmanagedCRDs(t.Context(), cl, tc.crds)
			g.Expect(err).ToNot(HaveOccurred())

			if tc.expectNoChange {
				config := &unstructured.Unstructured{}
				config.SetGroupVersionKind(gvk)
				err := cl.Get(t.Context(), client.ObjectKey{Name: "cluster"}, config)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected no ClusterAPI config to be created")
				return
			}

			config := &unstructured.Unstructured{}
			config.SetGroupVersionKind(gvk)
			err = cl.Get(t.Context(), client.ObjectKey{Name: "cluster"}, config)
			g.Expect(err).ToNot(HaveOccurred())

			spec, found, _ := unstructured.NestedMap(config.Object, "spec")
			g.Expect(found).To(BeTrue())
			unmanagedRaw, found, _ := unstructured.NestedStringSlice(spec, "unmanagedCustomResourceDefinitions")
			g.Expect(found).To(BeTrue())
			g.Expect(unmanagedRaw).To(ConsistOf(tc.expectedCRDNames))
		})
	}
}

func TestWaitForCAPIOperatorSync(t *testing.T) {
	gvk := schema.GroupVersionKind{
		Group:   "operator.openshift.io",
		Version: "v1alpha1",
		Kind:    "ClusterAPI",
	}

	tests := []struct {
		name            string
		config          *unstructured.Unstructured
		patchGeneration int64
		expectSuccess   bool
	}{
		{
			name:            "When operator has synced it should succeed",
			config:          makeClusterAPIConfigWithStatus(gvk, 2, 2, "rev-2", "rev-2"),
			patchGeneration: 2,
			expectSuccess:   true,
		},
		{
			name:            "When observedRevisionGeneration is ahead it should succeed",
			config:          makeClusterAPIConfigWithStatus(gvk, 3, 3, "rev-3", "rev-3"),
			patchGeneration: 2,
			expectSuccess:   true,
		},
		{
			name:            "When operator has not observed the patch generation it should time out",
			config:          makeClusterAPIConfigWithStatus(gvk, 3, 2, "rev-2", "rev-2"),
			patchGeneration: 3,
			expectSuccess:   false,
		},
		{
			name:            "When currentRevision does not match desiredRevision it should time out",
			config:          makeClusterAPIConfigWithStatus(gvk, 2, 2, "rev-1", "rev-2"),
			patchGeneration: 2,
			expectSuccess:   false,
		},
		{
			name:            "When currentRevision is empty it should time out",
			config:          makeClusterAPIConfigWithStatus(gvk, 1, 1, "", "rev-1"),
			patchGeneration: 1,
			expectSuccess:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			s := runtime.NewScheme()
			s.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
			s.AddKnownTypeWithName(
				schema.GroupVersionKind{Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind + "List"},
				&unstructured.UnstructuredList{},
			)

			cl := fake.NewClientBuilder().
				WithScheme(s).
				WithObjects(tc.config).
				Build()

			ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
			defer cancel()

			err := waitForCAPIOperatorSync(ctx, cl, tc.patchGeneration)
			if tc.expectSuccess {
				g.Expect(err).ToNot(HaveOccurred())
			} else {
				g.Expect(err).To(HaveOccurred())
			}
		})
	}
}

func makeCRD(name string) *unstructured.Unstructured {
	crd := &unstructured.Unstructured{}
	crd.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1",
		Kind:    "CustomResourceDefinition",
	})
	crd.SetName(name)
	return crd
}

func makeClusterAPIConfig(gvk schema.GroupVersionKind, unmanagedCRDs []string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": gvk.Group + "/" + gvk.Version,
			"kind":       gvk.Kind,
			"metadata": map[string]any{
				"name": "cluster",
			},
			"spec": map[string]any{
				"unmanagedCustomResourceDefinitions": toAnySlice(unmanagedCRDs),
			},
		},
	}
	return obj
}

func makeClusterAPIConfigWithStatus(gvk schema.GroupVersionKind, generation, observedGen int64, currentRev, desiredRev string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": gvk.Group + "/" + gvk.Version,
			"kind":       gvk.Kind,
			"metadata": map[string]any{
				"name":       "cluster",
				"generation": generation,
			},
			"status": map[string]any{
				"observedRevisionGeneration": observedGen,
				"currentRevision":            currentRev,
				"desiredRevision":            desiredRev,
			},
		},
	}
	return obj
}

func toAnySlice(ss []string) []any {
	result := make([]any, len(ss))
	for i, s := range ss {
		result[i] = s
	}
	return result
}
