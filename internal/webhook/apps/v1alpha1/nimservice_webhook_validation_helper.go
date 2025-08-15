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

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

var validPVCAccessModeStrs = []corev1.PersistentVolumeAccessMode{
	corev1.ReadWriteOnce,
	corev1.ReadOnlyMany,
	corev1.ReadWriteMany,
	corev1.ReadWriteOncePod,
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

		// Exactly one of resourceClaimName or resourceClaimTemplateName must be provided
		if hasName == hasTemplate { // both true or both false
			errList = append(errList, field.Invalid(
				idxPath,
				dra,
				fmt.Sprintf("must specify exactly one of %s or %s", fldPath.Child("resourceClaimName"), fldPath.Child("resourceClaimTemplateName"))))
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

// If Spec.Expose.Ingress.Enabled is true, Spec.Expose.Ingress.Spec must be non-nil.
func validateExposeConfiguration(expose *appsv1alpha1.Expose, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warningList := admission.Warnings{}
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
func validateScaleConfiguration(scale *appsv1alpha1.Autoscaling, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warningList := admission.Warnings{}
	errList := field.ErrorList{}
	if scale.Enabled != nil && *scale.Enabled {
		if reflect.DeepEqual(scale.HPA, appsv1alpha1.HorizontalPodAutoscalerSpec{}) {
			errList = append(errList, field.Required(fldPath.Child("hpa"), fmt.Sprintf("must be defined if %s is true", fldPath.Child("enabled"))))
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

// validateNIMServiceSpec aggregates all structural validation checks for a NIMService
// object. It is intended to be invoked by both ValidateCreate and ValidateUpdate to
// ensure the resource is well-formed before any other validation (e.g. immutability)
// is performed.
func validateNIMServiceSpec(spec *appsv1alpha1.NIMServiceSpec, fldPath *field.Path, kubeVersion string) (admission.Warnings, field.ErrorList) {
	errList := field.ErrorList{}
	warningList := admission.Warnings{}

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

	w, err = validateExposeConfiguration(&spec.Expose, fldPath.Child("expose").Child("ingress"))
	warningList = append(warningList, w...)
	errList = append(errList, err...)

	w, err = validateMetricsConfiguration(&spec.Metrics, fldPath.Child("metrics"))
	warningList = append(warningList, w...)
	errList = append(errList, err...)

	w, err = validateScaleConfiguration(&spec.Scale, fldPath.Child("scale"))
	warningList = append(warningList, w...)
	errList = append(errList, err...)

	w, err = validateResourcesConfiguration(spec.Resources, fldPath.Child("resources"))
	warningList = append(warningList, w...)
	errList = append(errList, err...)

	w, err = validateDRAResourcesConfiguration(spec, fldPath, kubeVersion)
	warningList = append(warningList, w...)
	errList = append(errList, err...)

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
