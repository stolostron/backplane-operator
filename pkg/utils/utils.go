// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"encoding/json"
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
	backplanev1.ImageBasedInstallOperator,
	// backplanev1.ConsoleMCE, // determined by OCP version
}

var offComponents = []string{}

// SetDefaultComponents returns true if changes are made
func SetDefaultComponents(m *backplanev1.MultiClusterEngine) bool {
	updated := false
	for _, c := range onComponents {
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

// SetHostedDefaultComponents returns true if changes are made
func SetHostedDefaultComponents(m *backplanev1.MultiClusterEngine) bool {
	onComponents := []string{
		backplanev1.ClusterManager,
		backplanev1.ServerFoundation,
	}

	updated := false
	for _, c := range onComponents {
		if !m.ComponentPresent(c) {
			m.Enable(c)
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

func GetTestImages() []string {
	return []string{"registration_operator", "openshift_hive", "multicloud_manager",
		"managedcluster_import_controller", "registration", "work", "discovery_operator", "cluster_curator_controller",
		"clusterlifecycle_state_metrics", "clusterclaims_controller", "provider_credential_controller", "managed_serviceaccount",
		"assisted_service", "assisted_image_service", "postgresql_12", "assisted_installer_agent", "assisted_installer_controller",
		"assisted_installer", "console_mce", "hypershift_addon_operator", "hypershift_operator",
		"apiserver_network_proxy", "aws_encryption_provider", "cluster_api", "cluster_api_provider_agent", "cluster_api_provider_aws",
		"cluster_api_provider_azure", "cluster_api_provider_kubevirt", "kube_rbac_proxy_mce", "cluster_proxy_addon", "cluster_proxy", "cluster_image_set_controller", "image_based_install_operator"}
}

func IsUnitTest() bool {
	if unitTest, found := os.LookupEnv(UnitTestEnvVar); found {
		if unitTest == "true" {
			return true
		}
	}
	return false
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

// isACMManaged returns true if operator is managed by ACM
func isACMManaged(mce *backplanev1.MultiClusterEngine) bool {
	managedByACMLabel := "multiclusterhubs.operator.open-cluster-management.io/managed-by"
	if labels := mce.GetLabels(); labels != nil {
		if labels[managedByACMLabel] == "true" {
			return true
		}
	}
	return false
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
	isManaged := isACMManaged(mce)
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
