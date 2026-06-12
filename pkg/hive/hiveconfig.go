// Copyright Contributors to the Open Cluster Management project

// Package hive manages Hive integration for cluster provisioning.
//
// This package handles HiveConfig CR creation and configuration
// for OpenShift cluster provisioning capabilities.
package hive

import (
	v1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/utils"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func HiveConfig(bpc *v1.MultiClusterEngine) *unstructured.Unstructured {

	cm := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "HiveConfig",
			"metadata": map[string]interface{}{
				"name": "hive",
			},
			"spec": map[string]interface{}{},
		},
	}

	utils.AddBackplaneConfigLabels(cm, bpc.GetName())

	return cm
}
