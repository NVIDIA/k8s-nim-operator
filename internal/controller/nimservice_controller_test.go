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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	resourcev1beta2 "k8s.io/api/resource/v1beta2"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

var _ = Describe("NIMService Controller", func() {
	var (
		reconciler *NIMServiceReconciler
		testClient client.Client
		scheme     *runtime.Scheme
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(appsv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(batchv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(resourcev1beta2.AddToScheme(scheme)).To(Succeed())
		Expect(gatewayv1.AddToScheme(scheme)).To(Succeed())

		testClient = fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&appsv1alpha1.NIMService{}).
			WithStatusSubresource(&batchv1.Job{}).
			WithIndex(&appsv1alpha1.NIMService{}, "spec.storage.nimCache.name", func(obj client.Object) []string {
				nimService, ok := obj.(*appsv1alpha1.NIMService)
				if !ok {
					return []string{}
				}
				return []string{nimService.Spec.Storage.NIMCache.Name}
			}).
			Build()
		reconciler = &NIMServiceReconciler{
			Client:   testClient,
			scheme:   scheme,
			recorder: record.NewFakeRecorder(1000),
		}
	})

	AfterEach(func() {
		// Clean up the NIMService instance
		nimCache := &appsv1alpha1.NIMService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
			},
		}
		_ = testClient.Delete(context.TODO(), nimCache)
	})

	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		nimservice := &appsv1alpha1.NIMService{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind NIMService")
			err := k8sClient.Get(ctx, typeNamespacedName, nimservice)
			if err != nil && errors.IsNotFound(err) {
				resource := &appsv1alpha1.NIMService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &appsv1alpha1.NIMService{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance NIMService")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})

	Describe("mapNIMCacheToNIMService tests", func() {
		Context("when NIMCache is referenced by NIMServices", func() {
			It("should return reconcile requests for matching NIMServices from same namespace", func() {
				nimCache := &appsv1alpha1.NIMCache{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-nimcache",
						Namespace: "default",
					},
					Spec: appsv1alpha1.NIMCacheSpec{
						Source: appsv1alpha1.NIMSource{
							NGC: &appsv1alpha1.NGCSource{},
						},
						Storage: appsv1alpha1.NIMCacheStorage{
							PVC: appsv1alpha1.PersistentVolumeClaim{
								Name: "test-pvc",
							},
						},
					},
				}
				Expect(testClient.Create(ctx, nimCache)).To(Succeed())

				nimService1 := &appsv1alpha1.NIMService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-service",
						Namespace: "default",
					},
					Spec: appsv1alpha1.NIMServiceSpec{
						Storage: appsv1alpha1.NIMServiceStorage{
							NIMCache: appsv1alpha1.NIMCacheVolSpec{
								Name: "test-nimcache",
							},
						},
					},
				}
				nimService2 := &appsv1alpha1.NIMService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-service-1",
						Namespace: "default",
					},
					Spec: appsv1alpha1.NIMServiceSpec{
						Storage: appsv1alpha1.NIMServiceStorage{
							NIMCache: appsv1alpha1.NIMCacheVolSpec{
								Name: "test-nimcache",
							},
						},
					},
				}
				nimService3 := &appsv1alpha1.NIMService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-service-2",
						Namespace: "test-ns",
					},
					Spec: appsv1alpha1.NIMServiceSpec{
						Storage: appsv1alpha1.NIMServiceStorage{
							NIMCache: appsv1alpha1.NIMCacheVolSpec{
								Name: "test-nimcache",
							},
						},
					},
				}
				Expect(testClient.Create(ctx, nimService1)).To(Succeed())
				Expect(testClient.Create(ctx, nimService2)).To(Succeed())
				Expect(testClient.Create(ctx, nimService3)).To(Succeed())

				var nimServices appsv1alpha1.NIMServiceList
				err := testClient.List(ctx, &nimServices, client.MatchingFields{"spec.storage.nimCache.name": nimCache.GetName()}, client.InNamespace(nimCache.GetNamespace()))
				Expect(err).NotTo(HaveOccurred())
				requests := reconciler.mapNIMCacheToNIMService(ctx, nimCache)

				Expect(requests).To(HaveLen(2))
				Expect(requests).To(ContainElements(
					ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-service", Namespace: "default"}},
					ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-service-1", Namespace: "default"}},
				))
			})

			It("should return empty requests when no NIMServices reference the cache", func() {
				nimCache := &appsv1alpha1.NIMCache{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cache",
						Namespace: "default",
					},
					Spec: appsv1alpha1.NIMCacheSpec{
						Source: appsv1alpha1.NIMSource{
							NGC: &appsv1alpha1.NGCSource{},
						},
						Storage: appsv1alpha1.NIMCacheStorage{
							PVC: appsv1alpha1.PersistentVolumeClaim{
								Name: "test-pvc",
							},
						},
					},
				}
				Expect(testClient.Create(ctx, nimCache)).To(Succeed())

				nimService := &appsv1alpha1.NIMService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-service",
						Namespace: "default",
					},
					Spec: appsv1alpha1.NIMServiceSpec{
						Storage: appsv1alpha1.NIMServiceStorage{
							NIMCache: appsv1alpha1.NIMCacheVolSpec{
								Name: "test-cache-1",
							},
						},
					},
				}
				Expect(testClient.Create(ctx, nimService)).To(Succeed())

				requests := reconciler.mapNIMCacheToNIMService(ctx, nimCache)

				Expect(requests).To(BeEmpty())
			})

			It("should handle invalid object type", func() {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
					},
				}

				requests := reconciler.mapNIMCacheToNIMService(ctx, pod)
				Expect(requests).To(BeEmpty())
			})
		})
	})

	Describe("mapResourceClaimToNIMService tests", func() {
		It("should return reconcile requests for NIMServices with matching ResourceClaimName in the same namespace", func() {
			resourceClaim := &resourcev1beta2.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-claim",
					Namespace: "default",
				},
			}
			Expect(testClient.Create(ctx, resourceClaim)).To(Succeed())

			nimService1 := &appsv1alpha1.NIMService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMServiceSpec{
					DRAResources: []appsv1alpha1.DRAResource{
						{
							ResourceClaimName: ptr.To("test-claim"),
						},
					},
				},
			}
			nimService2 := &appsv1alpha1.NIMService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service-1",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMServiceSpec{
					DRAResources: []appsv1alpha1.DRAResource{
						{
							ResourceClaimName: ptr.To("test-claim"),
						},
					},
				},
			}
			nimService3 := &appsv1alpha1.NIMService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service-2",
					Namespace: "test-ns",
				},
				Spec: appsv1alpha1.NIMServiceSpec{
					DRAResources: []appsv1alpha1.DRAResource{
						{
							ResourceClaimName: ptr.To("test-claim"),
						},
					},
				},
			}
			Expect(testClient.Create(ctx, nimService1)).To(Succeed())
			Expect(testClient.Create(ctx, nimService2)).To(Succeed())
			Expect(testClient.Create(ctx, nimService3)).To(Succeed())

			requests := reconciler.mapResourceClaimToNIMService(ctx, resourceClaim)

			Expect(requests).To(HaveLen(2))
			Expect(requests).To(ContainElements(
				ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-service", Namespace: "default"}},
				ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-service-1", Namespace: "default"}},
			))
		})

		It("should return reconcile requests for NIMServices with matching ResourceClaimTemplateName", func() {
			resourceClaim := &resourcev1beta2.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "template-generated-claim",
					Namespace: "default",
					Annotations: map[string]string{
						// Generated name for nimservice name `test-service` and spec.draResources[0].resouceclaimtemplatename `test-template`
						utils.DRAPodClaimNameAnnotationKey: "claim-8568b4fb55-1-5cb744997d-0",
					},
				},
			}
			Expect(testClient.Create(ctx, resourceClaim)).To(Succeed())

			nimService := &appsv1alpha1.NIMService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMServiceSpec{
					DRAResources: []appsv1alpha1.DRAResource{
						{
							ResourceClaimTemplateName: ptr.To("test-template"),
						},
					},
				},
			}
			Expect(testClient.Create(ctx, nimService)).To(Succeed())

			requests := reconciler.mapResourceClaimToNIMService(ctx, resourceClaim)

			Expect(requests).To(HaveLen(1))
			Expect(requests[0]).To(Equal(ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-service", Namespace: "default"},
			}))
		})

		It("should return empty requests when no NIMServices reference the claim", func() {
			resourceClaim := &resourcev1beta2.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-claim",
					Namespace: "default",
				},
			}
			Expect(testClient.Create(ctx, resourceClaim)).To(Succeed())

			nimService := &appsv1alpha1.NIMService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMServiceSpec{
					DRAResources: []appsv1alpha1.DRAResource{
						{
							ResourceClaimName: ptr.To("unused-claim"),
						},
					},
				},
			}
			Expect(testClient.Create(ctx, nimService)).To(Succeed())

			requests := reconciler.mapResourceClaimToNIMService(ctx, resourceClaim)

			Expect(requests).To(BeEmpty())
		})

		It("should handle invalid object type", func() {
			// Pass a non-ResourceClaim object
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
			}

			requests := reconciler.mapResourceClaimToNIMService(ctx, pod)
			Expect(requests).To(BeEmpty())
		})

		It("should handle NIMServices without DRA resources", func() {
			// Create a ResourceClaim
			resourceClaim := &resourcev1beta2.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-claim",
					Namespace: "default",
				},
			}
			Expect(testClient.Create(ctx, resourceClaim)).To(Succeed())

			// Create a NIMService without DRA resources
			nimService := &appsv1alpha1.NIMService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMServiceSpec{
					// No DRAResources specified
				},
			}
			Expect(testClient.Create(ctx, nimService)).To(Succeed())

			// Test the mapping function
			requests := reconciler.mapResourceClaimToNIMService(ctx, resourceClaim)

			Expect(requests).To(BeEmpty())
		})
	})
})
