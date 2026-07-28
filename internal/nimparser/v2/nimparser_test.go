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

package v2

import (
	"path/filepath"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NIMParser V2", func() {
	Context("Minimal", func() {
		It("should return a minimal manifest without workspace data", func() {
			filePath := filepath.Join("testdata", "manifest_v2.yaml")
			parser := NIMParser{}
			config, err := parser.ParseModelManifest(filePath)
			Expect(err).NotTo(HaveOccurred())
			nimManifest, ok := config.(NIMManifest)
			Expect(ok).To(BeTrue())

			Expect(nimManifest.Profiles).NotTo(BeEmpty())
			Expect(nimManifest.Profiles[0].Workspace).NotTo(BeNil())
			Expect(nimManifest.Profiles[0].Workspace.Files).NotTo(BeEmpty())

			minimalIface := nimManifest.Minimal()
			minimal, ok := minimalIface.(NIMManifest)
			Expect(ok).To(BeTrue())

			Expect(minimal.SchemaVersion).To(Equal("2.0.0"))
			Expect(minimal.ProfileSelectionCriteria).To(Equal("default"))
			Expect(minimal.Profiles).To(HaveLen(2))
			Expect(minimal.Profiles[0].ID).To(Equal("aaaabbbbccccddddeeeeffff0000111122223333444455556666777788889999"))
			Expect(minimal.Profiles[0].Tags["llm_engine"]).To(Equal("tensorrt_llm"))
			Expect(minimal.Profiles[0].Workspace).To(BeNil())
			Expect(minimal.Profiles[1].Workspace).To(BeNil())

			// Minimal must still support profile matching.
			modelSpec := appsv1alpha1.ModelSpec{
				Precision:         "fp16",
				Engine:            "tensorrt_llm",
				QoSProfile:        "throughput",
				TensorParallelism: "8",
				GPUs:              []appsv1alpha1.GPUSpec{{Product: "l40s", IDs: []string{"26b5"}}},
			}
			matchedProfiles, err := minimal.MatchProfiles(modelSpec, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(matchedProfiles).To(Equal([]string{"aaaabbbbccccddddeeeeffff0000111122223333444455556666777788889999"}))
		})
	})
})
