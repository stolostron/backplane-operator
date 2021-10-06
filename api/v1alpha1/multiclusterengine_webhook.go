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

package v1alpha1

import (
	"context"
	"errors"
	"fmt"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"

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

// log is for logging in this package.
var (
	backplaneconfiglog = logf.Log.WithName("backplaneconfig-resource")
	Client             cl.Client

	blockCreationResources = []struct {
		Name string
		GVK  schema.GroupVersionKind
	}{
		{
			Name: "MultiClusterHub",
			GVK: schema.GroupVersionKind{
				Group:   "operator.open-cluster-management.io",
				Version: "v1",
				Kind:    "MultiClusterHubList",
			},
		},
	}

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

	// TODO(user): fill in your defaulting logic.
}

var _ webhook.Validator = &MultiClusterEngine{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *MultiClusterEngine) ValidateCreate() error {
	ctx := context.Background()
	backplaneconfiglog.Info("validate create", "name", r.Name)

	cfg, err := config.GetConfig()
	if err != nil {
		return err
	}

	c, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return err
	}

	for _, resource := range blockCreationResources {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(resource.GVK)
		err := discovery.ServerSupportsVersion(c, list.GroupVersionKind().GroupVersion())
		if err == nil {
			if err := Client.List(ctx, list); err != nil {
				// Server may support Group Version, but explicitly also exempt Kind
				if strings.Contains(err.Error(), "no matches for kind") || k8serrors.IsNotFound(err) {
					continue
				}
				return fmt.Errorf("unable to list %s: %s", resource.Name, err)
			}
			if len(list.Items) == 0 {
				continue
			}
			return fmt.Errorf("cannot create %s resource. Existing %s resources must first be deleted", r.Name, resource.Name)
		}
	}

	backplaneConfigList := &MultiClusterEngineList{}
	if err := Client.List(context.TODO(), backplaneConfigList); err != nil {
		return fmt.Errorf("unable to list BackplaneConfigs: %s", err)
	}
	if len(backplaneConfigList.Items) == 0 {
		return nil
	}
	// TODO(user): fill in your validation logic upon object creation.
	return errors.New("only 1 backplaneconfig resource may exist")
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *MultiClusterEngine) ValidateUpdate(old runtime.Object) error {
	backplaneconfiglog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
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
