// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"os"

	"github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
)

// ContainsMap returns whether the expected map entries are included in the map
func ContainsMap(all map[string]string, expected map[string]string) bool {
	for key, exval := range expected {
		allval, ok := all[key]
		if !ok || allval != exval {
			return false
		}

	}
	return true
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

// DistributePods returns a anti-affinity rule that specifies a preference for pod replicas with
// the matching key-value label to run across different nodes and zones
func DistributePods(key string, value string) *corev1.Affinity {
	return &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
				{
					PodAffinityTerm: corev1.PodAffinityTerm{
						TopologyKey: "kubernetes.io/hostname",
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      key,
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{value},
								},
							},
						},
					},
					Weight: 35,
				},
				{
					PodAffinityTerm: corev1.PodAffinityTerm{
						TopologyKey: "failure-domain.beta.kubernetes.io/zone",
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      key,
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{value},
								},
							},
						},
					},
					Weight: 70,
				},
			},
		},
	}
}

func GetReplicaCount() int32 {
	return 1
}

//GetImagePullPolicy returns either pull policy from CR overrides or default of Always
func GetImagePullPolicy(m *v1alpha1.MultiClusterEngine) corev1.PullPolicy {
	// if m.Spec.Overrides == nil || m.Spec.Overrides.ImagePullPolicy == "" {
	// 	return corev1.PullAlways
	// }
	// return m.Spec.Overrides.ImagePullPolicy
	return corev1.PullAlways
}

// GetContainerArgs return arguments forfirst container in deployment
func GetContainerArgs(dep *appsv1.Deployment) []string {
	return dep.Spec.Template.Spec.Containers[0].Args
}

// GetContainerEnvVars returns environment variables for first container in deployment
func GetContainerEnvVars(dep *appsv1.Deployment) []corev1.EnvVar {
	return dep.Spec.Template.Spec.Containers[0].Env
}

// GetContainerVolumeMounts returns volume mount for first container in deployment
func GetContainerVolumeMounts(dep *appsv1.Deployment) []corev1.VolumeMount {
	return dep.Spec.Template.Spec.Containers[0].VolumeMounts
}

//GetContainerRequestResources returns Request Requirements for first container in deployment
func GetContainerRequestResources(dep *appsv1.Deployment) corev1.ResourceList {
	return dep.Spec.Template.Spec.Containers[0].Resources.Requests
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

func ValidateClusterRoleRules(found *unstructured.Unstructured, want *unstructured.Unstructured) (*unstructured.Unstructured, bool) {
	log := log.FromContext(context.Background())
	desired, err := yaml.Marshal(want.Object["rules"])
	if err != nil {
		log.Error(err, "issue parsing desired object values")
	}
	current, err := yaml.Marshal(found.Object["rules"])
	if err != nil {
		log.Error(err, "issue parsing current object values")
	}

	if res := bytes.Compare(desired, current); res != 0 {
		// Return current object with adjusted spec, preserving metadata
		log.V(1).Info("ClusterRole doesn't match spec", "Want", want.Object["rules"], "Have", found.Object["rules"])
		found.Object["rules"] = want.Object["rules"]
		return found, true
	}
	return nil, false
}

func GetTestImages() []string {
	return []string{"registration_operator", "openshift_hive", "multicloud_manager",
		"managedcluster_import_controller", "registration", "work"}
}
