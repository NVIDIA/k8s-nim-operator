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
	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

const (
	// BackendTypeTensorRT indicates tensortt backend.
	BackendTypeTensorRT = "tensorrt"
)

type NIMSchemaManifest struct {
	SchemaVersion string `yaml:"schema_version" json:"schema_version,omitempty"`
}

type NIMParserInterface interface {
	ParseModelManifest(filePath string) (NIMManifestInterface, error)
	ParseModelManifestFromRawOutput(data []byte) (NIMManifestInterface, error)
}

type NIMManifestInterface interface {
	MatchProfiles(modelSpec appsv1alpha1.ModelSpec, discoveredGPUs []string) ([]string, error)
	GetProfilesList() []string
	GetProfileModel(profileID string) string
	GetProfileTags(profileID string) map[string]string
	GetProfileRelease(profileID string) string
}
