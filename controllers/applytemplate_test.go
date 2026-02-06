// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
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
		expectSkip  bool
		expectError bool
	}{
		{
			name: "Should skip when CRD is missing",
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
			expectSkip:  true,
			expectError: false,
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
			expectSkip:  false,
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

			if tt.expectSkip {
				// Verify the resource was NOT created
				key := types.NamespacedName{
					Name:      tt.template.GetName(),
					Namespace: tt.template.GetNamespace(),
				}
				checkObj := tt.template.DeepCopy()
				err := fakeClient.Get(ctx, key, checkObj)
				if !apierrors.IsNotFound(err) {
					t.Errorf("Expected resource to not be created when CRD is missing, but got: %v", err)
				}
			} else if !tt.expectError {
				// Verify the resource was created
				key := types.NamespacedName{
					Name:      tt.template.GetName(),
					Namespace: tt.template.GetNamespace(),
				}
				checkObj := &corev1.ConfigMap{}
				err := fakeClient.Get(ctx, key, checkObj)
				if err != nil {
					t.Errorf("Expected resource to be created but got error: %v", err)
				}
			}

			// Verify result is not requeuing
			if result.Requeue || result.RequeueAfter > 0 {
				t.Errorf("Expected no requeue, but got result: %v", result)
			}
		})
	}
}
