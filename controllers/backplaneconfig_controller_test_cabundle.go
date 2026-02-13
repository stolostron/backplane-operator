// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnsureCRD_PreservesCABundle(t *testing.T) {
	testCABundle := "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURQakNDQWlhZ0F3SUJBZ0lVRXhHY2FBPT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="

	tests := []struct {
		name             string
		existingCRD      *unstructured.Unstructured
		newCRD           *unstructured.Unstructured
		expectCABundle   bool
		expectedCABundle string
		expectError      bool
		description      string
	}{
		{
			name: "Preserves caBundle when updating CRD with conversion webhook",
			existingCRD: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apiextensions.k8s.io/v1",
					"kind":       "CustomResourceDefinition",
					"metadata": map[string]interface{}{
						"name":            "test.example.com",
						"resourceVersion": "1",
					},
					"spec": map[string]interface{}{
						"group": "example.com",
						"names": map[string]interface{}{
							"kind":   "Test",
							"plural": "tests",
						},
						"conversion": map[string]interface{}{
							"strategy": "Webhook",
							"webhook": map[string]interface{}{
								"clientConfig": map[string]interface{}{
									"service": map[string]interface{}{
										"name":      "webhook-service",
										"namespace": "default",
										"path":      "/convert",
									},
									"caBundle": testCABundle,
								},
								"conversionReviewVersions": []interface{}{"v1"},
							},
						},
					},
				},
			},
			newCRD: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apiextensions.k8s.io/v1",
					"kind":       "CustomResourceDefinition",
					"metadata": map[string]interface{}{
						"name": "test.example.com",
					},
					"spec": map[string]interface{}{
						"group": "example.com",
						"names": map[string]interface{}{
							"kind":   "Test",
							"plural": "tests",
						},
						"conversion": map[string]interface{}{
							"strategy": "Webhook",
							"webhook": map[string]interface{}{
								"clientConfig": map[string]interface{}{
									"service": map[string]interface{}{
										"name":      "webhook-service",
										"namespace": "default",
										"path":      "/convert",
									},
								},
								"conversionReviewVersions": []interface{}{"v1"},
							},
						},
					},
				},
			},
			expectCABundle:   true,
			expectedCABundle: testCABundle,
			expectError:      false,
			description:      "caBundle should be preserved from existing CRD",
		},
		{
			name: "Does not add caBundle if it doesn't exist in existing CRD",
			existingCRD: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apiextensions.k8s.io/v1",
					"kind":       "CustomResourceDefinition",
					"metadata": map[string]interface{}{
						"name":            "test2.example.com",
						"resourceVersion": "1",
					},
					"spec": map[string]interface{}{
						"group": "example.com",
						"names": map[string]interface{}{
							"kind":   "Test2",
							"plural": "test2s",
						},
						"conversion": map[string]interface{}{
							"strategy": "Webhook",
							"webhook": map[string]interface{}{
								"clientConfig": map[string]interface{}{
									"service": map[string]interface{}{
										"name":      "webhook-service",
										"namespace": "default",
										"path":      "/convert",
									},
								},
								"conversionReviewVersions": []interface{}{"v1"},
							},
						},
					},
				},
			},
			newCRD: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apiextensions.k8s.io/v1",
					"kind":       "CustomResourceDefinition",
					"metadata": map[string]interface{}{
						"name": "test2.example.com",
					},
					"spec": map[string]interface{}{
						"group": "example.com",
						"names": map[string]interface{}{
							"kind":   "Test2",
							"plural": "test2s",
						},
						"conversion": map[string]interface{}{
							"strategy": "Webhook",
							"webhook": map[string]interface{}{
								"clientConfig": map[string]interface{}{
									"service": map[string]interface{}{
										"name":      "webhook-service",
										"namespace": "default",
										"path":      "/convert",
									},
								},
								"conversionReviewVersions": []interface{}{"v1"},
							},
						},
					},
				},
			},
			expectCABundle: false,
			expectError:    false,
			description:    "caBundle should not be added if it doesn't exist",
		},
		{
			name: "Handles CRD without conversion webhook",
			existingCRD: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apiextensions.k8s.io/v1",
					"kind":       "CustomResourceDefinition",
					"metadata": map[string]interface{}{
						"name":            "test3.example.com",
						"resourceVersion": "1",
					},
					"spec": map[string]interface{}{
						"group": "example.com",
						"names": map[string]interface{}{
							"kind":   "Test3",
							"plural": "test3s",
						},
					},
				},
			},
			newCRD: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apiextensions.k8s.io/v1",
					"kind":       "CustomResourceDefinition",
					"metadata": map[string]interface{}{
						"name": "test3.example.com",
					},
					"spec": map[string]interface{}{
						"group": "example.com",
						"names": map[string]interface{}{
							"kind":   "Test3",
							"plural": "test3s",
						},
					},
				},
			},
			expectCABundle: false,
			expectError:    false,
			description:    "Should handle CRDs without conversion webhooks",
		},
		{
			name: "Does not copy caBundle when new CRD has no conversion section",
			existingCRD: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apiextensions.k8s.io/v1",
					"kind":       "CustomResourceDefinition",
					"metadata": map[string]interface{}{
						"name":            "ipaddresses.ipam.metal3.io",
						"resourceVersion": "1",
					},
					"spec": map[string]interface{}{
						"group": "ipam.metal3.io",
						"names": map[string]interface{}{
							"kind":   "IPAddress",
							"plural": "ipaddresses",
						},
						"conversion": map[string]interface{}{
							"strategy": "Webhook",
							"webhook": map[string]interface{}{
								"clientConfig": map[string]interface{}{
									"service": map[string]interface{}{
										"name":      "webhook-service",
										"namespace": "default",
										"path":      "/convert",
									},
									"caBundle": testCABundle,
								},
								"conversionReviewVersions": []interface{}{"v1"},
							},
						},
					},
				},
			},
			newCRD: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apiextensions.k8s.io/v1",
					"kind":       "CustomResourceDefinition",
					"metadata": map[string]interface{}{
						"name": "ipaddresses.ipam.metal3.io",
					},
					"spec": map[string]interface{}{
						"group": "ipam.metal3.io",
						"names": map[string]interface{}{
							"kind":   "IPAddress",
							"plural": "ipaddresses",
						},
						// No conversion section at all
					},
				},
			},
			expectCABundle: false,
			expectError:    false,
			description:    "Should not copy caBundle when new CRD template has no conversion section (avoids invalid CRD)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a scheme
			s := runtime.NewScheme()

			// Create fake client with existing CRD
			objs := []client.Object{}
			if tt.existingCRD != nil {
				objs = append(objs, tt.existingCRD)
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(s).
				WithObjects(objs...).
				Build()

			// Call EnsureCRD
			err := EnsureCRD(context.TODO(), fakeClient, tt.newCRD)

			// Check error expectation
			if (err != nil) != tt.expectError {
				t.Errorf("%s: EnsureCRD() error = %v, expectError %v", tt.description, err, tt.expectError)
				return
			}

			if err != nil {
				return // If error was expected and received, test is done
			}

			// Get the updated CRD
			updatedCRD := &unstructured.Unstructured{}
			updatedCRD.SetGroupVersionKind(tt.newCRD.GroupVersionKind())
			err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: tt.newCRD.GetName()}, updatedCRD)
			if err != nil {
				t.Fatalf("%s: Failed to get updated CRD: %v", tt.description, err)
			}

			// Check caBundle
			actualCABundle, found, err := unstructured.NestedString(updatedCRD.Object, "spec", "conversion", "webhook", "clientConfig", "caBundle")
			if err != nil {
				t.Fatalf("%s: Error getting caBundle from updated CRD: %v", tt.description, err)
			}

			if tt.expectCABundle {
				if !found {
					t.Errorf("%s: Expected caBundle to be present but it was not found", tt.description)
				} else if actualCABundle != tt.expectedCABundle {
					t.Errorf("%s: caBundle mismatch. Expected %q, got %q", tt.description, tt.expectedCABundle, actualCABundle)
				}
			} else {
				if found && actualCABundle != "" {
					t.Errorf("%s: Expected no caBundle but found %q", tt.description, actualCABundle)
				}
			}
		})
	}
}

func TestEnsureCRD_CreateNewCRD(t *testing.T) {
	// Test creating a new CRD (should not have caBundle issues)
	newCRD := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"name": "newtest.example.com",
			},
			"spec": map[string]interface{}{
				"group": "example.com",
				"names": map[string]interface{}{
					"kind":   "NewTest",
					"plural": "newtests",
				},
			},
		},
	}

	s := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		Build()

	err := EnsureCRD(context.TODO(), fakeClient, newCRD)
	if err != nil {
		t.Errorf("EnsureCRD() should create new CRD without error, got: %v", err)
	}

	// Verify CRD was created
	createdCRD := &unstructured.Unstructured{}
	createdCRD.SetGroupVersionKind(newCRD.GroupVersionKind())
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: newCRD.GetName()}, createdCRD)
	if err != nil {
		t.Errorf("Failed to get created CRD: %v", err)
	}
}
