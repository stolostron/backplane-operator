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
	"fmt"
)

const (
	// Component names
	AssistedService                  = "assisted-service"
	ClusterAPI                       = "cluster-api"
	ClusterAPIPreview                = "cluster-api-preview"
	ClusterAPIProviderAWS            = "cluster-api-provider-aws"
	ClusterAPIProviderAWSPreview     = "cluster-api-provider-aws-preview"
	ClusterAPIProviderMetal          = "cluster-api-provider-metal3"
	ClusterAPIProviderMetalPreview   = "cluster-api-provider-metal3-preview"
	ClusterAPIProviderOA             = "cluster-api-provider-openshift-assisted"
	ClusterAPIProviderOAPreview      = "cluster-api-provider-openshift-assisted-preview"
	ClusterLifecycle                 = "cluster-lifecycle"
	ClusterManager                   = "cluster-manager"
	ClusterProxyAddon                = "cluster-proxy-addon"
	ConsoleMCE                       = "console-mce"
	Discovery                        = "discovery"
	Hive                             = "hive"
	HyperShift                       = "hypershift"
	HypershiftLocalHosting           = "hypershift-local-hosting"
	HyperShiftPreview                = "hypershift-preview"
	ImageBasedInstallOperator        = "image-based-install-operator"
	ImageBasedInstallOperatorPreview = "image-based-install-operator-preview"
	LocalCluster                     = "local-cluster"
	ManagedServiceAccount            = "managedserviceaccount"
	ManagedServiceAccountPreview     = "managedserviceaccount-preview"
	ServerFoundation                 = "server-foundation"

	// CRD directory names
	AssistedServiceCRDDir            = "assisted-service"
	ClusterAPICRDDir                 = "cluster-api"
	ClusterAPIK8SCRDDir              = "cluster-api-k8s"
	ClusterAPIProviderAWSCRDDir      = "cluster-api-provider-aws"
	ClusterAPIProviderMetalCRDDir    = "cluster-api-provider-metal3"
	ClusterAPIProviderMetalK8SCRDDir = "cluster-api-provider-metal3-k8s"
	ClusterAPIProviderOACRDDir       = "cluster-api-provider-openshift-assisted"
	ClusterAPIProviderOAK8SCRDDir    = "cluster-api-provider-openshift-assisted-k8s"
	ClusterLifecycleCRDDir           = "cluster-lifecycle"
	ClusterManagerCRDDir             = "cluster-manager"
	ClusterProxyAddonCRDDir          = "cluster-proxy-addon"
	DiscoveryCRDDir                  = "discovery-operator"
	HiveCRDDir                       = "hive-operator"
	ImageBasedInstallOperatorCRDDir  = "image-based-install-operator"
	ManagedServiceAccountCRDDir      = "managed-serviceaccount"
	ServerFoundationCRDDir           = "foundation"
)

// AllComponents is a slice containing all valid component names
var AllComponents = []string{
	AssistedService,
	ClusterAPI,
	ClusterAPIPreview,
	ClusterAPIProviderAWS,
	ClusterAPIProviderAWSPreview,
	ClusterAPIProviderMetal,
	ClusterAPIProviderOAPreview,
	ClusterAPIProviderMetalPreview,
	ClusterLifecycle,
	ClusterManager,
	ClusterProxyAddon,
	ConsoleMCE,
	Discovery,
	Hive,
	HyperShift,
	HypershiftLocalHosting,
	HyperShiftPreview,
	ImageBasedInstallOperator,
	ImageBasedInstallOperatorPreview,
	LocalCluster,
	ManagedServiceAccount,
	ManagedServiceAccountPreview,
	ServerFoundation,
}

// MCEComponents is a slice containing component names specific to the "MCE" category.
var MCEComponents = []string{
	AssistedService,
	ClusterAPI,
	ClusterAPIProviderAWS,
	ClusterAPIProviderMetalPreview,
	ClusterAPIProviderOAPreview,
	ClusterLifecycle,
	ClusterManager,
	ClusterProxyAddon,
	ConsoleMCE,
	Discovery,
	Hive,
	HyperShift,
	HypershiftLocalHosting,
	ImageBasedInstallOperator,
	ManagedServiceAccount,
	ServerFoundation,
}

/*
PreviewComponents is a list of all MCE component names that represent preview (tech preview) features.
These components are candidates for pruning when the stable version replaces them.
*/
var PreviewComponents = []string{
	ClusterAPIPreview,
	ClusterAPIProviderAWSPreview,
	HyperShiftPreview,
	ImageBasedInstallOperatorPreview,
	ManagedServiceAccountPreview,
}

/*
PreviewToStable maps each preview component to its corresponding stable replacement.
This mapping is used to enable the stable component when the preview version is pruned.
*/
var PreviewToStable = map[string]string{
	ClusterAPIPreview:                ClusterAPI,                // Upgraded in ACM 2.14 / MCE 2.9
	ClusterAPIProviderAWSPreview:     ClusterAPIProviderAWS,     // Upgraded in ACM 2.14 / MCE 2.9
	HyperShiftPreview:                HyperShift,                // Upgraded in ACM 2.8 / MCE 2.3
	ImageBasedInstallOperatorPreview: ImageBasedInstallOperator, // Upgraded in ACM 2.12 / MCE 2.7
	ManagedServiceAccountPreview:     ManagedServiceAccount,     // Upgraded in ACM 2.9 / MCE 2.4
}

/*
LegacyConfigKind is a slice of strings that represents the legacy resource kinds
supported by the Operator SDK and Prometheus. These kinds include "PrometheusRule", "Service",
and "ServiceMonitor".
*/
var LegacyConfigKind = []string{"PrometheusRule", "ServiceMonitor"}

// MCELegacyPrometheusRules is a map that associates certain component names with their corresponding prometheus rules.
var MCELegacyPrometheusRules = map[string]string{
	ConsoleMCE: "acm-console-prometheus-rules",
	// Add other components here when PrometheusRules is required.
}

// MCELegacyServiceMonitors is a map that associates certain component names with their corresponding service monitors.
var MCELegacyServiceMonitors = map[string]string{
	ClusterLifecycle: "clusterlifecycle-state-metrics-v2",
	ConsoleMCE:       "console-mce-monitor",
	// Add other components here when ServiceMonitors is required.
}

/*
ComponentPresent checks if a component with the given name is present in the MultiClusterEngine's Overrides.
Returns true if the component is present, otherwise false.
*/
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

/*
Enabled checks if a component with the given name is enabled in the MultiClusterEngine's Overrides.
Returns true if the component is enabled, otherwise false.
*/
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

/*
Enable enables a component with the given name in the MultiClusterEngine's Overrides.
If the component is not present, it adds it and sets it as enabled.
*/
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
		ConfigOverrides: ConfigOverride{},
		Enabled:         true,
		Name:            s,
	})
}

/*
Prune removes a component with the given name from the MultiClusterEngine's Overrides.
Returns true if the component is pruned, indicating changes were made.
*/
func (mce *MultiClusterEngine) Prune(s string) bool {
	if mce.Spec.Overrides == nil {
		return false
	}
	pruned := false
	prunedList := []ComponentConfig{}
	for _, c := range mce.Spec.Overrides.Components {
		if c.Name == s {
			pruned = true
		} else {
			prunedList = append(prunedList, c)
		}
	}

	if pruned {
		mce.Spec.Overrides.Components = prunedList
		return true
	}
	return false
}

/*
Disable disables a component with the given name in the MultiClusterEngine's Overrides.
If the component is not present, it adds it and sets it as disabled.
*/
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
		ConfigOverrides: ConfigOverride{},
		Enabled:         false,
		Name:            s,
	})
}

/*
validComponent checks if a ComponentConfig is valid by comparing its name to a list of known component names.
Returns true if the component is valid, otherwise false.
*/
func validComponent(c ComponentConfig) bool {
	for _, name := range AllComponents {
		if c.Name == name {
			return true
		}
	}
	return false
}

/*
IsInHostedMode checks if the MultiClusterEngine has an annotation indicating it is in hosted mode.
Returns true if the annotation is present and its value is "ModeHosted," otherwise false.
*/
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

/*
GetLegacyConfigKind returns a list of legacy kind resources that are required to be removed before updating to
MCE 2.4 and later.
*/
func GetLegacyConfigKind() []string {
	return LegacyConfigKind
}

// GetLegacyPrometheusRulesName returns the name of the PrometheusRules based on the provided component name.
func GetLegacyPrometheusRulesName(component string) (string, error) {
	if val, ok := MCELegacyPrometheusRules[component]; !ok {
		return val, fmt.Errorf("failed to find PrometheusRules name for: %s component", component)
	} else {
		return val, nil
	}
}

// GetLegacyServiceMonitorName returns the name of the ServiceMonitors based on the provided component name.
func GetLegacyServiceMonitorName(component string) (string, error) {
	if val, ok := MCELegacyServiceMonitors[component]; !ok {
		return val, fmt.Errorf("failed to find ServiceMonitors name for: %s component", component)
	} else {
		return val, nil
	}
}

// IsACMManaged returns true if operator is managed by ACM
func IsACMManaged(mce *MultiClusterEngine) bool {
	managedByACMLabel := "multiclusterhubs.operator.open-cluster-management.io/managed-by"
	if labels := mce.GetLabels(); labels != nil {
		if labels[managedByACMLabel] == "true" {
			return true
		}
	}
	return false
}
