// Copyright Contributors to the Open Cluster Management project
package status

import (
	"testing"

	bpv1 "github.com/stolostron/backplane-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// MockStatus fulfills the StatusReporter interface for deployments
type MockStatus struct {
	types.NamespacedName
	statusFunc func() bpv1.ComponentCondition
}

func (ms MockStatus) GetName() string {
	return ms.Name
}

func (ms MockStatus) GetNamespace() string {
	return ms.Namespace
}

func (ms MockStatus) GetKind() string {
	return "Mock"
}

// Converts a deployment's status to a backplane component status
func (ms MockStatus) Status(k8sClient client.Client) bpv1.ComponentCondition {
	if ms.statusFunc != nil {
		return ms.statusFunc()
	}
	return bpv1.ComponentCondition{
		Name:               ms.Name,
		Kind:               "Deployment",
		Type:               "Available",
		Status:             metav1.ConditionStatus("true"),
		LastUpdateTime:     metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Reason:             "Running",
		Message:            "Mock is running",
		Available:          true,
	}
}

func Test_AddComponent(t *testing.T) {
	tracker := StatusTracker{Client: fake.NewClientBuilder().Build()}

	t.Run("Add component", func(t *testing.T) {
		tracker.AddComponent(MockStatus{
			NamespacedName: types.NamespacedName{Name: "mock-name", Namespace: "mock-ns"},
		})
		if len(tracker.Components) != 1 {
			t.Errorf("StatusTracker.AddComponent() did not add new compotent")
		}
	})

	t.Run("Add component again", func(t *testing.T) {
		tracker.AddComponent(MockStatus{
			NamespacedName: types.NamespacedName{Name: "mock-name", Namespace: "mock-ns"},
		})
		if len(tracker.Components) > 1 {
			t.Errorf("StatusTracker.AddComponent() is not idempotent")
		}
	})
}

func Test_RemoveComponent(t *testing.T) {
	tracker := StatusTracker{Client: fake.NewClientBuilder().Build()}
	tracker.AddComponent(MockStatus{
		NamespacedName: types.NamespacedName{Name: "mock-name-a", Namespace: "mock-ns"},
	})
	tracker.AddComponent(MockStatus{
		NamespacedName: types.NamespacedName{Name: "mock-name-b", Namespace: "mock-ns"},
	})
	t.Run("Remove component", func(t *testing.T) {
		tracker.RemoveComponent(MockStatus{
			NamespacedName: types.NamespacedName{Name: "mock-name-a", Namespace: "mock-ns"},
		})
		if len(tracker.Components) != 1 {
			t.Errorf("StatusTracker.RemoveComponent() did not remove component")
		}
	})

	t.Run("Remove component again", func(t *testing.T) {
		tracker.RemoveComponent(MockStatus{
			NamespacedName: types.NamespacedName{Name: "mock-name-a", Namespace: "mock-ns"},
		})
		if len(tracker.Components) != 1 {
			t.Errorf("StatusTracker.RemoveComponent() is not idempotent")
		}
	})
}

func Test_AddCondition(t *testing.T) {
	tracker := StatusTracker{}

	t.Run("Add progressing condition", func(t *testing.T) {
		tracker.AddCondition(NewCondition(bpv1.MultiClusterEngineProgressing, metav1.ConditionTrue, DeploySuccessReason, "All components deployed"))
		cond := tracker.reportConditions()

		if len(cond) != 1 {
			t.Errorf("StatusTracker.AddCondition() did not add new condition")
		}
	})

	t.Run("Add available condition", func(t *testing.T) {
		tracker.AddCondition(NewCondition(bpv1.MultiClusterEngineAvailable, metav1.ConditionTrue, ComponentsAvailableReason, "All components available"))
		cond := tracker.reportConditions()

		if len(cond) != 2 {
			t.Errorf("Expected two conditions. Got %v", len(cond))
		}
	})

	t.Run("Update available condition", func(t *testing.T) {
		tracker.AddCondition(NewCondition(bpv1.MultiClusterEngineAvailable, metav1.ConditionFalse, ComponentsUnavailableReason, "Not all components available"))
		cond := tracker.reportConditions()

		if len(cond) != 2 {
			t.Errorf("Expected two conditions. Got %v", len(cond))
		}

		c := getCondition(cond, bpv1.MultiClusterEngineAvailable)
		if c.Status != metav1.ConditionFalse {
			t.Errorf("Condition was not updated. Expected %v to equal %v.", c.Status, metav1.ConditionFalse)
		}
	})
}

func TestStatusTracker_ReportStatus(t *testing.T) {
	tests := []struct {
		name       string
		Components []StatusReporter
		Conditions []bpv1.MultiClusterEngineCondition
		want       bpv1.MultiClusterEngineStatus
	}{
		{
			name: "Single running deployment",
			Components: []StatusReporter{
				MockStatus{
					NamespacedName: types.NamespacedName{Name: "mock-name", Namespace: "mock-ns"},
					statusFunc: func() bpv1.ComponentCondition {
						return bpv1.ComponentCondition{
							Name:               "mock-name",
							Kind:               "Deployment",
							Type:               "Available",
							Status:             metav1.ConditionStatus("true"),
							LastUpdateTime:     metav1.Now(),
							LastTransitionTime: metav1.Now(),
							Reason:             "Running",
							Message:            "Mock is running",
							Available:          true,
						}
					},
				},
			},
			Conditions: nil,
			want: bpv1.MultiClusterEngineStatus{
				CurrentVersion: "9.9.9",
				DesiredVersion: "9.9.9",
				Phase:          bpv1.MultiClusterEnginePhaseAvailable,
			},
		},
		{
			name: "Single non-running deployment",
			Components: []StatusReporter{
				MockStatus{
					NamespacedName: types.NamespacedName{Name: "mock-name", Namespace: "mock-ns"},
					statusFunc: func() bpv1.ComponentCondition {
						return bpv1.ComponentCondition{
							Name:               "mock-name",
							Kind:               "Deployment",
							Type:               "Pending",
							Status:             metav1.ConditionStatus("true"),
							LastUpdateTime:     metav1.Now(),
							LastTransitionTime: metav1.Now(),
							Reason:             "NotRunning",
							Message:            "Mock is not running",
							Available:          false,
						}
					},
				},
			},
			Conditions: nil,
			want: bpv1.MultiClusterEngineStatus{
				CurrentVersion: "",
				DesiredVersion: "9.9.9",
				Phase:          bpv1.MultiClusterEnginePhaseProgressing,
			},
		},
		{
			name:       "No components tracked",
			Components: nil,
			Conditions: nil,
			want: bpv1.MultiClusterEngineStatus{
				CurrentVersion: "",
				DesiredVersion: "9.9.9",
				Phase:          bpv1.MultiClusterEnginePhaseError,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := StatusTracker{Client: fake.NewClientBuilder().Build()}
			backplane := bpv1.MultiClusterEngine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: bpv1.MultiClusterEngineSpec{
					TargetNamespace: "mock-ns",
				},
			}
			for _, c := range tt.Components {
				tracker.AddComponent(c)
			}

			got := tracker.ReportStatus(backplane)

			if got.Phase != tt.want.Phase {
				t.Errorf("StatusTracker.ReportStatus() phase = %v, want %v", got.Phase, tt.want.Phase)
			}
			if got.DesiredVersion != tt.want.DesiredVersion {
				t.Errorf("StatusTracker.ReportStatus() desiredVersion = %v, want %v", got.DesiredVersion, tt.want.DesiredVersion)
			}
			if got.CurrentVersion != tt.want.CurrentVersion {
				t.Errorf("StatusTracker.ReportStatus() currentVersion = %v, want %v", got.CurrentVersion, tt.want.CurrentVersion)
			}
		})
	}
}

func TestStatusTracker_Reset(t *testing.T) {
	t.Run("Reset status tracker", func(t *testing.T) {
		tracker := StatusTracker{Client: fake.NewClientBuilder().Build()}
		tracker.AddComponent(MockStatus{
			NamespacedName: types.NamespacedName{Name: "mock-name", Namespace: "mock-ns"},
			statusFunc: func() bpv1.ComponentCondition {
				return bpv1.ComponentCondition{
					Name:               "mock-name",
					Kind:               "Deployment",
					Type:               "Pending",
					Status:             metav1.ConditionStatus("true"),
					LastUpdateTime:     metav1.Now(),
					LastTransitionTime: metav1.Now(),
					Reason:             "NotRunning",
					Message:            "Mock is not running",
					Available:          false,
				}
			},
		})

		if len(tracker.Components) != 1 {
			t.Errorf("StatusTracker should have 1 component")
		}
		tracker.Reset("123")
		if len(tracker.Components) != 0 {
			t.Errorf("StatusTracker should have 0 component")
		}
	})
}
