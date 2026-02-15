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

// FreezeExceptionReconciler reconciles a FreezeException object
type FreezeExceptionReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=freeze-operator.io,resources=freezeexceptions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=freeze-operator.io,resources=freezeexceptions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=freeze-operator.io,resources=freezeexceptions/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *FreezeExceptionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	startTime := time.Now()
	defer func() {
		metrics.ReconciliationDuration.WithLabelValues("freezeexception").Observe(time.Since(startTime).Seconds())
	}()

	logger := log.FromContext(ctx)

	// Fetch the FreezeException
	ex := &freezeoperatorv1alpha1.FreezeException{}
	if err := r.Get(ctx, req.NamespacedName, ex); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Evaluate current state
	now := time.Now().UTC()
	activeFrom := ex.Spec.ActiveFrom.Time
	activeTo := ex.Spec.ActiveTo.Time

	// Determine if exception is currently active
	active := !now.Before(activeFrom) && now.Before(activeTo)

	// Track state changes for events
	wasActive := ex.Status.Active

	// Update status
	ex.Status.Active = active
	ex.Status.ObservedGeneration = ex.Generation

	var requeueAfter time.Duration
	if active {
		// Calculate time until expiration
		remaining := activeTo.Sub(now)
		requeueAfter = remaining + time.Second

		meta.SetStatusCondition(&ex.Status.Conditions, metav1.Condition{
			Type:               conditionTypeActive,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: ex.Generation,
			Reason:             reasonActivated,
			Message:            fmt.Sprintf("Exception active until %s", activeTo.UTC().Format(time.RFC3339)),
		})

		if !wasActive && r.Recorder != nil {
			r.Recorder.Event(ex, corev1.EventTypeNormal, reasonActivated,
				fmt.Sprintf("Exception activated: %s (approved by: %s)", ex.Spec.Reason, ex.Spec.ApprovedBy))
		}
	} else {
		meta.SetStatusCondition(&ex.Status.Conditions, metav1.Condition{
			Type:               conditionTypeActive,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: ex.Generation,
			Reason:             reasonDeactivated,
			Message:            "Exception not active",
		})

		if wasActive && r.Recorder != nil {
			r.Recorder.Event(ex, corev1.EventTypeNormal, reasonDeactivated,
				"Exception expired")
		}

		// If not yet started, requeue at start time
		if now.Before(activeFrom) {
			requeueAfter = activeFrom.Sub(now) + time.Second
		} else {
			// Already ended, no need to requeue frequently
			requeueAfter = 10 * time.Minute
		}
	}

	meta.SetStatusCondition(&ex.Status.Conditions, metav1.Condition{
		Type:               conditionTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: ex.Generation,
		Reason:             reasonEvaluated,
		Message:            "Successfully evaluated exception period",
	})

	// Update status
	if err := r.Status().Update(ctx, ex); err != nil {
		return ctrl.Result{}, err
	}

	// Update metrics
	if active {
		metrics.ActiveFreezePolicies.WithLabelValues("freezeexception", ex.Name).Set(1)
	} else {
		metrics.ActiveFreezePolicies.WithLabelValues("freezeexception", ex.Name).Set(0)
	}

	logger.V(1).Info("reconciled", "active", active, "requeueAfter", requeueAfter)
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *FreezeExceptionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&freezeoperatorv1alpha1.FreezeException{}).
		Named("freezeexception").
		Complete(r)
}
