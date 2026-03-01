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

package gitops

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	freezev1alpha1 "github.com/jamalshahverdiev/kube-freeze-operator/api/v1alpha1"
)

// argoCDApplicationGVK is the GroupVersionKind for ArgoCD Application resources.
var argoCDApplicationGVK = schema.GroupVersionKind{
	Group:   "argoproj.io",
	Version: "v1alpha1",
	Kind:    "Application",
}

// reconcileArgoCD pauses or resumes ArgoCD Applications matching the selector.
// It operates via the unstructured client — no ArgoCD types need to be vendored.
// If ArgoCD CRDs are not installed the function returns (0, nil) gracefully.
//
// Returns the number of Applications currently paused by this policy.
func reconcileArgoCD(
	ctx context.Context,
	c client.Client,
	spec *freezev1alpha1.GitOpsArgoCDSpec,
	policyRef string,
	active bool,
) (int, error) {
	logger := log.FromContext(ctx).WithName("gitops.argocd")

	if spec == nil {
		return 0, nil
	}

	// Resolve namespaces to search.
	namespaces, err := listMatchingNamespaces(ctx, c, spec.NamespaceSelector)
	if err != nil {
		return 0, fmt.Errorf("list namespaces: %w", err)
	}

	// Build application label selector.
	var appSelector client.MatchingLabelsSelector
	if spec.ApplicationSelector != nil {
		sel, err := metav1.LabelSelectorAsSelector(spec.ApplicationSelector)
		if err != nil {
			return 0, fmt.Errorf("invalid applicationSelector: %w", err)
		}
		appSelector = client.MatchingLabelsSelector{Selector: sel}
	}

	totalPaused := 0

	for _, ns := range namespaces {
		appList := &unstructured.UnstructuredList{}
		appList.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   argoCDApplicationGVK.Group,
			Version: argoCDApplicationGVK.Version,
			Kind:    argoCDApplicationGVK.Kind + "List",
		})

		listOpts := []client.ListOption{client.InNamespace(ns)}
		if spec.ApplicationSelector != nil {
			listOpts = append(listOpts, appSelector)
		}

		if err := c.List(ctx, appList, listOpts...); err != nil {
			if isNoMatchError(err) {
				logger.V(1).Info("ArgoCD Application CRD not found, skipping")
				return 0, nil
			}
			return totalPaused, fmt.Errorf("list Applications in namespace %s: %w", ns, err)
		}

		for i := range appList.Items {
			app := &appList.Items[i]
			paused, err := reconcileArgoCDApp(ctx, c, app, policyRef, active)
			if err != nil {
				logger.Error(err, "failed to reconcile Application",
					"name", app.GetName(), "namespace", app.GetNamespace())
				// best effort — continue with remaining apps
				continue
			}
			if paused {
				totalPaused++
			}
		}
	}

	return totalPaused, nil
}

// reconcileArgoCDApp reconciles a single ArgoCD Application.
// Returns true if the app is currently in the paused state managed by the operator.
func reconcileArgoCDApp(
	ctx context.Context,
	c client.Client,
	app *unstructured.Unstructured,
	policyRef string,
	active bool,
) (bool, error) {
	annotations := app.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	managedByUs := annotations[AnnotationManaged] == annotationManagedValue &&
		annotations[AnnotationManagedByPolicy] == policyRef

	switch {
	case active && !managedByUs:
		// Freeze became active — pause the Application if not already managed.

		// Capture current autosync configuration.
		automated, found, _ := unstructured.NestedMap(app.Object, "spec", "syncPolicy", "automated")
		if found {
			b, err := json.Marshal(automated)
			if err != nil {
				return false, fmt.Errorf("marshal automated: %w", err)
			}
			annotations[AnnotationOriginalAutoSync] = string(b)
		} else {
			// autosync was already nil — record that so we don't accidentally enable it on restore.
			annotations[AnnotationOriginalAutoSync] = "null"
		}
		annotations[AnnotationManaged] = annotationManagedValue
		annotations[AnnotationManagedByPolicy] = policyRef
		app.SetAnnotations(annotations)

		// Disable autosync by removing spec.syncPolicy.automated.
		unstructured.RemoveNestedField(app.Object, "spec", "syncPolicy", "automated")

		if err := c.Update(ctx, app); err != nil {
			return false, fmt.Errorf("pause Application %s/%s: %w", app.GetNamespace(), app.GetName(), err)
		}
		log.FromContext(ctx).Info("ArgoCD Application paused",
			"name", app.GetName(), "namespace", app.GetNamespace(), "policy", policyRef)
		return true, nil

	case !active && managedByUs:
		// Freeze ended — restore the Application to its original state.
		originalJSON := annotations[AnnotationOriginalAutoSync]

		if originalJSON != "" && originalJSON != "null" {
			var automated map[string]any
			if err := json.Unmarshal([]byte(originalJSON), &automated); err == nil {
				if err := unstructured.SetNestedMap(app.Object, automated, "spec", "syncPolicy", "automated"); err != nil {
					return false, fmt.Errorf("set automated: %w", err)
				}
			}
		}
		// If originalJSON was "null", autosync remains removed — correct behaviour.

		delete(annotations, AnnotationManaged)
		delete(annotations, AnnotationManagedByPolicy)
		delete(annotations, AnnotationOriginalAutoSync)
		app.SetAnnotations(annotations)

		if err := c.Update(ctx, app); err != nil {
			return false, fmt.Errorf("restore Application %s/%s: %w", app.GetNamespace(), app.GetName(), err)
		}
		log.FromContext(ctx).Info("ArgoCD Application restored",
			"name", app.GetName(), "namespace", app.GetNamespace(), "policy", policyRef)
		return false, nil

	case active && managedByUs:
		// Still paused by us — count it.
		return true, nil

	default:
		// Not active, not managed by us — nothing to do.
		return false, nil
	}
}
