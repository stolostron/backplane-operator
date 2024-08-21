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
	"strings"
	"time"

	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	configv1 "github.com/openshift/api/config/v1"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/utils"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
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
		addonTemplate          *unstructured.Unstructured
		addonDeploymentConfig  *unstructured.Unstructured
		tests                  testList
		msaTests               testList
		secondTests            testList
	)

	AfterEach(func() {
		Expect(k8sClient.Delete(context.Background(), &backplanev1.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{
				Name: BackplaneConfigName,
			},
		})).To(Succeed())
		Eventually(func() bool {
			foundMCE := &backplanev1.MultiClusterEngine{}
			err := k8sClient.Get(context.TODO(), types.NamespacedName{Name: BackplaneConfigName}, foundMCE)
			return errors.IsNotFound(err)
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
			if errors.IsAlreadyExists(err) {
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

		addonTemplate = &unstructured.Unstructured{}
		addonTemplate.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "addon.open-cluster-management.io",
			Version: "v1alpha1",
			Kind:    "AddOnTemplate",
		})

		addonDeploymentConfig = &unstructured.Unstructured{}
		addonDeploymentConfig.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "addon.open-cluster-management.io",
			Version: "v1alpha1",
			Kind:    "AddOnDeploymentConfig",
		})

		tests = testList{
			{
				Name:           BackplaneConfigTestName,
				NamespacedName: types.NamespacedName{Name: BackplaneConfigName},
				ResourceType:   &backplanev1.MultiClusterEngine{},
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
				Name:           "Managed-ServiceAccount Addon Template",
				NamespacedName: types.NamespacedName{Name: "managed-serviceaccount"},
				ResourceType:   addonTemplate,
				Expected:       nil,
			},
			{
				Name:           "Managed-ServiceAccount agent registration clusterrole",
				NamespacedName: types.NamespacedName{Name: "managed-serviceaccount-addon-agent"},
				ResourceType:   &rbacv1.ClusterRole{},
				Expected:       nil,
			},
			{
				Name:           "Managed-ServiceAccount addon manager clusterrolebinding",
				NamespacedName: types.NamespacedName{Name: "open-cluster-management-addon-manager-managed-serviceaccount"},
				ResourceType:   &rbacv1.ClusterRoleBinding{},
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
		secondTests = testList{
			{
				Name:           BackplaneConfigTestName,
				NamespacedName: types.NamespacedName{Name: BackplaneConfigName},
				ResourceType:   &backplanev1.MultiClusterEngine{},
				Expected:       nil,
			},
			// {
			// 	Name:           "MCEConsole",
			// 	NamespacedName: types.NamespacedName{Name: "console-mce-console", Namespace: DestinationNamespace},
			// 	ResourceType:   &appsv1.Deployment{},
			// 	Expected:       nil,
			// },
		}
	})

	When("creating a new BackplaneConfig", func() {
		Context("and no image pull policy is specified", func() {
			It("should deploy sub components", func() {
				createCtx := context.Background()
				backplaneConfig := &backplanev1.MultiClusterEngine{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "multicluster.openshift.io/v1",
						Kind:       "MultiClusterEngine",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: BackplaneConfigName,
					},
					Spec: backplanev1.MultiClusterEngineSpec{
						TargetNamespace: DestinationNamespace,
						ImagePullSecret: "testsecret",
					},
				}

				By("creating the backplane config")
				Expect(k8sClient.Create(createCtx, backplaneConfig)).Should(Succeed())

				By("ensuring that no openshift.io/cluster-monitoring label is enabled if MCE does not exist")
				backplaneConfig2 := &backplanev1.MultiClusterEngine{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "multicluster.openshift.io/v1",
						Kind:       "MultiClusterEngine",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: BackplaneConfigName,
					},
					Spec: backplanev1.MultiClusterEngineSpec{
						TargetNamespace: "test-n2",
					},
				}

				res, _ := reconciler.ensureOpenShiftNamespaceLabel(createCtx, backplaneConfig2)
				Expect(res).To(Equal(ctrl.Result{Requeue: true}))

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
					u.SetNamespace(DestinationNamespace)
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
						Namespace: DestinationNamespace,
					}
					resourceType := &monitoringv1.ServiceMonitor{}
					return k8sClient.Get(ctx, namespacedName, resourceType)
				}, timeout, interval).Should(Succeed())

				By("ensuring the trusted-ca-bundle ConfigMap is created")
				Eventually(func(g Gomega) {
					ctx := context.Background()
					namespacedName := types.NamespacedName{
						Name:      defaultTrustBundleName,
						Namespace: DestinationNamespace,
					}
					res := &corev1.ConfigMap{}
					g.Expect(k8sClient.Get(ctx, namespacedName, res)).To(Succeed())
				}, timeout, interval).Should(Succeed())

				By("Pausing MCE to pause reconcilation")
				Eventually(func() bool {
					annotations := backplaneConfig.GetAnnotations()
					if annotations == nil {
						annotations = make(map[string]string)
					}

					annotations[utils.AnnotationMCEPause] = "true"
					backplaneConfig.Annotations = annotations
					_ = k8sClient.Update(ctx, backplaneConfig)

					return utils.IsPaused(backplaneConfig)
				}, timeout, interval).Should(BeTrue())
			})
		})

		Context("and OCP Console is disabled", func() {
			It("should deploy sub components", func() {
				os.Setenv("ACM_HUB_OCP_VERSION", "4.12.0")
				defer os.Unsetenv("ACM_HUB_OCP_VERSION")
				createCtx := context.Background()
				By("creating the backplane config")
				backplaneConfig := &backplanev1.MultiClusterEngine{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "multicluster.openshift.io/v1",
						Kind:       "MultiClusterEngine",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: BackplaneConfigName,
					},
					Spec: backplanev1.MultiClusterEngineSpec{
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
			})
		})

		Context("ensuring InternalHubComponent CRs", func() {
			It("should deploy sub components", func() {
				createCtx := context.Background()
				By("creating the backplane config with everything enabled")
				backplaneConfig := &backplanev1.MultiClusterEngine{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "multicluster.openshift.io/v1",
						Kind:       "MultiClusterEngine",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: BackplaneConfigName,
					},
					Spec: backplanev1.MultiClusterEngineSpec{
						TargetNamespace: DestinationNamespace,
						ImagePullSecret: "testsecret",
						Overrides: &backplanev1.Overrides{
							Components: []backplanev1.ComponentConfig{
								{
									Name:    backplanev1.AssistedService,
									Enabled: true,
								},
								{
									Name:    backplanev1.ClusterLifecycle,
									Enabled: true,
								},
								{
									Name:    backplanev1.ClusterManager,
									Enabled: true,
								},
								{
									Name:    backplanev1.ClusterProxyAddon,
									Enabled: true,
								},
								{
									Name:    backplanev1.ConsoleMCE,
									Enabled: false,
								},
								{
									Name:    backplanev1.Discovery,
									Enabled: true,
								},
								{
									Name:    backplanev1.Hive,
									Enabled: true,
								},
								{
									Name:    backplanev1.HyperShift,
									Enabled: true,
								},
								{
									Name:    backplanev1.HypershiftLocalHosting,
									Enabled: false,
								},
								{
									Name:    backplanev1.ManagedServiceAccount,
									Enabled: true,
								},
								{
									Name:    backplanev1.ServerFoundation,
									Enabled: true,
								},
								{
									Name:    backplanev1.ImageBasedInstallOperator,
									Enabled: false,
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(createCtx, backplaneConfig)).Should(Succeed())

				By("ensuring the InternalHubComponent CRD is created")
				ctx := context.Background()
				ihcCRD := &apixv1.CustomResourceDefinition{}

				Eventually(k8sClient.Get(ctx, types.NamespacedName{Name: "internalhubcomponents.multicluster.openshift.io"}, ihcCRD)).Should(Succeed())

				By("ensuring each enabled component's CR is created")
				for _, mcecomponent := range backplanev1.MCEComponents {
					if backplaneConfig.Enabled(mcecomponent) {
						By(fmt.Sprintf("ensuring %s CR is created", mcecomponent))
						Eventually(k8sClient.Get(ctx, types.NamespacedName{Name: mcecomponent, Namespace: backplaneConfig.Spec.TargetNamespace}, &backplanev1.InternalHubComponent{})).Should(Succeed())
					}
				}

				By("ensuring each disabled component's CR is not present")
				for _, mcecomponent := range backplanev1.MCEComponents {
					if !backplaneConfig.Enabled(mcecomponent) {
						By(fmt.Sprintf("ensuring %s CR is not present", mcecomponent))
						Eventually(k8sClient.Get(ctx, types.NamespacedName{Name: mcecomponent, Namespace: backplaneConfig.Spec.TargetNamespace}, &backplanev1.InternalHubComponent{})).Should(Not(Succeed()))
					}
				}
			})
		})

		Context("ensuring No InternalHubComponent CRs", func() {
			It("should deploy sub components", func() {
				createCtx := context.Background()
				By("creating the backplane config with everything enabled")
				backplaneConfig := &backplanev1.MultiClusterEngine{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "multicluster.openshift.io/v1",
						Kind:       "MultiClusterEngine",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: BackplaneConfigName,
					},
					Spec: backplanev1.MultiClusterEngineSpec{
						TargetNamespace: DestinationNamespace,
						ImagePullSecret: "testsecret",
						Overrides: &backplanev1.Overrides{
							Components: []backplanev1.ComponentConfig{
								{
									Name:    backplanev1.AssistedService,
									Enabled: false,
								},
								{
									Name:    backplanev1.ClusterLifecycle,
									Enabled: false,
								},
								{
									Name:    backplanev1.ClusterManager,
									Enabled: false,
								},
								{
									Name:    backplanev1.ClusterProxyAddon,
									Enabled: false,
								},
								{
									Name:    backplanev1.ConsoleMCE,
									Enabled: false,
								},
								{
									Name:    backplanev1.Discovery,
									Enabled: false,
								},
								{
									Name:    backplanev1.Hive,
									Enabled: false,
								},
								{
									Name:    backplanev1.HyperShift,
									Enabled: false,
								},
								{
									Name:    backplanev1.HypershiftLocalHosting,
									Enabled: false,
								},
								{
									Name:    backplanev1.ManagedServiceAccount,
									Enabled: false,
								},
								{
									Name:    backplanev1.ServerFoundation,
									Enabled: false,
								},
								{
									Name:    backplanev1.ImageBasedInstallOperator,
									Enabled: false,
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(createCtx, backplaneConfig)).Should(Succeed())

				By("ensuring the InternalHubComponent CRD is created")
				ctx := context.Background()
				ihcCRD := &apixv1.CustomResourceDefinition{}

				Eventually(k8sClient.Get(ctx, types.NamespacedName{Name: "internalhubcomponents.multicluster.openshift.io"}, ihcCRD)).Should(Succeed())

				By("ensuring each enabled component's CR is created")
				for _, mcecomponent := range backplanev1.MCEComponents {
					if backplaneConfig.Enabled(mcecomponent) {
						By(fmt.Sprintf("ensuring %s CR is created", mcecomponent))
						Eventually(k8sClient.Get(ctx, types.NamespacedName{Name: mcecomponent, Namespace: backplaneConfig.Spec.TargetNamespace}, &backplanev1.InternalHubComponent{})).Should(Succeed())
					}
				}

				By("ensuring each disabled component's CR is not present")
				for _, mcecomponent := range backplanev1.MCEComponents {
					if !backplaneConfig.Enabled(mcecomponent) {
						By(fmt.Sprintf("ensuring %s CR is not present", mcecomponent))
						Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mcecomponent, Namespace: backplaneConfig.Spec.TargetNamespace}, &backplanev1.InternalHubComponent{})).NotTo(Succeed())
					}
				}
			})
		})
		Context("2nd attempt", func() {
			It("should deploy sub components", func() {
				createCtx := context.Background()
				By("creating the backplane config")
				backplaneConfig := &backplanev1.MultiClusterEngine{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "multicluster.openshift.io/v1",
						Kind:       "MultiClusterEngine",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: BackplaneConfigName,
					},
					Spec: backplanev1.MultiClusterEngineSpec{
						TargetNamespace: DestinationNamespace,
						ImagePullSecret: "testsecret",
						Overrides: &backplanev1.Overrides{
							Components: []backplanev1.ComponentConfig{
								{
									Name:    backplanev1.ConsoleMCE,
									Enabled: true,
								},
								{
									Name:    backplanev1.ServerFoundation,
									Enabled: false,
								},
								{
									Name:    backplanev1.HyperShift,
									Enabled: false,
								},
								{
									Name:    backplanev1.Hive,
									Enabled: false,
								},
								{
									Name:    backplanev1.ClusterManager,
									Enabled: false,
								},
								{
									Name:    backplanev1.ClusterLifecycle,
									Enabled: false,
								},
								{
									Name:    backplanev1.ManagedServiceAccount,
									Enabled: false,
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(createCtx, backplaneConfig)).Should(Succeed())

				By("ensuring each deployment and config is created")
				for _, test := range secondTests {
					By(fmt.Sprintf("ensuring %s is created", test.Name))
					Eventually(func() bool {
						ctx := context.Background()
						err := k8sClient.Get(ctx, test.NamespacedName, test.ResourceType)
						return err == test.Expected
					}, timeout, interval).Should(BeTrue())
				}

			})
		})

		Context("and an image pull policy is specified in an override", func() {
			It("should deploy sub components with the image pull policy in the override", func() {
				By("creating the backplane config with an image pull policy override")
				backplaneConfig := &backplanev1.MultiClusterEngine{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "multicluster.openshift.io/v1",
						Kind:       "MultiClusterEngine",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: BackplaneConfigName,
					},
					Spec: backplanev1.MultiClusterEngineSpec{
						TargetNamespace: DestinationNamespace,
						ImagePullSecret: "testsecret",
						Overrides: &backplanev1.Overrides{
							ImagePullPolicy: corev1.PullAlways,
						},
					},
				}
				createCtx := context.Background()
				Expect(k8sClient.Create(createCtx, backplaneConfig)).Should(Succeed())
				_, err := reconciler.ensureNoClusterManager(createCtx, backplaneConfig)
				Expect(err).To(BeNil())
				_, err = reconciler.ensureNoClusterLifecycle(createCtx, backplaneConfig)
				Expect(err).To(BeNil())
				_, err = reconciler.ensureNoManagedServiceAccount(createCtx, backplaneConfig)
				Expect(err).To(BeNil())
				_, err = reconciler.ensureNoHive(createCtx, backplaneConfig)
				Expect(err).To(BeNil())
				_, err = reconciler.ensureNoHyperShift(createCtx, backplaneConfig)
				Expect(err).To(BeNil())
				_, err = reconciler.ensureNoServerFoundation(createCtx, backplaneConfig)
				Expect(err).To(BeNil())

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
				backplaneConfig := &backplanev1.MultiClusterEngine{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "multicluster.openshift.io/v1",
						Kind:       "MultiClusterEngine",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: BackplaneConfigName,
					},
					Spec: backplanev1.MultiClusterEngineSpec{
						TargetNamespace: DestinationNamespace,
						Overrides: &backplanev1.Overrides{
							Components: []backplanev1.ComponentConfig{
								{
									Name:    backplanev1.ManagedServiceAccount,
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
					if test.ResourceType == addonTemplate {
						// the name of the addon template increases with each version,
						// managed-serviceaccount-2.4, managed-serviceaccount-2.5, etc.
						continue
					}
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
					if test.ResourceType == addonTemplate {
						// the name of the addon template increases with each version,
						// managed-serviceaccount-2.4, managed-serviceaccount-2.5, etc.
						continue
					}
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

				By("ensuring each addon template is created and has an owner reference")
				for _, test := range withMSATests {
					if test.ResourceType != addonTemplate {
						continue
					}

					By(fmt.Sprintf("ensuring %s is created and has an ownerreference set", test.Name))
					// the name of the addon template increases with each version,
					// managed-serviceaccount-2.4, managed-serviceaccount-2.5, etc.
					Eventually(func() error {
						ctx := context.Background()
						ats := &unstructured.UnstructuredList{}
						ats.SetGroupVersionKind(schema.GroupVersionKind{
							Group:   "addon.open-cluster-management.io",
							Version: "v1alpha1",
							Kind:    "AddOnTemplateList",
						})
						err := k8sClient.List(ctx, ats)
						if err != test.Expected {
							return err
						}
						for _, at := range ats.Items {
							if strings.HasPrefix(at.GetName(), test.NamespacedName.Name) {
								Expect(len(at.GetOwnerReferences())).To(
									Equal(1),
									fmt.Sprintf("Missing ownerreference on %s", test.Name),
								)
								Expect(at.GetOwnerReferences()[0].Name).To(Equal(BackplaneConfigName))
								return nil
							}
						}
						return fmt.Errorf("addon template %s not found", test.NamespacedName.Name)
					}, timeout, interval).ShouldNot(HaveOccurred())
				}

			})
		})

		Context("and components are defined multiple times in overrides", func() {
			It("should deduplicate the component list in the override", func() {
				By("creating the backplane config with repeated component")
				backplaneConfig := &backplanev1.MultiClusterEngine{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "multicluster.openshift.io/v1",
						Kind:       "MultiClusterEngine",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: BackplaneConfigName,
					},
					Spec: backplanev1.MultiClusterEngineSpec{
						TargetNamespace: DestinationNamespace,
						ImagePullSecret: "testsecret",
						Overrides: &backplanev1.Overrides{
							ImagePullPolicy: corev1.PullAlways,
							Components: []backplanev1.ComponentConfig{
								{
									Name:    backplanev1.Discovery,
									Enabled: true,
								},
								{
									Name:    backplanev1.Discovery,
									Enabled: true,
								},
								{
									Name:    backplanev1.Discovery,
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
					existingMCE := &backplanev1.MultiClusterEngine{}
					g.Expect(k8sClient.Get(context.TODO(), multiClusterEngine, existingMCE)).To(Succeed(), "Failed to create new MCE")

					g.Expect(existingMCE.Spec.Overrides).To(Not(BeNil()))
					componentCount := 0
					for _, c := range existingMCE.Spec.Overrides.Components {
						if c.Name == backplanev1.Discovery {
							componentCount++
						}
					}
					g.Expect(componentCount).To(Equal(1), "Duplicate component still present")

					g.Expect(existingMCE.Enabled(backplanev1.Discovery)).To(BeFalse(), "Not using last defined config in components")

				}, timeout, interval).Should(Succeed())

			})
		})

		Context("and images are overriden using annotations", func() {
			It("should deploy images with a custom image repository", func() {
				imageRepo := "quay.io/testrepo"
				By("creating the backplane config with the image repository annotation")
				backplaneConfig := &backplanev1.MultiClusterEngine{
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
					Spec: backplanev1.MultiClusterEngineSpec{
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
				backplaneConfig := &backplanev1.MultiClusterEngine{
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
					Spec: backplanev1.MultiClusterEngineSpec{
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
				backplaneConfig := &backplanev1.MultiClusterEngine{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "multicluster.openshift.io/v1",
						Kind:       "MultiClusterEngine",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: BackplaneConfigName,
					},
					Spec: backplanev1.MultiClusterEngineSpec{
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
					existingMCE := &backplanev1.MultiClusterEngine{}
					g.Expect(k8sClient.Get(context.TODO(), multiClusterEngine, existingMCE)).To(Succeed(), "Failed to get MCE")

					g.Expect(existingMCE.Status.Phase).To(Equal(backplanev1.MultiClusterEnginePhaseError))
				}, timeout, interval).Should(Succeed())

			})
		})

		Context("and OCP is below minimum version", func() {
			It("should error due to bad OCP version", func() {
				By("creating the backplane config with nonexistant secret")
				os.Setenv("ACM_HUB_OCP_VERSION", "4.9.0")
				defer os.Unsetenv("ACM_HUB_OCP_VERSION")
				backplaneConfig := &backplanev1.MultiClusterEngine{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "multicluster.openshift.io/v1",
						Kind:       "MultiClusterEngine",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: BackplaneConfigName,
					},
					Spec: backplanev1.MultiClusterEngineSpec{
						TargetNamespace: DestinationNamespace,
						ImagePullSecret: "testsecret",
					},
				}
				createCtx := context.Background()
				Expect(k8sClient.Create(createCtx, backplaneConfig)).Should(Succeed())

				By("ensuring MCE reports error in Phase")
				Eventually(func(g Gomega) {
					multiClusterEngine := types.NamespacedName{
						Name: BackplaneConfigName,
					}
					existingMCE := &backplanev1.MultiClusterEngine{}
					g.Expect(k8sClient.Get(context.TODO(), multiClusterEngine, existingMCE)).To(Succeed(), "Failed to get MCE")

					g.Expect(existingMCE.Status.Phase).To(Equal(backplanev1.MultiClusterEnginePhaseError))
				}, timeout, interval).Should(Succeed())

				By("ensuring MCE no longer reports error in Phase when annotated")
				multiClusterEngine := types.NamespacedName{
					Name: BackplaneConfigName,
				}
				existingMCE := &backplanev1.MultiClusterEngine{}
				Expect(k8sClient.Get(context.TODO(), multiClusterEngine, existingMCE)).To(Succeed(), "Failed to get MCE")
				existingMCE.SetAnnotations(map[string]string{utils.AnnotationIgnoreOCPVersion: "true"})
				Expect(k8sClient.Update(context.TODO(), existingMCE)).To(Succeed(), "Failed to get MCE")

				Eventually(func(g Gomega) {
					multiClusterEngine := types.NamespacedName{
						Name: BackplaneConfigName,
					}
					existingMCE := &backplanev1.MultiClusterEngine{}
					g.Expect(k8sClient.Get(context.TODO(), multiClusterEngine, existingMCE)).To(Succeed(), "Failed to get MCE")

					g.Expect(existingMCE.Status.Phase).To(Not(Equal(backplanev1.MultiClusterEnginePhaseError)))
				}, timeout, interval).Should(Succeed())
			})
		})

		Context("and deploymentMode is Hosted", func() {
			It("should not deploy resources in regular fashion", func() {
				By("creating the hosted backplane config")
				os.Setenv("UNIT_TEST", "true")

				// Create test secret in target namespace
				testconfigsecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: DestinationNamespace,
					},
					Data: map[string][]byte{"kubeconfig": []byte("")},
				}
				Eventually(func() error {
					err := k8sClient.Create(context.TODO(), testconfigsecret)
					if errors.IsAlreadyExists(err) {
						return nil
					}
					return err
				}, timeout, interval).Should(Succeed())

				backplaneConfig := &backplanev1.MultiClusterEngine{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "multicluster.openshift.io/v1",
						Kind:       "MultiClusterEngine",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:        BackplaneConfigName,
						Annotations: map[string]string{"deploymentmode": string(backplanev1.ModeHosted), "mce-kubeconfig": "test"},
					},
					Spec: backplanev1.MultiClusterEngineSpec{
						TargetNamespace: DestinationNamespace,
						ImagePullSecret: "testsecret",
					},
				}
				createCtx := context.Background()
				Expect(k8sClient.Create(createCtx, backplaneConfig)).Should(Succeed())

				By("ensuring MCE reports phase as Unimplemented")
				Eventually(func(g Gomega) {
					multiClusterEngine := types.NamespacedName{
						Name: BackplaneConfigName,
					}
					existingMCE := &backplanev1.MultiClusterEngine{}
					g.Expect(k8sClient.Get(context.TODO(), multiClusterEngine, existingMCE)).To(Succeed(), "Failed to get MCE")

					g.Expect(existingMCE.Status.Phase).To(Equal(backplanev1.MultiClusterEnginePhaseError), "MCE should fail getting a kubeconfig secret")
				}, timeout, interval).Should(Succeed())

			})
		})
		Context("Legacy clean up tasks", func() {
			It("Removes the legacy CLC Prometheus configuration", func() {
				By("creating the backplane config with nonexistant secret")
				backplaneConfig := &backplanev1.MultiClusterEngine{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "multicluster.openshift.io/v1",
						Kind:       "MultiClusterEngine",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: BackplaneConfigName,
					},
					Spec: backplanev1.MultiClusterEngineSpec{
						TargetNamespace: DestinationNamespace,
						ImagePullSecret: "nonexistant",
					},
				}
				createCtx := context.Background()
				Expect(k8sClient.Create(createCtx, backplaneConfig)).Should(Succeed())
				By("Creating the legacy CLC ServiceMonitor")
				sm := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"endpoints": []interface{}{
								map[string]interface{}{
									"path": "/some/path",
								},
							},
							"selector": map[string]interface{}{
								"matchLabels": map[string]interface{}{
									"app": "grc",
								},
							},
						},
					},
				}
				sm.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "monitoring.coreos.com",
					Kind:    "ServiceMonitor",
					Version: "v1",
				})
				sm.SetName("clusterlifecycle-state-metrics-v2")
				sm.SetNamespace("openshift-monitoring")

				err := k8sClient.Create(context.TODO(), sm)
				Expect(err).To(BeNil())

				legacyResourceKind := backplanev1.GetLegacyPrometheusKind()
				ns := "openshift-monitoring"

				By("Running the cleanup of the legacy Prometheus configuration")
				for _, kind := range legacyResourceKind {
					err = reconciler.removeLegacyPrometheusConfigurations(context.TODO(), ns, kind)
					Expect(err).To(BeNil())
				}

				By("Verifying that the legacy CLC ServiceMonitor is deleted")
				err = k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(sm), sm)
				Expect(errors.IsNotFound(err)).To(BeTrue())

				By("Running the cleanup of the legacy Prometheus configuration again should do nothing")
				for _, kind := range legacyResourceKind {
					err = reconciler.removeLegacyPrometheusConfigurations(context.TODO(), ns, kind)
					Expect(err).To(BeNil())
				}
			})
		})
	})
})
