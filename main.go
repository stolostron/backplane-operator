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

//go:generate go run pkg/templates/rbac.go

package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	operatorsapiv2 "github.com/operator-framework/api/pkg/operators/v2"
	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/controllers"
	renderer "github.com/stolostron/backplane-operator/pkg/rendering"
	"github.com/stolostron/backplane-operator/pkg/status"
	"github.com/stolostron/backplane-operator/pkg/utils"
	"github.com/stolostron/backplane-operator/pkg/version"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clustermanager "open-cluster-management.io/api/operator/v1"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	hiveconfig "github.com/openshift/hive/apis/hive/v1"

	olmv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/util/retry"

	admissionregistration "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/yaml"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	//+kubebuilder:scaffold:imports
)

const (
	crdName    = "multiclusterengines.multicluster.openshift.io"
	crdsDir    = "pkg/templates/crds"
	NoCacheEnv = "DISABLE_CLIENT_CACHE"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	if _, exists := os.LookupEnv("OPERATOR_VERSION"); !exists {
		panic("OPERATOR_VERSION not defined")
	}

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(backplanev1.AddToScheme(scheme))

	utilruntime.Must(apiregistrationv1.AddToScheme(scheme))

	utilruntime.Must(operatorsapiv2.AddToScheme(scheme))

	utilruntime.Must(admissionregistration.AddToScheme(scheme))

	utilruntime.Must(apixv1.AddToScheme(scheme))

	utilruntime.Must(hiveconfig.AddToScheme(scheme))

	utilruntime.Must(clustermanager.AddToScheme(scheme))

	utilruntime.Must(monitoringv1.AddToScheme(scheme))

	utilruntime.Must(configv1.AddToScheme(scheme))

	utilruntime.Must(operatorv1.AddToScheme(scheme))

	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", true,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	ctrl.Log.WithName("Backplane Operator version").Info(fmt.Sprintf("%#v", version.Get()))

	mgrOptions := ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "797f9276.open-cluster-management.io",
		WebhookServer:          &webhook.Server{TLSMinVersion: "1.2"},
		// LeaderElectionNamespace: "backplane-operator-system", // Ensure this is commented out. Uncomment only for running operator locally.
	}

	cacheSecrets := os.Getenv(NoCacheEnv)
	if len(cacheSecrets) > 0 {
		setupLog.Info("Operator Client Cache Disabled")
		mgrOptions.ClientDisableCacheFor = []client.Object{
			&corev1.Secret{},
			&olmv1alpha1.ClusterServiceVersion{},
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOptions)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// use uncached client for setup before manager starts
	uncachedClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{
		Scheme: mgr.GetScheme(),
	})
	if err != nil {
		setupLog.Error(err, "unable to create uncached client")
		os.Exit(1)
	}

	// Force OperatorCondition Upgradeable to False
	//
	// We have to at least default the condition to False or
	// OLM will use the Readiness condition via our readiness probe instead:
	// https://olm.operatorframework.io/docs/advanced-tasks/communicating-operator-conditions-to-olm/#setting-defaults
	//
	// We want to force it to False to ensure that the final decision about whether
	// the operator can be upgraded stays within the hyperconverged controller.
	setupLog.Info("Setting OperatorCondition.")
	upgradeableCondition, err := utils.NewOperatorCondition(uncachedClient, operatorsapiv2.Upgradeable)
	ctx := context.Background()

	if err != nil {
		setupLog.Error(err, "Cannot create the Upgradeable Operator Condition")
		os.Exit(1)
	}
	err = upgradeableCondition.Set(ctx, metav1.ConditionFalse, utils.UpgradeableInitReason, utils.UpgradeableInitMessage)
	if err != nil {
		setupLog.Error(err, "unable to create set operator condition upgradable to false")
		os.Exit(1)
	}

	if err = (&controllers.MultiClusterEngineReconciler{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		StatusManager:   &status.StatusTracker{Client: mgr.GetClient()},
		UpgradeableCond: upgradeableCondition,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MultiClusterEngine")
		os.Exit(1)
	}

	// Render CRD templates
	crdsDir := crdsDir
	crds, errs := renderer.RenderCRDs(crdsDir)
	if len(errs) > 0 {
		for _, err := range errs {
			setupLog.Info(err.Error())
		}
		os.Exit(1)
	}

	// udpate CRDs with retry
	for i := range crds {
		retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			crd := crds[i]
			e := ensureCRD(context.TODO(), uncachedClient, crd)
			return e
		})
		if retryErr != nil {
			setupLog.Error(err, "unable to ensure CRD exists in alloted time. Failing.")
			os.Exit(1)
		}
	}

	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		// https://book.kubebuilder.io/cronjob-tutorial/running.html#running-webhooks-locally, https://book.kubebuilder.io/multiversion-tutorial/webhooks.html#and-maingo
		if err = ensureWebhooks(mgr); err != nil {
			setupLog.Error(err, "unable to ensure webhook", "webhook", "MultiClusterEngine")
			os.Exit(1)
		}

		if err = (&backplanev1.MultiClusterEngine{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "MultiClusterEngine")
			os.Exit(1)
		}
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	multiclusterengineList := &backplanev1.MultiClusterEngineList{}
	err = uncachedClient.List(context.TODO(), multiclusterengineList)
	if err != nil {
		setupLog.Error(err, "Could not set List multicluster engines")
		os.Exit(1)
	}

	if len(multiclusterengineList.Items) == 0 {
		err = upgradeableCondition.Set(ctx, metav1.ConditionTrue, utils.UpgradeableAllowReason, utils.UpgradeableAllowMessage)
		if err != nil {
			setupLog.Error(err, "Could not set Operator Condition")
			os.Exit(1)
		}
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

}

func ensureCRD(ctx context.Context, c client.Client, crd *unstructured.Unstructured) error {
	existingCRD := &unstructured.Unstructured{}
	existingCRD.SetGroupVersionKind(crd.GroupVersionKind())
	err := c.Get(ctx, types.NamespacedName{Name: crd.GetName()}, existingCRD)
	if err != nil && errors.IsNotFound(err) {
		// CRD not found. Create and return
		setupLog.Info(fmt.Sprintf("creating CRD '%s'", crd.GetName()))
		err = c.Create(ctx, crd)
		if err != nil {
			return fmt.Errorf("error creating CRD '%s': %w", crd.GetName(), err)
		}
	} else if err != nil {
		return fmt.Errorf("error getting CRD '%s': %w", crd.GetName(), err)
	} else if err == nil {
		// CRD already exists. Update and return
		if utils.AnnotationPresent(utils.AnnotationMCEIgnore, existingCRD) {
			setupLog.Info(fmt.Sprintf("CRD '%s' has ignore label. Skipping update.", crd.GetName()))
			return nil
		}
		crd.SetResourceVersion(existingCRD.GetResourceVersion())
		setupLog.Info(fmt.Sprintf("updating CRD '%s'", crd.GetName()))
		err = c.Update(ctx, crd)
		if err != nil {
			return fmt.Errorf("error updating CRD '%s': %w", crd.GetName(), err)
		}
	}
	return nil
}

func ensureWebhooks(mgr ctrl.Manager) error {
	ctx := context.Background()

	deploymentNamespace, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		setupLog.Info("Failing due to being unable to locate webhook service namespace")
		os.Exit(1)
	}

	validatingWebhookPath := "pkg/templates/core/validatingwebhook.yaml"
	bytesFile, err := ioutil.ReadFile(validatingWebhookPath)
	if err != nil {
		return err
	}

	validatingWebhook := &admissionregistration.ValidatingWebhookConfiguration{}
	if err = yaml.Unmarshal(bytesFile, validatingWebhook); err != nil {
		return err
	}
	// Override all webhook service namespace definitions to be the same as the pod namespace.
	for i := 0; i < len(validatingWebhook.Webhooks); i++ {
		validatingWebhook.Webhooks[i].ClientConfig.Service.Namespace = deploymentNamespace
	}

	// Wait for manager cache to start and create webhook
	maxAttempts := 10
	go func() {
		for i := 0; i < maxAttempts; i++ {
			setupLog.Info("Ensuring validatingwebhook exists")
			crdKey := types.NamespacedName{Name: crdName}
			owner := &apixv1.CustomResourceDefinition{}
			if err := mgr.GetClient().Get(context.TODO(), crdKey, owner); err != nil {
				setupLog.Error(err, "Failed to get deployment")
				time.Sleep(5 * time.Second)
				continue
			}

			validatingWebhook.SetOwnerReferences([]metav1.OwnerReference{
				{
					APIVersion: owner.APIVersion,
					Kind:       owner.Kind,
					Name:       owner.Name,
					UID:        owner.UID,
				},
			})

			existingWebhook := &admissionregistration.ValidatingWebhookConfiguration{}
			existingWebhook.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "admissionregistration.k8s.io",
				Version: "v1",
				Kind:    "ValidatingWebhookConfiguration",
			})
			err = mgr.GetClient().Get(ctx, types.NamespacedName{Name: validatingWebhook.GetName()}, existingWebhook)
			if err != nil && errors.IsNotFound(err) {
				// Webhook not found. Create and return
				err = mgr.GetClient().Create(ctx, validatingWebhook)
				if err != nil {
					setupLog.Error(err, "Error creating validatingwebhookconfiguration")
					time.Sleep(5 * time.Second)
					continue
				}
				return
			} else if err != nil {
				setupLog.Error(err, "Error getting validatingwebhookconfiguration")
			} else if err == nil {
				// Webhook already exists. Update and return
				setupLog.Info("Validatingwebhook already exists. Updating ")
				existingWebhook.Webhooks = validatingWebhook.Webhooks
				err = mgr.GetClient().Update(ctx, existingWebhook)
				if err != nil {
					setupLog.Error(err, "Error updating validatingwebhookconfiguration")
					time.Sleep(5 * time.Second)
					continue
				}
				return
			}
			time.Sleep(5 * time.Second)
		}

		setupLog.Info("Unable to ensure validatingwebhook exists in allotted time. Failing.")
		os.Exit(1)
	}()

	return nil
}
