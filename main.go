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
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/stolostron/backplane-operator/controllers/mcewebhook"
	"k8s.io/client-go/kubernetes"
	"open-cluster-management.io/sdk-go/pkg/servingcert"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	hiveconfig "github.com/openshift/hive/apis/hive/v1"
	operatorsapiv2 "github.com/operator-framework/api/pkg/operators/v2"
	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/controllers"
	"github.com/stolostron/backplane-operator/pkg/status"
	"github.com/stolostron/backplane-operator/pkg/utils"
	"github.com/stolostron/backplane-operator/pkg/version"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clustermanager "open-cluster-management.io/api/operator/v1"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"go.uber.org/zap/zapcore"
	admissionregistration "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	// +kubebuilder:scaffold:imports
)

const (
	crdName = "multiclusterengines.multicluster.openshift.io"
	crdsDir = "pkg/templates/crds"
)

var (
	cacheDuration time.Duration = time.Minute * 5
	scheme                      = runtime.NewScheme()
	setupLog                    = ctrl.Log.WithName("setup")
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

	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string

	var leaseDuration time.Duration
	var renewDeadline time.Duration
	var retryPeriod time.Duration
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", true,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	flag.DurationVar(&leaseDuration, "leader-election-lease-duration", 137*time.Second, ""+
		"The duration that non-leader candidates will wait after observing a leadership "+
		"renewal until attempting to acquire leadership of a led but unrenewed leader "+
		"slot. This is effectively the maximum duration that a leader can be stopped "+
		"before it is replaced by another candidate. This is only applicable if leader "+
		"election is enabled.")
	flag.DurationVar(&renewDeadline, "leader-election-renew-deadline", 107*time.Second, ""+
		"The interval between attempts by the acting master to renew a leadership slot "+
		"before it stops leading. This must be less than or equal to the lease duration. "+
		"This is only applicable if leader election is enabled.")
	flag.DurationVar(&retryPeriod, "leader-election-retry-period", 26*time.Second, ""+
		"The duration the clients should wait between attempting acquisition and renewal "+
		"of a leadership. This is only applicable if leader election is enabled.")
	opts := zap.Options{
		Development: true,
		TimeEncoder: zapcore.ISO8601TimeEncoder,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	ctrl.Log.WithName("Backplane Operator version").Info(fmt.Sprintf("%#v", version.Get()))

	mgrOptions := ctrl.Options{
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{},
			},
		},
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port: 9443,
			TLSOpts: []func(*tls.Config){
				func(config *tls.Config) {
					config.MinVersion = tls.VersionTLS13
				},
			},
		}),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "797f9276.open-cluster-management.io",
		LeaseDuration:          &leaseDuration,
		RenewDeadline:          &renewDeadline,
		RetryPeriod:            &retryPeriod,
		Controller: config.Controller{
			CacheSyncTimeout: cacheDuration,
		},
		// LeaderElectionNamespace: "backplane-operator-system", // Ensure this is commented out. Uncomment only for running operator locally.
	}

	setupLog.Info("Disabling Operator Client Cache for high-memory resources")
	mgrOptions.Client.Cache.DisableFor = []client.Object{
		&corev1.Secret{},
		&rbacv1.ClusterRole{},
		&rbacv1.ClusterRoleBinding{},
		&rbacv1.RoleBinding{},
		&corev1.ConfigMap{},
		&corev1.ServiceAccount{},
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
	err = utils.DetectOpenShift(uncachedClient)
	if err != nil {
		setupLog.Error(err, "unable to detect if cluster is openShift")
		os.Exit(1)
	}
	ctx := ctrl.SetupSignalHandler()
	upgradeableCondition := &utils.OperatorCondition{}

	if utils.DeployOnOCP() {
		// Force OperatorCondition Upgradeable to False
		//
		// We have to at least default the condition to False or
		// OLM will use the Readiness condition via our readiness probe instead:
		// https://olm.operatorframework.io/docs/advanced-tasks/communicating-operator-conditions-to-olm/#setting-defaults
		//
		// We want to force it to False to ensure that the final decision about whether
		// the operator can be upgraded stays within the mce controller.
		setupLog.Info("Setting OperatorCondition.")
		upgradeableCondition, err = utils.NewOperatorCondition(uncachedClient, operatorsapiv2.Upgradeable)
		if err != nil {
			setupLog.Error(err, "Cannot create the Upgradeable Operator Condition")
			os.Exit(1)
		}

		err = upgradeableCondition.Set(ctx, metav1.ConditionFalse, utils.UpgradeableInitReason, utils.UpgradeableInitMessage)
		if err != nil {
			setupLog.Error(err, "unable to create set operator condition upgradable to false")
			os.Exit(1)
		}
	}

	if err = (&controllers.MultiClusterEngineReconciler{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		UncachedClient:  uncachedClient,
		StatusManager:   &status.StatusTracker{Client: mgr.GetClient()},
		UpgradeableCond: upgradeableCondition,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MultiClusterEngine")
		os.Exit(1)
	}

	if !utils.DeployOnOCP() {
		kubeClient, err := kubernetes.NewForConfig(ctrl.GetConfigOrDie())
		if err != nil {
			setupLog.Error(err, "unable to create kubeClient")
			os.Exit(1)
		}

		operatorNamespace := utils.OperatorNamespace()

		utils.NewGlobalServingCertCABundleGetter(kubeClient, servingcert.DefaultCABundleConfigmapName, operatorNamespace)

		servingcert.NewServingCertController(utils.OperatorNamespace(), kubeClient).
			WithTargetServingCerts([]servingcert.TargetServingCertOptions{
				{
					Name:      "multicluster-engine-operator-webhook",
					HostNames: []string{fmt.Sprintf("multicluster-engine-operator-webhook-service.%s.svc", operatorNamespace)},
					LoadDir:   "/tmp/k8s-webhook-server/serving-certs",
				},
				{
					Name:      "ocm-webhook",
					HostNames: []string{fmt.Sprintf("ocm-webhook.%s.svc", operatorNamespace)},
				},
				{
					Name:      "clusterlifecycle-state-metrics-certs",
					HostNames: []string{fmt.Sprintf("clusterlifecycle-state-metrics-v2.%s.svc", operatorNamespace)},
				},
			}).Start(ctx)

		if err = (&mcewebhook.Reconciler{
			Client:    mgr.GetClient(),
			Namespace: operatorNamespace,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create mce webhook controller", "controller", "MultiClusterEngine")
			os.Exit(1)
		}
	}

	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		// https://book.kubebuilder.io/cronjob-tutorial/running.html#running-webhooks-locally, https://book.kubebuilder.io/multiversion-tutorial/webhooks.html#and-maingo
		if err = ensureWebhooks(uncachedClient); err != nil {
			setupLog.Error(err, "unable to ensure webhook", "webhook", "MultiClusterEngine")
			os.Exit(1)
		}

		if err = (&backplanev1.MultiClusterEngine{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "MultiClusterEngine")
			os.Exit(1)
		}
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	multiclusterengineList := &backplanev1.MultiClusterEngineList{}
	err = uncachedClient.List(ctx, multiclusterengineList)
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
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func ensureWebhooks(k8sClient client.Client) error {
	ctx := context.Background()

	deploymentNamespace, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		setupLog.Info("Failing due to being unable to locate webhook service namespace")
		os.Exit(1)
	}

	validatingWebhook := backplanev1.ValidatingWebhook(deploymentNamespace)

	maxAttempts := 10
	for i := 0; i < maxAttempts; i++ {
		setupLog.Info("Applying ValidatingWebhookConfiguration")

		// Get reference to MCE CRD to set as owner of the webhook
		// This way if the CRD is deleted the webhook will be removed with it
		crdKey := types.NamespacedName{Name: crdName}
		owner := &apixv1.CustomResourceDefinition{}
		if err := k8sClient.Get(ctx, crdKey, owner); err != nil {
			setupLog.Error(err, "Failed to get MCE CRD")
			time.Sleep(5 * time.Second)
			continue
		}
		validatingWebhook.SetOwnerReferences([]metav1.OwnerReference{
			{
				APIVersion: "apiextensions.k8s.io/v1",
				Kind:       "CustomResourceDefinition",
				Name:       owner.Name,
				UID:        owner.UID,
			},
		})
		if !utils.DeployOnOCP() {
			servingCertCABundle, err := utils.GetServingCertCABundle()
			if err != nil {
				fmt.Printf("error getting serving cert ca bundle: %s\n", err)
				time.Sleep(5 * time.Second)
				continue
			} else {
				validatingWebhook.Webhooks[0].ClientConfig.CABundle = []byte(servingCertCABundle)
			}
			if err = utils.DumpServingCertSecret(); err != nil {
				fmt.Printf("error dumping serving cert secret: %s\n", err)
				time.Sleep(5 * time.Second)
				continue
			}
		}

		existingWebhook := &admissionregistration.ValidatingWebhookConfiguration{}
		existingWebhook.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "admissionregistration.k8s.io",
			Version: "v1",
			Kind:    "ValidatingWebhookConfiguration",
		})
		err := k8sClient.Get(ctx, types.NamespacedName{Name: validatingWebhook.GetName()}, existingWebhook)
		if err != nil && errors.IsNotFound(err) {
			// Webhook not found. Create and return
			err = k8sClient.Create(ctx, validatingWebhook)
			if err != nil {
				setupLog.Error(err, "Error creating validatingwebhookconfiguration")
				time.Sleep(5 * time.Second)
				continue
			}
			return nil
		} else if err != nil {
			setupLog.Error(err, "Error getting validatingwebhookconfiguration")
			time.Sleep(5 * time.Second)
			continue

		} else {
			// Webhook already exists. Update and return
			setupLog.Info("Updating existing validatingwebhookconfiguration")
			existingWebhook.Webhooks = validatingWebhook.Webhooks
			err = k8sClient.Update(ctx, existingWebhook)
			if err != nil {
				setupLog.Error(err, "Error updating validatingwebhookconfiguration")
				time.Sleep(5 * time.Second)
				continue
			}
			return nil
		}
	}
	return fmt.Errorf("unable to ensure validatingwebhook exists in allotted time")
}
