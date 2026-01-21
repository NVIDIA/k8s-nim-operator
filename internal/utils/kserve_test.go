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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

		BeforeEach(func() {
			scheme = runtime.NewScheme()
			Expect(appsv1.AddToScheme(scheme)).To(Succeed())
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			Expect(kservev1beta1.AddToScheme(scheme)).To(Succeed())

			client = fake.NewClientBuilder().WithScheme(scheme).Build()

			kserveDeployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kserve-controller-manager",
					Namespace: namespace,
					Labels: map[string]string{
						"app.kubernetes.io/name": "kserve-controller-manager",
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
			err := client.Create(context.TODO(), kserveDeployment)
			Expect(err).NotTo(HaveOccurred())

			isvcConfig := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kserveconstants.InferenceServiceConfigMapName,
					Namespace: namespace,
				},
				Data: map[string]string{
					"deploy": "{\"defaultDeploymentMode\": \"RawDeployment\"}",
				},
			}
			err = client.Create(context.TODO(), isvcConfig)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			By("delete the KServe Deployment")
			kserveDeployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kserve-controller-manager",
					Namespace: namespace,
				},
			}
			err := client.Delete(context.TODO(), kserveDeployment)
			Expect(err).NotTo(HaveOccurred())

			By("delete the isvc ConfigMap")
			isvcConfig := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kserveconstants.InferenceServiceConfigMapName,
					Namespace: namespace,
				},
			}
			err = client.Delete(context.TODO(), isvcConfig)
			Expect(err).NotTo(HaveOccurred())
		})

		Describe("When InferenceService is not created", func() {
			It("should use the deployment mode from annotations", func() {
				mode, err := GetKServeDeploymentMode(context.TODO(), client,
					map[string]string{kserveconstants.DeploymentMode: string(kserveconstants.LegacyServerless)}, nil)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(mode).Should(Equal(kserveconstants.LegacyServerless))
			})

			It("should use the default deployment mode from the config map", func() {
				mode, err := GetKServeDeploymentMode(context.TODO(), client,
					nil, nil)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(mode).Should(Equal(kserveconstants.LegacyRawDeployment))
			})
		})

		Describe("When InferenceService is created", func() {
			var isvc *kservev1beta1.InferenceService

			isvcName := "test-isvc"

			BeforeEach(func() {
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
				_ = client.Create(context.TODO(), isvc)
			})

			AfterEach(func() {
				// Clean up the InferenceService instance
				_ = client.Delete(context.TODO(), isvc)
			})

			It("should use the deployment mode from InferenceService status", func() {
				mode, err := GetKServeDeploymentMode(context.TODO(), client,
					map[string]string{kserveconstants.DeploymentMode: string(kserveconstants.LegacyServerless)},
					&k8sclient.ObjectKey{Name: isvcName, Namespace: namespace})
				Expect(err).ShouldNot(HaveOccurred())
				Expect(mode).Should(Equal(kserveconstants.Knative))
			})
		})
	})
})
