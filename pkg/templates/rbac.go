// go:build ignore
// Copyright Contributors to the Open Cluster Management project

package main

import (
	"fmt"
	"os"
	"sort"
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
//+kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=addondeploymentconfigs,verbs=create;get;list;update;patch;watch;delete
//+kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=addontemplates,verbs=create;get;list;update;patch;watch;delete
//+kubebuilder:rbac:groups=console.openshift.io,resources=consoleplugins;consolequickstarts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;create;update;list;watch;delete;patch

// cluster-proxy-addon
//+kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;create;update;list;watch;delete;patch
//+kubebuilder:rbac:groups=proxy.open-cluster-management.io,resources=managedproxyconfigurations;managedproxyserviceresolvers,verbs=get;create;update;list;watch;delete;patch

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
	"ManagedProxyConfiguration",
	"ManagedProxyServiceResolver",
	"MutatingWebhookConfiguration",
	"PrometheusRule",
	"Role",
	"RoleBinding",
	"Route",
	"Service",
	"ServiceAccount",
	"ServiceMonitor",
	"ValidatingWebhookConfiguration",
	"AddOnDeploymentConfig",
	"AddOnTemplate",
}

func main() {
	// os.Setenv("DIRECTORY_OVERRIDE", "../../")
	// defer os.Unsetenv("DIRECTORY_OVERRIDE")
	os.Setenv("ACM_HUB_OCP_VERSION", "4.10.0")

	testBackplane := &backplanev1.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testBackplane",
		},
	}

	testImages := map[string]string{}
	for _, v := range utils.GetTestImages() {
		testImages[v] = "quay.io/test/test:Test"
	}
	testTemplateOverrides := map[string]string{}

	chartsDir := chartsDir
	templates, errs := renderer.RenderCharts(chartsDir, testBackplane, testImages, testTemplateOverrides)
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
	defer func() {
		if err := f.Close(); err != nil {
			panic(err)
		}
	}()

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

			newlines := extractFromRules(clusterrole.Rules)
			lines = append(lines, newlines...)
		} else if template.GetKind() == "Role" {
			role := &rbacv1.Role{}
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(template.Object, role)
			if err != nil {
				panic(err)
			}

			newlines := extractFromRules(role.Rules)
			lines = append(lines, newlines...)
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

	sort.Strings(lines)

	err = packageTemplate.Execute(f, struct {
		Markers []string
	}{
		Markers: lines,
	})
	if err != nil {
		panic(err)
	}
}

func extractFromRules(rules []rbacv1.PolicyRule) []string {
	lines := []string{}
	for _, rule := range rules {
		if len(rule.Resources) == 0 {
			continue
		}
		for i, group := range rule.APIGroups {
			if group == "" {
				rule.APIGroups[i] = "\"\""
			}
		}
		apiGroups := strings.Join(rule.APIGroups, ";")
		resources := strings.Join(rule.Resources, ";")
		verbs := strings.Join(rule.Verbs, ";")
		line := fmt.Sprintf("//+kubebuilder:rbac:groups=%s,resources=%s,verbs=%s", apiGroups, resources, verbs)
		lines = append(lines, line)
	}
	return lines
}

var packageTemplate = template.Must(template.New("").Parse(`// Code generated by go generate; DO NOT EDIT.

package main
{{ range .Markers }}
{{.}}
{{- end }}
`))
