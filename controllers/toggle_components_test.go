package controllers

import (
	"context"
	"reflect"
	"testing"

	"github.com/pkg/errors"
	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/status"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func getComponent(components []backplanev1.ComponentCondition, name string) backplanev1.ComponentCondition {
	for i := range components {
		if components[i].Name == name {
			return components[i]
		}
	}
	return backplanev1.ComponentCondition{}
}

func Test_reconcileLocalHosting(t *testing.T) {
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	appsv1.AddToScheme(scheme)
	backplanev1.AddToScheme(scheme)
	addonv1alpha1.AddToScheme(scheme)

	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	images := map[string]string{}
	ctx := context.TODO()
	statusManager := &status.StatusTracker{Client: cl}
	r := &MultiClusterEngineReconciler{
		Client:        cl,
		Scheme:        cl.Scheme(),
		Images:        images,
		StatusManager: statusManager,
	}
	mce := &backplanev1.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name: BackplaneConfigName,
		},
		Spec: backplanev1.MultiClusterEngineSpec{
			TargetNamespace: DestinationNamespace,
			Overrides: &backplanev1.Overrides{
				Components: []backplanev1.ComponentConfig{
					{
						Name:    backplanev1.HypershiftLocalHosting,
						Enabled: true,
					},
					{
						Name:    backplanev1.LocalCluster,
						Enabled: true,
					},
				},
			},
		},
	}

	// Hypershift not enabled
	_, _ = r.reconcileHypershiftLocalHosting(ctx, mce)
	mceStatus := r.StatusManager.ReportStatus(*mce)
	component := getComponent(mceStatus.Components, "hypershift-addon")
	if component.Type != "NotPresent" || component.Status != metav1.ConditionTrue || component.Reason != status.ComponentDisabledReason {
		t.Error("component should not be present due to missing requirements")
	}
	r.StatusManager.Reset("")

	// LocalHosting not enabled
	mce.Spec.Overrides.Components = []backplanev1.ComponentConfig{
		{Name: backplanev1.HypershiftLocalHosting, Enabled: false},
	}
	_, _ = r.reconcileHypershiftLocalHosting(ctx, mce)
	mceStatus = r.StatusManager.ReportStatus(*mce)
	component = getComponent(mceStatus.Components, "hypershift-addon")
	if component.Type != "NotPresent" || component.Status != metav1.ConditionTrue || component.Reason != status.ComponentDisabledReason {
		t.Error("component should not be present because it is disabled")
	}
	r.StatusManager.Reset("")

	// Hypershift enabled but local-cluster namespace not present
	mce.Spec.Overrides.Components = []backplanev1.ComponentConfig{
		{Name: backplanev1.HypershiftLocalHosting, Enabled: true},
		{Name: backplanev1.HyperShift, Enabled: true},
		{Name: backplanev1.LocalCluster, Enabled: true},
	}
	_, _ = r.reconcileHypershiftLocalHosting(ctx, mce)
	mceStatus = r.StatusManager.ReportStatus(*mce)
	component = getComponent(mceStatus.Components, "hypershift-addon")
	if component.Reason != status.WaitingForResourceReason {
		t.Error("component status should indicate it's waiting on another resource")
	}
	r.StatusManager.Reset("")

	// Hypershift enabled and local-cluster namespace present
	localns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "local-cluster",
		},
	}
	err := cl.Create(ctx, localns)
	if err != nil {
		t.Error("error creating namespace with fake client")
	}
	retrievedNS := &corev1.Namespace{}
	err = cl.Get(ctx, types.NamespacedName{Name: "local-cluster"}, retrievedNS)
	if err != nil {
		t.Errorf("error getting ManagedClusterAddOn: %s", err.Error())
	}

	// reconcile is not successful likely due to Server-Side Apply
	_, _ = r.reconcileHypershiftLocalHosting(ctx, mce)
	mceStatus = r.StatusManager.ReportStatus(*mce)
	component = getComponent(mceStatus.Components, "hypershift-addon")
	if component.Type != "Available" {
		t.Errorf("Got status %s, expected %s", component.Type, "Available")
	}
	r.StatusManager.Reset("")
}

func Test_HypershiftPreview(t *testing.T) {
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	appsv1.AddToScheme(scheme)
	backplanev1.AddToScheme(scheme)
	addonv1alpha1.AddToScheme(scheme)
	rbacv1.AddToScheme(scheme)

	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	images := map[string]string{}
	// ctx := context.TODO()
	statusManager := &status.StatusTracker{Client: cl}
	r := &MultiClusterEngineReconciler{
		Client:        cl,
		Scheme:        cl.Scheme(),
		Images:        images,
		StatusManager: statusManager,
	}

	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "open-cluster-management:hypershift-preview:hypershift-addon-manager", // Name of the cluster role
		},
		Rules: []rbacv1.PolicyRule{}, // No rules, so it's empty
	}

	err := cl.Create(context.Background(), clusterRole)
	if err != nil {
		wrappedErr := errors.Wrap(err, "failed to create ClusterRole")

		t.Errorf(wrappedErr.Error())
	}

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "open-cluster-management:hypershift-preview:hypershift-addon-manager", // Name of the ClusterRoleBinding
		},
		// No subjects and no roleRef, meaning it doesn't bind any ClusterRole
		Subjects: []rbacv1.Subject{},
		RoleRef:  rbacv1.RoleRef{},
	}

	// Create the ClusterRoleBinding in the fake client
	err = cl.Create(context.Background(), clusterRoleBinding)
	if err != nil {
		wrappedErr := errors.Wrap(err, "failed to create ClusterRoleBinding")

		t.Errorf(wrappedErr.Error())
	}
	_, err = r.removeDeprecatedRBAC(context.Background())
	if err != nil {
		wrappedErr := errors.Wrap(err, "failed to delete")

		t.Errorf(wrappedErr.Error())
	}

}

func Test_clusterManagementAddOnNotFoundStatus(t *testing.T) {
	type args struct {
		name      string
		namespace string
	}
	tests := []struct {
		name string
		args args
		want status.StatusReporter
	}{
		{
			name: "create static status",
			args: args{
				name:      "new-component",
				namespace: "new-namespace",
			},
			want: status.StaticStatus{
				NamespacedName: types.NamespacedName{Name: "new-component", Namespace: "new-namespace"},
				Kind:           "Component",
				Condition: backplanev1.ComponentCondition{
					Type:      "Available",
					Name:      "new-component",
					Status:    metav1.ConditionFalse,
					Reason:    status.WaitingForResourceReason,
					Kind:      "Component",
					Available: false,
					Message:   "Waiting for ClusterManagementAddOn CRD to be available",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clusterManagementAddOnNotFoundStatus(tt.args.name, tt.args.namespace); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("clusterManagementAddOnNotFoundStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}
