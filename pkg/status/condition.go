// Copyright Contributors to the Open Cluster Management project
package status

import (
	v1 "github.com/stolostron/backplane-operator/api/v1"
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
	// OldComponentRemovedReason is added when the hub calls delete on an old resource
	OldComponentRemovedReason = "OldResourceDeleted"
	// OldComponentNotRemovedReason is added when a component the hub is trying to delete has not been removed successfully
	OldComponentNotRemovedReason = "OldResourceDeleteFailed"
	// AllOldComponentsRemovedReason is added when the hub successfully prunes all old resources
	AllOldComponentsRemovedReason = "AllOldResourcesDeleted"
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
func NewCondition(condType v1.MultiClusterEngineConditionType, status metav1.ConditionStatus, reason, message string) v1.MultiClusterEngineCondition {
	return v1.MultiClusterEngineCondition{
		Type:               condType,
		Status:             status,
		LastUpdateTime:     metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// SetCondition sets the status condition. It either overwrites the existing one or creates a new one.
func setCondition(conditions []v1.MultiClusterEngineCondition, c v1.MultiClusterEngineCondition) []v1.MultiClusterEngineCondition {
	currentCond := GetCondition(conditions, c.Type)
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
func GetCondition(conditions []v1.MultiClusterEngineCondition, condType v1.MultiClusterEngineConditionType) *v1.MultiClusterEngineCondition {
	for i := range conditions {
		c := conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

// filterOutCondition returns a new slice of hub conditions without conditions with the provided type.
func filterOutCondition(conditions []v1.MultiClusterEngineCondition, condType v1.MultiClusterEngineConditionType) []v1.MultiClusterEngineCondition {
	var newConditions []v1.MultiClusterEngineCondition
	for _, c := range conditions {
		if c.Type == condType {
			continue
		}
		newConditions = append(newConditions, c)
	}
	return newConditions
}
