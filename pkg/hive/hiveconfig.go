// Copyright Contributors to the Open Cluster Management project

package hive

import (
	v1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/utils"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func HiveConfig(mce *v1.MultiClusterEngine) *unstructured.Unstructured {

	hc := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "HiveConfig",
			"metadata": map[string]interface{}{
				"name": "hive",
			},
			"spec": map[string]interface{}{},
		},
	}

	utils.AddBackplaneConfigLabels(hc, mce.GetName())
	return hc
}
