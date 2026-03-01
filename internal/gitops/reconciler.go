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

// Package gitops implements pause/resume logic for GitOps engines
// (Argo CD and Flux) during active freeze periods.
//
// The package uses the unstructured client so that ArgoCD and Flux
// CRDs do NOT need to be vendored. If CRDs are absent, all operations
// degrade gracefully to no-ops.
//
// Managed objects are annotated with:
//   - freeze-operator.io/managed = "true"
//   - freeze-operator.io/managed-by-policy = <policyName>
//   - freeze-operator.io/original-autosync  (ArgoCD only)
//   - freeze-operator.io/original-suspend   (Flux only)
//
// The operator will ONLY restore state it originally changed.
// Manual modifications by the user are left untouched (fail-safe).
package gitops

import (
	"context"
	"errors"
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	freezev1alpha1 "github.com/jamalshahverdiev/kube-freeze-operator/api/v1alpha1"
)

// Reconciler pauses/resumes GitOps resources according to active freeze policies.
type Reconciler struct {
	// Client is the controller-runtime client used for all K8s operations.
	Client client.Client
}

// Result holds the output of a single Reconcile call.
type Result struct {
	// PausedCount is the total number of GitOps objects currently paused by this policy.
	PausedCount int
	// ReconcileTime is when the reconciliation ran.
	ReconcileTime metav1.Time
}

// Reconcile pauses (active=true) or resumes (active=false) all configured GitOps resources.
//
// Parameters:
//   - gitops    the GitOpsSpec from the policy's behavior block
//   - policyRef the name of the owning policy (used as a managed-by annotation value)
//   - active    whether the freeze is currently active
func (r *Reconciler) Reconcile(
	ctx context.Context,
	gitops *freezev1alpha1.GitOpsSpec,
	policyRef string,
	active bool,
) (Result, error) {
	result := Result{ReconcileTime: metav1.Now()}

	if gitops == nil || !gitops.Enabled {
		return result, nil
	}

	var errs []error

	// ArgoCD
	if slices.Contains(gitops.Providers, freezev1alpha1.GitOpsProviderArgoCD) {
		n, err := reconcileArgoCD(ctx, r.Client, gitops.ArgoCD, nil, policyRef, active)
		if err != nil {
			errs = append(errs, fmt.Errorf("argocd: %w", err))
		}
		result.PausedCount += n
	}

	// Flux
	if slices.Contains(gitops.Providers, freezev1alpha1.GitOpsProviderFlux) {
		n, err := reconcileFlux(ctx, r.Client, gitops.Flux, policyRef, active)
		if err != nil {
			errs = append(errs, fmt.Errorf("flux: %w", err))
		}
		result.PausedCount += n
	}

	return result, errors.Join(errs...)
}

// ---------------------------------------------------------------------------
// helpers shared between argocd.go and flux.go
// ---------------------------------------------------------------------------

// listMatchingNamespaces returns the names of all namespaces that match the
// given LabelSelector. If selector is nil, all namespaces are returned.
func listMatchingNamespaces(ctx context.Context, c client.Client, selector *metav1.LabelSelector) ([]string, error) {
	nsList := &corev1.NamespaceList{}

	var listOpts []client.ListOption
	if selector != nil {
		sel, err := metav1.LabelSelectorAsSelector(selector)
		if err != nil {
			return nil, fmt.Errorf("invalid namespaceSelector: %w", err)
		}
		listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: sel})
	}

	if err := c.List(ctx, nsList, listOpts...); err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}

	names := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		names = append(names, ns.Name)
	}
	return names, nil
}

// isNoMatchError returns true if the error indicates that the requested
// API resource (CRD) is not registered in the cluster — i.e. the
// provider's CRDs are not installed.
func isNoMatchError(err error) bool {
	return meta.IsNoMatchError(err)
}
