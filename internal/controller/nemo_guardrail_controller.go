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
	scheme           *runtime.Scheme
	log              logr.Logger
	updater          conditions.Updater
	renderer         render.Renderer
	Config           *rest.Config
	recorder         record.EventRecorder
	orchestratorType k8sutil.OrchestratorType
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

	// Set container orchestrator type
	if r.GetOrchestratorType() != "" {
		orchestratorType, err := k8sutil.GetOrchestratorType(r.GetClient())
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("Unable to get container orchestrator type, %v", err)
		}
		r.orchestratorType = orchestratorType
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

// GetOrchestratorType returns the container platform type
func (r *NemoGuardrailReconciler) GetOrchestratorType() k8sutil.OrchestratorType {
	return r.orchestratorType
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

func (r *NemoGuardrailReconciler) reconcileNemoGuardrail(ctx context.Context, NemoGuardrail *appsv1alpha1.NemoGuardrail) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var err error
	defer func() {
		if err != nil {
			r.GetEventRecorder().Eventf(NemoGuardrail, corev1.EventTypeWarning, conditions.Failed,
				"NemoGuardrail %s failed, msg: %s", NemoGuardrail.Name, err.Error())
		}
	}()
	// Generate annotation for the current operator-version and apply to all resources
	// Get generic name for all resources
	namespacedName := types.NamespacedName{Name: NemoGuardrail.GetName(), Namespace: NemoGuardrail.GetNamespace()}

	renderer := r.GetRenderer()

	// Sync serviceaccount
	err = r.renderAndSyncResource(ctx, NemoGuardrail, &renderer, &corev1.ServiceAccount{}, func() (client.Object, error) {
		return renderer.ServiceAccount(NemoGuardrail.GetServiceAccountParams())
	}, "serviceaccount", conditions.ReasonServiceAccountFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync role
	err = r.renderAndSyncResource(ctx, NemoGuardrail, &renderer, &rbacv1.Role{}, func() (client.Object, error) {
		return renderer.Role(NemoGuardrail.GetRoleParams())
	}, "role", conditions.ReasonRoleFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync rolebinding
	err = r.renderAndSyncResource(ctx, NemoGuardrail, &renderer, &rbacv1.RoleBinding{}, func() (client.Object, error) {
		return renderer.RoleBinding(NemoGuardrail.GetRoleBindingParams())
	}, "rolebinding", conditions.ReasonRoleBindingFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync service
	err = r.renderAndSyncResource(ctx, NemoGuardrail, &renderer, &corev1.Service{}, func() (client.Object, error) {
		return renderer.Service(NemoGuardrail.GetServiceParams())
	}, "service", conditions.ReasonServiceFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync ingress
	if NemoGuardrail.IsIngressEnabled() {
		err = r.renderAndSyncResource(ctx, NemoGuardrail, &renderer, &networkingv1.Ingress{}, func() (client.Object, error) {
			return renderer.Ingress(NemoGuardrail.GetIngressParams())
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
	if NemoGuardrail.IsAutoScalingEnabled() {
		err = r.renderAndSyncResource(ctx, NemoGuardrail, &renderer, &autoscalingv2.HorizontalPodAutoscaler{}, func() (client.Object, error) {
			return renderer.HPA(NemoGuardrail.GetHPAParams())
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
	if NemoGuardrail.IsServiceMonitorEnabled() {
		err = r.renderAndSyncResource(ctx, NemoGuardrail, &renderer, &monitoringv1.ServiceMonitor{}, func() (client.Object, error) {
			return renderer.ServiceMonitor(NemoGuardrail.GetServiceMonitorParams())
		}, "servicemonitor", conditions.ReasonServiceMonitorFailed)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	deploymentParams := NemoGuardrail.GetDeploymentParams()

	// Setup volume mounts with model store
	deploymentParams.Volumes = NemoGuardrail.GetVolumes()
	deploymentParams.VolumeMounts = NemoGuardrail.GetVolumeMounts()

	logger.Info("Reconciling", "volumes", NemoGuardrail.GetVolumes())

	// Sync deployment
	err = r.renderAndSyncResource(ctx, NemoGuardrail, &renderer, &appsv1.Deployment{}, func() (client.Object, error) {
		return renderer.Deployment(deploymentParams)
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
		err = r.SetConditionsNotReady(ctx, NemoGuardrail, conditions.NotReady, msg)
		r.GetEventRecorder().Eventf(NemoGuardrail, corev1.EventTypeNormal, conditions.NotReady,
			"NemoGuardrail %s not ready yet, msg: %s", NemoGuardrail.Name, msg)
	} else {
		// Update status as ready
		err = r.SetConditionsReady(ctx, NemoGuardrail, conditions.Ready, msg)
		r.GetEventRecorder().Eventf(NemoGuardrail, corev1.EventTypeNormal, conditions.Ready,
			"NemoGuardrail %s ready, msg: %s", NemoGuardrail.Name, msg)
	}

	if err != nil {
		logger.Error(err, "Unable to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NemoGuardrailReconciler) renderAndSyncResource(ctx context.Context, NemoGuardrail client.Object, renderer *render.Renderer, obj client.Object, renderFunc func() (client.Object, error), conditionType string, reason string) error {
	logger := log.FromContext(ctx)

	namespacedName := types.NamespacedName{Name: NemoGuardrail.GetName(), Namespace: NemoGuardrail.GetNamespace()}

	resource, err := renderFunc()
	if err != nil {
		logger.Error(err, "failed to render", conditionType, namespacedName)
		statusError := r.updater.SetConditionsFailed(ctx, NemoGuardrail, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NemoGuardrail", NemoGuardrail.GetName())
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

	if err = controllerutil.SetControllerReference(NemoGuardrail, resource, r.GetScheme()); err != nil {
		logger.Error(err, "failed to set owner", conditionType, namespacedName)
		statusError := r.updater.SetConditionsFailed(ctx, NemoGuardrail, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NemoGuardrail", NemoGuardrail.GetName())
		}
		return err
	}

	err = r.syncResource(ctx, obj, resource, namespacedName)
	if err != nil {
		logger.Error(err, "failed to sync", conditionType, namespacedName)
		statusError := r.updater.SetConditionsFailed(ctx, NemoGuardrail, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NemoGuardrail", NemoGuardrail.GetName())
		}
		return err
	}
	return nil
}

// CheckDeploymentReadiness checks if the Deployment is ready
func (r *NemoGuardrailReconciler) isDeploymentReady(ctx context.Context, namespacedName *types.NamespacedName) (string, bool, error) {
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

func getDeploymentCondition(status appsv1.DeploymentStatus, condType appsv1.DeploymentConditionType) *appsv1.DeploymentCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

func (r *NemoGuardrailReconciler) syncResource(ctx context.Context, obj client.Object, desired client.Object, namespacedName types.NamespacedName) error {
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
func (r *NemoGuardrailReconciler) cleanupResource(ctx context.Context, obj client.Object, namespacedName types.NamespacedName) error {

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

func (r *NemoGuardrailReconciler) SetConditionsReady(ctx context.Context, cr *appsv1alpha1.NemoGuardrail, reason, message string) error {
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

	cr.Status.State = appsv1alpha1.NemoGuardrailStatusReady
	return r.updateNemoGuardrailStatus(ctx, cr)
}

func (r *NemoGuardrailReconciler) SetConditionsNotReady(ctx context.Context, cr *appsv1alpha1.NemoGuardrail, reason, message string) error {
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

	cr.Status.State = appsv1alpha1.NemoGuardrailStatusNotReady
	return r.updateNemoGuardrailStatus(ctx, cr)
}

func (r *NemoGuardrailReconciler) SetConditionsFailed(ctx context.Context, cr *appsv1alpha1.NemoGuardrail, reason, message string) error {
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
	cr.Status.State = appsv1alpha1.NemoGuardrailStatusFailed
	return r.updateNemoGuardrailStatus(ctx, cr)
}

func (r *NemoGuardrailReconciler) updateNemoGuardrailStatus(ctx context.Context, cr *appsv1alpha1.NemoGuardrail) error {

	obj := &appsv1alpha1.NemoGuardrail{}
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
