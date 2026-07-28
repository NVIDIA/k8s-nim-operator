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
	"encoding/json"
	"fmt"
	"io"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

// ModelDownloadProtocolLabel is the OCI image config label that advertises how a
// NIM downloads its model(s). The operator reads this label from the registry
// before starting any container.
const ModelDownloadProtocolLabel = "com.nvidia.nim.model_download_protocol"

// Protocol is the model-download contract advertised by a NIM image.
type Protocol string

const (
	// Legacy is the default Python/nim-sdk contract (model_manifest.yaml,
	// list-model-profiles, download-to-cache, /model-store).
	Legacy Protocol = ""
	// NativeV1 is the NIMCraft single-model runtime that downloads on start
	// (no download-to-cache) and persists weights under /model.
	NativeV1 Protocol = "native-v1"
)

// IsNative reports whether the protocol uses the native download/serve layout.
func (p Protocol) IsNative() bool {
	return p == NativeV1
}

// ProtocolResolver determines a NIM image's model-download protocol without
// starting the container.
type ProtocolResolver interface {
	Resolve(ctx context.Context, image, namespace string, pullSecrets []string) (Protocol, error)
}

// ModelLayout describes the volume layout for a serving workload.
type ModelLayout struct {
	Protocol Protocol
}

// ResolveModelLayout determines the model volume layout used by a serving
// workload.
//
// When the workload is backed by a NIMCache, the NIMCache controller has already
// resolved the model download protocol from the image's OCI label and persisted
// it on the NIMCache status. A NIMService only renders its workload after its
// NIMCache reports Ready, so that status is an authoritative, already-computed
// source of truth. Trusting it avoids a registry round-trip on every NIMService
// reconcile (and the accompanying failure surface) for the common cached path.
//
// For a direct NIMService with no NIMCache, the serving image is inspected via
// ResolveProtocolOrLegacy, which falls back to legacy if the image cannot be
// inspected (or no resolver is configured), keeping the feature zero-impact for
// legacy NIMs.
func ResolveModelLayout(ctx context.Context, resolver ProtocolResolver, nimService *appsv1alpha1.NIMService, nimCache *appsv1alpha1.NIMCache) (ModelLayout, error) {
	if nimService == nil {
		return ModelLayout{}, fmt.Errorf("nil NIMService")
	}

	if nimCache != nil && nimCache.Name != "" {
		if nimCache.IsNativeModelDownload() {
			return ModelLayout{Protocol: NativeV1}, nil
		}
		return ModelLayout{Protocol: Legacy}, nil
	}

	protocol := ResolveProtocolOrLegacy(ctx, resolver, nimService.GetImage(), nimService.Namespace, nimService.GetImagePullSecrets())
	return ModelLayout{Protocol: protocol}, nil
}

// ResolveProtocolOrLegacy resolves an image's model download protocol and defaults to Legacy.
// This keeps pre-start protocol detection zero-impact for legacy NIMs: only a successful inspection that positively
// reports native-v1 diverts to the native path.
func ResolveProtocolOrLegacy(ctx context.Context, resolver ProtocolResolver, image, namespace string, pullSecrets []string) Protocol {
	if resolver == nil {
		return Legacy
	}
	protocol, err := resolver.Resolve(ctx, image, namespace, pullSecrets)
	if err != nil {
		log.FromContext(ctx).Info("could not inspect image for model download protocol; assuming legacy",
			"image", image, "error", err.Error())
		return Legacy
	}
	return protocol
}

// NativeModelPathForCache returns the operator-owned model path for a native
// NIMCache instance.
func NativeModelPathForCache(nimCache *appsv1alpha1.NIMCache) string {
	if nimCache == nil || nimCache.Name == "" {
		return utils.NativeModelBasePath
	}
	return utils.NativeModelPath(nimCache.Name)
}

const mediaTypeDockerManifestList = "application/vnd.docker.distribution.manifest.list.v2+json"

const (
	dockerReferenceTypeAnnotation = "vnd.docker.reference.type"
	attestationManifestType       = "attestation-manifest"
)

// labelSource fetches OCI config labels for an image. Tests inject fakes.
type labelSource interface {
	Labels(ctx context.Context, image, username, password string) ([]map[string]string, error)
}

type registryResolver struct {
	secrets client.Reader
	images  labelSource
}

// NewProtocolResolver returns a ProtocolResolver that inspects image config
// labels via ORAS using Kubernetes image pull secrets for authentication.
func NewProtocolResolver(secrets client.Reader) ProtocolResolver {
	return newRegistryResolver(secrets, orasLabelSource{})
}

func newRegistryResolver(secrets client.Reader, images labelSource) ProtocolResolver {
	return &registryResolver{secrets: secrets, images: images}
}

// Resolve inspects every runnable platform configuration for image and returns
// NativeV1 only when every platform advertises that exact label value. An
// absent/unknown label selects Legacy. Registry, authentication, and
// inconsistent-platform errors fail reconciliation.
func (r *registryResolver) Resolve(ctx context.Context, image, namespace string, pullSecrets []string) (Protocol, error) {
	if image == "" {
		return Legacy, fmt.Errorf("empty image reference")
	}

	username, password, err := r.credentials(ctx, namespace, image, pullSecrets)
	if err != nil {
		return Legacy, err
	}

	platformLabels, err := r.images.Labels(ctx, image, username, password)
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

type orasLabelSource struct{}

func (orasLabelSource) Labels(ctx context.Context, image, username, password string) ([]map[string]string, error) {
	ref, err := registry.ParseReference(image)
	if err != nil {
		return nil, fmt.Errorf("parse image reference: %w", err)
	}

	repo, err := remote.NewRepository(image)
	if err != nil {
		return nil, fmt.Errorf("create repository client: %w", err)
	}

	authClient := &auth.Client{
		Client: retry.DefaultClient,
		Cache:  auth.NewCache(),
	}
	authClient.SetUserAgent("k8s-nim-operator")
	if username != "" || password != "" {
		authClient.Credential = auth.StaticCredential(ref.Registry, auth.Credential{
			Username: username,
			Password: password,
		})
	}
	repo.Client = authClient

	desc, rc, err := repo.FetchReference(ctx, ref.Reference)
	if err != nil {
		return nil, fmt.Errorf("resolve reference: %w", err)
	}
	manifestBytes, err := readAllAndClose(rc)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	if desc.MediaType == ocispec.MediaTypeImageIndex || desc.MediaType == mediaTypeDockerManifestList {
		var index ocispec.Index
		if err := json.Unmarshal(manifestBytes, &index); err != nil {
			return nil, fmt.Errorf("unmarshal image index: %w", err)
		}
		var labels []map[string]string
		for _, child := range index.Manifests {
			if !isPlatformImageDescriptor(child) {
				continue
			}
			prc, err := repo.Manifests().Fetch(ctx, child)
			if err != nil {
				return nil, fmt.Errorf("fetch platform manifest %s: %w", child.Digest, err)
			}
			childBytes, err := readAllAndClose(prc)
			if err != nil {
				return nil, fmt.Errorf("read platform manifest %s: %w", child.Digest, err)
			}
			cfgLabels, err := labelsFromManifestBytes(ctx, repo, childBytes)
			if err != nil {
				return nil, fmt.Errorf("read platform config %s: %w", child.Digest, err)
			}
			labels = append(labels, cfgLabels)
		}
		return labels, nil
	}

	cfgLabels, err := labelsFromManifestBytes(ctx, repo, manifestBytes)
	if err != nil {
		return nil, err
	}
	return []map[string]string{cfgLabels}, nil
}

func labelsFromManifestBytes(ctx context.Context, repo *remote.Repository, manifestBytes []byte) (map[string]string, error) {
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("unmarshal manifest: %w", err)
	}
	configBytes, err := content.FetchAll(ctx, repo.Blobs(), manifest.Config)
	if err != nil {
		return nil, fmt.Errorf("fetch image config: %w", err)
	}
	var image ocispec.Image
	if err := json.Unmarshal(configBytes, &image); err != nil {
		return nil, fmt.Errorf("unmarshal image config: %w", err)
	}
	if image.Config.Labels == nil {
		return map[string]string{}, nil
	}
	return image.Config.Labels, nil
}

func isPlatformImageDescriptor(d ocispec.Descriptor) bool {
	if d.Annotations[dockerReferenceTypeAnnotation] == attestationManifestType {
		return false
	}
	if d.Platform != nil && (d.Platform.OS == "unknown" || d.Platform.Architecture == "unknown") {
		return false
	}
	return true
}

func readAllAndClose(rc io.ReadCloser) ([]byte, error) {
	defer rc.Close()
	return io.ReadAll(rc)
}

type dockerConfigJSON struct {
	Auths map[string]dockerAuthEntry `json:"auths"`
}

type dockerAuthEntry struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Auth     string `json:"auth"`
}

func (r *registryResolver) credentials(ctx context.Context, namespace, image string, pullSecrets []string) (string, string, error) {
	if len(pullSecrets) == 0 {
		return "", "", nil
	}
	ref, err := registry.ParseReference(image)
	if err != nil {
		return "", "", fmt.Errorf("parse image reference %q: %w", image, err)
	}
	targetHost := normalizeRegistryHost(ref.Registry)

	for _, secretName := range pullSecrets {
		if secretName == "" {
			continue
		}
		secret := &corev1.Secret{}
		if err := r.secrets.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err != nil {
			return "", "", fmt.Errorf("get image pull secret %q in namespace %q: %w", secretName, namespace, err)
		}
		data, ok := secret.Data[corev1.DockerConfigJsonKey]
		if !ok {
			return "", "", fmt.Errorf("pull secret %q in namespace %q does not contain %s", secretName, namespace, corev1.DockerConfigJsonKey)
		}
		username, password, found, err := credentialsForHost(targetHost, data)
		if err != nil {
			return "", "", fmt.Errorf("parse image pull secret %q in namespace %q: %w", secretName, namespace, err)
		}
		if found {
			return username, password, nil
		}
	}
	// No matching registry entry: anonymous access. Do not fall back to an
	// unrelated single auth entry — that can send the wrong credentials.
	return "", "", nil
}

// credentialsForHost extracts credentials for targetHost from a dockerconfigjson
// payload. Matching is host-scoped (and Docker Hub alias aware). Returns
// found=false when no matching entry exists.
func credentialsForHost(targetHost string, configJSON []byte) (string, string, bool, error) {
	var cfg dockerConfigJSON
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return "", "", false, fmt.Errorf("unmarshal dockerconfigjson: %w", err)
	}

	aliases := registryAliases(targetHost)
	var entry dockerAuthEntry
	var matched bool
	for host, e := range cfg.Auths {
		normalized := normalizeRegistryHost(host)
		for _, alias := range aliases {
			if normalized == alias {
				entry = e
				matched = true
				break
			}
		}
		if matched {
			break
		}
	}
	if !matched {
		return "", "", false, nil
	}

	if entry.Username != "" || entry.Password != "" {
		return entry.Username, entry.Password, true, nil
	}
	if entry.Auth != "" {
		decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
		if err != nil {
			return "", "", false, fmt.Errorf("decode auth for registry %q: %w", targetHost, err)
		}
		user, pass, found := strings.Cut(string(decoded), ":")
		if !found {
			return "", "", false, fmt.Errorf("malformed auth entry for registry %q", targetHost)
		}
		return user, pass, true, nil
	}
	return "", "", true, nil
}

func registryAliases(host string) []string {
	host = normalizeRegistryHost(host)
	switch host {
	case "", "docker.io", "index.docker.io", "registry-1.docker.io":
		return []string{"docker.io", "index.docker.io", "registry-1.docker.io"}
	default:
		return []string{host}
	}
}

func normalizeRegistryHost(host string) string {
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	if idx := strings.IndexByte(host, '/'); idx != -1 {
		host = host[:idx]
	}
	return host
}
