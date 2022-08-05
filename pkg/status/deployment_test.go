// Copyright Contributors to the Open Cluster Management project
package status

import (
	"testing"

	bpv1 "github.com/stolostron/backplane-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_mapDeployment(t *testing.T) {
	tests := []struct {
		name string
		ds   *appsv1.Deployment
		want bpv1.ComponentCondition
	}{
		{
			name: "healthy deployment",
			ds: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-deployment",
				},
				Status: appsv1.DeploymentStatus{
					Conditions: []appsv1.DeploymentCondition{
						{
							Type:    appsv1.DeploymentAvailable,
							Status:  corev1.ConditionTrue,
							Reason:  "Available",
							Message: "deployment available",
						},
					},
				},
			},
			want: bpv1.ComponentCondition{
				Name:      "test-deployment",
				Kind:      "Deployment",
				Type:      "Available",
				Status:    metav1.ConditionTrue,
				Reason:    "Available",
				Available: true,
			},
		},
		{
			name: "unavailable replicas",
			ds: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-deployment",
				},
				Status: appsv1.DeploymentStatus{
					AvailableReplicas:   1,
					UnavailableReplicas: 1,
					Conditions: []appsv1.DeploymentCondition{
						{
							Type:    appsv1.DeploymentAvailable,
							Status:  corev1.ConditionTrue,
							Reason:  "Available",
							Message: "deployment available",
						},
						{
							Type:    appsv1.DeploymentProgressing,
							Status:  corev1.ConditionTrue,
							Reason:  "New replica",
							Message: "New replica",
						},
					},
				},
			},
			want: bpv1.ComponentCondition{
				Name:      "test-deployment",
				Kind:      "Deployment",
				Type:      "Progressing",
				Status:    metav1.ConditionTrue,
				Reason:    "New replica",
				Message:   "New replica",
				Available: false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapDeployment(tt.ds)

			if got.Name != tt.want.Name {
				t.Errorf("mapDeployment() name discrepancy = %v, want %v", got, tt.want)
			}
			if got.Kind != tt.want.Kind {
				t.Errorf("mapDeployment() kind discrepancy = %v, want %v", got, tt.want)
			}
			if string(got.Status) != string(tt.want.Status) {
				t.Errorf("mapDeployment() status discrepancy = %v, want %v", got, tt.want)
			}
			if got.Reason != tt.want.Reason {
				t.Errorf("mapDeployment() reason discrepancy = %v, want %v", got, tt.want)
			}
			if got.Message != tt.want.Message {
				t.Errorf("mapDeployment() message discrepancy = %v, want %v", got, tt.want)
			}
			if got.Available != tt.want.Available {
				t.Errorf("mapDeployment() availability discrepancy = %v, want %v", got, tt.want)
			}
		})
	}
}
