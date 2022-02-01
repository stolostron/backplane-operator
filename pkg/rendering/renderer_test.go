// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package renderer

import (
	backplane "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	// "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"os"
	"reflect"
	"testing"
)

const (
	chartsDir  = "pkg/templates/charts/always"
	chartsPath = "pkg/templates/charts/toggle/managed-serviceaccount"
	crdsDir    = "pkg/templates/crds"
)

func TestRender(t *testing.T) {

	os.Setenv("DIRECTORY_OVERRIDE", "../../")
	defer os.Unsetenv("DIRECTORY_OVERRIDE")
	crdsDir := crdsDir
	crds, errs := RenderCRDs(crdsDir)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Logf(err.Error())
		}
		t.Fatalf("failed to retrieve CRDs")
	}
	if len(crds) == 0 {
		t.Fatalf("Unable to render CRDs")
	}

	availabilityList := []string{"managedcluster-import-controller-v2", "ocm-controller", "ocm-proxyserver", "ocm-webhook"}
	backplaneNodeSelector := map[string]string{"select": "test"}
	backplaneImagePullSecret := "test"
	backplaneNamespace := "default"
	backplaneAvailability := backplane.HABasic
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
			AvailabilityConfig: backplaneAvailability,
			NodeSelector:       backplaneNodeSelector,
			ImagePullSecret:    backplaneImagePullSecret,
			Tolerations:        backplaneTolerations,
			TargetNamespace:    backplaneNamespace,
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
	// multiple charts
	chartsDir := chartsDir
	templates, errs := RenderCharts(chartsDir, testBackplane, testImages)
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
			if deployment.ObjectMeta.Namespace != backplaneNamespace {
				t.Fatalf("Namespace did not propagate to the deployments use")
			}

			if utils.Contains(availabilityList, deployment.ObjectMeta.Name) && *deployment.Spec.Replicas != 1 {
				t.Fatalf("AvailabilityConfig did not propagate to the deployments use")
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

	// single chart
	singleChartTestImages := map[string]string{}
	for _, v := range utils.GetTestImages() {
		singleChartTestImages[v] = "quay.io/test/test:Test"
	}
	chartsPath := chartsPath
	singleChartTemplates, errs := RenderChart(chartsPath, testBackplane, singleChartTestImages)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Logf(err.Error())
		}
		t.Fatalf("failed to retrieve templates")
		if len(singleChartTemplates) == 0 {
			t.Fatalf("Unable to render templates")
		}
	}
	for _, template := range singleChartTemplates {
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
			if deployment.ObjectMeta.Namespace != backplaneNamespace {
				t.Fatalf("Namespace did not propagate to the deployments use")
			}

			if utils.Contains(availabilityList, deployment.ObjectMeta.Name) && *deployment.Spec.Replicas != 1 {
				t.Fatalf("AvailabilityConfig did not propagate to the deployments use")
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
