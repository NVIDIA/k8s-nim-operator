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

package utils

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kservev1beta1 "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	kserveconstants "github.com/kserve/kserve/pkg/constants"
)

var _ = Describe("KServe Utilities", func() {
	Describe("Get Deployment Mode", func() {
		var (
			client k8sclient.Client
			scheme *runtime.Scheme
		)

		namespace := "default"

		var kserveDeployment *appsv1.Deployment
		var isvcConfig *corev1.ConfigMap

		isvcName := "test-isvc"

		// Helper function to create a fresh KServe deployment for isolated tests
		createKServeDeployment := func() *appsv1.Deployment {
			return &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      KServeControllerName,
					Namespace: namespace,
					Labels: map[string]string{
						"app.kubernetes.io/name": KServeControllerName,
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To[int32](1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "nim-test-kserve",
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": "nim-test-kserve",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nim-test-kserve-container",
									Image: "nim-test-kserve-image",
									Ports: []corev1.ContainerPort{
										{
											ContainerPort: 80,
										},
									},
								},
							},
						},
					},
				},
			}
		}

		BeforeEach(func() {
			scheme = runtime.NewScheme()
			Expect(appsv1.AddToScheme(scheme)).To(Succeed())
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			Expect(kservev1beta1.AddToScheme(scheme)).To(Succeed())

			client = fake.NewClientBuilder().WithScheme(scheme).Build()

			kserveDeployment = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      KServeControllerName,
					Namespace: namespace,
					Labels: map[string]string{
						"app.kubernetes.io/name": KServeControllerName,
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To[int32](1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "nim-test-kserve",
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": "nim-test-kserve",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nim-test-kserve-container",
									Image: "nim-test-kserve-image",
									Ports: []corev1.ContainerPort{
										{
											ContainerPort: 80,
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(client.Create(context.Background(), kserveDeployment)).To(Succeed())

			isvcConfig = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kserveconstants.InferenceServiceConfigMapName,
					Namespace: namespace,
				},
				Data: map[string]string{
					"deploy": "{\"defaultDeploymentMode\": \"RawDeployment\"}",
				},
			}
			Expect(client.Create(context.Background(), isvcConfig)).To(Succeed())
		})

		AfterEach(func() {
			By("delete the KServe Deployment")
			Expect(client.DeleteAllOf(context.Background(), &appsv1.Deployment{})).To(Succeed())

			By("delete the isvc ConfigMap - ignore NotFound errors")
			err := client.Delete(context.Background(), isvcConfig)
			if err != nil && !k8serrors.IsNotFound(err) {
				Expect(err).NotTo(HaveOccurred())
			}
		})

		Describe("Priority chain: status > annotation > config > default", func() {
			DescribeTable("deployment mode detection",
				func(annotations map[string]string, expected kserveconstants.DeploymentModeType, expectedErr error) {
					mode, err := GetKServeDeploymentMode(context.Background(), client, annotations, nil)
					if expectedErr != nil {
						Expect(err).To(MatchError(expectedErr.Error()))
						Expect(mode).To(BeEmpty())
					} else {
						Expect(err).NotTo(HaveOccurred())
						Expect(mode).To(Equal(expected))
					}
				},
				// Test annotation-based detection (no ISVC status)
				Entry("should use LegacyServerless from annotations",
					map[string]string{kserveconstants.DeploymentMode: string(kserveconstants.LegacyServerless)},
					kserveconstants.LegacyServerless, nil),
				Entry("should use Standard from annotations",
					map[string]string{kserveconstants.DeploymentMode: string(kserveconstants.Standard)},
					kserveconstants.Standard, nil),
				Entry("should use Knative from annotations",
					map[string]string{kserveconstants.DeploymentMode: string(kserveconstants.Knative)},
					kserveconstants.Knative, nil),
				Entry("should use LegacyRawDeployment from annotations",
					map[string]string{kserveconstants.DeploymentMode: string(kserveconstants.LegacyRawDeployment)},
					kserveconstants.LegacyRawDeployment, nil),
				Entry("should use ModelMesh from annotations",
					map[string]string{kserveconstants.DeploymentMode: string(kserveconstants.ModelMeshDeployment)},
					kserveconstants.ModelMeshDeployment, nil),
				// Test ConfigMap default (no annotations)
				Entry("should use the default deployment mode from the config map when no annotations",
					nil,
					kserveconstants.LegacyRawDeployment, nil),
				// Test invalid annotation falls back to config default
				Entry("should return error when annotation has invalid mode",
					map[string]string{kserveconstants.DeploymentMode: "InvalidMode"},
					kserveconstants.DeploymentModeType(""), fmt.Errorf("deployment mode annotation found but value is invalid: %s", "InvalidMode")),
				Entry("should fall back to config default when annotation is empty string",
					map[string]string{kserveconstants.DeploymentMode: ""},
					kserveconstants.LegacyRawDeployment, nil),
			)

			Context("Status priority tests", func() {
				var isvc *kservev1beta1.InferenceService

				BeforeEach(func() {
					// Create InferenceService only for tests that need it
					isvc = &kservev1beta1.InferenceService{
						ObjectMeta: metav1.ObjectMeta{
							Name:      isvcName,
							Namespace: namespace,
						},
						Spec: kservev1beta1.InferenceServiceSpec{},
						Status: kservev1beta1.InferenceServiceStatus{
							DeploymentMode: string(kserveconstants.Knative),
						},
					}
					Expect(client.Create(context.Background(), isvc)).To(Succeed())
				})

				AfterEach(func() {
					Expect(client.Delete(context.Background(), isvc)).To(Succeed())
				})

				It("should use InferenceService status over annotations", func() {
					mode, err := GetKServeDeploymentMode(context.Background(), client,
						map[string]string{kserveconstants.DeploymentMode: string(kserveconstants.LegacyServerless)},
						&types.NamespacedName{Name: isvcName, Namespace: namespace})
					Expect(err).NotTo(HaveOccurred())
					Expect(mode).To(Equal(kserveconstants.Knative))
				})

				It("should use InferenceService status over ConfigMap default when no annotation", func() {
					mode, err := GetKServeDeploymentMode(context.Background(), client,
						nil,
						&types.NamespacedName{Name: isvcName, Namespace: namespace})
					Expect(err).NotTo(HaveOccurred())
					Expect(mode).To(Equal(kserveconstants.Knative))
				})

				It("should ignore empty status string and use annotation", func() {
					isvc.Status.DeploymentMode = ""
					Expect(client.Update(context.Background(), isvc)).To(Succeed())

					mode, err := GetKServeDeploymentMode(context.Background(), client,
						map[string]string{kserveconstants.DeploymentMode: string(kserveconstants.Standard)},
						&types.NamespacedName{Name: isvcName, Namespace: namespace})
					Expect(err).NotTo(HaveOccurred())
					Expect(mode).To(Equal(kserveconstants.Standard))
				})

				It("should ignore empty status string and use ConfigMap default when no annotation", func() {
					isvc.Status.DeploymentMode = ""
					Expect(client.Update(context.Background(), isvc)).To(Succeed())

					mode, err := GetKServeDeploymentMode(context.Background(), client,
						nil,
						&types.NamespacedName{Name: isvcName, Namespace: namespace})
					Expect(err).NotTo(HaveOccurred())
					Expect(mode).To(Equal(kserveconstants.LegacyRawDeployment))
				})
			})
		})

		Describe("Nil client scenarios (webhook validation)", func() {
			DescribeTable("should handle nil client gracefully",
				func(annotations map[string]string, expected kserveconstants.DeploymentModeType, expectedErr error) {
					mode, err := GetKServeDeploymentMode(context.Background(), nil, annotations, nil)
					if expectedErr != nil {
						Expect(err).To(MatchError(expectedErr.Error()))
						Expect(mode).To(BeEmpty())
					} else {
						Expect(err).NotTo(HaveOccurred())
						Expect(mode).To(Equal(expected))
					}
				},
				Entry("with Standard annotation",
					map[string]string{kserveconstants.DeploymentMode: string(kserveconstants.Standard)},
					kserveconstants.Standard, nil),
				Entry("with Knative annotation",
					map[string]string{kserveconstants.DeploymentMode: string(kserveconstants.Knative)},
					kserveconstants.Knative, nil),
				Entry("with LegacyServerless annotation",
					map[string]string{kserveconstants.DeploymentMode: string(kserveconstants.LegacyServerless)},
					kserveconstants.LegacyServerless, nil),
				Entry("with LegacyRawDeployment annotation",
					map[string]string{kserveconstants.DeploymentMode: string(kserveconstants.LegacyRawDeployment)},
					kserveconstants.LegacyRawDeployment, nil),
				Entry("with no annotations should use default",
					nil,
					kserveconstants.DefaultDeployment, nil),
				Entry("with invalid annotation should use default",
					map[string]string{kserveconstants.DeploymentMode: "InvalidMode"},
					kserveconstants.DeploymentModeType(""), fmt.Errorf("deployment mode annotation found but value is invalid: %s", "InvalidMode")),
			)
		})

		Describe("Error cases", func() {
			// Use isolated clients for error tests to avoid state pollution
			It("should return error when ConfigMap has malformed JSON", func() {
				// Create isolated test client
				isolatedClient := fake.NewClientBuilder().WithScheme(scheme).Build()

				// Setup KServe deployment
				Expect(isolatedClient.Create(context.Background(), createKServeDeployment())).To(Succeed())

				// Create ConfigMap with malformed JSON
				badConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      kserveconstants.InferenceServiceConfigMapName,
						Namespace: namespace,
					},
					Data: map[string]string{
						"deploy": "{invalid json}",
					},
				}
				Expect(isolatedClient.Create(context.Background(), badConfigMap)).To(Succeed())

				_, err := GetKServeDeploymentMode(context.Background(), isolatedClient, nil, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unable to parse deploy config json"))
			})

			It("should return error when ConfigMap has empty defaultDeploymentMode", func() {
				isolatedClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				Expect(isolatedClient.Create(context.Background(), createKServeDeployment())).To(Succeed())

				badConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      kserveconstants.InferenceServiceConfigMapName,
						Namespace: namespace,
					},
					Data: map[string]string{
						"deploy": "{\"defaultDeploymentMode\": \"\"}",
					},
				}
				Expect(isolatedClient.Create(context.Background(), badConfigMap)).To(Succeed())

				_, err := GetKServeDeploymentMode(context.Background(), isolatedClient, nil, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("defaultDeploymentMode is required"))
			})

			It("should return error when ConfigMap has unsupported deployment mode", func() {
				isolatedClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				Expect(isolatedClient.Create(context.Background(), createKServeDeployment())).To(Succeed())

				badConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      kserveconstants.InferenceServiceConfigMapName,
						Namespace: namespace,
					},
					Data: map[string]string{
						"deploy": "{\"defaultDeploymentMode\": \"UnsupportedMode\"}",
					},
				}
				Expect(isolatedClient.Create(context.Background(), badConfigMap)).To(Succeed())

				_, err := GetKServeDeploymentMode(context.Background(), isolatedClient, nil, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid deployment mode"))
			})

			It("should return error when KServe deployment not found", func() {
				isolatedClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				// Don't create KServe deployment

				_, err := GetKServeDeploymentMode(context.Background(), isolatedClient, nil, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to find the namespace of KServe Deployment"))
			})

			It("should use annotation when ConfigMap doesn't exist", func() {
				isolatedClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				Expect(isolatedClient.Create(context.Background(), createKServeDeployment())).To(Succeed())
				// Don't create ConfigMap

				mode, err := GetKServeDeploymentMode(context.Background(), isolatedClient,
					map[string]string{kserveconstants.DeploymentMode: string(kserveconstants.Knative)}, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(mode).To(Equal(kserveconstants.Knative))
			})

			It("should use default when ConfigMap exists but has no deploy key", func() {
				isolatedClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				Expect(isolatedClient.Create(context.Background(), createKServeDeployment())).To(Succeed())

				emptyConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      kserveconstants.InferenceServiceConfigMapName,
						Namespace: namespace,
					},
					Data: map[string]string{
						// No "deploy" key
						"other": "value",
					},
				}
				Expect(isolatedClient.Create(context.Background(), emptyConfigMap)).To(Succeed())

				mode, err := GetKServeDeploymentMode(context.Background(), isolatedClient, nil, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(mode).To(Equal(kserveconstants.DefaultDeployment))
			})
		})

		Describe("ConfigMap variations", func() {
			// Use isolated clients to avoid test pollution
			It("should work with Standard as default in ConfigMap", func() {
				isolatedClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				Expect(isolatedClient.Create(context.Background(), createKServeDeployment())).To(Succeed())

				standardConfig := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      kserveconstants.InferenceServiceConfigMapName,
						Namespace: namespace,
					},
					Data: map[string]string{
						"deploy": "{\"defaultDeploymentMode\": \"Standard\"}",
					},
				}
				Expect(isolatedClient.Create(context.Background(), standardConfig)).To(Succeed())

				mode, err := GetKServeDeploymentMode(context.Background(), isolatedClient, nil, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(mode).To(Equal(kserveconstants.Standard))
			})

			It("should work with Knative as default in ConfigMap", func() {
				isolatedClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				Expect(isolatedClient.Create(context.Background(), createKServeDeployment())).To(Succeed())

				knativeConfig := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      kserveconstants.InferenceServiceConfigMapName,
						Namespace: namespace,
					},
					Data: map[string]string{
						"deploy": "{\"defaultDeploymentMode\": \"Knative\"}",
					},
				}
				Expect(isolatedClient.Create(context.Background(), knativeConfig)).To(Succeed())

				mode, err := GetKServeDeploymentMode(context.Background(), isolatedClient, nil, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(mode).To(Equal(kserveconstants.Knative))
			})

			It("should work with Serverless as default in ConfigMap", func() {
				isolatedClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				Expect(isolatedClient.Create(context.Background(), createKServeDeployment())).To(Succeed())

				serverlessConfig := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      kserveconstants.InferenceServiceConfigMapName,
						Namespace: namespace,
					},
					Data: map[string]string{
						"deploy": "{\"defaultDeploymentMode\": \"Serverless\"}",
					},
				}
				Expect(isolatedClient.Create(context.Background(), serverlessConfig)).To(Succeed())

				mode, err := GetKServeDeploymentMode(context.Background(), isolatedClient, nil, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(mode).To(Equal(kserveconstants.LegacyServerless))
			})
		})

		Describe("Annotation filtering", func() {
			It("should filter annotations based on ServiceAnnotationDisallowedList", func() {
				isolatedClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				Expect(isolatedClient.Create(context.Background(), createKServeDeployment())).To(Succeed())

				// Create ConfigMap with disallowed annotations list
				configWithFilter := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      kserveconstants.InferenceServiceConfigMapName,
						Namespace: namespace,
					},
					Data: map[string]string{
						"deploy": "{\"defaultDeploymentMode\": \"Standard\"}",
						"inferenceService": `{
							"serviceAnnotationDisallowedList": [
								"disallowed-annotation-key"
							]
						}`,
					},
				}
				Expect(isolatedClient.Create(context.Background(), configWithFilter)).To(Succeed())

				// Provide annotations including a disallowed one
				annotations := map[string]string{
					kserveconstants.DeploymentMode: string(kserveconstants.Knative),
					"disallowed-annotation-key":    "should-be-filtered",
					"allowed-annotation":           "should-remain",
				}

				mode, err := GetKServeDeploymentMode(context.Background(), isolatedClient, annotations, nil)
				Expect(err).NotTo(HaveOccurred())
				// Should still detect Knative mode since the deployment mode annotation is allowed
				Expect(mode).To(Equal(kserveconstants.Knative))
			})

			It("should filter out the deployment mode annotation based on ServiceAnnotationDisallowedList", func() {
				isolatedClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				Expect(isolatedClient.Create(context.Background(), createKServeDeployment())).To(Succeed())

				// Create ConfigMap with disallowed annotations list
				configWithFilter := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      kserveconstants.InferenceServiceConfigMapName,
						Namespace: namespace,
					},
					Data: map[string]string{
						"deploy": "{\"defaultDeploymentMode\": \"Standard\"}",
						"inferenceService": `{
							"serviceAnnotationDisallowedList": [
								"disallowed-annotation-key",
								"serving.kserve.io/deploymentMode"
							]
						}`,
					},
				}
				Expect(isolatedClient.Create(context.Background(), configWithFilter)).To(Succeed())

				// Provide annotations including deployment mode which will be filtered
				annotations := map[string]string{
					kserveconstants.DeploymentMode: string(kserveconstants.Knative),
					"disallowed-annotation-key":    "should-be-filtered",
					"allowed-annotation":           "should-remain",
				}

				mode, err := GetKServeDeploymentMode(context.Background(), isolatedClient, annotations, nil)
				Expect(err).NotTo(HaveOccurred())
				// Should fall back to ConfigMap default (Standard) since deployment mode annotation was filtered
				Expect(mode).To(Equal(kserveconstants.Standard))
			})

			It("should use status over filtered annotations", func() {
				isolatedClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				Expect(isolatedClient.Create(context.Background(), createKServeDeployment())).To(Succeed())

				// Create ConfigMap with disallowed annotations list including deployment mode
				configWithFilter := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      kserveconstants.InferenceServiceConfigMapName,
						Namespace: namespace,
					},
					Data: map[string]string{
						"deploy": "{\"defaultDeploymentMode\": \"Standard\"}",
						"inferenceService": `{
							"serviceAnnotationDisallowedList": [
								"serving.kserve.io/deploymentMode"
							]
						}`,
					},
				}
				Expect(isolatedClient.Create(context.Background(), configWithFilter)).To(Succeed())

				// Create InferenceService with status
				isvc := &kservev1beta1.InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      isvcName,
						Namespace: namespace,
					},
					Spec: kservev1beta1.InferenceServiceSpec{},
					Status: kservev1beta1.InferenceServiceStatus{
						DeploymentMode: string(kserveconstants.Knative),
					},
				}
				Expect(isolatedClient.Create(context.Background(), isvc)).To(Succeed())

				// Provide annotation that will be filtered
				annotations := map[string]string{
					kserveconstants.DeploymentMode: string(kserveconstants.LegacyServerless),
				}

				mode, err := GetKServeDeploymentMode(context.Background(), isolatedClient, annotations,
					&types.NamespacedName{Name: isvcName, Namespace: namespace})
				Expect(err).NotTo(HaveOccurred())
				// Status should take priority even when annotation is filtered
				// Status = Knative, filtered annotation = LegacyServerless, config = Standard
				// Result should be Knative (status wins)
				Expect(mode).To(Equal(kserveconstants.Knative))
			})
		})
	})
})
