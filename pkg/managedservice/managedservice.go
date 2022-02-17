// Copyright Contributors to the Open Cluster Management project

package managedservice

import (
	"context"

	"fmt"

	bpv1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/status"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	managedServiceAccountChartDir = "pkg/templates/charts/toggle/managed-serviceaccount"
	managedServiceAccountCRDPath  = "pkg/templates/managed-serviceaccount/crds"
)

//
func renderTemplates() ([]*unstructured.Unstructured, []error) {
	return []*unstructured.Unstructured{}, nil
}

func ManagedServiceEnabledStatus(ns string) status.StatusReporter {
	return status.DeploymentStatus{
		NamespacedName: types.NamespacedName{Name: "managed-serviceaccount-addon-manager", Namespace: ns},
	}
}

func ManagedServiceDisabledStatus(ns string, resourceList []*unstructured.Unstructured) status.StatusReporter {
	removals := []*unstructured.Unstructured{}
	for _, u := range resourceList {
		removals = append(removals, newUnstructured(
			types.NamespacedName{Name: u.GetName(), Namespace: u.GetNamespace()},
			u.GroupVersionKind(),
		))
	}

	return ToggledOffStatus{
		NamespacedName: types.NamespacedName{Name: "managedservice", Namespace: ns},
		resources:      removals,
	}
}

// ToggledOffStatus fulfills the StatusReporter interface for a toggleable component. It ensures all resources are removed
type ToggledOffStatus struct {
	types.NamespacedName
	resources []*unstructured.Unstructured
}

func (ts ToggledOffStatus) GetName() string {
	return ts.Name
}

func (ts ToggledOffStatus) GetNamespace() string {
	return ts.Namespace
}

func (ts ToggledOffStatus) GetKind() string {
	return "Component"
}

// Converts this component's status to a backplane component status
func (ts ToggledOffStatus) Status(k8sClient client.Client) bpv1.ComponentCondition {
	present := []*unstructured.Unstructured{}
	presentString := ""
	for _, u := range ts.resources {
		err := k8sClient.Get(context.TODO(), types.NamespacedName{
			Name:      u.GetName(),
			Namespace: u.GetNamespace(),
		}, u)

		if errors.IsNotFound(err) {
			continue
		}

		if err != nil {
			return bpv1.ComponentCondition{
				Name:               ts.GetName(),
				Kind:               ts.GetKind(),
				Type:               "Unknown",
				Status:             metav1.ConditionUnknown,
				LastUpdateTime:     metav1.Now(),
				LastTransitionTime: metav1.Now(),
				Reason:             "Error checking status",
				Message:            "Error getting resource",
				Available:          false,
			}
		}

		present = append(present, u)
		resourceName := u.GetName()
		if u.GetNamespace() != "" {
			resourceName = fmt.Sprintf("%s/%s", u.GetNamespace(), resourceName)
		}
		presentString = fmt.Sprintf("%s <%s %s>", presentString, u.GetKind(), resourceName)
	}

	if len(present) == 0 {
		// The good ending
		return bpv1.ComponentCondition{
			Name:               ts.GetName(),
			Kind:               ts.GetKind(),
			Type:               "NotPresent",
			Status:             metav1.ConditionTrue,
			LastUpdateTime:     metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "ComponentDisabled",
			Message:            "No resources present",
			Available:          true,
		}
	} else {
		conditionMessage := fmt.Sprintf("The following resources remain:%s", presentString)
		return bpv1.ComponentCondition{
			Name:               ts.GetName(),
			Kind:               ts.GetKind(),
			Type:               "Uninstalled",
			Status:             metav1.ConditionFalse,
			LastUpdateTime:     metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "ResourcesPresent",
			Message:            conditionMessage,
			Available:          false,
		}
	}
}

func newUnstructured(nn types.NamespacedName, gvk schema.GroupVersionKind) *unstructured.Unstructured {
	u := unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	u.SetName(nn.Name)
	u.SetNamespace((nn.Namespace))
	return &u
}
