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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	freezeoperatorv1alpha1 "github.com/jamalshahverdiev/kube-freeze-operator/api/v1alpha1"
)

var _ = Describe("MaintenanceWindow Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
		}
		maintenancewindow := &freezeoperatorv1alpha1.MaintenanceWindow{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind MaintenanceWindow")
			err := k8sClient.Get(ctx, typeNamespacedName, maintenancewindow)
			if err != nil && errors.IsNotFound(err) {
				resource := &freezeoperatorv1alpha1.MaintenanceWindow{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: freezeoperatorv1alpha1.MaintenanceWindowSpec{
						Timezone: "UTC",
						Mode:     freezeoperatorv1alpha1.MaintenanceWindowModeDenyOutsideWindows,
						Windows: []freezeoperatorv1alpha1.MaintenanceWindowWindowSpec{
							{
								Name:     "test-window",
								Schedule: "0 0 * * *",
								Duration: metav1.Duration{Duration: time.Hour},
							},
						},
						Target: freezeoperatorv1alpha1.TargetSpec{
							Kinds: []freezeoperatorv1alpha1.TargetKind{freezeoperatorv1alpha1.TargetKindDeployment},
						},
						Rules: freezeoperatorv1alpha1.PolicyRulesSpec{
							Deny: []freezeoperatorv1alpha1.Action{freezeoperatorv1alpha1.ActionRollout},
						},
						Behavior: freezeoperatorv1alpha1.PolicyBehaviorSpec{
							SuspendCronJobs: false,
						},
						Message: freezeoperatorv1alpha1.MessageSpec{
							Reason: "test",
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &freezeoperatorv1alpha1.MaintenanceWindow{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance MaintenanceWindow")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &MaintenanceWindowReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
