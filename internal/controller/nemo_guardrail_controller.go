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
	"encoding/base64"
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

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
	"github.com/NVIDIA/k8s-nim-operator/internal/render"
	"github.com/NVIDIA/k8s-nim-operator/internal/shared"
	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

// NemoGuardrailFinalizer is the finalizer annotation.
const NemoGuardrailFinalizer = "finalizer.NemoGuardrail.apps.nvidia.com"

// NemoGuardrailReconciler reconciles a NemoGuardrail object.
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

// Ensure NemoGuardrailReconciler implements the Reconciler interface.
var _ shared.Reconciler = &NemoGuardrailReconciler{}

// NewNemoGuardrailReconciler creates a new reconciler for NemoGuardrail with the given platform.
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
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
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
	if NemoGuardrail.DeletionTimestamp.IsZero() {
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
					"NemoGuardrail %s is being deleted", NemoGuardrail.Name)
				return ctrl.Result{}, err
			}
			// Remove finalizer to allow for deletion
			controllerutil.RemoveFinalizer(NemoGuardrail, NemoGuardrailFinalizer)
			if err := r.Update(ctx, NemoGuardrail); err != nil {
				r.GetEventRecorder().Eventf(NemoGuardrail, corev1.EventTypeNormal, "Delete",
					"NemoGuardrail %s finalizer removed", NemoGuardrail.Name)
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

// GetScheme returns the scheme of the reconciler.
func (r *NemoGuardrailReconciler) GetScheme() *runtime.Scheme {
	return r.scheme
}

// GetLogger returns the logger of the reconciler.
func (r *NemoGuardrailReconciler) GetLogger() logr.Logger {
	return r.log
}

// GetClient returns the client instance.
func (r *NemoGuardrailReconciler) GetClient() client.Client {
	return r.Client
}

// GetConfig returns the rest config.
func (r *NemoGuardrailReconciler) GetConfig() *rest.Config {
	return r.Config
}

// GetUpdater returns the conditions updater instance.
func (r *NemoGuardrailReconciler) GetUpdater() conditions.Updater {
	return r.updater
}

// GetDiscoveryClient returns the discovery client instance.
func (r *NemoGuardrailReconciler) GetDiscoveryClient() discovery.DiscoveryInterface {
	return nil
}

// GetRenderer returns the renderer instance.
func (r *NemoGuardrailReconciler) GetRenderer() render.Renderer {
	return r.renderer
}

// GetEventRecorder returns the event recorder.
func (r *NemoGuardrailReconciler) GetEventRecorder() record.EventRecorder {
	return r.recorder
}

// GetOrchestratorType returns the container platform type.
func (r *NemoGuardrailReconciler) GetOrchestratorType(ctx context.Context) (k8sutil.OrchestratorType, error) {
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
					newNemoGuardrail, ok := e.ObjectNew.(*appsv1alpha1.NemoGuardrail)
					if ok {
						// Handle case where object is marked for deletion
						if !newNemoGuardrail.ObjectMeta.DeletionTimestamp.IsZero() {
							return true
						}

						// Handle only spec updates
						return !reflect.DeepEqual(oldNemoGuardrail.Spec, newNemoGuardrail.Spec)
					}
				}
				// For other types we watch, reconcile them
				return true
			},
		}).
		Complete(r)
}

func (r *NemoGuardrailReconciler) refreshMetrics(ctx context.Context) {
	logger := log.FromContext(ctx)
	// List all guardrail instances
	NemoGuardrailList := &appsv1alpha1.NemoGuardrailList{}
	err := r.List(ctx, NemoGuardrailList, &client.ListOptions{})
	if err != nil {
		logger.Error(err, "unable to list NemoGuardrails in the cluster")
		return
	}
	refreshNemoGuardrailMetrics(NemoGuardrailList)
}

func (r *NemoGuardrailReconciler) reconcileNemoGuardrail(ctx context.Context, nemoGuardrail *appsv1alpha1.NemoGuardrail) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var err error
	defer func() {
		if err != nil {
			r.GetEventRecorder().Eventf(nemoGuardrail, corev1.EventTypeWarning, conditions.Failed,
				"NemoGuardrail %s failed, msg: %s", nemoGuardrail.Name, err.Error())
		}
	}()
	// Generate annotation for the current operator-version and apply to all resources
	// Get generic name for all resources
	namespacedName := types.NamespacedName{Name: nemoGuardrail.GetName(), Namespace: nemoGuardrail.GetNamespace()}

	renderer := r.GetRenderer()

	// Sync serviceaccount
	err = r.renderAndSyncResource(ctx, nemoGuardrail, &renderer, &corev1.ServiceAccount{}, func() (client.Object, error) {
		return renderer.ServiceAccount(nemoGuardrail.GetServiceAccountParams())
	}, "serviceaccount", conditions.ReasonServiceAccountFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync role
	err = r.renderAndSyncResource(ctx, nemoGuardrail, &renderer, &rbacv1.Role{}, func() (client.Object, error) {
		return renderer.Role(nemoGuardrail.GetRoleParams())
	}, "role", conditions.ReasonRoleFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync rolebinding
	err = r.renderAndSyncResource(ctx, nemoGuardrail, &renderer, &rbacv1.RoleBinding{}, func() (client.Object, error) {
		return renderer.RoleBinding(nemoGuardrail.GetRoleBindingParams())
	}, "rolebinding", conditions.ReasonRoleBindingFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync ConfigStore PVC
	if nemoGuardrail.Spec.ConfigStore.PVC != nil {
		if err = r.reconcilePVC(ctx, nemoGuardrail); err != nil {
			return ctrl.Result{}, err
		}
	}
	// Sync service
	err = r.renderAndSyncResource(ctx, nemoGuardrail, &renderer, &corev1.Service{}, func() (client.Object, error) {
		return renderer.Service(nemoGuardrail.GetServiceParams())
	}, "service", conditions.ReasonServiceFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync ingress
	if nemoGuardrail.IsIngressEnabled() {
		err = r.renderAndSyncResource(ctx, nemoGuardrail, &renderer, &networkingv1.Ingress{}, func() (client.Object, error) {
			return renderer.Ingress(nemoGuardrail.GetIngressParams())
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

	// Sync HPA
	if nemoGuardrail.IsAutoScalingEnabled() {
		err = r.renderAndSyncResource(ctx, nemoGuardrail, &renderer, &autoscalingv2.HorizontalPodAutoscaler{}, func() (client.Object, error) {
			return renderer.HPA(nemoGuardrail.GetHPAParams())
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
	if nemoGuardrail.IsServiceMonitorEnabled() {
		err = r.renderAndSyncResource(ctx, nemoGuardrail, &renderer, &monitoringv1.ServiceMonitor{}, func() (client.Object, error) {
			return renderer.ServiceMonitor(nemoGuardrail.GetServiceMonitorParams())
		}, "servicemonitor", conditions.ReasonServiceMonitorFailed)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if nemoGuardrail.Spec.DatabaseConfig != nil {
		secretValue, err := r.getValueFromSecret(ctx, nemoGuardrail.GetNamespace(), nemoGuardrail.Spec.DatabaseConfig.Credentials.SecretName, nemoGuardrail.Spec.DatabaseConfig.Credentials.PasswordKey)
		if err != nil {
			return ctrl.Result{}, err
		}
		data := nemoGuardrail.GeneratePostgresConnString(secretValue)
		// Encode to base64
		encoded := base64.StdEncoding.EncodeToString([]byte(data))

		secretMapData := map[string]string{
			"uri": encoded,
		}

		// Sync Evaluator Secret
		err = r.renderAndSyncResource(ctx, nemoGuardrail, &renderer, &corev1.Secret{}, func() (client.Object, error) {
			return renderer.Secret(nemoGuardrail.GetSecretParams(secretMapData))
		}, "secret", conditions.ReasonSecretFailed)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	deploymentParams := nemoGuardrail.GetDeploymentParams()

	// Setup volume mounts with model store
	deploymentParams.Volumes = nemoGuardrail.GetVolumes()
	deploymentParams.VolumeMounts = nemoGuardrail.GetVolumeMounts()

	logger.Info("Reconciling", "volumes", nemoGuardrail.GetVolumes())

	// Sync deployment
	err = r.renderAndSyncResource(ctx, nemoGuardrail, &renderer, &appsv1.Deployment{}, func() (client.Object, error) {
		result, err := renderer.Deployment(deploymentParams)
		if err != nil {
			return nil, err
		}
		if nemoGuardrail.Spec.DatabaseConfig != nil {
			initContainers := nemoGuardrail.GetInitContainers()
			if len(initContainers) > 0 {
				result.Spec.Template.Spec.InitContainers = initContainers
			}
		}
		return result, err
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
		err = r.updater.SetConditionsNotReady(ctx, nemoGuardrail, conditions.NotReady, msg)
		r.GetEventRecorder().Eventf(nemoGuardrail, corev1.EventTypeNormal, conditions.NotReady,
			"NemoGuardrail %s not ready yet, msg: %s", nemoGuardrail.Name, msg)
	} else {
		// Update status as ready
		err = r.updater.SetConditionsReady(ctx, nemoGuardrail, conditions.Ready, msg)
		r.GetEventRecorder().Eventf(nemoGuardrail, corev1.EventTypeNormal, conditions.Ready,
			"NemoGuardrail %s ready, msg: %s", nemoGuardrail.Name, msg)
	}

	if err != nil {
		logger.Error(err, "Unable to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NemoGuardrailReconciler) reconcilePVC(ctx context.Context, nemoGuardrail *appsv1alpha1.NemoGuardrail) error {
	logger := r.GetLogger()
	pvcName := shared.GetPVCName(nemoGuardrail, *nemoGuardrail.Spec.ConfigStore.PVC)
	pvcNamespacedName := types.NamespacedName{Name: pvcName, Namespace: nemoGuardrail.GetNamespace()}
	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, pvcNamespacedName, pvc)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return err
	}

	// If PVC does not exist, create a new one if creation flag is enabled
	if err != nil {
		if nemoGuardrail.Spec.ConfigStore.PVC.Create != nil && *nemoGuardrail.Spec.ConfigStore.PVC.Create {
			pvc, err = shared.ConstructPVC(*nemoGuardrail.Spec.ConfigStore.PVC, metav1.ObjectMeta{Name: pvcName, Namespace: nemoGuardrail.GetNamespace()})
			if err != nil {
				logger.Error(err, "Failed to construct pvc", "name", pvcName)
				return err
			}
			if err := controllerutil.SetControllerReference(nemoGuardrail, pvc, r.GetScheme()); err != nil {
				return err
			}
			err = r.Create(ctx, pvc)
			if err != nil {
				logger.Error(err, "Failed to create pvc", "name", pvcName)
				return err
			}
			logger.Info("Created PVC for NEMO Guardrail ConfigStore", "pvc", pvc.Name)
		} else {
			logger.Error(err, "PVC doesn't exist and auto-creation is not enabled", "name", pvcNamespacedName)
			return err
		}
	}
	return nil
}

func (r *NemoGuardrailReconciler) renderAndSyncResource(ctx context.Context, nemoGuardrail *appsv1alpha1.NemoGuardrail, renderer *render.Renderer, obj client.Object, renderFunc func() (client.Object, error), conditionType string, reason string) error {
	logger := log.FromContext(ctx)

	namespacedName := types.NamespacedName{Name: nemoGuardrail.GetName(), Namespace: nemoGuardrail.GetNamespace()}
	getErr := r.Get(ctx, namespacedName, obj)
	if getErr != nil && !errors.IsNotFound(getErr) {
		logger.Error(getErr, fmt.Sprintf("Error is not NotFound for %s: %v", obj.GetObjectKind(), getErr))
		return getErr
	}

	// Track an existing resource
	found := getErr == nil

	// Don't do anything if CR is unchanged.
	if found && !utils.IsParentSpecChanged(obj, utils.DeepHashObject(nemoGuardrail.Spec)) {
		return nil
	}

	resource, err := renderFunc()
	if err != nil {
		logger.Error(err, "failed to render", "conditionType", conditionType)
		statusError := r.updater.SetConditionsFailed(ctx, nemoGuardrail, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NemoGuardrail", nemoGuardrail.GetName())
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

	// If we found the object and autoscaling is enabled on the Guardrails,
	// copy the current replicas from the existing object into the desired (resource),
	// so we don't fight the HPA (or external scaler) on each reconcile.
	if found && nemoGuardrail.IsAutoScalingEnabled() {
		if curr, ok := obj.(*appsv1.Deployment); ok {
			if desired, ok := resource.(*appsv1.Deployment); ok && curr.Spec.Replicas != nil {
				replicas := *curr.Spec.Replicas
				desired.Spec.Replicas = &replicas
			}
		}
	}

	if err = controllerutil.SetControllerReference(nemoGuardrail, resource, r.GetScheme()); err != nil {
		logger.Error(err, "failed to set owner", "conditionType", conditionType)
		statusError := r.updater.SetConditionsFailed(ctx, nemoGuardrail, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NemoGuardrail", nemoGuardrail.GetName())
		}
		return err
	}

	err = k8sutil.SyncResource(ctx, r.GetClient(), obj, resource)
	if err != nil {
		logger.Error(err, "failed to sync", "conditionType", conditionType)
		statusError := r.updater.SetConditionsFailed(ctx, nemoGuardrail, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NemoGuardrail", nemoGuardrail.GetName())
		}
		return err
	}
	return nil
}

func (r *NemoGuardrailReconciler) getValueFromSecret(ctx context.Context, namespace, secretName, key string) (string, error) {
	// Get the secret
	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: secretName, Namespace: namespace}, secret)
	if err != nil {
		return "", fmt.Errorf("failed to get secret: %w", err)
	}

	// Extract the secret value
	value, exists := secret.Data[key]

	if !exists {
		return "", fmt.Errorf("key '%s' not found in secret '%s'", key, secretName)
	}
	return string(value), nil
}
