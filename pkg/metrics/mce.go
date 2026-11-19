// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

// Package metrics registers and populates the Prometheus metrics that expose
// MultiClusterEngine health. The metrics are served on the operator's existing
// metrics endpoint through the controller-runtime registry, so they are
// scrapeable by the OpenShift built-in monitoring stack without requiring ACM
// Observability.
package metrics

import (
	"maps"
	"sync"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	// PhaseMetricName is the name of the MultiClusterEngine phase gauge.
	PhaseMetricName = "mce_phase"

	// ComponentConditionMetricName is the name of the per-component health gauge.
	ComponentConditionMetricName = "mce_component_conditions"
)

// allMCEPhases lists every phase a MultiClusterEngine can report. Publishing a
// series for each phase (0 for inactive phases) lets consumers alert on a
// specific phase without first testing for series presence.
var allMCEPhases = []backplanev1.PhaseType{
	backplanev1.MultiClusterEnginePhaseProgressing,
	backplanev1.MultiClusterEnginePhasePaused,
	backplanev1.MultiClusterEnginePhaseAvailable,
	backplanev1.MultiClusterEnginePhaseUninstalling,
	backplanev1.MultiClusterEnginePhaseError,
	backplanev1.MultiClusterEnginePhaseUnimplemented,
	backplanev1.MultiClusterEnginePhaseUpdating,
}

// mcePhase is the mce_phase gauge. Each MultiClusterEngine has one series per
// entry in allMCEPhases, identified by the name and phase labels; exactly one of
// those series is 1 (the current phase) and the rest are 0.
var mcePhase = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: PhaseMetricName,
		Help: "Current phase of the MultiClusterEngine. The active phase reports 1; all other phases report 0.",
	},
	[]string{"name", "phase"},
)

// mceComponentConditions is the mce_component_conditions gauge. Each component
// in the MultiClusterEngine status contributes one series, labelled with the raw
// Kubernetes kind so it can be joined against kube-state-metrics.
var mceComponentConditions = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: ComponentConditionMetricName,
		Help: "Condition of each MultiClusterEngine component. 1 = the condition status is True, 0 = False or Unknown.",
	},
	[]string{"name", "component", "kind", "condition"},
)

// componentConditionSeries identifies a single mce_component_conditions series
// for an MCE. Tracking the label set lets a series be deleted when its component is
// disabled, paused, or removed.
type componentConditionSeries struct {
	component string
	kind      string
	condition string
}

// mceMetricsMu serializes updates to every MultiClusterEngine metric family and to the
// tracking maps below, so a published series always matches the tracked state.
var mceMetricsMu sync.Mutex

// unknownPhases tracks the last unknown phase published for each MCE (by name),
// so it can be removed on the next phase change.
var unknownPhases = map[string]string{}

// publishedPhases tracks the last phase published for each MCE (by name) so
// SetMCEPhase can report whether the gauge actually changed.
var publishedPhases = map[string]string{}

// publishedComponentConditions tracks the condition series published for each MCE
// so series belonging to components that are no longer reported can be removed.
var publishedComponentConditions = map[string]map[componentConditionSeries]float64{}

func init() {
	ctrlmetrics.Registry.MustRegister(mcePhase)
	ctrlmetrics.Registry.MustRegister(mceComponentConditions)
}

// SetMCEPhase publishes the phase of the given MultiClusterEngine, reporting 1 for
// the active phase and 0 for every other known phase. It reports whether the
// active phase changed, so callers can log transitions instead of every update.
func SetMCEPhase(name string, phase backplanev1.PhaseType) bool {
	mceMetricsMu.Lock()
	defer mceMetricsMu.Unlock()

	changed := publishedPhases[name] != string(phase)
	publishedPhases[name] = string(phase)

	known := false
	for _, candidate := range allMCEPhases {
		if candidate == phase {
			known = true
		}
		mcePhase.WithLabelValues(name, string(candidate)).Set(boolToFloat(candidate == phase))
	}

	if known {
		// Drop any previously reported unknown phase so it cannot stay active
		// alongside the known phase.
		if previous, ok := unknownPhases[name]; ok {
			mcePhase.DeleteLabelValues(name, previous)
			delete(unknownPhases, name)
		}
		return changed
	}

	// A phase added to the API but not to allMCEPhases would otherwise leave every
	// series at 0. Report it as active instead of losing the signal, replacing any
	// different unknown phase previously reported for this MCE.
	if previous, ok := unknownPhases[name]; ok && previous != string(phase) {
		mcePhase.DeleteLabelValues(name, previous)
	}
	unknownPhases[name] = string(phase)
	mcePhase.WithLabelValues(name, string(phase)).Set(1)
	return changed
}

// SetMCEComponentMetrics publishes the health of every component reported in the
// MultiClusterEngine status. Components is a slice of ComponentCondition, each
// carrying its own Name, Kind, Type (condition type), and Status.
//
// Health series for components that are no longer reported are removed, so a
// disabled, paused, or removed component leaves no stale series behind. It reports
// whether any published series changed, so callers can log transitions instead of
// every update.
func SetMCEComponentMetrics(name string, components []backplanev1.ComponentCondition) bool {
	mceMetricsMu.Lock()
	defer mceMetricsMu.Unlock()

	desiredConditions := make(map[componentConditionSeries]float64, len(components))
	for _, component := range components {
		series := componentConditionSeries{
			component: component.Name,
			kind:      component.Kind,
			condition: component.Type,
		}
		// Guard against an empty condition label.
		if series.condition == "" {
			series.condition = "Available"
		}
		desiredConditions[series] = conditionStatusValue(component.Status)
	}

	changed := false
	previousConditions := publishedComponentConditions[name]
	for series := range previousConditions {
		if _, ok := desiredConditions[series]; !ok {
			mceComponentConditions.DeleteLabelValues(name, series.component, series.kind, series.condition)
		}
	}
	for series, value := range desiredConditions {
		mceComponentConditions.WithLabelValues(name, series.component, series.kind, series.condition).Set(value)
	}
	if !maps.Equal(previousConditions, desiredConditions) {
		changed = true
	}
	if len(desiredConditions) == 0 {
		delete(publishedComponentConditions, name)
	} else {
		publishedComponentConditions[name] = desiredConditions
	}

	return changed
}

// DeleteMCEMetrics removes every metric series belonging to the given
// MultiClusterEngine so that a deleted MCE does not leave stale series behind.
// All MultiClusterEngine-scoped metrics must be cleaned up here so callers only
// need a single entry point. It reports whether any series were removed, so
// callers can log cleanup once instead of on every teardown reconcile.
func DeleteMCEMetrics(name string) bool {
	mceMetricsMu.Lock()
	defer mceMetricsMu.Unlock()

	_, phaseExisted := publishedPhases[name]
	_, conditionsExisted := publishedComponentConditions[name]
	existed := phaseExisted || conditionsExisted

	delete(unknownPhases, name)
	delete(publishedPhases, name)
	delete(publishedComponentConditions, name)

	partialLabels := prometheus.Labels{
		"name": name,
	}
	mcePhase.DeletePartialMatch(partialLabels)
	mceComponentConditions.DeletePartialMatch(partialLabels)
	return existed
}

// conditionStatusValue maps a condition status to the gauge value. Only True
// reports healthy; False and Unknown both report 0.
func conditionStatusValue(status metav1.ConditionStatus) float64 {
	return boolToFloat(status == metav1.ConditionTrue)
}

func boolToFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
