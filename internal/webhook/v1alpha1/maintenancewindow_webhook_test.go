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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	freezeoperatorv1alpha1 "github.com/jamalshahverdiev/kube-freeze-operator/api/v1alpha1"
)

var _ = Describe("MaintenanceWindow Webhook", func() {
	var (
		obj       *freezeoperatorv1alpha1.MaintenanceWindow
		oldObj    *freezeoperatorv1alpha1.MaintenanceWindow
		validator MaintenanceWindowCustomValidator
	)

	BeforeEach(func() {
		oldObj = &freezeoperatorv1alpha1.MaintenanceWindow{}
		validator = MaintenanceWindowCustomValidator{}
		obj = &freezeoperatorv1alpha1.MaintenanceWindow{
			Spec: freezeoperatorv1alpha1.MaintenanceWindowSpec{
				Timezone: "UTC",
				Mode:     freezeoperatorv1alpha1.MaintenanceWindowModeDenyOutsideWindows,
				Windows: []freezeoperatorv1alpha1.MaintenanceWindowWindowSpec{
					{
						Name:     "nightly",
						Schedule: "0 2 * * *",
						Duration: metav1.Duration{Duration: 2 * time.Hour},
					},
				},
				Target: freezeoperatorv1alpha1.TargetSpec{
					Kinds: []freezeoperatorv1alpha1.TargetKind{freezeoperatorv1alpha1.TargetKindDeployment},
				},
				Rules: freezeoperatorv1alpha1.PolicyRulesSpec{
					Deny: []freezeoperatorv1alpha1.Action{freezeoperatorv1alpha1.ActionRollout},
				},
			},
		}
	})

	Context("When creating MaintenanceWindow under Validating Webhook", func() {
		It("Should allow a valid MaintenanceWindow", func() {
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should deny creation with invalid timezone", func() {
			obj.Spec.Timezone = "Nowhere/Invalid"
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timezone"))
		})

		It("Should deny creation with invalid cron schedule", func() {
			obj.Spec.Windows[0].Schedule = "not-a-cron"
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("schedule"))
		})

		It("Should deny creation when window duration is zero", func() {
			obj.Spec.Windows[0].Duration = metav1.Duration{Duration: 0}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("duration"))
		})

		It("Should deny creation when window duration is negative", func() {
			obj.Spec.Windows[0].Duration = metav1.Duration{Duration: -time.Hour}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("duration"))
		})

		It("Should allow multiple valid windows", func() {
			obj.Spec.Windows = append(obj.Spec.Windows,
				freezeoperatorv1alpha1.MaintenanceWindowWindowSpec{
					Name:     "weekly",
					Schedule: "0 4 * * 0",
					Duration: metav1.Duration{Duration: time.Hour},
				})
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should allow common IANA timezones", func() {
			for _, tz := range []string{"Europe/Berlin", "America/New_York", "Asia/Tokyo", "UTC"} {
				obj.Spec.Timezone = tz
				_, err := validator.ValidateCreate(ctx, obj)
				Expect(err).ToNot(HaveOccurred(), "timezone %q should be valid", tz)
			}
		})
	})

	Context("When updating MaintenanceWindow under Validating Webhook", func() {
		It("Should allow a valid update", func() {
			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should deny update with invalid cron schedule", func() {
			obj.Spec.Windows[0].Schedule = "60 25 * * *"
			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("When deleting MaintenanceWindow under Validating Webhook", func() {
		It("Should always allow deletion", func() {
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
