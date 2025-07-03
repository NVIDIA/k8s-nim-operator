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

	corev1 "k8s.io/api/core/v1"
	resourcev1beta2 "k8s.io/api/resource/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

const (
	podClaimNamePrefix = "claim-"
)

type NamedDRAResource struct {
	Name string
	appsv1alpha1.DRAResource
}

func UpdateContainerResourceClaims(containers []corev1.Container, draContainerClaims []NamedDRAResource) {
	for _, resource := range draContainerClaims {
		var found bool
		// Check if the resource claim is already referenced by a container.
		for _, container := range containers {
			for _, claim := range container.Resources.Claims {
				if claim.Name == resource.Name {
					found = true
					break
				}
			}
		}

		// Add unreferenced resource claims to containers to prevent container-runtime from overprovisioning.
		if !found {
			for idx := range containers {
				if len(resource.Requests) == 0 {
					containers[idx].Resources.Claims = append(containers[idx].Resources.Claims, corev1.ResourceClaim{
						Name: resource.Name,
					})
					continue
				}

				for _, request := range resource.Requests {
					containers[idx].Resources.Claims = append(containers[idx].Resources.Claims, corev1.ResourceClaim{
						Name:    resource.Name,
						Request: request,
					})
				}
			}
		}
	}
}

func GenerateNamedDRAResources(nimService *appsv1alpha1.NIMService) []NamedDRAResource {
	nameCache := make(map[string]int)
	claims := make([]NamedDRAResource, len(nimService.Spec.DRAResources))
	for idx, resource := range nimService.Spec.DRAResources {
		draResource := appsv1alpha1.DRAResource{
			ResourceClaimName:         resource.ResourceClaimName,
			ResourceClaimTemplateName: resource.ResourceClaimTemplateName,
			Requests:                  resource.Requests,
		}
		claims[idx] = NamedDRAResource{
			Name:        GenerateUniquePodClaimName(nameCache, nimService.Name, &draResource),
			DRAResource: draResource,
		}
	}
	return claims
}

func GenerateUniquePodClaimName(nameCache map[string]int, nimServiceName string, resource *appsv1alpha1.DRAResource) string {
	var fieldIdx int
	claimName := resource.ResourceClaimName
	if resource.ResourceClaimTemplateName != nil {
		claimName = resource.ResourceClaimTemplateName
		fieldIdx = 1
	}

	nimServiceNameHash := utils.GetTruncatedStringHash(nimServiceName, 12)
	claimNameHash := utils.GetTruncatedStringHash(*claimName, 12)
	uniqueName := fmt.Sprintf("%s%s-%d-%s", podClaimNamePrefix, nimServiceNameHash, fieldIdx, claimNameHash)
	count, ok := nameCache[uniqueName]
	if ok {
		nameCache[uniqueName] = count + 1
	} else {
		nameCache[uniqueName] = 0
	}
	return fmt.Sprintf("%s-%d", uniqueName, nameCache[uniqueName])
}

func GetPodResourceClaims(draResources []NamedDRAResource) []corev1.PodResourceClaim {
	claims := make([]corev1.PodResourceClaim, len(draResources))
	for idx, resource := range draResources {
		claims[idx] = corev1.PodResourceClaim{
			Name:                      resource.Name,
			ResourceClaimName:         resource.ResourceClaimName,
			ResourceClaimTemplateName: resource.ResourceClaimTemplateName,
		}
	}
	return claims
}

func GenerateDRAResourceStatuses(ctx context.Context, client client.Client, namespace string, draResources []NamedDRAResource) ([]appsv1alpha1.DRAResourceStatus, error) {
	statuses := make([]appsv1alpha1.DRAResourceStatus, len(draResources))
	for idx, resource := range draResources {
		status, err := generateDRAResourceStatus(ctx, client, namespace, &resource)
		if err != nil {
			return nil, err
		}
		statuses[idx] = *status
	}
	return statuses, nil
}

func generateDRAResourceStatus(ctx context.Context, client client.Client, namespace string, resource *NamedDRAResource) (*appsv1alpha1.DRAResourceStatus, error) {
	status := &appsv1alpha1.DRAResourceStatus{
		Name: resource.Name,
	}
	if resource.ResourceClaimName != nil {
		claim, err := k8sutil.GetResourceClaim(ctx, client, *resource.ResourceClaimName, namespace)
		if err != nil {
			return nil, err
		}
		status.ResourceClaimStatus = getDRAResourceClaimStatus(claim)
		return status, nil
	}
	if resource.ResourceClaimTemplateName != nil {
		claimTemplateStatus := appsv1alpha1.DRAResourceClaimTemplateStatusInfo{
			Name: *resource.ResourceClaimTemplateName,
		}

		resourceClaims, err := k8sutil.ListResourceClaimsByPodClaimName(ctx, client, namespace, resource.Name)
		if err != nil {
			return nil, err
		}
		for _, claim := range resourceClaims {
			claimStatus := getDRAResourceClaimStatus(&claim)
			claimTemplateStatus.ResourceClaimStatuses = append(claimTemplateStatus.ResourceClaimStatuses, *claimStatus)
		}
		status.ResourceClaimTemplateStatus = &claimTemplateStatus
	}
	return status, nil
}

func getDRAResourceClaimStatus(resourceClaim *resourcev1beta2.ResourceClaim) *appsv1alpha1.DRAResourceClaimStatusInfo {
	claimStatus := &appsv1alpha1.DRAResourceClaimStatusInfo{
		Name:  resourceClaim.GetName(),
		State: k8sutil.GetResourceClaimState(resourceClaim),
	}
	return claimStatus
}
