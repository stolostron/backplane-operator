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
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/foundation"
	"github.com/stolostron/backplane-operator/pkg/hive"
	"github.com/stolostron/backplane-operator/pkg/images"
	renderer "github.com/stolostron/backplane-operator/pkg/rendering"
	"github.com/stolostron/backplane-operator/pkg/status"
	"github.com/stolostron/backplane-operator/pkg/utils"

	clustermanager "open-cluster-management.io/api/operator/v1"

	configv1 "github.com/openshift/api/config/v1"
	hiveconfig "github.com/openshift/hive/apis/hive/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
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

	semver "github.com/Masterminds/semver"
	pkgerrors "github.com/pkg/errors"
	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
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
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;create;update;list;watch;delete;patch
//+kubebuilder:rbac:groups="discovery.open-cluster-management.io",resources=discoveryconfigs,verbs=get
//+kubebuilder:rbac:groups="discovery.open-cluster-management.io",resources=discoveryconfigs,verbs=list
//+kubebuilder:rbac:groups="discovery.open-cluster-management.io",resources=discoveryconfigs;discoveredclusters,verbs=create;get;list;watch;update;delete;deletecollection;patch;approve;escalate;bind
//+kubebuilder:rbac:groups=config.openshift.io,resources=clusterversions,verbs=get;list;watch;
//+kubebuilder:rbac:groups=console.openshift.io,resources=consoleplugins;consolequickstarts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=operator.openshift.io,resources=consoles,verbs=get;list;watch;update;patch

// AgentServiceConfig webhook delete check
//+kubebuilder:rbac:groups=agent-install.openshift.io,resources=agentserviceconfigs,verbs=get;list;watch

// ClusterManager RBAC
//+kubebuilder:rbac:groups="",resources=configmaps;configmaps/status;namespaces;serviceaccounts;services;secrets,verbs=create;get;list;update;watch;patch;delete
//+kubebuilder:rbac:groups="",resources=nodes;endpoints,verbs=get;list;watch
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
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io;inventory.open-cluster-management.io;agent.open-cluster-management.io;operator.open-cluster-management.io,resources=klusterletaddonconfigs;managedclusters;baremetalassets;multiclusterhubs,verbs=get;list;watch;create;delete;watch;update;patch
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersets/join,verbs=create
//+kubebuilder:rbac:groups=migration.k8s.io,resources=storageversionmigrations,verbs=create;get;list;update;patch;watch;delete
//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=create;get;list;update;patch;watch;delete
//+kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=clustermanagementaddons;clustermanagementaddons/finalizers;managedclusteraddons;managedclusteraddons/finalizers;managedclusteraddons/status,verbs=create;get;list;update;patch;watch;delete;deletecollection
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=addonplacementscores,verbs=create;get;list;update;patch;watch;delete;deletecollection
//+kubebuilder:rbac:groups=proxy.open-cluster-management.io,resources=managedproxyconfigurations,verbs=create;get;list;update;patch;watch;delete;deletecollection

// Hive RBAC
//+kubebuilder:rbac:groups="hive.openshift.io",resources=hiveconfigs,verbs=get;create;update;delete;list;watch
//+kubebuilder:rbac:groups="hive.openshift.io",resources=clusterdeployments;clusterpools;clusterclaims;machinepools,verbs=approve;bind;create;delete;deletecollection;escalate;get;list;patch;update;watch

// CLC RBAC
//+kubebuilder:rbac:groups="internal.open-cluster-management.io",resources="managedclusterinfos",verbs=get;list;watch
//+kubebuilder:rbac:groups="config.openshift.io";"authentication.k8s.io",resources=clusterversions;tokenreviews,verbs=get;create
//+kubebuilder:rbac:groups="register.open-cluster-management.io",resources=managedclusters/accept,verbs=update
//+kubebuilder:rbac:groups="tower.ansible.com";"";"batch",resources=ansiblejobs;jobs;secrets;serviceaccounts,verbs=create
//+kubebuilder:rbac:groups="tower.ansible.com";"";"batch",resources=ansiblejobs;jobs;clusterdeployments;serviceaccounts;machinepools,verbs=get
//+kubebuilder:rbac:groups="action.open-cluster-management.io",resources=managedclusteractions,verbs=get;create;update;delete
//+kubebuilder:rbac:groups="cluster.open-cluster-management.io",resources=clustercurators;clustercurators/status,verbs=create;delete;get;list;patch;update;watch
//+kubebuilder:rbac:groups="operators.coreos.com",resources=subscriptions,verbs=get;list;watch

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
		return ctrl.Result{RequeueAfter: 5 * time.Second}, errors.New("Resource missing UID")
	}
	if r.StatusManager.UID == "" || r.StatusManager.UID != string(backplaneConfig.UID) {
		log.Info("Setting status manager to track new MCE", "UID", string(backplaneConfig.UID))
		r.StatusManager.Reset(string(backplaneConfig.UID))
	}

	defer func() {
		log.Info("Updating status")
		backplaneConfig.Status = r.StatusManager.ReportStatus(*backplaneConfig)
		err := r.Client.Status().Update(ctx, backplaneConfig)
		if backplaneConfig.Status.Phase != backplanev1.MultiClusterEnginePhaseAvailable && !utils.IsPaused(backplaneConfig) {
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

	var result ctrl.Result

	result, err = r.setDefaults(ctx, backplaneConfig)
	if result != (ctrl.Result{}) {
		return ctrl.Result{}, err
	}
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	result, err = r.validateNamespace(ctx, backplaneConfig)
	if result != (ctrl.Result{}) {
		return ctrl.Result{}, err
	}
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	result, err = r.validateImagePullSecret(ctx, backplaneConfig)
	if result != (ctrl.Result{}) {
		return ctrl.Result{}, err
	}
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	// Read images from environmental variables
	imgs, err := images.GetImagesWithOverrides(r.Client, backplaneConfig)
	if err != nil {
		r.StatusManager.AddCondition(status.NewCondition(backplanev1.MultiClusterEngineProgressing, metav1.ConditionFalse, status.RequirementsNotMetReason, fmt.Sprintf("Issue building image references: %s", err.Error())))
		return ctrl.Result{}, err
	}
	if len(imgs) == 0 {
		// If images are not set from environmental variables, fail
		r.StatusManager.AddCondition(status.NewCondition(backplanev1.MultiClusterEngineProgressing, metav1.ConditionFalse, status.RequirementsNotMetReason, "No image references defined in deployment"))
		return ctrl.Result{RequeueAfter: requeuePeriod}, errors.New("no image references exist. images must be defined as environment variables")
	}
	r.Images = imgs

	// Do not reconcile objects if this instance of mce is labeled "paused"
	if utils.IsPaused(backplaneConfig) {
		log.Info("MultiClusterEngine reconciliation is paused. Nothing more to do.")
		r.StatusManager.AddCondition(status.NewCondition(backplanev1.MultiClusterEngineProgressing, metav1.ConditionUnknown, status.PausedReason, "Multiclusterengine is paused"))
		return ctrl.Result{}, nil
	}

	result, err = r.adoptExistingSubcomponents(ctx, backplaneConfig)
	if err != nil {
		r.StatusManager.AddCondition(status.NewCondition(backplanev1.MultiClusterEngineProgressing, metav1.ConditionUnknown, status.DeployFailedReason, err.Error()))
		return result, err
	}

	result, err = r.DeployAlwaysSubcomponents(ctx, backplaneConfig)
	if err != nil {
		r.StatusManager.AddCondition(status.NewCondition(backplanev1.MultiClusterEngineProgressing, metav1.ConditionUnknown, status.DeployFailedReason, err.Error()))
		return result, err
	}

	result, err = r.ensureToggleableComponents(ctx, backplaneConfig)
	if err != nil {
		return result, err
	}

	r.StatusManager.AddCondition(status.NewCondition(backplanev1.MultiClusterEngineProgressing, metav1.ConditionTrue, status.DeploySuccessReason, "All components deployed"))

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MultiClusterEngineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&backplanev1.MultiClusterEngine{}).
		WithEventFilter(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{}, predicate.AnnotationChangedPredicate{})).
		Watches(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
			OwnerType: &backplanev1.MultiClusterEngine{},
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
		Watches(&source.Kind{Type: &monitorv1.ServiceMonitor{}}, &handler.Funcs{
			DeleteFunc: func(e event.DeleteEvent, q workqueue.RateLimitingInterface) {
				labels := e.Object.GetLabels()
				if label, ok := labels["backplaneconfig.name"]; ok {
					q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
						Name: label,
					}})
				}
			},
		}, builder.WithPredicates(predicate.LabelChangedPredicate{})).
		Complete(r)
}

// DeployAlwaysSubcomponents ensures all subcomponents exist
func (r *MultiClusterEngineReconciler) DeployAlwaysSubcomponents(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	chartsDir := renderer.AlwaysChartsDir
	// Renders all templates from charts
	templates, errs := renderer.RenderCharts(chartsDir, backplaneConfig, r.Images)
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
		result, err := r.applyTemplate(ctx, backplaneConfig, template)
		if err != nil {
			return result, err
		}
	}

	result, err := r.ensureCustomResources(ctx, backplaneConfig)
	if err != nil {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureToggleableComponents(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	errs := map[string]error{}
	requeue := false

	if backplaneConfig.Enabled(backplanev1.ManagedServiceAccount) {
		result, err := r.ensureManagedServiceAccount(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ManagedServiceAccount] = err
		}
	} else {
		result, err := r.ensureNoManagedServiceAccount(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ManagedServiceAccount] = err
		}
	}

	if backplaneConfig.Enabled(backplanev1.HyperShift) {
		result, err := r.ensureHyperShift(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.HyperShift] = err
		}
	} else {
		result, err := r.ensureNoHyperShift(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.HyperShift] = err
		}
	}

	if backplaneConfig.Enabled(backplanev1.ConsoleMCE) {
		result, err := r.ensureConsoleMCE(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ConsoleMCE] = err
		}
	} else {
		result, err := r.ensureNoConsoleMCE(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ConsoleMCE] = err
		}
	}

	if backplaneConfig.Enabled(backplanev1.Discovery) {
		result, err := r.ensureDiscovery(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.Discovery] = err
		}
	} else {
		result, err := r.ensureNoDiscovery(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.Discovery] = err
		}
	}

	if backplaneConfig.Enabled(backplanev1.Hive) {
		result, err := r.ensureHive(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.Hive] = err
		}
	} else {
		result, err := r.ensureNoHive(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.Hive] = err
		}
	}

	if backplaneConfig.Enabled(backplanev1.AssistedService) {
		result, err := r.ensureAssistedService(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.AssistedService] = err
		}
	} else {
		result, err := r.ensureNoAssistedService(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.AssistedService] = err
		}
	}

	if backplaneConfig.Enabled(backplanev1.ClusterLifecycle) {
		result, err := r.ensureClusterLifecycle(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ClusterLifecycle] = err
		}
	} else {
		result, err := r.ensureNoClusterLifecycle(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ClusterLifecycle] = err
		}
	}

	if backplaneConfig.Enabled(backplanev1.ClusterManager) {
		result, err := r.ensureClusterManager(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ClusterManager] = err
		}
	} else {
		result, err := r.ensureNoClusterManager(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ClusterManager] = err
		}
	}

	if backplaneConfig.Enabled(backplanev1.ServerFoundation) {
		result, err := r.ensureServerFoundation(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ServerFoundation] = err
		}
	} else {
		result, err := r.ensureNoServerFoundation(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ServerFoundation] = err
		}
	}

	if backplaneConfig.Enabled(backplanev1.ClusterProxyAddon) {
		result, err := r.ensureClusterProxyAddon(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ClusterProxyAddon] = err
		}
	} else {
		result, err := r.ensureNoClusterProxyAddon(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ClusterProxyAddon] = err
		}
	}

	if len(errs) > 0 {
		errorMessages := []string{}
		for k, v := range errs {
			errorMessages = append(errorMessages, fmt.Sprintf("error ensuring %s: %s", k, v.Error()))
		}
		combinedError := fmt.Sprintf(": %s", strings.Join(errorMessages, "; "))
		log.FromContext(ctx).Error(errors.New("Errors applying components"), combinedError)
		return ctrl.Result{RequeueAfter: requeuePeriod}, errors.New(combinedError)
	}
	if requeue {
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) applyTemplate(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine, template *unstructured.Unstructured) (ctrl.Result, error) {
	// Set owner reference.
	err := ctrl.SetControllerReference(backplaneConfig, template, r.Scheme)
	if err != nil {
		return ctrl.Result{}, pkgerrors.Wrapf(err, "Error setting controller reference on resource %s", template.GetName())
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
			return ctrl.Result{}, pkgerrors.Wrapf(err, "error applying object Name: %s Kind: %s", template.GetName(), template.GetKind())
		}
	}
	return ctrl.Result{}, nil
}

// deleteTemplate return true if resource does not exist and returns an error if a GET or DELETE errors unexpectedly. A false response without error
// means the resource is in the process of deleting.
func (r *MultiClusterEngineReconciler) deleteTemplate(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine, template *unstructured.Unstructured) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	err := r.Client.Get(ctx, types.NamespacedName{Name: template.GetName(), Namespace: template.GetNamespace()}, template)

	if err != nil && apierrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}

	// set status progressing condition
	if err != nil {
		log.Error(err, "Odd error delete template")
		return ctrl.Result{}, err
	}

	log.Info(fmt.Sprintf("finalizing template: %s\n", template.GetName()))
	err = r.Client.Delete(ctx, template)
	if err != nil {
		log.Error(err, "Failed to delete template")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureCustomResources(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if foundation.CanInstallAddons(ctx, r.Client) {
		addonTemplates, err := foundation.GetAddons()
		if err != nil {
			return ctrl.Result{}, err
		}
		for _, addonTemplate := range addonTemplates {
			addonTemplate.SetNamespace(backplaneConfig.Spec.TargetNamespace)
			if err := ctrl.SetControllerReference(backplaneConfig, addonTemplate, r.Scheme); err != nil {
				return ctrl.Result{}, pkgerrors.Wrapf(err, "Error setting controller reference on resource %s", addonTemplate.GetName())
			}
			force := true
			err := r.Client.Patch(ctx, addonTemplate, client.Apply, &client.PatchOptions{Force: &force, FieldManager: "backplane-operator"})
			if err != nil {
				return ctrl.Result{}, pkgerrors.Wrapf(err, "error applying object Name: %s Kind: %s", addonTemplate.GetName(), addonTemplate.GetKind())
			}
		}
	} else {
		log.Info("ClusterManagementAddon API is not installed. Waiting to install addons.")
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) finalizeBackplaneConfig(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) error {
	log := log.FromContext(ctx)
	_, err := r.removePluginFromConsoleResource(ctx, backplaneConfig)
	if err != nil {
		log.Info("Error ensuring plugin is removed from console resource")
		return err
	}

	clusterManager := &unstructured.Unstructured{}
	clusterManager.SetGroupVersionKind(
		schema.GroupVersionKind{
			Group:   "operator.open-cluster-management.io",
			Version: "v1",
			Kind:    "ClusterManager",
		},
	)

	err = r.Client.Get(ctx, types.NamespacedName{Name: "cluster-manager"}, clusterManager)
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
			terminatingCondition := status.NewCondition(backplanev1.MultiClusterEngineConditionType(backplanev1.MultiClusterEngineProgressing), metav1.ConditionTrue, status.WaitingForResourceReason, "Waiting for namespace open-cluster-management-hub to terminate.")
			r.StatusManager.AddCondition(terminatingCondition)
		} else {
			terminatingCondition := status.NewCondition(backplanev1.MultiClusterEngineConditionType(backplanev1.MultiClusterEngineProgressing), metav1.ConditionFalse, status.WaitingForResourceReason, "Namespace open-cluster-management-hub still exists.")
			r.StatusManager.AddCondition(terminatingCondition)
		}

		return fmt.Errorf("waiting for 'open-cluster-management-hub' namespace to be terminated before proceeding with uninstallation")
	} else if err != nil && !apierrors.IsNotFound(err) { // Return error, if error is not not found error
		return err
	}

	return nil
}

func (r *MultiClusterEngineReconciler) getBackplaneConfig(ctx context.Context, req ctrl.Request) (*backplanev1.MultiClusterEngine, error) {
	log := log.FromContext(ctx)
	backplaneConfig := &backplanev1.MultiClusterEngine{}
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
func (r *MultiClusterEngineReconciler) ensureUnstructuredResource(ctx context.Context, bpc *backplanev1.MultiClusterEngine, u *unstructured.Unstructured) (ctrl.Result, error) {
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

func (r *MultiClusterEngineReconciler) setDefaults(ctx context.Context, m *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	updateNecessary := false
	if !utils.AvailabilityConfigIsValid(m.Spec.AvailabilityConfig) {
		m.Spec.AvailabilityConfig = backplanev1.HAHigh
		updateNecessary = true
	}

	if len(m.Spec.TargetNamespace) == 0 {
		m.Spec.TargetNamespace = backplanev1.DefaultTargetNamespace
		updateNecessary = true
	}

	if utils.SetDefaultComponents(m) {
		updateNecessary = true
	}

	if utils.DeduplicateComponents(m) {
		updateNecessary = true
	}

	// Set and store cluster Ingress domain for use later
	clusterIngressDomain, err := r.getClusterIngressDomain(ctx, m)
	if err != nil {
		return ctrl.Result{}, pkgerrors.Wrapf(err, "failed to detect cluster ingress domain")
	}

	// Set OCP version as env var, so that charts can render this value
	os.Setenv("ACM_CLUSTER_INGRESS_DOMAIN", clusterIngressDomain)

	// If OCP 4.10+ then set then enable the MCE console. Else ensure it is disabled
	currentClusterVersion, err := r.getClusterVersion(ctx, m)
	if err != nil {
		return ctrl.Result{}, pkgerrors.Wrapf(err, "failed to detect clusterversion")
	}

	// Set OCP version as env var, so that charts can render this value
	os.Setenv("ACM_HUB_OCP_VERSION", currentClusterVersion)

	currentVersion, err := semver.NewVersion(currentClusterVersion)
	if err != nil {
		log.Error(err, "Failed to convert currentVersion of cluster to semver compatible value for comparison")
		return ctrl.Result{}, err
	}

	// -0 allows for prerelease builds to pass the validation.
	// If -0 is removed, developer/rc builds will not pass this check
	constraint, err := semver.NewConstraint(">= 4.10.0-0")
	if err != nil {
		log.Error(err, "Failed to set constraint of minimum supported version for plugins")
		return ctrl.Result{}, err
	}

	if constraint.Check(currentVersion) {
		// If ConsoleMCE config already exists, then don't overwrite it
		if !m.ComponentPresent(backplanev1.ConsoleMCE) {
			log.Info("Dynamic plugins are supported. ConsoleMCE Config is not detected. Enabling ConsoleMCE")
			m.Enable(backplanev1.ConsoleMCE)
			updateNecessary = true
		}
	} else {
		if m.Enabled(backplanev1.ConsoleMCE) {
			log.Info("Dynamic plugins are not supported. Disabling MCE console")
			m.Disable(backplanev1.ConsoleMCE)
			updateNecessary = true
		}
	}

	// Apply defaults to server
	if updateNecessary {
		log.Info("Setting defaults")
		err = r.Client.Update(ctx, m)
		if err != nil {
			log.Error(err, "Failed to update MultiClusterEngine")
			return ctrl.Result{}, err
		}
		log.Info("MultiClusterEngine successfully updated")
		return ctrl.Result{Requeue: true}, nil
	} else {
		return ctrl.Result{}, nil
	}

}

func (r *MultiClusterEngineReconciler) validateNamespace(ctx context.Context, m *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	newNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: m.Spec.TargetNamespace,
		},
	}
	checkNs := &corev1.Namespace{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{Name: m.Spec.TargetNamespace}, checkNs)
	if err != nil && apierrors.IsNotFound(err) {
		if err := ctrl.SetControllerReference(m, newNs, r.Scheme); err != nil {
			return ctrl.Result{}, pkgerrors.Wrapf(err, "Error setting controller reference on resource %s", m.Spec.TargetNamespace)
		}
		err = r.Client.Create(context.TODO(), newNs)
		if err != nil {
			log.Error(err, "Could not create namespace")
			return ctrl.Result{}, err
		}
		log.Info("Namespace created")
		return ctrl.Result{Requeue: true}, nil
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{Requeue: true}, err
	}
	return ctrl.Result{}, nil
}

// validateImagePullSecret returns an error if the namespace in spec.targetNamespace does not have a secret
// with the name in spec.imagePullSecret.
func (r *MultiClusterEngineReconciler) validateImagePullSecret(ctx context.Context, m *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	if m.Spec.ImagePullSecret == "" {
		return ctrl.Result{}, nil
	}

	pullSecret := &corev1.Secret{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      m.Spec.ImagePullSecret,
		Namespace: m.Spec.TargetNamespace,
	}, pullSecret)
	if apierrors.IsNotFound(err) {
		missingPullSecret := status.NewCondition(backplanev1.MultiClusterEngineConditionType(backplanev1.MultiClusterEngineProgressing), metav1.ConditionFalse, status.RequirementsNotMetReason, fmt.Sprintf("Could not find imagePullSecret %s in namespace %s", m.Spec.ImagePullSecret, m.Spec.TargetNamespace))
		r.StatusManager.AddCondition(missingPullSecret)
		return ctrl.Result{RequeueAfter: requeuePeriod}, err
	}
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	return ctrl.Result{}, nil
}

// adoptExistingSubcomponents checks for the existence of subcomponents installed by the MCH, and adds a label
// signaling that they have been adopted by the MCE.
func (r *MultiClusterEngineReconciler) adoptExistingSubcomponents(ctx context.Context, mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {

	log := log.FromContext(ctx)

	cmTemplate := foundation.ClusterManager(mce, r.Images)
	hiveTemplate := hive.HiveConfig(mce)

	resources := []*unstructured.Unstructured{cmTemplate, hiveTemplate}

	for _, resource := range resources {

		existingResource := &unstructured.Unstructured{}
		existingResource.SetGroupVersionKind(resource.GroupVersionKind())
		err := r.Get(ctx, types.NamespacedName{Name: resource.GetName(), Namespace: resource.GetNamespace()}, existingResource)
		if err != nil && !apierrors.IsNotFound(err) {
			log.Info(fmt.Sprintf("Unable to get existing resource: %+v", err.Error()))
			return ctrl.Result{}, err
		} else if apierrors.IsNotFound(err) {
			// Resource doesn't exist, no need to adopt
			continue
		}

		if err := ctrl.SetControllerReference(mce, existingResource, r.Scheme); err != nil {
			return ctrl.Result{}, pkgerrors.Wrapf(err, "Error setting controller reference on resource %s", existingResource.GetName())
		}

		err = r.Update(ctx, existingResource)
		if err != nil {
			log.Info(fmt.Sprintf("Unable to update existing resource: %+v", err.Error()))
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) getClusterVersion(ctx context.Context, mce *backplanev1.MultiClusterEngine) (string, error) {
	log := log.FromContext(ctx)
	// If Unit test
	if val, ok := os.LookupEnv("UNIT_TEST"); ok && val == "true" {
		return "4.9.0", nil
	}

	clusterVersion := &configv1.ClusterVersion{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: "version"}, clusterVersion)
	if err != nil {
		log.Error(err, "Failed to detect clusterversion")
		return "", err
	}

	if len(clusterVersion.Status.History) == 0 {
		log.Error(err, "Failed to detect status in clusterversion.status.history")
		return "", err
	}
	return clusterVersion.Status.History[0].Version, nil
}

func (r *MultiClusterEngineReconciler) getClusterIngressDomain(ctx context.Context, mce *backplanev1.MultiClusterEngine) (string, error) {
	log := log.FromContext(ctx)
	// If Unit test
	if val, ok := os.LookupEnv("UNIT_TEST"); ok && val == "true" {
		return "apps.installer-test-cluster.dev00.red-chesterfield.com", nil
	}

	clusterIngress := &configv1.Ingress{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: "cluster"}, clusterIngress)
	if err != nil {
		log.Error(err, "Failed to detect cluster ingress")
		return "", err
	}

	if clusterIngress.Spec.Domain == "" {
		log.Error(err, "Domain not found or empty in Ingress")
		return "", fmt.Errorf("Domain not found or empty in Ingress")
	}
	return clusterIngress.Spec.Domain, nil
}
