/*
Copyright 2021.

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
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	backplanev1alpha1 "github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	"github.com/open-cluster-management/backplane-operator/pkg/deploying"
	renderer "github.com/open-cluster-management/backplane-operator/pkg/rendering"
	status "github.com/open-cluster-management/backplane-operator/pkg/status"
)

// BackplaneConfigReconciler reconciles a BackplaneConfig object
type BackplaneConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	requeuePeriod      = 15 * time.Second
	backplaneFinalizer = "finalizer.backplane.open-cluster-management.io"
)

//+kubebuilder:rbac:groups=backplane.open-cluster-management.io,resources=backplaneconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=backplane.open-cluster-management.io,resources=backplaneconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=backplane.open-cluster-management.io,resources=backplaneconfigs/finalizers,verbs=update

//+kubebuilder:rbac:groups=apiextensions.k8s.io;rbac.authorization.k8s.io;"";apps,resources=deployments;serviceaccounts;customresourcedefinitions;clusterrolebindings;clusterroles,verbs=get;create;update;list

// ClusterManager RBAC
//+kubebuilder:rbac:groups="",resources=configmaps;namespaces;serviceaccounts;services;secrets,verbs=create;get;list;update;watch;patch;delete
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
//+kubebuilder:rbac:groups="";events.k8s.io,resources=events,verbs=create;update;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=create;get;list;update;watch;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings;rolebindings,verbs=create;get;list;update;watch;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;roles,verbs=create;get;list;update;watch;patch;delete;escalate;bind
//+kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=create;get;list;update;watch;patch;delete
//+kubebuilder:rbac:groups=apiregistration.k8s.io,resources=apiservices,verbs=create;get;list;update;watch;patch;delete
//+kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations;mutatingwebhookconfigurations,verbs=create;get;list;update;watch;patch;delete
//+kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=clustermanagers,verbs=get;list;watch;update;delete
//+kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=clustermanagers/status,verbs=update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the BackplaneConfig object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *BackplaneConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (retQueue ctrl.Result, retError error) {
	log := log.FromContext(ctx)

	// Fetch the BackplaneConfig instance
	backplaneConfig, err := r.getBackPlaneConfig(req)
	if err != nil && !errors.IsNotFound(err) {
		// Unknown error. Requeue
		log.Info("Failed to fetch backplaneConfig")
		return ctrl.Result{RequeueAfter: requeuePeriod}, err
	} else if err != nil && errors.IsNotFound(err) {
		// BackplaneConfig deleted or not found
		// Return and don't requeue
		return ctrl.Result{}, nil
	}

	statusWatcher, err := r.initializeStatus(backplaneConfig)
	if err != nil {
		// Failed initializing status watcher
		return ctrl.Result{RequeueAfter: requeuePeriod}, err
	}

	defer func() {
		status, shouldUpdate := statusWatcher.SyncStatus()
		if shouldUpdate {
			log.Info("Updating status")
			// Status has changed
			result, err := r.updateStatus(backplaneConfig, status)
			if err != nil {
				log.Error(retError, "Error updating status")
			}
			if empty := (ctrl.Result{}); retQueue == empty {
				retQueue = result
			}
			if retError == nil {
				retError = err
			}
		}
	}()

	// If deletion detected, finalize backplane config
	if backplaneConfig.GetDeletionTimestamp() != nil {
		errs := r.finalizeBackplaneConfig(backplaneConfig) // returns all errors
		if len(errs) > 0 {
			for _, err := range errs {
				log.Info(err.Error()) // Log all errors and requeue
			}
			return ctrl.Result{RequeueAfter: requeuePeriod}, errs[0]
		}
		return ctrl.Result{}, nil // Object finalized successfully
	}

	// Add finalizer for this CR
	if !contains(backplaneConfig.GetFinalizers(), backplaneFinalizer) {
		if err := r.addFinalizer(backplaneConfig); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Render CRD templates
	crds, errs := renderer.RenderCRDs()
	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Deploy CRDs
	log.Info("Deploying CustomResourceDefinitions")
	for _, crdToDeploy := range crds {
		_, err := deploying.Deploy(r.Client, crdToDeploy)
		if err != nil {
			return ctrl.Result{RequeueAfter: requeuePeriod}, err
		}
	}

	// Render Templates
	templates, errs := renderer.RenderTemplates(backplaneConfig)
	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	log.Info("Deploying Templates")
	for _, template := range templates {
		_, err := deploying.Deploy(r.Client, template)
		if err != nil {
			return ctrl.Result{RequeueAfter: requeuePeriod}, err
		}
	}

	return retQueue, retError
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackplaneConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&backplanev1alpha1.BackplaneConfig{}).
		Complete(r)
}

func (r *BackplaneConfigReconciler) getBackPlaneConfig(req ctrl.Request) (*backplanev1alpha1.BackplaneConfig, error) {
	log := log.FromContext(context.Background())
	backplaneConfig := &backplanev1alpha1.BackplaneConfig{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, backplaneConfig)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("BackplaneConfig resource not found. Ignoring since object must be deleted")
			return nil, err
		}
		// Error reading the object - requeue the request.
		return nil, err
	}
	return backplaneConfig, nil
}

func (r *BackplaneConfigReconciler) finalizeBackplaneConfig(backplaneConfig *backplanev1alpha1.BackplaneConfig) []error {
	log := log.FromContext(context.Background())
	if contains(backplaneConfig.GetFinalizers(), backplaneFinalizer) {
		// Run finalization logic
		log.Info("Deleting all templated resources")
		templates, errs := renderer.RenderTemplates(backplaneConfig)
		for _, template := range templates {
			err := r.Client.Delete(context.Background(), template)
			if err != nil && !errors.IsNotFound(err) {
				errs = append(errs, err)
			}
		}
		if len(errs) > 0 {
			return errs
		}

		log.Info("Deleting all packaged CustomResourceDefinitions")
		crds, errs := renderer.RenderCRDs()
		for _, crd := range crds {
			err := r.Client.Delete(context.Background(), crd)
			if err != nil && !errors.IsNotFound(err) {
				errs = append(errs, err)
			}
		}
		if len(errs) > 0 {
			return errs
		}

		// Remove finalizer. Once all finalizers have been
		// removed, the object will be deleted.
		backplaneConfig.SetFinalizers(remove(backplaneConfig.GetFinalizers(), backplaneFinalizer))

		err := r.Client.Update(context.Background(), backplaneConfig)
		if err != nil {
			return append(errs, err)
		}
	}
	return nil
}

func (r *BackplaneConfigReconciler) addFinalizer(backplaneConfig *backplanev1alpha1.BackplaneConfig) error {
	log := log.FromContext(context.Background())
	backplaneConfig.SetFinalizers(append(backplaneConfig.GetFinalizers(), backplaneFinalizer))
	// Update CR
	err := r.Client.Update(context.TODO(), backplaneConfig)
	if err != nil {
		log.Error(err, "Failed to update BackplaneConfig with finalizer")
		return err
	}
	return nil
}

func (r *BackplaneConfigReconciler) initializeStatus(backplaneConfig *v1alpha1.BackplaneConfig) (status.StatusWatcher, error) {
	deploymentList := &appsv1.DeploymentList{}
	err := r.Client.List(context.Background(), deploymentList, []client.ListOption{
		client.InNamespace(backplaneConfig.Namespace),
		client.MatchingLabels{"open-cluster-management.backplane-operator.name": backplaneConfig.Name},
		client.MatchingLabels{"open-cluster-management.backplane-operator.namespace": backplaneConfig.Namespace},
	}...)
	if err != nil {
		return status.StatusWatcher{}, err
	}
	return status.StatusWatcher{
		BackplaneConfig: backplaneConfig,
		OriginalStatus:  backplaneConfig.Status,
		Deployments:     deploymentList,
	}, nil
}

func (r *BackplaneConfigReconciler) updateStatus(bpc *v1alpha1.BackplaneConfig, newStatus v1alpha1.BackplaneConfigStatus) (ctrl.Result, error) {
	log := log.FromContext(context.Background())
	newBackPlaneConfig := bpc
	newBackPlaneConfig.Status = newStatus
	err := r.Client.Status().Update(context.TODO(), newBackPlaneConfig)
	if err != nil {
		if errors.IsConflict(err) {
			// Error from object being modified is normal behavior and should not be treated like an error
			log.Info("Failed to update status", "Reason", "Object has been modified")
			return ctrl.Result{RequeueAfter: requeuePeriod}, nil
		}

		log.Error(err, fmt.Sprintf("Failed to update %s/%s status ", bpc.Namespace, bpc.Name))
		return ctrl.Result{}, err
	}

	if bpc.Status.Phase != v1alpha1.Running {
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	} else {
		return ctrl.Result{}, nil
	}
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
