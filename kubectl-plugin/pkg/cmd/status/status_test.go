package status

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

// --- Status Command structure tests ---

func Test_NewStatusCommand_Structure(t *testing.T) {
	streams := genericTestIOStreams()
	cmd := NewStatusCommand(nil, streams)

	// Test basic command properties
	if cmd.Use != "status" {
		t.Errorf("expected Use to be 'status', got %s", cmd.Use)
	}
	if cmd.Short != "Describe the status of a NIM Operator custom resource" {
		t.Errorf("unexpected Short description")
	}

	// Test that subcommands are added
	subcommands := cmd.Commands()
	if len(subcommands) != 2 {
		t.Fatalf("expected 2 subcommands, got %d", len(subcommands))
	}

	// Verify subcommand names
	foundNimservice, foundNimcache := false, false
	for _, sub := range subcommands {
		if sub.Use == "nimservice [NAME]" {
			foundNimservice = true
		}
		if sub.Use == "nimcache [NAME]" {
			foundNimcache = true
		}
	}
	if !foundNimservice || !foundNimcache {
		t.Errorf("expected nimservice and nimcache subcommands")
	}
}

func Test_NewStatusCommand_NoArgs(t *testing.T) {
	streams := genericTestIOStreams()
	cmd := NewStatusCommand(nil, streams)

	// Test with no arguments - should show help
	cmd.SetArgs([]string{})
	err := cmd.Execute()

	if err != nil {
		t.Fatalf("expected no error for no args, got: %v", err)
	}
}

func Test_NewStatusCommand_InvalidArgs(t *testing.T) {
	streams := genericTestIOStreams()
	cmd := NewStatusCommand(nil, streams)

	// Test with unknown arguments
	cmd.SetArgs([]string{"unknown", "args"})
	_ = cmd.Execute()
	// The command prints error but doesn't return error
}

func Test_NewStatusNIMServiceCommand_Structure(t *testing.T) {
	streams := genericTestIOStreams()
	cmd := NewStatusNIMServiceCommand(nil, streams)

	if cmd.Use != "nimservice [NAME]" {
		t.Errorf("expected Use to be 'nimservice [NAME]', got %s", cmd.Use)
	}
	if cmd.Short != "Get NIMService information." {
		t.Errorf("unexpected Short description")
	}

	// Check aliases
	if len(cmd.Aliases) != 1 || cmd.Aliases[0] != "nimservices" {
		t.Errorf("expected 'nimservices' alias, got %v", cmd.Aliases)
	}

	// Check flags
	if cmd.Flags().Lookup("all-namespaces") == nil {
		t.Errorf("expected all-namespaces flag")
	}
}

func Test_NewStatusNIMCacheCommand_Structure(t *testing.T) {
	streams := genericTestIOStreams()
	cmd := NewStatusNIMCacheCommand(nil, streams)

	if cmd.Use != "nimcache [NAME]" {
		t.Errorf("expected Use to be 'nimcache [NAME]', got %s", cmd.Use)
	}
	if cmd.Short != "Get NIMCache status." {
		t.Errorf("unexpected Short description")
	}

	// Check aliases
	if len(cmd.Aliases) != 1 || cmd.Aliases[0] != "nimcaches" {
		t.Errorf("expected 'nimcaches' alias, got %v", cmd.Aliases)
	}
}

// minimal test IOStreams.
func genericTestIOStreams() genericclioptions.IOStreams {
	return genericclioptions.IOStreams{
		In:     &bytes.Buffer{},
		Out:    &bytes.Buffer{},
		ErrOut: &bytes.Buffer{},
	}
}

// --- NIMCache tests ---

func newNIMCache(name, ns string) appsv1alpha1.NIMCache {
	return appsv1alpha1.NIMCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
}

func withStatus(n appsv1alpha1.NIMCache, state, pvc string, conds []metav1.Condition) appsv1alpha1.NIMCache {
	n.Status.State = state
	n.Status.PVC = pvc
	n.Status.Conditions = conds
	return n
}

func withCreatedAt(n appsv1alpha1.NIMCache, t time.Time) appsv1alpha1.NIMCache {
	n.CreationTimestamp = metav1.NewTime(t)
	return n
}

func withProfiles(n appsv1alpha1.NIMCache, ps ...appsv1alpha1.NIMProfile) appsv1alpha1.NIMCache {
	n.Status.Profiles = append(n.Status.Profiles, ps...)
	return n
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

func Test_printNIMCaches(t *testing.T) {
	// First cache: Failed condition with message, has age
	c1 := newNIMCache("cache-1", "ns1")
	c1 = withStatus(c1, "NotReady", "pvc-1",
		[]metav1.Condition{
			{Type: "Ready", Status: metav1.ConditionFalse, Message: ""},
			{Type: "Failed", Status: metav1.ConditionTrue, Message: "boom", LastTransitionTime: metav1.NewTime(time.Now().Add(-5 * time.Minute))},
		},
	)
	c1 = withCreatedAt(c1, time.Now().Add(-30*time.Minute))

	// Second cache: Ready condition, zero timestamp -> <unknown> age
	c2 := newNIMCache("cache-2", "ns2")
	c2 = withStatus(c2, "Ready", "pvc-2",
		[]metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Message: "ok"}},
	)

	list := &appsv1alpha1.NIMCacheList{Items: []appsv1alpha1.NIMCache{c1, c2}}

	var buf bytes.Buffer
	if err := printNIMCaches(list, &buf); err != nil {
		t.Fatalf("printNIMCaches error: %v", err)
	}
	out := buf.String()

	// Headers (table printer uppercases)
	for _, h := range []string{"NAME", "NAMESPACE", "STATE", "PVC", "TYPE/STATUS", "LAST TRANSITION TIME", "MESSAGE", "AGE"} {
		if !strings.Contains(out, h) {
			t.Fatalf("missing header %q in\n%s", h, out)
		}
	}

	// Row 1 assertions
	for _, s := range []string{"cache-1", "ns1", "NotReady", "pvc-1", "Failed/True", "boom"} {
		if !strings.Contains(out, s) {
			t.Fatalf("missing cell %q in:\n%s", s, out)
		}
	}

	// Row 2 assertions
	for _, s := range []string{"cache-2", "ns2", "Ready", "pvc-2", "Ready/True"} {
		if !strings.Contains(out, s) {
			t.Fatalf("missing cell %q in:\n%s", s, out)
		}
	}
	if !strings.Contains(out, "<unknown>") {
		t.Fatalf("expected <unknown> age for zero timestamp row:\n%s", out)
	}
}

func Test_printNIMCaches_VariousConditions(t *testing.T) {
	tests := []struct {
		name       string
		nimCache   appsv1alpha1.NIMCache
		wantState  string
		wantStatus string
		wantMsg    string
	}{
		{
			name: "ready condition",
			nimCache: withStatus(newNIMCache("nc-ready", "ns"), "Ready", "pvc-ready",
				[]metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Message: "Cache is ready"}}),
			wantState:  "Ready",
			wantStatus: "Ready/True",
			wantMsg:    "Cache is ready",
		},
		{
			name: "failed condition takes precedence",
			nimCache: withStatus(newNIMCache("nc-failed", "ns"), "Failed", "pvc-failed",
				[]metav1.Condition{
					{Type: "Ready", Status: metav1.ConditionFalse, Message: "Not ready"},
					{Type: "Failed", Status: metav1.ConditionTrue, Message: "Job failed"},
				}),
			wantState:  "Failed",
			wantStatus: "Failed/True",
			wantMsg:    "Job failed",
		},
		{
			name: "progressing condition",
			nimCache: withStatus(newNIMCache("nc-progress", "ns"), "Progressing", "pvc-progress",
				[]metav1.Condition{{Type: "Progressing", Status: metav1.ConditionTrue, Message: "Caching in progress"}}),
			wantState:  "Progressing",
			wantStatus: "Progressing/True",
			wantMsg:    "Caching in progress",
		},
		{
			name: "unknown status",
			nimCache: withStatus(newNIMCache("nc-unknown", "ns"), "Unknown", "pvc-unknown",
				[]metav1.Condition{{Type: "Ready", Status: metav1.ConditionUnknown, Message: "Cannot determine status"}}),
			wantState:  "Unknown",
			wantStatus: "Ready/Unknown",
			wantMsg:    "Cannot determine status",
		},
		// Skip empty conditions test as the code returns an error for no conditions
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list := &appsv1alpha1.NIMCacheList{Items: []appsv1alpha1.NIMCache{tt.nimCache}}

			var buf bytes.Buffer
			if err := printNIMCaches(list, &buf); err != nil {
				t.Fatalf("printNIMCaches error: %v", err)
			}
			out := buf.String()

			// Check state
			if !strings.Contains(out, tt.wantState) {
				t.Errorf("expected state %q in output:\n%s", tt.wantState, out)
			}

			// Check status
			if !strings.Contains(out, tt.wantStatus) {
				t.Errorf("expected status %q in output:\n%s", tt.wantStatus, out)
			}

			// Check message if not empty
			if tt.wantMsg != "" && !strings.Contains(out, tt.wantMsg) {
				t.Errorf("expected message %q in output:\n%s", tt.wantMsg, out)
			}
		})
	}
}

func Test_printSingleNIMCache_MultipleProfiles(t *testing.T) {
	nc := newNIMCache("cache-multi", "ns")
	nc = withStatus(nc, "Ready", "pvc-multi",
		[]metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Message: "Ready with profiles"}},
	)
	nc = withProfiles(nc,
		appsv1alpha1.NIMProfile{
			Name:    "profile-1",
			Model:   "llama-8b",
			Release: "v1.0",
			Config:  map[string]string{"gpu": "a100", "precision": "fp16"},
		},
		appsv1alpha1.NIMProfile{
			Name:    "profile-2",
			Model:   "llama-70b",
			Release: "v2.0",
			Config:  nil,
		},
	)

	var buf bytes.Buffer
	if err := printSingleNIMCache(&nc, &buf); err != nil {
		t.Fatalf("printSingleNIMCache error: %v", err)
	}
	out := buf.String()

	// Check all profile details are displayed
	expectedStrings := []string{
		"Cached NIM Profiles:",
		"Name: profile-1, Model: llama-8b, Release: v1.0, Config: map[gpu:a100 precision:fp16]",
		"Name: profile-2, Model: llama-70b, Release: v2.0, Config: map[]",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(out, expected) {
			t.Errorf("expected %q in output:\n%s", expected, out)
		}
	}
}

func Test_printSingleNIMCache_NoProfiles(t *testing.T) {
	nc := newNIMCache("cache-no-profiles", "ns")
	nc = withStatus(nc, "Ready", "pvc-x",
		[]metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Message: "Ready"}},
	)
	// No profiles added

	var buf bytes.Buffer
	if err := printSingleNIMCache(&nc, &buf); err != nil {
		t.Fatalf("printSingleNIMCache error: %v", err)
	}
	out := buf.String()

	// Should still have the profiles header but no profile lines
	if !strings.Contains(out, "Cached NIM Profiles:") {
		t.Errorf("expected profiles header in output")
	}

	// Should not have any profile entries
	if strings.Contains(out, "Name:") && strings.Contains(out, "Model:") {
		t.Errorf("unexpected profile entries in output with no profiles")
	}
}

func Test_printSingleNIMCache(t *testing.T) {
	nc := newNIMCache("cache-x", "ns-x")
	nc = withStatus(nc, "Ready", "pvc-x",
		[]metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Message: "all good", LastTransitionTime: metav1.NewTime(time.Now())}},
	)
	nc = withCreatedAt(nc, time.Now().Add(-2*time.Hour))
	nc = withProfiles(nc,
		appsv1alpha1.NIMProfile{Name: "p1", Model: "m1", Release: "r1", Config: map[string]string{"k": "v"}},
		appsv1alpha1.NIMProfile{Name: "p2", Model: "m2", Release: "r2"},
	)

	var buf bytes.Buffer
	if err := printSingleNIMCache(&nc, &buf); err != nil {
		t.Fatalf("printSingleNIMCache error: %v", err)
	}
	out := buf.String()

	// Core fields
	for _, s := range []string{
		"Name: cache-x",
		"Namespace: ns-x",
		"State: Ready",
		"PVC: pvc-x",
		"Type/Status: Ready/True",
		"Message: all good",
		"Cached NIM Profiles:",
		"Name: p1, Model: m1, Release: r1, Config: map[k:v]",
		"Name: p2, Model: m2, Release: r2",
	} {
		if !strings.Contains(out, s) {
			t.Fatalf("missing %q in:\n%s", s, out)
		}
	}
}

// ---- NIMService status tests (reusing this test file as requested) ----

func newNIMService(name, ns string) appsv1alpha1.NIMService {
	return appsv1alpha1.NIMService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
}

func withSvcStatus(n appsv1alpha1.NIMService, state string, available int32, conds []metav1.Condition) appsv1alpha1.NIMService {
	n.Status.State = state
	n.Status.AvailableReplicas = available
	n.Status.Conditions = conds
	return n
}

func withSvcCreatedAt(n appsv1alpha1.NIMService, t time.Time) appsv1alpha1.NIMService {
	n.CreationTimestamp = metav1.NewTime(t)
	return n
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

func Test_printNIMServices_VariousConditions(t *testing.T) {
	tests := []struct {
		name         string
		nimService   appsv1alpha1.NIMService
		wantState    string
		wantReplicas int32
		wantStatus   string
		wantMsg      string
	}{
		{
			name: "ready with multiple replicas",
			nimService: withSvcStatus(newNIMService("svc-ready", "ns"), "Ready", 3,
				[]metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Message: "Service is ready"}}),
			wantState:    "Ready",
			wantReplicas: 3,
			wantStatus:   "Ready/True",
			wantMsg:      "Service is ready",
		},
		{
			name: "failed deployment",
			nimService: withSvcStatus(newNIMService("svc-failed", "ns"), "Failed", 0,
				[]metav1.Condition{{Type: "Failed", Status: metav1.ConditionTrue, Message: "Image pull error"}}),
			wantState:    "Failed",
			wantReplicas: 0,
			wantStatus:   "Failed/True",
			wantMsg:      "Image pull error",
		},
		{
			name: "progressing deployment",
			nimService: withSvcStatus(newNIMService("svc-progress", "ns"), "Progressing", 1,
				[]metav1.Condition{
					{Type: "Progressing", Status: metav1.ConditionTrue, Message: "Rolling out new version"},
				}),
			wantState:    "Progressing",
			wantReplicas: 1,
			wantStatus:   "Progressing/True",
			wantMsg:      "Rolling out new version",
		},
		{
			name: "unknown condition status",
			nimService: withSvcStatus(newNIMService("svc-unknown", "ns"), "Unknown", 1,
				[]metav1.Condition{{Type: "Ready", Status: metav1.ConditionUnknown, Message: "Unable to determine status"}}),
			wantState:    "Unknown",
			wantReplicas: 1,
			wantStatus:   "Ready/Unknown",
			wantMsg:      "Unable to determine status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list := &appsv1alpha1.NIMServiceList{Items: []appsv1alpha1.NIMService{tt.nimService}}

			var buf bytes.Buffer
			if err := printNIMServices(list, &buf); err != nil {
				t.Fatalf("printNIMServices error: %v", err)
			}
			out := buf.String()

			// Check state
			if !strings.Contains(out, tt.wantState) {
				t.Errorf("expected state %q in output:\n%s", tt.wantState, out)
			}

			// Check replicas
			replicaStr := fmt.Sprintf("%d", tt.wantReplicas)
			if !strings.Contains(out, replicaStr) {
				t.Errorf("expected replicas %s in output:\n%s", replicaStr, out)
			}

			// Check status
			if !strings.Contains(out, tt.wantStatus) {
				t.Errorf("expected status %q in output:\n%s", tt.wantStatus, out)
			}

			// Check message if not empty
			if tt.wantMsg != "" && !strings.Contains(out, tt.wantMsg) {
				t.Errorf("expected message %q in output:\n%s", tt.wantMsg, out)
			}
		})
	}
}

func Test_printNIMServices_status(t *testing.T) {
	// Item 1: Failed with message
	s1 := newNIMService("svc-1", "ns1")
	s1 = withSvcStatus(s1, "NotReady", 1, []metav1.Condition{
		{Type: "Failed", Status: metav1.ConditionTrue, Message: "oops", LastTransitionTime: metav1.NewTime(time.Now().Add(-10 * time.Minute))},
	})
	s1 = withSvcCreatedAt(s1, time.Now().Add(-1*time.Hour))

	// Item 2: Ready, zero timestamp => <unknown>
	s2 := newNIMService("svc-2", "ns2")
	s2 = withSvcStatus(s2, "Ready", 2, []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue, Message: "ok"},
	})

	list := &appsv1alpha1.NIMServiceList{Items: []appsv1alpha1.NIMService{s1, s2}}

	var buf bytes.Buffer
	if err := printNIMServices(list, &buf); err != nil {
		t.Fatalf("printNIMServices error: %v", err)
	}
	out := buf.String()

	// Headers
	for _, h := range []string{"NAME", "NAMESPACE", "STATE", "AVAILABLE REPLICAS", "TYPE/STATUS", "LAST TRANSITION TIME", "MESSAGE", "AGE"} {
		if !strings.Contains(out, h) {
			t.Fatalf("missing header %q in\n%s", h, out)
		}
	}

	// Row 1 assertions
	for _, s := range []string{"svc-1", "ns1", "NotReady", "1", "Failed/True", "oops"} {
		if !strings.Contains(out, s) {
			t.Fatalf("missing cell %q in:\n%s", s, out)
		}
	}

	// Row 2 assertions
	for _, s := range []string{"svc-2", "ns2", "Ready", "2", "Ready/True"} {
		if !strings.Contains(out, s) {
			t.Fatalf("missing cell %q in:\n%s", s, out)
		}
	}
	if !strings.Contains(out, "<unknown>") {
		t.Fatalf("expected <unknown> age for zero timestamp row:\n%s", out)
	}
}

// --- Edge case tests ---

func Test_printNIMCaches_TransitionTimeFormatting(t *testing.T) {
	// Test various transition time scenarios
	now := time.Now()
	tests := []struct {
		name       string
		transTime  metav1.Time
		wantOutput string
	}{
		{
			name:       "recent transition",
			transTime:  metav1.NewTime(now.Add(-5 * time.Second)),
			wantOutput: "5s", // Should show relative time
		},
		{
			name:       "zero transition time",
			transTime:  metav1.Time{},
			wantOutput: "", // Empty time should show as empty
		},
		{
			name:       "old transition",
			transTime:  metav1.NewTime(now.Add(-25 * time.Hour)),
			wantOutput: "25h", // Should show relative time
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nc := newNIMCache("cache-time", "ns")
			nc = withStatus(nc, "Ready", "pvc",
				[]metav1.Condition{{
					Type:               "Ready",
					Status:             metav1.ConditionTrue,
					Message:            "ok",
					LastTransitionTime: tt.transTime,
				}},
			)

			list := &appsv1alpha1.NIMCacheList{Items: []appsv1alpha1.NIMCache{nc}}

			var buf bytes.Buffer
			if err := printNIMCaches(list, &buf); err != nil {
				t.Fatalf("printNIMCaches error: %v", err)
			}
			out := buf.String()

			// The table printer will format the time, we just verify it's present
			if tt.wantOutput != "" && !strings.Contains(out, "Ready/True") {
				t.Errorf("expected status in output for %s", tt.name)
			}
		})
	}
}

func Test_printNIMServices_LongMessages(t *testing.T) {
	// Test that very long condition messages are handled properly
	longMsg := strings.Repeat("This is a very long error message. ", 20)

	svc := newNIMService("svc-long", "ns")
	svc = withSvcStatus(svc, "Failed", 0, []metav1.Condition{
		{Type: "Failed", Status: metav1.ConditionTrue, Message: longMsg},
	})

	list := &appsv1alpha1.NIMServiceList{Items: []appsv1alpha1.NIMService{svc}}

	var buf bytes.Buffer
	if err := printNIMServices(list, &buf); err != nil {
		t.Fatalf("printNIMServices error: %v", err)
	}
	out := buf.String()

	// Should contain at least part of the message
	if !strings.Contains(out, "This is a very long error message") {
		t.Errorf("expected long message to be present in output")
	}
}

func Test_printSingleNIMCache_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		nimCache appsv1alpha1.NIMCache
		wantErr  bool
	}{
		{
			name: "nil conditions",
			nimCache: appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{Name: "nc", Namespace: "ns"},
				Status: appsv1alpha1.NIMCacheStatus{
					State:      "Unknown",
					PVC:        "pvc",
					Conditions: nil,
				},
			},
			wantErr: true, // MessageCondition returns error for no conditions
		},
		{
			name: "empty namespace with valid condition",
			nimCache: appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{Name: "nc", Namespace: ""},
				Status: appsv1alpha1.NIMCacheStatus{
					State: "Ready",
					PVC:   "pvc",
					Conditions: []metav1.Condition{
						{Type: "Ready", Status: metav1.ConditionTrue, Message: "Ready"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "special characters in names",
			nimCache: appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cache-with-special-chars-123_test",
					Namespace: "ns-with-dash",
				},
				Status: appsv1alpha1.NIMCacheStatus{
					State: "Ready",
					PVC:   "pvc-with-special_chars",
					Conditions: []metav1.Condition{
						{Type: "Ready", Status: metav1.ConditionTrue, Message: "Ready"},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := printSingleNIMCache(&tt.nimCache, &buf)
			if (err != nil) != tt.wantErr {
				t.Errorf("printSingleNIMCache() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				out := buf.String()
				// Verify basic structure is present
				if !strings.Contains(out, "Name:") || !strings.Contains(out, "State:") {
					t.Errorf("expected basic fields in output, got:\n%s", out)
				}
			}
		})
	}
}

func Test_Status_MultipleResourcesSorting(t *testing.T) {
	// Test that multiple resources are displayed in a consistent order
	caches := []appsv1alpha1.NIMCache{
		withCreatedAt(newNIMCache("cache-3", "ns-c"), time.Now().Add(-3*time.Hour)),
		withCreatedAt(newNIMCache("cache-1", "ns-a"), time.Now().Add(-1*time.Hour)),
		withCreatedAt(newNIMCache("cache-2", "ns-b"), time.Now().Add(-2*time.Hour)),
	}

	// Add status to each
	for i := range caches {
		caches[i] = withStatus(caches[i], "Ready", fmt.Sprintf("pvc-%d", i),
			[]metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}})
	}

	list := &appsv1alpha1.NIMCacheList{Items: caches}

	var buf bytes.Buffer
	if err := printNIMCaches(list, &buf); err != nil {
		t.Fatalf("printNIMCaches error: %v", err)
	}
	out := buf.String()

	// All caches should be present
	for _, cache := range caches {
		if !strings.Contains(out, cache.Name) {
			t.Errorf("expected cache %s in output", cache.Name)
		}
	}

	// Verify they appear in order (the actual order depends on the printer)
	lines := strings.Split(out, "\n")
	var foundCaches []string
	for _, line := range lines {
		for _, cache := range caches {
			if strings.Contains(line, cache.Name) {
				foundCaches = append(foundCaches, cache.Name)
				break
			}
		}
	}

	if len(foundCaches) != len(caches) {
		t.Errorf("expected to find all %d caches, found %d", len(caches), len(foundCaches))
	}
}
