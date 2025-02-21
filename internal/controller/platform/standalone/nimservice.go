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

	"github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
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
	apiResource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
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

// GetEventRecorder returns the event recorder
func (r *NIMServiceReconciler) GetEventRecorder() record.EventRecorder {
	return r.recorder
}

// GetOrchestratorType returns the container platform type
func (r *NIMServiceReconciler) GetOrchestratorType() k8sutil.OrchestratorType {
	return r.orchestratorType
}

func (r *NIMServiceReconciler) cleanupNIMService(ctx context.Context, nimService *appsv1alpha1.NIMService) error {
	// All dependent (owned) objects will be automatically garbage collected.
	// TODO: Handle any custom cleanup logic for the NIM microservice
	return nil
}

func (r *NIMServiceReconciler) reconcileNIMService(ctx context.Context, nimService *appsv1alpha1.NIMService) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var err error
	defer func() {
		if err != nil {
			r.GetEventRecorder().Eventf(nimService, corev1.EventTypeWarning, conditions.Failed,
				"NIMService %s failed, msg: %s", nimService.Name, err.Error())
		}
	}()
	// Generate annotation for the current operator-version and apply to all resources
	// Get generic name for all resources
	namespacedName := types.NamespacedName{Name: nimService.GetName(), Namespace: nimService.GetNamespace()}

	renderer := r.GetRenderer()

	// Sync serviceaccount
	err = r.renderAndSyncResource(ctx, nimService, &renderer, &corev1.ServiceAccount{}, func() (client.Object, error) {
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
	} else {
		err = r.cleanupResource(ctx, &networkingv1.Ingress{}, namespacedName)
		if err != nil && !errors.IsNotFound(err) {
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
	} else {
		// If autoscaling is disabled, ensure the HPA is deleted
		err = r.cleanupResource(ctx, &autoscalingv2.HorizontalPodAutoscaler{}, namespacedName)
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

	deploymentParams.OrchestratorType = string(r.GetOrchestratorType())

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
		err = fmt.Errorf("neither external PVC name or NIMCache volume is provided")
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

		// Retrieve and set profile details from NIMCache
		var profile *appsv1alpha1.NIMProfile
		profile, err = r.getNIMCacheProfile(ctx, nimService, modelProfile)
		if err != nil {
			logger.Error(err, "Failed to get cached NIM profile")
			return ctrl.Result{}, err
		}

		// Auto assign GPU resources in case of the optimized profile
		if profile != nil {
			if err = r.assignGPUResources(ctx, nimService, profile, deploymentParams); err != nil {
				return ctrl.Result{}, err
			}
		}

		// TODO: assign GPU resources and node selector that is required for the selected profile
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
		r.GetEventRecorder().Eventf(nimService, corev1.EventTypeNormal, conditions.NotReady,
			"NIMService %s not ready yet, msg: %s", nimService.Name, msg)
	} else {
		// Update NIMServiceStatus with model config.
		err = r.updateModelStatus(ctx, nimService)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Update status as ready
		err = r.updater.SetConditionsReady(ctx, nimService, conditions.Ready, msg)
		r.GetEventRecorder().Eventf(nimService, corev1.EventTypeNormal, conditions.Ready,
			"NIMService %s ready, msg: %s", nimService.Name, msg)
	}

	if err != nil {
		logger.Error(err, "Unable to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NIMServiceReconciler) updateModelStatus(ctx context.Context, nimService *v1alpha1.NIMService) error {
	modelName, err := r.getNIMModelName(ctx, nimService)
	if err != nil {
		return err
	}
	nimService.Status.Model = v1alpha1.ModelStatus{
		Name:            modelName,
		EndpointAddress: fmt.Sprintf("%s.%s.svc.cluster.local:%d", nimService.GetName(), nimService.GetNamespace(), nimService.GetServicePort()),
		Engine:          v1alpha1.ModelEngineNIM,
	}

	return nil
}

func (r *NIMServiceReconciler) getNIMModelName(ctx context.Context, nimService *appsv1alpha1.NIMService) (string, error) {
	logger := log.FromContext(ctx)

	if nimService.GetNIMCacheName() == "" {
		// NIM cache is not used
		return "", nil
	}

	// Lookup NIMCache instance in the same namespace as the NIMService instance
	nimCache := &appsv1alpha1.NIMCache{}
	if err := r.Get(ctx, types.NamespacedName{Name: nimService.GetNIMCacheName(), Namespace: nimService.Namespace}, nimCache); err != nil {
		logger.Error(err, "unable to fetch nimcache", "nimcache", nimService.GetNIMCacheName(), "nimservice", nimService.Name)
		return "", err
	}

	if nimCache.Spec.Source.DataStore != nil && nimCache.Spec.Source.DataStore.ModelName != nil {
		return *nimCache.Spec.Source.DataStore.ModelName, nil
	}

	return "", nil
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
func (r *NIMServiceReconciler) cleanupResource(ctx context.Context, obj client.Object, namespacedName types.NamespacedName) error {

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

	// If explicit name is not provided in the spec, update it with the one created
	if nimService.Spec.Storage.PVC.Name == "" {
		nimService.Spec.Storage.PVC.Name = pvc.Name
	}

	return &nimService.Spec.Storage.PVC, nil
}

// getNIMCacheProfile returns model profile info from the NIM cache instance
func (r *NIMServiceReconciler) getNIMCacheProfile(ctx context.Context, nimService *appsv1alpha1.NIMService, profile string) (*appsv1alpha1.NIMProfile, error) {
	logger := log.FromContext(ctx)

	if nimService.GetNIMCacheName() == "" {
		// NIM cache is not used
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

	for _, cachedProfile := range nimCache.Status.Profiles {
		if cachedProfile.Name == profile {
			return &cachedProfile, nil
		}
	}

	// If the specified profile is not cached, return nil
	return nil, nil
}

// getTensorParallelismByProfile returns the value of tensor parallelism parameter in the given NIM profile
func (r *NIMServiceReconciler) getTensorParallelismByProfile(ctx context.Context, profile *appsv1alpha1.NIMProfile) (string, error) {
	// List of possible keys for tensor parallelism
	possibleKeys := []string{"tensorParallelism", "tp"}

	tensorParallelism := ""

	// Iterate through possible keys and return the first valid value
	for _, key := range possibleKeys {
		if value, exists := profile.Config[key]; exists {
			tensorParallelism = value
			break
		}
	}

	return tensorParallelism, nil
}

// assignGPUResources automatically assigns GPU resources to the NIMService based on the provided profile,
// but retains any user-specified GPU resources if they are explicitly provided.
//
// This function retrieves the tensor parallelism (TP) value from the provided profile config to determine
// the number of GPUs to be allocated. If the TP value is defined and no GPU resources have been
// explicitly provided by the user, the function allocates GPUs according to the TP value.
// If the TP value is not present, the function defaults to allocating 1 GPU.
func (r *NIMServiceReconciler) assignGPUResources(ctx context.Context, nimService *appsv1alpha1.NIMService, profile *appsv1alpha1.NIMProfile, deploymentParams *rendertypes.DeploymentParams) error {
	logger := log.FromContext(ctx)

	// TODO: Make the resource name configurable
	const gpuResourceName = corev1.ResourceName("nvidia.com/gpu")

	// Check if the user has already provided a GPU resource quantity in the requests or limits
	if deploymentParams.Resources != nil {
		if _, gpuRequested := deploymentParams.Resources.Requests[gpuResourceName]; gpuRequested {
			logger.V(2).Info("User has provided GPU resource requests, skipping auto-assignment", "gpuResource", gpuResourceName)
			return nil
		}
		if _, gpuLimit := deploymentParams.Resources.Limits[gpuResourceName]; gpuLimit {
			logger.V(2).Info("User has provided GPU resource limits, skipping auto-assignment", "gpuResource", gpuResourceName)
			return nil
		}
	}

	// If no user-provided GPU resource is found, proceed with auto-assignment
	// Get tensorParallelism from the profile
	tensorParallelism, err := r.getTensorParallelismByProfile(ctx, profile)
	if err != nil {
		logger.Error(err, "Failed to retrieve tensorParallelism")
		return err
	}

	// Initialize the Resources field if not already initialized
	if deploymentParams.Resources == nil {
		deploymentParams.Resources = &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{},
			Limits:   corev1.ResourceList{},
		}
	}

	// Assign GPU resources based on tensorParallelism, or default to 1 GPU if tensorParallelism is not available
	gpuQuantity := apiResource.MustParse("1") // Default to 1 GPU

	if tensorParallelism != "" {
		gpuQuantity, err = apiResource.ParseQuantity(tensorParallelism)
		if err != nil {
			return fmt.Errorf("failed to parse tensorParallelism: %w", err)
		}

		logger.V(2).Info("Auto-assigning GPU resources based on tensorParallelism", "tensorParallelism", tensorParallelism, "gpuQuantity", gpuQuantity.String())
	} else {
		logger.V(2).Info("tensorParallelism not found, assigning 1 GPU by default", "Profile", profile.Name)
	}

	// Assign the GPU quantity for both requests and limits
	deploymentParams.Resources.Requests[gpuResourceName] = gpuQuantity
	deploymentParams.Resources.Limits[gpuResourceName] = gpuQuantity

	return nil
}
