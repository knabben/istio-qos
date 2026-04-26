// Package v1alpha1 contains the PodLabelerPolicy CRD types.
//
// Hand-written, no kubebuilder. The DeepCopy methods at the bottom
// of this file are normally generated; here they are written by hand
// for the POC.
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupVersion is the group/version for this API.
var GroupVersion = schema.GroupVersion{
	Group:   "labeling.knabben.dev",
	Version: "v1alpha1",
}

// PodLabelerPolicy is a cluster-scoped CRD. It tells the controller
// which labels to apply to pods whose container image matches a pattern.
//
// Example:
//
//	apiVersion: labeling.knabben.dev/v1alpha1
//	kind: PodLabelerPolicy
//	metadata:
//	  name: app1-tier-high
//	spec:
//	  imagePattern: "app1:*"
//	  labels:
//	    tier: high
//	    customer: premium
type PodLabelerPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PodLabelerPolicySpec `json:"spec,omitempty"`
}

// PodLabelerPolicySpec defines the matching rule and labels to apply.
type PodLabelerPolicySpec struct {
	// ImagePattern matches against the pod's primary container image.
	// Supported forms:
	//   - "app1:latest"   exact match
	//   - "app1:*"        wildcard tag (any tag of app1)
	//   - "app1"          prefix match (anything starting with app1)
	// Registry prefixes (e.g. "registry.io/team/") are stripped before matching.
	ImagePattern string `json:"imagePattern"`

	// Labels are the labels to apply to matching pods.
	Labels map[string]string `json:"labels"`
}

// PodLabelerPolicyList is the list type for PodLabelerPolicy.
type PodLabelerPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodLabelerPolicy `json:"items"`
}

// ----- DeepCopy methods (normally generated) -----

func (in *PodLabelerPolicy) DeepCopyInto(out *PodLabelerPolicy) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

func (in *PodLabelerPolicy) DeepCopy() *PodLabelerPolicy {
	if in == nil {
		return nil
	}
	out := new(PodLabelerPolicy)
	in.DeepCopyInto(out)
	return out
}

func (in *PodLabelerPolicy) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

func (in *PodLabelerPolicySpec) DeepCopyInto(out *PodLabelerPolicySpec) {
	*out = *in
	if in.Labels != nil {
		out.Labels = make(map[string]string, len(in.Labels))
		for k, v := range in.Labels {
			out.Labels[k] = v
		}
	}
}

func (in *PodLabelerPolicyList) DeepCopyInto(out *PodLabelerPolicyList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]PodLabelerPolicy, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *PodLabelerPolicyList) DeepCopy() *PodLabelerPolicyList {
	if in == nil {
		return nil
	}
	out := new(PodLabelerPolicyList)
	in.DeepCopyInto(out)
	return out
}

func (in *PodLabelerPolicyList) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}
