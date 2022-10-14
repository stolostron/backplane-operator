// Copyright Contributors to the Open Cluster Management project
package status

import (
	"testing"

	bpv1 "github.com/stolostron/backplane-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
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

func Test_mapManagedClusterAddOn(t *testing.T) {
	tests := []struct {
		name  string
		addon *addonv1alpha1.ManagedClusterAddOn
		want  bpv1.ComponentCondition
	}{
		{
			name: "healthy",
			addon: &addonv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hypershift-addon",
				},
				Status: addonv1alpha1.ManagedClusterAddOnStatus{
					Conditions: []metav1.Condition{
						{
							Type:    "RegistrationApplied",
							Status:  metav1.ConditionTrue,
							Reason:  "RegistrationConfigured",
							Message: "Registration of the addon agent is configured",
						},
						{
							Type:    "Available",
							Status:  metav1.ConditionTrue,
							Reason:  "ManagedClusterAddOnLeaseUpdated",
							Message: "hypershift-addon add-on is available.",
						},
					},
				},
			},
			want: bpv1.ComponentCondition{
				Name:      "hypershift-addon",
				Kind:      "ManagedClusterAddOn",
				Type:      "Available",
				Status:    metav1.ConditionTrue,
				Reason:    "ManagedClusterAddOnLeaseUpdated",
				Message:   "hypershift-addon add-on is available.",
				Available: true,
			},
		},
		{
			name: "no conditions",
			addon: &addonv1alpha1.ManagedClusterAddOn{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "addon.open-cluster-management.io/v1alpha1",
					Kind:       "ManagedClusterAddOn",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "hypershift-addon",
				},
			},
			want: unknownStatus("hypershift-addon", "ManagedClusterAddOn"),
		},
	}
	for _, tt := range tests {
		cm, err := runtime.DefaultUnstructuredConverter.ToUnstructured(tt.addon)
		if err != nil {
			t.Errorf("conversion error: %v", err)
		}
		u := &unstructured.Unstructured{Object: cm}
		got := mapManagedClusterAddOn(u)
		if got.Name != tt.want.Name {
			t.Errorf("mapManagedClusterAddOn() name discrepancy = %v, want %v", got, tt.want)
		}
		if got.Kind != tt.want.Kind {
			t.Errorf("mapManagedClusterAddOn() kind discrepancy = %v, want %v", got, tt.want)
		}
		if string(got.Status) != string(tt.want.Status) {
			t.Errorf("mapManagedClusterAddOn() status discrepancy = %v, want %v", got, tt.want)
		}
		if got.Reason != tt.want.Reason {
			t.Errorf("mapManagedClusterAddOn() reason discrepancy = %v, want %v", got, tt.want)
		}
		if got.Message != tt.want.Message {
			t.Errorf("mapManagedClusterAddOn() message discrepancy = %v, want %v", got, tt.want)
		}
		if got.Available != tt.want.Available {
			t.Errorf("mapManagedClusterAddOn() availability discrepancy = %v, want %v", got, tt.want)
		}
	}
}
