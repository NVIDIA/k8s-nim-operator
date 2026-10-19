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
	"regexp"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

const (
	// BackendTypeTensorRT indicates tensortt backend.
	BackendTypeTensorRT = "tensorrt"
)

type NIMSchemaManifest struct {
	SchemaVersion string `yaml:"schema_version" json:"schema_version,omitempty"`
}

var (
	// v2 manifests begin with a schema_version key at the document root.
	v2ManifestStartPattern = regexp.MustCompile(`(?m)^schema_version\s*:`)
	// v1 manifests are a map keyed by 64-char hex profile hashes. Profile IDs
	// nested inside v2 manifests are indented, so they never match this pattern.
	v1ManifestStartPattern = regexp.MustCompile(`(?m)^[0-9a-f]{64}:`)
)

// ExtractManifest returns the model manifest portion of raw output captured from
// container logs, skipping any leading runtime noise lines (e.g. NVIDIA container
// toolkit messages) that precede the manifest document. If no manifest start is
// found, the input is returned unchanged so parse errors surface as-is.
func ExtractManifest(data []byte) []byte {
	start := -1
	for _, pattern := range []*regexp.Regexp{v2ManifestStartPattern, v1ManifestStartPattern} {
		if loc := pattern.FindIndex(data); loc != nil && (start == -1 || loc[0] < start) {
			start = loc[0]
		}
	}
	if start > 0 {
		return data[start:]
	}
	return data
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
	// Minimal returns a copy of the manifest with bulky fields removed (e.g. workspace
	// file lists) while retaining the metadata needed for profile matching and status.
	// The result is safe to persist in a ConfigMap without exceeding etcd size limits.
	Minimal() NIMManifestInterface
}
