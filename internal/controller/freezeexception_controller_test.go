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

var _ = Describe("FreezeException Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
		}
		freezeexception := &freezeoperatorv1alpha1.FreezeException{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind FreezeException")
			err := k8sClient.Get(ctx, typeNamespacedName, freezeexception)
			if err != nil && errors.IsNotFound(err) {
				now := time.Now().UTC()
				resource := &freezeoperatorv1alpha1.FreezeException{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: freezeoperatorv1alpha1.FreezeExceptionSpec{
						ActiveFrom: metav1.Time{Time: now},
						ActiveTo:   metav1.Time{Time: now.Add(time.Hour)},
						Target: freezeoperatorv1alpha1.TargetSpec{
							Kinds: []freezeoperatorv1alpha1.TargetKind{freezeoperatorv1alpha1.TargetKindDeployment},
						},
						Allow:  []freezeoperatorv1alpha1.Action{freezeoperatorv1alpha1.ActionRollout},
						Reason: "test exception",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &freezeoperatorv1alpha1.FreezeException{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance FreezeException")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &FreezeExceptionReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
