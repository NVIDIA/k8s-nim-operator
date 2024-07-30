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

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	platform "github.com/NVIDIA/k8s-nim-operator/internal/controller/platform"
	"github.com/NVIDIA/k8s-nim-operator/internal/render"
	"github.com/NVIDIA/k8s-nim-operator/internal/shared"
	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// NIMCacheFinalizer is the finalizer annotation
const NIMCacheFinalizer = "finalizer.nimcache.apps.nvidia.com"

// NIMCacheReconciler reconciles a NIMCache object
type NIMCacheReconciler struct {
	client.Client
	scheme   *runtime.Scheme
	log      logr.Logger
	Platform platform.Platform
	updater  conditions.Updater
}

// Ensure NIMCacheReconciler implements the Reconciler interface
var _ shared.Reconciler = &NIMCacheReconciler{}

// NewNIMCacheReconciler creates a new reconciler for NIMCache with the given platform
func NewNIMCacheReconciler(client client.Client, scheme *runtime.Scheme, log logr.Logger, platform platform.Platform) *NIMCacheReconciler {
	return &NIMCacheReconciler{
		Client:   client,
		scheme:   scheme,
		log:      log,
		Platform: platform,
	}
}

// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimcaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimcaches/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimcaches/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",resources=pods/log,verbs=get
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;create;delete
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NIMCache object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *NIMCacheReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the NIMCache instance
	nimCache := &appsv1alpha1.NIMCache{}
	if err := r.Get(ctx, req.NamespacedName, nimCache); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "unable to fetch NIMCache", "name", req.NamespacedName)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling", "NIMCache", nimCache.Name)

	// Check if the instance is marked for deletion
	if nimCache.ObjectMeta.DeletionTimestamp.IsZero() {
		// Add finalizer if not present
		if !controllerutil.ContainsFinalizer(nimCache, NIMCacheFinalizer) {
			controllerutil.AddFinalizer(nimCache, NIMCacheFinalizer)
			if err := r.Update(ctx, nimCache); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The instance is being deleted
		if controllerutil.ContainsFinalizer(nimCache, NIMCacheFinalizer) {
			// Perform platform specific cleanup of resources
			if err := r.Platform.Delete(ctx, r, nimCache); err != nil {
				return ctrl.Result{}, err
			}

			// Remove finalizer to allow for deletion
			controllerutil.RemoveFinalizer(nimCache, NIMCacheFinalizer)
			if err := r.Update(ctx, nimCache); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	// Handle platform-specific reconciliation
	if result, err := r.Platform.Sync(ctx, r, nimCache); err != nil {
		logger.Error(err, "error reconciling NIMCache", "name", nimCache.Name)
		return result, err
	}

	return ctrl.Result{}, nil
}

// GetScheme returns the scheme of the reconciler
func (r *NIMCacheReconciler) GetScheme() *runtime.Scheme {
	return r.scheme
}

// GetLogger returns the logger of the reconciler
func (r *NIMCacheReconciler) GetLogger() logr.Logger {
	return r.log
}

// GetClient returns the client instance
func (r *NIMCacheReconciler) GetClient() client.Client {
	return r.Client
}

// GetUpdater returns the conditions updater instance
func (r *NIMCacheReconciler) GetUpdater() conditions.Updater {
	return r.updater
}

// GetRenderer returns the renderer instance
func (r *NIMCacheReconciler) GetRenderer() render.Renderer {
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NIMCacheReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.NIMCache{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Complete(r)
}
