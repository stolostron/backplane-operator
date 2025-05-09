// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"reflect"
	"testing"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestIsPaused(t *testing.T) {
	t.Run("Unpaused MCE", func(t *testing.T) {
		mce := &backplanev1.MultiClusterEngine{}
		want := false
		if got := IsPaused(mce); got != want {
			t.Errorf("IsPaused() = %v, want %v", got, want)
		}
	})
	t.Run("Paused MCE", func(t *testing.T) {
		mce := &backplanev1.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{AnnotationMCEPause: "true"}},
		}
		want := true
		if got := IsPaused(mce); got != want {
			t.Errorf("IsPaused() = %v, want %v", got, want)
		}
	})
	t.Run("Pause label false MCE", func(t *testing.T) {
		mce := &backplanev1.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{AnnotationMCEPause: "false"}},
		}
		want := false
		if got := IsPaused(mce); got != want {
			t.Errorf("IsPaused() = %v, want %v", got, want)
		}
	})

}

func Test_AnnotationMatch(t *testing.T) {
	tests := []struct {
		name string
		new  map[string]string
		old  map[string]string
		want bool
	}{
		{
			name: "Annotations should match",
			new: map[string]string{
				AnnotationMCEPause:         "false",
				AnnotationImageRepo:        "sample-image-repo",
				AnnotationImageOverridesCM: "sample-image-override",
			},
			old: map[string]string{
				DeprecatedAnnotationMCEPause:         "false",
				DeprecatedAnnotationImageRepo:        "sample-image-repo",
				DeprecatedAnnotationImageOverridesCM: "sample-image-override",
			},
			want: true,
		},
		{
			name: "Annotations should not match",
			new: map[string]string{
				AnnotationMCEPause:         "false",
				AnnotationImageRepo:        "sample-image-repo",
				AnnotationImageOverridesCM: "sample-image-override",
			},
			old: map[string]string{
				DeprecatedAnnotationMCEPause:         "true",
				DeprecatedAnnotationImageRepo:        "sample-image-repo",
				DeprecatedAnnotationImageOverridesCM: "sample-image-override",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AnnotationsMatch(tt.old, tt.new); got != tt.want {
				t.Errorf("AnnotationsMatch(old, new) = got: %v, want: %v", got, tt.want)
			}
		})
	}
}

func Test_getAnnotationEdgeManagement(t *testing.T) {
	instance := &backplanev1.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{AnnotationEdgeManagerEnabled: "true"}},
	}

	result := GetEdgeManagerEnabled(instance)
	if result != "true" {
		t.Errorf("Was expecting true for the edge management annotation. Did not receive")
	}

	instance = &backplanev1.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{},
	}

	result = GetEdgeManagerEnabled(instance)
	if result != "false" {
		t.Errorf("Was expecting false for the edge management annotation. Did not receive")
	}
}

func TestIsTemplateAnnotationTrue(t *testing.T) {
	t.Run("Annotation true", func(t *testing.T) {
		tst := &unstructured.Unstructured{}
		want := false
		if got := IsTemplateAnnotationTrue(tst, AnnotationEditable); got != want {
			t.Errorf("IsTemplateAnnotationTrue() = %v, want %v", got, want)
		}
	})

	t.Run("Annotation true", func(t *testing.T) {
		tst := &unstructured.Unstructured{}
		tst.SetAnnotations(map[string]string{AnnotationEditable: "true"})
		want := true
		if got := IsTemplateAnnotationTrue(tst, AnnotationEditable); got != want {
			t.Errorf("IsTemplateAnnotationTrue() = %v, want %v", got, want)
		}
	})

}

func Test_getAnnotation(t *testing.T) {
	type args struct {
		instance *backplanev1.MultiClusterEngine
		key      string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Annotation does not exist",
			args: args{
				instance: &backplanev1.MultiClusterEngine{},
				key:      "",
			},
			want: "",
		},
		{
			name: "Annotation exists",
			args: args{
				instance: &backplanev1.MultiClusterEngine{
					ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"foo": "bar"}},
				},
				key: "foo",
			},
			want: "bar",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getAnnotation(tt.args.instance, tt.args.key); got != tt.want {
				t.Errorf("getAnnotation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_GetImageRepository(t *testing.T) {
	t.Run("Get image repository for MCE", func(t *testing.T) {
		mce := &backplanev1.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
				AnnotationImageRepo: "quay.io/foo",
			}},
		}
		want := "quay.io/foo"
		if got := GetImageRepository(mce); got != want {
			t.Errorf("GetImageRepository(mce) = %v, want %v", got, want)
		}
	})
}

func Test_GetImageOverridesConfigmapName(t *testing.T) {
	t.Run("Get image overrides configmap name for MCE", func(t *testing.T) {
		mce := &backplanev1.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
				AnnotationImageOverridesCM: "image-override-cm",
			}},
		}
		want := "image-override-cm"
		if got := GetImageOverridesConfigmapName(mce); got != want {
			t.Errorf("AnnotationImageOverridesCM(mce) = %v, want %v", got, want)
		}
	})
}

func Test_GetTemplateOverridesConfigmapName(t *testing.T) {
	t.Run("Get template overrides configmap name for MCE", func(t *testing.T) {
		mce := &backplanev1.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
				AnnotationTemplateOverridesCM: "template-override-cm",
			}},
		}
		want := "template-override-cm"
		if got := GetTemplateOverridesConfigmapName(mce); got != want {
			t.Errorf("GetTemplateOverridesConfigmapName() = %v, want %v", got, want)
		}
	})
}

func TestOverrideImageRepository(t *testing.T) {
	tests := []struct {
		ImageOverrides map[string]string
		ImageRepo      string
		Expected       map[string]string
	}{
		{
			ImageOverrides: map[string]string{
				"application_ui": "quay.io/stolostron/application-ui@sha256:c740fc7bac067f003145ab909504287360564016b7a4a51b7ad4987aca123ac1",
				"console_api":    "quay.io/stolostron/console-api@sha256:3ef1043b4e61a09b07ff37f9ad8fc6e707af9813936cf2c0d52f2fa0e489c75f",
				"rcm_controller": "quay.io/stolostron/rcm-controller@sha256:8fab4d788241bf364dbc1b8c1ea5ccf18d3145a640dbd456b0dc7ba204e36819",
			},
			ImageRepo: "quay.io:443/acm-d",
			Expected: map[string]string{
				"application_ui": "quay.io:443/acm-d/application-ui@sha256:c740fc7bac067f003145ab909504287360564016b7a4a51b7ad4987aca123ac1",
				"console_api":    "quay.io:443/acm-d/console-api@sha256:3ef1043b4e61a09b07ff37f9ad8fc6e707af9813936cf2c0d52f2fa0e489c75f",
				"rcm_controller": "quay.io:443/acm-d/rcm-controller@sha256:8fab4d788241bf364dbc1b8c1ea5ccf18d3145a640dbd456b0dc7ba204e36819",
			},
		},
		{
			ImageOverrides: map[string]string{},
			ImageRepo:      "",
			Expected:       map[string]string{},
		},
		{
			ImageOverrides: map[string]string{
				"application_ui": "registry.redhat.io/rhacm2/application-ui@sha256:d7b7b96d679dbbdace044a18cca56544faa65f66e593fc600c08c9f814e0ea6a",
				"console_api":    "registry.redhat.io/rhacm2/console-api@sha256:d7b7b96d679dbbdace044a18cca56544faa65f66e593fc600c08c9f814e0ea6a",
				"rcm_controller": "registry.redhat.io/rhacm2/rcm-controller@sha256:d7b7b96d679dbbdace044a18cca56544faa65f66e593fc600c08c9f814e0ea6a",
			},
			ImageRepo: "quay.io:443/acm-d",
			Expected: map[string]string{
				"application_ui": "quay.io:443/acm-d/application-ui@sha256:d7b7b96d679dbbdace044a18cca56544faa65f66e593fc600c08c9f814e0ea6a",
				"console_api":    "quay.io:443/acm-d/console-api@sha256:d7b7b96d679dbbdace044a18cca56544faa65f66e593fc600c08c9f814e0ea6a",
				"rcm_controller": "quay.io:443/acm-d/rcm-controller@sha256:d7b7b96d679dbbdace044a18cca56544faa65f66e593fc600c08c9f814e0ea6a",
			},
		},
	}

	for _, tt := range tests {
		if !reflect.DeepEqual(OverrideImageRepository(tt.ImageOverrides, tt.ImageRepo), tt.Expected) {
			t.Fatalf("ImageRepository override failure")
		}
	}
}

func TestShouldIgnoreOCPVersion(t *testing.T) {
	tests := []struct {
		name     string
		instance *backplanev1.MultiClusterEngine
		want     bool
	}{
		{
			name: "Annotation set to ignore",
			instance: &backplanev1.MultiClusterEngine{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{AnnotationIgnoreOCPVersion: ""}},
			},
			want: true,
		},
		{
			name:     "No annotations",
			instance: &backplanev1.MultiClusterEngine{},
			want:     false,
		},
		{
			name: "Different annotations",
			instance: &backplanev1.MultiClusterEngine{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{AnnotationMCEPause: "true"}},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldIgnoreOCPVersion(tt.instance); got != tt.want {
				t.Errorf("ShouldIgnoreOCPVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAnnotationPresent(t *testing.T) {
	type args struct {
		annotation string
		obj        client.Object
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Annotation is true",
			args: args{
				annotation: AnnotationMCEPause,
				obj: &backplanev1.MultiClusterEngine{
					ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{AnnotationMCEPause: "true"}},
				},
			},
			want: true,
		},
		{
			name: "Annotation is present",
			args: args{
				annotation: AnnotationMCEIgnore,
				obj: &backplanev1.MultiClusterEngine{
					ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{AnnotationMCEIgnore: ""}},
				},
			},
			want: true,
		},
		{
			name: "No annotations",
			args: args{
				annotation: AnnotationMCEIgnore,
				obj:        &backplanev1.MultiClusterEngine{},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AnnotationPresent(tt.args.annotation, tt.args.obj); got != tt.want {
				t.Errorf("AnnotationPresent() = %v, want %v", got, tt.want)
			}
		})
	}
}
