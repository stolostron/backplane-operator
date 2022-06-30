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
	"time"

	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	configv1 "github.com/openshift/api/config/v1"

	v1 "github.com/stolostron/backplane-operator/api/v1"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
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
		clusterManager         *unstructured.Unstructured
		hiveConfig             *unstructured.Unstructured
		clusterManagementAddon *unstructured.Unstructured
		tests                  testList
		msaTests               testList
	)

	AfterEach(func() {
		Expect(k8sClient.Delete(context.Background(), &v1.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{
				Name: BackplaneConfigName,
			},
		})).To(Succeed())
		Eventually(func() bool {
			foundMCE := &v1.MultiClusterEngine{}
			err := k8sClient.Get(context.TODO(), types.NamespacedName{Name: BackplaneConfigName}, foundMCE)
			return apierrors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue())
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

		clusterManagementAddon = &unstructured.Unstructured{}
		clusterManagementAddon.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "addon.open-cluster-management.io",
			Version: "v1alpha1",
			Kind:    "ClusterManagementAddOn",
		})

		tests = testList{
			{
				Name:           BackplaneConfigTestName,
				NamespacedName: types.NamespacedName{Name: BackplaneConfigName},
				ResourceType:   &v1.MultiClusterEngine{},
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
				Name:           "Discovery Operator Deployment",
				NamespacedName: types.NamespacedName{Name: "discovery-operator", Namespace: DestinationNamespace},
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
				Name:           "Cluster Curator Controller",
				NamespacedName: types.NamespacedName{Name: "cluster-curator-controller", Namespace: DestinationNamespace},
				ResourceType:   &appsv1.Deployment{},
				Expected:       nil,
			},
			{
				Name:           "Cluster Claims Controller",
				NamespacedName: types.NamespacedName{Name: "clusterclaims-controller", Namespace: DestinationNamespace},
				ResourceType:   &appsv1.Deployment{},
				Expected:       nil,
			},
			{
				Name:           "ClusterLifecycle State Metrics",
				NamespacedName: types.NamespacedName{Name: "clusterlifecycle-state-metrics-v2", Namespace: DestinationNamespace},
				ResourceType:   &appsv1.Deployment{},
				Expected:       nil,
			},
			{
				Name:           "Provider Credentials Controller",
				NamespacedName: types.NamespacedName{Name: "provider-credential-controller", Namespace: DestinationNamespace},
				ResourceType:   &appsv1.Deployment{},
				Expected:       nil,
			},
			{
				Name:           "Assisted Installer",
				NamespacedName: types.NamespacedName{Name: "infrastructure-operator", Namespace: DestinationNamespace},
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
			{
				Name:           "worker-manager ClusterManagementAddon",
				NamespacedName: types.NamespacedName{Name: "work-manager"},
				ResourceType:   clusterManagementAddon,
				Expected:       nil,
			},
			{
				Name:           "Cluster Proxy Addon Manager",
				NamespacedName: types.NamespacedName{Name: "cluster-proxy-addon-manager", Namespace: DestinationNamespace},
				ResourceType:   &appsv1.Deployment{},
				Expected:       nil,
			},
			{
				Name:           "Cluster Proxy Addon User",
				NamespacedName: types.NamespacedName{Name: "cluster-proxy-addon-user", Namespace: DestinationNamespace},
				ResourceType:   &appsv1.Deployment{},
				Expected:       nil,
			},
		}

		msaTests = testList{
			{
				Name:           "Managed-ServiceAccount Deployment",
				NamespacedName: types.NamespacedName{Name: "managed-serviceaccount-addon-manager", Namespace: DestinationNamespace},
				ResourceType:   &appsv1.Deployment{},
				Expected:       nil,
			},
			{
				Name:           "Managed-ServiceAccount ServiceAccount",
				NamespacedName: types.NamespacedName{Name: "managed-serviceaccount", Namespace: DestinationNamespace},
				ResourceType:   &corev1.ServiceAccount{},
				Expected:       nil,
			},
			{
				Name:           "Managed-ServiceAccount CRD",
				NamespacedName: types.NamespacedName{Name: "managedserviceaccounts.authentication.open-cluster-management.io"},
				ResourceType:   &apixv1.CustomResourceDefinition{},
				Expected:       nil,
			},
			{
				Name:           "Managed-ServiceAccount ClusterManagementAddon",
				NamespacedName: types.NamespacedName{Name: "managed-serviceaccount"},
				ResourceType:   clusterManagementAddon,
				Expected:       nil,
			},
		}
	})

	When("creating a new BackplaneConfig", func() {
		Context("and no image pull policy is specified", func() {
			It("should deploy sub components", func() {
				createCtx := context.Background()
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
							Not(Equal(0)),
							fmt.Sprintf("no containers in %s", test.Name),
						)
						g.Expect(res.Spec.Template.Spec.Containers[0].ImagePullPolicy).To(
							Equal(corev1.PullIfNotPresent),
						)
					}, timeout, interval).Should(Succeed())
				}

				By("ensuring the ServiceMonitor resource is recreated if deleted")
				Eventually(func() error {
					ctx := context.Background()
					u := &unstructured.Unstructured{}
					u.SetName("clusterlifecycle-state-metrics-v2")
					u.SetNamespace("openshift-monitoring")
					u.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "monitoring.coreos.com",
						Kind:    "ServiceMonitor",
						Version: "v1",
					})
					return k8sClient.Delete(ctx, u)
				}, timeout, interval).Should(Succeed())
				Eventually(func() error {
					ctx := context.Background()
					namespacedName := types.NamespacedName{
						Name:      "clusterlifecycle-state-metrics-v2",
						Namespace: "openshift-monitoring",
					}
					resourceType := &monitoringv1.ServiceMonitor{}
					return k8sClient.Get(ctx, namespacedName, resourceType)
				}, timeout, interval).Should(Succeed())

			})
		})

		Context("and an image pull policy is specified in an override", func() {
			It("should deploy sub components with the image pull policy in the override", func() {
				By("creating the backplane config with an image pull policy override")
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
						Overrides: &v1.Overrides{
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
							Not(Equal(0)),
							fmt.Sprintf("no containers in %s", test.Name),
						)
						g.Expect(res.Spec.Template.Spec.Containers[0].ImagePullPolicy).To(
							Equal(corev1.PullAlways),
						)
					}, timeout, interval).Should(Succeed())
				}
			})
		})

		Context("and enable ManagedServiceAccount", func() {
			It("should deploy sub components", func() {
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
						Overrides: &v1.Overrides{
							Components: []v1.ComponentConfig{
								{
									Name:    v1.ManagedServiceAccount,
									Enabled: true,
								},
							},
						},
					},
				}
				createCtx := context.Background()
				Expect(k8sClient.Create(createCtx, backplaneConfig)).Should(Succeed())
				withMSATests := append(tests, msaTests...)
				By("ensuring each deployment and config is created")
				for _, test := range withMSATests {
					By(fmt.Sprintf("ensuring %s is created", test.Name))
					Eventually(func() bool {
						ctx := context.Background()
						err := k8sClient.Get(ctx, test.NamespacedName, test.ResourceType)
						return err == test.Expected
					}, timeout, interval).Should(BeTrue())
				}

				By("ensuring each deployment and config has an owner reference")
				for _, test := range withMSATests {
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

			})
		})

		Context("and components are defined multiple times in overrides", func() {
			It("should deduplicate the component list in the override", func() {
				By("creating the backplane config with repeated component")
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
						Overrides: &v1.Overrides{
							ImagePullPolicy: corev1.PullAlways,
							Components: []v1.ComponentConfig{
								{
									Name:    v1.Discovery,
									Enabled: true,
								},
								{
									Name:    v1.Discovery,
									Enabled: true,
								},
								{
									Name:    v1.Discovery,
									Enabled: false,
								},
							},
						},
					},
				}
				createCtx := context.Background()
				Expect(k8sClient.Create(createCtx, backplaneConfig)).Should(Succeed())

				By("ensuring component is collapsed to one, matching last config")
				Eventually(func(g Gomega) {
					multiClusterEngine := types.NamespacedName{
						Name: BackplaneConfigName,
					}
					existingMCE := &v1.MultiClusterEngine{}
					g.Expect(k8sClient.Get(context.TODO(), multiClusterEngine, existingMCE)).To(Succeed(), "Failed to create new MCE")

					g.Expect(existingMCE.Spec.Overrides).To(Not(BeNil()))
					componentCount := 0
					for _, c := range existingMCE.Spec.Overrides.Components {
						if c.Name == v1.Discovery {
							componentCount++
						}
					}
					g.Expect(componentCount).To(Equal(1), "Duplicate component still present")

					g.Expect(existingMCE.Enabled(v1.Discovery)).To(BeFalse(), "Not using last defined config in components")

				}, timeout, interval).Should(Succeed())

			})
		})

		Context("and images are overriden using annotations", func() {
			It("should deploy images with a custom image repository", func() {
				imageRepo := "quay.io/testrepo"
				By("creating the backplane config with the image repository annotation")
				backplaneConfig := &v1.MultiClusterEngine{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "multicluster.openshift.io/v1",
						Kind:       "MultiClusterEngine",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: BackplaneConfigName,
						Annotations: map[string]string{
							"imageRepository": imageRepo,
						},
					},
					Spec: v1.MultiClusterEngineSpec{
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

				By("ensuring each deployment has its image repository overridden")
				for _, test := range tests {
					res, ok := test.ResourceType.(*appsv1.Deployment)
					if !ok {
						continue // only deployments will have an image pull policy
					}
					By(fmt.Sprintf("ensuring %s has its image using %s", test.Name, imageRepo))
					Eventually(func(g Gomega) {
						ctx := context.Background()
						g.Expect(k8sClient.Get(ctx, test.NamespacedName, res)).To(Succeed())
						g.Expect(res.Spec.Template.Spec.Containers[0].Image).To(
							HavePrefix(imageRepo),
							fmt.Sprintf("Image does not have expected repository"),
						)
					}, timeout, interval).Should(Succeed())
				}
			})

			It("should replace images as defined in a configmap", func() {
				By("creating a configmap with an image override")
				testCM := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: BackplaneOperatorNamespace,
					},
					Data: map[string]string{
						"overrides.json": `[
							{
								"image-name": "discovery-operator",
								"image-remote": "quay.io/stolostron",
								"image-digest": "sha256:9dc4d072dcd06eda3fda19a15f4b84677fbbbde2a476b4817272cde4724f02cc",
								"image-key": "discovery_operator"
								}
						]`,
					},
				}
				Expect(k8sClient.Create(context.TODO(), testCM)).To(Succeed())

				By("creating the backplane config with the configmap override annotation")
				backplaneConfig := &v1.MultiClusterEngine{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "multicluster.openshift.io/v1",
						Kind:       "MultiClusterEngine",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: BackplaneConfigName,
						Annotations: map[string]string{
							"imageOverridesCM": "test",
						},
					},
					Spec: v1.MultiClusterEngineSpec{
						TargetNamespace: DestinationNamespace,
						ImagePullSecret: "testsecret",
					},
				}
				createCtx := context.Background()
				Expect(k8sClient.Create(createCtx, backplaneConfig)).Should(Succeed())

				By("ensuring the deployment image is overridden")
				Eventually(func(g Gomega) {
					ctx := context.Background()
					discoveryNN := types.NamespacedName{Name: "discovery-operator", Namespace: DestinationNamespace}
					res := &appsv1.Deployment{}
					g.Expect(k8sClient.Get(ctx, discoveryNN, res)).To(Succeed())
					g.Expect(res.Spec.Template.Spec.Containers[0].Image).To(
						Equal("quay.io/stolostron/discovery-operator@sha256:9dc4d072dcd06eda3fda19a15f4b84677fbbbde2a476b4817272cde4724f02cc"),
						fmt.Sprintf("Image does not match that defined in configmap"),
					)
				}, timeout, interval).Should(Succeed())
			})
		})

		Context("and imagePullSecret is missing", func() {
			It("should error due to missing secret", func() {
				By("creating the backplane config with nonexistant secret")
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
						ImagePullSecret: "nonexistant",
					},
				}
				createCtx := context.Background()
				Expect(k8sClient.Create(createCtx, backplaneConfig)).Should(Succeed())

				By("ensuring MCE reports error in Phase")
				Eventually(func(g Gomega) {
					multiClusterEngine := types.NamespacedName{
						Name: BackplaneConfigName,
					}
					existingMCE := &v1.MultiClusterEngine{}
					g.Expect(k8sClient.Get(context.TODO(), multiClusterEngine, existingMCE)).To(Succeed(), "Failed to get MCE")

					g.Expect(existingMCE.Status.Phase).To(Equal(v1.MultiClusterEnginePhaseError))
				}, timeout, interval).Should(Succeed())

			})
		})
	})

})
