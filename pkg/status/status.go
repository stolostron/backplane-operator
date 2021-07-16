package status

import (
	"context"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	"github.com/open-cluster-management/backplane-operator/pkg/version"
)

type StatusWatcher struct {
	BackplaneConfig *v1alpha1.BackplaneConfig
	Deployments     *appsv1.DeploymentList
	OriginalStatus  v1alpha1.BackplaneConfigStatus
}

var unknownStatus = v1alpha1.StatusCondition{
	Type:               "Unknown",
	Status:             metav1.ConditionUnknown,
	LastUpdateTime:     metav1.Now(),
	LastTransitionTime: metav1.Now(),
	Reason:             "No conditions available",
	Message:            "No conditions available",
	Available:          false,
}

const (
	// ComponentsAvailableReason is added in a hub when all desired components are
	// installed successfully
	ComponentsAvailableReason = "ComponentsAvailable"
	// ComponentsUnavailableReason is added in a hub when one or more components are
	// in an unready state
	ComponentsUnavailableReason = "ComponentsUnavailable"
	// NewComponentReason is added when the hub creates a new install resource successfully
	NewComponentReason = "NewResourceCreated"
	// DeployFailedReason is added when the hub fails to deploy a resource
	DeployFailedReason = "FailedDeployingComponent"
	// OldComponentRemovedReason is added when the hub calls delete on an old resource
	OldComponentRemovedReason = "OldResourceDeleted"
	// OldComponentNotRemovedReason is added when a component the hub is trying to delete has not been removed successfully
	OldComponentNotRemovedReason = "OldResourceDeleteFailed"
	// AllOldComponentsRemovedReason is added when the hub successfully prunes all old resources
	AllOldComponentsRemovedReason = "AllOldResourcesDeleted"
	// CertManagerReason is added when the hub is waiting for cert manager CRDs to come up
	CertManagerReason = "CertManagerInitializing"
	// DeleteTimestampReason is added when the backplaneConfig has been targeted for delete
	DeleteTimestampReason = "DeletionTimestampPresent"
	// PausedReason is added when the backplaneConfig is paused
	PausedReason = "Paused"
	// ResumedReason is added when the backplaneConfig is resumed
	ResumedReason = "Resumed"
	// ReconcileReason is added when the backplaneConfig is actively reconciling
	ReconcileReason = "Reconciling"
	// HelmReleaseTerminatingReason is added when the backplaneConfig is waiting for the removal
	// of helm releases
	HelmReleaseTerminatingReason = "HelmReleaseTerminating"
	// ManagedClusterTerminatingReason is added when a managed cluster has been deleted and
	// is waiting to be finalized
	ManagedClusterTerminatingReason = "ManagedClusterTerminating"
	// NamespaceTerminatingReason is added when a managed cluster's namespace has been deleted and
	// is waiting to be finalized
	NamespaceTerminatingReason = "ManagedClusterNamespaceTerminating"
	// ResourceRenderReason is added when an error occurs while rendering a deployable resource
	ResourceRenderReason = "FailedRenderingResource"
)

func (s *StatusWatcher) getDeployments() []types.NamespacedName {
	return []types.NamespacedName{
		{Name: "hive-operator", Namespace: s.BackplaneConfig.Namespace},
		{Name: "cluster-manager", Namespace: s.BackplaneConfig.Namespace},
	}
}

func (s *StatusWatcher) SyncStatus() (v1alpha1.BackplaneConfigStatus, bool) {
	log := log.FromContext(context.Background())

	newStatus := s.calculateStatus()
	if reflect.DeepEqual(newStatus, s.OriginalStatus) {
		log.Info("Status hasn't changed")
		return newStatus, false
	}
	log.Info("Status has changed")
	return newStatus, true
}

func (s *StatusWatcher) calculateStatus() v1alpha1.BackplaneConfigStatus {
	components := s.getComponentStatuses()
	status := v1alpha1.BackplaneConfigStatus{
		CurrentVersion: s.BackplaneConfig.Status.CurrentVersion,
		DesiredVersion: version.Get().Version,
		Components:     components,
	}

	// Set current version
	successful := allComponentsSuccessful(components)
	if successful {
		status.CurrentVersion = version.Get().Version
	}

	// Copy conditions one by one to not affect original object
	conditions := s.BackplaneConfig.Status.Conditions
	for i := range conditions {
		status.Conditions = append(status.Conditions, conditions[i])
	}

	// Update hub conditions
	if successful {
		// don't label as complete until component pruning succeeds
		if !pruning(status) {
			available := NewCondition(v1alpha1.Complete, v1.ConditionTrue, ComponentsAvailableReason, "All hub components ready.")
			SetCondition(&status, *available)
		} else {
			// only add unavailable status if complete status already present
			if ConditionPresent(status, v1alpha1.Complete) {
				unavailable := NewCondition(v1alpha1.Complete, v1.ConditionFalse, OldComponentNotRemovedReason, "Not all components successfully pruned.")
				SetCondition(&status, *unavailable)
			}
		}
	} else {
		// hub is progressing unless otherwise specified
		if !ConditionPresent(status, v1alpha1.Progressing) {
			progressing := NewCondition(v1alpha1.Progressing, v1.ConditionTrue, ReconcileReason, "Hub is reconciling.")
			SetCondition(&status, *progressing)
		}
		// only add unavailable status if complete status already present
		if ConditionPresent(status, v1alpha1.Complete) {
			unavailable := NewCondition(v1alpha1.Complete, v1.ConditionFalse, ComponentsUnavailableReason, "Not all hub components ready.")
			SetCondition(&status, *unavailable)
		}
	}

	// Set overall phase
	isHubMarkedToBeDeleted := s.BackplaneConfig.GetDeletionTimestamp() != nil
	if isHubMarkedToBeDeleted {
		// Hub cleaning up
		status.Phase = v1alpha1.Uninstalling
	} else {
		status.Phase = aggregatePhase(status)
	}

	return status
}

func (s *StatusWatcher) getComponentStatuses() map[string]v1alpha1.StatusCondition {
	components := s.newComponentList()
	for _, d := range s.Deployments.Items {
		if _, ok := components[d.Name]; ok {
			components[d.Name] = mapDeployment(&d)
		}
	}
	return components
}

func (s *StatusWatcher) newComponentList() map[string]v1alpha1.StatusCondition {
	components := make(map[string]v1alpha1.StatusCondition)
	for _, d := range s.getDeployments() {
		components[d.Name] = unknownStatus
	}
	return components
}

func mapDeployment(ds *appsv1.Deployment) v1alpha1.StatusCondition {
	if len(ds.Status.Conditions) < 1 {
		return unknownStatus
	}

	dcs := latestDeployCondition(ds.Status.Conditions)
	ret := v1alpha1.StatusCondition{
		Kind:               "Deployment",
		Type:               string(dcs.Type),
		Status:             metav1.ConditionStatus(string(dcs.Status)),
		LastUpdateTime:     dcs.LastUpdateTime,
		LastTransitionTime: dcs.LastTransitionTime,
		Reason:             dcs.Reason,
		Message:            dcs.Message,
	}
	if successfulDeploy(ds) {
		ret.Available = true
		ret.Message = ""
	}

	// Because our definition of success is different than the deployment's it is possible we indicate failure
	// despite an available deployment present. To avoid confusion we should show a different status.
	if dcs.Type == appsv1.DeploymentAvailable && dcs.Status == corev1.ConditionTrue && ret.Available == false {
		sub := progressingDeployCondition(ds.Status.Conditions)
		ret = v1alpha1.StatusCondition{
			Kind:               "Deployment",
			Type:               string(sub.Type),
			Status:             metav1.ConditionStatus(string(sub.Status)),
			LastUpdateTime:     sub.LastUpdateTime,
			LastTransitionTime: sub.LastTransitionTime,
			Reason:             sub.Reason,
			Message:            sub.Message,
			Available:          false,
		}
	}

	return ret
}

func latestDeployCondition(conditions []appsv1.DeploymentCondition) appsv1.DeploymentCondition {
	if len(conditions) < 1 {
		return appsv1.DeploymentCondition{}
	}
	latest := conditions[0]
	for i := range conditions {
		if conditions[i].LastTransitionTime.Time.After(latest.LastTransitionTime.Time) {
			latest = conditions[i]
		}
	}
	return latest
}

func successfulDeploy(d *appsv1.Deployment) bool {
	for _, c := range d.Status.Conditions {
		if c.Type == appsv1.DeploymentAvailable && c.Status == corev1.ConditionFalse {
			return false
		}
	}

	if d.Status.UnavailableReplicas > 0 {
		return false
	}

	return true
	// latest := latestDeployCondition(d.Status.Conditions)
}

func successfulComponent(sc v1alpha1.StatusCondition) bool { return sc.Available }

// allComponentsSuccessful returns true if all components are successful, otherwise false
func allComponentsSuccessful(components map[string]v1alpha1.StatusCondition) bool {
	for _, val := range components {
		if !successfulComponent(val) {
			return false
		}
	}
	return true
}

func progressingDeployCondition(conditions []appsv1.DeploymentCondition) appsv1.DeploymentCondition {
	progressing := appsv1.DeploymentCondition{}
	for i := range conditions {
		if conditions[i].Type == appsv1.DeploymentProgressing {
			progressing = conditions[i]
		}
	}
	return progressing
}

// GetCondition returns the condition you're looking for or nil.
func GetCondition(status v1alpha1.BackplaneConfigStatus, condType v1alpha1.ConditionType) *v1alpha1.Condition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

// pruning returns true when the status reports hub is in the process of pruning
func pruning(status v1alpha1.BackplaneConfigStatus) bool {
	progressingCondition := GetCondition(status, v1alpha1.Progressing)
	if progressingCondition != nil {
		if progressingCondition.Reason == OldComponentRemovedReason || progressingCondition.Reason == OldComponentNotRemovedReason {
			return true
		}
	}
	return false
}

// NewCondition creates a new condition.
func NewCondition(condType v1alpha1.ConditionType, status v1.ConditionStatus, reason, message string) *v1alpha1.Condition {
	return &v1alpha1.Condition{
		Type:               condType,
		Status:             status,
		LastUpdateTime:     metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// SetCondition sets the status condition. It either overwrites the existing one or creates a new one.
func SetCondition(status *v1alpha1.BackplaneConfigStatus, condition v1alpha1.Condition) {
	currentCond := GetCondition(*status, condition.Type)
	if currentCond != nil && currentCond.Status == condition.Status && currentCond.Reason == condition.Reason {
		return
	}
	// Do not update lastTransitionTime if the status of the condition doesn't change.
	if currentCond != nil && currentCond.Status == condition.Status {
		condition.LastTransitionTime = currentCond.LastTransitionTime
	}
	newConditions := filterOutCondition(status.Conditions, condition.Type)
	status.Conditions = append(newConditions, condition)
}

// filterOutCondition returns a new slice of hub conditions without conditions with the provided type.
func filterOutCondition(conditions []v1alpha1.Condition, condType v1alpha1.ConditionType) []v1alpha1.Condition {
	var newConditions []v1alpha1.Condition
	for _, c := range conditions {
		if c.Type == condType {
			continue
		}
		newConditions = append(newConditions, c)
	}
	return newConditions
}

// ConditionPresent indicates if the condition is present and equal to the given status.
func ConditionPresent(status v1alpha1.BackplaneConfigStatus, conditionType v1alpha1.ConditionType) bool {
	for _, condition := range status.Conditions {
		if condition.Type == conditionType {
			return true
		}
	}
	return false
}

// aggregatePhase calculates overall HubPhaseType based on hub status. This does NOT account for
// a hub in the process of deletion.
func aggregatePhase(status v1alpha1.BackplaneConfigStatus) v1alpha1.PhaseType {
	successful := allComponentsSuccessful(status.Components)
	if successful {
		if pruning(status) {
			// hub is in pruning phase
			return v1alpha1.Pending
		}

		// Hub running
		return v1alpha1.Running
	}

	switch cv := status.CurrentVersion; {
	case cv == "":
		// Hub has not reached success for first time
		return v1alpha1.Installing
	case cv != version.Get().Version:
		// Hub has not completed upgrade to newest version
		return v1alpha1.Updating
	default:
		// Hub has reached desired version, but degraded
		return v1alpha1.Pending
	}

}
