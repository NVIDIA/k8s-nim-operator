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
	"k8s.io/utils/ptr"
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

// NemoDatastoreFinalizer is the finalizer annotation.
const NemoDatastoreFinalizer = "finalizer.nemodatastore.apps.nvidia.com"

// NemoDatastoreReconciler reconciles a NemoDatastore object.
type NemoDatastoreReconciler struct {
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

// Ensure NemoDatastoreReconciler implements the Reconciler interface.
var _ shared.Reconciler = &NemoDatastoreReconciler{}

// NewNemoDatastoreReconciler creates a new reconciler for NemoDatastore with the given platform.
func NewNemoDatastoreReconciler(client client.Client, scheme *runtime.Scheme, updater conditions.Updater, discoveryClient discovery.DiscoveryInterface, renderer render.Renderer, log logr.Logger) *NemoDatastoreReconciler {
	return &NemoDatastoreReconciler{
		Client:          client,
		scheme:          scheme,
		updater:         updater,
		discoveryClient: discoveryClient,
		renderer:        renderer,
		log:             log,
	}
}

// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemodatastores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemodatastores/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemodatastores/finalizers,verbs=update
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
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NemoDatastore object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *NemoDatastoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	defer r.refreshMetrics(ctx)

	// Fetch the NemoDatastore instance
	NemoDatastore := &appsv1alpha1.NemoDatastore{}
	if err := r.Get(ctx, req.NamespacedName, NemoDatastore); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "unable to fetch NemoDatastore", "name", req.NamespacedName)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling", "NemoDatastore", NemoDatastore.Name)

	// Check if the instance is marked for deletion
	if NemoDatastore.DeletionTimestamp.IsZero() {
		// Add finalizer if not present
		if !controllerutil.ContainsFinalizer(NemoDatastore, NemoDatastoreFinalizer) {
			controllerutil.AddFinalizer(NemoDatastore, NemoDatastoreFinalizer)
			if err := r.Update(ctx, NemoDatastore); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The instance is being deleted
		if controllerutil.ContainsFinalizer(NemoDatastore, NemoDatastoreFinalizer) {
			// Perform platform specific cleanup of resources
			if err := r.cleanupNemoDatastore(ctx, NemoDatastore); err != nil {
				r.GetEventRecorder().Eventf(NemoDatastore, corev1.EventTypeNormal, "Delete",
					"NemoDatastore %s is being deleted", NemoDatastore.Name)
				return ctrl.Result{}, err
			}
			// Remove finalizer to allow for deletion
			controllerutil.RemoveFinalizer(NemoDatastore, NemoDatastoreFinalizer)
			if err := r.Update(ctx, NemoDatastore); err != nil {
				r.GetEventRecorder().Eventf(NemoDatastore, corev1.EventTypeNormal, "Delete",
					"NemoDatastore %s finalizer removed", NemoDatastore.Name)
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
	if result, err := r.reconcileNemoDatastore(ctx, NemoDatastore); err != nil {
		logger.Error(err, "error reconciling NemoDatastore", "name", NemoDatastore.Name)
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *NemoDatastoreReconciler) cleanupNemoDatastore(ctx context.Context, nimService *appsv1alpha1.NemoDatastore) error {
	// All dependent (owned) objects will be automatically garbage collected.
	// TODO: Handle any custom cleanup logic for the NIM microservice
	return nil
}

// GetScheme returns the scheme of the reconciler.
func (r *NemoDatastoreReconciler) GetScheme() *runtime.Scheme {
	return r.scheme
}

// GetLogger returns the logger of the reconciler.
func (r *NemoDatastoreReconciler) GetLogger() logr.Logger {
	return r.log
}

// GetClient returns the client instance.
func (r *NemoDatastoreReconciler) GetClient() client.Client {
	return r.Client
}

// GetConfig returns the rest config.
func (r *NemoDatastoreReconciler) GetConfig() *rest.Config {
	return r.Config
}

// GetUpdater returns the conditions updater instance.
func (r *NemoDatastoreReconciler) GetUpdater() conditions.Updater {
	return r.updater
}

// GetDiscoveryClient returns the discovery client instance.
func (r *NemoDatastoreReconciler) GetDiscoveryClient() discovery.DiscoveryInterface {
	return nil
}

// GetRenderer returns the renderer instance.
func (r *NemoDatastoreReconciler) GetRenderer() render.Renderer {
	return r.renderer
}

// GetEventRecorder returns the event recorder.
func (r *NemoDatastoreReconciler) GetEventRecorder() record.EventRecorder {
	return r.recorder
}

// GetOrchestratorType returns the container platform type.
func (r *NemoDatastoreReconciler) GetOrchestratorType(ctx context.Context) (k8sutil.OrchestratorType, error) {
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
func (r *NemoDatastoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor("nemo-datastore-service-controller")
	bd := ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.NemoDatastore{}).
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
				// Type assert to NemoDatastore
				if oldNemoDatastore, ok := e.ObjectOld.(*appsv1alpha1.NemoDatastore); ok {
					newNemoDatastore, ok := e.ObjectNew.(*appsv1alpha1.NemoDatastore)
					if ok {

						// Handle case where object is marked for deletion
						if !newNemoDatastore.DeletionTimestamp.IsZero() {
							return true
						}

						// Handle only spec updates
						return !reflect.DeepEqual(oldNemoDatastore.Spec, newNemoDatastore.Spec)
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

func (r *NemoDatastoreReconciler) refreshMetrics(ctx context.Context) {
	logger := log.FromContext(ctx)
	// List all datastore instances
	NemoDatastoreList := &appsv1alpha1.NemoDatastoreList{}
	err := r.List(ctx, NemoDatastoreList, &client.ListOptions{})
	if err != nil {
		logger.Error(err, "unable to list NemoDatastores in the cluster")
		return
	}
	refreshNemoDatastoreMetrics(NemoDatastoreList)
}

func (r *NemoDatastoreReconciler) reconcileNemoDatastore(ctx context.Context, nemoDatastore *appsv1alpha1.NemoDatastore) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var err error
	defer func() {
		if err != nil {
			r.GetEventRecorder().Eventf(nemoDatastore, corev1.EventTypeWarning, conditions.Failed,
				"NemoDatastore %s failed, msg: %s", nemoDatastore.Name, err.Error())
		}
	}()

	err = r.reconcilePVC(ctx, nemoDatastore)
	if err != nil {
		logger.Error(err, "reconciliation of pvc failed", "pvc", nemoDatastore.GetPVCName())
		return ctrl.Result{}, err
	}

	// Generate annotation for the current operator-version and apply to all resources
	// Get generic name for all resources
	namespacedName := types.NamespacedName{Name: nemoDatastore.GetName(), Namespace: nemoDatastore.GetNamespace()}

	renderer := r.GetRenderer()

	// Sync serviceaccount
	err = r.renderAndSyncResource(ctx, nemoDatastore, &renderer, &corev1.ServiceAccount{}, func() (client.Object, error) {
		return renderer.ServiceAccount(nemoDatastore.GetServiceAccountParams())
	}, "serviceaccount", conditions.ReasonServiceAccountFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync role
	err = r.renderAndSyncResource(ctx, nemoDatastore, &renderer, &rbacv1.Role{}, func() (client.Object, error) {
		return renderer.Role(nemoDatastore.GetRoleParams())
	}, "role", conditions.ReasonRoleFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync rolebinding
	err = r.renderAndSyncResource(ctx, nemoDatastore, &renderer, &rbacv1.RoleBinding{}, func() (client.Object, error) {
		return renderer.RoleBinding(nemoDatastore.GetRoleBindingParams())
	}, "rolebinding", conditions.ReasonRoleBindingFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync service
	err = r.renderAndSyncResource(ctx, nemoDatastore, &renderer, &corev1.Service{}, func() (client.Object, error) {
		return renderer.Service(nemoDatastore.GetServiceParams())
	}, "service", conditions.ReasonServiceFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync ingress
	if nemoDatastore.IsIngressEnabled() {
		err = r.renderAndSyncResource(ctx, nemoDatastore, &renderer, &networkingv1.Ingress{}, func() (client.Object, error) {
			return renderer.Ingress(nemoDatastore.GetIngressParams())
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
	if nemoDatastore.IsHTTPRouteEnabled() {
		err = r.renderAndSyncResource(ctx, nemoDatastore, &renderer, &gatewayv1.HTTPRoute{}, func() (client.Object, error) {
			return renderer.HTTPRoute(nemoDatastore.GetHTTPRouteParams())
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
	if nemoDatastore.IsAutoScalingEnabled() {
		err = r.renderAndSyncResource(ctx, nemoDatastore, &renderer, &autoscalingv2.HorizontalPodAutoscaler{}, func() (client.Object, error) {
			return renderer.HPA(nemoDatastore.GetHPAParams())
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
	if nemoDatastore.IsServiceMonitorEnabled() {
		err = r.renderAndSyncResource(ctx, nemoDatastore, &renderer, &monitoringv1.ServiceMonitor{}, func() (client.Object, error) {
			return renderer.ServiceMonitor(nemoDatastore.GetServiceMonitorParams())
		}, "servicemonitor", conditions.ReasonServiceMonitorFailed)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	deploymentParams := nemoDatastore.GetDeploymentParams()

	// Setup volume mounts with model store
	deploymentParams.Volumes = nemoDatastore.GetVolumes()
	deploymentParams.VolumeMounts = nemoDatastore.GetVolumeMounts()

	logger.Info("Reconciling", "volumes", nemoDatastore.GetVolumes())

	// Sync deployment
	err = r.renderAndSyncResource(ctx, nemoDatastore, &renderer, &appsv1.Deployment{}, func() (client.Object, error) {
		result, err := renderer.Deployment(deploymentParams)
		if err != nil {
			return nil, err
		}
		initContainers := nemoDatastore.GetInitContainers()
		if len(initContainers) > 0 {
			result.Spec.Template.Spec.InitContainers = initContainers
		}
		fsGroup := ptr.To[int64](1000)
		if nemoDatastore.Spec.GroupID != nil {
			fsGroup = nemoDatastore.Spec.GroupID
		}
		result.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
			FSGroup: fsGroup,
		}
		return result, nil
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
		err = r.updater.SetConditionsNotReady(ctx, nemoDatastore, conditions.NotReady, msg)
		r.GetEventRecorder().Eventf(nemoDatastore, corev1.EventTypeNormal, conditions.NotReady,
			"NemoDatastore %s not ready yet, msg: %s", nemoDatastore.Name, msg)
	} else {
		// Update status as ready
		err = r.updater.SetConditionsReady(ctx, nemoDatastore, conditions.Ready, msg)
		r.GetEventRecorder().Eventf(nemoDatastore, corev1.EventTypeNormal, conditions.Ready,
			"NemoDatastore %s ready, msg: %s", nemoDatastore.Name, msg)
	}

	if err != nil {
		logger.Error(err, "Unable to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NemoDatastoreReconciler) reconcilePVC(ctx context.Context, nemoDatastore *appsv1alpha1.NemoDatastore) error {
	logger := r.GetLogger()
	pvcName := nemoDatastore.GetPVCName()
	pvcNamespacedName := types.NamespacedName{Name: pvcName, Namespace: nemoDatastore.GetNamespace()}
	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, pvcNamespacedName, pvc)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return err
	}

	// If PVC does not exist, create a new one if creation flag is enabled
	if err != nil {
		if nemoDatastore.ShouldCreatePersistentStorage() {
			pvc, err = shared.ConstructPVC(*nemoDatastore.Spec.PVC, metav1.ObjectMeta{Name: pvcName, Namespace: nemoDatastore.GetNamespace()})
			if err != nil {
				logger.Error(err, "Failed to construct pvc", "name", pvcName)
				return err
			}
			if err := controllerutil.SetControllerReference(nemoDatastore, pvc, r.GetScheme()); err != nil {
				return err
			}
			err = r.Create(ctx, pvc)
			if err != nil {
				logger.Error(err, "Failed to create pvc", "name", pvcName)
				return err
			}
			logger.Info("Created PVC for NeMo Datastore", "pvc", pvc.Name)
		} else {
			logger.Error(err, "PVC doesn't exist and auto-creation is not enabled", "name", pvcNamespacedName)
			return err
		}
	}
	return nil
}

func (r *NemoDatastoreReconciler) renderAndSyncResource(ctx context.Context, nemoDatastore *appsv1alpha1.NemoDatastore, renderer *render.Renderer, obj client.Object, renderFunc func() (client.Object, error), conditionType string, reason string) error {
	logger := log.FromContext(ctx)

	namespacedName := types.NamespacedName{Name: nemoDatastore.GetName(), Namespace: nemoDatastore.GetNamespace()}
	getErr := r.Get(ctx, namespacedName, obj)
	if getErr != nil && !errors.IsNotFound(getErr) {
		logger.Error(getErr, fmt.Sprintf("Error is not NotFound for %s: %v", obj.GetObjectKind(), getErr))
		return getErr
	}

	// Track an existing resource
	found := getErr == nil

	// Don't do anything if CR is unchanged.
	if found && !utils.IsParentSpecChanged(obj, utils.DeepHashObject(nemoDatastore.Spec)) {
		return nil
	}

	resource, err := renderFunc()
	if err != nil {
		logger.Error(err, "failed to render", conditionType)
		statusError := r.updater.SetConditionsFailed(ctx, nemoDatastore, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NemoDatastore", nemoDatastore.GetName())
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

	// If we found the object and autoscaling is enabled on the Datastore,
	// copy the current replicas from the existing object into the desired (resource),
	// so we don't fight the HPA (or external scaler) on each reconcile.
	if found && nemoDatastore.IsAutoScalingEnabled() {
		if curr, ok := obj.(*appsv1.Deployment); ok {
			if desired, ok := resource.(*appsv1.Deployment); ok && curr.Spec.Replicas != nil {
				replicas := *curr.Spec.Replicas
				desired.Spec.Replicas = &replicas
			}
		}
	}

	if err = controllerutil.SetControllerReference(nemoDatastore, resource, r.GetScheme()); err != nil {
		logger.Error(err, "failed to set owner", conditionType, namespacedName)
		statusError := r.updater.SetConditionsFailed(ctx, nemoDatastore, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NemoDatastore", nemoDatastore.GetName())
		}
		return err
	}

	err = k8sutil.SyncResource(ctx, r.GetClient(), obj, resource)
	if err != nil {
		logger.Error(err, "failed to sync", conditionType, namespacedName)
		statusError := r.updater.SetConditionsFailed(ctx, nemoDatastore, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NemoDatastore", nemoDatastore.GetName())
		}
		return err
	}
	return nil
}
