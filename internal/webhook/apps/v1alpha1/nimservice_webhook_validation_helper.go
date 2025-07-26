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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/validation/field"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

var validPVCAccessModeStrs = []corev1.PersistentVolumeAccessMode{
	corev1.ReadWriteOnce,
	corev1.ReadOnlyMany,
	corev1.ReadWriteMany,
	corev1.ReadWriteOncePod,
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

		// If PVC.Create is False, PVC.Name cannot be empty
		if storage.PVC.Create != nil && !*storage.PVC.Create && storage.PVC.Name == "" {
			errList = append(errList, field.Required(fldPath.Child("pvc").Child("name"), fmt.Sprintf("must be provided when %s is false", fldPath.Child("pvc").Child("create"))))
		}

		if storage.PVC.VolumeAccessMode != "" {
			found := false
			for _, vm := range validPVCAccessModeStrs {
				if storage.PVC.VolumeAccessMode == vm {
					found = true
					break
				}
			}

			if !found {
				errList = append(errList, field.Invalid(fldPath.Child("pvc").Child("volumeAccessMode"), storage.PVC.VolumeAccessMode, "unrecognized volumeAccessMode"))
			}
		}
	}

	return errList
}

func validateDRAResourcesConfiguration(spec *appsv1alpha1.NIMServiceSpec, fldPath *field.Path) field.ErrorList {
	// Rules for only DRAResource.ResourceClaimTemplateName being defined
	errList := field.ErrorList{}
	draResourcesPath := fldPath.Child("draResources")
	if spec.Replicas > 1 || (spec.Scale.Enabled != nil && *spec.Scale.Enabled) {
		// Only DRAResource.ResourceClaimTemplateName must be defined (not ResourceClaimName)
		for i, dra := range spec.DRAResources {
			if dra.ResourceClaimTemplateName == nil || *dra.ResourceClaimTemplateName == "" {
				errList = append(errList, field.Required(
					draResourcesPath.Index(i).Child("resourceClaimTemplateName"),
					fmt.Sprintf("must be defined when %s > 1 or %s is true", fldPath.Child("replicas"), fldPath.Child("scale").Child("enabled")),
				))
			}
			if dra.ResourceClaimName != nil && *dra.ResourceClaimName != "" {
				errList = append(errList, field.Forbidden(
					draResourcesPath.Index(i).Child("resourceClaimName"),
					fmt.Sprintf("must not be set when %s > 1 or %s, only %s is allowed", fldPath.Child("replicas"), fldPath.Child("scale").Child("enabled"), draResourcesPath.Index(i).Child("draResourceClaimTemplateName"))))
			}
		}
	} else {

		// If DRAResources is not empty, all DRAResources objects must have a unique DRAResource.ResourceClaimName
		draResources := spec.DRAResources
		if len(draResources) > 0 {
			seen := make(map[string]struct{})
			for i, dra := range draResources {
				if dra.ResourceClaimName == nil {
					errList = append(errList, field.Required(draResourcesPath.Index(i).Child("resourceClaimName"), "must not be empty"))
					continue
				}
				if _, exists := seen[*dra.ResourceClaimName]; exists {
					errList = append(errList, field.Duplicate(draResourcesPath.Index(i).Child("resourceClaimName"), *dra.ResourceClaimName))
				}
				seen[*dra.ResourceClaimName] = struct{}{}
			}
		}
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
		spec := expose.Ingress.Spec
		if spec.IngressClassName == nil && spec.DefaultBackend == nil &&
			len(spec.TLS) == 0 && len(spec.Rules) == 0 {
			errList = append(errList, field.Required(fldPath.Child("spec"), fmt.Sprintf("must be defined if %s is true", fldPath.Child("enabled"))))
		}
	}
	return errList
}

// If Spec.Metrics.Enabled is true, Spec.Metrics.ServiceMonitor must not be empty.
func validateMetricsConfiguration(metrics *appsv1alpha1.Metrics, fldPath *field.Path) field.ErrorList {
	errList := field.ErrorList{}
	if metrics.Enabled != nil && *metrics.Enabled {
		sm := metrics.ServiceMonitor
		if len(sm.AdditionalLabels) == 0 && len(sm.Annotations) == 0 &&
			sm.Interval == "" && sm.ScrapeTimeout == "" {
			errList = append(errList, field.Required(fldPath.Child("serviceMonitor"), fmt.Sprintf("must be defined if %s is true", fldPath.Child("enabled"))))
		}
	}
	return errList
}

// If Spec.Scale.Enabled is true, Spec.Scale.HPA must be non-empty HorizontalPodAutoScaler.
func validateScaleConfiguration(scale *appsv1alpha1.Autoscaling, fldPath *field.Path) field.ErrorList {
	errList := field.ErrorList{}
	if scale.Enabled != nil && *scale.Enabled {
		hpa := scale.HPA
		if hpa.MinReplicas == nil && hpa.MaxReplicas == 0 && len(hpa.Metrics) == 0 && hpa.Behavior == nil {
			errList = append(errList, field.Required(fldPath.Child("hpa"), fmt.Sprintf("must be defined if %s is true", fldPath.Child("enabled"))))
		}
	}
	return errList
}

// Spec.DRAResources must be empty unless K8s version is v1.33.1+.
func validateDRAResourcesVersionCompatibility(kubeVersion string, fldPath *field.Path) field.ErrorList {

	errList := field.ErrorList{}

	if !utils.IsVersionGreaterThanOrEqual(kubeVersion, utils.MinSupportedClusterVersionForDRA) {
		errList = append(errList, field.Forbidden(fldPath, fmt.Sprintf("is not supported by NIM-Operator on this cluster, please upgrade to k8s version '%s' or higher", utils.MinSupportedClusterVersionForDRA)))
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
