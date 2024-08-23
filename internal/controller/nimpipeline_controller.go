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

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NIMPipelineReconciler reconciles a NIMPipeline object
type NIMPipelineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	// NIMPipelineFinalizer is the finalizer annotation
	NIMPipelineFinalizer = "finalizer.nimpipeline.apps.nvidia.com"
)

// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimpipelines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimpipelines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimpipelines/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimservices,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *NIMPipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the NIMPipeline instance
	nimPipeline := &appsv1alpha1.NIMPipeline{}
	if err := r.Get(ctx, req.NamespacedName, nimPipeline); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "unable to fetch NIMPipeline", "name", req.NamespacedName)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling", "NIMPipeline", nimPipeline.Name)

	// Check if the instance is marked for deletion
	if nimPipeline.ObjectMeta.DeletionTimestamp.IsZero() {
		// Add finalizer if not present
		if !controllerutil.ContainsFinalizer(nimPipeline, NIMPipelineFinalizer) {
			controllerutil.AddFinalizer(nimPipeline, NIMPipelineFinalizer)
			if err := r.Update(ctx, nimPipeline); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The instance is being deleted
		if controllerutil.ContainsFinalizer(nimPipeline, NIMPipelineFinalizer) {
			// Perform cleanup of resources
			if err := r.cleanupNIMPipeline(ctx, nimPipeline); err != nil {
				return ctrl.Result{}, err
			}

			// Remove finalizer to allow for deletion
			controllerutil.RemoveFinalizer(nimPipeline, NIMPipelineFinalizer)
			if err := r.Update(ctx, nimPipeline); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	// Handle nim-cache reconciliation
	if result, err := r.reconcileNIMPipeline(ctx, nimPipeline); err != nil {
		logger.Error(err, "error reconciling NIMCache", "name", nimPipeline.Name)
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *NIMPipelineReconciler) cleanupNIMPipeline(ctx context.Context, nimPipeline *appsv1alpha1.NIMPipeline) error {
	// TODO: Add custom cleanup

	// All owned objects are garbage collected
	return nil
}

func (r *NIMPipelineReconciler) reconcileNIMPipeline(ctx context.Context, nimPipeline *appsv1alpha1.NIMPipeline) (ctrl.Result, error) {
	// Track enabled NIMServices
	enabledServices := make(map[string]bool)

	// Process each service specification in the pipeline
	for _, service := range nimPipeline.Spec.Services {
		enabled := service.Enabled != nil && *service.Enabled
		enabledServices[service.Name] = enabled

		if !enabled {
			continue
		}

		if err := r.reconcileNIMService(ctx, nimPipeline, service); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Clean up disabled NIMServices
	if err := r.cleanupDisabledNIMs(ctx, nimPipeline, enabledServices); err != nil {
		return ctrl.Result{}, err
	}

	// Update status of NIMServices
	if err := r.updateStatus(ctx, nimPipeline, enabledServices); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NIMPipelineReconciler) reconcileNIMService(ctx context.Context, nimPipeline *appsv1alpha1.NIMPipeline, service appsv1alpha1.NIMServicePipelineSpec) error {
	logger := log.FromContext(ctx)

	nimService := &appsv1alpha1.NIMService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      service.Name,
			Namespace: nimPipeline.Namespace,
		},
		Spec: service.Spec,
	}

	// Set NIMPipeline as the owner and controller of the NIMService
	if err := controllerutil.SetControllerReference(nimPipeline, nimService, r.Scheme); err != nil {
		return err
	}

	// Check if the NIMService already exists and create or update as necessary
	existingService := &appsv1alpha1.NIMService{}
	err := r.Get(ctx, types.NamespacedName{Name: nimService.Name, Namespace: nimService.Namespace}, existingService)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Creating new NIMService", "name", nimService.Name)
			return r.Create(ctx, nimService)
		}
		logger.Error(err, "Failed to get NIMService", "name", nimService.Name)
		return err
	}

	// Update the existing NIMService's spec if needed
	logger.Info("Updating existing NIMService", "name", nimService.Name)
	existingService.Spec = nimService.Spec
	if err := r.Update(ctx, existingService); err != nil {
		logger.Error(err, "Failed to update NIMService", "name", nimService.Name)
		return err
	}

	return nil
}

func (r *NIMPipelineReconciler) cleanupDisabledNIMs(ctx context.Context, nimPipeline *appsv1alpha1.NIMPipeline, enabledServices map[string]bool) error {
	logger := log.FromContext(ctx)

	serviceList := &appsv1alpha1.NIMServiceList{}
	if err := r.List(ctx, serviceList, client.InNamespace(nimPipeline.Namespace)); err != nil {
		logger.Error(err, "Failed to list NIMServices")
		return err
	}

	for _, svc := range serviceList.Items {
		// Cleanup any stale NIM services if they are part of the pipeline but are disabled
		if enabled, exists := enabledServices[svc.Name]; exists && !enabled {
			if err := r.deleteService(ctx, &svc); err != nil {
				logger.Error(err, "Unable to delete disabled NIM service", "Name", svc.Name)
				return err
			}
			continue
		}
	}
	return nil
}

func (r *NIMPipelineReconciler) updateStatus(ctx context.Context, nimPipeline *appsv1alpha1.NIMPipeline, enabledServices map[string]bool) error {
	logger := log.FromContext(ctx)

	serviceList := &appsv1alpha1.NIMServiceList{}
	if err := r.List(ctx, serviceList, client.InNamespace(nimPipeline.Namespace)); err != nil {
		logger.Error(err, "Failed to list NIMServices")
		return err
	}

	// Default overall state to "NotReady"
	overallState := appsv1alpha1.NIMServiceStatusNotReady
	serviceStates := make(map[string]string)
	allServicesReady := true

	for _, svc := range serviceList.Items {
		if enabled, exists := enabledServices[svc.Name]; !exists || !enabled {
			continue
		}

		// Update service states
		serviceStates[svc.Name] = svc.Status.State

		switch svc.Status.State {
		case appsv1alpha1.NIMServiceStatusFailed:
			// If any service has failed, the overall state should be "Failed"
			overallState = appsv1alpha1.NIMServiceStatusFailed
			allServicesReady = false
			break
		case appsv1alpha1.NIMServiceStatusNotReady:
			fallthrough
		case appsv1alpha1.NIMServiceStatusPending:
			// If any service is not ready, mark overall readiness as false
			allServicesReady = false
		}
	}

	// If all services are ready and no failures were detected, set overall state to "Ready"
	if allServicesReady && overallState != appsv1alpha1.NIMServiceStatusFailed {
		overallState = appsv1alpha1.NIMServiceStatusReady
	}

	// Update the NIMPipeline status
	nimPipeline.Status.State = overallState
	nimPipeline.Status.States = serviceStates

	if err := r.Status().Update(ctx, nimPipeline); err != nil {
		logger.Error(err, "Failed to update NIMPipeline status")
		return err
	}

	return nil
}

func (r *NIMPipelineReconciler) deleteService(ctx context.Context, svc *appsv1alpha1.NIMService) error {
	logger := log.FromContext(ctx)
	logger.Info("Deleting NIMService", "name", svc.Name, "namespace", svc.Namespace)
	if err := r.Delete(ctx, svc); err != nil {
		logger.Error(err, "Failed to delete NIMService", "name", svc.Name)
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NIMPipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.NIMPipeline{}).
		Owns(&appsv1alpha1.NIMService{}).
		Complete(r)
}
