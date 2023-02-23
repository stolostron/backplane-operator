// Copyright Contributors to the Open Cluster Management project
package status

import (
	"context"
	"fmt"

	bpv1 "github.com/stolostron/backplane-operator/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StaticStatus fulfills the StatusReporter interface for static predefined statuses
type StaticStatus struct {
	types.NamespacedName
	Kind      string
	Condition bpv1.ComponentCondition
}

func (s StaticStatus) GetName() string {
	return s.Name
}

func (s StaticStatus) GetNamespace() string {
	return s.Namespace
}

func (s StaticStatus) GetKind() string {
	return s.Kind
}

// Converts a deployment's status to a backplane component status
func (s StaticStatus) Status(k8sClient client.Client) bpv1.ComponentCondition {
	cc := s.Condition
	if cc.Kind == "" {
		cc.Kind = "Component"
	}
	return cc
}

func NewDisabledStatus(namespacedName types.NamespacedName, explanation string, resourceList []*unstructured.Unstructured) StatusReporter {
	removals := []*unstructured.Unstructured{}
	for _, u := range resourceList {
		removals = append(removals, newUnstructured(
			types.NamespacedName{Name: u.GetName(), Namespace: u.GetNamespace()},
			u.GroupVersionKind(),
		))
	}

	return DisabledStatus{
		NamespacedName: namespacedName,
		resources:      removals,
		Message:        explanation,
	}
}

// DisabledStatus fulfills the StatusReporter interface for a component that should not be present. It ensures all resources are removed.
// If in desired status will explain why with reason and message.
type DisabledStatus struct {
	types.NamespacedName
	// Explanation for why component is disabled
	Message string
	// Resource list to check for presence
	resources []*unstructured.Unstructured
}

func (s DisabledStatus) GetName() string {
	return s.Name
}

func (s DisabledStatus) GetNamespace() string {
	return s.Namespace
}

func (s DisabledStatus) GetKind() string {
	return "Component"
}

// Converts this component's status to a backplane component status
func (s DisabledStatus) Status(k8sClient client.Client) bpv1.ComponentCondition {
	present := []*unstructured.Unstructured{}
	presentString := ""
	for _, u := range s.resources {
		err := k8sClient.Get(context.TODO(), types.NamespacedName{
			Name:      u.GetName(),
			Namespace: u.GetNamespace(),
		}, u)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return bpv1.ComponentCondition{
				Name:               s.GetName(),
				Kind:               s.GetKind(),
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
			Name:      s.GetName(),
			Kind:      s.GetKind(),
			Type:      "NotPresent",
			Status:    metav1.ConditionTrue,
			Reason:    ComponentDisabledReason,
			Message:   s.Message,
			Available: true,
		}
	} else {
		conditionMessage := fmt.Sprintf("The following resources remain:%s", presentString)
		return bpv1.ComponentCondition{
			Name:      s.GetName(),
			Kind:      s.GetKind(),
			Type:      "NotPresent",
			Status:    metav1.ConditionFalse,
			Reason:    "ResourcesPresent",
			Message:   fmt.Sprintf("%s. %s", s.Message, conditionMessage),
			Available: false,
		}
	}
}

func NewPresentStatus(namespacedName types.NamespacedName, gvk schema.GroupVersionKind) StatusReporter {
	return PresentStatus{
		NamespacedName: namespacedName,
		gvk:            gvk,
	}
}

// PresentStatus fulfills the StatusReporter interface for a component that should be present. It ensures all resources exist.
// If in desired status will explain why with reason and message.
type PresentStatus struct {
	types.NamespacedName
	// Resource type
	gvk schema.GroupVersionKind
}

func (s PresentStatus) GetName() string {
	return s.Name
}

func (s PresentStatus) GetNamespace() string {
	return s.Namespace
}

func (s PresentStatus) GetKind() string {
	return s.gvk.Kind
}

// Converts this component's status to a backplane component status
func (s PresentStatus) Status(k8sClient client.Client) bpv1.ComponentCondition {
	u := newUnstructured(s.NamespacedName, s.gvk)
	err := k8sClient.Get(context.TODO(), types.NamespacedName{
		Name:      u.GetName(),
		Namespace: u.GetNamespace(),
	}, u)

	if err == nil {
		return bpv1.ComponentCondition{
			Name:      s.GetName(),
			Kind:      s.GetKind(),
			Type:      "Present",
			Status:    metav1.ConditionTrue,
			Reason:    DeploySuccessReason,
			Available: true,
		}
	}

	// Recognized error response
	if apimeta.IsNoMatchError(err) || apierrors.IsNotFound(err) {
		resourceName := u.GetName()
		if u.GetNamespace() != "" {
			resourceName = fmt.Sprintf("%s/%s", s.GetNamespace(), resourceName)
		}
		missingString := fmt.Sprintf("<%s %s>", s.GetKind(), resourceName)
		return bpv1.ComponentCondition{
			Name:      s.GetName(),
			Kind:      s.GetKind(),
			Type:      "Present",
			Status:    metav1.ConditionFalse,
			Reason:    DeployFailedReason,
			Message:   fmt.Sprintf("The following resource is missing: %s", missingString),
			Available: false,
		}
	}

	// Unknown error getting resource
	return bpv1.ComponentCondition{
		Name:               s.GetName(),
		Kind:               s.GetKind(),
		Type:               "Unknown",
		Status:             metav1.ConditionUnknown,
		LastUpdateTime:     metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Reason:             "Error checking status",
		Message:            "Error getting resource",
		Available:          false,
	}
}

func newUnstructured(nn types.NamespacedName, gvk schema.GroupVersionKind) *unstructured.Unstructured {
	u := unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	u.SetName(nn.Name)
	u.SetNamespace(nn.Namespace)
	return &u
}
