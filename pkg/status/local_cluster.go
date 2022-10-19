// Copyright Contributors to the Open Cluster Management project
package status

import (
	bpv1 "github.com/stolostron/backplane-operator/api/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LocalClusterStatus fulfills the StatusReporter interface for managing
// the local cluster
type LocalClusterStatus struct {
	types.NamespacedName
}

func (s LocalClusterStatus) GetName() string {
	return s.Name
}

func (s LocalClusterStatus) GetNamespace() string {
	return s.Namespace
}

func (s LocalClusterStatus) GetKind() string {
	return "ManagedCluster"
}

func (s LocalClusterStatus) Status(c client.Client) bpv1.ComponentCondition {
	return bpv1.ComponentCondition{}
}
