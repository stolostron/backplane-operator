// Copyright Contributors to the Open Cluster Management project

package v1

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
						TargetNamespace: DefaultTargetNamespace,
					},
				}
				Expect(k8sClient.Create(ctx, mce)).Should(Succeed())
			})
			By("by creating a new hosted Multiclusterengine resource", func() {
				mce := &MultiClusterEngine{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hosted-mce",
					},
					Spec: MultiClusterEngineSpec{
						TargetNamespace: "hostedNS",
						DeploymentMode:  ModeHosted,
					},
				}
				Expect(k8sClient.Create(ctx, mce)).Should(Succeed())
			})
		})

		It("Should fail to create multiclusterengine", func() {
			By("because of TargetNamespace", func() {
				mce := &MultiClusterEngine{
					ObjectMeta: metav1.ObjectMeta{
						Name: fmt.Sprintf("%s-2", multiClusterEngineName),
					},
					Spec: MultiClusterEngineSpec{
						DeploymentMode:  ModeHosted,
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
						DeploymentMode:  ModeStandalone,
					},
				}
				Expect(k8sClient.Create(ctx, mce)).NotTo(BeNil(), "Only one MCE in standalone mode allowed")
			})
			By("because of invalid DeploymentMode", func() {
				mce := &MultiClusterEngine{
					ObjectMeta: metav1.ObjectMeta{
						Name: fmt.Sprintf("%s-2", multiClusterEngineName),
					},
					Spec: MultiClusterEngineSpec{
						TargetNamespace: "new",
						DeploymentMode:  "nonMode",
					},
				}
				Expect(k8sClient.Create(ctx, mce)).NotTo(BeNil(), "Invalid deployment mode is not allowed")
			})
			By("because of invalid AvailabilityConfig", func() {
				mce := &MultiClusterEngine{
					ObjectMeta: metav1.ObjectMeta{
						Name: fmt.Sprintf("%s-2", multiClusterEngineName),
					},
					Spec: MultiClusterEngineSpec{
						TargetNamespace:    "new",
						DeploymentMode:     ModeHosted,
						AvailabilityConfig: "low",
					},
				}
				Expect(k8sClient.Create(ctx, mce)).NotTo(BeNil(), "Invalid availability config is not allowed")
			})
			By("because of component configuration", func() {
				mce := &MultiClusterEngine{
					ObjectMeta: metav1.ObjectMeta{
						Name: fmt.Sprintf("%s-2", multiClusterEngineName),
					},
					Spec: MultiClusterEngineSpec{
						TargetNamespace: "new",
						DeploymentMode:  ModeHosted,
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
				mce.Spec.DeploymentMode = ModeHosted
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
		})

	})

})
