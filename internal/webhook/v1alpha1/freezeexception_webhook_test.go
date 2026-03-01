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

var _ = Describe("FreezeException Webhook", func() {
	var (
		obj       *freezeoperatorv1alpha1.FreezeException
		oldObj    *freezeoperatorv1alpha1.FreezeException
		validator FreezeExceptionCustomValidator
	)

	BeforeEach(func() {
		now := time.Now().UTC()
		oldObj = &freezeoperatorv1alpha1.FreezeException{}
		validator = FreezeExceptionCustomValidator{}
		obj = &freezeoperatorv1alpha1.FreezeException{
			Spec: freezeoperatorv1alpha1.FreezeExceptionSpec{
				ActiveFrom: metav1.Time{Time: now.Add(-time.Hour)},
				ActiveTo:   metav1.Time{Time: now.Add(time.Hour)},
				Target: freezeoperatorv1alpha1.TargetSpec{
					Kinds: []freezeoperatorv1alpha1.TargetKind{freezeoperatorv1alpha1.TargetKindDeployment},
				},
				Allow:  []freezeoperatorv1alpha1.Action{freezeoperatorv1alpha1.ActionRollout},
				Reason: "emergency hotfix",
			},
		}
	})

	Context("When creating FreezeException under Validating Webhook", func() {
		It("Should allow a valid FreezeException", func() {
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should deny creation when activeTo is before activeFrom", func() {
			obj.Spec.ActiveFrom = metav1.Time{Time: time.Now().UTC().Add(2 * time.Hour)}
			obj.Spec.ActiveTo = metav1.Time{Time: time.Now().UTC().Add(time.Hour)}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("activeTo"))
		})

		It("Should deny creation when activeTo equals activeFrom", func() {
			t := time.Now().UTC().Add(time.Hour)
			obj.Spec.ActiveFrom = metav1.Time{Time: t}
			obj.Spec.ActiveTo = metav1.Time{Time: t}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
		})

		It("Should allow multiple actions in allow list", func() {
			obj.Spec.Allow = []freezeoperatorv1alpha1.Action{
				freezeoperatorv1alpha1.ActionRollout,
				freezeoperatorv1alpha1.ActionScale,
				freezeoperatorv1alpha1.ActionCreate,
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should allow exception with constraints", func() {
			obj.Spec.Constraints = &freezeoperatorv1alpha1.FreezeExceptionConstraintsSpec{
				RequireLabels: map[string]string{"hotfix": "true"},
				AllowedUsers:  []string{"oncall@example.com"},
				AllowedGroups: []string{"sre-team"},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should allow exception with ticket URL and approver", func() {
			obj.Spec.TicketURL = "https://jira.example.com/browse/OPS-1234"
			obj.Spec.ApprovedBy = "alice@example.com"
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should allow future exception (not yet active)", func() {
			future := time.Now().UTC().Add(24 * time.Hour)
			obj.Spec.ActiveFrom = metav1.Time{Time: future}
			obj.Spec.ActiveTo = metav1.Time{Time: future.Add(2 * time.Hour)}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When updating FreezeException under Validating Webhook", func() {
		It("Should allow a valid update", func() {
			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should deny update when activeTo is before activeFrom", func() {
			obj.Spec.ActiveFrom = metav1.Time{Time: time.Now().UTC().Add(2 * time.Hour)}
			obj.Spec.ActiveTo = metav1.Time{Time: time.Now().UTC().Add(time.Hour)}
			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("When deleting FreezeException under Validating Webhook", func() {
		It("Should always allow deletion", func() {
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
