// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"fmt"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

/*
legacyManagedResources lists resources that were removed from a component's Helm chart before
per-component resource tracking (InternalEngineComponentSpec.ManagedResources) existed. Because
InternalEngineComponent CRs created by older operator versions have no recorded resource history,
the generic drift-detection in cleanupOrphanedManagedResources cannot infer that these resources
should be deleted on the first reconcile after upgrading to an operator version that includes this
list. Entries here are checked unconditionally (in addition to the generic tracked-resource diff)
so that already-orphaned resources from prior releases are still cleaned up.

This is a temporary bridge: once a component has reconciled at least once with resource tracking
enabled, InternalEngineComponent.Spec.ManagedResources will accurately reflect its previously
deployed resources, and future drift will be caught generically. Entries below can be removed once
all supported upgrade paths have passed through a release that populates ManagedResources.

See ACM-40355 / stolostron/backplane-operator#3062: the console-mce component's legacy
ServiceMonitor ("console-mce-monitor") was removed from the chart, but upgrades that kept
console-mce enabled never rendered/deleted it, leaving customers with stale ServiceMonitors and
TargetDown alerts.
*/
var legacyManagedResources = map[string][]backplanev1.ManagedResource{
	backplanev1.ConsoleMCE: {
		{
			APIVersion: "monitoring.coreos.com/v1",
			Kind:       "ServiceMonitor",
			Name:       "console-mce-monitor",
			// Namespace is set to the MultiClusterEngine target namespace at cleanup time, since
			// the legacy resource was always deployed alongside MCE (not necessarily the
			// operator's own namespace, in cases where those differ).
		},
	},
}

// managedResourceKey returns a string uniquely identifying a ManagedResource for set comparisons.
func managedResourceKey(resource backplanev1.ManagedResource) string {
	return fmt.Sprintf("%s/%s/%s/%s", resource.APIVersion, resource.Kind, resource.Namespace, resource.Name)
}

// extractManagedResources builds the list of resources represented by the given rendered
// templates. NetworkPolicy resources are intentionally excluded because they are managed
// separately by ensureNetworkPolicies using a create-once pattern.
func extractManagedResources(templates []*unstructured.Unstructured) []backplanev1.ManagedResource {
	resources := make([]backplanev1.ManagedResource, 0, len(templates))
	for _, template := range templates {
		if template.GetKind() == "NetworkPolicy" {
			continue
		}

		resources = append(resources, backplanev1.ManagedResource{
			APIVersion: template.GetAPIVersion(),
			Kind:       template.GetKind(),
			Name:       template.GetName(),
			Namespace:  template.GetNamespace(),
		})
	}
	return resources
}

// managedResourcesEqual reports whether two ManagedResource lists represent the same set of
// resources, regardless of order.
func managedResourcesEqual(a, b []backplanev1.ManagedResource) bool {
	if len(a) != len(b) {
		return false
	}

	seen := make(map[string]struct{}, len(a))
	for _, resource := range a {
		seen[managedResourceKey(resource)] = struct{}{}
	}

	for _, resource := range b {
		if _, ok := seen[managedResourceKey(resource)]; !ok {
			return false
		}
	}
	return true
}

// getManagedResources returns the resources currently recorded on the component's
// InternalEngineComponent CR. It returns nil (without error) if the CR does not exist, since
// callers treat "no tracked resources" as an empty diff baseline rather than a failure.
func (r *MultiClusterEngineReconciler) getManagedResources(ctx context.Context, mce *backplanev1.MultiClusterEngine,
	component string) []backplanev1.ManagedResource {

	iec := &backplanev1.InternalEngineComponent{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: component, Namespace: mce.Spec.TargetNamespace},
		iec); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "failed to get InternalEngineComponent while reading managed resources",
				"Component", component, "Namespace", mce.Spec.TargetNamespace)
		}
		return nil
	}

	return iec.Spec.ManagedResources
}

// updateManagedResources patches the component's InternalEngineComponent CR with the current list
// of managed resources, but only if the list has actually changed, to avoid unnecessary writes on
// every reconcile.
func (r *MultiClusterEngineReconciler) updateManagedResources(ctx context.Context, mce *backplanev1.MultiClusterEngine,
	component string, resources []backplanev1.ManagedResource) error {

	iec := &backplanev1.InternalEngineComponent{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: component, Namespace: mce.Spec.TargetNamespace},
		iec); err != nil {
		if apierrors.IsNotFound(err) {
			// The InternalEngineComponent CR is created by ensureInternalEngineComponent; if it's
			// missing here there's nothing to update.
			return nil
		}
		return fmt.Errorf("failed to get InternalEngineComponent %s/%s: %v", mce.Spec.TargetNamespace, component, err)
	}

	if managedResourcesEqual(iec.Spec.ManagedResources, resources) {
		return nil
	}

	iec.Spec.ManagedResources = resources
	if err := r.Client.Update(ctx, iec); err != nil {
		return fmt.Errorf("failed to update InternalEngineComponent %s/%s managed resources: %v",
			mce.Spec.TargetNamespace, component, err)
	}

	return nil
}

// cleanupOrphanedManagedResources deletes resources that are present in oldResources but absent
// from newResources - i.e. resources that were deployed by a previous version of the component's
// templates but are no longer rendered. Deletion is delegated to deleteTemplate, which only
// removes resources that still carry this operator's backplaneconfig.name ownership label, so
// resources that were manually recreated (and therefore lack that label) are left untouched.
func (r *MultiClusterEngineReconciler) cleanupOrphanedManagedResources(ctx context.Context,
	mce *backplanev1.MultiClusterEngine, component string, oldResources,
	newResources []backplanev1.ManagedResource) (ctrl.Result, error) {

	current := make(map[string]struct{}, len(newResources))
	for _, resource := range newResources {
		current[managedResourceKey(resource)] = struct{}{}
	}

	// Merge in any known legacy resources for this component that predate resource tracking (see
	// legacyManagedResources doc comment). Namespace defaults to the MultiClusterEngine target
	// namespace when unset, matching how these resources were originally deployed.
	orphanCandidates := append([]backplanev1.ManagedResource{}, oldResources...)
	for _, legacy := range legacyManagedResources[component] {
		if legacy.Namespace == "" {
			legacy.Namespace = mce.Spec.TargetNamespace
		}
		orphanCandidates = append(orphanCandidates, legacy)
	}

	seenCandidates := make(map[string]struct{}, len(orphanCandidates))
	for _, resource := range orphanCandidates {
		key := managedResourceKey(resource)
		if _, alreadyHandled := seenCandidates[key]; alreadyHandled {
			continue
		}
		seenCandidates[key] = struct{}{}

		if _, stillRendered := current[key]; stillRendered {
			continue
		}

		stub := &unstructured.Unstructured{}
		stub.SetAPIVersion(resource.APIVersion)
		stub.SetKind(resource.Kind)
		stub.SetName(resource.Name)
		stub.SetNamespace(resource.Namespace)

		log.Info("Cleaning up resource no longer present in component templates",
			"Component", component, "APIVersion", resource.APIVersion, "Kind", resource.Kind,
			"Name", resource.Name, "Namespace", resource.Namespace)

		if result, err := r.deleteTemplate(ctx, mce, stub); result != (ctrl.Result{}) || err != nil {
			return result, err
		}
	}

	return ctrl.Result{}, nil
}
