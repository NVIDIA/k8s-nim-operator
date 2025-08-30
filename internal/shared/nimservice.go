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
	"fmt"
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

// GetGPUCountPerPod returns the number of GPUs per pod for the NIMService.
func GetGPUCountPerPod(ctx context.Context, client client.Client, nimService *appsv1alpha1.NIMService) (int, error) {

	if len(nimService.Spec.DRAResources) == 0 {
		if nimService.Spec.Resources == nil {
			return 0, fmt.Errorf("GPU resource not specified for NIMService %s in namespace %s", nimService.GetName(), nimService.GetNamespace())
		}
		gpuQuantity, ok := nimService.Spec.Resources.Requests["nvidia.com/gpu"]
		if !ok {
			gpuQuantity, ok = nimService.Spec.Resources.Limits["nvidia.com/gpu"]
			if !ok {
				return 0, fmt.Errorf("GPU resource requests/limits not specified for NIMService %s in namespace %s", nimService.GetName(), nimService.GetNamespace())
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

func CreateGPUCountPerPodAnnotation(ctx context.Context, client client.Client, nimService *appsv1alpha1.NIMService) error {

	if nimService.Annotations == nil {
		nimService.Annotations = map[string]string{}
	}
	if _, ok := nimService.Annotations[utils.GPUCountPerPodAnnotationKey]; !ok {
		gpuCountPerPod, err := GetGPUCountPerPod(ctx, client, nimService)
		if err != nil {
			return err
		}
		nimService.Annotations[utils.GPUCountPerPodAnnotationKey] = strconv.Itoa(gpuCountPerPod)
	}
	return nil
}
