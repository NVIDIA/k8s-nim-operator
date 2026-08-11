package v1

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NVIDIA/k8s-nim-operator/internal/nimparser"
)

func TestParseRealMolMIM100Manifest(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "testdata", "manifest_molmim_1.0.0_real.yaml"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	normalized, isLegacy, err := nimparser.NormalizeLegacyManifest(data)
	if err != nil {
		t.Fatalf("NormalizeLegacyManifest error: %v", err)
	}
	if !isLegacy {
		t.Fatal("expected real MolMIM manifest to be detected as legacy")
	}
	t.Logf("normalized:\n%s", string(normalized))

	manifest, err := NIMParser{}.ParseModelManifestFromRawOutput(data)
	if err != nil {
		t.Fatalf("ParseModelManifestFromRawOutput error: %v", err)
	}

	profiles := manifest.GetProfilesList()
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %v", profiles)
	}
	if profiles[0] != "MolMIM" {
		t.Fatalf("expected profile id MolMIM, got %q", profiles[0])
	}
	if got := manifest.GetProfileModel("MolMIM"); got != "MolMIM" {
		t.Fatalf("model = %q, want MolMIM", got)
	}
	if got := manifest.GetProfileRelease("MolMIM"); got != "1.0.0" {
		t.Fatalf("release = %q, want 1.0.0", got)
	}
	tags := manifest.GetProfileTags("MolMIM")
	if tags["backend"] != "pytorch" {
		t.Fatalf("tags = %v, want backend=pytorch", tags)
	}
}
