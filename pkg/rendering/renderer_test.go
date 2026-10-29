// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package renderer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	backplane "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/utils"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
	backplaneNodeSelector := map[string]string{"select": "test", "select2": "test2"}
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
			NetworkPolicies:    &backplane.NetworkPoliciesConfig{Enabled: true},
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
			t.Log(err.Error())
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
				t.Fatal(err.Error())
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
				t.Fatal(err.Error())
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
			t.Log(err.Error())
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
				t.Fatal(err.Error())
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

func TestClusterAPIProviderKubevirtCoexistence(t *testing.T) {
	t.Setenv("DIRECTORY_OVERRIDE", "../../")
	t.Setenv("POD_NAMESPACE", "default")
	t.Setenv("ACM_HUB_OCP_VERSION", "4.18.0")

	images := map[string]string{}
	for _, imageName := range utils.GetTestImages() {
		images[imageName] = "quay.io/test/test:latest"
	}

	config := &backplane.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec:       backplane.MultiClusterEngineSpec{TargetNamespace: "multicluster-engine"},
	}

	objects, errs := RenderChart(
		"pkg/templates/charts/toggle/cluster-api-provider-kubevirt",
		config,
		images,
		map[string]string{},
	)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Log(err)
		}
		t.Fatal("failed to render cluster-api-provider-kubevirt chart")
	}

	foundDeployment := false
	foundWebhookConfiguration := false
	for _, object := range objects {
		switch object.GetKind() {
		case "Deployment":
			if object.GetName() != "capk-controller-manager" {
				continue
			}
			foundDeployment = true
			deployment := &appsv1.Deployment{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, deployment); err != nil {
				t.Fatal(err)
			}
			for _, container := range deployment.Spec.Template.Spec.Containers {
				hasExcludedAnnotations := false
				for _, arg := range container.Args {
					if arg == "--watch-filter=multicluster-engine" {
						t.Fatal("CAPK deployment must not use a watch filter; HyperShift objects are excluded in-controller")
					}
					if arg == "--exclude-annotations=hypershift.openshift.io/nodePool,hypershift.openshift.io/cluster" {
						hasExcludedAnnotations = true
					}
				}
				if !hasExcludedAnnotations {
					t.Fatal("CAPK deployment must exclude HyperShift-owned objects by annotation")
				}
			}
		case "ValidatingWebhookConfiguration":
			if object.GetName() != "capk-validating-webhook-configuration" {
				continue
			}
			foundWebhookConfiguration = true
			webhookConfiguration := &admissionv1.ValidatingWebhookConfiguration{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, webhookConfiguration); err != nil {
				t.Fatal(err)
			}
			renderedWebhooks, ok := object.Object["webhooks"].([]interface{})
			if !ok {
				t.Fatalf("CAPK validating webhook configuration has invalid webhooks: %T", object.Object["webhooks"])
			}
			for index, renderedWebhook := range renderedWebhooks {
				webhook := webhookConfiguration.Webhooks[index]
				value, exists := renderedWebhook.(map[string]interface{})["objectSelector"]
				if !exists || value != nil {
					t.Fatalf("CAPK webhook %s must explicitly remove objectSelector with a null field", webhook.Name)
				}
				if webhook.ObjectSelector != nil {
					t.Fatalf("CAPK webhook %s must not use an object selector", webhook.Name)
				}
			}
		}
	}
	if !foundDeployment {
		t.Fatal("CAPK deployment was not rendered")
	}
	if !foundWebhookConfiguration {
		t.Fatal("CAPK validating webhook configuration was not rendered")
	}
}

func TestClusterProxyManagedProxyConfigurationArgs(t *testing.T) {
	t.Setenv("DIRECTORY_OVERRIDE", "../../")
	t.Setenv("ACM_HUB_OCP_VERSION", "4.12.0")
	t.Setenv("ACM_CLUSTER_INGRESS_DOMAIN", "example.com")

	testImages := map[string]string{}
	for _, image := range utils.GetTestImages() {
		testImages[image] = "quay.io/test/test:Test"
	}

	tests := []struct {
		name           string
		availability   backplane.AvailabilityType
		proxyAgentArgs []string
	}{
		{
			name:           "basic availability",
			availability:   backplane.HABasic,
			proxyAgentArgs: []string{"--keepalive-time=30s"},
		},
		{
			name:           "high availability",
			availability:   backplane.HAHigh,
			proxyAgentArgs: []string{"--sync-forever", "--keepalive-time=30s"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mce := &backplane.MultiClusterEngine{
				ObjectMeta: metav1.ObjectMeta{Name: "testBackplane"},
				Spec: backplane.MultiClusterEngineSpec{
					AvailabilityConfig: tt.availability,
					TargetNamespace:    "default",
				},
			}

			templates, errs := RenderChart(
				"pkg/templates/charts/toggle/cluster-proxy-addon",
				mce,
				testImages,
				map[string]string{},
			)
			if len(errs) > 0 {
				t.Fatalf("failed to render cluster-proxy-addon: %v", errs)
			}

			var managedProxy *unstructured.Unstructured
			for _, template := range templates {
				if template.GetKind() == "ManagedProxyConfiguration" && template.GetName() == "cluster-proxy" {
					managedProxy = template
					break
				}
			}
			if managedProxy == nil {
				t.Fatal("ManagedProxyConfiguration cluster-proxy was not rendered")
			}

			proxyAgentArgs, found, err := unstructured.NestedStringSlice(
				managedProxy.Object, "spec", "proxyAgent", "additionalArgs")
			if err != nil {
				t.Fatalf("read proxy-agent additionalArgs: %v", err)
			}
			if !found {
				t.Fatal("proxy-agent additionalArgs were not rendered")
			}
			if !reflect.DeepEqual(proxyAgentArgs, tt.proxyAgentArgs) {
				t.Fatalf("proxy-agent additionalArgs = %v, want %v", proxyAgentArgs, tt.proxyAgentArgs)
			}

			proxyServerArgs, found, err := unstructured.NestedStringSlice(
				managedProxy.Object, "spec", "proxyServer", "additionalArgs")
			if err != nil {
				t.Fatalf("read proxy-server additionalArgs: %v", err)
			}
			if !found {
				t.Fatal("proxy-server additionalArgs were not rendered")
			}
			wantProxyServerArgs := []string{"--keepalive-time=30s"}
			if !reflect.DeepEqual(proxyServerArgs, wantProxyServerArgs) {
				t.Fatalf("proxy-server additionalArgs = %v, want %v", proxyServerArgs, wantProxyServerArgs)
			}
		})
	}
}

func TestNonOCPRender(t *testing.T) {

	os.Setenv("DIRECTORY_OVERRIDE", "../../")
	defer os.Unsetenv("DIRECTORY_OVERRIDE")
	originalDeployOnOCP := utils.DeployOnOCP()
	defer utils.SetDeployOnOCP(originalDeployOnOCP)
	utils.SetDeployOnOCP(false)
	availabilityList := []string{"clusterclaims-controller", "cluster-curator-controller", "managedcluster-import-controller-v2", "ocm-controller", "ocm-proxyserver", "ocm-webhook"}
	backplaneNodeSelector := map[string]string{"select": "test", "select2": "test2"}
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
			NetworkPolicies:    &backplane.NetworkPoliciesConfig{Enabled: true},
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
			t.Log(err.Error())
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
				t.Fatal(err.Error())
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
				t.Fatal(err.Error())
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
			t.Log(err.Error())
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
				t.Fatal(err.Error())
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
			got, errs := RenderCRDs(tt.crdDir, backplaneConfig, []string{})
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
			got, errs := RenderCRDs(tt.crdDir, testBackplane, []string{})
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

func TestRenderCRDsSkipDirectories(t *testing.T) {
	// Test that "cluster-api" skip doesn't affect "cluster-api-provider-aws"
	tests := []struct {
		name         string
		crdDir       string
		skipDirs     []string
		expectedCRDs map[string]bool // CRD names that should be rendered
	}{
		{
			name:     "Skip cluster-api but not cluster-api-provider-aws",
			crdDir:   "../../test/unit-test-crds",
			skipDirs: []string{"cluster-api"},
			expectedCRDs: map[string]bool{
				// CAPA CRDs should still be rendered
				"awsclusters.infrastructure.cluster.x-k8s.io": true,
				// CAPI CRDs should be skipped
				"clusters.cluster.x-k8s.io": false,
			},
		},
		{
			name:     "Skip cluster-api-provider-aws but not cluster-api",
			crdDir:   "../../test/unit-test-crds",
			skipDirs: []string{"cluster-api-provider-aws"},
			expectedCRDs: map[string]bool{
				// CAPI CRDs should be rendered
				"clusters.cluster.x-k8s.io": true,
				// CAPA CRDs should be skipped
				"awsclusters.infrastructure.cluster.x-k8s.io": false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, errs := RenderCRDs(tt.crdDir, nil, tt.skipDirs)
			if len(errs) > 0 {
				t.Errorf("RenderCRDs() errors = %v", errs)
			}

			// Create map of rendered CRD names
			renderedCRDs := make(map[string]bool)
			for _, crd := range got {
				renderedCRDs[crd.GetName()] = true
			}

			// Check expectations
			for expectedCRD, shouldBeRendered := range tt.expectedCRDs {
				_, wasRendered := renderedCRDs[expectedCRD]
				if shouldBeRendered && !wasRendered {
					t.Errorf("Expected CRD %s to be rendered but it was skipped", expectedCRD)
				}
				if !shouldBeRendered && wasRendered {
					t.Errorf("Expected CRD %s to be skipped but it was rendered", expectedCRD)
				}
			}
		})
	}
}

func TestNetworkPoliciesValueInjection(t *testing.T) {
	os.Setenv("DIRECTORY_OVERRIDE", "../../")
	defer os.Unsetenv("DIRECTORY_OVERRIDE")
	os.Setenv("POD_NAMESPACE", "default")
	defer os.Unsetenv("POD_NAMESPACE")
	os.Setenv("ACM_HUB_OCP_VERSION", "4.12.0")
	defer os.Unsetenv("ACM_HUB_OCP_VERSION")

	testImages := map[string]string{}
	for _, v := range utils.GetTestImages() {
		testImages[v] = "quay.io/test/test:Test"
	}

	t.Run("NetworkPolicies enabled renders NP template", func(t *testing.T) {
		mce := &backplane.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-mce"},
			Spec: backplane.MultiClusterEngineSpec{
				TargetNamespace: "default",
				NetworkPolicies: &backplane.NetworkPoliciesConfig{Enabled: true},
			},
		}

		templates, errs := RenderChart("pkg/templates/charts/toggle/hypershift", mce, testImages, map[string]string{})
		if len(errs) > 0 {
			t.Fatalf("RenderChart failed: %v", errs)
		}

		expectedNPs := map[string]bool{
			"hypershift-addon-manager-network-policy": false,
			"hcp-cli-download-network-policy":         false,
		}
		for _, tmpl := range templates {
			if tmpl.GetKind() == "NetworkPolicy" {
				if _, ok := expectedNPs[tmpl.GetName()]; ok {
					expectedNPs[tmpl.GetName()] = true
				} else {
					t.Errorf("unexpected NetworkPolicy name: %s", tmpl.GetName())
				}
			}
		}
		for name, found := range expectedNPs {
			if !found {
				t.Errorf("expected NetworkPolicy %s when networkPolicies.enabled=true", name)
			}
		}
	})

	t.Run("NetworkPolicies disabled excludes NP template", func(t *testing.T) {
		mce := &backplane.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-mce"},
			Spec: backplane.MultiClusterEngineSpec{
				TargetNamespace: "default",
				NetworkPolicies: &backplane.NetworkPoliciesConfig{Enabled: false},
			},
		}

		templates, errs := RenderChart("pkg/templates/charts/toggle/hypershift", mce, testImages, map[string]string{})
		if len(errs) > 0 {
			t.Fatalf("RenderChart failed: %v", errs)
		}

		for _, tmpl := range templates {
			if tmpl.GetKind() == "NetworkPolicy" {
				t.Error("NetworkPolicy should not be rendered when networkPolicies.enabled=false")
			}
		}
	})
}

func TestHypershiftHCPProxyDisabled(t *testing.T) {
	t.Setenv("DIRECTORY_OVERRIDE", "../../")
	t.Setenv("POD_NAMESPACE", "default")
	t.Setenv("ACM_HUB_OCP_VERSION", "4.12.0")

	testImages := map[string]string{}
	for _, image := range utils.GetTestImages() {
		testImages[image] = "quay.io/test/test:Test"
	}
	mce := &backplane.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mce"},
		Spec:       backplane.MultiClusterEngineSpec{TargetNamespace: "default"},
	}

	templates, errs := RenderChart("pkg/templates/charts/toggle/hypershift", mce, testImages, map[string]string{})
	if len(errs) > 0 {
		t.Fatalf("RenderChart failed: %v", errs)
	}

	for _, template := range templates {
		switch template.GetKind() {
		case "APIService":
			if template.GetName() == "v1alpha1.hcp.ocm.io" {
				t.Error("HCP proxy APIService must not render while the shipped addon image lacks the proxy")
			}
		case "Service":
			if template.GetName() == "hypershift-addon-hcp-proxy" {
				t.Error("HCP proxy Service must not render while the shipped addon image lacks the proxy")
			}
		case "ClusterRole", "ClusterRoleBinding":
			if strings.Contains(template.GetName(), "hypershift-addon-hcp-proxy") {
				t.Errorf("HCP proxy %s must not render while the shipped addon image lacks the proxy", template.GetKind())
			}
		case "Deployment":
			if template.GetName() != "hypershift-addon-manager" {
				continue
			}
			deployment := &appsv1.Deployment{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(template.Object, deployment); err != nil {
				t.Fatal(err)
			}
			for _, container := range deployment.Spec.Template.Spec.Containers {
				for _, port := range container.Ports {
					if port.ContainerPort == 9443 {
						t.Error("HCP proxy port must not render while the shipped addon image lacks the proxy")
					}
				}
				for _, mount := range container.VolumeMounts {
					if mount.Name == "hcp-proxy-tls" {
						t.Error("HCP proxy TLS mount must not render while the shipped addon image lacks the proxy")
					}
				}
			}
			for _, volume := range deployment.Spec.Template.Spec.Volumes {
				if volume.Name == "hcp-proxy-tls" {
					t.Error("HCP proxy TLS volume must not render while the shipped addon image lacks the proxy")
				}
			}
		}
	}
}

func TestRendererLeavesHCPProxyToChartDefaults(t *testing.T) {
	values := &Values{}
	injectValuesOverrides(values, &backplane.MultiClusterEngine{}, map[string]string{}, map[string]string{})

	renderedValues, err := values.ToValues()
	if err != nil {
		t.Fatalf("ToValues() error = %v", err)
	}
	if _, exists := renderedValues.AsMap()["hcpProxyEnabled"]; exists {
		t.Fatal("renderer must not override the chart's hcpProxyEnabled default")
	}
}

func TestCAPOAProxyEgressNetworkPolicies(t *testing.T) {
	t.Setenv("DIRECTORY_OVERRIDE", "../../")
	t.Setenv("POD_NAMESPACE", "default")
	t.Setenv("ACM_HUB_OCP_VERSION", "4.18.0")
	t.Setenv("HTTP_PROXY", "http://192.0.2.10:3128")
	t.Setenv("HTTPS_PROXY", "http://192.0.2.10:3128")
	t.Setenv("NO_PROXY", "*")

	values := &Values{}
	injectValuesOverrides(values, &backplane.MultiClusterEngine{}, map[string]string{}, map[string]string{})
	if len(values.HubConfig.ProxyEgress) != 1 {
		t.Fatalf("proxy egress rule count = %d, want 1", len(values.HubConfig.ProxyEgress))
	}
	if values.HubConfig.ProxyEgress[0].Port != 3128 {
		t.Fatalf("proxy egress port = %d, want 3128", values.HubConfig.ProxyEgress[0].Port)
	}

	images := map[string]string{}
	for _, imageName := range utils.GetTestImages() {
		images[imageName] = "quay.io/test/test:latest"
	}
	for _, chartPath := range []string{
		"pkg/templates/charts/toggle/cluster-api-provider-openshift-assisted",
		"pkg/templates/charts/toggle/cluster-api-provider-openshift-assisted-k8s",
	} {
		tolerationSeconds := int64(0)
		templates, errs := RenderChart(
			chartPath,
			&backplane.MultiClusterEngine{Spec: backplane.MultiClusterEngineSpec{
				TargetNamespace: "multicluster-engine",
				ImagePullSecret: "*",
				Tolerations: []corev1.Toleration{{
					Key:               "capoa-test",
					Operator:          corev1.TolerationOpEqual,
					Value:             "123",
					Effect:            corev1.TaintEffectNoSchedule,
					TolerationSeconds: &tolerationSeconds,
				}},
			}},
			images,
			map[string]string{},
		)
		if len(errs) > 0 {
			t.Fatalf("RenderChart(%s) errors = %v", chartPath, errs)
		}

		for _, policyName := range []string{"capoa-bootstrap-controller-manager", "capoa-controlplane-controller-manager"} {
			foundProxyEgress := false
			for _, template := range templates {
				if template.GetKind() != "NetworkPolicy" || template.GetName() != policyName {
					continue
				}
				networkPolicy := &networkingv1.NetworkPolicy{}
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(template.Object, networkPolicy); err != nil {
					t.Fatal(err)
				}
				if hasEgressPortAndCIDR(networkPolicy, 3128, "192.0.2.10/32") {
					foundProxyEgress = true
				}
				assertLegacyEgressAnnotation(t, template)
				assertLegacyDNSBaselineUsesPreUpgradePort(t, template)
			}
			if !foundProxyEgress {
				t.Fatalf("%s %s NetworkPolicy does not allow the configured proxy endpoint", chartPath, policyName)
			}
		}
		for _, deploymentName := range []string{"capoa-bootstrap-controller-manager", "capoa-controlplane-controller-manager"} {
			foundDeployment := false
			for _, template := range templates {
				if template.GetKind() != "Deployment" || template.GetName() != deploymentName {
					continue
				}
				foundDeployment = true
				deployment := &appsv1.Deployment{}
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(template.Object, deployment); err != nil {
					t.Fatal(err)
				}
				for _, container := range deployment.Spec.Template.Spec.Containers {
					for _, envVar := range container.Env {
						if envVar.Name == "NO_PROXY" && envVar.Value != "*" {
							t.Fatalf("%s NO_PROXY value = %q, want %q", deploymentName, envVar.Value, "*")
						}
					}
				}
				foundToleration := false
				for _, toleration := range deployment.Spec.Template.Spec.Tolerations {
					if toleration.Key != "capoa-test" {
						continue
					}
					foundToleration = true
					if toleration.TolerationSeconds == nil || *toleration.TolerationSeconds != 0 {
						t.Fatalf("%s tolerationSeconds = %v, want 0", deploymentName, toleration.TolerationSeconds)
					}
				}
				if !foundToleration {
					t.Fatalf("%s deployment does not contain the configured toleration", deploymentName)
				}
				if len(deployment.Spec.Template.Spec.ImagePullSecrets) != 1 ||
					deployment.Spec.Template.Spec.ImagePullSecrets[0].Name != "*" {
					t.Fatalf("%s image pull secret was not rendered as a string", deploymentName)
				}
			}
			if !foundDeployment {
				t.Fatalf("%s %s deployment was not rendered", chartPath, deploymentName)
			}
		}
	}
}

func TestClusterAPIProviderKubevirtRendersQuotedProxyValuesAndCertificate(t *testing.T) {
	t.Setenv("DIRECTORY_OVERRIDE", "../../")
	t.Setenv("POD_NAMESPACE", "default")
	t.Setenv("ACM_HUB_OCP_VERSION", "4.18.0")
	t.Setenv("HTTP_PROXY", "http://192.0.2.10:3128")
	t.Setenv("HTTPS_PROXY", "http://192.0.2.10:3128")
	t.Setenv("NO_PROXY", "*")
	utils.SetDeployOnOCP(false)
	t.Cleanup(func() { utils.SetDeployOnOCP(true) })
	tolerationSeconds := int64(0)

	images := map[string]string{}
	for _, imageName := range utils.GetTestImages() {
		images[imageName] = "quay.io/test/test:latest"
	}
	templates, errs := RenderChart(
		"pkg/templates/charts/toggle/cluster-api-provider-kubevirt",
		&backplane.MultiClusterEngine{
			Spec: backplane.MultiClusterEngineSpec{
				TargetNamespace: "multicluster-engine",
				ImagePullSecret: "*",
				Tolerations: []corev1.Toleration{{
					Key:               "capk-test",
					Operator:          corev1.TolerationOpEqual,
					Value:             "123",
					Effect:            corev1.TaintEffectNoSchedule,
					TolerationSeconds: &tolerationSeconds,
				}},
			},
		},
		images,
		map[string]string{},
	)
	if len(errs) > 0 {
		t.Fatalf("RenderChart() errors = %v", errs)
	}

	foundDeployment := false
	foundCertificate := false
	for _, template := range templates {
		switch template.GetKind() {
		case "Deployment":
			if template.GetName() != "capk-controller-manager" {
				continue
			}
			foundDeployment = true
			deployment := &appsv1.Deployment{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(template.Object, deployment); err != nil {
				t.Fatal(err)
			}
			for _, container := range deployment.Spec.Template.Spec.Containers {
				values := map[string]string{}
				for _, envVar := range container.Env {
					values[envVar.Name] = envVar.Value
				}
				if values["HTTP_PROXY"] != "http://192.0.2.10:3128" ||
					values["HTTPS_PROXY"] != "http://192.0.2.10:3128" || values["NO_PROXY"] != "*" {
					t.Fatalf("CAPK proxy values were not rendered verbatim: %v", values)
				}
			}
			foundToleration := false
			for _, toleration := range deployment.Spec.Template.Spec.Tolerations {
				if toleration.Key == "capk-test" {
					foundToleration = true
					if toleration.Value != "123" {
						t.Fatalf("CAPK toleration value = %q, want %q", toleration.Value, "123")
					}
					if toleration.TolerationSeconds == nil || *toleration.TolerationSeconds != 0 {
						t.Fatalf("CAPK tolerationSeconds = %v, want 0", toleration.TolerationSeconds)
					}
				}
			}
			if !foundToleration {
				t.Fatal("CAPK deployment does not contain the configured toleration")
			}
			if len(deployment.Spec.Template.Spec.ImagePullSecrets) != 1 ||
				deployment.Spec.Template.Spec.ImagePullSecrets[0].Name != "*" {
				t.Fatal("CAPK image pull secret was not rendered as a string")
			}
		case "Certificate":
			if template.GetName() != "capk-cert" {
				continue
			}
			foundCertificate = true
			spec, ok := template.Object["spec"].(map[string]interface{})
			if !ok {
				t.Fatalf("CAPK Certificate spec has invalid type: %T", template.Object["spec"])
			}
			if _, exists := spec["commonName"]; exists {
				t.Fatal("CAPK Certificate must not set commonName")
			}
			subject, ok := spec["subject"].(map[string]interface{})
			if !ok {
				t.Fatalf("CAPK Certificate subject has invalid type: %T", spec["subject"])
			}
			organizations, ok := subject["organizations"].([]interface{})
			if !ok || len(organizations) == 0 {
				t.Fatalf("CAPK Certificate subject organizations have invalid value: %v", subject["organizations"])
			}
			dnsNames, ok := spec["dnsNames"].([]interface{})
			if !ok {
				t.Fatalf("CAPK Certificate dnsNames have invalid type: %T", spec["dnsNames"])
			}
			foundDNSName := false
			for _, dnsName := range dnsNames {
				if dnsName == "capk-webhook-service.multicluster-engine.svc" {
					foundDNSName = true
				}
			}
			if !foundDNSName {
				t.Fatal("CAPK Certificate does not contain the webhook service DNS name")
			}
		case "NetworkPolicy":
			if template.GetName() == "capk-controller-manager" {
				assertLegacyEgressAnnotation(t, template)
			}
		}
	}
	if !foundDeployment || !foundCertificate {
		t.Fatal("CAPK deployment or certificate was not rendered")
	}
}

func TestCAPOAChartsRenderWithoutProxyEgress(t *testing.T) {
	t.Setenv("DIRECTORY_OVERRIDE", "../../")
	t.Setenv("POD_NAMESPACE", "default")
	t.Setenv("ACM_HUB_OCP_VERSION", "4.18.0")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("NO_PROXY", "")

	values := &Values{}
	injectValuesOverrides(values, &backplane.MultiClusterEngine{}, map[string]string{}, map[string]string{})
	renderedValues, err := values.ToValues()
	if err != nil {
		t.Fatalf("ToValues() error = %v", err)
	}
	hubConfig, ok := renderedValues.AsMap()["hubconfig"].(map[string]interface{})
	if !ok {
		t.Fatalf("hubconfig values have invalid type: %T", renderedValues.AsMap()["hubconfig"])
	}
	proxyEgress, ok := hubConfig["proxyEgress"].([]interface{})
	if !ok {
		t.Fatalf("proxyEgress value has invalid type: %T", hubConfig["proxyEgress"])
	}
	if len(proxyEgress) != 0 {
		t.Fatalf("proxy egress rule count = %d, want 0", len(proxyEgress))
	}

	images := map[string]string{}
	for _, imageName := range utils.GetTestImages() {
		images[imageName] = "quay.io/test/test:latest"
	}
	for _, chartPath := range []string{
		"pkg/templates/charts/toggle/cluster-api-provider-openshift-assisted",
		"pkg/templates/charts/toggle/cluster-api-provider-openshift-assisted-k8s",
	} {
		_, errs := RenderChart(
			chartPath,
			&backplane.MultiClusterEngine{Spec: backplane.MultiClusterEngineSpec{TargetNamespace: "multicluster-engine"}},
			images,
			map[string]string{},
		)
		if len(errs) > 0 {
			t.Fatalf("RenderChart(%s) errors = %v", chartPath, errs)
		}
	}
}

func TestProxyEgressNormalizesIPv4MappedAddresses(t *testing.T) {
	rule, ok := parseProxyEgressRule("http://[::ffff:192.0.2.10]:3128")
	if !ok {
		t.Fatal("parseProxyEgressRule() failed to parse an IPv4-mapped address")
	}
	if len(rule.CIDRs) != 1 || rule.CIDRs[0] != "192.0.2.10/32" {
		t.Fatalf("proxy CIDRs = %v, want [192.0.2.10/32]", rule.CIDRs)
	}
}

func TestProxyEgressHostnameFallback(t *testing.T) {
	rule, ok := parseProxyEgressRule("http://proxy.example.com:3128")
	if !ok {
		t.Fatal("parseProxyEgressRule() failed to parse a hostname proxy address")
	}
	if rule.Port != 3128 {
		t.Fatalf("proxy port = %d, want 3128", rule.Port)
	}
	wantCIDRs := []string{"0.0.0.0/0", "::/0"}
	if len(rule.CIDRs) != len(wantCIDRs) || rule.CIDRs[0] != wantCIDRs[0] || rule.CIDRs[1] != wantCIDRs[1] {
		t.Fatalf("proxy CIDRs = %v, want %v", rule.CIDRs, wantCIDRs)
	}
}

func TestClusterAPIProviderKubevirtWebhookVersionCompatibility(t *testing.T) {
	tests := []struct {
		name                string
		ocpVersion          string
		deployOnOCP         bool
		wantFailurePolicy   string
		wantMatchConditions bool
	}{
		{name: "OpenShift 4.15 uses fail-closed matchConditions", ocpVersion: "4.15.0", deployOnOCP: true, wantFailurePolicy: "Fail", wantMatchConditions: true},
		{name: "OpenShift 4.12 omits unsupported matchConditions", ocpVersion: "4.12.0", deployOnOCP: true, wantFailurePolicy: "Ignore", wantMatchConditions: false},
		{name: "non-OCP deployments omit matchConditions", ocpVersion: "4.18.0", deployOnOCP: false, wantFailurePolicy: "Ignore", wantMatchConditions: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			originalDeployOnOCP := utils.DeployOnOCP()
			defer utils.SetDeployOnOCP(originalDeployOnOCP)
			utils.SetDeployOnOCP(test.deployOnOCP)
			t.Setenv("DIRECTORY_OVERRIDE", "../../")
			t.Setenv("POD_NAMESPACE", "default")
			t.Setenv("ACM_HUB_OCP_VERSION", test.ocpVersion)

			images := map[string]string{}
			for _, imageName := range utils.GetTestImages() {
				images[imageName] = "quay.io/test/test:latest"
			}
			templates, errs := RenderChart(
				"pkg/templates/charts/toggle/cluster-api-provider-kubevirt",
				&backplane.MultiClusterEngine{Spec: backplane.MultiClusterEngineSpec{
					TargetNamespace: "multicluster-engine",
				}},
				images,
				map[string]string{},
			)
			if len(errs) > 0 {
				t.Fatalf("RenderChart() errors = %v", errs)
			}

			foundWebhookConfiguration := false
			for _, template := range templates {
				if template.GetKind() != "ValidatingWebhookConfiguration" ||
					template.GetName() != "capk-validating-webhook-configuration" {
					continue
				}
				foundWebhookConfiguration = true
				webhookConfiguration := &admissionv1.ValidatingWebhookConfiguration{}
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(template.Object, webhookConfiguration); err != nil {
					t.Fatal(err)
				}
				if len(webhookConfiguration.Webhooks) != 3 {
					t.Fatalf("CAPK webhook count = %d, want 3", len(webhookConfiguration.Webhooks))
				}
				for _, webhook := range webhookConfiguration.Webhooks {
					if webhook.FailurePolicy == nil || string(*webhook.FailurePolicy) != test.wantFailurePolicy {
						t.Fatalf("CAPK webhook %s failurePolicy = %v, want %s",
							webhook.Name, webhook.FailurePolicy, test.wantFailurePolicy)
					}
					if hasMatchConditions := len(webhook.MatchConditions) != 0; hasMatchConditions != test.wantMatchConditions {
						t.Fatalf("CAPK webhook %s matchConditions = %v, want present=%t",
							webhook.Name, webhook.MatchConditions, test.wantMatchConditions)
					}
				}
			}
			if !foundWebhookConfiguration {
				t.Fatal("CAPK ValidatingWebhookConfiguration was not rendered")
			}
		})
	}
}

func TestClusterAPIProviderKubevirtNetworkPolicyWebhooksAndRBAC(t *testing.T) {
	t.Setenv("DIRECTORY_OVERRIDE", "../../")
	t.Setenv("POD_NAMESPACE", "default")
	t.Setenv("ACM_HUB_OCP_VERSION", "4.18.0")
	t.Setenv("HTTP_PROXY", "http://192.0.2.10:3128")
	t.Setenv("HTTPS_PROXY", "http://192.0.2.10:3128")
	t.Setenv("NO_PROXY", "localhost")

	images := map[string]string{}
	for _, imageName := range utils.GetTestImages() {
		images[imageName] = "quay.io/test/test:latest"
	}
	templates, errs := RenderChart(
		"pkg/templates/charts/toggle/cluster-api-provider-kubevirt",
		&backplane.MultiClusterEngine{Spec: backplane.MultiClusterEngineSpec{TargetNamespace: "multicluster-engine"}},
		images,
		map[string]string{},
	)
	if len(errs) > 0 {
		t.Fatalf("RenderChart() errors = %v", errs)
	}

	foundNetworkPolicy := false
	foundWebhookConfiguration := false
	foundAdminRole := false
	foundEditRole := false
	for _, template := range templates {
		switch template.GetKind() {
		case "NetworkPolicy":
			if template.GetName() != "capk-controller-manager" {
				continue
			}
			foundNetworkPolicy = true
			networkPolicy := &networkingv1.NetworkPolicy{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(template.Object, networkPolicy); err != nil {
				t.Fatal(err)
			}
			if !hasEgressPortAndCIDR(networkPolicy, 22, "0.0.0.0/0") || !hasEgressPortAndCIDR(networkPolicy, 22, "::/0") {
				t.Fatal("CAPK NetworkPolicy does not allow SSH to IPv4 and IPv6 guest networks")
			}
			if !hasEgressPortAndCIDR(networkPolicy, 3128, "192.0.2.10/32") {
				t.Fatal("CAPK NetworkPolicy does not allow the configured proxy endpoint")
			}
		case "ValidatingWebhookConfiguration":
			if template.GetName() != "capk-validating-webhook-configuration" {
				continue
			}
			foundWebhookConfiguration = true
			webhookConfiguration := &admissionv1.ValidatingWebhookConfiguration{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(template.Object, webhookConfiguration); err != nil {
				t.Fatal(err)
			}
			if len(webhookConfiguration.Webhooks) != 3 {
				t.Fatalf("CAPK webhook count = %d, want 3", len(webhookConfiguration.Webhooks))
			}
			for _, webhook := range webhookConfiguration.Webhooks {
				if len(webhook.MatchConditions) != 1 {
					t.Fatalf("CAPK webhook %s has %d match conditions, want 1", webhook.Name, len(webhook.MatchConditions))
				}
				condition := webhook.MatchConditions[0]
				if condition.Name != "exclude-hypershift-owned" || !strings.Contains(condition.Expression, "hypershift.openshift.io/nodePool") || !strings.Contains(condition.Expression, "hypershift.openshift.io/cluster") {
					t.Fatalf("CAPK webhook %s does not exclude HyperShift-owned objects", webhook.Name)
				}
			}
		case "ClusterRole":
			if template.GetName() != "capk-admin-aggregate" && template.GetName() != "capk-edit-aggregate" {
				continue
			}
			if template.GetName() == "capk-admin-aggregate" {
				foundAdminRole = true
			} else {
				foundEditRole = true
			}
			clusterRole := &rbacv1.ClusterRole{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(template.Object, clusterRole); err != nil {
				t.Fatal(err)
			}
			for _, rule := range clusterRole.Rules {
				if utils.Contains(rule.Resources, "kubevirtmachines") {
					t.Fatalf("%s grants aggregate access to KubevirtMachine resources", template.GetName())
				}
				if !utils.Contains(rule.Resources, "kubevirtclusters") {
					continue
				}
				verbs := map[string]bool{}
				for _, verb := range rule.Verbs {
					verbs[verb] = true
				}
				if len(verbs) != 3 || !verbs["get"] || !verbs["list"] || !verbs["watch"] {
					t.Fatalf("%s grants invalid KubevirtCluster verbs: %v", template.GetName(), rule.Verbs)
				}
			}
		}
	}
	if !foundNetworkPolicy || !foundWebhookConfiguration || !foundAdminRole || !foundEditRole {
		t.Fatal("CAPK NetworkPolicy, webhook configuration, or aggregate roles were not rendered")
	}
}

func hasEgressPortAndCIDR(policy *networkingv1.NetworkPolicy, port int32, cidr string) bool {
	for _, egressRule := range policy.Spec.Egress {
		hasPort := false
		for _, networkPort := range egressRule.Ports {
			if networkPort.Protocol != nil && *networkPort.Protocol == corev1.ProtocolTCP && networkPort.Port.IntValue() == int(port) {
				hasPort = true
			}
		}
		hasCIDR := false
		for _, peer := range egressRule.To {
			if peer.IPBlock != nil && peer.IPBlock.CIDR == cidr {
				hasCIDR = true
			}
		}
		if hasPort && hasCIDR {
			return true
		}
	}
	return false
}

func assertLegacyEgressAnnotation(t *testing.T, template *unstructured.Unstructured) {
	t.Helper()
	value, exists := template.GetAnnotations()["installer.open-cluster-management.io/legacy-managed-egress-rules"]
	if !exists {
		t.Fatalf("%s is missing its legacy managed egress annotation", template.GetName())
	}
	legacyRules := []networkingv1.NetworkPolicyEgressRule{}
	if err := json.Unmarshal([]byte(value), &legacyRules); err != nil {
		t.Fatalf("%s has an invalid legacy managed egress annotation: %v", template.GetName(), err)
	}
	if len(legacyRules) == 0 {
		t.Fatalf("%s has an empty legacy managed egress annotation", template.GetName())
	}
}

func assertLegacyDNSBaselineUsesPreUpgradePort(t *testing.T, template *unstructured.Unstructured) {
	t.Helper()
	if err := validateLegacyDNSBaselineUsesPreUpgradePort(template); err != nil {
		t.Fatal(err)
	}
}

func validateLegacyDNSBaselineUsesPreUpgradePort(template *unstructured.Unstructured) error {
	value := template.GetAnnotations()["installer.open-cluster-management.io/legacy-managed-egress-rules"]
	legacyRules := []networkingv1.NetworkPolicyEgressRule{}
	if err := json.Unmarshal([]byte(value), &legacyRules); err != nil {
		return fmt.Errorf("%s has an invalid legacy managed egress annotation: %w", template.GetName(), err)
	}
	foundPreUpgradePort := false
	for _, rule := range legacyRules {
		for _, networkPort := range rule.Ports {
			if networkPort.Port.IntValue() == 53 {
				return fmt.Errorf("%s legacy managed egress baseline must not include port 53", template.GetName())
			}
			if networkPort.Port.IntValue() == 5353 {
				foundPreUpgradePort = true
			}
		}
	}
	if !foundPreUpgradePort {
		return fmt.Errorf("%s legacy managed egress baseline does not include the pre-upgrade DNS port 5353", template.GetName())
	}
	return nil
}

func TestAssertLegacyDNSBaselineUsesPreUpgradePort(t *testing.T) {
	newTemplate := func(rules string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "test-network-policy",
				"annotations": map[string]interface{}{
					"installer.open-cluster-management.io/legacy-managed-egress-rules": rules,
				},
			},
		}}
	}

	if err := validateLegacyDNSBaselineUsesPreUpgradePort(
		newTemplate(`[{"ports":[{"port":5353,"protocol":"UDP"}]}]`),
	); err != nil {
		t.Fatalf("valid baseline rejected: %v", err)
	}

	err := validateLegacyDNSBaselineUsesPreUpgradePort(
		newTemplate(`[{"ports":[{"port":5353,"protocol":"UDP"}]},{"ports":[{"port":53,"protocol":"UDP"}]}]`),
	)
	if err == nil || !strings.Contains(err.Error(), "must not include port 53") {
		t.Fatalf("later port 53 error = %v, want must not include port 53", err)
	}
}

func TestHypershiftAddonAgentPBDRBAC(t *testing.T) {
	t.Setenv("DIRECTORY_OVERRIDE", "../../")
	t.Setenv("POD_NAMESPACE", "default")
	t.Setenv("ACM_HUB_OCP_VERSION", "4.12.0")

	testImages := map[string]string{}
	for _, image := range utils.GetTestImages() {
		testImages[image] = "quay.io/test/test:Test"
	}
	mce := &backplane.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mce"},
		Spec:       backplane.MultiClusterEngineSpec{TargetNamespace: "default"},
	}

	templates, errs := RenderChart("pkg/templates/charts/toggle/hypershift", mce, testImages, map[string]string{})
	if len(errs) > 0 {
		t.Fatalf("RenderChart failed: %v", errs)
	}

	roleFound := false
	bindingFound := false
	for _, template := range templates {
		if template.GetName() != "open-cluster-management:hypershift:hypershift-addon-agent-pdb" {
			continue
		}
		switch template.GetKind() {
		case "ClusterRole":
			roleFound = true
		case "ClusterRoleBinding":
			bindingFound = true
		}
	}
	if !roleFound || !bindingFound {
		t.Fatalf("expected addon-agent PDB RBAC, roleFound=%t bindingFound=%t", roleFound, bindingFound)
	}
}

func TestClusterLifecycleStateMetricsNetworkPolicies(t *testing.T) {

	t.Setenv("DIRECTORY_OVERRIDE", "../../")
	t.Setenv("POD_NAMESPACE", "default")
	t.Setenv("ACM_HUB_OCP_VERSION", "4.12.0")

	testImages := map[string]string{}
	for _, v := range utils.GetTestImages() {
		testImages[v] = "quay.io/test/test:Test"
	}

	clcChart := "pkg/templates/charts/toggle/cluster-lifecycle"
	const npName = "clusterlifecycle-state-metrics-network-policy"

	t.Run("NetworkPolicies enabled renders CLSM NP", func(t *testing.T) {
		mce := &backplane.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-mce"},
			Spec: backplane.MultiClusterEngineSpec{
				TargetNamespace: "default",
				NetworkPolicies: &backplane.NetworkPoliciesConfig{Enabled: true},
			},
		}

		templates, errs := RenderChart(clcChart, mce, testImages, map[string]string{})
		if len(errs) > 0 {
			t.Fatalf("RenderChart failed: %v", errs)
		}

		// The cluster-lifecycle chart renders a NetworkPolicy per controller from a
		// single multi-document networkpolicy.yaml, plus a separate CLSM policy file.
		// Assert all of them are present to guard against the renderer only decoding
		// the first document in a multi-document template file.
		wantNames := map[string]bool{
			npName:                                  false, // clusterlifecycle-state-metrics-network-policy
			"cluster-curator-controller-policy":     false,
			"clusterclaims-controller-policy":       false,
			"cluster-image-set-controller-policy":   false,
			"provider-credential-controller-policy": false,
		}
		var np *unstructured.Unstructured
		for _, tmpl := range templates {
			if tmpl.GetKind() != "NetworkPolicy" {
				continue
			}
			name := tmpl.GetName()
			if _, ok := wantNames[name]; ok {
				wantNames[name] = true
			}
			if name == npName {
				np = tmpl
			}
		}
		for name, found := range wantNames {
			if !found {
				t.Errorf("expected NetworkPolicy %s when networkPolicies.enabled=true", name)
			}
		}
		if np == nil {
			t.Fatalf("expected NetworkPolicy %s when networkPolicies.enabled=true", npName)
		}

		podSelector, found, err := unstructured.NestedStringMap(np.Object, "spec", "podSelector", "matchLabels")
		if err != nil || !found {
			t.Fatalf("podSelector NestedStringMap: found=%v err=%v", found, err)
		}
		if podSelector["app"] != "clusterlifecycle-state-metrics-v2" {
			t.Errorf("unexpected podSelector: %v", podSelector)
		}

		ingress, found, err := unstructured.NestedSlice(np.Object, "spec", "ingress")
		if err != nil || !found || len(ingress) == 0 {
			t.Fatalf("expected ingress rules, found=%v err=%v len=%d", found, err, len(ingress))
		}
		rule, ok := ingress[0].(map[string]interface{})
		if !ok {
			t.Fatal("ingress[0] is not a map")
		}
		from, found, err := unstructured.NestedSlice(rule, "from")
		if err != nil || !found || len(from) == 0 {
			t.Fatalf("expected ingress from openshift-monitoring, found=%v err=%v", found, err)
		}
		peer, ok := from[0].(map[string]interface{})
		if !ok {
			t.Fatal("ingress from[0] is not a map")
		}
		nsLabels, found, err := unstructured.NestedStringMap(peer, "namespaceSelector", "matchLabels")
		if err != nil || !found {
			t.Fatalf("namespaceSelector NestedStringMap: found=%v err=%v", found, err)
		}
		if nsLabels["kubernetes.io/metadata.name"] != "openshift-monitoring" {
			t.Errorf("unexpected ingress namespaceSelector: %v", nsLabels)
		}
	})

	t.Run("NetworkPolicies disabled excludes CLSM NP", func(t *testing.T) {
		mce := &backplane.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-mce"},
			Spec: backplane.MultiClusterEngineSpec{
				TargetNamespace: "default",
				NetworkPolicies: &backplane.NetworkPoliciesConfig{Enabled: false},
			},
		}

		templates, errs := RenderChart(clcChart, mce, testImages, map[string]string{})
		if len(errs) > 0 {
			t.Fatalf("RenderChart failed: %v", errs)
		}

		for _, tmpl := range templates {
			if tmpl.GetKind() == "NetworkPolicy" {
				t.Errorf("NetworkPolicy should not be rendered when networkPolicies.enabled=false: %s", tmpl.GetName())
			}
		}
	})
}

func TestManagedServiceAccountNetworkPolicies(t *testing.T) {
	os.Setenv("DIRECTORY_OVERRIDE", "../../")
	defer os.Unsetenv("DIRECTORY_OVERRIDE")
	os.Setenv("POD_NAMESPACE", "default")
	defer os.Unsetenv("POD_NAMESPACE")
	os.Setenv("ACM_HUB_OCP_VERSION", "4.12.0")
	defer os.Unsetenv("ACM_HUB_OCP_VERSION")

	testImages := map[string]string{}
	for _, v := range utils.GetTestImages() {
		testImages[v] = "quay.io/test/test:Test"
	}

	addonTemplateContainsAgentNP := func(templates []*unstructured.Unstructured) bool {
		for _, tmpl := range templates {
			if tmpl.GetKind() != "AddOnTemplate" {
				continue
			}
			manifests, found, err := unstructured.NestedSlice(tmpl.Object, "spec", "agentSpec", "workload", "manifests")
			if err != nil || !found {
				continue
			}
			for _, m := range manifests {
				manifest, ok := m.(map[string]interface{})
				if !ok {
					continue
				}
				if manifest["kind"] == "NetworkPolicy" {
					meta, _ := manifest["metadata"].(map[string]interface{})
					if meta != nil && meta["name"] == "managed-serviceaccount-addon-agent-network-policy" {
						return true
					}
				}
			}
		}
		return false
	}

	t.Run("NetworkPolicies enabled embeds agent NP in AddOnTemplate", func(t *testing.T) {
		mce := &backplane.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-mce"},
			Spec: backplane.MultiClusterEngineSpec{
				TargetNamespace: "default",
				NetworkPolicies: &backplane.NetworkPoliciesConfig{Enabled: true},
			},
		}

		templates, errs := RenderChart(chartsPath, mce, testImages, map[string]string{})
		if len(errs) > 0 {
			t.Fatalf("RenderChart failed: %v", errs)
		}
		if !addonTemplateContainsAgentNP(templates) {
			t.Error("expected managed-serviceaccount-addon-agent-network-policy in AddOnTemplate when networkPolicies.enabled=true")
		}
		// Spoke NP is nested in AddOnTemplate; ensure it is not applied as a hub NetworkPolicy.
		for _, tmpl := range templates {
			if tmpl.GetKind() == "NetworkPolicy" {
				t.Errorf("unexpected top-level NetworkPolicy %s (MSA has no hub manager Deployment)", tmpl.GetName())
			}
		}
	})

	t.Run("NetworkPolicies disabled omits agent NP from AddOnTemplate", func(t *testing.T) {
		mce := &backplane.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-mce"},
			Spec: backplane.MultiClusterEngineSpec{
				TargetNamespace: "default",
				NetworkPolicies: &backplane.NetworkPoliciesConfig{Enabled: false},
			},
		}

		templates, errs := RenderChart(chartsPath, mce, testImages, map[string]string{})
		if len(errs) > 0 {
			t.Fatalf("RenderChart failed: %v", errs)
		}
		if addonTemplateContainsAgentNP(templates) {
			t.Error("NetworkPolicy should not be embedded in AddOnTemplate when networkPolicies.enabled=false")
		}
	})
}

func TestServerFoundationNetworkPolicies(t *testing.T) {
	os.Setenv("DIRECTORY_OVERRIDE", "../../")
	defer os.Unsetenv("DIRECTORY_OVERRIDE")
	os.Setenv("POD_NAMESPACE", "default")
	defer os.Unsetenv("POD_NAMESPACE")
	os.Setenv("ACM_HUB_OCP_VERSION", "4.12.0")
	defer os.Unsetenv("ACM_HUB_OCP_VERSION")
	// ocm-proxyserver (and its NetworkPolicy) are OCP-only; pin deployOnOCP so
	// earlier tests that flip the package-level flag cannot flake this assertion.
	originalDeployOnOCP := utils.DeployOnOCP()
	defer utils.SetDeployOnOCP(originalDeployOnOCP)
	utils.SetDeployOnOCP(true)

	testImages := map[string]string{}
	for _, v := range utils.GetTestImages() {
		testImages[v] = "quay.io/test/test:Test"
	}

	sfChart := "pkg/templates/charts/toggle/server-foundation"
	expectedNPNames := map[string]bool{
		"managedcluster-import-controller-network-policy": false,
		"ocm-controller-network-policy":                   false,
		"ocm-webhook-network-policy":                      false,
		"ocm-proxyserver-network-policy":                  false,
	}

	t.Run("NetworkPolicies enabled renders hub NPs and controller flag", func(t *testing.T) {
		mce := &backplane.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-mce"},
			Spec: backplane.MultiClusterEngineSpec{
				TargetNamespace: "default",
				NetworkPolicies: &backplane.NetworkPoliciesConfig{Enabled: true},
			},
		}

		templates, errs := RenderChart(sfChart, mce, testImages, map[string]string{})
		if len(errs) > 0 {
			t.Fatalf("RenderChart failed: %v", errs)
		}

		found := map[string]bool{}
		for name := range expectedNPNames {
			found[name] = false
		}
		ocmControllerFlag := false
		importControllerFlag := false
		for _, tmpl := range templates {
			if tmpl.GetKind() == "NetworkPolicy" {
				if _, ok := found[tmpl.GetName()]; ok {
					found[tmpl.GetName()] = true
					assertServerFoundationNetworkPolicy(t, tmpl)
				} else {
					t.Errorf("unexpected NetworkPolicy name: %s", tmpl.GetName())
				}
			}
			if tmpl.GetKind() == "Deployment" && tmpl.GetName() == "ocm-controller" {
				containers, foundContainers, err := unstructured.NestedSlice(tmpl.Object, "spec", "template", "spec", "containers")
				if err != nil {
					t.Fatalf("ocm-controller containers NestedSlice: %v", err)
				}
				if !foundContainers {
					t.Fatal("ocm-controller missing spec.template.spec.containers")
				}
				for i, c := range containers {
					container, ok := c.(map[string]interface{})
					if !ok {
						t.Fatalf("ocm-controller containers[%d] is not a map", i)
					}
					args, foundArgs, err := unstructured.NestedStringSlice(container, "args")
					if err != nil {
						t.Fatalf("ocm-controller containers[%d] args NestedStringSlice: %v", i, err)
					}
					if !foundArgs {
						t.Fatalf("ocm-controller containers[%d] missing args", i)
					}
					for _, arg := range args {
						if arg == "--enable-network-policies=true" {
							ocmControllerFlag = true
						}
					}
				}
			}
			if tmpl.GetKind() == "Deployment" && tmpl.GetName() == "managedcluster-import-controller-v2" {
				containers, foundContainers, err := unstructured.NestedSlice(tmpl.Object, "spec", "template", "spec", "containers")
				if err != nil {
					t.Fatalf("managedcluster-import-controller-v2 containers NestedSlice: %v", err)
				}
				if !foundContainers {
					t.Fatal("managedcluster-import-controller-v2 missing spec.template.spec.containers")
				}
				for i, c := range containers {
					container, ok := c.(map[string]interface{})
					if !ok {
						t.Fatalf("managedcluster-import-controller-v2 containers[%d] is not a map", i)
					}
					args, foundArgs, err := unstructured.NestedStringSlice(container, "args")
					if err != nil {
						t.Fatalf("managedcluster-import-controller-v2 containers[%d] args NestedStringSlice: %v", i, err)
					}
					if !foundArgs {
						t.Fatalf("managedcluster-import-controller-v2 containers[%d] missing args", i)
					}
					for _, arg := range args {
						if arg == "--enable-klusterlet-network-policies=true" {
							importControllerFlag = true
						}
					}
				}
			}
		}
		for name, ok := range found {
			if !ok {
				t.Errorf("expected NetworkPolicy %s when networkPolicies.enabled=true", name)
			}
		}
		if !ocmControllerFlag {
			t.Error("expected ocm-controller --enable-network-policies=true when networkPolicies.enabled=true")
		}
		if !importControllerFlag {
			t.Error("expected managedcluster-import-controller-v2 --enable-klusterlet-network-policies=true when networkPolicies.enabled=true")
		}
	})

	t.Run("NetworkPolicies disabled excludes hub NPs and sets flag false", func(t *testing.T) {
		mce := &backplane.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-mce"},
			Spec: backplane.MultiClusterEngineSpec{
				TargetNamespace: "default",
				NetworkPolicies: &backplane.NetworkPoliciesConfig{Enabled: false},
			},
		}

		templates, errs := RenderChart(sfChart, mce, testImages, map[string]string{})
		if len(errs) > 0 {
			t.Fatalf("RenderChart failed: %v", errs)
		}

		foundOCMController := false
		foundDisabledFlag := false
		foundImportController := false
		foundImportDisabledFlag := false
		for _, tmpl := range templates {
			if tmpl.GetKind() == "NetworkPolicy" {
				t.Errorf("NetworkPolicy should not be rendered when networkPolicies.enabled=false: %s", tmpl.GetName())
			}
			if tmpl.GetKind() == "Deployment" && tmpl.GetName() == "ocm-controller" {
				foundOCMController = true
				containers, foundContainers, err := unstructured.NestedSlice(tmpl.Object, "spec", "template", "spec", "containers")
				if err != nil {
					t.Fatalf("ocm-controller containers NestedSlice: %v", err)
				}
				if !foundContainers {
					t.Fatal("ocm-controller missing spec.template.spec.containers")
				}
				for i, c := range containers {
					container, ok := c.(map[string]interface{})
					if !ok {
						t.Fatalf("ocm-controller containers[%d] is not a map", i)
					}
					args, foundArgs, err := unstructured.NestedStringSlice(container, "args")
					if err != nil {
						t.Fatalf("ocm-controller containers[%d] args NestedStringSlice: %v", i, err)
					}
					if !foundArgs {
						t.Fatalf("ocm-controller containers[%d] missing args", i)
					}
					for _, arg := range args {
						if arg == "--enable-network-policies=true" {
							t.Error("ocm-controller should not have --enable-network-policies=true when disabled")
						}
						if arg == "--enable-network-policies=false" {
							foundDisabledFlag = true
						}
					}
				}
			}
			if tmpl.GetKind() == "Deployment" && tmpl.GetName() == "managedcluster-import-controller-v2" {
				foundImportController = true
				containers, foundContainers, err := unstructured.NestedSlice(tmpl.Object, "spec", "template", "spec", "containers")
				if err != nil {
					t.Fatalf("managedcluster-import-controller-v2 containers NestedSlice: %v", err)
				}
				if !foundContainers {
					t.Fatal("managedcluster-import-controller-v2 missing spec.template.spec.containers")
				}
				for i, c := range containers {
					container, ok := c.(map[string]interface{})
					if !ok {
						t.Fatalf("managedcluster-import-controller-v2 containers[%d] is not a map", i)
					}
					args, foundArgs, err := unstructured.NestedStringSlice(container, "args")
					if err != nil {
						t.Fatalf("managedcluster-import-controller-v2 containers[%d] args NestedStringSlice: %v", i, err)
					}
					if !foundArgs {
						t.Fatalf("managedcluster-import-controller-v2 containers[%d] missing args", i)
					}
					for _, arg := range args {
						if arg == "--enable-klusterlet-network-policies=true" {
							t.Error("managedcluster-import-controller-v2 should not have --enable-klusterlet-network-policies=true when disabled")
						}
						if arg == "--enable-klusterlet-network-policies=false" {
							foundImportDisabledFlag = true
						}
					}
				}
			}
		}
		if !foundOCMController {
			t.Error("expected ocm-controller Deployment when rendering server-foundation chart")
		}
		if !foundDisabledFlag {
			t.Error("expected ocm-controller --enable-network-policies=false when networkPolicies.enabled=false")
		}
		if !foundImportController {
			t.Error("expected managedcluster-import-controller-v2 Deployment when rendering server-foundation chart")
		}
		if !foundImportDisabledFlag {
			t.Error("expected managedcluster-import-controller-v2 --enable-klusterlet-network-policies=false when networkPolicies.enabled=false")
		}
	})

	t.Run("non-OCP omits proxyserver Deployment and NetworkPolicy", func(t *testing.T) {
		utils.SetDeployOnOCP(false)
		defer utils.SetDeployOnOCP(true)

		mce := &backplane.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-mce"},
			Spec: backplane.MultiClusterEngineSpec{
				TargetNamespace: "default",
				NetworkPolicies: &backplane.NetworkPoliciesConfig{Enabled: true},
			},
		}

		templates, errs := RenderChart(sfChart, mce, testImages, map[string]string{})
		if len(errs) > 0 {
			t.Fatalf("RenderChart failed: %v", errs)
		}

		expectedNonOCP := map[string]bool{
			"managedcluster-import-controller-network-policy": false,
			"ocm-controller-network-policy":                   false,
			"ocm-webhook-network-policy":                      false,
		}
		importControllerFlag := false
		for _, tmpl := range templates {
			if tmpl.GetKind() == "Deployment" && tmpl.GetName() == "ocm-proxyserver" {
				t.Error("ocm-proxyserver Deployment should not render when deployOnOCP=false")
			}
			if tmpl.GetKind() == "Deployment" && tmpl.GetName() == "managedcluster-import-controller-v2" {
				containers, foundContainers, err := unstructured.NestedSlice(tmpl.Object, "spec", "template", "spec", "containers")
				if err != nil {
					t.Fatalf("managedcluster-import-controller-v2 containers NestedSlice: %v", err)
				}
				if !foundContainers {
					t.Fatal("managedcluster-import-controller-v2 missing spec.template.spec.containers")
				}
				for i, c := range containers {
					container, ok := c.(map[string]interface{})
					if !ok {
						t.Fatalf("managedcluster-import-controller-v2 containers[%d] is not a map", i)
					}
					args, foundArgs, err := unstructured.NestedStringSlice(container, "args")
					if err != nil {
						t.Fatalf("managedcluster-import-controller-v2 containers[%d] args NestedStringSlice: %v", i, err)
					}
					if !foundArgs {
						t.Fatalf("managedcluster-import-controller-v2 containers[%d] missing args", i)
					}
					for _, arg := range args {
						if arg == "--enable-klusterlet-network-policies=true" {
							importControllerFlag = true
						}
					}
				}
			}
			if tmpl.GetKind() != "NetworkPolicy" {
				continue
			}
			if tmpl.GetName() == "ocm-proxyserver-network-policy" {
				t.Error("ocm-proxyserver-network-policy should not render when deployOnOCP=false")
				continue
			}
			if _, ok := expectedNonOCP[tmpl.GetName()]; ok {
				expectedNonOCP[tmpl.GetName()] = true
				assertServerFoundationNetworkPolicy(t, tmpl)
			} else {
				t.Errorf("unexpected NetworkPolicy name: %s", tmpl.GetName())
			}
		}
		for name, ok := range expectedNonOCP {
			if !ok {
				t.Errorf("expected NetworkPolicy %s when networkPolicies.enabled=true on non-OCP", name)
			}
		}
		if !importControllerFlag {
			t.Error("expected managedcluster-import-controller-v2 --enable-klusterlet-network-policies=true on non-OCP path")
		}
	})

	t.Run("non-OCP sets klusterlet network policies flag false when disabled", func(t *testing.T) {
		utils.SetDeployOnOCP(false)
		defer utils.SetDeployOnOCP(true)

		mce := &backplane.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-mce"},
			Spec: backplane.MultiClusterEngineSpec{
				TargetNamespace: "default",
				NetworkPolicies: &backplane.NetworkPoliciesConfig{Enabled: false},
			},
		}

		templates, errs := RenderChart(sfChart, mce, testImages, map[string]string{})
		if len(errs) > 0 {
			t.Fatalf("RenderChart failed: %v", errs)
		}

		foundImportController := false
		foundDisabledFlag := false
		for _, tmpl := range templates {
			if tmpl.GetKind() != "Deployment" || tmpl.GetName() != "managedcluster-import-controller-v2" {
				continue
			}
			foundImportController = true
			containers, found, err := unstructured.NestedSlice(tmpl.Object, "spec", "template", "spec", "containers")
			if err != nil || !found {
				t.Fatalf("managedcluster-import-controller-v2 missing containers: %v", err)
			}
			for i, c := range containers {
				container, ok := c.(map[string]interface{})
				if !ok {
					t.Fatalf("managedcluster-import-controller-v2 containers[%d] is not a map", i)
				}
				args, foundArgs, err := unstructured.NestedStringSlice(container, "args")
				if err != nil || !foundArgs {
					t.Fatalf("managedcluster-import-controller-v2 containers[%d] missing args: %v", i, err)
				}
				for _, arg := range args {
					if arg == "--enable-klusterlet-network-policies=true" {
						t.Error("managedcluster-import-controller-v2 should not have --enable-klusterlet-network-policies=true when disabled on non-OCP")
					}
					if arg == "--enable-klusterlet-network-policies=false" {
						foundDisabledFlag = true
					}
				}
			}
		}
		if !foundImportController {
			t.Error("expected managedcluster-import-controller-v2 Deployment on non-OCP path")
		}
		if !foundDisabledFlag {
			t.Error("expected managedcluster-import-controller-v2 --enable-klusterlet-network-policies=false on non-OCP path when disabled")
		}
	})

	hostingChart := "pkg/templates/charts/hosting/server-foundation"

	t.Run("Hosting chart passes ENABLE_KLUSTERLET_NETWORK_POLICIES env var when enabled", func(t *testing.T) {
		mce := &backplane.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-mce"},
			Spec: backplane.MultiClusterEngineSpec{
				TargetNamespace: "default",
				NetworkPolicies: &backplane.NetworkPoliciesConfig{Enabled: true},
			},
		}

		templates, errs := RenderChart(hostingChart, mce, testImages, map[string]string{})
		if len(errs) > 0 {
			t.Fatalf("RenderChart failed: %v", errs)
		}

		foundEnvVar := false
		for _, tmpl := range templates {
			if tmpl.GetKind() != "Deployment" || tmpl.GetName() != "managedcluster-import-controller-v2" {
				continue
			}
			containers, found, err := unstructured.NestedSlice(tmpl.Object, "spec", "template", "spec", "containers")
			if err != nil || !found {
				t.Fatalf("hosting import controller missing containers: %v", err)
			}
			for i, c := range containers {
				container, ok := c.(map[string]interface{})
				if !ok {
					t.Fatalf("hosting import controller containers[%d] is not a map", i)
				}
				envList, found, err := unstructured.NestedSlice(container, "env")
				if err != nil || !found {
					t.Fatalf("hosting import controller missing env: %v", err)
				}
				for j, e := range envList {
					env, ok := e.(map[string]interface{})
					if !ok {
						t.Fatalf("hosting import controller env[%d] is not a map", j)
					}
					if env["name"] == "ENABLE_KLUSTERLET_NETWORK_POLICIES" && env["value"] == "true" {
						foundEnvVar = true
					}
				}
			}
		}
		if !foundEnvVar {
			t.Error("expected ENABLE_KLUSTERLET_NETWORK_POLICIES=true env var in hosting import controller when networkPolicies.enabled=true")
		}
	})

	t.Run("Hosting chart passes ENABLE_KLUSTERLET_NETWORK_POLICIES env var when disabled", func(t *testing.T) {
		mce := &backplane.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-mce"},
			Spec: backplane.MultiClusterEngineSpec{
				TargetNamespace: "default",
				NetworkPolicies: &backplane.NetworkPoliciesConfig{Enabled: false},
			},
		}

		templates, errs := RenderChart(hostingChart, mce, testImages, map[string]string{})
		if len(errs) > 0 {
			t.Fatalf("RenderChart failed: %v", errs)
		}

		foundEnvVar := false
		for _, tmpl := range templates {
			if tmpl.GetKind() != "Deployment" || tmpl.GetName() != "managedcluster-import-controller-v2" {
				continue
			}
			containers, found, err := unstructured.NestedSlice(tmpl.Object, "spec", "template", "spec", "containers")
			if err != nil || !found {
				t.Fatalf("hosting import controller missing containers: %v", err)
			}
			for i, c := range containers {
				container, ok := c.(map[string]interface{})
				if !ok {
					t.Fatalf("hosting import controller containers[%d] is not a map", i)
				}
				envList, found, err := unstructured.NestedSlice(container, "env")
				if err != nil || !found {
					t.Fatalf("hosting import controller missing env: %v", err)
				}
				for j, e := range envList {
					env, ok := e.(map[string]interface{})
					if !ok {
						t.Fatalf("hosting import controller env[%d] is not a map", j)
					}
					if env["name"] == "ENABLE_KLUSTERLET_NETWORK_POLICIES" {
						if env["value"] == "true" {
							t.Error("hosting import controller should not have ENABLE_KLUSTERLET_NETWORK_POLICIES=true when disabled")
						}
						if env["value"] == "false" {
							foundEnvVar = true
						}
					}
				}
			}
		}
		if !foundEnvVar {
			t.Error("expected ENABLE_KLUSTERLET_NETWORK_POLICIES=false env var in hosting import controller when networkPolicies.enabled=false")
		}
	})
}

func TestClusterManagerNetworkPolicies(t *testing.T) {
	os.Setenv("DIRECTORY_OVERRIDE", "../../")
	defer os.Unsetenv("DIRECTORY_OVERRIDE")
	os.Setenv("POD_NAMESPACE", "default")
	defer os.Unsetenv("POD_NAMESPACE")
	os.Setenv("ACM_HUB_OCP_VERSION", "4.12.0")
	defer os.Unsetenv("ACM_HUB_OCP_VERSION")

	testImages := map[string]string{}
	for _, v := range utils.GetTestImages() {
		testImages[v] = "quay.io/test/test:Test"
	}

	cmChart := "pkg/templates/charts/toggle/cluster-manager"
	const npName = "cluster-manager-network-policy"

	t.Run("NetworkPolicies enabled renders registration-operator NP", func(t *testing.T) {
		mce := &backplane.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-mce"},
			Spec: backplane.MultiClusterEngineSpec{
				TargetNamespace: "multicluster-engine",
				NetworkPolicies: &backplane.NetworkPoliciesConfig{Enabled: true},
			},
		}

		templates, errs := RenderChart(cmChart, mce, testImages, map[string]string{})
		if len(errs) > 0 {
			t.Fatalf("RenderChart failed: %v", errs)
		}

		var np *unstructured.Unstructured
		for _, tmpl := range templates {
			if tmpl.GetKind() == "NetworkPolicy" {
				if tmpl.GetName() != npName {
					t.Errorf("unexpected NetworkPolicy name: %s", tmpl.GetName())
					continue
				}
				np = tmpl
			}
		}
		if np == nil {
			t.Fatalf("expected NetworkPolicy %s when networkPolicies.enabled=true", npName)
		}
		if np.GetNamespace() != "multicluster-engine" {
			t.Errorf("NetworkPolicy namespace = %s, want multicluster-engine", np.GetNamespace())
		}
		assertServerFoundationNetworkPolicy(t, np)
	})

	t.Run("NetworkPolicies disabled excludes registration-operator NP", func(t *testing.T) {
		mce := &backplane.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-mce"},
			Spec: backplane.MultiClusterEngineSpec{
				TargetNamespace: "multicluster-engine",
				NetworkPolicies: &backplane.NetworkPoliciesConfig{Enabled: false},
			},
		}

		templates, errs := RenderChart(cmChart, mce, testImages, map[string]string{})
		if len(errs) > 0 {
			t.Fatalf("RenderChart failed: %v", errs)
		}

		for _, tmpl := range templates {
			if tmpl.GetKind() == "NetworkPolicy" {
				t.Errorf("NetworkPolicy should not be rendered when networkPolicies.enabled=false: %s", tmpl.GetName())
			}
		}
	})
}

func npPort(proto corev1.Protocol, port int32) networkingv1.NetworkPolicyPort {
	p := port
	protocol := proto
	return networkingv1.NetworkPolicyPort{
		Protocol: &protocol,
		Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: p},
	}
}

func sfDNSEgress() networkingv1.NetworkPolicyEgressRule {
	return networkingv1.NetworkPolicyEgressRule{
		Ports: []networkingv1.NetworkPolicyPort{
			npPort(corev1.ProtocolUDP, 53),
			npPort(corev1.ProtocolTCP, 53),
			npPort(corev1.ProtocolUDP, 5353),
			npPort(corev1.ProtocolTCP, 5353),
		},
	}
}

func sfAPIEgress() networkingv1.NetworkPolicyEgressRule {
	return networkingv1.NetworkPolicyEgressRule{
		Ports: []networkingv1.NetworkPolicyPort{
			npPort(corev1.ProtocolTCP, 443),
			npPort(corev1.ProtocolTCP, 6443),
		},
	}
}

func expectedServerFoundationNetworkPolicySpec(name string) (networkingv1.NetworkPolicySpec, bool) {
	defaultDenyTypes := []networkingv1.PolicyType{
		networkingv1.PolicyTypeIngress,
		networkingv1.PolicyTypeEgress,
	}
	switch name {
	case "managedcluster-import-controller-network-policy":
		return networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{
				"app": "managedcluster-import-controller-v2",
			}},
			PolicyTypes: defaultDenyTypes,
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{{
						NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
							"policy-group.network.openshift.io/ingress": "",
						}},
						PodSelector: &metav1.LabelSelector{},
					}},
					Ports: []networkingv1.NetworkPolicyPort{npPort(corev1.ProtocolTCP, 9091)},
				},
				{Ports: []networkingv1.NetworkPolicyPort{npPort(corev1.ProtocolTCP, 8383)}},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{sfDNSEgress(), sfAPIEgress()},
		}, true
	case "ocm-controller-network-policy":
		return networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{
				"control-plane": "ocm-controller",
			}},
			PolicyTypes: defaultDenyTypes,
			Egress:      []networkingv1.NetworkPolicyEgressRule{sfDNSEgress(), sfAPIEgress()},
		}, true
	case "ocm-webhook-network-policy":
		return networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{
				"control-plane": "ocm-webhook",
			}},
			PolicyTypes: defaultDenyTypes,
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{Ports: []networkingv1.NetworkPolicyPort{npPort(corev1.ProtocolTCP, 8000)}},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{sfDNSEgress(), sfAPIEgress()},
		}, true
	case "ocm-proxyserver-network-policy":
		return networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{
				"control-plane": "ocm-proxyserver",
			}},
			PolicyTypes: defaultDenyTypes,
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{Ports: []networkingv1.NetworkPolicyPort{npPort(corev1.ProtocolTCP, 6443)}},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				sfDNSEgress(),
				sfAPIEgress(),
				{
					To: []networkingv1.NetworkPolicyPeer{{
						PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
							"component": "cluster-proxy-addon-user",
						}},
					}},
					Ports: []networkingv1.NetworkPolicyPort{npPort(corev1.ProtocolTCP, 9092)},
				},
				{
					Ports: []networkingv1.NetworkPolicyPort{npPort(corev1.ProtocolTCP, 3010)},
				},
			},
		}, true
	case "cluster-manager-network-policy":
		return networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{
				"app": "cluster-manager",
			}},
			PolicyTypes: defaultDenyTypes,
			Egress:      []networkingv1.NetworkPolicyEgressRule{sfDNSEgress(), sfAPIEgress()},
		}, true
	default:
		return networkingv1.NetworkPolicySpec{}, false
	}
}

// assertServerFoundationNetworkPolicy checks podSelector, default-deny policyTypes,
// and exact ingress/egress peers/ports. Also rejects allow-all rules and health-probe
// ports that must stay closed (ocm-controller :8000).
func assertServerFoundationNetworkPolicy(t *testing.T, tmpl *unstructured.Unstructured) {
	t.Helper()
	expected, ok := expectedServerFoundationNetworkPolicySpec(tmpl.GetName())
	if !ok {
		t.Errorf("no expected NetworkPolicy spec for %s", tmpl.GetName())
		return
	}

	var np networkingv1.NetworkPolicy
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(tmpl.Object, &np); err != nil {
		t.Fatalf("convert NetworkPolicy %s: %v", tmpl.GetName(), err)
	}

	if !reflect.DeepEqual(np.Spec.PodSelector, expected.PodSelector) {
		t.Errorf("%s podSelector = %#v, want %#v", tmpl.GetName(), np.Spec.PodSelector, expected.PodSelector)
	}
	if !reflect.DeepEqual(np.Spec.PolicyTypes, expected.PolicyTypes) {
		t.Errorf("%s policyTypes = %#v, want %#v", tmpl.GetName(), np.Spec.PolicyTypes, expected.PolicyTypes)
	}
	if !reflect.DeepEqual(np.Spec.Ingress, expected.Ingress) {
		t.Errorf("%s ingress = %#v, want %#v", tmpl.GetName(), np.Spec.Ingress, expected.Ingress)
	}
	if !reflect.DeepEqual(np.Spec.Egress, expected.Egress) {
		t.Errorf("%s egress = %#v, want %#v", tmpl.GetName(), np.Spec.Egress, expected.Egress)
	}

	for i, rule := range np.Spec.Ingress {
		if len(rule.Ports) == 0 {
			t.Errorf("%s ingress[%d] has no ports (overly broad allow-all ingress)", tmpl.GetName(), i)
		}
	}
	for i, rule := range np.Spec.Egress {
		if len(rule.Ports) == 0 {
			t.Errorf("%s egress[%d] has no ports (overly broad allow-all egress)", tmpl.GetName(), i)
		}
	}

	// ocm-controller serves health on :8000; default-deny ingress must not open that port.
	if tmpl.GetName() == "ocm-controller-network-policy" {
		for i, rule := range np.Spec.Ingress {
			for _, p := range rule.Ports {
				if p.Port != nil && p.Port.IntVal == 8000 {
					t.Errorf("%s ingress[%d] exposes health-probe port 8000", tmpl.GetName(), i)
				}
			}
		}
	}
}

func TestRenderChartInvalidPath(t *testing.T) {
	os.Setenv("DIRECTORY_OVERRIDE", "../../")
	defer os.Unsetenv("DIRECTORY_OVERRIDE")

	mce := &backplane.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mce"},
		Spec: backplane.MultiClusterEngineSpec{
			TargetNamespace: "default",
		},
	}

	_, errs := RenderChart("pkg/templates/charts/toggle/nonexistent", mce, map[string]string{}, map[string]string{})
	if len(errs) == 0 {
		t.Error("expected error for invalid chart path")
	}
}

func TestParseProbeConfigFromAnnotations(t *testing.T) {
	t.Run("No annotations returns nil", func(t *testing.T) {
		mce := &backplane.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-mce",
			},
		}

		result := parseProbeConfigFromAnnotations(mce)
		if result != nil {
			t.Error("Expected nil when no annotations present")
		}
	})

	t.Run("All three annotations are parsed correctly", func(t *testing.T) {
		mce := &backplane.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-mce",
				Annotations: map[string]string{
					utils.AnnotationProbeTimeoutSeconds:   "10",
					utils.AnnotationProbeFailureThreshold: "5",
					utils.AnnotationProbeSuccessThreshold: "2",
				},
			},
		}

		result := parseProbeConfigFromAnnotations(mce)
		if result == nil {
			t.Fatal("Expected ProbeConfig, got nil")
		}

		if result.TimeoutSeconds == nil || *result.TimeoutSeconds != 10 {
			t.Errorf("Expected TimeoutSeconds=10, got %v", result.TimeoutSeconds)
		}
		if result.FailureThreshold == nil || *result.FailureThreshold != 5 {
			t.Errorf("Expected FailureThreshold=5, got %v", result.FailureThreshold)
		}
		if result.SuccessThreshold == nil || *result.SuccessThreshold != 2 {
			t.Errorf("Expected SuccessThreshold=2, got %v", result.SuccessThreshold)
		}
	})

	t.Run("Partial annotations are parsed correctly", func(t *testing.T) {
		mce := &backplane.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-mce",
				Annotations: map[string]string{
					utils.AnnotationProbeTimeoutSeconds: "15",
				},
			},
		}

		result := parseProbeConfigFromAnnotations(mce)
		if result == nil {
			t.Fatal("Expected ProbeConfig, got nil")
		}

		if result.TimeoutSeconds == nil || *result.TimeoutSeconds != 15 {
			t.Errorf("Expected TimeoutSeconds=15, got %v", result.TimeoutSeconds)
		}
		if result.FailureThreshold != nil {
			t.Errorf("Expected FailureThreshold=nil, got %v", *result.FailureThreshold)
		}
		if result.SuccessThreshold != nil {
			t.Errorf("Expected SuccessThreshold=nil, got %v", *result.SuccessThreshold)
		}
	})

	t.Run("Invalid values are ignored", func(t *testing.T) {
		mce := &backplane.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-mce",
				Annotations: map[string]string{
					utils.AnnotationProbeTimeoutSeconds:   "not-a-number",
					utils.AnnotationProbeFailureThreshold: "10",
				},
			},
		}

		result := parseProbeConfigFromAnnotations(mce)
		if result == nil {
			t.Fatal("Expected ProbeConfig, got nil")
		}

		if result.TimeoutSeconds != nil {
			t.Errorf("Expected TimeoutSeconds=nil (invalid value), got %v", *result.TimeoutSeconds)
		}
		if result.FailureThreshold == nil || *result.FailureThreshold != 10 {
			t.Errorf("Expected FailureThreshold=10, got %v", result.FailureThreshold)
		}
	})

	t.Run("Zero and negative values are ignored", func(t *testing.T) {
		mce := &backplane.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-mce",
				Annotations: map[string]string{
					utils.AnnotationProbeTimeoutSeconds:   "0",
					utils.AnnotationProbeFailureThreshold: "-5",
					utils.AnnotationProbeSuccessThreshold: "3",
				},
			},
		}

		result := parseProbeConfigFromAnnotations(mce)
		if result == nil {
			t.Fatal("Expected ProbeConfig, got nil")
		}

		if result.TimeoutSeconds != nil {
			t.Errorf("Expected TimeoutSeconds=nil (zero value), got %v", *result.TimeoutSeconds)
		}
		if result.FailureThreshold != nil {
			t.Errorf("Expected FailureThreshold=nil (negative value), got %v", *result.FailureThreshold)
		}
		if result.SuccessThreshold == nil || *result.SuccessThreshold != 3 {
			t.Errorf("Expected SuccessThreshold=3, got %v", result.SuccessThreshold)
		}
	})

	t.Run("Other annotations don't interfere", func(t *testing.T) {
		mce := &backplane.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-mce",
				Annotations: map[string]string{
					"installer.multicluster.openshift.io/pause": "true",
					utils.AnnotationProbeTimeoutSeconds:         "20",
					"some-other-annotation":                     "value",
				},
			},
		}

		result := parseProbeConfigFromAnnotations(mce)
		if result == nil {
			t.Fatal("Expected ProbeConfig, got nil")
		}

		if result.TimeoutSeconds == nil || *result.TimeoutSeconds != 20 {
			t.Errorf("Expected TimeoutSeconds=20, got %v", result.TimeoutSeconds)
		}
	})
}

func TestEnsureTLSProfileConfigMaps(t *testing.T) {
	os.Setenv("UNIT_TEST", "true")
	defer os.Unsetenv("UNIT_TEST")

	tests := []struct {
		name       string
		namespaces []string
		wantErr    bool
	}{
		{
			name:       "single namespace",
			namespaces: []string{"test-namespace"},
			wantErr:    false,
		},
		{
			name:       "multiple namespaces",
			namespaces: []string{"namespace1", "namespace2", "namespace3"},
			wantErr:    false,
		},
		{
			name:       "empty namespace list",
			namespaces: []string{},
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up scheme
			s := runtime.NewScheme()
			_ = scheme.AddToScheme(s)
			_ = backplane.AddToScheme(s)

			// Create test MCE object
			mce := &backplane.MultiClusterEngine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-mce",
					UID:  "test-uid-12345",
				},
				Spec: backplane.MultiClusterEngineSpec{
					TargetNamespace: "multicluster-engine",
				},
			}

			// Create namespaces
			var initObjs []client.Object
			initObjs = append(initObjs, mce)
			for _, ns := range tt.namespaces {
				initObjs = append(initObjs, &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: ns,
					},
				})
			}

			// Create fake client
			fakeClient := fake.NewClientBuilder().
				WithScheme(s).
				WithObjects(initObjs...).
				Build()

			ctx := context.Background()

			// Call function under test
			err := EnsureTLSProfileConfigMaps(ctx, fakeClient, mce, tt.namespaces, s)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("EnsureTLSProfileConfigMaps() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			// Verify ConfigMaps were created in each namespace
			for _, ns := range tt.namespaces {
				cm := &corev1.ConfigMap{}
				err := fakeClient.Get(ctx, types.NamespacedName{
					Name:      "ocm-tls-profile",
					Namespace: ns,
				}, cm)

				if err != nil {
					t.Errorf("Failed to get ConfigMap in namespace %s: %v", ns, err)
					continue
				}

				// Verify ConfigMap data
				if cm.Data == nil {
					t.Errorf("ConfigMap in namespace %s has nil data", ns)
					continue
				}

				minVersion, ok := cm.Data["minTLSVersion"]
				if !ok {
					t.Errorf("ConfigMap in namespace %s missing minTLSVersion key", ns)
				}
				if minVersion == "" {
					t.Errorf("ConfigMap in namespace %s has empty minTLSVersion", ns)
				}

				ciphers, ok := cm.Data["cipherSuites"]
				if !ok {
					t.Errorf("ConfigMap in namespace %s missing cipherSuites key", ns)
				}
				// In unit test mode, we use Intermediate profile which has ciphers
				if ciphers == "" {
					t.Errorf("ConfigMap in namespace %s has empty cipherSuites", ns)
				}

				// Verify cipherSuites is comma-separated
				cipherList := strings.Split(ciphers, ",")
				if len(cipherList) == 0 {
					t.Errorf("ConfigMap in namespace %s has no cipher suites", ns)
				}

				// Verify owner reference is set
				if len(cm.OwnerReferences) == 0 {
					t.Errorf("ConfigMap in namespace %s has no owner references", ns)
					continue
				}

				ownerRef := cm.OwnerReferences[0]
				if ownerRef.Kind != "MultiClusterEngine" {
					t.Errorf("ConfigMap in namespace %s owner reference kind = %s, want MultiClusterEngine", ns, ownerRef.Kind)
				}
				if ownerRef.Name != mce.Name {
					t.Errorf("ConfigMap in namespace %s owner reference name = %s, want %s", ns, ownerRef.Name, mce.Name)
				}
				if !*ownerRef.Controller {
					t.Errorf("ConfigMap in namespace %s owner reference controller = false, want true", ns)
				}
			}
		})
	}
}
