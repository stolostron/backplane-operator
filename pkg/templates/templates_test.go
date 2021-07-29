// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package templates

import (
	"os"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/open-cluster-management/backplane-operator/api/v1alpha1"
)

func TestTemplates(t *testing.T) {

	bpc := &v1alpha1.BackplaneConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: v1alpha1.BackplaneConfigSpec{},
	}

	os.Setenv("DIRECTORY_OVERRIDE", "../../")
	defer os.Unsetenv("DIRECTORY_OVERRIDE")

	templates, err := GetTemplates(bpc)
	if err != nil {
		t.Fatalf(err.Error())
	}
	if len(templates) == 0 {
		t.Fatalf("Unable to render templates")
	}
}
