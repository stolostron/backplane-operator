// Copyright Contributors to the Open Cluster Management project
package utils

import (
	"testing"
)

const (
	expName       = DefaultLocalClusterName
	expKind       = "ManagedCluster"
	expAPIVersion = "cluster.open-cluster-management.io/v1"
)

func TestNewManagedCluster(t *testing.T) {
	mc := NewManagedCluster(DefaultLocalClusterName)

	if name := mc.GetName(); name != expName {
		t.Errorf("NewManagedCluster Name: expected %q, got %q", expName, name)
	}

	if kind := mc.GetKind(); kind != expKind {
		t.Errorf("NewManagedCluster Kind: expected %q, got %q", expKind, kind)
	}

	if apiVersion := mc.GetAPIVersion(); apiVersion != expAPIVersion {
		t.Errorf("NewManagedCluster apiVersion: expected %q, got %q", expAPIVersion, apiVersion)
	}

	expLabels := map[string]string{
		"local-cluster":                 "true",
		"cloud":                         "auto-detect",
		"vendor":                        "auto-detect",
		"velero.io/exclude-from-backup": "true",
	}

	labels := mc.GetLabels()
	if len(labels) != len(expLabels) {
		t.Errorf("NewManagedCluster Labels: expected %d labels, found %d", len(expLabels), len(labels))
	}
	for k, expV := range expLabels {
		v, ok := labels[k]
		if !ok {
			t.Errorf("NewManagedCluster Labels: label %q not found", k)
			continue
		}
		if v != expV {
			t.Errorf("NewManagedCluster Labels: label %q expected %q, got %q", k, expV, v)
		}
	}
}

func TestNewLocalNamespace(t *testing.T) {
	ns := NewLocalNamespace(DefaultLocalClusterName)

	if name := ns.GetName(); name != expName {
		t.Errorf("NewLocalNamespace Name: expected %q, got %q", expName, name)
	}
}
