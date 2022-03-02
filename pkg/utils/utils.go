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

var onComponents = []string{
	backplanev1.AssistedService,
	backplanev1.ClusterLifecycle,
	backplanev1.ClusterManager,
	backplanev1.Discovery,
	backplanev1.Hive,
	backplanev1.ServerFoundation,
	// backplanev1.ConsoleMCE, // determined by OCP version
}

var offComponents = []string{
	backplanev1.ManagedServiceAccount,
}

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

//AvailabilityConfigIsValid ...
func AvailabilityConfigIsValid(config backplanev1.AvailabilityType) bool {
	switch config {
	case backplanev1.HAHigh, backplanev1.HABasic:
		return true
	default:
		return false
	}
}

func GetTestImages() []string {
	return []string{"registration_operator", "openshift_hive", "multicloud_manager",
		"managedcluster_import_controller", "registration", "work", "discovery_operator", "cluster_curator_controller",
		"clusterlifecycle_state_metrics", "clusterclaims_controller", "provider_credential_controller", "managed_serviceaccount",
		"assisted_service", "assisted_image_service", "postgresql_12", "assisted_installer_agent", "assisted_installer_controller",
		"assisted_installer", "console_mce"}
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
