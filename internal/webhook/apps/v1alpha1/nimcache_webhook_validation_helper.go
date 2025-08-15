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
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

var (
	reHostname = regexp.MustCompile(`^\.?[a-zA-Z0-9.-]+$`)             // .example.com or example.com
	reIPv4     = regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}(:\d+)?$`)  // 10.1.2.3 or 10.1.2.3:8080
	reIPv6     = regexp.MustCompile(`^\[[0-9a-fA-F:]+\](?::\d+)?$`)    // [2001:db8::1] or [2001:db8::1]:443
	reCIDR4    = regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}/\d{1,2}$`) // 10.0.0.0/8
	reCIDR6    = regexp.MustCompile(`^\[[0-9a-fA-F:]+\]/\d{1,3}$`)     // [2001:db8::]/32
)

var validQoSProfiles = []string{"latency", "throughput"}

// validateNIMSourceConfiguration validates the NIMSource configuration in the NIMCache spec.
func validateNIMSourceConfiguration(source *appsv1alpha1.NIMSource, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	// Evalutate NGCSource if it is set. NemoDataStoreSource and HuggingFaceHubSource do not require any additional validation.
	warnList, errList := validateNGCSource(source.NGC, fldPath.Child("ngc"))
	return warnList, errList
}

// ValidateNGCSource checks the NGCSource configuration.
func validateNGCSource(ngcSource *appsv1alpha1.NGCSource, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	// Return early if NGCSource is nil
	if ngcSource == nil {
		return nil, nil
	}

	// Evaluate NGCSource.Model fields
	warnList, errList := validateModel(ngcSource.Model, fldPath.Child("model"))

	// Ensure AuthSecret is a non-empty string
	if ngcSource.AuthSecret == "" {
		errList = append(errList, field.Required(fldPath.Child("authSecret"), "must be non-empty"))
	}

	// Ensure ModelPuller is a non-empty string
	if ngcSource.ModelPuller == "" {
		errList = append(errList, field.Required(fldPath.Child("modelPuller"), "must be non-empty"))
	}

	return warnList, errList
}

func validateModel(model *appsv1alpha1.ModelSpec, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	errList := field.ErrorList{}
	warnList := admission.Warnings{}

	if model == nil {
		return warnList, errList
	}

	// If Model.Profiles is not empty, ensure all other Model fields are empty. If Model.Profiles contains "all", length must be 1
	if len(model.Profiles) > 0 {

		for _, profile := range model.Profiles {
			if profile == "all" && len(model.Profiles) != 1 {
				errList = append(errList, field.Invalid(fldPath.Child("profiles"), model.Profiles, "must only have a single entry when it contains 'all'"))
				break
			}
		}

		// Ensure all other Model fields are empty
		if model.Precision != "" {
			errList = append(errList, field.Forbidden(fldPath.Child("precision"), fmt.Sprintf("must be empty if %s is provided", fldPath.Child("profiles"))))
		}
		if model.Engine != "" {
			errList = append(errList, field.Forbidden(fldPath.Child("engine"), fmt.Sprintf("must be empty if %s is provided", fldPath.Child("profiles"))))
		}
		if model.TensorParallelism != "" {
			errList = append(errList, field.Forbidden(fldPath.Child("tensorParallelism"), fmt.Sprintf("must be empty if %s is provided", fldPath.Child("profiles"))))
		}
		if model.QoSProfile != "" {
			errList = append(errList, field.Forbidden(fldPath.Child("qosProfile"), fmt.Sprintf("must be empty if %s is provided", fldPath.Child("profiles"))))
		}
		if len(model.GPUs) > 0 {
			errList = append(errList, field.Forbidden(fldPath.Child("gpus"), fmt.Sprintf("must be empty if %s is provided", fldPath.Child("profiles"))))
		}
		if model.Lora != nil {
			errList = append(errList, field.Forbidden(fldPath.Child("lora"), fmt.Sprintf("must be empty if %s is provided", fldPath.Child("profiles"))))
		}
		if model.Buildable != nil {
			errList = append(errList, field.Forbidden(fldPath.Child("buildable"), fmt.Sprintf("must be empty if %s is provided", fldPath.Child("profiles"))))
		}
	}

	if model.QoSProfile != "" && !isValidQoSProfile(model.QoSProfile) {
		errList = append(errList, field.NotSupported(fldPath.Child("qosProfile"), model.QoSProfile, validQoSProfiles))
	}

	return warnList, errList
}

func isValidQoSProfile(profile string) bool {
	for _, p := range validQoSProfiles {
		if profile == p {
			return true
		}
	}
	return false
}

func validateNIMCacheStorageConfiguration(storage *appsv1alpha1.NIMCacheStorage, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	// Spec.Storage must not be empty
	if reflect.DeepEqual(storage.PVC, appsv1alpha1.PersistentVolumeClaim{}) {
		// Don't validate PVC configuration if storage is completely empty
		return admission.Warnings{}, field.ErrorList{field.Required(fldPath, "must not be empty")}
	}

	return validatePVCConfiguration(&storage.PVC, fldPath.Child("pvc"))
}

func validatePVCConfiguration(pvc *appsv1alpha1.PersistentVolumeClaim, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	errList := field.ErrorList{}
	warnList := admission.Warnings{}

	// If PVC.Create is False, PVC.Name cannot be empty
	if (pvc.Create == nil || !*pvc.Create) && pvc.Name == "" {
		errList = append(errList, field.Required(fldPath.Child("name"), fmt.Sprintf("must be provided when %s is false", fldPath.Child("create"))))
	}

	// If PVC.VolumeAccessMode is defined, it must be one of the valid modes
	if pvc.VolumeAccessMode != "" {
		found := false
		for _, vm := range validPVCAccessModeStrs {
			if pvc.VolumeAccessMode == vm {
				found = true
				break
			}
		}

		if !found {
			errList = append(errList, field.Invalid(fldPath.Child("volumeAccessMode"), pvc.VolumeAccessMode, "unrecognized volumeAccessMode"))
		}
	}

	return warnList, errList
}

func validateProxyConfiguration(proxy *appsv1alpha1.ProxySpec, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warnList := admission.Warnings{}
	errList := field.ErrorList{}
	if proxy == nil {
		return nil, nil
	}

	// If Proxy is not nil, ensure Proxy.NoProxy is a valid proxy string
	if proxy.NoProxy != "" {
		for i, token := range strings.Split(proxy.NoProxy, ",") {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			if reHostname.MatchString(token) || reIPv4.MatchString(token) || reIPv6.MatchString(token) || reCIDR4.MatchString(token) || reCIDR6.MatchString(token) {
				continue
			}
			errList = append(errList, field.Invalid(fldPath.Child("noProxy").Index(i), token, "invalid token"))
		}
	}

	// Ensure Http or Https proxy is valid
	re := regexp.MustCompile(`^https?://`)
	if proxy.HttpsProxy != "" && !re.MatchString(proxy.HttpsProxy) {
		errList = append(errList, field.Invalid(fldPath.Child("httpsProxy"), proxy.HttpsProxy, "must start with http:// or https://"))
	}
	if proxy.HttpProxy != "" && !re.MatchString(proxy.HttpProxy) {
		errList = append(errList, field.Invalid(fldPath.Child("httpProxy"), proxy.HttpProxy, "must start with http:// or https://"))
	}

	return warnList, errList
}

// validateNIMCacheSpec aggregates all structural validation rules for a NIMCache
// resource. This central function is intended for use by both webhook
// ValidateCreate and ValidateUpdate methods so that they share identical
// well-formedness checks.
//
// Parameters:
//
//	– spec:  pointer to the NIMCacheSpec being validated.
//	– fldPath: field path pointing at the root of the spec (typically
//	  field.NewPath("nimcache").Child("spec")).
//
// Returns a field.ErrorList with any validation errors encountered.
func validateNIMCacheSpec(spec *appsv1alpha1.NIMCacheSpec, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	warningList, errList := validateNIMSourceConfiguration(&spec.Source, fldPath.Child("source"))

	// Delegate to existing granular validators.
	wList, eList := validateNIMCacheStorageConfiguration(&spec.Storage, fldPath.Child("storage"))
	warningList = append(warningList, wList...)
	errList = append(errList, eList...)

	wList, eList = validateProxyConfiguration(spec.Proxy, fldPath.Child("proxy"))
	warningList = append(warningList, wList...)
	errList = append(errList, eList...)

	return warningList, errList
}

func validateImmutableNIMCacheSpec(oldNIMCache, newNIMCache *appsv1alpha1.NIMCache, fldPath *field.Path) (admission.Warnings, field.ErrorList) {
	errList := field.ErrorList{}
	warningList := admission.Warnings{}
	if !equality.Semantic.DeepEqual(oldNIMCache.Spec, newNIMCache.Spec) {
		errList = append(errList, field.Forbidden(fldPath.Child("spec"), "is immutable once the object is created"))
	}

	return warningList, errList
}
