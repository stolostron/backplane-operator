// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package renderer

import (
	backplane "github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
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
	testBackplane := &backplane.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testBackplane",
		},
		Spec: backplane.MultiClusterEngineSpec{
			Foo:          "bar",
			NodeSelector: backplaneNodeSelector,
		},
		Status: backplane.MultiClusterEngineStatus{
			Phase: "",
		},
	}
	testImages := map[string]string{"registration_operator": "test", "openshift_hive": "test", "multicloud_manager": "test"}
	os.Setenv("POD_NAMESPACE", "default")
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
			deployment := appsv1.Deployment{}
			runtime.DefaultUnstructuredConverter.FromUnstructured(template.Object, &deployment)
			equality := reflect.DeepEqual(deployment.Spec.Template.Spec.NodeSelector, backplaneNodeSelector)
			if !equality {
				t.Fatalf("Node Selector did not propagate to the deployments use")
			}
		}
	}
	os.Unsetenv("POD_NAMESPACE")

}
