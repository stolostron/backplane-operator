package renderer

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	marshal "gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

const (
	crdsDir   = "bin/crds"
	chartsDir = "bin/charts"
)

type Values struct {
	Global    Global    `yaml:"global"`
	HubConfig HubConfig `yaml:"hubconfig"`
	Org       string    `yaml:"org"`
}

type Global struct {
	ImageOverrides map[string]string `yaml:"imageOverrides"`
	PullPolicy     string            `yaml:"pullPolicy"`
	PullSecret     string            `yaml:"pullSecret"`
}

type HubConfig struct {
	NodeSelector map[string]string `yaml:"nodeSelector"`
	ProxyConfigs map[string]string `yaml:"proxyConfigs"`
	ReplicaCount int               `yaml:"replicaCount"`
}

func RenderCRDs() ([]*unstructured.Unstructured, []error) {
	var crds []*unstructured.Unstructured
	errs := []error{}

	// Read CRD files
	err := filepath.Walk(crdsDir, func(path string, info os.FileInfo, err error) error {
		crd := &unstructured.Unstructured{}
		if info.IsDir() {
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

func RenderTemplates(backplaneConfig *v1alpha1.BackplaneConfig) ([]*unstructured.Unstructured, []error) {
	// log := log.FromContext(context.Background())
	var templates []*unstructured.Unstructured
	errs := []error{}

	// Read CRD files
	charts, err := ioutil.ReadDir(chartsDir)
	if err != nil {
		errs = append(errs, err)
	}

	for _, chart := range charts {
		templateFiles, err := ioutil.ReadDir(filepath.Join(chartsDir, chart.Name(), "templates"))
		if err != nil {
			errs = append(errs, err)
		}

		valuesYamlPath := filepath.Join(chartsDir, chart.Name(), "values.yaml")

		buf, err := ioutil.ReadFile(valuesYamlPath)
		if err != nil {
			return nil, append(errs, err)
		}

		valuesYaml := &Values{}
		err = yaml.Unmarshal(buf, valuesYaml)
		if err != nil {
			return nil, append(errs, err)
		}

		injectValuesOverrides(valuesYaml, backplaneConfig)

		buf, err = marshal.Marshal(valuesYaml)
		if err != nil {
			return nil, append(errs, err)
		}
		err = ioutil.WriteFile(valuesYamlPath, buf, 0777)
		if err != nil {
			return nil, append(errs, err)
		}

		for _, templateFile := range templateFiles {
			if templateFile.IsDir() {
				continue
			}
			command := exec.Command("./bin/helm", "template", filepath.Join(chartsDir, chart.Name()), "--name-template", strings.ToLower(backplaneConfig.Kind), "-s", filepath.Join("templates", templateFile.Name()))
			// set var to get the output
			var out bytes.Buffer
			// set the output to our variable
			command.Stdout = &out
			err = command.Run()
			if err != nil {
				errs = append(errs, err)
			}

			unstructured := &unstructured.Unstructured{}
			if err = yaml.Unmarshal(out.Bytes(), unstructured); err != nil {
				errs = append(errs, fmt.Errorf("error reading helm templated output"))
			}

			labels := unstructured.GetLabels()
			if len(labels) == 0 {
				labels = make(map[string]string)
			}
			labels["open-cluster-management.backplane-operator.name"] = backplaneConfig.Name
			labels["open-cluster-management.backplane-operator.namespace"] = backplaneConfig.Namespace
			unstructured.SetLabels(labels)

			// Add namespace to namespaced resources
			switch unstructured.GetKind() {
			case "Deployment", "ServiceAccount", "Role", "RoleBinding":
				unstructured.SetNamespace(backplaneConfig.Namespace)
			}
			templates = append(templates, unstructured)
		}
	}

	return templates, errs
}

func injectValuesOverrides(values *Values, backplaneConfig *v1alpha1.BackplaneConfig) {

	// values.Global.PullPolicy = "IfNotPresent"

	// TODO: Define all overrides
}
