// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"fmt"

	backplanev1alpha1 "github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	"github.com/open-cluster-management/backplane-operator/pkg/foundation"
	"github.com/open-cluster-management/backplane-operator/pkg/utils"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ensureUnstructuredResource ensures that the unstructured resource is applied in the cluster properly
func (r *MultiClusterEngineReconciler) ensureUnstructuredResource(bpc *backplanev1alpha1.MultiClusterEngine, u *unstructured.Unstructured) (ctrl.Result, error) {
	ctx := context.Background()
	log := log.FromContext(ctx)

	found := &unstructured.Unstructured{}
	found.SetGroupVersionKind(u.GroupVersionKind())

	utils.AddBackplaneConfigLabels(u, bpc.Name)

	// Try to get API group instance
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      u.GetName(),
		Namespace: u.GetNamespace(),
	}, found)
	if err != nil && errors.IsNotFound(err) {
		// Resource doesn't exist so create it
		err := r.Client.Create(ctx, u)
		if err != nil {
			// Creation failed
			log.Error(err, "Failed to create new instance")
			return ctrl.Result{}, err
		}
		// Creation was successful
		log.Info(fmt.Sprintf("Created new resource - kind: %s name: %s", u.GetKind(), u.GetName()))
		// condition := NewHubCondition(operatorsv1.Progressing, metav1.ConditionTrue, NewComponentReason, "Created new resource")
		// SetHubCondition(&m.Status, *condition)
		return ctrl.Result{}, nil

	} else if err != nil {
		// Error that isn't due to the resource not existing
		log.Error(err, "Failed to get resource")
		return ctrl.Result{}, err
	}

	// Validate object based on name
	var desired *unstructured.Unstructured
	var needsUpdate bool

	switch found.GetKind() {
	case "ClusterManager":
		desired, needsUpdate = foundation.ValidateSpec(found, u)
	case "ClusterRole":
		desired, needsUpdate = utils.ValidateClusterRoleRules(found, u)
	case "Deployment":
		desired = u
		needsUpdate = true
	case "CustomResourceDefinition", "HiveConfig":
		// skip update
		return ctrl.Result{}, nil
	default:
		log.Info("Could not validate unstructured resource. Skipping update.", "Type", found.GetKind())
		return ctrl.Result{}, nil
	}

	if needsUpdate {
		log.Info(fmt.Sprintf("Updating %s - %s", desired.GetKind(), desired.GetName()))
		err = r.Client.Update(ctx, desired)
		if err != nil {
			log.Error(err, "Failed to update resource.")
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func remove(list []string, s string) []string {
	for i, v := range list {
		if v == s {
			list = append(list[:i], list[i+1:]...)
		}
	}
	return list
}
