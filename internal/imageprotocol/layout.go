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
	"fmt"
	"path"

	corev1 "k8s.io/api/core/v1"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

const (
	LegacyModelMountPath = "/model-store"
	NativeModelMountPath = "/model"
	ModelPathEnv         = "NIM_ENGINE_MODEL_PATH"
)

type ModelLayout struct {
	Protocol  Protocol
	MountPath string
	ModelPath string
}

// ResolveModelLayout determines the model volume layout used by a serving
// workload and verifies that a referenced native cache uses the same protocol.
func ResolveModelLayout(ctx context.Context, resolver Resolver, nimService *appsv1alpha1.NIMService, nimCache *appsv1alpha1.NIMCache) (ModelLayout, error) {
	protocol, err := resolver.Resolve(ctx, nimService.GetImage(), nimService.Namespace, nimService.GetImagePullSecrets())
	if err != nil {
		return ModelLayout{}, fmt.Errorf("resolve serving image model download protocol: %w", err)
	}

	if nimCache != nil {
		cacheProtocol := Legacy
		if nimCache.Spec.Source.NGC != nil && nimCache.Spec.Source.NGC.ModelEndpoint == nil && nimCache.GetModelPuller() != "" {
			pullSecrets := []string(nil)
			if pullSecret := nimCache.GetPullSecret(); pullSecret != "" {
				pullSecrets = []string{pullSecret}
			}
			cacheProtocol, err = resolver.Resolve(ctx, nimCache.GetModelPuller(), nimCache.Namespace, pullSecrets)
			if err != nil {
				return ModelLayout{}, fmt.Errorf("resolve NIMCache image model download protocol: %w", err)
			}
		}
		if cacheProtocol != protocol {
			return ModelLayout{}, fmt.Errorf("serving image model download protocol %q does not match NIMCache image protocol %q", protocol, cacheProtocol)
		}
	}

	if !protocol.IsNative() {
		return ModelLayout{Protocol: protocol, MountPath: LegacyModelMountPath}, nil
	}

	modelPath := NativeModelMountPath
	if nimCache != nil {
		if !pvcIsCreated(nimCache.Spec.Storage.PVC) {
			modelPath = path.Join(NativeModelMountPath, nimCache.Name)
		}
		modelPath = envValueOrDefault(nimCache.Spec.Env, ModelPathEnv, modelPath)
	} else if !pvcIsCreated(nimService.Spec.Storage.PVC) {
		modelPath = path.Join(NativeModelMountPath, nimService.Name)
	}
	modelPath = envValueOrDefault(nimService.Spec.Env, ModelPathEnv, modelPath)

	return ModelLayout{Protocol: protocol, MountPath: NativeModelMountPath, ModelPath: modelPath}, nil
}

func pvcIsCreated(pvc appsv1alpha1.PersistentVolumeClaim) bool {
	return pvc.Create != nil && *pvc.Create
}

func envValueOrDefault(env []corev1.EnvVar, name, defaultValue string) string {
	for _, value := range env {
		if value.Name == name && value.Value != "" {
			return value.Value
		}
	}
	return defaultValue
}
