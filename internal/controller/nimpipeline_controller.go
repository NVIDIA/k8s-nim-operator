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
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	utils "github.com/NVIDIA/k8s-nim-operator/internal/utils"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

// NIMPipelineReconciler reconciles a NIMPipeline object.
type NIMPipelineReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	recorder record.EventRecorder
}

const (
	// NIMPipelineFinalizer is the finalizer annotation.
	NIMPipelineFinalizer = "finalizer.nimpipeline.apps.nvidia.com"
)

// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimpipelines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimpipelines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimpipelines/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *NIMPipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	defer r.refreshMetrics(ctx)

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
	if nimPipeline.DeletionTimestamp.IsZero() {
		// Add finalizer if not present
		if !controllerutil.ContainsFinalizer(nimPipeline, NIMPipelineFinalizer) {
			controllerutil.AddFinalizer(nimPipeline, NIMPipelineFinalizer)
			if err := r.Update(ctx, nimPipeline); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The instance is being deleted
		// Perform cleanup of resources
		if err := r.cleanupNIMPipeline(ctx, nimPipeline); err != nil {
			return ctrl.Result{}, err
		}

		if controllerutil.ContainsFinalizer(nimPipeline, NIMPipelineFinalizer) {
			// Remove finalizer to allow for deletion
			controllerutil.RemoveFinalizer(nimPipeline, NIMPipelineFinalizer)
			if err := r.Update(ctx, nimPipeline); err != nil {
				return ctrl.Result{}, err
			}
		}
		// return as the cr is being deleted and GC will cleanup owned objects
		return ctrl.Result{}, nil
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
	logger := log.FromContext(ctx)
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
			logger.Error(err, "Failed to reconcile NIMService", "name", service.Name)
			continue
		}
	}

	// Update status of NIMPipeline based on the status of related NIMServices
	if err := r.updateStatus(ctx, nimPipeline, enabledServices); err != nil {
		return ctrl.Result{}, err
	}

	// Clean up disabled NIMServices
	if err := r.cleanupDisabledNIMs(ctx, nimPipeline, enabledServices); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NIMPipelineReconciler) injectDependencies(nimService *appsv1alpha1.NIMService, dependencies []appsv1alpha1.ServiceDependency) {
	for _, dep := range dependencies {
		var serviceEnvVars []corev1.EnvVar

		// Check if custom environment variable names and values are provided and inject them
		if dep.EnvName != "" && dep.EnvValue != "" {
			// Use custom environment variables
			serviceEnvVars = []corev1.EnvVar{
				{
					Name:  dep.EnvName,
					Value: dep.EnvValue,
				},
			}
			// Merge and inject the environment variables
			nimService.Spec.Env = utils.MergeEnvVars(nimService.Spec.Env, serviceEnvVars)
		}
	}
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

	// Inject service dependencies
	r.injectDependencies(nimService, service.Dependencies)

	// Set NIMPipeline as the owner and controller of the NIMService
	if err := controllerutil.SetControllerReference(nimPipeline, nimService, r.Scheme); err != nil {
		return err
	}

	// Sync NIMService with the desired spec
	namespacedName := types.NamespacedName{Name: nimService.Name, Namespace: nimService.Namespace}
	err := r.syncResource(ctx, namespacedName, nimService)
	if err != nil {
		logger.Error(err, "Failed to sync NIMService", "name", nimService.Name)
		return err
	}

	return nil
}

func (r *NIMPipelineReconciler) syncResource(ctx context.Context, currentNamespacedName types.NamespacedName, desired *appsv1alpha1.NIMService) error {
	logger := log.FromContext(ctx)

	current := &appsv1alpha1.NIMService{}
	err := r.Get(ctx, currentNamespacedName, current)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if !utils.IsSpecChanged(current, desired) {
		logger.V(2).Info("NIMService spec has not changed, skipping update", "obj", current)
		return nil
	}
	logger.V(2).Info("NIMService spec has changed, updating")

	if errors.IsNotFound(err) {
		// Resource doesn't exist, so create it
		err = r.Create(ctx, desired)
		if err != nil {
			return err
		}
	} else {
		// Resource exists, so update it
		// Ensure the resource version is carried over to the desired object
		desired.ResourceVersion = current.ResourceVersion

		err = r.Update(ctx, desired)
		if err != nil {
			return err
		}
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

	var allErrors []error

	for _, svc := range serviceList.Items {
		owned := false
		for _, ownerRef := range svc.GetOwnerReferences() {
			if ownerRef.Kind == "NIMPipeline" && ownerRef.UID == nimPipeline.UID {
				owned = true
				break
			}
		}

		// Ignore NIM services not owned by the NIM pipeline
		if !owned {
			continue
		}

		// Cleanup any stale NIM services if they are part of the pipeline but are disabled
		if enabled, exists := enabledServices[svc.Name]; exists && !enabled {
			if err := r.deleteService(ctx, &svc); err != nil {
				logger.Error(err, "Unable to delete disabled NIM service", "Name", svc.Name)
				allErrors = append(allErrors, fmt.Errorf("failed to delete service %s: %w", svc.Name, err))
			}
		}
	}

	// If there were any errors during the cleanup process, return the overall error
	if len(allErrors) > 0 {
		return fmt.Errorf("errors during cleanup: %v", allErrors)
	}

	return nil
}

func (r *NIMPipelineReconciler) updateStatus(ctx context.Context, nimPipeline *appsv1alpha1.NIMPipeline, enabledServices map[string]bool) error {
	logger := log.FromContext(ctx)

	// List NIMServices in the pipeline's namespace
	serviceList := &appsv1alpha1.NIMServiceList{}
	if err := r.List(ctx, serviceList, client.InNamespace(nimPipeline.Namespace)); err != nil {
		logger.Error(err, "Failed to list NIMServices")
		return err
	}

	// Default overall state to "NotReady"
	overallState := appsv1alpha1.NIMServiceStatusNotReady
	serviceStates := make(map[string]string)
	allServicesReady := true

	// Track which enabled services are found
	foundServices := make(map[string]bool)

	for _, svc := range serviceList.Items {
		owned := false
		for _, ownerRef := range svc.GetOwnerReferences() {
			if ownerRef.Kind == "NIMPipeline" && ownerRef.UID == nimPipeline.UID {
				owned = true
				break
			}
		}

		if enabled, exists := enabledServices[svc.Name]; !owned || !exists || !enabled {
			continue
		}

		// Mark the service as found
		foundServices[svc.Name] = true

		// Update service states
		serviceStates[svc.Name] = svc.Status.State

		switch svc.Status.State {
		case appsv1alpha1.NIMServiceStatusFailed:
			// If any service has failed, set the overall state to "Failed"
			overallState = appsv1alpha1.NIMServiceStatusFailed
			allServicesReady = false
		case appsv1alpha1.NIMServiceStatusNotReady, appsv1alpha1.NIMServiceStatusPending:
			// If any service is not ready or pending, set overall readiness to false
			allServicesReady = false
		}
	}

	// Check if any enabled services are missing
	for serviceName := range enabledServices {
		if !foundServices[serviceName] {
			// A required service is missing, mark as "NotReady"
			allServicesReady = false
			serviceStates[serviceName] = appsv1alpha1.NIMServiceStatusNotReady
		}
	}

	// If all services are ready and no failures were detected, set the overall state to "Ready"
	if allServicesReady && overallState != appsv1alpha1.NIMServiceStatusFailed {
		overallState = appsv1alpha1.NIMServiceStatusReady
	}

	// Update the NIMPipeline status
	nimPipeline.Status.State = overallState
	nimPipeline.Status.States = serviceStates

	r.GetEventRecorder().Eventf(nimPipeline, corev1.EventTypeNormal, overallState,
		"NIMPipeline %s status %s, service states %v", nimPipeline.Name, overallState, serviceStates)

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

// GetEventRecorder returns the event recorder.
func (r *NIMPipelineReconciler) GetEventRecorder() record.EventRecorder {
	return r.recorder
}

// SetupWithManager sets up the controller with the Manager.
func (r *NIMPipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor("nimpipeline-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.NIMPipeline{}).
		Owns(&appsv1alpha1.NIMService{}).
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Type assert to NIMPipeline
				if oldNIMPipeline, ok := e.ObjectOld.(*appsv1alpha1.NIMPipeline); ok {
					newNIMPipeline, ok := e.ObjectNew.(*appsv1alpha1.NIMPipeline)
					if ok {
						// Handle case where object is marked for deletion
						if !newNIMPipeline.ObjectMeta.DeletionTimestamp.IsZero() {
							return true
						}

						// Handle only spec updates
						return !reflect.DeepEqual(oldNIMPipeline.Spec, newNIMPipeline.Spec)
					}
				}
				// For other types we watch, reconcile them
				return true
			},
		}).
		Complete(r)
}

func (r *NIMPipelineReconciler) refreshMetrics(ctx context.Context) {
	logger := log.FromContext(ctx)
	// List all nodes
	nimPipelineList := &appsv1alpha1.NIMPipelineList{}
	err := r.List(ctx, nimPipelineList, &client.ListOptions{})
	if err != nil {
		logger.Error(err, "unable to list nim pipelines in the cluster")
		return
	}
	refreshNIMPipelineMetrics(nimPipelineList)
}
