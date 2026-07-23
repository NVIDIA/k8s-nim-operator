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
	"strings"

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

var _ = Describe("Manifest ConfigMap compression helpers", func() {
	Describe("CompressData/DecompressData", func() {
		It("round-trips arbitrary content", func() {
			original := []byte(strings.Repeat("nim-operator manifest content\n", 5000))
			compressed, err := CompressData(original)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(compressed)).To(BeNumerically("<", len(original)), "gzip should shrink repetitive content")

			decompressed, err := DecompressData(compressed)
			Expect(err).NotTo(HaveOccurred())
			Expect(decompressed).To(Equal(original))
		})

		It("round-trips empty input", func() {
			compressed, err := CompressData([]byte{})
			Expect(err).NotTo(HaveOccurred())
			decompressed, err := DecompressData(compressed)
			Expect(err).NotTo(HaveOccurred())
			Expect(decompressed).To(HaveLen(0))
		})

		It("returns an error for non-gzip input", func() {
			_, err := DecompressData([]byte("this is not gzip"))
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("SetManifestConfigMapData", func() {
		const key = "model_manifest.yaml"
		const gzKey = "model_manifest.yaml" + GzipCompressedKeySuffix

		It("stores small payloads as plaintext in Data", func() {
			cm := &corev1.ConfigMap{}
			small := []byte("small manifest")

			compressed, err := SetManifestConfigMapData(cm, key, small)
			Expect(err).NotTo(HaveOccurred())
			Expect(compressed).To(BeFalse())
			Expect(cm.Data).To(HaveKeyWithValue(key, string(small)))
			Expect(cm.BinaryData).NotTo(HaveKey(gzKey))
		})

		It("stores payloads exactly at the threshold as plaintext", func() {
			cm := &corev1.ConfigMap{}
			atLimit := make([]byte, ManifestConfigMapMaxPlaintextBytes)

			compressed, err := SetManifestConfigMapData(cm, key, atLimit)
			Expect(err).NotTo(HaveOccurred())
			Expect(compressed).To(BeFalse())
			Expect(cm.Data).To(HaveKey(key))
			Expect(cm.BinaryData).NotTo(HaveKey(gzKey))
		})

		It("gzip-compresses payloads above the threshold into BinaryData", func() {
			cm := &corev1.ConfigMap{}
			// Repetitive content compresses well and exceeds the plaintext threshold.
			large := []byte(strings.Repeat("a", ManifestConfigMapMaxPlaintextBytes+1))

			compressed, err := SetManifestConfigMapData(cm, key, large)
			Expect(err).NotTo(HaveOccurred())
			Expect(compressed).To(BeTrue())
			Expect(cm.Data).NotTo(HaveKey(key))
			Expect(cm.BinaryData).To(HaveKey(gzKey))
			Expect(len(cm.BinaryData[gzKey])).To(BeNumerically("<", len(large)))
		})

		It("removes a stale plaintext entry when switching to compressed", func() {
			cm := &corev1.ConfigMap{Data: map[string]string{key: "stale plaintext"}}
			large := []byte(strings.Repeat("b", ManifestConfigMapMaxPlaintextBytes+1))

			compressed, err := SetManifestConfigMapData(cm, key, large)
			Expect(err).NotTo(HaveOccurred())
			Expect(compressed).To(BeTrue())
			Expect(cm.Data).NotTo(HaveKey(key))
			Expect(cm.BinaryData).To(HaveKey(gzKey))
		})

		It("removes a stale compressed entry when switching to plaintext", func() {
			cm := &corev1.ConfigMap{BinaryData: map[string][]byte{gzKey: []byte("stale gz")}}
			small := []byte("now small")

			compressed, err := SetManifestConfigMapData(cm, key, small)
			Expect(err).NotTo(HaveOccurred())
			Expect(compressed).To(BeFalse())
			Expect(cm.BinaryData).NotTo(HaveKey(gzKey))
			Expect(cm.Data).To(HaveKeyWithValue(key, string(small)))
		})
	})

	Describe("GetManifestConfigMapData", func() {
		const key = "model_manifest.yaml"
		const gzKey = "model_manifest.yaml" + GzipCompressedKeySuffix

		It("reads plaintext content from Data", func() {
			cm := &corev1.ConfigMap{Data: map[string]string{key: "plain content"}}
			data, ok, err := GetManifestConfigMapData(cm, key)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(data).To(Equal([]byte("plain content")))
		})

		It("reads and decompresses content from BinaryData", func() {
			gz, err := CompressData([]byte("compressed content"))
			Expect(err).NotTo(HaveOccurred())
			cm := &corev1.ConfigMap{BinaryData: map[string][]byte{gzKey: gz}}

			data, ok, err := GetManifestConfigMapData(cm, key)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(data).To(Equal([]byte("compressed content")))
		})

		It("prefers the compressed entry when both are present", func() {
			gz, err := CompressData([]byte("compressed wins"))
			Expect(err).NotTo(HaveOccurred())
			cm := &corev1.ConfigMap{
				Data:       map[string]string{key: "plaintext loses"},
				BinaryData: map[string][]byte{gzKey: gz},
			}

			data, ok, err := GetManifestConfigMapData(cm, key)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(data).To(Equal([]byte("compressed wins")))
		})

		It("reports not found when neither representation exists", func() {
			cm := &corev1.ConfigMap{}
			_, ok, err := GetManifestConfigMapData(cm, key)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeFalse())
		})

		It("returns an error for corrupt compressed content", func() {
			cm := &corev1.ConfigMap{BinaryData: map[string][]byte{gzKey: []byte("not gzip")}}
			_, ok, err := GetManifestConfigMapData(cm, key)
			Expect(err).To(HaveOccurred())
			Expect(ok).To(BeTrue())
		})
	})

	Describe("Round-trip via Set then Get", func() {
		const key = "model_manifest.yaml"

		DescribeTable("preserves content across the storage boundary",
			func(size int) {
				cm := &corev1.ConfigMap{}
				original := []byte(strings.Repeat("x", size))

				_, err := SetManifestConfigMapData(cm, key, original)
				Expect(err).NotTo(HaveOccurred())

				got, ok, err := GetManifestConfigMapData(cm, key)
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).To(BeTrue())
				Expect(got).To(Equal(original))
			},
			Entry("tiny plaintext", 16),
			Entry("just below threshold", ManifestConfigMapMaxPlaintextBytes-1),
			Entry("just above threshold (compressed)", ManifestConfigMapMaxPlaintextBytes+1),
			Entry("well above threshold (compressed)", 2*ManifestConfigMapMaxPlaintextBytes),
		)
	})
})
