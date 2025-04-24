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
	"fmt"
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
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
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

		nemoEntitystore *appsv1alpha1.NemoEntitystore
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

		nemoEntitystore = &appsv1alpha1.NemoEntitystore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "full-nemoentitystore",
				Namespace: "default",
			},
			Spec: appsv1alpha1.NemoEntitystoreSpec{
				Labels:      map[string]string{"app": "nemo-entitystore"},
				Annotations: map[string]string{"annotation-key": "annotation-value"},
				Image:       appsv1alpha1.Image{Repository: "nvcr.io/nvidia/nemo-entitystore", PullPolicy: "IfNotPresent", Tag: "v0.1.0", PullSecrets: []string{"ngc-secret"}},
				Env: []corev1.EnvVar{
					{
						Name:  "custom-env",
						Value: "custom-value",
					},
				},
				DatabaseConfig: &appsv1alpha1.DatabaseConfig{
					Credentials: appsv1alpha1.DatabaseCredentials{
						User:        "esuser",
						SecretName:  "es-pg-existing-secret",
						PasswordKey: "password",
					},
					Host:         "es-pg.default.svc.cluster.local",
					Port:         5432,
					DatabaseName: "esdb",
				},
				Datastore: appsv1alpha1.Datastore{
					Endpoint: "http://nemo-datastore:8000",
				},
				Replicas: 1,
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
				NodeSelector: map[string]string{"gpu-type": "h100"},
				Tolerations: []corev1.Toleration{
					{
						Key:      "key1",
						Operator: corev1.TolerationOpEqual,
						Value:    "value1",
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
				Expose: appsv1alpha1.ExposeV1{
					Service: appsv1alpha1.Service{
						Type: corev1.ServiceTypeClusterIP,
						Port: ptr.To[int32](8000),
						Annotations: map[string]string{
							"annotation-key-specific": "service",
						},
					},
					Ingress: appsv1alpha1.IngressV1{
						Enabled:     ptr.To(true),
						Annotations: map[string]string{"annotation-key-specific": "ingress"},
						Spec: &appsv1alpha1.IngressSpec{
							IngressClassName: "nginx",
							Host:             "full-nemoentitystore.default.example.com",
							Paths: []appsv1alpha1.IngressPath{
								{
									Path:     "/",
									PathType: ptr.To(networkingv1.PathTypePrefix),
								},
							},
						},
					},
				},
				Scale: appsv1alpha1.Autoscaling{
					Enabled:     ptr.To(true),
					Annotations: map[string]string{"annotation-key-specific": "HPA"},
					HPA: appsv1alpha1.HorizontalPodAutoscalerSpec{
						MinReplicas: ptr.To[int32](1),
						MaxReplicas: 10,
						Metrics: []autoscalingv2.MetricSpec{
							{
								Type: autoscalingv2.ResourceMetricSourceType,
								Resource: &autoscalingv2.ResourceMetricSource{
									Target: autoscalingv2.MetricTarget{
										Type: autoscalingv2.UtilizationMetricType,
									},
								},
							},
							{
								Type: autoscalingv2.PodsMetricSourceType,
								Pods: &autoscalingv2.PodsMetricSource{
									Target: autoscalingv2.MetricTarget{
										Type: autoscalingv2.UtilizationMetricType,
									},
								},
							},
						},
					},
				},
				Metrics: appsv1alpha1.Metrics{
					Enabled: ptr.To(true),
					ServiceMonitor: appsv1alpha1.ServiceMonitor{
						Annotations:   map[string]string{"annotation-key-specific": "service-monitor"},
						Interval:      "1m",
						ScrapeTimeout: "30s",
					},
				},
			},
			Status: appsv1alpha1.NemoEntitystoreStatus{
				State: conditions.NotReady,
			},
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

		It("should create all resources", func() {
			namespacedName := types.NamespacedName{Name: nemoEntitystore.Name, Namespace: nemoEntitystore.Namespace}
			err := client.Create(context.TODO(), nemoEntitystore)
			Expect(err).NotTo(HaveOccurred())
			result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
			err = client.Get(ctx, namespacedName, nemoEntitystore)
			Expect(err).ToNot(HaveOccurred())
			Expect(nemoEntitystore.Finalizers).To(ContainElement(NemoEntitystoreFinalizer))
			// Role should be created
			role := &rbacv1.Role{}
			err = client.Get(context.TODO(), namespacedName, role)
			Expect(err).NotTo(HaveOccurred())
			Expect(role.Name).To(Equal(nemoEntitystore.GetName()))
			Expect(role.Namespace).To(Equal(nemoEntitystore.GetNamespace()))
			// RoleBinding should be created
			roleBinding := &rbacv1.RoleBinding{}
			err = client.Get(context.TODO(), namespacedName, roleBinding)
			Expect(err).NotTo(HaveOccurred())
			Expect(roleBinding.Name).To(Equal(nemoEntitystore.GetName()))
			Expect(roleBinding.Namespace).To(Equal(nemoEntitystore.GetNamespace()))
			// Service Account should be created
			serviceAccount := &corev1.ServiceAccount{}
			err = client.Get(context.TODO(), namespacedName, serviceAccount)
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceAccount.Name).To(Equal(nemoEntitystore.GetName()))
			Expect(serviceAccount.Namespace).To(Equal(nemoEntitystore.GetNamespace()))
			// Service should be created
			service := &corev1.Service{}
			err = client.Get(context.TODO(), namespacedName, service)
			Expect(err).NotTo(HaveOccurred())
			Expect(service.Name).To(Equal(nemoEntitystore.GetName()))
			Expect(string(service.Spec.Type)).To(Equal(nemoEntitystore.GetServiceType()))
			Expect(service.Namespace).To(Equal(nemoEntitystore.GetNamespace()))
			Expect(service.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(service.Annotations["annotation-key-specific"]).To(Equal("service"))
			// Ingress should be created
			ingress := &networkingv1.Ingress{}
			err = client.Get(context.TODO(), namespacedName, ingress)
			Expect(err).NotTo(HaveOccurred())
			Expect(ingress.Name).To(Equal(nemoEntitystore.GetName()))
			Expect(ingress.Namespace).To(Equal(nemoEntitystore.GetNamespace()))
			Expect(ingress.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(ingress.Annotations["annotation-key-specific"]).To(Equal("ingress"))
			Expect(service.Spec.Ports[0].Name).To(Equal("api"))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(8000)))
			// HPA should be deployed
			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			err = client.Get(context.TODO(), namespacedName, hpa)
			Expect(err).NotTo(HaveOccurred())
			Expect(hpa.Name).To(Equal(nemoEntitystore.GetName()))
			Expect(hpa.Namespace).To(Equal(nemoEntitystore.GetNamespace()))
			Expect(hpa.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(hpa.Annotations["annotation-key-specific"]).To(Equal("HPA"))
			Expect(*hpa.Spec.MinReplicas).To(Equal(int32(1)))
			Expect(hpa.Spec.MaxReplicas).To(Equal(int32(10)))
			// Service Monitor should be created
			sm := &monitoringv1.ServiceMonitor{}
			err = client.Get(context.TODO(), namespacedName, sm)
			Expect(err).NotTo(HaveOccurred())
			Expect(sm.Name).To(Equal(nemoEntitystore.GetName()))
			Expect(sm.Namespace).To(Equal(nemoEntitystore.GetNamespace()))
			Expect(sm.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(sm.Annotations["annotation-key-specific"]).To(Equal("service-monitor"))
			Expect(sm.Spec.Endpoints[0].Port).To(Equal("api"))
			Expect(sm.Spec.Endpoints[0].ScrapeTimeout).To(Equal(monitoringv1.Duration("30s")))
			Expect(sm.Spec.Endpoints[0].Interval).To(Equal(monitoringv1.Duration("1m")))
			// Deployment should be created
			deployment := &appsv1.Deployment{}
			err = client.Get(context.TODO(), namespacedName, deployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Name).To(Equal(nemoEntitystore.GetName()))
			Expect(deployment.Namespace).To(Equal(nemoEntitystore.GetNamespace()))
			Expect(deployment.Annotations["annotation-key"]).To(Equal("annotation-value"))

			connCmd := fmt.Sprintf(
				"until nc -z %s %d; do echo \"Waiting for Postgres to start \"; sleep 5; done;",
				nemoEntitystore.Spec.DatabaseConfig.Host,
				nemoEntitystore.Spec.DatabaseConfig.Port)

			Expect(deployment.Spec.Template.Spec.InitContainers).To(HaveLen(2))
			Expect(deployment.Spec.Template.Spec.InitContainers[0].Name).To(Equal("wait-for-postgres"))
			Expect(deployment.Spec.Template.Spec.InitContainers[0].Image).To(Equal("busybox"))
			Expect(deployment.Spec.Template.Spec.InitContainers[0].Command).To(Equal([]string{"sh", "-c", connCmd}))

			Expect(deployment.Spec.Template.Spec.InitContainers[1].Name).To(Equal("entitystore-db-migration"))
			Expect(deployment.Spec.Template.Spec.InitContainers[1].Image).To(Equal(nemoEntitystore.GetImage()))
			Expect(deployment.Spec.Template.Spec.InitContainers[1].Command).To(Equal([]string{"/app/.venv/bin/python3"}))
			Expect(deployment.Spec.Template.Spec.InitContainers[1].Args).To(Equal([]string{"-m", "scripts.run_db_migration"}))
			initContainerEnvVars := deployment.Spec.Template.Spec.InitContainers[1].Env
			Expect(initContainerEnvVars).To(ContainElements(
				corev1.EnvVar{Name: "POSTGRES_USER", Value: nemoEntitystore.Spec.DatabaseConfig.Credentials.User},
				corev1.EnvVar{Name: "POSTGRES_HOST", Value: nemoEntitystore.Spec.DatabaseConfig.Host},
				corev1.EnvVar{Name: "POSTGRES_PORT", Value: fmt.Sprintf("%d", nemoEntitystore.Spec.DatabaseConfig.Port)},
				corev1.EnvVar{Name: "POSTGRES_DB", Value: nemoEntitystore.Spec.DatabaseConfig.DatabaseName},
			))

			Expect(initContainerEnvVars).To(ContainElement(corev1.EnvVar{
				Name: "POSTGRES_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						Key: nemoEntitystore.Spec.DatabaseConfig.Credentials.PasswordKey,
						LocalObjectReference: corev1.LocalObjectReference{
							Name: nemoEntitystore.Spec.DatabaseConfig.Credentials.SecretName,
						},
					},
				},
			}))

			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Containers[0].Name).To(Equal(nemoEntitystore.GetContainerName()))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal(nemoEntitystore.GetImage()))
			// Ensure default probes are added
			Expect(deployment.Spec.Template.Spec.Containers[0].ReadinessProbe).NotTo(BeNil())
			Expect(deployment.Spec.Template.Spec.Containers[0].LivenessProbe).NotTo(BeNil())
			Expect(deployment.Spec.Template.Spec.Containers[0].StartupProbe).NotTo(BeNil())
			Expect(deployment.Spec.Template.Spec.NodeSelector).To(Equal(nemoEntitystore.Spec.NodeSelector))
			Expect(deployment.Spec.Template.Spec.Tolerations).To(Equal(nemoEntitystore.Spec.Tolerations))
			envVars := deployment.Spec.Template.Spec.Containers[0].Env
			// Verify entitystore environment variables
			Expect(envVars).To(ContainElement(
				corev1.EnvVar{Name: "custom-env", Value: "custom-value"},
			))

			Expect(envVars).To(ContainElements(
				corev1.EnvVar{Name: "APP_VERSION", Value: nemoEntitystore.Spec.Image.Tag},
				corev1.EnvVar{Name: "POSTGRES_USER", Value: nemoEntitystore.Spec.DatabaseConfig.Credentials.User},
				corev1.EnvVar{Name: "POSTGRES_HOST", Value: nemoEntitystore.Spec.DatabaseConfig.Host},
				corev1.EnvVar{Name: "POSTGRES_PORT", Value: fmt.Sprintf("%d", nemoEntitystore.Spec.DatabaseConfig.Port)},
				corev1.EnvVar{Name: "POSTGRES_DB", Value: nemoEntitystore.Spec.DatabaseConfig.DatabaseName},
			))

			Expect(envVars).To(ContainElement(corev1.EnvVar{
				Name: "POSTGRES_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						Key: nemoEntitystore.Spec.DatabaseConfig.Credentials.PasswordKey,
						LocalObjectReference: corev1.LocalObjectReference{
							Name: nemoEntitystore.Spec.DatabaseConfig.Credentials.SecretName,
						},
					},
				},
			}))
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
					Datastore: appsv1alpha1.Datastore{
						Endpoint: "http://nemo-datastore:8000",
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
