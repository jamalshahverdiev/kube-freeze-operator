package controller

import (
	"context"
	"fmt"
	"slices"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	freezeoperatorv1alpha1 "github.com/jamalshahverdiev/kube-freeze-operator/api/v1alpha1"
)

const (
	// Annotation to track original suspend state before operator interference
	annotationOriginalSuspend = "freeze-operator.io/original-suspend"
	annotationManagedBy       = "freeze-operator.io/managed-by"
)

// updateCronJobsForPolicy updates CronJobs matching the target selector
func (r *MaintenanceWindowReconciler) updateCronJobsForPolicy(ctx context.Context, target *freezeoperatorv1alpha1.TargetSpec, policyName string, shouldSuspend bool) error {
	// List all namespaces
	var nsList corev1.NamespaceList
	if err := r.List(ctx, &nsList); err != nil {
		return fmt.Errorf("list namespaces: %w", err)
	}

	// Filter namespaces by selector
	matchingNS := []string{}
	for _, ns := range nsList.Items {
		if matchesNamespaceSelector(target.NamespaceSelector, ns.Labels) {
			matchingNS = append(matchingNS, ns.Name)
		}
	}

	// Check if CronJob is in target kinds
	cronJobTargeted := slices.Contains(target.Kinds, freezeoperatorv1alpha1.TargetKindCronJob)

	if !cronJobTargeted {
		return nil // CronJobs not targeted by this policy
	}

	// Update CronJobs in matching namespaces
	for _, ns := range matchingNS {
		var cronList batchv1.CronJobList
		if err := r.List(ctx, &cronList, client.InNamespace(ns)); err != nil {
			return fmt.Errorf("list cronjobs in %s: %w", ns, err)
		}

		for i := range cronList.Items {
			cron := &cronList.Items[i]

			// Check objectSelector
			if !matchesObjectSelector(target.ObjectSelector, cron.Labels) {
				continue
			}

			// Track original state if this is first time managing this CronJob
			if cron.Annotations == nil {
				cron.Annotations = make(map[string]string)
			}

			managedBy := cron.Annotations[annotationManagedBy]
			if managedBy == "" {
				// First time management - save original state
				cron.Annotations[annotationOriginalSuspend] = fmt.Sprintf("%v", cron.Spec.Suspend != nil && *cron.Spec.Suspend)
				cron.Annotations[annotationManagedBy] = policyName
			} else if managedBy != policyName {
				// Managed by different policy - skip to avoid conflicts
				continue
			}

			// Update suspend field
			suspend := shouldSuspend
			if cron.Spec.Suspend == nil || *cron.Spec.Suspend != suspend {
				cron.Spec.Suspend = &suspend
				if err := r.Update(ctx, cron); err != nil {
					return fmt.Errorf("update cronjob %s/%s: %w", ns, cron.Name, err)
				}
			}
		}
	}

	return nil
}

// updateCronJobsForChangeFreezePolicy is used by ChangeFreeze controller
func (r *ChangeFreezeReconciler) updateCronJobsForPolicy(ctx context.Context, target *freezeoperatorv1alpha1.TargetSpec, policyName string, shouldSuspend bool) error {
	// List all namespaces
	var nsList corev1.NamespaceList
	if err := r.List(ctx, &nsList); err != nil {
		return fmt.Errorf("list namespaces: %w", err)
	}

	// Filter namespaces by selector
	matchingNS := []string{}
	for _, ns := range nsList.Items {
		if matchesNamespaceSelector(target.NamespaceSelector, ns.Labels) {
			matchingNS = append(matchingNS, ns.Name)
		}
	}

	// Check if CronJob is in target kinds
	cronJobTargeted := slices.Contains(target.Kinds, freezeoperatorv1alpha1.TargetKindCronJob)

	if !cronJobTargeted {
		return nil // CronJobs not targeted by this policy
	}

	// Update CronJobs in matching namespaces
	for _, ns := range matchingNS {
		var cronList batchv1.CronJobList
		if err := r.List(ctx, &cronList, client.InNamespace(ns)); err != nil {
			return fmt.Errorf("list cronjobs in %s: %w", ns, err)
		}

		for i := range cronList.Items {
			cron := &cronList.Items[i]

			// Check objectSelector
			if !matchesObjectSelector(target.ObjectSelector, cron.Labels) {
				continue
			}

			// Track original state if this is first time managing this CronJob
			if cron.Annotations == nil {
				cron.Annotations = make(map[string]string)
			}

			managedBy := cron.Annotations[annotationManagedBy]
			if managedBy == "" {
				// First time management - save original state
				cron.Annotations[annotationOriginalSuspend] = fmt.Sprintf("%v", cron.Spec.Suspend != nil && *cron.Spec.Suspend)
				cron.Annotations[annotationManagedBy] = policyName
			} else if managedBy != policyName {
				// Managed by different policy - skip to avoid conflicts
				continue
			}

			// Update suspend field
			suspend := shouldSuspend
			if cron.Spec.Suspend == nil || *cron.Spec.Suspend != suspend {
				cron.Spec.Suspend = &suspend
				if err := r.Update(ctx, cron); err != nil {
					return fmt.Errorf("update cronjob %s/%s: %w", ns, cron.Name, err)
				}
			}
		}
	}

	return nil
}

func matchesNamespaceSelector(selector *metav1.LabelSelector, nsLabels map[string]string) bool {
	if selector == nil {
		return true // Match all namespaces
	}

	sel, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return false
	}

	return sel.Matches(labels.Set(nsLabels))
}

func matchesObjectSelector(selector *metav1.LabelSelector, objLabels map[string]string) bool {
	if selector == nil {
		return true // Match all objects
	}

	sel, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return false
	}

	return sel.Matches(labels.Set(objLabels))
}
