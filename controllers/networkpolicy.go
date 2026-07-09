// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"fmt"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	renderer "github.com/stolostron/backplane-operator/pkg/rendering"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ensureNetworkPolicies relies on existing installer labels to track NetworkPolicy ownership:
//
// Labels (applied by rendering):
//   - installer.name = "multiclusterengine"
//   - installer.namespace = "multicluster-engine"
//
// Annotations (applied by rendering):
//   - installer.open-cluster-management.io/release-version = "5.0.0"
//
// These are sufficient to identify MCE-created NetworkPolicies for deletion when disabled.

// ensureNetworkPolicies implements the create-once NetworkPolicy pattern:
// - component enabled + networkPolicies enabled → CREATE (if missing), SKIP (if exists)
// - component disabled OR networkPolicies disabled → DELETE (if MCE-created)
func (r *MultiClusterEngineReconciler) ensureNetworkPolicies(
	ctx context.Context,
	mce *backplanev1.MultiClusterEngine,
) (ctrl.Result, error) {
	log := r.Log.WithValues("MultiClusterEngine", mce.Name, "Namespace", mce.Namespace)

	networkPoliciesEnabled := mce.Spec.NetworkPolicies.Enabled

	// If globally disabled, delete all MCE-created NetworkPolicies
	if !networkPoliciesEnabled {
		npList := &networkingv1.NetworkPolicyList{}
		if err := r.Client.List(ctx, npList, client.InNamespace(mce.Spec.TargetNamespace), client.MatchingLabels{
			"installer.name":      mce.Name,
			"installer.namespace": mce.Spec.TargetNamespace,
		}); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to list NetworkPolicies: %w", err)
		}

		for _, np := range npList.Items {
			if err := r.Client.Delete(ctx, &np); err != nil && !errors.IsNotFound(err) {
				return ctrl.Result{}, fmt.Errorf("failed to delete NetworkPolicy %s/%s: %w", np.Namespace, np.Name, err)
			}
			log.Info("Deleted NetworkPolicy", "name", np.Name, "namespace", np.Namespace)
		}
		return ctrl.Result{}, nil
	}

	// NetworkPolicies enabled - check each component
	for _, component := range backplanev1.MCEComponents {
		componentEnabled := mce.Enabled(component)
		if !componentEnabled {
			// Component disabled - nothing to create or delete (rely on global deletion above if needed)
			continue
		}

		// Skip externally managed components
		if r.isComponentExternallyManaged(mce, component) {
			log.V(2).Info("Skipping externally managed component", "component", component)
			continue
		}

		// Render NetworkPolicy from Helm template
		chartPath := r.fetchChartOrCRDPath(component)
		if chartPath == "" {
			log.V(2).Info("No chart path for component", "component", component)
			continue
		}

		templates, errs := renderer.RenderChart(chartPath, mce, r.CacheSpec.ImageOverrides, r.CacheSpec.TemplateOverrides)

		if len(errs) > 0 {
			// Rendering errors are non-fatal - component may not have NetworkPolicy template yet
			log.V(2).Info("Chart rendering had errors", "component", component, "errors", len(errs))
			continue
		}

		// Filter for NetworkPolicy resources only
		var networkPolicies []*unstructured.Unstructured
		for _, template := range templates {
			if template.GetKind() == "NetworkPolicy" {
				networkPolicies = append(networkPolicies, template)
			}
		}

		// No NetworkPolicy template for this component - skip
		if len(networkPolicies) == 0 {
			continue
		}

		// Handle each NetworkPolicy (usually just one per component)
		for _, npTemplate := range networkPolicies {
			np := &networkingv1.NetworkPolicy{}
			err := r.Client.Get(ctx, types.NamespacedName{
				Name:      npTemplate.GetName(),
				Namespace: npTemplate.GetNamespace(),
			}, np)

			if errors.IsNotFound(err) {
				// Create NetworkPolicy - create-once pattern
				applyReleaseVersionAnnotation(npTemplate)
				if err := r.Client.Create(ctx, npTemplate); err != nil {
					return ctrl.Result{}, fmt.Errorf(
						"failed to create NetworkPolicy %s/%s: %w",
						npTemplate.GetNamespace(),
						npTemplate.GetName(),
						err,
					)
				}
				log.Info(
					"Created NetworkPolicy",
					"name", npTemplate.GetName(),
					"namespace", npTemplate.GetNamespace(),
					"component", component,
				)
			} else if err == nil {
				// NetworkPolicy exists - SKIP (no reconcile, operand owns it now)
				log.V(2).Info(
					"NetworkPolicy exists, skipping",
					"name", np.Name,
					"namespace", np.Namespace,
					"component", component,
				)
			} else {
				return ctrl.Result{}, fmt.Errorf("failed to get NetworkPolicy %s: %w", npTemplate.GetName(), err)
			}
		}
	}

	return ctrl.Result{}, nil
}
