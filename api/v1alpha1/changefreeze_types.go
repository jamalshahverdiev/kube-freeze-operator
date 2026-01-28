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

// ChangeFreezeSpec defines the desired state of ChangeFreeze
// +kubebuilder:validation:XValidation:rule="self.endTime > self.startTime",message="endTime must be after startTime"
type ChangeFreezeSpec struct {
	// startTime is the start of the freeze interval.
	StartTime metav1.Time `json:"startTime"`

	// endTime is the end of the freeze interval.
	EndTime metav1.Time `json:"endTime"`

	// timezone is optional; when provided it is an IANA timezone name used for display/UX.
	// +optional
	Timezone *string `json:"timezone,omitempty"`

	// target selects namespaces/objects/kinds this policy applies to.
	Target TargetSpec `json:"target"`

	// rules define which actions are denied while within [startTime, endTime].
	Rules PolicyRulesSpec `json:"rules"`

	// behavior configures optional side-effects.
	// +optional
	Behavior PolicyBehaviorSpec `json:"behavior,omitempty"`

	// message configures user-facing denial message data.
	// +optional
	Message MessageSpec `json:"message,omitempty"`
}

// ChangeFreezeStatus defines the observed state of ChangeFreeze.
type ChangeFreezeStatus struct {
	// active indicates whether the policy currently enforces denies.
	// +optional
	Active bool `json:"active,omitempty"`

	// timeRemaining is an optional derived value for UX.
	// +optional
	TimeRemaining *metav1.Duration `json:"timeRemaining,omitempty"`

	// observedGeneration is the last observed generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// conditions represent the current state of the ChangeFreeze resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// ChangeFreeze is the Schema for the changefreezes API
type ChangeFreeze struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of ChangeFreeze
	// +required
	Spec ChangeFreezeSpec `json:"spec"`

	// status defines the observed state of ChangeFreeze
	// +optional
	Status ChangeFreezeStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ChangeFreezeList contains a list of ChangeFreeze
type ChangeFreezeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ChangeFreeze `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ChangeFreeze{}, &ChangeFreezeList{})
}
