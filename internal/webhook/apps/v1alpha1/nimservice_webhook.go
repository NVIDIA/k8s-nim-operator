/*
Copyright 2024.

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

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var nimservicelog = logf.Log.WithName("nimservice-resource")

// SetupNIMServiceWebhookWithManager registers the webhook for NIMService in the manager.
func SetupNIMServiceWebhookWithManager(mgr ctrl.Manager) error {
	validator, err := NewNIMServiceCustomValidator()
	if err != nil {
		return err
	}
	return ctrl.NewWebhookManagedBy(mgr).For(&appsv1alpha1.NIMService{}).
		WithValidator(validator).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-apps-nvidia-com-v1alpha1-nimservice,mutating=false,failurePolicy=fail,sideEffects=None,groups=apps.nvidia.com,resources=nimservices,verbs=create;update,versions=v1alpha1,name=vnimservice-v1alpha1.kb.io,admissionReviewVersions=v1

// NIMServiceCustomValidator struct is responsible for validating the NIMService resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type NIMServiceCustomValidator struct {
	k8sVersion string
}

var _ webhook.CustomValidator = &NIMServiceCustomValidator{}

// NewNIMServiceCustomValidator fetches and caches the Kubernetes version.
func NewNIMServiceCustomValidator() (*NIMServiceCustomValidator, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %v", err)
	}
	disco, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %v", err)
	}
	versionInfo, err := disco.ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes server version: %v", err)
	}
	return &NIMServiceCustomValidator{k8sVersion: versionInfo.GitVersion}, nil
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type NIMService.
func (v *NIMServiceCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	nimservice, ok := obj.(*appsv1alpha1.NIMService)
	if !ok {
		return nil, fmt.Errorf("expected a NIMService object but got %T", obj)
	}
	nimservicelog.Info("Validation for NIMService upon creation", "name", nimservice.GetName())

	// Validate Image configuration
	if err := validateImageConfiguration(&nimservice.Spec.Image); err != nil {
		return nil, err
	}

	// Ensure AuthSecret is a non-empty string
	if nimservice.Spec.AuthSecret == "" {
		return nil, fmt.Errorf("NIMService.AuthSecret is required")
	}

	// Validate Storage configuration
	if err := validateServiceStorageConfiguration(nimservice); err != nil {
		return nil, err
	}

	// If Spec.Expose.Ingress.Enabled is true, Spec.Expose.Ingress.Spec must be non-nil
	if nimservice.Spec.Expose.Ingress.Enabled != nil && *nimservice.Spec.Expose.Ingress.Enabled {
		if nimservice.Spec.Expose.Ingress.Spec.IngressClassName == nil && nimservice.Spec.Expose.Ingress.Spec.DefaultBackend == nil &&
			len(nimservice.Spec.Expose.Ingress.Spec.TLS) == 0 && len(nimservice.Spec.Expose.Ingress.Spec.Rules) == 0 {
			return nil, fmt.Errorf("if Spec.Expose.Ingress.Enabled is true, Spec.Expose.Ingress.Spec must be defined")
		}
	}

	// If Spec.Metrics.Enabled is true, Spec.Metrics.ServiceMonitor must not be empty
	if nimservice.Spec.Metrics.Enabled != nil && *nimservice.Spec.Metrics.Enabled {
		if len(nimservice.Spec.Metrics.ServiceMonitor.AdditionalLabels) == 0 && len(nimservice.Spec.Metrics.ServiceMonitor.Annotations) == 0 &&
			nimservice.Spec.Metrics.ServiceMonitor.Interval == "" && nimservice.Spec.Metrics.ServiceMonitor.ScrapeTimeout == "" {
			return nil, fmt.Errorf("if Spec.Metrics.Enabled is true, Spec.Metrics.ServiceMonitor must be defined")
		}
	}

	// If Spec.Scale.Enabled is true, Spec.Scale.HPA must be non-empty HorizontalPodAutoScaler
	if nimservice.Spec.Scale.Enabled != nil && *nimservice.Spec.Scale.Enabled {
		if nimservice.Spec.Scale.HPA.MinReplicas == nil && nimservice.Spec.Scale.HPA.MaxReplicas == 0 && len(nimservice.Spec.Scale.HPA.Metrics) == 0 && nimservice.Spec.Scale.HPA.Behavior == nil {
			return nil, fmt.Errorf("if Spec.Scale.Enabled is true, Spec.Scale.HPA must be defined")
		}
	}

	// Spec.DRAResources must be empty unless K8s version is v1.33.1+
	if len(nimservice.Spec.DRAResources) > 0 {
		var major, minor, patch int
		n, err := fmt.Sscanf(v.k8sVersion, "v%d.%d.%d", &major, &minor, &patch)
		if err != nil || n < 3 {
			return nil, fmt.Errorf("unable to parse Kubernetes server version: %s", v.k8sVersion)
		}
		if major < 1 || (major == 1 && (minor < 33 || (minor == 33 && patch < 1))) {
			return nil, fmt.Errorf("Spec.DRAResources is only supported on Kubernetes v1.33.1 or newer (server version: %s)", v.k8sVersion)
		}
	}

	// Spec.Resources.Claims must be empty
	if nimservice.Spec.Resources != nil {
		if nimservice.Spec.Resources.Claims != nil || len(nimservice.Spec.Resources.Claims) != 0 {
			return nil, fmt.Errorf("NIMService.Spec.Resources.Claims must be empty")
		}
	}

	// Validate Spec.DRAResources
	if err := validateDRAResourcesConfiguration(nimservice); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type NIMService.
func (v *NIMServiceCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	nimservice, ok := newObj.(*appsv1alpha1.NIMService)
	if !ok {
		return nil, fmt.Errorf("expected a NIMService object for the newObj but got %T", newObj)
	}
	nimservicelog.Info("Validation for NIMService upon update", "name", nimservice.GetName())

	// All fields of NIMService.Spec are mutable, except for:
	// - Spec.MultiNode
	// - If PVC has been created with PVC.Create = true, reject any updates to any fields of PVC object

	oldNIMService, ok := oldObj.(*appsv1alpha1.NIMService)
	if !ok {
		return nil, fmt.Errorf("expected a NIMService object for oldObj but got %T", oldObj)
	}
	newNIMService, ok := newObj.(*appsv1alpha1.NIMService)
	if !ok {
		return nil, fmt.Errorf("expected a NIMService object for newObj but got %T", newObj)
	}

	// Spec.MultiNode is not mutable
	if !equality.Semantic.DeepEqual(oldNIMService.Spec.MultiNode,
		newNIMService.Spec.MultiNode) {
		return nil, fmt.Errorf("NIMService.Spec.Multinode is immutable once the resource is created")
	}

	// If PVC has been created with PVC.Create = true, reject any updates to any fields of PVC object
	oldPVC := oldNIMService.Spec.Storage.PVC
	newPVC := newNIMService.Spec.Storage.PVC
	if oldPVC.Create != nil && *oldPVC.Create {
		// Once the PVC has been created with `create: true` we reject any field changes.
		if !equality.Semantic.DeepEqual(oldPVC, newPVC) {
			return nil, fmt.Errorf("NIMService.Spec.Storage.PVC is immutable once it is created with PVC.Create = true")
		}
	}

	return nil, nil
}

func (v *NIMServiceCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	// No deletion-time validation logic for NIMService. Returning nil allows deletes without extra checks.
	return nil, nil
}
