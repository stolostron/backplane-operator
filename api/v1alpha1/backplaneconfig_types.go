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

// BackplaneConfigSpec defines the desired state of BackplaneConfig
type BackplaneConfigSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of BackplaneConfig. Edit backplaneconfig_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// BackplaneConfigStatus defines the observed state of BackplaneConfig
type BackplaneConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// BackplaneConfig is the Schema for the backplaneconfigs API
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
