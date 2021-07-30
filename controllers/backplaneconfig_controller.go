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

	corev1 "k8s.io/api/core/v1"

	appsv1 "k8s.io/api/apps/v1"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	backplanev1alpha1 "github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	"github.com/open-cluster-management/backplane-operator/pkg/foundation"
	"github.com/open-cluster-management/backplane-operator/pkg/hive"
	renderer "github.com/open-cluster-management/backplane-operator/pkg/rendering"
	"github.com/open-cluster-management/backplane-operator/pkg/templates"
	"github.com/open-cluster-management/backplane-operator/pkg/utils"

	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
)

// BackplaneConfigReconciler reconciles a BackplaneConfig object
type BackplaneConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Images map[string]string
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

	backplaneConfig.Status.Phase = backplanev1alpha1.BackplaneApplying
	defer r.Client.Status().Update(ctx, backplaneConfig)

	// Read image overrides from environmental variables
	r.Images = utils.GetImageOverrides()
	if len(r.Images) == 0 {
		// If imageoverrides are not set from environmental variables, fail
		return ctrl.Result{RequeueAfter: requeuePeriod}, e.New("no image references exist. images must be defined as environment variables")
	}

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

	result, err := r.ensureServerFoundation(backplaneConfig)
	if err != nil {
		return result, err
	}

	result, err = r.ensureCustomResources(backplaneConfig)
	if err != nil {
		return result, err
	}

	backplaneConfig.Status.Phase = backplanev1alpha1.BackplaneApplied
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackplaneConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&backplanev1alpha1.BackplaneConfig{}).
		Complete(r)
}

func (r *BackplaneConfigReconciler) ensureServerFoundation(backplaneConfig *backplanev1alpha1.BackplaneConfig) (ctrl.Result, error) {
	_ = log.FromContext(context.Background())

	templates, err := templates.GetTemplates(backplaneConfig)
	if err != nil {
		return ctrl.Result{RequeueAfter: requeuePeriod}, err
	}
	for _, template := range templates {
		result, err := r.ensureUnstructuredResource(backplaneConfig, template)
		if err != nil {
			return result, err
		}
	}

	result, err := r.ensureDeployment(backplaneConfig, foundation.WebhookDeployment(backplaneConfig, r.Images))
	if result != (ctrl.Result{}) {
		return result, err
	}

	result, err = r.ensureService(backplaneConfig, foundation.WebhookService(backplaneConfig))
	if result != (ctrl.Result{}) {
		return result, err
	}

	//OCM proxy server deployment
	result, err = r.ensureDeployment(backplaneConfig, foundation.OCMProxyServerDeployment(backplaneConfig, r.Images))
	if result != (ctrl.Result{}) {
		return result, err
	}

	//OCM proxy server service
	result, err = r.ensureService(backplaneConfig, foundation.OCMProxyServerService(backplaneConfig))
	if result != (ctrl.Result{}) {
		return result, err
	}

	// OCM proxy apiService
	result, err = r.ensureAPIService(backplaneConfig, foundation.OCMProxyAPIService(backplaneConfig))
	if result != (ctrl.Result{}) {
		return result, err
	}

	// OCM clusterView v1 apiService
	result, err = r.ensureAPIService(backplaneConfig, foundation.OCMClusterViewV1APIService(backplaneConfig))
	if result != (ctrl.Result{}) {
		return result, err
	}

	// OCM clusterView v1alpha1 apiService
	result, err = r.ensureAPIService(backplaneConfig, foundation.OCMClusterViewV1alpha1APIService(backplaneConfig))
	if result != (ctrl.Result{}) {
		return result, err
	}

	//OCM controller deployment
	result, err = r.ensureDeployment(backplaneConfig, foundation.OCMControllerDeployment(backplaneConfig, r.Images))
	if result != (ctrl.Result{}) {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *BackplaneConfigReconciler) ensureCustomResources(backplaneConfig *backplanev1alpha1.BackplaneConfig) (ctrl.Result, error) {
	_ = log.FromContext(context.Background())

	result, err := r.ensureUnstructuredResource(backplaneConfig, foundation.ClusterManager(backplaneConfig, r.Images))
	if err != nil {
		return result, err
	}

	result, err = r.ensureUnstructuredResource(backplaneConfig, hive.HiveConfig(backplaneConfig))
	if err != nil {
		return result, err
	}
	return ctrl.Result{}, nil
}

func (r *BackplaneConfigReconciler) ensureDeployment(bpc *backplanev1alpha1.BackplaneConfig, dep *appsv1.Deployment) (ctrl.Result, error) {
	log := log.FromContext(context.Background())

	// if utils.ProxyEnvVarsAreSet() {
	// 	dep = addProxyEnvVarsToDeployment(dep)
	// }

	// See if deployment already exists and create if it doesn't
	found := &appsv1.Deployment{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      dep.Name,
		Namespace: bpc.Namespace,
	}, found)
	if err != nil && errors.IsNotFound(err) {

		// Create the deployment
		err = r.Client.Create(context.TODO(), dep)
		if err != nil {
			// Deployment failed
			log.Error(err, "Failed to create new Deployment")
			return ctrl.Result{RequeueAfter: requeuePeriod}, err
		}

		// Deployment was successful
		log.Info("Created a new Deployment")
		// condition := NewHubCondition(operatorsv1.Progressing, metav1.ConditionTrue, NewComponentReason, "Created new resource")
		// SetHubCondition(&m.Status, *condition)
		return ctrl.Result{}, nil

	} else if err != nil {
		// Error that isn't due to the deployment not existing
		log.Error(err, "Failed to get Deployment")
		return ctrl.Result{RequeueAfter: requeuePeriod}, err
	}

	// Validate object based on name
	var desired *appsv1.Deployment
	var needsUpdate bool

	switch found.Name {
	// case helmrepo.HelmRepoName:
	// 	desired, needsUpdate = helmrepo.ValidateDeployment(m, r.CacheSpec.ImageOverrides, dep, found)
	case foundation.OCMControllerName, foundation.OCMProxyServerName, foundation.WebhookName:
		desired, needsUpdate = foundation.ValidateDeployment(bpc, r.Images, dep, found)
	default:
		log.Info("Could not validate deployment; unknown name")
		return ctrl.Result{}, nil
	}

	if needsUpdate {
		err = r.Client.Update(context.TODO(), desired)
		if err != nil {
			log.Error(err, "Failed to update Deployment.")
			return ctrl.Result{}, err
		}
		// Spec updated - return
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, nil
}

func (r *BackplaneConfigReconciler) ensureService(m *backplanev1alpha1.BackplaneConfig, s *corev1.Service) (ctrl.Result, error) {
	log := log.FromContext(context.Background())

	found := &corev1.Service{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      s.Name,
		Namespace: m.Namespace,
	}, found)
	if err != nil && errors.IsNotFound(err) {

		// Create the service
		err = r.Client.Create(context.TODO(), s)

		if err != nil {
			// Creation failed
			log.Error(err, "Failed to create new Service")
			return ctrl.Result{}, err
		}

		// Creation was successful
		log.Info("Created a new Service")
		// condition := NewHubCondition(operatorv1.Progressing, metav1.ConditionTrue, NewComponentReason, "Created new resource")
		// SetHubCondition(&m.Status, *condition)
		return ctrl.Result{}, nil

	} else if err != nil {
		// Error that isn't due to the service not existing
		log.Error(err, "Failed to get Service")
		return ctrl.Result{}, err
	}

	modified := resourcemerge.BoolPtr(false)
	existingCopy := found.DeepCopy()
	resourcemerge.EnsureObjectMeta(modified, &existingCopy.ObjectMeta, s.ObjectMeta)
	selectorSame := equality.Semantic.DeepEqual(existingCopy.Spec.Selector, s.Spec.Selector)

	typeSame := false
	requiredIsEmpty := len(s.Spec.Type) == 0
	existingCopyIsCluster := existingCopy.Spec.Type == corev1.ServiceTypeClusterIP
	if (requiredIsEmpty && existingCopyIsCluster) || equality.Semantic.DeepEqual(existingCopy.Spec.Type, s.Spec.Type) {
		typeSame = true
	}

	if selectorSame && typeSame && !*modified {
		return ctrl.Result{}, nil
	}

	existingCopy.Spec.Selector = s.Spec.Selector
	existingCopy.Spec.Type = s.Spec.Type
	err = r.Client.Update(context.TODO(), existingCopy)
	if err != nil {
		log.Error(err, "Failed to update Service")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *BackplaneConfigReconciler) ensureAPIService(m *backplanev1alpha1.BackplaneConfig, s *apiregistrationv1.APIService) (ctrl.Result, error) {
	log := log.FromContext(context.Background())

	found := &apiregistrationv1.APIService{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{
		Name: s.Name,
	}, found)
	if err != nil && errors.IsNotFound(err) {

		// Create the apiService
		err = r.Client.Create(context.TODO(), s)

		if err != nil {
			// Creation failed
			log.Error(err, "Failed to create new apiService")
			return ctrl.Result{}, err
		}

		// Creation was successful
		log.Info("Created a new apiService")
		// condition := NewHubCondition(operatorv1.Progressing, metav1.ConditionTrue, NewComponentReason, "Created new resource")
		// SetHubCondition(&m.Status, *condition)
		return ctrl.Result{}, nil

	} else if err != nil {
		// Error that isn't due to the apiService not existing
		log.Error(err, "Failed to get apiService")
		return ctrl.Result{}, err
	}

	modified := resourcemerge.BoolPtr(false)
	existingCopy := found.DeepCopy()

	resourcemerge.EnsureObjectMeta(modified, &existingCopy.ObjectMeta, s.ObjectMeta)
	serviceSame := equality.Semantic.DeepEqual(existingCopy.Spec.Service, s.Spec.Service)
	prioritySame := existingCopy.Spec.VersionPriority == s.Spec.VersionPriority && existingCopy.Spec.GroupPriorityMinimum == s.Spec.GroupPriorityMinimum
	insecureSame := existingCopy.Spec.InsecureSkipTLSVerify == s.Spec.InsecureSkipTLSVerify

	if !*modified && serviceSame && prioritySame && insecureSame {
		return ctrl.Result{}, nil
	}

	existingCopy.Spec = s.Spec
	err = r.Client.Update(context.TODO(), existingCopy)
	if err != nil {
		log.Error(err, "Failed to update apiService")
		return ctrl.Result{}, err
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
	default:
		log.Info("Could not validate unstructured resource. Skipping update.", "Type", found.GetKind())
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
