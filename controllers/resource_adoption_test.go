// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"testing"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/status"
	"github.com/stolostron/backplane-operator/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	clog "sigs.k8s.io/controller-runtime/pkg/log"
)

func TestGetAdoptionPolicy(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        string
	}{
		{
			name:        "No annotations returns Strict",
			annotations: nil,
			want:        "Strict",
		},
		{
			name:        "Empty annotations returns Strict",
			annotations: map[string]string{},
			want:        "Strict",
		},
		{
			name: "Annotation not set returns Strict",
			annotations: map[string]string{
				"some-other-annotation": "value",
			},
			want: "Strict",
		},
		{
			name: "Annotation set to empty string returns Strict",
			annotations: map[string]string{
				utils.AnnotationResourceAdoptionPolicy: "",
			},
			want: "Strict",
		},
		{
			name: "Valid Strict value",
			annotations: map[string]string{
				utils.AnnotationResourceAdoptionPolicy: "Strict",
			},
			want: "Strict",
		},
		{
			name: "Valid Adopt value",
			annotations: map[string]string{
				utils.AnnotationResourceAdoptionPolicy: "Adopt",
			},
			want: "Adopt",
		},
		{
			name: "Invalid value defaults to Strict",
			annotations: map[string]string{
				utils.AnnotationResourceAdoptionPolicy: "InvalidValue",
			},
			want: "Strict",
		},
		{
			name: "Case-sensitive - lowercase adopt is invalid",
			annotations: map[string]string{
				utils.AnnotationResourceAdoptionPolicy: "adopt",
			},
			want: "Strict",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mce := &backplanev1.MultiClusterEngine{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-mce",
					Namespace:   "test-namespace",
					Annotations: tt.annotations,
				},
			}

			r := &MultiClusterEngineReconciler{
				Log: clog.Log.WithName("test"),
			}

			got := r.getAdoptionPolicy(mce)
			if got != tt.want {
				t.Errorf("getAdoptionPolicy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnsureResourceOwnership(t *testing.T) {
	tests := []struct {
		name               string
		existingLabels     map[string]string
		templateLabels     map[string]string
		adoptionPolicy     string
		wantManage         bool
		wantTemplateLabels map[string]string
	}{
		{
			name: "Resource with matching backplaneconfig label - should manage",
			existingLabels: map[string]string{
				"backplaneconfig.name": "test-mce",
			},
			templateLabels:     map[string]string{},
			adoptionPolicy:     "Strict",
			wantManage:         true,
			wantTemplateLabels: map[string]string{}, // Template unchanged
		},
		{
			name: "Resource owned by different MCE - should not manage",
			existingLabels: map[string]string{
				"backplaneconfig.name": "other-mce",
			},
			templateLabels:     map[string]string{},
			adoptionPolicy:     "Strict",
			wantManage:         false,
			wantTemplateLabels: map[string]string{}, // Template unchanged
		},
		{
			name: "Resource owned by different MCE, Adopt policy - should not manage",
			existingLabels: map[string]string{
				"backplaneconfig.name": "other-mce",
			},
			templateLabels:     map[string]string{},
			adoptionPolicy:     "Adopt",
			wantManage:         false,
			wantTemplateLabels: map[string]string{}, // Template unchanged - don't adopt from other MCE
		},
		{
			name:               "Resource without labels, Strict policy - should not manage",
			existingLabels:     map[string]string{},
			templateLabels:     map[string]string{},
			adoptionPolicy:     "Strict",
			wantManage:         false,
			wantTemplateLabels: map[string]string{}, // Template unchanged
		},
		{
			name:           "Resource without labels, Adopt policy - should manage and add labels",
			existingLabels: map[string]string{},
			templateLabels: map[string]string{},
			adoptionPolicy: "Adopt",
			wantManage:     true,
			wantTemplateLabels: map[string]string{
				"backplaneconfig.name": "test-mce",
			},
		},
		{
			name: "Resource without backplaneconfig label but with existing app labels, Adopt policy - should adopt",
			existingLabels: map[string]string{
				"app": "myapp",
			},
			templateLabels: map[string]string{
				"app": "myapp",
			},
			adoptionPolicy: "Adopt",
			wantManage:     true,
			wantTemplateLabels: map[string]string{
				"app":                  "myapp",
				"backplaneconfig.name": "test-mce",
			},
		},
		{
			name:               "No annotation defaults to Strict - should not manage unlabeled resource",
			existingLabels:     map[string]string{},
			templateLabels:     map[string]string{},
			adoptionPolicy:     "", // Will default to Strict
			wantManage:         false,
			wantTemplateLabels: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test MCE with adoption policy
			annotations := map[string]string{}
			if tt.adoptionPolicy != "" {
				annotations[utils.AnnotationResourceAdoptionPolicy] = tt.adoptionPolicy
			}

			mce := &backplanev1.MultiClusterEngine{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-mce",
					Namespace:   "test-namespace",
					Annotations: annotations,
				},
			}

			// Create existing resource
			existing := &unstructured.Unstructured{}
			existing.SetKind("Service")
			existing.SetName("test-service")
			existing.SetNamespace("test-namespace")
			existing.SetLabels(tt.existingLabels)

			// Create template resource
			template := &unstructured.Unstructured{}
			template.SetKind("Service")
			template.SetName("test-service")
			template.SetNamespace("test-namespace")
			template.SetLabels(tt.templateLabels)

			r := &MultiClusterEngineReconciler{
				Log: clog.Log.WithName("test"),
			}

			got := r.ensureResourceOwnership(existing, template, mce)

			// Check if should manage
			if got != tt.wantManage {
				t.Errorf("ensureResourceOwnership() = %v, want %v", got, tt.wantManage)
			}

			// Check template labels
			gotLabels := template.GetLabels()
			if len(gotLabels) != len(tt.wantTemplateLabels) {
				t.Errorf("template labels count = %v, want %v", len(gotLabels), len(tt.wantTemplateLabels))
			}

			for key, wantValue := range tt.wantTemplateLabels {
				if gotValue, exists := gotLabels[key]; !exists || gotValue != wantValue {
					t.Errorf("template label %s = %v, want %v", key, gotValue, wantValue)
				}
			}
		})
	}
}

func TestEnsureResourceOwnership_NilLabels(t *testing.T) {
	// Test that function handles nil labels gracefully
	mce := &backplanev1.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mce",
			Namespace: "test-namespace",
			Annotations: map[string]string{
				utils.AnnotationResourceAdoptionPolicy: "Adopt",
			},
		},
	}

	existing := &unstructured.Unstructured{}
	existing.SetKind("Service")
	existing.SetName("test-service")
	// Don't set labels - should be nil

	template := &unstructured.Unstructured{}
	template.SetKind("Service")
	template.SetName("test-service")
	// Don't set labels - should be nil

	r := &MultiClusterEngineReconciler{
		Log: clog.Log.WithName("test"),
	}

	got := r.ensureResourceOwnership(existing, template, mce)

	if !got {
		t.Errorf("ensureResourceOwnership() with nil labels should manage and adopt, got false")
	}

	// Should have added labels to template
	labels := template.GetLabels()
	if labels == nil {
		t.Errorf("template labels should not be nil after adoption")
	}

	if labels["backplaneconfig.name"] != "test-mce" {
		t.Errorf("backplaneconfig.name = %v, want %v", labels["backplaneconfig.name"], "test-mce")
	}
}

func TestEnsureResourceOwnership_BothParameters(t *testing.T) {
	// Test that existing is checked but template is modified
	mce := &backplanev1.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mce",
			Namespace: "test-namespace",
			Annotations: map[string]string{
				utils.AnnotationResourceAdoptionPolicy: "Adopt",
			},
		},
	}

	// Existing has no labels
	existing := &unstructured.Unstructured{}
	existing.SetKind("ConfigMap")
	existing.SetName("test-config")
	existing.SetLabels(map[string]string{"app": "existing"})

	// Template has different labels
	template := &unstructured.Unstructured{}
	template.SetKind("ConfigMap")
	template.SetName("test-config")
	template.SetLabels(map[string]string{"component": "template"})

	r := &MultiClusterEngineReconciler{
		Log:           clog.Log.WithName("test"),
		Client:        fake.NewClientBuilder().Build(),
		StatusManager: &status.StatusTracker{Client: fake.NewClientBuilder().Build()},
	}

	got := r.ensureResourceOwnership(existing, template, mce)

	if !got {
		t.Errorf("ensureResourceOwnership() should adopt resource")
	}

	// Check that template was modified, not existing
	existingLabels := existing.GetLabels()
	if _, exists := existingLabels["backplaneconfig.name"]; exists {
		t.Errorf("existing resource should not be modified")
	}

	templateLabels := template.GetLabels()
	if templateLabels["backplaneconfig.name"] != "test-mce" {
		t.Errorf("template should have backplaneconfig.name label")
	}
	// Original template label should still exist
	if templateLabels["component"] != "template" {
		t.Errorf("template original labels should be preserved")
	}
}
