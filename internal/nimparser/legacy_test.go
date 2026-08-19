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

package nimparser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeLegacyManifestMolMIM(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "manifest_legacy_molmim.yaml"))
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	normalized, isLegacy, err := NormalizeLegacyManifest(data)
	if err != nil {
		t.Fatalf("NormalizeLegacyManifest returned error: %v", err)
	}
	if !isLegacy {
		t.Fatal("expected legacy manifest to be detected")
	}
	if len(normalized) == 0 {
		t.Fatal("expected normalized YAML bytes")
	}
}

func TestNormalizeLegacyManifestPassthroughV1(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("v1", "testdata", "manifest_non_llm.yaml"))
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	normalized, isLegacy, err := NormalizeLegacyManifest(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isLegacy {
		t.Fatalf("v1 manifest incorrectly detected as legacy: %s", normalized)
	}
}

func TestNormalizeLegacyManifestPassthroughV2(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("v2", "testdata", "manifest_v2.yaml"))
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	_, isLegacy, err := NormalizeLegacyManifest(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isLegacy {
		t.Fatal("v2 manifest incorrectly detected as legacy")
	}
}

func TestNormalizeLegacyManifestInvalidList(t *testing.T) {
	data := []byte("model_profile: MolMIM\nmanifest: not-a-list\n")
	_, isLegacy, err := NormalizeLegacyManifest(data)
	if !isLegacy {
		t.Fatal("expected legacy detection")
	}
	if err == nil {
		t.Fatal("expected error for non-list manifest")
	}
}
