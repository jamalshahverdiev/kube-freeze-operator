package diff

import (
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	freezev1alpha1 "github.com/jamalshahverdiev/kube-freeze-operator/api/v1alpha1"
)

func ptr[T any](v T) *T { return &v }

const imgV2 = "img:v2"

// ---------------------------------------------------------------------------
// Deployment
// ---------------------------------------------------------------------------

func TestClassifyUpdate_Deployment_TemplateChange_IsRollout(t *testing.T) {
	g := NewWithT(t)

	base := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "d", Namespace: "ns"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr(int32(2)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "x"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img:v1"}}},
			},
		},
	}
	updated := base.DeepCopy()
	updated.Spec.Template.Spec.Containers[0].Image = imgV2

	action, err := ClassifyUpdate(freezev1alpha1.TargetKindDeployment, base, updated)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(action).To(Equal(freezev1alpha1.ActionRollout))
}

func TestClassifyUpdate_Deployment_ReplicasChange_IsScale(t *testing.T) {
	g := NewWithT(t)

	base := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "d", Namespace: "ns"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr(int32(1)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "x"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img:v1"}}},
			},
		},
	}
	updated := base.DeepCopy()
	updated.Spec.Replicas = ptr(int32(5))

	action, err := ClassifyUpdate(freezev1alpha1.TargetKindDeployment, base, updated)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(action).To(Equal(freezev1alpha1.ActionScale))
}

func TestClassifyUpdate_Deployment_BothTemplateAndReplicas_IsRollout(t *testing.T) {
	g := NewWithT(t)

	base := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "d", Namespace: "ns"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr(int32(1)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "x"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img:v1"}}},
			},
		},
	}
	updated := base.DeepCopy()
	updated.Spec.Replicas = ptr(int32(3))
	updated.Spec.Template.Spec.Containers[0].Image = imgV2

	// Template change wins → ROLL_OUT
	action, err := ClassifyUpdate(freezev1alpha1.TargetKindDeployment, base, updated)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(action).To(Equal(freezev1alpha1.ActionRollout))
}

func TestClassifyUpdate_Deployment_NoMeaningfulChange_IsRollout(t *testing.T) {
	g := NewWithT(t)

	base := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "d", Namespace: "ns"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr(int32(1)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "x"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img:v1"}}},
			},
		},
	}
	updated := base.DeepCopy()

	// Nothing changed in template or replicas → still ROLL_OUT (safe default)
	action, err := ClassifyUpdate(freezev1alpha1.TargetKindDeployment, base, updated)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(action).To(Equal(freezev1alpha1.ActionRollout))
}

func TestClassifyUpdate_Deployment_WrongType_ReturnsError(t *testing.T) {
	g := NewWithT(t)

	dep := &appsv1.Deployment{}
	sts := &appsv1.StatefulSet{}

	_, err := ClassifyUpdate(freezev1alpha1.TargetKindDeployment, dep, sts)
	g.Expect(err).To(HaveOccurred())
}

// ---------------------------------------------------------------------------
// StatefulSet
// ---------------------------------------------------------------------------

func TestClassifyUpdate_StatefulSet_TemplateChange_IsRollout(t *testing.T) {
	g := NewWithT(t)

	base := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "s"},
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptr(int32(1)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "x"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img:v1"}}},
			},
		},
	}
	updated := base.DeepCopy()
	updated.Spec.Template.Spec.Containers[0].Image = imgV2

	action, err := ClassifyUpdate(freezev1alpha1.TargetKindStatefulSet, base, updated)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(action).To(Equal(freezev1alpha1.ActionRollout))
}

func TestClassifyUpdate_StatefulSet_ReplicasChange_IsScale(t *testing.T) {
	g := NewWithT(t)

	base := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "s"},
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptr(int32(1)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "x"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img:v1"}}},
			},
		},
	}
	updated := base.DeepCopy()
	updated.Spec.Replicas = ptr(int32(3))

	action, err := ClassifyUpdate(freezev1alpha1.TargetKindStatefulSet, base, updated)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(action).To(Equal(freezev1alpha1.ActionScale))
}

// ---------------------------------------------------------------------------
// DaemonSet
// ---------------------------------------------------------------------------

func TestClassifyUpdate_DaemonSet_TemplateChange_IsRollout(t *testing.T) {
	g := NewWithT(t)

	base := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ds"},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "x"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img:v1"}}},
			},
		},
	}
	updated := base.DeepCopy()
	updated.Spec.Template.Spec.Containers[0].Image = imgV2

	action, err := ClassifyUpdate(freezev1alpha1.TargetKindDaemonSet, base, updated)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(action).To(Equal(freezev1alpha1.ActionRollout))
}

func TestClassifyUpdate_DaemonSet_NoTemplateChange_IsRollout(t *testing.T) {
	g := NewWithT(t)

	base := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ds"},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "x"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img:v1"}}},
			},
		},
	}
	updated := base.DeepCopy()

	action, err := ClassifyUpdate(freezev1alpha1.TargetKindDaemonSet, base, updated)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(action).To(Equal(freezev1alpha1.ActionRollout))
}

// ---------------------------------------------------------------------------
// CronJob
// ---------------------------------------------------------------------------

func TestClassifyUpdate_CronJob_ScheduleChange_IsRollout(t *testing.T) {
	g := NewWithT(t)

	base := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "cj"},
		Spec:       batchv1.CronJobSpec{Schedule: "0 * * * *"},
	}
	updated := base.DeepCopy()
	updated.Spec.Schedule = "30 * * * *"

	action, err := ClassifyUpdate(freezev1alpha1.TargetKindCronJob, base, updated)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(action).To(Equal(freezev1alpha1.ActionRollout))
}

func TestClassifyUpdate_CronJob_NoSpecChange_IsRollout(t *testing.T) {
	g := NewWithT(t)

	base := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "cj"},
		Spec:       batchv1.CronJobSpec{Schedule: "0 * * * *"},
	}
	updated := base.DeepCopy()

	action, err := ClassifyUpdate(freezev1alpha1.TargetKindCronJob, base, updated)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(action).To(Equal(freezev1alpha1.ActionRollout))
}

// ---------------------------------------------------------------------------
// Unsupported kind
// ---------------------------------------------------------------------------

func TestClassifyUpdate_UnsupportedKind_ReturnsError(t *testing.T) {
	g := NewWithT(t)

	dep := &appsv1.Deployment{}
	dep2 := dep.DeepCopy()

	_, err := ClassifyUpdate("Pod", dep, dep2)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("unsupported kind"))
}
