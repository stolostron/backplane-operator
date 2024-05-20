// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package renderer

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	backplane "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
)

const (
	chartsDir  = "pkg/templates/charts/toggle"
	chartsPath = "pkg/templates/charts/toggle/managed-serviceaccount"
	crdsDir    = "../../hack/unit-test-crds"
)

func TestRender(t *testing.T) {

	os.Setenv("DIRECTORY_OVERRIDE", "../../")
	defer os.Unsetenv("DIRECTORY_OVERRIDE")

	availabilityList := []string{"clusterclaims-controller", "cluster-curator-controller", "managedcluster-import-controller-v2", "ocm-controller", "ocm-proxyserver", "ocm-webhook"}
	backplaneNodeSelector := map[string]string{"select": "test"}
	backplaneImagePullSecret := "test"
	backplaneNamespace := "default"
	backplaneAvailability := backplane.HAHigh
	backplaneTolerations := []corev1.Toleration{
		{
			Key:      "dedicated",
			Operator: "Exists",
			Effect:   "NoSchedule",
			Value:    "test",
		},
		{
			Key:      "node.ocs.openshift.io/storage",
			Operator: "Equal",
			Value:    "true",
			Effect:   "NoSchedule",
		},
		{
			Key:      "false",
			Operator: "false",
			Value:    "true",
			Effect:   "true",
		},
		{
			Key:      "22",
			Operator: "23",
			Value:    "24",
			Effect:   "25",
		},
		{
			Key:      "22.0",
			Operator: "23.1",
			Value:    "24.2",
			Effect:   "25.3",
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
	os.Setenv("ACM_HUB_OCP_VERSION", "4.10.0")

	testImages := map[string]string{}
	for _, v := range utils.GetTestImages() {
		testImages[v] = "quay.io/test/test:Test"
	}
	templateOverrides := map[string]string{}

	// multiple charts
	chartsDir := chartsDir
	templates, errs := RenderCharts(chartsDir, testBackplane, testImages, templateOverrides)
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
				fmt.Println("KV Pair: ", template)
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

			if deployment.Spec.Template.Spec.SecurityContext == nil {
				t.Logf("SecurityContext doesn't exist %s", deployment.Name)
			} else if profile := deployment.Spec.Template.Spec.SecurityContext.SeccompProfile; profile == nil || profile.Type != corev1.SeccompProfileTypeRuntimeDefault {
				t.Logf("Seccomp does not have runtime default type on deployment %s", deployment.Name)
			} else {
				t.Fatalf("Seccomp should not have runtime default type on deployment in 4.10 %s", deployment.Name)
			}

			if utils.Contains(availabilityList, deployment.ObjectMeta.Name) && *deployment.Spec.Replicas != 2 {
				t.Fatalf("AvailabilityConfig did not propagate to the %s deployment", deployment.ObjectMeta.Name)
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
				t.Fatalf("proxy variables not set in %s", deployment.ObjectMeta.Name)
			}
			containsHTTP = false
			containsHTTPS = false
			containsNO = false
		}
		if template.GetKind() == "AddOnDeploymentConfig" {
			addonDep := &addonv1alpha1.AddOnDeploymentConfig{}
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(template.Object, addonDep)
			if err != nil {
				fmt.Println("KV Pair: ", template)
				t.Fatalf(err.Error())
			}
			if nodePlacement := addonDep.Spec.NodePlacement.NodeSelector; !reflect.DeepEqual(nodePlacement, backplaneNodeSelector) {
				t.Fatalf("NodeSelector did not propagate to the template %s: expected %#v, got %#v", addonDep.Name, backplaneNodeSelector, nodePlacement)
			}
			if tolerationPlacement := addonDep.Spec.NodePlacement.Tolerations; !reflect.DeepEqual(tolerationPlacement, backplaneTolerations) {
				t.Fatalf("Toleration did not propagate to the template %s: expected %v, got %v", addonDep.Name, backplaneTolerations, tolerationPlacement)
			}
		}

	}

	// single chart
	singleChartTestImages := map[string]string{}
	for _, v := range utils.GetTestImages() {
		singleChartTestImages[v] = "quay.io/test/test:Test"
	}

	chartsPath := chartsPath
	singleChartTemplates, errs := RenderChart(chartsPath, testBackplane, singleChartTestImages, templateOverrides)

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
	os.Unsetenv("ACM_HUB_OCP_VERSION")

}

func TestRenderCoreCRDs(t *testing.T) {
	tests := []struct {
		name   string
		crdDir string
		want   []error
	}{
		{
			name:   "Render CRDs directory",
			crdDir: crdsDir,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var backplaneConfig *backplane.MultiClusterEngine
			got, errs := RenderCRDs(tt.crdDir, backplaneConfig)
			if errs != nil && len(errs) > 1 {
				t.Errorf("RenderCRDs() got = %v, want %v", errs, nil)
			}

			for _, u := range got {
				kind := "CustomResourceDefinition"
				apiVersion := "apiextensions.k8s.io/v1"
				if u.GetKind() != kind {
					t.Errorf("RenderCRDs() got Kind = %v, want Kind %v", errs, kind)
				}

				if u.GetAPIVersion() != apiVersion {
					t.Errorf("RenderCRDs() got apiversion = %v, want apiversion %v", errs, apiVersion)
				}
			}
		})
	}
}
func TestRenderCRDs(t *testing.T) {
	testBackplane := &backplane.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testBackplane",
		},
		Spec: backplane.MultiClusterEngineSpec{},
		Status: backplane.MultiClusterEngineStatus{
			Phase: "",
		},
	}
	tests := []struct {
		name   string
		crdDir string
		want   []error
	}{
		{
			name:   "Render CRDs directory",
			crdDir: crdsDir,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, errs := RenderCRDs(tt.crdDir, testBackplane)
			if errs != nil && len(errs) > 1 {
				t.Errorf("RenderCRDs() got = %v, want %v", errs, nil)
			}

			for _, u := range got {
				kind := "CustomResourceDefinition"
				apiVersion := "apiextensions.k8s.io/v1"
				if u.GetKind() != kind {
					t.Errorf("RenderCRDs() got Kind = %v, want Kind %v", errs, kind)
				}

				if u.GetAPIVersion() != apiVersion {
					t.Errorf("RenderCRDs() got apiversion = %v, want apiversion %v", errs, apiVersion)
				}

				namespace, conversion, _ := unstructured.NestedString(u.Object, "spec", "conversion", "webhook", "clientConfig", "service", "namespace")
				if conversion && namespace != "" {
					t.Errorf("did not properly set namespace")
				}

			}
		})
	}
}
