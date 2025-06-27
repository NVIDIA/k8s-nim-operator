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
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var nimcachelog = logf.Log.WithName("webhooks").WithName("NIMCache")

// SetupNIMCacheWebhookWithManager registers the webhook for NIMCache in the manager.
func SetupNIMCacheWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&appsv1alpha1.NIMCache{}).
		WithValidator(&NIMCacheCustomValidator{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-apps-nvidia-com-v1alpha1-nimcache,mutating=false,failurePolicy=fail,sideEffects=None,groups=apps.nvidia.com,resources=nimcaches,verbs=create;update,versions=v1alpha1,name=vnimcache-v1alpha1.kb.io,admissionReviewVersions=v1

// NIMCacheCustomValidator struct is responsible for validating the NIMCache resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type NIMCacheCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &NIMCacheCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type NIMCache.
// Validate initial resource configuration, required fields, resource limits.
func (v *NIMCacheCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	nimcache, ok := obj.(*appsv1alpha1.NIMCache)
	if !ok {
		return nil, fmt.Errorf("expected a NIMCache object but got %T", obj)
	}
	nimcachelog.V(4).Info("Validation for NIMCache upon creation", "name", nimcache.GetName())

	fldPath := field.NewPath("nimcache").Child("spec")
	// Perform structural validation via helper.
	errList := validateNIMCacheSpec(&nimcache.Spec, fldPath)

	if len(errList) > 0 {
		return nil, errList.ToAggregate()
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type NIMCache.
// Check immutable fields haven't changed, validate transitions between states, ensure updates don't break existing functionality.
func (v *NIMCacheCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	nimcache, ok := newObj.(*appsv1alpha1.NIMCache)
	if !ok {
		return nil, fmt.Errorf("expected a NIMCache object for the newObj but got %T", newObj)
	}
	nimcachelog.V(4).Info("Validation for NIMCache upon update", "name", nimcache.GetName())

	fldPath := field.NewPath("nimcache").Child("spec")

	// Begin by validating the new spec structurally.
	errList := validateNIMCacheSpec(&nimcache.Spec, fldPath)

	oldNIMCache, ok := oldObj.(*appsv1alpha1.NIMCache)
	if !ok {
		return nil, fmt.Errorf("expected a NIMCache object for oldObj but got %T", oldObj)
	}
	newNIMCache, ok := newObj.(*appsv1alpha1.NIMCache)
	if !ok {
		return nil, fmt.Errorf("expected a NIMCache object for newObj but got %T", newObj)
	}

	// Append immutability errors after structural checks.
	errList = append(errList, validateImmutableNIMCacheSpec(oldNIMCache, newNIMCache, field.NewPath("nimcache"))...)

	if len(errList) > 0 {
		return nil, errList.ToAggregate()
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type NIMCache.
// Verify safe deletion conditions, check for dependent resources, ensure cleanup requirements.
func (v *NIMCacheCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
