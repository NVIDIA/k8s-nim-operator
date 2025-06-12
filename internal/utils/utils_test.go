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
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Utils", func() {
	DescribeTable("GetStringHash",
		func(input string, expected string) {
			actual := GetStringHash(input)
			Expect(actual).To(Equal(expected))
		},
		Entry("UUID", "2269c984-db9a-4b0e-9fd5-86df0ad269f7", "7c6d7bd86b"),
		Entry("UUID with version", "2269c984-db9a-4b0e-9fd5-86df0ad269f7-5.15.0-1041-azure", "79d6bd954f"),
		Entry("UUID with RHCOS version", "2269c984-db9a-4b0e-9fd5-86df0ad269f7-rhcos4.14-414.92.202309282257", "646cdfdb96"),
		Entry("RHCOS version only", "rhcos4.14-414.92.202309282257", "5bbdb464cb"),
		Entry("GPU driver with UUID and RHCOS", "nvidia-gpu-driver-2269c984-db9a-4b0e-9fd5-86df0ad269f7-rhcos4.14-414.92.202309282257", "7bf6859b6d"),
		Entry("vGPU driver with UUID and RHCOS", "nvidia-vgpu-driver-2269c984-db9a-4b0e-9fd5-868df0ad269f7-rhcos4.14-414.92.202309282257", "7469f59898"),
	)

	Context("GetFilesWithSuffix", func() {
		var dir string

		BeforeEach(func() {
			dir = GinkgoT().TempDir()

			// Create test files
			testFiles := []string{"file1.txt", "file2.yaml", "file3.json"}
			for _, file := range testFiles {
				f, err := os.Create(filepath.Join(dir, file))
				Expect(err).NotTo(HaveOccurred())
				err = f.Close()
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("should return files with specified suffixes", func() {
			files, err := GetFilesWithSuffix(dir, ".txt", ".yaml")
			Expect(err).NotTo(HaveOccurred())

			expectedFiles := map[string]bool{
				filepath.Join(dir, "file1.txt"):  true,
				filepath.Join(dir, "file2.yaml"): true,
			}

			Expect(files).To(HaveLen(len(expectedFiles)))
			for _, file := range files {
				Expect(expectedFiles[file]).To(BeTrue())
			}
		})
	})

	DescribeTable("MergeEnvVars",
		func(env1, env2, expected []corev1.EnvVar) {
			mergedEnv := MergeEnvVars(env1, env2)
			Expect(mergedEnv).To(HaveLen(len(expected)))

			envMap := make(map[string]string)
			for _, env := range mergedEnv {
				envMap[env.Name] = env.Value
			}

			for _, env := range expected {
				Expect(envMap[env.Name]).To(Equal(env.Value))
			}
		},
		Entry("merge with override",
			[]corev1.EnvVar{
				{Name: "VAR1", Value: "value1"},
				{Name: "VAR2", Value: "value2"},
			},
			[]corev1.EnvVar{
				{Name: "VAR2", Value: "new_value2"},
				{Name: "VAR3", Value: "value3"},
			},
			[]corev1.EnvVar{
				{Name: "VAR1", Value: "value1"},
				{Name: "VAR2", Value: "new_value2"},
				{Name: "VAR3", Value: "value3"},
			},
		),
	)

	Context("GetResourceHash", func() {
		It("should return non-empty hash for object", func() {
			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
			})
			obj.SetName("test-deployment")
			obj.SetNamespace("default")

			hash := GetResourceHash(obj)
			Expect(hash).NotTo(BeEmpty())
		})
	})

	DescribeTable("IsSpecChanged",
		func(current, desired client.Object, expected bool) {
			current.SetAnnotations(map[string]string{
				NvidiaAnnotationHashKey: GetResourceHash(current),
			})
			Expect(IsSpecChanged(current, desired)).To(Equal(expected))
		},
		Entry("no change in hash with deployment spec and env variables",
			&unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "nim-deployment",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"replicas": 2,
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"app": "nim",
							},
						},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"app": "nim",
								},
							},
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "nim",
										"image": "nim:v0.1.0",
										"ports": []interface{}{
											map[string]interface{}{
												"containerPort": 80,
											},
										},
										"env": []interface{}{
											map[string]interface{}{"name": "ENV_VAR1", "value": "value1"},
											map[string]interface{}{"name": "ENV_VAR2", "value": "value2"},
										},
									},
								},
							},
						},
					},
				},
			},
			&unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "nim-deployment",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"replicas": 2,
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"app": "nim",
							},
						},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"app": "nim",
								},
							},
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "nim",
										"image": "nim:v0.1.0",
										"ports": []interface{}{
											map[string]interface{}{
												"containerPort": 80,
											},
										},
										"env": []interface{}{
											map[string]interface{}{"name": "ENV_VAR1", "value": "value1"},
											map[string]interface{}{"name": "ENV_VAR2", "value": "value2"},
										},
									},
								},
							},
						},
					},
				},
			},
			false,
		),
		Entry("change in hash with change in value of elements",
			&unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "nim-deployment",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"replicas": 2,
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"app": "nim",
							},
						},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"app": "nim",
								},
							},
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "nim",
										"image": "nim:v0.1.0",
										"ports": []interface{}{
											map[string]interface{}{
												"containerPort": 80,
											},
										},
										"env": []interface{}{
											map[string]interface{}{"name": "ENV_VAR1", "value": "value2"},
											map[string]interface{}{"name": "ENV_VAR2", "value": "value1"},
										},
									},
								},
							},
						},
					},
				},
			},
			&unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "nim-deployment",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"replicas": 3,
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"app": "nim",
							},
						},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"app": "nim",
								},
							},
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "nim",
										"image": "nim:v0.1.0",
										"ports": []interface{}{
											map[string]interface{}{
												"containerPort": 80,
											},
										},
										"env": []interface{}{
											map[string]interface{}{"name": "ENV_VAR1", "value": "asdf"},
											map[string]interface{}{"name": "ENV_VAR2", "value": "jljl"},
										},
									},
								},
							},
						},
					},
				},
			},
			true,
		),
	)

	DescribeTable("IsVersionGreaterThanOrEqual",
		func(version, minVersion string, expected bool) {
			Expect(IsVersionGreaterThanOrEqual(version, minVersion)).To(Equal(expected))
		},
		Entry("same version", "v1.33.0", "v1.33.0", true),
		Entry("higher version", "v1.34.0", "v1.33.0", true),
		Entry("lower version", "v1.32.0", "v1.33.0", false),
		Entry("version with build metadata", "v1.33.0+abc123", "v1.33.0", true),
		Entry("min version with build metadata", "v1.33.0", "v1.33.0+abc123", true),
		Entry("alpha version", "v1.33.0-alpha.1", "v1.33.0", false),
		Entry("beta version", "v1.33.0-beta.2", "v1.33.0", false),
		Entry("rc version", "v1.33.0-rc.1", "v1.33.0", false),
		Entry("alpha to beta", "v1.33.0-beta.1", "v1.33.0-alpha.2", true),
		Entry("beta to rc", "v1.33.0-rc.1", "v1.33.0-beta.2", true),
		Entry("rc to release", "v1.33.0", "v1.33.0-rc.1", true),
		Entry("higher patch version", "v1.33.1", "v1.33.0", true),
		Entry("lower patch version", "v1.33.0", "v1.33.1", false),
		Entry("higher minor version", "v1.34.0", "v1.33.0", true),
		Entry("lower minor version", "v1.32.0", "v1.33.0", false),
		Entry("higher major version", "v2.0.0", "v1.33.0", true),
		Entry("lower major version", "v0.33.0", "v1.33.0", false),
		Entry("invalid version", "invalid", "v1.33.0", false),
		Entry("invalid min version", "v1.33.0", "invalid", false),
		Entry("empty version", "", "v1.33.0", false),
		Entry("empty min version", "v1.33.0", "", false),
	)
})
