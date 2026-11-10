// Copyright Contributors to the Open Cluster Management project
package status

import (
	"context"

	bpv1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/utils"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	lcsKind = "local-cluster"

	Accepted  = "HubAcceptedManagedCluster"
	Joined    = "ManagedClusterJoined"
	Available = "ManagedClusterConditionAvailable"

	MCDisabled = "ManagedClusterDisabled"
	MCImported = "ManagedClusterImported"
)

// LocalClusterStatus fulfills the StatusReporter interface for managing
// the local cluster
type LocalClusterStatus struct {
	types.NamespacedName
	Enabled bool
}

// GetName returns the name of the local cluster ManagedCluster resource,
// satisfying the StatusReporter interface.
func (s LocalClusterStatus) GetName() string {
	return s.Name
}

// GetNamespace returns the namespace of the local cluster ManagedCluster
// resource, satisfying the StatusReporter interface.
func (s LocalClusterStatus) GetNamespace() string {
	return s.Namespace
}

// GetKind returns the constant "local-cluster" as the kind string,
// satisfying the StatusReporter interface.
func (s LocalClusterStatus) GetKind() string {
	return lcsKind
}

// Status queries the cluster via c for the local ManagedCluster resource and
// returns a ComponentCondition reflecting its health. When Enabled is true the
// ManagedCluster must be accepted, joined, and available to report as ready;
// if it is being deleted the condition reports it as unavailable with a
// detaching message. When Enabled is false the condition is available only if
// the ManagedCluster resource has been fully removed. An unknown condition is
// returned if the ManagedCluster cannot be fetched.
func (s LocalClusterStatus) Status(c client.Client) bpv1.ComponentCondition {
	mc, err := s.getManagedCluster(c)
	if err != nil {
		return unknownStatus(s.GetName(), s.GetKind())
	}

	if s.Enabled {
		return s.enabledStatus(mc)
	}

	return s.disabledStatus(mc)
}

func (s *LocalClusterStatus) getManagedCluster(c client.Client) (*unstructured.Unstructured, error) {
	mc := utils.NewManagedCluster(s.NamespacedName.Name)
	err := c.Get(context.Background(), s.NamespacedName, mc)
	if apierrors.IsNotFound(err) || apimeta.IsNoMatchError(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return mc, nil
}

func (s *LocalClusterStatus) enabledStatus(mc *unstructured.Unstructured) bpv1.ComponentCondition {
	if mc == nil {
		return unknownStatus(s.GetName(), s.GetKind())
	}

	status, ok := mc.Object["status"].(map[string]interface{})
	if !ok {
		return unknownStatus(s.GetName(), s.GetKind())
	}
	conditions, ok := status["conditions"].([]interface{})
	if !ok || len(conditions) == 0 {
		return unknownStatus(s.GetName(), s.GetKind())
	}

	var accepted, joined, available bool
	var latest map[string]interface{}
	for _, condition := range conditions {
		c, ok := condition.(map[string]interface{})
		if !ok {
			return unknownStatus(s.GetName(), s.GetKind())
		}
		latest = c
		switch c["type"] {
		case Accepted:
			accepted = true
		case Joined:
			joined = true
		case Available:
			available = true
		}
	}

	ready := accepted && joined && available
	if !ready {
		return bpv1.ComponentCondition{
			Name:               s.GetName(),
			Kind:               s.GetKind(),
			Available:          false,
			Type:               latest["type"].(string),
			Status:             metav1.ConditionStatus(latest["status"].(string)),
			LastUpdateTime:     metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             latest["reason"].(string),
			Message:            latest["message"].(string),
		}
	}

	// correct the user's change of the local-cluster in mce when it is owned by mch
	if detaching := mc.GetDeletionTimestamp(); detaching != nil {
		return bpv1.ComponentCondition{
			Name:               s.GetName(),
			Kind:               s.GetKind(),
			Available:          false,
			Type:               latest["type"].(string),
			Status:             metav1.ConditionStatus(latest["status"].(string)),
			LastUpdateTime:     metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             latest["reason"].(string),
			Message:            "Local-Cluster is currently being detached",
		}
	}

	return bpv1.ComponentCondition{
		Name:               s.GetName(),
		Kind:               s.GetKind(),
		Available:          true,
		Type:               "ManagedClusterImportSuccess",
		Status:             metav1.ConditionTrue,
		LastUpdateTime:     metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Reason:             MCImported,
		Message:            "ManagedCluster is accepted, joined, and available",
	}
}

func (s *LocalClusterStatus) disabledStatus(mc *unstructured.Unstructured) bpv1.ComponentCondition {
	if mc == nil {
		return bpv1.ComponentCondition{
			Name:               s.GetName(),
			Kind:               s.GetKind(),
			Available:          true,
			Type:               "NotPresent",
			Status:             metav1.ConditionTrue,
			LastUpdateTime:     metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             MCDisabled,
			Message:            "ManagedCluster resource is not present",
		}
	}

	return bpv1.ComponentCondition{
		Name:               s.GetName(),
		Kind:               s.GetKind(),
		Available:          false,
		Type:               "NotPresent",
		Status:             metav1.ConditionFalse,
		LastUpdateTime:     metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Reason:             MCDisabled,
		Message:            "ManagedCluster resource is still present",
	}
}
