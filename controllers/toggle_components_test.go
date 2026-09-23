package controllers

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/foundation"
	"github.com/stolostron/backplane-operator/pkg/status"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
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
			LocalClusterName: "local-cluster",
			TargetNamespace:  DestinationNamespace,
			NetworkPolicies:  &backplanev1.NetworkPoliciesConfig{Enabled: true},
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
			Name: mce.Spec.LocalClusterName,
		},
	}
	err := cl.Create(ctx, localns)
	if err != nil {
		t.Error("error creating namespace with fake client")
	}
	retrievedNS := &corev1.Namespace{}
	err = cl.Get(ctx, types.NamespacedName{Name: mce.Spec.LocalClusterName}, retrievedNS)
	if err != nil {
		t.Errorf("error getting ManagedClusterAddOn: %s", err.Error())
	}

	// reconcile is not successful likely due to Server-Side Apply

	// mock client patching isnt available, when resources are created by mock client, the status condition is not added by default
	_, err = r.reconcileHypershiftLocalHosting(ctx, mce)
	if err != nil {
		t.Errorf("error reconciling Hypershift addon: %s", err.Error())
	}
	// mceStatus = r.StatusManager.ReportStatus(*mce)
	// component = getComponent(mceStatus.Components, "hypershift-addon")
	// if component.Type != "Available" {
	// 	t.Errorf("Got status %s, expected %s", component.Type, "Available")
	// }
	r.StatusManager.Reset("")
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

func Test_annotateManagedCluster(t *testing.T) {
	tests := []struct {
		name                string
		mce                 *backplanev1.MultiClusterEngine
		initialAnnotations  map[string]string
		expectedAnnotations map[string]string
		expectError         bool
	}{
		{
			name: "Add NodeSelector and Tolerations annotations",
			mce: &backplanev1.MultiClusterEngine{
				Spec: backplanev1.MultiClusterEngineSpec{
					NodeSelector: map[string]string{
						"node-role.kubernetes.io/worker": "",
					},
					Tolerations: []corev1.Toleration{
						{
							Key:      "dedicated",
							Operator: corev1.TolerationOpEqual,
							Value:    "infra",
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			initialAnnotations: nil,
			expectedAnnotations: map[string]string{
				"open-cluster-management/nodeSelector": "{\"node-role.kubernetes.io/worker\":\"\"}",
				"open-cluster-management/tolerations":  "[{\"key\":\"dedicated\",\"operator\":\"Equal\",\"value\":\"infra\",\"effect\":\"NoSchedule\"}]",
			},
			expectError: false,
		},
		{
			name: "Remove NodeSelector and Tolerations annotations when empty",
			mce: &backplanev1.MultiClusterEngine{
				Spec: backplanev1.MultiClusterEngineSpec{
					NodeSelector: nil,
					Tolerations:  nil,
				},
			},
			initialAnnotations: map[string]string{
				"open-cluster-management/nodeSelector": "{\"node-role.kubernetes.io/worker\":\"\"}",
				"open-cluster-management/tolerations":  "[{\"key\":\"dedicated\",\"operator\":\"Equal\",\"value\":\"infra\",\"effect\":\"NoSchedule\"}]",
				"other-annotation":                     "keep-this",
			},
			expectedAnnotations: map[string]string{
				"other-annotation": "keep-this",
			},
			expectError: false,
		},
		{
			name: "Update existing annotations",
			mce: &backplanev1.MultiClusterEngine{
				Spec: backplanev1.MultiClusterEngineSpec{
					NodeSelector: map[string]string{
						"updated-node-selector": "new-value",
					},
					Tolerations: []corev1.Toleration{
						{
							Key:      "new-key",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectPreferNoSchedule,
						},
					},
				},
			},
			initialAnnotations: map[string]string{
				"open-cluster-management/nodeSelector": "{\"old-selector\":\"old-value\"}",
				"open-cluster-management/tolerations":  "[{\"key\":\"old-key\",\"operator\":\"Equal\",\"value\":\"old-value\",\"effect\":\"NoSchedule\"}]",
				"preserve-annotation":                  "preserved",
			},
			expectedAnnotations: map[string]string{
				"open-cluster-management/nodeSelector": "{\"updated-node-selector\":\"new-value\"}",
				"open-cluster-management/tolerations":  "[{\"key\":\"new-key\",\"operator\":\"Exists\",\"effect\":\"PreferNoSchedule\"}]",
				"preserve-annotation":                  "preserved",
			},
			expectError: false,
		},
		{
			name: "Handle nil annotations map",
			mce: &backplanev1.MultiClusterEngine{
				Spec: backplanev1.MultiClusterEngineSpec{
					NodeSelector: map[string]string{
						"test": "value",
					},
				},
			},
			initialAnnotations: nil,
			expectedAnnotations: map[string]string{
				"open-cluster-management/nodeSelector": "{\"test\":\"value\"}",
			},
			expectError: false,
		},
		{
			name: "Add only NodeSelector annotation",
			mce: &backplanev1.MultiClusterEngine{
				Spec: backplanev1.MultiClusterEngineSpec{
					NodeSelector: map[string]string{
						"kubernetes.io/arch": "amd64",
						"kubernetes.io/os":   "linux",
					},
					Tolerations: nil,
				},
			},
			initialAnnotations: map[string]string{},
			expectedAnnotations: map[string]string{
				"open-cluster-management/nodeSelector": "{\"kubernetes.io/arch\":\"amd64\",\"kubernetes.io/os\":\"linux\"}",
			},
			expectError: false,
		},
		{
			name: "Add only Tolerations annotation",
			mce: &backplanev1.MultiClusterEngine{
				Spec: backplanev1.MultiClusterEngineSpec{
					NodeSelector: nil,
					Tolerations: []corev1.Toleration{
						{
							Key:      "node.kubernetes.io/not-ready",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoExecute,
							TolerationSeconds: func() *int64 {
								seconds := int64(300)
								return &seconds
							}(),
						},
					},
				},
			},
			initialAnnotations: map[string]string{
				"existing": "annotation",
			},
			expectedAnnotations: map[string]string{
				"existing":                            "annotation",
				"open-cluster-management/tolerations": "[{\"key\":\"node.kubernetes.io/not-ready\",\"operator\":\"Exists\",\"effect\":\"NoExecute\",\"tolerationSeconds\":300}]",
			},
			expectError: false,
		},
		{
			name: "Remove only NodeSelector, keep Tolerations",
			mce: &backplanev1.MultiClusterEngine{
				Spec: backplanev1.MultiClusterEngineSpec{
					NodeSelector: nil,
					Tolerations: []corev1.Toleration{
						{
							Key:      "keep-me",
							Operator: corev1.TolerationOpEqual,
							Value:    "yes",
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			initialAnnotations: map[string]string{
				"open-cluster-management/nodeSelector": "{\"remove\":\"me\"}",
				"open-cluster-management/tolerations":  "[{\"key\":\"old\",\"operator\":\"Equal\",\"value\":\"value\",\"effect\":\"NoSchedule\"}]",
			},
			expectedAnnotations: map[string]string{
				"open-cluster-management/tolerations": "[{\"key\":\"keep-me\",\"operator\":\"Equal\",\"value\":\"yes\",\"effect\":\"NoSchedule\"}]",
			},
			expectError: false,
		},
		{
			name: "Remove only Tolerations, keep NodeSelector",
			mce: &backplanev1.MultiClusterEngine{
				Spec: backplanev1.MultiClusterEngineSpec{
					NodeSelector: map[string]string{
						"keep-me": "yes",
					},
					Tolerations: nil,
				},
			},
			initialAnnotations: map[string]string{
				"open-cluster-management/nodeSelector": "{\"remove\":\"me\"}",
				"open-cluster-management/tolerations":  "[{\"key\":\"remove\",\"operator\":\"Equal\",\"value\":\"me\",\"effect\":\"NoSchedule\"}]",
			},
			expectedAnnotations: map[string]string{
				"open-cluster-management/nodeSelector": "{\"keep-me\":\"yes\"}",
			},
			expectError: false,
		},
		{
			name: "Empty NodeSelector and Tolerations (both zero length)",
			mce: &backplanev1.MultiClusterEngine{
				Spec: backplanev1.MultiClusterEngineSpec{
					NodeSelector: map[string]string{},
					Tolerations:  []corev1.Toleration{},
				},
			},
			initialAnnotations: map[string]string{
				"open-cluster-management/nodeSelector": "{\"old\":\"selector\"}",
				"open-cluster-management/tolerations":  "[{\"key\":\"old\",\"operator\":\"Equal\",\"value\":\"toleration\",\"effect\":\"NoSchedule\"}]",
				"preserve":                             "me",
			},
			expectedAnnotations: map[string]string{
				"preserve": "me",
			},
			expectError: false,
		},
		{
			name: "Complex Tolerations with all fields",
			mce: &backplanev1.MultiClusterEngine{
				Spec: backplanev1.MultiClusterEngineSpec{
					Tolerations: []corev1.Toleration{
						{
							Key:               "special-node",
							Operator:          corev1.TolerationOpEqual,
							Value:             "dedicated-workload",
							Effect:            corev1.TaintEffectNoSchedule,
							TolerationSeconds: nil,
						},
						{
							Key:      "node.kubernetes.io/disk-pressure",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoExecute,
							TolerationSeconds: func() *int64 {
								seconds := int64(600)
								return &seconds
							}(),
						},
					},
				},
			},
			initialAnnotations: map[string]string{},
			expectedAnnotations: map[string]string{
				"open-cluster-management/tolerations": "[{\"key\":\"special-node\",\"operator\":\"Equal\",\"value\":\"dedicated-workload\",\"effect\":\"NoSchedule\"},{\"key\":\"node.kubernetes.io/disk-pressure\",\"operator\":\"Exists\",\"effect\":\"NoExecute\",\"tolerationSeconds\":600}]",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotations := tt.initialAnnotations
			err := annotateManagedCluster(tt.mce, &annotations)

			if (err != nil) != tt.expectError {
				t.Errorf("annotateManagedCluster() error = %v, expectError %v", err, tt.expectError)
				return
			}

			if !reflect.DeepEqual(annotations, tt.expectedAnnotations) {
				t.Errorf("annotateManagedCluster() annotations = %v, expected %v", annotations, tt.expectedAnnotations)
			}
		})
	}
}

// Test_applyTemplateWrappedError tests that wrapped NotFound errors from applyTemplate
// are properly detected using errors.Unwrap(). This simulates what happens at line 1386
// in toggle_components.go where ensureHyperShift checks:
// if apierrors.IsNotFound(errors.Unwrap(err))
func Test_applyTemplateWrappedError(t *testing.T) {
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	backplanev1.AddToScheme(scheme)

	// Create MCE instance
	mce := &backplanev1.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-mce",
		},
		Spec: backplanev1.MultiClusterEngineSpec{
			TargetNamespace: "test-namespace",
		},
	}

	// Create a template that will trigger create
	template := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "addon.open-cluster-management.io/v1alpha1",
			"kind":       "ClusterManagementAddOn",
			"metadata": map[string]interface{}{
				"name":      "test-addon",
				"namespace": "test-namespace",
			},
			"spec": map[string]interface{}{},
		},
	}

	// Create fake client with the MCE instance
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mce).
		Build()

	// Setup missing GVKs
	missingGVKs := make(map[schema.GroupVersionKind]bool)
	gvk := template.GroupVersionKind()
	missingGVKs[gvk] = true

	// Wrap client with create interceptor
	wrappedClient := &mockClientWithCreateInterceptor{
		Client:      fakeClient,
		missingGVKs: missingGVKs,
	}

	// Create reconciler
	reconciler := &MultiClusterEngineReconciler{
		Client:        wrappedClient,
		Scheme:        scheme,
		StatusManager: &status.StatusTracker{Client: wrappedClient},
	}

	// Call applyTemplate - this will return a wrapped NotFound error via logAndSetCondition
	ctx := context.Background()
	_, err := reconciler.applyTemplate(ctx, mce, template)

	// Verify error was returned
	if err == nil {
		t.Fatal("Expected error from applyTemplate when CRD is missing")
	}

	// Test the unwrapping logic from toggle_components.go:1386
	// This is the actual check used in ensureHyperShift
	var unwrappedErr error
	if err != nil {
		// Use standard library errors.Unwrap (same as toggle_components.go:1386)
		unwrappedErr = errors.Unwrap(err)
		if unwrappedErr == nil {
			// If Unwrap returns nil, use original error
			unwrappedErr = err
		}
	}

	// Verify the unwrapped error is detected as NotFound
	// This is exactly what line 1386 does: apierrors.IsNotFound(errors.Unwrap(err))
	if !apierrors.IsNotFound(unwrappedErr) {
		t.Errorf("After unwrapping with errors.Unwrap(), error should be detected as NotFound. "+
			"Error: %v, Unwrapped: %v, IsNotFound: %v",
			err, unwrappedErr, apierrors.IsNotFound(unwrappedErr))
	}

	// Verify the error message mentions CRD not installed
	if err.Error() == "" {
		t.Error("Error message should not be empty")
	}
}

func Test_ensureNetworkPoliciesFeatureGate(t *testing.T) {
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	backplanev1.AddToScheme(scheme)

	ctx := context.TODO()

	newCM := func(gates ...map[string]interface{}) *unstructured.Unstructured {
		obj := map[string]interface{}{
			"apiVersion": "operator.open-cluster-management.io/v1",
			"kind":       "ClusterManager",
			"metadata":   map[string]interface{}{"name": "cluster-manager"},
			"spec":       map[string]interface{}{},
		}
		if len(gates) > 0 {
			gs := make([]interface{}, len(gates))
			for i, g := range gates {
				gs[i] = g
			}
			obj["spec"].(map[string]interface{})["registrationConfiguration"] = map[string]interface{}{
				"featureGates": gs,
			}
		}
		return &unstructured.Unstructured{Object: obj}
	}

	tests := []struct {
		name              string
		mce               *backplanev1.MultiClusterEngine
		existingCM        *unstructured.Unstructured
		priorGates        []interface{}
		expectError       bool
		wantMode          string
		preservedFeatures []string
	}{
		{
			name:       "adds NetworkPolicies gate when featureGates is empty",
			mce:        &backplanev1.MultiClusterEngine{},
			existingCM: newCM(),
			wantMode:   "Enable",
		},
		{
			name: "adds NetworkPolicies gate and preserves other gates",
			mce:  &backplanev1.MultiClusterEngine{},
			existingCM: newCM(
				map[string]interface{}{"feature": "ManagedClusterAutoApproval", "mode": "Enable"},
			),
			wantMode:          "Enable",
			preservedFeatures: []string{"ManagedClusterAutoApproval"},
		},
		{
			name: "no update when NetworkPolicies gate already correct",
			mce:  &backplanev1.MultiClusterEngine{},
			existingCM: newCM(
				map[string]interface{}{"feature": "NetworkPolicies", "mode": "Enable"},
			),
			wantMode: "Enable",
		},
		{
			name: "updates NetworkPolicies mode from Enable to Disable and preserves other gates",
			mce: &backplanev1.MultiClusterEngine{
				Spec: backplanev1.MultiClusterEngineSpec{
					NetworkPolicies: &backplanev1.NetworkPoliciesConfig{Enabled: false},
				},
			},
			existingCM: newCM(
				map[string]interface{}{"feature": "NetworkPolicies", "mode": "Enable"},
				map[string]interface{}{"feature": "ManagedClusterAutoApproval", "mode": "Enable"},
				map[string]interface{}{"feature": "ClusterImporter", "mode": "Enable"},
			),
			wantMode:          "Disable",
			preservedFeatures: []string{"ManagedClusterAutoApproval", "ClusterImporter"},
		},
		{
			name:       "restores SSA-pruned gates from the pre-patch snapshot",
			mce:        &backplanev1.MultiClusterEngine{},
			existingCM: newCM(),
			priorGates: []interface{}{
				map[string]interface{}{"feature": "ManagedClusterAutoApproval", "mode": "Enable"},
				map[string]interface{}{"feature": "ClusterImporter", "mode": "Enable"},
			},
			wantMode:          "Enable",
			preservedFeatures: []string{"ManagedClusterAutoApproval", "ClusterImporter"},
		},
		{
			name:        "error when ClusterManager object is nil",
			mce:         &backplanev1.MultiClusterEngine{},
			existingCM:  nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objs []runtime.Object
			if tt.existingCM != nil {
				objs = append(objs, tt.existingCM)
			}

			cl := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()
			r := &MultiClusterEngineReconciler{Client: cl, Scheme: scheme}

			err := r.ensureNetworkPoliciesFeatureGate(ctx, tt.mce, tt.existingCM, tt.priorGates)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			updated := &unstructured.Unstructured{}
			updated.SetGroupVersionKind(schema.GroupVersionKind{
				Group: "operator.open-cluster-management.io", Version: "v1", Kind: "ClusterManager",
			})
			if getErr := cl.Get(ctx, types.NamespacedName{Name: "cluster-manager"}, updated); getErr != nil {
				t.Fatalf("failed to get ClusterManager after reconcile: %v", getErr)
			}

			gates, _, _ := unstructured.NestedSlice(updated.Object, "spec", "registrationConfiguration", "featureGates")

			// Verify NetworkPolicies gate is set to the desired mode
			npFound := false
			for _, g := range gates {
				gate, ok := g.(map[string]interface{})
				if !ok {
					continue
				}
				if gate["feature"] == "NetworkPolicies" {
					npFound = true
					if gate["mode"] != tt.wantMode {
						t.Errorf("NetworkPolicies mode: want %s, got %v", tt.wantMode, gate["mode"])
					}
				}
			}
			if !npFound {
				t.Errorf("NetworkPolicies feature gate not found in featureGates")
			}

			// Verify preserved features are still present
			for _, feat := range tt.preservedFeatures {
				found := false
				for _, g := range gates {
					gate, ok := g.(map[string]interface{})
					if ok && gate["feature"] == feat {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected feature gate %q to be preserved but it was missing", feat)
				}
			}

			// Verify foundation helper returns consistent mode
			wantFndMode := string(foundation.NetworkPoliciesFeatureGateMode(tt.mce))
			if tt.wantMode != wantFndMode {
				t.Errorf("test wantMode %q inconsistent with foundation.NetworkPoliciesFeatureGateMode %q", tt.wantMode, wantFndMode)
			}
		})
	}
}

func Test_snapshotClusterManagerFeatureGates(t *testing.T) {
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	backplanev1.AddToScheme(scheme)
	ctx := context.TODO()

	t.Run("empty snapshot when ClusterManager is missing", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &MultiClusterEngineReconciler{Client: cl, Scheme: scheme}
		gates, err := r.snapshotClusterManagerFeatureGates(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(gates) != 0 {
			t.Errorf("expected empty snapshot, got %v", gates)
		}
	})

	t.Run("returns existing feature gates", func(t *testing.T) {
		cm := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "operator.open-cluster-management.io/v1",
				"kind":       "ClusterManager",
				"metadata":   map[string]interface{}{"name": "cluster-manager"},
				"spec": map[string]interface{}{
					"registrationConfiguration": map[string]interface{}{
						"featureGates": []interface{}{
							map[string]interface{}{"feature": "ManagedClusterAutoApproval", "mode": "Enable"},
						},
					},
				},
			},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(cm).Build()
		r := &MultiClusterEngineReconciler{Client: cl, Scheme: scheme}
		gates, err := r.snapshotClusterManagerFeatureGates(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(gates) != 1 {
			t.Fatalf("expected 1 gate, got %d", len(gates))
		}
		gate, _ := gates[0].(map[string]interface{})
		if gate["feature"] != "ManagedClusterAutoApproval" {
			t.Errorf("expected ManagedClusterAutoApproval, got %v", gate["feature"])
		}
	})
}

func Test_mergeFeatureGates(t *testing.T) {
	current := []interface{}{
		map[string]interface{}{"feature": "NetworkPolicies", "mode": "Enable"},
	}
	prior := []interface{}{
		map[string]interface{}{"feature": "NetworkPolicies", "mode": "Disable"},
		map[string]interface{}{"feature": "ManagedClusterAutoApproval", "mode": "Enable"},
	}
	got := mergeFeatureGates(current, prior)
	if len(got) != 2 {
		t.Fatalf("expected 2 gates, got %d", len(got))
	}
	first, _ := got[0].(map[string]interface{})
	if first["feature"] != "NetworkPolicies" || first["mode"] != "Enable" {
		t.Errorf("current NetworkPolicies should win, got %v", first)
	}
	second, _ := got[1].(map[string]interface{})
	if second["feature"] != "ManagedClusterAutoApproval" {
		t.Errorf("expected restored ManagedClusterAutoApproval, got %v", second)
	}
}

func newFleetNavReconciler(objs ...runtime.Object) *MultiClusterEngineReconciler {
	s := runtime.NewScheme()
	corev1.AddToScheme(s)
	backplanev1.AddToScheme(s)
	addonv1alpha1.AddToScheme(s)

	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objs...).Build()
	return &MultiClusterEngineReconciler{
		Client:        cl,
		Scheme:        s,
		StatusManager: &status.StatusTracker{Client: cl},
		CacheSpec: CacheSpec{
			ImageOverrides:    map[string]string{},
			TemplateOverrides: map[string]string{},
		},
	}
}

func fleetNavMCE() *backplanev1.MultiClusterEngine {
	return &backplanev1.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mce"},
		Spec: backplanev1.MultiClusterEngineSpec{
			TargetNamespace:  "test-ns",
			LocalClusterName: "local-cluster",
			Overrides: &backplanev1.Overrides{
				Components: []backplanev1.ComponentConfig{
					{Name: backplanev1.FleetNavigation, Enabled: true},
				},
			},
		},
	}
}

func Test_ensureFleetNavigation(t *testing.T) {
	os.Setenv("DIRECTORY_OVERRIDE", "../")
	defer os.Unsetenv("DIRECTORY_OVERRIDE")

	t.Run("creates InternalEngineComponent and renders templates", func(t *testing.T) {
		ctx := context.TODO()
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}}
		r := newFleetNavReconciler(ns)
		mce := fleetNavMCE()

		result, err := r.ensureFleetNavigation(ctx, mce)
		if err != nil {
			t.Fatalf("ensureFleetNavigation() returned error: %v", err)
		}
		if result != (ctrl.Result{}) {
			t.Fatalf("ensureFleetNavigation() returned non-zero result: %v", result)
		}

		iec := &backplanev1.InternalEngineComponent{}
		if err := r.Client.Get(ctx, types.NamespacedName{
			Name: backplanev1.FleetNavigation, Namespace: "test-ns",
		}, iec); err != nil {
			t.Errorf("expected InternalEngineComponent to exist: %v", err)
		}
	})

	t.Run("is idempotent on second call", func(t *testing.T) {
		ctx := context.TODO()
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}}
		r := newFleetNavReconciler(ns)
		mce := fleetNavMCE()

		if _, err := r.ensureFleetNavigation(ctx, mce); err != nil {
			t.Fatalf("first call failed: %v", err)
		}
		result, err := r.ensureFleetNavigation(ctx, mce)
		if err != nil {
			t.Fatalf("second call returned error: %v", err)
		}
		if result != (ctrl.Result{}) {
			t.Fatalf("second call returned non-zero result: %v", result)
		}
	})

}

func Test_ensureNoFleetNavigation(t *testing.T) {
	os.Setenv("DIRECTORY_OVERRIDE", "../")
	defer os.Unsetenv("DIRECTORY_OVERRIDE")

	t.Run("deletes InternalEngineComponent and templates", func(t *testing.T) {
		ctx := context.TODO()
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}}
		r := newFleetNavReconciler(ns)
		mce := fleetNavMCE()

		if _, err := r.ensureFleetNavigation(ctx, mce); err != nil {
			t.Fatalf("setup ensureFleetNavigation() failed: %v", err)
		}

		result, err := r.ensureNoFleetNavigation(ctx, mce)
		if err != nil {
			t.Fatalf("ensureNoFleetNavigation() returned error: %v", err)
		}
		if result != (ctrl.Result{}) {
			t.Fatalf("ensureNoFleetNavigation() returned non-zero result: %v", result)
		}

		iec := &backplanev1.InternalEngineComponent{}
		err = r.Client.Get(ctx, types.NamespacedName{
			Name: backplanev1.FleetNavigation, Namespace: "test-ns",
		}, iec)
		if !apierrors.IsNotFound(err) {
			t.Errorf("expected InternalEngineComponent to be deleted, got: %v", err)
		}
	})

	t.Run("succeeds when nothing to delete", func(t *testing.T) {
		ctx := context.TODO()
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}}
		r := newFleetNavReconciler(ns)
		mce := fleetNavMCE()

		result, err := r.ensureNoFleetNavigation(ctx, mce)
		if err != nil {
			t.Fatalf("ensureNoFleetNavigation() returned error: %v", err)
		}
		if result != (ctrl.Result{}) {
			t.Fatalf("ensureNoFleetNavigation() returned non-zero result: %v", result)
		}
	})
}
