// Copyright Contributors to the Open Cluster Management project

package v1

import (
	"context"
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	cl "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

var (
	multiClusterEngineName = "multiclusterengine"
)

func TestValidateComponentDependencies(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		components  []ComponentConfig
		wantErr     string
	}{
		{
			name: "KubeVirt provider accepts omitted CAPI default",
			components: []ComponentConfig{
				{Name: ClusterAPIProviderKubeVirt, Enabled: true},
			},
		},
		{
			name: "CAPOA accepts omitted dependency defaults",
			components: []ComponentConfig{
				{Name: ClusterAPIProviderOA, Enabled: true},
			},
		},
		{
			name: "HyperShift and KubeVirt providers can coexist",
			components: []ComponentConfig{
				{Name: ClusterAPI, Enabled: true},
				{Name: ClusterAPIProviderOA, Enabled: true},
				{Name: ClusterAPIProviderKubeVirt, Enabled: true},
				{Name: AssistedService, Enabled: true},
				{Name: HyperShift, Enabled: true},
				{Name: HypershiftLocalHosting, Enabled: true},
			},
		},
		{
			name: "CAPOA requires CAPI",
			components: []ComponentConfig{
				{Name: ClusterAPIProviderOA, Enabled: true},
				{Name: AssistedService, Enabled: true},
				{Name: ClusterAPI, Enabled: false},
			},
			wantErr: "cluster-api-provider-openshift-assisted requires cluster-api",
		},
		{
			name: "Dependency-aware defaults remain valid when CAPI is disabled",
			components: []ComponentConfig{
				{Name: ClusterAPI, Enabled: false},
				{Name: ClusterAPIProviderKubeVirt, Enabled: false},
				{Name: ClusterAPIProviderOA, Enabled: false},
				{Name: AssistedService, Enabled: true},
			},
		},
		{
			name: "CAPOA requires Assisted Service",
			components: []ComponentConfig{
				{Name: ClusterAPIProviderOA, Enabled: true},
				{Name: ClusterAPI, Enabled: true},
				{Name: AssistedService, Enabled: false},
			},
			wantErr: "cluster-api-provider-openshift-assisted requires assisted-service",
		},
		{
			name: "CAPK requires CAPI",
			components: []ComponentConfig{
				{Name: ClusterAPIProviderKubeVirt, Enabled: true},
				{Name: ClusterAPI, Enabled: false},
			},
			wantErr: "cluster-api-provider-kubevirt requires cluster-api",
		},
		{
			name: "CAPK respects a disabled Cluster API preview default",
			components: []ComponentConfig{
				{Name: ClusterAPIPreview, Enabled: false},
				{Name: ClusterAPIProviderKubeVirt, Enabled: true},
				{Name: AssistedService, Enabled: true},
			},
			wantErr: "cluster-api-provider-kubevirt requires cluster-api",
		},
		{
			name: "Hosted mode rejects CAPK",
			annotations: map[string]string{
				"deploymentmode": string(ModeHosted),
			},
			components: []ComponentConfig{
				{Name: ClusterAPI, Enabled: true},
				{Name: ClusterAPIProviderKubeVirt, Enabled: true},
			},
			wantErr: "cluster-api-provider-kubevirt cannot be enabled in hosted mode",
		},
		{
			name: "Hosted mode rejects CAPK preview",
			annotations: map[string]string{
				"deploymentmode": string(ModeHosted),
			},
			components: []ComponentConfig{
				{Name: ClusterAPIProviderKubeVirtPreview, Enabled: true},
			},
			wantErr: "cluster-api-provider-kubevirt cannot be enabled in hosted mode",
		},
		{
			name: "Hosted mode rejects CAPOA",
			annotations: map[string]string{
				"deploymentmode": string(ModeHosted),
			},
			components: []ComponentConfig{
				{Name: ClusterAPI, Enabled: true},
				{Name: AssistedService, Enabled: true},
				{Name: ClusterAPIProviderOA, Enabled: true},
			},
			wantErr: "cluster-api-provider-openshift-assisted cannot be enabled in hosted mode",
		},
		{
			name: "Hosted mode rejects CAPOA preview",
			annotations: map[string]string{
				"deploymentmode": string(ModeHosted),
			},
			components: []ComponentConfig{
				{Name: ClusterAPIProviderOAPreview, Enabled: true},
			},
			wantErr: "cluster-api-provider-openshift-assisted cannot be enabled in hosted mode",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mce := &MultiClusterEngine{
				ObjectMeta: metav1.ObjectMeta{Annotations: test.annotations},
				Spec: MultiClusterEngineSpec{
					Overrides: &Overrides{Components: test.components},
				},
			}
			err := mce.validateComponentDependencies()
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("validateComponentDependencies() unexpected error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateComponentDependencies() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestEffectiveDependencyEnabled(t *testing.T) {
	tests := []struct {
		name       string
		components []ComponentConfig
		component  string
		want       bool
	}{
		{
			name:      "omitted Cluster API defaults to enabled",
			component: ClusterAPI,
			want:      true,
		},
		{
			name: "omitted Cluster API respects a disabled preview preference",
			components: []ComponentConfig{
				{Name: ClusterAPIPreview, Enabled: false},
			},
			component: ClusterAPI,
			want:      false,
		},
		{
			name: "omitted Cluster API respects an enabled preview preference",
			components: []ComponentConfig{
				{Name: ClusterAPIPreview, Enabled: true},
			},
			component: ClusterAPI,
			want:      true,
		},
		{
			name:      "omitted Assisted Service defaults to enabled",
			component: AssistedService,
			want:      true,
		},
		{
			name: "explicitly enabled dependency is enabled",
			components: []ComponentConfig{
				{Name: ClusterAPI, Enabled: true},
			},
			component: ClusterAPI,
			want:      true,
		},
		{
			name: "explicitly disabled dependency is disabled",
			components: []ComponentConfig{
				{Name: ClusterAPI, Enabled: false},
			},
			component: ClusterAPI,
			want:      false,
		},
		{
			name:      "unsupported component has no dependency default",
			component: Discovery,
			want:      false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mce := &MultiClusterEngine{
				Spec: MultiClusterEngineSpec{
					Overrides: &Overrides{Components: test.components},
				},
			}
			if got := mce.EffectiveDependencyEnabled(test.component); got != test.want {
				t.Errorf("EffectiveDependencyEnabled(%s) = %t, want %t", test.component, got, test.want)
			}
		})
	}
}

func TestValidateUpdateComponentDependencyCompatibility(t *testing.T) {
	invalidComponents := []ComponentConfig{
		{Name: ClusterAPIProviderOA, Enabled: true},
		{Name: ClusterAPIProviderKubeVirt, Enabled: true},
		{Name: ClusterAPI, Enabled: false},
		{Name: AssistedService, Enabled: true},
	}

	tests := []struct {
		name        string
		old         []ComponentConfig
		new         []ComponentConfig
		wantErr     bool
		wantErrMsg  string
		wantWarning string
	}{
		{
			name:        "Pre-existing invalid dependencies remain updateable",
			old:         invalidComponents,
			new:         invalidComponents,
			wantWarning: "cluster-api-provider-openshift-assisted requires cluster-api",
		},
		{
			name: "Unrelated component changes remain updateable",
			old:  invalidComponents,
			new: append([]ComponentConfig{}, append(invalidComponents, ComponentConfig{
				Name:    Discovery,
				Enabled: true,
			})...),
			wantWarning: "cluster-api-provider-openshift-assisted requires cluster-api",
		},
		{
			name: "Provider deployment overrides remain updateable",
			old:  invalidComponents,
			new: []ComponentConfig{
				{Name: ClusterAPIProviderOA, Enabled: true, ConfigOverrides: ConfigOverride{
					Deployments: []DeploymentConfig{{Name: "capoa-controlplane-controller-manager"}},
				}},
				{Name: ClusterAPIProviderKubeVirt, Enabled: true},
				{Name: ClusterAPI, Enabled: false},
				{Name: AssistedService, Enabled: true},
			},
			wantWarning: "cluster-api-provider-openshift-assisted requires cluster-api",
		},
		{
			name: "Defaulted disabled provider remains updateable",
			old: []ComponentConfig{
				{Name: ClusterAPIProviderOA, Enabled: true},
				{Name: ClusterAPI, Enabled: false},
				{Name: AssistedService, Enabled: true},
			},
			new: []ComponentConfig{
				{Name: ClusterAPIProviderOA, Enabled: true},
				{Name: ClusterAPIProviderKubeVirt, Enabled: false},
				{Name: ClusterAPI, Enabled: false},
				{Name: AssistedService, Enabled: true},
			},
			wantWarning: "cluster-api-provider-openshift-assisted requires cluster-api",
		},
		{
			name: "Disabled provider enablement changes are rejected",
			old: []ComponentConfig{
				{Name: ClusterAPIProviderOA, Enabled: true},
				{Name: ClusterAPIProviderKubeVirt, Enabled: false, ConfigOverrides: ConfigOverride{
					Deployments: []DeploymentConfig{{Name: "capk-controller-manager"}},
				}},
				{Name: ClusterAPI, Enabled: false},
				{Name: AssistedService, Enabled: true},
			},
			new: []ComponentConfig{
				{Name: ClusterAPIProviderOA, Enabled: true},
				{Name: ClusterAPIProviderKubeVirt, Enabled: true},
				{Name: ClusterAPI, Enabled: false},
				{Name: AssistedService, Enabled: true},
			},
			wantErr:    true,
			wantErrMsg: "cluster-api-provider-openshift-assisted requires cluster-api",
		},
		{
			name: "Changed invalid dependencies are rejected",
			old: []ComponentConfig{
				{Name: ClusterAPIProviderOA, Enabled: true},
				{Name: ClusterAPI, Enabled: false},
				{Name: AssistedService, Enabled: true},
			},
			new: []ComponentConfig{
				{Name: ClusterAPIProviderOA, Enabled: true},
				{Name: ClusterAPI, Enabled: false},
				{Name: AssistedService, Enabled: false},
			},
			wantErr:    true,
			wantErrMsg: "cluster-api-provider-openshift-assisted requires cluster-api",
		},
		{
			name: "New invalid dependencies are rejected",
			old: []ComponentConfig{
				{Name: ClusterAPIProviderOA, Enabled: true},
				{Name: ClusterAPI, Enabled: true},
				{Name: AssistedService, Enabled: true},
			},
			new: []ComponentConfig{
				{Name: ClusterAPIProviderOA, Enabled: true},
				{Name: ClusterAPI, Enabled: false},
				{Name: AssistedService, Enabled: true},
			},
			wantErr:    true,
			wantErrMsg: "cluster-api-provider-openshift-assisted requires cluster-api",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			oldObj := &MultiClusterEngine{Spec: MultiClusterEngineSpec{
				Overrides: &Overrides{Components: test.old},
			}}
			newObj := &MultiClusterEngine{Spec: MultiClusterEngineSpec{
				Overrides: &Overrides{Components: test.new},
			}}

			warnings, err := (&MultiClusterEngine{}).ValidateUpdate(context.Background(), oldObj, newObj)
			if !test.wantErr {
				if err != nil {
					t.Fatalf("ValidateUpdate() unexpected error = %v", err)
				}
				if test.wantWarning == "" {
					if len(warnings) != 0 {
						t.Fatalf("ValidateUpdate() unexpected warnings = %v", warnings)
					}
				} else if len(warnings) != 1 || !strings.Contains(warnings[0], test.wantWarning) {
					t.Fatalf("ValidateUpdate() warnings = %v, want one containing %q", warnings, test.wantWarning)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErrMsg) {
				t.Fatalf("ValidateUpdate() error = %v, want containing %q", err, test.wantErrMsg)
			}
		})
	}
}

func TestValidateUpdateCAPKWebhookCompatibility(t *testing.T) {
	testScheme := runtime.NewScheme()
	if err := AddToScheme(testScheme); err != nil {
		t.Fatal(err)
	}
	originalClient := Client
	Client = fake.NewClientBuilder().WithScheme(testScheme).Build()
	t.Cleanup(func() { Client = originalClient })

	tests := []struct {
		name       string
		ocpVersion string
		wantErr    string
	}{
		{name: "OpenShift 4.15 supports matchConditions", ocpVersion: "4.15.0"},
		{
			name:       "OpenShift 4.12 rejects newly enabled CAPK",
			ocpVersion: "4.12.0",
			wantErr:    "cluster-api-provider-kubevirt requires OpenShift 4.15.0 or later",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("ACM_HUB_OCP_VERSION", test.ocpVersion)
			oldObj := &MultiClusterEngine{Spec: MultiClusterEngineSpec{
				Overrides: &Overrides{Components: []ComponentConfig{
					{Name: ClusterAPIProviderKubeVirt, Enabled: false},
				}},
			}}
			newObj := &MultiClusterEngine{Spec: MultiClusterEngineSpec{
				Overrides: &Overrides{Components: []ComponentConfig{
					{Name: ClusterAPIProviderKubeVirt, Enabled: true},
				}},
			}}

			_, err := (&MultiClusterEngine{}).ValidateUpdate(context.Background(), oldObj, newObj)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateUpdate() unexpected error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("ValidateUpdate() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestValidateUpdateCAPKPreviewWebhookCompatibility(t *testing.T) {
	t.Setenv("ACM_HUB_OCP_VERSION", "4.12.0")
	oldObj := &MultiClusterEngine{}
	newObj := &MultiClusterEngine{Spec: MultiClusterEngineSpec{
		Overrides: &Overrides{Components: []ComponentConfig{
			{Name: ClusterAPIProviderKubeVirt, Enabled: false},
			{Name: ClusterAPIProviderKubeVirtPreview, Enabled: true},
		}},
	}}

	_, err := (&MultiClusterEngine{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err == nil || !strings.Contains(err.Error(), "cluster-api-provider-kubevirt requires OpenShift 4.15.0 or later") {
		t.Fatalf("ValidateUpdate() error = %v, want CAPK OpenShift compatibility error", err)
	}
}

func TestClusterOCPVersionTreatsNoMatchAsNoVersion(t *testing.T) {
	t.Setenv("ACM_HUB_OCP_VERSION", "")
	testScheme := runtime.NewScheme()
	if err := AddToScheme(testScheme); err != nil {
		t.Fatal(err)
	}

	originalClient := Client
	Client = fake.NewClientBuilder().
		WithScheme(testScheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(context.Context, cl.WithWatch, cl.ObjectKey, cl.Object, ...cl.GetOption) error {
				return &apimeta.NoKindMatchError{
					GroupKind: schema.GroupKind{
						Group: "config.openshift.io",
						Kind:  "ClusterVersion",
					},
				}
			},
		}).
		Build()
	t.Cleanup(func() { Client = originalClient })

	ocpVersion, err := clusterOCPVersion(context.Background())
	if err != nil {
		t.Fatalf("clusterOCPVersion() error = %v", err)
	}
	if ocpVersion != "" {
		t.Fatalf("clusterOCPVersion() = %q, want empty", ocpVersion)
	}
}

func TestValidateUpdateHostedProviderPreviewCompatibility(t *testing.T) {
	t.Setenv("ACM_HUB_OCP_VERSION", "4.15.0")
	hostedAnnotations := map[string]string{"deploymentmode": string(ModeHosted)}

	for _, provider := range []string{
		ClusterAPIProviderKubeVirtPreview,
		ClusterAPIProviderOAPreview,
	} {
		t.Run(provider, func(t *testing.T) {
			oldObj := &MultiClusterEngine{
				ObjectMeta: metav1.ObjectMeta{Annotations: hostedAnnotations},
			}
			newObj := &MultiClusterEngine{
				ObjectMeta: metav1.ObjectMeta{Annotations: hostedAnnotations},
				Spec: MultiClusterEngineSpec{
					Overrides: &Overrides{Components: []ComponentConfig{
						{Name: provider, Enabled: true},
					}},
				},
			}

			_, err := (&MultiClusterEngine{}).ValidateUpdate(context.Background(), oldObj, newObj)
			if err == nil || !strings.Contains(err.Error(), "cannot be enabled in hosted mode") {
				t.Fatalf("ValidateUpdate() error = %v, want hosted-mode error", err)
			}

			warnings, err := (&MultiClusterEngine{}).ValidateUpdate(context.Background(), newObj, newObj.DeepCopy())
			if err != nil {
				t.Fatalf("ValidateUpdate() existing preview error = %v", err)
			}
			if len(warnings) != 1 || !strings.Contains(warnings[0], "cannot be enabled in hosted mode") {
				t.Fatalf("ValidateUpdate() warnings = %v, want one hosted-mode warning", warnings)
			}
		})
	}
}

func TestValidateClusterAPIExclusivity(t *testing.T) {
	testScheme := runtime.NewScheme()
	if err := AddToScheme(testScheme); err != nil {
		t.Fatal(err)
	}

	existing := &MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "existing-hosted",
			UID:         types.UID("existing-hosted-uid"),
			Annotations: map[string]string{"deploymentmode": string(ModeHosted)},
		},
		Spec: MultiClusterEngineSpec{
			TargetNamespace: "existing-hosted",
			Overrides: &Overrides{Components: []ComponentConfig{
				{Name: ClusterAPI, Enabled: true},
			}},
		},
	}
	originalClient := Client
	Client = fake.NewClientBuilder().WithScheme(testScheme).WithObjects(existing).Build()
	t.Cleanup(func() { Client = originalClient })

	newHosted := func(name string) *MultiClusterEngine {
		return &MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Annotations: map[string]string{"deploymentmode": string(ModeHosted)},
			},
			Spec: MultiClusterEngineSpec{
				TargetNamespace: name,
				Overrides: &Overrides{Components: []ComponentConfig{
					{Name: ClusterAPI, Enabled: true},
				}},
			},
		}
	}

	err := validateClusterAPIExclusivity(context.Background(), nil, newHosted("new-hosted"))
	if err == nil || !strings.Contains(err.Error(), "only one MultiClusterEngine may enable cluster-api") {
		t.Fatalf("validateClusterAPIExclusivity() error = %v, want exclusivity error", err)
	}

	self := newHosted("existing-hosted")
	self.UID = existing.UID
	if err := validateClusterAPIExclusivity(context.Background(), existing, self); err != nil {
		t.Fatalf("validateClusterAPIExclusivity() self-update error = %v", err)
	}

	newPreviewOnly := &MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "new-preview-hosted",
			Annotations: map[string]string{"deploymentmode": string(ModeHosted)},
		},
		Spec: MultiClusterEngineSpec{
			TargetNamespace: "new-preview-hosted",
			Overrides: &Overrides{Components: []ComponentConfig{
				{Name: ClusterAPIPreview, Enabled: true},
			}},
		},
	}
	err = validateClusterAPIExclusivity(context.Background(), nil, newPreviewOnly)
	if err == nil || !strings.Contains(err.Error(), "only one MultiClusterEngine may enable cluster-api") {
		t.Fatalf("validateClusterAPIExclusivity() preview error = %v, want exclusivity error", err)
	}

	newPreviewWithDisabledStable := &MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "new-preview-with-disabled-stable",
			Annotations: map[string]string{"deploymentmode": string(ModeHosted)},
		},
		Spec: MultiClusterEngineSpec{
			TargetNamespace: "new-preview-with-disabled-stable",
			Overrides: &Overrides{Components: []ComponentConfig{
				{Name: ClusterAPI, Enabled: false},
				{Name: ClusterAPIPreview, Enabled: true},
			}},
		},
	}
	err = validateClusterAPIExclusivity(context.Background(), nil, newPreviewWithDisabledStable)
	if err == nil || !strings.Contains(err.Error(), "only one MultiClusterEngine may enable cluster-api") {
		t.Fatalf("validateClusterAPIExclusivity() preview with disabled stable error = %v, want exclusivity error", err)
	}
}

func TestValidateClusterAPIExclusivityAllowsExistingConflictUpdates(t *testing.T) {
	testScheme := runtime.NewScheme()
	if err := AddToScheme(testScheme); err != nil {
		t.Fatal(err)
	}

	existingOwner := &MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "existing-owner",
			UID:         types.UID("existing-owner-uid"),
			Annotations: map[string]string{"deploymentmode": string(ModeHosted)},
		},
		Spec: MultiClusterEngineSpec{
			TargetNamespace: "existing-owner",
			Overrides: &Overrides{Components: []ComponentConfig{
				{Name: ClusterAPI, Enabled: true},
			}},
		},
	}
	existingConflict := &MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "existing-conflict",
			UID:         types.UID("existing-conflict-uid"),
			Annotations: map[string]string{"deploymentmode": string(ModeHosted)},
		},
		Spec: MultiClusterEngineSpec{
			TargetNamespace:  "existing-conflict",
			LocalClusterName: "old-local-cluster",
			Overrides: &Overrides{Components: []ComponentConfig{
				{Name: ClusterAPI, Enabled: true},
			}},
		},
	}

	originalClient := Client
	Client = fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(existingOwner, existingConflict).
		Build()
	t.Cleanup(func() { Client = originalClient })

	updatedConflict := existingConflict.DeepCopy()
	updatedConflict.Spec.LocalClusterName = "new-local-cluster"
	if err := validateClusterAPIExclusivity(context.Background(), existingConflict, updatedConflict); err != nil {
		t.Fatalf("validateClusterAPIExclusivity() existing conflict update error = %v", err)
	}

	disabledConflict := existingConflict.DeepCopy()
	disabledConflict.Spec.Overrides.Components[0].Enabled = false
	reenabledConflict := disabledConflict.DeepCopy()
	reenabledConflict.Spec.Overrides.Components[0].Enabled = true
	err := validateClusterAPIExclusivity(context.Background(), disabledConflict, reenabledConflict)
	if err == nil || !strings.Contains(err.Error(), "only one MultiClusterEngine may enable cluster-api") {
		t.Fatalf("validateClusterAPIExclusivity() re-enable error = %v, want exclusivity error", err)
	}
}

var _ = Describe("Multiclusterengine webhook", func() {

	Context("Creating a Multiclusterengine", func() {
		It("Should successfully create multiclusterengine", func() {
			By("by creating a new standalone Multiclusterengine resource", func() {
				mce := &MultiClusterEngine{
					ObjectMeta: metav1.ObjectMeta{
						Name: multiClusterEngineName,
					},
					Spec: MultiClusterEngineSpec{
						LocalClusterName: "test-local-cluster",
						TargetNamespace:  DefaultTargetNamespace,
					},
				}
				Expect(k8sClient.Create(ctx, mce)).Should(Succeed())
			})
			By("by creating a new hosted Multiclusterengine resource", func() {
				mce := &MultiClusterEngine{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "hosted-mce",
						Annotations: map[string]string{"deploymentmode": string(ModeHosted)},
					},
					Spec: MultiClusterEngineSpec{
						TargetNamespace: "hostedNS",
					},
				}
				Expect(k8sClient.Create(ctx, mce)).Should(Succeed())
			})
		})

		It("Should fail to create multiclusterengine", func() {
			By("because of TargetNamespace", func() {
				mce := &MultiClusterEngine{
					ObjectMeta: metav1.ObjectMeta{
						Name:        fmt.Sprintf("%s-2", multiClusterEngineName),
						Annotations: map[string]string{"deploymentmode": string(ModeHosted)},
					},
					Spec: MultiClusterEngineSpec{
						TargetNamespace: DefaultTargetNamespace,
					},
				}
				Expect(k8sClient.Create(ctx, mce)).NotTo(BeNil(), "Only one MCE can target a namespace")
			})
			By("because of DeploymentMode", func() {
				mce := &MultiClusterEngine{
					ObjectMeta: metav1.ObjectMeta{
						Name: fmt.Sprintf("%s-2", multiClusterEngineName),
					},
					Spec: MultiClusterEngineSpec{
						TargetNamespace: "new",
					},
				}
				Expect(k8sClient.Create(ctx, mce)).NotTo(BeNil(), "Only one MCE in standalone mode allowed")
			})
			By("because of invalid AvailabilityConfig", func() {
				mce := &MultiClusterEngine{
					ObjectMeta: metav1.ObjectMeta{
						Name:        fmt.Sprintf("%s-2", multiClusterEngineName),
						Annotations: map[string]string{"deploymentmode": string(ModeHosted)},
					},
					Spec: MultiClusterEngineSpec{
						TargetNamespace:    "new",
						AvailabilityConfig: "low",
					},
				}
				Expect(k8sClient.Create(ctx, mce)).NotTo(BeNil(), "Invalid availability config is not allowed")
			})
			By("because of component configuration", func() {
				mce := &MultiClusterEngine{
					ObjectMeta: metav1.ObjectMeta{
						Name:        fmt.Sprintf("%s-2", multiClusterEngineName),
						Annotations: map[string]string{"deploymentmode": string(ModeHosted)},
					},
					Spec: MultiClusterEngineSpec{
						TargetNamespace: "new",
						Overrides: &Overrides{
							Components: []ComponentConfig{
								{
									Name:    "fake-component",
									Enabled: true,
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, mce)).NotTo(BeNil(), "Invalid components not allowed in config")
			})
			By("because of a removed component", func() {
				mce := &MultiClusterEngine{
					ObjectMeta: metav1.ObjectMeta{
						Name:        fmt.Sprintf("%s-2", multiClusterEngineName),
						Annotations: map[string]string{"deploymentmode": string(ModeHosted)},
					},
					Spec: MultiClusterEngineSpec{
						TargetNamespace: "new",
						Overrides: &Overrides{
							Components: []ComponentConfig{
								{
									Name:    MaestroPreview,
									Enabled: true,
								},
							},
						},
					},
				}
				err := k8sClient.Create(ctx, mce)
				Expect(err).NotTo(BeNil(), "Removed components should not be permitted on new resources")
				Expect(err.Error()).To(ContainSubstring("was removed"))
			})
		})

		It("Should fail to update multiclusterengine", func() {
			mce := &MultiClusterEngine{}

			By("because of TargetNamespace", func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: multiClusterEngineName}, mce)).To(Succeed())
				mce.Spec.TargetNamespace = "new"
				Expect(k8sClient.Update(ctx, mce)).NotTo(BeNil(), "Target namespace should not change")
			})
			By("because of DeploymentMode", func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: multiClusterEngineName}, mce)).To(Succeed())
				mce.SetAnnotations(map[string]string{"deploymentmode": string(ModeHosted)})
				Expect(k8sClient.Update(ctx, mce)).NotTo(BeNil(), "DeploymentMode should not change")
			})

			By("because of invalid component", func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: multiClusterEngineName}, mce)).To(Succeed())
				mce.Spec.Overrides = &Overrides{
					Components: []ComponentConfig{
						{
							Name:    "fake-component",
							Enabled: true,
						},
					},
				}
				Expect(k8sClient.Update(ctx, mce)).NotTo(BeNil(), "invalid components should not be permitted")
			})

			By("removing invalid component", func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: multiClusterEngineName}, mce)).To(Succeed())
				mce.Spec.Overrides = &Overrides{}
				Expect(k8sClient.Update(ctx, mce)).To(Succeed())
			})

			By("because of existing local-cluster resource", func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: multiClusterEngineName}, mce)).To(Succeed())
				managedCluster := NewManagedCluster(mce.Spec.LocalClusterName)
				Expect(k8sClient.Create(ctx, managedCluster)).To(Succeed())

				mce.Spec.LocalClusterName = "updated-local-cluster"
				Expect(k8sClient.Update(ctx, mce)).NotTo(BeNil(), "updating local-Cluster name while one exists should not be permitted")
			})

			By("because the local-cluster name must be less than 35 characters long", func() {
				mce.Spec.LocalClusterName = strings.Repeat("t", 35)
				err := k8sClient.Update(ctx, mce)
				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(&k8serrors.StatusError{}))

				statusErr := err.(*k8serrors.StatusError)
				Expect(statusErr.ErrStatus.Code).To(Equal(int32(403)))
				Expect(statusErr.ErrStatus.Message).To(ContainSubstring("local-cluster name"))
				Expect(statusErr.ErrStatus.Message).To(ContainSubstring("35 characters"))
			})
		})

		It("Should allow updating multiclusterengine with a removed component", func() {
			mce := &MultiClusterEngine{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: multiClusterEngineName}, mce)).To(Succeed())

			By("setting a removed component, simulating an upgrade from a version where it still existed", func() {
				mce.Spec.Overrides = &Overrides{
					Components: []ComponentConfig{
						{
							Name:    MaestroPreview,
							Enabled: true,
						},
					},
				}
				Expect(k8sClient.Update(ctx, mce)).To(Succeed(),
					"removed components should be accepted on update so they can be auto-pruned by the operator")
			})

			By("cleaning up the removed component", func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: multiClusterEngineName}, mce)).To(Succeed())
				mce.Spec.Overrides = &Overrides{}
				Expect(k8sClient.Update(ctx, mce)).To(Succeed())
			})
		})

		It("Should fail to delete multiclusterengine when AgentServiceConfig exists", func() {
			mce := &MultiClusterEngine{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: multiClusterEngineName}, mce)).To(Succeed())

			By("Creating an AgentServiceConfig resource", func() {
				agentServiceConfig := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "agent-install.openshift.io/v1beta1",
						"kind":       "AgentServiceConfig",
						"metadata": map[string]interface{}{
							"name": "test-agent-service-config",
						},
						"spec": map[string]interface{}{
							"databaseStorage": map[string]interface{}{
								"storageClassName": "test-storage",
							},
							"filesystemStorage": map[string]interface{}{
								"storageClassName": "test-storage",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentServiceConfig)).To(Succeed())
			})

			By("Attempting to delete MCE should fail", func() {
				err := k8sClient.Delete(ctx, mce)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("Existing AgentServiceConfig resources must first be deleted"))
			})

			By("Cleaning up AgentServiceConfig", func() {
				agentServiceConfig := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "agent-install.openshift.io/v1beta1",
						"kind":       "AgentServiceConfig",
						"metadata": map[string]interface{}{
							"name": "test-agent-service-config",
						},
					},
				}
				Expect(k8sClient.Delete(ctx, agentServiceConfig)).To(Succeed())
			})
		})

		It("Should fail to delete multiclusterengine when ClusterPool exists", func() {
			mce := &MultiClusterEngine{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: multiClusterEngineName}, mce)).To(Succeed())

			By("Creating a ClusterPool resource", func() {
				clusterPool := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "hive.openshift.io/v1",
						"kind":       "ClusterPool",
						"metadata": map[string]interface{}{
							"name":      "test-pool",
							"namespace": "default",
						},
						"spec": map[string]interface{}{
							"size":       1,
							"baseDomain": "test.example.com",
							"imageSetRef": map[string]interface{}{
								"name": "test-imageset",
							},
							"platform": map[string]interface{}{
								"aws": map[string]interface{}{
									"region": "us-east-1",
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, clusterPool)).To(Succeed())
			})

			By("Attempting to delete MCE should fail", func() {
				err := k8sClient.Delete(ctx, mce)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("Existing ClusterPool resources must first be deleted"))
			})

			By("Cleaning up ClusterPool", func() {
				clusterPool := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "hive.openshift.io/v1",
						"kind":       "ClusterPool",
						"metadata": map[string]interface{}{
							"name":      "test-pool",
							"namespace": "default",
						},
					},
				}
				Expect(k8sClient.Delete(ctx, clusterPool)).To(Succeed())
			})
		})

		It("Should fail to delete multiclusterengine when DiscoveryConfig exists", func() {
			mce := &MultiClusterEngine{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: multiClusterEngineName}, mce)).To(Succeed())

			By("Creating a DiscoveryConfig resource", func() {
				discoveryConfig := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "discovery.open-cluster-management.io/v1",
						"kind":       "DiscoveryConfig",
						"metadata": map[string]interface{}{
							"name":      "test-discovery",
							"namespace": "default",
						},
						"spec": map[string]interface{}{
							"credential": "test-credential",
						},
					},
				}
				Expect(k8sClient.Create(ctx, discoveryConfig)).To(Succeed())
			})

			By("Attempting to delete MCE should fail", func() {
				err := k8sClient.Delete(ctx, mce)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("Existing DiscoveryConfig resources must first be deleted"))
			})

			By("Cleaning up DiscoveryConfig", func() {
				discoveryConfig := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "discovery.open-cluster-management.io/v1",
						"kind":       "DiscoveryConfig",
						"metadata": map[string]interface{}{
							"name":      "test-discovery",
							"namespace": "default",
						},
					},
				}
				Expect(k8sClient.Delete(ctx, discoveryConfig)).To(Succeed())
			})
		})

		It("Should succeed in deleting multiclusterengine", func() {
			mce := &MultiClusterEngine{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: multiClusterEngineName}, mce)).To(Succeed())

			managedCluster := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "cluster.open-cluster-management.io/v1",
					"kind":       "ManagedCluster",
					"metadata": map[string]interface{}{
						"name": mce.Spec.LocalClusterName,
					},
				},
			}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mce.Spec.LocalClusterName}, managedCluster)).To(Succeed())

			By("Deleting the multiclusterengine", func() {
				Expect(k8sClient.Delete(ctx, mce)).To(Succeed())
			})
		})

	})

})

// re-defining the function here to avoid a import cycle
func NewManagedCluster(name string) *unstructured.Unstructured {
	managedCluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": name,
				"labels": map[string]interface{}{
					"local-cluster":                 "true",
					"cloud":                         "auto-detect",
					"vendor":                        "auto-detect",
					"velero.io/exclude-from-backup": "true",
				},
			},
			"spec": map[string]interface{}{
				"hubAcceptsClient": true,
			},
		},
	}
	return managedCluster
}
