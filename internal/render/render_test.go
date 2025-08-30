/**
# Copyright (c) NVIDIA CORPORATION.  All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package render_test

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	corev1 "k8s.io/api/core/v1"

	"github.com/NVIDIA/k8s-nim-operator/internal/render"
	"github.com/NVIDIA/k8s-nim-operator/internal/render/types"
)

type templateData struct {
	Foo string
	Bar string
	Baz string
}

func checkRenderedUnstructured(objs []*unstructured.Unstructured, t *templateData) {
	for idx, obj := range objs {
		Expect(obj.GetKind()).To(Equal(fmt.Sprint("TestObj", idx+1)))
		metadata, ok := obj.Object["metadata"].(map[string]interface{})["name"].(string)
		Expect(ok).To(BeTrue())
		Expect(metadata).To(Equal(t.Foo))
		attribute, ok := obj.Object["spec"].(map[string]interface{})["attribute"].(string)
		Expect(ok).To(BeTrue())
		Expect(attribute).To(Equal(t.Bar))
		anotherAttribute, ok := obj.Object["spec"].(map[string]interface{})["anotherAttribute"].(string)
		Expect(ok).To(BeTrue())
		Expect(anotherAttribute).To(Equal(t.Baz))
	}
}

var _ = Describe("Test Renderer via API", func() {
	t := &render.TemplateData{
		Funcs: nil,
		Data:  &templateData{"foo", "bar", "baz"},
	}

	cwd, err := os.Getwd()
	if err != nil {
		panic("Failed to get CWD")
	}
	manifestsTestDir := filepath.Join(cwd, "testdata")

	Context("Render objects from non-existent directory", func() {
		It("Should fail", func() {
			r := render.NewRenderer(filepath.Join(manifestsTestDir, "doesNotExist"))
			objs, err := r.RenderObjects(t)
			Expect(err).To(HaveOccurred())
			Expect(objs).To(BeNil())
		})
	})

	Context("Render objects from mal formatted files", func() {
		It("Should fail", func() {
			r := render.NewRenderer(filepath.Join(manifestsTestDir, "badManifests"))
			objs, err := r.RenderObjects(t)
			Expect(err).To(HaveOccurred())
			Expect(objs).To(BeNil())
		})
	})

	Context("Render objects from template with invalid template data", func() {
		It("Should fail", func() {
			r := render.NewRenderer(filepath.Join(manifestsTestDir, "invalidManifests"))
			objs, err := r.RenderObjects(t)
			Expect(err).To(HaveOccurred())
			Expect(objs).To(BeNil())
		})
	})

	Context("Render objects from valid manifests dir", func() {
		It("Should return objects in order as appear in the directory lexicographically", func() {
			r := render.NewRenderer(filepath.Join(manifestsTestDir, "manifests"))
			objs, err := r.RenderObjects(t)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(objs)).To(Equal(3))
			templateData, ok := t.Data.(*templateData)
			Expect(ok).To(BeTrue())
			checkRenderedUnstructured(objs, templateData)
		})
	})

	Context("Render objects from valid manifests dir with mixed file suffixes", func() {
		It("Should return objects in order as appear in the directory lexicographically", func() {
			r := render.NewRenderer(filepath.Join(manifestsTestDir, "mixedManifests"))
			objs, err := r.RenderObjects(t)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(objs)).To(Equal(3))
			templateData, ok := t.Data.(*templateData)
			Expect(ok).To(BeTrue())
			checkRenderedUnstructured(objs, templateData)
		})
	})
})

var _ = Describe("K8s Resources Rendering", func() {
	cwd, err := os.Getwd()
	if err != nil {
		panic("Failed to get CWD")
	}
	templatesDir := filepath.Join(path.Dir(path.Dir(cwd)), "manifests")

	Context("Rendering templates", func() {
		It("should render LeaderWorkerSet template correctly", func() {
			params := types.LeaderWorkerSetParams{
				Name:             "test-lws",
				Namespace:        "default",
				Labels:           map[string]string{"app": "test-app"},
				Annotations:      map[string]string{"annotation-key": "annotation-value"},
				Replicas:         3,
				Size:             2,
				Image:            "nim-llm:latest",
				ImagePullSecrets: []string{"ngc-secret"},
				LeaderVolumes: []corev1.Volume{
					{
						Name: "test-leader-volume",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "test-leader-pvc",
							},
						},
					},
				},
				WorkerVolumes: []corev1.Volume{
					{
						Name: "test-worker-volume",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "test-worker-pvc",
							},
						},
					},
				},
				LeaderVolumeMounts: []corev1.VolumeMount{
					{
						Name:      "test-leader-volume",
						MountPath: "/data",
					},
				},
				WorkerVolumeMounts: []corev1.VolumeMount{
					{
						Name:      "test-worker-volume",
						MountPath: "/data",
					},
				},
				LeaderEnvs: []corev1.EnvVar{
					{
						Name:  "LEADER_ENV_VAR",
						Value: "value",
					},
				},
				WorkerEnvs: []corev1.EnvVar{
					{
						Name:  "WORKER_ENV_VAR",
						Value: "value",
					},
				},
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 8080,
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
				ReadinessProbe: &corev1.Probe{
					InitialDelaySeconds: 15,
					TimeoutSeconds:      1,
					PeriodSeconds:       10,
					SuccessThreshold:    1,
					FailureThreshold:    3,
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/v1/health/live",
							Port: intstr.FromString("8080"),
						},
					},
				},
				LivenessProbe: &corev1.Probe{
					InitialDelaySeconds: 15,
					TimeoutSeconds:      1,
					PeriodSeconds:       10,
					SuccessThreshold:    1,
					FailureThreshold:    3,
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/v1/health/ready",
							Port: intstr.FromString("8080"),
						},
					},
				},
				StartupProbe: &corev1.Probe{
					InitialDelaySeconds: 15,
					TimeoutSeconds:      1,
					PeriodSeconds:       10,
					SuccessThreshold:    1,
					FailureThreshold:    3,
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/v1/health/ready",
							Port: intstr.FromString("8080"),
						},
					},
				},
				NodeSelector: map[string]string{"disktype": "ssd"},
				Tolerations: []corev1.Toleration{
					{
						Key:      "key1",
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
			}

			r := render.NewRenderer(templatesDir)
			lws, err := r.LeaderWorkerSet(&params)
			Expect(err).NotTo(HaveOccurred())
			Expect(lws.Name).To(Equal("test-lws"))
			Expect(lws.Namespace).To(Equal("default"))
			Expect(lws.Labels["app"]).To(Equal("test-app"))
			Expect(lws.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(*lws.Spec.Replicas).To(Equal(int32(3)))
			Expect(*lws.Spec.LeaderWorkerTemplate.Size).To(Equal(int32(2)))
			Expect(lws.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec.Containers[0].Name).To(Equal("nim-leader"))
			Expect(lws.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec.Containers[0].Image).To(Equal("nim-llm:latest"))
			Expect(lws.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.Containers[0].Name).To(Equal("nim-worker"))
			Expect(lws.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.Containers[0].Image).To(Equal("nim-llm:latest"))
			Expect(lws.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec.Volumes[0].Name).To(Equal("test-leader-volume"))
			Expect(lws.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec.Volumes[0].VolumeSource.PersistentVolumeClaim.ClaimName).To(Equal("test-leader-pvc"))
			Expect(lws.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.Volumes[0].Name).To(Equal("test-worker-volume"))
			Expect(lws.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.Volumes[0].VolumeSource.PersistentVolumeClaim.ClaimName).To(Equal("test-worker-pvc"))
			Expect(lws.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec.Containers[0].VolumeMounts[0].Name).To(Equal("test-leader-volume"))
			Expect(lws.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec.Containers[0].VolumeMounts[0].MountPath).To(Equal("/data"))
			Expect(lws.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.Containers[0].VolumeMounts[0].Name).To(Equal("test-worker-volume"))
			Expect(lws.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.Containers[0].VolumeMounts[0].MountPath).To(Equal("/data"))
			Expect(lws.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec.Containers[0].ReadinessProbe).ToNot(BeNil())
			Expect(lws.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec.Containers[0].StartupProbe).ToNot(BeNil())
			Expect(lws.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec.Containers[0].LivenessProbe).ToNot(BeNil())
			Expect(lws.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec.ImagePullSecrets[0].Name).To(Equal("ngc-secret"))
			Expect(lws.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.ImagePullSecrets[0].Name).To(Equal("ngc-secret"))
			Expect(lws.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec.Containers[0].Ports[0].ContainerPort).To(Equal(int32(8080)))
			Expect(lws.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.Containers[0].Ports[0].ContainerPort).To(Equal(int32(8080)))
			Expect(lws.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec.NodeSelector).To(Equal(map[string]string{"disktype": "ssd"}))
			Expect(lws.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.NodeSelector).To(Equal(map[string]string{"disktype": "ssd"}))
			Expect(lws.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec.Tolerations).To(Equal([]corev1.Toleration{{Key: "key1", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}}))
			Expect(lws.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.Tolerations).To(Equal([]corev1.Toleration{{Key: "key1", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}}))
		})
		It("should render Deployment template correctly", func() {
			params := types.DeploymentParams{
				Name:          "test-deployment",
				Namespace:     "default",
				Labels:        map[string]string{"app": "test-app"},
				Annotations:   map[string]string{"annotation-key": "annotation-value"},
				Replicas:      3,
				ContainerName: "test-container",
				Image:         "nim-llm:latest",
				Volumes: []corev1.Volume{
					{
						Name: "test-volume",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "test-pvc",
							},
						},
					},
				},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "test-volume",
						MountPath: "/data",
						SubPath:   "subPath",
					},
				},
				Env: []corev1.EnvVar{
					{
						Name:  "ENV_VAR",
						Value: "value",
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
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
			}

			r := render.NewRenderer(templatesDir)
			deployment, err := r.Deployment(&params)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Name).To(Equal("test-deployment"))
			Expect(deployment.Namespace).To(Equal("default"))
			Expect(deployment.Labels["app"]).To(Equal("test-app"))
			Expect(deployment.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(*deployment.Spec.Replicas).To(Equal(int32(3)))
			Expect(deployment.Spec.Template.Spec.Containers[0].Name).To(Equal("test-container"))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal("nim-llm:latest"))
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name).To(Equal("test-volume"))
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath).To(Equal("/data"))
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts[0].SubPath).To(Equal("subPath"))
		})

		It("should render StatefulSet template correctly", func() {
			params := types.StatefulSetParams{
				Name:          "test-statefulset",
				Namespace:     "default",
				Labels:        map[string]string{"app": "test-app"},
				Annotations:   map[string]string{"annotation-key": "annotation-value"},
				Replicas:      3,
				ContainerName: "test-container",
				ServiceName:   "test-app",
				Image:         "nim-llm:latest",
				Volumes: []corev1.Volume{
					{
						Name: "test-volume",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "test-pvc",
							},
						},
					},
				},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "test-volume",
						MountPath: "/data",
					},
				},
				Env: []corev1.EnvVar{
					{
						Name:  "ENV_VAR",
						Value: "value",
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
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
			}

			r := render.NewRenderer(templatesDir)
			deployment, err := r.StatefulSet(&params)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Name).To(Equal("test-statefulset"))
			Expect(deployment.Namespace).To(Equal("default"))
			Expect(deployment.Labels["app"]).To(Equal("test-app"))
			Expect(deployment.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(*deployment.Spec.Replicas).To(Equal(int32(3)))
			Expect(deployment.Spec.Template.Spec.Containers[0].Name).To(Equal("test-container"))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal("nim-llm:latest"))
		})

		It("should render Service template correctly", func() {
			params := types.ServiceParams{
				Name:      "test-service",
				Namespace: "default",
				Ports: []corev1.ServicePort{{
					Port:       80,
					TargetPort: intstr.FromInt(8080),
				}},
				Type: "ClusterIP",
			}
			r := render.NewRenderer(templatesDir)
			service, err := r.Service(&params)
			Expect(err).NotTo(HaveOccurred())
			Expect(service.Name).To(Equal("test-service"))
			Expect(service.Namespace).To(Equal("default"))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(80)))
			Expect(service.Spec.Ports[0].TargetPort.IntVal).To(Equal(int32(8080)))
		})

		It("should render ServiceAccount template correctly", func() {
			params := types.ServiceAccountParams{
				Name:      "test-serviceaccount",
				Namespace: "default",
			}
			r := render.NewRenderer(templatesDir)
			serviceAccount, err := r.ServiceAccount(&params)
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceAccount.Name).To(Equal("test-serviceaccount"))
			Expect(serviceAccount.Namespace).To(Equal("default"))
		})

		It("should render Role template correctly", func() {
			params := types.RoleParams{
				Name:      "test-role",
				Namespace: "default",
			}
			r := render.NewRenderer(templatesDir)
			role, err := r.Role(&params)
			Expect(err).NotTo(HaveOccurred())
			Expect(role.Name).To(Equal("test-role"))
			Expect(role.Namespace).To(Equal("default"))
		})

		It("should render RoleBinding template correctly", func() {
			params := types.RoleBindingParams{
				Name:      "test-rolebinding",
				Namespace: "default",
			}
			r := render.NewRenderer(templatesDir)
			roleBinding, err := r.RoleBinding(&params)
			Expect(err).NotTo(HaveOccurred())
			Expect(roleBinding.Name).To(Equal("test-rolebinding"))
			Expect(roleBinding.Namespace).To(Equal("default"))
		})

		It("should render SCC template correctly", func() {
			params := types.SCCParams{
				Name:               "test-scc",
				ServiceAccountName: "test-service-account",
			}

			r := render.NewRenderer(templatesDir)
			scc, err := r.SCC(&params)
			Expect(err).NotTo(HaveOccurred())
			Expect(scc.Name).To(Equal("test-scc"))
		})

		It("should render Ingress template correctly", func() {
			params := types.IngressParams{
				Enabled:   true,
				Name:      "test-ingress",
				Namespace: "default",
				Spec: v1.IngressSpec{Rules: []v1.IngressRule{
					{
						Host:             "chart-example.local",
						IngressRuleValue: v1.IngressRuleValue{},
					},
				},
				},
			}

			r := render.NewRenderer(templatesDir)
			ingress, err := r.Ingress(&params)
			Expect(err).NotTo(HaveOccurred())
			Expect(ingress.Name).To(Equal("test-ingress"))
			Expect(ingress.Namespace).To(Equal("default"))
		})

		It("should render HPA template correctly", func() {
			minRep := int32(1)
			params := types.HPAParams{
				Enabled:   true,
				Name:      "test-hpa",
				Namespace: "default",
				HPASpec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						Name:       "test-deployment",
						Kind:       "Deployment",
						APIVersion: "apps/v1",
					},
					MinReplicas: &minRep,
					MaxReplicas: 10,
				},
			}
			r := render.NewRenderer(templatesDir)
			hpa, err := r.HPA(&params)
			Expect(err).NotTo(HaveOccurred())
			Expect(hpa.Name).To(Equal("test-hpa"))
			Expect(hpa.Namespace).To(Equal("default"))
			Expect(*hpa.Spec.MinReplicas).To(Equal(int32(1)))
			Expect(hpa.Spec.MaxReplicas).To(Equal(int32(10)))
		})

		It("should render HPA template and sort metrics spec correctly", func() {
			minRep := int32(1)
			params := types.HPAParams{
				Enabled:   true,
				Name:      "test-hpa",
				Namespace: "default",
				HPASpec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						Name:       "test-deployment",
						Kind:       "Deployment",
						APIVersion: "apps/v1",
					},
					MinReplicas: &minRep,
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
			}
			r := render.NewRenderer(templatesDir)
			hpa, err := r.HPA(&params)
			Expect(err).NotTo(HaveOccurred())
			Expect(hpa.Name).To(Equal("test-hpa"))
			Expect(hpa.Namespace).To(Equal("default"))
			Expect(*hpa.Spec.MinReplicas).To(Equal(int32(1)))
			Expect(hpa.Spec.MaxReplicas).To(Equal(int32(10)))
			Expect(hpa.Spec.Metrics[0].Type).To(Equal(autoscalingv2.PodsMetricSourceType))
			Expect(hpa.Spec.Metrics[1].Type).To(Equal(autoscalingv2.ResourceMetricSourceType))
		})

		It("should render ConfigMap template correctly", func() {
			params := types.ConfigMapParams{
				Name:          "test-config",
				Namespace:     "default",
				Labels:        map[string]string{"app": "k8s-nim-operator"},
				Annotations:   map[string]string{"annotation-key": "annotation-value"},
				ConfigMapData: map[string]string{"config.yaml": "test-data"},
			}

			r := render.NewRenderer(templatesDir)
			cm, err := r.ConfigMap(&params)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Name).To(Equal("test-config"))
			Expect(cm.Namespace).To(Equal("default"))
			Expect(cm.Labels["app"]).To(Equal("k8s-nim-operator"))
			Expect(cm.Annotations["annotation-key"]).To(Equal("annotation-value"))
			// The data is added as a multi-line string, so verify with newline appended
			Expect(cm.Data["config.yaml"]).To(Equal("test-data\n"))
		})

		It("should render InferenceService template correctly", func() {
			params := types.InferenceServiceParams{
				Name:            "test-inferenceservice",
				Namespace:       "default",
				Labels:          map[string]string{"app": "test-app"},
				Annotations:     map[string]string{"annotation-key": "annotation-value"},
				MinReplicas:     ptr.To[int32](3),
				MaxReplicas:     ptr.To[int32](5),
				ScaleMetricType: "Value",
				ScaleMetric:     "cpu",
				ScaleTarget:     ptr.To[int32](80),
				UserID:          ptr.To[int64](1000),
				GroupID:         ptr.To[int64](2000),
				ContainerName:   "test-container",
				Image:           "nim-llm:latest",
				Volumes: []corev1.Volume{
					{
						Name: "test-volume",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "test-pvc",
							},
						},
					},
				},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "test-volume",
						MountPath: "/data",
						SubPath:   "subPath",
					},
				},
				Env: []corev1.EnvVar{
					{
						Name:  "ENV_VAR",
						Value: "value",
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
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
				DeploymentMode: "RawDeployment",
			}

			r := render.NewRenderer(templatesDir)
			inferenceservice, err := r.InferenceService(&params)
			Expect(err).NotTo(HaveOccurred())
			Expect(inferenceservice.Name).To(Equal("test-inferenceservice"))
			Expect(inferenceservice.Namespace).To(Equal("default"))
			Expect(inferenceservice.Labels["app"]).To(Equal("test-app"))
			Expect(inferenceservice.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(*inferenceservice.Spec.Predictor.MinReplicas).To(Equal(int32(3)))
			Expect(inferenceservice.Spec.Predictor.MaxReplicas).To(Equal(int32(5)))
			Expect(string(*inferenceservice.Spec.Predictor.ScaleMetricType)).To(Equal("Value"))
			Expect(string(*inferenceservice.Spec.Predictor.ScaleMetric)).To(Equal("cpu"))
			Expect(*inferenceservice.Spec.Predictor.ScaleTarget).To(Equal(int32(80)))
			Expect(string(inferenceservice.Spec.Predictor.DeploymentStrategy.Type)).To(Equal("RollingUpdate"))
			Expect(inferenceservice.Spec.Predictor.DeploymentStrategy.RollingUpdate.MaxSurge.IntValue()).To(Equal(0))
			Expect(inferenceservice.Spec.Predictor.DeploymentStrategy.RollingUpdate.MaxUnavailable.String()).To(Equal("25%"))

			Expect(inferenceservice.Spec.Predictor.Containers[0].Name).To(Equal("test-container"))
			Expect(inferenceservice.Spec.Predictor.Containers[0].Image).To(Equal("nim-llm:latest"))
			Expect(inferenceservice.Spec.Predictor.Containers[0].VolumeMounts[0].Name).To(Equal("test-volume"))
			Expect(inferenceservice.Spec.Predictor.Containers[0].VolumeMounts[0].MountPath).To(Equal("/data"))
			Expect(inferenceservice.Spec.Predictor.Containers[0].VolumeMounts[0].SubPath).To(Equal("subPath"))
			Expect(inferenceservice.Spec.Predictor.Containers[0].Env[0].Name).To(Equal("ENV_VAR"))
			Expect(inferenceservice.Spec.Predictor.Containers[0].Env[0].Value).To(Equal("value"))

			nodeSelectorValue, nodeSelectorOk := inferenceservice.Spec.Predictor.NodeSelector["disktype"]
			Expect(nodeSelectorOk).To(Equal(true))
			Expect(nodeSelectorValue).To(Equal("ssd"))

			Expect(inferenceservice.Spec.Predictor.Tolerations[0].Key).To(Equal("key1"))
			Expect(inferenceservice.Spec.Predictor.Tolerations[0].Operator).To(Equal(corev1.TolerationOpExists))
			Expect(inferenceservice.Spec.Predictor.Tolerations[0].Effect).To(Equal(corev1.TaintEffectNoSchedule))

			cpuLimitValue, cpuLimitOk := inferenceservice.Spec.Predictor.Containers[0].Resources.Limits[corev1.ResourceCPU]
			Expect(cpuLimitOk).To(Equal(true))
			Expect(cpuLimitValue).To(Equal(resource.MustParse("500m")))
			memoryLimitValue, memoryLimitOk := inferenceservice.Spec.Predictor.Containers[0].Resources.Limits[corev1.ResourceMemory]
			Expect(memoryLimitOk).To(Equal(true))
			Expect(memoryLimitValue).To(Equal(resource.MustParse("128Mi")))

			cpuRequestValue, cpuRequestOk := inferenceservice.Spec.Predictor.Containers[0].Resources.Requests[corev1.ResourceCPU]
			Expect(cpuRequestOk).To(Equal(true))
			Expect(cpuRequestValue).To(Equal(resource.MustParse("250m")))
			memoryRequestValue, memoryRequestOk := inferenceservice.Spec.Predictor.Containers[0].Resources.Requests[corev1.ResourceMemory]
			Expect(memoryRequestOk).To(Equal(true))
			Expect(memoryRequestValue).To(Equal(resource.MustParse("64Mi")))

			Expect(inferenceservice.Spec.Predictor.Volumes[0].Name).To(Equal("test-volume"))
			Expect(inferenceservice.Spec.Predictor.Volumes[0].VolumeSource.PersistentVolumeClaim.ClaimName).To(Equal("test-pvc"))

			Expect(*inferenceservice.Spec.Predictor.SecurityContext.RunAsUser).To(Equal(int64(1000)))
			Expect(*inferenceservice.Spec.Predictor.SecurityContext.RunAsGroup).To(Equal(int64(2000)))
			Expect(*inferenceservice.Spec.Predictor.SecurityContext.FSGroup).To(Equal(int64(2000)))

		})
	})
})
