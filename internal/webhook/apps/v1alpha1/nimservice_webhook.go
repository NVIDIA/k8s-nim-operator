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
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var nimservicelog = logf.Log.WithName("webhooks").WithName("NIMService")

// SetupNIMServiceWebhookWithManager registers the webhook for NIMService in the manager.
func SetupNIMServiceWebhookWithManager(mgr ctrl.Manager) error {
	validator, err := NewNIMServiceCustomValidator(mgr.GetClient())
	if err != nil {
		return err
	}
	return ctrl.NewWebhookManagedBy(mgr).For(&appsv1alpha1.NIMService{}).
		WithValidator(validator).
		Complete()
}

// TODO: make all paths proper then resume where you left off on varuns comments.

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
	k8sClient  client.Client
}

var _ webhook.CustomValidator = &NIMServiceCustomValidator{}

// NewNIMServiceCustomValidator fetches and caches the Kubernetes version.
func NewNIMServiceCustomValidator(k8sClient client.Client) (*NIMServiceCustomValidator, error) {
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
	return &NIMServiceCustomValidator{
		k8sVersion: versionInfo.GitVersion,
		k8sClient:  k8sClient,
	}, nil
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type NIMService.
func (v *NIMServiceCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	nimservice, ok := obj.(*appsv1alpha1.NIMService)
	if !ok {
		return nil, fmt.Errorf("expected a NIMService object but got %T", obj)
	}
	nimservicelog.V(4).Info("Validation for NIMService upon creation", "name", nimservice.GetName())

	fldPath := field.NewPath("nimservice").Child("spec")

	// Perform comprehensive spec validation via helper.
	warningList, errList := validateNIMServiceSpec(ctx, nimservice, fldPath, v.k8sVersion, v.k8sClient)

	if len(errList) > 0 {
		return warningList, errList.ToAggregate()
	}

	return warningList, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type NIMService.
func (v *NIMServiceCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldNIMService, ok := oldObj.(*appsv1alpha1.NIMService)
	if !ok {
		return nil, fmt.Errorf("expected a NIMService object for oldObj but got %T", oldObj)
	}
	newNIMService, ok := newObj.(*appsv1alpha1.NIMService)
	if !ok {
		return nil, fmt.Errorf("expected a NIMService object for newObj but got %T", newObj)
	}
	nimservicelog.V(4).Info("Validation for NIMService upon update", "name", newNIMService.GetName())

	fldPath := field.NewPath("nimservice").Child("spec")
	// Start with structural validation to ensure the updated object is well formed.
	warningList, errList := validateNIMServiceSpec(ctx, newNIMService, fldPath, v.k8sVersion, v.k8sClient)

	wList, eList := validateMultiNodeImmutability(oldNIMService, newNIMService, field.NewPath("spec").Child("multiNode"))
	warningList = append(warningList, wList...)
	errList = append(errList, eList...)

	wList, eList = validatePVCImmutability(oldNIMService, newNIMService, field.NewPath("spec").Child("storage").Child("pvc"))
	warningList = append(warningList, wList...)
	errList = append(errList, eList...)

	wList, eList = validateDRAResourceImmutability(oldNIMService, newNIMService, field.NewPath("spec").Child("draResources"))
	warningList = append(warningList, wList...)
	errList = append(errList, eList...)

	if len(errList) > 0 {
		return warningList, errList.ToAggregate()
	}

	return warningList, nil
}

func (v *NIMServiceCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	// No deletion-time validation logic for NIMService. Returning nil allows deletes without extra checks.
	return nil, nil
}
