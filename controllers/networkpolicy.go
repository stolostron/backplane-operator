// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	renderer "github.com/stolostron/backplane-operator/pkg/rendering"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	managedEgressRulesAnnotation       = "installer.open-cluster-management.io/managed-egress-rules"
	legacyManagedEgressRulesAnnotation = "installer.open-cluster-management.io/legacy-managed-egress-rules"
)

// ensureNetworkPolicies relies on the existing backplaneconfig.name label to track
// NetworkPolicy ownership:
//
// Labels (applied by rendering via utils.AddBackplaneConfigLabels):
//   - backplaneconfig.name = "<mce.Name>" (e.g. "multiclusterengine")
//
// Annotations (applied by rendering):
//   - installer.open-cluster-management.io/release-version = "5.0.0"
//
// The backplaneconfig.name label is sufficient to identify MCE-created NetworkPolicies
// for deletion when disabled.

// ensureNetworkPolicies creates missing NetworkPolicies and reconciles the
// egress rules it owns on existing MCE-created policies.
// - component disabled OR networkPolicies disabled → DELETE (if MCE-created)
func (r *MultiClusterEngineReconciler) ensureNetworkPolicies(
	ctx context.Context,
	mce *backplanev1.MultiClusterEngine,
) (ctrl.Result, error) {
	log := r.Log.WithValues("MultiClusterEngine", mce.Name, "Namespace", mce.Namespace)

	networkPoliciesEnabled := true
	if mce.Spec.NetworkPolicies != nil {
		networkPoliciesEnabled = mce.Spec.NetworkPolicies.Enabled
	}

	// If globally disabled, delete all MCE-created NetworkPolicies
	if !networkPoliciesEnabled {
		npList := &networkingv1.NetworkPolicyList{}
		if err := r.Client.List(ctx, npList, client.InNamespace(mce.Spec.TargetNamespace), client.MatchingLabels{
			"backplaneconfig.name": mce.Name,
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

		// Skip components without chart directories (e.g. hypershift-local-hosting uses RenderHypershiftAddon)
		if component == backplanev1.HypershiftLocalHosting {
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

		for _, npTemplate := range networkPolicies {
			if err := r.ensureNetworkPolicy(ctx, mce, npTemplate, log); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reconcile NetworkPolicy %s: %w", npTemplate.GetName(), err)
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNetworkPolicy(
	ctx context.Context,
	mce *backplanev1.MultiClusterEngine,
	npTemplate *unstructured.Unstructured,
	log logr.Logger,
) error {
	applyReleaseVersionAnnotation(npTemplate)
	legacyManagedRules, err := legacyManagedEgressRules(npTemplate)
	if err != nil {
		return fmt.Errorf("failed to read legacy managed egress rules: %w", err)
	}
	desired := &networkingv1.NetworkPolicy{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(npTemplate.Object, desired); err != nil {
		return fmt.Errorf("failed to convert rendered NetworkPolicy: %w", err)
	}
	removeLegacyManagedEgressRules(desired)

	existing := &networkingv1.NetworkPolicy{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      desired.Name,
		Namespace: desired.Namespace,
	}, existing); err != nil {
		if errors.IsNotFound(err) {
			if err := setManagedEgressRules(desired, desired.Spec.Egress); err != nil {
				return err
			}
			if err := r.Client.Create(ctx, desired); err != nil {
				return fmt.Errorf("failed to create NetworkPolicy %s/%s: %w", desired.Namespace, desired.Name, err)
			}
			log.Info("Created NetworkPolicy", "name", desired.Name, "namespace", desired.Namespace)
			return nil
		}
		return fmt.Errorf("failed to get NetworkPolicy %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	if existing.Labels["backplaneconfig.name"] != mce.Name {
		log.V(2).Info("NetworkPolicy exists, skipping", "name", existing.Name, "namespace", existing.Namespace)
		return nil
	}

	oldManagedRules, hasManagedRules, err := managedEgressRules(existing)
	if err != nil {
		return fmt.Errorf("failed to read managed egress rules for %s/%s: %w", existing.Namespace, existing.Name, err)
	}
	if !hasManagedRules {
		oldManagedRules = legacyManagedRules
	}
	desiredRules := desired.Spec.Egress
	existingRules := existing.Spec.Egress
	mergedRules := mergeManagedEgressRules(existing.Spec.Egress, oldManagedRules, desiredRules)
	managedRules := managedEgressRulesAfterMerge(existingRules, oldManagedRules, desiredRules)
	encodedManagedRules, err := managedEgressRulesJSON(managedRules)
	if err != nil {
		return fmt.Errorf("failed to encode managed egress rules: %w", err)
	}
	if equalEgressRules(existingRules, mergedRules) && equalEgressRules(oldManagedRules, managedRules) &&
		existing.Annotations[managedEgressRulesAnnotation] == string(encodedManagedRules) {
		return nil
	}

	existing.Spec.Egress = mergedRules
	if err := setManagedEgressRules(existing, managedRules); err != nil {
		return err
	}
	removeLegacyManagedEgressRules(existing)
	if err := r.Client.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update NetworkPolicy %s/%s: %w", existing.Namespace, existing.Name, err)
	}
	log.Info("Updated NetworkPolicy egress rules", "name", existing.Name, "namespace", existing.Namespace)
	return nil
}

func setManagedEgressRules(policy *networkingv1.NetworkPolicy, rules []networkingv1.NetworkPolicyEgressRule) error {
	managedRules, err := managedEgressRulesJSON(rules)
	if err != nil {
		return fmt.Errorf("failed to encode managed egress rules: %w", err)
	}
	if policy.Annotations == nil {
		policy.Annotations = map[string]string{}
	}
	policy.Annotations[managedEgressRulesAnnotation] = string(managedRules)
	return nil
}

func managedEgressRules(
	policy *networkingv1.NetworkPolicy,
) ([]networkingv1.NetworkPolicyEgressRule, bool, error) {
	value, exists := policy.Annotations[managedEgressRulesAnnotation]
	if !exists || value == "" {
		return nil, false, nil
	}
	managedRules := []networkingv1.NetworkPolicyEgressRule{}
	if err := json.Unmarshal([]byte(value), &managedRules); err != nil {
		return nil, false, fmt.Errorf("invalid annotation value: %w", err)
	}
	return managedRules, true, nil
}

func legacyManagedEgressRules(npTemplate *unstructured.Unstructured) ([]networkingv1.NetworkPolicyEgressRule, error) {
	value, exists := npTemplate.GetAnnotations()[legacyManagedEgressRulesAnnotation]
	if !exists || value == "" {
		return nil, nil
	}
	legacyRules := []networkingv1.NetworkPolicyEgressRule{}
	if err := json.Unmarshal([]byte(value), &legacyRules); err != nil {
		return nil, fmt.Errorf("invalid annotation value: %w", err)
	}
	return legacyRules, nil
}

func removeLegacyManagedEgressRules(policy *networkingv1.NetworkPolicy) {
	if policy.Annotations == nil {
		return
	}
	delete(policy.Annotations, legacyManagedEgressRulesAnnotation)
	if len(policy.Annotations) == 0 {
		policy.Annotations = nil
	}
}

func managedEgressRulesJSON(rules []networkingv1.NetworkPolicyEgressRule) ([]byte, error) {
	if rules == nil {
		rules = []networkingv1.NetworkPolicyEgressRule{}
	}
	return json.Marshal(rules)
}

func mergeManagedEgressRules(
	existing, oldManaged, desired []networkingv1.NetworkPolicyEgressRule,
) []networkingv1.NetworkPolicyEgressRule {
	merged := make([]networkingv1.NetworkPolicyEgressRule, 0, len(existing)+len(desired))
	for _, rule := range existing {
		if containsEgressRule(oldManaged, rule) {
			continue
		}
		merged = append(merged, rule)
	}
	for _, rule := range desired {
		if !containsEgressRule(merged, rule) {
			merged = append(merged, rule)
		}
	}
	return merged
}

func managedEgressRulesAfterMerge(
	existing, oldManaged, desired []networkingv1.NetworkPolicyEgressRule,
) []networkingv1.NetworkPolicyEgressRule {
	managedRules := []networkingv1.NetworkPolicyEgressRule{}
	for _, rule := range desired {
		ownedBefore := containsEgressRule(oldManaged, rule)
		inserted := !containsEgressRule(existing, rule)
		if (ownedBefore || inserted) && !containsEgressRule(managedRules, rule) {
			managedRules = append(managedRules, rule)
		}
	}
	return managedRules
}

func containsEgressRule(rules []networkingv1.NetworkPolicyEgressRule, rule networkingv1.NetworkPolicyEgressRule) bool {
	for _, candidate := range rules {
		if reflect.DeepEqual(candidate, rule) {
			return true
		}
	}
	return false
}

func equalEgressRules(first, second []networkingv1.NetworkPolicyEgressRule) bool {
	if len(first) == 0 && len(second) == 0 {
		return true
	}
	return reflect.DeepEqual(first, second)
}
