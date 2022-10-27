// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"errors"
	"fmt"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/foundation"
	renderer "github.com/stolostron/backplane-operator/pkg/rendering"
	"github.com/stolostron/backplane-operator/pkg/status"
	"github.com/stolostron/backplane-operator/pkg/toggle"
	"github.com/stolostron/backplane-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var ErrBadFormat = errors.New("bad format")

func (r *MultiClusterEngineReconciler) HostedReconcile(ctx context.Context, mce *backplanev1.MultiClusterEngine) error {
	secretNN, err := utils.GetHostedCredentialsSecret(mce)
	if err != nil {
		mce.Status = backplanev1.MultiClusterEngineStatus{
			Conditions: []backplanev1.MultiClusterEngineCondition{
				status.NewCondition(backplanev1.MultiClusterEngineFailure, metav1.ConditionTrue, status.RequirementsNotMetReason, err.Error()),
			},
			Phase: backplanev1.MultiClusterEnginePhaseError,
		}
	}

	// Parse Kube credentials from secret
	kubeConfigSecret := &corev1.Secret{}
	if err := r.Get(context.TODO(), secretNN, kubeConfigSecret); err != nil {
		if apierrors.IsNotFound(err) {
			mce.Status = backplanev1.MultiClusterEngineStatus{
				Conditions: []backplanev1.MultiClusterEngineCondition{
					status.NewCondition(backplanev1.MultiClusterEngineFailure, metav1.ConditionTrue, status.RequirementsNotMetReason, err.Error()),
				},
				Phase: backplanev1.MultiClusterEnginePhaseError,
			}
			return err
		}
	}
	kubeconfig, err := parseKubeCreds(kubeConfigSecret)
	if err != nil {
		err = fmt.Errorf("error parsing kubeconfig from secret `%s/%s`: %w", kubeConfigSecret.Namespace, kubeConfigSecret.Name, err)
		mce.Status = backplanev1.MultiClusterEngineStatus{
			Conditions: []backplanev1.MultiClusterEngineCondition{
				status.NewCondition(backplanev1.MultiClusterEngineFailure, metav1.ConditionTrue, status.RequirementsNotMetReason, err.Error()),
			},
			Phase: backplanev1.MultiClusterEnginePhaseError,
		}
		return err
	}

	restconfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		mce.Status = backplanev1.MultiClusterEngineStatus{
			Conditions: []backplanev1.MultiClusterEngineCondition{
				status.NewCondition(backplanev1.MultiClusterEngineFailure, metav1.ConditionTrue, status.RequirementsNotMetReason, err.Error()),
			},
			Phase: backplanev1.MultiClusterEnginePhaseError,
		}
		return err
	}

	uncachedClient, err := client.New(restconfig, client.Options{
		Scheme: r.Client.Scheme(),
	})
	if err != nil {
		mce.Status = backplanev1.MultiClusterEngineStatus{
			Conditions: []backplanev1.MultiClusterEngineCondition{
				status.NewCondition(backplanev1.MultiClusterEngineFailure, metav1.ConditionTrue, status.RequirementsNotMetReason, err.Error()),
			},
			Phase: backplanev1.MultiClusterEnginePhaseError,
		}
		return err
	}

	// Create hosted ClusterManager
	if !mce.Enabled(backplanev1.ClusterManager) {
		result, err := r.ensureNoClusterManager(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ClusterManager] = err
		}
	} else {
		result, err := r.ensureClusterManager(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ClusterManager] = err
		}
	}

	err = uncachedClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: mce.Spec.TargetNamespace},
	})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		mce.Status = backplanev1.MultiClusterEngineStatus{
			Conditions: []backplanev1.MultiClusterEngineCondition{
				status.NewCondition(backplanev1.MultiClusterEngineFailure, metav1.ConditionTrue, status.RequirementsNotMetReason, err.Error()),
			},
			Phase: backplanev1.MultiClusterEnginePhaseError,
		}
		return err
	}

	mce.Status = backplanev1.MultiClusterEngineStatus{
		Conditions: []backplanev1.MultiClusterEngineCondition{
			status.NewCondition(backplanev1.MultiClusterEngineAvailable, metav1.ConditionTrue, status.DeploySuccessReason, "Hosted reconcile completed successfully."),
		},
		Phase: backplanev1.MultiClusterEnginePhaseAvailable,
	}
	return nil
}

// parseKubeCreds takes a secret cotaining credentials and returns the stored Kubeconfig.
func parseKubeCreds(secret *corev1.Secret) ([]byte, error) {
	kubeconfig, ok := secret.Data["kubeconfig"]
	if !ok {
		return []byte{}, fmt.Errorf("%s: %w", secret.Name, ErrBadFormat)
	}
	return kubeconfig, nil
}

func (r *MultiClusterEngineReconciler) ensureClusterManager(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	r.StatusManager.AddComponent(status.ClusterManagerStatus{
		NamespacedName: types.NamespacedName{Name: "hosted-cluster-manager"},
	})

	// Apply clustermanager
	cmTemplate := foundation.HostedClusterManager(backplaneConfig, r.Images)
	if err := ctrl.SetControllerReference(backplaneConfig, cmTemplate, r.Scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("Error setting controller reference on resource `%s`: %w", cmTemplate.GetName(), err)
	}
	force := true
	err := r.Client.Patch(ctx, cmTemplate, client.Apply, &client.PatchOptions{Force: &force, FieldManager: "backplane-operator"})
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error applying object Name: %s Kind: %s", cmTemplate.GetName(), cmTemplate.GetKind())
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoClusterManager(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	namespacedName := types.NamespacedName{Name: "cluster-manager", Namespace: backplaneConfig.Spec.TargetNamespace}

	// Renders all templates from charts
	templates, errs := renderer.RenderChart(toggle.ClusterManagerChartDir, backplaneConfig, r.Images)
	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))
	r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.RemoveComponent(status.ClusterManagerStatus{
		NamespacedName: types.NamespacedName{Name: "cluster-manager"},
	})

	// Delete clustermanager
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
		err := r.Client.Delete(ctx, clusterManager)
		if err != nil {
			return ctrl.Result{RequeueAfter: requeuePeriod}, err
		}
	} else if err != nil && !apierrors.IsNotFound(err) { // Return error, if error is not not found error
		return ctrl.Result{RequeueAfter: requeuePeriod}, err
	}

	// Verify clustermanager namespace deleted
	ocmHubNamespace := &corev1.Namespace{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: "open-cluster-management-hub"}, ocmHubNamespace)
	if err == nil {
		return ctrl.Result{RequeueAfter: requeuePeriod}, fmt.Errorf("waiting for 'open-cluster-management-hub' namespace to be terminated before proceeding with clustermanager cleanup")
	}
	if err != nil && !apierrors.IsNotFound(err) { // Return error, if error is not not found error
		return ctrl.Result{RequeueAfter: requeuePeriod}, err
	}

	// Deletes all templates
	for _, template := range templates {
		result, err := r.deleteTemplate(ctx, backplaneConfig, template)
		if err != nil {
			log.Error(err, fmt.Sprintf("Failed to delete template: %s", template.GetName()))
			return result, err
		}
	}
	return ctrl.Result{}, nil
}
