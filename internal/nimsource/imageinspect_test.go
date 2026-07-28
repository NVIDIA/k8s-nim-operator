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

package nimsource

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

const testImage = "registry.example.com/team/retriever:2.0"

type fakeLabelSource struct {
	labels []map[string]string
	err    error
}

func (f *fakeLabelSource) Labels(_ context.Context, _ string, _, _ string) ([]map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.labels, nil
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name    string
		labels  []map[string]string
		err     error
		want    Protocol
		wantErr string
	}{
		{
			name: "native-v1 on single platform",
			labels: []map[string]string{
				{ModelDownloadProtocolLabel: string(NativeV1)},
			},
			want: NativeV1,
		},
		{
			name: "native-v1 on all platforms",
			labels: []map[string]string{
				{ModelDownloadProtocolLabel: string(NativeV1)},
				{ModelDownloadProtocolLabel: string(NativeV1)},
			},
			want: NativeV1,
		},
		{
			name: "absent label is legacy",
			labels: []map[string]string{
				{},
			},
			want: Legacy,
		},
		{
			name: "unknown label is legacy",
			labels: []map[string]string{
				{ModelDownloadProtocolLabel: "future-v2"},
			},
			want: Legacy,
		},
		{
			name: "mixed platform protocols fail",
			labels: []map[string]string{
				{ModelDownloadProtocolLabel: string(NativeV1)},
				{},
			},
			wantErr: "inconsistent model download protocols",
		},
		{
			name:    "inspect error fails",
			err:     context.DeadlineExceeded,
			wantErr: "inspect image",
		},
		{
			name:    "no platforms fail",
			labels:  []map[string]string{},
			wantErr: "no image configurations found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := newRegistryResolver(fake.NewClientBuilder().Build(), &fakeLabelSource{labels: tt.labels, err: tt.err})
			got, err := resolver.Resolve(context.Background(), testImage, "default", nil)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Resolve() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Resolve() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCredentialsForHost(t *testing.T) {
	auth := base64.StdEncoding.EncodeToString([]byte("user:pass"))
	cfg := []byte(`{"auths":{"https://nvcr.io/v2/":{"auth":"` + auth + `"}}}`)
	user, pass, found, err := credentialsForHost("nvcr.io", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !found || user != "user" || pass != "pass" {
		t.Fatalf("credentials = (%q,%q,%v), want (user,pass,true)", user, pass, found)
	}

	_, _, found, err = credentialsForHost("registry.example.com", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected no credentials for unmatched host")
	}
}

func TestResolveUsesMatchingPullSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	auth := base64.StdEncoding.EncodeToString([]byte("ngc:token"))
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ngc-secret", Namespace: "default"},
		Type:       corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: []byte(`{"auths":{"nvcr.io":{"auth":"` + auth + `"}}}`),
		},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	source := &recordingLabelSource{labels: []map[string]string{{ModelDownloadProtocolLabel: string(NativeV1)}}}
	resolver := newRegistryResolver(cli, source)
	got, err := resolver.Resolve(context.Background(), "nvcr.io/nim/retriever:2.0.0", "default", []string{"ngc-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if got != NativeV1 {
		t.Fatalf("got %q, want native-v1", got)
	}
	if source.username != "ngc" || source.password != "token" {
		t.Fatalf("credentials = (%q,%q), want (ngc,token)", source.username, source.password)
	}
}

type recordingLabelSource struct {
	labels             []map[string]string
	username, password string
}

func (r *recordingLabelSource) Labels(_ context.Context, _ string, username, password string) ([]map[string]string, error) {
	r.username, r.password = username, password
	return r.labels, nil
}

// failingResolver fails if Resolve is ever called. Used to prove that a
// NIMCache-backed layout is derived from the persisted NIMCache status without
// any registry inspection.
type failingResolver struct{}

func (failingResolver) Resolve(context.Context, string, string, []string) (Protocol, error) {
	return Legacy, fmt.Errorf("resolver should not be called")
}

// fixedResolver returns a fixed protocol for the direct (no NIMCache) path.
type fixedResolver struct{ protocol Protocol }

func (f fixedResolver) Resolve(context.Context, string, string, []string) (Protocol, error) {
	return f.protocol, nil
}

func TestResolveProtocolOrLegacy(t *testing.T) {
	if got := ResolveProtocolOrLegacy(context.Background(), nil, "img", "ns", nil); got != Legacy {
		t.Fatalf("nil resolver: got %q, want legacy", got)
	}
	if got := ResolveProtocolOrLegacy(context.Background(), failingResolver{}, "img", "ns", nil); got != Legacy {
		t.Fatalf("inspection error: got %q, want legacy", got)
	}
	if got := ResolveProtocolOrLegacy(context.Background(), fixedResolver{protocol: NativeV1}, "img", "ns", nil); got != NativeV1 {
		t.Fatalf("native: got %q, want native-v1", got)
	}
}

func TestResolveModelLayout(t *testing.T) {
	service := &appsv1alpha1.NIMService{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"},
		Spec: appsv1alpha1.NIMServiceSpec{
			Image: appsv1alpha1.Image{Repository: "nvcr.io/nim/retriever", Tag: "2.0.0"},
		},
	}

	t.Run("native NIMCache yields native layout without inspection", func(t *testing.T) {
		cache := &appsv1alpha1.NIMCache{
			ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "default"},
			Status:     appsv1alpha1.NIMCacheStatus{Type: appsv1alpha1.NIMCacheModelTypeNative},
		}
		layout, err := ResolveModelLayout(context.Background(), failingResolver{}, service, cache)
		if err != nil {
			t.Fatal(err)
		}
		if !layout.Protocol.IsNative() {
			t.Fatalf("protocol = %q, want native", layout.Protocol)
		}
	})

	t.Run("non-native NIMCache yields legacy layout without inspection", func(t *testing.T) {
		cache := &appsv1alpha1.NIMCache{
			ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "default"},
		}
		layout, err := ResolveModelLayout(context.Background(), failingResolver{}, service, cache)
		if err != nil {
			t.Fatal(err)
		}
		if layout.Protocol != Legacy {
			t.Fatalf("protocol = %q, want legacy", layout.Protocol)
		}
	})

	t.Run("direct NIMService inspects serving image", func(t *testing.T) {
		layout, err := ResolveModelLayout(context.Background(), fixedResolver{protocol: NativeV1}, service, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !layout.Protocol.IsNative() {
			t.Fatalf("protocol = %q, want native", layout.Protocol)
		}
	})

	t.Run("direct NIMService with nil resolver defaults to legacy", func(t *testing.T) {
		layout, err := ResolveModelLayout(context.Background(), nil, service, nil)
		if err != nil {
			t.Fatal(err)
		}
		if layout.Protocol != Legacy {
			t.Fatalf("protocol = %q, want legacy", layout.Protocol)
		}
	})

	t.Run("direct NIMService inspection error falls back to legacy without failing", func(t *testing.T) {
		layout, err := ResolveModelLayout(context.Background(), failingResolver{}, service, nil)
		if err != nil {
			t.Fatalf("inspection error must not fail reconciliation: %v", err)
		}
		if layout.Protocol != Legacy {
			t.Fatalf("protocol = %q, want legacy", layout.Protocol)
		}
	})
}
