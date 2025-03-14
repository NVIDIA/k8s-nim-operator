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
	"encoding/base64"
	"fmt"
	"path/filepath"
	"sort"

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	crClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	"github.com/NVIDIA/k8s-nim-operator/internal/render"
)

func sortVolumeMounts(volumeMounts []corev1.VolumeMount) {
	sort.SliceStable(volumeMounts, func(i, j int) bool {
		return volumeMounts[i].Name < volumeMounts[j].Name
	})
}

func sortVolumes(volumes []corev1.Volume) {
	sort.SliceStable(volumes, func(i, j int) bool {
		return volumes[i].Name < volumes[j].Name
	})
}

var _ = Describe("NemoCustomizer Controller", func() {
	var (
		client         crClient.Client
		reconciler     *NemoCustomizerReconciler
		scheme         *runtime.Scheme
		nemoCustomizer *appsv1alpha1.NemoCustomizer
		volumeMounts   []corev1.VolumeMount
		volumes        []corev1.Volume
		ctx            context.Context
		secrets        *corev1.Secret
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(appsv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())
		Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
		Expect(autoscalingv2.AddToScheme(scheme)).To(Succeed())
		Expect(networkingv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(monitoringv1.AddToScheme(scheme)).To(Succeed())
		Expect(batchv1.AddToScheme(scheme)).To(Succeed())

		client = fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&appsv1alpha1.NemoCustomizer{}).
			WithStatusSubresource(&appsv1.Deployment{}).
			Build()

		ctx = context.Background()
		minReplicas := int32(1)
		manifestsDir, err := filepath.Abs("../../manifests")
		Expect(err).ToNot(HaveOccurred())

		reconciler = &NemoCustomizerReconciler{
			Client:   client,
			scheme:   scheme,
			updater:  conditions.NewUpdater(client),
			renderer: render.NewRenderer(manifestsDir),
			recorder: record.NewFakeRecorder(1000),
		}

		nemoCustomizer = &appsv1alpha1.NemoCustomizer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-nemocustomizer",
				Namespace: "default",
			},
			Spec: appsv1alpha1.NemoCustomizerSpec{
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
				ReadinessProbe: appsv1alpha1.Probe{
					Enabled: ptr.To[bool](true),
					Probe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/v1/health/ready",
								Port: intstr.IntOrString{IntVal: 8000},
							},
						},
						InitialDelaySeconds: 15,
						PeriodSeconds:       10,
						TimeoutSeconds:      10,
						FailureThreshold:    5,
					},
				},
				LivenessProbe: appsv1alpha1.Probe{
					Enabled: ptr.To[bool](true),
					Probe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/v1/health/live",
								Port: intstr.IntOrString{StrVal: "api"},
							},
						},
						InitialDelaySeconds: 15,
						PeriodSeconds:       10,
						TimeoutSeconds:      10,
						FailureThreshold:    5,
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
				Expose: appsv1alpha1.Expose{
					Service: appsv1alpha1.Service{
						Type: corev1.ServiceTypeClusterIP,
						Port: 8000,
						Annotations: map[string]string{
							"annotation-key-specific": "service",
						},
					},
					Ingress: appsv1alpha1.Ingress{
						Enabled:     ptr.To[bool](true),
						Annotations: map[string]string{"annotation-key-specific": "ingress"},
						Spec: networkingv1.IngressSpec{
							Rules: []networkingv1.IngressRule{
								{
									Host: "test-nemocustomizer.default.example.com",
									IngressRuleValue: networkingv1.IngressRuleValue{
										HTTP: &networkingv1.HTTPIngressRuleValue{
											Paths: []networkingv1.HTTPIngressPath{
												{
													Path: "/",
													Backend: networkingv1.IngressBackend{
														Service: &networkingv1.IngressServiceBackend{
															Name: "test-nemocustomizer",
															Port: networkingv1.ServiceBackendPort{
																Number: 8000,
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				Scale: appsv1alpha1.Autoscaling{
					Enabled:     ptr.To[bool](true),
					Annotations: map[string]string{"annotation-key-specific": "HPA"},
					HPA: appsv1alpha1.HorizontalPodAutoscalerSpec{
						MinReplicas: &minReplicas,
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
					Enabled: ptr.To[bool](true),
					ServiceMonitor: appsv1alpha1.ServiceMonitor{
						Annotations:   map[string]string{"annotation-key-specific": "service-monitor"},
						Interval:      "1m",
						ScrapeTimeout: "30s",
					},
				},
			},
			Status: appsv1alpha1.NemoCustomizerStatus{
				State: conditions.NotReady,
			},
		}

		volumes = []corev1.Volume{
			{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: nemoCustomizer.GetConfigName(),
						},
						Items: []corev1.KeyToPath{
							{
								Key:  "config.yaml",
								Path: "config.yaml",
							},
						},
					},
				},
			},
		}

		volumeMounts = []corev1.VolumeMount{
			{
				Name:      "config",
				MountPath: "/app/config",
				ReadOnly:  true,
			},
		}

		encoded := base64.StdEncoding.EncodeToString([]byte("password-word"))

		secrets = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ncs-pg-existing-secret",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"password": []byte(encoded),
			},
		}
	})

	AfterEach(func() {
		// Cleanup the instance of NemoCustomizer
		namespacedName := types.NamespacedName{Name: nemoCustomizer.Name, Namespace: "default"}

		resource := &appsv1alpha1.NemoCustomizer{}
		err := k8sClient.Get(ctx, namespacedName, resource)
		if err == nil {
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		}

		namespacedName = types.NamespacedName{Name: "ncs-pg-existing-secret", Namespace: "default"}
		secret := &corev1.Secret{}
		err = k8sClient.Get(ctx, namespacedName, secret)
		if err == nil {
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
		}
	})

	Describe("Reconcile", func() {
		It("should create all resources for the NemoCustomizer", func() {
			namespacedName := types.NamespacedName{Name: nemoCustomizer.Name, Namespace: "default"}
			err := client.Create(context.TODO(), nemoCustomizer)
			Expect(err).NotTo(HaveOccurred())
			err = client.Create(context.TODO(), secrets)
			Expect(err).NotTo(HaveOccurred())

			result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			err = client.Get(ctx, namespacedName, nemoCustomizer)
			Expect(err).ToNot(HaveOccurred())
			Expect(nemoCustomizer.Finalizers).To(ContainElement(NemoCustomizerFinalizer))

			// Role should be created
			role := &rbacv1.Role{}
			err = client.Get(context.TODO(), namespacedName, role)
			Expect(err).NotTo(HaveOccurred())
			Expect(role.Name).To(Equal(nemoCustomizer.GetName()))
			Expect(role.Namespace).To(Equal(nemoCustomizer.GetNamespace()))

			// RoleBinding should be created
			roleBinding := &rbacv1.RoleBinding{}
			err = client.Get(context.TODO(), namespacedName, roleBinding)
			Expect(err).NotTo(HaveOccurred())
			Expect(roleBinding.Name).To(Equal(nemoCustomizer.GetName()))
			Expect(roleBinding.Namespace).To(Equal(nemoCustomizer.GetNamespace()))

			// Service Account should be created
			serviceAccount := &corev1.ServiceAccount{}
			err = client.Get(context.TODO(), namespacedName, serviceAccount)
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceAccount.Name).To(Equal(nemoCustomizer.GetName()))
			Expect(serviceAccount.Namespace).To(Equal(nemoCustomizer.GetNamespace()))

			// Service should be created
			service := &corev1.Service{}
			err = client.Get(context.TODO(), namespacedName, service)
			Expect(err).NotTo(HaveOccurred())
			Expect(service.Name).To(Equal(nemoCustomizer.GetName()))
			Expect(string(service.Spec.Type)).To(Equal(nemoCustomizer.GetServiceType()))
			Expect(service.Namespace).To(Equal(nemoCustomizer.GetNamespace()))
			Expect(service.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(service.Annotations["annotation-key-specific"]).To(Equal("service"))

			// Ingress should be created
			ingress := &networkingv1.Ingress{}
			err = client.Get(context.TODO(), namespacedName, ingress)
			Expect(err).NotTo(HaveOccurred())
			Expect(ingress.Name).To(Equal(nemoCustomizer.GetName()))
			Expect(ingress.Namespace).To(Equal(nemoCustomizer.GetNamespace()))
			Expect(ingress.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(ingress.Annotations["annotation-key-specific"]).To(Equal("ingress"))
			Expect(service.Spec.Ports[0].Name).To(Equal("api"))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(8000)))

			// HPA should be deployed
			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			err = client.Get(context.TODO(), namespacedName, hpa)
			Expect(err).NotTo(HaveOccurred())
			Expect(hpa.Name).To(Equal(nemoCustomizer.GetName()))
			Expect(hpa.Namespace).To(Equal(nemoCustomizer.GetNamespace()))
			Expect(hpa.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(hpa.Annotations["annotation-key-specific"]).To(Equal("HPA"))
			Expect(*hpa.Spec.MinReplicas).To(Equal(int32(1)))
			Expect(hpa.Spec.MaxReplicas).To(Equal(int32(10)))

			// Service Monitor should be created
			sm := &monitoringv1.ServiceMonitor{}
			err = client.Get(context.TODO(), namespacedName, sm)
			Expect(err).NotTo(HaveOccurred())
			Expect(sm.Name).To(Equal(nemoCustomizer.GetName()))
			Expect(sm.Namespace).To(Equal(nemoCustomizer.GetNamespace()))
			Expect(sm.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(sm.Annotations["annotation-key-specific"]).To(Equal("service-monitor"))
			Expect(sm.Spec.Endpoints[0].Port).To(Equal("api"))
			Expect(sm.Spec.Endpoints[0].ScrapeTimeout).To(Equal(monitoringv1.Duration("30s")))
			Expect(sm.Spec.Endpoints[0].Interval).To(Equal(monitoringv1.Duration("1m")))

			// Deployment should be created
			deployment := &appsv1.Deployment{}
			err = client.Get(context.TODO(), namespacedName, deployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Name).To(Equal(nemoCustomizer.GetName()))
			Expect(deployment.Namespace).To(Equal(nemoCustomizer.GetNamespace()))
			Expect(deployment.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(deployment.Spec.Template.Spec.Containers[0].Name).To(Equal(nemoCustomizer.GetContainerName()))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal(nemoCustomizer.GetImage()))
			// Ensure customized liveness and readiness probes are added
			Expect(deployment.Spec.Template.Spec.Containers[0].ReadinessProbe).To(Equal(nemoCustomizer.Spec.ReadinessProbe.Probe))
			Expect(deployment.Spec.Template.Spec.Containers[0].LivenessProbe).To(Equal(nemoCustomizer.Spec.LivenessProbe.Probe))
			// Ensure default startup probe is added
			Expect(deployment.Spec.Template.Spec.Containers[0].StartupProbe).NotTo(BeNil())

			sortVolumes(deployment.Spec.Template.Spec.Volumes)
			sortVolumes(volumes)
			Expect(deployment.Spec.Template.Spec.Volumes).To(Equal(volumes))

			sortVolumeMounts(deployment.Spec.Template.Spec.Containers[0].VolumeMounts)
			sortVolumeMounts(volumeMounts)
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).To(Equal(volumeMounts))

			Expect(deployment.Spec.Template.Spec.NodeSelector).To(Equal(nemoCustomizer.Spec.NodeSelector))
			Expect(deployment.Spec.Template.Spec.Tolerations).To(Equal(nemoCustomizer.Spec.Tolerations))

			envVars := deployment.Spec.Template.Spec.Containers[0].Env

			// Verify standard environment variables
			Expect(envVars).To(ContainElements(
				corev1.EnvVar{Name: "NAMESPACE", Value: nemoCustomizer.Namespace},
				corev1.EnvVar{Name: "CONFIG_PATH", Value: "/app/config/config.yaml"},
				corev1.EnvVar{Name: "CUSTOMIZATIONS_CALLBACK_URL", Value: fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", nemoCustomizer.Name, nemoCustomizer.Namespace, appsv1alpha1.CustomizerInternalPort)},
				corev1.EnvVar{Name: "LOG_LEVEL", Value: "INFO"},
			))

			// Verify WandB environment variables
			Expect(envVars).To(ContainElements(
				corev1.EnvVar{Name: "WANDB_SECRET_NAME", Value: nemoCustomizer.Spec.WandBSecret.Name},
				corev1.EnvVar{Name: "WANDB_SECRET_KEY", Value: nemoCustomizer.Spec.WandBSecret.APIKeyKey},
			))

			Expect(envVars).To(ContainElement(corev1.EnvVar{
				Name: "WANDB_ENCRYPTION_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						Key: nemoCustomizer.Spec.WandBSecret.APIKeyKey,
						LocalObjectReference: corev1.LocalObjectReference{
							Name: nemoCustomizer.Spec.WandBSecret.Name,
						},
					},
				},
			}))

			// Verify postgres environment variables
			Expect(envVars).To(ContainElement(corev1.EnvVar{
				Name: "POSTGRES_DB_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						Key: nemoCustomizer.Spec.DatabaseConfig.Credentials.PasswordKey,
						LocalObjectReference: corev1.LocalObjectReference{
							Name: nemoCustomizer.Spec.DatabaseConfig.Credentials.SecretName,
						},
					},
				},
			}))

			Expect(envVars).To(ContainElement(corev1.EnvVar{
				Name: "POSTGRES_DB_DSN",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						Key: "dsn",
						LocalObjectReference: corev1.LocalObjectReference{
							Name: nemoCustomizer.Name,
						},
					},
				},
			}))

			// Verify OTEL environment variables
			Expect(envVars).To(ContainElements(
				corev1.EnvVar{Name: "OTEL_EXPORTER_OTLP_ENDPOINT", Value: nemoCustomizer.Spec.OpenTelemetry.ExporterOtlpEndpoint},
				corev1.EnvVar{Name: "OTEL_PYTHON_LOGGING_AUTO_INSTRUMENTATION_ENABLED", Value: "true"},
			))
		})

		It("should delete HPA when NemoCustomizer is updated", func() {
			namespacedName := types.NamespacedName{Name: nemoCustomizer.Name, Namespace: "default"}
			err := client.Create(context.TODO(), nemoCustomizer)
			Expect(err).NotTo(HaveOccurred())
			err = client.Create(context.TODO(), secrets)
			Expect(err).NotTo(HaveOccurred())

			result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// HPA should be deployed
			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			err = client.Get(context.TODO(), namespacedName, hpa)
			Expect(err).NotTo(HaveOccurred())
			Expect(hpa.Name).To(Equal(nemoCustomizer.GetName()))
			Expect(hpa.Namespace).To(Equal(nemoCustomizer.GetNamespace()))
			Expect(*hpa.Spec.MinReplicas).To(Equal(int32(1)))
			Expect(hpa.Spec.MaxReplicas).To(Equal(int32(10)))

			nemoCustomizer := &appsv1alpha1.NemoCustomizer{}
			err = client.Get(context.TODO(), namespacedName, nemoCustomizer)
			Expect(err).NotTo(HaveOccurred())
			nemoCustomizer.Spec.Scale.Enabled = ptr.To[bool](false)
			nemoCustomizer.Spec.Expose.Ingress.Enabled = ptr.To[bool](false)
			err = client.Update(context.TODO(), nemoCustomizer)
			Expect(err).NotTo(HaveOccurred())

			result, err = reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
			hpa = &autoscalingv2.HorizontalPodAutoscaler{}
			err = client.Get(context.TODO(), namespacedName, hpa)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsNotFound(err)).To(Equal(true))
			ingress := &networkingv1.Ingress{}
			err = client.Get(context.TODO(), namespacedName, ingress)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsNotFound(err)).To(Equal(true))
		})
	})
})
