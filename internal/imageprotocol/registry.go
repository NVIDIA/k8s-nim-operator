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
	"encoding/json"
	"fmt"
	"net/url"
	pathpkg "path"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	registryv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type labelSource interface {
	Labels(ctx context.Context, image string, keychain authn.Keychain) ([]map[string]string, error)
}

type registryResolver struct {
	secrets client.Reader
	images  labelSource
}

func NewRegistryResolver(secrets client.Reader) Resolver {
	return newRegistryResolver(secrets, remoteLabelSource{})
}

func newRegistryResolver(secrets client.Reader, images labelSource) Resolver {
	return &registryResolver{secrets: secrets, images: images}
}

func (r *registryResolver) Resolve(ctx context.Context, image, namespace string, pullSecrets []string) (Protocol, error) {
	keychain, err := r.keychain(ctx, namespace, pullSecrets)
	if err != nil {
		return Legacy, err
	}

	platformLabels, err := r.images.Labels(ctx, image, keychain)
	if err != nil {
		return Legacy, fmt.Errorf("inspect image %q: %w", image, err)
	}
	if len(platformLabels) == 0 {
		return Legacy, fmt.Errorf("inspect image %q: no image configurations found", image)
	}

	labelValue := platformLabels[0][ModelDownloadProtocolLabel]
	for _, labels := range platformLabels[1:] {
		if current := labels[ModelDownloadProtocolLabel]; current != labelValue {
			return Legacy, fmt.Errorf("inspect image %q: inconsistent model download protocols across platforms (%q and %q)", image, labelValue, current)
		}
	}
	return protocolFromLabelValue(labelValue), nil
}

func protocolFromLabelValue(value string) Protocol {
	if value == string(NativeV1) {
		return NativeV1
	}
	return Legacy
}

type remoteLabelSource struct{}

func (remoteLabelSource) Labels(ctx context.Context, image string, keychain authn.Keychain) ([]map[string]string, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return nil, fmt.Errorf("parse image reference: %w", err)
	}
	descriptor, err := remote.Get(ref, remote.WithContext(ctx), remote.WithAuthFromKeychain(keychain))
	if err != nil {
		return nil, err
	}

	if descriptor.MediaType.IsIndex() {
		index, err := descriptor.ImageIndex()
		if err != nil {
			return nil, fmt.Errorf("read image index: %w", err)
		}
		manifest, err := index.IndexManifest()
		if err != nil {
			return nil, fmt.Errorf("read image index manifest: %w", err)
		}
		labels := make([]map[string]string, 0, len(manifest.Manifests))
		for _, child := range manifest.Manifests {
			if !isPlatformImageDescriptor(child) {
				continue
			}
			childImage, err := index.Image(child.Digest)
			if err != nil {
				return nil, fmt.Errorf("read platform image %s: %w", child.Digest, err)
			}
			config, err := childImage.ConfigFile()
			if err != nil {
				return nil, fmt.Errorf("read platform config %s: %w", child.Digest, err)
			}
			labels = append(labels, config.Config.Labels)
		}
		return labels, nil
	}

	img, err := descriptor.Image()
	if err != nil {
		return nil, fmt.Errorf("read image manifest: %w", err)
	}
	config, err := img.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("read image config: %w", err)
	}
	return []map[string]string{config.Config.Labels}, nil
}

func isPlatformImageDescriptor(descriptor registryv1.Descriptor) bool {
	if descriptor.ArtifactType != "" {
		return false
	}
	if descriptor.Platform != nil && descriptor.Platform.OS == "unknown" && descriptor.Platform.Architecture == "unknown" {
		return false
	}
	for key, value := range descriptor.Annotations {
		if strings.Contains(strings.ToLower(key), "attestation") || strings.Contains(strings.ToLower(value), "attestation") {
			return false
		}
	}
	return true
}

type dockerConfig struct {
	Auths map[string]dockerAuth `json:"auths"`
}

type dockerAuth struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	Auth          string `json:"auth"`
	IdentityToken string `json:"identitytoken"`
	RegistryToken string `json:"registrytoken"`
}

type staticKeychain struct {
	auths []registryAuth
}

func (k staticKeychain) Resolve(resource authn.Resource) (authn.Authenticator, error) {
	target := normalizeCredentialLocation(resource.String())
	bestMatch, bestScore := -1, -1
	for index, entry := range k.auths {
		if credentialLocationMatches(entry.location, target) && len(entry.location) > bestScore {
			bestMatch, bestScore = index, len(entry.location)
		}
	}
	if bestMatch >= 0 {
		return authn.FromConfig(k.auths[bestMatch].auth), nil
	}
	return authn.Anonymous, nil
}

type registryAuth struct {
	location string
	auth     authn.AuthConfig
}

func (r *registryResolver) keychain(ctx context.Context, namespace string, pullSecrets []string) (authn.Keychain, error) {
	var auths []registryAuth
	for _, secretName := range pullSecrets {
		secret := &corev1.Secret{}
		if err := r.secrets.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err != nil {
			return nil, fmt.Errorf("get image pull secret %q in namespace %q: %w", secretName, namespace, err)
		}
		secretAuths, err := parsePullSecret(secret)
		if err != nil {
			return nil, fmt.Errorf("parse image pull secret %q in namespace %q: %w", secretName, namespace, err)
		}
		auths = append(auths, secretAuths...)
	}
	return staticKeychain{auths: auths}, nil
}

func parsePullSecret(secret *corev1.Secret) ([]registryAuth, error) {
	var config dockerConfig
	switch secret.Type {
	case corev1.SecretTypeDockerConfigJson:
		data, ok := secret.Data[corev1.DockerConfigJsonKey]
		if !ok {
			return nil, fmt.Errorf("missing %s", corev1.DockerConfigJsonKey)
		}
		if err := json.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("decode %s: %w", corev1.DockerConfigJsonKey, err)
		}
	case corev1.SecretTypeDockercfg:
		data, ok := secret.Data[corev1.DockerConfigKey]
		if !ok {
			return nil, fmt.Errorf("missing %s", corev1.DockerConfigKey)
		}
		if err := json.Unmarshal(data, &config.Auths); err != nil {
			return nil, fmt.Errorf("decode %s: %w", corev1.DockerConfigKey, err)
		}
	default:
		return nil, fmt.Errorf("unsupported secret type %q", secret.Type)
	}

	auths := make([]registryAuth, 0, len(config.Auths))
	for registry, entry := range config.Auths {
		username, password := entry.Username, entry.Password
		if entry.Auth != "" && username == "" && password == "" {
			decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
			if err != nil {
				return nil, fmt.Errorf("decode auth for registry %q: %w", registry, err)
			}
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("decode auth for registry %q: expected username:password", registry)
			}
			username, password = parts[0], parts[1]
		}
		auths = append(auths, registryAuth{
			location: normalizeCredentialLocation(registry),
			auth: authn.AuthConfig{
				Username:      username,
				Password:      password,
				Auth:          entry.Auth,
				IdentityToken: entry.IdentityToken,
				RegistryToken: entry.RegistryToken,
			},
		})
	}
	return auths, nil
}

func normalizeCredentialLocation(location string) string {
	location = strings.TrimSpace(location)
	if parsed, err := url.Parse(location); err == nil && parsed.Host != "" {
		location = parsed.Host + "/" + strings.TrimPrefix(parsed.Path, "/")
	}
	location = strings.Trim(strings.TrimPrefix(location, "//"), "/")
	parts := strings.SplitN(location, "/", 2)
	parts[0] = normalizeDockerHubHost(parts[0])
	if len(parts) == 1 || (parts[0] == name.DefaultRegistry && parts[1] == "v1") {
		return parts[0]
	}
	return parts[0] + "/" + strings.Trim(parts[1], "/")
}

func normalizeDockerHubHost(host string) string {
	switch host {
	case "docker.io", "registry-1.docker.io", "index.docker.io":
		return name.DefaultRegistry
	default:
		return host
	}
}

func credentialLocationMatches(pattern, target string) bool {
	patternParts := strings.Split(pattern, "/")
	targetParts := strings.Split(target, "/")
	if len(patternParts) > len(targetParts) || !credentialHostMatches(patternParts[0], targetParts[0]) {
		return false
	}
	for index := 1; index < len(patternParts); index++ {
		matched, err := pathpkg.Match(patternParts[index], targetParts[index])
		if err != nil || !matched {
			return false
		}
	}
	return true
}

func credentialHostMatches(pattern, target string) bool {
	patternParts := strings.Split(pattern, ".")
	targetParts := strings.Split(target, ".")
	if len(patternParts) != len(targetParts) {
		return false
	}
	for index := range patternParts {
		matched, err := pathpkg.Match(patternParts[index], targetParts[index])
		if err != nil || !matched {
			return false
		}
	}
	return true
}
