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
	managedServiceAccountEnabledDefault = false
	consoleMCEEnabledDefault            = false
)

func (mce *MultiClusterEngine) ComponentEnabled(c ComponentEnabled) bool {
	switch c {
	case ManagedServiceAccount:
		return mce.managedServiceAccountEnabled()
	case ConsoleMCE:
		return mce.consoleMCEEnabled()
	default:
		return false
	}
}

func (mce *MultiClusterEngine) hasComponentConfig() bool {
	return mce.Spec.ComponentConfig != nil
}

func (mce *MultiClusterEngine) hasManagedServiceAccountConfig() bool {
	if !mce.hasComponentConfig() {
		return false
	}
	return mce.Spec.ComponentConfig.ManagedServiceAccount != nil
}

func (mce *MultiClusterEngine) hasConsoleMCEConfig() bool {
	if !mce.hasComponentConfig() {
		return false
	}
	return mce.Spec.ComponentConfig.ConsoleMCE != nil
}

func (mce *MultiClusterEngine) managedServiceAccountEnabled() bool {
	if !mce.hasManagedServiceAccountConfig() {
		return managedServiceAccountEnabledDefault
	}
	return mce.Spec.ComponentConfig.ManagedServiceAccount.Enable
}

func (mce *MultiClusterEngine) consoleMCEEnabled() bool {
	if !mce.hasConsoleMCEConfig() {
		return consoleMCEEnabledDefault
	}
	return mce.Spec.ComponentConfig.ConsoleMCE.Enable
}
