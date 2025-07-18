package v1alpha1

import (
	"fmt"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

func validateImageConfiguration(image *appsv1alpha1.Image) error {
	if image.Repository == "" {
		return fmt.Errorf("NIMService.Image.Repository is required")
	}

	if image.Tag == "" {
		return fmt.Errorf("NIMService.Image.Tag is required")
	}

	return nil
}

func validateServiceStorageConfiguration(nimService *appsv1alpha1.NIMService) error {
	storage := nimService.Spec.Storage
	// If size limit is defined, it must be greater than 0
	if storage.SharedMemorySizeLimit != nil {
		if storage.SharedMemorySizeLimit.Sign() <= 0 { // Sign(): -1 (<0), 0 (==0), 1 (>0)
			return fmt.Errorf("NIMService.Spec.Storage.SharedMemorySizeLimit must be > 0")
		}
	}

	// If NIMCache is non-nil, NIMCache.Name must not be empty
	if storage.NIMCache.Profile != "" {
		if storage.NIMCache.Name == "" {
			return fmt.Errorf("NIMService.Spec.Storage.NIMCache.Name is required when NIMService.Spec.Storage.NIMCache is defined")
		}
	}

	// Enforcing PVC rules if defined
	if storage.PVC.Create != nil || storage.PVC.Name != "" || storage.PVC.StorageClass != "" ||
		storage.PVC.Size != "" || storage.PVC.VolumeAccessMode != "" || storage.PVC.SubPath != "" ||
		len(storage.PVC.Annotations) > 0 {
		// If PVC.Create is False, PVC.Name cannot be empty
		if storage.PVC.Create != nil && !*storage.PVC.Create && storage.PVC.Name == "" {
			return fmt.Errorf("NIMCache.Spec.Storage.PVC.Name must be defined when PVC.Create is false")
		}

		// If PVC.VolumeAccessMode is defined, it must be one of the valid modes
		if storage.PVC.VolumeAccessMode != "" {
			if storage.PVC.VolumeAccessMode != "ReadWriteOnce" && storage.PVC.VolumeAccessMode != "ReadOnlyMany" &&
				storage.PVC.VolumeAccessMode != "ReadWriteMany" && storage.PVC.VolumeAccessMode != "ReadWriteOncePod" {
				return fmt.Errorf("NIMCache.Spec.Storage.PVC.VolumeAccessMode must be 'ReadWriteOnce', 'ReadOnlyMany', or 'ReadWriteMany'")
			}
		}
	}

	return nil
}

func validateDRAResourcesConfiguration(nimService *appsv1alpha1.NIMService) error {
	// Rules for only DRAResource.ResourceClaimTemplateName being defined
	if nimService.Spec.Replicas > 1 || (nimService.Spec.Scale.Enabled != nil && *nimService.Spec.Scale.Enabled) {
		// Only DRAResource.ResourceClaimTemplateName must be defined (not ResourceClaimName)
		for i, dra := range nimService.Spec.DRAResources {
			if dra.ResourceClaimTemplateName == nil || *dra.ResourceClaimTemplateName == "" {
				return fmt.Errorf("NIMCache.Spec.DRAResources[%d]: DRAResource.ResourceClaimTemplateName must be defined when using multiple replicas or autoscaling", i)
			}
			if dra.ResourceClaimName != nil && *dra.ResourceClaimName != "" {
				return fmt.Errorf("DRAResources[%d]: ResourceClaimName must not be set when using multiple replicas or autoscaling; only ResourceClaimTemplateName is allowed", i)
			}
		}
	} else {

		// If DRAResources is not empty, all DRAResources objects must have a unique DRAResource.ResourceClaimName
		draResources := nimService.Spec.DRAResources
		if len(draResources) > 0 {
			seen := make(map[string]struct{})
			for i, dra := range draResources {
				if dra.ResourceClaimName == nil {
					return fmt.Errorf("DRAResources[%d].ResourceClaimName must not be empty", i)
				}
				if _, exists := seen[*dra.ResourceClaimName]; exists {
					return fmt.Errorf("duplicate DRAResource.ResourceClaimName found: %q", *dra.ResourceClaimName)
				}
				seen[*dra.ResourceClaimName] = struct{}{}
			}
		}
	}
	return nil
}
