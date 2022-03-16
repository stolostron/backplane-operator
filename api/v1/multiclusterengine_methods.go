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
	"errors"
	"fmt"
)

const (
	ManagedServiceAccount string = "managed-service-account"
	ConsoleMCE            string = "console-mce"
	Discovery             string = "discovery"
	Hive                  string = "hive"
	AssistedService       string = "assisted-service"
	ClusterLifecycle      string = "cluster-lifecycle"
	ClusterManager        string = "cluster-manager"
	ServerFoundation      string = "server-foundation"
	HyperShift            string = "hypershift-preview"
)

var allComponents = []string{
	AssistedService,
	ClusterLifecycle,
	ClusterManager,
	Discovery,
	Hive,
	ServerFoundation,
	ConsoleMCE,
	ManagedServiceAccount,
	HyperShift,
}

var requiredComponents = []string{
	ServerFoundation,
}

func (mce *MultiClusterEngine) ComponentPresent(s string) bool {
	for _, c := range mce.Spec.Components {
		if c.Name == s {
			return true
		}
	}
	return false
}

func (mce *MultiClusterEngine) Enabled(s string) bool {
	for _, c := range mce.Spec.Components {
		if c.Name == s {
			return c.Enabled
		}
	}

	return false
}

func (mce *MultiClusterEngine) Enable(s string) {
	for i, c := range mce.Spec.Components {
		if c.Name == s {
			mce.Spec.Components[i].Enabled = true
			return
		}
	}
	mce.Spec.Components = append(mce.Spec.Components, ComponentConfig{
		Name:    s,
		Enabled: true,
	})
}

func (mce *MultiClusterEngine) Disable(s string) {
	for i, c := range mce.Spec.Components {
		if c.Name == s {
			mce.Spec.Components[i].Enabled = false
			return
		}
	}
	mce.Spec.Components = append(mce.Spec.Components, ComponentConfig{
		Name:    s,
		Enabled: false,
	})
}

// a component is valid if its name matches a known component
func validComponent(c ComponentConfig) bool {
	for _, name := range allComponents {
		if c.Name == name {
			return true
		}
	}
	return false
}

func requiredComponentsPresentCheck(mce *MultiClusterEngine) error {
	for _, req := range requiredComponents {
		if mce.ComponentPresent(req) && !mce.Enabled(req) {
			return errors.New(fmt.Sprintf("invalid component config: %s can not be disabled", req))
		}
	}
	return nil
}
