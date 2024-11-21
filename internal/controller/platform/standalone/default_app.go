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

package standalone

import (
	"context"
	"fmt"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	"github.com/NVIDIA/k8s-nim-operator/internal/render"
	rendertypes "github.com/NVIDIA/k8s-nim-operator/internal/render/types"
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
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DefaultAppReconciler reconciles a NemoGuardrail object
type DefaultAppReconciler struct {
	client.Client
	scheme   *runtime.Scheme
	log      logr.Logger
	updater  conditions.Updater
	renderer render.Renderer
	Config   *rest.Config
	recorder record.EventRecorder
}

// Ensure DefaultAppReconciler implements the Reconciler interface
var _ shared.Reconciler = &DefaultAppReconciler{}

type AppParamProvider interface {

	// GetStandardSelectorLabels returns the standard selector labels for the NemoGuardrail deployment
	GetName() string
	// GetStandardSelectorLabels returns the standard selector labels for the NemoGuardrail deployment
	GetStandardSelectorLabels() map[string]string

	// GetStandardLabels returns the standard set of labels for NemoGuardrail resources
	GetStandardLabels() map[string]string

	// GetStandardEnv returns the standard set of env variables for the NemoGuardrail container
	GetStandardEnv() []corev1.EnvVar
	// GetStandardAnnotations returns default annotations to apply to the NemoGuardrail instance
	GetStandardAnnotations() map[string]string
	// GetNemoGuardrailAnnotations returns annotations to apply to the NemoGuardrail instance
	GetNemoGuardrailAnnotations() map[string]string

	// GetServiceLabels returns merged labels to apply to the NemoGuardrail instance
	GetServiceLabels() map[string]string
	// GetSelectorLabels returns standard selector labels to apply to the NemoGuardrail instance
	GetSelectorLabels() map[string]string
	// GetNodeSelector returns node selector labels for the NemoGuardrail instance
	GetNodeSelector() map[string]string
	// GetTolerations returns tolerations for the NemoGuardrail instance
	GetTolerations() []corev1.Toleration
	// GetPodAffinity returns pod affinity for the NemoGuardrail instance
	GetPodAffinity() *corev1.PodAffinity
	// GetContainerName returns name of the container for NemoGuardrail deployment
	GetContainerName() string
	// GetCommand return command to override for the NemoGuardrail container
	GetCommand() []string
	// GetArgs return arguments for the NemoGuardrail container
	GetArgs() []string
	// GetEnv returns merged slice of standard and user specified env variables
	GetEnv() []corev1.EnvVar
	// GetImage returns container image for the NemoGuardrail
	GetImage() string
	// GetImagePullSecrets returns the image pull secrets for the NIM container
	GetImagePullSecrets() []string
	// GetImagePullPolicy returns the image pull policy for the NIM container
	GetImagePullPolicy() string
	// GetResources returns resources to allocate to the NemoGuardrail container
	GetResources() *corev1.ResourceRequirements
	// GetLivenessProbe returns liveness probe for the NemoGuardrail container
	GetLivenessProbe() *corev1.Probe
	// GetDefaultLivenessProbe returns the default liveness probe for the NemoGuardrail container
	GetDefaultLivenessProbe() *corev1.Probe
	// GetReadinessProbe returns readiness probe for the NemoGuardrail container
	GetReadinessProbe() *corev1.Probe

	// GetDefaultReadinessProbe returns the default readiness probe for the NemoGuardrail container
	GetDefaultReadinessProbe() *corev1.Probe
	// GetStartupProbe returns startup probe for the NemoGuardrail container
	GetStartupProbe() *corev1.Probe
	// GetDefaultStartupProbe returns the default startup probe for the NemoGuardrail container
	GetDefaultStartupProbe() *corev1.Probe
	// GetVolumes returns volumes for the NemoGuardrail container
	GetVolumes() []corev1.Volume
	// GetVolumeMounts returns volumes for the NemoGuardrail container
	GetVolumeMounts() []corev1.VolumeMount
	// GetServiceAccountName returns service account name for the NemoGuardrail deployment
	GetServiceAccountName() string
	// GetHPA returns the HPA spec for the NemoGuardrail deployment
	GetHPA() appsv1alpha1.HorizontalPodAutoscalerSpec
	// GetServiceMonitor returns the Service Monitor details for the NemoGuardrail deployment
	GetServiceMonitor() appsv1alpha1.ServiceMonitor
	// GetReplicas returns replicas for the NemoGuardrail deployment
	GetReplicas() int
	// GetDeploymentKind returns the kind of deployment for NemoGuardrail
	GetDeploymentKind() string
	// IsAutoScalingEnabled returns true if autoscaling is enabled for NemoGuardrail deployment
	IsAutoScalingEnabled() bool
	// IsIngressEnabled returns true if ingress is enabled for NemoGuardrail deployment
	IsIngressEnabled() bool
	// GetIngressSpec returns the Ingress spec NemoGuardrail deployment
	GetIngressSpec() networkingv1.IngressSpec
	// IsServiceMonitorEnabled returns true if servicemonitor is enabled for NemoGuardrail deployment
	IsServiceMonitorEnabled() bool
	// GetServicePort returns the service port for the NemoGuardrail deployment
	GetServicePort() int32
	// GetUserID returns the user ID for the NemoGuardrail deployment
	GetUserID() *int64
	// GetGroupID returns the group ID for the NemoGuardrail deployment
	GetGroupID() *int64
	// GetServiceAccountParams return params to render ServiceAccount from templates
	GetServiceAccountParams() *rendertypes.ServiceAccountParams

	// GetDeploymentParams returns params to render Deployment from templates
	GetDeploymentParams() *rendertypes.DeploymentParams

	// GetStatefulSetParams returns params to render StatefulSet from templates
	GetStatefulSetParams() *rendertypes.StatefulSetParams
	// GetIngressParams returns params to render Ingress from templates
	GetIngressParams() *rendertypes.IngressParams
	// GetIngressParams returns params to render Ingress from templates
	GetServiceParams() *rendertypes.ServiceParams

	// GetRoleParams returns params to render Role from templates
	GetRoleParams() *rendertypes.RoleParams
	// GetRoleBindingParams returns params to render RoleBinding from templates
	GetRoleBindingParams() *rendertypes.RoleBindingParams
	// GetHPAParams returns params to render HPA from templates
	GetHPAParams() *rendertypes.HPAParams
	// GetSCCParams return params to render SCC from templates
	GetSCCParams() *rendertypes.SCCParams

	// GetServiceMonitorParams return params to render Service Monitor from templates
	GetServiceMonitorParams() *rendertypes.ServiceMonitorParams

	GetIngressAnnotations() map[string]string

	GetServiceAnnotations() map[string]string
	GetHPAAnnotations() map[string]string
	GetServiceMonitorAnnotations() map[string]string
}

// NewDefaultAppReconciler creates a new reconciler for NemoGuardrail with the given platform
func NewDefaultAppReconciler(client client.Client, scheme *runtime.Scheme, updater conditions.Updater, renderer render.Renderer, log logr.Logger) *DefaultAppReconciler {
	return &DefaultAppReconciler{
		Client:   client,
		scheme:   scheme,
		updater:  updater,
		renderer: renderer,
		log:      log,
	}
}

// GetScheme returns the scheme of the reconciler
func (r *DefaultAppReconciler) GetScheme() *runtime.Scheme {
	return r.scheme
}

// GetLogger returns the logger of the reconciler
func (r *DefaultAppReconciler) GetLogger() logr.Logger {
	return r.log
}

// GetClient returns the client instance
func (r *DefaultAppReconciler) GetClient() client.Client {
	return r.Client
}

// GetConfig returns the rest config
func (r *DefaultAppReconciler) GetConfig() *rest.Config {
	return r.Config
}

// GetUpdater returns the conditions updater instance
func (r *DefaultAppReconciler) GetUpdater() conditions.Updater {
	return r.updater
}

// GetRenderer returns the renderer instance
func (r *DefaultAppReconciler) GetRenderer() render.Renderer {
	return r.renderer
}

// GetEventRecorder returns the event recorder
func (r *DefaultAppReconciler) GetEventRecorder() record.EventRecorder {
	return r.recorder
}

func (r *DefaultAppReconciler) ReconcileDefaultApp(ctx context.Context, appObject client.Object, appParams AppParamProvider) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var err error
	defer func() {
		if err != nil {
			r.GetEventRecorder().Eventf(appObject, corev1.EventTypeWarning, conditions.Failed,
				"NemoGuardrail %s failed, msg: %s", appObject.GetName(), err.Error())
		}
	}()
	// Generate annotation for the current operator-version and apply to all resources
	// Get generic name for all resources
	namespacedName := types.NamespacedName{Name: appObject.GetName(), Namespace: appObject.GetNamespace()}

	renderer := r.GetRenderer()

	// Sync serviceaccount
	err = r.renderAndSyncResource(ctx, appObject, &corev1.ServiceAccount{}, func() (client.Object, error) {
		return renderer.ServiceAccount(appParams.GetServiceAccountParams())
	}, "serviceaccount", conditions.ReasonServiceAccountFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync role
	err = r.renderAndSyncResource(ctx, appObject, &rbacv1.Role{}, func() (client.Object, error) {
		return renderer.Role(appParams.GetRoleParams())
	}, "role", conditions.ReasonRoleFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync rolebinding
	err = r.renderAndSyncResource(ctx, appObject, &rbacv1.RoleBinding{}, func() (client.Object, error) {
		return renderer.RoleBinding(appParams.GetRoleBindingParams())
	}, "rolebinding", conditions.ReasonRoleBindingFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync service
	err = r.renderAndSyncResource(ctx, appObject, &corev1.Service{}, func() (client.Object, error) {
		return renderer.Service(appParams.GetServiceParams())
	}, "service", conditions.ReasonServiceFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync ingress
	if appParams.IsIngressEnabled() {
		err = r.renderAndSyncResource(ctx, appObject, &networkingv1.Ingress{}, func() (client.Object, error) {
			return renderer.Ingress(appParams.GetIngressParams())
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
	if appParams.IsAutoScalingEnabled() {
		err = r.renderAndSyncResource(ctx, appObject, &autoscalingv2.HorizontalPodAutoscaler{}, func() (client.Object, error) {
			return renderer.HPA(appParams.GetHPAParams())
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
	if appParams.IsServiceMonitorEnabled() {
		err = r.renderAndSyncResource(ctx, appObject, &monitoringv1.ServiceMonitor{}, func() (client.Object, error) {
			return renderer.ServiceMonitor(appParams.GetServiceMonitorParams())
		}, "servicemonitor", conditions.ReasonServiceMonitorFailed)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	deploymentParams := appParams.GetDeploymentParams()

	// Setup volume mounts with model store
	deploymentParams.Volumes = appParams.GetVolumes()
	deploymentParams.VolumeMounts = appParams.GetVolumeMounts()

	logger.Info("Reconciling", "volumes", appParams.GetVolumes())

	// Sync deployment
	err = r.renderAndSyncResource(ctx, appObject, &appsv1.Deployment{}, func() (client.Object, error) {
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
		err = r.updater.SetConditionsNotReady(ctx, appObject, conditions.NotReady, msg)
		r.GetEventRecorder().Eventf(appObject, corev1.EventTypeNormal, conditions.NotReady,
			"NemoGuardrail %s not ready yet, msg: %s", appObject.GetName(), msg)
	} else {
		// Update status as ready
		err = r.updater.SetConditionsReady(ctx, appObject, conditions.Ready, msg)
		r.GetEventRecorder().Eventf(appObject, corev1.EventTypeNormal, conditions.Ready,
			"NemoGuardrail %s ready, msg: %s", appObject.GetName(), msg)
	}

	if err != nil {
		logger.Error(err, "Unable to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *DefaultAppReconciler) renderAndSyncResource(ctx context.Context, NemoGuardrail client.Object, obj client.Object, renderFunc func() (client.Object, error), conditionType string, reason string) error {
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
func (r *DefaultAppReconciler) isDeploymentReady(ctx context.Context, namespacedName *types.NamespacedName) (string, bool, error) {
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

func (r *DefaultAppReconciler) syncResource(ctx context.Context, obj client.Object, desired client.Object, namespacedName types.NamespacedName) error {
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
func (r *DefaultAppReconciler) cleanupResource(ctx context.Context, obj client.Object, namespacedName types.NamespacedName) error {

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
