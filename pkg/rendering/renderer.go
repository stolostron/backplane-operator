// Copyright Contributors to the Open Cluster Management project
package renderer

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	loader "helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"

	"github.com/fatih/structs"
	"github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	"github.com/open-cluster-management/backplane-operator/pkg/utils"
	"helm.sh/helm/v3/pkg/engine"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
)

const (
	crdsDir   = "pkg/templates/crds"
	chartsDir = "pkg/templates/charts"
)

type Values struct {
	Global    Global    `yaml:"global" structs:"global"`
	HubConfig HubConfig `yaml:"hubconfig" structs:"hubconfig"`
	Org       string    `yaml:"org" structs:"org"`
}

type Global struct {
	ImageOverrides map[string]string `yaml:"imageOverrides" structs:"imageOverrides"`
	PullPolicy     string            `yaml:"pullPolicy" structs:"pullPolicy"`
	PullSecret     string            `yaml:"pullSecret" structs:"pullSecret"`
	Namespace      string            `yaml:"namespace" structs:"namespace"`
}

type HubConfig struct {
	NodeSelector map[string]string `yaml:"nodeSelector" structs:"nodeSelector"`
	ProxyConfigs map[string]string `yaml:"proxyConfigs" structs:"proxyConfigs"`
	ReplicaCount int               `yaml:"replicaCount" structs:"replicaCount"`
}

func RenderCRDs() ([]*unstructured.Unstructured, []error) {
	var crds []*unstructured.Unstructured
	errs := []error{}

	crdPath := crdsDir
	if val, ok := os.LookupEnv("DIRECTORY_OVERRIDE"); ok {
		crdPath = path.Join(val, crdPath)
	}

	// Read CRD files
	err := filepath.Walk(crdPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println(err.Error())
			return err
		}
		crd := &unstructured.Unstructured{}
		if info == nil || info.IsDir() {
			return nil
		}
		bytesFile, e := ioutil.ReadFile(path)
		if e != nil {
			errs = append(errs, fmt.Errorf("%s - error reading file: %v", info.Name(), err.Error()))
		}
		if err = yaml.Unmarshal(bytesFile, crd); err != nil {
			errs = append(errs, fmt.Errorf("%s - error unmarshalling file to unstructured: %v", info.Name(), err.Error()))
		}
		crds = append(crds, crd)
		return nil
	})
	if err != nil {
		return crds, errs
	}

	return crds, errs
}

func RenderTemplates(backplaneConfig *v1alpha1.BackplaneConfig, images map[string]string) ([]*unstructured.Unstructured, []error) {
	log := log.FromContext(context.Background())
	var templates []*unstructured.Unstructured
	errs := []error{}
<<<<<<< HEAD
<<<<<<< HEAD

=======
	backplaneOperatorNamespace := ""
>>>>>>>  change backplane config scope and associated code
=======
	backplaneOperatorNamespace := ""
>>>>>>>  change backplane config scope and associated code
	chartDir := chartsDir
	if val, ok := os.LookupEnv("DIRECTORY_OVERRIDE"); ok {
		chartDir = path.Join(val, chartDir)
	}
<<<<<<< HEAD
<<<<<<< HEAD

=======
=======
>>>>>>>  change backplane config scope and associated code
	if val, ok := os.LookupEnv("POD_NAMESPACE"); ok {
		backplaneOperatorNamespace = val
	} else {
		log.Info(fmt.Sprintf("error retrieving namespace"))
		return nil, append(errs, fmt.Errorf("error retrieving namespace"))
	}
<<<<<<< HEAD
>>>>>>>  change backplane config scope and associated code
=======
>>>>>>>  change backplane config scope and associated code
	charts, err := ioutil.ReadDir(chartDir)
	if err != nil {
		errs = append(errs, err)
	}

	helmEngine := engine.Engine{
		Strict:   true,
		LintMode: false,
	}

	for _, chart := range charts {

		chart, err := loader.Load(filepath.Join(chartDir, chart.Name()))
		if err != nil {
			log.Info(fmt.Sprintf("error loading chart: %s", chart.Name()))
			return nil, append(errs, err)
		}

		valuesYaml := &Values{}
<<<<<<< HEAD
<<<<<<< HEAD
		injectValuesOverrides(valuesYaml, backplaneConfig, images)
=======
		injectValuesOverrides(valuesYaml, backplaneConfig, backplaneOperatorNamespace, images)
>>>>>>>  change backplane config scope and associated code
=======
		injectValuesOverrides(valuesYaml, backplaneConfig, backplaneOperatorNamespace, images)
>>>>>>>  change backplane config scope and associated code

		rawTemplates, err := helmEngine.Render(chart, chartutil.Values{"Values": structs.Map(valuesYaml)})
		if err != nil {
			log.Info(fmt.Sprintf("error rendering chart: %s", chart.Name()))
			return nil, append(errs, err)
		}

		for fileName, templateFile := range rawTemplates {
			unstructured := &unstructured.Unstructured{}
			if err = yaml.Unmarshal([]byte(templateFile), unstructured); err != nil {
				return nil, append(errs, fmt.Errorf("error converting file %s to unstructured", fileName))
			}

<<<<<<< HEAD
<<<<<<< HEAD
			utils.AddBackplaneConfigLabels(unstructured, backplaneConfig.Name, backplaneConfig.Namespace)
=======
			utils.AddBackplaneConfigLabels(unstructured, backplaneConfig.Name)
>>>>>>>  change backplane config scope and associated code
=======
			utils.AddBackplaneConfigLabels(unstructured, backplaneConfig.Name)
>>>>>>>  change backplane config scope and associated code

			// Add namespace to namespaced resources
			switch unstructured.GetKind() {
			case "Deployment", "ServiceAccount", "Role", "RoleBinding", "Service":
<<<<<<< HEAD
<<<<<<< HEAD
				unstructured.SetNamespace(backplaneConfig.Namespace)
=======
				unstructured.SetNamespace(backplaneOperatorNamespace)
>>>>>>>  change backplane config scope and associated code
=======
				unstructured.SetNamespace(backplaneOperatorNamespace)
>>>>>>>  change backplane config scope and associated code
			}
			templates = append(templates, unstructured)
		}
	}

	return templates, errs
}

<<<<<<< HEAD
<<<<<<< HEAD
func injectValuesOverrides(values *Values, backplaneConfig *v1alpha1.BackplaneConfig, images map[string]string) {
=======
func injectValuesOverrides(values *Values, backplaneConfig *v1alpha1.BackplaneConfig, backplaneOperatorNamespace string, images map[string]string) {
>>>>>>>  change backplane config scope and associated code
=======
func injectValuesOverrides(values *Values, backplaneConfig *v1alpha1.BackplaneConfig, backplaneOperatorNamespace string, images map[string]string) {
>>>>>>>  change backplane config scope and associated code

	values.Global.ImageOverrides = images

	values.Global.PullPolicy = "Always"

<<<<<<< HEAD
<<<<<<< HEAD
	values.Global.Namespace = backplaneConfig.Namespace
=======
	values.Global.Namespace = backplaneOperatorNamespace
>>>>>>>  change backplane config scope and associated code
=======
	values.Global.Namespace = backplaneOperatorNamespace
>>>>>>>  change backplane config scope and associated code

	values.HubConfig.ReplicaCount = 1

	values.Org = "open-cluster-management"

	// TODO: Define all overrides
}
