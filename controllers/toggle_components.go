package controllers

import (
	"context"
	"fmt"

	operatorv1 "github.com/openshift/api/operator/v1"
	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/foundation"
	renderer "github.com/stolostron/backplane-operator/pkg/rendering"
	"github.com/stolostron/backplane-operator/pkg/toggle"
	"github.com/stolostron/backplane-operator/pkg/utils"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *MultiClusterEngineReconciler) ensureConsoleMCE(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	namespacedName := types.NamespacedName{Name: "console-mce-console", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))

	log := log.FromContext(ctx)

	templates, errs := renderer.RenderChart(toggle.ConsoleMCEChartsDir, backplaneConfig, r.Images)
	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Applies all templates
	for _, template := range templates {
		result, err := r.applyTemplate(ctx, backplaneConfig, template)
		if err != nil {
			return result, err
		}
	}

	components := backplaneConfig.Status.Components
	for _, component := range components {
		if component.Name == "console-mce-console" {
			if component.Status == "True" && component.Type == "Available" {
				result, err := r.addPluginToConsoleResource(ctx, backplaneConfig)
				if err != nil {
					return result, err
				}
				return ctrl.Result{}, nil
			} else {
				log.Info("Console MCE is not yet available. Waiting to enable console plugin")
				return ctrl.Result{RequeueAfter: requeuePeriod}, nil
			}
		}
		continue
	}

	return ctrl.Result{Requeue: true}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoConsoleMCE(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	namespacedName := types.NamespacedName{Name: "console-mce-console", Namespace: backplaneConfig.Spec.TargetNamespace}

	result, err := r.removePluginFromConsoleResource(ctx, backplaneConfig)
	if err != nil {
		return result, err
	}

	// Renders all templates from charts
	templates, errs := renderer.RenderChart(toggle.ConsoleMCEChartsDir, backplaneConfig, r.Images)
	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))
	r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))

	// Deletes all templates
	for _, template := range templates {
		result, err := r.deleteTemplate(ctx, backplaneConfig, template)
		if err != nil {
			log.Error(err, fmt.Sprintf("Failed to delete Console MCE template: %s", template.GetName()))
			return result, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureManagedServiceAccount(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(types.NamespacedName{Name: "managedservice", Namespace: backplaneConfig.Spec.TargetNamespace}, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(types.NamespacedName{Name: "managed-serviceaccount-addon-manager", Namespace: backplaneConfig.Spec.TargetNamespace}))

	log := log.FromContext(ctx)

	if foundation.CanInstallAddons(ctx, r.Client) {
		// Render CRD templates
		crdPath := toggle.ManagedServiceAccountCRDPath
		crds, errs := renderer.RenderCRDs(crdPath)
		if len(errs) > 0 {
			for _, err := range errs {
				log.Info(err.Error())
			}
			return ctrl.Result{RequeueAfter: requeuePeriod}, nil
		}

		// Apply all CRDs
		for _, crd := range crds {
			result, err := r.applyTemplate(ctx, backplaneConfig, crd)
			if err != nil {
				return result, err
			}
		}

		// Renders all templates from charts
		chartPath := toggle.ManagedServiceAccountChartDir
		templates, errs := renderer.RenderChart(chartPath, backplaneConfig, r.Images)
		if len(errs) > 0 {
			for _, err := range errs {
				log.Info(err.Error())
			}
			return ctrl.Result{RequeueAfter: requeuePeriod}, nil
		}

		// Applies all templates
		for _, template := range templates {
			result, err := r.applyTemplate(ctx, backplaneConfig, template)
			if err != nil {
				return result, err
			}
		}
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoManagedServiceAccount(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Renders all templates from charts
	chartPath := toggle.ManagedServiceAccountChartDir
	templates, errs := renderer.RenderChart(chartPath, backplaneConfig, r.Images)
	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	r.StatusManager.RemoveComponent(toggle.EnabledStatus(types.NamespacedName{Name: "managed-serviceaccount-addon-manager", Namespace: backplaneConfig.Spec.TargetNamespace}))
	r.StatusManager.AddComponent(toggle.DisabledStatus(types.NamespacedName{Name: "managedservice", Namespace: backplaneConfig.Spec.TargetNamespace}, []*unstructured.Unstructured{}))

	// Deletes all templates
	for _, template := range templates {
		if template.GetKind() == foundation.ClusterManagementAddonKind && !foundation.CanInstallAddons(ctx, r.Client) {
			// Can't delete ClusterManagementAddon if Kind doesn't exists
			continue
		}
		result, err := r.deleteTemplate(ctx, backplaneConfig, template)
		if err != nil {
			log.Error(err, "Failed to delete MSA template")
			return result, err
		}
	}

	// Render CRD templates
	crdPath := toggle.ManagedServiceAccountCRDPath
	crds, errs := renderer.RenderCRDs(crdPath)
	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Delete all CRDs
	for _, crd := range crds {
		result, err := r.deleteTemplate(ctx, backplaneConfig, crd)
		if err != nil {
			log.Error(err, "Failed to delete CRD")
			return result, err
		}
	}
	return ctrl.Result{}, nil
}

// addPluginToConsoleResource ...
func (r *MultiClusterEngineReconciler) addPluginToConsoleResource(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	console := &operatorv1.Console{}
	// If trying to check this resource from the CLI run - `oc get consoles.operator.openshift.io cluster`.
	// The default `console` is not the correct resource
	err := r.Client.Get(ctx, types.NamespacedName{Name: "cluster"}, console)
	if err != nil {
		log.Info("Failed to find console: cluster")
		return ctrl.Result{Requeue: true}, err
	}

	if console.Spec.Plugins == nil {
		console.Spec.Plugins = []string{}
	}

	// Add mce to the plugins list if it is not already there
	if !utils.Contains(console.Spec.Plugins, "mce") {
		log.Info("Ready to add plugin")
		console.Spec.Plugins = append(console.Spec.Plugins, "mce")
		err = r.Client.Update(ctx, console)
		if err != nil {
			log.Info("Failed to add mce consoleplugin to console")
			return ctrl.Result{Requeue: true}, err
		} else {
			log.Info("Added mce consoleplugin to console")
		}
	}

	return ctrl.Result{}, nil
}

// removePluginFromConsoleResource ...
func (r *MultiClusterEngineReconciler) removePluginFromConsoleResource(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	console := &operatorv1.Console{}
	// If trying to check this resource from the CLI run - `oc get consoles.operator.openshift.io cluster`.
	// The default `console` is not the correct resource
	err := r.Client.Get(ctx, types.NamespacedName{Name: "cluster"}, console)
	if err != nil {
		log.Info("Failed to find console: cluster")
		return ctrl.Result{Requeue: true}, err
	}

	// If No plugins, it is already removed
	if console.Spec.Plugins == nil {
		return ctrl.Result{}, nil
	}

	// Remove mce to the plugins list if it is not already there
	if utils.Contains(console.Spec.Plugins, "mce") {
		console.Spec.Plugins = utils.Remove(console.Spec.Plugins, "mce")
		err = r.Client.Update(ctx, console)
		if err != nil {
			log.Info("Failed to remove mce consoleplugin to console")
			return ctrl.Result{Requeue: true}, err
		} else {
			log.Info("Removed mce consoleplugin to console")
		}
	}

	return ctrl.Result{}, nil
}
