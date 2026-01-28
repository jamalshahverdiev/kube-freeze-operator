package policy

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	freezev1alpha1 "github.com/jamalshahverdiev/kube-freeze-operator/api/v1alpha1"
)

func TestEvaluator_ChangeFreezeDenyAndExceptionAllow(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(freezev1alpha1.AddToScheme(scheme)).To(Succeed())

	now := time.Date(2026, 1, 28, 12, 0, 0, 0, time.UTC)

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prod", Labels: map[string]string{"env": "prod"}}}

	cf := &freezev1alpha1.ChangeFreeze{
		ObjectMeta: metav1.ObjectMeta{Name: "cf"},
		Spec: freezev1alpha1.ChangeFreezeSpec{
			StartTime: metav1.Time{Time: now.Add(-time.Hour)},
			EndTime:   metav1.Time{Time: now.Add(time.Hour)},
			Target: freezev1alpha1.TargetSpec{
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
				Kinds:             []freezev1alpha1.TargetKind{freezev1alpha1.TargetKindDeployment},
			},
			Rules: freezev1alpha1.PolicyRulesSpec{Deny: []freezev1alpha1.Action{freezev1alpha1.ActionRollout}},
		},
	}

	ex := &freezev1alpha1.FreezeException{
		ObjectMeta: metav1.ObjectMeta{Name: "ex"},
		Spec: freezev1alpha1.FreezeExceptionSpec{
			ActiveFrom: metav1.Time{Time: now.Add(-time.Minute)},
			ActiveTo:   metav1.Time{Time: now.Add(time.Minute)},
			Target: freezev1alpha1.TargetSpec{
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
				Kinds:             []freezev1alpha1.TargetKind{freezev1alpha1.TargetKindDeployment},
			},
			Allow:  []freezev1alpha1.Action{freezev1alpha1.ActionRollout},
			Reason: "hotfix",
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, cf, ex).Build()
	ev := &Evaluator{Client: cl}

	dec, err := ev.Evaluate(ctx, Input{
		Now:          now,
		Namespace:    "prod",
		Kind:         freezev1alpha1.TargetKindDeployment,
		Action:       freezev1alpha1.ActionRollout,
		ObjectLabels: map[string]string{"app": "x"},
	})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dec.Allowed).To(BeTrue())
	g.Expect(dec.MatchedPolicy).ToNot(BeNil())
	g.Expect(dec.MatchedPolicy.Kind).To(Equal(PolicyKindChangeFreeze))
	g.Expect(dec.MatchedOverride).ToNot(BeNil())
	g.Expect(dec.MatchedOverride.Kind).To(Equal(PolicyKindFreezeException))
}

func TestEvaluator_ChangeFreezeDeny(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(freezev1alpha1.AddToScheme(scheme)).To(Succeed())

	now := time.Date(2026, 1, 28, 12, 0, 0, 0, time.UTC)

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prod", Labels: map[string]string{"env": "prod"}}}

	cf := &freezev1alpha1.ChangeFreeze{
		ObjectMeta: metav1.ObjectMeta{Name: "cf"},
		Spec: freezev1alpha1.ChangeFreezeSpec{
			StartTime: metav1.Time{Time: now.Add(-time.Hour)},
			EndTime:   metav1.Time{Time: now.Add(time.Hour)},
			Target: freezev1alpha1.TargetSpec{
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
				Kinds:             []freezev1alpha1.TargetKind{freezev1alpha1.TargetKindDeployment},
			},
			Rules:   freezev1alpha1.PolicyRulesSpec{Deny: []freezev1alpha1.Action{freezev1alpha1.ActionRollout}},
			Message: freezev1alpha1.MessageSpec{Reason: "freeze"},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, cf).Build()
	ev := &Evaluator{Client: cl}

	dec, err := ev.Evaluate(ctx, Input{
		Now:          now,
		Namespace:    "prod",
		Kind:         freezev1alpha1.TargetKindDeployment,
		Action:       freezev1alpha1.ActionRollout,
		ObjectLabels: map[string]string{"app": "x"},
	})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dec.Allowed).To(BeFalse())
	g.Expect(dec.MatchedPolicy).ToNot(BeNil())
	g.Expect(dec.MatchedPolicy.Kind).To(Equal(PolicyKindChangeFreeze))
	g.Expect(dec.Reason).To(Equal("freeze"))
	g.Expect(dec.NextAllowedTime).ToNot(BeNil())
}
