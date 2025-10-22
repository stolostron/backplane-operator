// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"os"
	"reflect"
	"testing"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_deduplicate(t *testing.T) {
	tests := []struct {
		name string
		have []backplanev1.ComponentConfig
		want []backplanev1.ComponentConfig
	}{
		{
			name: "unique components",
			have: []backplanev1.ComponentConfig{
				{Name: "component1", Enabled: true},
				{Name: "component2", Enabled: true},
			},
			want: []backplanev1.ComponentConfig{
				{Name: "component1", Enabled: true},
				{Name: "component2", Enabled: true},
			},
		},
		{
			name: "duplicate components",
			have: []backplanev1.ComponentConfig{
				{Name: "component1", Enabled: false},
				{Name: "component2", Enabled: true},
				{Name: "component1", Enabled: true},
			},
			want: []backplanev1.ComponentConfig{
				{Name: "component1", Enabled: true},
				{Name: "component2", Enabled: true},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deduplicate(tt.have); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("deduplicate() = %v, want %v", got, tt.want)
			}
		})
	}
	m := &backplanev1.MultiClusterEngine{}
	yes := SetDefaultComponents(m)
	if !yes {
		t.Error("Setting default did not work")
	}

	yes = DeduplicateComponents(m)
	if yes {
		t.Error("Unexpected duplicates")
	}

	os.Setenv("NO_PROXY", "test")
	yes = ProxyEnvVarsAreSet()
	if !yes {
		t.Error("Unexpected proxy failure")
	}
	os.Unsetenv("NO_PROXY")
	yes = ProxyEnvVarsAreSet()
	if yes {
		t.Error("Unexpected proxy success")
	}

	var sample backplanev1.AvailabilityType
	sample = backplanev1.HAHigh

	yes = AvailabilityConfigIsValid(sample)
	if !yes {
		t.Error("Unexpected availabilitty config failure")
	}

	sample = "test"
	yes = AvailabilityConfigIsValid(sample)
	if yes {
		t.Error("Unexpected availabilitty config successs")
	}

	stringList := []string{"test1", "test2"}
	stringRemoveList := []string{"test2"}

	yes = Contains(stringList, "test1")
	if !yes {
		t.Error("Contains did not work")
	}
	attemptedRemove := Remove(stringList, "test1")
	if len(attemptedRemove) != len(stringRemoveList) {
		t.Error("Removes did not work")
	}
}

func TestGetHubType(t *testing.T) {
	tests := []struct {
		name string
		env  string
		mce  *backplanev1.MultiClusterEngine
		want string
	}{
		{
			name: "mce",
			env:  "multicluster-engine",
			mce: &backplanev1.MultiClusterEngine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mce",
				},
			},
			want: "mce",
		},
		{
			name: "acm",
			env:  "multicluster-engine",
			mce: &backplanev1.MultiClusterEngine{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "mce",
					Labels: map[string]string{"multiclusterhubs.operator.open-cluster-management.io/managed-by": "true"},
				},
			},
			want: "acm",
		},
		{
			name: "stolostron-engine",
			env:  "stolostron-engine",
			mce: &backplanev1.MultiClusterEngine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mce",
				},
			},
			want: "stolostron-engine",
		},
		{
			name: "stolostron",
			env:  "stolostron-engine",
			mce: &backplanev1.MultiClusterEngine{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "mce",
					Labels: map[string]string{"multiclusterhubs.operator.open-cluster-management.io/managed-by": "true"},
				},
			},
			want: "stolostron",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OPERATOR_PACKAGE", tt.env)
			if got := GetHubType(tt.mce); got != tt.want {
				t.Errorf("GetHubType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_GetTestImages(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{
			name: "should return correct number of test images",
			want: 80,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			images := GetTestImages()
			got := len(images)

			if got != tt.want {
				t.Errorf("GetTestImages() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_DefaultReplicaCount(t *testing.T) {
	tests := []struct {
		name string
		mce  *backplanev1.MultiClusterEngine
		want int
	}{
		{
			name: "should get default replica count for HABasic",
			mce: &backplanev1.MultiClusterEngine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "engine",
				},
				Spec: backplanev1.MultiClusterEngineSpec{
					AvailabilityConfig: backplanev1.HABasic,
				},
			},
			want: 1,
		},
		{
			name: "should get default replica count for HAHigh",
			mce: &backplanev1.MultiClusterEngine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "engine",
				},
				Spec: backplanev1.MultiClusterEngineSpec{
					AvailabilityConfig: backplanev1.HAHigh,
				},
			},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DefaultReplicaCount(tt.mce); got != tt.want {
				t.Errorf("DefaultReplicaCount(tt.mce) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsEUSUpgrading(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion string
		desiredVersion string
		want           bool
	}{
		{
			name:           "EUS upgrade - Y values differ by 2",
			currentVersion: "1.2.3",
			desiredVersion: "1.4.5",
			want:           true,
		},
		{
			name:           "Non-EUS upgrade - Y values differ by 1",
			currentVersion: "1.2.3",
			desiredVersion: "1.3.4",
			want:           false,
		},
		{
			name:           "Non-EUS upgrade - Y values differ by 3",
			currentVersion: "1.2.3",
			desiredVersion: "1.5.4",
			want:           false,
		},
		{
			name:           "Same version",
			currentVersion: "1.2.3",
			desiredVersion: "1.2.3",
			want:           false,
		},
		{
			name:           "Downgrade - Y values differ by -2",
			currentVersion: "1.4.3",
			desiredVersion: "1.2.5",
			want:           false,
		},
		{
			name:           "Current version is blank",
			currentVersion: "",
			desiredVersion: "1.4.5",
			want:           false,
		},
		{
			name:           "Non-numeric Y version in current",
			currentVersion: "1.x.3",
			desiredVersion: "1.4.5",
			want:           false,
		},
		{
			name:           "Non-numeric Y version in desired",
			currentVersion: "1.2.3",
			desiredVersion: "1.y.5",
			want:           false,
		},
		{
			name:           "Different X versions",
			currentVersion: "1.2.3",
			desiredVersion: "2.4.5",
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mce := &backplanev1.MultiClusterEngine{
				Status: backplanev1.MultiClusterEngineStatus{
					CurrentVersion: tt.currentVersion,
					DesiredVersion: tt.desiredVersion,
				},
			}
			if got := IsEUSUpgrading(mce); got != tt.want {
				t.Errorf("IsEUSUpgrading() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestComponentToCRDDirectory(t *testing.T) {
	tests := []struct {
		name        string
		deployOnOCP bool
		assertions  []struct {
			component       string
			expectedCRDDir  string
			description     string
		}
	}{
		{
			name:        "OCP deployment - uses OCP CRD directories",
			deployOnOCP: true,
			assertions: []struct {
				component       string
				expectedCRDDir  string
				description     string
			}{
				{
					component:      backplanev1.ClusterAPI,
					expectedCRDDir: backplanev1.ClusterAPICRDDir,
					description:    "ClusterAPI should use OCP CRD directory",
				},
				{
					component:      backplanev1.ClusterAPIPreview,
					expectedCRDDir: backplanev1.ClusterAPICRDDir,
					description:    "ClusterAPIPreview should use OCP CRD directory",
				},
				{
					component:      backplanev1.ClusterAPIProviderOA,
					expectedCRDDir: backplanev1.ClusterAPIProviderOACRDDir,
					description:    "ClusterAPIProviderOA should use OCP CRD directory",
				},
				{
					component:      backplanev1.ClusterAPIProviderOAPreview,
					expectedCRDDir: backplanev1.ClusterAPIProviderOACRDDir,
					description:    "ClusterAPIProviderOAPreview should use OCP CRD directory",
				},
				{
					component:      backplanev1.ClusterAPIProviderMetal,
					expectedCRDDir: backplanev1.ClusterAPIProviderMetalCRDDir,
					description:    "ClusterAPIProviderMetal should use OCP CRD directory",
				},
				{
					component:      backplanev1.ClusterAPIProviderMetalPreview,
					expectedCRDDir: backplanev1.ClusterAPIProviderMetalCRDDir,
					description:    "ClusterAPIProviderMetalPreview should use OCP CRD directory",
				},
				{
					component:      backplanev1.AssistedService,
					expectedCRDDir: backplanev1.AssistedServiceCRDDir,
					description:    "AssistedService should use correct CRD directory",
				},
				{
					component:      backplanev1.ClusterLifecycle,
					expectedCRDDir: backplanev1.ClusterLifecycleCRDDir,
					description:    "ClusterLifecycle should use correct CRD directory",
				},
				{
					component:      backplanev1.ClusterManager,
					expectedCRDDir: backplanev1.ClusterManagerCRDDir,
					description:    "ClusterManager should use correct CRD directory",
				},
			},
		},
		{
			name:        "Non-OCP deployment - uses K8S CRD directories",
			deployOnOCP: false,
			assertions: []struct {
				component       string
				expectedCRDDir  string
				description     string
			}{
				{
					component:      backplanev1.ClusterAPI,
					expectedCRDDir: backplanev1.ClusterAPIK8SCRDDir,
					description:    "ClusterAPI should use K8S CRD directory on non-OCP",
				},
				{
					component:      backplanev1.ClusterAPIPreview,
					expectedCRDDir: backplanev1.ClusterAPIK8SCRDDir,
					description:    "ClusterAPIPreview should use K8S CRD directory on non-OCP",
				},
				{
					component:      backplanev1.ClusterAPIProviderOA,
					expectedCRDDir: backplanev1.ClusterAPIProviderOAK8SCRDDir,
					description:    "ClusterAPIProviderOA should use K8S CRD directory on non-OCP",
				},
				{
					component:      backplanev1.ClusterAPIProviderOAPreview,
					expectedCRDDir: backplanev1.ClusterAPIProviderOAK8SCRDDir,
					description:    "ClusterAPIProviderOAPreview should use K8S CRD directory on non-OCP",
				},
				{
					component:      backplanev1.ClusterAPIProviderMetal,
					expectedCRDDir: backplanev1.ClusterAPIProviderMetalK8SCRDDir,
					description:    "ClusterAPIProviderMetal should use K8S CRD directory on non-OCP",
				},
				{
					component:      backplanev1.ClusterAPIProviderMetalPreview,
					expectedCRDDir: backplanev1.ClusterAPIProviderMetalK8SCRDDir,
					description:    "ClusterAPIProviderMetalPreview should use K8S CRD directory on non-OCP",
				},
				{
					component:      backplanev1.AssistedService,
					expectedCRDDir: backplanev1.AssistedServiceCRDDir,
					description:    "AssistedService should use correct CRD directory",
				},
				{
					component:      backplanev1.ClusterLifecycle,
					expectedCRDDir: backplanev1.ClusterLifecycleCRDDir,
					description:    "ClusterLifecycle should use correct CRD directory",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the deployment type
			originalDeployOnOCP := GlobalDeployOnOCP
			SetDeployOnOCP(tt.deployOnOCP)
			defer SetDeployOnOCP(originalDeployOnOCP) // Restore original value after test

			// Get the component to CRD directory mapping
			result := ComponentToCRDDirectory()

			// Verify the result is not nil
			if result == nil {
				t.Fatal("ComponentToCRDDirectory() returned nil")
			}

			// Run all assertions for this test case
			for _, assertion := range tt.assertions {
				if got, ok := result[assertion.component]; !ok {
					t.Errorf("%s: component %s not found in result map", assertion.description, assertion.component)
				} else if got != assertion.expectedCRDDir {
					t.Errorf("%s: got %v, want %v", assertion.description, got, assertion.expectedCRDDir)
				}
			}
		})
	}
}

func TestComponentToCRDDirectory_AllComponents(t *testing.T) {
	// Store original value
	originalDeployOnOCP := GlobalDeployOnOCP
	defer SetDeployOnOCP(originalDeployOnOCP)

	// Test on OCP
	SetDeployOnOCP(true)
	ocpResult := ComponentToCRDDirectory()

	// Verify all expected components are present in OCP mode
	expectedComponents := []string{
		backplanev1.AssistedService,
		backplanev1.ClusterAPI,
		backplanev1.ClusterAPIPreview,
		backplanev1.ClusterAPIProviderAWS,
		backplanev1.ClusterAPIProviderAWSPreview,
		backplanev1.ClusterAPIProviderMetal,
		backplanev1.ClusterAPIProviderMetalPreview,
		backplanev1.ClusterAPIProviderOA,
		backplanev1.ClusterAPIProviderOAPreview,
		backplanev1.ClusterLifecycle,
		backplanev1.ClusterManager,
		backplanev1.ClusterProxyAddon,
		backplanev1.Discovery,
		backplanev1.Hive,
		backplanev1.ImageBasedInstallOperator,
		backplanev1.ImageBasedInstallOperatorPreview,
		backplanev1.ManagedServiceAccount,
		backplanev1.ManagedServiceAccountPreview,
		backplanev1.ServerFoundation,
	}

	for _, component := range expectedComponents {
		if _, ok := ocpResult[component]; !ok {
			t.Errorf("Component %s not found in OCP result map", component)
		}
	}

	// Test on non-OCP (K8S)
	SetDeployOnOCP(false)
	k8sResult := ComponentToCRDDirectory()

	// Verify all expected components are present in K8S mode
	for _, component := range expectedComponents {
		if _, ok := k8sResult[component]; !ok {
			t.Errorf("Component %s not found in K8S result map", component)
		}
	}

	// Verify that ClusterAPI components use different directories on OCP vs K8S
	if ocpResult[backplanev1.ClusterAPI] == k8sResult[backplanev1.ClusterAPI] {
		t.Error("ClusterAPI should use different CRD directories on OCP vs K8S")
	}

	if ocpResult[backplanev1.ClusterAPIProviderOA] == k8sResult[backplanev1.ClusterAPIProviderOA] {
		t.Error("ClusterAPIProviderOA should use different CRD directories on OCP vs K8S")
	}

	if ocpResult[backplanev1.ClusterAPIProviderMetal] == k8sResult[backplanev1.ClusterAPIProviderMetal] {
		t.Error("ClusterAPIProviderMetal should use different CRD directories on OCP vs K8S")
	}

	// Verify that non-ClusterAPI components use the same directories on both platforms
	if ocpResult[backplanev1.AssistedService] != k8sResult[backplanev1.AssistedService] {
		t.Error("AssistedService should use the same CRD directory on OCP and K8S")
	}

	if ocpResult[backplanev1.ClusterAPIProviderAWS] != k8sResult[backplanev1.ClusterAPIProviderAWS] {
		t.Error("ClusterAPIProviderAWS should use the same CRD directory on OCP and K8S")
	}
}
