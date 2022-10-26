// Copyright Contributors to the Open Cluster Management project
package status

import (
	"context"

	bpv1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/utils"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	Accepted  = "HubAcceptedManagedCluster"
	Joined    = "ManagedClusterJoined"
	Available = "ManagedClusterConditionAvailable"
)

// LocalClusterStatus fulfills the StatusReporter interface for managing
// the local cluster
type LocalClusterStatus struct {
	types.NamespacedName
	Enabled bool
}

func (s LocalClusterStatus) GetName() string {
	return s.Name
}

func (s LocalClusterStatus) GetNamespace() string {
	return s.Namespace
}

func (s LocalClusterStatus) GetKind() string {
	return "local-cluster"
}

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
	mc := utils.NewManagedCluster()
	err := c.Get(context.Background(), s.NamespacedName, mc)
	if apierrors.IsNotFound(err) {
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

	return bpv1.ComponentCondition{
		Name:               s.GetName(),
		Kind:               s.GetKind(),
		Available:          true,
		Type:               "ManagedClusterImportSuccess",
		Status:             metav1.ConditionTrue,
		LastUpdateTime:     metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Reason:             "ManagedClusterImported",
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
			Reason:             "ManagedClusterDisabled",
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
		Reason:             "ManagedClusterDisabled",
		Message:            "ManagedCluster resource is still present",
	}
}
