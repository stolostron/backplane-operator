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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type PhaseType string

const (
	Pending      PhaseType = "Pending"
	Running      PhaseType = "Running"
	Installing   PhaseType = "Installing"
	Updating     PhaseType = "Updating"
	Uninstalling PhaseType = "Uninstalling"
)

// BackplaneConfigSpec defines the desired state of BackplaneConfig
type BackplaneConfigSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of BackplaneConfig. Edit backplaneconfig_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// BackplaneConfigStatus defines the observed state of BackplaneConfig
type BackplaneConfigStatus struct {
	// Important: Run "make" to regenerate code after modifying this file

	// Represents the running phase of the BackplaneConfig
	// +optional
	Phase PhaseType `json:"phase"`

	// CurrentVersion indicates the current version
	CurrentVersion string `json:"currentVersion,omitempty"`

	// DesiredVersion indicates the desired version
	DesiredVersion string `json:"desiredVersion,omitempty"`

	// Conditions contains the different condition statuses for the BackplaneConfig
	Conditions []Condition `json:"conditions,omitempty"`

	// Components contains the different statuses for the BackplaneConfig components
	Components map[string]StatusCondition `json:"components,omitempty"`
}

// StatusCondition contains condition information.
type StatusCondition struct {
	// The resource kind this condition represents
	Kind string `json:"-"`

	// Available indicates whether this component is considered properly running
	Available bool `json:"-"`

	// Type is the type of the cluster condition.
	// +required
	Type string `json:"type"`

	// Status is the status of the condition. One of True, False, Unknown.
	// +required
	Status metav1.ConditionStatus `json:"status"`

	// The last time this condition was updated.
	LastUpdateTime metav1.Time `json:"-"`

	// LastTransitionTime is the last time the condition changed from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// Reason is a (brief) reason for the condition's last status change.
	// +required
	Reason string `json:"reason"`

	// Message is a human-readable message indicating details about the last status change.
	// +required
	Message string `json:"message"`
}

type ConditionType string

const (
	// Progressing means the deployment is progressing.
	Progressing ConditionType = "Progressing"

	// Complete means that all desired components are configured and in a running state.
	Complete ConditionType = "Complete"

	// Terminating means that the backplaneConfig has been deleted and is cleaning up.
	Terminating ConditionType = "Terminating"
)

// Condition contains condition information.
type Condition struct {
	// Type is the type of the cluster condition.
	// +required
	Type ConditionType `json:"type,omitempty"`

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

// BackplaneConfig is the Schema for the backplaneconfigs API
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase",description="The overall status of the backplaneconfig"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
//+operator-sdk:csv:customresourcedefinitions:displayName="BackplaneConfig"
type BackplaneConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackplaneConfigSpec   `json:"spec,omitempty"`
	Status BackplaneConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BackplaneConfigList contains a list of BackplaneConfig
type BackplaneConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackplaneConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BackplaneConfig{}, &BackplaneConfigList{})
}
