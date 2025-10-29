// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package foundation

import (
	"context"
	"os"
	"testing"

	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ocmapiv1 "open-cluster-management.io/api/operator/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/stolostron/backplane-operator/api/v1"
)

func TestClusterManager(t *testing.T) {
	tests := []struct {
		name                      string
		mce                       *v1.MultiClusterEngine
		imageOverrides            map[string]string
		expectedNodeSelector      map[string]string
		expectedRegistrationImage string
		expectedWorkImage         string
		expectedPlacementImage    string
		expectedAddonManagerImage string
	}{
		{
			name: "create cluster manager without nodeSelector",
			mce:  &v1.MultiClusterEngine{},
			imageOverrides: map[string]string{
				"registration":  "quay.io/stolostron/registration@sha256:fe95bca419976ca8ffe608bc66afcead6ef333b863f22be55df57c89ded75dda",
				"work":          "quay.io/stolostron/work@sha256:856d2151423f020952d9b9253676c1c4d462fab6722c8af4885fe2b19ccd1be0",
				"placement":     "quay.io/stolostron/placement@sha256:8d69eb89ee008bf95c2b877887e66cc1541c2407c9d7339fff8a9a973200660f",
				"addon_manager": "quay.io/stolostron/addon-manager@sha256:8d69eb89ee008bf95c2b877887e66cc1541c2407c9d7339fff8a9a973200660e",
			},
			expectedRegistrationImage: "quay.io/stolostron/registration@sha256:fe95bca419976ca8ffe608bc66afcead6ef333b863f22be55df57c89ded75dda",
			expectedWorkImage:         "quay.io/stolostron/work@sha256:856d2151423f020952d9b9253676c1c4d462fab6722c8af4885fe2b19ccd1be0",
			expectedPlacementImage:    "quay.io/stolostron/placement@sha256:8d69eb89ee008bf95c2b877887e66cc1541c2407c9d7339fff8a9a973200660f",
			expectedAddonManagerImage: "quay.io/stolostron/addon-manager@sha256:8d69eb89ee008bf95c2b877887e66cc1541c2407c9d7339fff8a9a973200660e",
		},
		{
			name: "create cluster manager with nodeSelector",
			mce:  &v1.MultiClusterEngine{Spec: v1.MultiClusterEngineSpec{NodeSelector: map[string]string{"node-role.kubernetes.io/infra": ""}}},
			imageOverrides: map[string]string{
				"registration":  "quay.io/stolostron/registration@sha256:fe95bca419976ca8ffe608bc66afcead6ef333b863f22be55df57c89ded75dda",
				"work":          "quay.io/stolostron/work@sha256:856d2151423f020952d9b9253676c1c4d462fab6722c8af4885fe2b19ccd1be0",
				"placement":     "quay.io/stolostron/placement@sha256:8d69eb89ee008bf95c2b877887e66cc1541c2407c9d7339fff8a9a973200660f",
				"addon_manager": "quay.io/stolostron/addon-manager@sha256:8d69eb89ee008bf95c2b877887e66cc1541c2407c9d7339fff8a9a973200660e",
			},
			expectedNodeSelector:      map[string]string{"node-role.kubernetes.io/infra": ""},
			expectedRegistrationImage: "quay.io/stolostron/registration@sha256:fe95bca419976ca8ffe608bc66afcead6ef333b863f22be55df57c89ded75dda",
			expectedWorkImage:         "quay.io/stolostron/work@sha256:856d2151423f020952d9b9253676c1c4d462fab6722c8af4885fe2b19ccd1be0",
			expectedPlacementImage:    "quay.io/stolostron/placement@sha256:8d69eb89ee008bf95c2b877887e66cc1541c2407c9d7339fff8a9a973200660f",
			expectedAddonManagerImage: "quay.io/stolostron/addon-manager@sha256:8d69eb89ee008bf95c2b877887e66cc1541c2407c9d7339fff8a9a973200660e",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := ClusterManager(test.mce, test.imageOverrides)

			os.Setenv("DIRECTORY_OVERRIDE", "../../")

			_, err := GetAddons()
			if err != nil {
				t.Errorf("expected cluster manager add-ons not found")
			}

			registrationImage, found, err := unstructured.NestedString(c.Object, "spec", "registrationImagePullSpec")
			if err != nil || !found {
				t.Errorf("expected cluster manager registrationImagePullSpec not found")
			}
			if registrationImage != test.expectedRegistrationImage {
				t.Errorf("expected registrationImagePullSpec %s, got %s", test.expectedRegistrationImage, registrationImage)
			}

			workImage, found, err := unstructured.NestedString(c.Object, "spec", "workImagePullSpec")
			if err != nil || !found {
				t.Errorf("expected cluster manager workImagePullSpec not found")
			}
			if workImage != test.expectedWorkImage {
				t.Errorf("expected workImagePullSpec %s, got %s", test.expectedWorkImage, workImage)
			}

			placementImage, found, err := unstructured.NestedString(c.Object, "spec", "placementImagePullSpec")
			if err != nil || !found {
				t.Errorf("expected cluster manager placementImagePullSpec not found")
			}
			if placementImage != test.expectedPlacementImage {
				t.Errorf("expected placementImagePullSpec %s, got %s", test.expectedPlacementImage, placementImage)
			}

			addonManagerImage, found, err := unstructured.NestedString(
				c.Object, "spec", "addOnManagerImagePullSpec")
			if err != nil || !found {
				t.Errorf("expected cluster manager addOnManagerImagePullSpec not found")
			}
			if addonManagerImage != test.expectedAddonManagerImage {
				t.Errorf("expected addOnManagerImagePullSpec %s, got %s",
					test.expectedAddonManagerImage, addonManagerImage)
			}

			nodeSelector, found, err := unstructured.NestedMap(c.Object, "spec", "nodePlacement", "nodeSelector")
			if len(test.expectedNodeSelector) != 0 && (err != nil || !found) {
				t.Errorf("expected cluster manager NodeSelector not found")
			}

			for k, v := range test.expectedNodeSelector {
				if nodeSelector[k] != v {
					t.Errorf("expected NodeSelector %s, got %s", test.expectedNodeSelector, nodeSelector)
				}
			}

			mode, found, err := unstructured.NestedString(c.Object, "spec", "deployOption", "mode")
			if len(test.expectedNodeSelector) != 0 && (err != nil || !found) {
				t.Errorf("expected cluster manager deploy mode not found")
			}

			if mode != string(ocmapiv1.InstallModeDefault) {
				t.Errorf("expected deploy mode %s, got %s", ocmapiv1.InstallModeDefault, mode)
			}
		})
	}
}

func TestCRDNotFoundError(t *testing.T) {
	tests := []struct {
		name        string
		crdName     string
		expectedMsg string
	}{
		{
			name:        "clustermanagementaddons CRD not found",
			crdName:     "clustermanagementaddons.addon.open-cluster-management.io",
			expectedMsg: "CRD not found: clustermanagementaddons.addon.open-cluster-management.io",
		},
		{
			name:        "generic CRD not found",
			crdName:     "someresource.example.com",
			expectedMsg: "CRD not found: someresource.example.com",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := &CRDNotFoundError{CRDName: test.crdName}
			if err.Error() != test.expectedMsg {
				t.Errorf("expected error message %q, got %q", test.expectedMsg, err.Error())
			}
		})
	}
}

func TestCanInstallAddons(t *testing.T) {
	tests := []struct {
		name        string
		crdExists   bool
		expectError bool
	}{
		{
			name:        "CRD exists - can install addons",
			crdExists:   true,
			expectError: false,
		},
		{
			name:        "CRD does not exist - cannot install addons",
			crdExists:   false,
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = apixv1.AddToScheme(scheme)

			var objs []runtime.Object
			if test.crdExists {
				crd := &apixv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: clusterManagementAddonCRDName,
					},
				}
				objs = append(objs, crd)
			}

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

			err := CanInstallAddons(context.Background(), fakeClient)

			if test.expectError && err == nil {
				t.Errorf("expected error but got nil")
			}
			if !test.expectError && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
			if test.expectError {
				if _, ok := err.(*CRDNotFoundError); !ok {
					t.Errorf("expected CRDNotFoundError but got: %T", err)
				}
				if err.Error() != "CRD not found: "+clusterManagementAddonCRDName {
					t.Errorf("expected error message 'CRD not found: %s', got %q", clusterManagementAddonCRDName, err.Error())
				}
			}
		})
	}
}
