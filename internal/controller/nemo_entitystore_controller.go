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

	"github.com/go-logr/logr"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
	"github.com/NVIDIA/k8s-nim-operator/internal/render"
	"github.com/NVIDIA/k8s-nim-operator/internal/shared"
	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

// NemoEntitystoreFinalizer is the finalizer annotation.
const NemoEntitystoreFinalizer = "finalizer.NemoEntitystore.apps.nvidia.com"

// NemoEntitystoreReconciler reconciles a NemoEntitystore object.
type NemoEntitystoreReconciler struct {
	client.Client
	scheme           *runtime.Scheme
	log              logr.Logger
	updater          conditions.Updater
	discoveryClient  discovery.DiscoveryInterface
	renderer         render.Renderer
	Config           *rest.Config
	recorder         record.EventRecorder
	orchestratorType k8sutil.OrchestratorType
}

// Ensure NemoEntitystoreReconciler implements the Reconciler interface.
var _ shared.Reconciler = &NemoEntitystoreReconciler{}

// NewNemoEntitystoreReconciler creates a new reconciler for NemoEntitystore with the given platform.
func NewNemoEntitystoreReconciler(client client.Client, scheme *runtime.Scheme, updater conditions.Updater, discoveryClient discovery.DiscoveryInterface, renderer render.Renderer, log logr.Logger) *NemoEntitystoreReconciler {
	return &NemoEntitystoreReconciler{
		Client:          client,
		scheme:          scheme,
		updater:         updater,
		discoveryClient: discoveryClient,
		renderer:        renderer,
		log:             log,
	}
}

// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemoentitystores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemoentitystores/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemoentitystores/finalizers,verbs=update
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
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalars,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NemoEntitystore object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *NemoEntitystoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	defer r.refreshMetrics(ctx)

	// Fetch the NemoEntitystore instance
	NemoEntitystore := &appsv1alpha1.NemoEntitystore{}
	if err := r.Get(ctx, req.NamespacedName, NemoEntitystore); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "unable to fetch NemoEntitystore", "name", req.NamespacedName)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling", "NemoEntitystore", NemoEntitystore.Name)

	// Check if the instance is marked for deletion
	if NemoEntitystore.DeletionTimestamp.IsZero() {
		// Add finalizer if not present
		if !controllerutil.ContainsFinalizer(NemoEntitystore, NemoEntitystoreFinalizer) {
			controllerutil.AddFinalizer(NemoEntitystore, NemoEntitystoreFinalizer)
			if err := r.Update(ctx, NemoEntitystore); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The instance is being deleted
		if controllerutil.ContainsFinalizer(NemoEntitystore, NemoEntitystoreFinalizer) {
			// Perform platform specific cleanup of resources
			if err := r.cleanupNemoEntitystore(ctx, NemoEntitystore); err != nil {
				r.GetEventRecorder().Eventf(NemoEntitystore, corev1.EventTypeNormal, "Delete",
					"NemoEntitystore %s is being deleted", NemoEntitystore.Name)
				return ctrl.Result{}, err
			}
			// Remove finalizer to allow for deletion
			controllerutil.RemoveFinalizer(NemoEntitystore, NemoEntitystoreFinalizer)
			if err := r.Update(ctx, NemoEntitystore); err != nil {
				r.GetEventRecorder().Eventf(NemoEntitystore, corev1.EventTypeNormal, "Delete",
					"NemoEntitystore %s finalizer removed", NemoEntitystore.Name)
				return ctrl.Result{}, err
			}
		}
		// return as the cr is being deleted and GC will cleanup owned objects
		return ctrl.Result{}, nil
	}

	// Fetch container orchestrator type
	_, err := r.GetOrchestratorType(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to get container orchestrator type, %v", err)
	}

	// Handle platform-specific reconciliation
	if result, err := r.reconcileNemoEntitystore(ctx, NemoEntitystore); err != nil {
		logger.Error(err, "error reconciling NemoEntitystore", "name", NemoEntitystore.Name)
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *NemoEntitystoreReconciler) cleanupNemoEntitystore(ctx context.Context, nemoEntityStore *appsv1alpha1.NemoEntitystore) error {
	// All dependent (owned) objects will be automatically garbage collected.
	// TODO: Handle any custom cleanup logic for NEMO entitystore.
	return nil
}

// GetScheme returns the scheme of the reconciler.
func (r *NemoEntitystoreReconciler) GetScheme() *runtime.Scheme {
	return r.scheme
}

// GetLogger returns the logger of the reconciler.
func (r *NemoEntitystoreReconciler) GetLogger() logr.Logger {
	return r.log
}

// GetClient returns the client instance.
func (r *NemoEntitystoreReconciler) GetClient() client.Client {
	return r.Client
}

// GetConfig returns the rest config.
func (r *NemoEntitystoreReconciler) GetConfig() *rest.Config {
	return r.Config
}

// GetUpdater returns the conditions updater instance.
func (r *NemoEntitystoreReconciler) GetUpdater() conditions.Updater {
	return r.updater
}

// GetDiscoveryClient returns the discovery client instance.
func (r *NemoEntitystoreReconciler) GetDiscoveryClient() discovery.DiscoveryInterface {
	return nil
}

// GetRenderer returns the renderer instance.
func (r *NemoEntitystoreReconciler) GetRenderer() render.Renderer {
	return r.renderer
}

// GetEventRecorder returns the event recorder.
func (r *NemoEntitystoreReconciler) GetEventRecorder() record.EventRecorder {
	return r.recorder
}

// GetOrchestratorType returns the container platform type.
func (r *NemoEntitystoreReconciler) GetOrchestratorType(ctx context.Context) (k8sutil.OrchestratorType, error) {
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
func (r *NemoEntitystoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor("nemo-entitystore-service-controller")
	bd := ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.NemoEntitystore{}).
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
				// Type assert to NemoEntitystore
				if oldNemoEntitystore, ok := e.ObjectOld.(*appsv1alpha1.NemoEntitystore); ok {
					newNemoEntitystore, ok := e.ObjectNew.(*appsv1alpha1.NemoEntitystore)
					if ok {
						// Handle case where object is marked for deletion
						if !newNemoEntitystore.DeletionTimestamp.IsZero() {
							return true
						}

						// Handle only spec updates
						return !reflect.DeepEqual(oldNemoEntitystore.Spec, newNemoEntitystore.Spec)
					}
				}
				// For other types we watch, reconcile them
				return true
			},
		})

	bd, err := k8sutil.ControllerOwnsIfCRDExists(
		r.discoveryClient,
		bd,
		gatewayv1.SchemeGroupVersion.WithResource("httproutes"),
		&gatewayv1.HTTPRoute{},
	)
	if err != nil {
		return err
	}

	return bd.Complete(r)
}

func (r *NemoEntitystoreReconciler) refreshMetrics(ctx context.Context) {
	logger := log.FromContext(ctx)
	// List all instances
	NemoEntitystoreList := &appsv1alpha1.NemoEntitystoreList{}
	err := r.List(ctx, NemoEntitystoreList, &client.ListOptions{})
	if err != nil {
		logger.Error(err, "unable to list NemoEntitystores in the cluster")
		return
	}
	refreshNemoEntitystoreMetrics(NemoEntitystoreList)
}

func (r *NemoEntitystoreReconciler) reconcileNemoEntitystore(ctx context.Context, nemoEntitystore *appsv1alpha1.NemoEntitystore) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var err error
	defer func() {
		if err != nil {
			r.GetEventRecorder().Eventf(nemoEntitystore, corev1.EventTypeWarning, conditions.Failed,
				"NemoEntitystore %s failed, msg: %s", nemoEntitystore.Name, err.Error())
		}
	}()

	// Generate annotation for the current operator-version and apply to all resources
	// Get generic name for all resources
	namespacedName := types.NamespacedName{Name: nemoEntitystore.GetName(), Namespace: nemoEntitystore.GetNamespace()}

	renderer := r.GetRenderer()

	// Sync serviceaccount
	err = r.renderAndSyncResource(ctx, nemoEntitystore, &renderer, &corev1.ServiceAccount{}, func() (client.Object, error) {
		return renderer.ServiceAccount(nemoEntitystore.GetServiceAccountParams())
	}, "serviceaccount", conditions.ReasonServiceAccountFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync role
	err = r.renderAndSyncResource(ctx, nemoEntitystore, &renderer, &rbacv1.Role{}, func() (client.Object, error) {
		return renderer.Role(nemoEntitystore.GetRoleParams())
	}, "role", conditions.ReasonRoleFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync rolebinding
	err = r.renderAndSyncResource(ctx, nemoEntitystore, &renderer, &rbacv1.RoleBinding{}, func() (client.Object, error) {
		return renderer.RoleBinding(nemoEntitystore.GetRoleBindingParams())
	}, "rolebinding", conditions.ReasonRoleBindingFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync service
	err = r.renderAndSyncResource(ctx, nemoEntitystore, &renderer, &corev1.Service{}, func() (client.Object, error) {
		return renderer.Service(nemoEntitystore.GetServiceParams())
	}, "service", conditions.ReasonServiceFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync ingress
	if nemoEntitystore.IsIngressEnabled() {
		err = r.renderAndSyncResource(ctx, nemoEntitystore, &renderer, &networkingv1.Ingress{}, func() (client.Object, error) {
			return renderer.Ingress(nemoEntitystore.GetIngressParams())
		}, "ingress", conditions.ReasonIngressFailed)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		err = k8sutil.CleanupResource(ctx, r.GetClient(), &networkingv1.Ingress{}, namespacedName)
		if err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	// Sync HTTPRoute
	if nemoEntitystore.IsHTTPRouteEnabled() {
		err = r.renderAndSyncResource(ctx, nemoEntitystore, &renderer, &gatewayv1.HTTPRoute{}, func() (client.Object, error) {
			return renderer.HTTPRoute(nemoEntitystore.GetHTTPRouteParams())
		}, "httproute", conditions.ReasonHTTPRouteFailed)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		err = k8sutil.CleanupResource(ctx, r.GetClient(), &gatewayv1.HTTPRoute{}, namespacedName)
		if err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	// Sync HPA
	if nemoEntitystore.IsAutoScalingEnabled() {
		err = r.renderAndSyncResource(ctx, nemoEntitystore, &renderer, &autoscalingv2.HorizontalPodAutoscaler{}, func() (client.Object, error) {
			return renderer.HPA(nemoEntitystore.GetHPAParams())
		}, "hpa", conditions.ReasonHPAFailed)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		// If autoscaling is disabled, ensure the HPA is deleted
		err = k8sutil.CleanupResource(ctx, r.GetClient(), &autoscalingv2.HorizontalPodAutoscaler{}, namespacedName)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Sync Service Monitor
	if nemoEntitystore.IsServiceMonitorEnabled() {
		err = r.renderAndSyncResource(ctx, nemoEntitystore, &renderer, &monitoringv1.ServiceMonitor{}, func() (client.Object, error) {
			return renderer.ServiceMonitor(nemoEntitystore.GetServiceMonitorParams())
		}, "servicemonitor", conditions.ReasonServiceMonitorFailed)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	deploymentParams := nemoEntitystore.GetDeploymentParams()

	// Setup volume mounts with model store
	deploymentParams.Volumes = nemoEntitystore.GetVolumes()
	deploymentParams.VolumeMounts = nemoEntitystore.GetVolumeMounts()

	logger.Info("Reconciling", "volumes", nemoEntitystore.GetVolumes())

	// Sync deployment
	err = r.renderAndSyncResource(ctx, nemoEntitystore, &renderer, &appsv1.Deployment{}, func() (client.Object, error) {
		return renderer.Deployment(deploymentParams)
	}, "deployment", conditions.ReasonDeploymentFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Wait for deployment
	msg, ready, err := k8sutil.IsDeploymentReady(ctx, r.GetClient(), &namespacedName)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !ready {
		// Update status as NotReady
		err = r.updater.SetConditionsNotReady(ctx, nemoEntitystore, conditions.NotReady, msg)
		r.GetEventRecorder().Eventf(nemoEntitystore, corev1.EventTypeNormal, conditions.NotReady,
			"NemoEntitystore %s not ready yet, msg: %s", nemoEntitystore.Name, msg)
	} else {
		// Update status as ready
		err = r.updater.SetConditionsReady(ctx, nemoEntitystore, conditions.Ready, msg)
		r.GetEventRecorder().Eventf(nemoEntitystore, corev1.EventTypeNormal, conditions.Ready,
			"NemoEntitystore %s ready, msg: %s", nemoEntitystore.Name, msg)
	}

	if err != nil {
		logger.Error(err, "Unable to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NemoEntitystoreReconciler) renderAndSyncResource(ctx context.Context, nemoEntitystore *appsv1alpha1.NemoEntitystore, renderer *render.Renderer, obj client.Object, renderFunc func() (client.Object, error), conditionType string, reason string) error {
	logger := log.FromContext(ctx)

	resource, err := renderFunc()
	if err != nil {
		logger.Error(err, "failed to render", conditionType, nemoEntitystore.GetName(), nemoEntitystore.GetNamespace())
		statusError := r.updater.SetConditionsFailed(ctx, nemoEntitystore, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NemoEntitystore", nemoEntitystore.GetName())
		}
		return err
	}

	// Check if the resource is nil
	if resource == nil {
		logger.V(2).Info("rendered nil resource")
		return nil
	}

	metaAccessor, ok := resource.(metav1.Object)
	if !ok {
		logger.V(2).Info("rendered un-initialized resource")
		return nil
	}

	if metaAccessor == nil || metaAccessor.GetName() == "" || metaAccessor.GetNamespace() == "" {
		logger.V(2).Info("rendered un-initialized resource")
		return nil
	}

	namespacedName := types.NamespacedName{Name: nemoEntitystore.GetName(), Namespace: nemoEntitystore.GetNamespace()}
	err = r.Get(ctx, namespacedName, obj)
	if err != nil && !errors.IsNotFound(err) {
		logger.Error(err, fmt.Sprintf("Error is not NotFound for %s: %v", obj.GetObjectKind(), err))
		return err
	}

	// If we found the object and autoscaling is enabled on the EntityStore,
	// copy the current replicas from the existing object into the desired (resource),
	// so we don't fight the HPA (or external scaler) on each reconcile.
	if err == nil && nemoEntitystore.IsAutoScalingEnabled() {
		if curr, ok := obj.(*appsv1.Deployment); ok {
			if desired, ok := resource.(*appsv1.Deployment); ok && curr.Spec.Replicas != nil {
				replicas := *curr.Spec.Replicas
				desired.Spec.Replicas = &replicas
			}
		}
	}

	// Don't do anything if CR is unchanged.
	if err == nil && !utils.IsParentSpecChanged(obj, utils.DeepHashObject(nemoEntitystore.Spec)) {
		return nil
	}

	if err = controllerutil.SetControllerReference(nemoEntitystore, resource, r.GetScheme()); err != nil {
		logger.Error(err, "failed to set owner", conditionType, namespacedName)
		statusError := r.updater.SetConditionsFailed(ctx, nemoEntitystore, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NemoEntitystore", nemoEntitystore.GetName())
		}
		return err
	}

	err = k8sutil.SyncResource(ctx, r.GetClient(), obj, resource)
	if err != nil {
		logger.Error(err, "failed to sync", conditionType, namespacedName)
		statusError := r.updater.SetConditionsFailed(ctx, nemoEntitystore, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NemoEntitystore", nemoEntitystore.GetName())
		}
		return err
	}
	return nil
}
