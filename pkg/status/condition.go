// Copyright Contributors to the Open Cluster Management project
package status

import (
	"strings"

	v1 "github.com/stolostron/backplane-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ApplyFailedReason is added when the hub fails to apply a resource
	ApplyFailedReason = "FailedApplyingComponent"
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
	// ManagedClusterTerminatingReason is added when a managed cluster has been deleted and
	// is waiting to be finalized
	ManagedClusterTerminatingReason = "ManagedClusterTerminating"
	// NamespaceTerminatingReason is added when a managed cluster's namespace has been deleted and
	// is waiting to be finalized
	NamespaceTerminatingReason = "ManagedClusterNamespaceTerminating"
	// PausedReason is added when the multiclusterengine is paused
	PausedReason = "Paused"
	// UnsupportedConfigReason means the resource can't be deployed as intended based on current configuration
	// settings
	UnsupportedConfigReason = "UnsupportedConfiguration"
	// ComponentDisabledReason means the component has been specifically disabled by user in config
	ComponentDisabledReason = "ComponentDisabled"
	// ComponentUpdatingReason is added when the hub is actively updating a component resource
	ComponentsUpdatingReason = "UpdatingComponentResource"
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
	currentCond := getCondition(conditions, c.Type)
	if currentCond != nil && currentCond.Status == c.Status && currentCond.Reason == c.Reason && currentCond.Message == c.Message {
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
func getCondition(conditions []v1.MultiClusterEngineCondition, condType v1.MultiClusterEngineConditionType) *v1.MultiClusterEngineCondition {
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

// FilterOutConditionWithSubString returns a new slice of hub conditions without conditions with the provided type.
func FilterOutConditionWithSubString(conditions []v1.MultiClusterEngineCondition, condType v1.MultiClusterEngineConditionType) []v1.MultiClusterEngineCondition {
	var newConditions []v1.MultiClusterEngineCondition
	for _, c := range conditions {
		if strings.Contains(string(c.Type), string(condType)) {
			continue
		}
		newConditions = append(newConditions, c)
	}
	return newConditions
}

/*
ConditionPresentWithSubstring returns true or false if a MultiClusterEngineCondition is
present with a target substring.
*/
func ConditionPresentWithSubstring(conditions []v1.MultiClusterEngineCondition, substring string) bool {
	for _, c := range conditions {
		if strings.Contains(string(c.Type), substring) {
			return true
		}
	}
	return false
}
