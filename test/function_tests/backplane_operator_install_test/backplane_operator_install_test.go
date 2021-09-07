// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package backplane_install_test

import (
	"context"
	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io/ioutil"
	"reflect"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	backplane "github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
		It("Should check that the config spec has propagated", func() {

			By("Ensuring the node selectors is correct")
			nodeSelectorBackplane := &backplane.MultiClusterEngine{}

			err := k8sClient.Get(ctx, client.ObjectKey{Name: BackplaneConfigName}, nodeSelectorBackplane)
			Expect(err).To(BeNil())

			nodeSelectorBackplane.Spec.NodeSelector = backplaneNodeSelector

			err = k8sClient.Update(ctx, nodeSelectorBackplane)
			Expect(err).To(BeNil())

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

			err = k8sClient.Get(ctx, client.ObjectKey{Name: BackplaneConfigName}, pullSecretBackplane)
			Expect(err).To(BeNil())

			pullSecretBackplane.Spec.ImagePullSecret = backplanePullSecret

			err = k8sClient.Update(ctx, pullSecretBackplane)
			Expect(err).To(BeNil())

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

			err = k8sClient.Get(ctx, client.ObjectKey{Name: BackplaneConfigName}, tolerationBackplane)
			Expect(err).To(BeNil())

			tolerationBackplane.Spec.Tolerations = backplaneTolerations

			err = k8sClient.Update(ctx, tolerationBackplane)
			Expect(err).To(BeNil())

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
		Spec: backplane.MultiClusterEngineSpec{
			Foo: "bar",
			// NodeSelector: backplaneNodeSelector,
		},
		Status: backplane.MultiClusterEngineStatus{
			Phase: "",
		},
	}
}
