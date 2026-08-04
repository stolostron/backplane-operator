// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"fmt"

	consolev1 "github.com/openshift/api/console/v1"
	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/version"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ocpComplianceBannerName = "mce-ocp-version-compliance"
	bannerBackgroundColor   = "var(--pf-v6-c-banner--m-danger--BackgroundColor, var(--pf-v5-c-banner--m-red--BackgroundColor))"
	bannerTextColor         = "var(--pf-v6-c-banner--m-danger--Color, var(--pf-v5-global--Color--100))"
	bannerSupportLinkHref   = "https://access.redhat.com/support"
	bannerSupportLinkText   = "Contact Red Hat Support"
)

func ocpComplianceBannerText(currentVersion, minimumVersion string) string {
	return fmt.Sprintf(
		"WARNING: MCE in unsupported configuration: OCP %s is below the minimum supported version %s.",
		currentVersion, minimumVersion,
	)
}

func (r *MultiClusterEngineReconciler) ensureOCPComplianceBanner(ctx context.Context,
	mce *backplanev1.MultiClusterEngine,
	ocpVersion string) error {

	if ocpVersion == "" {
		return r.removeOCPComplianceBanner(ctx)
	}

	// If OCP version is valid, remove banner
	if err := version.ValidOCPVersion(ocpVersion); err == nil {
		return r.removeOCPComplianceBanner(ctx)
	}

	desired := &consolev1.ConsoleNotification{
		ObjectMeta: metav1.ObjectMeta{
			Name: ocpComplianceBannerName,
			Labels: map[string]string{
				"installer.name":      mce.GetName(),
				"installer.namespace": mce.GetNamespace(),
			},
		},
		Spec: consolev1.ConsoleNotificationSpec{
			Text:            ocpComplianceBannerText(ocpVersion, version.MinimumOCPVersion),
			Location:        consolev1.BannerTop,
			BackgroundColor: bannerBackgroundColor,
			Color:           bannerTextColor,
			Link: &consolev1.Link{
				Text: bannerSupportLinkText,
				Href: bannerSupportLinkHref,
			},
		},
	}

	existing := &consolev1.ConsoleNotification{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: ocpComplianceBannerName}, existing)
	if errors.IsNotFound(err) {
		log.Info("Creating OCP compliance ConsoleNotification banner")
		if err := r.Client.Create(ctx, desired); err != nil {
			return fmt.Errorf("failed to create ConsoleNotification %s: %w", ocpComplianceBannerName, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get ConsoleNotification %s: %w", ocpComplianceBannerName, err)
	}

	if existing.Spec.Text != desired.Spec.Text ||
		existing.Spec.BackgroundColor != desired.Spec.BackgroundColor ||
		existing.Spec.Color != desired.Spec.Color ||
		existing.Spec.Location != desired.Spec.Location {
		patch := client.MergeFrom(existing.DeepCopy())
		existing.Spec = desired.Spec
		existing.Labels = desired.Labels
		log.Info("Updating OCP compliance ConsoleNotification banner")
		if err := r.Client.Patch(ctx, existing, patch); err != nil {
			return fmt.Errorf("failed to patch ConsoleNotification %s: %w", ocpComplianceBannerName, err)
		}
	}

	return nil
}

func (r *MultiClusterEngineReconciler) removeOCPComplianceBanner(ctx context.Context) error {
	notification := &consolev1.ConsoleNotification{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: ocpComplianceBannerName}, notification)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get ConsoleNotification %s: %w", ocpComplianceBannerName, err)
	}

	log.Info("Removing OCP compliance ConsoleNotification banner")
	if err := r.Client.Delete(ctx, notification); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ConsoleNotification %s: %w", ocpComplianceBannerName, err)
	}
	return nil
}

func (r *MultiClusterEngineReconciler) cleanupConsoleNotifications(ctx context.Context,
	mce *backplanev1.MultiClusterEngine) error {
	// Only one ConsoleNotification is ever created by this operator (the OCP
	// compliance banner), so remove it directly by name instead of using
	// DeleteAllOf, which requires the cluster-scoped "deletecollection" verb.
	return r.removeOCPComplianceBanner(ctx)
}
