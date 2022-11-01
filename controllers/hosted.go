// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"errors"
	"fmt"
	"time"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/foundation"
	"github.com/stolostron/backplane-operator/pkg/images"
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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var ErrBadFormat = errors.New("bad format")

func (r *MultiClusterEngineReconciler) HostedReconcile(ctx context.Context, mce *backplanev1.MultiClusterEngine) (retRes ctrl.Result, retErr error) {
	log := log.FromContext(ctx)

	defer func() {
		mce.Status = r.StatusManager.ReportStatus(*mce)
		err := r.Client.Status().Update(ctx, mce)
		if mce.Status.Phase != backplanev1.MultiClusterEnginePhaseAvailable && !utils.IsPaused(mce) {
			retRes = ctrl.Result{RequeueAfter: requeuePeriod}
		}
		if err != nil {
			retErr = err
		}
	}()

	// If deletion detected, finalize backplane config
	if mce.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(mce, backplaneFinalizer) {
			err := r.finalizeHostedBackplaneConfig(ctx, mce) // returns all errors
			if err != nil {
				log.Info(err.Error())
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}

			log.Info("all subcomponents have been finalized successfully - removing finalizer")
			controllerutil.RemoveFinalizer(mce, backplaneFinalizer)
			if err := r.Client.Update(ctx, mce); err != nil {
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil // Object finalized successfully
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(mce, backplaneFinalizer) {
		controllerutil.AddFinalizer(mce, backplaneFinalizer)
		if err := r.Client.Update(ctx, mce); err != nil {
			return ctrl.Result{}, err
		}
	}

	var result ctrl.Result
	var err error

	result, err = r.setHostedDefaults(ctx, mce)
	if result != (ctrl.Result{}) {
		return ctrl.Result{}, err
	}
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	result, err = r.validateNamespace(ctx, mce)
	if result != (ctrl.Result{}) {
		return ctrl.Result{}, err
	}
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	result, err = r.validateImagePullSecret(ctx, mce)
	if result != (ctrl.Result{}) {
		return ctrl.Result{}, err
	}
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	// Read images from environmental variables
	imgs, err := images.GetImagesWithOverrides(r.Client, mce)
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
	if utils.IsPaused(mce) {
		log.Info("MultiClusterEngine reconciliation is paused. Nothing more to do.")
		r.StatusManager.AddCondition(status.NewCondition(backplanev1.MultiClusterEngineProgressing, metav1.ConditionUnknown, status.PausedReason, "Multiclusterengine is paused"))
		return ctrl.Result{}, nil
	}

	hostedClient, err := r.GetHostedClient(ctx, mce)

	// Create hosted ClusterManager
	if !mce.Enabled(backplanev1.ClusterManager) {
		result, err := r.ensureHostedClusterManager(ctx, hostedClient, mce)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ClusterManager] = err
		}
	} else {
		result, err := r.ensureNoHostedClusterManager(ctx, hostedClient, mce)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ClusterManager] = err
		}
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) GetHostedClient(ctx context.Context, mce *backplanev1.MultiClusterEngine) (client.Client, error) {
	secretNN, err := utils.GetHostedCredentialsSecret(mce)
	if err != nil {
		return nil, err
		// mce.Status = backplanev1.MultiClusterEngineStatus{
		// 	Conditions: []backplanev1.MultiClusterEngineCondition{
		// 		status.NewCondition(backplanev1.MultiClusterEngineFailure, metav1.ConditionTrue, status.RequirementsNotMetReason, err.Error()),
		// 	},
		// 	Phase: backplanev1.MultiClusterEnginePhaseError,
		// }
	}

	// Parse Kube credentials from secret
	kubeConfigSecret := &corev1.Secret{}
	if err := r.Get(context.TODO(), secretNN, kubeConfigSecret); err != nil {
		if apierrors.IsNotFound(err) {
			if err != nil {
				return nil, err
			}
			// mce.Status = backplanev1.MultiClusterEngineStatus{
			// 	Conditions: []backplanev1.MultiClusterEngineCondition{
			// 		status.NewCondition(backplanev1.MultiClusterEngineFailure, metav1.ConditionTrue, status.RequirementsNotMetReason, err.Error()),
			// 	},
			// 	Phase: backplanev1.MultiClusterEnginePhaseError,
			// }
			// return err
		}
	}
	kubeconfig, err := parseKubeCreds(kubeConfigSecret)
	if err != nil {
		return nil, err
	}
	// if err != nil {
	// 	err = fmt.Errorf("error parsing kubeconfig from secret `%s/%s`: %w", kubeConfigSecret.Namespace, kubeConfigSecret.Name, err)
	// 	mce.Status = backplanev1.MultiClusterEngineStatus{
	// 		Conditions: []backplanev1.MultiClusterEngineCondition{
	// 			status.NewCondition(backplanev1.MultiClusterEngineFailure, metav1.ConditionTrue, status.RequirementsNotMetReason, err.Error()),
	// 		},
	// 		Phase: backplanev1.MultiClusterEnginePhaseError,
	// 	}
	// 	return err
	// }

	restconfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	// if err != nil {
	// 	mce.Status = backplanev1.MultiClusterEngineStatus{
	// 		Conditions: []backplanev1.MultiClusterEngineCondition{
	// 			status.NewCondition(backplanev1.MultiClusterEngineFailure, metav1.ConditionTrue, status.RequirementsNotMetReason, err.Error()),
	// 		},
	// 		Phase: backplanev1.MultiClusterEnginePhaseError,
	// 	}
	// 	return err
	// }

	uncachedClient, err := client.New(restconfig, client.Options{
		Scheme: r.Client.Scheme(),
	})
	if err != nil {
		return nil, err
	}
	// if err != nil {
	// 	mce.Status = backplanev1.MultiClusterEngineStatus{
	// 		Conditions: []backplanev1.MultiClusterEngineCondition{
	// 			status.NewCondition(backplanev1.MultiClusterEngineFailure, metav1.ConditionTrue, status.RequirementsNotMetReason, err.Error()),
	// 		},
	// 		Phase: backplanev1.MultiClusterEnginePhaseError,
	// 	}
	// 	return err
	// }

	return uncachedClient, nil

	// err = uncachedClient.Create(ctx, &corev1.Namespace{
	// 	ObjectMeta: metav1.ObjectMeta{Name: mce.Spec.TargetNamespace},
	// })
	// if err != nil && !apierrors.IsAlreadyExists(err) {
	// 	mce.Status = backplanev1.MultiClusterEngineStatus{
	// 		Conditions: []backplanev1.MultiClusterEngineCondition{
	// 			status.NewCondition(backplanev1.MultiClusterEngineFailure, metav1.ConditionTrue, status.RequirementsNotMetReason, err.Error()),
	// 		},
	// 		Phase: backplanev1.MultiClusterEnginePhaseError,
	// 	}
	// 	return err
	// }

	// mce.Status = backplanev1.MultiClusterEngineStatus{
	// 	Conditions: []backplanev1.MultiClusterEngineCondition{
	// 		status.NewCondition(backplanev1.MultiClusterEngineAvailable, metav1.ConditionTrue, status.DeploySuccessReason, "Hosted reconcile completed successfully."),
	// 	},
	// 	Phase: backplanev1.MultiClusterEnginePhaseAvailable,
	// }
	// return nil
}

// parseKubeCreds takes a secret cotaining credentials and returns the stored Kubeconfig.
func parseKubeCreds(secret *corev1.Secret) ([]byte, error) {
	kubeconfig, ok := secret.Data["kubeconfig"]
	if !ok {
		return []byte{}, fmt.Errorf("%s: %w", secret.Name, ErrBadFormat)
	}
	return kubeconfig, nil
}

func (r *MultiClusterEngineReconciler) ensureHostedClusterManager(ctx context.Context, c client.Client, mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	r.StatusManager.AddComponent(status.ClusterManagerStatus{
		NamespacedName: types.NamespacedName{Name: "hosted-cluster-manager"},
	})

	cmName := fmt.Sprintf("%s-cluster-manager", mce.Name)

	// Apply namespace
	newNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: cmName,
		},
	}
	checkNs := &corev1.Namespace{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{Name: cmName}, checkNs)
	if err != nil && apierrors.IsNotFound(err) {
		err = r.Client.Create(context.TODO(), newNs)
		if err != nil {
			log.Error(err, "Could not create namespace")
			return ctrl.Result{}, err
		}
		log.Info("Namespace for hosted cluster-manager created")
		return ctrl.Result{Requeue: true}, nil
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{Requeue: true}, err
	}

	// Apply secret in namespace
	kubeconfigSecret := &corev1.Secret{}
	secretNN, err := utils.GetHostedCredentialsSecret(mce)
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}
	err = r.Client.Get(ctx, secretNN, kubeconfigSecret)
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	cmSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: kubeconfigSecret.APIVersion,
			Kind:       kubeconfigSecret.Kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "external-hub-kubeconfig",
			Namespace: cmName,
			Labels:    kubeconfigSecret.Labels,
		},
		Data: kubeconfigSecret.Data,
		Type: corev1.SecretTypeOpaque,
	}

	force := true
	err = r.Client.Patch(context.TODO(), cmSecret, client.Apply, &client.PatchOptions{Force: &force, FieldManager: "backplane-operator"})
	if err != nil {
		log.Info(fmt.Sprintf("Error applying kubeconfig secret to hosted cluster-manager namespace: %s", err.Error()))
		return ctrl.Result{Requeue: true}, nil
	}

	// Apply clustermanager
	cmTemplate := foundation.HostedClusterManager(mce, r.Images)
	if err := ctrl.SetControllerReference(mce, cmTemplate, r.Scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("Error setting controller reference on resource `%s`: %w", cmTemplate.GetName(), err)
	}
	force = true
	err = r.Client.Patch(ctx, cmTemplate, client.Apply, &client.PatchOptions{Force: &force, FieldManager: "backplane-operator"})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error applying object Name: %s Kind: %s, %w", cmTemplate.GetName(), cmTemplate.GetKind(), err)
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoHostedClusterManager(ctx context.Context, c client.Client, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
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

// setHostedDefaults configures the MCE with default values and updates
func (r *MultiClusterEngineReconciler) setHostedDefaults(ctx context.Context, m *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
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

	if utils.SetHostedDefaultComponents(m) {
		updateNecessary = true
	}

	if utils.DeduplicateComponents(m) {
		updateNecessary = true
	}

	// Apply defaults to server
	if updateNecessary {
		log.Info("Setting hosted defaults")
		err := r.Client.Update(ctx, m)
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

func (r *MultiClusterEngineReconciler) finalizeHostedBackplaneConfig(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) error {
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
