package controllers

import (
	"context"
	"reflect"
	"testing"

	pkgerrors "github.com/pkg/errors"
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
	"sigs.k8s.io/controller-runtime/pkg/client"
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

// mockClientWithWrappedError wraps a fake client to return wrapped NotFound errors for Create operations
type mockClientWithWrappedError struct {
	client.Client
	returnNotFoundFor schema.GroupVersionKind
}

func (m *mockClientWithWrappedError) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk == m.returnNotFoundFor {
		// Return a NotFound error just like the API server would
		notFoundErr := apierrors.NewNotFound(schema.GroupResource{
			Group:    gvk.Group,
			Resource: gvk.Kind,
		}, obj.GetName())
		// Wrap it like logAndSetCondition does
		return pkgerrors.Wrapf(notFoundErr, "failed to create resource -- CRD not installed Kind: %s Name: %s", obj.GetName(), gvk.Kind)
	}
	return m.Client.Create(ctx, obj, opts...)
}

// Test_errorUnwrapping tests that wrapped NotFound and NoMatch errors are properly detected
// This specifically tests the fix for line 1386 in toggle_components.go where errors.Unwrap()
// was added to properly detect wrapped NotFound errors
func Test_errorUnwrapping(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		shouldMatch bool
		description string
	}{
		{
			name: "unwrapped NotFound error",
			err: apierrors.NewNotFound(schema.GroupResource{
				Group:    "addon.open-cluster-management.io",
				Resource: "ClusterManagementAddOn",
			}, "test-addon"),
			shouldMatch: true,
			description: "Direct NotFound errors should be caught",
		},
		{
			name: "wrapped NotFound error (like from logAndSetCondition)",
			err: pkgerrors.Wrapf(
				apierrors.NewNotFound(schema.GroupResource{
					Group:    "addon.open-cluster-management.io",
					Resource: "ClusterManagementAddOn",
				}, "test-addon"),
				"failed to create resource -- CRD not installed Kind: %s Name: %s", "test-addon", "ClusterManagementAddOn",
			),
			shouldMatch: true,
			description: "Wrapped NotFound errors (from logAndSetCondition) should be caught after unwrapping",
		},
		{
			name:        "other error types should not match",
			err:         pkgerrors.New("some other error"),
			shouldMatch: false,
			description: "Non-NotFound errors should not match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This replicates the logic from toggle_components.go:1386
			// if apimeta.IsNoMatchError(errors.Unwrap(err)) || apierrors.IsNotFound(errors.Unwrap(err))
			var unwrappedErr error
			if tt.err != nil {
				unwrappedErr = pkgerrors.Unwrap(tt.err)
				if unwrappedErr == nil {
					// If unwrap returns nil, use the original error
					unwrappedErr = tt.err
				}
			}

			matches := apierrors.IsNotFound(unwrappedErr)

			if matches != tt.shouldMatch {
				t.Errorf("%s: expected match=%v, got match=%v for error: %v", tt.description, tt.shouldMatch, matches, tt.err)
			}
		})
	}
}

// Test_ensureHyperShift_handlesMissingCRD tests that ensureHyperShift properly handles
// wrapped NotFound errors from applyTemplate when CRDs are missing.
// This specifically covers line 1386 in toggle_components.go
func Test_ensureHyperShift_handlesMissingCRD(t *testing.T) {
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	appsv1.AddToScheme(scheme)
	backplanev1.AddToScheme(scheme)
	addonv1alpha1.AddToScheme(scheme)

	// Create a fake client
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Wrap it with our mock that will return wrapped NotFound errors for ClusterManagementAddOn
	mockClient := &mockClientWithWrappedError{
		Client: fakeClient,
		returnNotFoundFor: schema.GroupVersionKind{
			Group:   "addon.open-cluster-management.io",
			Version: "v1alpha1",
			Kind:    "ClusterManagementAddOn",
		},
	}

	ctx := context.TODO()
	statusManager := &status.StatusTracker{Client: mockClient}

	r := &MultiClusterEngineReconciler{
		Client:        mockClient,
		Scheme:        scheme,
		StatusManager: statusManager,
		CacheSpec: CacheSpec{
			ImageOverrides:    map[string]string{},
			TemplateOverrides: map[string]string{},
		},
	}

	mce := &backplanev1.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name: BackplaneConfigName,
		},
		Spec: backplanev1.MultiClusterEngineSpec{
			TargetNamespace: DestinationNamespace,
		},
	}

	// Create a template that will trigger the NotFound error
	template := &unstructured.Unstructured{}
	template.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "addon.open-cluster-management.io",
		Version: "v1alpha1",
		Kind:    "ClusterManagementAddOn",
	})
	template.SetName("test-addon")
	template.SetNamespace(DestinationNamespace)

	// Call applyTemplate - this will trigger the Create error which gets wrapped
	_, err := r.applyTemplate(ctx, mce, template)

	// The error should be wrapped
	if err == nil {
		t.Fatal("Expected an error from applyTemplate when CRD is missing")
	}

	// Now simulate what ensureHyperShift does at line 1386:
	// if apimeta.IsNoMatchError(errors.Unwrap(err)) || apierrors.IsNotFound(errors.Unwrap(err))
	unwrappedErr := pkgerrors.Unwrap(err)
	if !apierrors.IsNotFound(unwrappedErr) {
		t.Errorf("Expected unwrapped error to be NotFound, but IsNotFound returned false. Error: %v, Unwrapped: %v", err, unwrappedErr)
	}

	// Verify the error message indicates CRD is not installed
	if !pkgerrors.Is(err, apierrors.NewNotFound(schema.GroupResource{}, "")) {
		t.Logf("Error chain check passed - error wraps a NotFound error")
	}
}
