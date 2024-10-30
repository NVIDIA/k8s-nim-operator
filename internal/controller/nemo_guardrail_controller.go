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

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	"github.com/NVIDIA/k8s-nim-operator/internal/controller/platform/standalone"
	"github.com/NVIDIA/k8s-nim-operator/internal/render"
	"github.com/NVIDIA/k8s-nim-operator/internal/shared"
	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
	"github.com/go-logr/logr"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// NemoGuardrailFinalizer is the finalizer annotation
const NemoGuardrailFinalizer = "finalizer.NemoGuardrail.apps.nvidia.com"

// NemoGuardrailReconciler reconciles a NemoGuardrail object
type NemoGuardrailReconciler struct {
	client.Client
	scheme   *runtime.Scheme
	log      logr.Logger
	updater  conditions.Updater
	renderer render.Renderer
	Config   *rest.Config
	recorder record.EventRecorder
}

// Ensure NemoGuardrailReconciler implements the Reconciler interface
var _ shared.Reconciler = &NemoGuardrailReconciler{}

// NewNemoGuardrailReconciler creates a new reconciler for NemoGuardrail with the given platform
func NewNemoGuardrailReconciler(client client.Client, scheme *runtime.Scheme, updater conditions.Updater, renderer render.Renderer, log logr.Logger) *NemoGuardrailReconciler {
	return &NemoGuardrailReconciler{
		Client:   client,
		scheme:   scheme,
		updater:  updater,
		renderer: renderer,
		log:      log,
	}
}

// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemoguardrails,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemoguardrails/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemoguardrails/finalizers,verbs=update
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
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalars,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NemoGuardrail object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *NemoGuardrailReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	defer r.refreshMetrics(ctx)

	// Fetch the NemoGuardrail instance
	NemoGuardrail := &appsv1alpha1.NemoGuardrail{}
	if err := r.Get(ctx, req.NamespacedName, NemoGuardrail); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "unable to fetch NemoGuardrail", "name", req.NamespacedName)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling", "NemoGuardrail", NemoGuardrail.Name)

	// Check if the instance is marked for deletion
	if NemoGuardrail.ObjectMeta.DeletionTimestamp.IsZero() {
		// Add finalizer if not present
		if !controllerutil.ContainsFinalizer(NemoGuardrail, NemoGuardrailFinalizer) {
			controllerutil.AddFinalizer(NemoGuardrail, NemoGuardrailFinalizer)
			if err := r.Update(ctx, NemoGuardrail); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The instance is being deleted
		if controllerutil.ContainsFinalizer(NemoGuardrail, NemoGuardrailFinalizer) {
			// Perform platform specific cleanup of resources
			if err := r.cleanupNemoGuardrail(ctx, NemoGuardrail); err != nil {
				r.GetEventRecorder().Eventf(NemoGuardrail, corev1.EventTypeNormal, "Delete",
					"NemoGuardrail %s in deleted", NemoGuardrail.Name)
				return ctrl.Result{}, err
			}

			// Remove finalizer to allow for deletion
			controllerutil.RemoveFinalizer(NemoGuardrail, NemoGuardrailFinalizer)
			if err := r.Update(ctx, NemoGuardrail); err != nil {
				r.GetEventRecorder().Eventf(NemoGuardrail, corev1.EventTypeNormal, "Delete",
					"NemoGuardrail %s finalizer removed", NemoGuardrail.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	// Handle platform-specific reconciliation
	if result, err := r.reconcileNemoGuardrail(ctx, NemoGuardrail); err != nil {
		logger.Error(err, "error reconciling NemoGuardrail", "name", NemoGuardrail.Name)
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *NemoGuardrailReconciler) cleanupNemoGuardrail(ctx context.Context, nimService *appsv1alpha1.NemoGuardrail) error {
	// All dependent (owned) objects will be automatically garbage collected.
	// TODO: Handle any custom cleanup logic for the NIM microservice
	return nil
}

// GetScheme returns the scheme of the reconciler
func (r *NemoGuardrailReconciler) GetScheme() *runtime.Scheme {
	return r.scheme
}

// GetLogger returns the logger of the reconciler
func (r *NemoGuardrailReconciler) GetLogger() logr.Logger {
	return r.log
}

// GetClient returns the client instance
func (r *NemoGuardrailReconciler) GetClient() client.Client {
	return r.Client
}

// GetConfig returns the rest config
func (r *NemoGuardrailReconciler) GetConfig() *rest.Config {
	return r.Config
}

// GetUpdater returns the conditions updater instance
func (r *NemoGuardrailReconciler) GetUpdater() conditions.Updater {
	return r.updater
}

// GetRenderer returns the renderer instance
func (r *NemoGuardrailReconciler) GetRenderer() render.Renderer {
	return r.renderer
}

// GetEventRecorder returns the event recorder
func (r *NemoGuardrailReconciler) GetEventRecorder() record.EventRecorder {
	return r.recorder
}

// SetupWithManager sets up the controller with the Manager.
func (r *NemoGuardrailReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor("nemo-guardrail-service-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.NemoGuardrail{}).
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
				// Type assert to NemoGuardrail
				if oldNemoGuardrail, ok := e.ObjectOld.(*appsv1alpha1.NemoGuardrail); ok {
					newNemoGuardrail := e.ObjectNew.(*appsv1alpha1.NemoGuardrail)

					// Handle case where object is marked for deletion
					if !newNemoGuardrail.ObjectMeta.DeletionTimestamp.IsZero() {
						return true
					}

					// Handle only spec updates
					return !reflect.DeepEqual(oldNemoGuardrail.Spec, newNemoGuardrail.Spec)
				}
				// For other types we watch, reconcile them
				return true
			},
		}).
		Complete(r)
}

func (r *NemoGuardrailReconciler) refreshMetrics(ctx context.Context) {
	logger := log.FromContext(ctx)
	// List all nodes
	NemoGuardrailList := &appsv1alpha1.NemoGuardrailList{}
	err := r.Client.List(ctx, NemoGuardrailList, &client.ListOptions{})
	if err != nil {
		logger.Error(err, "unable to list NemoGuardrails in the cluster")
		return
	}
	//refreshNemoGuardrailMetrics(NemoGuardrailList)
}
func (r *NemoGuardrailReconciler) getDefaultAppReconciler() *standalone.DefaultAppReconciler{
	return standalone.NewDefaultAppReconciler(
		r.GetClient(),
		r.GetScheme(),
		r.GetUpdater(),
		r.GetRenderer(),
		r.GetLogger()
	)
}

func (r *NemoGuardrailReconciler) reconcileNemoGuardrail(ctx context.Context, NemoGuardrail *appsv1alpha1.NemoGuardrail) (ctrl.Result, error) {
	defaultAppReconciler := r.getDefaultAppReconciler()
	return defaultAppReconciler.ReconcileDefaultApp(ctx, NemoGuardrail, NemoGuardrail)
}
