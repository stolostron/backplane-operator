// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

// Package metrics registers and populates the Prometheus metrics that expose
// MultiClusterEngine health. The metrics are served on the operator's existing
// metrics endpoint through the controller-runtime registry, so they are
// scrapeable by the OpenShift built-in monitoring stack without requiring ACM
// Observability.
package metrics

import (
	"sync"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"

	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	// PhaseMetricName is the name of the MultiClusterEngine phase gauge.
	PhaseMetricName = "mce_phase"
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

// mceMetricsMu serializes updates to every MultiClusterEngine metric family and to the
// tracking maps below, so a published series always matches the tracked state.
var mceMetricsMu sync.Mutex

// unknownPhases tracks the last unknown phase published for each MCE (by name),
// so it can be removed on the next phase change.
var unknownPhases = map[string]string{}

// publishedPhases tracks the last phase published for each MCE (by name) so
// SetMCEPhase can report whether the gauge actually changed.
var publishedPhases = map[string]string{}

func init() {
	ctrlmetrics.Registry.MustRegister(mcePhase)
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

// DeleteMCEMetrics removes every metric series belonging to the given
// MultiClusterEngine so that a deleted MCE does not leave stale series behind.
// All MultiClusterEngine-scoped metrics must be cleaned up here so callers only
// need a single entry point. It reports whether any series were removed, so
// callers can log cleanup once instead of on every teardown reconcile.
func DeleteMCEMetrics(name string) bool {
	mceMetricsMu.Lock()
	defer mceMetricsMu.Unlock()

	_, phaseExisted := publishedPhases[name]
	existed := phaseExisted

	delete(unknownPhases, name)
	delete(publishedPhases, name)

	partialLabels := prometheus.Labels{
		"name": name,
	}
	mcePhase.DeletePartialMatch(partialLabels)
	return existed
}

func boolToFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
