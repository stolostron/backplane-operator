// Copyright Contributors to the Open Cluster Management project
package status

import (
	"testing"

	bpv1alpha1 "github.com/stolostron/backplane-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// MockStatus fulfills the StatusReporter interface for deployments
type MockStatus struct {
	types.NamespacedName
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
func (ms MockStatus) Status(k8sClient client.Client) bpv1alpha1.ComponentCondition {
	return bpv1alpha1.ComponentCondition{
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

func Test_AddCondition(t *testing.T) {
	tracker := StatusTracker{}

	t.Run("Add progressing condition", func(t *testing.T) {
		tracker.AddCondition(NewCondition(bpv1alpha1.MultiClusterEngineProgressing, metav1.ConditionTrue, DeploySuccessReason, "All components deployed"))
		cond := tracker.reportConditions()

		if len(cond) != 1 {
			t.Errorf("StatusTracker.AddCondition() did not add new condition")
		}
	})

	t.Run("Add available condition", func(t *testing.T) {
		tracker.AddCondition(NewCondition(bpv1alpha1.MultiClusterEngineAvailable, metav1.ConditionTrue, ComponentsAvailableReason, "All components available"))
		cond := tracker.reportConditions()

		if len(cond) != 2 {
			t.Errorf("Expected two conditions. Got %v", len(cond))
		}
	})

	t.Run("Update available condition", func(t *testing.T) {
		tracker.AddCondition(NewCondition(bpv1alpha1.MultiClusterEngineAvailable, metav1.ConditionFalse, ComponentsUnavailableReason, "Not all components available"))
		cond := tracker.reportConditions()

		if len(cond) != 2 {
			t.Errorf("Expected two conditions. Got %v", len(cond))
		}

		c := getCondition(cond, bpv1alpha1.MultiClusterEngineAvailable)
		if c.Status != metav1.ConditionFalse {
			t.Errorf("Condition was not updated. Expected %v to equal %v.", c.Status, metav1.ConditionFalse)
		}
	})
}
