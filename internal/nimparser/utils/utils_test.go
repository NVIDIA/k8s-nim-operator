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

package utils

import (
	"slices"
	"testing"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	nimparserv1 "github.com/NVIDIA/k8s-nim-operator/internal/nimparser/v1"
	nimparserv2 "github.com/NVIDIA/k8s-nim-operator/internal/nimparser/v2"
	"sigs.k8s.io/yaml"
)

func TestGetNIMParserUsesMajorVersion(t *testing.T) {
	cases := []struct {
		version string
		v2      bool
	}{
		{version: "2.0.0", v2: true},
		{version: "2", v2: true},
		{version: "1.0.0", v2: false},
		{version: "1.2.0", v2: false},
		{version: "12.0.0", v2: false},
		{version: "1.0.2", v2: false},
	}

	for _, tc := range cases {
		parser := GetNIMParser([]byte("schema_version: " + tc.version + "\n"))
		_, isV2 := parser.(nimparserv2.NIMParser)
		_, isV1 := parser.(nimparserv1.NIMParser)
		if tc.v2 && !isV2 {
			t.Errorf("schema %s: got %T, want v2", tc.version, parser)
		}
		if !tc.v2 && !isV1 {
			t.Errorf("schema %s: got %T, want v1", tc.version, parser)
		}
	}
}

func TestGetNIMParserIgnoresLeadingRuntimeNoise(t *testing.T) {
	// Pods that extract the manifest may emit container runtime noise (e.g. NVIDIA
	// container toolkit messages) on stdout before the manifest content.
	const noise = "nvidia driver modules are not yet loaded, invoking runc directly\n"

	parser := GetNIMParser([]byte(noise + "schema_version: '2.0'\n"))
	if _, isV2 := parser.(nimparserv2.NIMParser); !isV2 {
		t.Errorf("v2 manifest with runtime noise: got %T, want v2", parser)
	}

	const v1Manifest = "03fdb4d11f01be10c31b00e7c0540e2835e89a0079b483ad2dd3c25c8cc29b61:\n  model: meta/llama3-70b-instruct\n"
	parser = GetNIMParser([]byte(noise + v1Manifest))
	if _, isV1 := parser.(nimparserv1.NIMParser); !isV1 {
		t.Errorf("v1 manifest with runtime noise: got %T, want v1", parser)
	}
}

func TestParseV2ManifestPreservesProfilesRegardlessOfKeyOrder(t *testing.T) {
	const profiles = "profiles:\n- id: h100-profile\n  tags:\n    gpu: h100\n"
	const version = "schema_version: '2.0'\n"
	const criteria = "profile_selection_criteria: auto\n"
	for _, tc := range []struct {
		name string
		data string
	}{
		{name: "schema first", data: version + criteria + profiles},
		{name: "schema middle", data: criteria + version + profiles},
		{name: "schema last", data: criteria + profiles + version},
		{name: "quoted schema last", data: "\"profiles\":\n- id: h100-profile\n  tags:\n    gpu: h100\n'schema_version': '2.0'\n"},
	} {
		for _, prefix := range []struct {
			name string
			data string
		}{
			{name: "stored YAML"},
			{name: "runtime output", data: "nvidia driver modules are not yet loaded, invoking runc directly\n"},
		} {
			t.Run(tc.name+"/"+prefix.name, func(t *testing.T) {
				data := []byte(prefix.data + tc.data)
				manifest, err := GetNIMParser(data).ParseModelManifestFromRawOutput(data)
				if err != nil {
					t.Fatalf("parse manifest: %v", err)
				}
				if got := manifest.GetProfilesList(); !slices.Equal(got, []string{"h100-profile"}) {
					t.Fatalf("profiles = %v, want [h100-profile]", got)
				}
				if got := manifest.GetProfileTags("h100-profile")["gpu"]; got != "h100" {
					t.Errorf("profile GPU = %q, want h100", got)
				}
			})
		}
	}
}

func TestMinimalV2ManifestRoundTripPreservesAutomaticSelection(t *testing.T) {
	manifest := nimparserv2.NIMManifest{
		SchemaVersion:            "2.0",
		ProfileSelectionCriteria: "auto",
		Profiles: []nimparserv2.NIMProfile{
			{ID: "h100-profile", Tags: map[string]string{"gpu": "h100", "llm_engine": "tensorrt_llm", "tp": "1"}},
			{ID: "a100-profile", Tags: map[string]string{"gpu": "a100", "llm_engine": "tensorrt_llm", "tp": "1"}},
		},
	}
	// Use the same serializer as createManifestConfigMap. It can place the
	// schema_version key after profiles, unlike the original container output.
	data, err := yaml.Marshal(manifest.Minimal())
	if err != nil {
		t.Fatalf("marshal minimal manifest: %v", err)
	}
	parsed, err := GetNIMParser(data).ParseModelManifestFromRawOutput(data)
	if err != nil {
		t.Fatalf("parse persisted manifest: %v", err)
	}
	if got := parsed.GetProfilesList(); !slices.Equal(got, []string{"h100-profile", "a100-profile"}) {
		t.Fatalf("persisted profiles = %v, want [h100-profile a100-profile]", got)
	}
	selected, err := parsed.MatchProfiles(appsv1alpha1.ModelSpec{Engine: "tensorrt_llm", TensorParallelism: "1"}, []string{"NVIDIA-H100-80GB-HBM3"})
	if err != nil {
		t.Fatalf("select H100 profile: %v", err)
	}
	if !slices.Equal(selected, []string{"h100-profile"}) {
		t.Errorf("selected profiles = %v, want [h100-profile]", selected)
	}
}

func TestParseV2ManifestDoesNotDiscardMalformedProfiles(t *testing.T) {
	for _, key := range []string{"profiles", "\"profiles\"", "'profiles'"} {
		t.Run(key, func(t *testing.T) {
			data := []byte(key + ": [\nschema_version: '2.0'\n")
			if _, err := GetNIMParser(data).ParseModelManifestFromRawOutput(data); err == nil {
				t.Fatal("expected a YAML error for malformed profiles")
			}
		})
	}
}
