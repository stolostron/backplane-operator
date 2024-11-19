package mcewebhook

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/utils"
	admissionregistration "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	ControllerName           = "webhook-controller"
	MCEValidatingWebhookName = "multiclusterengines.multicluster.openshift.io"
)

// Reconciler reconciles for the webhooks
type Reconciler struct {
	Client    client.Client
	Namespace string
	log       logr.Logger
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (retRes ctrl.Result, retErr error) {
	r.log = log.Log.WithName(ControllerName)
	r.log.V(2).Info("Reconciling webhook controller")

	validatingWebhook := backplanev1.ValidatingWebhook(r.Namespace)
	existingWebhook := &admissionregistration.ValidatingWebhookConfiguration{}
	existingWebhook.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "admissionregistration.k8s.io",
		Version: "v1",
		Kind:    "ValidatingWebhookConfiguration",
	})
	err := r.Client.Get(ctx, types.NamespacedName{Name: MCEValidatingWebhookName}, existingWebhook)
	if err != nil && errors.IsNotFound(err) {
		return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, nil
	}
	if err != nil {
		r.log.Error(err, "Error getting validatingwebhookconfiguration")
		return ctrl.Result{}, err

	}

	if !utils.DeployOnOCP() {
		servingCertCABundle, err := utils.GetServingCertCABundle()
		if err != nil {
			fmt.Printf("error getting serving cert ca bundle: %s\n", err)
		} else {
			validatingWebhook.Webhooks[0].ClientConfig.CABundle = []byte(servingCertCABundle)
		}
	}

	if !equality.Semantic.DeepEqual(existingWebhook.Webhooks, validatingWebhook.Webhooks) {
		r.log.Info("Updating the validatingwebhookconfiguration " + MCEValidatingWebhookName)

		existingWebhook.Webhooks = validatingWebhook.Webhooks
		// err = r.Client.Update(ctx, existingWebhook)
		force := true
		err := r.Client.Patch(ctx, existingWebhook, client.Apply, &client.PatchOptions{Force: &force, FieldManager: "backplane-operator"})
		if err != nil {
			r.log.Error(err, "Error updating validatingwebhookconfiguration "+MCEValidatingWebhookName)
		}
		return ctrl.Result{}, err
	}

	// monitor if the ca bundle rotates
	return ctrl.Result{Requeue: true, RequeueAfter: time.Minute * 10}, nil
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named(ControllerName).
		Watches(&admissionregistration.ValidatingWebhookConfiguration{},
			&handler.Funcs{
				CreateFunc: func(ctx context.Context, e event.CreateEvent, q workqueue.RateLimitingInterface) {
					switch e.Object.GetName() {
					case MCEValidatingWebhookName:
						q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
							Name: e.Object.GetName(),
						}})
					}
				},
				UpdateFunc: func(ctx context.Context, e event.UpdateEvent, q workqueue.RateLimitingInterface) {
					switch e.ObjectNew.GetName() {
					case MCEValidatingWebhookName:
						q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
							Name: e.ObjectNew.GetName(),
						}})
					}
				},
				DeleteFunc: func(ctx context.Context, e event.DeleteEvent, q workqueue.RateLimitingInterface) {
					switch e.Object.GetName() {
					case MCEValidatingWebhookName:
						q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
							Name: e.Object.GetName(),
						}})
					}
				},
			}).Complete(r)
}
