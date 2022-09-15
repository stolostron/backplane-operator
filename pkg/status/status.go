// Copyright Contributors to the Open Cluster Management project
package status

import (
	"errors"
	"fmt"
	"strings"

	bpv1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/version"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func (st *StatusTracker) AllComponentsReady() error {
	components := st.reportComponents()

	if len(components) == 0 {
		return errors.New("component statuses have been reported yet")
	}

	notReady := make([]string, 0, len(components))
	for _, component := range components {
		if !component.Available {
			notReady = append(notReady, component.Name)
		}
	}
	if len(notReady) > 0 {
		return fmt.Errorf("components not ready: %s", strings.Join(notReady, ", "))
	}

	return nil
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
	for _, val := range components {
		if !val.Available {
			return false
		}
	}
	return true
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
