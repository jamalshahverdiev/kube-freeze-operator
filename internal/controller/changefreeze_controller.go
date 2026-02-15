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
)

// ChangeFreezeReconciler reconciles a ChangeFreeze object
type ChangeFreezeReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=freeze-operator.io,resources=changefreezes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=freeze-operator.io,resources=changefreezes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=freeze-operator.io,resources=changefreezes/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ChangeFreezeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	startTime := time.Now()
	defer func() {
		metrics.ReconciliationDuration.WithLabelValues("changefreeze").Observe(time.Since(startTime).Seconds())
	}()

	logger := log.FromContext(ctx)

	// Fetch the ChangeFreeze
	cf := &freezeoperatorv1alpha1.ChangeFreeze{}
	if err := r.Get(ctx, req.NamespacedName, cf); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Evaluate current state
	now := time.Now().UTC()
	freezeStartTime := cf.Spec.StartTime.Time
	freezeEndTime := cf.Spec.EndTime.Time

	// Determine if freeze is currently active
	active := !now.Before(freezeStartTime) && now.Before(freezeEndTime)

	// Track state changes for events
	wasActive := cf.Status.Active

	// Update status
	cf.Status.Active = active
	cf.Status.ObservedGeneration = cf.Generation

	var requeueAfter time.Duration
	if active {
		// Calculate time remaining
		remaining := freezeEndTime.Sub(now)
		cf.Status.TimeRemaining = &metav1.Duration{Duration: remaining}
		requeueAfter = remaining + time.Second

		meta.SetStatusCondition(&cf.Status.Conditions, metav1.Condition{
			Type:               conditionTypeActive,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cf.Generation,
			Reason:             reasonActivated,
			Message:            fmt.Sprintf("Freeze active until %s", freezeEndTime.UTC().Format(time.RFC3339)),
		})

		if !wasActive && r.Recorder != nil {
			r.Recorder.Event(cf, corev1.EventTypeWarning, reasonActivated,
				fmt.Sprintf("Change freeze activated until %s", freezeEndTime.UTC().Format(time.RFC3339)))
		}
	} else {
		cf.Status.TimeRemaining = nil

		meta.SetStatusCondition(&cf.Status.Conditions, metav1.Condition{
			Type:               conditionTypeActive,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: cf.Generation,
			Reason:             reasonDeactivated,
			Message:            "Freeze not active",
		})

		if wasActive && r.Recorder != nil {
			r.Recorder.Event(cf, corev1.EventTypeNormal, reasonDeactivated, "Change freeze deactivated")
		}

		// If not yet started, requeue at start time
		if now.Before(freezeStartTime) {
			requeueAfter = freezeStartTime.Sub(now) + time.Second
		} else {
			// Already ended, no need to requeue frequently
			requeueAfter = 10 * time.Minute
		}
	}

	meta.SetStatusCondition(&cf.Status.Conditions, metav1.Condition{
		Type:               conditionTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: cf.Generation,
		Reason:             reasonEvaluated,
		Message:            "Successfully evaluated freeze period",
	})

	// Update CronJobs if configured
	if cf.Spec.Behavior.SuspendCronJobs {
		// For ChangeFreeze: active=true means freeze is on, so DO suspend
		if err := r.updateCronJobsForPolicy(ctx, &cf.Spec.Target, cf.Name, active); err != nil {
			logger.Error(err, "failed to update CronJobs")
			if r.Recorder != nil {
				r.Recorder.Event(cf, corev1.EventTypeWarning, reasonCronJobUpdateFail, err.Error())
			}
		} else if r.Recorder != nil {
			r.Recorder.Event(cf, corev1.EventTypeNormal, reasonCronJobsUpdated,
				fmt.Sprintf("CronJobs suspend status updated (suspend=%v)", active))
		}
	}

	// Update status
	if err := r.Status().Update(ctx, cf); err != nil {
		return ctrl.Result{}, err
	}

	// Update metrics
	if active {
		metrics.ActiveFreezePolicies.WithLabelValues("changefreeze", cf.Name).Set(1)
	} else {
		metrics.ActiveFreezePolicies.WithLabelValues("changefreeze", cf.Name).Set(0)
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ChangeFreezeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&freezeoperatorv1alpha1.ChangeFreeze{}).
		Named("changefreeze").
		Complete(r)
}
