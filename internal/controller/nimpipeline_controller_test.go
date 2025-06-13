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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

var _ = Describe("NIMPipeline Controller", func() {
	var (
		client     client.Client
		reconciler *NIMPipelineReconciler
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(appsv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(batchv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		client = fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&appsv1alpha1.NIMPipeline{}).
			WithStatusSubresource(&appsv1alpha1.NIMService{}).
			Build()
		reconciler = &NIMPipelineReconciler{
			Client:   client,
			Scheme:   scheme,
			recorder: record.NewFakeRecorder(1000),
		}
	})

	AfterEach(func() {
		// Clean up the NIMPipeline instance
		nimPipeline := &appsv1alpha1.NIMPipeline{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pipeline",
				Namespace: "default",
			},
		}
		_ = client.Delete(context.TODO(), nimPipeline)
	})

	Context("When managing NIMServices", func() {
		It("Should create NIMServices for enabled services in the pipeline", func() {
			ctx := context.TODO()
			nimPipeline := &appsv1alpha1.NIMPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMPipelineSpec{
					Services: []appsv1alpha1.NIMServicePipelineSpec{
						{
							Name:    "nim-llm-service",
							Enabled: ptr.To(true),
							Spec: appsv1alpha1.NIMServiceSpec{
								Image: appsv1alpha1.Image{
									Repository: "llm-nim-container",
									Tag:        "latest",
								},
								Replicas: 1,
								Resources: &corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("100m"),
										corev1.ResourceMemory: resource.MustParse("128Mi"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("200m"),
										corev1.ResourceMemory: resource.MustParse("256Mi"),
									},
								},
							},
						},
						{
							Name:    "nim-embedding-service",
							Enabled: ptr.To(false),
							Spec: appsv1alpha1.NIMServiceSpec{
								Image: appsv1alpha1.Image{
									Repository: "llm-embedding-container",
									Tag:        "latest",
								},
								Replicas: 2,
							},
						},
					},
				},
			}
			Expect(client.Create(ctx, nimPipeline)).To(Succeed())

			// Reconcile the resource
			_, err := reconciler.reconcileNIMPipeline(ctx, nimPipeline)
			Expect(err).ToNot(HaveOccurred())

			By("Checking that the NIMService for the enabled service is created")
			Eventually(func() bool {
				namespacedName := types.NamespacedName{Name: "nim-llm-service", Namespace: nimPipeline.Namespace}
				nimService := &appsv1alpha1.NIMService{}
				err := client.Get(context.TODO(), namespacedName, nimService)
				return err == nil
			}, time.Second*5, time.Millisecond*500).Should(BeTrue())

			By("Ensuring the NIMService for the disabled service is not created")
			Consistently(func() bool {
				namespacedName := types.NamespacedName{Name: "nim-embedding-service", Namespace: nimPipeline.Namespace}
				nimService := &appsv1alpha1.NIMService{}
				err := client.Get(context.TODO(), namespacedName, nimService)
				return errors.IsNotFound(err)
			}, time.Second*2, time.Millisecond*500).Should(BeTrue())
		})

		It("Should delete NIMServices when they are disabled", func() {
			ctx := context.TODO()
			nimPipeline := &appsv1alpha1.NIMPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMPipelineSpec{
					Services: []appsv1alpha1.NIMServicePipelineSpec{
						{
							Name:    "nim-llm-service",
							Enabled: ptr.To(true),
							Spec: appsv1alpha1.NIMServiceSpec{
								Image: appsv1alpha1.Image{
									Repository: "llm-nim-container",
									Tag:        "latest",
								},
								Replicas: 1,
								Resources: &corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("100m"),
										corev1.ResourceMemory: resource.MustParse("128Mi"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("200m"),
										corev1.ResourceMemory: resource.MustParse("256Mi"),
									},
								},
							},
						},
						{
							Name:    "nim-embedding-service",
							Enabled: ptr.To(false),
							Spec: appsv1alpha1.NIMServiceSpec{
								Image: appsv1alpha1.Image{
									Repository: "llm-embedding-container",
									Tag:        "latest",
								},
								Replicas: 2,
							},
						},
					},
				},
			}
			Expect(client.Create(ctx, nimPipeline)).To(Succeed())

			// Reconcile the resource
			_, err := reconciler.reconcileNIMPipeline(ctx, nimPipeline)
			Expect(err).ToNot(HaveOccurred())

			// Enable nim-embedding-service in the pipeline spec
			updatePipeline := &appsv1alpha1.NIMPipeline{}
			namespacedName := types.NamespacedName{Name: "test-pipeline", Namespace: "default"}
			Expect(client.Get(context.TODO(), namespacedName, updatePipeline)).To(Succeed())

			updatePipeline.Spec.Services[1].Enabled = ptr.To(true)
			Expect(client.Update(ctx, updatePipeline)).To(Succeed())

			// Reconcile the resource
			_, err = reconciler.reconcileNIMPipeline(ctx, updatePipeline)
			Expect(err).ToNot(HaveOccurred())

			By("Checking that the NIMService for the newly enabled service is created")
			Eventually(func() bool {
				namespacedName := types.NamespacedName{Name: "nim-embedding-service", Namespace: "default"}
				nimService := &appsv1alpha1.NIMService{}
				err := client.Get(context.TODO(), namespacedName, nimService)
				return err == nil
			}, time.Second*5, time.Millisecond*500).Should(BeTrue())

			// Disable nim-llm-service in the pipeline spec
			updatePipeline.Spec.Services[0].Enabled = ptr.To(false)
			Expect(client.Update(ctx, updatePipeline)).To(Succeed())

			// Reconcile the resource
			_, err = reconciler.reconcileNIMPipeline(ctx, updatePipeline)
			Expect(err).ToNot(HaveOccurred())

			By("Checking that the NIMService for the disabled service is deleted")
			Eventually(func() bool {
				namespacedName := types.NamespacedName{Name: "nim-llm-service", Namespace: "default"}
				nimService := &appsv1alpha1.NIMService{}
				err := client.Get(context.TODO(), namespacedName, nimService)
				return errors.IsNotFound(err)
			}, time.Second*5, time.Millisecond*500).Should(BeTrue())
		})

		It("Should update NIMServices with dependencies, including service environment variables", func() {
			ctx := context.TODO()
			nimPipeline := &appsv1alpha1.NIMPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMPipelineSpec{
					Services: []appsv1alpha1.NIMServicePipelineSpec{
						{
							Name:    "nim-llm-service",
							Enabled: ptr.To(true),
							Spec: appsv1alpha1.NIMServiceSpec{
								Image: appsv1alpha1.Image{
									Repository: "llm-nim-container",
									Tag:        "latest",
								},
								Replicas: 1,
								Resources: &corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("100m"),
										corev1.ResourceMemory: resource.MustParse("128Mi"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("200m"),
										corev1.ResourceMemory: resource.MustParse("256Mi"),
									},
								},
							},
							Dependencies: []appsv1alpha1.ServiceDependency{
								{Name: "dependency-service", Port: 9090, EnvName: "CUSTOM_DEPENDENCY_SERVICE", EnvValue: "dependency-service-2.default.svc.local:9090"},
							},
						},
					},
				},
			}
			Expect(client.Create(ctx, nimPipeline)).To(Succeed())

			// Reconcile the resource
			_, err := reconciler.reconcileNIMPipeline(ctx, nimPipeline)
			Expect(err).ToNot(HaveOccurred())

			// Helper function to validate environment variables
			validateEnvVars := func(serviceName string, expectedEnvVars []corev1.EnvVar) {
				namespacedName := types.NamespacedName{Name: serviceName, Namespace: "default"}
				nimService := &appsv1alpha1.NIMService{}
				err := client.Get(context.TODO(), namespacedName, nimService)
				Expect(err).ToNot(HaveOccurred())

				for _, expectedEnvVar := range expectedEnvVars {
					Expect(nimService.Spec.Env).To(ContainElement(expectedEnvVar))
				}
			}

			// Validate environment variables for LLM service
			expectedEnvVarsForLLM := []corev1.EnvVar{
				{
					Name:  "CUSTOM_DEPENDENCY_SERVICE",
					Value: "dependency-service-2.default.svc.local:9090",
				},
			}
			validateEnvVars("nim-llm-service", expectedEnvVarsForLLM)
		})
	})
})
