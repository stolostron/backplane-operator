// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"os"
	"reflect"
	"strings"
	"testing"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
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

func TestComponentCRDDirectories(t *testing.T) {
	tests := []struct {
		name         string
		component    string
		expectedDirs []string
		description  string
	}{
		// CAPI components with platform variants - should return both OCP and K8s
		{
			name:      "ClusterAPI returns both variants",
			component: backplanev1.ClusterAPI,
			expectedDirs: []string{
				backplanev1.ClusterAPICRDDir,
				backplanev1.ClusterAPIK8SCRDDir,
			},
			description: "ClusterAPI should return both OCP and K8s CRD directories",
		},
		{
			name:      "ClusterAPIPreview returns both variants",
			component: backplanev1.ClusterAPIPreview,
			expectedDirs: []string{
				backplanev1.ClusterAPICRDDir,
				backplanev1.ClusterAPIK8SCRDDir,
			},
			description: "ClusterAPIPreview should return both OCP and K8s CRD directories",
		},
		{
			name:      "ClusterAPIProviderMetal returns both variants",
			component: backplanev1.ClusterAPIProviderMetal,
			expectedDirs: []string{
				backplanev1.ClusterAPIProviderMetalCRDDir,
				backplanev1.ClusterAPIProviderMetalK8SCRDDir,
			},
			description: "ClusterAPIProviderMetal should return both OCP and K8s CRD directories",
		},
		{
			name:      "ClusterAPIProviderMetalPreview returns both variants",
			component: backplanev1.ClusterAPIProviderMetalPreview,
			expectedDirs: []string{
				backplanev1.ClusterAPIProviderMetalCRDDir,
				backplanev1.ClusterAPIProviderMetalK8SCRDDir,
			},
			description: "ClusterAPIProviderMetalPreview should return both OCP and K8s CRD directories",
		},
		{
			name:      "ClusterAPIProviderOA returns both variants",
			component: backplanev1.ClusterAPIProviderOA,
			expectedDirs: []string{
				backplanev1.ClusterAPIProviderOACRDDir,
				backplanev1.ClusterAPIProviderOAK8SCRDDir,
			},
			description: "ClusterAPIProviderOA should return both OCP and K8s CRD directories",
		},
		{
			name:      "ClusterAPIProviderOAPreview returns both variants",
			component: backplanev1.ClusterAPIProviderOAPreview,
			expectedDirs: []string{
				backplanev1.ClusterAPIProviderOACRDDir,
				backplanev1.ClusterAPIProviderOAK8SCRDDir,
			},
			description: "ClusterAPIProviderOAPreview should return both OCP and K8s CRD directories",
		},

		// Non-CAPI components - should return single directory
		{
			name:      "AssistedService returns single directory",
			component: backplanev1.AssistedService,
			expectedDirs: []string{
				backplanev1.AssistedServiceCRDDir,
			},
			description: "AssistedService should return single CRD directory",
		},
		{
			name:      "ClusterAPIProviderAWS returns single directory",
			component: backplanev1.ClusterAPIProviderAWS,
			expectedDirs: []string{
				backplanev1.ClusterAPIProviderAWSCRDDir,
			},
			description: "ClusterAPIProviderAWS should return single CRD directory",
		},
		{
			name:      "ClusterLifecycle returns single directory",
			component: backplanev1.ClusterLifecycle,
			expectedDirs: []string{
				backplanev1.ClusterLifecycleCRDDir,
			},
			description: "ClusterLifecycle should return single CRD directory",
		},
		{
			name:      "ClusterManager returns single directory",
			component: backplanev1.ClusterManager,
			expectedDirs: []string{
				backplanev1.ClusterManagerCRDDir,
			},
			description: "ClusterManager should return single CRD directory",
		},
		{
			name:         "Unknown component returns empty slice",
			component:    "unknown-component",
			expectedDirs: []string{},
			description:  "Unknown component should return empty slice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComponentCRDDirectories(tt.component)

			// Check if the length matches
			if len(result) != len(tt.expectedDirs) {
				t.Errorf("%s: got %d directories, want %d", tt.description, len(result), len(tt.expectedDirs))
				t.Errorf("  got: %v", result)
				t.Errorf("  want: %v", tt.expectedDirs)
				return
			}

			// Check if all expected directories are present in order
			for i, expected := range tt.expectedDirs {
				if result[i] != expected {
					t.Errorf("%s: directory[%d] = %v, want %v", tt.description, i, result[i], expected)
				}
			}
		})
	}
}

func TestComponentCRDDirectories_AllComponents(t *testing.T) {
	// Verify all expected components return at least one directory
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
		result := ComponentCRDDirectories(component)
		if len(result) == 0 {
			t.Errorf("Component %s returned no CRD directories", component)
		}
	}

	// Verify that CAPI components return exactly 2 directories (OCP and K8s variants)
	capiComponents := []string{
		backplanev1.ClusterAPI,
		backplanev1.ClusterAPIPreview,
		backplanev1.ClusterAPIProviderMetal,
		backplanev1.ClusterAPIProviderMetalPreview,
		backplanev1.ClusterAPIProviderOA,
		backplanev1.ClusterAPIProviderOAPreview,
	}

	for _, component := range capiComponents {
		result := ComponentCRDDirectories(component)
		if len(result) != 2 {
			t.Errorf("CAPI component %s should return 2 directories, got %d: %v", component, len(result), result)
		}
	}

	// Verify that non-CAPI components return exactly 1 directory
	nonCapiComponents := []string{
		backplanev1.AssistedService,
		backplanev1.ClusterAPIProviderAWS,
		backplanev1.ClusterAPIProviderAWSPreview,
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

	for _, component := range nonCapiComponents {
		result := ComponentCRDDirectories(component)
		if len(result) != 1 {
			t.Errorf("Non-CAPI component %s should return 1 directory, got %d: %v", component, len(result), result)
		}
	}
}

func TestDumpServingCertSecret_Validation(t *testing.T) {
	namespace := "test-namespace"

	// Valid PEM certificate and key for successful test cases
	validCert := []byte(`-----BEGIN CERTIFICATE-----
MIICljCCAX4CCQCKz8Vz1V9Z8jANBgkqhkiG9w0BAQsFADANMQswCQYDVQQGEwJV
UzAeFw0yNDAxMDEwMDAwMDBaFw0yNTAxMDEwMDAwMDBaMA0xCzAJBgNVBAYTAlVT
-----END CERTIFICATE-----`)

	validKey := []byte(`-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC7VJTUt9Us8cKj
MzEfYyjiWA4R4/M2bS1+fWIcPm15j9m26wEqEPwYPVmT2rM0eBWaNcl0qMTktQKm
-----END PRIVATE KEY-----`)

	tests := []struct {
		name        string
		secret      *corev1.Secret
		wantErr     bool
		expectedErr string
	}{
		{
			name: "valid secret with tls.crt and tls.key",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-engine-operator-webhook",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"tls.crt": validCert,
					"tls.key": validKey,
				},
			},
			wantErr: false,
		},
		{
			name: "missing tls.crt key",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-engine-operator-webhook",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"tls.key": validKey,
				},
			},
			wantErr:     true,
			expectedErr: "secret multicluster-engine-operator-webhook is missing required keys (tls.crt or tls.key)",
		},
		{
			name: "missing tls.key key",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-engine-operator-webhook",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"tls.crt": validCert,
				},
			},
			wantErr:     true,
			expectedErr: "secret multicluster-engine-operator-webhook is missing required keys (tls.crt or tls.key)",
		},
		{
			name: "both keys missing",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-engine-operator-webhook",
					Namespace: namespace,
				},
				Data: map[string][]byte{},
			},
			wantErr:     true,
			expectedErr: "secret multicluster-engine-operator-webhook is missing required keys (tls.crt or tls.key)",
		},
		{
			name: "empty tls.crt data",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-engine-operator-webhook",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"tls.crt": []byte(""),
					"tls.key": validKey,
				},
			},
			wantErr:     true,
			expectedErr: "secret multicluster-engine-operator-webhook contains empty certificate or key data",
		},
		{
			name: "empty tls.key data",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-engine-operator-webhook",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"tls.crt": validCert,
					"tls.key": []byte(""),
				},
			},
			wantErr:     true,
			expectedErr: "secret multicluster-engine-operator-webhook contains empty certificate or key data",
		},
		{
			name: "both empty",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-engine-operator-webhook",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"tls.crt": []byte(""),
					"tls.key": []byte(""),
				},
			},
			wantErr:     true,
			expectedErr: "secret multicluster-engine-operator-webhook contains empty certificate or key data",
		},
		{
			name: "invalid PEM certificate data - missing BEGIN CERTIFICATE",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-engine-operator-webhook",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"tls.crt": []byte("invalid certificate data"),
					"tls.key": validKey,
				},
			},
			wantErr:     true,
			expectedErr: "tls.crt does not contain valid PEM certificate data",
		},
		{
			name: "invalid PEM certificate data - corrupted PEM",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-engine-operator-webhook",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"tls.crt": []byte("-----END CERTIFICATE-----"),
					"tls.key": validKey,
				},
			},
			wantErr:     true,
			expectedErr: "tls.crt does not contain valid PEM certificate data",
		},
		{
			name: "invalid PEM private key data - missing BEGIN",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-engine-operator-webhook",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"tls.crt": validCert,
					"tls.key": []byte("PRIVATE KEY-----\nsome data\n-----END PRIVATE KEY-----"),
				},
			},
			wantErr:     true,
			expectedErr: "tls.key does not contain valid PEM private key data",
		},
		{
			name: "invalid PEM private key data - missing PRIVATE KEY",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-engine-operator-webhook",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"tls.crt": validCert,
					"tls.key": []byte("-----BEGIN SOMETHING-----\nsome data\n-----END SOMETHING-----"),
				},
			},
			wantErr:     true,
			expectedErr: "tls.key does not contain valid PEM private key data",
		},
		{
			name: "invalid PEM private key data - completely invalid",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-engine-operator-webhook",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"tls.crt": validCert,
					"tls.key": []byte("invalid key data"),
				},
			},
			wantErr:     true,
			expectedErr: "tls.key does not contain valid PEM private key data",
		},
		{
			name: "valid RSA PRIVATE KEY format",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-engine-operator-webhook",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"tls.crt": validCert,
					"tls.key": []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEAu1SU1LfVLPHCozMxH2Mo4lgOEePzNm0tfn1iHD5teY/Ztus=
-----END RSA PRIVATE KEY-----`),
				},
			},
			wantErr: false,
		},
		{
			name: "valid EC PRIVATE KEY format",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-engine-operator-webhook",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"tls.crt": validCert,
					"tls.key": []byte(`-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIIGlRHdF6i0VfzJNMG9HMF8VfzJNMG9HMF8VfzJNMG9HoAoGCCqGSM49
-----END EC PRIVATE KEY-----`),
				},
			},
			wantErr: false,
		},
		{
			name: "nil Data field",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-engine-operator-webhook",
					Namespace: namespace,
				},
				Data: nil,
			},
			wantErr:     true,
			expectedErr: "secret multicluster-engine-operator-webhook is missing required keys",
		},
		{
			name: "tls.crt with only whitespace",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-engine-operator-webhook",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"tls.crt": []byte("   \n\t  "),
					"tls.key": validKey,
				},
			},
			wantErr:     true,
			expectedErr: "tls.crt does not contain valid PEM certificate data",
		},
		{
			name: "tls.key with only whitespace",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-engine-operator-webhook",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"tls.crt": validCert,
					"tls.key": []byte("   \n\t  "),
				},
			},
			wantErr:     true,
			expectedErr: "tls.key does not contain valid PEM private key data",
		},
		{
			name: "tls.key with ENCRYPTED PRIVATE KEY (valid)",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-engine-operator-webhook",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"tls.crt": validCert,
					"tls.key": []byte(`-----BEGIN ENCRYPTED PRIVATE KEY-----
MIIFHDBOBgkqhkiG9w0BBQ0wQTApBgkqhkiG9w0BBQwwHAQIhKLn4g0M5GcCAggA
-----END ENCRYPTED PRIVATE KEY-----`),
				},
			},
			wantErr: false,
		},
		{
			name: "tls.key with only BEGIN but no PRIVATE KEY",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-engine-operator-webhook",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"tls.crt": validCert,
					"tls.key": []byte("-----BEGIN PUBLIC KEY-----\ndata\n-----END PUBLIC KEY-----"),
				},
			},
			wantErr:     true,
			expectedErr: "tls.key does not contain valid PEM private key data",
		},
		{
			name: "tls.crt with multiple certificates (valid)",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-engine-operator-webhook",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"tls.crt": []byte(`-----BEGIN CERTIFICATE-----
MIICljCCAX4CCQCKz8Vz1V9Z8jANBgkqhkiG9w0BAQsFADANMQswCQYDVQQGEwJV
-----END CERTIFICATE-----
-----BEGIN CERTIFICATE-----
MIICljCCAX4CCQCKz8Vz1V9Z8jANBgkqhkiG9w0BAQsFADANMQswCQYDVQQGEwJV
-----END CERTIFICATE-----`),
					"tls.key": validKey,
				},
			},
			wantErr: false,
		},
		{
			name: "case sensitivity - BEGIN certificate (lowercase)",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-engine-operator-webhook",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"tls.crt": []byte("-----BEGIN certificate-----\ndata\n-----END certificate-----"),
					"tls.key": validKey,
				},
			},
			wantErr:     true,
			expectedErr: "tls.crt does not contain valid PEM certificate data",
		},
		{
			name: "case sensitivity - BEGIN private key (lowercase)",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-engine-operator-webhook",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"tls.crt": validCert,
					"tls.key": []byte("-----BEGIN private key-----\ndata\n-----END private key-----"),
				},
			},
			wantErr:     true,
			expectedErr: "tls.key does not contain valid PEM private key data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fake Kubernetes client with the test secret
			fakeClient := fake.NewSimpleClientset(tt.secret)

			// Initialize the global serving cert getter with the fake client
			NewGlobalServingCertCABundleGetter(fakeClient, "test-ca-bundle", namespace)

			// Call DumpServingCertSecret
			err := DumpServingCertSecret()

			// Check if error expectation matches
			if (err != nil) != tt.wantErr {
				t.Errorf("DumpServingCertSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// If we expect an error, verify the error message
			if tt.wantErr && err != nil {
				if !strings.Contains(err.Error(), tt.expectedErr) {
					t.Errorf("DumpServingCertSecret() error = %v, expected error to contain %v", err, tt.expectedErr)
				}
			}
		})
	}
}

func TestDumpServingCertSecret_SecretNotFound(t *testing.T) {
	namespace := "test-namespace"

	// Create a fake Kubernetes client without the required secret
	fakeClient := fake.NewSimpleClientset()

	// Initialize the global serving cert getter with the fake client
	NewGlobalServingCertCABundleGetter(fakeClient, "test-ca-bundle", namespace)

	// Call DumpServingCertSecret
	err := DumpServingCertSecret()

	// Verify that an error is returned
	if err == nil {
		t.Error("DumpServingCertSecret() expected error when secret not found, got nil")
		return
	}

	// Verify the error message indicates the secret was not found
	expectedErrSubstring := "failed to get secret multicluster-engine-operator-webhook"
	if !strings.Contains(err.Error(), expectedErrSubstring) {
		t.Errorf("DumpServingCertSecret() error = %v, expected error to contain %v", err, expectedErrSubstring)
	}
}

func TestGetServingCertCABundle(t *testing.T) {
	namespace := "test-namespace"
	configMapName := "test-ca-bundle"

	tests := []struct {
		name        string
		setupClient func() *fake.Clientset
		wantErr     bool
		expectedErr string
		wantResult  string
	}{
		{
			name: "valid CA bundle",
			setupClient: func() *fake.Clientset {
				cm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configMapName,
						Namespace: namespace,
					},
					Data: map[string]string{
						"ca-bundle.crt": "-----BEGIN CERTIFICATE-----\ntest-ca-bundle\n-----END CERTIFICATE-----",
					},
				}
				return fake.NewSimpleClientset(cm)
			},
			wantErr:    false,
			wantResult: "-----BEGIN CERTIFICATE-----\ntest-ca-bundle\n-----END CERTIFICATE-----",
		},
		{
			name: "empty CA bundle",
			setupClient: func() *fake.Clientset {
				cm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configMapName,
						Namespace: namespace,
					},
					Data: map[string]string{
						"ca-bundle.crt": "",
					},
				}
				return fake.NewSimpleClientset(cm)
			},
			wantErr:     true,
			expectedErr: "CA bundle ConfigMap does not contain a CA bundle",
		},
		{
			name: "missing ca-bundle.crt key",
			setupClient: func() *fake.Clientset {
				cm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configMapName,
						Namespace: namespace,
					},
					Data: map[string]string{},
				}
				return fake.NewSimpleClientset(cm)
			},
			wantErr:     true,
			expectedErr: "CA bundle ConfigMap does not contain a CA bundle",
		},
		{
			name: "configmap not found",
			setupClient: func() *fake.Clientset {
				return fake.NewSimpleClientset()
			},
			wantErr:     true,
			expectedErr: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup the fake client
			fakeClient := tt.setupClient()

			// Initialize the global serving cert getter
			NewGlobalServingCertCABundleGetter(fakeClient, configMapName, namespace)

			// Call GetServingCertCABundle
			result, err := GetServingCertCABundle()

			// Check if error expectation matches
			if (err != nil) != tt.wantErr {
				t.Errorf("GetServingCertCABundle() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// If we expect an error, verify the error message
			if tt.wantErr && err != nil {
				if !strings.Contains(err.Error(), tt.expectedErr) {
					t.Errorf("GetServingCertCABundle() error = %v, expected error to contain %v", err, tt.expectedErr)
				}
			}

			// If we don't expect an error, verify the result
			if !tt.wantErr && result != tt.wantResult {
				t.Errorf("GetServingCertCABundle() = %v, want %v", result, tt.wantResult)
			}
		})
	}
}

func TestGetServingCertCABundle_NilGlobalGetter(t *testing.T) {
	// Save the original global getter
	originalGetter := GlobalServingCertGetter
	defer func() {
		GlobalServingCertGetter = originalGetter
	}()

	// Set GlobalServingCertGetter to nil
	GlobalServingCertGetter = nil

	// Call GetServingCertCABundle
	_, err := GetServingCertCABundle()

	// Verify that an error is returned
	if err == nil {
		t.Error("GetServingCertCABundle() expected error when GlobalServingCertGetter is nil, got nil")
		return
	}

	// Verify the error message
	expectedErr := "GlobalServingCertCABundleGetter is nil"
	if err.Error() != expectedErr {
		t.Errorf("GetServingCertCABundle() error = %v, want %v", err, expectedErr)
	}
}
