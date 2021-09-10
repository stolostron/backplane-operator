// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package renderer

import (
	backplane "github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	"github.com/open-cluster-management/backplane-operator/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	// "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"os"
	"reflect"
	"testing"
)

func TestRender(t *testing.T) {

	os.Setenv("DIRECTORY_OVERRIDE", "../../")
	defer os.Unsetenv("DIRECTORY_OVERRIDE")

	crds, errs := RenderCRDs()
	if len(errs) > 0 {
		for _, err := range errs {
			t.Logf(err.Error())
		}
		t.Fatalf("failed to retrieve CRDs")
	}
	if len(crds) == 0 {
		t.Fatalf("Unable to render CRDs")
	}

	backplaneNodeSelector := map[string]string{"select": "test"}
	backplaneImagePullSecret := "test"
	backplaneTolerations := []corev1.Toleration{
		{
			Key:      "dedicated",
			Operator: "Exists",
			Effect:   "NoSchedule",
		},
	}
	testBackplane := &backplane.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testBackplane",
		},
		Spec: backplane.MultiClusterEngineSpec{
			Foo:             "bar",
			NodeSelector:    backplaneNodeSelector,
			ImagePullSecret: backplaneImagePullSecret,
			Tolerations:     backplaneTolerations,
		},
		Status: backplane.MultiClusterEngineStatus{
			Phase: "",
		},
	}
	containsHTTP := false
	containsHTTPS := false
	containsNO := false
	os.Setenv("POD_NAMESPACE", "default")
	os.Setenv("HTTP_PROXY", "test1")
	os.Setenv("HTTPS_PROXY", "test2")
	os.Setenv("NO_PROXY", "test3")

	testImages := map[string]string{}
	for _, v := range utils.GetTestImages() {
		testImages[v] = "quay.io/test/test:Test"
	}

	templates, errs := RenderTemplates(testBackplane, testImages)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Logf(err.Error())
		}
		t.Fatalf("failed to retrieve templates")
		if len(templates) == 0 {
			t.Fatalf("Unable to render templates")
		}
	}
	for _, template := range templates {
		if template.GetKind() == "Deployment" {
			deployment := &appsv1.Deployment{}
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(template.Object, deployment)
			if err != nil {
				t.Fatalf(err.Error())
			}

			selectorEquality := reflect.DeepEqual(deployment.Spec.Template.Spec.NodeSelector, backplaneNodeSelector)
			if !selectorEquality {
				t.Fatalf("Node Selector did not propagate to the deployments use")
			}
			secretEquality := reflect.DeepEqual(deployment.Spec.Template.Spec.ImagePullSecrets[0].Name, backplaneImagePullSecret)
			if !secretEquality {
				t.Fatalf("Image Pull Secret did not propagate to the deployments use")
			}
			tolerationEquality := reflect.DeepEqual(deployment.Spec.Template.Spec.Tolerations, backplaneTolerations)
			if !tolerationEquality {
				t.Fatalf("Toleration did not propagate to the deployments use")
			}

			for _, proxyVar := range deployment.Spec.Template.Spec.Containers[0].Env {
				switch proxyVar.Name {
				case "HTTP_PROXY":
					containsHTTP = true
					if proxyVar.Value != "test1" {
						t.Fatalf("HTTP_PROXY not propagated")
					}
				case "HTTPS_PROXY":
					containsHTTPS = true
					if proxyVar.Value != "test2" {
						t.Fatalf("HTTPS_PROXY not propagated")
					}
				case "NO_PROXY":
					containsNO = true
					if proxyVar.Value != "test3" {
						t.Fatalf("NO_PROXY not propagated")
					}
				}

			}

			if !containsHTTP || !containsHTTPS || !containsNO {
				t.Fatalf("proxy variables not set")
			}
			containsHTTP = false
			containsHTTPS = false
			containsNO = false
		}

	}
	os.Unsetenv("HTTP_PROXY")
	os.Unsetenv("HTTPS_PROXY")
	os.Unsetenv("NO_PROXY")
	os.Unsetenv("POD_NAMESPACE")

}
