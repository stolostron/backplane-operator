// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"errors"
	"fmt"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/status"
	"github.com/stolostron/backplane-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
