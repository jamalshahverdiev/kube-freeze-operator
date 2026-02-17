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

package v1alpha1

import (
	"context"
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	freezeoperatorv1alpha1 "github.com/jamalshahverdiev/kube-freeze-operator/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var changefreezeLog = logf.Log.WithName("changefreeze-resource")

// SetupChangeFreezeWebhookWithManager registers the webhook for ChangeFreeze in the manager.
func SetupChangeFreezeWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &freezeoperatorv1alpha1.ChangeFreeze{}).
		WithValidator(&ChangeFreezeCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-freeze-operator-io-v1alpha1-changefreeze,mutating=false,failurePolicy=fail,sideEffects=None,groups=freeze-operator.io,resources=changefreezes,verbs=create;update,versions=v1alpha1,name=vchangefreeze-v1alpha1.kb.io,admissionReviewVersions=v1

// ChangeFreezeCustomValidator struct is responsible for validating the ChangeFreeze resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ChangeFreezeCustomValidator struct{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ChangeFreeze.
func (v *ChangeFreezeCustomValidator) ValidateCreate(_ context.Context, obj *freezeoperatorv1alpha1.ChangeFreeze) (admission.Warnings, error) {
	changefreezeLog.Info("Validation for ChangeFreeze upon creation", "name", obj.GetName())

	if err := v.validateChangeFreeze(obj); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ChangeFreeze.
func (v *ChangeFreezeCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *freezeoperatorv1alpha1.ChangeFreeze) (admission.Warnings, error) {
	changefreezeLog.Info("Validation for ChangeFreeze upon update", "name", newObj.GetName())

	if err := v.validateChangeFreeze(newObj); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ChangeFreeze.
func (v *ChangeFreezeCustomValidator) ValidateDelete(_ context.Context, obj *freezeoperatorv1alpha1.ChangeFreeze) (admission.Warnings, error) {
	changefreezeLog.Info("Validation for ChangeFreeze upon deletion", "name", obj.GetName())

	// No validation needed for deletion
	return nil, nil
}

func (v *ChangeFreezeCustomValidator) validateChangeFreeze(obj *freezeoperatorv1alpha1.ChangeFreeze) error {
	// Validate timezone if specified
	if obj.Spec.Timezone != nil && *obj.Spec.Timezone != "" {
		if _, err := time.LoadLocation(*obj.Spec.Timezone); err != nil {
			return fmt.Errorf("spec.timezone: invalid timezone %q: %w", *obj.Spec.Timezone, err)
		}
	}

	// startTime < endTime validation is already handled by CEL validation in the CRD
	// But we double-check here for safety
	if !obj.Spec.StartTime.Time.Before(obj.Spec.EndTime.Time) {
		return fmt.Errorf("spec.endTime must be after spec.startTime")
	}

	return nil
}
