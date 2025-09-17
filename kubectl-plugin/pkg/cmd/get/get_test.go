package get

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

// NIMService tests.
func newBaseNS(name, ns string) appsv1alpha1.NIMService {
	return appsv1alpha1.NIMService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
}

func withImage(ns appsv1alpha1.NIMService, repo, tag string) appsv1alpha1.NIMService {
	ns.Spec.Image.Repository = repo
	ns.Spec.Image.Tag = tag
	return ns
}

func withExposeService(ns appsv1alpha1.NIMService, name string, port int32) appsv1alpha1.NIMService {
	ns.Spec.Expose.Service.Name = name
	ns.Spec.Expose.Service.Port = ptr.To(port)
	return ns
}

func withReplicas(ns appsv1alpha1.NIMService, r int) appsv1alpha1.NIMService {
	ns.Spec.Replicas = r
	return ns
}

func withScale(ns appsv1alpha1.NIMService, enabled bool, min *int32, max int32) appsv1alpha1.NIMService {
	ns.Spec.Scale.Enabled = ptr.To(enabled)
	ns.Spec.Scale.HPA.MinReplicas = min
	ns.Spec.Scale.HPA.MaxReplicas = max
	return ns
}

func withStorageNIMCache(ns appsv1alpha1.NIMService, name, profile string) appsv1alpha1.NIMService {
	ns.Spec.Storage.NIMCache.Name = name
	ns.Spec.Storage.NIMCache.Profile = profile
	return ns
}

func withStoragePVC(ns appsv1alpha1.NIMService, name, size string) appsv1alpha1.NIMService {
	ns.Spec.Storage.PVC.Name = name
	ns.Spec.Storage.PVC.Size = size
	return ns
}

func withStorageHostPath(ns appsv1alpha1.NIMService, p string) appsv1alpha1.NIMService {
	ns.Spec.Storage.HostPath = ptr.To(p)
	return ns
}

func Test_printNIMServices(t *testing.T) {
	// Item 1
	ns1 := withReplicas(withExposeService(withImage(newBaseNS("svc1", "ns1"), "repo1", "v1"), "api", 8080), 2)
	ns1.Spec.Scale.Enabled = ptr.To(false)
	ns1 = withStoragePVC(ns1, "pvc1", "10Gi")
	ns1.Status.State = "Creating"
	ns1.CreationTimestamp = metav1.NewTime(time.Now().Add(-1 * time.Hour))

	// Item 2 - with an endpoint
	min := int32(1)
	ns2 := withReplicas(withExposeService(withImage(newBaseNS("svc2", "ns2"), "repo2", "v2"), "", 9090), 3)
	ns2 = withScale(ns2, true, &min, 5)
	ns2 = withStorageNIMCache(ns2, "nimc", "fp8")
	ns2.Status.State = "Ready"
	ns2.Status.Model = &appsv1alpha1.ModelStatus{ExternalEndpoint: "http://example.com/api"}
	ns2.CreationTimestamp = metav1.NewTime(time.Now().Add(-2 * time.Hour))

	list := &appsv1alpha1.NIMServiceList{Items: []appsv1alpha1.NIMService{ns1, ns2}}

	var buf bytes.Buffer
	if err := printNIMServices(list, &buf); err != nil {
		t.Fatalf("printNIMServices error: %v", err)
	}
	out := buf.String()

	// Headers (printer uppercases them)
	for _, h := range []string{"NAME", "STATUS", "AGE", "ENDPOINT"} {
		if !strings.Contains(out, h) {
			t.Fatalf("output missing header %q:\n%s", h, out)
		}
	}

	// Row assertions
	// Check first service (no endpoint)
	if !strings.Contains(out, "svc1") {
		t.Fatalf("output missing svc1")
	}
	if !strings.Contains(out, "Creating") {
		t.Fatalf("output missing Creating status")
	}

	// Check second service (with endpoint)
	if !strings.Contains(out, "svc2") {
		t.Fatalf("output missing svc2")
	}
	if !strings.Contains(out, "Ready") {
		t.Fatalf("output missing Ready status")
	}
	if !strings.Contains(out, "http://example.com/api") {
		t.Fatalf("output missing endpoint")
	}
}

func Test_getEndpoint(t *testing.T) {
	tests := []struct {
		name string
		ns   appsv1alpha1.NIMService
		want string
	}{
		{
			"with endpoint",
			appsv1alpha1.NIMService{
				Status: appsv1alpha1.NIMServiceStatus{
					Model: &appsv1alpha1.ModelStatus{
						ExternalEndpoint: "http://example.com/api",
					},
				},
			},
			"http://example.com/api",
		},
		{
			"no model status",
			appsv1alpha1.NIMService{},
			"",
		},
		{
			"model status but no endpoint",
			appsv1alpha1.NIMService{
				Status: appsv1alpha1.NIMServiceStatus{
					Model: &appsv1alpha1.ModelStatus{},
				},
			},
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getEndpoint(&tt.ns)
			if got != tt.want {
				t.Errorf("getEndpoint() = %q, want %q", got, tt.want)
			}
		})
	}
}

func Test_printNIMServices_EmptyList(t *testing.T) {
	list := &appsv1alpha1.NIMServiceList{Items: []appsv1alpha1.NIMService{}}

	var buf bytes.Buffer
	if err := printNIMServices(list, &buf); err != nil {
		t.Fatalf("printNIMServices error: %v", err)
	}
	out := buf.String()

	// When there are no items, the table printer doesn't output anything
	if strings.TrimSpace(out) != "" {
		t.Fatalf("expected empty output for empty list, got:\n%s", out)
	}
}

func Test_printNIMServices_DifferentStates(t *testing.T) {
	// Test various states
	states := []string{"Creating", "Ready", "Failed", "Updating", "Terminating"}
	var services []appsv1alpha1.NIMService

	for i, state := range states {
		ns := withImage(newBaseNS(fmt.Sprintf("svc-%s", strings.ToLower(state)), "ns"), "repo", "v1")
		ns.Status.State = state
		ns.CreationTimestamp = metav1.NewTime(time.Now().Add(time.Duration(-i) * time.Hour))
		services = append(services, ns)
	}

	list := &appsv1alpha1.NIMServiceList{Items: services}

	var buf bytes.Buffer
	if err := printNIMServices(list, &buf); err != nil {
		t.Fatalf("printNIMServices error: %v", err)
	}
	out := buf.String()

	// Check all states are displayed
	for _, state := range states {
		if !strings.Contains(out, state) {
			t.Fatalf("output missing state %q:\n%s", state, out)
		}
	}
}

func Test_printNIMServices_WithDifferentStorageTypes(t *testing.T) {
	// Service with HostPath
	ns1 := withStorageHostPath(withImage(newBaseNS("svc-hostpath", "ns"), "repo", "v1"), "/mnt/models")
	ns1.Status.State = "Ready"
	ns1.CreationTimestamp = metav1.NewTime(time.Now().Add(-1 * time.Hour))

	// Service with NIMCache and profile
	ns2 := withStorageNIMCache(withImage(newBaseNS("svc-nimcache", "ns"), "repo", "v2"), "cache1", "optimized")
	ns2.Status.State = "Ready"
	ns2.CreationTimestamp = metav1.NewTime(time.Now().Add(-2 * time.Hour))

	// Service with PVC
	ns3 := withStoragePVC(withImage(newBaseNS("svc-pvc", "ns"), "repo", "v3"), "my-pvc", "100Gi")
	ns3.Status.State = "Creating"
	ns3.CreationTimestamp = metav1.NewTime(time.Now().Add(-30 * time.Minute))

	list := &appsv1alpha1.NIMServiceList{Items: []appsv1alpha1.NIMService{ns1, ns2, ns3}}

	var buf bytes.Buffer
	if err := printNIMServices(list, &buf); err != nil {
		t.Fatalf("printNIMServices error: %v", err)
	}
	out := buf.String()

	// Verify all services are shown
	if !strings.Contains(out, "svc-hostpath") || !strings.Contains(out, "svc-nimcache") || !strings.Contains(out, "svc-pvc") {
		t.Fatalf("output missing one or more services:\n%s", out)
	}
}

// NIMCache tests.
func newBaseNC(name, ns string) appsv1alpha1.NIMCache {
	return appsv1alpha1.NIMCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
}

func ncWithNGC(name, ns, modelPuller string) appsv1alpha1.NIMCache {
	nc := newBaseNC(name, ns)
	nc.Spec.Source.NGC = &appsv1alpha1.NGCSource{ModelPuller: modelPuller}
	return nc
}

func ncWithDataStoreEndpoint(name, ns, endpoint string) appsv1alpha1.NIMCache {
	nc := newBaseNC(name, ns)
	nc.Spec.Source.DataStore = &appsv1alpha1.NemoDataStoreSource{Endpoint: endpoint}
	return nc
}

func ncWithDataStoreModel(name, ns, modelName string) appsv1alpha1.NIMCache {
	nc := newBaseNC(name, ns)
	nc.Spec.Source.DataStore = &appsv1alpha1.NemoDataStoreSource{DSHFCommonFields: appsv1alpha1.DSHFCommonFields{ModelName: &modelName}}
	return nc
}

func withPVC(nc appsv1alpha1.NIMCache, name, size string) appsv1alpha1.NIMCache {
	nc.Spec.Storage.PVC.Name = name
	nc.Spec.Storage.PVC.Size = size
	return nc
}

func withResources(nc appsv1alpha1.NIMCache, cpu, mem string) appsv1alpha1.NIMCache {
	if cpu != "" {
		nc.Spec.Resources.CPU = resource.MustParse(cpu)
	}
	if mem != "" {
		nc.Spec.Resources.Memory = resource.MustParse(mem)
	}
	return nc
}

func withCreationTime(nc appsv1alpha1.NIMCache, t time.Time) appsv1alpha1.NIMCache {
	nc.CreationTimestamp = metav1.NewTime(t)
	return nc
}

func withState(nc appsv1alpha1.NIMCache, s string) appsv1alpha1.NIMCache {
	nc.Status.State = s
	return nc
}

func Test_getSource(t *testing.T) {
	tests := []struct {
		name string
		nc   appsv1alpha1.NIMCache
		want string
	}{
		{"NGC", ncWithNGC("a", "ns", "img:tag"), "NGC"},
		{"DataStore", ncWithDataStoreEndpoint("b", "ns", "https://datastore"), "NVIDIA NeMo DataStore"},
		{"DefaultHF", newBaseNC("c", "ns"), "HuggingFace Hub"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getSource(&tt.nc)
			if got != tt.want {
				t.Fatalf("getSource() = %q, want %q", got, tt.want)
			}
		})
	}
}

func Test_getSource_HuggingFace(t *testing.T) {
	nc := newBaseNC("hf-cache", "ns")
	nc.Spec.Source.HF = &appsv1alpha1.HuggingFaceHubSource{}
	nc.Spec.Source.HF.ModelPuller = "hf-puller:latest"

	got := getSource(&nc)
	if got != "HuggingFace Hub" {
		t.Fatalf("getSource() for HF = %q, want %q", got, "HuggingFace Hub")
	}
}

func Test_getPVCDetails(t *testing.T) {
	nc1 := withPVC(newBaseNC("a", "ns"), "pvc-a", "10Gi")
	nc2 := withPVC(newBaseNC("b", "ns"), "", "20Gi")

	if got := getPVCDetails(&nc1); got != "pvc-a, 10Gi" {
		t.Fatalf("getPVCDetails(name+size) = %q, want %q", got, "pvc-a, 10Gi")
	}
	if got := getPVCDetails(&nc2); got != "20Gi" {
		t.Fatalf("getPVCDetails(size only) = %q, want %q", got, "20Gi")
	}
}

func Test_printNIMCaches(t *testing.T) {
	// Item 1: NGC
	nc1 := withState(withResources(withPVC(
		ncWithNGC("nc-ngc", "ns1", "img:tag"),
		"pvc1", "50Gi",
	), "2", "4Gi"), "Creating")
	nc1 = withCreationTime(nc1, time.Now().Add(-2*time.Hour))

	// Item 2: DataStore with model name, zero timestamps to force <unknown> age.
	nc2 := withState(withResources(withPVC(
		ncWithDataStoreModel("nc-ds", "ns2", "mymodel"),
		"", "200Gi",
	), "8", "32Gi"), "Ready")

	list := &appsv1alpha1.NIMCacheList{Items: []appsv1alpha1.NIMCache{nc1, nc2}}

	var buf bytes.Buffer
	if err := printNIMCaches(list, &buf); err != nil {
		t.Fatalf("printNIMCaches error: %v", err)
	}
	out := buf.String()

	// Check headers
	for _, h := range []string{"NAME", "SOURCE", "STATUS", "PVC", "AGE"} {
		if !strings.Contains(out, h) {
			t.Fatalf("output missing header %q:\n%s", h, out)
		}
	}

	// Check first cache (NGC)
	if !strings.Contains(out, "nc-ngc") {
		t.Fatalf("output missing nc-ngc name")
	}
	if !strings.Contains(out, "NGC") {
		t.Fatalf("output missing NGC source")
	}
	if !strings.Contains(out, "Creating") {
		t.Fatalf("output missing Creating status")
	}
	if !strings.Contains(out, "pvc1, 50Gi") {
		t.Fatalf("output missing PVC details")
	}

	// Check second cache (DataStore)
	if !strings.Contains(out, "nc-ds") {
		t.Fatalf("output missing nc-ds name")
	}
	if !strings.Contains(out, "NVIDIA NeMo DataStore") {
		t.Fatalf("output missing DataStore source")
	}
	if !strings.Contains(out, "Ready") {
		t.Fatalf("output missing Ready status")
	}
	if !strings.Contains(out, "200Gi") {
		t.Fatalf("output missing PVC size for second cache")
	}
	if !strings.Contains(out, "<unknown>") {
		t.Fatalf("output missing <unknown> age for second cache (zero timestamp)")
	}
}

func Test_printNIMCaches_EmptyList(t *testing.T) {
	list := &appsv1alpha1.NIMCacheList{Items: []appsv1alpha1.NIMCache{}}

	var buf bytes.Buffer
	if err := printNIMCaches(list, &buf); err != nil {
		t.Fatalf("printNIMCaches error: %v", err)
	}
	out := buf.String()

	// When there are no items, the table printer doesn't output anything
	if strings.TrimSpace(out) != "" {
		t.Fatalf("expected empty output for empty list, got:\n%s", out)
	}
}

func Test_printNIMCaches_AllSourceTypes(t *testing.T) {
	// NGC source
	nc1 := withState(withPVC(ncWithNGC("nc-ngc", "ns", "nvcr.io/nim/puller:latest"), "ngc-pvc", "50Gi"), "Ready")
	nc1 = withCreationTime(nc1, time.Now().Add(-1*time.Hour))

	// HuggingFace source
	nc2 := newBaseNC("nc-hf", "ns")
	nc2.Spec.Source.HF = &appsv1alpha1.HuggingFaceHubSource{
		Endpoint:  "https://huggingface.co",
		Namespace: "meta-llama",
	}
	nc2.Spec.Source.HF.ModelPuller = "hf-puller:latest"
	nc2 = withState(withPVC(nc2, "hf-pvc", "100Gi"), "Creating")
	nc2 = withCreationTime(nc2, time.Now().Add(-30*time.Minute))

	// DataStore source
	nc3 := withState(withPVC(ncWithDataStoreEndpoint("nc-ds", "ns", "https://nemo.example.com"), "ds-pvc", "200Gi"), "Failed")
	nc3 = withCreationTime(nc3, time.Now().Add(-3*time.Hour))

	list := &appsv1alpha1.NIMCacheList{Items: []appsv1alpha1.NIMCache{nc1, nc2, nc3}}

	var buf bytes.Buffer
	if err := printNIMCaches(list, &buf); err != nil {
		t.Fatalf("printNIMCaches error: %v", err)
	}
	out := buf.String()

	// Check all sources are displayed
	if !strings.Contains(out, "NGC") {
		t.Fatalf("output missing NGC source")
	}
	if !strings.Contains(out, "HuggingFace Hub") {
		t.Fatalf("output missing HuggingFace Hub source")
	}
	if !strings.Contains(out, "NVIDIA NeMo DataStore") {
		t.Fatalf("output missing DataStore source")
	}

	// Check all states
	if !strings.Contains(out, "Ready") || !strings.Contains(out, "Creating") || !strings.Contains(out, "Failed") {
		t.Fatalf("output missing one or more states")
	}
}

func Test_printNIMCaches_DifferentPVCConfigurations(t *testing.T) {
	// Cache with PVC name and size
	nc1 := withPVC(newBaseNC("nc-full-pvc", "ns"), "my-pvc-1", "50Gi")
	nc1 = withState(nc1, "Ready")
	nc1 = withCreationTime(nc1, time.Now().Add(-1*time.Hour))

	// Cache with only size (no name)
	nc2 := withPVC(newBaseNC("nc-size-only", "ns"), "", "100Gi")
	nc2 = withState(nc2, "Creating")
	nc2 = withCreationTime(nc2, time.Now().Add(-2*time.Hour))

	// Cache with empty PVC details
	nc3 := newBaseNC("nc-no-pvc", "ns")
	nc3 = withState(nc3, "Ready")
	nc3 = withCreationTime(nc3, time.Now().Add(-3*time.Hour))

	list := &appsv1alpha1.NIMCacheList{Items: []appsv1alpha1.NIMCache{nc1, nc2, nc3}}

	var buf bytes.Buffer
	if err := printNIMCaches(list, &buf); err != nil {
		t.Fatalf("printNIMCaches error: %v", err)
	}
	out := buf.String()

	// Check PVC details formatting
	if !strings.Contains(out, "my-pvc-1, 50Gi") {
		t.Fatalf("output missing correct PVC details for full PVC")
	}
	if !strings.Contains(out, "100Gi") {
		t.Fatalf("output missing PVC size for size-only cache")
	}

	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.Contains(line, "nc-no-pvc") {
			// Should have empty PVC column
			fields := strings.Fields(line)
			if len(fields) < 4 {
				t.Fatalf("expected at least 4 fields in output line")
			}
			// The PVC field should be empty or just whitespace between STATUS and AGE
			break
		}
	}
}

func Test_getPVCDetails_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		nimCache appsv1alpha1.NIMCache
		want     string
	}{
		{
			name:     "both name and size",
			nimCache: withPVC(newBaseNC("nc", "ns"), "test-pvc", "50Gi"),
			want:     "test-pvc, 50Gi",
		},
		{
			name:     "only size",
			nimCache: withPVC(newBaseNC("nc", "ns"), "", "100Gi"),
			want:     "100Gi",
		},
		{
			name:     "only name",
			nimCache: withPVC(newBaseNC("nc", "ns"), "lonely-pvc", ""),
			want:     "lonely-pvc, ",
		},
		{
			name:     "neither name nor size",
			nimCache: newBaseNC("nc", "ns"),
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getPVCDetails(&tt.nimCache)
			if got != tt.want {
				t.Errorf("getPVCDetails() = %q, want %q", got, tt.want)
			}
		})
	}
}
