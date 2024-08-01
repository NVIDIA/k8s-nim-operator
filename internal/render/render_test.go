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
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/NVIDIA/k8s-nim-operator/internal/render"
	"github.com/NVIDIA/k8s-nim-operator/internal/render/types"
	corev1 "k8s.io/api/core/v1"
)

type templateData struct {
	Foo string
	Bar string
	Baz string
}

func checkRenderedUnstructured(objs []*unstructured.Unstructured, t *templateData) {
	for idx, obj := range objs {
		Expect(obj.GetKind()).To(Equal(fmt.Sprint("TestObj", idx+1)))
		Expect(obj.Object["metadata"].(map[string]interface{})["name"].(string)).To(Equal(t.Foo))
		Expect(obj.Object["spec"].(map[string]interface{})["attribute"].(string)).To(Equal(t.Bar))
		Expect(obj.Object["spec"].(map[string]interface{})["anotherAttribute"].(string)).To(Equal(t.Baz))
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
			checkRenderedUnstructured(objs, t.Data.(*templateData))
		})
	})

	Context("Render objects from valid manifests dir with mixed file suffixes", func() {
		It("Should return objects in order as appear in the directory lexicographically", func() {
			r := render.NewRenderer(filepath.Join(manifestsTestDir, "mixedManifests"))
			objs, err := r.RenderObjects(t)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(objs)).To(Equal(3))
			checkRenderedUnstructured(objs, t.Data.(*templateData))
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
						Operator: corev1.TolerationOpEqual,
						Value:    "value1",
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
						Operator: corev1.TolerationOpEqual,
						Value:    "value1",
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
				Name:       "test-service",
				Namespace:  "default",
				Port:       80,
				TargetPort: 8080,
				Type:       "ClusterIP",
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
				Host:      "example.com",
				Path:      "/",
				Port:      80,
				ClassName: "test",
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
	})
})
