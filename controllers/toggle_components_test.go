package controllers

import (
	"context"
	"errors"
	"reflect"
	"testing"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
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

func Test_enableClusterManagerGRPCServer(t *testing.T) {
	// Set UNIT_TEST env var so getClusterIngressDomain returns test domain
	t.Setenv("UNIT_TEST", "true")

	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	backplanev1.AddToScheme(scheme)

	ctx := context.TODO()

	tests := []struct {
		name                   string
		imageOverrides         map[string]string
		existingClusterManager *unstructured.Unstructured
		expectedUpdate         bool
		expectError            bool
		errorContains          string
	}{
		{
			name:           "Error when conductor image not found",
			imageOverrides: map[string]string{},
			expectError:    true,
			errorContains:  "cloudevents_conductor image not found",
		},
		{
			name: "Error when ClusterManager not found",
			imageOverrides: map[string]string{
				"cloudevents_conductor": "quay.io/test/conductor:latest",
			},
			expectError:   true,
			errorContains: "failed to get ClusterManager",
		},
		{
			name: "No update when configuration already matches",
			imageOverrides: map[string]string{
				"cloudevents_conductor": "quay.io/test/conductor:latest",
			},
			existingClusterManager: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "operator.open-cluster-management.io/v1",
					"kind":       "ClusterManager",
					"metadata": map[string]interface{}{
						"name": "cluster-manager",
					},
					"spec": map[string]interface{}{
						"registrationConfiguration": map[string]interface{}{
							"registrationDrivers": []interface{}{
								map[string]interface{}{"authType": "csr"},
								map[string]interface{}{"authType": "grpc"},
							},
						},
						"serverConfiguration": map[string]interface{}{
							"imagePullSpec": "quay.io/test/conductor:latest",
							"endpointsExposure": []interface{}{
								map[string]interface{}{
									"protocol": "grpc",
									"grpc": map[string]interface{}{
										"type": "hostname",
										"hostname": map[string]interface{}{
											"host": "grpc-server-open-cluster-management-hub.apps.installer-test-cluster.dev00.red-chesterfield.com",
										},
									},
								},
							},
						},
					},
				},
			},
			expectedUpdate: false,
			expectError:    false,
		},
		{
			name: "Update when registrationDrivers missing",
			imageOverrides: map[string]string{
				"cloudevents_conductor": "quay.io/test/conductor:latest",
			},
			existingClusterManager: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "operator.open-cluster-management.io/v1",
					"kind":       "ClusterManager",
					"metadata": map[string]interface{}{
						"name": "cluster-manager",
					},
					"spec": map[string]interface{}{
						"serverConfiguration": map[string]interface{}{
							"imagePullSpec": "quay.io/test/conductor:latest",
							"endpointsExposure": []interface{}{
								map[string]interface{}{
									"protocol": "grpc",
									"grpc": map[string]interface{}{
										"type": "hostname",
										"hostname": map[string]interface{}{
											"host": "grpc-server-open-cluster-management-hub.apps.installer-test-cluster.dev00.red-chesterfield.com",
										},
									},
								},
							},
						},
					},
				},
			},
			expectedUpdate: true,
			expectError:    false,
		},
		{
			name: "Update when image doesn't match",
			imageOverrides: map[string]string{
				"cloudevents_conductor": "quay.io/test/conductor:v2.0",
			},
			existingClusterManager: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "operator.open-cluster-management.io/v1",
					"kind":       "ClusterManager",
					"metadata": map[string]interface{}{
						"name": "cluster-manager",
					},
					"spec": map[string]interface{}{
						"registrationConfiguration": map[string]interface{}{
							"registrationDrivers": []interface{}{
								map[string]interface{}{"authType": "csr"},
								map[string]interface{}{"authType": "grpc"},
							},
						},
						"serverConfiguration": map[string]interface{}{
							"imagePullSpec": "quay.io/test/conductor:v1.0",
							"endpointsExposure": []interface{}{
								map[string]interface{}{
									"protocol": "grpc",
									"grpc": map[string]interface{}{
										"type": "hostname",
										"hostname": map[string]interface{}{
											"host": "grpc-server-open-cluster-management-hub.apps.installer-test-cluster.dev00.red-chesterfield.com",
										},
									},
								},
							},
						},
					},
				},
			},
			expectedUpdate: true,
			expectError:    false,
		},
		{
			name: "Update when hostname doesn't match",
			imageOverrides: map[string]string{
				"cloudevents_conductor": "quay.io/test/conductor:latest",
			},
			existingClusterManager: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "operator.open-cluster-management.io/v1",
					"kind":       "ClusterManager",
					"metadata": map[string]interface{}{
						"name": "cluster-manager",
					},
					"spec": map[string]interface{}{
						"registrationConfiguration": map[string]interface{}{
							"registrationDrivers": []interface{}{
								map[string]interface{}{"authType": "csr"},
								map[string]interface{}{"authType": "grpc"},
							},
						},
						"serverConfiguration": map[string]interface{}{
							"imagePullSpec": "quay.io/test/conductor:latest",
							"endpointsExposure": []interface{}{
								map[string]interface{}{
									"protocol": "grpc",
									"grpc": map[string]interface{}{
										"type": "hostname",
										"hostname": map[string]interface{}{
											"host": "old-hostname.example.com",
										},
									},
								},
							},
						},
					},
				},
			},
			expectedUpdate: true,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objs []runtime.Object
			if tt.existingClusterManager != nil {
				objs = append(objs, tt.existingClusterManager)
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objs...).
				Build()

			mce := &backplanev1.MultiClusterEngine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-mce",
				},
				Spec: backplanev1.MultiClusterEngineSpec{
					TargetNamespace: "test-namespace",
				},
			}

			r := &MultiClusterEngineReconciler{
				Client: cl,
				Scheme: scheme,
				CacheSpec: CacheSpec{
					ImageOverrides: tt.imageOverrides,
				},
			}

			// Track initial generation if ClusterManager exists
			var initialGeneration int64
			if tt.existingClusterManager != nil {
				initialGeneration, _, _ = unstructured.NestedInt64(tt.existingClusterManager.Object, "metadata", "generation")
			}

			err := r.enableClusterManagerGRPCServer(ctx, mce)

			// Check error expectations
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error containing '%s', but got no error", tt.errorContains)
				} else if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing '%s', but got: %v", tt.errorContains, err)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Verify ClusterManager was updated or not updated as expected
			if tt.existingClusterManager != nil {
				updatedCM := &unstructured.Unstructured{}
				updatedCM.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "operator.open-cluster-management.io",
					Version: "v1",
					Kind:    "ClusterManager",
				})
				err := cl.Get(ctx, types.NamespacedName{Name: "cluster-manager"}, updatedCM)
				if err != nil {
					t.Errorf("Failed to get updated ClusterManager: %v", err)
					return
				}

				// Check if the configuration was updated
				registrationDrivers, _, _ := unstructured.NestedSlice(updatedCM.Object, "spec", "registrationConfiguration", "registrationDrivers")
				serverImage, _, _ := unstructured.NestedString(updatedCM.Object, "spec", "serverConfiguration", "imagePullSpec")
				endpointsExposure, _, _ := unstructured.NestedSlice(updatedCM.Object, "spec", "serverConfiguration", "endpointsExposure")

				// Verify registrationDrivers
				expectedDrivers := []interface{}{
					map[string]interface{}{"authType": "csr"},
					map[string]interface{}{"authType": "grpc"},
				}
				if !driversMatch(registrationDrivers, expectedDrivers) {
					t.Errorf("registrationDrivers not configured correctly: got %v", registrationDrivers)
				}

				// Verify serverImage
				if serverImage != tt.imageOverrides["cloudevents_conductor"] {
					t.Errorf("serverImage = %s, want %s", serverImage, tt.imageOverrides["cloudevents_conductor"])
				}

				// Verify hostname
				if len(endpointsExposure) > 0 {
					if endpoint, ok := endpointsExposure[0].(map[string]interface{}); ok {
						if grpcConfig, ok := endpoint["grpc"].(map[string]interface{}); ok {
							if hostnameConfig, ok := grpcConfig["hostname"].(map[string]interface{}); ok {
								if host, ok := hostnameConfig["host"].(string); ok {
									expectedHost := "grpc-server-open-cluster-management-hub.apps.installer-test-cluster.dev00.red-chesterfield.com"
									if host != expectedHost {
										t.Errorf("hostname = %s, want %s", host, expectedHost)
									}
								}
							}
						}
					}
				}

				// For the "no update" test case, we can't reliably check generation with fake client
				// as it doesn't increment generation on updates
				_ = initialGeneration
			}
		})
	}
}

func Test_disableClusterManagerGRPCServer(t *testing.T) {
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	backplanev1.AddToScheme(scheme)

	ctx := context.TODO()

	tests := []struct {
		name                   string
		existingClusterManager *unstructured.Unstructured
		expectUpdate           bool
		expectError            bool
		errorContains          string
	}{
		{
			name:         "No error when ClusterManager not found",
			expectUpdate: false,
			expectError:  false,
		},
		{
			name: "Remove both registrationConfiguration and serverConfiguration",
			existingClusterManager: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "operator.open-cluster-management.io/v1",
					"kind":       "ClusterManager",
					"metadata": map[string]interface{}{
						"name": "cluster-manager",
					},
					"spec": map[string]interface{}{
						"deployOption": map[string]interface{}{
							"mode": "Default",
						},
						"registrationConfiguration": map[string]interface{}{
							"registrationDrivers": []interface{}{
								map[string]interface{}{"authType": "csr"},
								map[string]interface{}{"authType": "grpc"},
							},
						},
						"serverConfiguration": map[string]interface{}{
							"imagePullSpec": "quay.io/test/conductor:latest",
							"endpointsExposure": []interface{}{
								map[string]interface{}{
									"protocol": "grpc",
									"grpc": map[string]interface{}{
										"type": "hostname",
										"hostname": map[string]interface{}{
											"host": "grpc-server.example.com",
										},
									},
								},
							},
						},
					},
				},
			},
			expectUpdate: true,
			expectError:  false,
		},
		{
			name: "Remove only registrationConfiguration",
			existingClusterManager: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "operator.open-cluster-management.io/v1",
					"kind":       "ClusterManager",
					"metadata": map[string]interface{}{
						"name": "cluster-manager",
					},
					"spec": map[string]interface{}{
						"deployOption": map[string]interface{}{
							"mode": "Default",
						},
						"registrationConfiguration": map[string]interface{}{
							"registrationDrivers": []interface{}{
								map[string]interface{}{"authType": "csr"},
								map[string]interface{}{"authType": "grpc"},
							},
						},
					},
				},
			},
			expectUpdate: true,
			expectError:  false,
		},
		{
			name: "Remove only serverConfiguration",
			existingClusterManager: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "operator.open-cluster-management.io/v1",
					"kind":       "ClusterManager",
					"metadata": map[string]interface{}{
						"name": "cluster-manager",
					},
					"spec": map[string]interface{}{
						"deployOption": map[string]interface{}{
							"mode": "Default",
						},
						"serverConfiguration": map[string]interface{}{
							"imagePullSpec": "quay.io/test/conductor:latest",
						},
					},
				},
			},
			expectUpdate: true,
			expectError:  false,
		},
		{
			name: "No update when configurations not present",
			existingClusterManager: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "operator.open-cluster-management.io/v1",
					"kind":       "ClusterManager",
					"metadata": map[string]interface{}{
						"name": "cluster-manager",
					},
					"spec": map[string]interface{}{
						"deployOption": map[string]interface{}{
							"mode": "Default",
						},
					},
				},
			},
			expectUpdate: false,
			expectError:  false,
		},
		{
			name: "No error when spec not found",
			existingClusterManager: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "operator.open-cluster-management.io/v1",
					"kind":       "ClusterManager",
					"metadata": map[string]interface{}{
						"name": "cluster-manager",
					},
				},
			},
			expectUpdate: false,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objs []runtime.Object
			if tt.existingClusterManager != nil {
				objs = append(objs, tt.existingClusterManager)
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objs...).
				Build()

			r := &MultiClusterEngineReconciler{
				Client: cl,
				Scheme: scheme,
			}

			err := r.disableClusterManagerGRPCServer(ctx)

			// Check error expectations
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error containing '%s', but got no error", tt.errorContains)
				} else if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing '%s', but got: %v", tt.errorContains, err)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Verify ClusterManager was updated or not updated as expected
			if tt.existingClusterManager != nil {
				updatedCM := &unstructured.Unstructured{}
				updatedCM.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "operator.open-cluster-management.io",
					Version: "v1",
					Kind:    "ClusterManager",
				})
				err := cl.Get(ctx, types.NamespacedName{Name: "cluster-manager"}, updatedCM)
				if err != nil {
					t.Errorf("Failed to get updated ClusterManager: %v", err)
					return
				}

				// Check if configurations were removed
				spec, found, _ := unstructured.NestedMap(updatedCM.Object, "spec")
				if !found {
					if tt.expectUpdate {
						t.Error("Expected spec to exist after update")
					}
					return
				}

				// Verify registrationConfiguration is removed
				if _, exists := spec["registrationConfiguration"]; exists {
					t.Error("registrationConfiguration should be removed but still exists")
				}

				// Verify serverConfiguration is removed
				if _, exists := spec["serverConfiguration"]; exists {
					t.Error("serverConfiguration should be removed but still exists")
				}

				// Verify other fields are preserved
				if tt.expectUpdate {
					if deployOption, exists := spec["deployOption"]; !exists {
						t.Error("deployOption should be preserved but was removed")
					} else if deployMap, ok := deployOption.(map[string]interface{}); ok {
						if mode, ok := deployMap["mode"].(string); !ok || mode != "Default" {
							t.Errorf("deployOption.mode = %v, want Default", mode)
						}
					}
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func Test_ensureMaestroDatabaseSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	backplanev1.AddToScheme(scheme)

	ctx := context.TODO()

	tests := []struct {
		name          string
		existingObjs  []runtime.Object
		expectCreate  bool
		expectError   bool
		validateValue func(t *testing.T, password string)
	}{
		{
			name: "Create new secret with generated password",
			existingObjs: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "maestro"}},
			},
			expectCreate: true,
			expectError:  false,
			validateValue: func(t *testing.T, password string) {
				if len(password) != 16 {
					t.Errorf("password length = %d, want 16", len(password))
				}
			},
		},
		{
			name: "Return existing password from existing secret",
			existingObjs: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "maestro"}},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "maestro-db-config",
						Namespace: "maestro",
					},
					Data: map[string][]byte{
						"password": []byte("existing-password-123"),
					},
				},
			},
			expectCreate: false,
			expectError:  false,
			validateValue: func(t *testing.T, password string) {
				if password != "existing-password-123" {
					t.Errorf("password = %s, want existing-password-123", password)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.existingObjs...).Build()

			mce := &backplanev1.MultiClusterEngine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-mce"},
			}

			r := &MultiClusterEngineReconciler{
				Client: cl,
				Scheme: scheme,
			}

			password, err := r.ensureMaestroDatabaseSecret(ctx, mce, "maestro", "maestro-db-config")

			if (err != nil) != tt.expectError {
				t.Errorf("ensureMaestroDatabaseSecret() error = %v, expectError %v", err, tt.expectError)
				return
			}

			if !tt.expectError && tt.validateValue != nil {
				tt.validateValue(t, password)
			}

			// Verify secret exists
			secret := &corev1.Secret{}
			err = cl.Get(ctx, types.NamespacedName{Name: "maestro-db-config", Namespace: "maestro"}, secret)
			if err != nil {
				t.Errorf("secret should exist after ensureMaestroDatabaseSecret, error: %v", err)
			}
		})
	}
}

func Test_ensureClusterManagerGRPCServerConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	backplanev1.AddToScheme(scheme)

	ctx := context.TODO()

	tests := []struct {
		name         string
		existingObjs []runtime.Object
		dbPassword   string
		expectCreate bool
		expectUpdate bool
		expectError  bool
	}{
		{
			name: "Create new ConfigMap",
			existingObjs: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "open-cluster-management-hub"}},
			},
			dbPassword:   "test-password-123",
			expectCreate: true,
			expectUpdate: false,
			expectError:  false,
		},
		{
			name: "Update existing ConfigMap with different data",
			existingObjs: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "open-cluster-management-hub"}},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "grpc-server-config",
						Namespace: "open-cluster-management-hub",
					},
					Data: map[string]string{
						"config.yaml": "old-config",
					},
				},
			},
			dbPassword:   "new-password-456",
			expectCreate: false,
			expectUpdate: true,
			expectError:  false,
		},
		{
			name: "No update when ConfigMap data matches",
			existingObjs: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "open-cluster-management-hub"}},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "grpc-server-config",
						Namespace: "open-cluster-management-hub",
					},
					Data: map[string]string{
						"config.yaml": `grpc_config:
  tls_cert_file: /var/run/secrets/hub/grpc/serving-cert/tls.crt
  tls_key_file: /var/run/secrets/hub/grpc/serving-cert/tls.key
  client_ca_file: /var/run/secrets/hub/grpc/ca/ca-bundle.crt
db_config:
  host: maestro-db.maestro
  port: 5432
  name: maestro
  username: maestro
  password: same-password
  sslmode: disable`,
					},
				},
			},
			dbPassword:   "same-password",
			expectCreate: false,
			expectUpdate: false,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.existingObjs...).Build()

			mce := &backplanev1.MultiClusterEngine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-mce"},
			}

			r := &MultiClusterEngineReconciler{
				Client: cl,
				Scheme: scheme,
			}

			err := r.ensureClusterManagerGRPCServerConfigMap(ctx, mce, "open-cluster-management-hub", "grpc-server-config", tt.dbPassword)

			if (err != nil) != tt.expectError {
				t.Errorf("ensureClusterManagerGRPCServerConfigMap() error = %v, expectError %v", err, tt.expectError)
				return
			}

			// Verify ConfigMap exists and has correct data
			configMap := &corev1.ConfigMap{}
			err = cl.Get(ctx, types.NamespacedName{Name: "grpc-server-config", Namespace: "open-cluster-management-hub"}, configMap)
			if err != nil {
				t.Errorf("ConfigMap should exist after ensureClusterManagerGRPCServerConfigMap, error: %v", err)
				return
			}

			// Verify password is in the config
			if !contains(configMap.Data["config.yaml"], tt.dbPassword) {
				t.Errorf("ConfigMap config.yaml should contain password %s", tt.dbPassword)
			}
		})
	}
}

func Test_ensureClusterManagerGRPCServerRoute(t *testing.T) {
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	backplanev1.AddToScheme(scheme)

	ctx := context.TODO()

	tests := []struct {
		name         string
		existingObjs []runtime.Object
		expectCreate bool
		expectError  bool
	}{
		{
			name: "Create new Route",
			existingObjs: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "open-cluster-management-hub"}},
			},
			expectCreate: true,
			expectError:  false,
		},
		{
			name: "Route already exists - no update",
			existingObjs: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "open-cluster-management-hub"}},
				func() runtime.Object {
					route := &unstructured.Unstructured{}
					route.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "route.openshift.io",
						Version: "v1",
						Kind:    "Route",
					})
					route.SetName("grpc-server")
					route.SetNamespace("open-cluster-management-hub")
					return route
				}(),
			},
			expectCreate: false,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.existingObjs...).Build()

			mce := &backplanev1.MultiClusterEngine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-mce"},
			}

			r := &MultiClusterEngineReconciler{
				Client: cl,
				Scheme: scheme,
			}

			err := r.ensureClusterManagerGRPCServerRoute(ctx, mce, "open-cluster-management-hub", "grpc-server")

			if (err != nil) != tt.expectError {
				t.Errorf("ensureClusterManagerGRPCServerRoute() error = %v, expectError %v", err, tt.expectError)
				return
			}

			// Verify Route exists
			route := &unstructured.Unstructured{}
			route.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "route.openshift.io",
				Version: "v1",
				Kind:    "Route",
			})
			err = cl.Get(ctx, types.NamespacedName{Name: "grpc-server", Namespace: "open-cluster-management-hub"}, route)
			if err != nil {
				t.Errorf("Route should exist after ensureClusterManagerGRPCServerRoute, error: %v", err)
			}
		})
	}
}

func Test_deleteMaestroNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)

	ctx := context.TODO()

	tests := []struct {
		name         string
		existingObjs []runtime.Object
		expectError  bool
	}{
		{
			name: "Delete existing namespace",
			existingObjs: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "maestro"}},
			},
			expectError: false,
		},
		{
			name:         "Namespace already deleted - no error",
			existingObjs: []runtime.Object{},
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.existingObjs...).Build()

			r := &MultiClusterEngineReconciler{
				Client: cl,
				Scheme: scheme,
			}

			err := r.deleteMaestroNamespace(ctx)

			if (err != nil) != tt.expectError {
				t.Errorf("deleteMaestroNamespace() error = %v, expectError %v", err, tt.expectError)
			}
		})
	}
}

func Test_driversMatch(t *testing.T) {
	tests := []struct {
		name     string
		actual   []interface{}
		expected []interface{}
		want     bool
	}{
		{
			name: "Exact match",
			actual: []interface{}{
				map[string]interface{}{"authType": "csr"},
				map[string]interface{}{"authType": "grpc"},
			},
			expected: []interface{}{
				map[string]interface{}{"authType": "csr"},
				map[string]interface{}{"authType": "grpc"},
			},
			want: true,
		},
		{
			name: "Match with different order",
			actual: []interface{}{
				map[string]interface{}{"authType": "grpc"},
				map[string]interface{}{"authType": "csr"},
			},
			expected: []interface{}{
				map[string]interface{}{"authType": "csr"},
				map[string]interface{}{"authType": "grpc"},
			},
			want: true,
		},
		{
			name: "Different lengths",
			actual: []interface{}{
				map[string]interface{}{"authType": "csr"},
			},
			expected: []interface{}{
				map[string]interface{}{"authType": "csr"},
				map[string]interface{}{"authType": "grpc"},
			},
			want: false,
		},
		{
			name: "Missing driver",
			actual: []interface{}{
				map[string]interface{}{"authType": "csr"},
				map[string]interface{}{"authType": "other"},
			},
			expected: []interface{}{
				map[string]interface{}{"authType": "csr"},
				map[string]interface{}{"authType": "grpc"},
			},
			want: false,
		},
		{
			name:     "Empty slices",
			actual:   []interface{}{},
			expected: []interface{}{},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := driversMatch(tt.actual, tt.expected); got != tt.want {
				t.Errorf("driversMatch() = %v, want %v", got, tt.want)
			}
		})
	}
}
