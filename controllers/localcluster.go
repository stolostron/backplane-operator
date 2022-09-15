// Copyright Contributors to the Open Cluster Management project

/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"encoding/json"
	"fmt"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// LocalClusterName name of the hub cluster managedcluster resource
	LocalClusterName = "local-cluster"

	// AnnotationNodeSelector key name of nodeSelector annotation synced from mch
	AnnotationNodeSelector = "open-cluster-management/nodeSelector"
)

func newInstallerLabels(mce *backplanev1.MultiClusterEngine) map[string]string {
	labels := make(map[string]string)
	labels["installer.name"] = mce.GetName()
	labels["installer.namespace"] = mce.GetNamespace()
	return labels
}

func newManagedClusterLabels(mce *backplanev1.MultiClusterEngine) map[string]string {
	labels := newInstallerLabels(mce)
	labels["local-cluster"] = "true"
	labels["cloud"] = "auto-detect"
	labels["vendor"] = "auto-detect"
	labels["velero.io/exclude-from-backup"] = "true"
	return labels
}

func newManagedCluster() *unstructured.Unstructured {
	managedCluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": LocalClusterName,
				"labels": map[string]interface{}{
					"local-cluster":                 "true",
					"cloud":                         "auto-detect",
					"vendor":                        "auto-detect",
					"velero.io/exclude-from-backup": "true",
				},
			},
			"spec": map[string]interface{}{
				"hubAcceptsClient": true,
			},
		},
	}
	return managedCluster
}

func newManagedClusterWithInstallerLabels(mce *backplanev1.MultiClusterEngine) *unstructured.Unstructured {
	managedCluster := newManagedCluster()
	labels := managedCluster.GetLabels()
	installerLabels := newInstallerLabels(mce)
	for k, v := range installerLabels {
		labels[k] = v
	}
	managedCluster.SetLabels(labels)
	return managedCluster
}

func newLocalNamespace() *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: LocalClusterName,
		},
	}
}

func newLocalNamespaceWithInstallerLabels(mce *backplanev1.MultiClusterEngine) *corev1.Namespace {
	ns := newLocalNamespace()
	labels := ns.GetLabels()
	installerLabels := newInstallerLabels(mce)
	for k, v := range installerLabels {
		labels[k] = v
	}
	ns.SetLabels(labels)
	return ns
}

func (r *MultiClusterEngineReconciler) CreateManagedCluster(ctx context.Context, mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.Info("Check if ManagedCluster CR exists")
	managedCluster := newManagedCluster()
	err := r.Client.Get(ctx, types.NamespacedName{Name: LocalClusterName}, managedCluster)
	if errors.IsNotFound(err) {
		log.Info("ManagedCluster CR does not exist, need to create it")
		log.Info("Check if local cluster namespace %q exists", LocalClusterName)
		localNS := newLocalNamespace()
		localNS.SetLabels(newInstallerLabels(mce))
		err := r.Client.Get(ctx, types.NamespacedName{Name: localNS.GetName()}, localNS)
		if err == nil {
			log.Info("Waiting on local cluster namespace to be removed before creating ManagedCluster CR", "Namespace", localNS.GetName())
			return ctrl.Result{RequeueAfter: requeuePeriod}, nil
		} else if errors.IsNotFound(err) {
			log.Info("Local cluster namespace does not exist. Creating ManagedCluster CR")
			managedCluster = newManagedClusterWithInstallerLabels(mce)
			err := r.Client.Create(ctx, managedCluster)
			if err != nil {
				log.Error(err, "Failed to create ManagedCluster CR")
				return ctrl.Result{}, err
			}
			log.Info("Created ManagedCluster CR")
		} else {
			log.Error(err, "Failed to get local cluster namespace")
			return ctrl.Result{}, err
		}
	} else if err != nil {
		log.Error(err, "Failed to get ManagedCluster CR")
		return ctrl.Result{RequeueAfter: requeuePeriod}, err
	}

	log.Info("Setting installer labels on ManagedCluster CR")
	labels := newManagedClusterLabels(mce)
	for k, v := range managedCluster.GetLabels() {
		labels[k] = v
	}
	managedCluster.SetLabels(labels)

	log.Info("Setting annotations on ManagedCluster CR")
	annotations := managedCluster.GetAnnotations()
	if len(mce.Spec.NodeSelector) > 0 {
		log.Info("Adding NodeSelector annotation")
		nodeSelector, err := json.Marshal(mce.Spec.NodeSelector)
		if err != nil {
			log.Error(err, "Failed to json marshal MCE NodeSelector")
			return ctrl.Result{}, err
		}
		annotations[AnnotationNodeSelector] = string(nodeSelector)
	} else {
		log.Info("Removing NodeSelector annotation")
		delete(annotations, AnnotationNodeSelector)
	}
	managedCluster.SetAnnotations(annotations)

	log.Info("Updating ManagedCluster CR")
	err = r.Client.Update(ctx, managedCluster)
	if err != nil {
		log.Error(err, "Failed to update ManagedCluster CR")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, err
}

func (r *MultiClusterEngineReconciler) DeleteManagedCluster(ctx context.Context, mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.Info("Check if ManagedCluster CR exists")
	managedCluster := newManagedCluster()
	err := r.Client.Get(ctx, types.NamespacedName{Name: LocalClusterName}, managedCluster)
	if errors.IsNotFound(err) {
		log.Info("ManagedCluster CR has been removed")
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "Failed to get ManagedCluster CR")
		return ctrl.Result{RequeueAfter: requeuePeriod}, err
	} else {
		log.Info("ManagedCluster CR still exists")
	}

	log.Info("Deleting ManagedCluster CR")
	managedCluster = newManagedCluster()
	err = r.Client.Delete(ctx, managedCluster)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Error deleting ManagedCluster CR")
		return ctrl.Result{}, err
	}

	log.Info("ManagedCluster CR has been deleted")
	return ctrl.Result{RequeueAfter: requeuePeriod}, nil
}

func (r *MultiClusterEngineReconciler) DeleteManagedClusterNamespace(ctx context.Context, mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.Info("Check if managed cluster namespace exists")
	ns := newLocalNamespaceWithInstallerLabels(mce)
	err := r.Client.Get(ctx, types.NamespacedName{Name: ns.GetName()}, ns)
	if errors.IsNotFound(err) {
		log.Info("Managed cluster namespace has been removed")
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "Failed to get managed cluster namespace")
		return ctrl.Result{RequeueAfter: requeuePeriod}, err
	} else {
		log.Info("Managed clsuter namespace still exists")
	}

	log.Info("Deleting managed cluster namespace")
	ns = newLocalNamespaceWithInstallerLabels(mce)
	err = r.Client.Delete(ctx, ns)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Error deleting managed cluster ns")
		return ctrl.Result{}, err
	}

	log.Info("Managed cluster namespace has been deleted")
	return ctrl.Result{RequeueAfter: requeuePeriod}, nil
}

func (r *MultiClusterEngineReconciler) ImportLocalCluster(ctx context.Context, mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.Info("Check if MCE components are available")
	err := r.StatusManager.AllComponentsReady()
	if err != nil {
		msg := fmt.Sprintf("Components not ready while importing local cluster: %s", err.Error())
		log.Info(msg)
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	log.Info("Ensure ManagedCluster CR is created")
	result, err := r.CreateManagedCluster(ctx, mce)

	return result, err
}

func (r *MultiClusterEngineReconciler) DetachLocalCluster(ctx context.Context, mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.Info("Delete ManagedCluster CR")
	result, err := r.DeleteManagedCluster(ctx, mce)
	if err != nil {
		log.Error(err, "Error while deleting ManagedCluster CR")
		return result, err
	} else if result != (ctrl.Result{}) {
		msg := "Waiting for local managed cluster to terminate."
		condition := status.NewCondition(
			backplanev1.MultiClusterEngineProgressing,
			metav1.ConditionTrue,
			status.ManagedClusterTerminatingReason,
			msg,
		)
		r.StatusManager.AddCondition(condition)
		log.Info(msg)
		return result, err
	}

	log.Info("Delete managed cluster namespace")
	result, err = r.DeleteManagedClusterNamespace(ctx, mce)
	if err != nil {
		log.Error(err, "Error while deleting managed cluster namespace")
		return result, err
	} else if result != (ctrl.Result{}) {
		msg := "Waiting for local managed cluster namespace to terminate."
		condition := status.NewCondition(
			backplanev1.MultiClusterEngineProgressing,
			metav1.ConditionTrue,
			status.NamespaceTerminatingReason,
			msg,
		)
		r.StatusManager.AddCondition(condition)
		log.Info(msg)
		return result, err
	}

	return result, err
}
