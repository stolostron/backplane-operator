// Copyright Contributors to the Open Cluster Management project
package status

import (
	"testing"

	bpv1 "github.com/stolostron/backplane-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_mapConsoleDeployment(t *testing.T) {
	tests := []struct {
		name string
		ds   *appsv1.Deployment
		want bpv1.ComponentCondition
	}{
		{
			name: "healthy deployment",
			ds: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "operator.open-cluster-management.io/v1",
					Kind:       "Deployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-console",
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
				Name:      "test-console",
				Kind:      "Deployment",
				Type:      "Available",
				Status:    metav1.ConditionFalse,
				Reason:    "OCP Console missing",
				Message:   "The OCP Console must be enabled before using ACM Console",
				Available: true,
			},
		},
		{
			name: "unhealthy deployment",
			ds: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "operator.open-cluster-management.io/v1",
					Kind:       "Deployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-console",
				},
				Status: appsv1.DeploymentStatus{
					Conditions: []appsv1.DeploymentCondition{
						{
							Type:    appsv1.DeploymentAvailable,
							Status:  corev1.ConditionFalse,
							Reason:  "Available",
							Message: "deployment available",
						},
					},
				},
			},
			want: bpv1.ComponentCondition{
				Name:      "test-console",
				Kind:      "Deployment",
				Type:      "Available",
				Status:    metav1.ConditionFalse,
				Reason:    "OCP Console missing",
				Message:   "The OCP Console must be enabled before using ACM Console",
				Available: false,
			},
		},
		{
			name: "no conditions",
			ds: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "operator.open-cluster-management.io/v1",
					Kind:       "Deployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-console",
				},
			},
			want: unknownStatus("test-console", "Deployment"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapConsoleDeployment(tt.ds)

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
