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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/go-logr/logr"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	resourcev1beta2 "k8s.io/api/resource/v1beta2"
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
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	lws "sigs.k8s.io/lws/api/leaderworkerset/v1"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
	"github.com/NVIDIA/k8s-nim-operator/internal/nimmodels"
	"github.com/NVIDIA/k8s-nim-operator/internal/render"
	rendertypes "github.com/NVIDIA/k8s-nim-operator/internal/render/types"
	"github.com/NVIDIA/k8s-nim-operator/internal/shared"
	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

// GetScheme returns the scheme of the reconciler.
func (r *NIMServiceReconciler) GetScheme() *runtime.Scheme {
	return r.scheme
}

// GetLogger returns the logger of the reconciler.
func (r *NIMServiceReconciler) GetLogger() logr.Logger {
	return r.log
}

// GetClient returns the client instance.
func (r *NIMServiceReconciler) GetClient() client.Client {
	return r.Client
}

// GetUpdater returns the conditions updater instance.
func (r *NIMServiceReconciler) GetUpdater() conditions.Updater {
	return r.updater
}

func (r *NIMServiceReconciler) GetDiscoveryClient() discovery.DiscoveryInterface {
	return r.discoveryClient
}

// GetRenderer returns the renderer instance.
func (r *NIMServiceReconciler) GetRenderer() render.Renderer {
	return r.renderer
}

// GetEventRecorder returns the event recorder.
func (r *NIMServiceReconciler) GetEventRecorder() record.EventRecorder {
	return r.recorder
}

// GetOrchestratorType returns the container platform type.
func (r *NIMServiceReconciler) GetOrchestratorType() k8sutil.OrchestratorType {
	return r.orchestratorType
}

func (r *NIMServiceReconciler) cleanupNIMService(ctx context.Context, nimService *appsv1alpha1.NIMService) error {
	// All dependent (owned) objects will be automatically garbage collected.
	// TODO: Handle any custom cleanup logic for the NIM microservice
	return nil
}

// TODO: Move to validation webhook.
func (r *NIMServiceReconciler) validateDRAResources(ctx context.Context, nimService *appsv1alpha1.NIMService) (bool, string, error) {
	logger := log.FromContext(ctx)

	if len(nimService.Spec.DRAResources) == 0 {
		return true, "", nil
	}

	// Check if the cluster version is supported for DRA resources
	clusterVersion, err := k8sutil.GetClusterVersion(r.GetDiscoveryClient())
	if err != nil {
		logger.Error(err, "failed to get cluster version")
		return false, "", err
	}
	if !utils.IsVersionGreaterThanOrEqual(clusterVersion, utils.MinSupportedClusterVersionForDRA) {
		msg := fmt.Sprintf("DRA resources are not supported by NIM-Operator on this cluster, please upgrade to k8s version '%s' or higher", utils.MinSupportedClusterVersionForDRA)
		logger.Error(errors.New(msg), msg, "nimService", nimService.Name)
		return false, msg, nil
	}

	// Check if the resource claim CRD exists
	crdExists, err := k8sutil.CRDExists(r.GetDiscoveryClient(), resourcev1beta2.SchemeGroupVersion.WithResource("resourceclaims"))
	if err != nil {
		logger.Error(err, "failed to check if resource claim CRD exists")
		return false, "", err
	}
	if !crdExists {
		msg := "DRA resources are not supported by NIM-Operator on this cluster, please ensure resource.k8s.io/v1beta2 API group is enabled"
		logger.Error(fmt.Errorf("%s", msg), msg, "nimService", nimService.Name)
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

	// Validations.
	isValid, msg, err := r.validateDRAResources(ctx, nimService)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !isValid {
		err = r.updater.SetConditionsFailed(ctx, nimService, conditions.ReasonDRAResourcesUnsupported, msg)
		r.GetEventRecorder().Eventf(nimService, corev1.EventTypeWarning, conditions.Failed, msg)
		return ctrl.Result{}, err
	}

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
		err = k8sutil.CleanupResource(ctx, r.GetClient(), &networkingv1.Ingress{}, namespacedName)
		if err != nil && !k8serrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	// Sync HTTPRoute
	if nimService.IsHTTPRouteEnabled() {
		err = r.renderAndSyncResource(ctx, nimService, &renderer, &gatewayv1.HTTPRoute{}, func() (client.Object, error) {
			return renderer.HTTPRoute(nimService.GetHTTPRouteParams())
		}, "httproute", conditions.ReasonHTTPRouteFailed)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		err = k8sutil.CleanupResource(ctx, r.GetClient(), &gatewayv1.HTTPRoute{}, namespacedName)
		if err != nil && !k8serrors.IsNotFound(err) {
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
		err = k8sutil.CleanupResource(ctx, r.GetClient(), &autoscalingv2.HorizontalPodAutoscaler{}, namespacedName)
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

	var modelPVC *appsv1alpha1.PersistentVolumeClaim
	modelProfile := ""

	// Select PVC for model store
	nimCacheName := nimService.GetNIMCacheName()
	nimCache := appsv1alpha1.NIMCache{}
	if nimCacheName != "" { // nolint:gocritic
		if err := r.Get(ctx, types.NamespacedName{Name: nimCacheName, Namespace: nimService.GetNamespace()}, &nimCache); err != nil {
			// Fail the NIMService if the NIMCache is not found
			if k8serrors.IsNotFound(err) {
				msg := fmt.Sprintf("NIMCache %s not found", nimCacheName)
				statusUpdateErr := r.updater.SetConditionsFailed(ctx, nimService, conditions.ReasonNIMCacheNotFound, msg)
				r.GetEventRecorder().Eventf(nimService, corev1.EventTypeWarning, conditions.Failed, msg)
				logger.Info(msg, "nimcache", nimCacheName, "nimservice", nimService.Name)
				if statusUpdateErr != nil {
					logger.Error(statusUpdateErr, "failed to update status", "nimservice", nimService.Name)
					return ctrl.Result{}, statusUpdateErr
				}
				return ctrl.Result{}, nil
			}
			return ctrl.Result{}, err
		}

		switch nimCache.Status.State {
		case appsv1alpha1.NimCacheStatusReady:
			logger.V(4).Info("NIMCache is ready", "nimcache", nimCacheName, "nimservice", nimService.Name)
		case appsv1alpha1.NimCacheStatusFailed:
			msg := r.getNIMCacheFailedMessage(&nimCache)
			err = r.updater.SetConditionsFailed(ctx, nimService, conditions.ReasonNIMCacheFailed, msg)
			r.GetEventRecorder().Eventf(nimService, corev1.EventTypeWarning, conditions.Failed, msg)
			logger.Info(msg, "nimcache", nimCacheName, "nimservice", nimService.Name)
			if err != nil {
				logger.Error(err, "failed to update status", "nimservice", nimService.Name)
			}
			return ctrl.Result{}, err
		default:
			msg := fmt.Sprintf("NIMCache %s not ready", nimCacheName)
			err = r.updater.SetConditionsNotReady(ctx, nimService, conditions.ReasonNIMCacheNotReady, msg)
			r.GetEventRecorder().Eventf(nimService, corev1.EventTypeNormal, conditions.NotReady,
				"NIMService %s not ready yet, msg: %s", nimService.Name, msg)
			logger.V(4).Info(msg, "nimservice", nimService.Name)
			if err != nil {
				logger.Error(err, "failed to update status", "nimservice", nimService.Name)
			}
			return ctrl.Result{}, err
		}

		// Fetch PVC for the associated NIMCache instance and mount it
		nimCachePVC, err := r.getNIMCachePVC(&nimCache)
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
	} else if nimService.Spec.Storage.EmptyDir != nil && *nimService.Spec.Storage.EmptyDir {
		modelPVC = nil
	} else {
		err = fmt.Errorf("neither external PVC name, NIMCache volume or empty dir is provided")
		logger.Error(err, "failed to determine PVC , NIMCache volume or empty dir for model-store")
		return ctrl.Result{}, err
	}

	var profileEnv *[]corev1.EnvVar
	var profile *appsv1alpha1.NIMProfile
	var gpuResources *corev1.ResourceRequirements
	var initContainers []corev1.Container
	var renderFunc func() (client.Object, error)
	var conType, failedCon string
	var renderObj client.Object

	if modelProfile != "" {
		profileEnv = &[]corev1.EnvVar{{
			Name:  "NIM_MODEL_PROFILE",
			Value: modelProfile,
		}}

		// Only assign GPU resources if the NIMCache is for optimized NIM
		if nimCache.IsOptimizedNIM() {
			// Retrieve and set profile details from NIMCache
			profile, err = r.getNIMCacheProfile(ctx, nimService, modelProfile)
			if err != nil {
				logger.Error(err, "Failed to get cached NIM profile")
				return ctrl.Result{}, err
			}

			// Auto assign GPU resources in case of the optimized profile
			if profile != nil {
				gpuResources, err = r.addGPUResources(ctx, nimService, profile)
				if err != nil {
					logger.Error(err, "Failed to get GPU resources")
					return ctrl.Result{}, err
				}
			}
		}

		// TODO: assign GPU resources and node selector that is required for the selected profile
	}

	initContainers = nimService.GetInitContainers()
	namedDraResources := shared.GenerateNamedDRAResources(nimService)

	err = r.reconcileDRAResources(ctx, nimService, namedDraResources)
	if err != nil {
		return ctrl.Result{}, err
	}

	if nimService.Spec.MultiNode != nil && nimService.Spec.MultiNode.BackendType == appsv1alpha1.NIMBackendTypeLWS {
		lwsParams := nimService.GetLWSParams()
		lwsParams.PodResourceClaims = shared.GetPodResourceClaims(namedDraResources)
		lwsParams.OrchestratorType = string(r.GetOrchestratorType())
		lwsParams.LeaderVolumes = nimService.GetLeaderVolumes(modelPVC)
		lwsParams.WorkerVolumes = nimService.GetWorkerVolumes(modelPVC)
		if nimCache.IsUniversalNIM() {
			lwsParams.WorkerEnvs = utils.MergeEnvVars([]corev1.EnvVar{{
				Name:  "NIM_MODEL_NAME",
				Value: utils.DefaultModelStorePath,
			}}, lwsParams.WorkerEnvs)
			lwsParams.LeaderEnvs = utils.MergeEnvVars([]corev1.EnvVar{{
				Name:  "NIM_MODEL_NAME",
				Value: utils.DefaultModelStorePath,
			}}, lwsParams.LeaderEnvs)
		}
		lwsParams.LeaderVolumeMounts = nimService.GetLeaderVolumeMounts(modelPVC)
		lwsParams.WorkerVolumeMounts = nimService.GetWorkerVolumeMounts(modelPVC)
		if profileEnv != nil {
			lwsParams.WorkerEnvs = utils.MergeEnvVars(*profileEnv, lwsParams.WorkerEnvs)
			lwsParams.LeaderEnvs = utils.MergeEnvVars(*profileEnv, lwsParams.LeaderEnvs)
		}
		if gpuResources != nil {
			lwsParams.Resources = gpuResources
		}
		renderFunc = func() (client.Object, error) {
			result, err := renderer.LeaderWorkerSet(lwsParams)
			if err != nil {
				return nil, err
			}
			if len(initContainers) > 0 {
				result.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec.InitContainers = initContainers
				result.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.InitContainers = initContainers
			}
			shared.UpdateContainerResourceClaims(result.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec.Containers, namedDraResources)
			shared.UpdateContainerResourceClaims(result.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.Containers, namedDraResources)
			return result, nil
		}
		conType = "LeaderWorkerSet"
		failedCon = conditions.ReasonLeaderWorkerSetFailed
		renderObj = &lws.LeaderWorkerSet{}

		// Create configmap for MPI
		err = r.createMultiNodeVolumeObjects(ctx, nimService)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create multi-node volumes: %v", err)
		}
	} else {
		deploymentParams := nimService.GetDeploymentParams()
		deploymentParams.OrchestratorType = string(r.GetOrchestratorType())
		deploymentParams.PodResourceClaims = shared.GetPodResourceClaims(namedDraResources)
		if nimCache.IsUniversalNIM() {
			deploymentParams.Env = utils.MergeEnvVars([]corev1.EnvVar{{
				Name:  "NIM_MODEL_NAME",
				Value: utils.DefaultModelStorePath,
			}}, deploymentParams.Env)
		}
		// Setup volume mounts with model store
		deploymentParams.Volumes = nimService.GetVolumes(modelPVC)
		deploymentParams.VolumeMounts = nimService.GetVolumeMounts(modelPVC)
		if profileEnv != nil {
			deploymentParams.Env = utils.MergeEnvVars(*profileEnv, deploymentParams.Env)
		}
		// Auto assign GPU resources in case of the optimized profile
		if gpuResources != nil {
			deploymentParams.Resources = gpuResources
		}
		renderFunc = func() (client.Object, error) {
			result, err := renderer.Deployment(deploymentParams)
			if err != nil {
				return nil, err
			}
			if len(initContainers) > 0 {
				result.Spec.Template.Spec.InitContainers = initContainers
			}
			// Update Container resources with DRA resource claims.
			shared.UpdateContainerResourceClaims(result.Spec.Template.Spec.Containers, namedDraResources)
			return result, nil
		}
		conType = "Deployment"
		failedCon = conditions.ReasonDeploymentFailed
		renderObj = &appsv1.Deployment{}
	}

	err = r.renderAndSyncResource(ctx, nimService, &renderer, renderObj, renderFunc, conType, failedCon)
	if err != nil {
		return ctrl.Result{}, err
	}

	var ready bool
	if nimService.Spec.MultiNode != nil {
		msg, ready, err = r.isLeaderWorkerSetReady(ctx, nimService)
	} else {
		msg, ready, err = r.isDeploymentReady(ctx, &namespacedName)
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	if len(namedDraResources) > 0 {
		// Update NIMServiceStatus with resource claims.
		updateErr := r.updateResourceClaimStatus(ctx, nimService, namedDraResources)
		if updateErr != nil {
			logger.Info("WARN: Resource claim status update failed, will retry in 5 seconds", "error", updateErr.Error())
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
	}

	// TODO: Rework NIMService Status to split into MODIFY and APPLY phases for better readability
	// (Currently we're using `updater.SetConditions*` to implicitly take all previous changes and
	// apply them along with the conditions.)
	if !ready {
		// Update status as NotReady
		err = r.updater.SetConditionsNotReady(ctx, nimService, conditions.NotReady, msg)
		r.GetEventRecorder().Eventf(nimService, corev1.EventTypeNormal, conditions.NotReady,
			"NIMService %s not ready yet, msg: %s", nimService.Name, msg)
	} else {
		// Update NIMServiceStatus with model config.
		updateErr := r.updateModelStatus(ctx, nimService)
		if updateErr != nil {
			logger.Info("WARN: Model status update failed, will retry in 5 seconds", "error", updateErr.Error())
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		// Update status as ready
		err = r.updater.SetConditionsReady(ctx, nimService, conditions.Ready, msg)
		r.GetEventRecorder().Eventf(nimService, corev1.EventTypeNormal, conditions.Ready,
			"NIMService %s ready, msg: %s", nimService.Name, msg)
	}

	if err != nil {
		logger.Error(err, "failed to update status", "nimservice", nimService.Name, "state", conditions.Ready)
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *NIMServiceReconciler) createMultiNodeVolumeObjects(ctx context.Context, nimService *appsv1alpha1.NIMService) error {
	if err := r.createMultiNodeConfigMap(ctx, nimService, nimService.GetMPIConfigParams()); err != nil {
		return fmt.Errorf("failed to create MPI configmap for %s: %v", nimService.Name, err)
	}

	if err := r.createMultiNodeConfigMap(ctx, nimService, nimService.GetDefaultMPIScriptConfigParams()); err != nil {
		return fmt.Errorf("failed to create MPI script configmap for %s: %v", nimService.Name, err)
	}

	if err := r.createMultiNodeSSHPK(ctx, nimService); err != nil {
		return fmt.Errorf("failed to create MPI SSH secret for %s: %v", nimService.Name, err)
	}
	return nil
}

func (r *NIMServiceReconciler) createMultiNodeSSHPK(ctx context.Context, nimService *appsv1alpha1.NIMService) error {
	secretParams, err := nimService.GetMPISSHSecretParams()
	if err != nil {
		return err
	}

	namespacedName := types.NamespacedName{
		Name:      secretParams.Name,
		Namespace: secretParams.Namespace,
	}
	secret := &corev1.Secret{}
	err = r.Get(ctx, namespacedName, secret)
	if err == nil {
		// secret already exists, do nothing
		return nil
	}
	if client.IgnoreNotFound(err) != nil {
		return err
	}

	err = r.renderAndSyncResource(ctx, nimService, &r.renderer, &corev1.Secret{}, func() (client.Object, error) {
		return r.renderer.Secret(secretParams)
	}, "secret", conditions.ReasonSecretFailed)
	if err != nil {
		return fmt.Errorf("failed to create MPI SSH secret %s: %v", secretParams.Name, err)
	}

	return nil
}

func (r *NIMServiceReconciler) createMultiNodeConfigMap(ctx context.Context, nimService *appsv1alpha1.NIMService, cmParams *rendertypes.ConfigMapParams) error {
	renderer := r.GetRenderer()

	namespacedName := types.NamespacedName{
		Name:      cmParams.Name,
		Namespace: cmParams.Namespace,
	}
	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, namespacedName, cm)
	if err == nil {
		return nil
	}
	if client.IgnoreNotFound(err) != nil {
		return err
	}

	err = r.renderAndSyncResource(ctx, nimService, &renderer, &corev1.ConfigMap{}, func() (client.Object, error) {
		return renderer.ConfigMap(cmParams)
	}, "configmap", conditions.ReasonConfigMapFailed)
	if err != nil {
		return fmt.Errorf("failed to create configmap %s: %v", cmParams.Name, err)
	}
	return nil
}

func (r *NIMServiceReconciler) updateModelStatus(ctx context.Context, nimService *appsv1alpha1.NIMService) error {
	clusterEndpoint, externalEndpoint, err := r.getNIMModelEndpoints(ctx, nimService)
	if err != nil {
		return err
	}
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

func (r *NIMServiceReconciler) updateResourceClaimStatus(ctx context.Context, nimService *appsv1alpha1.NIMService, namedDraResources []shared.NamedDRAResource) error {
	logger := log.FromContext(ctx)

	draResourceStatuses, err := shared.GenerateDRAResourceStatuses(ctx, r.GetClient(), nimService.GetNamespace(), namedDraResources)
	if err != nil {
		logger.Error(err, "Failed to generate DRA resource statuses", "nimservice", nimService.Name)
		return err
	}

	nimService.Status.DRAResourceStatuses = draResourceStatuses
	return nil
}

func (r *NIMServiceReconciler) getNIMModelName(ctx context.Context, nimServiceEndpoint string) (string, error) {
	logger := log.FromContext(ctx)

	httpScheme := "http"
	// List nimservice /v1/models endpoint.
	modelsList, err := nimmodels.ListModelsV1(ctx, nimServiceEndpoint, httpScheme)
	if err != nil {
		logger.Error(err, "Failed to list models", "endpoint", nimServiceEndpoint)
		// Check if err is an HTTP error with a status code
		if nimmodels.IsNotFound(err) {
			// The endpoint does not exist
			logger.V(2).Error(err, "URI does not exist", "uri", nimmodels.ModelsV1URI, "endpoint", nimServiceEndpoint)
			metadata, err := nimmodels.GetMetadataV1(ctx, nimServiceEndpoint, httpScheme)
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

func getModelNameFromMetadata(metadata *nimmodels.MetadataV1) (string, error) {
	if len(metadata.ModelInfo) == 0 {
		return "", fmt.Errorf("no model info found")
	}
	return strings.Split(metadata.ModelInfo[0].ShortName, ":")[0], nil
}

func (r *NIMServiceReconciler) getNIMModelEndpoints(ctx context.Context, nimService *appsv1alpha1.NIMService) (string, string, error) {
	logger := log.FromContext(ctx)

	// Lookup NIMCache instance in the same namespace as the NIMService instance
	svc := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Name: nimService.GetName(), Namespace: nimService.GetNamespace()}, svc); err != nil {
		logger.Error(err, "unable to fetch k8s service", "nimservice", nimService.GetName())
		return "", "", err
	}

	var externalEndpoint string
	port := nimService.GetServicePort()
	if nimService.IsIngressEnabled() {
		ingress := &networkingv1.Ingress{}
		if err := r.Get(ctx, types.NamespacedName{Name: nimService.GetName(), Namespace: nimService.GetNamespace()}, ingress); err != nil {
			logger.Error(err, "unable to fetch ingress", "nimservice", nimService.GetName())
			return "", "", err
		}

		var found bool
		for _, rule := range ingress.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}
			for _, path := range rule.HTTP.Paths {
				if path.Backend.Service != nil && path.Backend.Service.Name == nimService.GetName() {
					if rule.Host != "" {
						externalEndpoint = rule.Host
						found = true
						break
					}
				}
			}
			if found {
				break
			}
		}
		if !found && len(ingress.Status.LoadBalancer.Ingress) > 0 {
			ing := ingress.Status.LoadBalancer.Ingress[0]
			externalEndpoint = ing.IP
			if ing.Hostname != "" {
				externalEndpoint = ing.Hostname
			}
		}
	} else if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		externalEndpoint = utils.FormatEndpoint(svc.Spec.LoadBalancerIP, port)
	}

	return utils.FormatEndpoint(svc.Spec.ClusterIP, port), externalEndpoint, nil
}

func (r *NIMServiceReconciler) renderAndSyncResource(ctx context.Context, nimService *appsv1alpha1.NIMService, renderer *render.Renderer, obj client.Object, renderFunc func() (client.Object, error), conditionType string, reason string) error {
	logger := log.FromContext(ctx)

	resource, err := renderFunc()
	if err != nil {
		logger.Error(err, "failed to render", "conditionType", conditionType)
		statusError := r.updater.SetConditionsFailed(ctx, nimService, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NIMService", nimService.GetName())
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

	// Use rendered resource name here as in some cases it can be different than nim service name
	// e.g. (with LWS resources with suffix)
	namespacedName := types.NamespacedName{Name: resource.GetName(), Namespace: resource.GetNamespace()}
	getErr := r.Get(ctx, namespacedName, obj)
	if getErr != nil && !k8serrors.IsNotFound(getErr) {
		logger.Error(getErr, fmt.Sprintf("Error is not NotFound for %s: %v", obj.GetObjectKind(), getErr))
		return getErr
	}

	// Track an existing resource
	found := getErr == nil

	// Don't do anything if CR is unchanged.
	if found && !utils.IsParentSpecChanged(obj, utils.DeepHashObject(nimService.Spec)) {
		return nil
	}

	// If we found the object and autoscaling is enabled on the NIMService,
	// copy the current replicas from the existing object into the desired (resource),
	// so we don't fight the HPA (or external scaler) on each reconcile.
	if found && nimService.IsAutoScalingEnabled() {
		if curr, ok := obj.(*appsv1.Deployment); ok {
			if desired, ok := resource.(*appsv1.Deployment); ok && curr.Spec.Replicas != nil {
				replicas := *curr.Spec.Replicas
				desired.Spec.Replicas = &replicas
			}
		}
	}

	if err = controllerutil.SetControllerReference(nimService, resource, r.GetScheme()); err != nil {
		logger.Error(err, "failed to set owner", "conditionType", conditionType)
		statusError := r.updater.SetConditionsFailed(ctx, nimService, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NIMService", nimService.GetName())
		}
		return err
	}

	err = k8sutil.SyncResource(ctx, r.GetClient(), obj, resource)
	if err != nil {
		logger.Error(err, "failed to sync", "conditionType", conditionType)
		statusError := r.updater.SetConditionsFailed(ctx, nimService, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NIMService", nimService.GetName())
		}
		return err
	}
	return nil
}

func (r *NIMServiceReconciler) isLeaderWorkerSetReady(ctx context.Context, nimService *appsv1alpha1.NIMService) (string, bool, error) {
	leaderWorkerSet := &lws.LeaderWorkerSet{}
	err := r.Get(ctx, client.ObjectKey{Name: nimService.GetLWSName(), Namespace: nimService.GetNamespace()}, leaderWorkerSet)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}

	for _, cond := range leaderWorkerSet.Status.Conditions {
		if cond.Type == string(lws.LeaderWorkerSetAvailable) && cond.Status == metav1.ConditionTrue {
			return fmt.Sprintf("leaderworkerset %q is ready", leaderWorkerSet.Name), true, nil
		}
	}
	return fmt.Sprintf("leaderworkerset %q is not ready", leaderWorkerSet.Name), false, nil
}

// CheckDeploymentReadiness checks if the Deployment is ready.
func (r *NIMServiceReconciler) isDeploymentReady(ctx context.Context, namespacedName *types.NamespacedName) (string, bool, error) {
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Name: namespacedName.Name, Namespace: namespacedName.Namespace}, deployment)
	if err != nil {
		if k8serrors.IsNotFound(err) {
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

// getNIMCachePVC returns PVC backing the NIM cache instance.
func (r *NIMServiceReconciler) getNIMCachePVC(nimCache *appsv1alpha1.NIMCache) (*appsv1alpha1.PersistentVolumeClaim, error) {
	if nimCache.Status.PVC == "" {
		return nil, fmt.Errorf("missing PVC for the nimcache instance %s", nimCache.GetName())
	}

	if nimCache.Spec.Storage.PVC.Name == "" {
		nimCache.Spec.Storage.PVC.Name = nimCache.Status.PVC
	}
	// Get the underlying PVC for the NIMCache instance
	return &nimCache.Spec.Storage.PVC, nil
}

func (r *NIMServiceReconciler) getNIMCacheFailedMessage(nimCache *appsv1alpha1.NIMCache) string {
	cond := meta.FindStatusCondition(nimCache.Status.Conditions, conditions.Failed)
	if cond != nil && cond.Status == metav1.ConditionTrue {
		return cond.Message
	}
	return ""
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
				logger.Error(err, "Failed to construct pvc", "name", pvcName)
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

// getNIMCacheProfile returns model profile info from the NIM cache instance.
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

// assignGPUResources automatically assigns GPU resources to the NIMService based on the provided profile,
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

func (r *NIMServiceReconciler) reconcileDRAResources(ctx context.Context, nimService *appsv1alpha1.NIMService, namedDraResources []shared.NamedDRAResource) error {
	logger := log.FromContext(ctx)

	renderer := r.GetRenderer()
	for _, namedDraResource := range namedDraResources {
		if !shared.ShouldCreateDRAResource(namedDraResource.DRAResource) {
			continue
		}

		labels := nimService.GetServiceLabels()
		annotations := nimService.GetNIMServiceAnnotations()
		claimAnnotations := nimService.GetNIMServiceAnnotations()
		delete(claimAnnotations, utils.NvidiaAnnotationParentSpecHashKey)
		// Sync ResourceClaimTemplate
		err := r.renderAndSyncResource(ctx, nimService, &renderer, &resourcev1beta2.ResourceClaimTemplate{}, func() (client.Object, error) {
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
					logger.Error(err, "failed to get CEL expressions for device", "device", device.Name)
					return nil, err
				}
				resourceClaimTemplateParams.Devices = append(resourceClaimTemplateParams.Devices, rendertypes.DRADeviceParams{
					Name:            device.Name,
					Count:           device.Count,
					DeviceClassName: device.DeviceClassName,
					CELExpressions:  exprs,
				})
			}
			return renderer.ResourceClaimTemplate(resourceClaimTemplateParams)
		}, "resourceclaimtemplate", conditions.ReasonResourceClaimTemplateFailed)

		if err != nil {
			return fmt.Errorf("failed to reconcile DRAResource %s: %w", namedDraResource.ResourceName, err)
		}
	}
	return nil
}
