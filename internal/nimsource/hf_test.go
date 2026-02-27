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

package nimsource_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/nimsource"
)

var _ = Describe("HF", func() {
	DescribeTable("HFDownloadToCacheCommand",
		func(src nimsource.HFInterface, outputPath string, expectedCommand []string) {
			result := nimsource.HFDownloadToCacheCommand(src, outputPath)
			Expect(result).To(Equal(expectedCommand))
		},
		Entry("HuggingFaceHubSource model download without revision",
			&appsv1alpha1.HuggingFaceHubSource{
				Endpoint:  "https://huggingface.co",
				Namespace: "nvidia",
				DSHFCommonFields: appsv1alpha1.DSHFCommonFields{
					ModelName:   ptr.To("llama3-7b"),
					AuthSecret:  "hf-token-secret",
					ModelPuller: "huggingface-cli:latest",
					PullSecret:  "ngc-secret",
				},
			},
			"/cache/models",
			[]string{"huggingface-cli", "download", "nvidia/llama3-7b", "--local-dir", "/cache/models", "--repo-type", "model"},
		),
		Entry("HuggingFaceHubSource model download with revision",
			&appsv1alpha1.HuggingFaceHubSource{
				Endpoint:  "https://huggingface.co",
				Namespace: "nvidia",
				DSHFCommonFields: appsv1alpha1.DSHFCommonFields{
					ModelName:   ptr.To("llama3-7b"),
					AuthSecret:  "hf-token-secret",
					ModelPuller: "huggingface-cli:latest",
					PullSecret:  "ngc-secret",
					Revision:    ptr.To("v1.0.0"),
				},
			},
			"/cache/models",
			[]string{"huggingface-cli", "download", "nvidia/llama3-7b", "--local-dir", "/cache/models", "--repo-type", "model", "--revision", "v1.0.0"},
		),
		Entry("HuggingFaceHubSource dataset download without revision",
			&appsv1alpha1.HuggingFaceHubSource{
				Endpoint:  "https://huggingface.co",
				Namespace: "nvidia",
				DSHFCommonFields: appsv1alpha1.DSHFCommonFields{
					DatasetName: ptr.To("test-dataset"),
					AuthSecret:  "hf-token-secret",
					ModelPuller: "huggingface-cli:latest",
					PullSecret:  "ngc-secret",
				},
			},
			"/cache/datasets",
			[]string{"huggingface-cli", "download", "nvidia/test-dataset", "--local-dir", "/cache/datasets", "--repo-type", "dataset"},
		),
		Entry("HuggingFaceHubSource dataset download with revision",
			&appsv1alpha1.HuggingFaceHubSource{
				Endpoint:  "https://huggingface.co",
				Namespace: "nvidia",
				DSHFCommonFields: appsv1alpha1.DSHFCommonFields{
					DatasetName: ptr.To("test-dataset"),
					AuthSecret:  "hf-token-secret",
					ModelPuller: "huggingface-cli:latest",
					PullSecret:  "ngc-secret",
					Revision:    ptr.To("v1.0.0"),
				},
			},
			"/cache/datasets",
			[]string{"huggingface-cli", "download", "nvidia/test-dataset", "--local-dir", "/cache/datasets", "--repo-type", "dataset", "--revision", "v1.0.0"},
		),
		Entry("NemoDataStoreSource model download without revision",
			&appsv1alpha1.NemoDataStoreSource{
				Endpoint:  "https://datastore.nemo.nvidia.com/v1/hf/",
				Namespace: "nvidia",
				DSHFCommonFields: appsv1alpha1.DSHFCommonFields{
					ModelName:   ptr.To("llama3-7b"),
					AuthSecret:  "hf-token-secret",
					ModelPuller: "nvcr.io/nvidia/nemo-microservices/nds-v2-huggingface-cli:latest",
					PullSecret:  "ngc-secret",
				},
			},
			"/cache/models",
			[]string{"huggingface-cli", "download", "nvidia/llama3-7b", "--local-dir", "/cache/models", "--repo-type", "model"},
		),
		Entry("NemoDataStoreSource model download with revision",
			&appsv1alpha1.NemoDataStoreSource{
				Endpoint:  "https://datastore.nemo.nvidia.com/v1/hf/",
				Namespace: "nvidia",
				DSHFCommonFields: appsv1alpha1.DSHFCommonFields{
					ModelName:   ptr.To("llama3-7b"),
					AuthSecret:  "hf-token-secret",
					ModelPuller: "nvcr.io/nvidia/nemo-microservices/nds-v2-huggingface-cli:latest",
					PullSecret:  "ngc-secret",
					Revision:    ptr.To("v1.0.0"),
				},
			},
			"/cache/models",
			[]string{"huggingface-cli", "download", "nvidia/llama3-7b", "--local-dir", "/cache/models", "--repo-type", "model", "--revision", "v1.0.0"},
		),
		Entry("NemoDataStoreSource dataset download without revision",
			&appsv1alpha1.NemoDataStoreSource{
				Endpoint:  "https://datastore.nemo.nvidia.com/v1/hf/",
				Namespace: "nvidia",
				DSHFCommonFields: appsv1alpha1.DSHFCommonFields{
					DatasetName: ptr.To("test-dataset"),
					AuthSecret:  "hf-token-secret",
					ModelPuller: "nvcr.io/nvidia/nemo-microservices/nds-v2-huggingface-cli:latest",
					PullSecret:  "ngc-secret",
				},
			},
			"/cache/datasets",
			[]string{"huggingface-cli", "download", "nvidia/test-dataset", "--local-dir", "/cache/datasets", "--repo-type", "dataset"},
		),
		Entry("NemoDataStoreSource dataset download with revision",
			&appsv1alpha1.NemoDataStoreSource{
				Endpoint:  "https://datastore.nemo.nvidia.com/v1/hf/",
				Namespace: "nvidia",
				DSHFCommonFields: appsv1alpha1.DSHFCommonFields{
					DatasetName: ptr.To("test-dataset"),
					AuthSecret:  "hf-token-secret",
					ModelPuller: "nvcr.io/nvidia/nemo-microservices/nds-v2-huggingface-cli:latest",
					PullSecret:  "ngc-secret",
					Revision:    ptr.To("v1.0.0"),
				},
			},
			"/cache/datasets",
			[]string{"huggingface-cli", "download", "nvidia/test-dataset", "--local-dir", "/cache/datasets", "--repo-type", "dataset", "--revision", "v1.0.0"},
		),
	)
})
