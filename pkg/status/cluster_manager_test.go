// Copyright Contributors to the Open Cluster Management project
package status

import (
	"testing"

	bpv1 "github.com/stolostron/backplane-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ocmapiv1 "open-cluster-management.io/api/operator/v1"
)

func Test_mapClusterManager(t *testing.T) {
	tests := []struct {
		name string
		cm   *ocmapiv1.ClusterManager
		want bpv1.ComponentCondition
	}{
		{
			name: "healthy",
			cm: &ocmapiv1.ClusterManager{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster-manager",
				},
				Status: ocmapiv1.ClusterManagerStatus{
					Conditions: []metav1.Condition{
						{
							Type:    "Applied",
							Status:  metav1.ConditionTrue,
							Reason:  "ClusterManagerApplied",
							Message: "Components of cluster manager are applied",
						},
					},
				},
			},
			want: bpv1.ComponentCondition{
				Name:      "cluster-manager",
				Kind:      "ClusterManager",
				Type:      "Applied",
				Status:    metav1.ConditionTrue,
				Reason:    "ClusterManagerApplied",
				Message:   "Components of cluster manager are applied",
				Available: true,
			},
		},
		{
			name: "no conditions",
			cm: &ocmapiv1.ClusterManager{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "operator.open-cluster-management.io/v1",
					Kind:       "ClusterManager",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster-manager",
				},
			},
			want: unknownStatus("cluster-manager", "ClusterManager"),
		},
	}
	for _, tt := range tests {
		cm, err := runtime.DefaultUnstructuredConverter.ToUnstructured(tt.cm)
		if err != nil {
			t.Errorf("conversion error: %v", err)
		}
		u := &unstructured.Unstructured{Object: cm}
		got := mapClusterManager(u)
		if got.Name != tt.want.Name {
			t.Errorf("mapClusterManager() name discrepancy = %v, want %v", got, tt.want)
		}
		if got.Kind != tt.want.Kind {
			t.Errorf("mapClusterManager() kind discrepancy = %v, want %v", got, tt.want)
		}
		if string(got.Status) != string(tt.want.Status) {
			t.Errorf("mapClusterManager() status discrepancy = %v, want %v", got, tt.want)
		}
		if got.Reason != tt.want.Reason {
			t.Errorf("mapClusterManager() reason discrepancy = %v, want %v", got, tt.want)
		}
		if got.Message != tt.want.Message {
			t.Errorf("mapClusterManager() message discrepancy = %v, want %v", got, tt.want)
		}
		if got.Available != tt.want.Available {
			t.Errorf("mapClusterManager() availability discrepancy = %v, want %v", got, tt.want)
		}
	}
}
