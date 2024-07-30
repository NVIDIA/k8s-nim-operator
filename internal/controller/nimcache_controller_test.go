/*
Copyright 2024.

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
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/controller/platform/standalone"
)

var _ = Describe("NIMCache Controller", func() {
	var (
		reconciler *NIMCacheReconciler
		client     client.Client
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(appsv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(batchv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		client = fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&appsv1alpha1.NIMCache{}).
			WithStatusSubresource(&batchv1.Job{}).
			Build()
		reconciler = &NIMCacheReconciler{
			Client:   client,
			scheme:   scheme,
			Platform: &standalone.Standalone{},
		}
	})

	AfterEach(func() {
		// Clean up the NIMCache instance
		nimCache := &appsv1alpha1.NIMCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-nimcache",
				Namespace: "default",
			},
		}
		_ = client.Delete(context.TODO(), nimCache)
	})

	Context("When creating a NIMCache", func() {
		It("should create a Job and PVC", func() {
			ctx := context.TODO()
			NIMCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimcache",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source:  appsv1alpha1.NIMSource{NGC: &appsv1alpha1.NGCSource{ModelPuller: "test-container", PullSecret: "my-secret"}},
					Storage: appsv1alpha1.Storage{PVC: appsv1alpha1.PersistentVolumeClaim{Create: ptr.To[bool](true), StorageClass: "standard", Size: "1Gi"}},
				},
				Status: appsv1alpha1.NIMCacheStatus{
					State: appsv1alpha1.NimCacheStatusNotReady,
				},
			}
			Expect(client.Create(ctx, NIMCache)).To(Succeed())

			// Reconcile the resource
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-nimcache",
					Namespace: "default",
				},
			})
			Expect(err).ToNot(HaveOccurred())

			// Check if the Job was created
			// Wait for reconciliation to complete with a timeout
			Eventually(func() error {
				job := &batchv1.Job{}
				jobName := types.NamespacedName{Name: "test-nimcache-job", Namespace: "default"}
				return client.Get(ctx, jobName, job)
			}, time.Second*10).Should(Succeed())

			// Check if the PVC was created
			// Wait for reconciliation to complete with a timeout
			Eventually(func() error {
				pvc := &corev1.PersistentVolumeClaim{}
				pvcName := types.NamespacedName{Name: "test-nimcache-pvc", Namespace: "default"}
				return client.Get(ctx, pvcName, pvc)
			}, time.Second*10).Should(Succeed())
		})
	})

	Context("When deleting a NIMCache", func() {
		It("should clean up resources", func() {
			ctx := context.TODO()
			NIMCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-nimcache",
					Namespace:         "default",
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
					Finalizers:        []string{NIMCacheFinalizer},
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source:  appsv1alpha1.NIMSource{NGC: &appsv1alpha1.NGCSource{ModelPuller: "test-container", PullSecret: "my-secret"}},
					Storage: appsv1alpha1.Storage{PVC: appsv1alpha1.PersistentVolumeClaim{Create: ptr.To[bool](true), StorageClass: "standard", Size: "1Gi"}},
				},
				Status: appsv1alpha1.NIMCacheStatus{
					State: "Initializing",
				},
			}
			Expect(client.Create(ctx, NIMCache)).To(Succeed())

			// Reconcile the resource
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-nimcache",
					Namespace: "default",
				},
			})
			Expect(err).ToNot(HaveOccurred())

			// Check if the Job was created
			// Wait for reconciliation to complete with a timeout
			Eventually(func() error {
				job := &batchv1.Job{}
				jobName := types.NamespacedName{Name: "test-nimcache-job", Namespace: "default"}
				return client.Get(ctx, jobName, job)
			}, time.Second*10).Should(Succeed())

			// Check if the PVC was created
			// Wait for reconciliation to complete with a timeout
			Eventually(func() error {
				pvc := &corev1.PersistentVolumeClaim{}
				pvcName := types.NamespacedName{Name: "test-nimcache-pvc", Namespace: "default"}
				return client.Get(ctx, pvcName, pvc)
			}, time.Second*10).Should(Succeed())

			// Delete the NIMCache instance
			Expect(client.Delete(ctx, NIMCache)).To(Succeed())

			// Reconcile the resource again
			_, err = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-nimcache",
					Namespace: "default",
				},
			})
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
