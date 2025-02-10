/*
Copyright 2025.

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

	utils "github.com/NVIDIA/k8s-nim-operator/internal/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("NemoService Controller", func() {
	var (
		client     client.Client
		reconciler *NemoServiceReconciler
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(appsv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(batchv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		client = fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&appsv1alpha1.NemoService{}).
			WithStatusSubresource(&appsv1alpha1.NemoCustomizer{}).
			WithStatusSubresource(&appsv1alpha1.NemoDatastore{}).
			WithStatusSubresource(&appsv1alpha1.NemoEvaluator{}).
			WithStatusSubresource(&appsv1alpha1.NemoGuardrail{}).
			WithStatusSubresource(&appsv1alpha1.NemoEntitystore{}).
			Build()
		reconciler = &NemoServiceReconciler{
			Client:   client,
			Scheme:   scheme,
			recorder: record.NewFakeRecorder(1000),
		}
	})

	AfterEach(func() {
		// Clean up the NemoService instance
		nemoService := &appsv1alpha1.NemoService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-nemoservice",
				Namespace: "default",
			},
		}
		_ = client.Delete(context.TODO(), nemoService)
	})

	Context("When managing NemoServices", func() {
		It("Should create NemoServices for enabled services in the nemoservice", func() {
			ctx := context.TODO()
			nemoService := &appsv1alpha1.NemoService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nemoservice",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NemoServiceSpec{
					Datastore: appsv1alpha1.NemoServiceDatastoreSpec{
						Name:    "test-nemodatastore",
						Enabled: ptr.To[bool](false),
					},
					Customizer: appsv1alpha1.NemoServiceCustomizerSpec{
						Name:    "test-nemocustomizer",
						Enabled: ptr.To[bool](true),
						Spec: &appsv1alpha1.NemoCustomizerSpec{
							Labels:      map[string]string{"app": "nemo-customizer"},
							Annotations: map[string]string{"annotation-key": "annotation-value"},
							Image:       appsv1alpha1.Image{Repository: "nvcr.io/nvidia/nemo-customizer", PullPolicy: "IfNotPresent", Tag: "v0.1.0", PullSecrets: []string{"ngc-secret"}},
							Env: []corev1.EnvVar{
								{
									Name:  "custom-env",
									Value: "custom-value",
								},
							},
							CustomizerConfig: "test-training-data",
							WandBSecret: appsv1alpha1.WandBSecret{
								Name:          "wandb-secret",
								APIKeyKey:     "api_key",
								EncryptionKey: "encryption_key",
							},
							OpenTelemetry: appsv1alpha1.OTelSpec{
								Enabled:              ptr.To[bool](true),
								DisableLogging:       ptr.To[bool](false),
								ExporterOtlpEndpoint: "http://opentelemetry-collector.default.svc.cluster.local:4317",
							},
							DatabaseConfig: appsv1alpha1.DatabaseConfig{
								Credentials: appsv1alpha1.DatabaseCredentials{
									User:        "ncsuser",
									SecretName:  "ncs-pg-existing-secret",
									PasswordKey: "password",
								},
								Host:         "ncs-pg.default.svc.cluster.local",
								Port:         5432,
								DatabaseName: "ncsdb",
							},
							Replicas: 1,
							Expose: appsv1alpha1.Expose{
								Service: appsv1alpha1.Service{
									Type: corev1.ServiceTypeClusterIP,
									Ports: []corev1.ServicePort{
										{Name: "api", Port: 8000, Protocol: corev1.ProtocolTCP},
										{Name: "internal", Port: 9009, Protocol: corev1.ProtocolTCP},
									},
									Annotations: map[string]string{
										"annotation-key-specific": "service",
									},
								},
							},
						},
					},
				},
			}
			Expect(client.Create(ctx, nemoService)).To(Succeed())

			// Reconcile the resource
			namespacedName := types.NamespacedName{Name: nemoService.Name, Namespace: "default"}
			result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			By("Checking that the NemoServices for the enabled service is created")
			Eventually(func() bool {
				namespacedName := types.NamespacedName{Name: "test-nemocustomizer", Namespace: nemoService.Namespace}
				nimService := &appsv1alpha1.NemoCustomizer{}
				err := client.Get(context.TODO(), namespacedName, nimService)
				return err == nil
			}, time.Second*5, time.Millisecond*500).Should(BeTrue())

			By("Ensuring the NIMService for the disabled service is not created")
			Consistently(func() bool {
				namespacedName := types.NamespacedName{Name: "test-nemodatastore", Namespace: nemoService.Namespace}
				nimService := &appsv1alpha1.NIMService{}
				err := client.Get(context.TODO(), namespacedName, nimService)
				return errors.IsNotFound(err)
			}, time.Second*2, time.Millisecond*500).Should(BeTrue())
		})

		It("Should delete NemoServices when they are disabled", func() {
			ctx := context.TODO()
			nemoService := &appsv1alpha1.NemoService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nemoservice",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NemoServiceSpec{
					Customizer: appsv1alpha1.NemoServiceCustomizerSpec{
						Name:    "test-nemocustomizer",
						Enabled: ptr.To[bool](true),
						Spec: &appsv1alpha1.NemoCustomizerSpec{
							Labels:      map[string]string{"app": "nemo-customizer"},
							Annotations: map[string]string{"annotation-key": "annotation-value"},
							Image:       appsv1alpha1.Image{Repository: "nvcr.io/nvidia/nemo-customizer", PullPolicy: "IfNotPresent", Tag: "v0.1.0", PullSecrets: []string{"ngc-secret"}},
							Env: []corev1.EnvVar{
								{
									Name:  "custom-env",
									Value: "custom-value",
								},
							},
							CustomizerConfig: "test-training-data",
							WandBSecret: appsv1alpha1.WandBSecret{
								Name:          "wandb-secret",
								APIKeyKey:     "api_key",
								EncryptionKey: "encryption_key",
							},
							OpenTelemetry: appsv1alpha1.OTelSpec{
								Enabled:              ptr.To[bool](true),
								DisableLogging:       ptr.To[bool](false),
								ExporterOtlpEndpoint: "http://opentelemetry-collector.default.svc.cluster.local:4317",
							},
							DatabaseConfig: appsv1alpha1.DatabaseConfig{
								Credentials: appsv1alpha1.DatabaseCredentials{
									User:        "ncsuser",
									SecretName:  "ncs-pg-existing-secret",
									PasswordKey: "password",
								},
								Host:         "ncs-pg.default.svc.cluster.local",
								Port:         5432,
								DatabaseName: "ncsdb",
							},
							Replicas: 1,
							Expose: appsv1alpha1.Expose{
								Service: appsv1alpha1.Service{
									Type: corev1.ServiceTypeClusterIP,
									Ports: []corev1.ServicePort{
										{Name: "api", Port: 8000, Protocol: corev1.ProtocolTCP},
										{Name: "internal", Port: 9009, Protocol: corev1.ProtocolTCP},
									},
									Annotations: map[string]string{
										"annotation-key-specific": "service",
									},
								},
							},
						},
					},
				},
			}
			Expect(client.Create(ctx, nemoService)).To(Succeed())

			// Reconcile the resource
			namespacedName := types.NamespacedName{Name: nemoService.Name, Namespace: "default"}
			result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Disable nemo-customizer in the nemoservice spec
			updateNemoService := &appsv1alpha1.NemoService{}
			Expect(client.Get(context.TODO(), namespacedName, updateNemoService)).To(Succeed())

			updateNemoService.Spec.Customizer.Enabled = utils.BoolPtr(false)
			Expect(client.Update(ctx, updateNemoService)).To(Succeed())

			// Reconcile the resource
			result, err = reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			By("Checking that the newly disabled Datastore service is deleted")
			Eventually(func() bool {
				namespacedName := types.NamespacedName{Name: "test-nemocustomizer", Namespace: "default"}
				nimService := &appsv1alpha1.NIMService{}
				err := client.Get(context.TODO(), namespacedName, nimService)
				return errors.IsNotFound(err)
			}, time.Second*5, time.Millisecond*500).Should(BeTrue())
		})
	})
})
