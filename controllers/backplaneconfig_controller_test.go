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
	"time"

	"github.com/stolostron/backplane-operator/api/v1alpha1"
	"github.com/stolostron/backplane-operator/pkg/status"
	"github.com/stolostron/backplane-operator/pkg/utils"

	clustermanager "open-cluster-management.io/api/operator/v1"

	hiveconfig "github.com/openshift/hive/apis/hive/v1"

	admissionregistration "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	//+kubebuilder:scaffold:imports
)

// Define utility constants for object names and testing timeouts/durations and intervals.
const (
	BackplaneConfigName        = "test-backplaneconfig"
	BackplaneConfigTestName    = "Backplane Config"
	BackplaneOperatorNamespace = "default"
	DestinationNamespace       = "test"
	JobName                    = "test-job"

	timeout  = time.Second * 60
	duration = time.Second * 10
	interval = time.Millisecond * 250
)

type testList []struct {
	Name           string
	NamespacedName types.NamespacedName
	ResourceType   client.Object
	Expected       error
}

var _ = Describe("BackplaneConfig controller", func() {
	var (
		testEnv        *envtest.Environment
		clientConfig   *rest.Config
		k8sClient      client.Client
		clusterManager *unstructured.Unstructured
		hiveConfig     *unstructured.Unstructured
		tests          testList
	)

	BeforeEach(func() {
		testEnv = &envtest.Environment{
			CRDDirectoryPaths: []string{
				filepath.Join("..", "config", "crd", "bases"),
				filepath.Join("..", "pkg", "templates", "crds", "cluster-manager"),
				filepath.Join("..", "pkg", "templates", "crds", "hive-operator"),
				filepath.Join("..", "pkg", "templates", "crds", "foundation"),
			},
			CRDInstallOptions: envtest.CRDInstallOptions{
				CleanUpAfterUse: true,
			},
			ErrorIfCRDPathMissing: true,
		}

		clusterManager = &unstructured.Unstructured{}
		clusterManager.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "operator.open-cluster-management.io",
			Version: "v1",
			Kind:    "ClusterManager",
		})

		hiveConfig = &unstructured.Unstructured{}
		hiveConfig.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "hive.openshift.io",
			Version: "v1",
			Kind:    "HiveConfig",
		})

		tests = testList{
			{
				Name:           BackplaneConfigTestName,
				NamespacedName: types.NamespacedName{Name: BackplaneConfigName},
				ResourceType:   &v1alpha1.MultiClusterEngine{},
				Expected:       nil,
			},
			{
				Name:           "OCM Webhook",
				NamespacedName: types.NamespacedName{Name: "ocm-webhook", Namespace: DestinationNamespace},
				ResourceType:   &appsv1.Deployment{},
				Expected:       nil,
			},
			{
				Name:           "OCM Controller",
				NamespacedName: types.NamespacedName{Name: "ocm-controller", Namespace: DestinationNamespace},
				ResourceType:   &appsv1.Deployment{},
				Expected:       nil,
			},
			{
				Name:           "OCM Proxy Server",
				NamespacedName: types.NamespacedName{Name: "ocm-proxyserver", Namespace: DestinationNamespace},
				ResourceType:   &appsv1.Deployment{},
				Expected:       nil,
			},
			{
				Name:           "Cluster Manager Deployment",
				NamespacedName: types.NamespacedName{Name: "cluster-manager", Namespace: DestinationNamespace},
				ResourceType:   &appsv1.Deployment{},
				Expected:       nil,
			},
			{
				Name:           "Hive Operator Deployment",
				NamespacedName: types.NamespacedName{Name: "hive-operator", Namespace: DestinationNamespace},
				ResourceType:   &appsv1.Deployment{},
				Expected:       nil,
			},
			{
				Name:           "Managed Cluster Import Controller",
				NamespacedName: types.NamespacedName{Name: "managedcluster-import-controller-v2", Namespace: DestinationNamespace},
				ResourceType:   &appsv1.Deployment{},
				Expected:       nil,
			},
			{
				Name:           "Cluster Manager",
				NamespacedName: types.NamespacedName{Name: "cluster-manager"},
				ResourceType:   clusterManager,
				Expected:       nil,
			},
			{
				Name:           "Hive Config",
				NamespacedName: types.NamespacedName{Name: "hive"},
				ResourceType:   hiveConfig,
				Expected:       nil,
			},
		}
	})

	JustBeforeEach(func() {
		By("bootstrap test environment")
		var err error
		Eventually(func() error {
			clientConfig, err = testEnv.Start()
			return err
		}, timeout, interval).Should(Succeed())
		Expect(clientConfig).NotTo(BeNil())

		err = v1alpha1.AddToScheme(scheme.Scheme)
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

		err = os.Setenv("POD_NAMESPACE", "default")
		Expect(err).NotTo(HaveOccurred())

		for _, v := range utils.GetTestImages() {
			key := fmt.Sprintf("OPERAND_IMAGE_%s", strings.ToUpper(v))
			err := os.Setenv(key, "quay.io/test/test:test")
			Expect(err).NotTo(HaveOccurred())
		}
		//+kubebuilder:scaffold:scheme

		k8sClient, err = client.New(clientConfig, client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient).NotTo(BeNil())

		k8sManager, err := ctrl.NewManager(clientConfig, ctrl.Options{
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
		err = reconciler.SetupWithManager(k8sManager)
		Expect(err).ToNot(HaveOccurred())

		go func() {
			// For explanation of GinkgoRecover in a go routine, see
			// https://onsi.github.io/ginkgo/#mental-model-how-ginkgo-handles-failure
			defer GinkgoRecover()
			err := k8sManager.Start(signalHandlerContext)
			Expect(err).ToNot(HaveOccurred())
		}()
	})

	When("creating a new BackplaneConfig", func() {
		Context("and no image pull policy is specified", func() {
			It("should deploy sub components", func() {
				By("creating the backplane config")
				backplaneConfig := &v1alpha1.MultiClusterEngine{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "multicluster.openshift.io/v1alpha1",
						Kind:       "MultiClusterEngine",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: BackplaneConfigName,
					},
					Spec: v1alpha1.MultiClusterEngineSpec{
						TargetNamespace: DestinationNamespace,
						ImagePullSecret: "testsecret",
					},
				}
				createCtx := context.Background()
				Expect(k8sClient.Create(createCtx, backplaneConfig)).Should(Succeed())

				By("ensuring each deployment and config is created")
				for _, test := range tests {
					By(fmt.Sprintf("ensuring %s is created", test.Name))
					Eventually(func() bool {
						ctx := context.Background()
						err := k8sClient.Get(ctx, test.NamespacedName, test.ResourceType)
						return err == test.Expected
					}, timeout, interval).Should(BeTrue())
				}

				By("ensuring each deployment and config has an owner reference")
				for _, test := range tests {
					if test.Name == BackplaneConfigTestName {
						continue // config itself won't have ownerreference
					}
					By(fmt.Sprintf("ensuring %s has an ownerreference set", test.Name))
					Eventually(func(g Gomega) {
						ctx := context.Background()
						g.Expect(k8sClient.Get(ctx, test.NamespacedName, test.ResourceType)).To(Succeed())
						g.Expect(len(test.ResourceType.GetOwnerReferences())).To(
							Equal(1),
							fmt.Sprintf("Missing ownerreference on %s", test.Name),
						)
						g.Expect(test.ResourceType.GetOwnerReferences()[0].Name).To(Equal(BackplaneConfigName))
					}, timeout, interval).Should(Succeed())
				}

				By("ensuring each deployment has its imagePullPolicy set to IfNotPresent")
				for _, test := range tests {
					res, ok := test.ResourceType.(*appsv1.Deployment)
					if !ok {
						continue // only deployments will have an image pull policy
					}
					By(fmt.Sprintf("ensuring %s has its imagePullPolicy set to IfNotPresent", test.Name))
					Eventually(func(g Gomega) {
						ctx := context.Background()
						g.Expect(k8sClient.Get(ctx, test.NamespacedName, res)).To(Succeed())
						g.Expect(len(res.Spec.Template.Spec.Containers)).To(
							Equal(1),
							fmt.Sprintf("no containers in %s", test.Name),
						)
						g.Expect(res.Spec.Template.Spec.Containers[0].ImagePullPolicy).To(
							Equal(corev1.PullIfNotPresent),
						)
					}, timeout, interval).Should(Succeed())
				}
			})
		})

		Context("and an image pull policy is specified in an override", func() {
			It("should deploy sub components with the image pull policy in the override", func() {
				By("creating the backplane config with an image pull policy override")
				backplaneConfig := &v1alpha1.MultiClusterEngine{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "multicluster.openshift.io/v1alpha1",
						Kind:       "MultiClusterEngine",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: BackplaneConfigName,
					},
					Spec: v1alpha1.MultiClusterEngineSpec{
						TargetNamespace: DestinationNamespace,
						ImagePullSecret: "testsecret",
						Overrides: &v1alpha1.Overrides{
							ImagePullPolicy: corev1.PullAlways,
						},
					},
				}
				createCtx := context.Background()
				Expect(k8sClient.Create(createCtx, backplaneConfig)).Should(Succeed())

				By("ensuring each deployment and config is created")
				for _, test := range tests {
					By(fmt.Sprintf("ensuring %s is created", test.Name))
					Eventually(func() bool {
						ctx := context.Background()
						err := k8sClient.Get(ctx, test.NamespacedName, test.ResourceType)
						return err == test.Expected
					}, timeout, interval).Should(BeTrue())
				}

				By("ensuring each deployment has its imagePullPolicy set to Always (the override)")
				for _, test := range tests {
					res, ok := test.ResourceType.(*appsv1.Deployment)
					if !ok {
						continue // only deployments will have an image pull policy
					}
					By(fmt.Sprintf("ensuring %s has its imagePullPolicy set to Always", test.Name))
					Eventually(func(g Gomega) {
						ctx := context.Background()
						g.Expect(k8sClient.Get(ctx, test.NamespacedName, res)).To(Succeed())
						g.Expect(len(res.Spec.Template.Spec.Containers)).To(
							Equal(1),
							fmt.Sprintf("no containers in %s", test.Name),
						)
						g.Expect(res.Spec.Template.Spec.Containers[0].ImagePullPolicy).To(
							Equal(corev1.PullAlways),
						)
					}, timeout, interval).Should(Succeed())
				}
			})
		})
	})

	AfterEach(func() {
		By("tearing down the test environment")
		err := os.Unsetenv("OPERAND_IMAGE_TEST_IMAGE")
		Expect(err).NotTo(HaveOccurred())
		err = os.Unsetenv("POD_NAMESPACE")
		Expect(err).NotTo(HaveOccurred())
		err = testEnv.Stop()
		Expect(err).NotTo(HaveOccurred())
	})
})
