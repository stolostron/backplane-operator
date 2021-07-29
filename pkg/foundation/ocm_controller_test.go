// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package foundation

import (
	"testing"

	v1alpha1 "github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestOcmControllerDeployment(t *testing.T) {
	empty := &v1alpha1.BackplaneConfig{
		ObjectMeta: metav1.ObjectMeta{Namespace: "test"},
		Spec:       v1alpha1.BackplaneConfigSpec{},
	}

	ovr := map[string]string{}

	t.Run("MCH with empty fields", func(t *testing.T) {
		_ = OCMControllerDeployment(empty, ovr)
	})
}
