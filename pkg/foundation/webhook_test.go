// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package foundation

import (
	"testing"

	v1alpha1 "github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWebhookDeployment(t *testing.T) {
	empty := &v1alpha1.BackplaneConfig{
		ObjectMeta: metav1.ObjectMeta{Namespace: "test"},
		Spec:       v1alpha1.BackplaneConfigSpec{},
	}
	ovr := map[string]string{}

	t.Run("MCH with empty fields", func(t *testing.T) {
		_ = WebhookDeployment(empty, ovr)
	})
}

func TestWebhookService(t *testing.T) {
	bpc := &v1alpha1.BackplaneConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testName",
			Namespace: "testNS",
		},
	}

	t.Run("Create service", func(t *testing.T) {
		s := WebhookService(bpc)
		if ns := s.Namespace; ns != "testNS" {
			t.Errorf("expected namespace %s, got %s", "testNS", ns)
		}
		if ref := s.GetOwnerReferences(); ref[0].Name != "testName" {
			t.Errorf("expected ownerReference %s, got %s", "testName", ref[0].Name)
		}
	})
}
