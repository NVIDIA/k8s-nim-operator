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
	"strings"

	corev1 "k8s.io/api/core/v1"
	resourcev1beta2 "k8s.io/api/resource/v1beta2"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
	k8sutilcel "github.com/NVIDIA/k8s-nim-operator/internal/k8sutil/cel"
	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

const (
	podClaimNamePrefix = "claim"
)

type DraResourceFieldType int

const (
	DRAResourceFieldTypeClaim DraResourceFieldType = iota
	DRAResourceFieldTypeClaimTemplate
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

func UpdateContainerResourceClaims(containers []corev1.Container, resources []NamedDRAResource) {
	for _, resource := range resources {
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
	namedDraResources := make([]NamedDRAResource, len(nimService.Spec.DRAResources))
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
	return namedDraResources
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
	uniqueName := fmt.Sprintf("%s-%s-%d", namePrefix, nimServiceNameHash, idx)
	return uniqueName
}

func GetPodResourceClaims(resources []NamedDRAResource) []corev1.PodResourceClaim {
	claims := make([]corev1.PodResourceClaim, len(resources))
	for idx, resource := range resources {
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

func getDRAResourceClaimStatus(resourceClaim *resourcev1beta2.ResourceClaim) *appsv1alpha1.DRAResourceClaimStatusInfo {
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

// Constants for GPU device detection.
const (
	// NVIDIA identifiers used to detect GPU devices.
	NVIDIAGPUComIdentifier = "gpu.nvidia.com"
)

// isNVIDIAGPU checks if a device class represents an NVIDIA GPU.
func isNVIDIAGPU(ctx context.Context, client client.Client, deviceClassName string) (bool, error) {
	if deviceClassName == "" {
		return false, nil
	}

	var dc resourcev1beta2.DeviceClass
	if err := client.Get(ctx, types.NamespacedName{Name: deviceClassName}, &dc); err != nil {
		return false, fmt.Errorf("failed to get device class %s: %w", deviceClassName, err)
	}

	hint := strings.ToLower(fmt.Sprint(dc.Spec))
	return strings.Contains(hint, NVIDIAGPUComIdentifier), nil
}

// GetGPUCountForDRAResources calculates the total GPU count across all DRA resources.
func GetGPUCountForDRAResources(ctx context.Context, client client.Client, namespace string, resourceClaimName string, nimService *appsv1alpha1.NIMService) (int, error) {
	totalGPUCount := 0
	for i, resource := range nimService.Spec.DRAResources {
		var gpuCount int
		var err error

		switch {
		case resource.ResourceClaimName != nil:
			gpuCount, err = GetGPUDeviceCountForClaim(ctx, client, *resource.ResourceClaimName, namespace)
		case resource.ResourceClaimTemplateName != nil:
			gpuCount, err = GetGPUDeviceCountForClaimTemplate(ctx, client, *resource.ResourceClaimTemplateName, namespace)
		case resource.ClaimCreationSpec != nil:
			gpuCount, err = GetGPUDeviceCountForClaimCreationSpec(ctx, client, *resource.ClaimCreationSpec, namespace)
		}

		if err != nil {
			return 0, fmt.Errorf("failed to get GPU count for DRA resource at index %d: %w", i, err)
		}

		totalGPUCount += gpuCount
	}

	return totalGPUCount, nil
}

// GetGPUDeviceCountForClaim calculates GPU count for a specific resource claim.
func GetGPUDeviceCountForClaim(ctx context.Context, client client.Client, resourceClaimName string, namespace string) (int, error) {
	if resourceClaimName == "" {
		return 0, fmt.Errorf("resourceClaimName cannot be empty")
	}
	if namespace == "" {
		return 0, fmt.Errorf("namespace cannot be empty")
	}

	claim, err := k8sutil.GetResourceClaim(ctx, client, resourceClaimName, namespace)
	if err != nil {
		return 0, fmt.Errorf("failed to get resource claim %s in namespace %s: %w", resourceClaimName, namespace, err)
	}

	return getGPUCountFromDeviceRequests(ctx, client, claim.Spec.Devices.Requests)
}

// GetGPUDeviceCountForClaimTemplate calculates GPU count for a resource claim template.
func GetGPUDeviceCountForClaimTemplate(ctx context.Context, client client.Client, resourceClaimTemplateName string, namespace string) (int, error) {
	if resourceClaimTemplateName == "" {
		return 0, fmt.Errorf("resourceClaimTemplateName cannot be empty")
	}
	if namespace == "" {
		return 0, fmt.Errorf("namespace cannot be empty")
	}

	claimTemplate, err := k8sutil.GetResourceClaimTemplate(ctx, client, resourceClaimTemplateName, namespace)
	if err != nil {
		return 0, fmt.Errorf("failed to get resource claim template %s in namespace %s: %w", resourceClaimTemplateName, namespace, err)
	}

	return getGPUCountFromDeviceRequests(ctx, client, claimTemplate.Spec.Spec.Devices.Requests)
}

// GetGPUDeviceCountForClaimCreationSpec calculates GPU count for a claim creation specification.
func GetGPUDeviceCountForClaimCreationSpec(ctx context.Context, client client.Client, claimCreationSpec appsv1alpha1.DRAClaimCreationSpec, namespace string) (int, error) {
	if namespace == "" {
		return 0, fmt.Errorf("namespace cannot be empty")
	}

	return getGPUCountFromDeviceSpecs(ctx, client, claimCreationSpec.Devices)
}

// getGPUCountFromDeviceRequests is a helper function to extract GPU count from device requests.
func getGPUCountFromDeviceRequests(ctx context.Context, client client.Client, requests []resourcev1beta2.DeviceRequest) (int, error) {

	for _, req := range requests {
		if req.Exactly.DeviceClassName == "" {
			continue
		}

		isGPU, err := isNVIDIAGPU(ctx, client, req.Exactly.DeviceClassName)
		if err != nil {
			return 0, fmt.Errorf("failed to check if device class %s is a GPU: %w", req.Exactly.DeviceClassName, err)
		}

		if isGPU {
			return int(req.Exactly.Count), nil
		}
	}

	return 0, nil
}

// getGPUCountFromDeviceSpecs is a helper function to extract GPU count from device specifications.
func getGPUCountFromDeviceSpecs(ctx context.Context, client client.Client, devices []appsv1alpha1.DRADeviceSpec) (int, error) {

	for _, dev := range devices {
		if dev.DeviceClassName == "" {
			continue
		}

		isGPU, err := isNVIDIAGPU(ctx, client, dev.DeviceClassName)
		if err != nil {
			return 0, fmt.Errorf("failed to check if device class %s is a GPU: %w", dev.DeviceClassName, err)
		}

		if isGPU {
			return int(dev.Count), nil
		}
	}

	return 0, nil
}
