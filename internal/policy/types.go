package policy

import (
	"time"

	freezev1alpha1 "github.com/jamalshahverdiev/kube-freeze-operator/api/v1alpha1"
)

type PolicyKind string

const (
	PolicyKindFreezeException   PolicyKind = "FreezeException"
	PolicyKindChangeFreeze      PolicyKind = "ChangeFreeze"
	PolicyKindMaintenanceWindow PolicyKind = "MaintenanceWindow"
)

type PolicyRef struct {
	Kind PolicyKind
	Name string
}

type Input struct {
	Now time.Time

	Namespace     string
	NamespaceTags map[string]string

	Kind   freezev1alpha1.TargetKind
	Action freezev1alpha1.Action

	ObjectLabels map[string]string

	Username string
	Groups   []string
}

type Decision struct {
	Allowed bool

	MatchedPolicy   *PolicyRef
	MatchedOverride *PolicyRef

	Reason string

	NextAllowedTime *time.Time
	FreezeEndTime   *time.Time

	EvaluationTime  time.Time
	EvaluatedNS     string
	EvaluatedKind   freezev1alpha1.TargetKind
	EvaluatedAction freezev1alpha1.Action
}
