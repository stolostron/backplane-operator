// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package templates

import (
	"context"
	"io/ioutil"
	"os"
	"path"

	"github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
)

const (
	TemplatesPath = "bin/components/foundation"
)

func GetTemplates(backplaneConfig *v1alpha1.BackplaneConfig) ([]*unstructured.Unstructured, error) {
	log := log.FromContext(context.Background())

	templatesPath := TemplatesPath
	if val, ok := os.LookupEnv("DIRECTORY_OVERRIDE"); ok {
		templatesPath = path.Join(val, TemplatesPath)
	}

	templates, err := ioutil.ReadDir(templatesPath)
	if err != nil {
		log.Error(err, err.Error())
		return nil, err
	}

	var templatesResults []*unstructured.Unstructured
	// templates := &unstructured.Unstructured[]
	for _, template := range templates {
		if template.IsDir() {
			continue
		}
		bytes, err := ioutil.ReadFile(path.Join(templatesPath, template.Name()))
		if err != nil {
			return nil, err
		}
		templated := &unstructured.Unstructured{}
		err = yaml.Unmarshal(bytes, templated)
		if err != nil {
			log.Error(err, err.Error())
		}
		templated.SetNamespace(backplaneConfig.GetNamespace())
		if templated.GetKind() == "ClusterRoleBinding" {
			templated, err = updateSubjectNamespace(backplaneConfig, templated)
			if err != nil {
				return nil, err
			}
		}
		templatesResults = append(templatesResults, templated)
	}
	return templatesResults, nil
}

func updateSubjectNamespace(backplaneConfig *v1alpha1.BackplaneConfig, template *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	bytes, err := yaml.Marshal(template)
	if err != nil {
		return nil, err
	}
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	err = yaml.Unmarshal(bytes, clusterRoleBinding)
	if err != nil {
		return nil, err
	}

	for i := 0; i < len(clusterRoleBinding.Subjects); i = i + 1 {
		clusterRoleBinding.Subjects[i].Namespace = backplaneConfig.GetNamespace()
	}
	bytes, err = yaml.Marshal(clusterRoleBinding)
	if err != nil {
		return nil, err
	}
	unstructured := &unstructured.Unstructured{}
	err = yaml.Unmarshal(bytes, unstructured)
	if err != nil {
		return nil, err
	}
	return unstructured, nil
}
