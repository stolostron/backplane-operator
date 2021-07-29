// Copyright Contributors to the Open Cluster Management project
package renderer

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

const (
	crdsDir = "bin/crds"
)

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
		fmt.Println(path)
		crd := &unstructured.Unstructured{}
		if info == nil || info.IsDir() {
			return nil
		}
		fmt.Println()
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
