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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Action represents an operation category that can be denied/allowed by policies.
//
// Note: UPDATE is mapped into more specific actions like ROLL_OUT / SCALE.
// +kubebuilder:validation:Enum=CREATE;DELETE;ROLL_OUT;SCALE
type Action string

const (
	ActionCreate  Action = "CREATE"
	ActionDelete  Action = "DELETE"
	ActionRollout Action = "ROLL_OUT"
	ActionScale   Action = "SCALE"
)

// MaintenanceWindowMode defines how maintenance windows are evaluated.
// +kubebuilder:validation:Enum=DenyOutsideWindows
type MaintenanceWindowMode string

const (
	MaintenanceWindowModeDenyOutsideWindows MaintenanceWindowMode = "DenyOutsideWindows"
)

// TargetKind represents Kubernetes workload kinds targeted by policies.
// +kubebuilder:validation:Enum=Deployment;StatefulSet;DaemonSet;CronJob
type TargetKind string

const (
	TargetKindDeployment  TargetKind = "Deployment"
	TargetKindStatefulSet TargetKind = "StatefulSet"
	TargetKindDaemonSet   TargetKind = "DaemonSet"
	TargetKindCronJob     TargetKind = "CronJob"
)

// TargetSpec selects namespaces/objects/kinds to which a policy applies.
type TargetSpec struct {
	// namespaceSelector selects target namespaces by labels.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// objectSelector selects target objects by labels.
	// +optional
	ObjectSelector *metav1.LabelSelector `json:"objectSelector,omitempty"`

	// kinds limits the set of resource kinds the policy applies to.
	// +kubebuilder:validation:MinItems=1
	Kinds []TargetKind `json:"kinds"`
}

// PolicyRulesSpec defines deny rules for a policy.
type PolicyRulesSpec struct {
	// deny lists which actions are denied when the policy is active.
	// +kubebuilder:validation:MinItems=1
	Deny []Action `json:"deny"`
}

// MessageSpec configures user-facing denial messages.
type MessageSpec struct {
	// reason is a short human-readable description.
	// +optional
	Reason string `json:"reason,omitempty"`

	// docsURL is a link to documentation.
	// +optional
	DocsURL string `json:"docsURL,omitempty"`

	// contact is a contact point (team, oncall, etc.).
	// +optional
	Contact string `json:"contact,omitempty"`
}

// GitOpsProvider names a supported GitOps engine.
// +kubebuilder:validation:Enum=argocd;flux
type GitOpsProvider string

const (
	GitOpsProviderArgoCD GitOpsProvider = "argocd"
	GitOpsProviderFlux   GitOpsProvider = "flux"
)

// GitOpsPauseMode controls how the operator pauses ArgoCD Applications.
// +kubebuilder:validation:Enum=DisableAutoSync
type GitOpsPauseMode string

const (
	// GitOpsPauseModeDisableAutoSync removes spec.syncPolicy.automated to prevent automated syncs.
	GitOpsPauseModeDisableAutoSync GitOpsPauseMode = "DisableAutoSync"
)

// GitOpsArgoCDSpec configures ArgoCD Application pause/resume behavior.
type GitOpsArgoCDSpec struct {
	// applicationSelector selects ArgoCD Application resources to manage.
	// If nil, all Applications in matching namespaces are targeted.
	// +optional
	ApplicationSelector *metav1.LabelSelector `json:"applicationSelector,omitempty"`

	// namespaceSelector selects namespaces where Applications are located.
	// If nil, all namespaces are searched.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// pauseMode controls how Applications are paused.
	// Defaults to DisableAutoSync.
	// +optional
	// +kubebuilder:default=DisableAutoSync
	PauseMode GitOpsPauseMode `json:"pauseMode,omitempty"`
}

// GitOpsFluxSpec configures Flux Kustomization/HelmRelease pause/resume behavior.
type GitOpsFluxSpec struct {
	// kustomizationSelector selects Flux Kustomization resources.
	// If nil, all Kustomizations in matching namespaces are targeted.
	// +optional
	KustomizationSelector *metav1.LabelSelector `json:"kustomizationSelector,omitempty"`

	// helmReleaseSelector selects Flux HelmRelease resources.
	// If nil, all HelmReleases in matching namespaces are targeted.
	// +optional
	HelmReleaseSelector *metav1.LabelSelector `json:"helmReleaseSelector,omitempty"`

	// namespaceSelector selects namespaces where Flux resources are located.
	// If nil, all namespaces are searched.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
}

// GitOpsSpec configures GitOps engine pause/resume behavior during active freeze.
type GitOpsSpec struct {
	// enabled activates GitOps pause/resume integration.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`

	// providers lists which GitOps engines to manage.
	// Valid values: "argocd", "flux".
	// +optional
	Providers []GitOpsProvider `json:"providers,omitempty"`

	// argocd configures ArgoCD-specific pause behavior.
	// +optional
	ArgoCD *GitOpsArgoCDSpec `json:"argocd,omitempty"`

	// flux configures Flux-specific suspend behavior.
	// +optional
	Flux *GitOpsFluxSpec `json:"flux,omitempty"`
}

// PolicyBehaviorSpec defines optional behavior side-effects.
type PolicyBehaviorSpec struct {
	// suspendCronJobs indicates whether the operator should suspend matching CronJobs while the policy is active.
	// +optional
	SuspendCronJobs bool `json:"suspendCronJobs,omitempty"`

	// gitops configures GitOps engine pause/resume behavior during active freeze.
	// When enabled, the operator will pause ArgoCD Applications and/or suspend Flux resources
	// to prevent sync noise during the freeze window.
	// +optional
	GitOps *GitOpsSpec `json:"gitops,omitempty"`
}

// WindowStatus describes an evaluated maintenance window interval.
type WindowStatus struct {
	// name is the name of the window.
	// +optional
	Name string `json:"name,omitempty"`

	// startTime is the start of the interval.
	// +optional
	StartTime metav1.Time `json:"startTime,omitempty"`

	// endTime is the end of the interval.
	// +optional
	EndTime metav1.Time `json:"endTime,omitempty"`
}
