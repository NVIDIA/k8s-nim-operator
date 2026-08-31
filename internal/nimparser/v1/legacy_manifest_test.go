/*
Copyright 2026.

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

package v1

import (
	"os"
	"path/filepath"
	"testing"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

func TestParseLegacyMolMIMManifest(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "testdata", "manifest_legacy_molmim.yaml"))
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	manifest, err := NIMParser{}.ParseModelManifestFromRawOutput(data)
	if err != nil {
		t.Fatalf("ParseModelManifestFromRawOutput returned error: %v", err)
	}

	profiles := manifest.GetProfilesList()
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d (%v)", len(profiles), profiles)
	}

	const profileID = "a1b2c3d4e5f60718293a4b5c6d7e8f90123456789abcdef0123456789abcdef0"
	if got := manifest.GetProfileModel(profileID); got != "MolMIM" {
		t.Fatalf("GetProfileModel(%q) = %q, want MolMIM", profileID, got)
	}
	tags := manifest.GetProfileTags(profileID)
	if tags["backend"] != "tensorrt" {
		t.Fatalf("expected backend=tensorrt, got %q", tags["backend"])
	}

	selected, err := manifest.MatchProfiles(appsv1alpha1.ModelSpec{
		Engine: "tensorrt",
		GPUs: []appsv1alpha1.GPUSpec{{
			Product: "A100",
		}},
	}, nil)
	if err != nil {
		t.Fatalf("MatchProfiles returned error: %v", err)
	}
	if len(selected) != 1 || selected[0] != profileID {
		t.Fatalf("unexpected selected profiles: %v", selected)
	}
}

func TestParseLegacySingleKeyManifest(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "testdata", "manifest_legacy_single_key.yaml"))
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	manifest, err := NIMParser{}.ParseModelManifestFromRawOutput(data)
	if err != nil {
		t.Fatalf("ParseModelManifestFromRawOutput returned error: %v", err)
	}

	const profileID = "f1e2d3c4b5a697887766554433221100ffeeddccbbaa99887766554433221100"
	profiles := manifest.GetProfilesList()
	if len(profiles) != 1 || profiles[0] != profileID {
		t.Fatalf("unexpected profiles list: %v", profiles)
	}
	if got := manifest.GetProfileModel(profileID); got != "MolMIM" {
		t.Fatalf("GetProfileModel(%q) = %q, want MolMIM", profileID, got)
	}
}
