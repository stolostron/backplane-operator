// Copyright Contributors to the Open Cluster Management project
package utils

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	// DefaultLocalClusterName name of the hub cluster managedcluster resource
	DefaultLocalClusterName = "local-cluster"

	// AnnotationNodeSelector key name of nodeSelector annotation synced from mch
	AnnotationNodeSelector = "open-cluster-management/nodeSelector"
	AnnotationTolerations  = "open-cluster-management/tolerations"
)

// NewManagedCluster returns an unstructured ManagedCluster (cluster.open-cluster-management.io/v1)
// with the given name, configured as a local hub cluster. The resource has
// local-cluster, cloud, vendor, and velero backup-exclusion labels set, and
// spec.hubAcceptsClient is true.
func NewManagedCluster(name string) *unstructured.Unstructured {
	managedCluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": name,
				"labels": map[string]interface{}{
					"local-cluster":                 "true",
					"cloud":                         "auto-detect",
					"vendor":                        "auto-detect",
					"velero.io/exclude-from-backup": "true",
				},
			},
			"spec": map[string]interface{}{
				"hubAcceptsClient": true,
			},
		},
	}
	return managedCluster
}

// NewLocalNamespace returns a corev1.Namespace with the given name and no
// labels or annotations, intended for use as the local cluster's namespace.
func NewLocalNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}
