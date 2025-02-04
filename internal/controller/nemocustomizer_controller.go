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

const (
	// NemoCustomizerFinalizer is the finalizer annotation
	NemoCustomizerFinalizer = "finalizer.nemocustomizer.apps.nvidia.com"
	// ConfigHashAnnotationKey is the annotation key for storing hash of the training configuration
	ConfigHashAnnotationKey = "apps.nvidia.com/training-config-hash"
)

// NemoCustomizerReconciler reconciles a NemoCustomizer object
type NemoCustomizerReconciler struct {
	client.Client
	scheme           *runtime.Scheme
	log              logr.Logger
	updater          conditions.Updater
	renderer         render.Renderer
	Config           *rest.Config
	recorder         record.EventRecorder
	orchestratorType k8sutil.OrchestratorType
}

// Ensure NemoCustomizerReconciler implements the Reconciler interface
var _ shared.Reconciler = &NemoCustomizerReconciler{}

// NewNemoCustomizerReconciler creates a new reconciler for NemoCustomizer with the given platform
func NewNemoCustomizerReconciler(client client.Client, scheme *runtime.Scheme, updater conditions.Updater, renderer render.Renderer, log logr.Logger) *NemoCustomizerReconciler {
	return &NemoCustomizerReconciler{
		Client:   client,
		scheme:   scheme,
		updater:  updater,
		renderer: renderer,
		log:      log,
	}
}

// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemocustomizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemocustomizers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemocustomizers/finalizers,verbs=update
// +kubebuilder:rbac:groups=nvidia.com,resources=nemotrainingjobs;nemotrainingjobs/status;nemoentityhandlers,verbs=create;get;list;watch;update;delete;patch
// +kubebuilder:rbac:groups=batch.volcano.sh,resources=jobs;jobs/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=nodeinfo.volcano.sh,resources=numatopologies,verbs=get;list;watch
// +kubebuilder:rbac:groups=scheduling.incubator.k8s.io;scheduling.volcano.sh,resources=queues;queues/status;podgroups,verbs=get;list;watch
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
// +kubebuilder:rbac:groups="nodeinfo.volcano.sh",resources=numatopologies,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NemoCustomizer object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *NemoCustomizerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	defer r.refreshMetrics(ctx)

	// Fetch the NemoCustomizer instance
	NemoCustomizer := &appsv1alpha1.NemoCustomizer{}
	if err := r.Get(ctx, req.NamespacedName, NemoCustomizer); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "unable to fetch NemoCustomizer", "name", req.NamespacedName)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling", "NemoCustomizer", NemoCustomizer.Name)

	// Check if the instance is marked for deletion
	if NemoCustomizer.ObjectMeta.DeletionTimestamp.IsZero() {
		// Add finalizer if not present
		if !controllerutil.ContainsFinalizer(NemoCustomizer, NemoCustomizerFinalizer) {
			controllerutil.AddFinalizer(NemoCustomizer, NemoCustomizerFinalizer)
			if err := r.Update(ctx, NemoCustomizer); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The instance is being deleted
		if controllerutil.ContainsFinalizer(NemoCustomizer, NemoCustomizerFinalizer) {
			// Perform platform specific cleanup of resources
			if err := r.cleanupNemoCustomizer(ctx, NemoCustomizer); err != nil {
				r.GetEventRecorder().Eventf(NemoCustomizer, corev1.EventTypeNormal, "Delete",
					"NemoCustomizer %s in deleted", NemoCustomizer.Name)
				return ctrl.Result{}, err
			}

			// Remove finalizer to allow for deletion
			controllerutil.RemoveFinalizer(NemoCustomizer, NemoCustomizerFinalizer)
			if err := r.Update(ctx, NemoCustomizer); err != nil {
				r.GetEventRecorder().Eventf(NemoCustomizer, corev1.EventTypeNormal, "Delete",
					"NemoCustomizer %s finalizer removed", NemoCustomizer.Name)
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
	if result, err := r.reconcileNemoCustomizer(ctx, NemoCustomizer); err != nil {
		logger.Error(err, "error reconciling NemoCustomizer", "name", NemoCustomizer.Name)
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *NemoCustomizerReconciler) cleanupNemoCustomizer(ctx context.Context, nemoCustomizer *appsv1alpha1.NemoCustomizer) error {
	// All dependent (owned) objects will be automatically garbage collected.
	// TODO: Handle any custom cleanup logic for the NIM microservice
	return nil
}

// GetScheme returns the scheme of the reconciler
func (r *NemoCustomizerReconciler) GetScheme() *runtime.Scheme {
	return r.scheme
}

// GetLogger returns the logger of the reconciler
func (r *NemoCustomizerReconciler) GetLogger() logr.Logger {
	return r.log
}

// GetClient returns the client instance
func (r *NemoCustomizerReconciler) GetClient() client.Client {
	return r.Client
}

// GetConfig returns the rest config
func (r *NemoCustomizerReconciler) GetConfig() *rest.Config {
	return r.Config
}

// GetUpdater returns the conditions updater instance
func (r *NemoCustomizerReconciler) GetUpdater() conditions.Updater {
	return r.updater
}

// GetRenderer returns the renderer instance
func (r *NemoCustomizerReconciler) GetRenderer() render.Renderer {
	return r.renderer
}

// GetEventRecorder returns the event recorder
func (r *NemoCustomizerReconciler) GetEventRecorder() record.EventRecorder {
	return r.recorder
}

// GetOrchestratorType returns the container platform type
func (r *NemoCustomizerReconciler) GetOrchestratorType() (k8sutil.OrchestratorType, error) {
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
func (r *NemoCustomizerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor("nemo-customizer-service-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.NemoCustomizer{}).
		Owns(&appsv1.Deployment{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&networkingv1.Ingress{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		Owns(&corev1.ConfigMap{}).
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Type assert to NemoCustomizer
				if oldNemoCustomizer, ok := e.ObjectOld.(*appsv1alpha1.NemoCustomizer); ok {
					newNemoCustomizer := e.ObjectNew.(*appsv1alpha1.NemoCustomizer)

					// Handle case where object is marked for deletion
					if !newNemoCustomizer.ObjectMeta.DeletionTimestamp.IsZero() {
						return true
					}

					// Handle only spec updates
					return !reflect.DeepEqual(oldNemoCustomizer.Spec, newNemoCustomizer.Spec)
				}
				// For other types we watch, reconcile them
				return true
			},
		}).
		Complete(r)
}

func (r *NemoCustomizerReconciler) refreshMetrics(ctx context.Context) {
	logger := log.FromContext(ctx)
	// List all instances
	NemoCustomizerList := &appsv1alpha1.NemoCustomizerList{}
	err := r.Client.List(ctx, NemoCustomizerList, &client.ListOptions{})
	if err != nil {
		logger.Error(err, "unable to list NemoCustomizer in the cluster")
		return
	}
}

func (r *NemoCustomizerReconciler) reconcileNemoCustomizer(ctx context.Context, NemoCustomizer *appsv1alpha1.NemoCustomizer) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var err error
	defer func() {
		if err != nil {
			r.GetEventRecorder().Eventf(NemoCustomizer, corev1.EventTypeWarning, conditions.Failed,
				"NemoCustomizer %s failed, msg: %s", NemoCustomizer.Name, err.Error())
		}
	}()
	// Generate annotation for the current operator-version and apply to all resources
	// Get generic name for all resources
	namespacedName := types.NamespacedName{Name: NemoCustomizer.GetName(), Namespace: NemoCustomizer.GetNamespace()}
	renderer := r.GetRenderer()

	// Sync serviceaccount
	err = r.renderAndSyncResource(ctx, NemoCustomizer, &renderer, &corev1.ServiceAccount{}, func() (client.Object, error) {
		return renderer.ServiceAccount(NemoCustomizer.GetServiceAccountParams())
	}, "serviceaccount", conditions.ReasonServiceAccountFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync role
	err = r.renderAndSyncResource(ctx, NemoCustomizer, &renderer, &rbacv1.Role{}, func() (client.Object, error) {
		return renderer.Role(NemoCustomizer.GetRoleParams())
	}, "role", conditions.ReasonRoleFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync rolebinding
	err = r.renderAndSyncResource(ctx, NemoCustomizer, &renderer, &rbacv1.RoleBinding{}, func() (client.Object, error) {
		return renderer.RoleBinding(NemoCustomizer.GetRoleBindingParams())
	}, "rolebinding", conditions.ReasonRoleBindingFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync service
	err = r.renderAndSyncResource(ctx, NemoCustomizer, &renderer, &corev1.Service{}, func() (client.Object, error) {
		return renderer.Service(NemoCustomizer.GetServiceParams())
	}, "service", conditions.ReasonServiceFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync ingress
	if NemoCustomizer.IsIngressEnabled() {
		err = r.renderAndSyncResource(ctx, NemoCustomizer, &renderer, &networkingv1.Ingress{}, func() (client.Object, error) {
			return renderer.Ingress(NemoCustomizer.GetIngressParams())
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
	if NemoCustomizer.IsAutoScalingEnabled() {
		err = r.renderAndSyncResource(ctx, NemoCustomizer, &renderer, &autoscalingv2.HorizontalPodAutoscaler{}, func() (client.Object, error) {
			return renderer.HPA(NemoCustomizer.GetHPAParams())
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
	if NemoCustomizer.IsServiceMonitorEnabled() {
		err = r.renderAndSyncResource(ctx, NemoCustomizer, &renderer, &monitoringv1.ServiceMonitor{}, func() (client.Object, error) {
			return renderer.ServiceMonitor(NemoCustomizer.GetServiceMonitorParams())
		}, "servicemonitor", conditions.ReasonServiceMonitorFailed)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Sync Customizer ConfigMap
	err = r.renderAndSyncResource(ctx, NemoCustomizer, &renderer, &corev1.ConfigMap{}, func() (client.Object, error) {
		return renderer.ConfigMap(NemoCustomizer.GetConfigMapParams())
	}, "configmap", conditions.ReasonConfigMapFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Get params to render Deployment resource
	deploymentParams := NemoCustomizer.GetDeploymentParams()

	// Calculate the hash of the config data
	configHash := utils.CalculateSHA256(NemoCustomizer.Spec.CustomizerConfig)
	annotations := deploymentParams.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// Check if the hash has changed
	if annotations[ConfigHashAnnotationKey] != configHash {
		annotations[ConfigHashAnnotationKey] = configHash
		deploymentParams.Annotations = annotations
	}

	// Setup volume mounts with customizer config
	deploymentParams.Volumes = NemoCustomizer.GetVolumes()
	deploymentParams.VolumeMounts = NemoCustomizer.GetVolumeMounts()

	// Sync deployment
	err = r.renderAndSyncResource(ctx, NemoCustomizer, &renderer, &appsv1.Deployment{}, func() (client.Object, error) {
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
		err = r.updater.SetConditionsNotReady(ctx, NemoCustomizer, conditions.NotReady, msg)
		r.GetEventRecorder().Eventf(NemoCustomizer, corev1.EventTypeNormal, conditions.NotReady,
			"NemoCustomizer %s not ready yet, msg: %s", NemoCustomizer.Name, msg)
	} else {
		// Update status as ready
		err = r.updater.SetConditionsReady(ctx, NemoCustomizer, conditions.Ready, msg)
		r.GetEventRecorder().Eventf(NemoCustomizer, corev1.EventTypeNormal, conditions.Ready,
			"NemoCustomizer %s ready, msg: %s", NemoCustomizer.Name, msg)
	}

	if err != nil {
		logger.Error(err, "Unable to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NemoCustomizerReconciler) renderAndSyncResource(ctx context.Context, NemoCustomizer client.Object, renderer *render.Renderer, obj client.Object, renderFunc func() (client.Object, error), conditionType string, reason string) error {
	logger := log.FromContext(ctx)

	resource, err := renderFunc()
	if err != nil {
		logger.Error(err, "failed to render", conditionType, NemoCustomizer.GetName(), NemoCustomizer.GetNamespace())
		statusError := r.updater.SetConditionsFailed(ctx, NemoCustomizer, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NemoCustomizer", NemoCustomizer.GetName())
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

	namespacedName := types.NamespacedName{Name: metaAccessor.GetName(), Namespace: metaAccessor.GetNamespace()}

	if err = controllerutil.SetControllerReference(NemoCustomizer, resource, r.GetScheme()); err != nil {
		logger.Error(err, "failed to set owner", conditionType, namespacedName)
		statusError := r.updater.SetConditionsFailed(ctx, NemoCustomizer, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NemoCustomizer", NemoCustomizer.GetName())
		}
		return err
	}

	err = r.syncResource(ctx, obj, resource, namespacedName)
	if err != nil {
		logger.Error(err, "failed to sync", conditionType, namespacedName)
		statusError := r.updater.SetConditionsFailed(ctx, NemoCustomizer, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NemoCustomizer", NemoCustomizer.GetName())
		}
		return err
	}
	return nil
}

// CheckDeploymentReadiness checks if the Deployment is ready
func (r *NemoCustomizerReconciler) isDeploymentReady(ctx context.Context, namespacedName *types.NamespacedName) (string, bool, error) {
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

func (r *NemoCustomizerReconciler) syncResource(ctx context.Context, obj client.Object, desired client.Object, namespacedName types.NamespacedName) error {
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
func (r *NemoCustomizerReconciler) cleanupResource(ctx context.Context, obj client.Object, namespacedName types.NamespacedName) error {

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
