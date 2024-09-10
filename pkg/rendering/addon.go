// Copyright Contributors to the Open Cluster Management project

package renderer

import (
	v1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/utils"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func RenderHypershiftAddon(mce *v1.MultiClusterEngine) *unstructured.Unstructured {
	addon := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "addon.open-cluster-management.io/v1alpha1",
			"kind":       "ManagedClusterAddOn",
			"metadata": map[string]interface{}{
				"name":      "hypershift-addon",
				"namespace": "local-cluster",
			},
			"spec": map[string]interface{}{
				"installNamespace": "open-cluster-management-agent-addon",
			},
		},
	}

	utils.AddBackplaneConfigLabels(addon, mce.GetName())
	return addon
}
