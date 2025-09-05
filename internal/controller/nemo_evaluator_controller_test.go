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
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	crClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	"github.com/NVIDIA/k8s-nim-operator/internal/render"
)

var _ = Describe("NemoEvaluator Controller", func() {
	var (
		client        crClient.Client
		reconciler    *NemoEvaluatorReconciler
		scheme        *runtime.Scheme
		nemoEvaluator *appsv1alpha1.NemoEvaluator
		ctx           context.Context
		secrets       *corev1.Secret
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
		Expect(gatewayv1.Install(scheme)).To(Succeed())

		client = fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&appsv1alpha1.NemoEvaluator{}).
			WithStatusSubresource(&appsv1.Deployment{}).
			Build()

		ctx = context.Background()
		minReplicas := int32(1)
		manifestsDir, err := filepath.Abs("../../manifests")
		Expect(err).ToNot(HaveOccurred())

		reconciler = &NemoEvaluatorReconciler{
			Client:   client,
			scheme:   scheme,
			updater:  conditions.NewUpdater(client),
			renderer: render.NewRenderer(manifestsDir),
			recorder: record.NewFakeRecorder(1000),
		}

		nemoEvaluator = &appsv1alpha1.NemoEvaluator{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-nemoevaluator",
				Namespace: "default",
			},
			Spec: appsv1alpha1.NemoEvaluatorSpec{
				Labels:      map[string]string{"app": "nemo-evaluator"},
				Annotations: map[string]string{"annotation-key": "annotation-value"},
				Image:       appsv1alpha1.Image{Repository: "nvcr.io/nvidia/nemo-evaluator", PullPolicy: "IfNotPresent", Tag: "v0.1.0", PullSecrets: []string{"ngc-secret"}},
				Env: []corev1.EnvVar{
					{
						Name:  "custom-env",
						Value: "custom-value",
					},
				},
				DatabaseConfig: &appsv1alpha1.DatabaseConfig{
					Credentials: appsv1alpha1.DatabaseCredentials{
						User:        "evaluser",
						SecretName:  "eval-pg-existing-secret",
						PasswordKey: "password",
					},
					Host:         "eval-pg.default.svc.cluster.local",
					Port:         5432,
					DatabaseName: "evaldb",
				},
				OpenTelemetry: appsv1alpha1.OTelSpec{
					Enabled:              ptr.To[bool](true),
					DisableLogging:       ptr.To[bool](false),
					ExporterOtlpEndpoint: "http://opentelemetry-collector.default.svc.cluster.local:4317",
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
				Expose: appsv1alpha1.Expose{
					Service: appsv1alpha1.Service{
						Type: corev1.ServiceTypeClusterIP,
						Port: ptr.To[int32](8000),
						Annotations: map[string]string{
							"annotation-key-specific": "service",
						},
					},
				},
				Router: appsv1alpha1.Router{
					IngressClass: ptr.To("nginx"),
					Annotations: map[string]string{
						"annotation-key-specific": "ingress",
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
				VectorDB:      appsv1alpha1.VectorDB{Endpoint: "http://milvus.default.svc.cluster.local:8000"},
				ArgoWorkflows: appsv1alpha1.ArgoWorkflows{Endpoint: "http://argo.default.svc.cluster.local:8000"},
				Datastore:     appsv1alpha1.Datastore{Endpoint: "http://nemo-datastore.default.svc.cluster.local:8000"},
				Entitystore:   appsv1alpha1.Entitystore{Endpoint: "http://nemo-entitystore.default.svc.cluster.local:8000"},
				EvaluationImages: appsv1alpha1.EvaluationImages{
					BigcodeEvalHarness: "BigcodeEvalHarness",
					LmEvalHarness:      "LmEvalHarness",
					SimilarityMetrics:  "SimilarityMetrics",
					LlmAsJudge:         "LlmAsJudge",
					MtBench:            "MtBench",
					Retriever:          "Retriever",
					Rag:                "Rag",
					BFCL:               "BFCL",
					AgenticEval:        "AgenticEval",
				},
			},
			Status: appsv1alpha1.NemoEvaluatorStatus{
				State: conditions.NotReady,
			},
		}

		encoded := base64.StdEncoding.EncodeToString([]byte("password-word"))

		secrets = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "eval-pg-existing-secret",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"password": []byte(encoded),
			},
		}

	})

	AfterEach(func() {
		// Cleanup the instance of NemoEvaluator
		namespacedName := types.NamespacedName{Name: nemoEvaluator.Name, Namespace: "default"}

		resource := &appsv1alpha1.NemoEvaluator{}
		err := k8sClient.Get(ctx, namespacedName, resource)
		if err == nil {
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		}

		namespacedName = types.NamespacedName{Name: "eval-pg-existing-secret", Namespace: "default"}
		secret := &corev1.Secret{}
		err = k8sClient.Get(ctx, namespacedName, secret)
		if err == nil {
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
		}
	})

	Describe("Reconcile", func() {
		It("should create all resources for the NemoEvaluator", func() {
			namespacedName := types.NamespacedName{Name: nemoEvaluator.Name, Namespace: "default"}
			err := client.Create(context.TODO(), nemoEvaluator)
			Expect(err).NotTo(HaveOccurred())
			err = client.Create(context.TODO(), secrets)
			Expect(err).NotTo(HaveOccurred())

			result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			err = client.Get(ctx, namespacedName, nemoEvaluator)
			Expect(err).ToNot(HaveOccurred())
			Expect(nemoEvaluator.Finalizers).To(ContainElement(NemoEvaluatorFinalizer))

			// Role should be created
			role := &rbacv1.Role{}
			err = client.Get(context.TODO(), namespacedName, role)
			Expect(err).NotTo(HaveOccurred())
			Expect(role.Name).To(Equal(nemoEvaluator.GetName()))
			Expect(role.Namespace).To(Equal(nemoEvaluator.GetNamespace()))

			// RoleBinding should be created
			roleBinding := &rbacv1.RoleBinding{}
			err = client.Get(context.TODO(), namespacedName, roleBinding)
			Expect(err).NotTo(HaveOccurred())
			Expect(roleBinding.Name).To(Equal(nemoEvaluator.GetName()))
			Expect(roleBinding.Namespace).To(Equal(nemoEvaluator.GetNamespace()))

			// Service Account should be created
			serviceAccount := &corev1.ServiceAccount{}
			err = client.Get(context.TODO(), namespacedName, serviceAccount)
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceAccount.Name).To(Equal(nemoEvaluator.GetName()))
			Expect(serviceAccount.Namespace).To(Equal(nemoEvaluator.GetNamespace()))

			// Service should be created
			service := &corev1.Service{}
			err = client.Get(context.TODO(), namespacedName, service)
			Expect(err).NotTo(HaveOccurred())
			Expect(service.Name).To(Equal(nemoEvaluator.GetName()))
			Expect(string(service.Spec.Type)).To(Equal(nemoEvaluator.GetServiceType()))
			Expect(service.Namespace).To(Equal(nemoEvaluator.GetNamespace()))
			Expect(service.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(service.Annotations["annotation-key-specific"]).To(Equal("service"))

			// Ingress should be created
			ingress := &networkingv1.Ingress{}
			err = client.Get(context.TODO(), namespacedName, ingress)
			Expect(err).NotTo(HaveOccurred())
			Expect(ingress.Name).To(Equal(nemoEvaluator.GetName()))
			Expect(ingress.Namespace).To(Equal(nemoEvaluator.GetNamespace()))
			Expect(ingress.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(ingress.Annotations["annotation-key-specific"]).To(Equal("ingress"))
			Expect(service.Spec.Ports[0].Name).To(Equal("api"))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(8000)))

			// HPA should be deployed
			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			err = client.Get(context.TODO(), namespacedName, hpa)
			Expect(err).NotTo(HaveOccurred())
			Expect(hpa.Name).To(Equal(nemoEvaluator.GetName()))
			Expect(hpa.Namespace).To(Equal(nemoEvaluator.GetNamespace()))
			Expect(hpa.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(hpa.Annotations["annotation-key-specific"]).To(Equal("HPA"))
			Expect(*hpa.Spec.MinReplicas).To(Equal(int32(1)))
			Expect(hpa.Spec.MaxReplicas).To(Equal(int32(10)))

			// Service Monitor should be created
			sm := &monitoringv1.ServiceMonitor{}
			err = client.Get(context.TODO(), namespacedName, sm)
			Expect(err).NotTo(HaveOccurred())
			Expect(sm.Name).To(Equal(nemoEvaluator.GetName()))
			Expect(sm.Namespace).To(Equal(nemoEvaluator.GetNamespace()))
			Expect(sm.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(sm.Annotations["annotation-key-specific"]).To(Equal("service-monitor"))
			Expect(sm.Spec.Endpoints[0].Port).To(Equal("api"))
			Expect(sm.Spec.Endpoints[0].ScrapeTimeout).To(Equal(monitoringv1.Duration("30s")))
			Expect(sm.Spec.Endpoints[0].Interval).To(Equal(monitoringv1.Duration("1m")))

			// Deployment should be created
			deployment := &appsv1.Deployment{}
			err = client.Get(context.TODO(), namespacedName, deployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Name).To(Equal(nemoEvaluator.GetName()))
			Expect(deployment.Namespace).To(Equal(nemoEvaluator.GetNamespace()))
			Expect(deployment.Annotations["annotation-key"]).To(Equal("annotation-value"))

			connCmd := fmt.Sprintf(
				"until nc -z %s %d; do echo \"Waiting for Postgres to start \"; sleep 5; done",
				nemoEvaluator.Spec.DatabaseConfig.Host,
				nemoEvaluator.Spec.DatabaseConfig.Port)

			Expect(deployment.Spec.Template.Spec.InitContainers).To(HaveLen(2))
			Expect(deployment.Spec.Template.Spec.InitContainers[0].Name).To(Equal("wait-for-postgres"))
			Expect(deployment.Spec.Template.Spec.InitContainers[0].Image).To(Equal("busybox"))
			Expect(deployment.Spec.Template.Spec.InitContainers[0].Command).To(Equal([]string{"sh", "-c", connCmd}))

			Expect(deployment.Spec.Template.Spec.InitContainers[1].Name).To(Equal("evaluator-db-migration"))
			Expect(deployment.Spec.Template.Spec.InitContainers[1].Image).To(Equal(nemoEvaluator.GetImage()))
			Expect(deployment.Spec.Template.Spec.InitContainers[1].Command).To(Equal([]string{"sh", "-c", "/app/scripts/run-db-migration.sh"}))
			initContainerEnvVars := deployment.Spec.Template.Spec.InitContainers[1].Env
			Expect(initContainerEnvVars).To(ContainElements(
				corev1.EnvVar{Name: "NAMESPACE", Value: nemoEvaluator.GetNamespace()},
				corev1.EnvVar{Name: "ARGO_HOST", Value: nemoEvaluator.Spec.ArgoWorkflows.Endpoint},
				corev1.EnvVar{Name: "EVAL_CONTAINER", Value: nemoEvaluator.GetImage()},
				corev1.EnvVar{Name: "DATA_STORE_URL", Value: nemoEvaluator.Spec.Datastore.Endpoint},
			))

			Expect(initContainerEnvVars).To(ContainElement(corev1.EnvVar{
				Name: "POSTGRES_DB_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						Key: nemoEvaluator.Spec.DatabaseConfig.Credentials.PasswordKey,
						LocalObjectReference: corev1.LocalObjectReference{
							Name: nemoEvaluator.Spec.DatabaseConfig.Credentials.SecretName,
						},
					},
				},
			}))

			Expect(initContainerEnvVars).To(ContainElement(corev1.EnvVar{
				Name: "POSTGRES_URI",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						Key: "uri",
						LocalObjectReference: corev1.LocalObjectReference{
							Name: nemoEvaluator.Name,
						},
					},
				},
			}))

			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Containers[0].Name).To(Equal(nemoEvaluator.GetContainerName()))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal(nemoEvaluator.GetImage()))
			// Ensure default probes are added
			Expect(deployment.Spec.Template.Spec.Containers[0].ReadinessProbe).NotTo(BeNil())
			Expect(deployment.Spec.Template.Spec.Containers[0].LivenessProbe).NotTo(BeNil())
			Expect(deployment.Spec.Template.Spec.Containers[0].StartupProbe).NotTo(BeNil())

			Expect(deployment.Spec.Template.Spec.NodeSelector).To(Equal(nemoEvaluator.Spec.NodeSelector))
			Expect(deployment.Spec.Template.Spec.Tolerations).To(Equal(nemoEvaluator.Spec.Tolerations))

			envVars := deployment.Spec.Template.Spec.Containers[0].Env

			// Verify evaluator environment variables
			Expect(envVars).To(ContainElements(
				corev1.EnvVar{Name: "EVALUATOR_HOST", Value: "0.0.0.0"},
				corev1.EnvVar{Name: "EVALUATOR_PORT", Value: fmt.Sprintf("%d", appsv1alpha1.EvaluatorAPIPort)},
				corev1.EnvVar{Name: "EVAL_CONTAINER", Value: nemoEvaluator.GetImage()},
				corev1.EnvVar{Name: "EVAL_ENABLE_VALIDATION", Value: "True"},
			))

			// Verify postgres password environment variable
			Expect(envVars).To(ContainElement(corev1.EnvVar{
				Name: "POSTGRES_DB_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						Key: nemoEvaluator.Spec.DatabaseConfig.Credentials.PasswordKey,
						LocalObjectReference: corev1.LocalObjectReference{
							Name: nemoEvaluator.Spec.DatabaseConfig.Credentials.SecretName,
						},
					},
				},
			}))

			Expect(envVars).To(ContainElement(corev1.EnvVar{
				Name: "POSTGRES_URI",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						Key: "uri",
						LocalObjectReference: corev1.LocalObjectReference{
							Name: nemoEvaluator.Name,
						},
					},
				},
			}))

			// Verify OTEL environment variables
			// Note envtest doesn't run admission controller to apply defaults, hence we cannot validate
			// fields that depend on defaults in the CRD
			Expect(envVars).To(ContainElements(
				corev1.EnvVar{Name: "OTEL_EXPORTER_OTLP_ENDPOINT", Value: nemoEvaluator.Spec.OpenTelemetry.ExporterOtlpEndpoint},
				corev1.EnvVar{Name: "OTEL_PYTHON_LOGGING_AUTO_INSTRUMENTATION_ENABLED", Value: "true"},
			))

			// Verify Datastore environment variables
			Expect(envVars).To(ContainElements(
				corev1.EnvVar{Name: "DATA_STORE_URL", Value: nemoEvaluator.Spec.Datastore.Endpoint},
			))

			Expect(envVars).To(ContainElements(
				corev1.EnvVar{Name: "ENTITY_STORE_URL", Value: nemoEvaluator.Spec.Entitystore.Endpoint},
			))

			// Verify Milvus environment variables
			Expect(envVars).To(ContainElements(
				corev1.EnvVar{Name: "MILVUS_URL", Value: nemoEvaluator.Spec.VectorDB.Endpoint},
			))

			// Verify Argo environment variables
			Expect(envVars).To(ContainElements(
				corev1.EnvVar{Name: "ARGO_HOST", Value: nemoEvaluator.Spec.ArgoWorkflows.Endpoint},
				corev1.EnvVar{Name: "SERVICE_ACCOUNT", Value: nemoEvaluator.Spec.ArgoWorkflows.ServiceAccount},
			))

			// Verify Evaluation Images environment variables
			Expect(envVars).To(ContainElements(
				corev1.EnvVar{Name: "BIGCODE_EVALUATION_HARNESS", Value: nemoEvaluator.Spec.EvaluationImages.BigcodeEvalHarness},
				corev1.EnvVar{Name: "LM_EVAL_HARNESS", Value: nemoEvaluator.Spec.EvaluationImages.LmEvalHarness},
				corev1.EnvVar{Name: "SIMILARITY_METRICS", Value: nemoEvaluator.Spec.EvaluationImages.SimilarityMetrics},
				corev1.EnvVar{Name: "LLM_AS_A_JUDGE", Value: nemoEvaluator.Spec.EvaluationImages.LlmAsJudge},
				corev1.EnvVar{Name: "MT_BENCH", Value: nemoEvaluator.Spec.EvaluationImages.MtBench},
				corev1.EnvVar{Name: "RETRIEVER", Value: nemoEvaluator.Spec.EvaluationImages.Retriever},
				corev1.EnvVar{Name: "RAG", Value: nemoEvaluator.Spec.EvaluationImages.Rag},
				corev1.EnvVar{Name: "BFCL", Value: nemoEvaluator.Spec.EvaluationImages.BFCL},
				corev1.EnvVar{Name: "AGENTIC_EVAL", Value: nemoEvaluator.Spec.EvaluationImages.AgenticEval},
			))
		})

		It("should delete HPA when NemoEvaluator is updated", func() {
			namespacedName := types.NamespacedName{Name: nemoEvaluator.Name, Namespace: "default"}
			err := client.Create(context.TODO(), nemoEvaluator)
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
			Expect(hpa.Name).To(Equal(nemoEvaluator.GetName()))
			Expect(hpa.Namespace).To(Equal(nemoEvaluator.GetNamespace()))
			Expect(*hpa.Spec.MinReplicas).To(Equal(int32(1)))
			Expect(hpa.Spec.MaxReplicas).To(Equal(int32(10)))

			nemoEvaluator := &appsv1alpha1.NemoEvaluator{}
			err = client.Get(context.TODO(), namespacedName, nemoEvaluator)
			Expect(err).NotTo(HaveOccurred())
			nemoEvaluator.Spec.Scale.Enabled = ptr.To[bool](false)
			nemoEvaluator.Spec.Router.IngressClass = nil
			err = client.Update(context.TODO(), nemoEvaluator)
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
