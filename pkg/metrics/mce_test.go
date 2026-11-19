// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package metrics

import (
	"testing"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"

	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// gatherGaugeValue retrieves the value of a specific gauge metric from the
// controller-runtime registry. It returns (value, found).
func gatherGaugeValue(t *testing.T, metricName string, labels map[string]string) (float64, bool) {
	t.Helper()
	families, err := ctrlmetrics.Registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != metricName {
			continue
		}
		for _, m := range family.GetMetric() {
			labelMap := make(map[string]string)
			for _, lp := range m.GetLabel() {
				labelMap[lp.GetName()] = lp.GetValue()
			}
			match := true
			for k, v := range labels {
				if labelMap[k] != v {
					match = false
					break
				}
			}
			if match && len(labelMap) == len(labels) {
				return m.GetGauge().GetValue(), true
			}
		}
	}
	return 0, false
}

// countSeriesForMetric counts how many series exist for a given metric name
// that match the given partial labels (all given labels must match, but extra
// labels are allowed).
func countSeriesForMetric(t *testing.T, metricName string, partialLabels map[string]string) int {
	t.Helper()
	families, err := ctrlmetrics.Registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	count := 0
	for _, family := range families {
		if family.GetName() != metricName {
			continue
		}
		for _, m := range family.GetMetric() {
			labelMap := make(map[string]string)
			for _, lp := range m.GetLabel() {
				labelMap[lp.GetName()] = lp.GetValue()
			}
			match := true
			for k, v := range partialLabels {
				if labelMap[k] != v {
					match = false
					break
				}
			}
			if match {
				count++
			}
		}
	}
	return count
}

// resetMCETracking clears all tracking maps so each test starts with a clean
// state. Must be called under the test, not in init(), since tests run serially.
func resetMCETracking() {
	mceMetricsMu.Lock()
	defer mceMetricsMu.Unlock()

	// Clear tracking maps.
	unknownPhases = map[string]string{}
	publishedPhases = map[string]string{}

	// Reset gauge vecs so series from prior tests don't leak.
	mcePhase.Reset()
}

// --- SetMCEPhase tests ---

func TestSetMCEPhase_InitialPublish(t *testing.T) {
	resetMCETracking()

	changed := SetMCEPhase("test-mce", backplanev1.MultiClusterEnginePhaseAvailable)
	if !changed {
		t.Error("expected changed=true on initial publish")
	}

	// Active phase should be 1.
	val, found := gatherGaugeValue(t, PhaseMetricName, map[string]string{
		"name": "test-mce", "phase": "Available",
	})
	if !found {
		t.Fatal("expected Available phase series to exist")
	}
	if val != 1 {
		t.Errorf("expected Available=1, got %v", val)
	}

	// Inactive phases should be 0.
	for _, phase := range allMCEPhases {
		if phase == backplanev1.MultiClusterEnginePhaseAvailable {
			continue
		}
		val, found := gatherGaugeValue(t, PhaseMetricName, map[string]string{
			"name": "test-mce", "phase": string(phase),
		})
		if !found {
			t.Errorf("expected phase %s series to exist", phase)
			continue
		}
		if val != 0 {
			t.Errorf("expected phase %s=0, got %v", phase, val)
		}
	}
}

func TestSetMCEPhase_SameValueNoChange(t *testing.T) {
	resetMCETracking()

	SetMCEPhase("test-mce", backplanev1.MultiClusterEnginePhaseAvailable)
	changed := SetMCEPhase("test-mce", backplanev1.MultiClusterEnginePhaseAvailable)
	if changed {
		t.Error("expected changed=false when re-publishing same phase")
	}
}

func TestSetMCEPhase_Transition(t *testing.T) {
	resetMCETracking()

	SetMCEPhase("test-mce", backplanev1.MultiClusterEnginePhaseAvailable)
	changed := SetMCEPhase("test-mce", backplanev1.MultiClusterEnginePhaseError)
	if !changed {
		t.Error("expected changed=true on phase transition")
	}

	// Error should be 1.
	val, found := gatherGaugeValue(t, PhaseMetricName, map[string]string{
		"name": "test-mce", "phase": "Error",
	})
	if !found {
		t.Fatal("expected Error phase series to exist")
	}
	if val != 1 {
		t.Errorf("expected Error=1, got %v", val)
	}

	// Available should be 0.
	val, found = gatherGaugeValue(t, PhaseMetricName, map[string]string{
		"name": "test-mce", "phase": "Available",
	})
	if !found {
		t.Fatal("expected Available phase series to exist")
	}
	if val != 0 {
		t.Errorf("expected Available=0, got %v", val)
	}
}

func TestSetMCEPhase_UnknownPhase(t *testing.T) {
	resetMCETracking()

	changed := SetMCEPhase("test-mce", backplanev1.PhaseType("NewPhase"))
	if !changed {
		t.Error("expected changed=true on initial unknown phase")
	}

	// Unknown phase should be 1.
	val, found := gatherGaugeValue(t, PhaseMetricName, map[string]string{
		"name": "test-mce", "phase": "NewPhase",
	})
	if !found {
		t.Fatal("expected NewPhase series to exist")
	}
	if val != 1 {
		t.Errorf("expected NewPhase=1, got %v", val)
	}

	// Transition to known phase should clean up unknown.
	SetMCEPhase("test-mce", backplanev1.MultiClusterEnginePhaseAvailable)
	_, found = gatherGaugeValue(t, PhaseMetricName, map[string]string{
		"name": "test-mce", "phase": "NewPhase",
	})
	if found {
		t.Error("expected NewPhase series to be deleted after transition to known phase")
	}
}

func TestSetMCEPhase_UnknownToUnknownTransition(t *testing.T) {
	resetMCETracking()

	SetMCEPhase("test-mce", backplanev1.PhaseType("PhaseA"))
	changed := SetMCEPhase("test-mce", backplanev1.PhaseType("PhaseB"))
	if !changed {
		t.Error("expected changed=true on unknown-to-unknown transition")
	}

	// PhaseA should be gone.
	_, found := gatherGaugeValue(t, PhaseMetricName, map[string]string{
		"name": "test-mce", "phase": "PhaseA",
	})
	if found {
		t.Error("expected PhaseA series to be deleted")
	}

	// PhaseB should be 1.
	val, found := gatherGaugeValue(t, PhaseMetricName, map[string]string{
		"name": "test-mce", "phase": "PhaseB",
	})
	if !found {
		t.Fatal("expected PhaseB series to exist")
	}
	if val != 1 {
		t.Errorf("expected PhaseB=1, got %v", val)
	}
}

func TestSetMCEPhase_MultipleMCEs(t *testing.T) {
	resetMCETracking()

	SetMCEPhase("mce-1", backplanev1.MultiClusterEnginePhaseAvailable)
	SetMCEPhase("mce-2", backplanev1.MultiClusterEnginePhaseError)

	val1, found := gatherGaugeValue(t, PhaseMetricName, map[string]string{
		"name": "mce-1", "phase": "Available",
	})
	if !found || val1 != 1 {
		t.Errorf("mce-1 Available expected 1, got %v (found=%v)", val1, found)
	}

	val2, found := gatherGaugeValue(t, PhaseMetricName, map[string]string{
		"name": "mce-2", "phase": "Error",
	})
	if !found || val2 != 1 {
		t.Errorf("mce-2 Error expected 1, got %v (found=%v)", val2, found)
	}
}

// --- DeleteMCEMetrics tests ---

func TestDeleteMCEMetrics_ClearsPhases(t *testing.T) {
	resetMCETracking()

	SetMCEPhase("test-mce", backplanev1.MultiClusterEnginePhaseAvailable)

	existed := DeleteMCEMetrics("test-mce")
	if !existed {
		t.Error("expected existed=true")
	}

	count := countSeriesForMetric(t, PhaseMetricName, map[string]string{"name": "test-mce"})
	if count != 0 {
		t.Errorf("expected 0 phase series after delete, got %d", count)
	}
}

func TestDeleteMCEMetrics_NonexistentMCE(t *testing.T) {
	resetMCETracking()

	existed := DeleteMCEMetrics("does-not-exist")
	if existed {
		t.Error("expected existed=false for nonexistent MCE")
	}
}

func TestDeleteMCEMetrics_DoesNotAffectOtherMCE(t *testing.T) {
	resetMCETracking()

	SetMCEPhase("mce-1", backplanev1.MultiClusterEnginePhaseAvailable)
	SetMCEPhase("mce-2", backplanev1.MultiClusterEnginePhaseError)

	DeleteMCEMetrics("mce-1")

	// mce-2 should still have its phases.
	val, found := gatherGaugeValue(t, PhaseMetricName, map[string]string{
		"name": "mce-2", "phase": "Error",
	})
	if !found || val != 1 {
		t.Errorf("expected mce-2 Error=1 to survive deletion of mce-1, got %v (found=%v)", val, found)
	}

	// mce-1 should be gone.
	count := countSeriesForMetric(t, PhaseMetricName, map[string]string{"name": "mce-1"})
	if count != 0 {
		t.Errorf("expected 0 phase series for mce-1 after delete, got %d", count)
	}
}

func TestDeleteMCEMetrics_DeleteAfterUnknownPhase(t *testing.T) {
	resetMCETracking()

	SetMCEPhase("test-mce", backplanev1.PhaseType("UnknownPhase"))
	existed := DeleteMCEMetrics("test-mce")
	if !existed {
		t.Error("expected existed=true")
	}

	_, found := gatherGaugeValue(t, PhaseMetricName, map[string]string{
		"name": "test-mce", "phase": "UnknownPhase",
	})
	if found {
		t.Error("expected UnknownPhase series to be deleted")
	}
}

// --- Helper function tests ---

func TestBoolToFloat(t *testing.T) {
	if boolToFloat(true) != 1 {
		t.Error("expected boolToFloat(true)=1")
	}
	if boolToFloat(false) != 0 {
		t.Error("expected boolToFloat(false)=0")
	}
}
