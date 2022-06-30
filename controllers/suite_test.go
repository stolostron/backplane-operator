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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	hiveconfig "github.com/openshift/hive/apis/hive/v1"
	admissionregistration "k8s.io/api/admissionregistration/v1"
	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	clustermanager "open-cluster-management.io/api/operator/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/status"
	"github.com/stolostron/backplane-operator/pkg/utils"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var signalHandlerContext context.Context

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	// SetupSignalHandler can only be called once, so we'll save the
	// context it returns and reuse it each time we start a new
	// manager.
	signalHandlerContext = ctrl.SetupSignalHandler()

	By("bootstrap test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "config", "crd", "bases"),
			filepath.Join("..", "pkg", "templates", "crds", "cluster-manager"),
			filepath.Join("..", "pkg", "templates", "crds", "hive-operator"),
			filepath.Join("..", "pkg", "templates", "crds", "foundation"),
			filepath.Join("..", "pkg", "templates", "crds", "cluster-lifecycle"),
			filepath.Join("..", "pkg", "templates", "crds", "discovery-operator"),
			filepath.Join("..", "pkg", "templates", "crds", "cluster-proxy-addon"),
			filepath.Join("..", "hack", "unit-test-crds"),
		},
		CRDInstallOptions: envtest.CRDInstallOptions{
			CleanUpAfterUse: true,
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = v1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = scheme.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = apiregistrationv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = admissionregistration.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = apixv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = hiveconfig.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = clustermanager.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = monitoringv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = configv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = operatorv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = os.Setenv("POD_NAMESPACE", "default")
	Expect(err).NotTo(HaveOccurred())

	err = os.Setenv("DIRECTORY_OVERRIDE", "../")
	Expect(err).NotTo(HaveOccurred())

	err = os.Setenv("UNIT_TEST", "true")
	Expect(err).NotTo(HaveOccurred())

	for _, v := range utils.GetTestImages() {
		key := fmt.Sprintf("OPERAND_IMAGE_%s", strings.ToUpper(v))
		err := os.Setenv(key, "quay.io/test/test:test")
		Expect(err).NotTo(HaveOccurred())
	}
	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme.Scheme,
		MetricsBindAddress:     "0",
		HealthProbeBindAddress: "0",
	})
	Expect(err).ToNot(HaveOccurred())

	err = (&MultiClusterEngineReconciler{
		Client:        k8sManager.GetClient(),
		Scheme:        k8sManager.GetScheme(),
		StatusManager: &status.StatusTracker{Client: k8sManager.GetClient()},
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		// For explanation of GinkgoRecover in a go routine, see
		// https://onsi.github.io/ginkgo/#mental-model-how-ginkgo-handles-failure
		defer GinkgoRecover()
		err = k8sManager.Start(signalHandlerContext)
		Expect(err).ToNot(HaveOccurred())
	}()
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := os.Unsetenv("OPERAND_IMAGE_TEST_IMAGE")
	Expect(err).NotTo(HaveOccurred())
	err = os.Unsetenv("POD_NAMESPACE")
	Expect(err).NotTo(HaveOccurred())
	err = os.Unsetenv("UNIT_TEST")
	Expect(err).NotTo(HaveOccurred())
	err = os.Unsetenv("DIRECTORY_OVERRIDE")
	Expect(err).NotTo(HaveOccurred())
})
