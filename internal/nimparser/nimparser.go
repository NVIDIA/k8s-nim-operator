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
	"os"
	"strconv"
	"strings"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/v1alpha1"
	"gopkg.in/yaml.v2"
)

// File represents the model files
type File struct {
	Name string `yaml:"name" json:"name,omitempty"`
}

// Src represents model source
type Src struct {
	RepoID string `yaml:"repo_id" json:"repo_id,omitempty"`
	Files  []File `yaml:"files" json:"files,omitempty"`
}

// Component represents source and destination for model files
type Component struct {
	Dst string `yaml:"dst" json:"dst,omitempty"`
	Src Src    `yaml:"src" json:"src,omitempty"`
}

// Workspace represents workspace for model components
type Workspace struct {
	Components []Component `yaml:"components" json:"components,omitempty"`
}

// NIMProfile is the model profile supported by the NIM container
type NIMProfile struct {
	Model        string            `yaml:"model" json:"model,omitempty"`
	Release      string            `yaml:"release" json:"release,omitempty"`
	Tags         map[string]string `yaml:"tags" json:"tags,omitempty"`
	ContainerURL string            `yaml:"container_url" json:"container_url,omitempty"`
	Workspace    Workspace         `yaml:"workspace" json:"workspace,omitempty"`
}

// NIMManifest is the model manifest file
type NIMManifest map[string]NIMProfile

// UnmarshalYAML is the custom unmarshal function for Src
func (s *Src) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var raw map[string]interface{}
	if err := unmarshal(&raw); err != nil {
		return err
	}

	if repoID, ok := raw["repo_id"].(string); ok {
		s.RepoID = repoID
	}

	if files, ok := raw["files"].([]interface{}); ok {
		for _, file := range files {
			if fileStr, ok := file.(string); ok {
				s.Files = append(s.Files, File{Name: fileStr})
			} else if fileMap, ok := file.(map[interface{}]interface{}); ok {
				for k := range fileMap {
					if fileName, ok := k.(string); ok {
						s.Files = append(s.Files, File{Name: fileName})
					}
				}
			}
		}
	}

	return nil
}

// UnmarshalYAML unmarshalls given yaml data into NIM manifest struct
func (f *File) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var name string
	if err := unmarshal(&name); err != nil {
		return err
	}
	f.Name = name
	return nil
}

// ParseModelManifest parses the given NIM manifest yaml file
func ParseModelManifest(filePath string) (*NIMManifest, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var config NIMManifest
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// ParseModelManifestFromRawOutput parses the given raw NIM manifest data
func ParseModelManifestFromRawOutput(data []byte) (*NIMManifest, error) {
	var config NIMManifest
	err := yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// MatchProfiles matches the given model parameters with the profiles in the manifest
func MatchProfiles(modelSpec appsv1alpha1.ModelSpec, manifest NIMManifest, discoveredGPUs []string) ([]string, error) {
	var selectedProfiles []string

	for hash, profile := range manifest {
		if modelSpec.Precision != "" && profile.Tags["precision"] != modelSpec.Precision {
			// continue with the next profile in the manifest
			continue
		}
		if modelSpec.TensorParallelism != "" && profile.Tags["tp"] != modelSpec.TensorParallelism {
			// continue with the next profile in the manifest
			continue
		}
		if modelSpec.QoSProfile != "" && profile.Tags["profile"] != modelSpec.QoSProfile {
			// continue with the next profile in the manifest
			continue
		}
		if modelSpec.Lora != nil && profile.Tags["feat_lora"] != strconv.FormatBool(*modelSpec.Lora) {
			// continue with the next profile in the manifest
			continue
		}
		if modelSpec.Engine != "" && profile.Tags["llm_engine"] != modelSpec.Engine {
			// continue with the next profile in the manifest
			continue
		}

		foundGPU := false
		foundID := false

		for _, gpu := range modelSpec.GPUs {
			if gpu.Product != "" && strings.Contains(strings.ToLower(gpu.Product), strings.ToLower(profile.Tags["gpu"])) {
				foundGPU = true
			}

			if len(gpu.IDs) > 0 {
				for _, id := range gpu.IDs {
					if id == strings.TrimSuffix(profile.Tags["gpu_device"], ":10de") {
						foundID = true
						break
					}
				}
				if foundID {
					break
				}
			}

			if foundGPU && (len(gpu.IDs) == 0 || foundID) {
				break
			} else {
				// toggle flag as none of the specified ID's match with the profile
				foundGPU = false
			}
		}

		if !foundGPU {
			// didn't match any GPUs from the model spec, match using discovered GPUs
			if len(discoveredGPUs) > 0 {
				foundGPU = false
				for _, product := range discoveredGPUs {
					if strings.Contains(strings.ToLower(product), strings.ToLower(profile.Tags["gpu"])) {
						foundGPU = true
						break
					}
				}
			}
			if !foundGPU {
				// continue with next profile
				continue
			}
		}

		// profile matched with the given model parameters, add hash to the selected profiles
		selectedProfiles = append(selectedProfiles, hash)
		// contine with next profile in the manifest
	}
	return selectedProfiles, nil
}
