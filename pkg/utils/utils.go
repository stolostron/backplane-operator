// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	"os"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	UnitTestEnvVar = "UNIT_TEST"

	// OpenShiftClusterMonitoringLabel is the label for OpenShift cluster monitoring.
	OpenShiftClusterMonitoringLabel = "openshift.io/cluster-monitoring"
)

const (
	/*
	   MCEOperatorMetricsServiceName is the name of the service used to expose the metrics
	   endpoint for the multicluster-engine-operator.
	*/
	MCEOperatorMetricsServiceName = "multicluster-engine-operator-metrics"

	/*
	   MCEOperatorMetricsServiceMonitorName is the name of the service monitor used to expose
	   the metrics for the multicluster-engine-operator.
	*/
	MCEOperatorMetricsServiceMonitorName = "multicluster-engine-operator-metrics"
)

var onComponents = []string{
	backplanev1.AssistedService,
	backplanev1.ClusterLifecycle,
	backplanev1.ClusterManager,
	backplanev1.Discovery,
	backplanev1.Hive,
	backplanev1.ServerFoundation,
	backplanev1.ClusterProxyAddon,
	backplanev1.LocalCluster,
	backplanev1.HypershiftLocalHosting,
	backplanev1.HyperShift,
	backplanev1.ManagedServiceAccount,
	// backplanev1.ConsoleMCE, // determined by OCP version
}

var offComponents = []string{
	backplanev1.ClusterAPI,
	backplanev1.ClusterAPIProviderAWS,
	backplanev1.ClusterAPIProviderMetalPreview,
	backplanev1.ClusterAPIProviderOA,
	backplanev1.ImageBasedInstallOperator,
}

var nonOCPComponents = []string{
	backplanev1.ClusterLifecycle,
	backplanev1.ClusterManager,
	backplanev1.HyperShift,
	backplanev1.HypershiftLocalHosting,
	backplanev1.LocalCluster,
	backplanev1.ServerFoundation,
}

var GlobalDeployOnOCP = true

// SetDefaultComponents returns true if changes are made
func SetDefaultComponents(m *backplanev1.MultiClusterEngine) bool {
	components := onComponents
	updated := false
	for _, c := range components {
		if !m.ComponentPresent(c) {
			m.Enable(c)
			updated = true
		}
	}
	for _, c := range offComponents {
		if !m.ComponentPresent(c) {
			m.Disable(c)
			updated = true
		}
	}
	return updated
}

// AddBackplaneConfigLabels adds BackplaneConfig Labels ...
func AddBackplaneConfigLabels(u client.Object, name string) {
	labels := make(map[string]string)
	for key, value := range u.GetLabels() {
		labels[key] = value
	}
	labels["backplaneconfig.name"] = name

	u.SetLabels(labels)
}

// CoreToUnstructured converts a Core Kube resource to unstructured
func CoreToUnstructured(obj runtime.Object) (*unstructured.Unstructured, error) {
	content, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	u := &unstructured.Unstructured{}
	err = u.UnmarshalJSON(content)
	return u, err
}

// ProxyEnvVarIsSet ...
// OLM handles these environment variables as a unit;
// if at least one of them is set, all three are considered overridden
// and the cluster-wide defaults are not used for the deployments of the subscribed Operator.
// https://docs.openshift.com/container-platform/4.6/operators/admin/olm-configuring-proxy-support.html
// GetProxyEnvVars
func ProxyEnvVarsAreSet() bool {
	if os.Getenv("HTTP_PROXY") != "" || os.Getenv("HTTPS_PROXY") != "" || os.Getenv("NO_PROXY") != "" {
		return true
	}
	return false
}

func DefaultReplicaCount(mce *backplanev1.MultiClusterEngine) int {
	if mce.Spec.AvailabilityConfig == backplanev1.HABasic {
		return 1
	}
	return 2
}

// AvailabilityConfigIsValid ...
func AvailabilityConfigIsValid(config backplanev1.AvailabilityType) bool {
	switch config {
	case backplanev1.HAHigh, backplanev1.HABasic:
		return true
	default:
		return false
	}
}

// DeduplicateComponents removes duplicate componentconfigs by name, keeping the config of the last
// componentconfig in the list. Returns true if changes are made.
func DeduplicateComponents(m *backplanev1.MultiClusterEngine) bool {
	config := m.Spec.Overrides.Components
	newConfig := deduplicate(m.Spec.Overrides.Components)
	if len(newConfig) != len(config) {
		m.Spec.Overrides.Components = newConfig
		return true
	}
	return false
}

// deduplicate removes duplicate componentconfigs by name, keeping the config of the last
// componentconfig in the list
func deduplicate(config []backplanev1.ComponentConfig) []backplanev1.ComponentConfig {
	newConfig := []backplanev1.ComponentConfig{}
	for _, cc := range config {
		duplicate := false
		// if name in newConfig update newConfig at existing index
		for i, ncc := range newConfig {
			if cc.Name == ncc.Name {
				duplicate = true
				newConfig[i] = cc
				break
			}
		}
		if !duplicate {
			newConfig = append(newConfig, cc)
		}
	}
	return newConfig
}

// GetImagePullPolicy returns either pull policy from CR overrides or default of Always
func GetImagePullPolicy(m *backplanev1.MultiClusterEngine) corev1.PullPolicy {
	if m.Spec.Overrides == nil || m.Spec.Overrides.ImagePullPolicy == "" {
		return corev1.PullIfNotPresent
	}
	return m.Spec.Overrides.ImagePullPolicy
}

func IsUpgrading(m *backplanev1.MultiClusterEngine) bool {
	if m.Status.DesiredVersion != m.Status.CurrentVersion && m.Status.CurrentVersion != "" {
		return true
	}
	return false
}

func IsEUSUpgrading(m *backplanev1.MultiClusterEngine) bool {
	if m.Status.CurrentVersion == "" {
		return false
	}

	currentParts := strings.Split(m.Status.CurrentVersion, ".")
	desiredParts := strings.Split(m.Status.DesiredVersion, ".")

	if len(currentParts) < 2 || len(desiredParts) < 2 {
		return false
	}

	currentY, err := strconv.Atoi(currentParts[1])
	if err != nil {
		return false
	}

	desiredY, err := strconv.Atoi(desiredParts[1])
	if err != nil {
		return false
	}

	return desiredY-currentY == 2
}

func GetTestImages() []string {
	return []string{
		"REGISTRATION_OPERATOR", "OPENSHIFT_HIVE", "MULTICLOUD_MANAGER", "MANAGEDCLUSTER_IMPORT_CONTROLLER",
		"REGISTRATION", "WORK", "DISCOVERY_OPERATOR", "CLUSTER_CURATOR_CONTROLLER", "CLUSTERLIFECYCLE_STATE_METRICS",
		"CLUSTERCLAIMS_CONTROLLER", "PROVIDER_CREDENTIAL_CONTROLLER", "MANAGED_SERVICEACCOUNT",
		"ASSISTED_SERVICE_9", "ASSISTED_IMAGE_SERVICE", "POSTGRESQL_12", "ASSISTED_INSTALLER_AGENT",
		"ASSISTED_INSTALLER_CONTROLLER", "ASSISTED_INSTALLER", "CONSOLE_MCE", "HYPERSHIFT_ADDON_OPERATOR",
		"HYPERSHIFT_OPERATOR", "APISERVER_NETWORK_PROXY", "AWS_ENCRYPTION_PROVIDER", "CLUSTER_API",
		"CLUSTER_API_PROVIDER_AGENT", "CLUSTER_API_PROVIDER_AWS", "CLUSTER_API_PROVIDER_AZURE",
		"CLUSTER_API_PROVIDER_KUBEVIRT", "KUBE_RBAC_PROXY_MCE", "CLUSTER_PROXY_ADDON", "CLUSTER_PROXY",
		"CLUSTER_IMAGE_SET_CONTROLLER", "IMAGE_BASED_INSTALL_OPERATOR", "OSE_CLUSTER_API_RHEL9",
		"OSE_AWS_CLUSTER_API_CONTROLLERS_RHEL9", "MCE_CAPI_WEBHOOK_CONFIG_RHEL9", "CLUSTER_API_PROVIDER_AWS_RHEL9",
		"registration_operator", "openshift_hive", "multicloud_manager", "managedcluster_import_controller",
		"registration", "work", "discovery_operator", "cluster_curator_controller", "clusterlifecycle_state_metrics",
		"clusterclaims_controller", "provider_credential_controller", "managed_serviceaccount",
		"assisted_service_9", "assisted_image_service", "postgresql_12", "assisted_installer_agent",
		"assisted_installer_controller", "assisted_installer", "console_mce", "hypershift_addon_operator",
		"hypershift_operator", "apiserver_network_proxy", "aws_encryption_provider", "cluster_api",
		"cluster_api_provider_agent", "cluster_api_provider_aws", "cluster_api_provider_azure",
		"cluster_api_provider_kubevirt", "mce_capi_webhook_config_rhel9", "kube_rbac_proxy_mce", "cluster_proxy_addon",
		"cluster_proxy", "cluster_image_set_controller", "image_based_install_operator", "ose_cluster_api_rhel9",
		"ose_aws_cluster_api_controllers_rhel9", "cluster_api_bootstrap_provider_openshift_assisted",
		"cluster_api_provider_openshift_assisted_bootstrap", "cluster_api_provider_openshift_assisted_control_plane",
		"cluster_api_controlplane_provider_openshift_assisted", "ip_address_manager",
		"ose_baremetal_cluster_api_controllers_rhel9", "cluster_api_provider_aws_rhel9"}
}

func IsUnitTest() bool {
	if unitTest, found := os.LookupEnv(UnitTestEnvVar); found {
		if unitTest == "true" {
			return true
		}
	}
	return false
}

func GetMCHNamespace(m *backplanev1.MultiClusterEngine) string {
	val, ok := m.Labels["installer.namespace"]
	if ok {
		return val
	}
	return ""
}

func DefaultTolerations() []corev1.Toleration {
	return []corev1.Toleration{
		{
			Effect:   "NoSchedule",
			Key:      "node-role.kubernetes.io/infra",
			Operator: "Exists",
		},
		{
			Effect:   "NoSchedule",
			Key:      "dedicated",
			Operator: "Exists",
		},
	}
}

func Contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

func Remove(s []string, str string) []string {
	for i, v := range s {
		if v == str {
			return append(s[:i], s[i+1:]...)
		}
	}
	return s
}

func OperatorNamespace() string {
	deploymentNamespace, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		panic("Missing POD_NAMESPACE variable")
	}
	return deploymentNamespace
}

// isCommunityMode returns true if operator is running in community mode
func isCommunityMode() bool {
	packageName := os.Getenv("OPERATOR_PACKAGE")
	if packageName == "multicluster-engine" {
		return false
	} else {
		// other option is "stolostron-engine"
		return true
	}
}

// HubType defines the circumstances of how MCE is being run
type HubType string

const (
	// HubType MCE is the product version of MCE running standalone
	HubTypeMCE HubType = "mce"
	// HubType ACM is the product version of MCE managed by ACM
	HubTypeACM HubType = "acm"
	// HubType StolostronEngine is the community version of MCE running standalone
	HubTypeStolostronEngine HubType = "stolostron-engine"
	// HubType Stolostron is the community version of MCE managed by ACM
	HubTypeStolostron HubType = "stolostron"
)

// GetHubType returns the HubType which defines the circumstances of how MCE is being run
func GetHubType(mce *backplanev1.MultiClusterEngine) string {
	isCommunity := isCommunityMode()
	isManaged := backplanev1.IsACMManaged(mce)
	if isCommunity {
		if isManaged {
			return string(HubTypeStolostron)
		} else {
			return string(HubTypeStolostronEngine)
		}
	} else {
		if isManaged {
			return string(HubTypeACM)
		} else {
			return string(HubTypeMCE)
		}
	}
}

func SetDeployOnOCP(v bool) {
	GlobalDeployOnOCP = v
}

func DeployOnOCP() bool {
	return GlobalDeployOnOCP
}

func DetectOpenShift(cl client.Client) error {
	checkNs := &corev1.Namespace{}
	err := cl.Get(context.TODO(), types.NamespacedName{Name: "openshift-config"}, checkNs)
	if err != nil {
		if errors.IsNotFound(err) {
			fmt.Println("### The operator is running on non-OCP ###")
			SetDeployOnOCP(false)
			return nil
		}

		return err
	}
	SetDeployOnOCP(true)
	return nil
}

func ComponentOnNonOCP(name string) bool {
	for _, component := range nonOCPComponents {
		if name == component {
			return true
		}
	}
	return false
}

type ServingCertGetter struct {
	caBundleConfigMapName, namespace string
	kubeClient                       kubernetes.Interface
}

var GlobalServingCertGetter *ServingCertGetter

func NewGlobalServingCertCABundleGetter(kubeClient kubernetes.Interface,
	caBundleConfigMapName, namespace string) {
	GlobalServingCertGetter = &ServingCertGetter{
		kubeClient:            kubeClient,
		namespace:             namespace,
		caBundleConfigMapName: caBundleConfigMapName,
	}
}

func GetServingCertCABundle() (string, error) {
	if GlobalServingCertGetter == nil {
		return "", fmt.Errorf("GlobalServingCertCABundleGetter is nil")
	}
	caBundleConfigMap, err := GlobalServingCertGetter.kubeClient.CoreV1().ConfigMaps(GlobalServingCertGetter.namespace).
		Get(context.Background(), GlobalServingCertGetter.caBundleConfigMapName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	caBundle := caBundleConfigMap.Data["ca-bundle.crt"]
	if caBundle == "" {
		return "", fmt.Errorf("CA bundle ConfigMap does not contain a CA bundle")
	}
	return caBundle, nil
	// return base64.StdEncoding.EncodeToString([]byte(caBundle)), nil
}

func DumpServingCertSecret() error {
	certKeySecret, err := GlobalServingCertGetter.kubeClient.CoreV1().Secrets(GlobalServingCertGetter.namespace).
		Get(context.Background(), "multicluster-engine-operator-webhook", metav1.GetOptions{})

	if err != nil {
		return fmt.Errorf("failed to get secret multicluster-engine-operator-webhook: %v", err)
	}

	// Validate that the secret contains the required keys and valid PEM data
	tlsCert, certExists := certKeySecret.Data["tls.crt"]
	tlsKey, keyExists := certKeySecret.Data["tls.key"]

	if !certExists || !keyExists {
		return fmt.Errorf("secret multicluster-engine-operator-webhook is missing required keys (tls.crt or tls.key)")
	}

	if len(tlsCert) == 0 || len(tlsKey) == 0 {
		return fmt.Errorf("secret multicluster-engine-operator-webhook contains empty certificate or key data")
	}

	// Validate that the certificate data contains valid PEM data
	if !bytes.Contains(tlsCert, []byte("BEGIN CERTIFICATE")) {
		return fmt.Errorf("tls.crt does not contain valid PEM certificate data")
	}

	if !bytes.Contains(tlsKey, []byte("BEGIN")) || !bytes.Contains(tlsKey, []byte("PRIVATE KEY")) {
		return fmt.Errorf("tls.key does not contain valid PEM private key data")
	}

	dir := "/tmp/k8s-webhook-server/serving-certs"

	err = os.MkdirAll(dir, 0700)
	if err != nil {
		return fmt.Errorf("failed to create directory %q: %v", dir, err)
	}

	for key, data := range certKeySecret.Data {
		filename := path.Clean(path.Join(dir, key))
		lastData, err := os.ReadFile(filepath.Clean(filename))
		switch {
		case os.IsNotExist(err):
			// create file
			if err := os.WriteFile(filename, data, 0600); err != nil {
				return fmt.Errorf("unable to write file %q: %w", filename, err)
			}
		case err != nil:
			return fmt.Errorf("unable to write file %q: %w", filename, err)
		case bytes.Equal(lastData, data):
			// skip file without any change
			continue
		default:
			// update file
			if err := os.WriteFile(path.Clean(filename), data, 0600); err != nil {
				return fmt.Errorf("unable to write file %q: %w", filename, err)
			}
		}
	}

	return nil
}

/*
ComponentToCRDDirectory returns a map of component names to their corresponding CRD directory names.
This mapping is used to skip CRD rendering for externally managed components.
When NonOCP() returns true, the ClusterAPI component uses 'k8s' as its CRD directory.
*/
func ComponentToCRDDirectory() map[string]string {
	var clusterAPICRDDir string
	var clusterAPIProviderOACRDDir string
	var clusterAPIProviderMetalCRDDir string

	if DeployOnOCP() {
		clusterAPICRDDir = backplanev1.ClusterAPICRDDir
		clusterAPIProviderOACRDDir = backplanev1.ClusterAPIProviderOACRDDir
		clusterAPIProviderMetalCRDDir = backplanev1.ClusterAPIProviderMetalCRDDir
	} else {
		clusterAPICRDDir = backplanev1.ClusterAPIK8SCRDDir
		clusterAPIProviderOACRDDir = backplanev1.ClusterAPIProviderOAK8SCRDDir
		clusterAPIProviderMetalCRDDir = backplanev1.ClusterAPIProviderMetalK8SCRDDir
	}
	return map[string]string{
		backplanev1.AssistedService:                  backplanev1.AssistedServiceCRDDir,
		backplanev1.ClusterAPI:                       clusterAPICRDDir,
		backplanev1.ClusterAPIPreview:                clusterAPICRDDir,
		backplanev1.ClusterAPIProviderAWS:            backplanev1.ClusterAPIProviderAWSCRDDir,
		backplanev1.ClusterAPIProviderAWSPreview:     backplanev1.ClusterAPIProviderAWSCRDDir,
		backplanev1.ClusterAPIProviderMetal:          clusterAPIProviderMetalCRDDir,
		backplanev1.ClusterAPIProviderMetalPreview:   clusterAPIProviderMetalCRDDir,
		backplanev1.ClusterAPIProviderOA:             clusterAPIProviderOACRDDir,
		backplanev1.ClusterAPIProviderOAPreview:      clusterAPIProviderOACRDDir,
		backplanev1.ClusterLifecycle:                 backplanev1.ClusterLifecycleCRDDir,
		backplanev1.ClusterManager:                   backplanev1.ClusterManagerCRDDir,
		backplanev1.ClusterProxyAddon:                backplanev1.ClusterProxyAddonCRDDir,
		backplanev1.Discovery:                        backplanev1.DiscoveryCRDDir,
		backplanev1.Hive:                             backplanev1.HiveCRDDir,
		backplanev1.ImageBasedInstallOperator:        backplanev1.ImageBasedInstallOperatorCRDDir,
		backplanev1.ImageBasedInstallOperatorPreview: backplanev1.ImageBasedInstallOperatorCRDDir,
		backplanev1.ManagedServiceAccount:            backplanev1.ManagedServiceAccountCRDDir,
		backplanev1.ManagedServiceAccountPreview:     backplanev1.ManagedServiceAccountCRDDir,
		backplanev1.ServerFoundation:                 backplanev1.ServerFoundationCRDDir,
	}
}
