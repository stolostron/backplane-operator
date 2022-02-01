// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"os"
	"testing"

	v1 "github.com/stolostron/backplane-operator/api/v1"
)

func TestGetImageOverridesRelatedImage(t *testing.T) {
	os.Setenv("RELATED_IMAGE_APPLICATION_UI", "quay.io/stolostron/application-ui:test-image")
	os.Setenv("RELATED_IMAGE_CERT_POLICY_CONTROLLER", "quay.io/stolostron/cert-policy-controller:test-image")

	if len(GetImageOverrides(&v1.MultiClusterEngine{})) != 2 {
		t.Fatal("Expected image overrides")
	}

	os.Unsetenv("RELATED_IMAGE_APPLICATION_UI")
	os.Unsetenv("RELATED_IMAGE_CERT_POLICY_CONTROLLER")

	if len(GetImageOverrides(&v1.MultiClusterEngine{})) != 0 {
		t.Fatal("Expected no image overrides")
	}
}

func TestGetImageOverridesOperandImage(t *testing.T) {
	os.Setenv("OPERAND_IMAGE_APPLICATION_UI", "quay.io/stolostron/application-ui:test-image")
	os.Setenv("OPERAND_IMAGE_CERT_POLICY_CONTROLLER", "quay.io/stolostron/cert-policy-controller:test-image")

	if len(GetImageOverrides(&v1.MultiClusterEngine{})) != 2 {
		t.Fatal("Expected image overrides")
	}

	os.Unsetenv("OPERAND_IMAGE_APPLICATION_UI")
	os.Unsetenv("OPERAND_IMAGE_CERT_POLICY_CONTROLLER")

	if len(GetImageOverrides(&v1.MultiClusterEngine{})) != 0 {
		t.Fatal("Expected no image overrides")
	}
}

func TestGetImageOverridesBothEnvVars(t *testing.T) {
	os.Setenv("RELATED_IMAGE_APPLICATION_UI", "quay.io/stolostron/application-ui:test-image")
	os.Setenv("OPERAND_IMAGE_CERT_POLICY_CONTROLLER", "quay.io/stolostron/cert-policy-controller:test-image")

	if len(GetImageOverrides(&v1.MultiClusterEngine{})) != 1 {
		t.Fatal("Expected image overrides")
	}

	os.Unsetenv("OPERAND_IMAGE_CERT_POLICY_CONTROLLER")

	if len(GetImageOverrides(&v1.MultiClusterEngine{})) != 1 {
		t.Fatal("Expected no image overrides")
	}
}
