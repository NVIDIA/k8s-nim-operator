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

package standalone

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"path"
	"sort"
	"strings"

	"os"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	"github.com/NVIDIA/k8s-nim-operator/internal/render"
	rendertypes "github.com/NVIDIA/k8s-nim-operator/internal/render/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	apiResource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func sortEnvVars(envVars []corev1.EnvVar) {
	sort.SliceStable(envVars, func(i, j int) bool {
		return envVars[i].Name < envVars[j].Name
	})
}

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

var _ = Describe("NIMServiceReconciler for a standalone platform", func() {
	var (
		client       client.Client
		reconciler   *NIMServiceReconciler
		scheme       *runtime.Scheme
		nimService   *appsv1alpha1.NIMService
		nimCache     *appsv1alpha1.NIMCache
		volumeMounts []corev1.VolumeMount
		volumes      []corev1.Volume
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

		client = fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&appsv1alpha1.NIMService{}).
			WithStatusSubresource(&appsv1alpha1.NIMCache{}).
			Build()
		boolTrue := true
		cwd, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		reconciler = &NIMServiceReconciler{
			Client:   client,
			scheme:   scheme,
			updater:  conditions.NewUpdater(client),
			renderer: render.NewRenderer(path.Join(strings.TrimSuffix(cwd, "internal/controller/platform/standalone"), "manifests")),
		}
		pvcName := "test-pvc"
		minReplicas := int32(1)
		nimService = &appsv1alpha1.NIMService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-nimservice",
				Namespace: "default",
			},
			Spec: appsv1alpha1.NIMServiceSpec{
				Labels:      map[string]string{"app": "test-app"},
				Annotations: map[string]string{"annotation-key": "annotation-value"},
				Image:       appsv1alpha1.Image{Repository: "nvcr.io/nvidia/nim-llm", PullPolicy: "IfNotPresent", Tag: "v0.1.0", PullSecrets: []string{"ngc-secret"}},
				Storage: appsv1alpha1.NIMServiceStorage{
					PVC: appsv1alpha1.PersistentVolumeClaim{
						Name: pvcName,
					},
					NIMCache: appsv1alpha1.NIMCacheVolSpec{
						Name: "test-nimcache",
					},
				},
				Env: []corev1.EnvVar{
					{
						Name:  "custom-env",
						Value: "custom-value",
					},
					{
						Name:  "NIM_CACHE_PATH",
						Value: "/model-store",
					},
					{
						Name: "NGC_API_KEY",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "ngc-api-secret",
								},
								Key: "NGC_API_KEY",
							},
						},
					},
					{
						Name:  "OUTLINES_CACHE_DIR",
						Value: "/tmp/outlines",
					},
					{
						Name:  "NIM_SERVER_PORT",
						Value: "9000",
					},
					{
						Name:  "NIM_JSONL_LOGGING",
						Value: "1",
					},
					{
						Name:  "NIM_LOG_LEVEL",
						Value: "info",
					},
				},
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
				},
				NodeSelector: map[string]string{"disktype": "ssd"},
				Tolerations: []corev1.Toleration{
					{
						Key:      "key1",
						Operator: corev1.TolerationOpEqual,
						Value:    "value1",
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
				Expose: appsv1alpha1.Expose{
					Service: appsv1alpha1.Service{Type: corev1.ServiceTypeClusterIP, Port: 8123, Annotations: map[string]string{"annotation-key-specific": "service"}},
					Ingress: appsv1alpha1.Ingress{
						Enabled:     ptr.To[bool](true),
						Annotations: map[string]string{"annotation-key-specific": "ingress"},
						Spec: networkingv1.IngressSpec{
							Rules: []networkingv1.IngressRule{
								{
									Host: "test-nimservice.default.example.com",
									IngressRuleValue: networkingv1.IngressRuleValue{
										HTTP: &networkingv1.HTTPIngressRuleValue{
											Paths: []networkingv1.HTTPIngressPath{
												{
													Path: "/",
													Backend: networkingv1.IngressBackend{
														Service: &networkingv1.IngressServiceBackend{
															Name: "test-nimservice",
															Port: networkingv1.ServiceBackendPort{
																Number: 8080,
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
					Enabled: &boolTrue,
					ServiceMonitor: appsv1alpha1.ServiceMonitor{
						Annotations:   map[string]string{"annotation-key-specific": "service-monitor"},
						Interval:      "1m",
						ScrapeTimeout: "30s",
					},
				},
				ReadinessProbe: appsv1alpha1.Probe{
					Enabled: &boolTrue,
					Probe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/ready",
								Port: intstr.IntOrString{IntVal: 8000},
							},
						},
					},
				},
				LivenessProbe: appsv1alpha1.Probe{
					Enabled: &boolTrue,
					Probe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/live",
								Port: intstr.IntOrString{IntVal: 8000},
							},
						},
					},
				},
				StartupProbe: appsv1alpha1.Probe{
					Enabled: &boolTrue,
					Probe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/start",
								Port: intstr.IntOrString{IntVal: 8000},
							},
						},
					},
				},
			},
			Status: appsv1alpha1.NIMServiceStatus{
				State: conditions.NotReady,
			},
		}

		volumes = []corev1.Volume{
			{
				Name: "dshm",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{
						Medium: corev1.StorageMediumMemory,
					},
				},
			},
			{
				Name: "model-store",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "test-pvc",
						ReadOnly:  false,
					},
				},
			},
		}

		volumeMounts = []corev1.VolumeMount{
			{
				Name:      "model-store",
				MountPath: "/model-store",
				SubPath:   "subPath",
			},
			{
				Name:      "dshm",
				MountPath: "/dev/shm",
			},
		}
		nimCache = &appsv1alpha1.NIMCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-nimcache",
				Namespace: "default",
			},
			Spec: appsv1alpha1.NIMCacheSpec{
				Source:  appsv1alpha1.NIMSource{NGC: &appsv1alpha1.NGCSource{ModelPuller: "test-container", PullSecret: "my-secret"}},
				Storage: appsv1alpha1.NIMCacheStorage{PVC: appsv1alpha1.PersistentVolumeClaim{Create: ptr.To[bool](true), StorageClass: "standard", Size: "1Gi", SubPath: "subPath"}},
			},
			Status: appsv1alpha1.NIMCacheStatus{
				State: appsv1alpha1.NimCacheStatusReady,
				PVC:   pvcName,
				Profiles: []appsv1alpha1.NIMProfile{{
					Name:   "test-profile",
					Config: map[string]string{"tp": "4"}},
				},
			},
		}
		_ = client.Create(context.TODO(), nimCache)
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: "default",
			},
		}
		_ = client.Create(context.TODO(), pvc)

		var buf bytes.Buffer
		log.SetOutput(&buf)
		defer func() {
			log.SetOutput(os.Stderr)
		}()

	})

	AfterEach(func() {
		// Clean up the NIMService instance
		nimService := &appsv1alpha1.NIMService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-nimservice",
				Namespace: "default",
			},
		}
		_ = client.Delete(context.TODO(), nimService)

		// Ensure that nimCache status is ready before each test
		nimCache.Status = appsv1alpha1.NIMCacheStatus{
			State: appsv1alpha1.NimCacheStatusReady,
		}

		// Update nimCache status
		Expect(client.Status().Update(context.TODO(), nimCache)).To(Succeed())
	})

	Describe("Reconcile", func() {
		It("should create all resources for the NIMService", func() {
			namespacedName := types.NamespacedName{Name: nimService.Name, Namespace: nimService.Namespace}
			err := client.Create(context.TODO(), nimService)
			Expect(err).NotTo(HaveOccurred())

			result, err := reconciler.reconcileNIMService(context.TODO(), nimService)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Role should be created
			role := &rbacv1.Role{}
			err = client.Get(context.TODO(), namespacedName, role)
			Expect(err).NotTo(HaveOccurred())
			Expect(role.Name).To(Equal(nimService.GetName()))
			Expect(role.Namespace).To(Equal(nimService.GetNamespace()))

			// RoleBinding should be created
			roleBinding := &rbacv1.RoleBinding{}
			err = client.Get(context.TODO(), namespacedName, roleBinding)
			Expect(err).NotTo(HaveOccurred())
			Expect(roleBinding.Name).To(Equal(nimService.GetName()))
			Expect(roleBinding.Namespace).To(Equal(nimService.GetNamespace()))

			// Service Account should be created
			serviceAccount := &corev1.ServiceAccount{}
			err = client.Get(context.TODO(), namespacedName, serviceAccount)
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceAccount.Name).To(Equal(nimService.GetName()))
			Expect(serviceAccount.Namespace).To(Equal(nimService.GetNamespace()))

			// Service should be created
			service := &corev1.Service{}
			err = client.Get(context.TODO(), namespacedName, service)
			Expect(err).NotTo(HaveOccurred())
			Expect(service.Name).To(Equal(nimService.GetName()))
			Expect(service.Namespace).To(Equal(nimService.GetNamespace()))
			Expect(service.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(service.Annotations["annotation-key-specific"]).To(Equal("service"))

			// Ingress should be created
			ingress := &networkingv1.Ingress{}
			err = client.Get(context.TODO(), namespacedName, ingress)
			Expect(err).NotTo(HaveOccurred())
			Expect(ingress.Name).To(Equal(nimService.GetName()))
			Expect(ingress.Namespace).To(Equal(nimService.GetNamespace()))
			Expect(ingress.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(ingress.Annotations["annotation-key-specific"]).To(Equal("ingress"))
			Expect(service.Spec.Ports[0].Name).To(Equal("service-port"))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(8123)))

			// HPA should be deployed
			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			err = client.Get(context.TODO(), namespacedName, hpa)
			Expect(err).NotTo(HaveOccurred())
			Expect(hpa.Name).To(Equal(nimService.GetName()))
			Expect(hpa.Namespace).To(Equal(nimService.GetNamespace()))
			Expect(hpa.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(hpa.Annotations["annotation-key-specific"]).To(Equal("HPA"))
			Expect(*hpa.Spec.MinReplicas).To(Equal(int32(1)))
			Expect(hpa.Spec.MaxReplicas).To(Equal(int32(10)))

			// Service Monitor should be created
			sm := &monitoringv1.ServiceMonitor{}
			err = client.Get(context.TODO(), namespacedName, sm)
			Expect(err).NotTo(HaveOccurred())
			Expect(sm.Name).To(Equal(nimService.GetName()))
			Expect(sm.Namespace).To(Equal(nimService.GetNamespace()))
			Expect(sm.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(sm.Annotations["annotation-key-specific"]).To(Equal("service-monitor"))
			Expect(sm.Spec.Endpoints[0].Port).To(Equal("service-port"))
			Expect(sm.Spec.Endpoints[0].ScrapeTimeout).To(Equal(monitoringv1.Duration("30s")))
			Expect(sm.Spec.Endpoints[0].Interval).To(Equal(monitoringv1.Duration("1m")))

			// Deployment should be created
			deployment := &appsv1.Deployment{}
			err = client.Get(context.TODO(), namespacedName, deployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Name).To(Equal(nimService.GetName()))
			Expect(deployment.Namespace).To(Equal(nimService.GetNamespace()))
			Expect(deployment.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(deployment.Spec.Template.Spec.Containers[0].Name).To(Equal(nimService.GetContainerName()))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal(nimService.GetImage()))
			Expect(deployment.Spec.Template.Spec.Containers[0].ReadinessProbe).To(Equal(nimService.Spec.ReadinessProbe.Probe))
			Expect(deployment.Spec.Template.Spec.Containers[0].LivenessProbe).To(Equal(nimService.Spec.LivenessProbe.Probe))
			Expect(deployment.Spec.Template.Spec.Containers[0].StartupProbe).To(Equal(nimService.Spec.StartupProbe.Probe))

			sortEnvVars(deployment.Spec.Template.Spec.Containers[0].Env)
			sortEnvVars(nimService.Spec.Env)
			Expect(deployment.Spec.Template.Spec.Containers[0].Env).To(Equal(nimService.Spec.Env))

			sortVolumes(deployment.Spec.Template.Spec.Volumes)
			sortVolumes(volumes)
			Expect(deployment.Spec.Template.Spec.Volumes).To(Equal(volumes))

			sortVolumeMounts(deployment.Spec.Template.Spec.Containers[0].VolumeMounts)
			sortVolumeMounts(volumeMounts)
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).To(Equal(volumeMounts))

			Expect(deployment.Spec.Template.Spec.NodeSelector).To(Equal(nimService.Spec.NodeSelector))
			Expect(deployment.Spec.Template.Spec.Tolerations).To(Equal(nimService.Spec.Tolerations))
		})

		It("should delete Deployment when the NIMService is deleted", func() {
			nimServiceKey := types.NamespacedName{Name: nimService.Name, Namespace: nimService.Namespace}
			err := client.Create(context.TODO(), nimService)
			Expect(err).NotTo(HaveOccurred())

			_, err = reconciler.reconcileNIMService(context.TODO(), nimService)
			Expect(err).NotTo(HaveOccurred())

			deployment := &appsv1.Deployment{}
			err = client.Get(context.TODO(), nimServiceKey, deployment)
			Expect(err).NotTo(HaveOccurred())

			err = client.Delete(context.TODO(), nimService)
			Expect(err).NotTo(HaveOccurred())

			// Simulate the finalizer logic
			err = reconciler.cleanupNIMService(context.TODO(), nimService)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should delete HPA when NIMService is updated", func() {
			namespacedName := types.NamespacedName{Name: nimService.Name, Namespace: nimService.Namespace}
			err := client.Create(context.TODO(), nimService)
			Expect(err).NotTo(HaveOccurred())

			result, err := reconciler.reconcileNIMService(context.TODO(), nimService)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// HPA should be deployed
			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			err = client.Get(context.TODO(), namespacedName, hpa)
			Expect(err).NotTo(HaveOccurred())
			Expect(hpa.Name).To(Equal(nimService.GetName()))
			Expect(hpa.Namespace).To(Equal(nimService.GetNamespace()))
			Expect(*hpa.Spec.MinReplicas).To(Equal(int32(1)))
			Expect(hpa.Spec.MaxReplicas).To(Equal(int32(10)))

			nimService := &appsv1alpha1.NIMService{}
			err = client.Get(context.TODO(), namespacedName, nimService)
			Expect(err).NotTo(HaveOccurred())
			nimService.Spec.Scale.Enabled = ptr.To[bool](false)
			nimService.Spec.Expose.Ingress.Enabled = ptr.To[bool](false)
			err = client.Update(context.TODO(), nimService)
			Expect(err).NotTo(HaveOccurred())

			result, err = reconciler.reconcileNIMService(context.TODO(), nimService)
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

	Describe("isDeploymentReady for setting status on NIMService", func() {

		AfterEach(func() {
			// Clean up the Deployment instance
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimservice",
					Namespace: "default",
				},
			}
			_ = client.Delete(context.TODO(), deployment)
		})
		It("Deployment exceeded in its progress", func() {
			nimServiceKey := types.NamespacedName{Name: nimService.Name, Namespace: nimService.Namespace}
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimservice",
					Namespace: "default",
				},
				Status: appsv1.DeploymentStatus{
					Conditions: []appsv1.DeploymentCondition{
						{
							Type:   appsv1.DeploymentProgressing,
							Reason: "ProgressDeadlineExceeded",
						},
					},
				},
			}
			err := client.Create(context.TODO(), deployment)
			Expect(err).NotTo(HaveOccurred())
			msg, ready, err := reconciler.isDeploymentReady(context.TODO(), &nimServiceKey)
			Expect(err).ToNot(HaveOccurred())
			Expect(ready).To(Equal(false))
			Expect(msg).To(Equal(fmt.Sprintf("deployment %q exceeded its progress deadline", deployment.Name)))
		})

		It("Waiting for deployment rollout to finish: new replicas are coming up", func() {
			nimServiceKey := types.NamespacedName{Name: nimService.Name, Namespace: nimService.Namespace}
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimservice",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &[]int32{4}[0],
				},
				Status: appsv1.DeploymentStatus{
					UpdatedReplicas: 1,
				},
			}
			err := client.Create(context.TODO(), deployment)
			Expect(err).NotTo(HaveOccurred())
			msg, ready, err := reconciler.isDeploymentReady(context.TODO(), &nimServiceKey)
			Expect(err).ToNot(HaveOccurred())
			Expect(ready).To(Equal(false))
			Expect(msg).To(Equal(fmt.Sprintf("Waiting for deployment %q rollout to finish: %d out of %d new replicas have been updated...\n", deployment.Name, deployment.Status.UpdatedReplicas, *deployment.Spec.Replicas)))
		})

		It("Waiting for deployment rollout to finish: old replicas are pending termination", func() {
			nimServiceKey := types.NamespacedName{Name: nimService.Name, Namespace: nimService.Namespace}
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimservice",
					Namespace: "default",
				},
				Status: appsv1.DeploymentStatus{
					UpdatedReplicas: 1,
					Replicas:        4,
				},
			}
			err := client.Create(context.TODO(), deployment)
			Expect(err).NotTo(HaveOccurred())
			msg, ready, err := reconciler.isDeploymentReady(context.TODO(), &nimServiceKey)
			Expect(err).ToNot(HaveOccurred())
			Expect(ready).To(Equal(false))
			Expect(msg).To(Equal(fmt.Sprintf("Waiting for deployment %q rollout to finish: %d old replicas are pending termination...\n", deployment.Name, deployment.Status.Replicas-deployment.Status.UpdatedReplicas)))
		})

		It("Waiting for deployment rollout to finish:", func() {
			nimServiceKey := types.NamespacedName{Name: nimService.Name, Namespace: nimService.Namespace}
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimservice",
					Namespace: "default",
				},
				Status: appsv1.DeploymentStatus{
					UpdatedReplicas:   4,
					AvailableReplicas: 1,
				},
			}
			err := client.Create(context.TODO(), deployment)
			Expect(err).NotTo(HaveOccurred())
			msg, ready, err := reconciler.isDeploymentReady(context.TODO(), &nimServiceKey)
			Expect(err).ToNot(HaveOccurred())
			Expect(ready).To(Equal(false))
			Expect(msg).To(Equal(fmt.Sprintf("Waiting for deployment %q rollout to finish: %d of %d updated replicas are available...\n", deployment.Name, deployment.Status.AvailableReplicas, deployment.Status.UpdatedReplicas)))
		})

		It("Deployment successfully rolled out", func() {
			nimServiceKey := types.NamespacedName{Name: nimService.Name, Namespace: nimService.Namespace}
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimservice",
					Namespace: "default",
				},
				Status: appsv1.DeploymentStatus{
					UpdatedReplicas:   4,
					AvailableReplicas: 4,
				},
			}
			err := client.Create(context.TODO(), deployment)
			Expect(err).NotTo(HaveOccurred())
			msg, ready, err := reconciler.isDeploymentReady(context.TODO(), &nimServiceKey)
			Expect(err).ToNot(HaveOccurred())
			Expect(ready).To(Equal(true))
			Expect(msg).To(Equal(fmt.Sprintf("deployment %q successfully rolled out\n", deployment.Name)))
		})
	})

	Describe("getNIMCacheProfile", func() {
		It("should return nil when NIMCache is not used", func() {
			nimService.Spec.Storage.NIMCache.Name = ""
			profile, err := reconciler.getNIMCacheProfile(context.TODO(), nimService, "some-profile")
			Expect(err).To(BeNil())
			Expect(profile).To(BeNil())
		})

		It("should return an error when NIMCache is not found", func() {
			nimService.Spec.Storage.NIMCache.Name = "non-existent-cache"
			_, err := reconciler.getNIMCacheProfile(context.TODO(), nimService, "some-profile")
			Expect(err).To(HaveOccurred())
		})

		It("should return an error when NIMCache is not ready", func() {
			nimService.Spec.Storage.NIMCache.Name = "test-nimcache"
			nimCache.Status = appsv1alpha1.NIMCacheStatus{
				State: appsv1alpha1.NimCacheStatusPending,
			}

			// Update nimCache status
			Expect(reconciler.Client.Status().Update(context.TODO(), nimCache)).To(Succeed())
			_, err := reconciler.getNIMCacheProfile(context.TODO(), nimService, "test-profile")
			Expect(err).To(HaveOccurred())
		})

		It("should return nil when NIMCache profile is not found", func() {
			nimService.Spec.Storage.NIMCache.Name = "test-nimcache"
			profile, err := reconciler.getNIMCacheProfile(context.TODO(), nimService, "non-existent-profile")
			Expect(err).To(BeNil())
			Expect(profile).To(BeNil())
		})

		It("should return the profile if found in NIMCache", func() {
			nimService.Spec.Storage.NIMCache.Name = "test-nimcache"
			profile, err := reconciler.getNIMCacheProfile(context.TODO(), nimService, "test-profile")
			Expect(err).To(BeNil())
			Expect(profile.Name).To(Equal("test-profile"))
		})
	})

	Describe("getTensorParallelismByProfile", func() {
		It("should return tensor parallelism value if exists", func() {
			profile := &appsv1alpha1.NIMProfile{
				Name:   "test-profile",
				Config: map[string]string{"tp": "4"},
			}

			tensorParallelism, err := reconciler.getTensorParallelismByProfile(context.TODO(), profile)
			Expect(err).To(BeNil())
			Expect(tensorParallelism).To(Equal("4"))
		})

		It("should return empty string if tensor parallelism does not exist", func() {
			profile := &appsv1alpha1.NIMProfile{
				Name:   "test-profile",
				Config: map[string]string{},
			}
			tensorParallelism, err := reconciler.getTensorParallelismByProfile(context.TODO(), profile)
			Expect(err).To(BeNil())
			Expect(tensorParallelism).To(BeEmpty())
		})
	})

	Describe("assignGPUResources", func() {
		It("should retain user-provided GPU resources and not override them", func() {
			profile := &appsv1alpha1.NIMProfile{
				Name:   "test-profile",
				Config: map[string]string{"tp": "4"},
			}

			// Initialize deployment params with user-provided GPU resources
			deploymentParams := &rendertypes.DeploymentParams{
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceName("nvidia.com/gpu"): apiResource.MustParse("8"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceName("nvidia.com/gpu"): apiResource.MustParse("8"),
					},
				},
			}

			Expect(reconciler.assignGPUResources(context.TODO(), nimService, profile, deploymentParams)).To(Succeed())

			// Ensure the user-provided GPU resources (8) are retained and not overridden
			Expect(deploymentParams.Resources.Requests).To(HaveKeyWithValue(corev1.ResourceName("nvidia.com/gpu"), apiResource.MustParse("8")))
			Expect(deploymentParams.Resources.Limits).To(HaveKeyWithValue(corev1.ResourceName("nvidia.com/gpu"), apiResource.MustParse("8")))
		})

		It("should assign GPU resources when tensor parallelism is provided", func() {
			profile := &appsv1alpha1.NIMProfile{
				Name:   "test-profile",
				Config: map[string]string{"tp": "4"},
			}
			// Initialize deployment params with no user-provided GPU resources
			deploymentParams := &rendertypes.DeploymentParams{}

			Expect(reconciler.assignGPUResources(context.TODO(), nimService, profile, deploymentParams)).To(Succeed())
			Expect(deploymentParams.Resources.Requests).To(HaveKeyWithValue(corev1.ResourceName("nvidia.com/gpu"), apiResource.MustParse("4")))
			Expect(deploymentParams.Resources.Limits).To(HaveKeyWithValue(corev1.ResourceName("nvidia.com/gpu"), apiResource.MustParse("4")))
		})

		It("should assign 1 GPU resource if tensor parallelism is not provided", func() {
			profile := &appsv1alpha1.NIMProfile{
				Name:   "test-profile",
				Config: map[string]string{},
			}
			// Initialize deployment params with no user-provided GPU resources
			deploymentParams := &rendertypes.DeploymentParams{}

			Expect(reconciler.assignGPUResources(context.TODO(), nimService, profile, deploymentParams)).To(Succeed())
			Expect(deploymentParams.Resources.Requests).To(HaveKeyWithValue(corev1.ResourceName("nvidia.com/gpu"), apiResource.MustParse("1")))
			Expect(deploymentParams.Resources.Limits).To(HaveKeyWithValue(corev1.ResourceName("nvidia.com/gpu"), apiResource.MustParse("1")))
		})

		It("should return an error if tensor parallelism cannot be parsed", func() {
			profile := &appsv1alpha1.NIMProfile{
				Name:   "test-profile",
				Config: map[string]string{"tp": "invalid"},
			}
			// Initialize deployment params with no user-provided GPU resources
			deploymentParams := &rendertypes.DeploymentParams{}

			err := reconciler.assignGPUResources(context.TODO(), nimService, profile, deploymentParams)
			Expect(err).To(HaveOccurred())
		})
	})
})
