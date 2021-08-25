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

	rbacv1 "k8s.io/api/rbac/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	backplanev1alpha1 "github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	"github.com/open-cluster-management/backplane-operator/pkg/foundation"
	"github.com/open-cluster-management/backplane-operator/pkg/hive"
	renderer "github.com/open-cluster-management/backplane-operator/pkg/rendering"
	"github.com/open-cluster-management/backplane-operator/pkg/status"
	"github.com/open-cluster-management/backplane-operator/pkg/utils"
)

// BackplaneConfigReconciler reconciles a BackplaneConfig object
type BackplaneConfigReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Images        map[string]string
	StatusManager *status.StatusTracker
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
//+kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=clustermanagers,verbs=create;get;list;watch;update;delete
//+kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=clustermanagers/status,verbs=update;patch

// Hive RBAC
//+kubebuilder:rbac:groups="hive.openshift.io",resources=hiveconfigs,verbs=get;create;update;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the BackplaneConfig object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *BackplaneConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (retRes ctrl.Result, retErr error) {
	log := log.FromContext(ctx)

	// Fetch the BackplaneConfig instance
	backplaneConfig, err := r.getBackplaneConfig(req)
	if err != nil && !errors.IsNotFound(err) {
		// Unknown error. Requeue
		log.Info("Failed to fetch backplaneConfig")
		return ctrl.Result{RequeueAfter: requeuePeriod}, err
	} else if err != nil && errors.IsNotFound(err) {
		// BackplaneConfig deleted or not found
		// Return and don't requeue
		return ctrl.Result{}, nil
	}

	// If deletion detected, finalize backplane config
	if backplaneConfig.GetDeletionTimestamp() != nil {
		err := r.finalizeBackplaneConfig(backplaneConfig) // returns all errors
		if err != nil {
			log.Info(err.Error())
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		return ctrl.Result{}, nil // Object finalized successfully
	}

	// Add finalizer for this CR
	if !contains(backplaneConfig.GetFinalizers(), backplaneFinalizer) {
		if err := r.addFinalizer(backplaneConfig); err != nil {
			return ctrl.Result{}, err
		}
	}

	defer func() {
		log.Info("Updating status")
		backplaneConfig.Status = r.StatusManager.ReportStatus()
		err := r.Client.Status().Update(ctx, backplaneConfig)
		if backplaneConfig.Status.Phase != backplanev1alpha1.BackplanePhaseAvailable {
			retRes = ctrl.Result{RequeueAfter: 10 * time.Second}
		}
		if err != nil {
			retErr = err
		}
	}()

	// Read image overrides from environmental variables
	r.Images = utils.GetImageOverrides()
	if len(r.Images) == 0 {
		// If imageoverrides are not set from environmental variables, fail
		r.StatusManager.AddCondition(status.NewCondition(backplanev1alpha1.BackplaneProgressing, metav1.ConditionFalse, status.RequirementsNotMetReason, "No image references defined in deployment"))
		return ctrl.Result{RequeueAfter: requeuePeriod}, e.New("no image references exist. images must be defined as environment variables")
	}

	result, err := r.DeploySubcomponents(backplaneConfig)
	if err != nil {
		r.StatusManager.AddCondition(status.NewCondition(backplanev1alpha1.BackplaneProgressing, metav1.ConditionUnknown, status.DeployFailedReason, err.Error()))
		return result, err
	}
	r.StatusManager.AddCondition(status.NewCondition(backplanev1alpha1.BackplaneProgressing, metav1.ConditionTrue, status.DeploySuccessReason, "All components deployed"))

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackplaneConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&backplanev1alpha1.BackplaneConfig{}).
		WithEventFilter(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{}, predicate.AnnotationChangedPredicate{})).
		Complete(r)
}

// DeploySubcomponents ensures all subcomponents exist
func (r *BackplaneConfigReconciler) DeploySubcomponents(backplaneConfig *backplanev1alpha1.BackplaneConfig) (ctrl.Result, error) {
	log := log.FromContext(context.Background())

	// Render CRD templates
	crds, errs := renderer.RenderCRDs()
	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	for _, crd := range crds {
		result, err := r.ensureUnstructuredResource(backplaneConfig, crd)
		if err != nil {
			return result, err
		}
	}

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
		result, err := r.ensureUnstructuredResource(backplaneConfig, template)
		if err != nil {
			return result, err
		}
	}

	result, err := r.ensureCustomResources(backplaneConfig)
	if err != nil {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *BackplaneConfigReconciler) ensureCustomResources(backplaneConfig *backplanev1alpha1.BackplaneConfig) (ctrl.Result, error) {

	result, err := r.ensureUnstructuredResource(backplaneConfig, foundation.ClusterManager(backplaneConfig, r.Images))
	if err != nil {
		return result, err
	}
	r.StatusManager.AddComponent(status.ClusterManagerStatus{
		NamespacedName: types.NamespacedName{Name: "cluster-manager"},
	})

	result, err = r.ensureUnstructuredResource(backplaneConfig, hive.HiveConfig(backplaneConfig))
	if err != nil {
		return result, err
	}
	return ctrl.Result{}, nil
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

func (r *BackplaneConfigReconciler) finalizeBackplaneConfig(backplaneConfig *backplanev1alpha1.BackplaneConfig) error {
	ctx := context.Background()
	log := log.FromContext(ctx)
	if contains(backplaneConfig.GetFinalizers(), backplaneFinalizer) {
		// Run finalization logic
		labelSelector := client.MatchingLabels{
			"backplaneconfig.name": backplaneConfig.Name}

		apiServiceList := &apiregistrationv1.APIServiceList{}
		serviceList := &corev1.ServiceList{}
		deploymentList := &appsv1.DeploymentList{}
		clusterRoleBindingList := &rbacv1.ClusterRoleBindingList{}
		clusterRoleList := &rbacv1.ClusterRoleList{}
		serviceAccountList := &corev1.ServiceAccountList{}
		clusterManager := &unstructured.Unstructured{}
		clusterManager.SetGroupVersionKind(
			schema.GroupVersionKind{
				Group:   "operator.open-cluster-management.io",
				Version: "v1",
				Kind:    "ClusterManager",
			},
		)
		ocmHubNamespace := &corev1.Namespace{}

		hiveConfig := &unstructured.Unstructured{}
		hiveConfig.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "hive.openshift.io",
			Version: "v1",
			Kind:    "HiveConfig",
		})

		if err := r.Client.List(ctx, apiServiceList, labelSelector); err != nil {
			return err
		}
		if err := r.Client.List(ctx, serviceList, labelSelector); err != nil {
			return err
		}
		if err := r.Client.List(ctx, deploymentList, labelSelector); err != nil {
			return err
		}
		if err := r.Client.List(ctx, clusterRoleBindingList, labelSelector); err != nil {
			return err
		}
		if err := r.Client.List(ctx, clusterRoleList, labelSelector); err != nil {
			return err
		}
		if err := r.Client.List(ctx, serviceAccountList, labelSelector); err != nil {
			return err
		}

		for _, apiService := range apiServiceList.Items {
			log.Info(fmt.Sprintf("finalizing apiservice - %s", apiService.Name))
			err := r.Client.Delete(ctx, &apiService)
			if err != nil {
				return err
			}
		}

		err := r.Client.Get(ctx, types.NamespacedName{Name: "cluster-manager"}, clusterManager)
		if err == nil { // If resource exists, delete
			log.Info("finalizing cluster-manager custom resource")
			err := r.Client.Delete(ctx, clusterManager)
			if err != nil {
				return err
			}
		} else if err != nil && !errors.IsNotFound(err) { // Return error, if error is not not found error
			return err
		}

		err = r.Client.Get(ctx, types.NamespacedName{Name: "open-cluster-management-hub"}, ocmHubNamespace)
		if err == nil { // If resource exists, delete
			return fmt.Errorf("waiting for 'open-cluster-management-hub' namespace to be terminated before proceeding with uninstallation")
		} else if err != nil && !errors.IsNotFound(err) { // Return error, if error is not not found error
			return err
		}

		err = r.Client.Get(ctx, types.NamespacedName{Name: "hive"}, hiveConfig)
		if err == nil { // If resource exists, delete
			log.Info("finalizing hiveconfig custom resource")
			err := r.Client.Delete(ctx, hiveConfig)
			if err != nil {
				return err
			}
		} else if err != nil && !errors.IsNotFound(err) { // Return error, if error is not not found error
			return err
		}

		for _, service := range serviceList.Items {
			log.Info(fmt.Sprintf("finalizing service - %s/%s", service.Namespace, service.Name))
			err := r.Client.Delete(ctx, &service)
			if err != nil {
				return err
			}
		}
		for _, deployment := range deploymentList.Items {
			log.Info(fmt.Sprintf("finalizing deployment - %s/%s", deployment.Namespace, deployment.Name))
			err := r.Client.Delete(ctx, &deployment)
			if err != nil {
				return err
			}
		}
		for _, serviceAccount := range serviceAccountList.Items {
			log.Info(fmt.Sprintf("finalizing clusterrole - %s", serviceAccount.Name))
			err := r.Client.Delete(ctx, &serviceAccount)
			if err != nil {
				return err
			}
		}
		for _, clusterRole := range clusterRoleList.Items {
			log.Info(fmt.Sprintf("finalizing clusterrole - %s", clusterRole.Name))
			err := r.Client.Delete(ctx, &clusterRole)
			if err != nil {
				return err
			}
		}
		for _, clusterRoleBinding := range clusterRoleBindingList.Items {
			log.Info(fmt.Sprintf("finalizing clusterrolebinding - %s", clusterRoleBinding.Name))
			err := r.Client.Delete(ctx, &clusterRoleBinding)
			if err != nil {
				return err
			}
		}

		remainingSubcomponents := len(serviceList.Items) + len(apiServiceList.Items) + len(deploymentList.Items) + len(clusterRoleBindingList.Items) + len(clusterRoleList.Items) + len(serviceAccountList.Items)
		if remainingSubcomponents > 0 {
			return fmt.Errorf("%d subcomponents may still exist", remainingSubcomponents)
		}

		log.Info("all subcomponents have been finalized successfully - removing finalizer")
		// Remove finalizer. Once all finalizers have been
		// removed, the object will be deleted.
		backplaneConfig.SetFinalizers(remove(backplaneConfig.GetFinalizers(), backplaneFinalizer))

		err = r.Client.Update(ctx, backplaneConfig)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *BackplaneConfigReconciler) getBackplaneConfig(req ctrl.Request) (*backplanev1alpha1.BackplaneConfig, error) {
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
