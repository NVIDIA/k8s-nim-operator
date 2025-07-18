package v1alpha1

import (
	"fmt"
	"regexp"
	"strings"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

var (
	reHostname = regexp.MustCompile(`^\.?[a-zA-Z0-9.-]+$`)             // .example.com or example.com
	reIPv4     = regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}(:\d+)?$`)  // 10.1.2.3 or 10.1.2.3:8080
	reIPv6     = regexp.MustCompile(`^\[[0-9a-fA-F:]+\](?::\d+)?$`)    // [2001:db8::1] or [2001:db8::1]:443
	reCIDR4    = regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}/\d{1,2}$`) // 10.0.0.0/8
	reCIDR6    = regexp.MustCompile(`^\[[0-9a-fA-F:]+\]/\d{1,3}$`)     // [2001:db8::]/32
)

// validateNIMSourceConfiguration validates the NIMSource configuration in the NIMCache spec.
func validateNIMSourceConfiguration(source *appsv1alpha1.NIMSource) error {
	// Evalutate NGCSource if it is set. NemoDataStoreSource and HuggingFaceHubSource do not require any additional validation.
	if err := validateNGCSource(source.NGC); err != nil {
		return err // propagate the error
	}

	return nil
}

// ValidateNGCSource checks the NGCSource configuration.
func validateNGCSource(ngcSource *appsv1alpha1.NGCSource) error {
	// Return early if NGCSource is nil
	if ngcSource == nil {
		return nil
	}

	// Ensure AuthSecret is a non-empty string
	if ngcSource.AuthSecret == "" {
		return fmt.Errorf("NIMCache.Spec.Source.NGC.Authsecret must be non-empty")
	}

	// Ensure ModelPuller is a non-empty string
	if ngcSource.ModelPuller == "" {
		return fmt.Errorf("NIMCache.Spec.Source.NGC.ModelPuller must be non-empty")
	}

	// If Model.Profiles is not empty, ensure all other Model fields are empty
	if len(ngcSource.Model.Profiles) > 0 {
		// Check if it contains "all"
		for _, profile := range ngcSource.Model.Profiles {
			if profile == "all" {
				if len(ngcSource.Model.Profiles) != 1 {
					return fmt.Errorf("NIMCache.Spec.Source.NGC.Model.Profiles must only have a single entry when it contains 'all'")
				}
			}
		}

		// Ensure all other Model fields are empty
		if ngcSource.Model.Precision != "" || ngcSource.Model.Engine != "" || ngcSource.Model.TensorParallelism != "" ||
			ngcSource.Model.QoSProfile != "" || ngcSource.Model.GPUs != nil || len(ngcSource.Model.GPUs) > 0 ||
			ngcSource.Model.Lora != nil || ngcSource.Model.Buildable != nil {
			return fmt.Errorf("the rest of NIMCache.Spec.Source.NGC.Model fields must be empty when Model.Profiles is defined")
		}

	}

	if ngcSource.Model.QoSProfile != "" && (ngcSource.Model.QoSProfile != "latency" && ngcSource.Model.QoSProfile != "throughput") {
		return fmt.Errorf("NIMCache.Spec.Source.NGC.Model.QoSProfile must be 'latency', 'throughput', or empty")
	}

	return nil
}

func validateNIMCacheStorageConfiguration(storage *appsv1alpha1.NIMCacheStorage) error {
	// Spec.Storage must not be empty
	if storage.PVC.Create == nil && storage.PVC.Name == "" && storage.PVC.StorageClass == "" &&
		storage.PVC.Size == "" && storage.PVC.VolumeAccessMode == "" && storage.PVC.SubPath == "" &&
		len(storage.PVC.Annotations) == 0 {
		return fmt.Errorf("NIMCache.Spec.Storage must not be empty")
	}

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

	return nil
}

func validateProxyConfiguration(proxy *appsv1alpha1.ProxySpec) error {
	// If Proxy is not nil, ensure Proxy.NoProxy is a valid proxy string
	if proxy == nil {
		return nil
	}

	// Ensure NoProxy is valid
	if proxy.NoProxy == "" {
		return nil
	} // empty == valid
	for _, token := range strings.Split(proxy.NoProxy, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if reHostname.MatchString(token) ||
			reIPv4.MatchString(token) ||
			reIPv6.MatchString(token) ||
			reCIDR4.MatchString(token) ||
			reCIDR6.MatchString(token) {
			continue
		}
		return fmt.Errorf("invalid NO_PROXY token: %q", token)
	}

	// Ensure Http or Https proxy is valid
	re := regexp.MustCompile(`^https?://`)
	if proxy.HttpsProxy != "" {
		if !re.MatchString(proxy.HttpsProxy) {
			return fmt.Errorf("Proxy.HttpsProxy must start with http:// or https://")
		}
	}
	if proxy.HttpProxy != "" {
		if !re.MatchString(proxy.HttpProxy) {
			return fmt.Errorf("Proxy.HttpProxy must start with http:// or https://")
		}
	}

	return nil
}
