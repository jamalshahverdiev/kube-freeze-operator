package diff

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"

	freezev1alpha1 "github.com/jamalshahverdiev/kube-freeze-operator/api/v1alpha1"
)

func ClassifyUpdate(kind freezev1alpha1.TargetKind, oldObj runtime.Object, newObj runtime.Object) (freezev1alpha1.Action, error) {
	switch kind {
	case freezev1alpha1.TargetKindDeployment:
		oldD, ok1 := oldObj.(*appsv1.Deployment)
		newD, ok2 := newObj.(*appsv1.Deployment)
		if !ok1 || !ok2 {
			return "", fmt.Errorf("expected *appsv1.Deployment")
		}
		templateChanged := !equality.Semantic.DeepEqual(oldD.Spec.Template, newD.Spec.Template)
		replicasChanged := !equality.Semantic.DeepEqual(oldD.Spec.Replicas, newD.Spec.Replicas)
		if templateChanged {
			return freezev1alpha1.ActionRollout, nil
		}
		if replicasChanged {
			return freezev1alpha1.ActionScale, nil
		}
		return freezev1alpha1.ActionRollout, nil

	case freezev1alpha1.TargetKindStatefulSet:
		oldS, ok1 := oldObj.(*appsv1.StatefulSet)
		newS, ok2 := newObj.(*appsv1.StatefulSet)
		if !ok1 || !ok2 {
			return "", fmt.Errorf("expected *appsv1.StatefulSet")
		}
		templateChanged := !equality.Semantic.DeepEqual(oldS.Spec.Template, newS.Spec.Template)
		replicasChanged := !equality.Semantic.DeepEqual(oldS.Spec.Replicas, newS.Spec.Replicas)
		if templateChanged {
			return freezev1alpha1.ActionRollout, nil
		}
		if replicasChanged {
			return freezev1alpha1.ActionScale, nil
		}
		return freezev1alpha1.ActionRollout, nil

	case freezev1alpha1.TargetKindDaemonSet:
		oldD, ok1 := oldObj.(*appsv1.DaemonSet)
		newD, ok2 := newObj.(*appsv1.DaemonSet)
		if !ok1 || !ok2 {
			return "", fmt.Errorf("expected *appsv1.DaemonSet")
		}
		templateChanged := !equality.Semantic.DeepEqual(oldD.Spec.Template, newD.Spec.Template)
		if templateChanged {
			return freezev1alpha1.ActionRollout, nil
		}
		return freezev1alpha1.ActionRollout, nil

	case freezev1alpha1.TargetKindCronJob:
		oldC, ok1 := oldObj.(*batchv1.CronJob)
		newC, ok2 := newObj.(*batchv1.CronJob)
		if !ok1 || !ok2 {
			return "", fmt.Errorf("expected *batchv1.CronJob")
		}

		// v0.1: any meaningful spec change is treated as ROLL_OUT.
		if !equality.Semantic.DeepEqual(oldC.Spec, newC.Spec) {
			return freezev1alpha1.ActionRollout, nil
		}
		return freezev1alpha1.ActionRollout, nil

	default:
		return "", fmt.Errorf("unsupported kind: %s", kind)
	}
}
