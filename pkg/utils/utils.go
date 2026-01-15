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
	backplanev1.ClusterAPIProviderAzurePreview,
	backplanev1.ClusterAPIProviderMetal,
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
		"APISERVER_NETWORK_PROXY", "ASSISTED_IMAGE_SERVICE", "ASSISTED_INSTALLER", "ASSISTED_INSTALLER_AGENT",
		"ASSISTED_INSTALLER_CONTROLLER", "ASSISTED_SERVICE_9", "AWS_ENCRYPTION_PROVIDER", "AZURE_SERVICE_OPERATOR_RHEL9",
		"CLUSTERCLAIMS_CONTROLLER", "CLUSTER_API", "CLUSTER_API_PROVIDER_AGENT", "CLUSTER_API_PROVIDER_AWS",
		"CLUSTER_API_PROVIDER_AWS_RHEL9", "CLUSTER_API_PROVIDER_AZURE", "CLUSTER_API_PROVIDER_AZURE_RHEL9",
		"CLUSTER_API_PROVIDER_KUBEVIRT", "CLUSTER_CURATOR_CONTROLLER", "CLUSTER_IMAGE_SET_CONTROLLER", "CLUSTER_PROXY",
		"CLUSTER_PROXY_ADDON", "CLUSTERLIFECYCLE_STATE_METRICS", "CONSOLE_MCE", "DISCOVERY_OPERATOR",
		"HYPERSHIFT_ADDON_OPERATOR", "HYPERSHIFT_OPERATOR", "IMAGE_BASED_INSTALL_OPERATOR", "KUBE_RBAC_PROXY_MCE",
		"MANAGEDCLUSTER_IMPORT_CONTROLLER", "MANAGED_SERVICEACCOUNT", "MCE_CAPI_WEBHOOK_CONFIG_RHEL9",
		"MULTICLOUD_MANAGER", "OPENSHIFT_HIVE", "OSE_AWS_CLUSTER_API_CONTROLLERS_RHEL9", "OSE_CLUSTER_API_RHEL9",
		"POSTGRESQL_13", "PROVIDER_CREDENTIAL_CONTROLLER", "REGISTRATION", "REGISTRATION_OPERATOR", "WORK",
		"apiserver_network_proxy", "assisted_image_service", "assisted_installer", "assisted_installer_agent",
		"assisted_installer_controller", "assisted_service_9", "aws_encryption_provider", "azure_service_operator_rhel9",
		"cluster_api", "cluster_api_bootstrap_provider_openshift_assisted",
		"cluster_api_controlplane_provider_openshift_assisted", "cluster_api_provider_agent", "cluster_api_provider_aws",
		"cluster_api_provider_aws_rhel9", "cluster_api_provider_azure", "cluster_api_provider_azure_rhel9",
		"cluster_api_provider_kubevirt", "cluster_api_provider_openshift_assisted_bootstrap",
		"cluster_api_provider_openshift_assisted_control_plane", "cluster_curator_controller",
		"cluster_image_set_controller", "cluster_proxy", "cluster_proxy_addon", "clusterclaims_controller",
		"clusterlifecycle_state_metrics", "console_mce", "discovery_operator", "hypershift_addon_operator",
		"hypershift_operator", "image_based_install_operator", "ip_address_manager", "kube_rbac_proxy_mce",
		"managed_serviceaccount", "managedcluster_import_controller", "mce_capi_webhook_config_rhel9",
		"multicloud_manager", "openshift_hive", "ose_aws_cluster_api_controllers_rhel9",
		"ose_baremetal_cluster_api_controllers_rhel9", "ose_cluster_api_rhel9", "postgresql_13",
		"provider_credential_controller", "registration", "registration_operator", "work",
	}
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

// ComponentCRDDirectories returns all CRD directory paths for a given component.
// For CAPI components with platform variants (ClusterAPI, ClusterAPIProviderMetal,
// ClusterAPIProviderOA), this returns both OCP and K8s directory variants to ensure
// proper CRD handling regardless of the platform.
// For all other components, returns a single directory.
func ComponentCRDDirectories(component string) []string {
	switch component {
	// ClusterAPI - has both OCP and K8s variants
	case backplanev1.ClusterAPI, backplanev1.ClusterAPIPreview:
		return []string{
			backplanev1.ClusterAPICRDDir,    // cluster-api
			backplanev1.ClusterAPIK8SCRDDir, // cluster-api-k8s
		}

	// ClusterAPI Provider Metal - has both OCP and K8s variants
	case backplanev1.ClusterAPIProviderMetal, backplanev1.ClusterAPIProviderMetalPreview:
		return []string{
			backplanev1.ClusterAPIProviderMetalCRDDir,    // cluster-api-provider-metal3
			backplanev1.ClusterAPIProviderMetalK8SCRDDir, // cluster-api-provider-metal3-k8s
		}

	// ClusterAPI Provider OpenShift Assisted - has both OCP and K8s variants
	case backplanev1.ClusterAPIProviderOA, backplanev1.ClusterAPIProviderOAPreview:
		return []string{
			backplanev1.ClusterAPIProviderOACRDDir,    // cluster-api-provider-openshift-assisted
			backplanev1.ClusterAPIProviderOAK8SCRDDir, // cluster-api-provider-openshift-assisted-k8s
		}

	// ClusterAPI Provider Azure - has both OCP and K8s variants
	case backplanev1.ClusterAPIProviderAzure, backplanev1.ClusterAPIProviderAzurePreview:
		return []string{
			backplanev1.ClusterAPIProviderAzureCRDDir,    // cluster-api-provider-azure
			backplanev1.ClusterAPIProviderAzureK8SCRDDir, // cluster-api-provider-azure-k8s
		}

	// All other components - single directory (no platform variants)
	case backplanev1.AssistedService:
		return []string{backplanev1.AssistedServiceCRDDir}
	case backplanev1.ClusterAPIProviderAWS, backplanev1.ClusterAPIProviderAWSPreview:
		return []string{backplanev1.ClusterAPIProviderAWSCRDDir}
	case backplanev1.ClusterLifecycle:
		return []string{backplanev1.ClusterLifecycleCRDDir}
	case backplanev1.ClusterManager:
		return []string{backplanev1.ClusterManagerCRDDir}
	case backplanev1.ClusterProxyAddon:
		return []string{backplanev1.ClusterProxyAddonCRDDir}
	case backplanev1.Discovery:
		return []string{backplanev1.DiscoveryCRDDir}
	case backplanev1.Hive:
		return []string{backplanev1.HiveCRDDir}
	case backplanev1.ImageBasedInstallOperator, backplanev1.ImageBasedInstallOperatorPreview:
		return []string{backplanev1.ImageBasedInstallOperatorCRDDir}
	case backplanev1.ManagedServiceAccount, backplanev1.ManagedServiceAccountPreview:
		return []string{backplanev1.ManagedServiceAccountCRDDir}
	case backplanev1.ServerFoundation:
		return []string{backplanev1.ServerFoundationCRDDir}
	default:
		return []string{}
	}
}
