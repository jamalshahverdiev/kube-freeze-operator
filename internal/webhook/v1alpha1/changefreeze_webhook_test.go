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

var _ = Describe("ChangeFreeze Webhook", func() {
	var (
		obj       *freezeoperatorv1alpha1.ChangeFreeze
		oldObj    *freezeoperatorv1alpha1.ChangeFreeze
		validator ChangeFreezeCustomValidator
	)

	BeforeEach(func() {
		now := time.Now().UTC()
		oldObj = &freezeoperatorv1alpha1.ChangeFreeze{}
		validator = ChangeFreezeCustomValidator{}
		obj = &freezeoperatorv1alpha1.ChangeFreeze{
			Spec: freezeoperatorv1alpha1.ChangeFreezeSpec{
				StartTime: metav1.Time{Time: now.Add(time.Hour)},
				EndTime:   metav1.Time{Time: now.Add(3 * time.Hour)},
				Target: freezeoperatorv1alpha1.TargetSpec{
					Kinds: []freezeoperatorv1alpha1.TargetKind{freezeoperatorv1alpha1.TargetKindDeployment},
				},
				Rules: freezeoperatorv1alpha1.PolicyRulesSpec{
					Deny: []freezeoperatorv1alpha1.Action{freezeoperatorv1alpha1.ActionRollout},
				},
			},
		}
	})

	Context("When creating ChangeFreeze under Validating Webhook", func() {
		It("Should allow a valid ChangeFreeze", func() {
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should deny creation when endTime is before startTime", func() {
			obj.Spec.StartTime = metav1.Time{Time: time.Now().UTC().Add(3 * time.Hour)}
			obj.Spec.EndTime = metav1.Time{Time: time.Now().UTC().Add(time.Hour)}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("endTime"))
		})

		It("Should deny creation when endTime equals startTime", func() {
			t := time.Now().UTC().Add(time.Hour)
			obj.Spec.StartTime = metav1.Time{Time: t}
			obj.Spec.EndTime = metav1.Time{Time: t}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
		})

		It("Should deny creation with invalid timezone", func() {
			tz := "Nowhere/Invalid"
			obj.Spec.Timezone = &tz
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timezone"))
		})

		It("Should allow creation without optional timezone", func() {
			obj.Spec.Timezone = nil
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should allow creation with valid IANA timezone", func() {
			for _, tz := range []string{"UTC", "Europe/Berlin", "America/Los_Angeles"} {
				tzCopy := tz
				obj.Spec.Timezone = &tzCopy
				_, err := validator.ValidateCreate(ctx, obj)
				Expect(err).ToNot(HaveOccurred(), "timezone %q should be valid", tz)
			}
		})

		It("Should allow active (currently running) freeze", func() {
			now := time.Now().UTC()
			obj.Spec.StartTime = metav1.Time{Time: now.Add(-time.Hour)}
			obj.Spec.EndTime = metav1.Time{Time: now.Add(time.Hour)}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When updating ChangeFreeze under Validating Webhook", func() {
		It("Should allow a valid update", func() {
			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should deny update when endTime is before startTime", func() {
			obj.Spec.StartTime = metav1.Time{Time: time.Now().UTC().Add(3 * time.Hour)}
			obj.Spec.EndTime = metav1.Time{Time: time.Now().UTC().Add(time.Hour)}
			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("When deleting ChangeFreeze under Validating Webhook", func() {
		It("Should always allow deletion", func() {
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
