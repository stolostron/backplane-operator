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

const (
	ManagedServiceAccount  = "managedserviceaccount-preview"
	ConsoleMCE             = "console-mce"
	Discovery              = "discovery"
	Hive                   = "hive"
	AssistedService        = "assisted-service"
	ClusterLifecycle       = "cluster-lifecycle"
	ClusterManager         = "cluster-manager"
	ServerFoundation       = "server-foundation"
	HyperShift             = "hypershift-preview"
	ClusterProxyAddon      = "cluster-proxy-addon"
	HypershiftLocalHosting = "hypershift-local-hosting"
	LocalCluster           = "local-cluster"
	AddOnManager           = "addon-manager"
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
	HypershiftLocalHosting,
	ClusterProxyAddon,
	LocalCluster,
	AddOnManager,
}

func (mce *MultiClusterEngine) ComponentPresent(s string) bool {
	if mce.Spec.Overrides == nil {
		return false
	}
	for _, c := range mce.Spec.Overrides.Components {
		if c.Name == s {
			return true
		}
	}
	return false
}

func (mce *MultiClusterEngine) Enabled(s string) bool {
	if mce.Spec.Overrides == nil {
		return false
	}
	for _, c := range mce.Spec.Overrides.Components {
		if c.Name == s {
			return c.Enabled
		}
	}

	return false
}

func (mce *MultiClusterEngine) Enable(s string) {
	if mce.Spec.Overrides == nil {
		mce.Spec.Overrides = &Overrides{}
	}
	for i, c := range mce.Spec.Overrides.Components {
		if c.Name == s {
			mce.Spec.Overrides.Components[i].Enabled = true
			return
		}
	}
	mce.Spec.Overrides.Components = append(mce.Spec.Overrides.Components, ComponentConfig{
		Name:    s,
		Enabled: true,
	})
}

func (mce *MultiClusterEngine) Disable(s string) {
	if mce.Spec.Overrides == nil {
		mce.Spec.Overrides = &Overrides{}
	}
	for i, c := range mce.Spec.Overrides.Components {
		if c.Name == s {
			mce.Spec.Overrides.Components[i].Enabled = false
			return
		}
	}
	mce.Spec.Overrides.Components = append(mce.Spec.Overrides.Components, ComponentConfig{
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

func IsInHostedMode(mce *MultiClusterEngine) bool {
	a := mce.GetAnnotations()
	if a == nil {
		return false
	}
	if a["deploymentmode"] == string(ModeHosted) {
		return true
	}
	return false
}
