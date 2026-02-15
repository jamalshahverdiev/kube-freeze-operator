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

package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	freezeoperatorv1alpha1 "github.com/jamalshahverdiev/kube-freeze-operator/api/v1alpha1"
	"github.com/jamalshahverdiev/kube-freeze-operator/internal/metrics"
	"github.com/jamalshahverdiev/kube-freeze-operator/internal/policy"
)

const (
	conditionTypeReady  = "Ready"
	conditionTypeActive = "Active"

	reasonEvaluated         = "Evaluated"
	reasonEvaluationFailed  = "EvaluationFailed"
	reasonActivated         = "Activated"
	reasonDeactivated       = "Deactivated"
	reasonCronJobsUpdated   = "CronJobsUpdated"
	reasonCronJobUpdateFail = "CronJobUpdateFailed"
)

// MaintenanceWindowReconciler reconciles a MaintenanceWindow object
type MaintenanceWindowReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=freeze-operator.io,resources=maintenancewindows,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=freeze-operator.io,resources=maintenancewindows/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=freeze-operator.io,resources=maintenancewindows/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *MaintenanceWindowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	startTime := time.Now()
	defer func() {
		metrics.ReconciliationDuration.WithLabelValues("maintenancewindow").Observe(time.Since(startTime).Seconds())
	}()

	logger := log.FromContext(ctx)

	// Fetch the MaintenanceWindow
	mw := &freezeoperatorv1alpha1.MaintenanceWindow{}
	if err := r.Get(ctx, req.NamespacedName, mw); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Evaluate current state
	now := time.Now().UTC()
	result, err := r.evaluateWindows(now, mw)
	if err != nil {
		logger.Error(err, "failed to evaluate windows")
		if r.Recorder != nil {
			r.Recorder.Event(mw, corev1.EventTypeWarning, reasonEvaluationFailed, err.Error())
		}

		meta.SetStatusCondition(&mw.Status.Conditions, metav1.Condition{
			Type:               conditionTypeReady,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: mw.Generation,
			Reason:             reasonEvaluationFailed,
			Message:            err.Error(),
		})

		if statusErr := r.Status().Update(ctx, mw); statusErr != nil {
			logger.Error(statusErr, "failed to update status after evaluation error")
		}
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	// Track state changes for events
	wasActive := mw.Status.Active

	// Update status
	mw.Status.Active = result.Active
	mw.Status.ActiveWindow = result.ActiveWindow
	mw.Status.NextWindow = result.NextWindow
	mw.Status.ObservedGeneration = mw.Generation

	// Update conditions
	if result.Active {
		meta.SetStatusCondition(&mw.Status.Conditions, metav1.Condition{
			Type:               conditionTypeActive,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: mw.Generation,
			Reason:             reasonActivated,
			Message:            fmt.Sprintf("Window '%s' is active", result.ActiveWindow.Name),
		})
		if !wasActive && r.Recorder != nil {
			r.Recorder.Event(mw, corev1.EventTypeNormal, reasonActivated,
				fmt.Sprintf("Maintenance window '%s' activated", result.ActiveWindow.Name))
		}
	} else {
		meta.SetStatusCondition(&mw.Status.Conditions, metav1.Condition{
			Type:               conditionTypeActive,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: mw.Generation,
			Reason:             reasonDeactivated,
			Message:            "No active window",
		})
		if wasActive && r.Recorder != nil {
			r.Recorder.Event(mw, corev1.EventTypeNormal, reasonDeactivated,
				"Maintenance window deactivated")
		}
	}

	meta.SetStatusCondition(&mw.Status.Conditions, metav1.Condition{
		Type:               conditionTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: mw.Generation,
		Reason:             reasonEvaluated,
		Message:            "Successfully evaluated windows",
	})

	// Update CronJobs if configured
	if mw.Spec.Behavior.SuspendCronJobs {
		if err := r.updateCronJobs(ctx, mw, result.Active); err != nil {
			logger.Error(err, "failed to update CronJobs")
			if r.Recorder != nil {
				r.Recorder.Event(mw, corev1.EventTypeWarning, reasonCronJobUpdateFail, err.Error())
			}
			// Don't fail reconciliation, just log and continue
		} else if r.Recorder != nil {
			r.Recorder.Event(mw, corev1.EventTypeNormal, reasonCronJobsUpdated,
				fmt.Sprintf("CronJobs suspend status updated (active=%v)", result.Active))
		}
	}

	// Update status
	if err := r.Status().Update(ctx, mw); err != nil {
		return ctrl.Result{}, err
	}

	// Update metrics
	if result.Active {
		metrics.ActiveFreezePolicies.WithLabelValues("maintenancewindow", mw.Name).Set(1)
	} else {
		metrics.ActiveFreezePolicies.WithLabelValues("maintenancewindow", mw.Name).Set(0)
	}

	// Requeue at next state change
	return ctrl.Result{RequeueAfter: result.RequeueAfter}, nil
}

type evaluationResult struct {
	Active       bool
	ActiveWindow *freezeoperatorv1alpha1.WindowStatus
	NextWindow   *freezeoperatorv1alpha1.WindowStatus
	RequeueAfter time.Duration
}

func (r *MaintenanceWindowReconciler) evaluateWindows(now time.Time, mw *freezeoperatorv1alpha1.MaintenanceWindow) (*evaluationResult, error) {
	if mw.Spec.Mode != freezeoperatorv1alpha1.MaintenanceWindowModeDenyOutsideWindows {
		return nil, fmt.Errorf("unsupported mode: %s", mw.Spec.Mode)
	}

	result := &evaluationResult{
		Active:       false,
		RequeueAfter: 5 * time.Minute, // Default requeue
	}

	var earliestNext *time.Time

	for _, w := range mw.Spec.Windows {
		res, err := policy.EvalCronWindow(now, mw.Spec.Timezone, w.Schedule, w.Duration)
		if err != nil {
			return nil, fmt.Errorf("evaluate window %q: %w", w.Name, err)
		}

		// Check if this window is currently active
		if res.Active {
			result.Active = true
			result.ActiveWindow = &freezeoperatorv1alpha1.WindowStatus{
				Name:      w.Name,
				StartTime: metav1.NewTime(*res.ActiveStart),
				EndTime:   metav1.NewTime(*res.ActiveEnd),
			}
			// Requeue when this window ends
			timeUntilEnd := res.ActiveEnd.Sub(now)
			if timeUntilEnd > 0 && timeUntilEnd < result.RequeueAfter {
				result.RequeueAfter = timeUntilEnd + time.Second
			}
		}

		// Track earliest next window
		if res.NextStart != nil {
			if earliestNext == nil || res.NextStart.Before(*earliestNext) {
				earliestNext = res.NextStart
				nextEnd := res.NextStart.Add(w.Duration.Duration)
				result.NextWindow = &freezeoperatorv1alpha1.WindowStatus{
					Name:      w.Name,
					StartTime: metav1.NewTime(*res.NextStart),
					EndTime:   metav1.NewTime(nextEnd),
				}
			}
		}
	}

	// If not active, requeue when next window starts
	if !result.Active && earliestNext != nil {
		timeUntilNext := earliestNext.Sub(now)
		if timeUntilNext > 0 && timeUntilNext < result.RequeueAfter {
			result.RequeueAfter = timeUntilNext + time.Second
		}
	}

	return result, nil
}

func (r *MaintenanceWindowReconciler) updateCronJobs(ctx context.Context, mw *freezeoperatorv1alpha1.MaintenanceWindow, active bool) error {
	// For MaintenanceWindow: active=true means INSIDE window, so DON'T suspend
	// For ChangeFreeze: active=true means freeze is on, so DO suspend
	// MaintenanceWindow logic: suspend when OUTSIDE window (active=false)
	shouldSuspend := !active

	return r.updateCronJobsForPolicy(ctx, &mw.Spec.Target, mw.Name, shouldSuspend)
}

// SetupWithManager sets up the controller with the Manager.
func (r *MaintenanceWindowReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&freezeoperatorv1alpha1.MaintenanceWindow{}).
		Named("maintenancewindow").
		Complete(r)
}
