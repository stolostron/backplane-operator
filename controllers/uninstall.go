// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"fmt"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/status"
	"github.com/stolostron/backplane-operator/pkg/toggle"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	// The uninstallList is the list of all resources from previous installs to remove. Items can be removed
	// from this list in future releases if they are sure to not exist prior to the current installer version
	uninstallList = func(backplaneConfig *backplanev1.MultiClusterEngine) []*unstructured.Unstructured {
		removals := []*unstructured.Unstructured{
			newUnstructured(
				types.NamespacedName{Name: "hypershift-deployment", Namespace: backplaneConfig.Spec.TargetNamespace},
				schema.GroupVersionKind{Group: "", Kind: "ServiceAccount", Version: "v1"},
			),
			newUnstructured(
				types.NamespacedName{Name: "hypershift-deployment-controller", Namespace: backplaneConfig.Spec.TargetNamespace},
				schema.GroupVersionKind{Group: "apps", Kind: "Deployment", Version: "v1"},
			),
			newUnstructured(
				types.NamespacedName{Name: "open-cluster-management:hypershift-preview:hypershiftDeployment-leader-election", Namespace: backplaneConfig.Spec.TargetNamespace},
				schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Kind: "Role", Version: "v1"},
			),
			newUnstructured(
				types.NamespacedName{Name: "open-cluster-management:hypershift-preview:hypershiftDeployment-leader-election", Namespace: backplaneConfig.Spec.TargetNamespace},
				schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Kind: "RoleBinding", Version: "v1"},
			),
			newUnstructured(
				types.NamespacedName{Name: "hypershiftdeployments.cluster.open-cluster-management.io"},
				schema.GroupVersionKind{Group: "apiextensions.k8s.io", Kind: "CustomResourceDefinition", Version: "v1"},
			),
			newUnstructured(
				types.NamespacedName{Name: "open-cluster-management:hypershift-preview:hypershift-deployment-controller"},
				schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Kind: "ClusterRole", Version: "v1"},
			),
			newUnstructured(
				types.NamespacedName{Name: "open-cluster-management:hypershift-preview:hypershift-deployment-controller"},
				schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Kind: "ClusterRoleBinding", Version: "v1"},
			),
		}

		return removals
	}
)

func newUnstructured(nn types.NamespacedName, gvk schema.GroupVersionKind) *unstructured.Unstructured {
	u := unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	u.SetName(nn.Name)
	u.SetNamespace((nn.Namespace))
	return &u
}

// ensureRemovalsGone validates successful removal of everything in the uninstallList. Return on first error encounter.
func (r *MultiClusterEngineReconciler) ensureRemovalsGone(backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	removals := uninstallList(backplaneConfig)

	namespacedName := types.NamespacedName{Name: "hypershift-removals", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, removals))

	allResourcesDeleted := true
	for i := range removals {
		gone, err := r.uninstall(backplaneConfig, removals[i])
		if err != nil {
			return ctrl.Result{}, err
		}
		if !gone {
			allResourcesDeleted = false
		}
	}

	if !allResourcesDeleted {
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Emit hubcondition once pruning complete if other pruning condition present
	progressingCondition := status.GetCondition(r.StatusManager.Conditions, backplanev1.MultiClusterEngineProgressing)
	if progressingCondition != nil {
		if progressingCondition.Reason == status.OldComponentRemovedReason || progressingCondition.Reason == status.OldComponentNotRemovedReason {
			r.StatusManager.AddCondition(status.NewCondition(backplanev1.MultiClusterEngineProgressing, metav1.ConditionTrue, status.AllOldComponentsRemovedReason, "All old resources pruned"))
		}
	}

	return ctrl.Result{}, nil
}

// uninstall return true if resource does not exist and returns an error if a GET or DELETE errors unexpectedly. A false response without error
// means the resource is in the process of deleting.
func (r *MultiClusterEngineReconciler) uninstall(backplaneConfig *backplanev1.MultiClusterEngine, u *unstructured.Unstructured) (bool, error) {

	err := r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      u.GetName(),
		Namespace: u.GetNamespace(),
	}, u)

	if errors.IsNotFound(err) {
		return true, nil
	}

	// Get resource. Successful if it doesn't exist.
	if err != nil {
		// Error that isn't due to the resource not existing
		return false, err
	}

	// If resource has deletionTimestamp then re-reconcile and don't try deleting
	if u.GetDeletionTimestamp() != nil {
		r.StatusManager.AddCondition(status.NewCondition(backplanev1.MultiClusterEngineProgressing, metav1.ConditionUnknown, status.OldComponentNotRemovedReason, fmt.Sprintf("Resource %s/%s finalizing", u.GetKind(), u.GetName())))
		return false, nil
	}

	// Attempt deleting resource. No error does not necessarily mean the resource is gone.
	err = r.Client.Delete(context.TODO(), u)
	if err != nil {
		r.StatusManager.AddCondition(status.NewCondition(backplanev1.MultiClusterEngineProgressing, metav1.ConditionUnknown, status.OldComponentNotRemovedReason, fmt.Sprintf("Failed to remove resource %s/%s", u.GetKind(), u.GetName())))
		return false, err
	}
	r.StatusManager.AddCondition(status.NewCondition(backplanev1.MultiClusterEngineProgressing, metav1.ConditionUnknown, status.OldComponentRemovedReason, "Removed old resource"))
	return false, nil
}
