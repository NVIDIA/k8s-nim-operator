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
	corev1 "k8s.io/api/core/v1"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

func UpdateContainerResourceClaims(containers []corev1.Container, resourceClaims []appsv1alpha1.DRAResource) {
	for _, rc := range resourceClaims {
		var found bool
		// Check if the resource claim is already referenced by a container.
		for _, container := range containers {
			for _, claim := range container.Resources.Claims {
				if claim.Name == rc.Name {
					found = true
					break
				}
			}
		}

		// Add unreferenced resource claims to containers to prevent container-runtime from overprovisioning.
		if !found {
			for idx := range containers {
				if len(rc.Requests) == 0 {
					containers[idx].Resources.Claims = append(containers[idx].Resources.Claims, corev1.ResourceClaim{
						Name: rc.Name,
					})
					continue
				}

				for _, request := range rc.Requests {
					containers[idx].Resources.Claims = append(containers[idx].Resources.Claims, corev1.ResourceClaim{
						Name:    rc.Name,
						Request: request,
					})
				}
			}
		}
	}
}
