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

	nvidiaresourcev1beta1 "github.com/NVIDIA/k8s-dra-driver-gpu/api/nvidia.com/resource/v1beta1"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
	k8sutilcel "github.com/NVIDIA/k8s-nim-operator/internal/k8sutil/cel"
	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

const (
	podClaimNamePrefix = "claim"
)

const (
	noIndexSuffix = -1
)

type DraResourceFieldType int

const (
	DRAResourceFieldTypeClaim DraResourceFieldType = iota
	DRAResourceFieldTypeClaimTemplate
	DRAResourceFieldTypeComputeDomainClaimTemplate
)

type NamedDRAResource struct {
	Name string
	appsv1alpha1.DRAResource
	FieldType    DraResourceFieldType
	ResourceName string
}

func (n *NamedDRAResource) IsClaim() bool {
	return n.FieldType == DRAResourceFieldTypeClaim
}

func (n *NamedDRAResource) IsClaimTemplate() bool {
	return n.FieldType == DRAResourceFieldTypeClaimTemplate
}

func (n *NamedDRAResource) IsComputeDomainClaimTemplate() bool {
	return n.FieldType == DRAResourceFieldTypeComputeDomainClaimTemplate
}

type NamedDRAResourceList struct {
	Resources []NamedDRAResource
	NameCache map[string]int
}

func NewNamedDRAResourceList(ctx context.Context, k8sClient client.Client, nimService *appsv1alpha1.NIMService) (*NamedDRAResourceList, error) {
	nameCache := make(map[string]int)
	// Provide an extra buffer for a compute domain, if any.
	namedDraResources := make([]NamedDRAResource, len(nimService.Spec.DRAResources), len(nimService.Spec.DRAResources)+1)
	namedDraResourcesList := &NamedDRAResourceList{
		Resources: namedDraResources,
		NameCache: nameCache,
	}

	for idx, resource := range nimService.Spec.DRAResources {
		namedDraResources[idx] = NamedDRAResource{
			DRAResource: resource,
		}

		switch {
		case resource.ResourceClaimName != nil:
			namedDraResources[idx].FieldType = DRAResourceFieldTypeClaim
			namedDraResources[idx].ResourceName = *resource.ResourceClaimName
		case resource.ResourceClaimTemplateName != nil:
			namedDraResources[idx].FieldType = DRAResourceFieldTypeClaimTemplate
			namedDraResources[idx].ResourceName = *resource.ResourceClaimTemplateName
		case ShouldCreateDRAResource(resource):
			namedDraResources[idx].FieldType = DRAResourceFieldTypeClaimTemplate
			namedDraResources[idx].ResourceName = generateUniqueDRAResourceName(nimService.Name, resource.ClaimCreationSpec.GetNamePrefix(), idx)
		}

		namedDraResources[idx].Name = generateUniquePodClaimName(nameCache, nimService.Name, namedDraResources[idx].ResourceName, namedDraResources[idx].FieldType)
	}

	if nimService.IsComputeDomainEnabled() {
		shouldCreate := (nimService.Spec.MultiNode.ComputeDomain.Create != nil && *nimService.Spec.MultiNode.ComputeDomain.Create)
		// Add the resourceClaimTemplate from the computeDomain to the namedDraResources list.
		err := namedDraResourcesList.addComputeDomainNamedDRAResource(ctx, k8sClient, nimService, shouldCreate)
		if err != nil {
			return nil, err
		}
	}
	return namedDraResourcesList, nil
}

func (n *NamedDRAResourceList) addComputeDomainNamedDRAResource(ctx context.Context, k8sClient client.Client, nimService *appsv1alpha1.NIMService, shouldCreate bool) error {
	idx := len(n.Resources)
	n.Resources = n.Resources[:idx+1]
	n.Resources[idx] = NamedDRAResource{
		FieldType: DRAResourceFieldTypeComputeDomainClaimTemplate,
	}

	// Generate a unique resourceClaimTemplate name for the computeDomain that will be created.
	if shouldCreate {
		resourceName := generateUniqueDRAResourceName(nimService.GetComputeDomainName(), "cd-claimtemplate", noIndexSuffix)
		n.Resources[idx].ResourceName = resourceName
		n.Resources[idx].Name = generateUniquePodClaimName(n.NameCache, nimService.Name, resourceName, n.Resources[idx].FieldType)
		return nil
	}

	computeDomain := &nvidiaresourcev1beta1.ComputeDomain{}
	err := k8sClient.Get(ctx, client.ObjectKey{Name: nimService.GetComputeDomainName(), Namespace: nimService.GetNamespace()}, computeDomain)
	if err != nil {
		return err
	}
	// Use the resourceClaimTemplate name from the existing computeDomain.
	resourceName := computeDomain.Spec.Channel.ResourceClaimTemplate.Name
	n.Resources[idx].ResourceName = resourceName
	n.Resources[idx].Name = generateUniquePodClaimName(n.NameCache, nimService.Name, resourceName, n.Resources[idx].FieldType)
	return nil
}

func (n *NamedDRAResourceList) GetComputeDomainNamedDRAResource() *NamedDRAResource {
	for _, resource := range n.Resources {
		if resource.IsComputeDomainClaimTemplate() {
			return &resource
		}
	}
	return nil
}

func (n *NamedDRAResourceList) UpdateContainerResourceClaims(containers []corev1.Container) []corev1.Container {
	for _, resource := range n.Resources {
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
	return containers
}

func (n *NamedDRAResourceList) GetPodResourceClaims() []corev1.PodResourceClaim {
	claims := make([]corev1.PodResourceClaim, len(n.Resources))
	for idx, resource := range n.Resources {
		claims[idx] = corev1.PodResourceClaim{
			Name: resource.Name,
		}
		if resource.IsClaim() {
			claims[idx].ResourceClaimName = &resource.ResourceName
		} else {
			claims[idx].ResourceClaimTemplateName = &resource.ResourceName
		}
	}
	return claims
}

func (n *NamedDRAResourceList) GenerateDRAResourceStatuses(ctx context.Context, client client.Client, namespace string) ([]appsv1alpha1.DRAResourceStatus, error) {
	statuses := make([]appsv1alpha1.DRAResourceStatus, len(n.Resources))
	for idx, resource := range n.Resources {
		status, err := generateDRAResourceStatus(ctx, client, namespace, &resource)
		if err != nil {
			return nil, err
		}
		statuses[idx] = *status
	}
	return statuses, nil
}

func generateUniquePodClaimName(nameCache map[string]int, nimServiceName string, resourceName string, fieldType DraResourceFieldType) string {
	nimServiceNameHash := utils.GetTruncatedStringHash(nimServiceName, 12)
	resourceNameHash := utils.GetTruncatedStringHash(resourceName, 12)
	uniqueName := fmt.Sprintf("%s-%s-%d-%s", podClaimNamePrefix, nimServiceNameHash, fieldType, resourceNameHash)
	count, ok := nameCache[uniqueName]
	if ok {
		nameCache[uniqueName] = count + 1
	} else {
		nameCache[uniqueName] = 0
	}
	return fmt.Sprintf("%s-%d", uniqueName, nameCache[uniqueName])
}

func generateUniqueDRAResourceName(nimServiceName string, namePrefix string, idx int) string {
	nimServiceNameHash := utils.GetTruncatedStringHash(nimServiceName, 12)
	// If idx is
	if idx == noIndexSuffix {
		return fmt.Sprintf("%s-%s", namePrefix, nimServiceNameHash)
	}
	return fmt.Sprintf("%s-%s-%d", namePrefix, nimServiceNameHash, idx)
}

func generateDRAResourceStatus(ctx context.Context, client client.Client, namespace string, resource *NamedDRAResource) (*appsv1alpha1.DRAResourceStatus, error) {
	status := &appsv1alpha1.DRAResourceStatus{
		Name: resource.Name,
	}
	if resource.IsClaim() {
		claim, err := k8sutil.GetResourceClaim(ctx, client, resource.ResourceName, namespace)
		if err != nil {
			return nil, err
		}
		status.ResourceClaimStatus = getDRAResourceClaimStatus(claim)
		return status, nil
	}
	claimTemplateStatus := appsv1alpha1.DRAResourceClaimTemplateStatusInfo{
		Name: resource.ResourceName,
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
	return status, nil
}

func getDRAResourceClaimStatus(resourceClaim *resourcev1.ResourceClaim) *appsv1alpha1.DRAResourceClaimStatusInfo {
	claimStatus := &appsv1alpha1.DRAResourceClaimStatusInfo{
		Name:  resourceClaim.GetName(),
		State: k8sutil.GetResourceClaimState(resourceClaim),
	}
	return claimStatus
}

func ShouldCreateDRAResource(resource appsv1alpha1.DRAResource) bool {
	return resource.ClaimCreationSpec != nil
}

func GetDRADeviceCELExpressions(device appsv1alpha1.DRADeviceSpec) ([]string, error) {
	celExpressions := make([]string, 0)
	celExpressions = append(celExpressions, fmt.Sprintf("device.driver == %q", device.DriverName))
	if len(device.CELExpressions) > 0 {
		if len(device.AttributeSelectors) > 0 || len(device.CapacitySelectors) > 0 {
			return nil, fmt.Errorf("CELExpressions must not be set if attributeSelectors or capacitySelectors are set")
		}
		for _, expr := range device.CELExpressions {
			err := k8sutilcel.ValidateExpr(expr)
			if err != nil {
				return nil, err
			}
			celExpressions = append(celExpressions, expr)
		}
		return celExpressions, nil
	}

	for _, selector := range device.AttributeSelectors {
		expr, err := selector.GetCELExpression(device.DriverName)
		if err != nil {
			return nil, err
		}
		if expr != "" {
			celExpressions = append(celExpressions, expr)
		}
	}

	for _, selector := range device.CapacitySelectors {
		expr, err := selector.GetCELExpression(device.DriverName)
		if err != nil {
			return nil, err
		}
		if expr != "" {
			celExpressions = append(celExpressions, expr)
		}
	}
	return celExpressions, nil
}
