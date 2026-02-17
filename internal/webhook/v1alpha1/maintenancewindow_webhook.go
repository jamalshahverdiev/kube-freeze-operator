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

	"github.com/robfig/cron/v3"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	freezeoperatorv1alpha1 "github.com/jamalshahverdiev/kube-freeze-operator/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var maintenancewindowlog = logf.Log.WithName("maintenancewindow-resource")

// SetupMaintenanceWindowWebhookWithManager registers the webhook for MaintenanceWindow in the manager.
func SetupMaintenanceWindowWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &freezeoperatorv1alpha1.MaintenanceWindow{}).
		WithValidator(&MaintenanceWindowCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-freeze-operator-io-v1alpha1-maintenancewindow,mutating=false,failurePolicy=fail,sideEffects=None,groups=freeze-operator.io,resources=maintenancewindows,verbs=create;update,versions=v1alpha1,name=vmaintenancewindow-v1alpha1.kb.io,admissionReviewVersions=v1

// MaintenanceWindowCustomValidator struct is responsible for validating the MaintenanceWindow resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type MaintenanceWindowCustomValidator struct{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type MaintenanceWindow.
func (v *MaintenanceWindowCustomValidator) ValidateCreate(_ context.Context, obj *freezeoperatorv1alpha1.MaintenanceWindow) (admission.Warnings, error) {
	maintenancewindowlog.Info("Validation for MaintenanceWindow upon creation", "name", obj.GetName())

	if err := v.validateMaintenanceWindow(obj); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type MaintenanceWindow.
func (v *MaintenanceWindowCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *freezeoperatorv1alpha1.MaintenanceWindow) (admission.Warnings, error) {
	maintenancewindowlog.Info("Validation for MaintenanceWindow upon update", "name", newObj.GetName())

	if err := v.validateMaintenanceWindow(newObj); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type MaintenanceWindow.
func (v *MaintenanceWindowCustomValidator) ValidateDelete(_ context.Context, obj *freezeoperatorv1alpha1.MaintenanceWindow) (admission.Warnings, error) {
	maintenancewindowlog.Info("Validation for MaintenanceWindow upon deletion", "name", obj.GetName())

	// No validation needed for deletion
	return nil, nil
}

func (v *MaintenanceWindowCustomValidator) validateMaintenanceWindow(obj *freezeoperatorv1alpha1.MaintenanceWindow) error {
	// Validate timezone
	if obj.Spec.Timezone != "" {
		if _, err := time.LoadLocation(obj.Spec.Timezone); err != nil {
			return fmt.Errorf("spec.timezone: invalid timezone %q: %w", obj.Spec.Timezone, err)
		}
	}

	// Validate windows
	if len(obj.Spec.Windows) == 0 {
		return fmt.Errorf("spec.windows: must have at least one window")
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	for i, w := range obj.Spec.Windows {
		// Validate cron schedule
		if _, err := parser.Parse(w.Schedule); err != nil {
			return fmt.Errorf("spec.windows[%d].schedule: invalid cron expression %q: %w", i, w.Schedule, err)
		}

		// Validate duration
		if w.Duration.Duration <= 0 {
			return fmt.Errorf("spec.windows[%d].duration: must be greater than 0", i)
		}
	}

	return nil
}
