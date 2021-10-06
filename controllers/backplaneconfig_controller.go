// Copyright Contributors to the Open Cluster Management project

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
	e "errors"
	"fmt"
	"time"

	"k8s.io/client-go/util/workqueue"
	clustermanager "open-cluster-management.io/api/operator/v1"

	hiveconfig "github.com/openshift/hive/apis/hive/v1"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	backplanev1alpha1 "github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	"github.com/open-cluster-management/backplane-operator/pkg/foundation"
	"github.com/open-cluster-management/backplane-operator/pkg/hive"
	renderer "github.com/open-cluster-management/backplane-operator/pkg/rendering"
	"github.com/open-cluster-management/backplane-operator/pkg/status"

	"github.com/open-cluster-management/backplane-operator/pkg/utils"
)

// MultiClusterEngineReconciler reconciles a MultiClusterEngine object
type MultiClusterEngineReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Images        map[string]string
	StatusManager *status.StatusTracker
}

const (
	requeuePeriod      = 15 * time.Second
	backplaneFinalizer = "finalizer.multicluster.openshift.io"
)

//+kubebuilder:rbac:groups=multicluster.openshift.io,resources=multiclusterengines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=multicluster.openshift.io,resources=multiclusterengines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=multicluster.openshift.io,resources=multiclusterengines/finalizers,verbs=update
//+kubebuilder:rbac:groups=apiextensions.k8s.io;rbac.authorization.k8s.io;"";apps,resources=deployments;serviceaccounts;customresourcedefinitions;clusterrolebindings;clusterroles,verbs=get;create;update;list

// ClusterManager RBAC
//+kubebuilder:rbac:groups="",resources=configmaps;namespaces;serviceaccounts;services;secrets,verbs=create;get;list;update;watch;patch;delete
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
//+kubebuilder:rbac:groups="";events.k8s.io,resources=events,verbs=create;update;patch
//+kubebuilder:rbac:groups=apps,resources=deployments;replicasets,verbs=create;get;list;update;watch;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings;rolebindings,verbs=create;get;list;update;watch;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;roles,verbs=create;get;list;update;watch;patch;delete;escalate;bind
//+kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=create;get;list;update;watch;patch;delete
//+kubebuilder:rbac:groups=apiregistration.k8s.io,resources=apiservices,verbs=create;get;list;update;watch;patch;delete
//+kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations;mutatingwebhookconfigurations,verbs=create;get;list;update;watch;patch;delete
//+kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=clustermanagers,verbs=create;get;list;watch;update;delete;patch
//+kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=clustermanagers/status,verbs=update;patch
//+kubebuilder:rbac:groups=imageregistry.open-cluster-management.io,resources=managedclusterimageregistries;managedclusterimageregistries/status,verbs=approve;bind;create;delete;deletecollection;escalate;get;list;patch;update;watch
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io;inventory.open-cluster-management.io;operator.open-cluster-management.io,resources=managedclusters;baremetalassets;multiclusterhubs,verbs=get;list;watch
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersets/join,verbs=create
//+kubebuilder:rbac:groups=migration.k8s.io,resources=storageversionmigrations,verbs=create;get;list;update;patch;watch;delete
//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=create;get;list;update;patch;watch;delete

// Hive RBAC
//+kubebuilder:rbac:groups="hive.openshift.io",resources=hiveconfigs,verbs=get;create;update;delete;list;watch
//+kubebuilder:rbac:groups="hive.openshift.io",resources=clusterdeployments;clusterpools;clusterclaims;machinepools,verbs=approve;bind;create;delete;deletecollection;escalate;get;list;patch;update;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *MultiClusterEngineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (retRes ctrl.Result, retErr error) {
	log := log.FromContext(ctx)
	// Fetch the BackplaneConfig instance
	backplaneConfig, err := r.getBackplaneConfig(ctx, req)
	if err != nil && !apierrors.IsNotFound(err) {
		// Unknown error. Requeue
		log.Info("Failed to fetch backplaneConfig")
		return ctrl.Result{RequeueAfter: requeuePeriod}, err
	} else if err != nil && apierrors.IsNotFound(err) {
		// BackplaneConfig deleted or not found
		// Return and don't requeue
		return ctrl.Result{}, nil
	}

	// reset status manager if req is a new MCE
	uid := string(backplaneConfig.UID)
	if uid == "" {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, e.New("Resource missing UID")
	}
	if r.StatusManager.UID == "" || r.StatusManager.UID != string(backplaneConfig.UID) {
		log.Info("Setting status manager to track new MCE", "UID", string(backplaneConfig.UID))
		r.StatusManager.Reset(string(backplaneConfig.UID))
	}

	defer func() {
		log.Info("Updating status")
		backplaneConfig.Status = r.StatusManager.ReportStatus(*backplaneConfig)
		err := r.Client.Status().Update(ctx, backplaneConfig)
		if backplaneConfig.Status.Phase != backplanev1alpha1.MultiClusterEnginePhaseAvailable && !utils.IsPaused(backplaneConfig) {
			retRes = ctrl.Result{RequeueAfter: 10 * time.Second}
		}
		if err != nil {
			retErr = err
		}
	}()

	// If deletion detected, finalize backplane config
	if backplaneConfig.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(backplaneConfig, backplaneFinalizer) {
			err := r.finalizeBackplaneConfig(ctx, backplaneConfig) // returns all errors
			if err != nil {
				log.Info(err.Error())
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}

			log.Info("all subcomponents have been finalized successfully - removing finalizer")
			controllerutil.RemoveFinalizer(backplaneConfig, backplaneFinalizer)
			if err := r.Client.Update(ctx, backplaneConfig); err != nil {
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil // Object finalized successfully
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(backplaneConfig, backplaneFinalizer) {
		controllerutil.AddFinalizer(backplaneConfig, backplaneFinalizer)
		if err := r.Client.Update(ctx, backplaneConfig); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Read image overrides from environmental variables
	r.Images = utils.GetImageOverrides(backplaneConfig)
	if len(r.Images) == 0 {
		// If imageoverrides are not set from environmental variables, fail
		r.StatusManager.AddCondition(status.NewCondition(backplanev1alpha1.MultiClusterEngineProgressing, metav1.ConditionFalse, status.RequirementsNotMetReason, "No image references defined in deployment"))
		return ctrl.Result{RequeueAfter: requeuePeriod}, e.New("no image references exist. images must be defined as environment variables")
	}

	// Do not reconcile objects if this instance of mce is labeled "paused"
	if utils.IsPaused(backplaneConfig) {
		log.Info("MultiClusterEngine reconciliation is paused. Nothing more to do.")
		r.StatusManager.AddCondition(status.NewCondition(backplanev1alpha1.MultiClusterEngineProgressing, metav1.ConditionUnknown, status.PausedReason, "Multiclusterengine is paused"))
		return ctrl.Result{}, nil
	}

	result, err := r.DeploySubcomponents(ctx, backplaneConfig)
	if err != nil {
		r.StatusManager.AddCondition(status.NewCondition(backplanev1alpha1.MultiClusterEngineProgressing, metav1.ConditionUnknown, status.DeployFailedReason, err.Error()))
		return result, err
	}
	r.StatusManager.AddCondition(status.NewCondition(backplanev1alpha1.MultiClusterEngineProgressing, metav1.ConditionTrue, status.DeploySuccessReason, "All components deployed"))

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MultiClusterEngineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&backplanev1alpha1.MultiClusterEngine{}).
		WithEventFilter(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{}, predicate.AnnotationChangedPredicate{})).
		Watches(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
			OwnerType: &backplanev1alpha1.MultiClusterEngine{},
		}).
		Watches(&source.Kind{Type: &hiveconfig.HiveConfig{}}, &handler.Funcs{
			DeleteFunc: func(e event.DeleteEvent, q workqueue.RateLimitingInterface) {
				labels := e.Object.GetLabels()
				q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
					Name: labels["backplaneconfig.name"],
				}})
			},
		}, builder.WithPredicates(predicate.LabelChangedPredicate{})).
		Watches(&source.Kind{Type: &clustermanager.ClusterManager{}}, &handler.Funcs{
			DeleteFunc: func(e event.DeleteEvent, q workqueue.RateLimitingInterface) {
				labels := e.Object.GetLabels()
				q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
					Name: labels["backplaneconfig.name"],
				}})
			},
			UpdateFunc: func(e event.UpdateEvent, q workqueue.RateLimitingInterface) {
				labels := e.ObjectOld.GetLabels()
				q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
					Name: labels["backplaneconfig.name"],
				}})
			},
		}, builder.WithPredicates(predicate.LabelChangedPredicate{})).
		Complete(r)
}

// DeploySubcomponents ensures all subcomponents exist
func (r *MultiClusterEngineReconciler) DeploySubcomponents(ctx context.Context, backplaneConfig *backplanev1alpha1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Renders all templates from charts
	templates, errs := renderer.RenderTemplates(backplaneConfig, r.Images)
	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Applies all templates
	for _, template := range templates {
		if template.GetKind() == "Deployment" {
			r.StatusManager.AddComponent(status.DeploymentStatus{
				NamespacedName: types.NamespacedName{Name: template.GetName(), Namespace: template.GetNamespace()},
			})
		}

		// Set owner reference.
		err := ctrl.SetControllerReference(backplaneConfig, template, r.Scheme)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "Error setting controller reference on resource %s", template.GetName())
		}

		if template.GetKind() == "APIService" {
			result, err := r.ensureUnstructuredResource(ctx, backplaneConfig, template)
			if err != nil {
				return result, err
			}
		} else {
			// Apply the object data.
			force := true
			err = r.Client.Patch(ctx, template, client.Apply, &client.PatchOptions{Force: &force, FieldManager: "backplane-operator"})
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "error applying object Name: %s Kind: %s", template.GetName(), template.GetKind())
			}
		}

	}

	result, err := r.ensureCustomResources(ctx, backplaneConfig)
	if err != nil {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureCustomResources(ctx context.Context, backplaneConfig *backplanev1alpha1.MultiClusterEngine) (ctrl.Result, error) {

	cmTemplate := foundation.ClusterManager(backplaneConfig, r.Images)
	if err := ctrl.SetControllerReference(backplaneConfig, cmTemplate, r.Scheme); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Error setting controller reference on resource %s", cmTemplate.GetName())
	}
	force := true
	err := r.Client.Patch(ctx, cmTemplate, client.Apply, &client.PatchOptions{Force: &force, FieldManager: "backplane-operator"})
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error applying object Name: %s Kind: %s", cmTemplate.GetName(), cmTemplate.GetKind())
	}

	r.StatusManager.AddComponent(status.ClusterManagerStatus{
		NamespacedName: types.NamespacedName{Name: "cluster-manager"},
	})

	hiveTemplate := hive.HiveConfig(backplaneConfig)
	if err := ctrl.SetControllerReference(backplaneConfig, hiveTemplate, r.Scheme); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Error setting controller reference on resource %s", hiveTemplate.GetName())
	}

	result, err := r.ensureUnstructuredResource(ctx, backplaneConfig, hiveTemplate)
	if err != nil {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) finalizeBackplaneConfig(ctx context.Context, backplaneConfig *backplanev1alpha1.MultiClusterEngine) error {
	log := log.FromContext(ctx)

	clusterManager := &unstructured.Unstructured{}
	clusterManager.SetGroupVersionKind(
		schema.GroupVersionKind{
			Group:   "operator.open-cluster-management.io",
			Version: "v1",
			Kind:    "ClusterManager",
		},
	)

	err := r.Client.Get(ctx, types.NamespacedName{Name: "cluster-manager"}, clusterManager)
	if err == nil { // If resource exists, delete
		log.Info("finalizing cluster-manager custom resource")
		err := r.Client.Delete(ctx, clusterManager)
		if err != nil {
			return err
		}
	} else if err != nil && !apierrors.IsNotFound(err) { // Return error, if error is not not found error
		return err
	}

	ocmHubNamespace := &corev1.Namespace{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: "open-cluster-management-hub"}, ocmHubNamespace)
	if err == nil {
		// If wait time exceeds expected then uninstall may not be able to progress
		if time.Since(backplaneConfig.DeletionTimestamp.Time) < 5*time.Minute {
			terminatingCondition := status.NewCondition(backplanev1alpha1.MultiClusterEngineConditionType(backplanev1alpha1.MultiClusterEngineProgressing), metav1.ConditionTrue, status.WaitingForResourceReason, "Waiting for namespace open-cluster-management-hub to terminate.")
			r.StatusManager.AddCondition(terminatingCondition)
		} else {
			terminatingCondition := status.NewCondition(backplanev1alpha1.MultiClusterEngineConditionType(backplanev1alpha1.MultiClusterEngineProgressing), metav1.ConditionFalse, status.WaitingForResourceReason, "Namespace open-cluster-management-hub still exists.")
			r.StatusManager.AddCondition(terminatingCondition)
		}

		return fmt.Errorf("waiting for 'open-cluster-management-hub' namespace to be terminated before proceeding with uninstallation")
	} else if err != nil && !apierrors.IsNotFound(err) { // Return error, if error is not not found error
		return err
	}

	return nil
}

func (r *MultiClusterEngineReconciler) getBackplaneConfig(ctx context.Context, req ctrl.Request) (*backplanev1alpha1.MultiClusterEngine, error) {
	log := log.FromContext(ctx)
	backplaneConfig := &backplanev1alpha1.MultiClusterEngine{}
	err := r.Client.Get(ctx, req.NamespacedName, backplaneConfig)
	if err != nil {
		if apierrors.IsNotFound(err) {
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

// ensureUnstructuredResource ensures that the unstructured resource is applied in the cluster properly
func (r *MultiClusterEngineReconciler) ensureUnstructuredResource(ctx context.Context, bpc *backplanev1alpha1.MultiClusterEngine, u *unstructured.Unstructured) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	found := &unstructured.Unstructured{}
	found.SetGroupVersionKind(u.GroupVersionKind())

	utils.AddBackplaneConfigLabels(u, bpc.Name)

	// Try to get API group instance
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      u.GetName(),
		Namespace: u.GetNamespace(),
	}, found)
	if err != nil && apierrors.IsNotFound(err) {
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

	return ctrl.Result{}, nil
}
