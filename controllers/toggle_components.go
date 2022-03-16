package controllers

import (
	"context"
	"fmt"
	"os"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/pkg/errors"
	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/foundation"
	"github.com/stolostron/backplane-operator/pkg/hive"
	renderer "github.com/stolostron/backplane-operator/pkg/rendering"
	"github.com/stolostron/backplane-operator/pkg/status"
	"github.com/stolostron/backplane-operator/pkg/toggle"
	"github.com/stolostron/backplane-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
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
				log.Info("MCE console is not yet available. Waiting to enable console plugin")
				return ctrl.Result{RequeueAfter: requeuePeriod}, fmt.Errorf("MCE console is not yet available. Waiting to enable console plugin")
			}
		}
	}

	return ctrl.Result{RequeueAfter: requeuePeriod}, fmt.Errorf("MCE console is not yet available. Waiting to enable console plugin")
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
	if val, ok := os.LookupEnv("UNIT_TEST"); ok && val == "true" {
		return ctrl.Result{}, nil
	}

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

func (r *MultiClusterEngineReconciler) ensureDiscovery(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	namespacedName := types.NamespacedName{Name: "discovery-operator", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))

	log := log.FromContext(ctx)

	templates, errs := renderer.RenderChart(toggle.DiscoveryChartDir, backplaneConfig, r.Images)
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

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoDiscovery(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	namespacedName := types.NamespacedName{Name: "discovery-operator", Namespace: backplaneConfig.Spec.TargetNamespace}

	// Renders all templates from charts
	templates, errs := renderer.RenderChart(toggle.DiscoveryChartDir, backplaneConfig, r.Images)
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
			log.Error(err, fmt.Sprintf("Failed to delete template: %s", template.GetName()))
			return result, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureHive(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	namespacedName := types.NamespacedName{Name: "hive-operator", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))

	log := log.FromContext(ctx)

	templates, errs := renderer.RenderChart(toggle.HiveChartDir, backplaneConfig, r.Images)
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

	hiveTemplate := hive.HiveConfig(backplaneConfig)
	if err := ctrl.SetControllerReference(backplaneConfig, hiveTemplate, r.Scheme); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Error setting controller reference on resource %s", hiveTemplate.GetName())
	}

	result, err := r.ensureUnstructuredResource(ctx, backplaneConfig, hiveTemplate)
	if err != nil {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoHive(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	namespacedName := types.NamespacedName{Name: "hive-operator", Namespace: backplaneConfig.Spec.TargetNamespace}

	// Renders all templates from charts
	templates, errs := renderer.RenderChart(toggle.HiveChartDir, backplaneConfig, r.Images)
	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))
	r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))

	// Delete hivconfig
	hiveConfig := hive.HiveConfig(backplaneConfig)
	err := r.Client.Get(ctx, types.NamespacedName{Name: "hive"}, hiveConfig)
	if err == nil { // If resource exists, delete
		err := r.Client.Delete(ctx, hiveConfig)
		if err != nil {
			return ctrl.Result{RequeueAfter: requeuePeriod}, err
		}
	} else if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Deletes all templates
	for _, template := range templates {
		result, err := r.deleteTemplate(ctx, backplaneConfig, template)
		if err != nil {
			log.Error(err, fmt.Sprintf("Failed to delete template: %s", template.GetName()))
			return result, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureAssistedService(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	namespacedName := types.NamespacedName{Name: "infrastructure-operator", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))

	log := log.FromContext(ctx)

	templates, errs := renderer.RenderChart(toggle.AssistedServiceChartDir, backplaneConfig, r.Images)
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

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoAssistedService(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	namespacedName := types.NamespacedName{Name: "infrastructure-operator", Namespace: backplaneConfig.Spec.TargetNamespace}

	// Renders all templates from charts
	templates, errs := renderer.RenderChart(toggle.AssistedServiceChartDir, backplaneConfig, r.Images)
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
			log.Error(err, fmt.Sprintf("Failed to delete template: %s", template.GetName()))
			return result, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureServerFoundation(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	namespacedName := types.NamespacedName{Name: "ocm-controller", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))
	namespacedName = types.NamespacedName{Name: "ocm-proxyserver", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))
	namespacedName = types.NamespacedName{Name: "ocm-webhook", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))

	log := log.FromContext(ctx)

	templates, errs := renderer.RenderChart(toggle.ServerFoundationChartDir, backplaneConfig, r.Images)
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

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoServerFoundation(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Renders all templates from charts
	templates, errs := renderer.RenderChart(toggle.ServerFoundationChartDir, backplaneConfig, r.Images)
	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	namespacedName := types.NamespacedName{Name: "ocm-controller", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))
	r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	namespacedName = types.NamespacedName{Name: "ocm-proxyserver", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))
	r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	namespacedName = types.NamespacedName{Name: "ocm-webhook", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))
	r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))

	// Deletes all templates
	for _, template := range templates {
		result, err := r.deleteTemplate(ctx, backplaneConfig, template)
		if err != nil {
			log.Error(err, fmt.Sprintf("Failed to delete template: %s", template.GetName()))
			return result, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureClusterLifecycle(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	namespacedName := types.NamespacedName{Name: "cluster-curator-controller", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))
	namespacedName = types.NamespacedName{Name: "clusterclaims-controller", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))
	namespacedName = types.NamespacedName{Name: "provider-credential-controller", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))
	namespacedName = types.NamespacedName{Name: "clusterlifecycle-state-metrics-v2", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))

	log := log.FromContext(ctx)

	templates, errs := renderer.RenderChart(toggle.ClusterLifecycleChartDir, backplaneConfig, r.Images)
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

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoClusterLifecycle(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Renders all templates from charts
	templates, errs := renderer.RenderChart(toggle.ClusterLifecycleChartDir, backplaneConfig, r.Images)
	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	namespacedName := types.NamespacedName{Name: "cluster-curator-controller", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))
	r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	namespacedName = types.NamespacedName{Name: "clusterclaims-controller", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))
	r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	namespacedName = types.NamespacedName{Name: "provider-credential-controller", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))
	r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))

	// Deletes all templates
	for _, template := range templates {
		result, err := r.deleteTemplate(ctx, backplaneConfig, template)
		if err != nil {
			log.Error(err, fmt.Sprintf("Failed to delete template: %s", template.GetName()))
			return result, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureClusterManager(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	namespacedName := types.NamespacedName{Name: "cluster-manager", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))
	r.StatusManager.AddComponent(status.ClusterManagerStatus{
		NamespacedName: types.NamespacedName{Name: "cluster-manager"},
	})

	log := log.FromContext(ctx)

	templates, errs := renderer.RenderChart(toggle.ClusterManagerChartDir, backplaneConfig, r.Images)
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

	// Apply clustermanager
	cmTemplate := foundation.ClusterManager(backplaneConfig, r.Images)
	if err := ctrl.SetControllerReference(backplaneConfig, cmTemplate, r.Scheme); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Error setting controller reference on resource %s", cmTemplate.GetName())
	}
	force := true
	err := r.Client.Patch(ctx, cmTemplate, client.Apply, &client.PatchOptions{Force: &force, FieldManager: "backplane-operator"})
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error applying object Name: %s Kind: %s", cmTemplate.GetName(), cmTemplate.GetKind())
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoClusterManager(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	namespacedName := types.NamespacedName{Name: "cluster-manager", Namespace: backplaneConfig.Spec.TargetNamespace}

	// Renders all templates from charts
	templates, errs := renderer.RenderChart(toggle.ClusterManagerChartDir, backplaneConfig, r.Images)
	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))
	r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.RemoveComponent(status.ClusterManagerStatus{
		NamespacedName: types.NamespacedName{Name: "cluster-manager"},
	})

	// Delete clustermanager
	clusterManager := &unstructured.Unstructured{}
	clusterManager.SetGroupVersionKind(
		schema.GroupVersionKind{
			Group:   "operator.open-cluster-management.io",
			Version: "v1",
			Kind:    "ClusterManager",
		},
	)
	err := r.Client.Get(ctx, types.NamespacedName{Name: "cluster-manager"}, clusterManager)
	if err == nil { // If resource exists, delete
		err := r.Client.Delete(ctx, clusterManager)
		if err != nil {
			return ctrl.Result{RequeueAfter: requeuePeriod}, err
		}
	} else if err != nil && !apierrors.IsNotFound(err) { // Return error, if error is not not found error
		return ctrl.Result{RequeueAfter: requeuePeriod}, err
	}

	// Verify clustermanager namespace deleted
	ocmHubNamespace := &corev1.Namespace{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: "open-cluster-management-hub"}, ocmHubNamespace)
	if err == nil {
		return ctrl.Result{RequeueAfter: requeuePeriod}, fmt.Errorf("waiting for 'open-cluster-management-hub' namespace to be terminated before proceeding with clustermanager cleanup")
	}
	if err != nil && !apierrors.IsNotFound(err) { // Return error, if error is not not found error
		return ctrl.Result{RequeueAfter: requeuePeriod}, err
	}

	// Deletes all templates
	for _, template := range templates {
		result, err := r.deleteTemplate(ctx, backplaneConfig, template)
		if err != nil {
			log.Error(err, fmt.Sprintf("Failed to delete template: %s", template.GetName()))
			return result, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureHyperShift(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	namespacedName := types.NamespacedName{Name: "hypershift-addon-manager", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))
	namespacedName = types.NamespacedName{Name: "hypershift-deployment-controller", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))

	log := log.FromContext(ctx)

	templates, errs := renderer.RenderChart(toggle.HyperShiftChartDir, backplaneConfig, r.Images)
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

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoHyperShift(ctx context.Context, backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	namespacedName := types.NamespacedName{Name: "hypershift-addon-manager", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))
	r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	namespacedName = types.NamespacedName{Name: "hypershift-deployment-controller", Namespace: backplaneConfig.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))
	r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	// Renders all templates from charts
	templates, errs := renderer.RenderChart(toggle.HyperShiftChartDir, backplaneConfig, r.Images)
	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Deletes all templates
	for _, template := range templates {
		result, err := r.deleteTemplate(ctx, backplaneConfig, template)
		if err != nil {
			log.Error(err, fmt.Sprintf("Failed to delete template: %s", template.GetName()))
			return result, err
		}
	}
	return ctrl.Result{}, nil
}
