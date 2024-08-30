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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// GetScheme returns the scheme of the reconciler
func (r *NIMServiceReconciler) GetScheme() *runtime.Scheme {
	return r.scheme
}

// GetLogger returns the logger of the reconciler
func (r *NIMServiceReconciler) GetLogger() logr.Logger {
	return r.log
}

// GetClient returns the client instance
func (r *NIMServiceReconciler) GetClient() client.Client {
	return r.Client
}

// GetUpdater returns the conditions updater instance
func (r *NIMServiceReconciler) GetUpdater() conditions.Updater {
	return r.updater
}

// GetRenderer returns the renderer instance
func (r *NIMServiceReconciler) GetRenderer() render.Renderer {
	return r.renderer
}

func (r *NIMServiceReconciler) cleanupNIMService(ctx context.Context, nimService *appsv1alpha1.NIMService) error {
	// All dependent (owned) objects will be automatically garbage collected.
	// TODO: Handle any custom cleanup logic for the NIM microservice
	return nil
}

func (r *NIMServiceReconciler) reconcileNIMService(ctx context.Context, nimService *appsv1alpha1.NIMService) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	// Generate annotation for the current operator-version and apply to all resources
	// Get generic name for all resources
	namespacedName := types.NamespacedName{Name: nimService.GetName(), Namespace: nimService.GetNamespace()}

	renderer := r.GetRenderer()

	// Sync serviceaccount
	err := r.renderAndSyncResource(ctx, nimService, &renderer, &corev1.ServiceAccount{}, func() (client.Object, error) {
		return renderer.ServiceAccount(nimService.GetServiceAccountParams())
	}, "serviceaccount", conditions.ReasonServiceAccountFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync role
	err = r.renderAndSyncResource(ctx, nimService, &renderer, &rbacv1.Role{}, func() (client.Object, error) {
		return renderer.Role(nimService.GetRoleParams())
	}, "role", conditions.ReasonRoleFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync rolebinding
	err = r.renderAndSyncResource(ctx, nimService, &renderer, &rbacv1.RoleBinding{}, func() (client.Object, error) {
		return renderer.RoleBinding(nimService.GetRoleBindingParams())
	}, "rolebinding", conditions.ReasonRoleBindingFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync service
	err = r.renderAndSyncResource(ctx, nimService, &renderer, &corev1.Service{}, func() (client.Object, error) {
		return renderer.Service(nimService.GetServiceParams())
	}, "service", conditions.ReasonServiceFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync ingress
	if nimService.IsIngressEnabled() {
		err = r.renderAndSyncResource(ctx, nimService, &renderer, &networkingv1.Ingress{}, func() (client.Object, error) {
			return renderer.Ingress(nimService.GetIngressParams())
		}, "ingress", conditions.ReasonIngressFailed)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Sync HPA
	if nimService.IsAutoScalingEnabled() {
		err = r.renderAndSyncResource(ctx, nimService, &renderer, &autoscalingv2.HorizontalPodAutoscaler{}, func() (client.Object, error) {
			return renderer.HPA(nimService.GetHPAParams())
		}, "hpa", conditions.ReasonHPAFailed)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Sync Service Monitor
	if nimService.IsServiceMonitorEnabled() {
		err = r.renderAndSyncResource(ctx, nimService, &renderer, &monitoringv1.ServiceMonitor{}, func() (client.Object, error) {
			return renderer.ServiceMonitor(nimService.GetServiceMonitorParams())
		}, "servicemonitor", conditions.ReasonServiceMonitorFailed)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	deploymentParams := nimService.GetDeploymentParams()
	var modelPVC *appsv1alpha1.PersistentVolumeClaim
	modelProfile := ""

	// Select PVC for model store
	if nimService.GetNIMCacheName() != "" {
		// Fetch PVC for the associated NIMCache instance and mount it
		nimCachePVC, err := r.getNIMCachePVC(ctx, nimService)
		if err != nil {
			logger.Error(err, "unable to obtain pvc backing the nimcache instance")
			return ctrl.Result{}, err
		}
		logger.V(2).Info("obtained the backing pvc for nimcache instance", "pvc", nimCachePVC)
		modelPVC = nimCachePVC

		if profile := nimService.GetNIMCacheProfile(); profile != "" {
			logger.Info("overriding model profile", "profile", profile)
			modelProfile = profile
		}
	} else if nimService.Spec.Storage.PVC.Create != nil && *nimService.Spec.Storage.PVC.Create {
		// Create a new PVC
		modelPVC, err = r.reconcilePVC(ctx, nimService)
		if err != nil {
			logger.Error(err, "unable to create pvc")
			return ctrl.Result{}, err
		}
	} else if nimService.Spec.Storage.PVC.Name != "" {
		// Use an existing PVC
		modelPVC = &nimService.Spec.Storage.PVC
	} else {
		err := fmt.Errorf("neither external PVC name or NIMCache volume is provided")
		logger.Error(err, "failed to determine PVC for model-store")
		return ctrl.Result{}, err
	}
	// Setup volume mounts with model store
	deploymentParams.Volumes = nimService.GetVolumes(*modelPVC)
	deploymentParams.VolumeMounts = nimService.GetVolumeMounts(*modelPVC)

	// Setup env for explicit override profile is specified
	if modelProfile != "" {
		profileEnv := corev1.EnvVar{
			Name:  "NIM_MODEL_PROFILE",
			Value: modelProfile,
		}
		deploymentParams.Env = append(deploymentParams.Env, profileEnv)

		// TODO: assign GPU resources and node selector that is required for the selected profile
		// TODO: assign GPU resources
		// TODO: update the node selector
	}

	// Sync deployment
	err = r.renderAndSyncResource(ctx, nimService, &renderer, &appsv1.Deployment{}, func() (client.Object, error) {
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
		err = r.updater.SetConditionsNotReady(ctx, nimService, conditions.NotReady, msg)
	} else {
		// Update status as ready
		err = r.updater.SetConditionsReady(ctx, nimService, conditions.Ready, msg)
	}

	if err != nil {
		logger.Error(err, "Unable to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NIMServiceReconciler) renderAndSyncResource(ctx context.Context, nimService *appsv1alpha1.NIMService, renderer *render.Renderer, obj client.Object, renderFunc func() (client.Object, error), conditionType string, reason string) error {
	logger := log.FromContext(ctx)

	namespacedName := types.NamespacedName{Name: nimService.GetName(), Namespace: nimService.GetNamespace()}

	resource, err := renderFunc()
	if err != nil {
		logger.Error(err, "failed to render", conditionType, namespacedName)
		statusError := r.updater.SetConditionsFailed(ctx, nimService, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "nimservice", nimService.Name)
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

	if err = controllerutil.SetControllerReference(nimService, resource, r.GetScheme()); err != nil {
		logger.Error(err, "failed to set owner", conditionType, namespacedName)
		statusError := r.updater.SetConditionsFailed(ctx, nimService, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "nimservice", nimService.Name)
		}
		return err
	}

	err = r.syncResource(ctx, obj, resource, namespacedName)
	if err != nil {
		logger.Error(err, "failed to sync", conditionType, namespacedName)
		statusError := r.updater.SetConditionsFailed(ctx, nimService, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "nimservice", nimService.Name)
		}
		return err
	}
	return nil
}

// CheckDeploymentReadiness checks if the Deployment is ready
func (r *NIMServiceReconciler) isDeploymentReady(ctx context.Context, namespacedName *types.NamespacedName) (string, bool, error) {
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

func (r *NIMServiceReconciler) syncResource(ctx context.Context, obj client.Object, desired client.Object, namespacedName types.NamespacedName) error {
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

// getNIMCachePVC returns PVC backing the NIM cache instance
func (r *NIMServiceReconciler) getNIMCachePVC(ctx context.Context, nimService *appsv1alpha1.NIMService) (*appsv1alpha1.PersistentVolumeClaim, error) {
	logger := log.FromContext(ctx)

	if nimService.GetNIMCacheName() == "" {
		// NIM cache PVC is not used
		return nil, nil
	}
	// Lookup NIMCache instance in the same namespace as the NIMService instance
	nimCache := &appsv1alpha1.NIMCache{}
	if err := r.Get(ctx, types.NamespacedName{Name: nimService.GetNIMCacheName(), Namespace: nimService.Namespace}, nimCache); err != nil {
		logger.Error(err, "unable to fetch nimcache", "nimcache", nimService.GetNIMCacheName(), "nimservice", nimService.Name)
		return nil, err
	}
	// Get the status of NIMCache
	if nimCache.Status.State != appsv1alpha1.NimCacheStatusReady {
		return nil, fmt.Errorf("nimcache %s is not ready, nimservice %s", nimCache.GetName(), nimService.GetName())
	}

	if nimCache.Status.PVC == "" {
		return nil, fmt.Errorf("missing PVC for the nimcache instance %s, nimservice %s", nimCache.GetName(), nimService.GetName())
	}

	if nimCache.Spec.Storage.PVC.Name == "" {
		nimCache.Spec.Storage.PVC.Name = nimCache.Status.PVC
	}
	// Get the underlying PVC for the NIMCache instance
	return &nimCache.Spec.Storage.PVC, nil
}

func (r *NIMServiceReconciler) reconcilePVC(ctx context.Context, nimService *appsv1alpha1.NIMService) (*appsv1alpha1.PersistentVolumeClaim, error) {
	logger := r.GetLogger()
	pvcName := nimService.GetPVCName(nimService.Spec.Storage.PVC)
	pvcNamespacedName := types.NamespacedName{Name: pvcName, Namespace: nimService.GetNamespace()}
	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, pvcNamespacedName, pvc)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return nil, err
	}

	// If PVC does not exist, create a new one if creation flag is enabled
	if err != nil {
		if nimService.Spec.Storage.PVC.Create != nil && *nimService.Spec.Storage.PVC.Create {
			pvc, err = shared.ConstructPVC(nimService.Spec.Storage.PVC, metav1.ObjectMeta{Name: pvcName, Namespace: nimService.GetNamespace()})
			if err != nil {
				logger.Error(err, "Failed to construct pvc", "name", pvc.Name)
				return nil, err
			}
			if err := controllerutil.SetControllerReference(nimService, pvc, r.GetScheme()); err != nil {
				return nil, err
			}
			err = r.Create(ctx, pvc)
			if err != nil {
				logger.Error(err, "Failed to create pvc", "name", pvc.Name)
				return nil, err
			}
			logger.Info("Created PVC for NIM Service", "pvc", pvcName)

			conditions.UpdateCondition(&nimService.Status.Conditions, appsv1alpha1.NimCacheConditionPVCCreated, metav1.ConditionTrue, "PVCCreated", "The PVC has been created for storing NIM")
			nimService.Status.State = appsv1alpha1.NimCacheStatusPVCCreated
			if err := r.Status().Update(ctx, nimService); err != nil {
				logger.Error(err, "Failed to update status", "NIMService", nimService.Name)
				return nil, err
			}
		} else {
			logger.Error(err, "PVC doesn't exist and auto-creation is not enabled", "name", pvcNamespacedName)
			return nil, err
		}
	}
	return &nimService.Spec.Storage.PVC, nil
}
