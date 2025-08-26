/*
Copyright 2025.

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

package shared

import (
	"context"
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

// GetGPUCountPerPod returns the number of GPUs per pod for the NIMService.
func GetGPUCountPerPod(ctx context.Context, client client.Client, nimService *appsv1alpha1.NIMService) (int, error) {

	if len(nimService.Spec.DRAResources) == 0 {
		if nimService.Spec.Resources == nil {
			return 0, nil
		}
		gpuQuantity, ok := nimService.Spec.Resources.Requests["nvidia.com/gpu"]
		if !ok {
			gpuQuantity, ok = nimService.Spec.Resources.Limits["nvidia.com/gpu"]
			if !ok {
				return 0, errors.New("no GPU request or limit is specified for multi-node NIMService")
			}
		}
		return int(gpuQuantity.Value()), nil
	} else {
		gpuCount, err := GetGPUCountForDRAResources(ctx, client, nimService.GetNamespace(), nimService.GetName(), nimService)
		if err != nil {
			return 0, err
		}
		return gpuCount, nil
	}
}
