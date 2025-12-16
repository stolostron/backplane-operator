// Copyright Contributors to the Open Cluster Management project

package v1

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

var (
	multiClusterEngineName = "multiclusterengine"
)

var _ = Describe("Multiclusterengine webhook", func() {

	Context("Creating a Multiclusterengine", func() {
		It("Should successfully create multiclusterengine", func() {
			By("by creating a new standalone Multiclusterengine resource", func() {
				mce := &MultiClusterEngine{
					ObjectMeta: metav1.ObjectMeta{
						Name: multiClusterEngineName,
					},
					Spec: MultiClusterEngineSpec{
						LocalClusterName: "test-local-cluster",
						TargetNamespace:  DefaultTargetNamespace,
					},
				}
				Expect(k8sClient.Create(ctx, mce)).Should(Succeed())
			})
			By("by creating a new hosted Multiclusterengine resource", func() {
				mce := &MultiClusterEngine{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "hosted-mce",
						Annotations: map[string]string{"deploymentmode": string(ModeHosted)},
					},
					Spec: MultiClusterEngineSpec{
						TargetNamespace: "hostedNS",
					},
				}
				Expect(k8sClient.Create(ctx, mce)).Should(Succeed())
			})
		})

		It("Should fail to create multiclusterengine", func() {
			By("because of TargetNamespace", func() {
				mce := &MultiClusterEngine{
					ObjectMeta: metav1.ObjectMeta{
						Name:        fmt.Sprintf("%s-2", multiClusterEngineName),
						Annotations: map[string]string{"deploymentmode": string(ModeHosted)},
					},
					Spec: MultiClusterEngineSpec{
						TargetNamespace: DefaultTargetNamespace,
					},
				}
				Expect(k8sClient.Create(ctx, mce)).NotTo(BeNil(), "Only one MCE can target a namespace")
			})
			By("because of DeploymentMode", func() {
				mce := &MultiClusterEngine{
					ObjectMeta: metav1.ObjectMeta{
						Name: fmt.Sprintf("%s-2", multiClusterEngineName),
					},
					Spec: MultiClusterEngineSpec{
						TargetNamespace: "new",
					},
				}
				Expect(k8sClient.Create(ctx, mce)).NotTo(BeNil(), "Only one MCE in standalone mode allowed")
			})
			By("because of invalid AvailabilityConfig", func() {
				mce := &MultiClusterEngine{
					ObjectMeta: metav1.ObjectMeta{
						Name:        fmt.Sprintf("%s-2", multiClusterEngineName),
						Annotations: map[string]string{"deploymentmode": string(ModeHosted)},
					},
					Spec: MultiClusterEngineSpec{
						TargetNamespace:    "new",
						AvailabilityConfig: "low",
					},
				}
				Expect(k8sClient.Create(ctx, mce)).NotTo(BeNil(), "Invalid availability config is not allowed")
			})
			By("because of component configuration", func() {
				mce := &MultiClusterEngine{
					ObjectMeta: metav1.ObjectMeta{
						Name:        fmt.Sprintf("%s-2", multiClusterEngineName),
						Annotations: map[string]string{"deploymentmode": string(ModeHosted)},
					},
					Spec: MultiClusterEngineSpec{
						TargetNamespace: "new",
						Overrides: &Overrides{
							Components: []ComponentConfig{
								{
									Name:    "fake-component",
									Enabled: true,
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, mce)).NotTo(BeNil(), "Invalid components not allowed in config")
			})
		})

		It("Should fail to update multiclusterengine", func() {
			mce := &MultiClusterEngine{}

			By("because of TargetNamespace", func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: multiClusterEngineName}, mce)).To(Succeed())
				mce.Spec.TargetNamespace = "new"
				Expect(k8sClient.Update(ctx, mce)).NotTo(BeNil(), "Target namespace should not change")
			})
			By("because of DeploymentMode", func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: multiClusterEngineName}, mce)).To(Succeed())
				mce.SetAnnotations(map[string]string{"deploymentmode": string(ModeHosted)})
				Expect(k8sClient.Update(ctx, mce)).NotTo(BeNil(), "DeploymentMode should not change")
			})

			By("because of invalid component", func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: multiClusterEngineName}, mce)).To(Succeed())
				mce.Spec.Overrides = &Overrides{
					Components: []ComponentConfig{
						{
							Name:    "fake-component",
							Enabled: true,
						},
					},
				}
				Expect(k8sClient.Update(ctx, mce)).NotTo(BeNil(), "invalid components should not be permitted")
			})

			By("removing invalid component", func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: multiClusterEngineName}, mce)).To(Succeed())
				mce.Spec.Overrides = &Overrides{}
				Expect(k8sClient.Update(ctx, mce)).To(Succeed())
			})

			By("because of existing local-cluster resource", func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: multiClusterEngineName}, mce)).To(Succeed())
				managedCluster := NewManagedCluster(mce.Spec.LocalClusterName)
				Expect(k8sClient.Create(ctx, managedCluster)).To(Succeed())

				mce.Spec.LocalClusterName = "updated-local-cluster"
				Expect(k8sClient.Update(ctx, mce)).NotTo(BeNil(), "updating local-Cluster name while one exists should not be permitted")
			})

			By("because the local-cluster name must be less than 35 characters long", func() {
				mce.Spec.LocalClusterName = strings.Repeat("t", 35)
				expectedError := &k8serrors.StatusError{
					ErrStatus: metav1.Status{
						TypeMeta: metav1.TypeMeta{Kind: "", APIVersion: ""},
						ListMeta: metav1.ListMeta{
							SelfLink:           "",
							ResourceVersion:    "",
							Continue:           "",
							RemainingItemCount: nil,
						},
						Status:  "Failure",
						Message: "admission webhook \"multiclusterengines.multicluster.openshift.io\" denied the request: local-cluster name must be shorter than 35 characters",
						Reason:  "Forbidden",
						Details: nil,
						Code:    403,
					},
				}
				Expect(k8sClient.Update(ctx, mce)).To(Equal(expectedError), "local-cluster name must be less than 35 characters long")
			})
		})

		It("Should fail to delete multiclusterengine when AgentServiceConfig exists", func() {
			mce := &MultiClusterEngine{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: multiClusterEngineName}, mce)).To(Succeed())

			By("Creating an AgentServiceConfig resource", func() {
				agentServiceConfig := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "agent-install.openshift.io/v1beta1",
						"kind":       "AgentServiceConfig",
						"metadata": map[string]interface{}{
							"name": "test-agent-service-config",
						},
						"spec": map[string]interface{}{
							"databaseStorage": map[string]interface{}{
								"storageClassName": "test-storage",
							},
							"filesystemStorage": map[string]interface{}{
								"storageClassName": "test-storage",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentServiceConfig)).To(Succeed())
			})

			By("Attempting to delete MCE should fail", func() {
				err := k8sClient.Delete(ctx, mce)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("Existing AgentServiceConfig resources must first be deleted"))
			})

			By("Cleaning up AgentServiceConfig", func() {
				agentServiceConfig := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "agent-install.openshift.io/v1beta1",
						"kind":       "AgentServiceConfig",
						"metadata": map[string]interface{}{
							"name": "test-agent-service-config",
						},
					},
				}
				Expect(k8sClient.Delete(ctx, agentServiceConfig)).To(Succeed())
			})
		})

		It("Should fail to delete multiclusterengine when ClusterPool exists", func() {
			mce := &MultiClusterEngine{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: multiClusterEngineName}, mce)).To(Succeed())

			By("Creating a ClusterPool resource", func() {
				clusterPool := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "hive.openshift.io/v1",
						"kind":       "ClusterPool",
						"metadata": map[string]interface{}{
							"name":      "test-pool",
							"namespace": "default",
						},
						"spec": map[string]interface{}{
							"size":       1,
							"baseDomain": "test.example.com",
							"imageSetRef": map[string]interface{}{
								"name": "test-imageset",
							},
							"platform": map[string]interface{}{
								"aws": map[string]interface{}{
									"region": "us-east-1",
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, clusterPool)).To(Succeed())
			})

			By("Attempting to delete MCE should fail", func() {
				err := k8sClient.Delete(ctx, mce)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("Existing ClusterPool resources must first be deleted"))
			})

			By("Cleaning up ClusterPool", func() {
				clusterPool := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "hive.openshift.io/v1",
						"kind":       "ClusterPool",
						"metadata": map[string]interface{}{
							"name":      "test-pool",
							"namespace": "default",
						},
					},
				}
				Expect(k8sClient.Delete(ctx, clusterPool)).To(Succeed())
			})
		})

		It("Should fail to delete multiclusterengine when DiscoveryConfig exists", func() {
			mce := &MultiClusterEngine{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: multiClusterEngineName}, mce)).To(Succeed())

			By("Creating a DiscoveryConfig resource", func() {
				discoveryConfig := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "discovery.open-cluster-management.io/v1",
						"kind":       "DiscoveryConfig",
						"metadata": map[string]interface{}{
							"name":      "test-discovery",
							"namespace": "default",
						},
						"spec": map[string]interface{}{
							"credential": "test-credential",
						},
					},
				}
				Expect(k8sClient.Create(ctx, discoveryConfig)).To(Succeed())
			})

			By("Attempting to delete MCE should fail", func() {
				err := k8sClient.Delete(ctx, mce)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("Existing DiscoveryConfig resources must first be deleted"))
			})

			By("Cleaning up DiscoveryConfig", func() {
				discoveryConfig := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "discovery.open-cluster-management.io/v1",
						"kind":       "DiscoveryConfig",
						"metadata": map[string]interface{}{
							"name":      "test-discovery",
							"namespace": "default",
						},
					},
				}
				Expect(k8sClient.Delete(ctx, discoveryConfig)).To(Succeed())
			})
		})

		It("Should succeed in deleting multiclusterengine", func() {
			mce := &MultiClusterEngine{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: multiClusterEngineName}, mce)).To(Succeed())

			managedCluster := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "cluster.open-cluster-management.io/v1",
					"kind":       "ManagedCluster",
					"metadata": map[string]interface{}{
						"name": mce.Spec.LocalClusterName,
					},
				},
			}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mce.Spec.LocalClusterName}, managedCluster)).To(Succeed())

			By("Deleting the multiclusterengine", func() {
				Expect(k8sClient.Delete(ctx, mce)).To(Succeed())
			})
		})

	})

})

// re-defining the function here to avoid a import cycle
func NewManagedCluster(name string) *unstructured.Unstructured {
	managedCluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": name,
				"labels": map[string]interface{}{
					"local-cluster":                 "true",
					"cloud":                         "auto-detect",
					"vendor":                        "auto-detect",
					"velero.io/exclude-from-backup": "true",
				},
			},
			"spec": map[string]interface{}{
				"hubAcceptsClient": true,
			},
		},
	}
	return managedCluster
}
