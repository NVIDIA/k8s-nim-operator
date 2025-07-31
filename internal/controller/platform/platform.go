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

package platform

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/controller/platform/kserve"
	"github.com/NVIDIA/k8s-nim-operator/internal/controller/platform/standalone"
	"github.com/NVIDIA/k8s-nim-operator/internal/shared"
)

// InferencePlatform defines the methods required for an inference platform integration.
type InferencePlatform interface {
	Delete(ctx context.Context, r shared.Reconciler, resource client.Object) error
	Sync(ctx context.Context, r shared.Reconciler, resource client.Object) (ctrl.Result, error)
}

// GetInferencePlatform returns an inference platform implementation based on the platform type.
func GetInferencePlatform(inferencePlatformType appsv1alpha1.PlatformType) (InferencePlatform, error) {
	switch inferencePlatformType {
	case appsv1alpha1.PlatformTypeStandalone, "": // Default to standalone for empty values
		return &standalone.Standalone{}, nil
	case appsv1alpha1.PlatformTypeKServe:
		return &kserve.KServe{}, nil
	default:
		return nil, fmt.Errorf("unsupported platform type: %s", inferencePlatformType)
	}
}
