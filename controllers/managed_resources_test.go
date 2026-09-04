// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"testing"

	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	backplanev1 "github.com/stolostron/backplane-operator/api/v1"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newManagedResourcesTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := appsv1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add appsv1 to scheme: %v", err)
	}
	if err := backplanev1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add backplanev1 to scheme: %v", err)
	}
	if err := promv1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add promv1 to scheme: %v", err)
	}
	return s
}

func newManagedResource(apiVersion, kind, name, namespace string) backplanev1.ManagedResource {
	return backplanev1.ManagedResource{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       name,
		Namespace:  namespace,
	}
}

func TestManagedResourcesEqual(t *testing.T) {
	tests := []struct {
		name string
		a    []backplanev1.ManagedResource
		b    []backplanev1.ManagedResource
		want bool
	}{
		{
			name: "both empty",
			a:    nil,
			b:    []backplanev1.ManagedResource{},
			want: true,
		},
		{
			name: "identical order",
			a: []backplanev1.ManagedResource{
				newManagedResource("apps/v1", "Deployment", "a", "ns"),
				newManagedResource("v1", "Service", "b", "ns"),
			},
			b: []backplanev1.ManagedResource{
				newManagedResource("apps/v1", "Deployment", "a", "ns"),
				newManagedResource("v1", "Service", "b", "ns"),
			},
			want: true,
		},
		{
			name: "same set, different order",
			a: []backplanev1.ManagedResource{
				newManagedResource("apps/v1", "Deployment", "a", "ns"),
				newManagedResource("v1", "Service", "b", "ns"),
			},
			b: []backplanev1.ManagedResource{
				newManagedResource("v1", "Service", "b", "ns"),
				newManagedResource("apps/v1", "Deployment", "a", "ns"),
			},
			want: true,
		},
		{
			name: "different lengths",
			a: []backplanev1.ManagedResource{
				newManagedResource("apps/v1", "Deployment", "a", "ns"),
			},
			b: []backplanev1.ManagedResource{
				newManagedResource("apps/v1", "Deployment", "a", "ns"),
				newManagedResource("v1", "Service", "b", "ns"),
			},
			want: false,
		},
		{
			name: "same length, different contents",
			a: []backplanev1.ManagedResource{
				newManagedResource("apps/v1", "Deployment", "a", "ns"),
			},
			b: []backplanev1.ManagedResource{
				newManagedResource("apps/v1", "Deployment", "c", "ns"),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := managedResourcesEqual(tt.a, tt.b); got != tt.want {
				t.Errorf("managedResourcesEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractManagedResources(t *testing.T) {
	deployment := &unstructured.Unstructured{}
	deployment.SetAPIVersion("apps/v1")
	deployment.SetKind("Deployment")
	deployment.SetName("my-deploy")
	deployment.SetNamespace("test-ns")

	networkPolicy := &unstructured.Unstructured{}
	networkPolicy.SetAPIVersion("networking.k8s.io/v1")
	networkPolicy.SetKind("NetworkPolicy")
	networkPolicy.SetName("my-np")
	networkPolicy.SetNamespace("test-ns")

	clusterRole := &unstructured.Unstructured{}
	clusterRole.SetAPIVersion("rbac.authorization.k8s.io/v1")
	clusterRole.SetKind("ClusterRole")
	clusterRole.SetName("my-cr")

	resources := extractManagedResources([]*unstructured.Unstructured{deployment, networkPolicy, clusterRole})

	want := []backplanev1.ManagedResource{
		newManagedResource("apps/v1", "Deployment", "my-deploy", "test-ns"),
		newManagedResource("rbac.authorization.k8s.io/v1", "ClusterRole", "my-cr", ""),
	}

	if !managedResourcesEqual(resources, want) {
		t.Errorf("extractManagedResources() = %v, want %v (NetworkPolicy should be excluded)", resources, want)
	}
	if len(resources) != 2 {
		t.Errorf("extractManagedResources() returned %d resources, want 2 (NetworkPolicy should be skipped)",
			len(resources))
	}
}

func TestGetAndUpdateManagedResources(t *testing.T) {
	s := newManagedResourcesTestScheme(t)

	mce := &backplanev1.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{Name: "mce"},
		Spec:       backplanev1.MultiClusterEngineSpec{TargetNamespace: "test-ns"},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()
	r := &MultiClusterEngineReconciler{Client: fakeClient}

	// No InternalEngineComponent exists yet - should return nil without error.
	if got := r.getManagedResources(context.TODO(), mce, "console-mce"); got != nil {
		t.Errorf("getManagedResources() with no CR = %v, want nil", got)
	}

	// updateManagedResources should be a no-op (not an error) when the CR doesn't exist yet.
	if err := r.updateManagedResources(context.TODO(), mce, "console-mce",
		[]backplanev1.ManagedResource{newManagedResource("v1", "ConfigMap", "cm", "test-ns")}); err != nil {
		t.Errorf("updateManagedResources() with no CR returned error: %v", err)
	}

	// Create the InternalEngineComponent CR (mirrors ensureInternalEngineComponent).
	iec := &backplanev1.InternalEngineComponent{
		ObjectMeta: metav1.ObjectMeta{Name: "console-mce", Namespace: "test-ns"},
	}
	if err := fakeClient.Create(context.TODO(), iec); err != nil {
		t.Fatalf("failed to create InternalEngineComponent: %v", err)
	}

	initial := []backplanev1.ManagedResource{
		newManagedResource("apps/v1", "Deployment", "console-mce-console", "test-ns"),
		newManagedResource("monitoring.coreos.com/v1", "ServiceMonitor", "console-mce-monitor", "test-ns"),
	}
	if err := r.updateManagedResources(context.TODO(), mce, "console-mce", initial); err != nil {
		t.Fatalf("updateManagedResources() returned error: %v", err)
	}

	got := r.getManagedResources(context.TODO(), mce, "console-mce")
	if !managedResourcesEqual(got, initial) {
		t.Errorf("getManagedResources() = %v, want %v", got, initial)
	}

	// Updating with the same set (different order) should be a no-op but not error.
	reordered := []backplanev1.ManagedResource{initial[1], initial[0]}
	if err := r.updateManagedResources(context.TODO(), mce, "console-mce", reordered); err != nil {
		t.Fatalf("updateManagedResources() with reordered list returned error: %v", err)
	}

	// Updating with a smaller set (ServiceMonitor removed from chart) should persist the new list.
	updated := []backplanev1.ManagedResource{initial[0]}
	if err := r.updateManagedResources(context.TODO(), mce, "console-mce", updated); err != nil {
		t.Fatalf("updateManagedResources() returned error: %v", err)
	}

	got = r.getManagedResources(context.TODO(), mce, "console-mce")
	if !managedResourcesEqual(got, updated) {
		t.Errorf("getManagedResources() after update = %v, want %v", got, updated)
	}
}

func TestCleanupOrphanedManagedResources(t *testing.T) {
	s := newManagedResourcesTestScheme(t)

	mce := &backplanev1.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{Name: "mce"},
		Spec:       backplanev1.MultiClusterEngineSpec{TargetNamespace: "multicluster-engine"},
	}

	tests := []struct {
		name          string
		component     string
		oldResources  []backplanev1.ManagedResource
		newResources  []backplanev1.ManagedResource
		setupClient   func(t *testing.T) client.Client
		verify        func(t *testing.T, c client.Client)
		expectRequeue bool
	}{
		{
			name:      "resource removed from chart and owned by MCE is deleted",
			component: "example",
			oldResources: []backplanev1.ManagedResource{
				newManagedResource("apps/v1", "Deployment", "old-deploy", "multicluster-engine"),
			},
			newResources: nil,
			setupClient: func(t *testing.T) client.Client {
				deploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "old-deploy",
						Namespace: "multicluster-engine",
						Labels:    map[string]string{"backplaneconfig.name": "mce"},
					},
				}
				return fake.NewClientBuilder().WithScheme(s).WithObjects(deploy).Build()
			},
			verify: func(t *testing.T, c client.Client) {
				err := c.Get(context.TODO(), types.NamespacedName{Name: "old-deploy",
					Namespace: "multicluster-engine"}, &appsv1.Deployment{})
				if err == nil {
					t.Errorf("expected old-deploy to be deleted, but it still exists")
				}
			},
		},
		{
			name:      "resource removed from chart but manually recreated without MCE label is left alone",
			component: "example",
			oldResources: []backplanev1.ManagedResource{
				newManagedResource("apps/v1", "Deployment", "adopted-deploy", "multicluster-engine"),
			},
			newResources: nil,
			setupClient: func(t *testing.T) client.Client {
				deploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "adopted-deploy",
						Namespace: "multicluster-engine",
						// No backplaneconfig.name label - simulates a resource manually recreated
						// by a user after MCE deleted it, or never owned by MCE.
					},
				}
				return fake.NewClientBuilder().WithScheme(s).WithObjects(deploy).Build()
			},
			verify: func(t *testing.T, c client.Client) {
				err := c.Get(context.TODO(), types.NamespacedName{Name: "adopted-deploy",
					Namespace: "multicluster-engine"}, &appsv1.Deployment{})
				if err != nil {
					t.Errorf("expected adopted-deploy to be left alone, but got error: %v", err)
				}
			},
		},
		{
			name:      "resource still present in current templates is not touched",
			component: "example",
			oldResources: []backplanev1.ManagedResource{
				newManagedResource("apps/v1", "Deployment", "kept-deploy", "multicluster-engine"),
			},
			newResources: []backplanev1.ManagedResource{
				newManagedResource("apps/v1", "Deployment", "kept-deploy", "multicluster-engine"),
			},
			setupClient: func(t *testing.T) client.Client {
				deploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kept-deploy",
						Namespace: "multicluster-engine",
						Labels:    map[string]string{"backplaneconfig.name": "mce"},
					},
				}
				return fake.NewClientBuilder().WithScheme(s).WithObjects(deploy).Build()
			},
			verify: func(t *testing.T, c client.Client) {
				err := c.Get(context.TODO(), types.NamespacedName{Name: "kept-deploy",
					Namespace: "multicluster-engine"}, &appsv1.Deployment{})
				if err != nil {
					t.Errorf("expected kept-deploy to still exist, got error: %v", err)
				}
			},
		},
		{
			name:         "legacy console-mce ServiceMonitor is cleaned up even with no tracked history (ACM-40355)",
			component:    backplanev1.ConsoleMCE,
			oldResources: nil, // Simulates an InternalEngineComponent CR from before resource tracking existed.
			newResources: nil,
			setupClient: func(t *testing.T) client.Client {
				sm := &promv1.ServiceMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "console-mce-monitor",
						Namespace: "multicluster-engine",
						Labels:    map[string]string{"backplaneconfig.name": "mce"},
					},
				}
				return fake.NewClientBuilder().WithScheme(s).WithObjects(sm).Build()
			},
			verify: func(t *testing.T, c client.Client) {
				err := c.Get(context.TODO(), types.NamespacedName{Name: "console-mce-monitor",
					Namespace: "multicluster-engine"}, &promv1.ServiceMonitor{})
				if err == nil {
					t.Errorf("expected legacy console-mce-monitor ServiceMonitor to be deleted, but it still exists")
				}
			},
		},
		{
			name:         "legacy console-mce ServiceMonitor absent from cluster is a no-op",
			component:    backplanev1.ConsoleMCE,
			oldResources: nil,
			newResources: nil,
			setupClient: func(t *testing.T) client.Client {
				return fake.NewClientBuilder().WithScheme(s).Build()
			},
			verify: func(t *testing.T, c client.Client) {
				// Nothing to verify beyond "no error/requeue", asserted below.
			},
		},
		{
			name:      "legacy cleanup does not run for unrelated components",
			component: "example",
			setupClient: func(t *testing.T) client.Client {
				sm := &promv1.ServiceMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "console-mce-monitor",
						Namespace: "multicluster-engine",
						Labels:    map[string]string{"backplaneconfig.name": "mce"},
					},
				}
				return fake.NewClientBuilder().WithScheme(s).WithObjects(sm).Build()
			},
			verify: func(t *testing.T, c client.Client) {
				err := c.Get(context.TODO(), types.NamespacedName{Name: "console-mce-monitor",
					Namespace: "multicluster-engine"}, &promv1.ServiceMonitor{})
				if err != nil {
					t.Errorf("console-mce-monitor should only be cleaned up for console-mce, got error: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.setupClient(t)
			r := &MultiClusterEngineReconciler{Client: c}

			result, err := r.cleanupOrphanedManagedResources(context.TODO(), mce, tt.component,
				tt.oldResources, tt.newResources)

			if err != nil {
				t.Errorf("cleanupOrphanedManagedResources() unexpected error: %v", err)
			}
			if tt.expectRequeue && result == (ctrl.Result{}) {
				t.Errorf("cleanupOrphanedManagedResources() expected requeue, got empty result")
			}
			if !tt.expectRequeue && result != (ctrl.Result{}) {
				t.Errorf("cleanupOrphanedManagedResources() expected no requeue, got %v", result)
			}

			if tt.verify != nil {
				tt.verify(t, c)
			}
		})
	}
}
