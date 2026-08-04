// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"time"

	consolev1 "github.com/openshift/api/console/v1"
	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/version"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("OCP compliance ConsoleNotification banner", func() {
	var (
		ctx context.Context
		mce *backplanev1.MultiClusterEngine
	)

	BeforeEach(func() {
		ctx = context.TODO()
		mce = &backplanev1.MultiClusterEngine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multiclusterengine",
				Namespace: "multicluster-engine",
			},
		}
	})

	AfterEach(func() {
		// Clean up banner if it exists
		notification := &consolev1.ConsoleNotification{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: ocpComplianceBannerName}, notification); err == nil {
			_ = k8sClient.Delete(ctx, notification)
		}
	})

	Context("ensureOCPComplianceBanner", func() {
		It("creates a banner when OCP version is below minimum", func() {
			err := reconciler.ensureOCPComplianceBanner(ctx, mce, "4.17.0")
			Expect(err).NotTo(HaveOccurred())

			notification := &consolev1.ConsoleNotification{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: ocpComplianceBannerName}, notification)
			Expect(err).NotTo(HaveOccurred())

			Expect(notification.Spec.Text).To(Equal(ocpComplianceBannerText("4.17.0", version.MinimumOCPVersion)))
			Expect(notification.Spec.Location).To(Equal(consolev1.BannerTop))
			Expect(notification.Spec.BackgroundColor).To(Equal(bannerBackgroundColor))
			Expect(notification.Spec.Color).To(Equal(bannerTextColor))
			Expect(notification.Spec.Link).NotTo(BeNil())
			Expect(notification.Spec.Link.Href).To(Equal(bannerSupportLinkHref))
			Expect(notification.Spec.Link.Text).To(Equal(bannerSupportLinkText))
			Expect(notification.Labels["installer.name"]).To(Equal(mce.GetName()))
			Expect(notification.Labels["installer.namespace"]).To(Equal(mce.GetNamespace()))
		})

		It("does not create a banner when OCP version meets minimum", func() {
			err := reconciler.ensureOCPComplianceBanner(ctx, mce, version.MinimumOCPVersion)
			Expect(err).NotTo(HaveOccurred())

			notification := &consolev1.ConsoleNotification{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: ocpComplianceBannerName}, notification)
			Expect(err).To(HaveOccurred())
		})

		It("removes existing banner when OCP version meets minimum", func() {
			// Pre-create a banner
			existing := &consolev1.ConsoleNotification{
				ObjectMeta: metav1.ObjectMeta{
					Name: ocpComplianceBannerName,
					Labels: map[string]string{
						"installer.name":      mce.GetName(),
						"installer.namespace": mce.GetNamespace(),
					},
				},
				Spec: consolev1.ConsoleNotificationSpec{
					Text:            "old warning",
					Location:        consolev1.BannerTop,
					BackgroundColor: bannerBackgroundColor,
					Color:           bannerTextColor,
				},
			}
			Expect(k8sClient.Create(ctx, existing)).To(Succeed())

			// Now pass a valid OCP version. ensureOCPComplianceBanner no-ops if
			// the reconciler's cached client hasn't yet observed the
			// pre-created banner, so retry the call together with the
			// existence check until the removal actually takes effect.
			Eventually(func() error {
				_ = reconciler.ensureOCPComplianceBanner(ctx, mce, version.MinimumOCPVersion)
				notification := &consolev1.ConsoleNotification{}
				return k8sClient.Get(ctx, types.NamespacedName{Name: ocpComplianceBannerName}, notification)
			}).WithTimeout(5 * time.Second).Should(HaveOccurred())
		})

		It("does not create a banner when OCP version is empty", func() {
			err := reconciler.ensureOCPComplianceBanner(ctx, mce, "")
			Expect(err).NotTo(HaveOccurred())

			notification := &consolev1.ConsoleNotification{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: ocpComplianceBannerName}, notification)
			Expect(err).To(HaveOccurred())
		})

		It("updates an existing banner when OCP version changes", func() {
			// Create initial banner
			Expect(reconciler.ensureOCPComplianceBanner(ctx, mce, "4.17.0")).To(Succeed())

			notification := &consolev1.ConsoleNotification{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ocpComplianceBannerName}, notification)).To(Succeed())
			expectedText := ocpComplianceBannerText("4.17.0", version.MinimumOCPVersion)

			// Force a stale text via direct update to simulate out-of-sync state
			notification.Spec.Text = "stale text"
			Expect(k8sClient.Update(ctx, notification)).To(Succeed())

			// ensureOCPComplianceBanner is a no-op if the reconciler's cached
			// client hasn't yet observed the "stale text" update, so retry the
			// call together with the text check until the patch actually lands.
			Eventually(func() string {
				_ = reconciler.ensureOCPComplianceBanner(ctx, mce, "4.17.0")
				updated := &consolev1.ConsoleNotification{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: ocpComplianceBannerName}, updated); err != nil {
					return ""
				}
				return updated.Spec.Text
			}).WithTimeout(5 * time.Second).Should(Equal(expectedText))
		})
	})

	Context("removeOCPComplianceBanner", func() {
		It("removes an existing banner", func() {
			existing := &consolev1.ConsoleNotification{
				ObjectMeta: metav1.ObjectMeta{
					Name: ocpComplianceBannerName,
					Labels: map[string]string{
						"installer.name":      mce.GetName(),
						"installer.namespace": mce.GetNamespace(),
					},
				},
				Spec: consolev1.ConsoleNotificationSpec{
					Text:     "warning",
					Location: consolev1.BannerTop,
				},
			}
			Expect(k8sClient.Create(ctx, existing)).To(Succeed())

			// removeOCPComplianceBanner is a no-op (returns nil) if the
			// reconciler's cached client hasn't yet observed the pre-created
			// banner, so retry the call together with the existence check
			// until the deletion actually takes effect.
			Eventually(func() bool {
				_ = reconciler.removeOCPComplianceBanner(ctx)
				notification := &consolev1.ConsoleNotification{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: ocpComplianceBannerName}, notification)
				return err != nil
			}).WithTimeout(5 * time.Second).Should(BeTrue())
		})

		It("is a no-op when banner does not exist", func() {
			Expect(reconciler.removeOCPComplianceBanner(ctx)).To(Succeed())
		})
	})

	Context("ocpComplianceBannerText", func() {
		It("includes current and minimum version in text", func() {
			text := ocpComplianceBannerText("4.18.0", "4.19.0")
			Expect(text).To(ContainSubstring("4.18.0"))
			Expect(text).To(ContainSubstring("4.19.0"))
			Expect(text).To(ContainSubstring("WARNING"))
		})
	})
})
