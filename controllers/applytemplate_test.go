// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"fmt"
	"testing"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/status"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// mockClientWithCreateInterceptor wraps a client to intercept Create calls
type mockClientWithCreateInterceptor struct {
	client.Client
	missingGVKs map[schema.GroupVersionKind]bool
}

func (m *mockClientWithCreateInterceptor) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	gvk := obj.GetObjectKind().GroupVersionKind()
	if m.missingGVKs[gvk] {
		// Return NotFound error to simulate CRD not existing
		return apierrors.NewNotFound(schema.GroupResource{
			Group:    gvk.Group,
			Resource: gvk.Kind,
		}, obj.GetName())
	}
	return m.Client.Create(ctx, obj, opts...)
}

func TestApplyTemplate_MissingCRD(t *testing.T) {
	// Register scheme
	s := scheme.Scheme
	backplanev1.AddToScheme(s)

	// Create MCE instance
	mce := &backplanev1.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-mce",
		},
		Spec: backplanev1.MultiClusterEngineSpec{
			TargetNamespace: "test-namespace",
		},
	}

	tests := []struct {
		name        string
		template    *unstructured.Unstructured
		missingCRD  bool
		expectError bool
	}{
		{
			name: "Should return error when CRD is missing",
			template: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "addon.open-cluster-management.io/v1alpha1",
					"kind":       "AddOnDeploymentConfig",
					"metadata": map[string]interface{}{
						"name":      "test-addon-config",
						"namespace": "test-namespace",
					},
					"spec": map[string]interface{}{},
				},
			},
			missingCRD:  true,
			expectError: true,
		},
		{
			name: "Should create when CRD exists and resource doesn't",
			template: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test-configmap",
						"namespace": "test-namespace",
					},
					"data": map[string]interface{}{
						"key": "value",
					},
				},
			},
			missingCRD:  false,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with the MCE instance
			fakeClient := fake.NewClientBuilder().
				WithScheme(s).
				WithObjects(mce).
				Build()

			// Setup missing GVKs
			missingGVKs := make(map[schema.GroupVersionKind]bool)
			if tt.missingCRD {
				gvk := tt.template.GroupVersionKind()
				missingGVKs[gvk] = true
			}

			// Wrap client with create interceptor
			wrappedClient := &mockClientWithCreateInterceptor{
				Client:      fakeClient,
				missingGVKs: missingGVKs,
			}

			// Create reconciler
			reconciler := &MultiClusterEngineReconciler{
				Client:        wrappedClient,
				Scheme:        s,
				StatusManager: &status.StatusTracker{Client: wrappedClient},
			}

			// Call applyTemplate
			ctx := context.Background()
			result, err := reconciler.applyTemplate(ctx, mce, tt.template)

			// Verify results
			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			// Verify resource creation based on expectError
			key := types.NamespacedName{
				Name:      tt.template.GetName(),
				Namespace: tt.template.GetNamespace(),
			}

			if tt.expectError {
				// When error is expected, verify the resource was NOT created
				checkObj := tt.template.DeepCopy()
				getErr := fakeClient.Get(ctx, key, checkObj)
				if !apierrors.IsNotFound(getErr) {
					t.Errorf("Expected resource to not be created when CRD is missing, but got: %v", getErr)
				}
			} else {
				// When no error is expected, verify the resource WAS created
				checkObj := &corev1.ConfigMap{}
				getErr := fakeClient.Get(ctx, key, checkObj)
				if getErr != nil {
					t.Errorf("Expected resource to be created but got error: %v", getErr)
				}
			}

			// Verify result is not requeuing
			if result.Requeue || result.RequeueAfter > 0 {
				t.Errorf("Expected no requeue, but got result: %v", result)
			}
		})
	}
}

// TestApplyTemplate_AlreadyExists tests the race condition where Get returns NotFound
// but Create returns AlreadyExists (another reconcile created it). This should be handled
// gracefully without returning an error. This covers line 1823-1827 in backplaneconfig_controller.go
func TestApplyTemplate_AlreadyExists(t *testing.T) {
	s := scheme.Scheme
	backplanev1.AddToScheme(s)

	mce := &backplanev1.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-mce",
		},
		Spec: backplanev1.MultiClusterEngineSpec{
			TargetNamespace: "test-namespace",
		},
	}

	template := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "race-configmap",
				"namespace": "test-namespace",
			},
			"data": map[string]interface{}{
				"key": "value",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(mce).
		Build()

	// Create a mock client that simulates a race: Get returns NotFound, Create returns AlreadyExists
	mockClient := &mockClientAlreadyExists{
		Client: fakeClient,
	}

	reconciler := &MultiClusterEngineReconciler{
		Client:        mockClient,
		Scheme:        s,
		StatusManager: &status.StatusTracker{Client: mockClient},
	}

	ctx := context.Background()
	result, err := reconciler.applyTemplate(ctx, mce, template)

	// Should handle AlreadyExists gracefully (line 1825-1827)
	if err != nil {
		t.Errorf("Expected no error when resource already exists, got: %v", err)
	}

	// Verify result is not requeuing
	if result.Requeue || result.RequeueAfter > 0 {
		t.Errorf("Expected no requeue, but got result: %v", result)
	}
}

// mockClientAlreadyExists simulates a race condition: Get returns NotFound, Create returns AlreadyExists
type mockClientAlreadyExists struct {
	client.Client
}

func (m *mockClientAlreadyExists) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	// Simulate resource not found
	return apierrors.NewNotFound(schema.GroupResource{Resource: "ConfigMap"}, key.Name)
}

func (m *mockClientAlreadyExists) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	// Simulate resource was created between Get and Create (race condition)
	return apierrors.NewAlreadyExists(schema.GroupResource{Resource: obj.GetObjectKind().GroupVersionKind().Kind}, obj.GetName())
}

// Test applyTemplate_AlreadyExists_DifferentError tests that when Create returns an error
// that is NOT NotFound and NOT AlreadyExists, it properly returns an error.
// This helps ensure line 1824 is covered when the condition is false.
func Test_applyTemplate_CreateOtherError(t *testing.T) {
	s := scheme.Scheme
	backplanev1.AddToScheme(s)

	mce := &backplanev1.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-mce",
		},
		Spec: backplanev1.MultiClusterEngineSpec{
			TargetNamespace: "test-namespace",
		},
	}

	template := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-configmap",
				"namespace": "test-namespace",
			},
			"data": map[string]interface{}{
				"key": "value",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(mce).
		Build()

	// Create a mock client that returns a different error (not NotFound, not AlreadyExists)
	mockClient := &mockClientReturningError{
		Client: fakeClient,
	}

	reconciler := &MultiClusterEngineReconciler{
		Client:        mockClient,
		Scheme:        s,
		StatusManager: &status.StatusTracker{Client: mockClient},
	}

	ctx := context.Background()
	_, err := reconciler.applyTemplate(ctx, mce, template)

	// Should return an error for other types of errors (line 1824)
	if err == nil {
		t.Error("Expected error for other create errors, got nil")
	}
}

// mockClientReturningError returns a generic error on Create to test the else path at line 1824
type mockClientReturningError struct {
	client.Client
}

func (m *mockClientReturningError) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	// Return a generic error that is NOT NotFound and NOT AlreadyExists
	return apierrors.NewInternalError(fmt.Errorf("internal error"))
}

func (m *mockClientReturningError) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	// Return NotFound so Create gets called
	return apierrors.NewNotFound(schema.GroupResource{}, key.Name)
}

// This covers line 1824 in backplaneconfig_controller.go
