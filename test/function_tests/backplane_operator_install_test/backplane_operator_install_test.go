// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package backplane_install_test

import (
	"context"
	"io/ioutil"
	"reflect"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"

	"sigs.k8s.io/controller-runtime/pkg/client"

	backplane "github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	// apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/apimachinery/pkg/types"
)

const (
	BackplaneConfigName        = "backplane"
	BackplaneOperatorNamespace = "backplane-operator-system"
	installTimeout             = time.Minute * 5
	deleteTimeout              = time.Minute * 3
	listTimeout                = time.Second * 30
	duration                   = time.Second * 1
	interval                   = time.Millisecond * 250
)

var (
	ctx                = context.Background()
	globalsInitialized = false
	baseURL            = ""

	k8sClient client.Client

	backplaneConfig = types.NamespacedName{}

	blockCreationResources = []struct {
		Name     string
		GVK      schema.GroupVersionKind
		Filepath string
		crdPath  string
		Expected string
	}{
		{
			Name: "MultiClusterHub",
			GVK: schema.GroupVersionKind{
				Group:   "operator.open-cluster-management.io",
				Version: "v1",
				Kind:    "MultiClusterHub",
			},
			Filepath: "../resources/multiclusterhub.yaml",
			crdPath:  "../resources/multiclusterhub_crd.yaml",
			Expected: "Existing MultiClusterHub resources must first be deleted",
		},
	}
	blockDeletionResources = []struct {
		Name     string
		GVK      schema.GroupVersionKind
		Filepath string
		crdPath  string
		Expected string
	}{
		{
			Name: "BareMetalAsset",
			GVK: schema.GroupVersionKind{
				Group:   "inventory.open-cluster-management.io",
				Version: "v1alpha1",
				Kind:    "BareMetalAsset",
			},
			Filepath: "../resources/baremetalassets.yaml",
			Expected: "Existing BareMetalAsset resources must first be deleted",
		},
		{
			Name: "MultiClusterObservability",
			GVK: schema.GroupVersionKind{
				Group:   "observability.open-cluster-management.io",
				Version: "v1beta2",
				Kind:    "MultiClusterObservability",
			},
			crdPath:  "../resources/multiclusterobservabilities_crd.yaml",
			Filepath: "../resources/multiclusterobservability.yaml",
			Expected: "Existing MultiClusterObservability resources must first be deleted",
		},
		{
			Name: "ManagedCluster",
			GVK: schema.GroupVersionKind{
				Group:   "cluster.open-cluster-management.io",
				Version: "v1",
				Kind:    "ManagedClusterList",
			},
			Filepath: "../resources/managedcluster.yaml",
			Expected: "Existing ManagedCluster resources must first be deleted",
		},
	}

	backplaneNodeSelector map[string]string
	backplanePullSecret   string
	backplaneTolerations  []corev1.Toleration
)

func initializeGlobals() {
	// baseURL = *BaseURL
	backplaneConfig = types.NamespacedName{
		Name: BackplaneConfigName,
	}
	backplaneNodeSelector = map[string]string{"beta.kubernetes.io/os": "linux"}
	backplanePullSecret = "test"
	backplaneTolerations = []corev1.Toleration{
		corev1.Toleration{
			Key:      "dedicated",
			Operator: "Exists",
			Effect:   "NoSchedule",
		},
	}

}

var _ = Describe("BackplaneConfig Test Suite", func() {

	BeforeEach(func() {
		if !globalsInitialized {
			initializeGlobals()
			globalsInitialized = true
		}
	})

	Context("Creating a BackplaneConfig", func() {
		It("Should install all components ", func() {
			By("By creating a new BackplaneConfig", func() {
				Expect(k8sClient.Create(ctx, defaultBackplaneConfig())).Should(Succeed())
			})
		})

		It("Should check that all components were installed correctly", func() {
			By("Ensuring the BackplaneConfig becomes available", func() {
				Eventually(func() bool {
					key := &backplane.MultiClusterEngine{}
					k8sClient.Get(context.Background(), types.NamespacedName{
						Name: BackplaneConfigName,
					}, key)
					return key.Status.Phase == backplane.MultiClusterEnginePhaseAvailable
				}, installTimeout, interval).Should(BeTrue())

			})
		})

		It("Should check for a healthy status", func() {
			config := &backplane.MultiClusterEngine{}
			Expect(k8sClient.Get(ctx, backplaneConfig, config)).To(Succeed())

			By("Checking the phase", func() {
				Expect(config.Status.Phase).To(Equal(backplane.MultiClusterEnginePhaseAvailable))
			})
			By("Checking the components", func() {
				Expect(len(config.Status.Components)).Should(BeNumerically(">=", 6), "Expected at least 6 components in status")
			})
			By("Checking the conditions", func() {
				available := backplane.MultiClusterEngineCondition{}
				for _, c := range config.Status.Conditions {
					if c.Type == backplane.MultiClusterEngineAvailable {
						available = c
					}
				}
				Expect(available.Status).To(Equal(metav1.ConditionTrue))
			})
		})

		It("Should ensure validatingwebhook blocks deletion if resouces exist", func() {
			for _, r := range blockDeletionResources {
				By("Creating a new "+r.Name, func() {

					if r.crdPath != "" {
						applyResource(r.crdPath)
						defer deleteResource(r.crdPath)
					}
					applyResource(r.Filepath)
					defer deleteResource(r.Filepath)

					config := &backplane.MultiClusterEngine{}
					Expect(k8sClient.Get(ctx, backplaneConfig, config)).To(Succeed()) // Get Backplaneconfig

					err := k8sClient.Delete(ctx, config) // Attempt to delete backplaneconfig. Ensure it does not succeed.
					Expect(err).ShouldNot(BeNil())
					Expect(err.Error()).Should(ContainSubstring(r.Expected))
				})
			}
		})

		It("Should ensure validatingwebhook blocks creation if resouces exist", func() {
			for _, r := range blockCreationResources {
				By("Creating a new "+r.Name, func() {

					if r.crdPath != "" {
						applyResource(r.crdPath)
						defer deleteResource(r.crdPath)
					}
					applyResource(r.Filepath)
					defer deleteResource(r.Filepath)

					backplaneConfig := defaultBackplaneConfig()
					backplaneConfig.Name = "test"

					err := k8sClient.Create(ctx, backplaneConfig)
					Expect(err).ShouldNot(BeNil())
					Expect(err.Error()).Should(ContainSubstring(r.Expected))
				})
			}
		})

		It("Should ensure the Backplane is self-correcting", func() {
			By("Checking metadata is maintained but not overwritten", func() {
				By("Manipulating annotations and backplane labels in the ocm-controller deployment", func() {
					targetDeploy := &appsv1.Deployment{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ocm-controller", Namespace: BackplaneOperatorNamespace}, targetDeploy)).To(Succeed())

					targetDeploy.SetLabels(map[string]string{})
					targetDeploy.SetAnnotations(map[string]string{"testannotation": "test"})
					Expect(k8sClient.Update(ctx, targetDeploy)).Should(Succeed())
				})

				By("Checking backplane labels are added back and custom annotations are preserved", func() {
					Eventually(func(g Gomega) {
						deploy := &appsv1.Deployment{}
						g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ocm-controller", Namespace: BackplaneOperatorNamespace}, deploy)).To(Succeed())

						l := deploy.GetLabels()
						g.Expect(l["backplaneconfig.name"]).To(Equal(backplaneConfig.Name), "Missing backplane label")

						a := deploy.GetAnnotations()
						g.Expect(a["testannotation"]).To(Equal("test"), "Test annotation may have been stripped out of deployment")
					}, 20*time.Second, interval).Should(Succeed())
				})
			})

			By("Checking spec changes are set to their expected values", func() {
				By("Manipulating the spec in the ocm-controller deployment", func() {
					targetDeploy := &appsv1.Deployment{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ocm-controller", Namespace: BackplaneOperatorNamespace}, targetDeploy)).To(Succeed())

					targetDeploy.Spec.Template.Spec.ServiceAccountName = "test-sa"
					targetDeploy.SetAnnotations(map[string]string{"testannotation": "test2"})
					Expect(k8sClient.Update(ctx, targetDeploy)).Should(Succeed())
				})

				By("Confirming the spec is reset", func() {
					Eventually(func(g Gomega) {
						deploy := &appsv1.Deployment{}
						g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ocm-controller", Namespace: BackplaneOperatorNamespace}, deploy)).To(Succeed())

						// annotation is used to verify the deployment has been updated
						a := deploy.GetAnnotations()
						g.Expect(a["testannotation"]).To(Equal("test2"), "Deployment may not have been updated")

						g.Expect(deploy.Spec.Template.Spec.ServiceAccountName).To(Equal("test-sa"), "Deployment restart policy change was not reverted by operator")
					}, 20*time.Second, interval).Should(Succeed())
				})
			})
		})

		It("Should check that the config spec has propagated", func() {

			By("Ensuring the node selectors is correct")
			nodeSelectorBackplane := &backplane.MultiClusterEngine{}

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: BackplaneConfigName}, nodeSelectorBackplane)
				g.Expect(err).To(BeNil())

				nodeSelectorBackplane.Spec.NodeSelector = backplaneNodeSelector

				err = k8sClient.Update(ctx, nodeSelectorBackplane)
				g.Expect(err).To(BeNil())
			}, 20*time.Second, interval).Should(Succeed())

			deployments := &appsv1.DeploymentList{}
			Eventually(func() bool {
				err := k8sClient.List(ctx, deployments,
					client.InNamespace(BackplaneOperatorNamespace),
					client.MatchingLabels{
						"backplaneconfig.name": backplaneConfig.Name,
					})
				if err != nil {
					return false
				}
				if len(deployments.Items) == 0 {
					return false
				}

				for _, deployment := range deployments.Items {
					componentSelector := deployment.Spec.Template.Spec.NodeSelector
					if !reflect.DeepEqual(componentSelector, backplaneNodeSelector) {
						return false
					}

				}
				return true
			}, listTimeout, interval).Should(BeTrue())

			By("Ensuring the image pull secret is correct")
			pullSecretBackplane := &backplane.MultiClusterEngine{}

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: BackplaneConfigName}, pullSecretBackplane)
				g.Expect(err).To(BeNil())

				pullSecretBackplane.Spec.ImagePullSecret = backplanePullSecret

				err = k8sClient.Update(ctx, pullSecretBackplane)
				g.Expect(err).To(BeNil())
			}, 20*time.Second, interval).Should(Succeed())

			deployments = &appsv1.DeploymentList{}
			Eventually(func() bool {
				err := k8sClient.List(ctx, deployments,
					client.InNamespace(BackplaneOperatorNamespace),
					client.MatchingLabels{
						"backplaneconfig.name": backplaneConfig.Name,
					})
				if err != nil {
					return false
				}
				if len(deployments.Items) == 0 {
					return false
				}

				for _, deployment := range deployments.Items {
					if len(deployment.Spec.Template.Spec.ImagePullSecrets) == 0 {
						return false
					}
					componentSecret := deployment.Spec.Template.Spec.ImagePullSecrets[0].Name
					if !reflect.DeepEqual(componentSecret, backplanePullSecret) {
						return false
					}

				}
				return true
			}, listTimeout, interval).Should(BeTrue())

			By("Ensuring the tolerations are correct")
			tolerationBackplane := &backplane.MultiClusterEngine{}

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: BackplaneConfigName}, tolerationBackplane)
				g.Expect(err).To(BeNil())

				tolerationBackplane.Spec.Tolerations = backplaneTolerations

				err = k8sClient.Update(ctx, tolerationBackplane)
				g.Expect(err).To(BeNil())
			}, 20*time.Second, interval).Should(Succeed())

			deployments = &appsv1.DeploymentList{}
			Eventually(func() bool {
				err := k8sClient.List(ctx, deployments,
					client.InNamespace(BackplaneOperatorNamespace),
					client.MatchingLabels{
						"backplaneconfig.name": backplaneConfig.Name,
					})
				if err != nil {
					return false
				}
				if len(deployments.Items) == 0 {
					return false
				}

				for _, deployment := range deployments.Items {
					componentTolerations := deployment.Spec.Template.Spec.Tolerations
					if !reflect.DeepEqual(componentTolerations, backplaneTolerations) {
						return false
					}

				}
				return true
			}, listTimeout, interval).Should(BeTrue())

		})

		It("Should ensure deletion works properly", func() {
			ctx := context.Background()

			By("Deleting the backplane", func() {
				err := k8sClient.Delete(ctx, defaultBackplaneConfig())
				if err != nil {
					Expect(apierrors.IsNotFound(err)).To(BeTrue())
				}

				Eventually(func() bool {
					err := k8sClient.Get(ctx, backplaneConfig, &backplane.MultiClusterEngine{})
					if err == nil {
						return false
					}
					return apierrors.IsNotFound(err)
				}, deleteTimeout, interval).Should(BeTrue(), "There was an issue cleaning up the backplane.")
			})

			labelSelector := client.MatchingLabels{"backplaneconfig.name": backplaneConfig.Name}

			By("Checking for remaining services", func() {
				Eventually(func(g Gomega) {
					serviceList := &corev1.ServiceList{}
					g.Expect(k8sClient.List(ctx, serviceList, labelSelector)).To(Succeed())
					g.Expect(len(serviceList.Items)).To(BeZero())
				}, deleteTimeout, interval).Should(Succeed())
			})

			By("Checking for remaining serviceaccounts", func() {
				Eventually(func(g Gomega) {
					serviceAccountList := &corev1.ServiceAccountList{}
					g.Expect(k8sClient.List(ctx, serviceAccountList, labelSelector)).To(Succeed())
					g.Expect(len(serviceAccountList.Items)).To(BeZero())
				}, deleteTimeout, interval).Should(Succeed())
			})

			By("Checking for remaining deployments", func() {
				Eventually(func(g Gomega) {
					deploymentList := &appsv1.DeploymentList{}
					g.Expect(k8sClient.List(ctx, deploymentList, labelSelector)).To(Succeed())
					g.Expect(len(deploymentList.Items)).To(BeZero())
				}, deleteTimeout, interval).Should(Succeed())
			})

			By("Checking for remaining clusterroles", func() {
				Eventually(func(g Gomega) {
					clusterRoleList := &unstructured.UnstructuredList{}
					clusterRoleList.SetGroupVersionKind(
						schema.GroupVersionKind{
							Group:   "rbac.authorization.k8s.io",
							Version: "v1",
							Kind:    "ClusterRole",
						},
					)
					g.Expect(k8sClient.List(ctx, clusterRoleList, labelSelector)).To(Succeed())
					g.Expect(len(clusterRoleList.Items)).To(BeZero())
				}, deleteTimeout, interval).Should(Succeed())
			})

			By("Checking for remaining clusterrolebindings", func() {
				Eventually(func(g Gomega) {
					clusterRoleBindingList := &unstructured.UnstructuredList{}
					clusterRoleBindingList.SetGroupVersionKind(
						schema.GroupVersionKind{
							Group:   "rbac.authorization.k8s.io",
							Version: "v1",
							Kind:    "ClusterRoleBinding",
						},
					)
					g.Expect(k8sClient.List(ctx, clusterRoleBindingList, labelSelector)).To(Succeed())
					g.Expect(len(clusterRoleBindingList.Items)).To(BeZero())
				}, deleteTimeout, interval).Should(Succeed())
			})

			By("Checking for remaining apiservices", func() {
				Eventually(func(g Gomega) {
					apiServiceList := &unstructured.UnstructuredList{}
					apiServiceList.SetGroupVersionKind(
						schema.GroupVersionKind{
							Group:   "apiregistration.k8s.io",
							Version: "v1",
							Kind:    "APIService",
						},
					)
					g.Expect(k8sClient.List(ctx, apiServiceList, labelSelector)).To(Succeed())
					g.Expect(len(apiServiceList.Items)).To(BeZero())
				}, deleteTimeout, interval).Should(Succeed())
			})

			By("Checking for remaining clustermanager", func() {
				Eventually(func(g Gomega) {
					clusterManager := &unstructured.Unstructured{}
					clusterManager.SetGroupVersionKind(
						schema.GroupVersionKind{
							Group:   "operator.open-cluster-management.io",
							Version: "v1",
							Kind:    "ClusterManager",
						},
					)
					err := k8sClient.Get(ctx, types.NamespacedName{Name: "cluster-manager"}, clusterManager)
					g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "Expected IsNotFound error, got error:", err)
				}, deleteTimeout, interval).Should(Succeed())
			})

			By("Checking for remaining hiveconfig", func() {
				Eventually(func(g Gomega) {
					hiveConfig := &unstructured.Unstructured{}
					hiveConfig.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "hive.openshift.io",
						Version: "v1",
						Kind:    "HiveConfig",
					})
					err := k8sClient.Get(ctx, types.NamespacedName{Name: "hive"}, hiveConfig)
					g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "Expected IsNotFound error, got error:", err)
				}, deleteTimeout, interval).Should(Succeed())
			})

		})
	})
})

func applyResource(resourceFile string) {
	resourceData, err := ioutil.ReadFile(resourceFile) // Get resource as bytes
	Expect(err).To(BeNil())

	unstructured := &unstructured.Unstructured{Object: map[string]interface{}{}}
	err = yaml.Unmarshal(resourceData, &unstructured.Object) // Render resource as unstructured
	Expect(err).To(BeNil())

	Expect(k8sClient.Create(ctx, unstructured)).Should(Succeed()) // Create resource on cluster
}

func deleteResource(resourceFile string) {
	resourceData, err := ioutil.ReadFile(resourceFile) // Get resource as bytes
	Expect(err).To(BeNil())

	unstructured := &unstructured.Unstructured{Object: map[string]interface{}{}}
	err = yaml.Unmarshal(resourceData, &unstructured.Object) // Render resource as unstructured
	Expect(err).To(BeNil())

	Expect(k8sClient.Delete(ctx, unstructured)).Should(Succeed()) // Delete resource on cluster
}

func defaultBackplaneConfig() *backplane.MultiClusterEngine {
	return &backplane.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name: BackplaneConfigName,
		},
		Spec: backplane.MultiClusterEngineSpec{},
		Status: backplane.MultiClusterEngineStatus{
			Phase: "",
		},
	}
}
