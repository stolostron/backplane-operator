// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"fmt"

	backplanev1alpha1 "github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	"github.com/open-cluster-management/backplane-operator/pkg/foundation"
	"github.com/open-cluster-management/backplane-operator/pkg/utils"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *BackplaneConfigReconciler) ensureDeployment(bpc *backplanev1alpha1.BackplaneConfig, dep *appsv1.Deployment) (ctrl.Result, error) {
	log := log.FromContext(context.Background())

	// if utils.ProxyEnvVarsAreSet() {
	// 	dep = addProxyEnvVarsToDeployment(dep)
	// }

	utils.AddBackplaneConfigLabels(dep, bpc.Name, bpc.Namespace)

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

func (r *BackplaneConfigReconciler) ensureService(bpc *backplanev1alpha1.BackplaneConfig, s *corev1.Service) (ctrl.Result, error) {
	log := log.FromContext(context.Background())

	utils.AddBackplaneConfigLabels(s, bpc.Name, bpc.Namespace)

	found := &corev1.Service{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      s.Name,
		Namespace: bpc.Namespace,
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

func (r *BackplaneConfigReconciler) ensureAPIService(bpc *backplanev1alpha1.BackplaneConfig, s *apiregistrationv1.APIService) (ctrl.Result, error) {
	log := log.FromContext(context.Background())

	utils.AddBackplaneConfigLabels(s, bpc.Name, bpc.Namespace)

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

func (r *BackplaneConfigReconciler) ensureUnstructuredResource(bpc *backplanev1alpha1.BackplaneConfig, u *unstructured.Unstructured) (ctrl.Result, error) {
	log := log.FromContext(context.Background())

	found := &unstructured.Unstructured{}
	found.SetGroupVersionKind(u.GroupVersionKind())

	utils.AddBackplaneConfigLabels(u, bpc.Name, bpc.Namespace)

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
	case "ClusterRole":
		desired, needsUpdate = utils.ValidateClusterRoleRules(found, u)
	case "CustomResourceDefinition", "HiveConfig":
		// skip update
		return ctrl.Result{}, nil
	default:
		log.Info("Could not validate unstructured resource. Skipping update.", "Type", found.GetKind())
		return ctrl.Result{}, nil
	}

	if needsUpdate {
		log.Info(fmt.Sprintf("Updating %s - %s", desired.GetKind(), desired.GetName()))
		err = r.Client.Update(context.TODO(), desired)
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
