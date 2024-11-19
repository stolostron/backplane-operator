package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	semver "github.com/Masterminds/semver"
	configv1 "github.com/openshift/api/config/v1"
	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/foundation"
	"github.com/stolostron/backplane-operator/pkg/hive"
	renderer "github.com/stolostron/backplane-operator/pkg/rendering"
	"github.com/stolostron/backplane-operator/pkg/status"
	"github.com/stolostron/backplane-operator/pkg/toggle"
	"github.com/stolostron/backplane-operator/pkg/utils"
	"github.com/stolostron/backplane-operator/pkg/version"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1 "github.com/openshift/api/operator/v1"

	"github.com/pkg/errors"
)

const (
	localCluster = "local-cluster"
)

var clusterManagementAddOnGVK = schema.GroupVersionKind{
	Group:   "addon.open-cluster-management.io",
	Version: "v1alpha1",
	Kind:    "ClusterManagementAddOn",
}

func (r *MultiClusterEngineReconciler) ensureConsoleMCE(ctx context.Context, mce *backplanev1.MultiClusterEngine) (
	ctrl.Result, error) {

	namespacedName := types.NamespacedName{Name: "console-mce-console", Namespace: mce.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))

	// Ensure that the InternalHubComponent CR instance is created for component in MCE.
	if result, err := r.ensureInternalEngineComponent(ctx, mce, backplanev1.ConsoleMCE); err != nil {
		return result, err
	}

	// Renders all templates from charts
	chartPath := r.fetchChartOrCRDPath(backplanev1.ConsoleMCE, false)
	templates, errs := renderer.RenderChart(chartPath, mce, r.CacheSpec.ImageOverrides,
		r.CacheSpec.TemplateOverrides)

	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Apply deployment config overrides
	if result, err := r.applyComponentDeploymentOverrides(mce, templates, backplanev1.ConsoleMCE); err != nil {
		return result, err
	}

	// Applies all templates
	for _, template := range templates {
		applyReleaseVersionAnnotation(template)
		result, err := r.applyTemplate(ctx, mce, template)
		if err != nil {
			return result, err
		}
	}

	// Check console-mce deployment health before adding plugin
	consoleDeployment := &appsv1.Deployment{}
	err := r.Client.Get(ctx, namespacedName, consoleDeployment)
	if err != nil {
		log.Error(err, "Failed to get console-mce deployment for addon. Requeuing.")
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	for _, dc := range consoleDeployment.Status.Conditions {
		if dc.Type == appsv1.DeploymentAvailable && dc.Status == corev1.ConditionTrue {
			return r.addPluginToConsoleResource(ctx)
		}
	}

	log.Info("MCE console is not yet available. Waiting to enable console plugin")
	return ctrl.Result{RequeueAfter: requeuePeriod}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoConsoleMCE(ctx context.Context, mce *backplanev1.MultiClusterEngine,
	ocpConsole bool) (ctrl.Result, error) {

	namespacedName := types.NamespacedName{Name: "console-mce-console", Namespace: mce.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))

	// Ensure that the InternalHubComponent CR instance is deleted for component in MCE.
	if result, err := r.ensureNoInternalEngineComponent(ctx, mce,
		backplanev1.ConsoleMCE); (result != ctrl.Result{}) || err != nil {
		return result, err
	}

	if !ocpConsole {
		// If Openshift console is disabled then no cleanup to be done, because MCE console cannot be installed
		r.StatusManager.AddComponent(status.ConsoleUnavailableStatus{
			NamespacedName: types.NamespacedName{Name: "console-mce-console", Namespace: mce.Spec.TargetNamespace},
		})
		return ctrl.Result{}, nil
	}
	r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))

	result, err := r.removePluginFromConsoleResource(ctx)
	if err != nil {
		return result, err
	}

	// Renders all templates from charts
	chartPath := r.fetchChartOrCRDPath(backplanev1.ConsoleMCE, false)
	templates, errs := renderer.RenderChart(chartPath, mce, r.CacheSpec.ImageOverrides, r.CacheSpec.TemplateOverrides)

	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Deletes all templates
	for _, template := range templates {
		result, err := r.deleteTemplate(ctx, mce, template)
		if err != nil {
			log.Error(err, fmt.Sprintf("Failed to delete Console MCE template: %s || %s || %s", template.GetName(),
				template.GetAPIVersion(), template.GetNamespace()))

			return result, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureManagedServiceAccount(ctx context.Context,
	mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {

	r.StatusManager.RemoveComponent(toggle.DisabledStatus(types.NamespacedName{Name: "managedservice",
		Namespace: mce.Spec.TargetNamespace}, []*unstructured.Unstructured{}))

	// from 2.9, we change the managed-serviceaccount to a template type addon, so the agent will be managed by the
	// global addon manager, no need to add the managed-serviceaccount-addon-manager deployment as a component here
	r.StatusManager.AddComponent(status.NewPresentStatus(types.NamespacedName{Name: "managed-serviceaccount"},
		clusterManagementAddOnGVK))

	// Ensure that the InternalHubComponent CR instance is created for component in MCE.
	if result, err := r.ensureInternalEngineComponent(ctx, mce, backplanev1.ManagedServiceAccount); err != nil {
		return result, err
	}

	// Render CRD templates
	crdPath := r.fetchChartOrCRDPath(backplanev1.ManagedServiceAccount, true)
	crds, errs := renderer.RenderCRDs(crdPath, mce)
	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Apply all CRDs
	for _, crd := range crds {
		result, err := r.applyTemplate(ctx, mce, crd)
		if err != nil {
			return result, err
		}
	}

	// Renders all templates from charts
	chartPath := r.fetchChartOrCRDPath(backplanev1.ManagedServiceAccount, false)
	templates, errs := renderer.RenderChart(chartPath, mce, r.CacheSpec.ImageOverrides, r.CacheSpec.TemplateOverrides)

	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Apply deployment config overrides
	if result, err := r.applyComponentDeploymentOverrides(mce, templates, backplanev1.ManagedServiceAccount); err != nil {
		return result, err
	}

	// Applies all templates
	missingCRDErrorOccured := false
	for _, template := range templates {
		applyReleaseVersionAnnotation(template)
		result, err := r.applyTemplate(ctx, mce, template)
		if err != nil {
			if apimeta.IsNoMatchError(errors.Unwrap(err)) || apierrors.IsNotFound(err) {
				// addon CRD does not yet exist. Replace status.
				log.Info("Couldn't apply template for managed-serviceaccount due to missing CRD", "error is", err.Error())

				missingCRDErrorOccured = true
				r.StatusManager.AddComponent(clusterManagementAddOnNotFoundStatus("managed-serviceaccount",
					mce.Spec.TargetNamespace))
			} else {
				return result, err
			}
		}
	}

	if missingCRDErrorOccured {
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoManagedServiceAccount(ctx context.Context,
	mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {

	// Ensure that the InternalHubComponent CR instance is deleted for component in MCE.
	if result, err := r.ensureNoInternalEngineComponent(ctx, mce,
		backplanev1.ManagedServiceAccount); (result != ctrl.Result{}) || err != nil {
		return result, err
	}

	// Renders all templates from charts
	chartPath := r.fetchChartOrCRDPath(backplanev1.ManagedServiceAccount, false)
	templates, errs := renderer.RenderChart(chartPath, mce, r.CacheSpec.ImageOverrides, r.CacheSpec.TemplateOverrides)
	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	r.StatusManager.AddComponent(toggle.DisabledStatus(types.NamespacedName{Name: "managedservice",
		Namespace: mce.Spec.TargetNamespace}, []*unstructured.Unstructured{}))

	// Deletes all templates
	for _, template := range templates {
		if template.GetKind() == foundation.ClusterManagementAddonKind && !foundation.CanInstallAddons(ctx, r.Client) {
			// Can't delete ClusterManagementAddon if Kind doesn't exists
			continue
		}
		result, err := r.deleteTemplate(ctx, mce, template)
		if err != nil {
			log.Error(err, "Failed to delete MSA template")
			return result, err
		}
	}

	// Render CRD templates
	crdPath := r.fetchChartOrCRDPath(backplanev1.ManagedServiceAccount, true)
	crds, errs := renderer.RenderCRDs(crdPath, mce)
	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Delete all CRDs
	for _, crd := range crds {
		result, err := r.deleteTemplate(ctx, mce, crd)
		if err != nil {
			log.Error(err, "Failed to delete CRD")
			return result, err
		}
	}
	return ctrl.Result{}, nil
}

// addPluginToConsoleResource ...
func (r *MultiClusterEngineReconciler) addPluginToConsoleResource(ctx context.Context) (ctrl.Result, error) {
	console := &operatorv1.Console{}

	// If trying to check this resource from the CLI run - `oc get consoles.operator.openshift.io cluster`.
	// The default `console` is not the correct resource
	err := r.Client.Get(ctx, types.NamespacedName{Name: "cluster"}, console)
	if err != nil {
		log.Info("Failed to find console: cluster")
		return ctrl.Result{}, err
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
			return ctrl.Result{}, err
		} else {
			log.Info("Added mce consoleplugin to console")
		}
	}

	return ctrl.Result{}, nil
}

// removePluginFromConsoleResource ...
func (r *MultiClusterEngineReconciler) removePluginFromConsoleResource(ctx context.Context) (ctrl.Result, error) {
	if val, ok := os.LookupEnv("UNIT_TEST"); ok && val == "true" {
		return ctrl.Result{}, nil
	}

	console := &operatorv1.Console{}
	// If trying to check this resource from the CLI run - `oc get consoles.operator.openshift.io cluster`.
	// The default `console` is not the correct resource
	err := r.Client.Get(ctx, types.NamespacedName{Name: "cluster"}, console)
	if err != nil {
		log.Info("Failed to find console: cluster")
		return ctrl.Result{}, err
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
			return ctrl.Result{}, err
		} else {
			log.Info("Removed mce consoleplugin to console")
		}
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureDiscovery(ctx context.Context, mce *backplanev1.MultiClusterEngine) (
	ctrl.Result, error) {

	namespacedName := types.NamespacedName{Name: "discovery-operator", Namespace: mce.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))

	// Ensure that the InternalHubComponent CR instance is created for component in MCE.
	if result, err := r.ensureInternalEngineComponent(ctx, mce, backplanev1.Discovery); err != nil {
		return result, err
	}

	// Renders all templates from charts
	chartPath := r.fetchChartOrCRDPath(backplanev1.Discovery, false)
	templates, errs := renderer.RenderChart(chartPath, mce, r.CacheSpec.ImageOverrides, r.CacheSpec.TemplateOverrides)

	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Apply deployment config overrides
	if result, err := r.applyComponentDeploymentOverrides(mce, templates, backplanev1.Discovery); err != nil {
		return result, err
	}

	// Applies all templates
	for _, template := range templates {
		applyReleaseVersionAnnotation(template)
		result, err := r.applyTemplate(ctx, mce, template)
		if err != nil {
			return result, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoDiscovery(ctx context.Context,
	mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {

	namespacedName := types.NamespacedName{Name: "discovery-operator", Namespace: mce.Spec.TargetNamespace}

	// Ensure that the InternalHubComponent CR instance is deleted for component in MCE.
	if result, err := r.ensureNoInternalEngineComponent(ctx, mce,
		backplanev1.Discovery); (result != ctrl.Result{}) || err != nil {
		return result, err
	}

	// Renders all templates from charts
	chartPath := r.fetchChartOrCRDPath(backplanev1.Discovery, false)
	templates, errs := renderer.RenderChart(chartPath, mce, r.CacheSpec.ImageOverrides, r.CacheSpec.TemplateOverrides)

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
		result, err := r.deleteTemplate(ctx, mce, template)
		if err != nil {
			logTemplateDeletionError(err, template.GetName())
			return result, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureHive(ctx context.Context, mce *backplanev1.MultiClusterEngine) (
	ctrl.Result, error) {

	namespacedName := types.NamespacedName{Name: "hive-operator", Namespace: mce.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))

	// Ensure that the InternalHubComponent CR instance is created for component in MCE.
	if result, err := r.ensureInternalEngineComponent(ctx, mce, backplanev1.Hive); err != nil {
		return result, err
	}

	// Renders all templates from charts
	chartPath := r.fetchChartOrCRDPath(backplanev1.Hive, false)
	templates, errs := renderer.RenderChart(chartPath, mce, r.CacheSpec.ImageOverrides, r.CacheSpec.TemplateOverrides)

	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Apply deployment config overrides
	if result, err := r.applyComponentDeploymentOverrides(mce, templates, backplanev1.Hive); err != nil {
		return result, err
	}

	// Applies all templates
	for _, template := range templates {
		applyReleaseVersionAnnotation(template)
		result, err := r.applyTemplate(ctx, mce, template)
		if err != nil {
			return result, err
		}
	}

	hiveTemplate := hive.HiveConfig(mce)
	if err := ctrl.SetControllerReference(mce, hiveTemplate, r.Scheme); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Error setting controller reference on resource %s", hiveTemplate.GetName())
	}

	result, err := r.ensureUnstructuredResource(ctx, mce, hiveTemplate)
	if err != nil {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoHive(ctx context.Context, mce *backplanev1.MultiClusterEngine) (
	ctrl.Result, error) {

	namespacedName := types.NamespacedName{Name: "hive-operator", Namespace: mce.Spec.TargetNamespace}

	// Ensure that the InternalHubComponent CR instance is deleted for component in MCE.
	if result, err := r.ensureNoInternalEngineComponent(ctx, mce,
		backplanev1.Hive); (result != ctrl.Result{}) || err != nil {
		return result, err
	}

	// Renders all templates from charts
	chartPath := r.fetchChartOrCRDPath(backplanev1.Hive, false)
	templates, errs := renderer.RenderChart(chartPath, mce, r.CacheSpec.ImageOverrides, r.CacheSpec.TemplateOverrides)

	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))
	r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))

	// Delete hivconfig
	hiveConfig := hive.HiveConfig(mce)
	err := r.Client.Get(ctx, types.NamespacedName{Name: "hive"}, hiveConfig)
	if err == nil { // If resource exists, delete
		err := r.Client.Delete(ctx, hiveConfig)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Deletes all templates
	for _, template := range templates {
		result, err := r.deleteTemplate(ctx, mce, template)
		if err != nil {
			logTemplateDeletionError(err, template.GetName())
			return result, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureAssistedService(ctx context.Context,
	mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {

	targetNamespace := mce.Spec.TargetNamespace
	if mce.Spec.Overrides != nil && mce.Spec.Overrides.InfrastructureCustomNamespace != "" {
		targetNamespace = mce.Spec.Overrides.InfrastructureCustomNamespace
	}

	namespacedName := types.NamespacedName{Name: "infrastructure-operator", Namespace: targetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))

	// Ensure that the InternalHubComponent CR instance is created for component in MCE.
	if result, err := r.ensureInternalEngineComponent(ctx, mce, backplanev1.AssistedService); err != nil {
		return result, err
	}

	// Renders all templates from charts
	chartPath := r.fetchChartOrCRDPath(backplanev1.AssistedService, false)
	templates, errs := renderer.RenderChartWithNamespace(chartPath, mce,
		r.CacheSpec.ImageOverrides, r.CacheSpec.TemplateOverrides, targetNamespace)

	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Apply deployment config overrides
	if result, err := r.applyComponentDeploymentOverrides(mce, templates, backplanev1.AssistedService); err != nil {
		return result, err
	}

	// Applies all templates
	for _, template := range templates {
		applyReleaseVersionAnnotation(template)
		result, err := r.applyTemplate(ctx, mce, template)
		if err != nil {
			return result, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoAssistedService(ctx context.Context,
	mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {

	targetNamespace := mce.Spec.TargetNamespace
	if mce.Spec.Overrides != nil && mce.Spec.Overrides.InfrastructureCustomNamespace != "" {
		targetNamespace = mce.Spec.Overrides.InfrastructureCustomNamespace
	}
	namespacedName := types.NamespacedName{Name: "infrastructure-operator", Namespace: targetNamespace}

	// Ensure that the InternalHubComponent CR instance is deleted for component in MCE.
	if result, err := r.ensureNoInternalEngineComponent(ctx, mce,
		backplanev1.AssistedService); (result != ctrl.Result{}) || err != nil {
		return result, err
	}

	// Renders all templates from charts
	chartPath := r.fetchChartOrCRDPath(backplanev1.AssistedService, false)
	templates, errs := renderer.RenderChartWithNamespace(chartPath, mce, r.CacheSpec.ImageOverrides,
		r.CacheSpec.TemplateOverrides, targetNamespace)

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
		result, err := r.deleteTemplate(ctx, mce, template)
		if err != nil {
			logTemplateDeletionError(err, template.GetName())
			return result, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureServerFoundation(ctx context.Context,
	mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {

	namespacedName := types.NamespacedName{Name: "ocm-controller", Namespace: mce.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))
	if utils.DeployOnOCP() {
		namespacedName = types.NamespacedName{Name: "ocm-proxyserver", Namespace: mce.Spec.TargetNamespace}
		r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
		r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))
	}

	namespacedName = types.NamespacedName{Name: "ocm-webhook", Namespace: mce.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))

	// Ensure that the InternalHubComponent CR instance is created for component in MCE.
	if result, err := r.ensureInternalEngineComponent(ctx, mce, backplanev1.ServerFoundation); err != nil {
		return result, err
	}

	// Renders all templates from charts
	chartPath := r.fetchChartOrCRDPath(backplanev1.ServerFoundation, false)
	templates, errs := renderer.RenderChart(chartPath, mce, r.CacheSpec.ImageOverrides, r.CacheSpec.TemplateOverrides)

	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Apply deployment config overrides
	if result, err := r.applyComponentDeploymentOverrides(mce, templates, backplanev1.ServerFoundation); err != nil {
		return result, err
	}

	// Applies all templates
	for _, template := range templates {
		applyReleaseVersionAnnotation(template)
		result, err := r.applyTemplate(ctx, mce, template)
		if err != nil {
			return result, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoServerFoundation(ctx context.Context,
	mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {

	// Ensure that the InternalHubComponent CR instance is created for component in MCE.
	if result, err := r.ensureNoInternalEngineComponent(ctx, mce,
		backplanev1.ServerFoundation); (result != ctrl.Result{}) || err != nil {
		return result, err
	}

	// Renders all templates from charts
	chartPath := r.fetchChartOrCRDPath(backplanev1.ServerFoundation, false)
	templates, errs := renderer.RenderChart(chartPath, mce, r.CacheSpec.ImageOverrides, r.CacheSpec.TemplateOverrides)

	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	namespacedName := types.NamespacedName{Name: "ocm-controller", Namespace: mce.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))
	r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))

	if utils.DeployOnOCP() {
		namespacedName = types.NamespacedName{Name: "ocm-proxyserver", Namespace: mce.Spec.TargetNamespace}
		r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))
		r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	}

	namespacedName = types.NamespacedName{Name: "ocm-webhook", Namespace: mce.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))
	r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))

	// Deletes all templates
	for _, template := range templates {
		result, err := r.deleteTemplate(ctx, mce, template)
		if err != nil {
			logTemplateDeletionError(err, template.GetName())
			return result, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureImageBasedInstallOperator(ctx context.Context,
	mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {

	namespacedName := types.NamespacedName{Name: "image-based-install-operator", Namespace: mce.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))

	// Ensure that the InternalHubComponent CR instance is created for component in MCE.
	if result, err := r.ensureInternalEngineComponent(ctx, mce, backplanev1.ImageBasedInstallOperator); err != nil {
		return result, err
	}

	// Renders all templates from charts
	chartPath := r.fetchChartOrCRDPath(backplanev1.ImageBasedInstallOperator, false)
	templates, errs := renderer.RenderChart(chartPath, mce, r.CacheSpec.ImageOverrides, r.CacheSpec.TemplateOverrides)

	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Apply deployment config overrides
	if result, err := r.applyComponentDeploymentOverrides(mce, templates,
		backplanev1.ImageBasedInstallOperator); err != nil {
		return result, err
	}

	// Applies all templates
	for _, template := range templates {
		applyReleaseVersionAnnotation(template)
		result, err := r.applyTemplate(ctx, mce, template)
		if err != nil {
			return result, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoImageBasedInstallOperator(ctx context.Context,
	mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {

	targetNamespace := mce.Spec.TargetNamespace
	namespacedName := types.NamespacedName{Name: "image-based-install-operator", Namespace: targetNamespace}

	// Ensure that the InternalHubComponent CR instance is deleted for component in MCE.
	if result, err := r.ensureNoInternalEngineComponent(ctx, mce,
		backplanev1.ImageBasedInstallOperator); (result != ctrl.Result{}) || err != nil {
		return result, err
	}

	// Renders all templates from charts
	chartPath := r.fetchChartOrCRDPath(backplanev1.ImageBasedInstallOperator, false)
	templates, errs := renderer.RenderChart(chartPath, mce, r.CacheSpec.ImageOverrides, r.CacheSpec.TemplateOverrides)

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
		result, err := r.deleteTemplate(ctx, mce, template)
		if err != nil {
			logTemplateDeletionError(err, template.GetName())
			return result, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureClusterLifecycle(ctx context.Context,
	mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {

	if utils.DeployOnOCP() {
		namespacedName := types.NamespacedName{Name: "cluster-curator-controller", Namespace: mce.Spec.TargetNamespace}
		r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
		r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))
		namespacedName = types.NamespacedName{Name: "clusterclaims-controller", Namespace: mce.Spec.TargetNamespace}
		r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
		r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))
		namespacedName = types.NamespacedName{Name: "provider-credential-controller", Namespace: mce.Spec.TargetNamespace}
		r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
		r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))
		namespacedName = types.NamespacedName{Name: "cluster-image-set-controller", Namespace: mce.Spec.TargetNamespace}
		r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
		r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))
	}
	namespacedName := types.NamespacedName{Name: "clusterlifecycle-state-metrics-v2", Namespace: mce.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))

	// Ensure that the InternalHubComponent CR instance is created for component in MCE.
	if result, err := r.ensureInternalEngineComponent(ctx, mce, backplanev1.ClusterLifecycle); err != nil {
		return result, err
	}

	// Renders all templates from charts
	chartPath := r.fetchChartOrCRDPath(backplanev1.ClusterLifecycle, false)
	templates, errs := renderer.RenderChart(chartPath, mce, r.CacheSpec.ImageOverrides,
		r.CacheSpec.TemplateOverrides)

	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Apply deployment config overrides
	if result, err := r.applyComponentDeploymentOverrides(mce, templates, backplanev1.ClusterLifecycle); err != nil {
		return result, err
	}

	// Applies all templates
	for _, template := range templates {
		applyReleaseVersionAnnotation(template)
		result, err := r.applyTemplate(ctx, mce, template)
		if err != nil {
			return result, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoClusterLifecycle(ctx context.Context,
	mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {

	// Ensure that the InternalHubComponent CR instance is deleted for component in MCE.
	if result, err := r.ensureNoInternalEngineComponent(ctx, mce,
		backplanev1.ClusterLifecycle); (result != ctrl.Result{}) || err != nil {
		return result, err
	}

	// Renders all templates from charts
	chartPath := r.fetchChartOrCRDPath(backplanev1.ClusterLifecycle, false)
	templates, errs := renderer.RenderChart(chartPath, mce, r.CacheSpec.ImageOverrides, r.CacheSpec.TemplateOverrides)

	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	if utils.DeployOnOCP() {
		namespacedName := types.NamespacedName{Name: "cluster-curator-controller", Namespace: mce.Spec.TargetNamespace}
		r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))
		r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
		namespacedName = types.NamespacedName{Name: "clusterclaims-controller", Namespace: mce.Spec.TargetNamespace}
		r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))
		r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
		namespacedName = types.NamespacedName{Name: "provider-credential-controller", Namespace: mce.Spec.TargetNamespace}
		r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))
		r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
		namespacedName = types.NamespacedName{Name: "cluster-image-set-controller", Namespace: mce.Spec.TargetNamespace}
		r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))
		r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	}

	// Deletes all templates
	for _, template := range templates {
		result, err := r.deleteTemplate(ctx, mce, template)
		if err != nil {
			logTemplateDeletionError(err, template.GetName())
			return result, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureClusterManager(ctx context.Context,
	mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {

	namespacedName := types.NamespacedName{Name: "cluster-manager", Namespace: mce.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))
	r.StatusManager.AddComponent(status.ClusterManagerStatus{
		NamespacedName: types.NamespacedName{Name: "cluster-manager"},
	})

	// Ensure that the InternalHubComponent CR instance is created for component in MCE.
	if result, err := r.ensureInternalEngineComponent(ctx, mce, backplanev1.ClusterManager); err != nil {
		return result, err
	}

	// Renders all templates from charts
	chartPath := r.fetchChartOrCRDPath(backplanev1.ClusterManager, false)
	templates, errs := renderer.RenderChart(chartPath, mce, r.CacheSpec.ImageOverrides, r.CacheSpec.TemplateOverrides)

	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Apply deployment config overrides
	if result, err := r.applyComponentDeploymentOverrides(mce, templates, backplanev1.ClusterManager); err != nil {
		return result, err
	}

	// Applies all templates
	for _, template := range templates {
		applyReleaseVersionAnnotation(template)
		result, err := r.applyTemplate(ctx, mce, template)
		if err != nil {
			return result, err
		}
	}

	// Apply clustermanager
	cmTemplate := foundation.ClusterManager(mce, r.CacheSpec.ImageOverrides)
	if err := ctrl.SetControllerReference(mce, cmTemplate, r.Scheme); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Error setting controller reference on resource %s", cmTemplate.GetName())
	}
	force := true
	err := r.Client.Patch(ctx, cmTemplate, client.Apply, &client.PatchOptions{Force: &force, FieldManager: controlPlane})
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error applying object Name: %s Kind: %s", cmTemplate.GetName(), cmTemplate.GetKind())
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoClusterManager(ctx context.Context,
	mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	namespacedName := types.NamespacedName{Name: "cluster-manager", Namespace: mce.Spec.TargetNamespace}

	// Ensure that the InternalHubComponent CR instance is deleted for component in MCE.
	if result, err := r.ensureNoInternalEngineComponent(ctx, mce,
		backplanev1.ClusterManager); (result != ctrl.Result{}) || err != nil {
		return result, err
	}

	// Renders all templates from charts
	chartPath := r.fetchChartOrCRDPath(backplanev1.ClusterManager, false)
	templates, errs := renderer.RenderChart(chartPath, mce, r.CacheSpec.ImageOverrides, r.CacheSpec.TemplateOverrides)

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
			return ctrl.Result{}, err
		}
	} else if err != nil && !apierrors.IsNotFound(err) { // Return error, if error is not not found error
		return ctrl.Result{}, err
	}

	// Verify clustermanager namespace deleted
	ocmHubNamespace := &corev1.Namespace{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: "open-cluster-management-hub"}, ocmHubNamespace)
	if err == nil {
		return ctrl.Result{}, fmt.Errorf("waiting for 'open-cluster-management-hub' namespace to be terminated before proceeding with clustermanager cleanup")

	} else if !apierrors.IsNotFound(err) { // Return error, if error is not not found error
		return ctrl.Result{}, err
	}

	// Deletes all templates
	for _, template := range templates {
		result, err := r.deleteTemplate(ctx, mce, template)
		if err != nil {
			logTemplateDeletionError(err, template.GetName())
			return result, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureHyperShift(ctx context.Context, mce *backplanev1.MultiClusterEngine) (
	ctrl.Result, error) {

	namespacedName := types.NamespacedName{Name: "hypershift-addon-manager", Namespace: mce.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))
	r.StatusManager.AddComponent(status.NewPresentStatus(types.NamespacedName{Name: "hypershift-addon"}, clusterManagementAddOnGVK))

	// Ensure that the InternalHubComponent CR instance is created for component in MCE.
	if result, err := r.ensureInternalEngineComponent(ctx, mce, backplanev1.HyperShift); err != nil {
		return result, err
	}

	// Renders all templates from charts
	chartPath := r.fetchChartOrCRDPath(backplanev1.HyperShift, false)
	templates, errs := renderer.RenderChart(chartPath, mce, r.CacheSpec.ImageOverrides, r.CacheSpec.TemplateOverrides)

	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Apply deployment config overrides
	if result, err := r.applyComponentDeploymentOverrides(mce, templates, backplanev1.HyperShift); err != nil {
		return result, err
	}

	// Applies all templates
	missingCRDErrorOccured := false
	for _, template := range templates {
		applyReleaseVersionAnnotation(template)
		result, err := r.applyTemplate(ctx, mce, template)
		if err != nil {
			if apimeta.IsNoMatchError(errors.Unwrap(err)) || apierrors.IsNotFound(err) {
				// addon CRD does not yet exist. Replace status.
				log.Info("Couldn't apply template for hypershift due to missing CRD", "error is", err.Error())

				missingCRDErrorOccured = true
				r.StatusManager.AddComponent(clusterManagementAddOnNotFoundStatus("hypershift", mce.Spec.TargetNamespace))
			} else {
				return result, err
			}
		}
	}

	if missingCRDErrorOccured {
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoHyperShift(ctx context.Context,
	mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {

	// Ensure that the InternalHubComponent CR instance is deleted for component in MCE.
	if result, err := r.ensureNoInternalEngineComponent(ctx, mce,
		backplanev1.HyperShift); (result != ctrl.Result{}) || err != nil {
		return result, err
	}

	namespacedName := types.NamespacedName{Name: "hypershift-addon-manager", Namespace: mce.Spec.TargetNamespace}

	// Ensure hypershift-addon is removed first
	waitingForHypershiftAddon := status.StaticStatus{
		NamespacedName: namespacedName,
		Kind:           "Component",
		Condition: backplanev1.ComponentCondition{
			Type:      "Uninstalled",
			Name:      "hypershift",
			Status:    metav1.ConditionFalse,
			Reason:    status.WaitingForResourceReason,
			Kind:      "Component",
			Available: false,
			Message:   "Waiting for 'hypershift-addon' ManagedClusterAddOn to be removed",
		},
	}
	hypershiftAddon, err := renderer.RenderHypershiftAddon(mce)
	if err != nil {
		return ctrl.Result{}, err
	}
	err = r.Client.Get(ctx, types.NamespacedName{Name: hypershiftAddon.GetName(), Namespace: hypershiftAddon.GetNamespace()}, hypershiftAddon)
	if err != nil {
		if !(apierrors.IsNotFound(err) || apimeta.IsNoMatchError(err)) {
			// Unexpected error getting addon
			log.Error(err, "error while looking for hypershift-addon ManagedClusterAddOn")
			r.StatusManager.AddComponent(waitingForHypershiftAddon)
			return ctrl.Result{RequeueAfter: requeuePeriod}, nil
		}
	} else {
		// Resource still present
		r.StatusManager.AddComponent(waitingForHypershiftAddon)
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))

	// Renders all templates from charts
	chartPath := r.fetchChartOrCRDPath(backplanev1.HyperShift, false)
	templates, errs := renderer.RenderChart(chartPath, mce, r.CacheSpec.ImageOverrides, r.CacheSpec.TemplateOverrides)

	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Deletes all templates
	for _, template := range templates {
		result, err := r.deleteTemplate(ctx, mce, template)
		if err != nil {
			logTemplateDeletionError(err, template.GetName())
			return result, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) reconcileHypershiftLocalHosting(ctx context.Context, mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	addon, err := renderer.RenderHypershiftAddon(mce)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !mce.Enabled(backplanev1.HypershiftLocalHosting) {
		r.StatusManager.AddComponent(status.NewDisabledStatus(
			types.NamespacedName{Name: addon.GetName(), Namespace: addon.GetNamespace()},
			"Component is disabled",
			[]*unstructured.Unstructured{addon},
		))
		return r.removeHypershiftLocalHosting(ctx, mce)
	}

	if !mce.Enabled(backplanev1.HyperShift) {
		// report that hypershift must be enabled
		r.StatusManager.AddComponent(status.NewDisabledStatus(
			types.NamespacedName{Name: addon.GetName(), Namespace: addon.GetNamespace()},
			"Local hosting only available when hypershift is enabled",
			[]*unstructured.Unstructured{addon},
		))
		return r.removeHypershiftLocalHosting(ctx, mce)
	}

	if !mce.Enabled(backplanev1.LocalCluster) {
		// report that local-cluster must be enabled
		r.StatusManager.AddComponent(status.NewDisabledStatus(
			types.NamespacedName{Name: addon.GetName(), Namespace: addon.GetNamespace()},
			"Local hosting only available when local-cluster is enabled",
			[]*unstructured.Unstructured{addon},
		))
		return r.removeHypershiftLocalHosting(ctx, mce)
	}

	localNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: localCluster}}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: localNS.GetName()}, localNS)
	if apierrors.IsNotFound(err) {
		// wait for local-cluster namespace
		r.StatusManager.AddComponent(status.StaticStatus{
			NamespacedName: types.NamespacedName{Name: addon.GetName(), Namespace: addon.GetNamespace()},
			Kind:           addon.GetKind(),
			Condition: backplanev1.ComponentCondition{
				Type:      "Available",
				Name:      addon.GetName(),
				Status:    metav1.ConditionFalse,
				Reason:    status.WaitingForResourceReason,
				Kind:      addon.GetKind(),
				Available: false,
				Message:   "Waiting for namespace 'local-cluster'",
			},
		})
		log.Info("Can't apply hypershift-addon, waiting for local-cluster namespace")
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}
	r.StatusManager.AddComponent(status.ManagedClusterAddOnStatus{
		NamespacedName: types.NamespacedName{Name: addon.GetName(), Namespace: addon.GetNamespace()},
	})
	return r.applyHypershiftLocalHosting(ctx, mce)
}

func (r *MultiClusterEngineReconciler) applyHypershiftLocalHosting(ctx context.Context,
	mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	addon, err := renderer.RenderHypershiftAddon(mce)
	if err != nil {
		return ctrl.Result{}, err
	}
	applyReleaseVersionAnnotation(addon)
	result, err := r.applyTemplate(ctx, mce, addon)
	if err != nil {
		if apimeta.IsNoMatchError(errors.Unwrap(err)) || apierrors.IsNotFound(errors.Unwrap(err)) {
			// addon CRD does not yet exist. Replace status.
			log.Info("Couldn't apply template for hypershiftlocalhosting due to missing CRD", "error is", err.Error())

			r.StatusManager.RemoveComponent(status.ManagedClusterAddOnStatus{
				NamespacedName: types.NamespacedName{Name: addon.GetName(), Namespace: addon.GetNamespace()},
			})
			r.StatusManager.AddComponent(status.StaticStatus{
				NamespacedName: types.NamespacedName{Name: addon.GetName(), Namespace: addon.GetNamespace()},
				Kind:           addon.GetKind(),
				Condition: backplanev1.ComponentCondition{
					Type:      "Available",
					Name:      addon.GetName(),
					Status:    metav1.ConditionFalse,
					Reason:    status.WaitingForResourceReason,
					Kind:      addon.GetKind(),
					Available: false,
					Message:   "Waiting for ManagedClusterAddOn CRD to be available",
				},
			})
			return ctrl.Result{RequeueAfter: requeuePeriod}, nil
		}
		if apierrors.IsInternalError(errors.Unwrap(err)) {
			// likely failed to call webhook
			log.Info("Couldn't apply template for hypershiftlocalhosting likely due to webhook not ready", "error is",
				err.Error())

			r.StatusManager.RemoveComponent(status.ManagedClusterAddOnStatus{
				NamespacedName: types.NamespacedName{Name: addon.GetName(), Namespace: addon.GetNamespace()},
			})
			r.StatusManager.AddComponent(status.StaticStatus{
				NamespacedName: types.NamespacedName{Name: addon.GetName(), Namespace: addon.GetNamespace()},
				Kind:           addon.GetKind(),
				Condition: backplanev1.ComponentCondition{
					Type:      "Available",
					Name:      addon.GetName(),
					Status:    metav1.ConditionUnknown,
					Reason:    status.WaitingForResourceReason,
					Kind:      addon.GetKind(),
					Available: false,
					Message:   err.Error(),
				},
			})
			return ctrl.Result{RequeueAfter: requeuePeriod}, nil
		}
		return result, err
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) removeHypershiftLocalHosting(ctx context.Context,
	mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	addon, err := renderer.RenderHypershiftAddon(mce)
	if err != nil {
		return ctrl.Result{}, err
	}
	result, err := r.deleteTemplate(ctx, mce, addon)
	if err != nil {
		return result, err
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureClusterProxyAddon(ctx context.Context,
	mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {

	namespacedName := types.NamespacedName{Name: "cluster-proxy-addon-manager", Namespace: mce.Spec.TargetNamespace}
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	namespacedName = types.NamespacedName{Name: "cluster-proxy-addon-user", Namespace: mce.Spec.TargetNamespace}
	r.StatusManager.AddComponent(toggle.EnabledStatus(namespacedName))
	r.StatusManager.RemoveComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	r.StatusManager.AddComponent(status.NewPresentStatus(types.NamespacedName{Name: "cluster-proxy"}, clusterManagementAddOnGVK))

	// Ensure that the InternalHubComponent CR instance is created for component in MCE.
	if result, err := r.ensureInternalEngineComponent(ctx, mce, backplanev1.ClusterProxyAddon); err != nil {
		return result, err
	}

	// Renders all templates from charts
	chartPath := r.fetchChartOrCRDPath(backplanev1.ClusterProxyAddon, false)
	templates, errs := renderer.RenderChart(chartPath, mce, r.CacheSpec.ImageOverrides, r.CacheSpec.TemplateOverrides)

	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Apply deployment config overrides
	if result, err := r.applyComponentDeploymentOverrides(mce, templates, backplanev1.ClusterProxyAddon); err != nil {
		return result, err
	}

	// Applies all templates
	missingCRDErrorOccured := false
	for _, template := range templates {
		applyReleaseVersionAnnotation(template)
		result, err := r.applyTemplate(ctx, mce, template)
		if err != nil {
			if apimeta.IsNoMatchError(errors.Unwrap(err)) || apierrors.IsNotFound(errors.Unwrap(err)) {
				missingCRDErrorOccured = true
				r.StatusManager.AddComponent(clusterManagementAddOnNotFoundStatus("cluster-proxy-addon", mce.Spec.TargetNamespace))
			} else {
				return result, err
			}
		}
	}

	if missingCRDErrorOccured {
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoClusterProxyAddon(ctx context.Context,
	mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {

	namespacedName := types.NamespacedName{Name: "cluster-proxy-addon-manager", Namespace: mce.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))
	r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))
	namespacedName = types.NamespacedName{Name: "cluster-proxy-addon-user", Namespace: mce.Spec.TargetNamespace}
	r.StatusManager.RemoveComponent(toggle.EnabledStatus(namespacedName))
	r.StatusManager.AddComponent(toggle.DisabledStatus(namespacedName, []*unstructured.Unstructured{}))

	// Ensure that the InternalHubComponent CR instance is deleted for component in MCE.
	if result, err := r.ensureNoInternalEngineComponent(ctx, mce,
		backplanev1.ClusterProxyAddon); (result != ctrl.Result{}) || err != nil {
		return result, err
	}

	// Renders all templates from charts
	chartPath := r.fetchChartOrCRDPath(backplanev1.ClusterProxyAddon, false)
	templates, errs := renderer.RenderChart(chartPath, mce, r.CacheSpec.ImageOverrides, r.CacheSpec.TemplateOverrides)

	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Deletes all templates
	for _, template := range templates {
		result, err := r.deleteTemplate(ctx, mce, template)
		if err != nil {
			logTemplateDeletionError(err, template.GetName())
			return result, err
		}
	}
	return ctrl.Result{}, nil
}

// Checks if OCP Console is enabled and return true if so. If <OCP v4.12, always return true
// Otherwise check in the EnabledCapabilities spec for OCP console
func (r *MultiClusterEngineReconciler) CheckConsole(ctx context.Context) (bool, error) {
	versionStatus := &configv1.ClusterVersion{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: "version"}, versionStatus)
	if err != nil {
		return false, err
	}
	ocpVersion, err := r.getClusterVersion(ctx)
	if err != nil {
		return false, err
	}
	if hubOCPVersion, ok := os.LookupEnv("ACM_HUB_OCP_VERSION"); ok {
		ocpVersion = hubOCPVersion
	}
	semverVersion, err := semver.NewVersion(ocpVersion)
	if err != nil {
		return false, fmt.Errorf("failed to convert ocp version to semver compatible value: %w", err)
	}
	// -0 allows for prerelease builds to pass the validation.
	// If -0 is removed, developer/rc builds will not pass this check
	// OCP Console can only be disabled in OCP 4.12+
	constraint, err := semver.NewConstraint(">= 4.12.0-0")
	if err != nil {
		return false, fmt.Errorf("failed to set ocp version constraint: %w", err)
	}
	if !constraint.Check(semverVersion) {
		return true, nil
	}
	for _, v := range versionStatus.Status.Capabilities.EnabledCapabilities {
		if v == "Console" {
			return true, nil
		}
	}
	return false, nil
}

func (r *MultiClusterEngineReconciler) ensureLocalCluster(ctx context.Context, mce *backplanev1.MultiClusterEngine) (
	ctrl.Result, error) {

	if utils.IsUnitTest() {
		log.Info("skipping local cluster creation in unit tests")
		return ctrl.Result{}, nil
	}

	nsn := types.NamespacedName{Name: localCluster, Namespace: mce.Spec.TargetNamespace}
	lcs := status.LocalClusterStatus{NamespacedName: nsn, Enabled: true}
	r.StatusManager.RemoveComponent(lcs)
	r.StatusManager.AddComponent(lcs)

	log.Info("Check if ManagedCluster CR exists")
	managedCluster := utils.NewManagedCluster()
	err := r.Client.Get(ctx, types.NamespacedName{Name: utils.LocalClusterName}, managedCluster)
	if apierrors.IsNotFound(err) {
		log.Info("ManagedCluster CR does not exist, need to create it")
		log.Info(fmt.Sprintf("Check if local cluster namespace %q exists", utils.LocalClusterName))
		localNS := utils.NewLocalNamespace()
		err := r.Client.Get(ctx, types.NamespacedName{Name: localNS.GetName()}, localNS)
		if err == nil {
			log.Info("Waiting on local cluster namespace to be removed before creating ManagedCluster CR",
				"Namespace", localNS.GetName())

			return ctrl.Result{RequeueAfter: requeuePeriod}, nil
		} else if apierrors.IsNotFound(err) {
			log.Info("Local cluster namespace does not exist. Creating ManagedCluster CR")
			managedCluster = utils.NewManagedCluster()
			err := r.Client.Create(ctx, managedCluster)
			if err != nil {
				if apierrors.IsInternalError(err) {
					// webhook not available
					log.Info("ManagedCluster webhook not available, waiting for controller")
					r.StatusManager.RemoveComponent(lcs)
					r.StatusManager.AddComponent(status.StaticStatus{
						NamespacedName: types.NamespacedName{Name: localCluster, Namespace: mce.Spec.TargetNamespace},
						Kind:           localCluster,
						Condition: backplanev1.ComponentCondition{
							Type:      "Available",
							Name:      localCluster,
							Status:    metav1.ConditionFalse,
							Reason:    status.WaitingForResourceReason,
							Kind:      localCluster,
							Available: false,
							Message:   "Waiting for ManagedCluster webhook",
						},
					})
					return ctrl.Result{RequeueAfter: requeuePeriod}, nil
				} else if apimeta.IsNoMatchError(err) || apierrors.IsNotFound(err) {
					log.Info("ManagedCluster CRD not available while creating ManagedCluster CR")
					return ctrl.Result{RequeueAfter: requeuePeriod}, nil
				} else {
					log.Error(err, "Failed to create ManagedCluster CR")
					return ctrl.Result{}, err
				}
			}
			log.Info("Created ManagedCluster CR")
		} else {
			log.Error(err, "Failed to get local cluster namespace")
			return ctrl.Result{}, err
		}
	} else if apimeta.IsNoMatchError(err) {
		// managedCluster CRD does not yet exist. Replace status.
		r.StatusManager.RemoveComponent(lcs)
		r.StatusManager.AddComponent(status.StaticStatus{
			NamespacedName: types.NamespacedName{Name: localCluster, Namespace: mce.Spec.TargetNamespace},
			Kind:           localCluster,
			Condition: backplanev1.ComponentCondition{
				Type:      "Available",
				Name:      localCluster,
				Status:    metav1.ConditionFalse,
				Reason:    status.WaitingForResourceReason,
				Kind:      localCluster,
				Available: false,
				Message:   "Waiting for ManagedCluster CRD to be available",
			},
		})
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	} else if apierrors.IsInternalError(err) {
		// webhook not available
		log.Info("ManagedCluster webhook not available, waiting for controller")
		r.StatusManager.RemoveComponent(lcs)
		r.StatusManager.AddComponent(status.StaticStatus{
			NamespacedName: types.NamespacedName{Name: localCluster, Namespace: mce.Spec.TargetNamespace},
			Kind:           localCluster,
			Condition: backplanev1.ComponentCondition{
				Type:      "Available",
				Name:      localCluster,
				Status:    metav1.ConditionFalse,
				Reason:    status.WaitingForResourceReason,
				Kind:      localCluster,
				Available: false,
				Message:   "Waiting for ManagedCluster webhook",
			},
		})
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	} else if err != nil {
		log.Error(err, "Failed to get ManagedCluster CR")
		return ctrl.Result{}, err
	}

	log.Info("Setting annotations on ManagedCluster CR")
	annotations := managedCluster.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	if len(mce.Spec.NodeSelector) > 0 {
		log.Info("Adding NodeSelector annotation")
		nodeSelector, err := json.Marshal(mce.Spec.NodeSelector)
		if err != nil {
			log.Error(err, "Failed to json marshal MCE NodeSelector")
			return ctrl.Result{}, err
		}
		annotations[utils.AnnotationNodeSelector] = string(nodeSelector)
	} else {
		log.Info("Removing NodeSelector annotation")
		delete(annotations, utils.AnnotationNodeSelector)
	}
	managedCluster.SetAnnotations(annotations)
	applyReleaseVersionAnnotation(managedCluster)

	log.Info("Updating ManagedCluster CR")
	err = r.Client.Update(ctx, managedCluster)
	if err != nil {
		log.Error(err, "Failed to update ManagedCluster CR")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, err
}

func (r *MultiClusterEngineReconciler) ensureNoLocalCluster(ctx context.Context, mce *backplanev1.MultiClusterEngine) (
	ctrl.Result, error) {

	if utils.IsUnitTest() {
		log.Info("skipping local cluster removal in unit tests")
		return ctrl.Result{}, nil
	}

	nsn := types.NamespacedName{Name: localCluster, Namespace: mce.Spec.TargetNamespace}
	lcs := status.LocalClusterStatus{
		NamespacedName: nsn,
		Enabled:        false,
	}
	r.StatusManager.RemoveComponent(lcs)
	r.StatusManager.AddComponent(lcs)

	log.Info("Check if ManagedCluster CR exists")
	managedCluster := utils.NewManagedCluster()
	err := r.Client.Get(ctx, types.NamespacedName{Name: utils.LocalClusterName}, managedCluster)
	if apierrors.IsNotFound(err) || apimeta.IsNoMatchError(err) {
		log.Info("ManagedCluster CR is not present")
	} else if err != nil {
		return ctrl.Result{}, err
	} else {
		log.Info("Deleting ManagedCluster CR")
		managedCluster = utils.NewManagedCluster()
		utils.AddBackplaneConfigLabels(managedCluster, mce.GetName())
		err = r.Client.Delete(ctx, managedCluster)
		if err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "Error deleting ManagedCluster CR")
			return ctrl.Result{}, err
		}
		log.Info("ManagedCluster CR has been deleted")

		msg := "Waiting for local managed cluster to terminate."
		condition := status.NewCondition(
			backplanev1.MultiClusterEngineProgressing,
			metav1.ConditionTrue,
			status.ManagedClusterTerminatingReason,
			msg,
		)
		r.StatusManager.AddCondition(condition)
		log.Info(msg)
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	log.Info("Check if managed cluster namespace exists")
	ns := utils.NewLocalNamespace()
	err = r.Client.Get(ctx, types.NamespacedName{Name: ns.GetName()}, ns)
	if apierrors.IsNotFound(err) {
		log.Info("Managed cluster namespace has been removed")
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "Failed to get managed cluster namespace")
		return ctrl.Result{}, err
	}
	log.Info("Managed cluster namespace still exists")

	log.Info("Deleting managed cluster namespace")
	ns = utils.NewLocalNamespace()
	err = r.Client.Delete(ctx, ns)
	if err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "Error deleting managed cluster ns")
		return ctrl.Result{}, err
	}

	log.Info("Managed cluster namespace has been deleted")
	msg := "Waiting for local managed cluster namespace to terminate."
	condition := status.NewCondition(
		backplanev1.MultiClusterEngineProgressing,
		metav1.ConditionTrue,
		status.NamespaceTerminatingReason,
		msg,
	)
	r.StatusManager.AddCondition(condition)
	log.Info(msg)
	return ctrl.Result{RequeueAfter: requeuePeriod}, nil
}

// clusterManagementAddOnNotFoundStatus reports that a component is not available because
// the ClusterManagementAddOn CRD is not present on the cluster
func clusterManagementAddOnNotFoundStatus(name, namespace string) status.StatusReporter {
	return status.StaticStatus{
		NamespacedName: types.NamespacedName{Name: name, Namespace: namespace},
		Kind:           "Component",
		Condition: backplanev1.ComponentCondition{
			Type:      "Available",
			Name:      name,
			Status:    metav1.ConditionFalse,
			Reason:    status.WaitingForResourceReason,
			Kind:      "Component",
			Available: false,
			Message:   "Waiting for ClusterManagementAddOn CRD to be available",
		},
	}
}

// applyReleaseVersionAnnotation applies the semver version the operator is reconciling
// towards annotation to the resource template
func applyReleaseVersionAnnotation(template *unstructured.Unstructured) {
	annotations := template.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[utils.AnnotationReleaseVersion] = version.Version
	template.SetAnnotations(annotations)
}

func logTemplateDeletionError(err error, name string) {
	log.Error(err, fmt.Sprintf("Failed to delete template: %s", name))
}
