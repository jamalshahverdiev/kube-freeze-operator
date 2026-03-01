package workloads

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	freezev1alpha1 "github.com/jamalshahverdiev/kube-freeze-operator/api/v1alpha1"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	must(t, corev1.AddToScheme(s))
	must(t, appsv1.AddToScheme(s))
	must(t, freezev1alpha1.AddToScheme(s))
	return s
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("scheme setup: %v", err)
	}
}

func ptrInt32(v int32) *int32 { return &v }

// makeDeployment returns a minimal Deployment ready for JSON serialization.
func makeDeployment(ns, name string, lbls map[string]string, image string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: lbls},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptrInt32(replicas),
			Selector: &metav1.LabelSelector{MatchLabels: lbls},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: lbls},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: image}}},
			},
		},
	}
}

func mustJSON(t *testing.T, obj interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// deploymentGVK is the GroupVersionKind for Deployment admission requests.
var deploymentGVK = metav1.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
var deploymentGVR = metav1.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

func makeCreateRequest(t *testing.T, dep *appsv1.Deployment, username string, groups []string) admission.Request {
	t.Helper()
	return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		UID:       "uid-create",
		Kind:      deploymentGVK,
		Resource:  deploymentGVR,
		Operation: admissionv1.Create,
		Namespace: dep.Namespace,
		Name:      dep.Name,
		Object:    runtime.RawExtension{Raw: mustJSON(t, dep)},
		UserInfo:  authv1.UserInfo{Username: username, Groups: groups},
	}}
}

func makeUpdateRequest(t *testing.T, oldDep, newDep *appsv1.Deployment, username string, groups []string) admission.Request {
	t.Helper()
	return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		UID:       "uid-update",
		Kind:      deploymentGVK,
		Resource:  deploymentGVR,
		Operation: admissionv1.Update,
		Namespace: newDep.Namespace,
		Name:      newDep.Name,
		OldObject: runtime.RawExtension{Raw: mustJSON(t, oldDep)},
		Object:    runtime.RawExtension{Raw: mustJSON(t, newDep)},
		UserInfo:  authv1.UserInfo{Username: username, Groups: groups},
	}}
}

func makeDeleteRequest(t *testing.T, dep *appsv1.Deployment, username string, groups []string) admission.Request {
	t.Helper()
	return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		UID:       "uid-delete",
		Kind:      deploymentGVK,
		Resource:  deploymentGVR,
		Operation: admissionv1.Delete,
		Namespace: dep.Namespace,
		Name:      dep.Name,
		OldObject: runtime.RawExtension{Raw: mustJSON(t, dep)},
		UserInfo:  authv1.UserInfo{Username: username, Groups: groups},
	}}
}

func makeScaleRequest(t *testing.T, ns, name string, username string) admission.Request {
	t.Helper()
	// Scale subresource — object is autoscaler Scale, not a Deployment
	scaleObj := map[string]interface{}{
		"apiVersion": "autoscaling/v1",
		"kind":       "Scale",
		"metadata":   map[string]string{"name": name, "namespace": ns},
		"spec":       map[string]int{"replicas": 5},
	}
	return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		UID:         "uid-scale",
		Kind:        deploymentGVK,
		Resource:    deploymentGVR,
		SubResource: "scale",
		Operation:   admissionv1.Update,
		Namespace:   ns,
		Name:        name,
		Object:      runtime.RawExtension{Raw: mustJSON(t, scaleObj)},
		UserInfo:    authv1.UserInfo{Username: username, Groups: []string{"system:authenticated"}},
	}}
}

// activeChangeFreeze returns a ChangeFreeze that is active right now.
func activeChangeFreeze(name, ns string, deny []freezev1alpha1.Action) *freezev1alpha1.ChangeFreeze {
	now := time.Now().UTC()
	return &freezev1alpha1.ChangeFreeze{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: freezev1alpha1.ChangeFreezeSpec{
			StartTime: metav1.Time{Time: now.Add(-time.Hour)},
			EndTime:   metav1.Time{Time: now.Add(time.Hour)},
			Target: freezev1alpha1.TargetSpec{
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": ns}},
				Kinds:             []freezev1alpha1.TargetKind{freezev1alpha1.TargetKindDeployment},
			},
			Rules:   freezev1alpha1.PolicyRulesSpec{Deny: deny},
			Message: freezev1alpha1.MessageSpec{Reason: "test freeze"},
		},
	}
}

// activeFreezeException returns a FreezeException that is active right now.
func activeFreezeException(name, ns string, allow []freezev1alpha1.Action) *freezev1alpha1.FreezeException {
	now := time.Now().UTC()
	return &freezev1alpha1.FreezeException{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: freezev1alpha1.FreezeExceptionSpec{
			ActiveFrom: metav1.Time{Time: now.Add(-time.Minute)},
			ActiveTo:   metav1.Time{Time: now.Add(time.Minute)},
			Target: freezev1alpha1.TargetSpec{
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": ns}},
				Kinds:             []freezev1alpha1.TargetKind{freezev1alpha1.TargetKindDeployment},
			},
			Allow:  allow,
			Reason: "hotfix",
		},
	}
}

func prodNamespace() *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "prod",
			Labels: map[string]string{"env": "prod"},
		},
	}
}

// buildValidator constructs a ready-to-use Validator backed by a fake client.
func buildValidator(t *testing.T, objs ...runtime.Object) *Validator {
	t.Helper()
	s := testScheme(t)
	clientObjs := make([]runtime.Object, 0, len(objs))
	clientObjs = append(clientObjs, objs...)

	// fake.NewClientBuilder expects client.Object, convert
	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(clientObjs...).
		Build()

	decoder := admission.NewDecoder(s)
	return &Validator{
		Client:            cl,
		Reader:            cl,
		Decoder:           decoder,
		OperatorNamespace: "kube-system",
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// 1. No policies → everything is allowed.
func TestValidator_NoPolicies_AllowCreate(t *testing.T) {
	g := NewWithT(t)
	v := buildValidator(t, prodNamespace())
	dep := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v1", 1)

	resp := v.Handle(context.Background(), makeCreateRequest(t, dep, "user@example.com", []string{"system:authenticated"}))
	g.Expect(resp.Allowed).To(BeTrue(), "expected allow without any freeze policies: %s", resp.Result.Message)
}

// 2. Active ChangeFreeze that denies CREATE → request denied.
func TestValidator_ChangeFreezeActive_DenyCreate(t *testing.T) {
	g := NewWithT(t)
	cf := activeChangeFreeze("cf-create", "prod", []freezev1alpha1.Action{freezev1alpha1.ActionCreate})
	v := buildValidator(t, prodNamespace(), cf)
	dep := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v1", 1)

	resp := v.Handle(context.Background(), makeCreateRequest(t, dep, "user@example.com", []string{"system:authenticated"}))
	g.Expect(resp.Allowed).To(BeFalse(), "expected deny during active freeze")
	g.Expect(resp.Result.Message).To(ContainSubstring("cf-create"))
}

// 3. Active ChangeFreeze denies DELETE → delete request denied.
func TestValidator_ChangeFreezeActive_DenyDelete(t *testing.T) {
	g := NewWithT(t)
	cf := activeChangeFreeze("cf-delete", "prod", []freezev1alpha1.Action{freezev1alpha1.ActionDelete})
	v := buildValidator(t, prodNamespace(), cf)
	dep := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v1", 1)

	resp := v.Handle(context.Background(), makeDeleteRequest(t, dep, "user@example.com", []string{"system:authenticated"}))
	g.Expect(resp.Allowed).To(BeFalse())
	g.Expect(resp.Result.Message).To(ContainSubstring("cf-delete"))
}

// 4. Active ChangeFreeze denies ROLL_OUT → update that changes image is denied.
func TestValidator_ChangeFreezeActive_DenyRollout_UpdateImageDenied(t *testing.T) {
	g := NewWithT(t)
	cf := activeChangeFreeze("cf-rollout", "prod", []freezev1alpha1.Action{freezev1alpha1.ActionRollout})
	v := buildValidator(t, prodNamespace(), cf)

	old := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v1", 2)
	newDep := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v2", 2) // image changed → ROLL_OUT

	resp := v.Handle(context.Background(), makeUpdateRequest(t, old, newDep, "user@example.com", []string{"system:authenticated"}))
	g.Expect(resp.Allowed).To(BeFalse(), "rollout must be denied during freeze")
	g.Expect(resp.Result.Message).To(ContainSubstring("cf-rollout"))
}

// 5. Active ChangeFreeze denies only ROLL_OUT → scale-only update (replicas change) is allowed.
func TestValidator_ChangeFreezeActive_DenyRollout_ScaleUpdateAllowed(t *testing.T) {
	g := NewWithT(t)
	cf := activeChangeFreeze("cf-rollout-only", "prod", []freezev1alpha1.Action{freezev1alpha1.ActionRollout})
	v := buildValidator(t, prodNamespace(), cf)

	old := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v1", 1)
	newDep := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v1", 5) // only replicas changed → SCALE

	resp := v.Handle(context.Background(), makeUpdateRequest(t, old, newDep, "user@example.com", []string{"system:authenticated"}))
	g.Expect(resp.Allowed).To(BeTrue(), "scale must be allowed when freeze only denies ROLL_OUT: %s", resp.Result.Message)
}

// 6. Active ChangeFreeze denies SCALE → scale-only update is denied.
func TestValidator_ChangeFreezeActive_DenyScale_ScaleUpdateDenied(t *testing.T) {
	g := NewWithT(t)
	cf := activeChangeFreeze("cf-scale", "prod", []freezev1alpha1.Action{freezev1alpha1.ActionScale})
	v := buildValidator(t, prodNamespace(), cf)

	old := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v1", 1)
	newDep := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v1", 5)

	resp := v.Handle(context.Background(), makeUpdateRequest(t, old, newDep, "user@example.com", []string{"system:authenticated"}))
	g.Expect(resp.Allowed).To(BeFalse())
}

// 7. Active FreezeException overrides active ChangeFreeze → allowed.
func TestValidator_FreezeException_OverridesChangeFreeze_Allowed(t *testing.T) {
	g := NewWithT(t)
	cf := activeChangeFreeze("cf-exc", "prod", []freezev1alpha1.Action{freezev1alpha1.ActionRollout})
	ex := activeFreezeException("ex-hotfix", "prod", []freezev1alpha1.Action{freezev1alpha1.ActionRollout})
	v := buildValidator(t, prodNamespace(), cf, ex)

	old := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v1", 1)
	newDep := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v2", 1)

	resp := v.Handle(context.Background(), makeUpdateRequest(t, old, newDep, "user@example.com", []string{"system:authenticated"}))
	g.Expect(resp.Allowed).To(BeTrue(), "exception must override freeze: %s", resp.Result.Message)
}

// 8. Exception covers only SCALE — rollout is still denied.
func TestValidator_FreezeException_OnlyAllowsScale_RolloutStillDenied(t *testing.T) {
	g := NewWithT(t)
	cf := activeChangeFreeze("cf-mixed", "prod", []freezev1alpha1.Action{freezev1alpha1.ActionRollout, freezev1alpha1.ActionScale})
	ex := activeFreezeException("ex-scale-only", "prod", []freezev1alpha1.Action{freezev1alpha1.ActionScale}) // only allows SCALE
	v := buildValidator(t, prodNamespace(), cf, ex)

	old := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v1", 1)
	newDep := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v2", 1) // image change → ROLL_OUT

	resp := v.Handle(context.Background(), makeUpdateRequest(t, old, newDep, "user@example.com", []string{"system:authenticated"}))
	g.Expect(resp.Allowed).To(BeFalse(), "rollout must still be denied when exception only covers SCALE")
}

// 9. Operator service-account bypass → always allowed.
func TestValidator_OperatorServiceAccount_Bypassed(t *testing.T) {
	g := NewWithT(t)
	cf := activeChangeFreeze("cf-bypass", "prod", []freezev1alpha1.Action{freezev1alpha1.ActionCreate})
	v := buildValidator(t, prodNamespace(), cf)
	dep := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v1", 1)

	// Group that matches operator namespace SA group: system:serviceaccounts:<operatorNamespace>
	req := makeCreateRequest(t, dep, "system:serviceaccount:kube-system:kube-freeze-operator-controller-manager",
		[]string{"system:serviceaccounts:kube-system"})
	resp := v.Handle(context.Background(), req)
	g.Expect(resp.Allowed).To(BeTrue(), "operator SA must always be bypassed: %s", resp.Result.Message)
}

// 10. Terminating namespace → always allowed (avoid deadlock).
func TestValidator_TerminatingNamespace_Allowed(t *testing.T) {
	g := NewWithT(t)
	cf := activeChangeFreeze("cf-term", "prod", []freezev1alpha1.Action{freezev1alpha1.ActionDelete})

	now := metav1.Now()
	terminatingNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "prod",
			Labels:            map[string]string{"env": "prod"},
			DeletionTimestamp: &now,
			Finalizers:        []string{"kubernetes"}, // fake client requires finalizer for deletion timestamp
		},
	}
	v := buildValidator(t, terminatingNS, cf)
	dep := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v1", 1)

	resp := v.Handle(context.Background(), makeDeleteRequest(t, dep, "user@example.com", []string{"system:authenticated"}))
	g.Expect(resp.Allowed).To(BeTrue(), "terminating namespace must allow all operations: %s", resp.Result.Message)
}

// 11. Kind not enforced (Pod) → always allowed.
func TestValidator_KindNotEnforced_Allowed(t *testing.T) {
	g := NewWithT(t)
	v := buildValidator(t, prodNamespace())

	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		UID:       "uid-pod",
		Kind:      metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
		Resource:  metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		Operation: admissionv1.Create,
		Namespace: "prod",
		Name:      "my-pod",
		Object:    runtime.RawExtension{Raw: []byte(`{}`)},
		UserInfo:  authv1.UserInfo{Username: "user"},
	}}
	resp := v.Handle(context.Background(), req)
	g.Expect(resp.Allowed).To(BeTrue())
}

// 12. ChangeFreeze not yet started → allowed.
func TestValidator_ChangeFreezeNotYetStarted_Allowed(t *testing.T) {
	g := NewWithT(t)
	future := time.Now().UTC().Add(2 * time.Hour)
	cf := &freezev1alpha1.ChangeFreeze{
		ObjectMeta: metav1.ObjectMeta{Name: "cf-future"},
		Spec: freezev1alpha1.ChangeFreezeSpec{
			StartTime: metav1.Time{Time: future},
			EndTime:   metav1.Time{Time: future.Add(time.Hour)},
			Target: freezev1alpha1.TargetSpec{
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
				Kinds:             []freezev1alpha1.TargetKind{freezev1alpha1.TargetKindDeployment},
			},
			Rules: freezev1alpha1.PolicyRulesSpec{Deny: []freezev1alpha1.Action{freezev1alpha1.ActionCreate}},
		},
	}
	v := buildValidator(t, prodNamespace(), cf)
	dep := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v1", 1)

	resp := v.Handle(context.Background(), makeCreateRequest(t, dep, "user@example.com", []string{"system:authenticated"}))
	g.Expect(resp.Allowed).To(BeTrue(), "freeze not started yet, must be allowed: %s", resp.Result.Message)
}

// 13. ChangeFreeze already ended → allowed.
func TestValidator_ChangeFreezeAlreadyEnded_Allowed(t *testing.T) {
	g := NewWithT(t)
	past := time.Now().UTC().Add(-2 * time.Hour)
	cf := &freezev1alpha1.ChangeFreeze{
		ObjectMeta: metav1.ObjectMeta{Name: "cf-past"},
		Spec: freezev1alpha1.ChangeFreezeSpec{
			StartTime: metav1.Time{Time: past.Add(-time.Hour)},
			EndTime:   metav1.Time{Time: past},
			Target: freezev1alpha1.TargetSpec{
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
				Kinds:             []freezev1alpha1.TargetKind{freezev1alpha1.TargetKindDeployment},
			},
			Rules: freezev1alpha1.PolicyRulesSpec{Deny: []freezev1alpha1.Action{freezev1alpha1.ActionCreate}},
		},
	}
	v := buildValidator(t, prodNamespace(), cf)
	dep := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v1", 1)

	resp := v.Handle(context.Background(), makeCreateRequest(t, dep, "user@example.com", []string{"system:authenticated"}))
	g.Expect(resp.Allowed).To(BeTrue(), "freeze already ended, must be allowed: %s", resp.Result.Message)
}

// 14. Scale subresource — denied when SCALE is in deny list.
func TestValidator_ScaleSubresource_DeniedWhenScaleInDenyList(t *testing.T) {
	g := NewWithT(t)
	cf := activeChangeFreeze("cf-sub-scale", "prod", []freezev1alpha1.Action{freezev1alpha1.ActionScale})

	dep := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v1", 1)
	v := buildValidator(t, prodNamespace(), cf, dep)

	resp := v.Handle(context.Background(), makeScaleRequest(t, "prod", "my-dep", "user@example.com"))
	g.Expect(resp.Allowed).To(BeFalse(), "scale subresource must be denied when SCALE is in deny list")
}

// 15. Scale subresource — allowed when only ROLL_OUT is in deny list.
func TestValidator_ScaleSubresource_AllowedWhenOnlyRolloutDenied(t *testing.T) {
	g := NewWithT(t)
	cf := activeChangeFreeze("cf-sub-rollout", "prod", []freezev1alpha1.Action{freezev1alpha1.ActionRollout})

	dep := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v1", 1)
	v := buildValidator(t, prodNamespace(), cf, dep)

	resp := v.Handle(context.Background(), makeScaleRequest(t, "prod", "my-dep", "user@example.com"))
	g.Expect(resp.Allowed).To(BeTrue(), "scale subresource must be allowed when freeze only denies ROLL_OUT: %s", resp.Result.Message)
}

// 16. ChangeFreeze targets a different namespace selector → not matched → allowed.
func TestValidator_ChangeFreezeWrongNamespace_Allowed(t *testing.T) {
	g := NewWithT(t)

	now := time.Now().UTC()
	cf := &freezev1alpha1.ChangeFreeze{
		ObjectMeta: metav1.ObjectMeta{Name: "cf-other-ns"},
		Spec: freezev1alpha1.ChangeFreezeSpec{
			StartTime: metav1.Time{Time: now.Add(-time.Hour)},
			EndTime:   metav1.Time{Time: now.Add(time.Hour)},
			Target: freezev1alpha1.TargetSpec{
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "staging"}}, // different env
				Kinds:             []freezev1alpha1.TargetKind{freezev1alpha1.TargetKindDeployment},
			},
			Rules: freezev1alpha1.PolicyRulesSpec{Deny: []freezev1alpha1.Action{freezev1alpha1.ActionCreate}},
		},
	}
	v := buildValidator(t, prodNamespace(), cf)
	dep := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v1", 1)

	resp := v.Handle(context.Background(), makeCreateRequest(t, dep, "user@example.com", []string{"system:authenticated"}))
	g.Expect(resp.Allowed).To(BeTrue(), "freeze targets different namespace selector, must allow: %s", resp.Result.Message)
}

// 17. Deny message contains policy name, reason, and next-allowed time.
func TestValidator_DenyMessage_ContainsUsefulInfo(t *testing.T) {
	g := NewWithT(t)
	now := time.Now().UTC()
	cf := &freezev1alpha1.ChangeFreeze{
		ObjectMeta: metav1.ObjectMeta{Name: "blackout-2026"},
		Spec: freezev1alpha1.ChangeFreezeSpec{
			StartTime: metav1.Time{Time: now.Add(-time.Hour)},
			EndTime:   metav1.Time{Time: now.Add(2 * time.Hour)},
			Target: freezev1alpha1.TargetSpec{
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
				Kinds:             []freezev1alpha1.TargetKind{freezev1alpha1.TargetKindDeployment},
			},
			Rules:   freezev1alpha1.PolicyRulesSpec{Deny: []freezev1alpha1.Action{freezev1alpha1.ActionCreate}},
			Message: freezev1alpha1.MessageSpec{Reason: "year-end blackout"},
		},
	}
	v := buildValidator(t, prodNamespace(), cf)
	dep := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v1", 1)

	resp := v.Handle(context.Background(), makeCreateRequest(t, dep, "user@example.com", []string{"system:authenticated"}))
	g.Expect(resp.Allowed).To(BeFalse())
	g.Expect(resp.Result.Message).To(ContainSubstring("blackout-2026"))
	g.Expect(resp.Result.Message).To(ContainSubstring("year-end blackout"))
}

// 18. StatefulSet is also enforced — not just Deployments.
func TestValidator_StatefulSet_Denied(t *testing.T) {
	g := NewWithT(t)
	s := testScheme(t)

	// StatefulSet scheme registration is covered by appsv1.AddToScheme
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prod", Labels: map[string]string{"env": "prod"}}},
		&freezev1alpha1.ChangeFreeze{
			ObjectMeta: metav1.ObjectMeta{Name: "cf-sts"},
			Spec: freezev1alpha1.ChangeFreezeSpec{
				StartTime: metav1.Time{Time: time.Now().UTC().Add(-time.Hour)},
				EndTime:   metav1.Time{Time: time.Now().UTC().Add(time.Hour)},
				Target: freezev1alpha1.TargetSpec{
					NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
					Kinds:             []freezev1alpha1.TargetKind{freezev1alpha1.TargetKindStatefulSet},
				},
				Rules: freezev1alpha1.PolicyRulesSpec{Deny: []freezev1alpha1.Action{freezev1alpha1.ActionCreate}},
			},
		},
	).Build()

	v := &Validator{
		Client:            cl,
		Reader:            cl,
		Decoder:           admission.NewDecoder(s),
		OperatorNamespace: "kube-system",
	}

	sts := &appsv1.StatefulSet{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "StatefulSet"},
		ObjectMeta: metav1.ObjectMeta{Name: "my-sts", Namespace: "prod", Labels: map[string]string{"app": "db"}},
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptrInt32(1),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "db"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "db"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "pg:14"}}},
			},
		},
	}
	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		UID:       "uid-sts",
		Kind:      metav1.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"},
		Resource:  metav1.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"},
		Operation: admissionv1.Create,
		Namespace: "prod",
		Name:      "my-sts",
		Object:    runtime.RawExtension{Raw: mustJSON(t, sts)},
		UserInfo:  authv1.UserInfo{Username: "user", Groups: []string{"system:authenticated"}},
	}}

	resp := v.Handle(context.Background(), req)
	g.Expect(resp.Allowed).To(BeFalse(), "StatefulSet create must be denied during freeze")
}

// 19. FreezeException with user constraint — right user → allowed.
func TestValidator_FreezeException_UserConstraint_AllowedForMatchingUser(t *testing.T) {
	g := NewWithT(t)
	now := time.Now().UTC()
	cf := activeChangeFreeze("cf-user", "prod", []freezev1alpha1.Action{freezev1alpha1.ActionCreate})
	ex := &freezev1alpha1.FreezeException{
		ObjectMeta: metav1.ObjectMeta{Name: "ex-user"},
		Spec: freezev1alpha1.FreezeExceptionSpec{
			ActiveFrom: metav1.Time{Time: now.Add(-time.Minute)},
			ActiveTo:   metav1.Time{Time: now.Add(time.Minute)},
			Target: freezev1alpha1.TargetSpec{
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
				Kinds:             []freezev1alpha1.TargetKind{freezev1alpha1.TargetKindDeployment},
			},
			Allow:  []freezev1alpha1.Action{freezev1alpha1.ActionCreate},
			Reason: "approved",
			Constraints: &freezev1alpha1.FreezeExceptionConstraintsSpec{
				AllowedUsers: []string{"oncall@example.com"},
			},
		},
	}
	v := buildValidator(t, prodNamespace(), cf, ex)
	dep := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v1", 1)

	resp := v.Handle(context.Background(), makeCreateRequest(t, dep, "oncall@example.com", []string{"system:authenticated"}))
	g.Expect(resp.Allowed).To(BeTrue(), "matching user must be allowed by exception: %s", resp.Result.Message)
}

// 20. FreezeException with user constraint — wrong user → denied.
func TestValidator_FreezeException_UserConstraint_DeniedForOtherUser(t *testing.T) {
	g := NewWithT(t)
	now := time.Now().UTC()
	cf := activeChangeFreeze("cf-user2", "prod", []freezev1alpha1.Action{freezev1alpha1.ActionCreate})
	ex := &freezev1alpha1.FreezeException{
		ObjectMeta: metav1.ObjectMeta{Name: "ex-user2"},
		Spec: freezev1alpha1.FreezeExceptionSpec{
			ActiveFrom: metav1.Time{Time: now.Add(-time.Minute)},
			ActiveTo:   metav1.Time{Time: now.Add(time.Minute)},
			Target: freezev1alpha1.TargetSpec{
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
				Kinds:             []freezev1alpha1.TargetKind{freezev1alpha1.TargetKindDeployment},
			},
			Allow:  []freezev1alpha1.Action{freezev1alpha1.ActionCreate},
			Reason: "approved",
			Constraints: &freezev1alpha1.FreezeExceptionConstraintsSpec{
				AllowedUsers: []string{"oncall@example.com"},
			},
		},
	}
	v := buildValidator(t, prodNamespace(), cf, ex)
	dep := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v1", 1)

	resp := v.Handle(context.Background(), makeCreateRequest(t, dep, "random-dev@example.com", []string{"system:authenticated"}))
	g.Expect(resp.Allowed).To(BeFalse(), "user not in exception.allowedUsers must be denied")
}

// 21. FreezeException with requireLabels — matching labels → allowed.
func TestValidator_FreezeException_RequireLabels_Matched_Allowed(t *testing.T) {
	g := NewWithT(t)
	now := time.Now().UTC()
	cf := activeChangeFreeze("cf-labels", "prod", []freezev1alpha1.Action{freezev1alpha1.ActionCreate})
	ex := &freezev1alpha1.FreezeException{
		ObjectMeta: metav1.ObjectMeta{Name: "ex-labels"},
		Spec: freezev1alpha1.FreezeExceptionSpec{
			ActiveFrom: metav1.Time{Time: now.Add(-time.Minute)},
			ActiveTo:   metav1.Time{Time: now.Add(time.Minute)},
			Target: freezev1alpha1.TargetSpec{
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
				Kinds:             []freezev1alpha1.TargetKind{freezev1alpha1.TargetKindDeployment},
			},
			Allow:  []freezev1alpha1.Action{freezev1alpha1.ActionCreate},
			Reason: "approved",
			Constraints: &freezev1alpha1.FreezeExceptionConstraintsSpec{
				RequireLabels: map[string]string{"hotfix": "true"},
			},
		},
	}
	v := buildValidator(t, prodNamespace(), cf, ex)
	dep := makeDeployment("prod", "my-dep", map[string]string{"app": "x", "hotfix": "true"}, "img:v1", 1)

	resp := v.Handle(context.Background(), makeCreateRequest(t, dep, "user@example.com", []string{"system:authenticated"}))
	g.Expect(resp.Allowed).To(BeTrue(), "label matches, exception must allow: %s", resp.Result.Message)
}

// 22. FreezeException with requireLabels — missing label → denied.
func TestValidator_FreezeException_RequireLabels_NotMatched_Denied(t *testing.T) {
	g := NewWithT(t)
	now := time.Now().UTC()
	cf := activeChangeFreeze("cf-labels2", "prod", []freezev1alpha1.Action{freezev1alpha1.ActionCreate})
	ex := &freezev1alpha1.FreezeException{
		ObjectMeta: metav1.ObjectMeta{Name: "ex-labels2"},
		Spec: freezev1alpha1.FreezeExceptionSpec{
			ActiveFrom: metav1.Time{Time: now.Add(-time.Minute)},
			ActiveTo:   metav1.Time{Time: now.Add(time.Minute)},
			Target: freezev1alpha1.TargetSpec{
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
				Kinds:             []freezev1alpha1.TargetKind{freezev1alpha1.TargetKindDeployment},
			},
			Allow:  []freezev1alpha1.Action{freezev1alpha1.ActionCreate},
			Reason: "approved",
			Constraints: &freezev1alpha1.FreezeExceptionConstraintsSpec{
				RequireLabels: map[string]string{"hotfix": "true"},
			},
		},
	}
	v := buildValidator(t, prodNamespace(), cf, ex)
	dep := makeDeployment("prod", "my-dep", map[string]string{"app": "x"}, "img:v1", 1) // no hotfix label

	resp := v.Handle(context.Background(), makeCreateRequest(t, dep, "user@example.com", []string{"system:authenticated"}))
	g.Expect(resp.Allowed).To(BeFalse(), "missing required label, exception must not apply")
}

// 23. fake.NewClientBuilder needs the GVK to list resources — register schema for the test.
// This test explicitly verifies the GVK registration via fake client schema.
func TestValidator_SchemaRegistered_for(t *testing.T) {
	s := testScheme(t)
	g := NewWithT(t)
	// Verify that key resources are in the scheme
	gvks, _, err := s.ObjectKinds(&appsv1.Deployment{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(gvks).To(ContainElement(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}))
	gvks2, _, err := s.ObjectKinds(&freezev1alpha1.ChangeFreeze{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(gvks2).To(ContainElement(schema.GroupVersionKind{
		Group: "freeze-operator.io", Version: "v1alpha1", Kind: "ChangeFreeze",
	}))
}
