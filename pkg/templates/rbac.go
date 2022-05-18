// go:build ignore
// Copyright Contributors to the Open Cluster Management project

package main

import (
	"fmt"
	"os"
	"strings"
	"text/template"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	renderer "github.com/stolostron/backplane-operator/pkg/rendering"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stolostron/backplane-operator/pkg/utils"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	chartsDir  = "pkg/templates/charts/toggle"
	chartsPath = "pkg/templates/charts/toggle/managed-serviceaccount"
	crdsDir    = "pkg/templates/crds"
)

//+kubebuilder:rbac:groups="",resources=configmaps;serviceaccounts;services,verbs=create;get;list;update;watch;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=create;get;list;update;watch;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings;rolebindings,verbs=create;get;list;update;watch;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;roles,verbs=create;get;list;update;watch;patch;delete;bind
//+kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=create;get;list;update;watch;patch;delete
//+kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations;mutatingwebhookconfigurations,verbs=create;get;list;update;watch;patch;delete
//+kubebuilder:rbac:groups=apiregistration.k8s.io,resources=apiservices,verbs=create;get;list;update;watch;patch;delete

//+kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=clustermanagementaddons,verbs=create;get;list;update;patch;watch;delete
//+kubebuilder:rbac:groups=console.openshift.io,resources=consoleplugins;consolequickstarts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;create;update;list;watch;delete;patch

var resources = []string{
	"APIService",
	"ClusterManagementAddOn",
	"ClusterRoleBinding",
	"ClusterRole",
	"ConfigMap",
	"ConsolePlugin",
	"ConsoleQuickStart",
	"CustomResourceDefinition",
	"Deployment",
	"MutatingWebhookConfiguration",
	"Role",
	"RoleBinding",
	"Service",
	"ServiceAccount",
	"ServiceMonitor",
	"ValidatingWebhookConfiguration",
}

func main() {
	// os.Setenv("DIRECTORY_OVERRIDE", "../../")
	// defer os.Unsetenv("DIRECTORY_OVERRIDE")

	testBackplane := &backplanev1.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testBackplane",
		},
	}

	testImages := map[string]string{}
	for _, v := range utils.GetTestImages() {
		testImages[v] = "quay.io/test/test:Test"
	}
	chartsDir := chartsDir
	templates, errs := renderer.RenderCharts(chartsDir, testBackplane, testImages)
	if len(errs) > 0 {
		panic(errs)
	}
	if len(templates) == 0 {
		panic("No templates rendered")
	}

	for _, template := range templates {
		if template.GetKind() == "ClusterRole" {
			clusterrole := &rbacv1.ClusterRole{}
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(template.Object, clusterrole)
			if err != nil {
				panic(err)
			}

		}
	}

	f, err := os.Create("pkg/templates/rbac_gen.go")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	lines := []string{}
	for _, template := range templates {
		if template.GetKind() == "ClusterRole" {
			// Copy all permission defined in Clusterroles
			// Duplicate permissions will be deduplicated by controller gen

			clusterrole := &rbacv1.ClusterRole{}
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(template.Object, clusterrole)
			if err != nil {
				panic(err)
			}

			for _, rule := range clusterrole.Rules {
				apiGroups := strings.Join(rule.APIGroups, ";")
				resources := strings.Join(rule.Resources, ";")
				verbs := strings.Join(rule.Verbs, ";")
				line := fmt.Sprintf("//+kubebuilder:rbac:groups=%s,resources=%s,verbs=%s", apiGroups, resources, verbs)
				lines = append(lines, line)
			}
		} else if template.GetKind() == "Role" {
			role := &rbacv1.Role{}
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(template.Object, role)
			if err != nil {
				panic(err)
			}

			for _, rule := range role.Rules {
				apiGroups := strings.Join(rule.APIGroups, ";")
				resources := strings.Join(rule.Resources, ";")
				verbs := strings.Join(rule.Verbs, ";")
				line := fmt.Sprintf("//+kubebuilder:rbac:groups=%s,resources=%s,verbs=%s", apiGroups, resources, verbs)
				lines = append(lines, line)
			}
		} else {
			if template.GetKind() != "" {
				// check that we have the permissions to apply this resource and
				// error if not
				apiGroup := template.GroupVersionKind().Group
				resource := template.GetKind()

				found := false
				for _, k := range resources {
					if k == resource {
						found = true
					}
				}
				if !found {
					panic(fmt.Sprintf("resource %s/%s not accounted for in RBAC generation", apiGroup, resource))
				}
			}
		}
	}

	packageTemplate.Execute(f, struct {
		Markers []string
	}{
		Markers: lines,
	})
}

var packageTemplate = template.Must(template.New("").Parse(`// Code generated by go generate; DO NOT EDIT.

package main

{{- range .Markers }}
{{.}}
{{- end }}
`))
