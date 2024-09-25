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

package nimparser

import (
	"path/filepath"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"

	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NIMParser", func() {

	Context("ParseModelManifest", func() {
		It("should parse a model profile for trtllm engine files correctly", func() {

			filePath := filepath.Join("testdata", "manifest_trtllm.yaml")
			config, err := ParseModelManifest(filePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(*config).To(HaveLen(1))

			profile, exists := (*config)["03fdb4d11f01be10c31b00e7c0540e2835e89a0079b483ad2dd3c25c8cc29b61"]
			Expect(exists).To(BeTrue())
			Expect(profile.Model).To(Equal("meta/llama3-70b-instruct"))
			Expect(profile.Tags["llm_engine"]).To(Equal("tensorrt_llm"))
			Expect(profile.Tags["precision"]).To(Equal("fp16"))
			Expect(profile.ContainerURL).To(Equal("nvcr.io/nim/meta/llama3-70b-instruct:1.0.0"))
		})
		It("should parse a model profile for vllm engine files correctly", func() {

			filePath := filepath.Join("testdata", "manifest_vllm.yaml")
			config, err := ParseModelManifest(filePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(*config).To(HaveLen(1))

			profile, exists := (*config)["0f3de1afe11d355e01657424a267fbaad19bfea3143a9879307c49aed8299db0"]
			Expect(exists).To(BeTrue())
			Expect(profile.Model).To(Equal("meta/llama3-70b-instruct"))
			Expect(profile.Tags["llm_engine"]).To(Equal("vllm"))
			Expect(profile.Tags["precision"]).To(Equal("fp16"))
			Expect(profile.ContainerURL).To(Equal("nvcr.io/nim/meta/llama3-70b-instruct:1.0.0"))
		})
		It("should parse a model profile with lora adapters correctly", func() {

			filePath := filepath.Join("testdata", "manifest_lora.yaml")
			config, err := ParseModelManifest(filePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(*config).To(HaveLen(1))

			profile, exists := (*config)["36fc1fa4fc35c1d54da115a39323080b08d7937dceb8ba47be44f4da0ec720ff"]
			Expect(exists).To(BeTrue())
			Expect(profile.Model).To(Equal("meta/llama3-70b-instruct"))
			Expect(profile.Tags["feat_lora"]).To(Equal("true"))
			Expect(profile.Tags["feat_lora_max_rank"]).To(Equal("32"))
			Expect(profile.Tags["precision"]).To(Equal("fp16"))
			Expect(profile.ContainerURL).To(Equal("nvcr.io/nim/meta/llama3-70b-instruct:1.0.0"))
		})
		It("should match model profiles with valid parameters", func() {
			filePath := filepath.Join("testdata", "manifest_trtllm.yaml")
			config, err := ParseModelManifest(filePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(*config).To(HaveLen(1))

			profile, exists := (*config)["03fdb4d11f01be10c31b00e7c0540e2835e89a0079b483ad2dd3c25c8cc29b61"]
			Expect(exists).To(BeTrue())
			Expect(profile.Model).To(Equal("meta/llama3-70b-instruct"))
			Expect(profile.Tags["llm_engine"]).To(Equal("tensorrt_llm"))
			Expect(profile.Tags["precision"]).To(Equal("fp16"))
			Expect(profile.ContainerURL).To(Equal("nvcr.io/nim/meta/llama3-70b-instruct:1.0.0"))
			// Add valid spec to match
			modelSpec := appsv1alpha1.ModelSpec{Precision: "fp16",
				Engine:            "tensorrt_llm",
				QoSProfile:        "throughput",
				TensorParallelism: "8",
				GPUs: []appsv1alpha1.GPUSpec{{Product: "l40s",
					IDs: []string{"26b5"}},
				},
			}
			matchedProfiles, err := MatchProfiles(modelSpec, *config, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(matchedProfiles).NotTo(BeEmpty())
			Expect(matchedProfiles).To(HaveLen(1))
		})
		It("should not match model profiles with invalid parameters", func() {
			filePath := filepath.Join("testdata", "manifest_trtllm.yaml")
			config, err := ParseModelManifest(filePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(*config).To(HaveLen(1))

			modelSpec := appsv1alpha1.ModelSpec{Precision: "fp16",
				Engine:            "tensorrt_llm",
				QoSProfile:        "throughput",
				TensorParallelism: "8",
				GPUs: []appsv1alpha1.GPUSpec{{Product: "l40s",
					IDs: []string{"abcd"}}, // invalid entry
				},
			}
			matchedProfiles, err := MatchProfiles(modelSpec, *config, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(matchedProfiles).To(BeEmpty())
		})
		It("should match model profiles using automatically discovered GPUs", func() {
			filePath := filepath.Join("testdata", "manifest_trtllm.yaml")
			config, err := ParseModelManifest(filePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(*config).To(HaveLen(1))

			// Match using discovered GPUs (node product labels)
			modelSpec := appsv1alpha1.ModelSpec{Precision: "fp16",
				Engine:            "tensorrt_llm",
				QoSProfile:        "throughput",
				TensorParallelism: "8",
			}
			matchedProfiles, err := MatchProfiles(modelSpec, *config, []string{"NVIDIA-L40S-48C"})
			Expect(err).NotTo(HaveOccurred())
			Expect(matchedProfiles).NotTo(BeEmpty())
			Expect(matchedProfiles).To(HaveLen(1))
		})
		It("should match model profiles when lora is enabled", func() {
			filePath := filepath.Join("testdata", "manifest_lora.yaml")
			config, err := ParseModelManifest(filePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(*config).To(HaveLen(1))

			// Match using Lora
			modelSpec := appsv1alpha1.ModelSpec{
				Lora: utils.BoolPtr(true),
			}
			matchedProfiles, err := MatchProfiles(modelSpec, *config, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(matchedProfiles).To(HaveLen(1))
		})
		It("should not match model profiles when lora is not provided and profile has lora enabled", func() {
			filePath := filepath.Join("testdata", "manifest_lora.yaml")
			config, err := ParseModelManifest(filePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(*config).To(HaveLen(1))

			modelSpec := appsv1alpha1.ModelSpec{
				Engine: "tensorrt_llm",
			}
			matchedProfiles, err := MatchProfiles(modelSpec, *config, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(matchedProfiles).To(BeEmpty())
		})

		It("should match model profiles with different engine parameters for non-llm manifest", func() {
			filePath := filepath.Join("testdata", "manifest_non_llm.yaml")
			config, err := ParseModelManifest(filePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(*config).To(HaveLen(2))

			// Match using backend
			modelSpec := appsv1alpha1.ModelSpec{
				Engine: "tensorrt", // instead of tensorrt_llm for llm nims
			}
			matchedProfiles, err := MatchProfiles(modelSpec, *config, []string{"NVIDIA-A10G"})
			Expect(err).NotTo(HaveOccurred())
			Expect(matchedProfiles).To(HaveLen(1))
		})
		It("should match model profiles with different gpu parameters for non-llm manifest", func() {
			filePath := filepath.Join("testdata", "manifest_non_llm.yaml")
			config, err := ParseModelManifest(filePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(*config).To(HaveLen(2))

			// Match using GPU product name
			modelSpec := appsv1alpha1.ModelSpec{
				GPUs: []appsv1alpha1.GPUSpec{{Product: "A10G"}},
			}
			matchedProfiles, err := MatchProfiles(modelSpec, *config, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(matchedProfiles).To(HaveLen(1))
		})
	})
})
