// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package images

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_GetImages(t *testing.T) {
	t.Run("No env vars", func(t *testing.T) {
		if ln := len(GetImages()); ln != 0 {
			t.Fatalf("Got images without setting env vars. Expected 0; Got %d", ln)
		}
	})
	t.Run("'OPERAND_IMAGE' env vars", func(t *testing.T) {
		t.Setenv("OPERAND_IMAGE_CONSOLE", "quay.io/stolostron/console:test-image")
		if ln := len(GetImages()); ln != 1 {
			t.Fatalf("Expected single image from environment. Expected 1; got %d.", ln)
		}
	})
	t.Run("'RELATED_IMAGE' env vars", func(t *testing.T) {
		t.Setenv("RELATED_IMAGE_APPLICATION_UI", "quay.io/stolostron/app-ui:test-image")
		if ln := len(GetImages()); ln != 1 {
			t.Fatalf("Expected single image from environment. Expected 1; got %d.", ln)
		}
	})
	t.Run("Both env vars", func(t *testing.T) {
		t.Setenv("OPERAND_IMAGE_CONSOLE", "quay.io/stolostron/console:test-image")
		t.Setenv("RELATED_IMAGE_APPLICATION_UI", "quay.io/stolostron/app-ui:test-image")
		if ln := len(GetImages()); ln != 1 {
			t.Fatalf("Expected single image from environment If 'OPERAND_IMAGE' present then 'RELATED_IMAGE' images should not be read. Expected 1; got %d.", ln)
		}
	})
}

func TestOverrideImageRepository(t *testing.T) {
	tests := []struct {
		name      string
		images    map[string]string
		imageRepo string
		want      map[string]string
	}{
		{
			name: "Replace stolostron repo",
			images: map[string]string{
				"discovery_operator": "quay.io/stolostron/discovery-operator:latest",
			},
			imageRepo: "quay.io/acm-d",
			want: map[string]string{
				"discovery_operator": "quay.io/acm-d/discovery-operator:latest",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OverrideImageRepository(tt.images, tt.imageRepo); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("OverrideImageRepository() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOverrideImagesWithConfigmap(t *testing.T) {
	testCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Data: map[string]string{
			"overrides.json": `[
			{
				"image-name": "discovery-operator",
				"image-version": "0.2",
				"image-tag": "0.2-5bf12929112cdb5d94856a847583f84718c2033e",
				"git-sha256": "5bf12929112cdb5d94856a847583f84718c2033e",
				"git-repository": "stolostron/discovery",
				"image-remote": "quay.io/stolostron",
				"image-remote-src": "registry.ci.openshift.org/stolostron",
				"image-digest": "sha256:9dc4d072dcd06eda3fda19a15f4b84677fbbbde2a476b4817272cde4724f02cc",
				"image-key": "discovery_operator"
				}
		]`,
		},
	}

	tests := []struct {
		name      string
		images    map[string]string
		configmap *corev1.ConfigMap
		want      map[string]string
		wantErr   bool
	}{
		{
			name: "Replace image",
			images: map[string]string{
				"discovery_operator": "quay.io/stolostron/discovery-operator:latest",
			},
			configmap: testCM,
			want: map[string]string{
				"discovery_operator": "quay.io/stolostron/discovery-operator@sha256:9dc4d072dcd06eda3fda19a15f4b84677fbbbde2a476b4817272cde4724f02cc",
			},
		},
		{
			name: "Add image",
			images: map[string]string{
				"cluster_api": "quay.io/stolostron/cluster-api:latest",
			},
			configmap: testCM,
			want: map[string]string{
				"cluster_api":        "quay.io/stolostron/cluster-api:latest",
				"discovery_operator": "quay.io/stolostron/discovery-operator@sha256:9dc4d072dcd06eda3fda19a15f4b84677fbbbde2a476b4817272cde4724f02cc",
			},
		},
		{
			name: "Invalid configmap",
			images: map[string]string{
				"cluster_api": "quay.io/stolostron/cluster-api:latest",
			},
			configmap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
				Data: map[string]string{
					"discovery_operator": "quay.io/stolostron/discovery-operator:latest",
					"cluster_api":        "quay.io/stolostron/cluster-api:latest",
				},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := OverrideImagesWithConfigmap(tt.images, tt.configmap)
			if (err != nil) != tt.wantErr {
				t.Errorf("OverrideImagesWithConfigmap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("OverrideImagesWithConfigmap() = %v, want %v", got, tt.want)
			}
		})
	}
}
