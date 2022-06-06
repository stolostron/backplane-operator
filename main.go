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

	renderer "github.com/stolostron/backplane-operator/pkg/rendering"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	operatorv1 "github.com/openshift/api/operator/v1"
	admissionregistration "k8s.io/api/admissionregistration/v1"
	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/yaml"

	configv1 "github.com/openshift/api/config/v1"
	hiveconfig "github.com/openshift/hive/apis/hive/v1"
	"github.com/stolostron/backplane-operator/pkg/status"
	"github.com/stolostron/backplane-operator/pkg/version"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/controllers"
	clustermanager "open-cluster-management.io/api/operator/v1"
	//+kubebuilder:scaffold:imports
)

const (
	crdName = "multiclusterengines.multicluster.openshift.io"
	crdsDir = "pkg/templates/crds"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(backplanev1.AddToScheme(scheme))

	utilruntime.Must(apiregistrationv1.AddToScheme(scheme))

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

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "797f9276.open-cluster-management.io",
		// LeaderElectionNamespace: "backplane-operator-system", // Ensure this is commented out. Uncomment only for running operator locally.
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.MultiClusterEngineReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		StatusManager: &status.StatusTracker{Client: mgr.GetClient()},
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

	for _, crd := range crds {
		err := ensureCRD(mgr, crd)
		if err != nil {
			setupLog.Info(err.Error())
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

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func ensureCRD(mgr ctrl.Manager, crd *unstructured.Unstructured) error {
	ctx := context.Background()
	maxAttempts := 5
	go func() {
		for i := 0; i < maxAttempts; i++ {
			setupLog.Info(fmt.Sprintf("Ensuring '%s' CRD exists", crd.GetName()))
			existingCRD := &unstructured.Unstructured{}
			existingCRD.SetGroupVersionKind(crd.GroupVersionKind())
			err := mgr.GetClient().Get(ctx, types.NamespacedName{Name: crd.GetName()}, existingCRD)
			if err != nil && errors.IsNotFound(err) {
				// CRD not found. Create and return
				err = mgr.GetClient().Create(ctx, crd)
				if err != nil {
					setupLog.Error(err, fmt.Sprintf("Error creating '%s' CRD", crd.GetName()))
					time.Sleep(5 * time.Second)
					continue
				}
				return
			} else if err != nil {
				setupLog.Error(err, fmt.Sprintf("Error getting '%s' CRD", crd.GetName()))
			} else if err == nil {
				// CRD already exists. Update and return
				setupLog.Info(fmt.Sprintf("'%s' CRD already exists. Updating.", crd.GetName()))
				crd.SetResourceVersion(existingCRD.GetResourceVersion())
				err = mgr.GetClient().Update(ctx, crd)
				if err != nil {
					setupLog.Error(err, fmt.Sprintf("Error updating '%s' CRD", crd.GetName()))
					time.Sleep(5 * time.Second)
					continue
				}
				return
			}
			time.Sleep(5 * time.Second)
		}

		setupLog.Info(fmt.Sprintf("Unable to ensure '%s' CRD exists in allotted time. Failing.", crd.GetName()))
		os.Exit(1)
	}()
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
