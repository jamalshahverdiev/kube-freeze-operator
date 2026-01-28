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

// FreezeExceptionSpec defines the desired state of FreezeException
// +kubebuilder:validation:XValidation:rule="self.activeTo > self.activeFrom",message="activeTo must be after activeFrom"
type FreezeExceptionSpec struct {
	// activeFrom is when this exception becomes effective.
	ActiveFrom metav1.Time `json:"activeFrom"`

	// activeTo is when this exception expires.
	ActiveTo metav1.Time `json:"activeTo"`

	// target selects namespaces/objects/kinds this exception applies to.
	Target TargetSpec `json:"target"`

	// allow lists which actions are allowed even when policies would deny.
	// +kubebuilder:validation:MinItems=1
	Allow []Action `json:"allow"`

	// constraints optionally limits exception usage.
	// +optional
	Constraints *FreezeExceptionConstraintsSpec `json:"constraints,omitempty"`

	// reason explains why this exception exists.
	// +kubebuilder:validation:MinLength=1
	Reason string `json:"reason"`

	// ticketURL links to an approval or tracking ticket.
	// +optional
	TicketURL string `json:"ticketURL,omitempty"`

	// approvedBy is a free-form approver identifier.
	// +optional
	ApprovedBy string `json:"approvedBy,omitempty"`
}

// FreezeExceptionConstraintsSpec adds optional constraints to an exception.
type FreezeExceptionConstraintsSpec struct {
	// requireLabels requires these labels to be present on the target object.
	// +optional
	RequireLabels map[string]string `json:"requireLabels,omitempty"`

	// allowedUsers restricts exception usage to these usernames.
	// +optional
	AllowedUsers []string `json:"allowedUsers,omitempty"`

	// allowedGroups restricts exception usage to these groups.
	// +optional
	AllowedGroups []string `json:"allowedGroups,omitempty"`
}

// FreezeExceptionStatus defines the observed state of FreezeException.
type FreezeExceptionStatus struct {
	// active indicates whether this exception is currently active.
	// +optional
	Active bool `json:"active,omitempty"`

	// observedGeneration is the last observed generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// conditions represent the current state of the FreezeException resource.
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

// FreezeException is the Schema for the freezeexceptions API
type FreezeException struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of FreezeException
	// +required
	Spec FreezeExceptionSpec `json:"spec"`

	// status defines the observed state of FreezeException
	// +optional
	Status FreezeExceptionStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// FreezeExceptionList contains a list of FreezeException
type FreezeExceptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []FreezeException `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FreezeException{}, &FreezeExceptionList{})
}
