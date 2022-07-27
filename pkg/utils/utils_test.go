// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"os"
	"reflect"
	"testing"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
)

func Test_deduplicate(t *testing.T) {
	tests := []struct {
		name string
		have []backplanev1.ComponentConfig
		want []backplanev1.ComponentConfig
	}{
		{
			name: "unique components",
			have: []backplanev1.ComponentConfig{
				{Name: "component1", Enabled: true},
				{Name: "component2", Enabled: true},
			},
			want: []backplanev1.ComponentConfig{
				{Name: "component1", Enabled: true},
				{Name: "component2", Enabled: true},
			},
		},
		{
			name: "duplicate components",
			have: []backplanev1.ComponentConfig{
				{Name: "component1", Enabled: false},
				{Name: "component2", Enabled: true},
				{Name: "component1", Enabled: true},
			},
			want: []backplanev1.ComponentConfig{
				{Name: "component1", Enabled: true},
				{Name: "component2", Enabled: true},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deduplicate(tt.have); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("deduplicate() = %v, want %v", got, tt.want)
			}
		})
	}
	m := &backplanev1.MultiClusterEngine{}
	yes := SetDefaultComponents(m)
	if !yes {
		t.Error("Setting default did not work")
	}

	yes = DeduplicateComponents(m)
	if yes {
		t.Error("Unexpected duplicates")
	}

	os.Setenv("NO_PROXY", "test")
	yes = ProxyEnvVarsAreSet()
	if !yes {
		t.Error("Unexpected proxy failure")
	}
	os.Unsetenv("NO_PROXY")
	yes = ProxyEnvVarsAreSet()
	if yes {
		t.Error("Unexpected proxy success")
	}

	var sample backplanev1.AvailabilityType
	sample = backplanev1.HAHigh

	yes = AvailabilityConfigIsValid(sample)
	if !yes {
		t.Error("Unexpected availabilitty config failure")
	}

	sample = "test"
	yes = AvailabilityConfigIsValid(sample)
	if yes {
		t.Error("Unexpected availabilitty config successs")
	}

	stringList := []string{"test1", "test2"}
	stringRemoveList := []string{"test2"}

	yes = Contains(stringList, "test1")
	if !yes {
		t.Error("Contains did not work")
	}
	attemptedRemove := Remove(stringList, "test1")
	if len(attemptedRemove) != len(stringRemoveList) {
		t.Error("Removes did not work")
	}
}
