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

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	platform "github.com/NVIDIA/k8s-nim-operator/internal/controller/platform"
	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
	"github.com/NVIDIA/k8s-nim-operator/internal/render"
	"github.com/NVIDIA/k8s-nim-operator/internal/shared"
)

// NIMServiceFinalizer is the finalizer annotation.
const NIMServiceFinalizer = "finalizer.nimservice.apps.nvidia.com"

// NIMServiceReconciler reconciles a NIMService object.
type NIMServiceReconciler struct {
	client.Client
	scheme           *runtime.Scheme
	log              logr.Logger
	updater          conditions.Updater
	renderer         render.Renderer
	Config           *rest.Config
	Platform         platform.Platform
	orchestratorType k8sutil.OrchestratorType
	recorder         record.EventRecorder
}

// Ensure NIMServiceReconciler implements the Reconciler interface.
var _ shared.Reconciler = &NIMServiceReconciler{}

// NewNIMServiceReconciler creates a new reconciler for NIMService with the given platform.
func NewNIMServiceReconciler(client client.Client, scheme *runtime.Scheme, updater conditions.Updater, renderer render.Renderer, log logr.Logger, platform platform.Platform) *NIMServiceReconciler {
	return &NIMServiceReconciler{
		Client:   client,
		scheme:   scheme,
		updater:  updater,
		renderer: renderer,
		log:      log,
		Platform: platform,
	}
}

// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimservices/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimcaches,verbs=get;list;watch;
// +kubebuilder:rbac:groups=config.openshift.io,resources=clusterversions;proxies,verbs=get;list;watch
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=use,resourceNames=nonroot
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts;pods;pods/eviction;services;services/finalizers;endpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims;configmaps;secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors;prometheusrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scheduling.k8s.io,resources=priorityclasses,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NIMService object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *NIMServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	defer r.refreshMetrics(ctx)

	// Fetch the NIMService instance
	nimService := &appsv1alpha1.NIMService{}
	if err := r.Get(ctx, req.NamespacedName, nimService); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "unable to fetch NIMService", "name", req.NamespacedName)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling", "NIMService", nimService.Name)

	// Check if the instance is marked for deletion
	if nimService.DeletionTimestamp.IsZero() {
		// Add finalizer if not present
		if !controllerutil.ContainsFinalizer(nimService, NIMServiceFinalizer) {
			controllerutil.AddFinalizer(nimService, NIMServiceFinalizer)
			if err := r.Update(ctx, nimService); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The instance is being deleted
		if controllerutil.ContainsFinalizer(nimService, NIMServiceFinalizer) {
			// Perform platform specific cleanup of resources
			if err := r.Platform.Delete(ctx, r, nimService); err != nil {
				r.GetEventRecorder().Eventf(nimService, corev1.EventTypeNormal, "Delete",
					"NIMService %s in deleted", nimService.Name)
				return ctrl.Result{}, err
			}

			// Remove finalizer to allow for deletion
			controllerutil.RemoveFinalizer(nimService, NIMServiceFinalizer)
			if err := r.Update(ctx, nimService); err != nil {
				r.GetEventRecorder().Eventf(nimService, corev1.EventTypeNormal, "Delete",
					"NIMService %s finalizer removed", nimService.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	// Fetch container orchestrator type
	_, err := r.GetOrchestratorType(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to get container orchestrator type, %v", err)
	}

	// Handle platform-specific reconciliation
	result, err := r.Platform.Sync(ctx, r, nimService)
	if err != nil {
		logger.Error(err, "error reconciling NIMService", "name", nimService.Name)
		return result, err
	}

	return result, nil
}

// GetScheme returns the scheme of the reconciler.
func (r *NIMServiceReconciler) GetScheme() *runtime.Scheme {
	return r.scheme
}

// GetLogger returns the logger of the reconciler.
func (r *NIMServiceReconciler) GetLogger() logr.Logger {
	return r.log
}

// GetClient returns the client instance.
func (r *NIMServiceReconciler) GetClient() client.Client {
	return r.Client
}

// GetConfig returns the rest config.
func (r *NIMServiceReconciler) GetConfig() *rest.Config {
	return r.Config
}

// GetUpdater returns the conditions updater instance.
func (r *NIMServiceReconciler) GetUpdater() conditions.Updater {
	return r.updater
}

// GetRenderer returns the renderer instance.
func (r *NIMServiceReconciler) GetRenderer() render.Renderer {
	return r.renderer
}

// GetEventRecorder returns the event recorder.
func (r *NIMServiceReconciler) GetEventRecorder() record.EventRecorder {
	return r.recorder
}

// GetOrchestratorType returns the container platform type.
func (r *NIMServiceReconciler) GetOrchestratorType(ctx context.Context) (k8sutil.OrchestratorType, error) {
	if r.orchestratorType == "" {
		orchestratorType, err := k8sutil.GetOrchestratorType(ctx, r.GetClient())
		if err != nil {
			return k8sutil.Unknown, fmt.Errorf("unable to get container orchestrator type, %v", err)
		}
		r.orchestratorType = orchestratorType
		r.GetLogger().Info("Container orchestrator is successfully set", "type", orchestratorType)
	}
	return r.orchestratorType, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NIMServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor("nimservice-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.NIMService{}).
		Owns(&appsv1.Deployment{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&networkingv1.Ingress{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Type assert to NIMService
				if oldNIMService, ok := e.ObjectOld.(*appsv1alpha1.NIMService); ok {
					newNIMService, ok := e.ObjectNew.(*appsv1alpha1.NIMService)
					if ok {
						// Handle case where object is marked for deletion
						if !newNIMService.ObjectMeta.DeletionTimestamp.IsZero() {
							return true
						}

						// Handle only spec updates
						return !reflect.DeepEqual(oldNIMService.Spec, newNIMService.Spec)
					}
				}
				// For other types we watch, reconcile them
				return true
			},
		}).
		Complete(r)
}

func (r *NIMServiceReconciler) refreshMetrics(ctx context.Context) {
	logger := log.FromContext(ctx)
	// List all nodes
	nimServiceList := &appsv1alpha1.NIMServiceList{}
	err := r.List(ctx, nimServiceList, &client.ListOptions{})
	if err != nil {
		logger.Error(err, "unable to list nimServices in the cluster")
		return
	}
	refreshNIMServiceMetrics(nimServiceList)
}
