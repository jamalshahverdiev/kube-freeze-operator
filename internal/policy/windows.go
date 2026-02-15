package policy

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EvaluatedWindow represents the result of evaluating a cron-based window
type EvaluatedWindow struct {
	Active      bool
	ActiveStart *time.Time
	ActiveEnd   *time.Time
	NextStart   *time.Time
	NextEnd     *time.Time
}

// EvalCronWindow evaluates a cron-based maintenance window at a given time
func EvalCronWindow(now time.Time, tz string, schedule string, duration metav1.Duration) (EvaluatedWindow, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return EvaluatedWindow{}, fmt.Errorf("invalid timezone %q: %w", tz, err)
	}
	if duration.Duration <= 0 {
		return EvaluatedWindow{}, fmt.Errorf("duration must be > 0")
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sch, err := parser.Parse(schedule)
	if err != nil {
		return EvaluatedWindow{}, fmt.Errorf("invalid schedule %q: %w", schedule, err)
	}

	nowLoc := now.In(loc)

	// Find the most recent scheduled start <= nowLoc by iterating from a bounded time in the past.
	// This is not perfect, but is deterministic and good enough for v0.1.
	searchStart := nowLoc.Add(-14 * 24 * time.Hour)
	prev := time.Time{}
	iter := 0
	for t := sch.Next(searchStart); !t.After(nowLoc); t = sch.Next(t) {
		prev = t
		iter++
		if iter > 10000 {
			break
		}
	}

	next := sch.Next(nowLoc)

	out := EvaluatedWindow{}
	if !prev.IsZero() {
		end := prev.Add(duration.Duration)
		if !nowLoc.Before(prev) && nowLoc.Before(end) {
			out.Active = true
			ps := prev
			pe := end
			out.ActiveStart = &ps
			out.ActiveEnd = &pe
		}
	}

	ns := next
	ne := next.Add(duration.Duration)
	out.NextStart = &ns
	out.NextEnd = &ne

	return out, nil
}

// evalCronWindow is the old unexported version for backward compatibility
func evalCronWindow(now time.Time, tz string, schedule string, duration metav1.Duration) (evaluatedWindow, error) {
	res, err := EvalCronWindow(now, tz, schedule, duration)
	if err != nil {
		return evaluatedWindow{}, err
	}
	return evaluatedWindow(res), nil
}

// evaluatedWindow is the old unexported type for backward compatibility
type evaluatedWindow struct {
	Active      bool
	ActiveStart *time.Time
	ActiveEnd   *time.Time
	NextStart   *time.Time
	NextEnd     *time.Time
}
