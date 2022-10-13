// Copyright Contributors to the Open Cluster Management project

package renderer

import (
	v1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/utils"

	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	// addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
)

func RenderHypershiftAddon(mce *v1.MultiClusterEngine) (*unstructured.Unstructured, error) {
	// addon := addonv1alpha1.ManagedClusterAddOn{
	// 	ObjectMeta: metav1.ObjectMeta{
	// 		Name:      "hypershift-addon",
	// 		Namespace: "local-cluster",
	// 	},
	// 	Spec: addonv1alpha1.ManagedClusterAddOnSpec{
	// 		InstallNamespace: "open-cluster-management-agent-addon",
	// 	},
	// }
	// content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(addon)
	// if err != nil {
	// 	return nil, err
	// }

	// addonUnstructured := &unstructured.Unstructured{Object: content}

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

	return addon, nil
}
