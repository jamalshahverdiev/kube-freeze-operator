package workloads

// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=freeze-operator.io,resources=maintenancewindows,verbs=get;list;watch
// +kubebuilder:rbac:groups=freeze-operator.io,resources=changefreezes,verbs=get;list;watch
// +kubebuilder:rbac:groups=freeze-operator.io,resources=freezeexceptions,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	freezev1alpha1 "github.com/jamalshahverdiev/kube-freeze-operator/api/v1alpha1"
	"github.com/jamalshahverdiev/kube-freeze-operator/internal/diff"
	"github.com/jamalshahverdiev/kube-freeze-operator/internal/policy"
)

const (
	WebhookPath = "/validate-freeze-operator-io-v1alpha1-workloads"
	appsGroup   = "apps"
)

type Validator struct {
	Client  client.Client
	Reader  client.Reader
	Decoder admission.Decoder

	OperatorNamespace string
}

func (v *Validator) Handle(ctx context.Context, req admission.Request) admission.Response {
	log := ctrl.Log.WithName("webhook").WithName("workloads")

	if v.Client == nil {
		return admission.Errored(500, fmt.Errorf("client is nil"))
	}
	reader := v.Reader
	if reader == nil {
		// Fallback to the cached client if no APIReader was injected.
		reader = v.Client
	}

	opNs := v.OperatorNamespace
	if opNs == "" {
		opNs = os.Getenv("POD_NAMESPACE")
	}

	// Allow the operator itself to bypass enforcement to avoid deadlocks.
	if opNs != "" && slices.Contains(req.UserInfo.Groups, "system:serviceaccounts:"+opNs) {
		return admission.Allowed("operator serviceaccount bypass")
	}

	var (
		kind      freezev1alpha1.TargetKind
		action    freezev1alpha1.Action
		objLabels map[string]string
	)

	// NOTE: `kubectl scale` typically hits the /scale subresource (e.g. deployments/scale),
	// which would bypass enforcement if we only match on Kind=Deployment and Resource=deployments.
	if req.SubResource == "scale" && req.Resource.Group == appsGroup {
		if req.Operation != admissionv1.Update {
			return admission.Allowed("scale subresource non-update")
		}
		switch req.Resource.Resource {
		case "deployments":
			kind = freezev1alpha1.TargetKindDeployment
		case "statefulsets":
			kind = freezev1alpha1.TargetKindStatefulSet
		default:
			return admission.Allowed("scale subresource not enforced")
		}
		action = freezev1alpha1.ActionScale

		// Scale subresource request objects are typically autoscaling/v1 Scale and do not carry
		// the workload labels we need for objectSelector/constraints; fetch the workload.
		ns := req.Namespace
		name := req.Name
		if ns == "" || name == "" {
			return admission.Allowed("scale request missing namespace or name")
		}
		switch kind {
		case freezev1alpha1.TargetKindDeployment:
			dep := &appsv1.Deployment{}
			if err := reader.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, dep); err != nil {
				return admission.Errored(500, fmt.Errorf("get deployment %s/%s: %w", ns, name, err))
			}
			objLabels = dep.Labels
		case freezev1alpha1.TargetKindStatefulSet:
			sts := &appsv1.StatefulSet{}
			if err := reader.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, sts); err != nil {
				return admission.Errored(500, fmt.Errorf("get statefulset %s/%s: %w", ns, name, err))
			}
			objLabels = sts.Labels
		}
	} else {
		k, ok := mapGVKToTargetKind(req.Kind.Group, req.Kind.Kind)
		if !ok {
			return admission.Allowed("kind not enforced")
		}
		kind = k

		a, labels, err := v.classify(req, kind)
		if err != nil {
			log.Error(err, "classify request")
			return admission.Errored(400, err)
		}
		action = a
		objLabels = labels
	}

	ns := req.Namespace
	if ns == "" {
		// Workloads are namespaced; if we ever see cluster-scoped here, allow.
		return admission.Allowed("cluster-scoped request")
	}

	nsObj := &corev1.Namespace{}
	if err := v.Client.Get(ctx, types.NamespacedName{Name: ns}, nsObj); err != nil {
		return admission.Errored(500, fmt.Errorf("get namespace %q: %w", ns, err))
	}

	// Never block operations inside a terminating namespace.
	// The namespace is already being deleted; blocking controller cleanup operations
	// (DELETE, finalizer patches, status updates) can cause permanent deadlocks.
	if nsObj.DeletionTimestamp != nil {
		return admission.Allowed("namespace is terminating: bypass freeze policies")
	}

	ev := &policy.Evaluator{Client: v.Client}
	dec, err := ev.Evaluate(ctx, policy.Input{
		Now:           time.Now().UTC(),
		Namespace:     ns,
		NamespaceTags: nsObj.Labels,
		Kind:          kind,
		Action:        action,
		ObjectLabels:  objLabels,
		Username:      req.UserInfo.Username,
		Groups:        req.UserInfo.Groups,
	})
	if err != nil {
		return admission.Errored(500, err)
	}

	if dec.Allowed {
		return admission.Allowed("allowed by policy")
	}

	msg := formatDenyMessage(dec)
	log.Info("denied", "namespace", ns, "kind", kind, "action", action, "user", req.UserInfo.Username, "policy", dec.MatchedPolicy, "reason", dec.Reason)
	return admission.Denied(msg)
}

func (v *Validator) classify(req admission.Request, kind freezev1alpha1.TargetKind) (freezev1alpha1.Action, map[string]string, error) {
	switch req.Operation {
	case admissionv1.Create:
		labels, err := v.decodeLabels(req.Object, kind)
		return freezev1alpha1.ActionCreate, labels, err
	case admissionv1.Delete:
		labels, err := v.decodeLabels(req.OldObject, kind)
		return freezev1alpha1.ActionDelete, labels, err
	case admissionv1.Update:
		oldObj, newObj, labels, err := v.decodeOldNew(req, kind)
		if err != nil {
			return "", nil, err
		}
		a, err := diff.ClassifyUpdate(kind, oldObj, newObj)
		return a, labels, err
	default:
		return "", nil, fmt.Errorf("unsupported operation: %s", req.Operation)
	}
}

func (v *Validator) decodeLabels(raw runtime.RawExtension, kind freezev1alpha1.TargetKind) (map[string]string, error) {
	obj, err := v.decode(raw, kind)
	if err != nil {
		return nil, err
	}
	accessor, err := metaAccessor(obj)
	if err != nil {
		return nil, err
	}
	return accessor.GetLabels(), nil
}

func (v *Validator) decodeOldNew(req admission.Request, kind freezev1alpha1.TargetKind) (runtime.Object, runtime.Object, map[string]string, error) {
	oldObj, err := v.decode(req.OldObject, kind)
	if err != nil {
		return nil, nil, nil, err
	}
	newObj, err := v.decode(req.Object, kind)
	if err != nil {
		return nil, nil, nil, err
	}
	accessor, err := metaAccessor(newObj)
	if err != nil {
		return nil, nil, nil, err
	}
	return oldObj, newObj, accessor.GetLabels(), nil
}

func (v *Validator) decode(raw runtime.RawExtension, kind freezev1alpha1.TargetKind) (runtime.Object, error) {
	if v.Decoder == nil {
		return nil, fmt.Errorf("decoder is nil")
	}

	switch kind {
	case freezev1alpha1.TargetKindDeployment:
		obj := &appsv1.Deployment{}
		if err := v.Decoder.DecodeRaw(raw, obj); err != nil {
			return nil, err
		}
		return obj, nil
	case freezev1alpha1.TargetKindStatefulSet:
		obj := &appsv1.StatefulSet{}
		if err := v.Decoder.DecodeRaw(raw, obj); err != nil {
			return nil, err
		}
		return obj, nil
	case freezev1alpha1.TargetKindDaemonSet:
		obj := &appsv1.DaemonSet{}
		if err := v.Decoder.DecodeRaw(raw, obj); err != nil {
			return nil, err
		}
		return obj, nil
	case freezev1alpha1.TargetKindCronJob:
		obj := &batchv1.CronJob{}
		if err := v.Decoder.DecodeRaw(raw, obj); err != nil {
			return nil, err
		}
		return obj, nil
	default:
		return nil, fmt.Errorf("unsupported kind: %s", kind)
	}
}

func mapGVKToTargetKind(group string, kind string) (freezev1alpha1.TargetKind, bool) {
	switch {
	case group == appsGroup && kind == "Deployment":
		return freezev1alpha1.TargetKindDeployment, true
	case group == appsGroup && kind == "StatefulSet":
		return freezev1alpha1.TargetKindStatefulSet, true
	case group == appsGroup && kind == "DaemonSet":
		return freezev1alpha1.TargetKindDaemonSet, true
	case group == "batch" && kind == "CronJob":
		return freezev1alpha1.TargetKindCronJob, true
	default:
		return "", false
	}
}

func formatDenyMessage(dec policy.Decision) string {
	parts := []string{}
	if dec.MatchedPolicy != nil {
		parts = append(parts, fmt.Sprintf("Denied by %s/%s", dec.MatchedPolicy.Kind, dec.MatchedPolicy.Name))
	}
	if dec.Reason != "" {
		parts = append(parts, dec.Reason)
	}
	if dec.NextAllowedTime != nil {
		parts = append(parts, fmt.Sprintf("Next allowed at %s", dec.NextAllowedTime.UTC().Format(time.RFC3339)))
	} else if dec.FreezeEndTime != nil {
		parts = append(parts, fmt.Sprintf("Allowed after %s", dec.FreezeEndTime.UTC().Format(time.RFC3339)))
	}
	return strings.Join(parts, ": ")
}

func metaAccessor(obj runtime.Object) (metav1Object, error) {
	if o, ok := obj.(metav1Object); ok {
		return o, nil
	}
	return nil, fmt.Errorf("object does not implement metav1.Object")
}

type metav1Object interface {
	GetLabels() map[string]string
}
