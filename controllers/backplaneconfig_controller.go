// Copyright Contributors to the Open Cluster Management project

/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/foundation"
	"github.com/stolostron/backplane-operator/pkg/overrides"
	renderer "github.com/stolostron/backplane-operator/pkg/rendering"
	"github.com/stolostron/backplane-operator/pkg/status"
	"github.com/stolostron/backplane-operator/pkg/toggle"
	"github.com/stolostron/backplane-operator/pkg/utils"
	"github.com/stolostron/backplane-operator/pkg/version"
	"k8s.io/client-go/util/retry"
	clustermanager "open-cluster-management.io/api/operator/v1"

	configv1 "github.com/openshift/api/config/v1"
	hiveconfig "github.com/openshift/hive/apis/hive/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	semver "github.com/Masterminds/semver"
	pkgerrors "github.com/pkg/errors"
	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
)

// MultiClusterEngineReconciler reconciles a MultiClusterEngine object
type MultiClusterEngineReconciler struct {
	Client           client.Client
	UncachedClient   client.Client
	CacheSpec        CacheSpec
	Scheme           *runtime.Scheme
	Images           map[string]string
	StatusManager    *status.StatusTracker
	Log              logr.Logger
	UpgradeableCond  utils.Condition
	DeprecatedFields map[string]bool
}

const (
	requeuePeriod      = 15 * time.Second
	backplaneFinalizer = "finalizer.multicluster.openshift.io"

	trustBundleNameEnvVar  = "TRUSTED_CA_BUNDLE"
	defaultTrustBundleName = "trusted-ca-bundle"

	controlPlane = "backplane-operator"
)

var (
	log = logf.Log.WithName("reconcile")
)

// +kubebuilder:rbac:groups=multicluster.openshift.io,resources=multiclusterengines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=multicluster.openshift.io,resources=multiclusterengines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=multicluster.openshift.io,resources=multiclusterengines/finalizers,verbs=update
// +kubebuilder:rbac:groups=apiextensions.k8s.io;rbac.authorization.k8s.io;"";apps,resources=deployments;serviceaccounts;customresourcedefinitions;clusterrolebindings;clusterroles,verbs=get;create;update;list
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;create;update;list;watch;delete;patch
// +kubebuilder:rbac:groups="discovery.open-cluster-management.io",resources=discoveryconfigs,verbs=get
// +kubebuilder:rbac:groups="discovery.open-cluster-management.io",resources=discoveryconfigs,verbs=list
// +kubebuilder:rbac:groups="discovery.open-cluster-management.io",resources=discoveryconfigs;discoveredclusters,verbs=create;get;list;watch;update;delete;deletecollection;patch;approve;escalate;bind
// +kubebuilder:rbac:groups=config.openshift.io,resources=clusterversions,verbs=get;list;watch;
// +kubebuilder:rbac:groups=console.openshift.io,resources=consoleplugins;consolequickstarts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.openshift.io,resources=consoles,verbs=get;list;watch;update;patch

// AgentServiceConfig webhook delete check
// +kubebuilder:rbac:groups=agent-install.openshift.io,resources=agentserviceconfigs,verbs=get;list;watch

// ClusterManager RBAC
// +kubebuilder:rbac:groups="",resources=configmaps;configmaps/status;namespaces;serviceaccounts;services;secrets,verbs=create;get;list;update;watch;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes;endpoints,verbs=get;list;watch
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
// +kubebuilder:rbac:groups="";events.k8s.io,resources=events,verbs=create;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments;replicasets,verbs=create;get;list;update;watch;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings;rolebindings,verbs=create;get;list;update;watch;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;roles,verbs=create;get;list;update;watch;patch;delete;escalate;bind
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=create;get;list;update;watch;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions/status,verbs=update;patch
// +kubebuilder:rbac:groups=apiregistration.k8s.io,resources=apiservices,verbs=create;get;list;update;watch;patch;delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations;mutatingwebhookconfigurations,verbs=create;get;list;update;watch;patch;delete
// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=clustermanagers,verbs=create;get;list;watch;update;delete;patch
// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=clustermanagers/status,verbs=update;patch
// +kubebuilder:rbac:groups=imageregistry.open-cluster-management.io,resources=managedclusterimageregistries;managedclusterimageregistries/status,verbs=approve;bind;create;delete;deletecollection;escalate;get;list;patch;update;watch
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io;agent.open-cluster-management.io;operator.open-cluster-management.io,resources=klusterletaddonconfigs;managedclusters;multiclusterhubs,verbs=get;list;watch;create;delete;watch;update;patch
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersets/join,verbs=create
// +kubebuilder:rbac:groups=migration.k8s.io,resources=storageversionmigrations,verbs=create;get;list;update;patch;watch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=create;get;list;update;patch;watch;delete
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=clustermanagementaddons;clustermanagementaddons/finalizers;managedclusteraddons;managedclusteraddons/finalizers;managedclusteraddons/status,verbs=create;get;list;update;patch;watch;delete;deletecollection
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=addondeploymentconfigs,verbs=get;list;watch;
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=addonplacementscores,verbs=create;get;list;update;patch;watch;delete;deletecollection
// +kubebuilder:rbac:groups=proxy.open-cluster-management.io,resources=managedproxyconfigurations,verbs=create;get;list;update;patch;watch;delete;deletecollection
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersetbindings,verbs=create;get;list;update;patch;watch;delete;deletecollection
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersets/bind,verbs=create

// Hive RBAC
// +kubebuilder:rbac:groups="hive.openshift.io",resources=hiveconfigs,verbs=get;create;update;delete;list;watch
// +kubebuilder:rbac:groups="hive.openshift.io",resources=clusterdeployments;clusterpools;clusterclaims;machinepools,verbs=approve;bind;create;delete;deletecollection;escalate;get;list;patch;update;watch

// CLC RBAC
// +kubebuilder:rbac:groups="internal.open-cluster-management.io",resources="managedclusterinfos",verbs=get;list;watch
// +kubebuilder:rbac:groups="config.openshift.io";"authentication.k8s.io",resources=clusterversions;tokenreviews,verbs=get;create
// +kubebuilder:rbac:groups="register.open-cluster-management.io",resources=managedclusters/accept,verbs=update
// +kubebuilder:rbac:groups="tower.ansible.com";"";"batch",resources=ansiblejobs;jobs;secrets;serviceaccounts,verbs=create
// +kubebuilder:rbac:groups="tower.ansible.com";"";"batch",resources=ansiblejobs;jobs;clusterdeployments;serviceaccounts;machinepools,verbs=get
// +kubebuilder:rbac:groups="action.open-cluster-management.io",resources=managedclusteractions,verbs=get;create;update;delete
// +kubebuilder:rbac:groups="cluster.open-cluster-management.io",resources=clustercurators;clustercurators/status,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups="config.open-cluster-management.io",resources=klusterletconfigs,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups="operators.coreos.com",resources=subscriptions,verbs=get;list;watch
// +kubebuilder:rbac:groups="operators.coreos.com",resources=operatorconditions,verbs=create;get;list;patch;update;delete;watch
// +kubebuilder:rbac:groups="",resources=serviceaccounts/token,verbs=create
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenrequests,verbs=create

// InternalEngineComponent
// +kubebuilder:rbac:groups="multicluster.openshift.io",resources="internalenginecomponents",verbs=create;get;delete;patch;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *MultiClusterEngineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (retRes ctrl.Result, retErr error) {
	r.Log = log
	r.Log.Info("Reconciling MultiClusterEngine")

	// Fetch the BackplaneConfig instance
	backplaneConfig, err := r.getBackplaneConfig(ctx, req)
	if err != nil && !apierrors.IsNotFound(err) {
		// Unknown error. Requeue
		r.Log.Info("Failed to fetch backplaneConfig")
		return ctrl.Result{}, err
	} else if err != nil && apierrors.IsNotFound(err) {
		// BackplaneConfig deleted or not found
		// Return and don't requeue
		return ctrl.Result{}, nil
	}

	// reset the status conditions for failures that has occurred in previous iterations.
	backplaneConfig.Status.Conditions = status.FilterOutConditionWithSubString(backplaneConfig.Status.Conditions,
		backplanev1.MultiClusterEngineComponentFailure)

	// Check if any deprecated fields are present within the backplaneConfig spec.
	r.CheckDeprecatedFieldUsage(backplaneConfig)

	// reset status manager
	r.StatusManager.Reset("")
	for _, c := range backplaneConfig.Status.Conditions {
		r.StatusManager.AddCondition(c)
	}

	// Check to see if upgradeable
	r.StatusManager.AddCondition(status.NewCondition(backplanev1.MultiClusterEngineProgressing, metav1.ConditionTrue,
		status.WaitingForResourceReason, "Setting the operator"))

	upgrade := false
	if utils.DeployOnOCP() {
		upgrade, err = r.setOperatorUpgradeableStatus(ctx, backplaneConfig)
		if err != nil {
			r.Log.Error(err, "Trouble with Upgradable Operator Condition")
			r.StatusManager.AddCondition(status.NewCondition(backplanev1.MultiClusterEngineProgressing,
				metav1.ConditionFalse, status.RequirementsNotMetReason, err.Error()))
		}
	}

	defer func() {
		r.Log.Info("Updating status")
		backplaneConfig.Status = r.StatusManager.ReportStatus(*backplaneConfig)
		err := r.Client.Status().Update(ctx, backplaneConfig)
		if backplaneConfig.Status.Phase != backplanev1.MultiClusterEnginePhaseAvailable && !utils.IsPaused(backplaneConfig) {
			retRes = ctrl.Result{RequeueAfter: requeuePeriod}
		}

		if err != nil {
			if apierrors.IsConflict(err) {
				// Error from object being modified is normal behavior and should not be treated like an error
				r.Log.Info("Failed to update status", "Reason", "Object has been modified")
				retRes = ctrl.Result{RequeueAfter: requeuePeriod}
			} else {
				retErr = err
			}
		}
	}()

	// If deletion detected, finalize backplane config
	if backplaneConfig.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(backplaneConfig, backplaneFinalizer) {
			result, err := r.finalizeBackplaneConfig(ctx, backplaneConfig) // returns all errors
			if err != nil {
				r.Log.Info(err.Error())
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
			r.Log.Info(fmt.Sprintf("Result returned from finalizeBackplaneConfig: %v", result))
			if result != (ctrl.Result{}) {
				return result, nil
			}

			r.Log.Info("all subcomponents have been finalized successfully - removing finalizer")
			controllerutil.RemoveFinalizer(backplaneConfig, backplaneFinalizer)
			if err := r.Client.Update(ctx, backplaneConfig); err != nil {
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil // Object finalized successfully
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(backplaneConfig, backplaneFinalizer) {
		controllerutil.AddFinalizer(backplaneConfig, backplaneFinalizer)
		if err := r.Client.Update(ctx, backplaneConfig); err != nil {
			return ctrl.Result{}, err
		}
	}

	var result ctrl.Result

	result, err = r.setDefaults(ctx, backplaneConfig)
	if result != (ctrl.Result{}) {
		return ctrl.Result{}, err
	}
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	result, err = r.removeDeprecatedRBAC(ctx)
	if err != nil {
		return result, err
	}

	if !utils.ShouldIgnoreOCPVersion(backplaneConfig) && utils.DeployOnOCP() {
		currentOCPVersion, err := r.getClusterVersion(ctx)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to detect clusterversion: %w", err)
		}

		if err := version.ValidOCPVersion(currentOCPVersion); err != nil {
			r.StatusManager.AddCondition(status.NewCondition(backplanev1.MultiClusterEngineProgressing,
				metav1.ConditionFalse, status.RequirementsNotMetReason, err.Error()))
			return ctrl.Result{}, err
		}
	}

	result, err = r.validateNamespace(ctx, backplaneConfig)
	if result != (ctrl.Result{}) {
		return ctrl.Result{}, err
	}
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	/*
		In MCE 2.4, we need to ensure that the openshift.io/cluster-monitoring is added to the same namespace as the
		MultiClusterEngine to avoid conflicts with the openshift-* namespace when deploying PrometheusRules and
		ServiceMonitors in ACM and MCE.
	*/
	_, err = r.ensureOpenShiftNamespaceLabel(ctx, backplaneConfig)
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Failed to add to %s label to namespace: %s", utils.OpenShiftClusterMonitoringLabel,
			backplaneConfig.Spec.TargetNamespace))
		return ctrl.Result{}, err
	}

	result, err = r.validateImagePullSecret(ctx, backplaneConfig)
	if result != (ctrl.Result{}) {
		return ctrl.Result{}, err
	}
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	// Attempt to retrieve image overrides from environmental variables.
	imageOverrides := overrides.GetOverridesFromEnv(overrides.OperandImagePrefix)

	// If no overrides found using OperandImagePrefix, attempt to retrieve using OSBSImagePrefix.
	if len(imageOverrides) == 0 {
		imageOverrides = overrides.GetOverridesFromEnv(overrides.OSBSImagePrefix)
	}

	// Check if no image overrides were found using either prefix.
	if len(imageOverrides) == 0 {
		r.Log.Error(err, "Could not get map of image overrides")

		// If images are not set from environmental variables, fail
		r.StatusManager.AddCondition(status.NewCondition(backplanev1.MultiClusterEngineProgressing,
			metav1.ConditionFalse, status.RequirementsNotMetReason, "No image references defined in deployment"))

		return ctrl.Result{}, errors.New(
			"no image references exist. images must be defined as environment variables")
	}

	// Apply image repository override from annotation if present.
	if imageRepo := utils.GetImageRepository(backplaneConfig); imageRepo != "" {
		r.Log.Info(fmt.Sprintf("Overriding Image Repository from annotation 'imageRepository': %s", imageRepo))
		imageOverrides = utils.OverrideImageRepository(imageOverrides, imageRepo)
	}

	// Check for developer overrides in configmap.
	if cmName := utils.GetImageOverridesConfigmapName(backplaneConfig); cmName != "" {
		imageOverrides, err = overrides.GetOverridesFromConfigmap(r.Client, imageOverrides,
			utils.OperatorNamespace(), cmName, false)

		if err != nil {
			r.Log.Error(err, fmt.Sprintf("Failed to find image override configmap: %s/%s",
				utils.OperatorNamespace(), cmName))

			r.StatusManager.AddCondition(status.NewCondition(backplanev1.MultiClusterEngineProgressing,
				metav1.ConditionFalse, status.RequirementsNotMetReason,
				fmt.Sprintf("Issue building image references: %s", err.Error())))

			return ctrl.Result{}, err
		}
	}

	// Update cache with image overrides and related information.
	r.CacheSpec.ImageOverrides = imageOverrides
	r.CacheSpec.ImageRepository = utils.GetImageRepository(backplaneConfig)
	r.CacheSpec.ImageOverridesCM = utils.GetImageOverridesConfigmapName(backplaneConfig)

	// Attempt to retrieve template overrides from environmental variables.
	templateOverrides := overrides.GetOverridesFromEnv(overrides.TemplateOverridePrefix)

	// Check for developer overrides in configmap
	if toConfigmapName := utils.GetTemplateOverridesConfigmapName(backplaneConfig); toConfigmapName != "" {
		templateOverrides, err = overrides.GetOverridesFromConfigmap(r.Client, templateOverrides,
			utils.OperatorNamespace(), toConfigmapName, true)

		if err != nil {
			r.Log.Error(err, fmt.Sprintf("Failed to find template override configmap: %s/%s",
				utils.OperatorNamespace(), toConfigmapName))

			return ctrl.Result{}, err
		}
	}

	// Update cache with template overrides and related information.
	r.CacheSpec.TemplateOverrides = templateOverrides
	r.CacheSpec.TemplateOverridesCM = utils.GetTemplateOverridesConfigmapName(backplaneConfig)

	// Do not reconcile objects if this instance of mce is labeled "paused"
	if utils.IsPaused(backplaneConfig) {
		r.Log.Info("MultiClusterEngine reconciliation is paused. Nothing more to do.")

		cond := status.NewCondition(
			backplanev1.MultiClusterEngineProgressing,
			metav1.ConditionUnknown,
			status.PausedReason,
			"Multiclusterengine is paused",
		)
		r.StatusManager.AddCondition(cond)
		return ctrl.Result{}, nil
	}

	var crdsDir string

	if val, ok := os.LookupEnv("UNIT_TEST"); ok && val == "true" {
		crdsDir = "test/unit-test-crds"
	} else {
		crdsDir = "pkg/templates/crds"
	}

	crds, errs := renderer.RenderCRDs(crdsDir, backplaneConfig)
	if len(errs) > 0 {
		for _, err := range errs {

			return result, err
		}
	}

	// udpate CRDs with retrygo
	for i := range crds {
		_, conversion, _ := unstructured.NestedMap(crds[i].Object, "spec", "conversion", "webhook", "clientConfig", "service")
		if conversion {
			retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				crd := crds[i]
				e := ensureCRD(context.TODO(), r.Client, crd)
				return e
			})
			if retryErr != nil {
				r.Log.Error(retryErr, "Failed to apply CRD")
				return result, retryErr
			}
		}
	}

	result, err = r.DeployAlwaysSubcomponents(ctx, backplaneConfig)
	if err != nil {
		cond := status.NewCondition(
			backplanev1.MultiClusterEngineProgressing,
			metav1.ConditionUnknown,
			status.DeployFailedReason,
			err.Error(),
		)
		r.StatusManager.AddCondition(cond)
		return result, err
	}

	if utils.DeployOnOCP() {
		for _, kind := range backplanev1.GetLegacyConfigKind() {
			_ = r.removeLegacyPrometheusConfigurations(ctx, "openshift-monitoring", kind)
		}
	}

	result, err = r.ensureToggleableComponents(ctx, backplaneConfig)
	if err != nil {
		return result, err
	}

	result, err = r.createTrustBundleConfigmap(ctx, backplaneConfig)
	if err != nil {
		return result, err
	}

	if utils.DeployOnOCP() {
		result, err = r.createMetricsService(ctx, backplaneConfig)
		if err != nil {
			return result, err
		}

		result, err = r.createMetricsServiceMonitor(ctx, backplaneConfig)
		if err != nil {
			return result, err
		}
	}
	result, err = r.ensureRemovalsGone(backplaneConfig)
	if err != nil {
		return result, err
	}

	if upgrade {
		return ctrl.Result{Requeue: true}, nil
	}

	r.StatusManager.AddCondition(status.NewCondition(backplanev1.MultiClusterEngineProgressing, metav1.ConditionTrue,
		status.DeploySuccessReason, "All components deployed"))

	r.Log.Info("Reconcile completed. Requeuing after " + utils.ShortRefreshInterval.String())
	return ctrl.Result{RequeueAfter: utils.ShortRefreshInterval}, nil
}

// This function set the operator condition created by OLM to either allow or disallow upgrade based on whether X.Y desired version matches current version
// It returns an error as well as Boolean determining whether or not the reconcile needs to be rerun in order to update status

func (r *MultiClusterEngineReconciler) setOperatorUpgradeableStatus(ctx context.Context, m *backplanev1.MultiClusterEngine) (bool, error) {
	// Temporary variable
	var upgradeable bool

	// Checking to see if the current version of the MCE matches the desired to determine if we are in an upgrade scenario
	// If the current version doesn't exist, we are currently in a install which will also not allow it to upgrade
	parts1 := strings.Split(m.Status.CurrentVersion, ".")
	parts2 := strings.Split(version.Version, ".")

	if parts1[0] == parts2[0] && parts1[1] == parts2[1] {
		upgradeable = true
	} else {
		upgradeable = false
	}

	// 	These messages are drawn from operator condition
	// Right now, they just indicate between upgrading and not
	msg := utils.UpgradeableAllowMessage
	status := metav1.ConditionTrue
	reason := utils.UpgradeableAllowReason

	// 	The condition is the only field that affects whether or not we can upgrade
	// The rest are just status info
	if !upgradeable {
		status = metav1.ConditionFalse
		reason = utils.UpgradeableUpgradingReason
		msg = utils.UpgradeableUpgradingMessage

	}
	// This error should only occur if the operator condition does not exist for some reason
	// We will return true so that we re-reconcile on the failed update of the operator condition
	if err := r.UpgradeableCond.Set(ctx, status, reason, msg); err != nil {
		return true, err
	}

	if !upgradeable {
		return true, nil
	} else {
		return false, nil
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *MultiClusterEngineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mceBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&backplanev1.MultiClusterEngine{}).
		WithEventFilter(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{},
			predicate.AnnotationChangedPredicate{})).
		Watches(&appsv1.Deployment{},
			handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &backplanev1.MultiClusterEngine{}),
		).
		Watches(&hiveconfig.HiveConfig{}, &handler.Funcs{
			DeleteFunc: func(ctx context.Context, e event.DeleteEvent, q workqueue.RateLimitingInterface) {
				labels := e.Object.GetLabels()
				q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
					Name: labels["backplaneconfig.name"],
				}})
			},
		}, builder.WithPredicates(predicate.LabelChangedPredicate{})).
		Watches(&clustermanager.ClusterManager{}, &handler.Funcs{
			DeleteFunc: func(ctx context.Context, e event.DeleteEvent, q workqueue.RateLimitingInterface) {
				labels := e.Object.GetLabels()
				q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
					Name: labels["backplaneconfig.name"],
				}})
			},
			UpdateFunc: func(ctx context.Context, e event.UpdateEvent, q workqueue.RateLimitingInterface) {
				labels := e.ObjectOld.GetLabels()
				q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
					Name: labels["backplaneconfig.name"],
				}})
			},
		}, builder.WithPredicates(predicate.LabelChangedPredicate{}))

	if utils.DeployOnOCP() {
		mceBuilder.Watches(&monitorv1.ServiceMonitor{}, &handler.Funcs{
			DeleteFunc: func(ctx context.Context, e event.DeleteEvent, q workqueue.RateLimitingInterface) {
				labels := e.Object.GetLabels()
				if label, ok := labels["backplaneconfig.name"]; ok {
					q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
						Name: label,
					}})
				}
			},
		}, builder.WithPredicates(predicate.LabelChangedPredicate{})).
			Watches(&configv1.ClusterVersion{},
				handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, a client.Object) []reconcile.Request {
					req := []reconcile.Request{}
					multiclusterengineList := &backplanev1.MultiClusterEngineList{}
					if err := r.Client.List(ctx, multiclusterengineList); err == nil && len(multiclusterengineList.Items) > 0 {
						for _, mce := range multiclusterengineList.Items {
							tmpreq := reconcile.Request{
								NamespacedName: types.NamespacedName{
									Name: mce.GetName(),
								},
							}
							req = append(req, tmpreq)
						}
					}
					return req
				}))
	}

	return mceBuilder.Complete(r)
}

// createTrustBundleConfigmap creates a configmap that will be injected with the
// trusted CA bundle for use with the OCP cluster wide proxy
func (r *MultiClusterEngineReconciler) createTrustBundleConfigmap(ctx context.Context,
	mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {

	// Get Trusted Bundle configmap name
	trustBundleName := defaultTrustBundleName
	trustBundleNamespace := mce.Spec.TargetNamespace
	if name, ok := os.LookupEnv(trustBundleNameEnvVar); ok && name != "" {
		trustBundleName = name
	}
	namespacedName := types.NamespacedName{
		Name:      trustBundleName,
		Namespace: trustBundleNamespace,
	}

	// Check if configmap exists
	cm := &corev1.ConfigMap{}
	err := r.Client.Get(ctx, namespacedName, cm)
	if err != nil && !apierrors.IsNotFound(err) {
		// Unknown error. Requeue
		log.Info(fmt.Sprintf("error while getting trust bundle configmap %s: %s", trustBundleName, err))
		return ctrl.Result{}, err
	} else if err == nil {
		// configmap exists
		return ctrl.Result{}, nil
	}

	// Create configmap
	cm = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trustBundleName,
			Namespace: trustBundleNamespace,
			Labels: map[string]string{
				"config.openshift.io/inject-trusted-cabundle": "true",
			},
		},
	}
	err = ctrl.SetControllerReference(mce, cm, r.Scheme)
	if err != nil {
		return ctrl.Result{}, pkgerrors.Wrapf(
			err, "Error setting controller reference on trust bundle configmap %s",
			trustBundleName,
		)
	}
	log.Info(fmt.Sprintf("creating trust bundle configmap %s: %s", trustBundleNamespace, trustBundleName))
	err = r.Client.Create(ctx, cm)
	if err != nil {
		// Error creating configmap
		log.Info(fmt.Sprintf("error creating trust bundle configmap %s: %s", trustBundleName, err))
		return ctrl.Result{}, err
	}
	// Configmap created successfully
	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) createMetricsService(ctx context.Context,
	mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {

	const Port = 8080

	sName := utils.MCEOperatorMetricsServiceName
	sNamespace := mce.Spec.TargetNamespace

	namespacedName := types.NamespacedName{
		Name:      sName,
		Namespace: sNamespace,
	}

	// Check if service exists
	if err := r.Client.Get(ctx, namespacedName, &corev1.Service{}); err != nil {
		if !apierrors.IsNotFound(err) {
			// Unknown error. Requeue
			log.Error(err, fmt.Sprintf("error while getting multicluster-engine metrics service: %s/%s",
				sNamespace, sName))
			return ctrl.Result{}, err
		}

		// Create metrics service
		s := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      sName,
				Namespace: sNamespace,
				Labels: map[string]string{
					"control-plane": controlPlane,
				},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name:       "metrics",
						Port:       int32(Port),
						Protocol:   "TCP",
						TargetPort: intstr.FromInt(Port),
					},
				},
				Selector: map[string]string{
					"control-plane": controlPlane,
				},
			},
		}

		if err = ctrl.SetControllerReference(mce, s, r.Scheme); err != nil {
			return ctrl.Result{}, pkgerrors.Wrapf(
				err, "error setting controller reference on metrics service: %s", sName,
			)
		}

		if err = r.Client.Create(ctx, s); err != nil {
			// Error creating metrics service
			log.Error(err, fmt.Sprintf("error creating multicluster-engine metrics service: %s", sName))
			return ctrl.Result{}, err
		}

		log.Info(fmt.Sprintf("Created multicluster-engine metrics service: %s", sName))
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) createMetricsServiceMonitor(ctx context.Context,
	mce *backplanev1.MultiClusterEngine) (ctrl.Result, error) {

	smName := utils.MCEOperatorMetricsServiceMonitorName
	smNamespace := mce.Spec.TargetNamespace

	namespacedName := types.NamespacedName{
		Name:      smName,
		Namespace: smNamespace,
	}

	// Check if service exists
	if err := r.Client.Get(ctx, namespacedName, &monitorv1.ServiceMonitor{}); err != nil {
		if !apierrors.IsNotFound(err) {
			// Unknown error. Requeue
			log.Error(err, fmt.Sprintf("error while getting multicluster-engine metrics service: %s/%s",
				smNamespace, smName))
			return ctrl.Result{}, err
		}

		// Create metrics service
		sm := &monitorv1.ServiceMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      smName,
				Namespace: smNamespace,
				Labels: map[string]string{
					"control-plane": controlPlane,
				},
			},
			Spec: monitorv1.ServiceMonitorSpec{
				Endpoints: []monitorv1.Endpoint{
					{
						BearerTokenFile: "/var/run/secrets/kubernetes.io/serviceaccount/token",
						BearerTokenSecret: &corev1.SecretKeySelector{
							Key: "",
						},
						Port: "metrics",
					},
				},
				NamespaceSelector: monitorv1.NamespaceSelector{
					MatchNames: []string{
						mce.Spec.TargetNamespace,
					},
				},
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"control-plane": controlPlane,
					},
				},
			},
		}

		if err = ctrl.SetControllerReference(mce, sm, r.Scheme); err != nil {
			return ctrl.Result{}, pkgerrors.Wrapf(
				err, "error setting controller reference on multicluster-engine metrics servicemonitor: %s", smName)
		}

		if err = r.Client.Create(ctx, sm); err != nil {
			// Error creating metrics servicemonitor
			log.Error(err, fmt.Sprintf("error creating metrics servicemonitor: %s", smName))
			return ctrl.Result{}, err
		}

		log.Info(fmt.Sprintf("Created multicluster-engine metrics servicemonitor: %s", smName))
	}

	return ctrl.Result{}, nil
}

// DeployAlwaysSubcomponents ensures all subcomponents exist
func (r *MultiClusterEngineReconciler) DeployAlwaysSubcomponents(ctx context.Context,
	backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	chartsDir := renderer.AlwaysChartsDir
	// Renders all templates from charts
	templates, errs := renderer.RenderCharts(chartsDir, backplaneConfig, r.CacheSpec.ImageOverrides,
		r.CacheSpec.TemplateOverrides)

	if len(errs) > 0 {
		for _, err := range errs {
			log.Info(err.Error())
		}
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	// Applies all templates
	for _, template := range templates {
		if template.GetKind() == "Deployment" {
			r.StatusManager.AddComponent(status.DeploymentStatus{
				NamespacedName: types.NamespacedName{Name: template.GetName(), Namespace: template.GetNamespace()},
			})
		}
		result, err := r.applyTemplate(ctx, backplaneConfig, template)
		if err != nil {
			return result, err
		}
	}

	result, err := r.ensureCustomResources(ctx, backplaneConfig)
	if err != nil {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureInternalEngineComponent(
	ctx context.Context,
	backplaneConfig *backplanev1.MultiClusterEngine,
	component string) (ctrl.Result, error) {

	iec := &backplanev1.InternalEngineComponent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: backplanev1.GroupVersion.String(),
			Kind:       "InternalEngineComponent",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      component,
			Namespace: backplaneConfig.Spec.TargetNamespace,
		},
	}

	if err := r.Client.Get(
		ctx, types.NamespacedName{Name: iec.GetName(), Namespace: iec.GetNamespace()}, iec); err != nil {

		if apierrors.IsNotFound(err) {
			log.Info("Creating InternalEngineComponent", "Name", iec.GetName(), "Namespace", iec.GetNamespace())
			if err := r.Client.Create(ctx, iec); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to create InternalEngineComponent CR: %s/%s: %v",
					iec.GetNamespace(), iec.GetName(), err)
			}
		} else {
			return ctrl.Result{}, fmt.Errorf("failed to get InternalEngineComponent CR: %s/%s: %v",
				iec.GetNamespace(), iec.GetName(), err)
		}
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoInternalEngineComponent(ctx context.Context,
	backplaneConfig *backplanev1.MultiClusterEngine, component string) (ctrl.Result, error) {
	// Get target namespace for MCE
	mceNS := backplaneConfig.Spec.TargetNamespace

	iec := &backplanev1.InternalEngineComponent{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: component, Namespace: mceNS}, iec); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return reconcile.Result{}, fmt.Errorf("failed to get InternalEngineComponent: %s/%s: %v",
			mceNS, component, err)
	}

	// Check if it has a deletion timestamp (indicating it's in the process of being deleted)
	if iec.GetDeletionTimestamp() != nil {
		log.Info("InternalEngineComponent deletion in progress", "Name", iec.GetName(), "Namespace", iec.GetNamespace(),
			"DeletionTimestamp", iec.GetDeletionTimestamp())

		return reconcile.Result{RequeueAfter: requeuePeriod}, nil
	}

	log.Info("Deleting InternalEngineComponent", "Name", iec.GetName(), "Namespace", iec.GetNamespace())
	if err := r.Client.Delete(ctx, iec); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to delete InternalEngineComponent CR: %s/%s: %v",
				iec.GetNamespace(), iec.GetName(), err)
		}
	}

	// Ensure that the resource is fully deleted by attempting to refetch it
	if err := r.Client.Get(ctx,
		types.NamespacedName{Name: iec.GetName(), Namespace: iec.GetNamespace()}, iec); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("InternalEngineComponent successfully deleted", "Name", iec.GetName(), "Namespace", iec.GetNamespace())
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("failed to get InternalEngineComponent %s/%s: %v",
			iec.GetNamespace(), iec.GetName(), err)
	}

	// Requeue to check again after a short delay
	return ctrl.Result{RequeueAfter: requeuePeriod}, nil
}

func (r *MultiClusterEngineReconciler) fetchChartOrCRDPath(component string, useCRDPath bool) string {

	chartDirs := map[string]string{
		backplanev1.AssistedService:           toggle.AssistedServiceChartDir,
		backplanev1.ClusterLifecycle:          toggle.ClusterLifecycleChartDir,
		backplanev1.ClusterManager:            toggle.ClusterManagerChartDir,
		backplanev1.ClusterProxyAddon:         toggle.ClusterProxyAddonDir,
		backplanev1.ConsoleMCE:                toggle.ConsoleMCEChartsDir,
		backplanev1.Discovery:                 toggle.DiscoveryChartDir,
		backplanev1.Hive:                      toggle.HiveChartDir,
		backplanev1.HyperShift:                toggle.HyperShiftChartDir,
		backplanev1.ImageBasedInstallOperator: toggle.ImageBasedInstallOperatorChartDir,
		backplanev1.ManagedServiceAccount:     toggle.ManagedServiceAccountChartDir,
		backplanev1.ServerFoundation:          toggle.ServerFoundationChartDir,
	}

	crdDirs := map[string]string{
		backplanev1.ManagedServiceAccount: toggle.ManagedServiceAccountCRDPath,
	}

	// Return CRD path if `useCRDPath` is true and the component has a CRD path defined
	if useCRDPath {
		if dir, exists := crdDirs[component]; exists {
			return dir
		}

		log.Info("CRD path not found for component: %v", "Component", component)
		return "" // No CRD path defined for this component
	}

	// Return chart directory if `useCRDPath` is false or the component has no CRD path
	if dir, exists := chartDirs[component]; exists {
		return dir
	}

	// Log and return a default path for chart directory not found
	log.Info("Chart directory not found for component detected", "Component", component)
	return fmt.Sprintf("/chart/toggle/%v", component)
}

func (r *MultiClusterEngineReconciler) ensureToggleableComponents(ctx context.Context,
	backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	errs := map[string]error{}
	requeue := false

	if backplaneConfig.Enabled(backplanev1.ManagedServiceAccount) && foundation.CanInstallAddons(ctx, r.Client) {
		result, err := r.ensureManagedServiceAccount(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ManagedServiceAccount] = err
		}
	} else {
		result, err := r.ensureNoManagedServiceAccount(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ManagedServiceAccount] = err
		}
	}

	if backplaneConfig.Enabled(backplanev1.ImageBasedInstallOperator) {
		result, err := r.ensureImageBasedInstallOperator(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ImageBasedInstallOperator] = err
		}
	} else {
		result, err := r.ensureNoImageBasedInstallOperator(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ImageBasedInstallOperator] = err
		}
	}

	if backplaneConfig.Enabled(backplanev1.HyperShift) && foundation.CanInstallAddons(ctx, r.Client) {
		result, err := r.ensureHyperShift(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.HyperShift] = err
		}
	} else {
		result, err := r.ensureNoHyperShift(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.HyperShift] = err
		}
	}

	result, err := r.reconcileHypershiftLocalHosting(ctx, backplaneConfig)
	if result != (ctrl.Result{}) {
		requeue = true
	}
	if err != nil {
		errs[backplanev1.HypershiftLocalHosting] = err
	}

	if utils.DeployOnOCP() {
		ocpConsole, err := r.CheckConsole(ctx)
		if err != nil {
			return ctrl.Result{}, err
		}

		if backplaneConfig.Enabled(backplanev1.ConsoleMCE) && ocpConsole {
			result, err = r.ensureConsoleMCE(ctx, backplaneConfig)
			if result != (ctrl.Result{}) {
				requeue = true
			}
			if err != nil {
				errs[backplanev1.ConsoleMCE] = err
			}
		} else {
			result, err = r.ensureNoConsoleMCE(ctx, backplaneConfig, ocpConsole)
			if result != (ctrl.Result{}) {
				requeue = true
			}
			if err != nil {
				errs[backplanev1.ConsoleMCE] = err
			}
		}
	}

	if backplaneConfig.Enabled(backplanev1.Discovery) {
		result, err = r.ensureDiscovery(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.Discovery] = err
		}
	} else {
		result, err = r.ensureNoDiscovery(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.Discovery] = err
		}
	}

	if backplaneConfig.Enabled(backplanev1.Hive) {
		result, err = r.ensureHive(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.Hive] = err
		}
	} else {
		result, err = r.ensureNoHive(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.Hive] = err
		}
	}

	if backplaneConfig.Enabled(backplanev1.AssistedService) {
		result, err = r.ensureAssistedService(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.AssistedService] = err
		}
	} else {
		result, err = r.ensureNoAssistedService(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.AssistedService] = err
		}
	}

	if backplaneConfig.Enabled(backplanev1.ClusterLifecycle) {
		result, err = r.ensureClusterLifecycle(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ClusterLifecycle] = err
		}
	} else {
		result, err = r.ensureNoClusterLifecycle(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ClusterLifecycle] = err
		}
	}

	if backplaneConfig.Enabled(backplanev1.ClusterManager) && foundation.CanInstallAddons(ctx, r.Client) {
		result, err = r.ensureClusterManager(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ClusterManager] = err
		}
	} else {
		result, err = r.ensureNoClusterManager(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ClusterManager] = err
		}
	}

	if backplaneConfig.Enabled(backplanev1.ServerFoundation) {
		result, err = r.ensureServerFoundation(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ServerFoundation] = err
		}
	} else {
		result, err = r.ensureNoServerFoundation(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ServerFoundation] = err
		}
	}

	if backplaneConfig.Enabled(backplanev1.ClusterProxyAddon) && foundation.CanInstallAddons(ctx, r.Client) {
		result, err = r.ensureClusterProxyAddon(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ClusterProxyAddon] = err
		}
	} else {
		result, err = r.ensureNoClusterProxyAddon(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.ClusterProxyAddon] = err
		}
	}
	if backplaneConfig.Enabled(backplanev1.LocalCluster) {
		result, err := r.ensureLocalCluster(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.LocalCluster] = err
		}
	} else {
		result, err := r.ensureNoLocalCluster(ctx, backplaneConfig)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[backplanev1.LocalCluster] = err
		}
	}

	if len(errs) > 0 {
		errorMessages := []string{}
		for k, v := range errs {
			errorMessages = append(errorMessages, fmt.Sprintf("error ensuring %s: %s", k, v.Error()))
		}
		combinedError := fmt.Sprintf(": %s", strings.Join(errorMessages, "; "))

		r.Log.Error(errors.New("errors applying components"), combinedError)
		return ctrl.Result{}, errors.New(combinedError)
	}
	if requeue {
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}
	return ctrl.Result{}, nil
}

/*
getComponentConfig searches for a component configuration in the provided list
by component name. It returns the configuration and a boolean indicating
whether it was found.
*/
func (r *MultiClusterEngineReconciler) getComponentConfig(components []backplanev1.ComponentConfig,
	componentName string) (backplanev1.ComponentConfig, bool) {
	for _, c := range components {
		if c.Name == componentName {
			return c, true
		}
	}
	return backplanev1.ComponentConfig{}, false
}

/*
getDeploymentConfig searches for a deployment configuration in the provided list
by deployment name. It returns a pointer to the configuration and nil if not found.
*/
func (r *MultiClusterEngineReconciler) getDeploymentConfig(deployments []backplanev1.DeploymentConfig,
	deploymentName string) (*backplanev1.DeploymentConfig, bool) {
	for _, d := range deployments {
		if d.Name == deploymentName {
			return &d, true
		}
	}
	return &backplanev1.DeploymentConfig{}, false
}

/*
applyEnvConfig updates the specified container in the provided template with
new environment variables. Logs errors if encountered during retrieval or update operations.
*/
func (r *MultiClusterEngineReconciler) applyEnvConfig(template *unstructured.Unstructured, containerName string,
	envConfigs []backplanev1.EnvConfig) error {

	containers, found, err := unstructured.NestedSlice(template.Object, "spec", "template", "spec", "containers")
	if err != nil || !found {
		log.Error(err, "Failed to get containers from template", "Kind", template.GetKind(), "Name", template.GetName())
		return err
	}

	for i, container := range containers {
		// We need to cast the container to a map of string interfaces to access the container fields.
		containerMap := container.(map[string]interface{})

		if containerMap["name"] == containerName {
			existingEnv, _, _ := unstructured.NestedSlice(containerMap, "env")
			for _, envConfig := range envConfigs {
				envVar := map[string]interface{}{
					"name":  envConfig.Name,
					"value": envConfig.Value,
				}
				existingEnv = append(existingEnv, envVar)
			}

			if err := unstructured.SetNestedSlice(containerMap, existingEnv, "env"); err != nil {
				log.Error(err, "Failed to set environment variable", "Container", containerName)
				return err

			} else {
				containers[i] = containerMap
			}
			break
		}
	}

	if err = unstructured.SetNestedSlice(template.Object, containers, "spec", "template", "spec",
		"containers"); err != nil {
		log.Error(err, "Failed to set containers in template", "Template", template.GetName())
		return err
	}

	return nil
}

func (r *MultiClusterEngineReconciler) applyComponentDeploymentOverrides(mce *backplanev1.MultiClusterEngine,
	templates []*unstructured.Unstructured, component string) (ctrl.Result, error) {

	// Check if the component has overrides available
	componentConfig, found := r.getComponentConfig(mce.Spec.Overrides.Components, component)

	if !found {
		log.V(2).Info("No component config found", "Component", component)
		return ctrl.Result{}, nil
	}

	for _, template := range templates {
		// Check if the template is of kind "Deployment"
		if template.GetKind() != "Deployment" {
			continue // Skip if the template is not of a deployment
		}

		deploymentConfig, found := r.getDeploymentConfig(
			componentConfig.ConfigOverrides.Deployments, template.GetName())

		if !found {
			log.V(2).Info("No deployment config found for deployment", "Name", template.GetName())
			continue // Skip this template and check the next one
		}

		log.V(2).Info("Applying deployment overrides for template", "Name", template.GetName())
		// Apply environment variable overrides for each container
		for _, container := range deploymentConfig.Containers {
			if err := r.applyEnvConfig(template, container.Name, container.Env); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) applyTemplate(ctx context.Context,
	backplaneConfig *backplanev1.MultiClusterEngine, template *unstructured.Unstructured) (ctrl.Result, error) {

	// Set owner reference.
	// Don't set owner reference on hypershift-addon ManagedClusterAddOn. See ACM-2289
	if !(template.GetName() == "hypershift-addon" && template.GetKind() == "ManagedClusterAddOn") {
		err := ctrl.SetControllerReference(backplaneConfig, template, r.Scheme)
		if err != nil {
			return ctrl.Result{},
				fmt.Errorf("error setting controller reference on resource Name: %s Kind: %s Error: %w",
					template.GetName(), template.GetKind(), err)
		}
	}

	if template.GetKind() == "APIService" {
		result, err := r.ensureUnstructuredResource(ctx, backplaneConfig, template)
		if err != nil {
			return result, err
		}
	} else {
		// Apply the object data.
		force := true
		err := r.Client.Patch(ctx, template, client.Apply,
			&client.PatchOptions{Force: &force, FieldManager: "backplane-operator"})

		if err != nil {
			errMessage := fmt.Errorf(
				"error applying object Name: %s Kind: %s Error: %w", template.GetName(), template.GetKind(), err)

			condType := fmt.Sprintf("%v: %v (Kind:%v)", backplanev1.MultiClusterEngineComponentFailure,
				template.GetName(), template.GetKind())

			r.StatusManager.AddCondition(
				status.NewCondition(
					backplanev1.MultiClusterEngineConditionType(condType), metav1.ConditionTrue,
					status.ApplyFailedReason, errMessage.Error()),
			)

			return ctrl.Result{}, errMessage
		}
	}

	return ctrl.Result{}, nil
}

// deleteTemplate return true if resource does not exist and returns an error if a GET or DELETE errors unexpectedly. A false response without error
// means the resource is in the process of deleting.
func (r *MultiClusterEngineReconciler) deleteTemplate(ctx context.Context,
	backplaneConfig *backplanev1.MultiClusterEngine, template *unstructured.Unstructured) (ctrl.Result, error) {

	// before := template.DeepCopy()
	err := r.Client.Get(ctx, types.NamespacedName{Name: template.GetName(), Namespace: template.GetNamespace()}, template)
	if err != nil && (apierrors.IsNotFound(err) || apimeta.IsNoMatchError(err)) {
		return ctrl.Result{}, nil
	}

	// set status progressing condition
	if err != nil {
		log.Error(err, "Odd error delete template")
		return ctrl.Result{}, err
	}

	log.Info("Finalizing template", "Kind", template.GetKind(), "Name", template.GetName())
	err = r.Client.Delete(ctx, template)
	if err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to delete template")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureCustomResources(ctx context.Context,
	backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {

	if foundation.CanInstallAddons(ctx, r.Client) {
		addonTemplates, err := foundation.GetAddons()
		if err != nil {
			return ctrl.Result{}, err
		}
		for _, addonTemplate := range addonTemplates {
			addonTemplate.SetNamespace(backplaneConfig.Spec.TargetNamespace)
			if err := ctrl.SetControllerReference(backplaneConfig, addonTemplate, r.Scheme); err != nil {
				return ctrl.Result{}, pkgerrors.Wrapf(err, "Error setting controller reference on resource %s", addonTemplate.GetName())
			}
			force := true
			err := r.Client.Patch(ctx, addonTemplate, client.Apply,
				&client.PatchOptions{Force: &force, FieldManager: "backplane-operator"})
			if err != nil {
				return ctrl.Result{}, pkgerrors.Wrapf(err, "error applying object Name: %s Kind: %s",
					addonTemplate.GetName(), addonTemplate.GetKind())
			}
		}
	} else {
		log.Info("ClusterManagementAddon API is not installed. Waiting to install addons.")
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureOpenShiftNamespaceLabel(ctx context.Context,
	backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	existingNs := &corev1.Namespace{}

	err := r.Client.Get(ctx, types.NamespacedName{Name: backplaneConfig.Spec.TargetNamespace}, existingNs)
	if err != nil || apierrors.IsNotFound(err) {
		log.Error(err, fmt.Sprintf("Failed to find namespace for MultiClusterEngine: %s",
			backplaneConfig.Spec.TargetNamespace))
		return ctrl.Result{}, err
	}

	if existingNs.Labels == nil || len(existingNs.Labels) == 0 {
		existingNs.Labels = make(map[string]string)
	}

	if _, ok := existingNs.Labels[utils.OpenShiftClusterMonitoringLabel]; !ok {
		log.Info(fmt.Sprintf("Adding label: %s to namespace: %s", utils.OpenShiftClusterMonitoringLabel,
			backplaneConfig.Spec.TargetNamespace))
		existingNs.Labels[utils.OpenShiftClusterMonitoringLabel] = "true"

		err = r.Client.Update(ctx, existingNs)
		if err != nil {
			log.Error(err, fmt.Sprintf("Failed to update namespace for MultiClusterEngine: %s with the label: %s",
				backplaneConfig.Spec.TargetNamespace, utils.OpenShiftClusterMonitoringLabel))
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) ensureNoAllInternalEngineComponents(ctx context.Context,
	backplaneConfig *backplanev1.MultiClusterEngine) (ctrl.Result, error) {
	errs := map[string]error{}
	requeue := false

	components := []string{
		backplanev1.ConsoleMCE,
		backplanev1.ManagedServiceAccount,
		backplanev1.Discovery,
		backplanev1.Hive,
		backplanev1.AssistedService,
		backplanev1.ServerFoundation,
		backplanev1.ImageBasedInstallOperator,
		backplanev1.ClusterLifecycle,
		backplanev1.HyperShift,
		backplanev1.ClusterProxyAddon,
		backplanev1.LocalCluster,
		backplanev1.ClusterManager,
	}

	for _, v := range components {
		result, err := r.ensureNoInternalEngineComponent(ctx, backplaneConfig, v)
		if result != (ctrl.Result{}) {
			requeue = true
		}
		if err != nil {
			errs[v] = err
		}
	}

	if len(errs) > 0 {
		errorMessages := []string{}
		for k, v := range errs {
			errorMessages = append(errorMessages, fmt.Sprintf("error ensuring %s: %s", k, v.Error()))
		}
		combinedError := fmt.Sprintf(": %s", strings.Join(errorMessages, "; "))
		return ctrl.Result{}, errors.New(combinedError)
	}
	if requeue {
		return ctrl.Result{RequeueAfter: requeuePeriod}, nil
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) finalizeBackplaneConfig(ctx context.Context,
	backplaneConfig *backplanev1.MultiClusterEngine) (reconcile.Result, error) {

	result, err := r.ensureNoAllInternalEngineComponents(ctx, backplaneConfig)
	if err != nil {
		return ctrl.Result{}, err
	}
	if result != (ctrl.Result{}) {
		return result, nil
	}

	if utils.DeployOnOCP() {
		ocpConsole, err := r.CheckConsole(ctx)
		if err != nil {
			return ctrl.Result{}, err
		}
		if ocpConsole {
			_, err := r.removePluginFromConsoleResource(ctx)
			if err != nil {
				log.Info("Error ensuring plugin is removed from console resource")
				return ctrl.Result{}, err
			}
		}
	}

	// Remove hypershift-addon ManagedClusterAddOn if present
	hypershiftAddon, err := renderer.RenderHypershiftAddon(backplaneConfig)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.Client.Get(ctx, types.NamespacedName{Name: hypershiftAddon.GetName(),
		Namespace: hypershiftAddon.GetNamespace()}, hypershiftAddon)

	if err != nil {
		if !(apierrors.IsNotFound(err) || apimeta.IsNoMatchError(err)) {
			log.Error(err, "error while looking for hypershift-addon ManagedClusterAddOn")
			return ctrl.Result{}, err
		}
	} else {
		log.Info("finalizing hypershift-addon ManagedClusterAddOn")
		err := r.Client.Delete(ctx, hypershiftAddon)
		if err != nil {
			log.Error(err, "error deleting hypershift-addon ManagedClusterAddOn")
			return ctrl.Result{}, err
		}

		// If wait time exceeds expected then uninstall may not be able to progress
		if time.Since(backplaneConfig.DeletionTimestamp.Time) < 3*time.Minute {
			terminatingCondition := status.NewCondition(backplanev1.MultiClusterEngineConditionType(backplanev1.MultiClusterEngineProgressing), metav1.ConditionTrue, status.WaitingForResourceReason, "Waiting for ManagedClusterAddOn hypershift-addon to terminate.")
			r.StatusManager.AddCondition(terminatingCondition)
		} else {
			terminatingCondition := status.NewCondition(backplanev1.MultiClusterEngineConditionType(backplanev1.MultiClusterEngineProgressing), metav1.ConditionFalse, status.WaitingForResourceReason, "ManagedClusterAddOn hypershift-addon still exists.")
			r.StatusManager.AddCondition(terminatingCondition)
		}

		return ctrl.Result{}, fmt.Errorf("waiting for 'hypershift-addon' ManagedClusterAddOn to be terminated before proceeding with uninstallation")
	}

	localCluster := &unstructured.Unstructured{}
	localCluster.SetGroupVersionKind(
		schema.GroupVersionKind{
			Group:   "cluster.open-cluster-management.io",
			Version: "v1",
			Kind:    "ManagedCluster",
		},
	)

	if err = r.Client.Get(ctx, types.NamespacedName{Name: "local-cluster"}, localCluster); err == nil { // If resource exists, delete
		log.Info("finalizing local-cluster custom resource")

		if err := r.Client.Delete(ctx, localCluster); err != nil {
			log.Error(err, "error deleting local-cluster ManagedCluster CR")
			return ctrl.Result{}, err
		}

		// If wait time exceeds expected then uninstall may not be able to progress
		if time.Since(backplaneConfig.DeletionTimestamp.Time) < 10*time.Minute {
			terminatingCondition := status.NewCondition(
				backplanev1.MultiClusterEngineConditionType(backplanev1.MultiClusterEngineProgressing),
				metav1.ConditionTrue, status.WaitingForResourceReason,
				"Waiting for ManagedCluster local-cluster to terminate.")

			r.StatusManager.AddCondition(terminatingCondition)
		} else {
			terminatingCondition := status.NewCondition(
				backplanev1.MultiClusterEngineConditionType(backplanev1.MultiClusterEngineProgressing),
				metav1.ConditionFalse, status.WaitingForResourceReason, "ManagedCluster local-cluster still exists.")

			r.StatusManager.AddCondition(terminatingCondition)
		}

		return ctrl.Result{}, fmt.Errorf(
			"waiting for 'local-cluster' ManagedCluster to be terminated before proceeding with uninstallation")

	} else if !apierrors.IsNotFound(err) { // Return error, if error is not not found error
		log.Error(err, "error while looking for local-cluster ManagedCluster CR")
		return ctrl.Result{}, err
	}

	clusterManager := &unstructured.Unstructured{}
	clusterManager.SetGroupVersionKind(
		schema.GroupVersionKind{
			Group:   "operator.open-cluster-management.io",
			Version: "v1",
			Kind:    "ClusterManager",
		},
	)

	if err = r.Client.Get(ctx, types.NamespacedName{Name: "cluster-manager"}, clusterManager); err == nil { // If resource exists, delete
		log.Info("finalizing cluster-manager custom resource")
		if err := r.Client.Delete(ctx, clusterManager); err != nil {
			return ctrl.Result{}, err
		}

		// If wait time exceeds expected then uninstall may not be able to progress
		if time.Since(backplaneConfig.DeletionTimestamp.Time) < 10*time.Minute {
			terminatingCondition := status.NewCondition(
				backplanev1.MultiClusterEngineConditionType(backplanev1.MultiClusterEngineProgressing),
				metav1.ConditionTrue, status.WaitingForResourceReason,
				"Waiting for ClusterManager cluster-manager to terminate.")

			r.StatusManager.AddCondition(terminatingCondition)
		} else {
			terminatingCondition := status.NewCondition(
				backplanev1.MultiClusterEngineConditionType(backplanev1.MultiClusterEngineProgressing),
				metav1.ConditionFalse, status.WaitingForResourceReason, "ClusterManager cluster-manager still exists.")

			r.StatusManager.AddCondition(terminatingCondition)
		}

		return ctrl.Result{}, fmt.Errorf(
			"waiting for 'cluster-manager' ClusterManager to be terminated before proceeding with uninstallation")

	} else if !apierrors.IsNotFound(err) { // Return error, if error is not not found error
		return ctrl.Result{}, err
	}

	ocmHubNamespace := &corev1.Namespace{}

	if err = r.Client.Get(ctx, types.NamespacedName{Name: "open-cluster-management-hub"}, ocmHubNamespace); err == nil {
		// If wait time exceeds expected then uninstall may not be able to progress
		if time.Since(backplaneConfig.DeletionTimestamp.Time) < 10*time.Minute {
			terminatingCondition := status.NewCondition(
				backplanev1.MultiClusterEngineConditionType(
					backplanev1.MultiClusterEngineProgressing), metav1.ConditionTrue, status.WaitingForResourceReason,
				"Waiting for namespace open-cluster-management-hub to terminate.")

			r.StatusManager.AddCondition(terminatingCondition)
		} else {
			terminatingCondition := status.NewCondition(
				backplanev1.MultiClusterEngineConditionType(backplanev1.MultiClusterEngineProgressing),
				metav1.ConditionFalse, status.WaitingForResourceReason,
				"Namespace open-cluster-management-hub still exists.")

			r.StatusManager.AddCondition(terminatingCondition)
		}

		return ctrl.Result{}, fmt.Errorf(
			"waiting for 'open-cluster-management-hub' namespace to be terminated before proceeding with uninstallation")

	} else if !apierrors.IsNotFound(err) { // Return error, if error is not not found error
		return ctrl.Result{}, err
	}

	globalSetNamespace := &corev1.Namespace{}
	if err = r.Client.Get(ctx, types.NamespacedName{Name: "open-cluster-management-global-set"}, globalSetNamespace); err == nil {
		if err := r.Client.Delete(ctx, globalSetNamespace); err != nil {
			return ctrl.Result{}, err
		}

	} else if !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	localClusterNS := &corev1.Namespace{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: "local-cluster"}, localClusterNS); err == nil {
		// If wait time exceeds expected then uninstall may not be able to progress
		if backplaneConfig.GetDeletionTimestamp() != nil {
			deletionTime := backplaneConfig.GetDeletionTimestamp().Time
			if time.Since(deletionTime) < 10*time.Minute {
				terminatingCondition := status.NewCondition(
					backplanev1.MultiClusterEngineConditionType(
						backplanev1.MultiClusterEngineProgressing), metav1.ConditionTrue, status.WaitingForResourceReason,
					"Waiting for namespace local-cluster to terminate.")

				r.StatusManager.AddCondition(terminatingCondition)
			}
		} else {
			terminatingCondition := status.NewCondition(
				backplanev1.MultiClusterEngineConditionType(backplanev1.MultiClusterEngineProgressing),
				metav1.ConditionFalse, status.WaitingForResourceReason,
				"Namespace local-cluster still exists.")

			r.StatusManager.AddCondition(terminatingCondition)
		}

		return ctrl.Result{}, fmt.Errorf(
			"waiting for 'local-cluster' namespace to be terminated before proceeding with uninstallation")

	} else if !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) getBackplaneConfig(ctx context.Context, req ctrl.Request) (
	*backplanev1.MultiClusterEngine, error) {
	backplaneConfig := &backplanev1.MultiClusterEngine{}
	err := r.Client.Get(ctx, req.NamespacedName, backplaneConfig)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			r.Log.Info("BackplaneConfig resource not found. Ignoring since object must be deleted")
			return nil, err
		}
		// Error reading the object - requeue the request.
		return nil, err
	}
	return backplaneConfig, nil
}

// ensureUnstructuredResource ensures that the unstructured resource is applied in the cluster properly
func (r *MultiClusterEngineReconciler) ensureUnstructuredResource(ctx context.Context,
	bpc *backplanev1.MultiClusterEngine, u *unstructured.Unstructured) (ctrl.Result, error) {

	found := &unstructured.Unstructured{}
	found.SetGroupVersionKind(u.GroupVersionKind())

	utils.AddBackplaneConfigLabels(u, bpc.Name)

	// Try to get API group instance
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      u.GetName(),
		Namespace: u.GetNamespace(),
	}, found)
	if err != nil && apierrors.IsNotFound(err) {
		// Resource doesn't exist so create it
		err := r.Client.Create(ctx, u)
		if err != nil {
			// Creation failed
			r.Log.Error(err, "Failed to create new instance")
			return ctrl.Result{}, err
		}
		// Creation was successful
		r.Log.Info(fmt.Sprintf("Created new resource - kind: %s name: %s", u.GetKind(), u.GetName()))
		// condition := NewHubCondition(operatorsv1.Progressing, metav1.ConditionTrue, NewComponentReason, "Created new resource")
		// SetHubCondition(&m.Status, *condition)
		return ctrl.Result{}, nil

	} else if err != nil {
		// Error that isn't due to the resource not existing
		r.Log.Error(err, "Failed to get resource")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) setDefaults(ctx context.Context, m *backplanev1.MultiClusterEngine) (ctrl.Result, error) {

	updateNecessary := false
	if !utils.AvailabilityConfigIsValid(m.Spec.AvailabilityConfig) {
		m.Spec.AvailabilityConfig = backplanev1.HAHigh
		updateNecessary = true
	}

	if len(m.Spec.TargetNamespace) == 0 {
		m.Spec.TargetNamespace = backplanev1.DefaultTargetNamespace
		updateNecessary = true
	}

	if utils.SetDefaultComponents(m) {
		updateNecessary = true
	}

	// hypershift preview component upgraded in ACM 2.8.0
	if m.Prune(backplanev1.HyperShiftPreview) {
		updateNecessary = true
	}

	if m.Enabled(backplanev1.ManagedServiceAccountPreview) {
		// if the preview was pruned, enable the non-preview version instead
		m.Enable(backplanev1.ManagedServiceAccount)
		// no need to disable -preview version, as it will get pruned below
		updateNecessary = true
	}

	// managedserviceaccount preview component upgraded in ACM 2.9.0
	if m.Prune(backplanev1.ManagedServiceAccountPreview) {
		updateNecessary = true
	}

	// image based install operator preview component upgraded in ACM 2.12.0
	if m.Prune(backplanev1.ImageBasedInstallOperatorPreview) {
		updateNecessary = true
	}

	if utils.DeduplicateComponents(m) {
		updateNecessary = true
	}

	if utils.DeployOnOCP() {
		// Set and store cluster Ingress domain for use later
		clusterIngressDomain, err := r.getClusterIngressDomain(ctx, m)
		if err != nil {
			return ctrl.Result{}, pkgerrors.Wrapf(err, "failed to detect cluster ingress domain")
		}

		// Set OCP version as env var, so that charts can render this value
		os.Setenv("ACM_CLUSTER_INGRESS_DOMAIN", clusterIngressDomain)

		// If OCP 4.10+ then set then enable the MCE console. Else ensure it is disabled
		currentClusterVersion, err := r.getClusterVersion(ctx)
		if err != nil {
			return ctrl.Result{}, pkgerrors.Wrapf(err, "failed to detect clusterversion")
		}

		// Set OCP version as env var, so that charts can render this value
		os.Setenv("ACM_HUB_OCP_VERSION", currentClusterVersion)

		currentVersion, err := semver.NewVersion(currentClusterVersion)
		if err != nil {
			log.Error(err, fmt.Sprintf("Failed to convert currentClusterVersion %s to semver compatible value for comparison", currentClusterVersion))
			return ctrl.Result{}, err
		}
		// -0 allows for prerelease builds to pass the validation.
		// If -0 is removed, developer/rc builds will not pass this check
		constraint, err := semver.NewConstraint(">= 4.10.0-0")
		if err != nil {
			log.Error(err, "Failed to set constraint of minimum supported version for plugins")
			return ctrl.Result{}, err
		}

		if constraint.Check(currentVersion) {
			// If ConsoleMCE config already exists, then don't overwrite it
			if !m.ComponentPresent(backplanev1.ConsoleMCE) {
				log.Info("Dynamic plugins are supported. ConsoleMCE Config is not detected. Enabling ConsoleMCE")
				m.Enable(backplanev1.ConsoleMCE)
				updateNecessary = true
			}
		} else {
			if m.Enabled(backplanev1.ConsoleMCE) {
				log.Info("Dynamic plugins are not supported. Disabling MCE console")
				m.Disable(backplanev1.ConsoleMCE)
				updateNecessary = true
			}
		}
	}

	// Apply defaults to server
	if updateNecessary {
		log.Info("Setting defaults")
		err := r.Client.Update(ctx, m)
		if err != nil {
			log.Error(err, "Failed to update MultiClusterEngine")
			return ctrl.Result{}, err
		}
		log.Info("MultiClusterEngine successfully updated")
		return ctrl.Result{Requeue: true}, nil
	} else {
		return ctrl.Result{}, nil
	}
}

func (r *MultiClusterEngineReconciler) validateNamespace(ctx context.Context, m *backplanev1.MultiClusterEngine) (
	ctrl.Result, error) {

	newNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: m.Spec.TargetNamespace,
		},
	}
	checkNs := &corev1.Namespace{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{Name: m.Spec.TargetNamespace}, checkNs)
	if err != nil && apierrors.IsNotFound(err) {
		if err := ctrl.SetControllerReference(m, newNs, r.Scheme); err != nil {
			return ctrl.Result{}, pkgerrors.Wrapf(err, "Error setting controller reference on resource %s",
				m.Spec.TargetNamespace)
		}

		err = r.Client.Create(context.TODO(), newNs)
		if err != nil {
			log.Error(err, "Could not create namespace")
			return ctrl.Result{}, err
		}

		log.Info("Namespace created")
		return ctrl.Result{Requeue: true}, nil
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// validateImagePullSecret returns an error if the namespace in spec.targetNamespace does not have a secret
// with the name in spec.imagePullSecret.
func (r *MultiClusterEngineReconciler) validateImagePullSecret(ctx context.Context, m *backplanev1.MultiClusterEngine) (
	ctrl.Result, error) {
	if m.Spec.ImagePullSecret == "" {
		return ctrl.Result{}, nil
	}

	pullSecret := &corev1.Secret{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      m.Spec.ImagePullSecret,
		Namespace: m.Spec.TargetNamespace,
	}, pullSecret)
	if apierrors.IsNotFound(err) {
		missingPullSecret := status.NewCondition(
			backplanev1.MultiClusterEngineConditionType(backplanev1.MultiClusterEngineProgressing),
			metav1.ConditionFalse, status.RequirementsNotMetReason,
			fmt.Sprintf("Could not find imagePullSecret %s in namespace %s", m.Spec.ImagePullSecret,
				m.Spec.TargetNamespace))

		r.StatusManager.AddCondition(missingPullSecret)
		return ctrl.Result{}, err
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterEngineReconciler) getClusterVersion(ctx context.Context) (string, error) {
	// If Unit test
	if val, ok := os.LookupEnv("UNIT_TEST"); ok && val == "true" {
		if _, exists := os.LookupEnv("ACM_HUB_OCP_VERSION"); exists {
			return os.Getenv("ACM_HUB_OCP_VERSION"), nil
		}
		return "4.99.99", nil
	}

	clusterVersion := &configv1.ClusterVersion{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: "version"}, clusterVersion)
	if err != nil {
		r.Log.Error(err, "Failed to detect clusterversion")
		return "", err
	}

	if len(clusterVersion.Status.History) == 0 {
		r.Log.Error(err, "Failed to detect status in clusterversion.status.history")
		return "", err
	}
	return clusterVersion.Status.History[0].Version, nil
}

// +kubebuilder:rbac:groups="config.openshift.io",resources="ingresses",verbs=get;list;watch

func (r *MultiClusterEngineReconciler) getClusterIngressDomain(ctx context.Context, mce *backplanev1.MultiClusterEngine) (string, error) {
	// If Unit test
	if val, ok := os.LookupEnv("UNIT_TEST"); ok && val == "true" {
		return "apps.installer-test-cluster.dev00.red-chesterfield.com", nil
	}

	clusterIngress := &configv1.Ingress{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: "cluster"}, clusterIngress)
	if err != nil {
		r.Log.Error(err, "Failed to detect cluster ingress")
		return "", err
	}

	if clusterIngress.Spec.Domain == "" {
		r.Log.Error(err, "Domain not found or empty in Ingress")
		return "", fmt.Errorf("Domain not found or empty in Ingress")
	}
	return clusterIngress.Spec.Domain, nil
}

func (r *MultiClusterEngineReconciler) CheckDeprecatedFieldUsage(m *backplanev1.MultiClusterEngine) {
	a := m.GetAnnotations()
	df := []struct {
		name      string
		isPresent bool
	}{
		{utils.DeprecatedAnnotationIgnoreOCPVersion, a[utils.DeprecatedAnnotationIgnoreOCPVersion] != ""},
		{utils.DeprecatedAnnotationImageOverridesCM, a[utils.DeprecatedAnnotationImageOverridesCM] != ""},
		{utils.DeprecatedAnnotationImageRepo, a[utils.DeprecatedAnnotationImageRepo] != ""},
		{utils.DeprecatedAnnotationKubeconfig, a[utils.DeprecatedAnnotationKubeconfig] != ""},
		{utils.DeprecatedAnnotationMCEPause, a[utils.DeprecatedAnnotationMCEPause] != ""},
	}

	if r.DeprecatedFields == nil {
		r.DeprecatedFields = make(map[string]bool)
	}

	for _, f := range df {
		if f.isPresent && !r.DeprecatedFields[f.name] {
			r.Log.Info(fmt.Sprintf("Warning: %s field usage is deprecated in operator.", f.name))
			r.DeprecatedFields[f.name] = true
		}
	}
}

func ensureCRD(ctx context.Context, c client.Client, crd *unstructured.Unstructured) error {
	existingCRD := &unstructured.Unstructured{}
	existingCRD.SetGroupVersionKind(crd.GroupVersionKind())
	err := c.Get(ctx, types.NamespacedName{Name: crd.GetName()}, existingCRD)
	if err != nil && apierrors.IsNotFound(err) {
		// CRD not found. Create and return
		err = c.Create(ctx, crd)
		if err != nil {
			return fmt.Errorf("error creating CRD '%s': %w", crd.GetName(), err)
		}
	} else if err != nil {
		return fmt.Errorf("error getting CRD '%s': %w", crd.GetName(), err)
	} else if err == nil {
		// CRD already exists. Update and return
		if utils.AnnotationPresent(utils.AnnotationMCEIgnore, existingCRD) {
			return nil
		}
		crd.SetResourceVersion(existingCRD.GetResourceVersion())
		err = c.Update(ctx, crd)
		if err != nil {
			return fmt.Errorf("error updating CRD '%s': %w", crd.GetName(), err)
		}
	}
	return nil
}

func (r *MultiClusterEngineReconciler) removeDeprecatedRBAC(ctx context.Context) (ctrl.Result, error) {
	hyperShiftPreviewClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: "open-cluster-management:hypershift-preview:hypershift-addon-manager"}, hyperShiftPreviewClusterRoleBinding)
	if err == nil {
		err = r.Client.Delete(ctx, hyperShiftPreviewClusterRoleBinding)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}
	hyperShiftPreviewClusterRole := &rbacv1.ClusterRole{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: "open-cluster-management:hypershift-preview:hypershift-addon-manager"}, hyperShiftPreviewClusterRole)
	if err == nil {
		err = r.Client.Delete(ctx, hyperShiftPreviewClusterRole)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}
