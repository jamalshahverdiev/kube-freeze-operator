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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	freezev1alpha1 "github.com/jamalshahverdiev/kube-freeze-operator/api/v1alpha1"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

const (
	testNSArgoCD = "argocd"
	testNSFlux   = "flux-system"
	originalNull = "null"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	return s
}

func newNamespace(name string, labels map[string]string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
	}
}

func newArgoCDApp(name string, labels map[string]string, autoSync map[string]any) *unstructured.Unstructured {
	app := &unstructured.Unstructured{}
	app.SetGroupVersionKind(argoCDApplicationGVK)
	app.SetName(name)
	app.SetNamespace(testNSArgoCD)
	if labels != nil {
		app.SetLabels(labels)
	}
	spec := map[string]any{
		"project": "default",
		"source":  map[string]any{"repoURL": "https://example.com/repo", "path": "."},
	}
	if autoSync != nil {
		spec["syncPolicy"] = map[string]any{"automated": autoSync}
	}
	app.Object["spec"] = spec
	return app
}

func newFluxResource(gvk schema.GroupVersionKind, name string, labels map[string]string, suspend bool) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetName(name)
	obj.SetNamespace(testNSFlux)
	if labels != nil {
		obj.SetLabels(labels)
	}
	obj.Object["spec"] = map[string]any{
		"suspend":  suspend,
		"interval": "5m",
	}
	return obj
}

func buildFakeClient(objs ...client.Object) client.Client {
	s := newScheme()
	s.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "argoproj.io", Version: "v1alpha1", Kind: "ApplicationList"},
		&unstructured.UnstructuredList{},
	)
	s.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Kind: "KustomizationList"},
		&unstructured.UnstructuredList{},
	)
	s.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "helm.toolkit.fluxcd.io", Version: "v2", Kind: "HelmReleaseList"},
		&unstructured.UnstructuredList{},
	)

	return fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		Build()
}

func getApp(ctx context.Context, c client.Client, name string) *unstructured.Unstructured {
	app := &unstructured.Unstructured{}
	app.SetGroupVersionKind(argoCDApplicationGVK)
	_ = c.Get(ctx, client.ObjectKey{Name: name, Namespace: testNSArgoCD}, app)
	return app
}

func getFlux(ctx context.Context, c client.Client, gvk schema.GroupVersionKind, name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	_ = c.Get(ctx, client.ObjectKey{Name: name, Namespace: testNSFlux}, obj)
	return obj
}

// ---------------------------------------------------------------------------
// ArgoCD tests
// ---------------------------------------------------------------------------

func TestReconcileArgoCDApp_Pause(t *testing.T) {
	ctx := context.Background()
	autoSync := map[string]any{"prune": true, "selfHeal": false}
	app := newArgoCDApp("myapp", map[string]string{"env": "prod"}, autoSync)
	c := buildFakeClient(
		newNamespace("argocd", map[string]string{"env": "prod"}),
		app,
	)

	spec := &freezev1alpha1.GitOpsArgoCDSpec{
		ApplicationSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
		NamespaceSelector:   &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
	}

	n, err := reconcileArgoCD(ctx, c, spec, "test-freeze", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 paused, got %d", n)
	}

	got := getApp(ctx, c, "myapp")
	ann := got.GetAnnotations()
	if ann[AnnotationManaged] != annotationManagedValue {
		t.Errorf("expected managed=true, got %q", ann[AnnotationManaged])
	}
	if ann[AnnotationManagedByPolicy] != "test-freeze" {
		t.Errorf("expected policy=test-freeze, got %q", ann[AnnotationManagedByPolicy])
	}
	if ann[AnnotationOriginalAutoSync] == "" || ann[AnnotationOriginalAutoSync] == originalNull {
		t.Errorf("expected original autosync stored, got %q", ann[AnnotationOriginalAutoSync])
	}
	// autoSync should be removed
	_, found, _ := unstructured.NestedMap(got.Object, "spec", "syncPolicy", "automated")
	if found {
		t.Error("expected spec.syncPolicy.automated to be removed after pause")
	}
}

func TestReconcileArgoCDApp_Restore(t *testing.T) {
	ctx := context.Background()
	// Create app already paused by the operator
	app := newArgoCDApp("myapp", map[string]string{"env": "prod"}, nil)
	app.SetAnnotations(map[string]string{
		AnnotationManaged:          annotationManagedValue,
		AnnotationManagedByPolicy:  "test-freeze",
		AnnotationOriginalAutoSync: `{"prune":true,"selfHeal":false}`,
	})
	c := buildFakeClient(
		newNamespace("argocd", map[string]string{"env": "prod"}),
		app,
	)

	spec := &freezev1alpha1.GitOpsArgoCDSpec{
		ApplicationSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
		NamespaceSelector:   &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
	}

	n, err := reconcileArgoCD(ctx, c, spec, "test-freeze", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 paused after restore, got %d", n)
	}

	got := getApp(ctx, c, "myapp")
	ann := got.GetAnnotations()
	if ann[AnnotationManaged] != "" {
		t.Errorf("expected managed annotation removed, got %q", ann[AnnotationManaged])
	}
	// autoSync should be restored
	automated, found, _ := unstructured.NestedMap(got.Object, "spec", "syncPolicy", "automated")
	if !found {
		t.Fatal("expected spec.syncPolicy.automated to be restored")
	}
	if automated["prune"] != true {
		t.Errorf("expected prune=true restored, got %v", automated["prune"])
	}
}

func TestReconcileArgoCDApp_AlreadyPaused_Idempotent(t *testing.T) {
	ctx := context.Background()
	app := newArgoCDApp("myapp", map[string]string{"env": "prod"}, nil)
	app.SetAnnotations(map[string]string{
		AnnotationManaged:          annotationManagedValue,
		AnnotationManagedByPolicy:  "test-freeze",
		AnnotationOriginalAutoSync: `{"prune":true}`,
	})
	c := buildFakeClient(
		newNamespace("argocd", map[string]string{"env": "prod"}),
		app,
	)

	spec := &freezev1alpha1.GitOpsArgoCDSpec{
		ApplicationSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
		NamespaceSelector:   &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
	}

	// Active=true but already managed — should just count, not re-pause
	n, err := reconcileArgoCD(ctx, c, spec, "test-freeze", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 (counted), got %d", n)
	}
}

func TestReconcileArgoCDApp_AutoSyncNull_Pause(t *testing.T) {
	ctx := context.Background()
	// App without autoSync
	app := newArgoCDApp("myapp", map[string]string{"env": "prod"}, nil)
	c := buildFakeClient(
		newNamespace("argocd", map[string]string{"env": "prod"}),
		app,
	)

	spec := &freezev1alpha1.GitOpsArgoCDSpec{
		ApplicationSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
		NamespaceSelector:   &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
	}

	n, err := reconcileArgoCD(ctx, c, spec, "test-freeze", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 paused, got %d", n)
	}

	got := getApp(ctx, c, "myapp")
	ann := got.GetAnnotations()
	if ann[AnnotationOriginalAutoSync] != originalNull {
		t.Errorf("expected original-autosync=null, got %q", ann[AnnotationOriginalAutoSync])
	}
}

func TestReconcileArgoCDApp_AutoSyncNull_Restore(t *testing.T) {
	ctx := context.Background()
	// App paused with original=null — should NOT enable autosync on restore
	app := newArgoCDApp("myapp", map[string]string{"env": "prod"}, nil)
	app.SetAnnotations(map[string]string{
		AnnotationManaged:          annotationManagedValue,
		AnnotationManagedByPolicy:  "test-freeze",
		AnnotationOriginalAutoSync: originalNull,
	})
	c := buildFakeClient(
		newNamespace("argocd", map[string]string{"env": "prod"}),
		app,
	)

	spec := &freezev1alpha1.GitOpsArgoCDSpec{
		ApplicationSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
		NamespaceSelector:   &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
	}

	_, err := reconcileArgoCD(ctx, c, spec, "test-freeze", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := getApp(ctx, c, "myapp")
	_, found, _ := unstructured.NestedMap(got.Object, "spec", "syncPolicy", "automated")
	if found {
		t.Error("expected autosync to remain absent when original was null")
	}
}

func TestReconcileArgoCDApp_DifferentPolicy_NotTouched(t *testing.T) {
	ctx := context.Background()
	// App paused by a different policy
	app := newArgoCDApp("myapp", map[string]string{"env": "prod"}, nil)
	app.SetAnnotations(map[string]string{
		AnnotationManaged:          annotationManagedValue,
		AnnotationManagedByPolicy:  "other-freeze",
		AnnotationOriginalAutoSync: `{"prune":true}`,
	})
	c := buildFakeClient(
		newNamespace("argocd", map[string]string{"env": "prod"}),
		app,
	)

	spec := &freezev1alpha1.GitOpsArgoCDSpec{
		ApplicationSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
		NamespaceSelector:   &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
	}

	// Active=false for our policy — should NOT restore an app managed by different policy
	n, err := reconcileArgoCD(ctx, c, spec, "test-freeze", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}

	got := getApp(ctx, c, "myapp")
	ann := got.GetAnnotations()
	if ann[AnnotationManagedByPolicy] != "other-freeze" {
		t.Errorf("expected other-freeze untouched, got %q", ann[AnnotationManagedByPolicy])
	}
}

func TestReconcileArgoCDApp_SelectorFiltering(t *testing.T) {
	ctx := context.Background()
	// Two apps: one matches label selector, one doesn't
	app1 := newArgoCDApp("prod-app", map[string]string{"env": "prod"}, map[string]any{"prune": true})
	app2 := newArgoCDApp("staging-app", map[string]string{"env": "staging"}, map[string]any{"selfHeal": true})
	c := buildFakeClient(
		newNamespace("argocd", map[string]string{"env": "prod"}),
		app1, app2,
	)

	spec := &freezev1alpha1.GitOpsArgoCDSpec{
		ApplicationSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
		NamespaceSelector:   &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
	}

	n, err := reconcileArgoCD(ctx, c, spec, "test-freeze", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 paused (only prod), got %d", n)
	}

	// staging-app should not be touched
	staging := getApp(ctx, c, "staging-app")
	ann := staging.GetAnnotations()
	if ann[AnnotationManaged] == annotationManagedValue {
		t.Error("staging-app should not be managed")
	}
	_, found, _ := unstructured.NestedMap(staging.Object, "spec", "syncPolicy", "automated")
	if !found {
		t.Error("staging-app autoSync should still be present")
	}
}

func TestReconcileArgoCDApp_MultipleApps(t *testing.T) {
	ctx := context.Background()
	app1 := newArgoCDApp("app1", map[string]string{"env": "prod"}, map[string]any{"prune": true})
	app2 := newArgoCDApp("app2", map[string]string{"env": "prod"}, map[string]any{"selfHeal": true})
	app3 := newArgoCDApp("app3", map[string]string{"env": "prod"}, map[string]any{})
	c := buildFakeClient(
		newNamespace("argocd", map[string]string{"env": "prod"}),
		app1, app2, app3,
	)

	spec := &freezev1alpha1.GitOpsArgoCDSpec{
		ApplicationSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
		NamespaceSelector:   &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
	}

	n, err := reconcileArgoCD(ctx, c, spec, "test-freeze", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 paused, got %d", n)
	}

	for _, name := range []string{"app1", "app2", "app3"} {
		got := getApp(ctx, c, name)
		ann := got.GetAnnotations()
		if ann[AnnotationManaged] != annotationManagedValue {
			t.Errorf("%s: expected managed=true", name)
		}
	}
}

func TestReconcileArgoCD_NilSpec(t *testing.T) {
	ctx := context.Background()
	c := buildFakeClient()
	n, err := reconcileArgoCD(ctx, c, nil, "test-freeze", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// Flux tests
// ---------------------------------------------------------------------------

func TestReconcileFluxObject_Suspend(t *testing.T) {
	ctx := context.Background()
	ks := newFluxResource(fluxKustomizationGVK, "my-ks", map[string]string{"env": "prod"}, false)
	c := buildFakeClient(
		newNamespace("flux-system", map[string]string{"env": "prod"}),
		ks,
	)

	spec := &freezev1alpha1.GitOpsFluxSpec{
		KustomizationSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
		NamespaceSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
	}

	n, err := reconcileFlux(ctx, c, spec, "test-freeze", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 suspended, got %d", n)
	}

	got := getFlux(ctx, c, fluxKustomizationGVK, "my-ks")
	ann := got.GetAnnotations()
	if ann[AnnotationManaged] != annotationManagedValue {
		t.Errorf("expected managed=true, got %q", ann[AnnotationManaged])
	}
	if ann[AnnotationOriginalSuspend] != "false" {
		t.Errorf("expected original-suspend=false, got %q", ann[AnnotationOriginalSuspend])
	}
	suspended, _, _ := unstructured.NestedBool(got.Object, "spec", "suspend")
	if !suspended {
		t.Error("expected spec.suspend=true after pause")
	}
}

func TestReconcileFluxObject_Restore(t *testing.T) {
	ctx := context.Background()
	ks := newFluxResource(fluxKustomizationGVK, "my-ks", map[string]string{"env": "prod"}, true)
	ks.SetAnnotations(map[string]string{
		AnnotationManaged:         annotationManagedValue,
		AnnotationManagedByPolicy: "test-freeze",
		AnnotationOriginalSuspend: "false",
	})
	c := buildFakeClient(
		newNamespace("flux-system", map[string]string{"env": "prod"}),
		ks,
	)

	spec := &freezev1alpha1.GitOpsFluxSpec{
		KustomizationSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
		NamespaceSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
	}

	n, err := reconcileFlux(ctx, c, spec, "test-freeze", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 after restore, got %d", n)
	}

	got := getFlux(ctx, c, fluxKustomizationGVK, "my-ks")
	ann := got.GetAnnotations()
	if ann[AnnotationManaged] != "" {
		t.Errorf("expected managed annotation removed, got %q", ann[AnnotationManaged])
	}
	suspended, _, _ := unstructured.NestedBool(got.Object, "spec", "suspend")
	if suspended {
		t.Error("expected spec.suspend=false after restore")
	}
}

func TestReconcileFluxObject_AlreadySuspended_PreservesOriginal(t *testing.T) {
	ctx := context.Background()
	// Resource was already suspended=true before freeze
	ks := newFluxResource(fluxKustomizationGVK, "my-ks", map[string]string{"env": "prod"}, true)
	c := buildFakeClient(
		newNamespace("flux-system", map[string]string{"env": "prod"}),
		ks,
	)

	spec := &freezev1alpha1.GitOpsFluxSpec{
		KustomizationSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
		NamespaceSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
	}

	// Pause
	_, err := reconcileFlux(ctx, c, spec, "test-freeze", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := getFlux(ctx, c, fluxKustomizationGVK, "my-ks")
	ann := got.GetAnnotations()
	if ann[AnnotationOriginalSuspend] != "true" {
		t.Errorf("expected original-suspend=true (was already suspended), got %q", ann[AnnotationOriginalSuspend])
	}

	// Restore — should restore to true (was already suspended)
	_, err = reconcileFlux(ctx, c, spec, "test-freeze", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got = getFlux(ctx, c, fluxKustomizationGVK, "my-ks")
	suspended, _, _ := unstructured.NestedBool(got.Object, "spec", "suspend")
	if !suspended {
		t.Error("expected spec.suspend=true after restore (was originally suspended)")
	}
}

func TestReconcileFlux_HelmRelease(t *testing.T) {
	ctx := context.Background()
	hr := newFluxResource(fluxHelmReleaseGVK, "my-hr", map[string]string{"env": "prod"}, false)
	c := buildFakeClient(
		newNamespace("flux-system", map[string]string{"env": "prod"}),
		hr,
	)

	spec := &freezev1alpha1.GitOpsFluxSpec{
		HelmReleaseSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
		NamespaceSelector:   &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
	}

	n, err := reconcileFlux(ctx, c, spec, "test-freeze", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 suspended, got %d", n)
	}

	got := getFlux(ctx, c, fluxHelmReleaseGVK, "my-hr")
	suspended, _, _ := unstructured.NestedBool(got.Object, "spec", "suspend")
	if !suspended {
		t.Error("expected HelmRelease spec.suspend=true")
	}

	// Restore
	n, err = reconcileFlux(ctx, c, spec, "test-freeze", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 after restore, got %d", n)
	}

	got = getFlux(ctx, c, fluxHelmReleaseGVK, "my-hr")
	suspended, _, _ = unstructured.NestedBool(got.Object, "spec", "suspend")
	if suspended {
		t.Error("expected HelmRelease spec.suspend=false after restore")
	}
}

func TestReconcileFlux_KustomizationAndHelmRelease(t *testing.T) {
	ctx := context.Background()
	ks := newFluxResource(fluxKustomizationGVK, "my-ks", map[string]string{"env": "prod"}, false)
	hr := newFluxResource(fluxHelmReleaseGVK, "my-hr", map[string]string{"env": "prod"}, false)
	c := buildFakeClient(
		newNamespace("flux-system", map[string]string{"env": "prod"}),
		ks, hr,
	)

	spec := &freezev1alpha1.GitOpsFluxSpec{
		KustomizationSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
		HelmReleaseSelector:   &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
		NamespaceSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
	}

	n, err := reconcileFlux(ctx, c, spec, "test-freeze", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 suspended (ks+hr), got %d", n)
	}
}

func TestReconcileFlux_SelectorFiltering(t *testing.T) {
	ctx := context.Background()
	ksProd := newFluxResource(fluxKustomizationGVK, "prod-ks", map[string]string{"env": "prod"}, false)
	ksStaging := newFluxResource(fluxKustomizationGVK, "staging-ks", map[string]string{"env": "staging"}, false)
	c := buildFakeClient(
		newNamespace("flux-system", map[string]string{"env": "prod"}),
		ksProd, ksStaging,
	)

	spec := &freezev1alpha1.GitOpsFluxSpec{
		KustomizationSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
		NamespaceSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
	}

	n, err := reconcileFlux(ctx, c, spec, "test-freeze", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 (only prod), got %d", n)
	}

	// staging should not be touched
	staging := getFlux(ctx, c, fluxKustomizationGVK, "staging-ks")
	ann := staging.GetAnnotations()
	if ann[AnnotationManaged] == annotationManagedValue {
		t.Error("staging-ks should not be managed")
	}
}

func TestReconcileFlux_DifferentPolicy_NotTouched(t *testing.T) {
	ctx := context.Background()
	ks := newFluxResource(fluxKustomizationGVK, "my-ks", map[string]string{"env": "prod"}, true)
	ks.SetAnnotations(map[string]string{
		AnnotationManaged:         annotationManagedValue,
		AnnotationManagedByPolicy: "other-freeze",
		AnnotationOriginalSuspend: "false",
	})
	c := buildFakeClient(
		newNamespace("flux-system", map[string]string{"env": "prod"}),
		ks,
	)

	spec := &freezev1alpha1.GitOpsFluxSpec{
		KustomizationSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
		NamespaceSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
	}

	n, err := reconcileFlux(ctx, c, spec, "test-freeze", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}

	got := getFlux(ctx, c, fluxKustomizationGVK, "my-ks")
	ann := got.GetAnnotations()
	if ann[AnnotationManagedByPolicy] != "other-freeze" {
		t.Errorf("expected other-freeze untouched, got %q", ann[AnnotationManagedByPolicy])
	}
}

func TestReconcileFlux_NilSpec(t *testing.T) {
	ctx := context.Background()
	c := buildFakeClient()
	n, err := reconcileFlux(ctx, c, nil, "test-freeze", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// Reconciler (top-level) tests
// ---------------------------------------------------------------------------

func TestReconciler_BothProviders(t *testing.T) {
	ctx := context.Background()
	app := newArgoCDApp("myapp", map[string]string{"env": "prod"}, map[string]any{"prune": true})
	ks := newFluxResource(fluxKustomizationGVK, "my-ks", map[string]string{"env": "prod"}, false)
	hr := newFluxResource(fluxHelmReleaseGVK, "my-hr", map[string]string{"env": "prod"}, false)
	c := buildFakeClient(
		newNamespace("argocd", map[string]string{"env": "prod"}),
		newNamespace("flux-system", map[string]string{"env": "prod"}),
		app, ks, hr,
	)

	gitopsSpec := &freezev1alpha1.GitOpsSpec{
		Enabled:   true,
		Providers: []freezev1alpha1.GitOpsProvider{freezev1alpha1.GitOpsProviderArgoCD, freezev1alpha1.GitOpsProviderFlux},
		ArgoCD: &freezev1alpha1.GitOpsArgoCDSpec{
			ApplicationSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
			NamespaceSelector:   &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
		},
		Flux: &freezev1alpha1.GitOpsFluxSpec{
			KustomizationSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
			HelmReleaseSelector:   &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
			NamespaceSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
		},
	}

	r := &Reconciler{Client: c}
	result, err := r.Reconcile(ctx, gitopsSpec, "test-freeze", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PausedCount != 3 {
		t.Fatalf("expected 3 paused (1 argo + 1 ks + 1 hr), got %d", result.PausedCount)
	}
	if result.ReconcileTime.IsZero() {
		t.Error("expected ReconcileTime to be set")
	}
}

func TestReconciler_Disabled(t *testing.T) {
	ctx := context.Background()
	c := buildFakeClient()

	gitopsSpec := &freezev1alpha1.GitOpsSpec{
		Enabled: false,
	}
	r := &Reconciler{Client: c}
	result, err := r.Reconcile(ctx, gitopsSpec, "test-freeze", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PausedCount != 0 {
		t.Fatalf("expected 0 when disabled, got %d", result.PausedCount)
	}
}

func TestReconciler_NilSpec(t *testing.T) {
	ctx := context.Background()
	c := buildFakeClient()
	r := &Reconciler{Client: c}
	result, err := r.Reconcile(ctx, nil, "test-freeze", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PausedCount != 0 {
		t.Fatalf("expected 0 for nil spec, got %d", result.PausedCount)
	}
}

func TestReconciler_OnlyArgoCD(t *testing.T) {
	ctx := context.Background()
	app := newArgoCDApp("myapp", map[string]string{"env": "prod"}, map[string]any{"prune": true})
	c := buildFakeClient(
		newNamespace("argocd", map[string]string{"env": "prod"}),
		app,
	)

	gitopsSpec := &freezev1alpha1.GitOpsSpec{
		Enabled:   true,
		Providers: []freezev1alpha1.GitOpsProvider{freezev1alpha1.GitOpsProviderArgoCD},
		ArgoCD: &freezev1alpha1.GitOpsArgoCDSpec{
			ApplicationSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
			NamespaceSelector:   &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
		},
	}

	r := &Reconciler{Client: c}
	result, err := r.Reconcile(ctx, gitopsSpec, "test-freeze", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PausedCount != 1 {
		t.Fatalf("expected 1, got %d", result.PausedCount)
	}
}

func TestReconciler_OnlyFlux(t *testing.T) {
	ctx := context.Background()
	ks := newFluxResource(fluxKustomizationGVK, "my-ks", map[string]string{"env": "prod"}, false)
	c := buildFakeClient(
		newNamespace("flux-system", map[string]string{"env": "prod"}),
		ks,
	)

	gitopsSpec := &freezev1alpha1.GitOpsSpec{
		Enabled:   true,
		Providers: []freezev1alpha1.GitOpsProvider{freezev1alpha1.GitOpsProviderFlux},
		Flux: &freezev1alpha1.GitOpsFluxSpec{
			KustomizationSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
			NamespaceSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
		},
	}

	r := &Reconciler{Client: c}
	result, err := r.Reconcile(ctx, gitopsSpec, "test-freeze", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PausedCount != 1 {
		t.Fatalf("expected 1, got %d", result.PausedCount)
	}
}

func TestReconciler_FullCycle_PauseAndRestore(t *testing.T) {
	ctx := context.Background()
	app := newArgoCDApp("myapp", map[string]string{"env": "prod"}, map[string]any{"prune": true, "selfHeal": false})
	ks := newFluxResource(fluxKustomizationGVK, "my-ks", map[string]string{"env": "prod"}, false)
	hr := newFluxResource(fluxHelmReleaseGVK, "my-hr", map[string]string{"env": "prod"}, false)
	c := buildFakeClient(
		newNamespace("argocd", map[string]string{"env": "prod"}),
		newNamespace("flux-system", map[string]string{"env": "prod"}),
		app, ks, hr,
	)

	gitopsSpec := &freezev1alpha1.GitOpsSpec{
		Enabled:   true,
		Providers: []freezev1alpha1.GitOpsProvider{freezev1alpha1.GitOpsProviderArgoCD, freezev1alpha1.GitOpsProviderFlux},
		ArgoCD: &freezev1alpha1.GitOpsArgoCDSpec{
			ApplicationSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
			NamespaceSelector:   &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
		},
		Flux: &freezev1alpha1.GitOpsFluxSpec{
			KustomizationSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
			HelmReleaseSelector:   &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
			NamespaceSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
		},
	}

	r := &Reconciler{Client: c}

	// --- PAUSE ---
	result, err := r.Reconcile(ctx, gitopsSpec, "test-freeze", true)
	if err != nil {
		t.Fatalf("pause: unexpected error: %v", err)
	}
	if result.PausedCount != 3 {
		t.Fatalf("pause: expected 3, got %d", result.PausedCount)
	}

	// Verify ArgoCD paused
	gotApp := getApp(ctx, c, "myapp")
	_, found, _ := unstructured.NestedMap(gotApp.Object, "spec", "syncPolicy", "automated")
	if found {
		t.Error("pause: expected ArgoCD autosync removed")
	}

	// Verify Flux Kustomization suspended
	gotKs := getFlux(ctx, c, fluxKustomizationGVK, "my-ks")
	suspended, _, _ := unstructured.NestedBool(gotKs.Object, "spec", "suspend")
	if !suspended {
		t.Error("pause: expected Kustomization suspend=true")
	}

	// Verify Flux HelmRelease suspended
	gotHr := getFlux(ctx, c, fluxHelmReleaseGVK, "my-hr")
	suspended, _, _ = unstructured.NestedBool(gotHr.Object, "spec", "suspend")
	if !suspended {
		t.Error("pause: expected HelmRelease suspend=true")
	}

	// --- RESTORE ---
	result, err = r.Reconcile(ctx, gitopsSpec, "test-freeze", false)
	if err != nil {
		t.Fatalf("restore: unexpected error: %v", err)
	}
	if result.PausedCount != 0 {
		t.Fatalf("restore: expected 0, got %d", result.PausedCount)
	}

	// Verify ArgoCD restored
	gotApp = getApp(ctx, c, "myapp")
	automated, found, _ := unstructured.NestedMap(gotApp.Object, "spec", "syncPolicy", "automated")
	if !found {
		t.Error("restore: expected ArgoCD autosync restored")
	}
	if automated["prune"] != true {
		t.Errorf("restore: expected prune=true, got %v", automated["prune"])
	}

	// Verify Flux Kustomization restored
	gotKs = getFlux(ctx, c, fluxKustomizationGVK, "my-ks")
	suspended, _, _ = unstructured.NestedBool(gotKs.Object, "spec", "suspend")
	if suspended {
		t.Error("restore: expected Kustomization suspend=false")
	}

	// Verify Flux HelmRelease restored
	gotHr = getFlux(ctx, c, fluxHelmReleaseGVK, "my-hr")
	suspended, _, _ = unstructured.NestedBool(gotHr.Object, "spec", "suspend")
	if suspended {
		t.Error("restore: expected HelmRelease suspend=false")
	}

	// All annotations should be removed
	for _, obj := range []*unstructured.Unstructured{gotApp, gotKs, gotHr} {
		ann := obj.GetAnnotations()
		if ann[AnnotationManaged] != "" {
			t.Errorf("restore: %s %s still has managed annotation", obj.GetKind(), obj.GetName())
		}
	}
}
