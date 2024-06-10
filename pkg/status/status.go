// Copyright Contributors to the Open Cluster Management project
package status

import (
	bpv1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/version"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	prevAvailability = make(map[string]bool)
	log              = logf.Log.WithName("status")
)

type StatusTracker struct {
	Client     client.Client
	UID        string
	Components []StatusReporter
	Conditions []bpv1.MultiClusterEngineCondition
}

// Flush out any cached data being tracked, and assigns the tracker to a UID
func (sm *StatusTracker) Reset(uid string) {
	sm.UID = uid
	sm.Components = []StatusReporter{}
	sm.Conditions = []bpv1.MultiClusterEngineCondition{}
}

// Adds a StatusReporter to the list of statuses to watch
func (sm *StatusTracker) AddComponent(sr StatusReporter) {
	for _, c := range sm.Components {
		if c.GetName() == sr.GetName() &&
			c.GetNamespace() == sr.GetNamespace() &&
			c.GetKind() == sr.GetKind() {
			// Component already tracked
			return
		}
	}
	sm.Components = append(sm.Components, sr)
}

// Removes a StatusReporter from the list of statuses to watch
func (sm *StatusTracker) RemoveComponent(sr StatusReporter) {
	for i, c := range sm.Components {
		if c.GetName() == sr.GetName() &&
			c.GetNamespace() == sr.GetNamespace() &&
			c.GetKind() == sr.GetKind() {
			sm.Components = append(sm.Components[:i], sm.Components[i+1:]...)
			break
		}
	}
}

func (sm *StatusTracker) AddCondition(c bpv1.MultiClusterEngineCondition) {
	sm.Conditions = setCondition(sm.Conditions, c)
}

func (sm *StatusTracker) ReportStatus(mce bpv1.MultiClusterEngine) bpv1.MultiClusterEngineStatus {
	components := sm.reportComponents()

	// Infer available condition from component health
	if allComponentsReady(components) {
		sm.AddCondition(NewCondition(bpv1.MultiClusterEngineAvailable, metav1.ConditionTrue, ComponentsAvailableReason, ""))

	} else {
		sm.AddCondition(NewCondition(bpv1.MultiClusterEngineAvailable, metav1.ConditionFalse, ComponentsUnavailableReason, ""))
	}

	conditions := sm.reportConditions()
	phase := sm.reportPhase(mce, components, conditions)

	currentVersion := mce.Status.CurrentVersion
	if phase == bpv1.MultiClusterEnginePhaseAvailable {
		currentVersion = version.Version
	}

	return bpv1.MultiClusterEngineStatus{
		Components:     components,
		Conditions:     conditions,
		Phase:          phase,
		DesiredVersion: version.Version,
		CurrentVersion: currentVersion,
	}
}

func (sm *StatusTracker) reportComponents() []bpv1.ComponentCondition {
	components := []bpv1.ComponentCondition{}
	for _, c := range sm.Components {
		components = append(components, c.Status(sm.Client))
	}
	return components
}

func (sm *StatusTracker) reportConditions() []bpv1.MultiClusterEngineCondition {
	return sm.Conditions
}

func (sm *StatusTracker) reportPhase(mce bpv1.MultiClusterEngine, components []bpv1.ComponentCondition, conditions []bpv1.MultiClusterEngineCondition) bpv1.PhaseType {
	progress := getCondition(conditions, bpv1.MultiClusterEngineProgressing)

	for _, condition := range conditions {
		if condition.Reason == PausedReason {
			return bpv1.MultiClusterEnginePhasePaused
		}
	}

	// If operator isn't progressing show error phase
	if progress != nil && progress.Status == metav1.ConditionFalse {
		return bpv1.MultiClusterEnginePhaseError
	}

	// If deleting show uninstall phase
	if mce.GetDeletionTimestamp() != nil {
		return bpv1.MultiClusterEnginePhaseUninstalling
	}

	// If status isn't tracking anything show error phase
	if len(components) == 0 {
		return bpv1.MultiClusterEnginePhaseProgressing
	}

	// If status contains failure status return error
	if ok := ConditionPresentWithSubstring(conditions, string(bpv1.MultiClusterEngineComponentFailure)); ok {
		return bpv1.MultiClusterEnginePhaseError
	}

	// If a component isn't ready show progressing phase
	if !allComponentsReady(components) {
		return bpv1.MultiClusterEnginePhaseProgressing
	}

	return bpv1.MultiClusterEnginePhaseAvailable
}

func allComponentsReady(components []bpv1.ComponentCondition) bool {
	if len(components) == 0 {
		return false
	}

	// Track availability status.
	allAvailable := true

	for _, val := range components {
		if !val.Available {
			// Check if the component's availability status has changed since the last reconciliation.
			if prevStatus, exists := prevAvailability[val.Name]; !exists || prevStatus {
				// Log the information about the newly unavailable component
				log.Info("The component is not yet available.", "Kind", val.Kind, "Name", val.Name, "Reason", val.Reason)
			}

			// Update the previous availability status for this component
			prevAvailability[val.Name] = false
			allAvailable = false
		} else {
			// Check if the component's availability status has changed since the last reconciliation
			if prevStatus, exists := prevAvailability[val.Name]; !exists || !prevStatus {
				// Log the information about the newly available component
				log.Info("The component is now available.", "Kind", val.Kind, "Name", val.Name)
			}

			// Update the previous availability status for this component
			prevAvailability[val.Name] = true
		}
	}

	// Return the overall availability status
	return allAvailable
}

// StatusReporter is a resource that can report back a status
type StatusReporter interface {
	GetName() string
	GetNamespace() string
	GetKind() string
	Status(client.Client) bpv1.ComponentCondition
}

func unknownStatus(name, kind string) bpv1.ComponentCondition {
	return bpv1.ComponentCondition{
		Name:               name,
		Kind:               kind,
		Type:               "Unknown",
		Status:             metav1.ConditionUnknown,
		LastUpdateTime:     metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Reason:             "No conditions available",
		Message:            "No conditions available",
		Available:          false,
	}
}
