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
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	"github.com/NVIDIA/k8s-nim-operator/internal/render"
)

var _ = Describe("NemoEntitystore Controller", func() {
	var (
		reconciler *NemoEntitystoreReconciler

		client   crclient.Client
		scheme   *runtime.Scheme
		updater  conditions.Updater
		renderer render.Renderer
	)

	// status causes.
	var (
		requiredDatabaseConfigCause = metav1.StatusCause{
			Type:    "FieldValueRequired",
			Message: "Required value",
			Field:   "spec.databaseConfig",
		}
		invalidDatabaseHostCause = metav1.StatusCause{
			Type:    "FieldValueInvalid",
			Message: "Invalid value: \"\": spec.databaseConfig.host in body should be at least 1 chars long",
			Field:   "spec.databaseConfig.host",
		}
		invalidUserCause = metav1.StatusCause{
			Type:    "FieldValueInvalid",
			Message: "Invalid value: \"\": spec.databaseConfig.credentials.user in body should be at least 1 chars long",
			Field:   "spec.databaseConfig.credentials.user",
		}
		invalidSecretNameCause = metav1.StatusCause{
			Type:    "FieldValueInvalid",
			Message: "Invalid value: \"\": spec.databaseConfig.credentials.secretName in body should be at least 1 chars long",
			Field:   "spec.databaseConfig.credentials.secretName",
		}
		invalidDatabaseNameCause = metav1.StatusCause{
			Type:    "FieldValueInvalid",
			Message: "Invalid value: \"\": spec.databaseConfig.databaseName in body should be at least 1 chars long",
			Field:   "spec.databaseConfig.databaseName",
		}
		invalidDatabasePortCause = metav1.StatusCause{
			Type:    "FieldValueInvalid",
			Message: "Invalid value: 65536: spec.databaseConfig.port in body should be less than or equal to 65535",
			Field:   "spec.databaseConfig.port",
		}
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(appsv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(monitoringv1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())
		Expect(autoscalingv2.AddToScheme(scheme)).To(Succeed())
		Expect(batchv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(networkingv1.AddToScheme(scheme)).To(Succeed())
		Expect(rbacv1.AddToScheme(scheme)).To(Succeed())

		client = fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&appsv1alpha1.NemoEntitystore{}).
			WithStatusSubresource(&appsv1.Deployment{}).
			Build()
		updater = conditions.NewUpdater(client)
		manifestsDir, err := filepath.Abs("../../manifests")
		Expect(err).ToNot(HaveOccurred())
		renderer = render.NewRenderer(manifestsDir)
		reconciler = &NemoEntitystoreReconciler{
			Client:   client,
			scheme:   scheme,
			updater:  updater,
			renderer: renderer,
			recorder: record.NewFakeRecorder(1000),
		}
	})

	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		nemoEntityStore := &appsv1alpha1.NemoEntitystore{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind NemoEntitystore")
			err := k8sClient.Get(ctx, typeNamespacedName, nemoEntityStore)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		AfterEach(func() {
			resource := &appsv1alpha1.NemoEntitystore{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err != nil {
				Expect(errors.IsNotFound(err)).To(BeTrue())
			} else {
				By("Cleanup the specific resource instance NemoEntitystore")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			Expect(CleanupChildEntities(ctx, client, typeNamespacedName, schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})).To(Succeed())
			Expect(CleanupChildEntities(ctx, client, typeNamespacedName, schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"})).To(Succeed())
			Expect(CleanupChildEntities(ctx, client, typeNamespacedName, schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ServiceAccount"})).To(Succeed())
			Expect(CleanupChildEntities(ctx, client, typeNamespacedName, schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"})).To(Succeed())
			Expect(CleanupChildEntities(ctx, client, typeNamespacedName, schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBinding"})).To(Succeed())
			Expect(CleanupChildEntities(ctx, client, typeNamespacedName, schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"})).To(Succeed())
			Expect(CleanupChildEntities(ctx, client, typeNamespacedName, schema.GroupVersionKind{Group: "autoscaling", Version: "v1", Kind: "HorizontalPodAutoscaler"})).To(Succeed())
			Expect(CleanupChildEntities(ctx, client, typeNamespacedName, schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "ServiceMonitor"})).To(Succeed())
		})

		It("should reject the resource if databaseConfig is missing", func() {
			resource := &appsv1alpha1.NemoEntitystore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: appsv1alpha1.NemoEntitystoreSpec{},
			}
			By("creating the custom resource for the Kind NemoEntitystore")
			err := k8sClient.Create(ctx, resource)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsInvalid(err)).To(BeTrue())
			statusErr := err.(*errors.StatusError)
			Expect(statusErr.ErrStatus.Details.Causes).To(ContainElement(requiredDatabaseConfigCause))
		})

		It("should reject the resource if databaseConfig is missing required fields", func() {
			resource := &appsv1alpha1.NemoEntitystore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: appsv1alpha1.NemoEntitystoreSpec{
					DatabaseConfig: &appsv1alpha1.DatabaseConfig{},
				},
			}
			By("creating the custom resource for the Kind NemoEntitystore")
			err := k8sClient.Create(ctx, resource)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsInvalid(err)).To(BeTrue())
			statusErr := err.(*errors.StatusError)
			Expect(statusErr.ErrStatus.Details.Causes).To(ContainElements(invalidDatabaseHostCause, invalidDatabaseNameCause, invalidUserCause, invalidSecretNameCause))
		})

		It("should reject the resource if databasePort is invalid", func() {
			resource := &appsv1alpha1.NemoEntitystore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: appsv1alpha1.NemoEntitystoreSpec{
					DatabaseConfig: &appsv1alpha1.DatabaseConfig{
						Host:         "test-pg-host",
						DatabaseName: "test-pg-database",
						Port:         65536,
						Credentials: appsv1alpha1.DatabaseCredentials{
							User:       "test-pg-user",
							SecretName: "test-pg-secret",
						},
					},
				},
			}
			By("creating the custom resource for the Kind NemoEntitystore")
			err := k8sClient.Create(ctx, resource)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsInvalid(err)).To(BeTrue())
			statusErr := err.(*errors.StatusError)
			Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(1))
			Expect(statusErr.ErrStatus.Details.Causes).To(ContainElement(invalidDatabasePortCause))
		})

		It("should reject the resource if databaseConfig credentials is missing required fields", func() {
			resource := &appsv1alpha1.NemoEntitystore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: appsv1alpha1.NemoEntitystoreSpec{
					DatabaseConfig: &appsv1alpha1.DatabaseConfig{
						Host:         "test-pg-host",
						DatabaseName: "test-pg-database",
						Credentials:  appsv1alpha1.DatabaseCredentials{},
					},
				},
			}
			By("creating the custom resource for the Kind NemoEntitystore")
			err := k8sClient.Create(ctx, resource)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsInvalid(err)).To(BeTrue())
			statusErr := err.(*errors.StatusError)
			Expect(statusErr.ErrStatus.Details.Causes).To(ContainElements(invalidSecretNameCause, invalidUserCause))
		})

		It("should reject the resource if databaseConfig credentials is invalid", func() {
			resource := &appsv1alpha1.NemoEntitystore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: appsv1alpha1.NemoEntitystoreSpec{
					DatabaseConfig: &appsv1alpha1.DatabaseConfig{
						Host:         "test-pg-host",
						DatabaseName: "test-pg-database",
						Credentials: appsv1alpha1.DatabaseCredentials{
							User:       "",
							SecretName: "",
						},
					},
				},
			}
			By("creating the custom resource for the Kind NemoEntitystore")
			err := k8sClient.Create(ctx, resource)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsInvalid(err)).To(BeTrue())
			statusErr := err.(*errors.StatusError)
			Expect(statusErr.ErrStatus.Details.Causes).To(ContainElements(invalidSecretNameCause, invalidUserCause))
		})

		It("should successfully reconcile the NemoEntityStore resource", func() {
			resource := &appsv1alpha1.NemoEntitystore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: appsv1alpha1.NemoEntitystoreSpec{
					Image: appsv1alpha1.Image{
						Repository: "test-repo",
						Tag:        "test-tag",
					},
					Replicas: 1,
					DatabaseConfig: &appsv1alpha1.DatabaseConfig{
						Host:         "test-pg-host",
						DatabaseName: "test-pg-database",
						Credentials: appsv1alpha1.DatabaseCredentials{
							User:       "test-pg-user",
							SecretName: "test-pg-secret",
						},
					},
				},
			}
			By("creating the custom resource for the Kind NemoEntitystore")
			err := client.Create(ctx, resource)
			Expect(err).ToNot(HaveOccurred())

			By("Reconciling the created resource")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).ToNot(HaveOccurred())

			err = client.Get(ctx, typeNamespacedName, nemoEntityStore)
			Expect(err).ToNot(HaveOccurred())
			Expect(nemoEntityStore.Finalizers).To(ContainElement(NemoEntitystoreFinalizer))
			Expect(nemoEntityStore.Status.State).To(Equal(appsv1alpha1.NemoEntitystoreStatusNotReady))

			// Deployment should exist.
			esDeploy := &appsv1.Deployment{}
			err = client.Get(ctx, typeNamespacedName, esDeploy)
			Expect(err).ToNot(HaveOccurred())

			By("deleting the custom resource")
			err = client.Delete(ctx, resource)
			Expect(err).ToNot(HaveOccurred())

			By("Reconciling the deleted resource")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).ToNot(HaveOccurred())

			// Nemo Entitystore CR should not exist.
			Eventually(func() bool {
				nemoEntityStore = &appsv1alpha1.NemoEntitystore{}
				err := client.Get(ctx, typeNamespacedName, nemoEntityStore)
				return err != nil && errors.IsNotFound(err)
			}, time.Second*5, time.Millisecond*500).Should(BeTrue())
		})
	})
})
