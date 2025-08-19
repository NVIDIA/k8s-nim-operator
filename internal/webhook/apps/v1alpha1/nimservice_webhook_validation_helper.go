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

package v1alpha1

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"

	utilversion "k8s.io/apimachinery/pkg/util/version"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

var validPVCAccessModeStrs = []corev1.PersistentVolumeAccessMode{
	corev1.ReadWriteOnce,
	corev1.ReadOnlyMany,
	corev1.ReadWriteMany,
	corev1.ReadWriteOncePod,
}

var validDRADeviceAttributeSelectorOps = []appsv1alpha1.DRADeviceAttributeSelectorOp{
	appsv1alpha1.DRADeviceAttributeSelectorOpEqual,
	appsv1alpha1.DRADeviceAttributeSelectorOpNotEqual,
	appsv1alpha1.DRADeviceAttributeSelectorOpGreaterThan,
	appsv1alpha1.DRADeviceAttributeSelectorOpGreaterThanOrEqual,
	appsv1alpha1.DRADeviceAttributeSelectorOpLessThan,
	appsv1alpha1.DRADeviceAttributeSelectorOpLessThanOrEqual,
}

var validDRAResourceQuantitySelectorOps = []appsv1alpha1.DRAResourceQuantitySelectorOp{
	appsv1alpha1.DRAResourceQuantitySelectorOpEqual,
}

// validateNIMServiceSpec aggregates all structural validation checks for a NIMService
// object. It is intended to be invoked by both ValidateCreate and ValidateUpdate to
// ensure the resource is well-formed before any other validation (e.g. immutability)
// is performed.
func validateNIMServiceSpec(spec *appsv1alpha1.NIMServiceSpec, fldPath *field.Path, kubeVersion string) field.ErrorList {
	errList := field.ErrorList{}

	// Validate individual sections of the spec using existing helper functions.
	errList = append(errList, validateImageConfiguration(&spec.Image, fldPath.Child("image"))...)
	errList = append(errList, validateAuthSecret(&spec.AuthSecret, fldPath.Child("authSecret"))...)
	errList = append(errList, validateServiceStorageConfiguration(&spec.Storage, fldPath.Child("storage"))...)
	errList = append(errList, validateExposeConfiguration(&spec.Expose, fldPath.Child("expose").Child("ingress"))...)
	errList = append(errList, validateMetricsConfiguration(&spec.Metrics, fldPath.Child("metrics"))...)
	errList = append(errList, validateScaleConfiguration(&spec.Scale, fldPath.Child("scale"))...)
	errList = append(errList, validateResourcesConfiguration(spec.Resources, fldPath.Child("resources"))...)
	errList = append(errList, validateDRAResourcesConfiguration(spec, fldPath, kubeVersion)...)
	errList = append(errList, validateKServeConfiguration(spec, fldPath)...)

	return errList
}

func validateImageConfiguration(image *appsv1alpha1.Image, fldPath *field.Path) field.ErrorList {
	errList := field.ErrorList{}
	if image.Repository == "" {
		errList = append(errList, field.Required(fldPath.Child("repository"), "is required"))
	}
	if image.Tag == "" {
		errList = append(errList, field.Required(fldPath.Child("tag"), "is required"))
	}
	return errList
}

func validateServiceStorageConfiguration(storage *appsv1alpha1.NIMServiceStorage, fldPath *field.Path) field.ErrorList {
	errList := field.ErrorList{}
	// If size limit is defined, it must be greater than 0
	if storage.SharedMemorySizeLimit != nil {
		if storage.SharedMemorySizeLimit.Sign() <= 0 {
			errList = append(errList, field.Invalid(fldPath.Child("sharedMemorySizeLimit"), storage.SharedMemorySizeLimit.String(), "must be > 0"))
		}
	}

	// Check if nimCache is defined (non-empty)
	nimCacheDefined := storage.NIMCache != (appsv1alpha1.NIMCacheVolSpec{})

	// Check if PVC is defined (non-empty)
	pvcDefined := storage.PVC.Create != nil || storage.PVC.Name != "" || storage.PVC.StorageClass != "" ||
		storage.PVC.Size != "" || storage.PVC.VolumeAccessMode != "" || storage.PVC.SubPath != "" ||
		len(storage.PVC.Annotations) > 0

	// Check if HostPath is defined (non-empty)
	hostPathDefined := storage.HostPath != nil && *storage.HostPath != ""

	// Count how many are defined
	definedCount := 0
	if nimCacheDefined {
		definedCount++
	}
	if pvcDefined {
		definedCount++
	}
	if hostPathDefined {
		definedCount++
	}

	// Ensure only one of nimCache, PVC, or HostPath is defined
	if definedCount == 0 {
		errList = append(errList, field.Required(fldPath, fmt.Sprintf("one of %s, %s, or %s must be defined", fldPath.Child("nimCache"), fldPath.Child("pvc"), fldPath.Child("hostPath"))))
	} else if definedCount > 1 {
		errList = append(errList, field.Invalid(fldPath, "multiple storage sources defined", fmt.Sprintf("only one of %s, %s, or %s must be defined", fldPath.Child("nimCache"), fldPath.Child("pvc"), fldPath.Child("hostPath"))))
	}

	// If NIMCache is non-nil, NIMCache.Name must not be empty
	if storage.NIMCache.Profile != "" {
		if storage.NIMCache.Name == "" {
			errList = append(errList, field.Required(fldPath.Child("nimCache").Child("name"), fmt.Sprintf("is required when %s is defined", fldPath.Child("nimCache"))))
		}
	}

	// Enforcing PVC rules if defined
	if pvcDefined {
		errList = append(errList, validatePVCConfiguration(&storage.PVC, fldPath.Child("pvc"))...)
	}

	return errList
}

func validateDRAResourcesConfiguration(spec *appsv1alpha1.NIMServiceSpec, fldPath *field.Path, k8sVersion string) field.ErrorList {
	errList := field.ErrorList{}
	draResourcesPath := fldPath.Child("draResources")

	// If the length is > 0, check k8s compatibility version
	if len(spec.DRAResources) > 0 {
		if !utils.IsVersionGreaterThanOrEqual(k8sVersion, utils.MinSupportedClusterVersionForDRA) {
			errList = append(errList, field.Forbidden(draResourcesPath, fmt.Sprintf("is not supported by NIM-Operator on this cluster, please upgrade to k8s version '%s' or higher", utils.MinSupportedClusterVersionForDRA)))
		}
	}

	seen := make(map[string]struct{})

	for i, dra := range spec.DRAResources {
		idxPath := draResourcesPath.Index(i)

		hasName := dra.ResourceClaimName != nil && *dra.ResourceClaimName != ""
		hasTemplate := dra.ResourceClaimTemplateName != nil && *dra.ResourceClaimTemplateName != ""
		hasSpec := dra.ClaimSpec != nil

		var fieldCount int
		if hasName {
			fieldCount++
		}
		if hasTemplate {
			fieldCount++
		}
		if hasSpec {
			fieldCount++
		}

		// Exactly one of resourceClaimName, resourceClaimTemplateName, or claimSpec must be provided
		if fieldCount == 0 {
			errList = append(errList, field.Required(idxPath, fmt.Sprintf("one of %s, %s, or %s must be provided", fldPath.Child("resourceClaimName"), fldPath.Child("resourceClaimTemplateName"), fldPath.Child("claimSpec"))))
		} else if fieldCount > 1 {
			errList = append(errList, field.Invalid(
				idxPath,
				"multiple dra resource sources defined",
				fmt.Sprintf("must specify exactly one of %s, %s, or %s", fldPath.Child("resourceClaimName"), fldPath.Child("resourceClaimTemplateName"), fldPath.Child("claimSpec"))))
		}

		if hasName {
			// resourceClaimName: spec.relicas must be <= 1 and spec.scale.enabled must be false.
			if spec.Replicas > 1 {
				errList = append(errList, field.Forbidden(
					idxPath.Child("resourceClaimName"),
					fmt.Sprintf("must not be set when %s > 1, use %s instead", fldPath.Child("replicas"), idxPath.Child("resourceClaimTemplateName")),
				))
			}
			if spec.Scale.Enabled != nil && *spec.Scale.Enabled {
				errList = append(errList, field.Forbidden(
					idxPath.Child("resourceClaimName"),
					fmt.Sprintf("must not be set when %s is true, use %s instead", fldPath.Child("scale").Child("enabled"), idxPath.Child("resourceClaimTemplateName")),
				))
			}
			// Ensure resourceClaimName values are unique within draResources
			if _, exists := seen[*dra.ResourceClaimName]; exists {
				errList = append(errList, field.Duplicate(idxPath.Child("resourceClaimName"), *dra.ResourceClaimName))
			} else {
				seen[*dra.ResourceClaimName] = struct{}{}
			}
		}

		if hasSpec {
			errList = append(errList, validateDRAClaimSpec(&dra, idxPath.Child("claimSpec"))...)
		}
	}
	return errList
}

func validateDRAClaimSpec(dra *appsv1alpha1.DRAResource, fldPath *field.Path) field.ErrorList {
	errList := field.ErrorList{}
	// Ensure claimSpec.devices is non-empty
	if len(dra.ClaimSpec.Devices) == 0 {
		errList = append(errList, field.Required(fldPath.Child("devices"), "must be non-empty"))
	}

	// Validate attributes.
	for j, device := range dra.ClaimSpec.Devices {
		devicePath := fldPath.Child("devices").Index(j)
		errList = append(errList, validateDRADeviceSpec(&device, devicePath)...)
	}
	return errList
}

func validateDRADeviceSpec(device *appsv1alpha1.DRADeviceSpec, fldPath *field.Path) field.ErrorList {
	errList := field.ErrorList{}
	if device.Name == "" {
		errList = append(errList, field.Required(fldPath.Child("name"), "is required"))
	}
	if device.Count <= 0 {
		errList = append(errList, field.Invalid(fldPath.Child("count"), int(device.Count), "must be > 0"))
	}
	if device.DeviceClassName == "" {
		errList = append(errList, field.Required(fldPath.Child("deviceClassName"), "is required"))
	}
	if device.DriverName == "" {
		errList = append(errList, field.Required(fldPath.Child("driverName"), "is required"))
	}
	seen := make(map[string]struct{})
	for idx, selector := range device.AttributeSelectors {
		qualifiedName := k8sutil.NormalizeQualifiedName(selector.Key, device.DriverName)
		if _, exists := seen[qualifiedName]; exists {
			errList = append(errList, field.Duplicate(fldPath.Child("attributeSelectors").Index(idx), qualifiedName))
		} else {
			seen[qualifiedName] = struct{}{}
		}
		errList = append(errList, validateDRADeviceAttributeSelector(&selector, fldPath.Child("attributeSelectors").Index(idx))...)
	}

	for idx, selector := range device.CapacitySelectors {
		qualifiedName := k8sutil.NormalizeQualifiedName(selector.Key, device.DriverName)
		if _, exists := seen[qualifiedName]; exists {
			errList = append(errList, field.Duplicate(fldPath.Child("capacitySelectors").Index(idx), qualifiedName))
		} else {
			seen[qualifiedName] = struct{}{}
		}
		errList = append(errList, validateDRAResourceQuantitySelector(&selector, fldPath.Child("capacitySelectors").Index(idx))...)
	}

	return errList
}

func validateDRADeviceAttributeSelector(selector *appsv1alpha1.DRADeviceAttributeSelector, fldPath *field.Path) field.ErrorList {
	errList := field.ErrorList{}
	errList = append(errList, validateDRADeviceAttributeSelectorOp(selector.Op, fldPath.Child("op"))...)
	errList = append(errList, validateDRADeviceAttributeSelectorValue(selector.Value, fldPath.Child("value"))...)
	return errList
}

func validateDRADeviceAttributeSelectorOp(op appsv1alpha1.DRADeviceAttributeSelectorOp, fldPath *field.Path) field.ErrorList {
	errList := field.ErrorList{}
	if op == "" {
		errList = append(errList, field.Required(fldPath, "is required"))
		return errList
	}

	if !slices.Contains(validDRADeviceAttributeSelectorOps, op) {
		errList = append(errList, field.Invalid(fldPath, op, fmt.Sprintf("must be one of %v", validDRADeviceAttributeSelectorOps)))
	}
	return errList
}

func validateDRADeviceAttributeSelectorValue(attribute *appsv1alpha1.DRADeviceAttributeSelectorValue, fldPath *field.Path) field.ErrorList {
	errList := field.ErrorList{}
	var fieldCount int
	if attribute.BoolValue != nil {
		fieldCount++
	}
	if attribute.IntValue != nil {
		fieldCount++
	}
	if attribute.StringValue != nil {
		fieldCount++
	}
	if attribute.VersionValue != nil {
		fieldCount++
	}
	if fieldCount == 0 {
		errList = append(errList, field.Required(fldPath, fmt.Sprintf("must specify exactly one of %s, %s, %s, or %s", fldPath.Child("boolValue"), fldPath.Child("intValue"), fldPath.Child("stringValue"), fldPath.Child("versionValue"))))
	} else if fieldCount > 1 {
		errList = append(errList, field.Invalid(fldPath, "multiple attribute values defined", fmt.Sprintf("must specify exactly one of %s, %s, %s, or %s", fldPath.Child("boolValue"), fldPath.Child("intValue"), fldPath.Child("stringValue"), fldPath.Child("versionValue"))))
	}

	if attribute.VersionValue != nil {
		_, err := utilversion.ParseSemantic(*attribute.VersionValue)
		if err != nil {
			errList = append(errList, field.Invalid(fldPath, attribute.VersionValue, "must be a valid semantic version"))
		}
	}
	return errList
}

func validateDRAResourceQuantitySelector(selector *appsv1alpha1.DRAResourceQuantitySelector, fldPath *field.Path) field.ErrorList {
	errList := field.ErrorList{}
	errList = append(errList, validateDRAResourceQuantitySelectorOp(selector.Op, fldPath.Child("op"))...)
	errList = append(errList, validateDRAResourceQuantitySelectorValue(selector.Value, fldPath.Child("value"))...)
	return errList
}

func validateDRAResourceQuantitySelectorOp(op appsv1alpha1.DRAResourceQuantitySelectorOp, fldPath *field.Path) field.ErrorList {
	errList := field.ErrorList{}
	if !slices.Contains(validDRAResourceQuantitySelectorOps, op) {
		errList = append(errList, field.Invalid(fldPath, op, fmt.Sprintf("must be one of %v", validDRAResourceQuantitySelectorOps)))
	}
	return errList
}

func validateDRAResourceQuantitySelectorValue(value *apiresource.Quantity, fldPath *field.Path) field.ErrorList {

	errList := field.ErrorList{}
	if value == nil {
		errList = append(errList, field.Required(fldPath, "is required"))
	}
	return errList
}

func validateAuthSecret(authSecret *string, fldPath *field.Path) field.ErrorList {
	errList := field.ErrorList{}
	if *authSecret == "" {
		errList = append(errList, field.Required(fldPath, "is required"))
	}
	return errList
}

// If Spec.Expose.Ingress.Enabled is true, Spec.Expose.Ingress.Spec must be non-nil.
func validateExposeConfiguration(expose *appsv1alpha1.Expose, fldPath *field.Path) field.ErrorList {
	errList := field.ErrorList{}
	if expose.Ingress.Enabled != nil && *expose.Ingress.Enabled {
		if reflect.DeepEqual(expose.Ingress.Spec, networkingv1.IngressSpec{}) {
			errList = append(errList, field.Required(fldPath.Child("spec"), fmt.Sprintf("must be defined if %s is true", fldPath.Child("enabled"))))
		}
	}

	if expose.HTTPRoute.Enabled != nil && *expose.HTTPRoute.Enabled {
		if reflect.DeepEqual(expose.HTTPRoute.Spec, appsv1alpha1.HTTPRouteSpec{}) {
			errList = append(errList, field.Required(fldPath.Child("spec"), fmt.Sprintf("must be defined if %s is true", fldPath.Child("enabled"))))
		}
	}
	return errList
}

// If Spec.Metrics.Enabled is true, Spec.Metrics.ServiceMonitor must not be empty.
func validateMetricsConfiguration(metrics *appsv1alpha1.Metrics, fldPath *field.Path) field.ErrorList {
	errList := field.ErrorList{}
	if metrics.Enabled != nil && *metrics.Enabled {
		if reflect.DeepEqual(metrics.ServiceMonitor, appsv1alpha1.ServiceMonitor{}) {
			errList = append(errList, field.Required(fldPath.Child("serviceMonitor"), fmt.Sprintf("must be defined if %s is true", fldPath.Child("enabled"))))
		}
	}
	return errList
}

// If Spec.Scale.Enabled is true, Spec.Scale.HPA must be non-empty HorizontalPodAutoScaler.
func validateScaleConfiguration(scale *appsv1alpha1.Autoscaling, fldPath *field.Path) field.ErrorList {
	errList := field.ErrorList{}
	if scale.Enabled != nil && *scale.Enabled {
		if reflect.DeepEqual(scale.HPA, appsv1alpha1.HorizontalPodAutoscalerSpec{}) {
			errList = append(errList, field.Required(fldPath.Child("hpa"), fmt.Sprintf("must be defined if %s is true", fldPath.Child("enabled"))))
		}
	}
	return errList
}

// Spec.Resources.Claims must be empty.
func validateResourcesConfiguration(resources *corev1.ResourceRequirements, fldPath *field.Path) field.ErrorList {
	errList := field.ErrorList{}
	if resources != nil {
		if resources.Claims != nil || len(resources.Claims) != 0 {
			errList = append(errList, field.Forbidden(fldPath.Child("claims"), "must be empty"))
		}
	}
	return errList
}

// validateKServeonfiguration implements required KServe validations.
func validateKServeConfiguration(spec *appsv1alpha1.NIMServiceSpec, fldPath *field.Path) field.ErrorList {
	errList := field.ErrorList{}

	platformIsKServe := spec.InferencePlatform == appsv1alpha1.PlatformTypeKServe

	// mode is the value, and annotated is true if the key-value pair exist.
	mode, annotated := spec.Annotations["serving.kserve.org/deploymentMode"]
	// If the annotation is absent, kserve defaults to serverless.
	serverless := !annotated || strings.EqualFold(mode, "serverless")

	// When Spec.InferencePlatform is "kserve" and used in "serverless" mode:
	if platformIsKServe && serverless {
		// Spec.Scale (autoscaling) cannot be set.
		if spec.Scale.Enabled != nil && *spec.Scale.Enabled {
			errList = append(errList, field.Forbidden(fldPath.Child("scale").Child("enabled"), fmt.Sprintf("%s (autoscaling) cannot be set when KServe runs in serverless mode", fldPath.Child("scale"))))
		}

		// Spec.Expose.Ingress cannot be set.
		if spec.Expose.Ingress.Enabled != nil && *spec.Expose.Ingress.Enabled {
			errList = append(errList, field.Forbidden(fldPath.Child("expose").Child("ingress").Child("enabled"), fmt.Sprintf("%s cannot be set when KServe runs in serverless mode", fldPath.Child("expose").Child("ingress"))))
		}

		// Spec.Metrics.ServiceMonitor cannot be set.
		if spec.Metrics.Enabled != nil && *spec.Metrics.Enabled {
			errList = append(errList, field.Forbidden(fldPath.Child("metrics").Child("enabled"), fmt.Sprintf("%s cannot be set when KServe runs in serverless mode", fldPath.Child("metrics").Child("serviceMonitor"))))
		}
	}

	// Spec.MultiNode cannot be enabled when inferencePlatform is kserve.
	if platformIsKServe && spec.MultiNode != nil {
		errList = append(errList, field.Forbidden(fldPath.Child("multiNode"), "cannot be set when KServe runs in serverless mode"))
	}

	return errList
}

// validateMultiNodeImmutability ensures that the MultiNode field remains unchanged after creation.
func validateMultiNodeImmutability(oldNs, newNs *appsv1alpha1.NIMService, fldPath *field.Path) field.ErrorList {
	errList := field.ErrorList{}
	if !equality.Semantic.DeepEqual(oldNs.Spec.MultiNode, newNs.Spec.MultiNode) {
		errList = append(errList, field.Forbidden(fldPath, "is immutable once the resource is created"))
	}
	return errList
}

// validatePVCImmutability verifies that once a PVC is created with create: true, no further modifications are allowed.
func validatePVCImmutability(oldNs, newNs *appsv1alpha1.NIMService, fldPath *field.Path) field.ErrorList {
	errList := field.ErrorList{}
	oldPVC := oldNs.Spec.Storage.PVC
	newPVC := newNs.Spec.Storage.PVC
	if oldPVC.Create != nil && *oldPVC.Create {
		if !equality.Semantic.DeepEqual(oldPVC, newPVC) {
			errList = append(errList, field.Forbidden(fldPath, fmt.Sprintf("is immutable once it is created with %s = true", fldPath.Child("create"))))
		}
	}
	return errList
}

// validateDRAResourceImmutability ensures that the DRA resources remain unchanged after creation.
func validateDRAResourceImmutability(oldNs, newNs *appsv1alpha1.NIMService, fldPath *field.Path) field.ErrorList {
	errList := field.ErrorList{}
	oldDRAResources := oldNs.Spec.DRAResources
	newDRAResources := newNs.Spec.DRAResources
	if len(oldDRAResources) > 0 {
		if !equality.Semantic.DeepEqual(oldDRAResources, newDRAResources) {
			errList = append(errList, field.Forbidden(fldPath, "is immutable"))
		}
	}
	return errList
}
