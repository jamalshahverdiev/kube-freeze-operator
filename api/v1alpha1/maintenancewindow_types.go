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

// MaintenanceWindowSpec defines the desired state of MaintenanceWindow
type MaintenanceWindowSpec struct {
	// timezone is an IANA timezone name.
	// +kubebuilder:validation:MinLength=1
	Timezone string `json:"timezone"`

	// mode defines how windows are interpreted.
	// v0.1 supports only DenyOutsideWindows.
	Mode MaintenanceWindowMode `json:"mode"`

	// windows defines allowed maintenance intervals.
	// +kubebuilder:validation:MinItems=1
	Windows []MaintenanceWindowWindowSpec `json:"windows"`

	// target selects namespaces/objects/kinds this policy applies to.
	Target TargetSpec `json:"target"`

	// rules define which actions are denied when the policy is active.
	Rules PolicyRulesSpec `json:"rules"`

	// behavior configures optional side-effects.
	// +optional
	Behavior PolicyBehaviorSpec `json:"behavior,omitempty"`

	// message configures user-facing denial message data.
	// +optional
	Message MessageSpec `json:"message,omitempty"`
}

// MaintenanceWindowWindowSpec defines a recurring maintenance window.
type MaintenanceWindowWindowSpec struct {
	// name is a human-readable identifier.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// schedule is a cron expression.
	// +kubebuilder:validation:MinLength=1
	Schedule string `json:"schedule"`

	// duration is how long the window lasts.
	Duration metav1.Duration `json:"duration"`
}

// MaintenanceWindowStatus defines the observed state of MaintenanceWindow.
type MaintenanceWindowStatus struct {
	// active indicates whether the policy currently enforces denies.
	// +optional
	Active bool `json:"active,omitempty"`

	// activeWindow, if active, describes the current window.
	// +optional
	ActiveWindow *WindowStatus `json:"activeWindow,omitempty"`

	// nextWindow describes the next upcoming window.
	// +optional
	NextWindow *WindowStatus `json:"nextWindow,omitempty"`

	// observedGeneration is the last observed generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// conditions represent the current state of the MaintenanceWindow resource.
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

// MaintenanceWindow is the Schema for the maintenancewindows API
type MaintenanceWindow struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of MaintenanceWindow
	// +required
	Spec MaintenanceWindowSpec `json:"spec"`

	// status defines the observed state of MaintenanceWindow
	// +optional
	Status MaintenanceWindowStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// MaintenanceWindowList contains a list of MaintenanceWindow
type MaintenanceWindowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []MaintenanceWindow `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MaintenanceWindow{}, &MaintenanceWindowList{})
}
