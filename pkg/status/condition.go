// Copyright Contributors to the Open Cluster Management project
package status

import (
	bpv1alpha1 "github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ComponentsAvailableReason is when all desired components are running successfully
	ComponentsAvailableReason = "ComponentsAvailable"
	// ComponentsUnavailableReason is when one or more components are in an unready state
	ComponentsUnavailableReason = "ComponentsUnavailable"
	// DeployFailedReason is added when the hub fails to deploy a resource
	DeployFailedReason = "FailedDeployingComponent"
	// DeploySuccessReason is when all component have been deployed
	DeploySuccessReason = "ComponentsDeployed"
	// RequirementsNotMetReason is when there is something missing or misconfigured
	// that is preventing progress
	RequirementsNotMetReason = "RequirementsNotMet"
	// DeleteTimestampReason means the resource is schedule for deletion with a DeletionTimestamp present
	DeleteTimestampReason = "DeletionTimestampPresent"
	// WaitingForResourceReason means the reconciler is waiting on a resource before it can progress
	WaitingForResourceReason = "WaitingForResource"
	// PausedReason is added when the multiclusterengine is paused
	PausedReason = "Paused"
)

// NewCondition creates a new condition.
func NewCondition(condType bpv1alpha1.MultiClusterEngineConditionType, status metav1.ConditionStatus, reason, message string) bpv1alpha1.MultiClusterEngineCondition {
	return bpv1alpha1.MultiClusterEngineCondition{
		Type:               condType,
		Status:             status,
		LastUpdateTime:     metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// SetCondition sets the status condition. It either overwrites the existing one or creates a new one.
func setCondition(conditions []bpv1alpha1.MultiClusterEngineCondition, c bpv1alpha1.MultiClusterEngineCondition) []bpv1alpha1.MultiClusterEngineCondition {
	currentCond := getCondition(conditions, c.Type)
	if currentCond != nil && currentCond.Status == c.Status && currentCond.Reason == c.Reason {
		// Condition already present
		return conditions
	}

	// Do not update lastTransitionTime if the status of the condition doesn't change.
	if currentCond != nil && currentCond.Status == c.Status {
		c.LastTransitionTime = currentCond.LastTransitionTime
	}

	newConditions := filterOutCondition(conditions, c.Type)
	return append(newConditions, c)
}

// GetCondition returns the condition you're looking for by type
func getCondition(conditions []bpv1alpha1.MultiClusterEngineCondition, condType bpv1alpha1.MultiClusterEngineConditionType) *bpv1alpha1.MultiClusterEngineCondition {
	for i := range conditions {
		c := conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

// filterOutCondition returns a new slice of hub conditions without conditions with the provided type.
func filterOutCondition(conditions []bpv1alpha1.MultiClusterEngineCondition, condType bpv1alpha1.MultiClusterEngineConditionType) []bpv1alpha1.MultiClusterEngineCondition {
	var newConditions []bpv1alpha1.MultiClusterEngineCondition
	for _, c := range conditions {
		if c.Type == condType {
			continue
		}
		newConditions = append(newConditions, c)
	}
	return newConditions
}
