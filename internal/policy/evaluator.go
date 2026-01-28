package policy

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	freezev1alpha1 "github.com/jamalshahverdiev/kube-freeze-operator/api/v1alpha1"
)

type Evaluator struct {
	Client client.Reader
}

type denyCandidate struct {
	ref            PolicyRef
	reason         string
	nextAllowed    *time.Time
	freezeEnd      *time.Time
	behavior       *freezev1alpha1.PolicyBehaviorSpec
	determinsticId string
}

func (e *Evaluator) Evaluate(ctx context.Context, in Input) (Decision, error) {
	if e == nil || e.Client == nil {
		return Decision{}, errors.New("policy evaluator: client is required")
	}

	if in.Now.IsZero() {
		in.Now = time.Now().UTC()
	}

	nsLabels := in.NamespaceTags
	if nsLabels == nil {
		if in.Namespace == "" {
			return Decision{}, errors.New("policy evaluator: namespace is required")
		}
		ns := &corev1.Namespace{}
		if err := e.Client.Get(ctx, types.NamespacedName{Name: in.Namespace}, ns); err != nil {
			return Decision{}, fmt.Errorf("get namespace %q: %w", in.Namespace, err)
		}
		nsLabels = ns.Labels
	}

	dec := Decision{
		Allowed:         true,
		EvaluatedAction: in.Action,
		EvaluatedKind:   in.Kind,
		EvaluatedNS:     in.Namespace,
		EvaluationTime:  in.Now,
	}

	matchedDenies := make([]denyCandidate, 0, 4)

	// 1) ChangeFreeze (deny in range)
	{
		var list freezev1alpha1.ChangeFreezeList
		if err := e.Client.List(ctx, &list); err != nil {
			return Decision{}, fmt.Errorf("list ChangeFreeze: %w", err)
		}
		for i := range list.Items {
			cf := &list.Items[i]
			if !targetMatches(&cf.Spec.Target, nsLabels, in.ObjectLabels, in.Kind) {
				continue
			}
			if !actionIn(in.Action, cf.Spec.Rules.Deny) {
				continue
			}
			if !in.Now.Before(cf.Spec.EndTime.Time) || in.Now.Before(cf.Spec.StartTime.Time) {
				continue
			}
			end := cf.Spec.EndTime.Time
			matchedDenies = append(matchedDenies, denyCandidate{
				ref:            PolicyRef{Kind: PolicyKindChangeFreeze, Name: cf.Name},
				reason:         firstNonEmpty(cf.Spec.Message.Reason, "ChangeFreeze is active"),
				nextAllowed:    &end,
				freezeEnd:      &end,
				behavior:       &cf.Spec.Behavior,
				determinsticId: cf.Name,
			})
		}
	}

	// 2) MaintenanceWindow (deny outside windows)
	{
		var list freezev1alpha1.MaintenanceWindowList
		if err := e.Client.List(ctx, &list); err != nil {
			return Decision{}, fmt.Errorf("list MaintenanceWindow: %w", err)
		}
		for i := range list.Items {
			mw := &list.Items[i]
			if !targetMatches(&mw.Spec.Target, nsLabels, in.ObjectLabels, in.Kind) {
				continue
			}
			if !actionIn(in.Action, mw.Spec.Rules.Deny) {
				continue
			}
			if mw.Spec.Mode != freezev1alpha1.MaintenanceWindowModeDenyOutsideWindows {
				continue
			}

			inAny := false
			var bestNext *time.Time
			for _, w := range mw.Spec.Windows {
				res, err := evalCronWindow(in.Now, mw.Spec.Timezone, w.Schedule, w.Duration)
				if err != nil {
					// Treat invalid schedule as non-matching for enforcement; controller should surface condition.
					continue
				}
				if res.Active {
					inAny = true
					break
				}
				if res.NextStart != nil {
					if bestNext == nil || res.NextStart.Before(*bestNext) {
						t := *res.NextStart
						bestNext = &t
					}
				}
			}

			if inAny {
				continue
			}

			matchedDenies = append(matchedDenies, denyCandidate{
				ref:            PolicyRef{Kind: PolicyKindMaintenanceWindow, Name: mw.Name},
				reason:         firstNonEmpty(mw.Spec.Message.Reason, "Outside maintenance window"),
				nextAllowed:    bestNext,
				behavior:       &mw.Spec.Behavior,
				determinsticId: mw.Name,
			})
		}
	}

	if len(matchedDenies) == 0 {
		return dec, nil
	}

	// Pick deterministic deny to report.
	sort.SliceStable(matchedDenies, func(i, j int) bool {
		a := matchedDenies[i]
		b := matchedDenies[j]
		if a.nextAllowed != nil && b.nextAllowed != nil {
			if !a.nextAllowed.Equal(*b.nextAllowed) {
				return a.nextAllowed.Before(*b.nextAllowed)
			}
		}
		if a.nextAllowed != nil && b.nextAllowed == nil {
			return true
		}
		if a.nextAllowed == nil && b.nextAllowed != nil {
			return false
		}
		return a.determinsticId < b.determinsticId
	})
	chosen := matchedDenies[0]

	// 3) FreezeException overrides denies when allowed.
	{
		var list freezev1alpha1.FreezeExceptionList
		if err := e.Client.List(ctx, &list); err != nil {
			return Decision{}, fmt.Errorf("list FreezeException: %w", err)
		}
		for i := range list.Items {
			ex := &list.Items[i]
			if !targetMatches(&ex.Spec.Target, nsLabels, in.ObjectLabels, in.Kind) {
				continue
			}
			if !actionIn(in.Action, ex.Spec.Allow) {
				continue
			}
			if !in.Now.Before(ex.Spec.ActiveTo.Time) || in.Now.Before(ex.Spec.ActiveFrom.Time) {
				continue
			}
			if !constraintsPass(ex.Spec.Constraints, in.ObjectLabels, in.Username, in.Groups) {
				continue
			}

			dec.Allowed = true
			dec.MatchedPolicy = &chosen.ref
			dec.MatchedOverride = &PolicyRef{Kind: PolicyKindFreezeException, Name: ex.Name}
			dec.Reason = firstNonEmpty(ex.Spec.Reason, "Exception granted")
			dec.NextAllowedTime = chosen.nextAllowed
			dec.FreezeEndTime = chosen.freezeEnd
			return dec, nil
		}
	}

	dec.Allowed = false
	dec.MatchedPolicy = &chosen.ref
	dec.Reason = chosen.reason
	dec.NextAllowedTime = chosen.nextAllowed
	dec.FreezeEndTime = chosen.freezeEnd
	return dec, nil
}

func targetMatches(t *freezev1alpha1.TargetSpec, nsLabels map[string]string, objLabels map[string]string, kind freezev1alpha1.TargetKind) bool {
	if t == nil {
		return false
	}
	kindOk := false
	for _, k := range t.Kinds {
		if k == kind {
			kindOk = true
			break
		}
	}
	if !kindOk {
		return false
	}
	ok, err := matchLabelSelector(t.NamespaceSelector, nsLabels)
	if err != nil || !ok {
		return false
	}
	ok, err = matchLabelSelector(t.ObjectSelector, objLabels)
	if err != nil || !ok {
		return false
	}
	return true
}

func actionIn(a freezev1alpha1.Action, list []freezev1alpha1.Action) bool {
	for _, v := range list {
		if v == a {
			return true
		}
	}
	return false
}

func firstNonEmpty(v string, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}

func constraintsPass(c *freezev1alpha1.FreezeExceptionConstraintsSpec, objLabels map[string]string, username string, groups []string) bool {
	if c == nil {
		return true
	}
	for k, v := range c.RequireLabels {
		if objLabels == nil {
			return false
		}
		if objLabels[k] != v {
			return false
		}
	}
	if len(c.AllowedUsers) > 0 {
		allowed := false
		for _, u := range c.AllowedUsers {
			if u == username {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	if len(c.AllowedGroups) > 0 {
		allowed := false
		for _, want := range c.AllowedGroups {
			for _, g := range groups {
				if g == want {
					allowed = true
					break
				}
			}
			if allowed {
				break
			}
		}
		if !allowed {
			return false
		}
	}
	return true
}
