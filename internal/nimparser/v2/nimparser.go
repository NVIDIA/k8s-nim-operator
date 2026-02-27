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
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/nimparser"
)

const (
	// BackendTypeTensorRT indicates tensortt backend.
	BackendTypeTensorRT = "tensorrt"
)

// Uri represents model source.
type Uri struct {
	Uri string `yaml:"uri" json:"uri,omitempty"`
}

// Workspace represents workspace for model components.
type Workspace struct {
	Files map[string]Uri `yaml:"files" json:"files,omitempty"`
}

// NIMProfile is the model profile supported by the NIM container.
type NIMProfile struct {
	ID        string            `yaml:"id" json:"id,omitempty"`
	Tags      map[string]string `yaml:"tags" json:"tags,omitempty"`
	Workspace Workspace         `yaml:"workspace" json:"workspace,omitempty"`
}

// NIMManifest is the model manifest file.
type NIMManifest struct {
	SchemaVersion            string       `yaml:"schema_version" json:"schema_version,omitempty"`
	ProfileSelectionCriteria string       `yaml:"profile_selection_criteria" json:"profile_selection_criteria,omitempty"`
	Profiles                 []NIMProfile `yaml:"profiles" json:"profiles,omitempty"`
}

func (manifest NIMManifest) GetProfilesList() []string {

	profileIDs := make([]string, len(manifest.Profiles))

	for k, profile := range manifest.Profiles {
		profileIDs[k] = profile.ID
	}
	return profileIDs
}

func (manifest NIMManifest) GetProfileModel(profileID string) string {
	return ""
}

func (manifest NIMManifest) GetProfileTags(profileID string) map[string]string {
	for _, profile := range manifest.Profiles {
		if profileID == profile.ID {
			return profile.Tags
		}
	}
	return nil
}

func (manifest NIMManifest) GetProfileRelease(profileID string) string {
	return ""
}

type NIMParser struct{}

func (NIMParser) ParseModelManifest(filePath string) (nimparser.NIMManifestInterface, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var config NIMManifest
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}
	return config, nil

}

func (NIMParser) ParseModelManifestFromRawOutput(data []byte) (nimparser.NIMManifestInterface, error) {
	var config NIMManifest
	err := yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

// MatchProfiles returns all manifest profile IDs that satisfy modelSpec and GPU filters.
func (manifest NIMManifest) MatchProfiles(modelSpec appsv1alpha1.ModelSpec, discoveredGPUs []string) ([]string, error) {
	var selected []string

	for _, p := range manifest.Profiles {
		// Match with all non-GPU filters
		if !matchNonGPUFilters(modelSpec, p) {
			continue
		}

		// Engine selection
		if !matchEngine(modelSpec.Engine, p) {
			continue
		}

		// GPU filtering
		gpuOK := matchGPUProfile(modelSpec, p, discoveredGPUs)

		// Exception: if GPU selection fails but profile is buildable, include it
		if !gpuOK && strings.EqualFold(p.Tags["trtllm_buildable"], "true") {
			selected = append(selected, p.ID)
			continue
		}

		if gpuOK {
			selected = append(selected, p.ID)
		}
	}

	return selected, nil
}

func matchesRegex(productLabel, regexPattern string) (bool, error) {
	// If regexPattern is empty, return false
	if regexPattern == "" {
		return false, nil
	}

	// Compile the regex pattern
	regex, err := regexp.Compile(regexPattern)
	if err != nil {
		return false, err
	}

	// Check if the productLabel matches the regex
	return regex.MatchString(productLabel), nil
}

func matchNonGPUFilters(modelSpec appsv1alpha1.ModelSpec, p NIMProfile) bool {
	// precision
	if modelSpec.Precision != "" && !strings.EqualFold(p.Tags["precision"], modelSpec.Precision) {
		return false
	}
	// tensor parallelism
	if modelSpec.TensorParallelism != "" && !strings.EqualFold(p.Tags["tp"], modelSpec.TensorParallelism) {
		return false
	}
	// QoS profile
	if modelSpec.QoSProfile != "" && !strings.EqualFold(p.Tags["profile"], modelSpec.QoSProfile) {
		return false
	}
	// LoRA
	if modelSpec.Lora != nil && !boolTagEquals(p.Tags["feat_lora"], *modelSpec.Lora) {
		return false
	}
	// Buildable profile
	if modelSpec.Buildable != nil && !boolTagEquals(p.Tags["trtllm_buildable"], *modelSpec.Buildable) {
		return false
	}
	return true
}

func matchEngine(requestedEngine string, p NIMProfile) bool {
	// Determine backend from tags
	backend := p.Tags["llm_engine"]
	if backend == "" {
		backend = p.Tags["model_type"]
	}
	if backend == "" {
		// deprecated fallback
		backend = p.Tags["backend"]
	}
	// Map "triton" -> "tensorrt" for non-LLM consistency
	if strings.EqualFold(backend, "triton") {
		backend = "tensorrt"
	}

	if requestedEngine == "" {
		return true
	}
	// Allow matching "tensorrt_llm" against "tensorrt"
	req := strings.TrimSuffix(strings.ToLower(requestedEngine), "_llm")
	return strings.Contains(strings.ToLower(backend), req)
}

func matchGPUProfile(modelSpec appsv1alpha1.ModelSpec, p NIMProfile, discoveredGPUs []string) bool {
	// If the user specified no GPU filters and there are no discovered GPUs or GPU-identifying tags,
	// we consider the profile as non gpu specific
	hasGPUIdentityTags := p.Tags["gpu"] != "" || p.Tags["key"] != "" || p.Tags["product_name_regex"] != ""
	if len(modelSpec.GPUs) == 0 && (!hasGPUIdentityTags || len(discoveredGPUs) == 0) {
		return true
	}

	// If IDs are specified in any of the requested GPUs, enforce gpu_device match
	if hasAnyIDs(modelSpec) {
		tagID := strings.TrimSuffix(p.Tags["gpu_device"], ":10de")
		if tagID == "" {
			return false
		}
		if !anyIDEquals(modelSpec, tagID) {
			return false
		}
	}

	// If Product is specified, require product to match "gpu" or "key"
	if hasAnyProduct(modelSpec) {
		if !anyProductMatchesProfile(modelSpec, p) {
			return false
		}
	}

	// If nothing specified in modelSpec.GPUs matched yet and we have GPU identity tags,
	// try discovered GPUs
	if len(modelSpec.GPUs) == 0 && hasGPUIdentityTags && len(discoveredGPUs) > 0 {
		if discoveredMatchesProfile(discoveredGPUs, p) {
			return true
		}
		// No discovered GPU matched
		return false
	}

	return true
}

func anyProductMatchesProfile(modelSpec appsv1alpha1.ModelSpec, p NIMProfile) bool {
	gpuTag := strings.ToLower(p.Tags["gpu"])
	keyTag := strings.ToLower(p.Tags["key"])

	for _, g := range modelSpec.GPUs {
		if g.Product == "" {
			continue
		}
		prod := strings.ToLower(g.Product)

		// match against "gpu" or "key"
		if (gpuTag != "" && strings.Contains(gpuTag, prod)) ||
			(keyTag != "" && strings.Contains(keyTag, prod)) {
			return true
		}
	}
	return false
}

func discoveredMatchesProfile(discovered []string, p NIMProfile) bool {
	gpuTag := strings.ToLower(p.Tags["gpu"])
	keyTag := strings.ToLower(p.Tags["key"])
	regex := p.Tags["product_name_regex"]

	for _, label := range discovered {
		l := strings.ToLower(label)
		// gpu type match
		if gpuTag != "" && strings.Contains(l, gpuTag) {
			return true
		}
		if keyTag != "" && strings.Contains(l, keyTag) {
			return true
		}
		// product name regex match
		if regex != "" {
			if ok, _ := matchesRegex(label, regex); ok {
				return true
			}
		}
	}
	return false
}

func hasAnyIDs(modelSpec appsv1alpha1.ModelSpec) bool {
	for _, g := range modelSpec.GPUs {
		if len(g.IDs) > 0 {
			return true
		}
	}
	return false
}

func anyIDEquals(modelSpec appsv1alpha1.ModelSpec, tagID string) bool {
	for _, g := range modelSpec.GPUs {
		for _, id := range g.IDs {
			if id == tagID {
				return true
			}
		}
	}
	return false
}

func hasAnyProduct(modelSpec appsv1alpha1.ModelSpec) bool {
	for _, g := range modelSpec.GPUs {
		if g.Product != "" {
			return true
		}
	}
	return false
}

func boolTagEquals(tag string, want bool) bool {
	return strings.EqualFold(tag, strconv.FormatBool(want))
}
