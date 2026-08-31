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

package controller

import (
	"strings"
	"testing"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

func TestParseProbeOutput(t *testing.T) {
	output := strings.Join([]string{
		"NIM_OPERATOR_CAP download_to_cache=no",
		"NIM_OPERATOR_CAP nimlib_download=yes",
		"model_profile: MolMIM",
		"manifest:",
		"  - id: MolMIM",
	}, "\n")

	caps, manifest := parseProbeOutput(output)
	if !caps.Complete() {
		t.Fatalf("Complete() = false, want true")
	}
	if caps.HasDownloadToCache {
		t.Fatalf("HasDownloadToCache = true, want false")
	}
	if !caps.HasNimlibDownload {
		t.Fatalf("HasNimlibDownload = false, want true")
	}
	if !strings.Contains(manifest, "model_profile: MolMIM") {
		t.Fatalf("manifest missing model_profile: %q", manifest)
	}
	if strings.Contains(manifest, "NIM_OPERATOR_CAP") {
		t.Fatalf("manifest still contains capability markers: %q", manifest)
	}
}

func TestParseProbeOutputIncomplete(t *testing.T) {
	// Partial log read can capture only the first capability marker.
	caps, manifest := parseProbeOutput("NIM_OPERATOR_CAP download_to_cache=no\n")
	if caps.Complete() {
		t.Fatal("Complete() = true for partial probe output, want false")
	}
	if !caps.SawDownloadToCache {
		t.Fatal("SawDownloadToCache = false, want true")
	}
	if caps.SawNimlibDownload {
		t.Fatal("SawNimlibDownload = true, want false")
	}
	if strings.TrimSpace(manifest) != "" {
		t.Fatalf("manifest = %q, want empty", manifest)
	}
}

func TestResolveDownloadMethod(t *testing.T) {
	t.Run("prefers download-to-cache", func(t *testing.T) {
		method, err := resolveDownloadMethod(probeCapabilities{HasDownloadToCache: true, HasNimlibDownload: true, SawDownloadToCache: true, SawNimlibDownload: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if method != appsv1alpha1.NIMCacheDownloadMethodCLI {
			t.Fatalf("method = %q, want %q", method, appsv1alpha1.NIMCacheDownloadMethodCLI)
		}
	})

	t.Run("falls back to nimlib", func(t *testing.T) {
		method, err := resolveDownloadMethod(probeCapabilities{HasNimlibDownload: true, SawDownloadToCache: true, SawNimlibDownload: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if method != appsv1alpha1.NIMCacheDownloadMethodNIMLib {
			t.Fatalf("method = %q, want %q", method, appsv1alpha1.NIMCacheDownloadMethodNIMLib)
		}
	})

	t.Run("errors when neither capability is present", func(t *testing.T) {
		_, err := resolveDownloadMethod(probeCapabilities{SawDownloadToCache: true, SawNimlibDownload: true})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "neither download-to-cache nor nimlib") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestNimlibDownloadCommand(t *testing.T) {
	t.Run("all profiles", func(t *testing.T) {
		cmd := nimlibDownloadCommand([]string{AllProfiles})
		want := []string{"python3", "-c", "from nimlib import nimutils; nimutils.download_models()"}
		if len(cmd) != len(want) {
			t.Fatalf("cmd = %#v, want %#v", cmd, want)
		}
		for i := range want {
			if cmd[i] != want[i] {
				t.Fatalf("cmd = %#v, want %#v", cmd, want)
			}
		}
	})

	t.Run("selected profiles", func(t *testing.T) {
		cmd := nimlibDownloadCommand([]string{"MolMIM"})
		if len(cmd) != 3 || cmd[0] != "sh" || cmd[1] != "-c" {
			t.Fatalf("unexpected command: %#v", cmd)
		}
		if !strings.Contains(cmd[2], "NIM_MANIFEST_PROFILE=MolMIM") {
			t.Fatalf("script missing profile env: %q", cmd[2])
		}
		if !strings.Contains(cmd[2], "from nimlib import nimutils; nimutils.download_models()") {
			t.Fatalf("script missing download call: %q", cmd[2])
		}
	})
}

func TestUsesNimlibDownload(t *testing.T) {
	n := &appsv1alpha1.NIMCache{}
	if n.UsesNimlibDownload() {
		t.Fatal("empty status should not report nimlib download")
	}
	n.Status.DownloadMethod = appsv1alpha1.NIMCacheDownloadMethodNIMLib
	if !n.UsesNimlibDownload() {
		t.Fatal("expected UsesNimlibDownload true")
	}
}
