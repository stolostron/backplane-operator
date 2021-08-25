// Copyright Contributors to the Open Cluster Management project
package status

import (
	bpv1alpha1 "github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type StatusTracker struct {
	Client     client.Client
	Components []StatusReporter
	Conditions []bpv1alpha1.BackplaneCondition
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

func (sm *StatusTracker) AddCondition(c bpv1alpha1.BackplaneCondition) {
	sm.Conditions = setCondition(sm.Conditions, c)
}

func (sm *StatusTracker) ReportStatus() bpv1alpha1.BackplaneConfigStatus {
	components := sm.reportComponents()
	phase := sm.reportPhase(components)
	if phase == bpv1alpha1.BackplanePhaseAvailable {
		sm.AddCondition(NewCondition(bpv1alpha1.BackplaneAvailable, metav1.ConditionTrue, ComponentsAvailableReason, ""))
	} else {
		sm.AddCondition(NewCondition(bpv1alpha1.BackplaneAvailable, metav1.ConditionFalse, ComponentsUnavailableReason, ""))
	}
	conditions := sm.reportConditions()

	return bpv1alpha1.BackplaneConfigStatus{
		Components: components,
		Conditions: conditions,
		Phase:      phase,
	}
}

func (sm *StatusTracker) reportComponents() []bpv1alpha1.ComponentCondition {
	components := []bpv1alpha1.ComponentCondition{}
	for _, c := range sm.Components {
		components = append(components, c.Status(sm.Client))
	}
	return components
}

func (sm *StatusTracker) reportConditions() []bpv1alpha1.BackplaneCondition {
	return sm.Conditions
}

func (sm *StatusTracker) reportPhase(cc []bpv1alpha1.ComponentCondition) bpv1alpha1.PhaseType {
	if len(cc) == 0 {
		return bpv1alpha1.BackplanePhaseError
	}
	for _, val := range cc {
		if !val.Available {
			return bpv1alpha1.BackplanePhaseProgressing
		}
	}
	return bpv1alpha1.BackplanePhaseAvailable
}

// StatusReporter is a resource that can report back a status
type StatusReporter interface {
	GetName() string
	GetNamespace() string
	GetKind() string
	Status(client.Client) bpv1alpha1.ComponentCondition
}

func unknownStatus(name, kind string) bpv1alpha1.ComponentCondition {
	return bpv1alpha1.ComponentCondition{
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
