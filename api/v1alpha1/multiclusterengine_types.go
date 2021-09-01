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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MultiClusterEngineSpec defines the desired state of MultiClusterEngine
type MultiClusterEngineSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of MultiClusterEngine. Edit multiclusterengine_types.go to remove/update
	Foo string `json:"foo,omitempty"`

	// Set the nodeselectors
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

// MultiClusterEngineStatus defines the observed state of MultiClusterEngine
type MultiClusterEngineStatus struct {
	// Latest observed overall state
	Phase PhaseType `json:"phase,omitempty"`

	Components []ComponentCondition `json:"components,omitempty"`

	Conditions []MultiClusterEngineCondition `json:"conditions,omitempty"`
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
	MultiClusterEnginePhaseProgressing PhaseType = "Progressing"
	MultiClusterEnginePhaseAvailable   PhaseType = "Available"
	MultiClusterEnginePhaseError       PhaseType = "Error"
)

type MultiClusterEngineConditionType string

// These are valid conditions of the multiclusterengine.
const (
	// Available means the deployment is available, ie. at least the minimum available
	// replicas required are up and running for at least minReadySeconds.
	MultiClusterEngineAvailable MultiClusterEngineConditionType = "Available"
	// Progressing means the deployment is progressing. Progress for a deployment is
	// considered when a new replica set is created or adopted, and when new pods scale
	// up or old pods scale down. Progress is not estimated for paused deployments or
	// when progressDeadlineSeconds is not specified.
	MultiClusterEngineProgressing MultiClusterEngineConditionType = "Progressing"
	// Failure is added in a deployment when one of its pods fails to be created
	// or deleted.
	MultiClusterEngineFailure MultiClusterEngineConditionType = "MultiClusterEngineFailure"
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
//+kubebuilder:resource:scope=Cluster,shortName=me

// MultiClusterEngine is the Schema for the multiclusterengines API
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase",description="The overall state of the MultiClusterEngine"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
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

func init() {
	SchemeBuilder.Register(&MultiClusterEngine{}, &MultiClusterEngineList{})
}
