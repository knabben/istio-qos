/*
Copyright 2026.

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

// PodLabelerPolicySpec defines the desired state of PodLabelerPolicy.
type PodLabelerPolicySpec struct {
	// imagePattern is a glob pattern matched against each container image in a pod.
	// A pod is matched when at least one container image matches this pattern.
	// Examples: "nginx:*", "*/myapp:v1.*", "registry.example.com/team/app:*"
	// +kubebuilder:validation:MinLength=1
	ImagePattern string `json:"imagePattern"`

	// tier is the traffic classification label applied to matched pods.
	// +kubebuilder:validation:Enum=high;standard
	Tier string `json:"tier"`
}

// PodLabelerPolicyStatus defines the observed state of PodLabelerPolicy.
type PodLabelerPolicyStatus struct {
	// matchedPods is the number of pods currently labeled by this policy.
	// +optional
	MatchedPods int32 `json:"matchedPods,omitempty"`

	// conditions represent the current state of the PodLabelerPolicy resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="ImagePattern",type=string,JSONPath=`.spec.imagePattern`
// +kubebuilder:printcolumn:name="Tier",type=string,JSONPath=`.spec.tier`
// +kubebuilder:printcolumn:name="MatchedPods",type=integer,JSONPath=`.status.matchedPods`

// PodLabelerPolicy is the Schema for the podlabelerpolicies API
type PodLabelerPolicy struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of PodLabelerPolicy
	// +required
	Spec PodLabelerPolicySpec `json:"spec"`

	// status defines the observed state of PodLabelerPolicy
	// +optional
	Status PodLabelerPolicyStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PodLabelerPolicyList contains a list of PodLabelerPolicy
type PodLabelerPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []PodLabelerPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PodLabelerPolicy{}, &PodLabelerPolicyList{})
}
