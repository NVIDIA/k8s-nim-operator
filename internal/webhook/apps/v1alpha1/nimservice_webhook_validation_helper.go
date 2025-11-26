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
	"k8s.io/apimachinery/pkg/api/equality"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/blang/semver/v4"
	kserveconstants "github.com/kserve/kserve/pkg/constants"

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
	appsv1alpha1.DRAResourceQuantitySelectorOpNotEqual,
	appsv1alpha1.DRAResourceQuantitySelectorOpGreaterThan,
	appsv1alpha1.DRAResourceQuantitySelectorOpGreaterThanOrEqual,
	appsv1alpha1.DRAResourceQuantitySelectorOpLessThan,
	appsv1alpha1.DRAResourceQuantitySelectorOpLessThanOrEqual,
}

// validateNIMServiceSpec aggregates all structural validation checks for a NIMService
// object. It is intended to be invoked by both ValidateCreate and ValidateUpdate to
// ensure the resource is well-formed before any other validation (e.g. immutability)
// is performed.
func validateNIMServiceSpec(spec *appsv1alpha1.NIMServiceSpec, fldPath *field.Path, kubeVersion string) (admission.Warnings, field.ErrorList) {
	warningList, errList := validateImageConfiguration(&spec.Image, fldPath.Child("image"))

	// TODO abstract all validation functions with a single signature validateFunc(*appsv1alpha1.NIMServiceSpec, *field.Path) (admission.Warnings, field.ErrorList)
	w, err := validateImageConfiguration(&spec.Image, fldPath.Child("image"))
	warningList = append(warningList, w...)
	errList = append(errList, err...)

	w, err = validateAuthSecret(&spec.AuthSecret, fldPath.Child("authSecret"))
	warningList = append(warningList, w...)
	errList = append(errList, err...)

	w, err = validateServiceStorageConfiguration(&spec.Storage, fldPath.Child("storage"))
	warningList = append(warningList, w...)
	errList = append(errList, err...)

	w, err = validateRouterConfiguration(&spec.Expose.Router, fldPath.Child("expose").Child("router"))
	warningList = append(warningList, w...)
	errList = append(errList, err...)

	w, err = validateMetricsConfiguration(&spec.Metrics, fldPath.Child("metrics"))
	warningList = append(warningList, w...)
	errList = append(errList, err...)

	w, err = validateScaleConfiguration(&spec.Scale, spec.Replicas, fldPath.Child("scale"))
	warningList = append(warningList, w...)
	errList = append(errList, err...)

	w, err = validateResourcesConfiguration(spec.Resources, fldPath.Child("resources"))
	warningList = append(warningList, w...)
	errList = append(errList, err...)

	w, err = validateDRAResourcesConfiguration(spec, fldPath, kubeVersion)
	warningList = append(warningList, w...)
	errList = append(errList, err...)

	w, err = validateKServeConfiguration(spec, fldPath)
	warningList = append(warningList, w...)
	errList = append(errList, err...)

	w, err = validateExposeConfiguration(&spec.Expose, fldPath.Child("expose"))
	warningList = append(warningList, w...)
	errList = append(errList, err...)

	w, err = validateRedundantIngressConfiguration(spec, fldPath)
	warningList = append(warningList, w...)
	errList = append(errList, err...)

	w, err = validateExposeRouterConfiguration(spec, fldPath)
	warningList = append(warningList, w...)
	errList = append(errList, err...)

	return warningList, errList
}

func validateExposeRouterConfiguration(spec *appsv1alpha1.NIMServiceSpec, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warningList := admission.Warnings{}
	errList := field.ErrorList{}
	if spec.Expose.Service.Type == corev1.ServiceTypeLoadBalancer && (spec.Expose.Router.Gateway != nil || spec.Expose.Router.Ingress != nil) {
		warningList = append(warningList, "spec.expose.service.type is set to LoadBalancer, but spec.expose.router.gateway or spec.expose.router.ingress is also set. This creates two entry points for the service. Consider only using one of them.")
	}

	if spec.Expose.Router.Gateway != nil && spec.Expose.Router.Gateway.GRPCRoutesEnabled && spec.Expose.Service.GRPCPort == nil {
		errList = append(errList, field.Required(fldPath.Child("expose").Child("service").Child("grpcPort"), "must be set when .spec.expose.router.gateway.grpcRoutesEnabled is true"))
	}
	return warningList, errList
}

func validateRedundantIngressConfiguration(spec *appsv1alpha1.NIMServiceSpec, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warningList := admission.Warnings{}
	errList := field.ErrorList{}
	if spec.Expose.Ingress.Enabled != nil && *spec.Expose.Ingress.Enabled && spec.Expose.Router.Ingress != nil { //nolint:staticcheck
		errList = append(errList, field.Forbidden(fldPath.Child("expose").Child("ingress"), fmt.Sprintf("%s is deprecated. Omit the field if you use .spec.expose.router instead", fldPath.Child("expose").Child("ingress"))))
	}
	return warningList, errList
}

func validateExposeConfiguration(expose *appsv1alpha1.Expose, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warningList := admission.Warnings{}
	errList := field.ErrorList{}
	if expose.Ingress.Enabled != nil && *expose.Ingress.Enabled { //nolint:staticcheck
		warningList = append(warningList, ".spec.expose.ingress is deprecated, use .spec.expose.router instead.")
	}
	return warningList, errList
}

func validateImageConfiguration(image *appsv1alpha1.Image, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warningList := admission.Warnings{}
	errList := field.ErrorList{}
	if image.Repository == "" {
		errList = append(errList, field.Required(fldPath.Child("repository"), "is required"))
	}
	if image.Tag == "" {
		errList = append(errList, field.Required(fldPath.Child("tag"), "is required"))
	}
	return warningList, errList
}

func validateServiceStorageConfiguration(storage *appsv1alpha1.NIMServiceStorage, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warningList := admission.Warnings{}
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

	// Check if EmptyDir is defined (non-empty)
	emptyDirDefined := storage.EmptyDir != nil

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
	if emptyDirDefined {
		definedCount++
	}
	if hostPathDefined {
		definedCount++
	}

	// Ensure only one of nimCache, PVC, or EmptyDir is defined
	if definedCount == 0 {
		errList = append(errList, field.Required(fldPath, fmt.Sprintf("one of %s, %s, %s or %s, must be defined", fldPath.Child("nimCache"), fldPath.Child("pvc"), fldPath.Child("emptyDir"), fldPath.Child("hostPath"))))
	} else if definedCount > 1 {
		errList = append(errList, field.Invalid(fldPath, "multiple storage sources defined", fmt.Sprintf("only one of %s, %s, %s or %s must be defined", fldPath.Child("nimCache"), fldPath.Child("pvc"), fldPath.Child("emptyDir"), fldPath.Child("hostPath"))))
	}

	// If NIMCache is non-nil, NIMCache.Name must not be empty
	if storage.NIMCache.Profile != "" {
		if storage.NIMCache.Name == "" {
			errList = append(errList, field.Required(fldPath.Child("nimCache").Child("name"), fmt.Sprintf("is required when %s is defined", fldPath.Child("nimCache"))))
		}
	}

	// Enforcing PVC rules if defined
	if pvcDefined {
		wList, eList := validatePVCConfiguration(&storage.PVC, fldPath.Child("pvc"))
		warningList = append(warningList, wList...)
		errList = append(errList, eList...)
	}

	return warningList, errList
}

func validateDRAResourcesConfiguration(spec *appsv1alpha1.NIMServiceSpec, fldPath *field.Path, k8sVersion string) (admission.Warnings, field.ErrorList) {
	warningList := admission.Warnings{}
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
		hasSpec := dra.ClaimCreationSpec != nil

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

		// Exactly one of resourceClaimName, resourceClaimTemplateName, or claimCreationSpec must be provided
		if fieldCount == 0 {
			errList = append(errList, field.Required(idxPath, fmt.Sprintf("one of %s, %s, or %s must be provided", fldPath.Child("resourceClaimName"), fldPath.Child("resourceClaimTemplateName"), fldPath.Child("claimCreationSpec"))))
		} else if fieldCount > 1 {
			errList = append(errList, field.Invalid(
				idxPath,
				"multiple dra resource sources defined",
				fmt.Sprintf("must specify exactly one of %s, %s, or %s", fldPath.Child("resourceClaimName"), fldPath.Child("resourceClaimTemplateName"), fldPath.Child("claimCreationSpec"))))
		}

		if hasName {
			// resourceClaimName: spec.relicas must be <= 1 and spec.scale.enabled must be false.
			if spec.Replicas != nil && *spec.Replicas > 1 {
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
			wList, eList := validateDRAClaimCreationSpec(&dra, idxPath.Child("claimCreationSpec"))
			warningList = append(warningList, wList...)
			errList = append(errList, eList...)
		}
	}
	return warningList, errList
}

func validateDRAClaimCreationSpec(dra *appsv1alpha1.DRAResource, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warningList := admission.Warnings{}
	errList := field.ErrorList{}
	// Ensure claimCreationSpec.devices is non-empty
	if len(dra.ClaimCreationSpec.Devices) == 0 {
		errList = append(errList, field.Required(fldPath.Child("devices"), "must be non-empty"))
	}

	// Validate attributes.
	for j, device := range dra.ClaimCreationSpec.Devices {
		devicePath := fldPath.Child("devices").Index(j)
		wList, eList := validateDRADeviceSpec(&device, devicePath)
		warningList = append(warningList, wList...)
		errList = append(errList, eList...)
	}
	return warningList, errList
}

func validateDRADeviceSpec(device *appsv1alpha1.DRADeviceSpec, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warningList := admission.Warnings{}
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
		wList, eList := validateDRADeviceAttributeSelector(&selector, fldPath.Child("attributeSelectors").Index(idx))
		warningList = append(warningList, wList...)
		errList = append(errList, eList...)
	}

	for idx, selector := range device.CapacitySelectors {
		qualifiedName := k8sutil.NormalizeQualifiedName(selector.Key, device.DriverName)
		if _, exists := seen[qualifiedName]; exists {
			errList = append(errList, field.Duplicate(fldPath.Child("capacitySelectors").Index(idx), qualifiedName))
		} else {
			seen[qualifiedName] = struct{}{}
		}
		wList, eList := validateDRAResourceQuantitySelector(&selector, fldPath.Child("capacitySelectors").Index(idx))
		warningList = append(warningList, wList...)
		errList = append(errList, eList...)
	}

	return warningList, errList
}

func validateDRADeviceAttributeSelector(selector *appsv1alpha1.DRADeviceAttributeSelector, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warningList := admission.Warnings{}
	errList := field.ErrorList{}
	errList = append(errList, validateDRADeviceAttributeSelectorOp(selector.Op, fldPath.Child("op"))...)
	errList = append(errList, validateDRADeviceAttributeSelectorValue(selector.Value, fldPath.Child("value"))...)
	return warningList, errList
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
		_, err := semver.Parse(*attribute.VersionValue)
		if err != nil {
			errList = append(errList, field.Invalid(fldPath, attribute.VersionValue, fmt.Sprintf("must be a valid semantic version: %v", err)))
		}
	}
	return errList
}

func validateDRAResourceQuantitySelector(selector *appsv1alpha1.DRAResourceQuantitySelector, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warningList, errList := validateDRAResourceQuantitySelectorOp(selector.Op, fldPath.Child("op"))
	wList, eList := validateDRAResourceQuantitySelectorValue(selector.Value, fldPath.Child("value"))
	warningList = append(warningList, wList...)
	errList = append(errList, eList...)
	return warningList, errList
}

func validateDRAResourceQuantitySelectorOp(op appsv1alpha1.DRAResourceQuantitySelectorOp, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warningList := admission.Warnings{}
	errList := field.ErrorList{}
	if !slices.Contains(validDRAResourceQuantitySelectorOps, op) {
		errList = append(errList, field.Invalid(fldPath, op, fmt.Sprintf("must be one of %v", validDRAResourceQuantitySelectorOps)))
	}
	return warningList, errList
}

func validateDRAResourceQuantitySelectorValue(value *apiresource.Quantity, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warningList := admission.Warnings{}
	errList := field.ErrorList{}
	if value == nil {
		errList = append(errList, field.Required(fldPath, "is required"))
	}
	return warningList, errList
}

func validateAuthSecret(authSecret *string, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warningList := admission.Warnings{}
	errList := field.ErrorList{}
	if *authSecret == "" {
		errList = append(errList, field.Required(fldPath, "is required"))
	}
	return warningList, errList
}

func validateRouterConfiguration(router *appsv1alpha1.Router, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warningList := admission.Warnings{}
	errList := field.ErrorList{}
	if (router.Ingress != nil || router.Gateway != nil) && router.HostDomainName == "" {
		errList = append(errList, field.Required(fldPath.Child("hostDomainName"), "is required"))
	}
	if router.Ingress != nil && router.Gateway != nil {
		errList = append(errList, field.Forbidden(fldPath, "ingress and gateway cannot be specified together"))
	}
	return warningList, errList
}

// If Spec.Metrics.Enabled is true, Spec.Metrics.ServiceMonitor must not be empty.
func validateMetricsConfiguration(metrics *appsv1alpha1.Metrics, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warningList := admission.Warnings{}
	errList := field.ErrorList{}
	if metrics.Enabled != nil && *metrics.Enabled {
		if reflect.DeepEqual(metrics.ServiceMonitor, appsv1alpha1.ServiceMonitor{}) {
			errList = append(errList, field.Required(fldPath.Child("serviceMonitor"), fmt.Sprintf("must be defined if %s is true", fldPath.Child("enabled"))))
		}
	}
	return warningList, errList
}

// If Spec.Scale.Enabled is true, Spec.Scale.HPA must be non-empty HorizontalPodAutoScaler.
func validateScaleConfiguration(scale *appsv1alpha1.Autoscaling, replicas *int32, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warningList := admission.Warnings{}
	errList := field.ErrorList{}
	if scale.Enabled != nil && *scale.Enabled {
		if reflect.DeepEqual(scale.HPA, appsv1alpha1.HorizontalPodAutoscalerSpec{}) {
			errList = append(errList, field.Required(fldPath.Child("hpa"), fmt.Sprintf("must be defined if %s is true", fldPath.Child("enabled"))))
		}

		if replicas != nil {
			errList = append(errList,
				field.Forbidden(
					field.NewPath("spec", "replicas"),
					"cannot be set when autoscaling is enabled; configure min/max replicas in spec.scale.hpa instead",
				),
			)
		}
	}
	return warningList, errList
}

// Spec.Resources.Claims must be empty.
func validateResourcesConfiguration(resources *corev1.ResourceRequirements, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warningList := admission.Warnings{}
	errList := field.ErrorList{}
	if resources != nil {
		if resources.Claims != nil || len(resources.Claims) != 0 {
			errList = append(errList, field.Forbidden(fldPath.Child("claims"), "must be empty"))
		}
	}
	return warningList, errList
}

// validateKServeonfiguration implements required KServe validations.
func validateKServeConfiguration(spec *appsv1alpha1.NIMServiceSpec, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	errList := field.ErrorList{}
	warningList := admission.Warnings{}

	platformIsKServe := spec.InferencePlatform == appsv1alpha1.PlatformTypeKServe

	// mode is the value, and annotated is true if the key-value pair exist.
	mode, annotated := spec.Annotations[utils.KServeDeploymentModeAnnotationKey]
	// If the annotation is absent, kserve defaults to Knative.
	knative := !annotated || strings.EqualFold(mode, string(kserveconstants.Knative)) || strings.EqualFold(mode, string(kserveconstants.LegacyServerless))

	// When Spec.InferencePlatform is "kserve" and used in "Knative" mode:
	if platformIsKServe && knative {
		// Spec.Scale (autoscaling) cannot be set.
		if spec.Scale.Enabled != nil && *spec.Scale.Enabled {
			errList = append(errList, field.Forbidden(fldPath.Child("scale").Child("enabled"), fmt.Sprintf("%s (autoscaling) cannot be set when KServe runs in knative mode", fldPath.Child("scale"))))
		}

		// TODO deprecate this once we have removed the .spec.expose.ingress field from the spec
		if spec.Expose.Ingress.Enabled != nil && *spec.Expose.Ingress.Enabled { //nolint:staticcheck
			errList = append(errList, field.Forbidden(fldPath.Child("expose").Child("ingress").Child("enabled"), fmt.Sprintf("%s cannot be set when KServe runs in knative mode", fldPath.Child("expose").Child("ingress").Child("enabled"))))
		}

		// Spec.Expose.Router.Ingress cannot be set.
		if spec.Expose.Router.Ingress != nil {
			errList = append(errList, field.Forbidden(fldPath.Child("router").Child("ingress"), fmt.Sprintf("%s cannot be set when KServe runs in knative mode", fldPath.Child("router").Child("ingress"))))
		}

		// Spec.Expose.Router.Gateway cannot be set.
		if spec.Expose.Router.Gateway != nil {
			errList = append(errList, field.Forbidden(fldPath.Child("router").Child("gateway"), fmt.Sprintf("%s cannot be set when KServe runs in knative mode", fldPath.Child("router").Child("gateway"))))
		}

		// Spec.Metrics.ServiceMonitor cannot be set.
		if spec.Metrics.Enabled != nil && *spec.Metrics.Enabled {
			errList = append(errList, field.Forbidden(fldPath.Child("metrics").Child("enabled"), fmt.Sprintf("%s cannot be set when KServe runs in knative mode", fldPath.Child("metrics").Child("serviceMonitor"))))
		}
	}

	// Spec.MultiNode cannot be enabled when inferencePlatform is kserve.
	if platformIsKServe && spec.MultiNode != nil {
		errList = append(errList, field.Forbidden(fldPath.Child("multiNode"), "cannot be set when KServe runs in knative mode"))
	}

	return warningList, errList
}

// validateMultiNodeImmutability ensures that the MultiNode field remains unchanged after creation.
func validateMultiNodeImmutability(oldNs, newNs *appsv1alpha1.NIMService, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warningList := admission.Warnings{}
	errList := field.ErrorList{}
	if !equality.Semantic.DeepEqual(oldNs.Spec.MultiNode, newNs.Spec.MultiNode) {
		errList = append(errList, field.Forbidden(fldPath, "is immutable once the resource is created"))
	}
	return warningList, errList
}

// validatePVCImmutability verifies that once a PVC is created with create: true, no further modifications are allowed.
func validatePVCImmutability(oldNs, newNs *appsv1alpha1.NIMService, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warningList := admission.Warnings{}
	errList := field.ErrorList{}
	oldPVC := oldNs.Spec.Storage.PVC
	newPVC := newNs.Spec.Storage.PVC
	if oldPVC.Create != nil && *oldPVC.Create {
		if !equality.Semantic.DeepEqual(oldPVC, newPVC) {
			errList = append(errList, field.Forbidden(fldPath, fmt.Sprintf("is immutable once it is created with %s = true", fldPath.Child("create"))))
		}
	}
	return warningList, errList
}

// validateDRAResourceImmutability ensures that the DRA resources remain unchanged after creation.
func validateDRAResourceImmutability(oldNs, newNs *appsv1alpha1.NIMService, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warningList := admission.Warnings{}
	errList := field.ErrorList{}
	oldDRAResources := oldNs.Spec.DRAResources
	newDRAResources := newNs.Spec.DRAResources
	if len(oldDRAResources) > 0 {
		if !equality.Semantic.DeepEqual(oldDRAResources, newDRAResources) {
			errList = append(errList, field.Forbidden(fldPath, "is immutable"))
		}
	}
	return warningList, errList
}
