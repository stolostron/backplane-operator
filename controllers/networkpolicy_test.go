// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("NetworkPolicy Controller", Ordered, func() {
	const (
		mceName           = "test-mce"
		targetNS          = "multicluster-engine"
		networkPolicyName = "test-networkpolicy"
	)

	var (
		mce *backplanev1.MultiClusterEngine
		np  *networkingv1.NetworkPolicy
	)

	BeforeAll(func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: targetNS,
			},
		}
		Expect(k8sClient.Create(context.Background(), ns)).To(Succeed())
	})

	BeforeEach(func() {
		mce = &backplanev1.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{
				Name: mceName,
			},
			Spec: backplanev1.MultiClusterEngineSpec{
				TargetNamespace: targetNS,
				NetworkPolicies: backplanev1.NetworkPoliciesConfig{
					Enabled: true,
				},
			},
		}

		np = &networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      networkPolicyName,
				Namespace: targetNS,
				Labels: map[string]string{
					"installer.name":      mceName,
					"installer.namespace": targetNS,
				},
			},
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{},
				PolicyTypes: []networkingv1.PolicyType{
					networkingv1.PolicyTypeIngress,
				},
			},
		}
	})

	AfterEach(func() {
		ctx := context.Background()

		// Delete all NetworkPolicies in namespace
		npList := &networkingv1.NetworkPolicyList{}
		_ = k8sClient.List(ctx, npList, client.InNamespace(targetNS))
		for i := range npList.Items {
			_ = k8sClient.Delete(ctx, &npList.Items[i])
		}
	})

	Context("when NetworkPolicies are disabled", func() {
		It("should delete all MCE-created NetworkPolicies", func() {
			ctx := context.Background()

			// Create NetworkPolicy with MCE installer labels
			Expect(k8sClient.Create(ctx, np)).To(Succeed())

			// Verify NetworkPolicy exists
			createdNP := &networkingv1.NetworkPolicy{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      networkPolicyName,
					Namespace: targetNS,
				}, createdNP)
			}).Should(Succeed())

			// Disable NetworkPolicies
			mce.Spec.NetworkPolicies.Enabled = false

			// Run ensureNetworkPolicies
			result, err := reconciler.ensureNetworkPolicies(ctx, mce)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			// Verify NetworkPolicy was deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      networkPolicyName,
					Namespace: targetNS,
				}, createdNP)
				return client.IgnoreNotFound(err) == nil && err != nil
			}).Should(BeTrue())
		})

		It("should delete multiple MCE-created NetworkPolicies", func() {
			ctx := context.Background()

			// Create multiple NetworkPolicies
			np2 := np.DeepCopy()
			np2.Name = "test-networkpolicy-2"

			Expect(k8sClient.Create(ctx, np)).To(Succeed())
			Expect(k8sClient.Create(ctx, np2)).To(Succeed())

			// Disable NetworkPolicies
			mce.Spec.NetworkPolicies.Enabled = false

			// Run ensureNetworkPolicies
			result, err := reconciler.ensureNetworkPolicies(ctx, mce)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			// Verify both NetworkPolicies were deleted
			Eventually(func() int {
				npList := &networkingv1.NetworkPolicyList{}
				err := k8sClient.List(ctx, npList,
					client.InNamespace(targetNS),
					client.MatchingLabels{
						"installer.name":      mceName,
						"installer.namespace": targetNS,
					})
				if err != nil {
					return -1
				}
				return len(npList.Items)
			}).Should(Equal(0))
		})

		It("should not delete NetworkPolicies from other MCE instances", func() {
			ctx := context.Background()

			// Create NetworkPolicy for different MCE
			otherNP := np.DeepCopy()
			otherNP.Name = "other-mce-networkpolicy"
			otherNP.Labels["installer.name"] = "other-mce"

			Expect(k8sClient.Create(ctx, np)).To(Succeed())
			Expect(k8sClient.Create(ctx, otherNP)).To(Succeed())

			// Disable NetworkPolicies for this MCE
			mce.Spec.NetworkPolicies.Enabled = false

			// Run ensureNetworkPolicies
			result, err := reconciler.ensureNetworkPolicies(ctx, mce)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			// Verify only this MCE's NetworkPolicy was deleted
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      networkPolicyName,
					Namespace: targetNS,
				}, &networkingv1.NetworkPolicy{})
			}).ShouldNot(Succeed())

			// Verify other MCE's NetworkPolicy still exists
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      otherNP.Name,
				Namespace: targetNS,
			}, &networkingv1.NetworkPolicy{})).To(Succeed())
		})

		It("should handle delete errors gracefully for non-existent NetworkPolicies", func() {
			ctx := context.Background()

			// Don't create any NetworkPolicies
			mce.Spec.NetworkPolicies.Enabled = false

			// Run ensureNetworkPolicies
			result, err := reconciler.ensureNetworkPolicies(ctx, mce)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	Context("when NetworkPolicies are enabled", func() {
		It("should not delete existing NetworkPolicies", func() {
			ctx := context.Background()

			// Create NetworkPolicy
			Expect(k8sClient.Create(ctx, np)).To(Succeed())

			// Keep NetworkPolicies enabled
			mce.Spec.NetworkPolicies.Enabled = true

			// Run ensureNetworkPolicies (will skip creation since no chart templates in test)
			result, err := reconciler.ensureNetworkPolicies(ctx, mce)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			// Verify NetworkPolicy still exists
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      networkPolicyName,
				Namespace: targetNS,
			}, &networkingv1.NetworkPolicy{})).To(Succeed())
		})
	})
})
