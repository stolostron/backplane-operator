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

	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	clustermanager "open-cluster-management.io/api/operator/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	hiveconfig "github.com/openshift/hive/apis/hive/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	v1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/status"
	"github.com/stolostron/backplane-operator/pkg/utils"

	admissionregistration "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	//+kubebuilder:scaffold:imports
)

// Define utility constants for object names and testing timeouts/durations and intervals.

var _ = Describe("BackplaneConfig controller", func() {
	var (
		tests testList
	)

	AfterEach(func() {
		Expect(k8sClient.Delete(context.Background(), &configv1.ClusterVersion{
			ObjectMeta: metav1.ObjectMeta{
				Name: "version",
			},
		})).To(Succeed())
		Expect(k8sClient.Delete(context.Background(), &configv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster",
			},
		})).To(Succeed())
		Expect(k8sClient.Delete(context.Background(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testsecret",
				Namespace: DestinationNamespace,
			},
		})).To(Succeed())
	})

	BeforeEach(func() {
		// Create openshift-monitoring namespace because metrics stands up prometheus endpoint here
		err := k8sClient.Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "openshift-monitoring",
			},
			Spec: corev1.NamespaceSpec{},
		})
		if err != nil && !errors.IsAlreadyExists(err) {
			Expect(err).To(BeNil())
		}

		// Create target namespace
		err = k8sClient.Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: DestinationNamespace,
			},
			Spec: corev1.NamespaceSpec{},
		})
		if err != nil && !errors.IsAlreadyExists(err) {
			Expect(err).To(BeNil())
		}
		// Create ClusterVersion
		// Attempted to Store Version in status. Unable to get it to stick.
		Expect(k8sClient.Create(context.Background(), &configv1.ClusterVersion{
			ObjectMeta: metav1.ObjectMeta{
				Name: "version",
			},
			Spec: configv1.ClusterVersionSpec{
				Channel:   "stable-4.9",
				ClusterID: "12345678910",
			},
		})).To(Succeed())

		// Create ClusterIngress
		// Attempted to Store Version in status. Unable to get it to stick.
		Expect(k8sClient.Create(context.Background(), &configv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster",
			},
			Spec: configv1.IngressSpec{
				Domain: "apps.installer-test-cluster.dev00.red-chesterfield.com",
			},
		})).To(Succeed())

		Expect(k8sClient.Create(context.Background(), &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "hypershift-deployment",
				Namespace: DestinationNamespace,
			},
		})).To(Succeed())

		// Create test secret in target namespace
		testsecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testsecret",
				Namespace: DestinationNamespace,
			},
		}
		Eventually(func() error {
			err := k8sClient.Create(context.TODO(), testsecret)
			if apierrors.IsAlreadyExists(err) {
				return nil
			}
			return err
		}, timeout, interval).Should(Succeed())

		tests = testList{
			{
				Name:           "OCM Webhook",
				NamespacedName: types.NamespacedName{Name: "hypershift-deployment", Namespace: DestinationNamespace},
				ResourceType:   &corev1.ServiceAccount{},
			},
		}
	})

	When("creating a new BackplaneConfig", func() {

		Context("and no image pull policy is specified", func() {
			It("should deploy sub components", func() {
				// createCtx := context.Background()
				By("creating the backplane config")
				backplaneConfig := &v1.MultiClusterEngine{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "multicluster.openshift.io/v1",
						Kind:       "MultiClusterEngine",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: BackplaneConfigName,
					},
					Spec: v1.MultiClusterEngineSpec{
						TargetNamespace: DestinationNamespace,
						ImagePullSecret: "testsecret",
					},
				}
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

				k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient).NotTo(BeNil())

				k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
					Scheme:                 scheme.Scheme,
					MetricsBindAddress:     "0",
					HealthProbeBindAddress: "0",
				})
				Expect(err).ToNot(HaveOccurred())
				reconciler := &MultiClusterEngineReconciler{
					Client:        k8sManager.GetClient(),
					Scheme:        k8sManager.GetScheme(),
					StatusManager: &status.StatusTracker{Client: k8sManager.GetClient()},
				}
				err = (reconciler).SetupWithManager(k8sManager)
				Expect(err).ToNot(HaveOccurred())
				_, _ = reconciler.ensureRemovalsGone(backplaneConfig)
				for _, test := range tests {
					By(fmt.Sprintf("ensuring %s is created", test.Name))
					Eventually(func() bool {
						ctx := context.Background()
						err := k8sClient.Get(ctx, test.NamespacedName, test.ResourceType)
						return errors.IsNotFound(err)
					}, timeout, interval).Should(BeTrue())
				}

			})
		})

	})
})
