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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AvailabilityType ...
type AvailabilityType string

// DeploymentMode
type DeploymentMode string

const (
	// HABasic stands up most app subscriptions with a replicaCount of 1
	HABasic AvailabilityType = "Basic"
	// HAHigh stands up most app subscriptions with a replicaCount of 2
	HAHigh AvailabilityType = "High"
	// ModeHosted deploys the MCE on a hosted virtual cluster
	ModeHosted DeploymentMode = "Hosted"
	// ModeStandalone deployos the MCE in the default manner
	ModeStandalone DeploymentMode = "Standalone"
)

// MultiClusterEngineSpec defines the desired state of MultiClusterEngine
type MultiClusterEngineSpec struct {

	// Specifies deployment replication for improved availability. Options are: Basic and High (default)
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Availability Configuration",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced","urn:alm:descriptor:com.tectonic.ui:select:High","urn:alm:descriptor:com.tectonic.ui:select:Basic"}
	AvailabilityConfig AvailabilityType `json:"availabilityConfig,omitempty"`

	// Set the nodeselectors
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Override pull secret for accessing MultiClusterEngine operand and endpoint images
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Image Pull Secret",xDescriptors={"urn:alm:descriptor:io.kubernetes:Secret","urn:alm:descriptor:com.tectonic.ui:advanced"}
	ImagePullSecret string `json:"imagePullSecret,omitempty"`

	// Developer Overrides
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Developer Overrides",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:hidden"}
	Overrides *Overrides `json:"overrides,omitempty"`

	// Tolerations causes all components to tolerate any taints.
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Location where MCE resources will be placed
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Target Namespace",xDescriptors={"urn:alm:descriptor:io.kubernetes:text","urn:alm:descriptor:com.tectonic.ui:advanced"}
	TargetNamespace string `json:"targetNamespace,omitempty"`

	// The name of the local-cluster resource
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Local Cluster Name",xDescriptors={"urn:alm:descriptor:io.kubernetes:text","urn:alm:descriptor:com.tectonic.ui:advanced"}
	//+kubebuilder:default="local-cluster"
	LocalClusterName string `json:"localClusterName,omitempty"`
}

// ComponentConfig provides optional configuration items for individual components
type ComponentConfig struct {
	// Enabled specifies whether the component is enabled or disabled.
	Enabled bool `json:"enabled"`

	// Name denotes the name of the component being configured.
	Name string `json:"name"`

	// ConfigOverrides contains optional configuration overrides for deployments and containers.
	ConfigOverrides ConfigOverride `json:"configOverrides,omitempty"`
}

// ConfigOverride holds overrides for configurations specific to deployments and containers.
type ConfigOverride struct {
	// Deployments is a list of deployment specific configuration overrides.
	Deployments []DeploymentConfig `json:"deployments,omitempty"`
}

// DeploymentConfig provides configuration details for a specific deployment.
type DeploymentConfig struct {
	// Name specifies the name of the deployment being configured.
	Name string `json:"name"`

	// Containers is a list of container specific configurations within the deployment.
	Containers []ContainerConfig `json:"containers"`
}

// ContainerConfig holds configuration details for a specific container within a deployment.
type ContainerConfig struct {
	// Name specifies the name of the container being configured.
	Name string `json:"name"`

	// Env is a list of environment variable overrides for the container.
	Env []EnvConfig `json:"env"`
}

// EnvConfig represents an override for an environment variable within a container.
type EnvConfig struct {
	// Name specifies the name of the environment variable.
	Name string `json:"name,omitempty"`

	// Value specifies the value of the environment variable.
	Value string `json:"value,omitempty"`
}

// Overrides provides developer overrides for MCE installation
type Overrides struct {
	// Pull policy for the MCE images
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// Provides optional configuration for components, the list of which can be found here: https://github.com/stolostron/backplane-operator/tree/main/docs/available-components.md
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Component Configuration",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:hidden"}
	// +optional
	Components []ComponentConfig `json:"components,omitempty"`

	// Namespace to install Assisted Installer operator
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Custom Infrastructure Operator Namespace",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:hidden"}
	// +optional
	InfrastructureCustomNamespace string `json:"infrastructureCustomNamespace,omitempty"`
}

// MultiClusterEngineStatus defines the observed state of MultiClusterEngine
type MultiClusterEngineStatus struct {
	// Latest observed overall state
	Phase PhaseType `json:"phase,omitempty"`

	Components []ComponentCondition `json:"components,omitempty"`

	Conditions []MultiClusterEngineCondition `json:"conditions,omitempty"`

	// CurrentVersion is the most recent version successfully installed
	CurrentVersion string `json:"currentVersion,omitempty"`

	// DesiredVersion is the version the operator is reconciling towards
	DesiredVersion string `json:"desiredVersion,omitempty"`
}

// ComponentCondition contains condition information for tracked components
type ComponentCondition struct {
	// The component name
	Name string `json:"name,omitempty"`

	// The resource kind this condition represents
	Kind string `json:"kind,omitempty"`

	// Available indicates whether this component is considered properly running
	Available bool `json:"-"`

	// Type is the type of the cluster condition.
	// +required
	Type string `json:"type,omitempty"`

	// Status is the status of the condition. One of True, False, Unknown.
	// +required
	Status metav1.ConditionStatus `json:"status,omitempty"`

	// The last time this condition was updated.
	LastUpdateTime metav1.Time `json:"-"`

	// LastTransitionTime is the last time the condition changed from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// Reason is a (brief) reason for the condition's last status change.
	// +required
	Reason string `json:"reason,omitempty"`

	// Message is a human-readable message indicating details about the last status change.
	// +required
	Message string `json:"message,omitempty"`
}

// PhaseType is a summary of the current state of the MultiClusterEngine in its lifecycle
type PhaseType string

const (
	MultiClusterEnginePhaseProgressing   PhaseType = "Progressing"
	MultiClusterEnginePhasePaused        PhaseType = "Paused"
	MultiClusterEnginePhaseAvailable     PhaseType = "Available"
	MultiClusterEnginePhaseUninstalling  PhaseType = "Uninstalling"
	MultiClusterEnginePhaseError         PhaseType = "Error"
	MultiClusterEnginePhaseUnimplemented PhaseType = "Unimplemented"
	MultiClusterEnginePhaseUpdating      PhaseType = "Updating"
)

type MultiClusterEngineConditionType string

// These are valid conditions of the multiclusterengine.
const (
	/*
		Available means the deployment is available, ie. at least the minimum available
		replicas required are up and running for at least minReadySeconds.
	*/
	MultiClusterEngineAvailable MultiClusterEngineConditionType = "Available"
	/*
		Progressing means the deployment is progressing. Progress for a deployment is
		considered when a new replica set is created or adopted, and when new pods scale
		up or old pods scale down. Progress is not estimated for paused deployments or
		when progressDeadlineSeconds is not specified.
	*/
	MultiClusterEngineProgressing MultiClusterEngineConditionType = "Progressing"
	/*
		ComponentFailure is added in a deployment when one of its pods fails to be created
		or deleted.
	*/
	MultiClusterEngineComponentFailure MultiClusterEngineConditionType = "ComponentFailure"
	/*
		Failure is added in a deployment when one of its pods fails to be created
		or deleted.
	*/
	MultiClusterEngineFailure MultiClusterEngineConditionType = "Failure"
	/*
		ComponentsExternallyManaged indicates that one or more components are being managed
		externally and MCE will not reconcile them.
	*/
	MultiClusterEngineComponentsExternallyManaged MultiClusterEngineConditionType = "ComponentsExternallyManaged"
)

type MultiClusterEngineCondition struct {
	// Type is the type of the cluster condition.
	// +required
	Type MultiClusterEngineConditionType `json:"type,omitempty"`

	// Status is the status of the condition. One of True, False, Unknown.
	// +required
	Status metav1.ConditionStatus `json:"status,omitempty"`

	// The last time this condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`

	// LastTransitionTime is the last time the condition changed from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// Reason is a (brief) reason for the condition's last status change.
	// +required
	Reason string `json:"reason,omitempty"`

	// Message is a human-readable message indicating details about the last status change.
	// +required
	Message string `json:"message,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster,shortName=mce

// MultiClusterEngine defines the configuration for an instance
// of a multicluster engine, a central point providing the foundational components for managing multiple Kubernetes-based clusters. The deployment of the multicluster engine components is
// determined based on the configuration that is defined in this resource.
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase",description="The overall state of the MultiClusterEngine"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="CurrentVersion",type="string",JSONPath=".status.currentVersion",description="The current version of the MultiClusterEngine"
// +kubebuilder:printcolumn:name="DesiredVersion",type="string",JSONPath=".status.desiredVersion",description="The desired version of the MultiClusterEngine"
// +operator-sdk:csv:customresourcedefinitions:displayName="MultiCluster Engine"
type MultiClusterEngine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MultiClusterEngineSpec   `json:"spec,omitempty"`
	Status MultiClusterEngineStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MultiClusterEngineList contains a list of MultiClusterEngine
type MultiClusterEngineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MultiClusterEngine `json:"items"`
}

// +kubebuilder:object:root=true
// +operator-sdk:csv:customresourcedefinitions:displayName="InternalEngineComponent"
type InternalEngineComponent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              InternalEngineComponentSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
type InternalEngineComponentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InternalEngineComponent `json:"items"`
}

type InternalEngineComponentSpec struct{}

func init() {
	SchemeBuilder.Register(&MultiClusterEngine{}, &MultiClusterEngineList{})
	SchemeBuilder.Register(&InternalEngineComponent{}, &InternalEngineComponentList{})
}
