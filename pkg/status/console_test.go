// Copyright Contributors to the Open Cluster Management project
package status

import (
	"testing"

	bpv1 "github.com/stolostron/backplane-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
				Available: true,
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

func Test_consoleGetName(t *testing.T) {
	tests := []struct {
		name   string
		status ConsoleUnavailableStatus
		want   string
	}{
		{
			name: "console name",
			status: ConsoleUnavailableStatus{
				NamespacedName: types.NamespacedName{Name: "console-mce-console", Namespace: "test-namespace"},
			},
			want: "console-mce-console",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.GetName()

			if got != tt.want {
				t.Errorf("GetName() discrepancy = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_consoleGetNamespace(t *testing.T) {
	tests := []struct {
		name   string
		status ConsoleUnavailableStatus
		want   string
	}{
		{
			name: "console namespace",
			status: ConsoleUnavailableStatus{
				NamespacedName: types.NamespacedName{Name: "console-mce-console", Namespace: "test-namespace"},
			},
			want: "test-namespace",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.GetNamespace()

			if got != tt.want {
				t.Errorf("GetNamespace() discrepancy = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_consoleGetKind(t *testing.T) {
	tests := []struct {
		name   string
		status ConsoleUnavailableStatus
		want   string
	}{
		{
			name: "console kind",
			status: ConsoleUnavailableStatus{
				NamespacedName: types.NamespacedName{Name: "console-mce-console", Namespace: "test-namespace"},
			},
			want: "Deployment",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.GetKind()

			if got != tt.want {
				t.Errorf("GetKind() discrepancy = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_statusUnknown(t *testing.T) {
	tests := []struct {
		name   string
		status ConsoleUnavailableStatus
		want   bpv1.ComponentCondition
	}{
		{
			name: "status unknown check",
			status: ConsoleUnavailableStatus{
				NamespacedName: types.NamespacedName{Name: "test-console", Namespace: "test-namespace"},
			},
			want: bpv1.ComponentCondition{
				Name:               "test-console",
				Kind:               "Deployment",
				Type:               "Unknown",
				Status:             metav1.ConditionUnknown,
				LastUpdateTime:     metav1.Now(),
				LastTransitionTime: metav1.Now(),
				Reason:             "No conditions available",
				Message:            "No conditions available",
				Available:          false,
			},
		},
	}
	for _, tt := range tests {
		k8sClient := fake.NewClientBuilder().Build()
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.Status(k8sClient)

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
