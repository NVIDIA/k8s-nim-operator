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
	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
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
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// NemoDatastoreFinalizer is the finalizer annotation
const NemoDatastoreFinalizer = "finalizer.NemoDatastore.apps.nvidia.com"

// NemoDatastoreReconciler reconciles a NemoDatastore object
type NemoDatastoreReconciler struct {
	client.Client
	scheme           *runtime.Scheme
	log              logr.Logger
	updater          conditions.Updater
	renderer         render.Renderer
	Config           *rest.Config
	recorder         record.EventRecorder
	orchestratorType k8sutil.OrchestratorType
}

// Ensure NemoDatastoreReconciler implements the Reconciler interface
var _ shared.Reconciler = &NemoDatastoreReconciler{}

// NewNemoDatastoreReconciler creates a new reconciler for NemoDatastore with the given platform
func NewNemoDatastoreReconciler(client client.Client, scheme *runtime.Scheme, updater conditions.Updater, renderer render.Renderer, log logr.Logger) *NemoDatastoreReconciler {
	return &NemoDatastoreReconciler{
		Client:   client,
		scheme:   scheme,
		updater:  updater,
		renderer: renderer,
		log:      log,
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
	if NemoDatastore.ObjectMeta.DeletionTimestamp.IsZero() {
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
					"NemoDatastore %s in deleted", NemoDatastore.Name)
				return ctrl.Result{}, err
			}

			// Remove finalizer to allow for deletion
			controllerutil.RemoveFinalizer(NemoDatastore, NemoDatastoreFinalizer)
			if err := r.Update(ctx, NemoDatastore); err != nil {
				r.GetEventRecorder().Eventf(NemoDatastore, corev1.EventTypeNormal, "Delete",
					"NemoDatastore %s finalizer removed", NemoDatastore.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	// Fetch container orchestrator type
	_, err := r.GetOrchestratorType()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("Unable to get container orchestrator type, %v", err)
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

// GetScheme returns the scheme of the reconciler
func (r *NemoDatastoreReconciler) GetScheme() *runtime.Scheme {
	return r.scheme
}

// GetLogger returns the logger of the reconciler
func (r *NemoDatastoreReconciler) GetLogger() logr.Logger {
	return r.log
}

// GetClient returns the client instance
func (r *NemoDatastoreReconciler) GetClient() client.Client {
	return r.Client
}

// GetConfig returns the rest config
func (r *NemoDatastoreReconciler) GetConfig() *rest.Config {
	return r.Config
}

// GetUpdater returns the conditions updater instance
func (r *NemoDatastoreReconciler) GetUpdater() conditions.Updater {
	return r.updater
}

// GetRenderer returns the renderer instance
func (r *NemoDatastoreReconciler) GetRenderer() render.Renderer {
	return r.renderer
}

// GetEventRecorder returns the event recorder
func (r *NemoDatastoreReconciler) GetEventRecorder() record.EventRecorder {
	return r.recorder
}

// GetOrchestratorType returns the container platform type
func (r *NemoDatastoreReconciler) GetOrchestratorType() (k8sutil.OrchestratorType, error) {
	if r.orchestratorType == "" {
		orchestratorType, err := k8sutil.GetOrchestratorType(r.GetClient())
		if err != nil {
			return k8sutil.Unknown, fmt.Errorf("Unable to get container orchestrator type, %v", err)
		}
		r.orchestratorType = orchestratorType
		r.GetLogger().Info("Container orchestrator is successfully set", "type", orchestratorType)
	}
	return r.orchestratorType, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NemoDatastoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor("nemo-datastore-service-controller")
	return ctrl.NewControllerManagedBy(mgr).
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
					newNemoDatastore := e.ObjectNew.(*appsv1alpha1.NemoDatastore)

					// Handle case where object is marked for deletion
					if !newNemoDatastore.ObjectMeta.DeletionTimestamp.IsZero() {
						return true
					}

					// Handle only spec updates
					return !reflect.DeepEqual(oldNemoDatastore.Spec, newNemoDatastore.Spec)
				}
				// For other types we watch, reconcile them
				return true
			},
		}).
		Complete(r)
}

func (r *NemoDatastoreReconciler) refreshMetrics(ctx context.Context) {
	logger := log.FromContext(ctx)
	// List all nodes
	NemoDatastoreList := &appsv1alpha1.NemoDatastoreList{}
	err := r.Client.List(ctx, NemoDatastoreList, &client.ListOptions{})
	if err != nil {
		logger.Error(err, "unable to list NemoDatastores in the cluster")
		return
	}
	//refreshNemoDatastoreMetrics(NemoDatastoreList)
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
		err = r.cleanupResource(ctx, &networkingv1.Ingress{}, namespacedName)
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
		err = r.cleanupResource(ctx, &autoscalingv2.HorizontalPodAutoscaler{}, namespacedName)
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
	msg, ready, err := r.isDeploymentReady(ctx, &namespacedName)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !ready {
		// Update status as NotReady
		err = r.SetConditionsNotReady(ctx, nemoDatastore, conditions.NotReady, msg)
		r.GetEventRecorder().Eventf(nemoDatastore, corev1.EventTypeNormal, conditions.NotReady,
			"NemoDatastore %s not ready yet, msg: %s", nemoDatastore.Name, msg)
	} else {
		// Update status as ready
		err = r.SetConditionsReady(ctx, nemoDatastore, conditions.Ready, msg)
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

func (r *NemoDatastoreReconciler) renderAndSyncResource(ctx context.Context, NemoDatastore client.Object, renderer *render.Renderer, obj client.Object, renderFunc func() (client.Object, error), conditionType string, reason string) error {
	logger := log.FromContext(ctx)

	namespacedName := types.NamespacedName{Name: NemoDatastore.GetName(), Namespace: NemoDatastore.GetNamespace()}

	resource, err := renderFunc()
	if err != nil {
		logger.Error(err, "failed to render", conditionType, namespacedName)
		statusError := r.updater.SetConditionsFailed(ctx, NemoDatastore, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NemoDatastore", NemoDatastore.GetName())
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

	if err = controllerutil.SetControllerReference(NemoDatastore, resource, r.GetScheme()); err != nil {
		logger.Error(err, "failed to set owner", conditionType, namespacedName)
		statusError := r.updater.SetConditionsFailed(ctx, NemoDatastore, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NemoDatastore", NemoDatastore.GetName())
		}
		return err
	}

	err = r.syncResource(ctx, obj, resource, namespacedName)
	if err != nil {
		logger.Error(err, "failed to sync", conditionType, namespacedName)
		statusError := r.updater.SetConditionsFailed(ctx, NemoDatastore, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NemoDatastore", NemoDatastore.GetName())
		}
		return err
	}
	return nil
}

// CheckDeploymentReadiness checks if the Deployment is ready
func (r *NemoDatastoreReconciler) isDeploymentReady(ctx context.Context, namespacedName *types.NamespacedName) (string, bool, error) {
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Name: namespacedName.Name, Namespace: namespacedName.Namespace}, deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}

	cond := getDeploymentCondition(deployment.Status, appsv1.DeploymentProgressing)
	if cond != nil && cond.Reason == "ProgressDeadlineExceeded" {
		return fmt.Sprintf("deployment %q exceeded its progress deadline", deployment.Name), false, nil
	}
	if deployment.Spec.Replicas != nil && deployment.Status.UpdatedReplicas < *deployment.Spec.Replicas {
		return fmt.Sprintf("Waiting for deployment %q rollout to finish: %d out of %d new replicas have been updated...\n", deployment.Name, deployment.Status.UpdatedReplicas, *deployment.Spec.Replicas), false, nil
	}
	if deployment.Status.Replicas > deployment.Status.UpdatedReplicas {
		return fmt.Sprintf("Waiting for deployment %q rollout to finish: %d old replicas are pending termination...\n", deployment.Name, deployment.Status.Replicas-deployment.Status.UpdatedReplicas), false, nil
	}
	if deployment.Status.AvailableReplicas < deployment.Status.UpdatedReplicas {
		return fmt.Sprintf("Waiting for deployment %q rollout to finish: %d of %d updated replicas are available...\n", deployment.Name, deployment.Status.AvailableReplicas, deployment.Status.UpdatedReplicas), false, nil
	}
	return fmt.Sprintf("deployment %q successfully rolled out\n", deployment.Name), true, nil
}

func (r *NemoDatastoreReconciler) syncResource(ctx context.Context, obj client.Object, desired client.Object, namespacedName types.NamespacedName) error {
	logger := log.FromContext(ctx)

	err := r.Get(ctx, namespacedName, obj)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if !utils.IsSpecChanged(obj, desired) {
		logger.V(2).Info("Object spec has not changed, skipping update", "obj", obj)
		return nil
	}
	logger.V(2).Info("Object spec has changed, updating")

	if errors.IsNotFound(err) {
		err = r.Create(ctx, desired)
		if err != nil {
			return err
		}
	} else {
		err = r.Update(ctx, desired)
		if err != nil {
			return err
		}
	}
	return nil
}

// cleanupResource deletes the given Kubernetes resource if it exists.
// If the resource does not exist or an error occurs during deletion, the function returns nil or the error.
//
// Parameters:
// ctx (context.Context): The context for the operation.
// obj (client.Object): The Kubernetes resource to delete.
// namespacedName (types.NamespacedName): The namespaced name of the resource.
//
// Returns:
// error: An error if the resource deletion fails, or nil if the resource is not found or deletion is successful.
func (r *NemoDatastoreReconciler) cleanupResource(ctx context.Context, obj client.Object, namespacedName types.NamespacedName) error {

	logger := log.FromContext(ctx)

	err := r.Get(ctx, namespacedName, obj)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if errors.IsNotFound(err) {
		return nil
	}

	err = r.Delete(ctx, obj)
	if err != nil {
		return err
	}
	logger.V(2).Info("NIM Service object changed, deleting ", "obj", obj)
	return nil
}

func (r *NemoDatastoreReconciler) SetConditionsReady(ctx context.Context, cr *appsv1alpha1.NemoDatastore, reason, message string) error {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    conditions.Ready,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:   conditions.Failed,
		Status: metav1.ConditionFalse,
		Reason: conditions.Ready,
	})

	cr.Status.State = appsv1alpha1.NemoDatastoreStatusReady
	return r.updateNemoDatastoreStatus(ctx, cr)
}

func (r *NemoDatastoreReconciler) SetConditionsNotReady(ctx context.Context, cr *appsv1alpha1.NemoDatastore, reason, message string) error {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    conditions.Ready,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    conditions.Failed,
		Status:  metav1.ConditionFalse,
		Reason:  conditions.Ready,
		Message: message,
	})

	cr.Status.State = appsv1alpha1.NemoDatastoreStatusNotReady
	return r.updateNemoDatastoreStatus(ctx, cr)
}

func (r *NemoDatastoreReconciler) SetConditionsFailed(ctx context.Context, cr *appsv1alpha1.NemoDatastore, reason, message string) error {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:   conditions.Ready,
		Status: metav1.ConditionFalse,
		Reason: conditions.Failed,
	})

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    conditions.Failed,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
	cr.Status.State = appsv1alpha1.NemoDatastoreStatusFailed
	return r.updateNemoDatastoreStatus(ctx, cr)
}

func (r *NemoDatastoreReconciler) updateNemoDatastoreStatus(ctx context.Context, cr *appsv1alpha1.NemoDatastore) error {

	obj := &appsv1alpha1.NemoDatastore{}
	errGet := r.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.GetNamespace()}, obj)
	if errGet != nil {
		return errGet
	}
	obj.Status = cr.Status
	if err := r.Status().Update(ctx, obj); err != nil {
		return err
	}
	return nil
}
