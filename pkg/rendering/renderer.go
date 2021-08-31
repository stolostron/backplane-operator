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

func RenderTemplates(backplaneConfig *v1alpha1.MultiClusterEngine, images map[string]string) ([]*unstructured.Unstructured, []error) {
	log := log.FromContext(context.Background())
	var templates []*unstructured.Unstructured
	errs := []error{}
	backplaneOperatorNamespace := ""
	chartDir := chartsDir
	if val, ok := os.LookupEnv("DIRECTORY_OVERRIDE"); ok {
		chartDir = path.Join(val, chartDir)
	}
	if val, ok := os.LookupEnv("POD_NAMESPACE"); ok {
		backplaneOperatorNamespace = val
	} else {
		log.Info(fmt.Sprintf("error retrieving namespace"))
		return nil, append(errs, fmt.Errorf("error retrieving namespace"))
	}
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
		injectValuesOverrides(valuesYaml, backplaneConfig, backplaneOperatorNamespace, images)

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

			utils.AddBackplaneConfigLabels(unstructured, backplaneConfig.Name)

			// Add namespace to namespaced resources
			switch unstructured.GetKind() {
			case "Deployment", "ServiceAccount", "Role", "RoleBinding", "Service":
				unstructured.SetNamespace(backplaneOperatorNamespace)
			}
			templates = append(templates, unstructured)
		}
	}

	return templates, errs
}

func injectValuesOverrides(values *Values, backplaneConfig *v1alpha1.MultiClusterEngine, backplaneOperatorNamespace string, images map[string]string) {

	values.Global.ImageOverrides = images

	values.Global.PullPolicy = "Always"

	values.Global.Namespace = backplaneOperatorNamespace

	values.HubConfig.ReplicaCount = 1

	values.Org = "open-cluster-management"

	// TODO: Define all overrides
}
