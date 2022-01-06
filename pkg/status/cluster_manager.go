// Copyright Contributors to the Open Cluster Management project
package status

import (
	"context"
	"fmt"

	bpv1alpha1 "github.com/stolostron/backplane-operator/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DeploymentStatus fulfills the StatusReporter interface for deployments
type ClusterManagerStatus struct {
	types.NamespacedName
}

func (cms ClusterManagerStatus) GetName() string {
	return cms.Name
}

func (cms ClusterManagerStatus) GetNamespace() string {
	return ""
}

func (cms ClusterManagerStatus) GetKind() string {
	return "ClusterManager"
}

// Converts a ClusterManager's status to a backplane component status
func (cms ClusterManagerStatus) Status(k8sClient client.Client) bpv1alpha1.ComponentCondition {
	cm := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "operator.open-cluster-management.io/v1",
			"kind":       "ClusterManager",
			"metadata": map[string]interface{}{
				"name":      cms.GetName(),
				"namespace": "",
			},
		},
	}
	err := k8sClient.Get(context.TODO(), cms.NamespacedName, cm)
	if err != nil && !apierrors.IsNotFound(err) {
		fmt.Println("Err getting cluster manager", err)
		return unknownStatus(cms.GetName(), cms.GetKind())
	} else if apierrors.IsNotFound(err) {
		return unknownStatus(cms.GetName(), cms.GetKind())
	}

	return mapClusterManager(cm)
}

func mapClusterManager(cm *unstructured.Unstructured) bpv1alpha1.ComponentCondition {
	if cm == nil {
		return unknownStatus(cm.GetName(), cm.GetKind())
	}

	conditions, ok, err := unstructured.NestedSlice(cm.UnstructuredContent(), "status", "conditions")
	if !ok || err != nil {
		return unknownStatus(cm.GetName(), cm.GetKind())
	}

	componentCondition := bpv1alpha1.ComponentCondition{}

	for _, condition := range conditions {
		statusCondition, ok := condition.(map[string]interface{})
		if !ok {
			return unknownStatus(cm.GetName(), cm.GetKind())
		}

		sType, _ := statusCondition["type"].(string)
		status, _ := statusCondition["status"].(string)
		message, _ := statusCondition["message"].(string)
		reason, _ := statusCondition["reason"].(string)

		componentCondition = bpv1alpha1.ComponentCondition{
			Name:               cm.GetName(),
			Kind:               "ClusterManager",
			Type:               sType,
			Status:             metav1.ConditionStatus(status),
			LastUpdateTime:     metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             reason,
			Message:            message,
			Available:          false,
		}

		// Return condition with Applied = true
		if sType == "Applied" && status == "True" {
			componentCondition.Available = true
			return componentCondition
		}
	}

	// If no condition with applied true, then return last condition in list
	return componentCondition
}
