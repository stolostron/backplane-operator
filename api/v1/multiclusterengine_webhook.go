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

package v1

import (
	"context"
	"errors"
	"fmt"
	"os"

	admissionregistration "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	cl "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	DefaultTargetNamespace = "multicluster-engine"
)

type BlockDeletionResource struct {
	Name            string
	GVK             schema.GroupVersionKind
	NameExceptions  []string
	LabelExceptions map[string]string
}

// log is for logging in this package.
var (
	backplaneconfiglog = logf.Log.WithName("backplaneconfig-resource")
	Client             cl.Client

	ErrInvalidComponent    = errors.New("invalid component config")
	ErrInvalidNamespace    = errors.New("invalid TargetNamespace")
	ErrInvalidDeployMode   = errors.New("invalid DeploymentMode")
	ErrInvalidAvailability = errors.New("invalid AvailabilityConfig")
	ErrInvalidInfraNS      = errors.New("invalid InfrastructureCustomNamespace")

	blockDeletionResources = []BlockDeletionResource{
		{
			Name: "DiscoveryConfig",
			GVK: schema.GroupVersionKind{
				Group:   "discovery.open-cluster-management.io",
				Version: "v1",
				Kind:    "DiscoveryConfigList",
			},
		},
		{
			Name: "AgentServiceConfig",
			GVK: schema.GroupVersionKind{
				Group:   "agent-install.openshift.io",
				Version: "v1beta1",
				Kind:    "AgentServiceConfigList",
			},
		},
	}
)

// ValidatingWebhook returns the ValidatingWebhookConfiguration used for the multiclusterengine
// linked to a service in the provided namespace
func ValidatingWebhook(namespace string) *admissionregistration.ValidatingWebhookConfiguration {
	fail := admissionregistration.Fail
	none := admissionregistration.SideEffectClassNone
	path := "/validate-multicluster-openshift-io-v1-multiclusterengine"
	return &admissionregistration.ValidatingWebhookConfiguration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admissionregistration.k8s.io/v1",
			Kind:       "ValidatingWebhookConfiguration",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "multiclusterengines.multicluster.openshift.io",
			Annotations: map[string]string{"service.beta.openshift.io/inject-cabundle": "true"},
		},
		Webhooks: []admissionregistration.ValidatingWebhook{
			{
				AdmissionReviewVersions: []string{
					"v1",
					"v1beta1",
				},
				Name: "multiclusterengines.multicluster.openshift.io",
				ClientConfig: admissionregistration.WebhookClientConfig{
					Service: &admissionregistration.ServiceReference{
						Name:      "multicluster-engine-operator-webhook-service",
						Namespace: namespace,
						Path:      &path,
					},
				},
				FailurePolicy: &fail,
				Rules: []admissionregistration.RuleWithOperations{
					{
						Rule: admissionregistration.Rule{
							APIGroups:   []string{GroupVersion.Group},
							APIVersions: []string{GroupVersion.Version},
							Resources:   []string{"multiclusterengines"},
						},
						Operations: []admissionregistration.OperationType{
							admissionregistration.Create,
							admissionregistration.Update,
							admissionregistration.Delete,
						},
					},
				},
				SideEffects: &none,
			},
		},
	}
}

func (r *MultiClusterEngine) SetupWebhookWithManager(mgr ctrl.Manager) error {
	Client = mgr.GetClient()
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
var _ webhook.CustomDefaulter = &MultiClusterEngine{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type
func (r *MultiClusterEngine) Default(ctx context.Context, obj runtime.Object) error {
	backplaneconfiglog.Info("default", "name", r.Name)
	if r.Spec.TargetNamespace == "" {
		r.Spec.TargetNamespace = DefaultTargetNamespace
	}
	return nil
}

var _ webhook.CustomValidator = &MultiClusterEngine{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type
func (r *MultiClusterEngine) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	backplaneconfiglog.Info("validate create", "Kind", r.Kind, "Name", r.GetName())

	if (r.Spec.AvailabilityConfig != HABasic) && (r.Spec.AvailabilityConfig != HAHigh) && (r.Spec.AvailabilityConfig != "") {
		return nil, ErrInvalidAvailability
	}

	// Validate components
	if r.Spec.Overrides != nil {
		for _, c := range r.Spec.Overrides.Components {
			if !validComponent(c) {
				return nil, fmt.Errorf("%w: %s is not a known component", ErrInvalidComponent, c.Name)
			}
		}
	}

	mceList := &MultiClusterEngineList{}
	if err := Client.List(ctx, mceList); err != nil {
		return nil, fmt.Errorf("unable to list BackplaneConfigs: %s", err)
	}

	targetNS := r.Spec.TargetNamespace
	if targetNS == "" {
		targetNS = DefaultTargetNamespace
	}

	if err := validateLocalClusterNameLength(r.Spec.LocalClusterName); err != nil {
		return nil, err
	}

	for _, mce := range mceList.Items {
		mce := mce
		if mce.Spec.TargetNamespace == targetNS || (targetNS == DefaultTargetNamespace && mce.Spec.TargetNamespace == "") {
			return nil, fmt.Errorf("%w: MultiClusterEngine with targetNamespace already exists: '%s'",
				ErrInvalidNamespace, mce.Name)
		}
		if !IsInHostedMode(r) && !IsInHostedMode(&mce) {
			return nil, fmt.Errorf("%w: MultiClusterEngine in Standalone mode already exists: `%s`. "+
				"Only one resource may exist in Standalone mode.", ErrInvalidDeployMode, mce.Name)
		}
	}
	return nil, nil
}

func validateLocalClusterNameLength(name string) (err error) {
	if len(name) >= 35 {
		return fmt.Errorf("local-cluster name must be shorter than 35 characters")
	}
	return nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (r *MultiClusterEngine) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	backplaneconfiglog.Info("validate update", "Kind", r.Kind, "Name", r.GetName())

	oldMCE := oldObj.(*MultiClusterEngine)
	backplaneconfiglog.Info(oldMCE.Spec.TargetNamespace)
	if (r.Spec.TargetNamespace != oldMCE.Spec.TargetNamespace) && (oldMCE.Spec.TargetNamespace != "") {
		return nil, fmt.Errorf("%w: changes cannot be made to target namespace", ErrInvalidNamespace)
	}
	if IsInHostedMode(r) != IsInHostedMode(oldMCE) {
		return nil, fmt.Errorf("%w: changes cannot be made to DeploymentMode", ErrInvalidDeployMode)
	}

	oldNS, newNS := "", ""
	if oldMCE.Spec.Overrides != nil {
		oldNS = oldMCE.Spec.Overrides.InfrastructureCustomNamespace
	}
	if r.Spec.Overrides != nil {
		newNS = r.Spec.Overrides.InfrastructureCustomNamespace
	}
	if oldNS != newNS {
		return nil, fmt.Errorf("%w: changes cannot be made to InfrastructureCustomNamespace", ErrInvalidInfraNS)
	}

	if (r.Spec.AvailabilityConfig != HABasic) && (r.Spec.AvailabilityConfig != HAHigh) && (r.Spec.AvailabilityConfig != "") {
		return nil, ErrInvalidAvailability
	}

	// Validate components
	if r.Spec.Overrides != nil {
		for _, c := range r.Spec.Overrides.Components {
			if !validComponent(c) {
				return nil, fmt.Errorf("%w: %s is not a known component", ErrInvalidComponent, c.Name)
			}
		}
	}

	// Block disable if relevant resources present
	if r.ComponentPresent(Discovery) && !r.Enabled(Discovery) {
		cfg, err := config.GetConfig()
		if err != nil {
			return nil, err
		}

		c, err := discovery.NewDiscoveryClientForConfig(cfg)
		if err != nil {
			return nil, err
		}

		gvk := schema.GroupVersionKind{
			Group:   "discovery.open-cluster-management.io",
			Version: "v1",
			Kind:    "DiscoveryConfigList",
		}
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(gvk)
		err = discovery.ServerSupportsVersion(c, gvk.GroupVersion())
		if err == nil {
			if err := Client.List(ctx, list); err != nil {
				return nil, fmt.Errorf("unable to list %s: %s", "DiscoveryConfig", err)
			}
			if len(list.Items) != 0 {
				return nil, fmt.Errorf("existing %s resources must first be deleted", "DiscoveryConfig")
			}
		}
	}

	// if the Spec.LocalClusterName field has changed
	if oldMCE.Spec.LocalClusterName != r.Spec.LocalClusterName {
		if err := validateLocalClusterNameLength(r.Spec.LocalClusterName); err != nil {
			return nil, err
		}
		// block changing localClusterName if the label `managedBy` is set to `true`
		if IsACMManaged(r) {
			logf.Log.Info("MCE is managed by ACM, local-cluster name will be set through MultiClusterHub CR")
		}

		// Block changing localClusterName if ManagedCluster with label `local-cluster = true` exists
		managedClusterGVK := schema.GroupVersionKind{
			Group:   "cluster.open-cluster-management.io",
			Version: "v1",
			Kind:    "ManagedClusterList",
		}
		mcName := oldMCE.Spec.LocalClusterName

		// list ManagedClusters
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(managedClusterGVK)
		if err := Client.List(ctx, list); err != nil {
			return nil, fmt.Errorf("unable to list ManagedCluster: %s", err)
		}

		// Error if any of the ManagedClusters is the `local-cluster`
		for _, managedCluster := range list.Items {
			if managedCluster.GetName() == mcName || managedCluster.GetLabels()["local-cluster"] == "true" {
				return nil, fmt.Errorf("cannot update Spec.LocalClusterName while local-cluster is enabled")
			}
		}
	}

	return nil, nil
}

var cfg *rest.Config

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type
func (r *MultiClusterEngine) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	// TODO(user): fill in your validation logic upon object deletion.
	backplaneconfiglog.Info("validate delete", "Kind", r.Kind, "Name", r.GetName())
	if val, ok := os.LookupEnv("ENV_TEST"); !ok || val == "false" {
		var err error
		cfg, err = config.GetConfig()
		if err != nil {
			return nil, err
		}
	}

	c, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}

	managedClusterGVK := schema.GroupVersionKind{
		Group:   "cluster.open-cluster-management.io",
		Version: "v1",
		Kind:    "ManagedClusterList",
	}

	tmpBlockDeletionResources := append(blockDeletionResources, BlockDeletionResource{ // only adds the dynamic localClusterName to a temporary variable so duplicates aren't created
		Name:            "ManagedCluster",
		GVK:             managedClusterGVK,
		NameExceptions:  []string{r.Spec.LocalClusterName},
		LabelExceptions: map[string]string{"local-cluster": "true"},
	})

	for _, resource := range tmpBlockDeletionResources {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(resource.GVK)
		err := discovery.ServerSupportsVersion(c, list.GroupVersionKind().GroupVersion())
		if err != nil {
			continue
		}
		if err := Client.List(ctx, list); err != nil {
			return nil, fmt.Errorf("unable to list %s: %s", resource.Name, err)
		}
		for _, item := range list.Items {
			if !contains(resource.NameExceptions, item.GetName()) {
				return nil, fmt.Errorf("cannot delete %s resource. Existing %s resources must first be deleted",
					r.Name, resource.Name)
			}
			if !hasIntersection(resource.LabelExceptions, item.GetLabels()) {
				return nil, fmt.Errorf("cannot delete %s resource. Necessary labels %v are not present", r.Name, resource.LabelExceptions)
			}
		}
	}
	return nil, nil
}

func hasIntersection(smallerMap map[string]string, largerMap map[string]string) bool {
	// iterate through the keys of the smaller map to save time
	for k, sVal := range smallerMap {
		if lVal := largerMap[k]; lVal == sVal {
			return true // return true if A and B share any complete key-value pair
		}
	}
	return false
}

func contains(s []string, v string) bool {
	for _, vs := range s {
		if vs == v {
			return true
		}
	}
	return false
}
