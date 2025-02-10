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

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

// NemoServiceReconciler reconciles a NemoService object
type NemoServiceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// NemoServiceSpec defines the generic nemo service spec
type NemoServiceSpec[T any] struct {
	Enabled *bool
	Name    string
	Spec    T
}

// ServiceType represents all NeMo microservice types
type ServiceType string

const (
	// NemoServiceFinalizer is the finalizer annotation
	NemoServiceFinalizer = "finalizer.nemoservice.apps.nvidia.com"
	// Customizer service type
	Customizer ServiceType = "Customizer"
	// Datastore service type
	Datastore ServiceType = "Datastore"
	// Evaluator service type
	Evaluator ServiceType = "Evaluator"
	// Guardrails service type
	Guardrails ServiceType = "Guardrails"
	// Entitystore service type
	Entitystore ServiceType = "Entitystore"
)

// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemoservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemoservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemoservices/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemocustomizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemodatastores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemoevaluators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemoguardrails,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemoentitystores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *NemoServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	defer r.refreshMetrics(ctx)

	// Fetch the NemoService instance
	nemoService := &appsv1alpha1.NemoService{}
	if err := r.Get(ctx, req.NamespacedName, nemoService); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "unable to fetch NemoService", "name", req.NamespacedName)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling", "NemoService", nemoService.Name)

	// Check if the instance is marked for deletion
	if nemoService.ObjectMeta.DeletionTimestamp.IsZero() {
		// Add finalizer if not present
		if !controllerutil.ContainsFinalizer(nemoService, NemoServiceFinalizer) {
			controllerutil.AddFinalizer(nemoService, NemoServiceFinalizer)
			if err := r.Update(ctx, nemoService); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The instance is being deleted
		if controllerutil.ContainsFinalizer(nemoService, NemoServiceFinalizer) {
			// Perform cleanup of resources
			if err := r.cleanupNemoService(ctx, nemoService); err != nil {
				return ctrl.Result{}, err
			}

			// Remove finalizer to allow for deletion
			controllerutil.RemoveFinalizer(nemoService, NemoServiceFinalizer)
			if err := r.Update(ctx, nemoService); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	// Handle nemo services reconciliation
	if result, err := r.reconcileNemoServices(ctx, nemoService); err != nil {
		logger.Error(err, "error reconciling Nemo services", "name", nemoService.Name)
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *NemoServiceReconciler) cleanupNemoService(ctx context.Context, nemoService *appsv1alpha1.NemoService) error {
	// TODO: Add custom cleanup

	// All owned objects are garbage collected
	return nil
}

// Reconcile all enabled and disabled Nemo services
func (r *NemoServiceReconciler) reconcileNemoServices(ctx context.Context, nemoService *appsv1alpha1.NemoService) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Nemo services", "name", nemoService.Name)

	enabledServices := make(map[string]bool)

	// Define service reconcilers
	serviceReconcilers := map[ServiceType]func(context.Context, *NemoServiceReconciler, *appsv1alpha1.NemoService) error{
		Customizer: func(ctx context.Context, r *NemoServiceReconciler, ns *appsv1alpha1.NemoService) error {
			spec := NemoServiceSpec[appsv1alpha1.NemoServiceCustomizerSpec]{
				Enabled: ns.Spec.Customizer.Enabled,
				Name:    ns.Spec.Customizer.Name,
				Spec:    ns.Spec.Customizer,
			}
			enabledServices[spec.Name] = *spec.Enabled
			return reconcileGenericService(ctx, r, ns, spec, &appsv1alpha1.NemoCustomizer{})
		},
		Datastore: func(ctx context.Context, r *NemoServiceReconciler, ns *appsv1alpha1.NemoService) error {
			spec := NemoServiceSpec[appsv1alpha1.NemoServiceDatastoreSpec]{
				Enabled: ns.Spec.Datastore.Enabled,
				Name:    ns.Spec.Datastore.Name,
				Spec:    ns.Spec.Datastore,
			}
			enabledServices[spec.Name] = *spec.Enabled
			return reconcileGenericService(ctx, r, ns, spec, &appsv1alpha1.NemoDatastore{})
		},
		Evaluator: func(ctx context.Context, r *NemoServiceReconciler, ns *appsv1alpha1.NemoService) error {
			spec := NemoServiceSpec[appsv1alpha1.NemoServiceEvaluatorSpec]{
				Enabled: ns.Spec.Evaluator.Enabled,
				Name:    ns.Spec.Evaluator.Name,
				Spec:    ns.Spec.Evaluator,
			}
			enabledServices[spec.Name] = *spec.Enabled
			return reconcileGenericService(ctx, r, ns, spec, &appsv1alpha1.NemoEvaluator{})
		},
		Guardrails: func(ctx context.Context, r *NemoServiceReconciler, ns *appsv1alpha1.NemoService) error {
			spec := NemoServiceSpec[appsv1alpha1.NemoServiceGuardrailsSpec]{
				Enabled: ns.Spec.Guardrails.Enabled,
				Name:    ns.Spec.Guardrails.Name,
				Spec:    ns.Spec.Guardrails,
			}
			enabledServices[spec.Name] = *spec.Enabled
			return reconcileGenericService(ctx, r, ns, spec, &appsv1alpha1.NemoGuardrail{})
		},
		Entitystore: func(ctx context.Context, r *NemoServiceReconciler, ns *appsv1alpha1.NemoService) error {
			spec := NemoServiceSpec[appsv1alpha1.NemoServiceEntitystoreSpec]{
				Enabled: ns.Spec.Entitystore.Enabled,
				Name:    ns.Spec.Entitystore.Name,
				Spec:    ns.Spec.Entitystore,
			}
			enabledServices[spec.Name] = *spec.Enabled
			return reconcileGenericService(ctx, r, ns, spec, &appsv1alpha1.NemoEntitystore{})
		},
	}

	// Reconcile all enabled services
	for serviceType, reconcileFunc := range serviceReconcilers {
		serviceSpec := getServiceSpec(nemoService, serviceType)
		if serviceSpec.Enabled != nil && *serviceSpec.Enabled {
			logger.Info("Ensuring NeMo service is deployed", "service", serviceType, "name", serviceSpec.Name)
			if err := reconcileFunc(ctx, r, nemoService); err != nil {
				logger.Error(err, "Failed to reconcile NeMo service", "service", serviceType, "name", serviceSpec.Name)
				return ctrl.Result{}, err
			}
		}
	}

	// Update status of NeMoService
	if err := r.updateStatus(ctx, nemoService, enabledServices); err != nil {
		return ctrl.Result{}, err
	}

	// Cleanup disabled services
	if err := r.cleanupDisabledNemoMicroservices(ctx, nemoService, enabledServices); err != nil {
		logger.Error(err, "Failed to cleanup disabled NeMo microservices")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// Generic function to set spec for all NeMo service types
func SetObjectSpec[T any](obj client.Object, service NemoServiceSpec[T]) error {
	// Ensure obj implements runtime.Object
	if _, ok := obj.(runtime.Object); !ok {
		return fmt.Errorf("obj does not implement runtime.Object")
	}

	// Extract the spec from the generic service type
	spec := service.Spec

	// Handle all NeMo service types
	switch t := obj.(type) {
	case *appsv1alpha1.NemoCustomizer:
		serviceSpec, ok := any(spec).(appsv1alpha1.NemoServiceCustomizerSpec)
		if !ok {
			return fmt.Errorf("service spec is not of type NemoServiceCustomizerSpec")
		}
		t.Spec = *serviceSpec.Spec

	case *appsv1alpha1.NemoDatastore:
		serviceSpec, ok := any(spec).(appsv1alpha1.NemoServiceDatastoreSpec)
		if !ok {
			return fmt.Errorf("service spec is not of type NemoServiceDatastoreSpec")
		}
		t.Spec = *serviceSpec.Spec

	case *appsv1alpha1.NemoEvaluator:
		serviceSpec, ok := any(spec).(appsv1alpha1.NemoServiceEvaluatorSpec)
		if !ok {
			return fmt.Errorf("service spec is not of type NemoServiceEvaluatorSpec")
		}
		t.Spec = *serviceSpec.Spec

	case *appsv1alpha1.NemoGuardrail:
		serviceSpec, ok := any(spec).(appsv1alpha1.NemoServiceGuardrailsSpec)
		if !ok {
			return fmt.Errorf("service spec is not of type NemoServiceGuardrailSpec")
		}
		t.Spec = *serviceSpec.Spec

	case *appsv1alpha1.NemoEntitystore:
		serviceSpec, ok := any(spec).(appsv1alpha1.NemoServiceEntitystoreSpec)
		if !ok {
			return fmt.Errorf("service spec is not of type NemoServiceEntitystoreSpec")
		}
		t.Spec = *serviceSpec.Spec

	default:
		return fmt.Errorf("unsupported object type: %T", obj)
	}
	return nil
}

// Generic function for reconciling any Nemo service
func reconcileGenericService[T any](
	ctx context.Context,
	r *NemoServiceReconciler,
	nemoService *appsv1alpha1.NemoService,
	service NemoServiceSpec[T],
	obj client.Object,
) error {
	logger := log.FromContext(ctx)

	if service.Enabled == nil || !*service.Enabled {
		return nil
	}

	// Set metadata
	obj.SetName(service.Name)
	obj.SetNamespace(nemoService.Namespace)

	// Set service spec
	err := SetObjectSpec(obj, service)
	if err != nil {
		logger.Error(err, "Failed to set object spec")
		return err
	}

	// Set controller reference
	if err := controllerutil.SetControllerReference(nemoService, obj, r.Scheme); err != nil {
		return err
	}

	// Fetch current state
	current := obj.DeepCopyObject().(client.Object)
	err = r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: nemoService.Namespace}, current)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if errors.IsNotFound(err) {
		logger.Info("Creating new service", "name", service.Name)
		return r.Create(ctx, obj)
	}

	// Preserve resource version for update
	obj.SetResourceVersion(current.GetResourceVersion())

	logger.Info("Updating existing service", "name", service.Name)
	return r.Update(ctx, obj)
}

// Helper function to extract service spec
func getServiceSpec(nemoService *appsv1alpha1.NemoService, serviceType ServiceType) NemoServiceSpec[any] {
	switch serviceType {
	case Customizer:
		return NemoServiceSpec[any]{nemoService.Spec.Customizer.Enabled, nemoService.Spec.Customizer.Name, nemoService.Spec.Customizer.Spec}
	case Datastore:
		return NemoServiceSpec[any]{nemoService.Spec.Datastore.Enabled, nemoService.Spec.Datastore.Name, nemoService.Spec.Datastore.Spec}
	case Evaluator:
		return NemoServiceSpec[any]{nemoService.Spec.Evaluator.Enabled, nemoService.Spec.Evaluator.Name, nemoService.Spec.Evaluator.Spec}
	case Guardrails:
		return NemoServiceSpec[any]{nemoService.Spec.Guardrails.Enabled, nemoService.Spec.Guardrails.Name, nemoService.Spec.Guardrails.Spec}
	case Entitystore:
		return NemoServiceSpec[any]{nemoService.Spec.Entitystore.Enabled, nemoService.Spec.Entitystore.Name, nemoService.Spec.Entitystore.Spec}
	default:
		return NemoServiceSpec[any]{nil, "", nil}
	}
}

func (r *NemoServiceReconciler) cleanupDisabledNemoMicroservices(ctx context.Context, nemoService *appsv1alpha1.NemoService, enabledServices map[string]bool) error {
	logger := log.FromContext(ctx)

	var allErrors []error

	// Define the list of Nemo microservice types to check for cleanup
	serviceLists := map[ServiceType]client.ObjectList{
		Customizer:  &appsv1alpha1.NemoCustomizerList{},
		Datastore:   &appsv1alpha1.NemoDatastoreList{},
		Evaluator:   &appsv1alpha1.NemoEvaluatorList{},
		Guardrails:  &appsv1alpha1.NemoGuardrailList{},
		Entitystore: &appsv1alpha1.NemoEntitystoreList{},
	}

	// Iterate over each Nemo microservice type and clean up disabled instances
	for serviceType, serviceList := range serviceLists {
		if err := r.List(ctx, serviceList, client.InNamespace(nemoService.Namespace)); err != nil {
			logger.Error(err, "Failed to list services", "type", serviceType)
			allErrors = append(allErrors, fmt.Errorf("failed to list %s services: %w", serviceType, err))
			continue
		}

		services := getObjectListItems(serviceList)
		for _, svc := range services {
			owned := false
			for _, ownerRef := range svc.GetOwnerReferences() {
				if ownerRef.Kind == "NemoService" && ownerRef.UID == nemoService.UID {
					owned = true
					break
				}
			}

			if !owned {
				continue
			}

			if enabled, exists := enabledServices[svc.GetName()]; !exists || !enabled {
				logger.Info("Deleting disabled Nemo microservice", "type", serviceType, "name", svc.GetName())
				if err := r.Delete(ctx, svc); err != nil {
					logger.Error(err, "Unable to delete disabled Nemo microservice", "type", serviceType, "name", svc.GetName())
					allErrors = append(allErrors, fmt.Errorf("failed to delete %s service %s: %w", serviceType, svc.GetName(), err))
				}
			}
		}
	}

	if len(allErrors) > 0 {
		return fmt.Errorf("errors during cleanup: %v", allErrors)
	}

	return nil
}

func getObjectListItems(list client.ObjectList) []client.Object {
	switch t := list.(type) {
	case *appsv1alpha1.NemoCustomizerList:
		objects := make([]client.Object, len(t.Items))
		for i := range t.Items {
			objects[i] = &t.Items[i]
		}
		return objects
	case *appsv1alpha1.NemoDatastoreList:
		objects := make([]client.Object, len(t.Items))
		for i := range t.Items {
			objects[i] = &t.Items[i]
		}
		return objects
	case *appsv1alpha1.NemoEvaluatorList:
		objects := make([]client.Object, len(t.Items))
		for i := range t.Items {
			objects[i] = &t.Items[i]
		}
		return objects
	case *appsv1alpha1.NemoGuardrailList:
		objects := make([]client.Object, len(t.Items))
		for i := range t.Items {
			objects[i] = &t.Items[i]
		}
		return objects
	case *appsv1alpha1.NemoEntitystoreList:
		objects := make([]client.Object, len(t.Items))
		for i := range t.Items {
			objects[i] = &t.Items[i]
		}
		return objects
	default:
		return nil
	}
}

func (r *NemoServiceReconciler) updateStatus(ctx context.Context, nemoService *appsv1alpha1.NemoService, enabledServices map[string]bool) error {
	logger := log.FromContext(ctx)

	// Default overall state to "NotReady"
	overallState := appsv1alpha1.NemoServiceStatusNotReady
	serviceStates := make(map[string]string)
	allServicesReady := true

	foundServices := make(map[string]bool)
	serviceLists := map[ServiceType]client.ObjectList{
		Customizer:  &appsv1alpha1.NemoCustomizerList{},
		Datastore:   &appsv1alpha1.NemoDatastoreList{},
		Evaluator:   &appsv1alpha1.NemoEvaluatorList{},
		Guardrails:  &appsv1alpha1.NemoGuardrailList{},
		Entitystore: &appsv1alpha1.NemoEntitystoreList{},
	}

	for serviceType, serviceList := range serviceLists {
		if err := r.List(ctx, serviceList, client.InNamespace(nemoService.Namespace)); err != nil {
			logger.Error(err, "Failed to list services", "type", serviceType)
			continue
		}

		services := getObjectListItems(serviceList)
		for _, svc := range services {
			owned := false
			for _, ownerRef := range svc.GetOwnerReferences() {
				if ownerRef.Kind == "NemoService" && ownerRef.UID == nemoService.UID {
					owned = true
					break
				}
			}

			if !owned {
				continue
			}

			serviceName := svc.GetName()
			if enabled, exists := enabledServices[serviceName]; !exists || !enabled {
				continue
			}

			foundServices[serviceName] = true
			state := getServiceState(svc)
			serviceStates[serviceName] = state

			switch state {
			case appsv1alpha1.NemoServiceStatusFailed:
				// If any service has failed, set overall state to "Failed"
				overallState = appsv1alpha1.NemoServiceStatusFailed
				allServicesReady = false
			case appsv1alpha1.NemoServiceStatusNotReady, appsv1alpha1.NemoServiceStatusPending:
				// If any service is not ready or pending, set overall readiness to false
				allServicesReady = false
			}
		}
	}

	// Check for missing services
	for serviceName := range enabledServices {
		if !foundServices[serviceName] {
			// A required service is missing, mark as "NotReady"
			allServicesReady = false
			serviceStates[serviceName] = appsv1alpha1.NemoServiceStatusNotReady
		}
	}

	// If all services are ready and no failures were detected, set the overall state to "Ready"
	if allServicesReady && overallState != appsv1alpha1.NemoServiceStatusFailed {
		overallState = appsv1alpha1.NemoServiceStatusReady
	}

	// Update the NemoService status
	nemoService.Status.State = overallState
	nemoService.Status.States = serviceStates

	r.GetEventRecorder().Eventf(nemoService, corev1.EventTypeNormal, overallState,
		"NemoService %s status %s, service states %v", nemoService.Name, overallState, serviceStates)

	// Update the overall status
	if err := r.Status().Update(ctx, nemoService); err != nil {
		logger.Error(err, "Failed to update NemoService status")
		return err
	}

	return nil
}

func getServiceState(obj client.Object) string {
	switch svc := obj.(type) {
	case *appsv1alpha1.NemoCustomizer:
		return svc.Status.State
	case *appsv1alpha1.NemoDatastore:
		return svc.Status.State
	case *appsv1alpha1.NemoEvaluator:
		return svc.Status.State
	case *appsv1alpha1.NemoGuardrail:
		return svc.Status.State
	case *appsv1alpha1.NemoEntitystore:
		return svc.Status.State
	default:
		return appsv1alpha1.NemoServiceStatusUnknown
	}
}

// GetEventRecorder returns the event recorder
func (r *NemoServiceReconciler) GetEventRecorder() record.EventRecorder {
	return r.recorder
}

// SetupWithManager sets up the controller with the Manager.
func (r *NemoServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor("nemoservice-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.NemoService{}).
		Owns(&appsv1alpha1.NemoCustomizer{}).
		Owns(&appsv1alpha1.NemoDatastore{}).
		Owns(&appsv1alpha1.NemoEvaluator{}).
		Owns(&appsv1alpha1.NemoGuardrail{}).
		Owns(&appsv1alpha1.NemoEntitystore{}).
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Type assert to NemoService
				if oldNemoService, ok := e.ObjectOld.(*appsv1alpha1.NemoService); ok {
					newNemoService := e.ObjectNew.(*appsv1alpha1.NemoService)

					// Handle case where object is marked for deletion
					if !newNemoService.ObjectMeta.DeletionTimestamp.IsZero() {
						return true
					}

					// Handle only spec updates
					return !reflect.DeepEqual(oldNemoService.Spec, newNemoService.Spec)
				}
				// For other types we watch, reconcile them
				return true
			},
		}).
		Complete(r)
}

func (r *NemoServiceReconciler) refreshMetrics(ctx context.Context) {
	logger := log.FromContext(ctx)
	// List all nodes
	nemoServiceList := &appsv1alpha1.NemoServiceList{}
	err := r.Client.List(ctx, nemoServiceList, &client.ListOptions{})
	if err != nil {
		logger.Error(err, "unable to list nim pipelines in the cluster")
		return
	}
	refresNemoServiceMetrics(nemoServiceList)
}
