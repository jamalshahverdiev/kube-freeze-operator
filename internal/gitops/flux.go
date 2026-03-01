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
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	freezev1alpha1 "github.com/jamalshahverdiev/kube-freeze-operator/api/v1alpha1"
)

var (
	fluxKustomizationGVK = schema.GroupVersionKind{
		Group:   "kustomize.toolkit.fluxcd.io",
		Version: "v1",
		Kind:    "Kustomization",
	}
	fluxHelmReleaseGVK = schema.GroupVersionKind{
		Group:   "helm.toolkit.fluxcd.io",
		Version: "v2",
		Kind:    "HelmRelease",
	}
)

// reconcileFlux manages spec.suspend on Flux Kustomization and HelmRelease objects.
// If Flux CRDs are not installed the function returns (0, nil) gracefully.
//
// Returns the total number of Flux objects currently suspended by this policy.
func reconcileFlux(
	ctx context.Context,
	c client.Client,
	spec *freezev1alpha1.GitOpsFluxSpec,
	policyRef string,
	active bool,
) (int, error) {
	if spec == nil {
		return 0, nil
	}

	namespaces, err := listMatchingNamespaces(ctx, c, spec.NamespaceSelector)
	if err != nil {
		return 0, fmt.Errorf("list namespaces: %w", err)
	}

	total := 0

	// Kustomizations
	n, err := reconcileFluxResources(ctx, c, fluxKustomizationGVK, spec.KustomizationSelector, namespaces, policyRef, active)
	if err != nil {
		return total, err
	}
	total += n

	// HelmReleases
	n, err = reconcileFluxResources(ctx, c, fluxHelmReleaseGVK, spec.HelmReleaseSelector, namespaces, policyRef, active)
	if err != nil {
		return total, err
	}
	total += n

	return total, nil
}

// reconcileFluxResources manages a single Flux CRD kind across matching namespaces.
func reconcileFluxResources(
	ctx context.Context,
	c client.Client,
	gvk schema.GroupVersionKind,
	selector *metav1.LabelSelector,
	namespaces []string,
	policyRef string,
	active bool,
) (int, error) {
	logger := log.FromContext(ctx).WithName("gitops.flux").WithValues("kind", gvk.Kind)

	var matchingSelector client.MatchingLabelsSelector
	if selector != nil {
		sel, err := metav1.LabelSelectorAsSelector(selector)
		if err != nil {
			return 0, fmt.Errorf("invalid selector for %s: %w", gvk.Kind, err)
		}
		matchingSelector = client.MatchingLabelsSelector{Selector: sel}
	}

	total := 0

	for _, ns := range namespaces {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   gvk.Group,
			Version: gvk.Version,
			Kind:    gvk.Kind + "List",
		})

		listOpts := []client.ListOption{client.InNamespace(ns)}
		if selector != nil {
			listOpts = append(listOpts, matchingSelector)
		}

		if err := c.List(ctx, list, listOpts...); err != nil {
			if isNoMatchError(err) {
				logger.V(1).Info("Flux CRD not found, skipping", "namespace", ns)
				return 0, nil
			}
			return total, fmt.Errorf("list %s in namespace %s: %w", gvk.Kind, ns, err)
		}

		for i := range list.Items {
			obj := &list.Items[i]
			suspended, err := reconcileFluxObject(ctx, c, obj, gvk.Kind, policyRef, active)
			if err != nil {
				logger.Error(err, "failed to reconcile Flux object",
					"name", obj.GetName(), "namespace", obj.GetNamespace())
				continue
			}
			if suspended {
				total++
			}
		}
	}

	return total, nil
}

// reconcileFluxObject manages spec.suspend on a single Flux object.
// Returns true if the object is currently suspended by the operator.
func reconcileFluxObject(
	ctx context.Context,
	c client.Client,
	obj *unstructured.Unstructured,
	kind string,
	policyRef string,
	active bool,
) (bool, error) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	managedByUs := annotations[AnnotationManaged] == "true" &&
		annotations[AnnotationManagedByPolicy] == policyRef

	switch {
	case active && !managedByUs:
		// Capture current suspend state.
		suspended, _, _ := unstructured.NestedBool(obj.Object, "spec", "suspend")
		annotations[AnnotationOriginalSuspend] = strconv.FormatBool(suspended)
		annotations[AnnotationManaged] = "true"
		annotations[AnnotationManagedByPolicy] = policyRef
		obj.SetAnnotations(annotations)

		// Suspend the resource.
		if err := unstructured.SetNestedField(obj.Object, true, "spec", "suspend"); err != nil {
			return false, fmt.Errorf("set spec.suspend: %w", err)
		}

		if err := c.Update(ctx, obj); err != nil {
			return false, fmt.Errorf("suspend %s %s/%s: %w", kind, obj.GetNamespace(), obj.GetName(), err)
		}
		log.FromContext(ctx).Info("Flux resource suspended",
			"kind", kind, "name", obj.GetName(), "namespace", obj.GetNamespace(), "policy", policyRef)
		return true, nil

	case !active && managedByUs:
		// Restore original suspend value.
		originalStr := annotations[AnnotationOriginalSuspend]
		original, parseErr := strconv.ParseBool(originalStr)
		if parseErr != nil {
			original = false // safe default: resume
		}

		if err := unstructured.SetNestedField(obj.Object, original, "spec", "suspend"); err != nil {
			return false, fmt.Errorf("restore spec.suspend: %w", err)
		}

		delete(annotations, AnnotationManaged)
		delete(annotations, AnnotationManagedByPolicy)
		delete(annotations, AnnotationOriginalSuspend)
		obj.SetAnnotations(annotations)

		if err := c.Update(ctx, obj); err != nil {
			return false, fmt.Errorf("restore %s %s/%s: %w", kind, obj.GetNamespace(), obj.GetName(), err)
		}
		log.FromContext(ctx).Info("Flux resource restored",
			"kind", kind, "name", obj.GetName(), "namespace", obj.GetNamespace(), "policy", policyRef)
		return false, nil

	case active && managedByUs:
		return true, nil

	default:
		return false, nil
	}
}
