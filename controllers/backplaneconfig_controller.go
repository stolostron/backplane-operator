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
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	backplanev1alpha1 "github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	"github.com/open-cluster-management/backplane-operator/pkg/foundation"
	"github.com/open-cluster-management/backplane-operator/pkg/hive"
)

// BackplaneConfigReconciler reconciles a BackplaneConfig object
type BackplaneConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	requeuePeriod = 15 * time.Second
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
//+kubebuilder:rbac:groups="hive.openshift.io",resources=hiveconfigs,verbs=get;create;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the BackplaneConfig object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *BackplaneConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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

	result, err := r.applyCustomResources(backplaneConfig)
	if err != nil {
		return result, err
	}

	// your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackplaneConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&backplanev1alpha1.BackplaneConfig{}).
		Complete(r)
}

func (r *BackplaneConfigReconciler) applyCustomResources(backplaneConfig *backplanev1alpha1.BackplaneConfig) (ctrl.Result, error) {
	_ = log.FromContext(context.Background())

	// TODO: Get proper images
	imagesDictionary := make(map[string]string)
	result, err := r.ensureUnstructuredResource(backplaneConfig, foundation.ClusterManager(backplaneConfig, imagesDictionary))
	if err != nil {
		return result, err
	}

	result, err = r.ensureUnstructuredResource(backplaneConfig, hive.HiveConfig(backplaneConfig))
	if err != nil {
		return result, err
	}
	return ctrl.Result{}, nil
}

func (r *BackplaneConfigReconciler) ensureUnstructuredResource(m *backplanev1alpha1.BackplaneConfig, u *unstructured.Unstructured) (ctrl.Result, error) {
	log := log.FromContext(context.Background())

	found := &unstructured.Unstructured{}
	found.SetGroupVersionKind(u.GroupVersionKind())

	// Try to get API group instance
	err := r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      u.GetName(),
		Namespace: u.GetNamespace(),
	}, found)
	if err != nil && errors.IsNotFound(err) {
		// Resource doesn't exist so create it
		err := r.Client.Create(context.TODO(), u)
		if err != nil {
			// Creation failed
			log.Error(err, "Failed to create new instance")
			return ctrl.Result{}, err
		}
		// Creation was successful
		log.Info("Created new resource")
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
		desired, needsUpdate = foundation.ValidateClusterManager(found, u)
	default:
		log.Info("Could not validate unstrucuted resource with type.", "Type", found.GetKind())
		return ctrl.Result{}, nil
	}

	if needsUpdate {
		log.Info("Updating resource")
		err = r.Client.Update(context.TODO(), desired)
		if err != nil {
			log.Error(err, "Failed to update resource.")
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
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
