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

package kserve

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/go-logr/logr"
	kservev1beta1 "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	kserveconstants "github.com/kserve/kserve/pkg/constants"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	knativeapis "knative.dev/pkg/apis"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	resourcev1 "k8s.io/api/resource/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	apiResource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
	"github.com/NVIDIA/k8s-nim-operator/internal/nimmodels"
	"github.com/NVIDIA/k8s-nim-operator/internal/render"
	rendertypes "github.com/NVIDIA/k8s-nim-operator/internal/render/types"
	"github.com/NVIDIA/k8s-nim-operator/internal/shared"
	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

const (
	// ManifestsDir is the directory to render k8s resource manifests.
	ManifestsDir = "/manifests"
)

// NIMServiceReconciler represents the NIMService reconciler instance for KServe platform.
type NIMServiceReconciler struct {
	client.Client
	scheme          *runtime.Scheme
	log             logr.Logger
	discoveryClient discovery.DiscoveryInterface

	updater          conditions.Updater
	renderer         render.Renderer
	recorder         record.EventRecorder
	orchestratorType k8sutil.OrchestratorType
}

// NewNIMServiceReconciler returns NIMServiceReconciler for KServe platform.
func NewNIMServiceReconciler(ctx context.Context, r shared.Reconciler) *NIMServiceReconciler {
	orchestratorType, _ := r.GetOrchestratorType(ctx)

	return &NIMServiceReconciler{
		Client:           r.GetClient(),
		scheme:           r.GetScheme(),
		log:              r.GetLogger(),
		discoveryClient:  r.GetDiscoveryClient(),
		updater:          r.GetUpdater(),
		renderer:         render.NewRenderer(ManifestsDir),
		recorder:         r.GetEventRecorder(),
		orchestratorType: orchestratorType,
	}
}

func (r *NIMServiceReconciler) GetRenderer() render.Renderer {
	return r.renderer
}

func (r *NIMServiceReconciler) cleanupNIMService(ctx context.Context, nimService *appsv1alpha1.NIMService) error {
	// All dependent (owned) objects will be automatically garbage collected.
	return nil
}

func (r *NIMServiceReconciler) reconcileNIMService(ctx context.Context, nimService *appsv1alpha1.NIMService) (ctrl.Result, error) {
	var err error
	defer func() {
		if err != nil {
			r.recorder.Eventf(nimService, corev1.EventTypeWarning, conditions.Failed,
				"NIMService %s failed, msg: %s", nimService.Name, err.Error())
		}
	}()

	// Validations.
	isValid, msg, err := r.validateDRAResources(ctx, nimService)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !isValid {
		err = r.updater.SetConditionsFailed(ctx, nimService, conditions.ReasonDRAResourcesUnsupported, msg)
		r.recorder.Eventf(nimService, corev1.EventTypeWarning, conditions.Failed, msg)
		return ctrl.Result{}, err
	}

	// Sync serviceaccount
	err = r.renderAndSyncResource(ctx, nimService, &corev1.ServiceAccount{}, func() (client.Object, error) {
		return r.renderer.ServiceAccount(nimService.GetServiceAccountParams())
	}, "serviceaccount", conditions.ReasonServiceAccountFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync role
	err = r.renderAndSyncResource(ctx, nimService, &rbacv1.Role{}, func() (client.Object, error) {
		return r.renderer.Role(nimService.GetRoleParams())
	}, "role", conditions.ReasonRoleFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync rolebinding
	err = r.renderAndSyncResource(ctx, nimService, &rbacv1.RoleBinding{}, func() (client.Object, error) {
		return r.renderer.RoleBinding(nimService.GetRoleBindingParams())
	}, "rolebinding", conditions.ReasonRoleBindingFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	var modelPVC *appsv1alpha1.PersistentVolumeClaim
	var modelProfile string
	var nimCache *appsv1alpha1.NIMCache
	modelPVC, modelProfile, nimCache, err = r.renderAndSyncCache(ctx, nimService)
	if err != nil {
		return ctrl.Result{}, err
	} else if nimCache == nil {
		return ctrl.Result{}, nil
	}

	var deploymentMode kserveconstants.DeploymentModeType
	// Check KServe deployment mode
	namespacedName := client.ObjectKeyFromObject(nimService)
	deploymentMode, err = utils.GetKServeDeploymentMode(ctx, r.Client, nimService.Spec.Annotations, &namespacedName)
	if err != nil {
		return ctrl.Result{}, err
	}

	if utils.IsKServeStandardDeploymentMode(deploymentMode) {
		// Sync Service Monitor
		if nimService.IsServiceMonitorEnabled() {
			err = r.renderAndSyncResource(ctx, nimService, &monitoringv1.ServiceMonitor{}, func() (client.Object, error) {
				return r.renderer.ServiceMonitor(nimService.GetServiceMonitorParams())
			}, "servicemonitor", conditions.ReasonServiceMonitorFailed)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	err = r.renderAndSyncInferenceService(ctx, nimService, modelPVC, modelProfile, nimCache, deploymentMode)
	if err != nil {
		return ctrl.Result{}, err
	}

	var result *ctrl.Result
	result, err = r.checkInferenceServiceStatus(ctx, nimService, deploymentMode)

	if err != nil {
		r.log.Error(err, "Unable to update status")
		return ctrl.Result{}, err
	} else {
		return *result, err
	}
}

func (r *NIMServiceReconciler) renderAndSyncResource(ctx context.Context, nimService *appsv1alpha1.NIMService,
	obj client.Object, renderFunc func() (client.Object, error), conditionType string, reason string) error {
	logger := r.log

	resource, err := renderFunc()
	if err != nil {
		logger.Error(err, "failed to render", "conditionType", conditionType)
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

	namespacedName := types.NamespacedName{Name: resource.GetName(), Namespace: resource.GetNamespace()}

	err = r.Get(ctx, namespacedName, obj)
	if err != nil && !k8serrors.IsNotFound(err) {
		logger.Error(err, fmt.Sprintf("Error is not NotFound for %s: %v", obj.GetObjectKind(), err))
		return err
	}
	// Don't do anything if CR is unchanged.
	if err == nil && !utils.IsParentSpecChanged(obj, utils.DeepHashObject(nimService.Spec)) {
		return nil
	}

	if err = controllerutil.SetControllerReference(nimService, resource, r.scheme); err != nil {
		logger.Error(err, "failed to set owner", conditionType, namespacedName)
		statusError := r.updater.SetConditionsFailed(ctx, nimService, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "nimservice", nimService.Name)
		}
		return err
	}

	err = k8sutil.SyncResource(ctx, r.Client, obj, resource)
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

func (r *NIMServiceReconciler) renderAndSyncCache(ctx context.Context,
	nimService *appsv1alpha1.NIMService) (*appsv1alpha1.PersistentVolumeClaim, string, *appsv1alpha1.NIMCache, error) {
	logger := r.log

	var modelPVC *appsv1alpha1.PersistentVolumeClaim
	modelProfile := ""

	// Select PVC for model store
	nimCacheName := nimService.GetNIMCacheName()
	nimCache := &appsv1alpha1.NIMCache{}
	if nimCacheName != "" { // nolint:gocritic
		if err := r.Get(ctx, types.NamespacedName{Name: nimCacheName, Namespace: nimService.GetNamespace()}, nimCache); err != nil {
			// Fail the NIMService if the NIMCache is not found
			if k8serrors.IsNotFound(err) {
				msg := fmt.Sprintf("NIMCache %s not found", nimCacheName)
				statusUpdateErr := r.updater.SetConditionsFailed(ctx, nimService, conditions.ReasonNIMCacheNotFound, msg)
				r.recorder.Eventf(nimService, corev1.EventTypeWarning, conditions.Failed, msg)
				logger.Info(msg, "nimcache", nimCacheName, "nimservice", nimService.Name)
				if statusUpdateErr != nil {
					logger.Error(statusUpdateErr, "failed to update status", "nimservice", nimService.Name)
					return nil, "", nil, statusUpdateErr
				}
				return nil, "", nil, nil
			}
			return nil, "", nil, err
		}

		switch nimCache.Status.State {
		case appsv1alpha1.NimCacheStatusReady:
			logger.V(4).Info("NIMCache is ready", "nimcache", nimCacheName, "nimservice", nimService.Name)
		case appsv1alpha1.NimCacheStatusFailed:
			var msg string
			cond := meta.FindStatusCondition(nimCache.Status.Conditions, conditions.Failed)
			if cond != nil && cond.Status == metav1.ConditionTrue {
				msg = cond.Message
			} else {
				msg = ""
			}
			err := r.updater.SetConditionsFailed(ctx, nimService, conditions.ReasonNIMCacheFailed, msg)
			r.recorder.Eventf(nimService, corev1.EventTypeWarning, conditions.Failed, msg)
			logger.Info(msg, "nimcache", nimCacheName, "nimservice", nimService.Name)
			if err != nil {
				logger.Error(err, "failed to update status", "nimservice", nimService.Name)
			}
			return nil, "", nil, err
		default:
			msg := fmt.Sprintf("NIMCache %s not ready", nimCacheName)
			err := r.updater.SetConditionsNotReady(ctx, nimService, conditions.ReasonNIMCacheNotReady, msg)
			r.recorder.Eventf(nimService, corev1.EventTypeNormal, conditions.NotReady,
				"NIMService %s not ready yet, msg: %s", nimService.Name, msg)
			logger.V(4).Info(msg, "nimservice", nimService.Name)
			if err != nil {
				logger.Error(err, "failed to update status", "nimservice", nimService.Name)
			}
			return nil, "", nil, err
		}

		// Fetch PVC for the associated NIMCache instance and mount it
		if nimCache.Status.PVC == "" {
			err := fmt.Errorf("missing PVC for the nimcache instance %s", nimCache.GetName())
			logger.Error(err, "unable to obtain pvc backing the nimcache instance")
			return nil, "", nil, err
		}
		if nimCache.Spec.Storage.PVC.Name == "" {
			nimCache.Spec.Storage.PVC.Name = nimCache.Status.PVC
		}
		// Get the underlying PVC for the NIMCache instance
		modelPVC = &nimCache.Spec.Storage.PVC
		logger.V(2).Info("obtained the backing pvc for nimcache instance", "pvc", modelPVC)

		if profile := nimService.GetNIMCacheProfile(); profile != "" {
			logger.Info("overriding model profile", "profile", profile)
			modelProfile = profile
		}
	} else if nimService.Spec.Storage.PVC.Create != nil && *nimService.Spec.Storage.PVC.Create {
		// Create a new PVC
		var err error
		modelPVC, err = r.reconcilePVC(ctx, nimService)
		if err != nil {
			logger.Error(err, "unable to create pvc")
			return nil, "", nil, err
		}
	} else if nimService.Spec.Storage.PVC.Name != "" {
		// Use an existing PVC
		modelPVC = &nimService.Spec.Storage.PVC
	} else if nimService.Spec.Storage.EmptyDir != nil {
		modelPVC = nil
	} else if nimService.Spec.Storage.HostPath != nil && *nimService.Spec.Storage.HostPath != "" {
		modelPVC = nil
	} else {
		err := fmt.Errorf("neither external PVC name, NIMCache volume, empty dir or local host path should be provided")
		logger.Error(err, "failed to determine PVC , NIMCache volume, empty dir or local host path for model-store")
		return nil, "", nil, err
	}

	return modelPVC, modelProfile, nimCache, nil
}

func (r *NIMServiceReconciler) reconcilePVC(ctx context.Context, nimService *appsv1alpha1.NIMService) (*appsv1alpha1.PersistentVolumeClaim, error) {
	logger := r.log

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
				logger.Error(err, "Failed to construct pvc", "name", pvcName)
				return nil, err
			}
			if err := controllerutil.SetControllerReference(nimService, pvc, r.scheme); err != nil {
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

func (r *NIMServiceReconciler) renderAndSyncInferenceService(ctx context.Context,
	nimService *appsv1alpha1.NIMService, modelPVC *appsv1alpha1.PersistentVolumeClaim, modelProfile string,
	nimCache *appsv1alpha1.NIMCache, deploymentMode kserveconstants.DeploymentModeType) error {

	logger := r.log

	var profileEnv *[]corev1.EnvVar
	var profile *appsv1alpha1.NIMProfile
	var gpuResources *corev1.ResourceRequirements
	var initContainers []corev1.Container
	var renderFunc func() (client.Object, error)
	var conType, failedCon string
	var renderObj client.Object

	// Setup env for explicit override profile is specified
	if modelProfile != "" {
		profileEnv = &[]corev1.EnvVar{
			{
				Name:  "NIM_MODEL_PROFILE",
				Value: modelProfile,
			},
		}

		// Only assign GPU resources if the NIMCache is for optimized NIM
		if nimCache.IsOptimizedNIM() {
			// Retrieve and set profile details from NIMCache
			var err error
			profile, err = r.getNIMCacheProfile(ctx, nimService, modelProfile)
			if err != nil {
				logger.Error(err, "Failed to get cached NIM profile")
				return err
			}

			// Auto assign GPU resources in case of the optimized profile
			if profile != nil {
				gpuResources, err = r.addGPUResources(ctx, nimService, profile)
				if err != nil {
					logger.Error(err, "Failed to get GPU resources")
					return err
				}
			}
		}

		// TODO: assign GPU resources and node selector that is required for the selected profile
	}

	initContainers = nimService.GetInitContainers()
	namedDraResources, err := shared.NewNamedDRAResourceList(ctx, r.Client, nimService)
	if err != nil {
		logger.Error(err, "Failed to get named dra resources")
		return err
	}
	err = r.reconcileDRAResources(ctx, nimService, namedDraResources)
	if err != nil {
		logger.Error(err, "Failed to reconcile DRAResources")
		return err
	}

	isvcParams := nimService.GetInferenceServiceParams(deploymentMode)
	isvcParams.DeploymentMode = string(deploymentMode)

	// Setup metrics exporting
	isvcParams.Annotations[kserveconstants.EnableMetricAggregation] = "true"
	isvcParams.Annotations[kserveconstants.SetPrometheusAnnotation] = "true"

	// Ensure deployment mode annotation is always set
	if _, ok := isvcParams.Annotations[kserveconstants.DeploymentMode]; !ok {
		isvcParams.Annotations[kserveconstants.DeploymentMode] = string(deploymentMode)
	}

	// Sync ingress
	// Only if network visibility is not explicitly configured
	if _, hasVisibility := isvcParams.Labels[kserveconstants.NetworkVisibility]; !hasVisibility {
		// User has not explicitly set visibility
		if !nimService.IsIngressEnabled() {
			isvcParams.Labels[kserveconstants.NetworkVisibility] = kserveconstants.ClusterLocalVisibility
		}
	}

	isvcParams.OrchestratorType = string(r.orchestratorType)

	isvcParams.PodResourceClaims = namedDraResources.GetPodResourceClaims()
	if nimCache.IsUniversalNIM() {
		isvcParams.Env = utils.MergeEnvVars([]corev1.EnvVar{
			{
				Name:  "NIM_MODEL_NAME",
				Value: utils.DefaultModelStorePath,
			},
		}, isvcParams.Env)
	}
	// If NIMCache or NIMService is a Hugging Face Multi-LLM NIM, add the HF_TOKEN to the environment variables
	if nimCache.IsHFModel() || nimService.IsHFModel() {
		isvcParams.Env = utils.RemoveEnvVar(isvcParams.Env, appsv1alpha1.NGCAPIKey)
		isvcParams.Env = utils.MergeEnvVars(isvcParams.Env, []corev1.EnvVar{
			{
				Name: appsv1alpha1.HFToken,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: nimService.Spec.AuthSecret,
						},
						Key: appsv1alpha1.HFToken,
					},
				},
			},
		})
	}

	// Setup volume mounts with model store
	isvcParams.Volumes = nimService.GetVolumes(modelPVC)
	isvcParams.VolumeMounts = nimService.GetVolumeMounts(modelPVC)
	if profileEnv != nil {
		isvcParams.Env = utils.MergeEnvVars(*profileEnv, isvcParams.Env)
	}
	// Auto assign GPU resources in case of the optimized profile
	if gpuResources != nil {
		isvcParams.Resources = gpuResources
	}
	renderFunc = func() (client.Object, error) {
		result, err := r.renderer.InferenceService(isvcParams)
		if err != nil {
			return nil, err
		}
		if len(initContainers) > 0 {
			result.Spec.Predictor.InitContainers = initContainers
		}
		// Update Container resources with DRA resource claims.
		namedDraResources.UpdateContainerResourceClaims(result.Spec.Predictor.Containers)
		return result, nil
	}
	conType = "InferenceService"
	failedCon = conditions.ReasonDeploymentFailed
	renderObj = &kservev1beta1.InferenceService{}

	err = r.renderAndSyncResource(ctx, nimService, renderObj, renderFunc, conType, failedCon)
	if err != nil {
		return err
	}

	return nil
}

// getNIMCacheProfile returns model profile info from the NIM cache instance.
func (r *NIMServiceReconciler) getNIMCacheProfile(ctx context.Context, nimService *appsv1alpha1.NIMService, profile string) (*appsv1alpha1.NIMProfile, error) {
	logger := r.log

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

// addGPUResources automatically assigns GPU resources to the NIMService based on the provided profile,
// but retains any user-specified GPU resources if they are explicitly provided.
//
// In case of monolithic NIMs, this function retrieves the tensor parallelism (TP) value from the provided profile config to determine
// the number of GPUs to be allocated. If the TP value is defined and no GPU resources have been
// explicitly provided by the user, the function allocates GPUs according to the TP value.
// If the TP value is not present, the function defaults to allocating 1 GPU.
//
// In case of multi-node NIMs, this function assigns the number of GPUs equal to .spec.multiNode.gpuPerWorker.
func (r *NIMServiceReconciler) addGPUResources(ctx context.Context, nimService *appsv1alpha1.NIMService, profile *appsv1alpha1.NIMProfile) (*corev1.ResourceRequirements, error) {
	logger := log.FromContext(ctx)

	// TODO: Refine this to detect GPU claims and only assign GPU resources if no GPU claims are present.
	if len(nimService.Spec.DRAResources) > 0 {
		logger.Info("DRAResources found, skipping GPU resource assignment", "DRAResources", nimService.Spec.DRAResources)
		return nil, nil
	}

	// TODO: Make the resource name configurable
	const gpuResourceName = corev1.ResourceName("nvidia.com/gpu")

	// Check if the user has already provided a GPU resource quantity in the requests or limits
	if nimService.Spec.Resources != nil {
		if _, gpuRequested := nimService.Spec.Resources.Requests[gpuResourceName]; gpuRequested {
			logger.V(2).Info("User has provided GPU resource requests, skipping auto-assignment", "gpuResource", gpuResourceName)
			return nimService.Spec.Resources, nil
		}
		if _, gpuLimit := nimService.Spec.Resources.Limits[gpuResourceName]; gpuLimit {
			logger.V(2).Info("User has provided GPU resource limits, skipping auto-assignment", "gpuResource", gpuResourceName)
			return nimService.Spec.Resources, nil
		}
	}

	gpuQuantity := apiResource.MustParse("1")
	var err error
	// if deployed as multi-node, use the GPU per worker value to assign GPU resources to each worker
	// TODO auto determine base on tp*pp/(.spec.multiNode.size)
	if nimService.Spec.MultiNode != nil {
		gpuQuantity, err = apiResource.ParseQuantity(fmt.Sprintf("%d", nimService.GetMultiNodeTensorParallelism()))
		if err != nil {
			logger.Error(err, "Failed to parse GPU per worker value")
			return nil, err
		}
	} else {
		// If no user-provided GPU resource is found, proceed with auto-assignment
		// Get tensorParallelism from the profile
		tensorParallelism, err := utils.GetTensorParallelismByProfileTags(profile.Config)
		if err != nil {
			logger.Error(err, "Failed to retrieve tensorParallelism")
			return nil, err
		}
		if tensorParallelism != "" {
			gpuQuantity, err = apiResource.ParseQuantity(tensorParallelism)
			if err != nil {
				logger.Error(err, "Failed to parse tensorParallelism")
				return nil, err
			}
		}
	}

	var resources *corev1.ResourceRequirements
	if nimService.Spec.Resources != nil {
		resources = nimService.Spec.Resources
	} else {
		resources = &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{},
			Limits:   corev1.ResourceList{},
		}
	}

	resources.Requests[gpuResourceName] = gpuQuantity
	resources.Limits[gpuResourceName] = gpuQuantity
	return resources, nil
}

func (r *NIMServiceReconciler) checkInferenceServiceStatus(ctx context.Context, nimService *appsv1alpha1.NIMService,
	deploymentMode kserveconstants.DeploymentModeType) (*ctrl.Result, error) {
	logger := r.log

	// Check if InferenceService is ready
	msg, ready, err := r.isInferenceServiceReady(ctx, nimService)
	if err != nil {
		return &ctrl.Result{}, err
	}

	namedDraResources, err := shared.NewNamedDRAResourceList(ctx, r.Client, nimService)
	if err != nil {
		logger.Error(err, "Failed to get named dra resources")
		return &ctrl.Result{}, err
	}
	if len(namedDraResources.Resources) > 0 {
		// Update NIMServiceStatus with resource claims.
		updateErr := r.updateResourceClaimStatus(ctx, nimService, namedDraResources)
		if updateErr != nil {
			logger.Info("WARN: Resource claim status update failed, will retry in 5 seconds", "error", updateErr.Error())
			return &ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
	}

	// TODO: Rework NIMService Status to split into MODIFY and APPLY phases for better readability
	// (Currently we're using `updater.SetConditions*` to implicitly take all previous changes and
	// apply them along with the conditions.)
	if !ready {
		// Update status as NotReady
		err = r.updater.SetConditionsNotReady(ctx, nimService, conditions.NotReady, msg)
		r.recorder.Eventf(nimService, corev1.EventTypeNormal, conditions.NotReady,
			"NIMService %s not ready yet, msg: %s", nimService.Name, msg)
	} else {
		// Update NIMServiceStatus with model config.
		updateErr := r.updateModelStatus(ctx, nimService, deploymentMode)
		if updateErr != nil {
			logger.Info("WARN: Model status update failed, will retry in 5 seconds", "error", updateErr.Error())
			return &ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		// Update status as ready
		err = r.updater.SetConditionsReady(ctx, nimService, conditions.Ready, msg)
		r.recorder.Eventf(nimService, corev1.EventTypeNormal, conditions.Ready,
			"NIMService %s ready, msg: %s", nimService.Name, msg)
	}

	if err != nil {
		logger.Error(err, "failed to update status", "nimservice", nimService.Name, "state", conditions.Ready)
		return &ctrl.Result{}, err
	}

	return &ctrl.Result{}, nil
}

// isInferenceServiceReady checks if the InferenceService is ready.
func (r *NIMServiceReconciler) isInferenceServiceReady(ctx context.Context, nimService *appsv1alpha1.NIMService) (string, bool, error) {
	logger := r.log

	isvc := &kservev1beta1.InferenceService{}
	err := r.Get(ctx, client.ObjectKey{Name: nimService.Name, Namespace: nimService.Namespace}, isvc)
	if err != nil {
		logger.Error(err, "failed to fetch inferenceservice")
		if k8serrors.IsNotFound(err) {
			return fmt.Sprintf("Waiting for InferenceService %q creation", nimService.Name), false, nil
		}
		return "", false, err
	}

	var cond *knativeapis.Condition
	for i := range isvc.Status.Conditions {
		c := isvc.Status.Conditions[i]
		if c.Type == kservev1beta1.PredictorReady {
			cond = &c
			break
		}
	}
	if cond != nil {
		return cond.GetMessage(), cond.IsTrue(), nil
	}
	return fmt.Sprintf("Waiting for InferenceService %q reporting Predictor readiness", isvc.Name), false, nil
}

func (r *NIMServiceReconciler) updateModelStatus(ctx context.Context, nimService *appsv1alpha1.NIMService,
	deploymentMode kserveconstants.DeploymentModeType) error {
	clusterEndpoint, externalEndpoint, err := r.getNIMModelEndpoints(ctx, nimService, deploymentMode)
	if err != nil {
		return err
	}

	// KServe RawDeployment mode creates headless services (clusterIP: None) by default, which prevents
	// standard service-based access for model endpoints. To enable regular ClusterIP services:
	//   - Upstream KServe: Set "serviceClusterIPNone: false" in the "deploy" section of the
	//     "inferenceservice-config" ConfigMap (in the KServe controller namespace)
	//   - OpenDataHub: Set "rawDeploymentServiceConfig: Headed" (not "Headless") in the
	//     kserve spec of the DataScienceCluster object
	modelName, err := r.getNIMModelName(ctx, clusterEndpoint)
	if err != nil {
		return err
	}
	nimService.Status.Model = &appsv1alpha1.ModelStatus{
		Name:             modelName,
		ClusterEndpoint:  clusterEndpoint,
		ExternalEndpoint: externalEndpoint,
	}

	return nil
}

func (r *NIMServiceReconciler) getNIMModelEndpoints(ctx context.Context, nimService *appsv1alpha1.NIMService,
	deploymentMode kserveconstants.DeploymentModeType) (string, string, error) {
	logger := r.log

	isvc := &kservev1beta1.InferenceService{}
	err := r.Get(ctx, client.ObjectKey{Name: nimService.Name, Namespace: nimService.Namespace}, isvc)
	if err != nil {
		logger.Error(err, "unable to fetch k8s service", "nimservice", nimService.GetName())
		return "", "", err
	}

	var externalEndpoint string
	if isvc.Status.URL != nil {
		externalEndpoint = isvc.Status.URL.String()
	} else {
		err := fmt.Errorf("external endpoint not available, nimservice %s", nimService.GetName())
		logger.Error(err, "unable to get external endpoint", "nimservice", nimService.GetName(), "inferenceservice", isvc.GetName())
		return "", "", err
	}

	var clusterEndpoint string
	if isvc.Status.Address != nil && isvc.Status.Address.URL != nil {
		clusterEndpoint = isvc.Status.Address.URL.String()
	} else {
		err := fmt.Errorf("cluster endpoint not available, nimservice %s", nimService.GetName())
		logger.Error(err, "unable to get cluster endpoint", "nimservice", nimService.GetName(), "inferenceservice", isvc.GetName())
		return "", "", err
	}

	return clusterEndpoint, externalEndpoint, nil
}

func (r *NIMServiceReconciler) getNIMModelName(ctx context.Context, nimServiceEndpoint string) (string, error) {
	logger := log.FromContext(ctx)

	// List nimservice /v1/models endpoint.
	modelsList, err := nimmodels.ListModelsV1(ctx, nimServiceEndpoint, "")
	if err != nil {
		logger.Error(err, "Failed to list models", "endpoint", nimServiceEndpoint)
		// Check if err is an HTTP error with a status code
		if nimmodels.IsNotFound(err) {
			// The endpoint does not exist
			logger.V(2).Error(err, "URI does not exist", "uri", nimmodels.ModelsV1URI, "endpoint", nimServiceEndpoint)
			metadata, err := nimmodels.GetMetadataV1(ctx, nimServiceEndpoint, "")
			if err != nil {
				logger.Error(err, "Failed to get metadata", "endpoint", nimServiceEndpoint)
				if nimmodels.IsNotFound(err) {
					logger.V(2).Error(err, "URI does not exist", "uri", nimmodels.MetadataV1URI, "endpoint", nimServiceEndpoint)
					return "", nil
				}
				return "", err
			}
			modelName, err := getModelNameFromMetadata(metadata)
			if err != nil {
				logger.V(2).Error(err, "Failed to get model name from metadata", "endpoint", nimServiceEndpoint)
				return "", nil
			}

			return modelName, nil
		}

		return "", err
	}

	modelName, err := getModelNameFromModelsList(modelsList)
	if err != nil {
		logger.V(2).Error(err, "Failed to get model name from models list", "endpoint", nimServiceEndpoint)
		return "", nil
	}
	return modelName, nil
}

// TODO: Move to validation webhook.
func (r *NIMServiceReconciler) validateDRAResources(ctx context.Context, nimService *appsv1alpha1.NIMService) (bool, string, error) {
	logger := log.FromContext(ctx)

	if len(nimService.Spec.DRAResources) == 0 {
		return true, "", nil
	}

	// Check if the cluster version is supported for DRA resources
	clusterVersion, err := k8sutil.GetClusterVersion(r.discoveryClient)
	if err != nil {
		logger.Error(err, "failed to get cluster version")
		return false, "", err
	}
	if !utils.IsVersionGreaterThanOrEqual(clusterVersion, utils.MinSupportedClusterVersionForDRA) {
		msg := fmt.Sprintf("DRA resources are not supported by NIM-Operator on this cluster, please upgrade to k8s version '%s' or higher", utils.MinSupportedClusterVersionForDRA)
		logger.Error(errors.New(msg), msg, "nimService", nimService.Name)
		return false, msg, nil
	}

	// Check if duplicate resource claim names are provided
	resourceClaimNames := make(map[string]bool)
	for idx, resource := range nimService.Spec.DRAResources {
		if resource.ResourceClaimName == nil {
			continue
		}

		if _, ok := resourceClaimNames[*resource.ResourceClaimName]; ok {
			msg := fmt.Sprintf("spec.draResources[%d].resourceClaimName: duplicate resource claim name: '%s'", idx, *resource.ResourceClaimName)
			logger.Error(errors.New(msg), msg, "nimService", nimService.Name)
			return false, msg, nil
		}
		resourceClaimNames[*resource.ResourceClaimName] = true
	}

	// Check AttributeSelector version values.
	for idx, resource := range nimService.Spec.DRAResources {
		if resource.ClaimCreationSpec == nil {
			continue
		}
		for deviceIdx, device := range resource.ClaimCreationSpec.Devices {
			for attributeIdx, attribute := range device.AttributeSelectors {
				if attribute.Value.VersionValue == nil {
					continue
				}
				_, err := semver.Parse(*attribute.Value.VersionValue)
				if err != nil {
					msg := fmt.Sprintf("spec.draResources[%d].claimCreationSpec.devices[%d].attributeSelectors[%d].value.versionValue.version: invalid version %q: %v", idx, deviceIdx, attributeIdx, *attribute.Value.VersionValue, err)
					logger.Error(err, msg, "nimService", nimService.Name)
					return false, msg, nil
				}
			}
		}
	}
	return true, "", nil
}

func (r *NIMServiceReconciler) updateResourceClaimStatus(ctx context.Context, nimService *appsv1alpha1.NIMService, namedDraResources *shared.NamedDRAResourceList) error {
	logger := log.FromContext(ctx)

	draResourceStatuses, err := namedDraResources.GenerateDRAResourceStatuses(ctx, r.Client, nimService.GetNamespace())
	if err != nil {
		logger.Error(err, "Failed to generate DRA resource statuses", "nimservice", nimService.Name)
		return err
	}

	nimService.Status.DRAResourceStatuses = draResourceStatuses
	return nil
}

func getModelNameFromMetadata(metadata *nimmodels.MetadataV1) (string, error) {
	if len(metadata.ModelInfo) == 0 {
		return "", fmt.Errorf("no model info found")
	}
	return strings.Split(metadata.ModelInfo[0].ShortName, ":")[0], nil
}

func getModelNameFromModelsList(modelsList *nimmodels.ModelsV1List) (string, error) {
	if modelsList.Object != nimmodels.ObjectTypeList {
		return "", fmt.Errorf("unexpected object type: %s", modelsList.Object)
	}

	if len(modelsList.Data) == 0 {
		return "", fmt.Errorf("no models found")
	}

	if len(modelsList.Data) == 1 {
		return modelsList.Data[0].Id, nil
	}

	for _, model := range modelsList.Data {
		if model.Object != nimmodels.ObjectTypeModel {
			continue
		}
		if model.Root != nil && *model.Root == model.Id {
			return model.Id, nil
		}
	}

	return "", fmt.Errorf("no valid model found")
}

func (r *NIMServiceReconciler) reconcileDRAResources(ctx context.Context, nimService *appsv1alpha1.NIMService, namedDraResources *shared.NamedDRAResourceList) error {
	for _, namedDraResource := range namedDraResources.Resources {
		if !shared.ShouldCreateDRAResource(namedDraResource.DRAResource) {
			continue
		}

		labels := nimService.GetServiceLabels()
		annotations := nimService.GetNIMServiceAnnotations()
		claimAnnotations := nimService.GetNIMServiceAnnotations()
		delete(claimAnnotations, utils.NvidiaAnnotationParentSpecHashKey)
		// Sync ResourceClaimTemplate
		err := r.renderAndSyncResource(ctx, nimService, &resourcev1.ResourceClaimTemplate{}, func() (client.Object, error) {
			resourceClaimTemplateParams := &rendertypes.ResourceClaimTemplateParams{
				Name:             namedDraResource.ResourceName,
				Namespace:        nimService.GetNamespace(),
				Labels:           labels,
				Annotations:      annotations,
				ClaimAnnotations: claimAnnotations,
			}
			for _, device := range namedDraResource.ClaimCreationSpec.Devices {
				exprs, err := shared.GetDRADeviceCELExpressions(device)
				if err != nil {
					return nil, err
				}
				resourceClaimTemplateParams.Devices = append(resourceClaimTemplateParams.Devices, rendertypes.DRADeviceParams{
					Name:            device.Name,
					Count:           device.Count,
					DeviceClassName: device.DeviceClassName,
					CELExpressions:  exprs,
				})
			}
			return r.renderer.ResourceClaimTemplate(resourceClaimTemplateParams)
		}, "resourceclaimtemplate", conditions.ReasonResourceClaimTemplateFailed)

		if err != nil {
			return fmt.Errorf("failed to reconcile DRAResource %s: %w", namedDraResource.ResourceName, err)
		}
	}
	return nil
}
