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

package imageprotocol

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	registryv1 "github.com/google/go-containerregistry/pkg/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testImage = "registry.example.com/team/retriever:2.0"

type fakeLabelSource struct {
	labels []map[string]string
	err    error
	auth   *authn.AuthConfig
}

func (f *fakeLabelSource) Labels(_ context.Context, image string, keychain authn.Keychain) ([]map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	ref, err := name.ParseReference(image)
	if err != nil {
		return nil, err
	}
	authenticator, err := keychain.Resolve(ref.Context())
	if err != nil {
		return nil, err
	}
	f.auth, err = authenticator.Authorization()
	if err != nil {
		return nil, err
	}
	return f.labels, nil
}

func TestRegistryResolverSelectsProtocolAcrossPlatforms(t *testing.T) {
	tests := []struct {
		name    string
		labels  []map[string]string
		want    Protocol
		wantErr string
	}{
		{
			name: "exact native label on every platform",
			labels: []map[string]string{
				{ModelDownloadProtocolLabel: string(NativeV1)},
				{ModelDownloadProtocolLabel: string(NativeV1)},
			},
			want: NativeV1,
		},
		{
			name:   "label absent",
			labels: []map[string]string{{}, {}},
			want:   Legacy,
		},
		{
			name: "unknown label is legacy",
			labels: []map[string]string{
				{ModelDownloadProtocolLabel: "future-v2"},
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
			name: "different legacy label values still fail consistency",
			labels: []map[string]string{
				{ModelDownloadProtocolLabel: "future-v2"},
				{},
			},
			wantErr: "inconsistent model download protocols",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := &fakeLabelSource{labels: tt.labels}
			resolver := newRegistryResolver(fake.NewClientBuilder().Build(), source)

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

func TestPlatformImageDescriptorFiltering(t *testing.T) {
	if !isPlatformImageDescriptor(registryv1.Descriptor{Platform: &registryv1.Platform{OS: "linux", Architecture: "amd64"}}) {
		t.Fatal("runnable linux image descriptor was filtered")
	}
	if isPlatformImageDescriptor(registryv1.Descriptor{ArtifactType: "application/vnd.example.sbom"}) {
		t.Fatal("artifact descriptor was treated as a platform image")
	}
	if isPlatformImageDescriptor(registryv1.Descriptor{Platform: &registryv1.Platform{OS: "unknown", Architecture: "unknown"}}) {
		t.Fatal("attestation platform descriptor was treated as a runnable image")
	}
}

func TestRegistryResolverUsesDockerConfigPullSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	auth := base64.StdEncoding.EncodeToString([]byte("robot:secret"))
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "registry-creds", Namespace: "models"},
		Type:       corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: []byte(`{"auths":{"registry.example.com":{"auth":"` + auth + `"}}}`),
		},
	}
	source := &fakeLabelSource{labels: []map[string]string{{}}}
	resolver := newRegistryResolver(fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(), source)

	if _, err := resolver.Resolve(context.Background(), testImage, "models", []string{"registry-creds"}); err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if source.auth == nil || source.auth.Username != "robot" || source.auth.Password != "secret" {
		t.Fatalf("registry auth = %#v, want robot credentials", source.auth)
	}
}

func TestRegistryResolverRespectsCredentialScope(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	auth := func(username string) string {
		return base64.StdEncoding.EncodeToString([]byte(username + ":secret"))
	}

	tests := []struct {
		name         string
		image        string
		dockerConfig string
		wantUsername string
	}{
		{
			name:         "selects the matching repository path",
			image:        testImage,
			dockerConfig: `{"auths":{"registry.example.com/other":{"auth":"` + auth("other") + `"},"registry.example.com/team":{"auth":"` + auth("team") + `"}}}`,
			wantUsername: "team",
		},
		{
			name:         "does not broaden path credentials to the host",
			image:        testImage,
			dockerConfig: `{"auths":{"registry.example.com/other":{"auth":"` + auth("other") + `"}}}`,
			wantUsername: "",
		},
		{
			name:         "matches the conventional Docker Hub auth key",
			image:        "ubuntu:latest",
			dockerConfig: `{"auths":{"https://index.docker.io/v1/":{"auth":"` + auth("dockerhub") + `"}}}`,
			wantUsername: "dockerhub",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "registry-creds", Namespace: "models"},
				Type:       corev1.SecretTypeDockerConfigJson,
				Data:       map[string][]byte{corev1.DockerConfigJsonKey: []byte(tt.dockerConfig)},
			}
			source := &fakeLabelSource{labels: []map[string]string{{}}}
			resolver := newRegistryResolver(fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(), source)
			if _, err := resolver.Resolve(context.Background(), tt.image, "models", []string{"registry-creds"}); err != nil {
				t.Fatal(err)
			}
			if source.auth == nil || source.auth.Username != tt.wantUsername {
				t.Fatalf("resolved username = %#v, want %q", source.auth, tt.wantUsername)
			}
		})
	}
}

func TestRegistryResolverReportsCredentialAndRegistryErrors(t *testing.T) {
	t.Run("missing pull secret", func(t *testing.T) {
		resolver := newRegistryResolver(fake.NewClientBuilder().Build(), &fakeLabelSource{})
		_, err := resolver.Resolve(context.Background(), testImage, "models", []string{"missing"})
		if err == nil || !strings.Contains(err.Error(), "pull secret") {
			t.Fatalf("Resolve() error = %v, want pull secret context", err)
		}
	})

	t.Run("malformed pull secret", func(t *testing.T) {
		scheme := runtime.NewScheme()
		if err := corev1.AddToScheme(scheme); err != nil {
			t.Fatal(err)
		}
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "broken", Namespace: "models"},
			Type:       corev1.SecretTypeDockerConfigJson,
			Data:       map[string][]byte{corev1.DockerConfigJsonKey: []byte(`{"auths":`)},
		}
		resolver := newRegistryResolver(fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(), &fakeLabelSource{})
		_, err := resolver.Resolve(context.Background(), testImage, "models", []string{"broken"})
		if err == nil || !strings.Contains(err.Error(), "parse image pull secret") {
			t.Fatalf("Resolve() error = %v, want malformed pull secret context", err)
		}
	})

	t.Run("registry failure", func(t *testing.T) {
		resolver := newRegistryResolver(fake.NewClientBuilder().Build(), &fakeLabelSource{err: errors.New("unauthorized")})
		_, err := resolver.Resolve(context.Background(), testImage, "models", nil)
		if err == nil || !strings.Contains(err.Error(), "inspect image") || !strings.Contains(err.Error(), "unauthorized") {
			t.Fatalf("Resolve() error = %v, want image and registry context", err)
		}
	})
}
