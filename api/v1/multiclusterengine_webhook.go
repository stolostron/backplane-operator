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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	ctrl "sigs.k8s.io/controller-runtime"
	cl "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	DefaultTargetNamespace = "multicluster-engine"
)

// log is for logging in this package.
var (
	backplaneconfiglog = logf.Log.WithName("backplaneconfig-resource")
	Client             cl.Client

	blockDeletionResources = []struct {
		Name string
		GVK  schema.GroupVersionKind
	}{
		{
			Name: "ManagedCluster",
			GVK: schema.GroupVersionKind{
				Group:   "cluster.open-cluster-management.io",
				Version: "v1",
				Kind:    "ManagedClusterList",
			},
		},
		{
			Name: "BareMetalAsset",
			GVK: schema.GroupVersionKind{
				Group:   "inventory.open-cluster-management.io",
				Version: "v1alpha1",
				Kind:    "BareMetalAssetList",
			},
		},
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

func (r *MultiClusterEngine) SetupWebhookWithManager(mgr ctrl.Manager) error {
	Client = mgr.GetClient()
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
var _ webhook.Defaulter = &MultiClusterEngine{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *MultiClusterEngine) Default() {
	backplaneconfiglog.Info("default", "name", r.Name)
	if r.Spec.TargetNamespace == "" {
		r.Spec.TargetNamespace = DefaultTargetNamespace
	}
}

var _ webhook.Validator = &MultiClusterEngine{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *MultiClusterEngine) ValidateCreate() error {
	ctx := context.Background()
	backplaneconfiglog.Info("validate create", "name", r.Name)

	if (r.Spec.AvailabilityConfig != HABasic) && (r.Spec.AvailabilityConfig != HAHigh) && (r.Spec.AvailabilityConfig != "") {
		return errors.New("Invalid AvailabilityConfig given")
	}
	if (r.Spec.DeploymentMode != ModeHosted) && (r.Spec.DeploymentMode != ModeStandalone) && (r.Spec.DeploymentMode != "") {
		return errors.New("Invalid DeploymentMode given")
	}

	// Validate components
	if r.Spec.Overrides != nil {
		for _, c := range r.Spec.Overrides.Components {
			if !validComponent(c) {
				return errors.New(fmt.Sprintf("invalid component config: %s is not a known component", c.Name))
			}
		}
	}

	mceList := &MultiClusterEngineList{}
	if err := Client.List(ctx, mceList); err != nil {
		return fmt.Errorf("unable to list BackplaneConfigs: %s", err)
	}

	targetNS := r.Spec.TargetNamespace
	if targetNS == "" {
		targetNS = DefaultTargetNamespace
	}
	mode := r.Spec.DeploymentMode

	for _, mce := range mceList.Items {
		if mce.Spec.TargetNamespace == targetNS {
			return errors.New(fmt.Sprintf("MultiClusterEngine with targetNamespace already exists: `%s`", mce.Name))
		}
		if mode == ModeStandalone && mce.Spec.DeploymentMode == ModeStandalone {
			return errors.New(fmt.Sprintf("MultiClusterEngine in Standalone mode already exists: `%s`. Only one resource may exist in Standalone mode.", mce.Name))
		}
	}
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *MultiClusterEngine) ValidateUpdate(old runtime.Object) error {
	backplaneconfiglog.Info("validate update", "name", r.Name)

	oldMCE := old.(*MultiClusterEngine)
	backplaneconfiglog.Info(oldMCE.Spec.TargetNamespace)
	if (r.Spec.TargetNamespace != oldMCE.Spec.TargetNamespace) && (oldMCE.Spec.TargetNamespace != "") {
		return errors.New("changes cannot be made to target namespace")
	}
	if r.Spec.DeploymentMode != oldMCE.Spec.DeploymentMode {
		return errors.New("changes cannot be made to DeploymentMode")
	}

	oldNS, newNS := "", ""
	if oldMCE.Spec.Overrides != nil {
		oldNS = oldMCE.Spec.Overrides.InfrastructureCustomNamespace
	}
	if r.Spec.Overrides != nil {
		newNS = r.Spec.Overrides.InfrastructureCustomNamespace
	}
	if oldNS != newNS {
		return errors.New("changes cannot be made to InfrastructureCustomNamespace")
	}

	if (r.Spec.AvailabilityConfig != HABasic) && (r.Spec.AvailabilityConfig != HAHigh) && (r.Spec.AvailabilityConfig != "") {
		return errors.New("Invalid AvailabilityConfig given")
	}
	if (r.Spec.DeploymentMode != ModeHosted) && (r.Spec.DeploymentMode != ModeStandalone) && (r.Spec.DeploymentMode != "") {
		return errors.New("Invalid DeploymentMode given")
	}

	// Validate components
	if r.Spec.Overrides != nil {
		for _, c := range r.Spec.Overrides.Components {
			if !validComponent(c) {
				return errors.New(fmt.Sprintf("invalid component config: %s is not a known component", c.Name))
			}
		}
	}

	// Block disable if relevant resources present
	if r.ComponentPresent(Discovery) && !r.Enabled(Discovery) {
		cfg, err := config.GetConfig()
		if err != nil {
			return err
		}

		c, err := discovery.NewDiscoveryClientForConfig(cfg)
		if err != nil {
			return err
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
			if err := Client.List(context.TODO(), list); err != nil {
				return fmt.Errorf("unable to list %s: %s", "DiscoveryConfig", err)
			}
			if len(list.Items) != 0 {
				return fmt.Errorf("existing %s resources must first be deleted", "DiscoveryConfig")
			}
		}
	}

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *MultiClusterEngine) ValidateDelete() error {
	// TODO(user): fill in your validation logic upon object deletion.
	backplaneconfiglog.Info("validate delete", "name", r.Name)
	ctx := context.Background()

	cfg, err := config.GetConfig()
	if err != nil {
		return err
	}

	c, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return err
	}

	for _, resource := range blockDeletionResources {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(resource.GVK)
		err := discovery.ServerSupportsVersion(c, list.GroupVersionKind().GroupVersion())
		if err == nil {
			if err := Client.List(ctx, list); err != nil {
				return fmt.Errorf("unable to list %s: %s", resource.Name, err)
			}
			if len(list.Items) == 0 {
				continue
			}
			return fmt.Errorf("cannot delete %s resource. Existing %s resources must first be deleted", r.Name, resource.Name)
		}
	}
	return nil
}
