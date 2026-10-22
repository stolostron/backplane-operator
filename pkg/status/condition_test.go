// Copyright Contributors to the Open Cluster Management project
package status

import (
	"fmt"
	"testing"

	bpv1 "github.com/stolostron/backplane-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_NewCondition(t *testing.T) {
	tests := []struct {
		name     string
		condType bpv1.MultiClusterEngineConditionType
		status   metav1.ConditionStatus
		reason   string
		message  string
		want     bool
	}{
		{
			name:     "added new multiclusterengine condition",
			condType: bpv1.MultiClusterEngineAvailable,
			status:   metav1.ConditionTrue,
			reason:   DeploySuccessReason,
			message:  "All components deployed",
			want:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := NewCondition(tt.condType, tt.status, tt.reason, tt.message)

			if got := cond.Type != tt.condType; got {
				t.Errorf("%s: expected condition condType: %v to equal: %v, got %v", tt.name, cond.Type, tt.condType, got)
			}

			if got := cond.Status != tt.status; got {
				t.Errorf("%s: expected condition status: %v to equal: %v, got %v", tt.name, cond.Status, tt.status, got)
			}

			if got := cond.Reason != tt.reason; got {
				t.Errorf("%s: expected condition reason: %v to equal: %v, got %v", tt.name, cond.Reason, tt.reason, got)
			}

			if got := cond.Message != tt.message; got {
				t.Errorf("%s: expected condition message: %v to equal: %v, got %v", tt.name, cond.Message, tt.message, got)
			}
		})
	}
}

func Test_setCondition(t *testing.T) {
	tests := []struct {
		name       string
		conditions []bpv1.MultiClusterEngineCondition
		want       int
	}{
		{
			name: "set single multiclusterengine condition",
			conditions: []bpv1.MultiClusterEngineCondition{
				{
					Reason:             DeploySuccessReason,
					Status:             metav1.ConditionTrue,
					Type:               bpv1.MultiClusterEngineProgressing,
					LastUpdateTime:     metav1.Now(),
					LastTransitionTime: metav1.Now(),
				},
			},
			want: 1,
		},
		{
			name: "set multiple multiclusterengine conditions",
			conditions: []bpv1.MultiClusterEngineCondition{
				{
					Reason:             DeploySuccessReason,
					Status:             metav1.ConditionTrue,
					Type:               bpv1.MultiClusterEngineProgressing,
					LastUpdateTime:     metav1.Now(),
					LastTransitionTime: metav1.Now(),
				},
				{
					Message:            "All components deployed",
					Reason:             ComponentsAvailableReason,
					Status:             metav1.ConditionTrue,
					Type:               bpv1.MultiClusterEngineAvailable,
					LastUpdateTime:     metav1.Now(),
					LastTransitionTime: metav1.Now(),
				},
			},
			want: 2,
		},
		{
			name: "set duplicate multiclusterengine conditions",
			conditions: []bpv1.MultiClusterEngineCondition{
				{
					Reason:             DeploySuccessReason,
					Status:             metav1.ConditionTrue,
					Type:               bpv1.MultiClusterEngineProgressing,
					LastUpdateTime:     metav1.Now(),
					LastTransitionTime: metav1.Now(),
				},
				{
					Message:            "All components deployed",
					Reason:             ComponentsAvailableReason,
					Status:             metav1.ConditionTrue,
					Type:               bpv1.MultiClusterEngineAvailable,
					LastUpdateTime:     metav1.Now(),
					LastTransitionTime: metav1.Now(),
				},
			},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newCond := bpv1.MultiClusterEngineCondition{
				Reason:             DeploySuccessReason,
				Status:             metav1.ConditionUnknown,
				Type:               bpv1.MultiClusterEngineProgressing,
				LastUpdateTime:     metav1.Now(),
				LastTransitionTime: metav1.Now(),
			}

			cond := setCondition(tt.conditions, newCond)

			if len(cond) != tt.want {
				t.Errorf("%s: expected condition to exist: %d, got %v", tt.name, tt.want, len(cond))
			}
		})
	}
}

func Test_getCondition(t *testing.T) {
	tests := []struct {
		name       string
		conditions []bpv1.MultiClusterEngineCondition
		condType   bpv1.MultiClusterEngineConditionType
		want       bool
	}{
		{
			name: "get multiclusterengine condition",
			conditions: []bpv1.MultiClusterEngineCondition{
				{
					Message:            "failed to create typed patch object (mutlicluster-engine/foo; apps/v1, Kind=Deployment)",
					Reason:             ApplyFailedReason,
					Status:             metav1.ConditionTrue,
					Type:               bpv1.MultiClusterEngineComponentFailure,
					LastUpdateTime:     metav1.Now(),
					LastTransitionTime: metav1.Now(),
				},
				{
					Message:            "setting up operator",
					Reason:             ComponentsUnavailableReason,
					Status:             metav1.ConditionFalse,
					Type:               bpv1.MultiClusterEngineProgressing,
					LastUpdateTime:     metav1.Now(),
					LastTransitionTime: metav1.Now(),
				},
			},
			condType: bpv1.MultiClusterEngineComponentFailure,
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := getCondition(tt.conditions, tt.condType)

			exists := cond != nil
			if exists != tt.want {
				t.Errorf("%s: expected condition to exist: %t, got %v", tt.name, tt.want, exists)
			}
		})
	}
}

func Test_filterOutCondition(t *testing.T) {
	tests := []struct {
		name       string
		conditions []bpv1.MultiClusterEngineCondition
		condType   bpv1.MultiClusterEngineConditionType
		want       int
	}{
		{
			name: "filter out multiclusterengine condition",
			conditions: []bpv1.MultiClusterEngineCondition{
				{
					Message:            "failed to create typed patch object (mutlicluster-engine/foo; apps/v1, Kind=Deployment)",
					Reason:             ApplyFailedReason,
					Status:             metav1.ConditionTrue,
					Type:               bpv1.MultiClusterEngineComponentFailure,
					LastUpdateTime:     metav1.Now(),
					LastTransitionTime: metav1.Now(),
				},
				{
					Message:            "setting up operator",
					Reason:             ComponentsUnavailableReason,
					Status:             metav1.ConditionFalse,
					Type:               bpv1.MultiClusterEngineProgressing,
					LastUpdateTime:     metav1.Now(),
					LastTransitionTime: metav1.Now(),
				},
			},
			condType: bpv1.MultiClusterEngineComponentFailure,
			want:     1,
		},
		{
			name: "filter out no multiclusterengine condition",
			conditions: []bpv1.MultiClusterEngineCondition{
				{
					Message:            "failed to create typed patch object (mutlicluster-engine/foo; apps/v1, Kind=Deployment)",
					Reason:             ApplyFailedReason,
					Status:             metav1.ConditionTrue,
					Type:               bpv1.MultiClusterEngineComponentFailure,
					LastUpdateTime:     metav1.Now(),
					LastTransitionTime: metav1.Now(),
				},
				{
					Message:            "setting up operator",
					Reason:             ComponentsUnavailableReason,
					Status:             metav1.ConditionFalse,
					Type:               bpv1.MultiClusterEngineProgressing,
					LastUpdateTime:     metav1.Now(),
					LastTransitionTime: metav1.Now(),
				},
			},
			condType: bpv1.MultiClusterEngineAvailable,
			want:     2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := filterOutCondition(tt.conditions, tt.condType)

			if len(cond) != tt.want {
				t.Errorf("%s: expected condition to exist: %v, got %v", tt.name, tt.want, len(cond))
			}
		})
	}
}

func Test_FilterOutConditionWithSubString(t *testing.T) {
	tests := []struct {
		name       string
		conditions []bpv1.MultiClusterEngineCondition
		condType   bpv1.MultiClusterEngineConditionType
		subString  string
		want       int
	}{
		{
			name: "filter out multiclusterengine condition by substring",
			conditions: []bpv1.MultiClusterEngineCondition{
				{
					Message:            "failed to create typed patch object (mutlicluster-engine/foo; apps/v1, Kind=Deployment)",
					Reason:             ApplyFailedReason,
					Status:             metav1.ConditionTrue,
					Type:               bpv1.MultiClusterEngineConditionType(fmt.Sprintf("%v: %v (Kind: %v)", bpv1.MultiClusterEngineComponentFailure, "foo", "Deployment")),
					LastUpdateTime:     metav1.Now(),
					LastTransitionTime: metav1.Now(),
				},
			},
			condType:  bpv1.MultiClusterEngineComponentFailure,
			subString: string(bpv1.MultiClusterEngineComponentFailure),
			want:      0,
		},
		{
			name: "filter out no multiclusterengine condition",
			conditions: []bpv1.MultiClusterEngineCondition{
				{
					Message:            "failed to create typed patch object (mutlicluster-engine/foo; apps/v1, Kind=Deployment)",
					Reason:             ApplyFailedReason,
					Status:             metav1.ConditionTrue,
					Type:               bpv1.MultiClusterEngineComponentFailure,
					LastUpdateTime:     metav1.Now(),
					LastTransitionTime: metav1.Now(),
				},
				{
					Message:            "setting up operator",
					Reason:             ComponentsUnavailableReason,
					Status:             metav1.ConditionFalse,
					Type:               bpv1.MultiClusterEngineProgressing,
					LastUpdateTime:     metav1.Now(),
					LastTransitionTime: metav1.Now(),
				},
			},
			condType:  bpv1.MultiClusterEngineAvailable,
			subString: string(bpv1.MultiClusterEngineAvailable),
			want:      2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := FilterOutConditionWithSubString(tt.conditions, tt.condType)

			if len(cond) != tt.want {
				t.Errorf("%s: expected condition to exist: %v, got %v", tt.name, tt.want, len(cond))
			}
		})
	}
}

func Test_ConditionPresentWithSubstring(t *testing.T) {
	tests := []struct {
		name       string
		conditions []bpv1.MultiClusterEngineCondition
		subString  string
		want       bool
	}{
		{
			name: "check if multiclusterengine condition contains substring",
			conditions: []bpv1.MultiClusterEngineCondition{
				{
					Message:            "failed to create typed patch object (mutlicluster-engine/foo; apps/v1, Kind=Deployment)",
					Reason:             ApplyFailedReason,
					Status:             metav1.ConditionTrue,
					Type:               bpv1.MultiClusterEngineConditionType(fmt.Sprintf("%v: %v (Kind: %v)", bpv1.MultiClusterEngineComponentFailure, "foo", "Deployment")),
					LastUpdateTime:     metav1.Now(),
					LastTransitionTime: metav1.Now(),
				},
			},
			subString: string(bpv1.MultiClusterEngineComponentFailure),
			want:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if ok := ConditionPresentWithSubstring(tt.conditions, tt.subString); !ok {
				t.Errorf("%s: expected condition to exist: %v, got %v", tt.name, tt.want, ok)
			}
		})
	}
}
