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
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	apiResource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	"github.com/NVIDIA/k8s-nim-operator/internal/nimparser"
	nimparserutils "github.com/NVIDIA/k8s-nim-operator/internal/nimparser/utils"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
	"github.com/NVIDIA/k8s-nim-operator/internal/render"
	"github.com/NVIDIA/k8s-nim-operator/internal/shared"
	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

const (
	// NIMBuildFinalizer is the finalizer annotation.
	NIMBuildFinalizer = "finalizer.nimbuild.apps.nvidia.com"

	// NIMBuildContainerName returns the name of the container used for NIM Build operations.
	NIMBuildContainerName = "nim-build-ctr"

	NIMBuildManifestContainerName = "nim-build-manifest-ctr"
)

// NIMBuildReconciler reconciles a NIMBuild object.
type NIMBuildReconciler struct {
	client.Client
	scheme           *runtime.Scheme
	log              logr.Logger
	orchestratorType k8sutil.OrchestratorType
	updater          conditions.Updater
	recorder         record.EventRecorder
}

// Ensure NIMBuildReconciler implements the Reconciler interface.
var _ shared.Reconciler = &NIMBuildReconciler{}

// NewNIMBuildReconciler creates a new reconciler for NIMBuild with the given platform.
func NewNIMBuildReconciler(client client.Client, scheme *runtime.Scheme, log logr.Logger) *NIMBuildReconciler {
	return &NIMBuildReconciler{
		Client: client,
		scheme: scheme,
		log:    log,
	}
}

// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimbuilds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimbuilds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimbuilds/finalizers,verbs=update
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=use,resourceNames=nonroot
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",resources=pods/log,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *NIMBuildReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var err error
	var result reconcile.Result

	// Fetch the NIMBuild instance
	nimBuild := &appsv1alpha1.NIMBuild{}
	if err = r.Get(ctx, req.NamespacedName, nimBuild); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "unable to fetch NIMBuild", "name", req.NamespacedName)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	logger.Info("Reconciling", "NIMBuild", nimBuild.Name)

	defer func() {
		if err != nil {
			r.GetEventRecorder().Eventf(nimBuild, corev1.EventTypeWarning, "ReconcileFailed",
				"NIMBuild %s reconcile failed, msg: %s", nimBuild.Name, err.Error())
		}
	}()
	// Check if the instance is marked for deletion
	if nimBuild.DeletionTimestamp.IsZero() {
		// Add finalizer if not present
		if !controllerutil.ContainsFinalizer(nimBuild, NIMBuildFinalizer) {
			controllerutil.AddFinalizer(nimBuild, NIMBuildFinalizer)
			if err = r.Update(ctx, nimBuild); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The instance is being deleted
		if controllerutil.ContainsFinalizer(nimBuild, NIMBuildFinalizer) {
			// Perform cleanup of resources
			if err = r.cleanupNIMBuild(ctx, nimBuild); err != nil {
				return ctrl.Result{}, err
			}
			// Remove finalizer to allow for deletion
			controllerutil.RemoveFinalizer(nimBuild, NIMBuildFinalizer)
			if err := r.Update(ctx, nimBuild); err != nil {
				return ctrl.Result{}, err
			}
		}
		// return as the cr is being deleted and GC will cleanup owned objects
		return ctrl.Result{}, nil
	}

	// Fetch container orchestrator type
	_, err = r.GetOrchestratorType(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to get container orchestrator type, %v", err)
	}

	// Handle nim-cache reconciliation
	result, err = r.reconcileNIMBuild(ctx, nimBuild)
	if err != nil {
		logger.Error(err, "error reconciling NIMBuild", "name", nimBuild.Name)
		conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionReconcileFailed, metav1.ConditionTrue, "ReconcileFailed", err.Error())
		nimBuild.Status.State = appsv1alpha1.NimBuildStatusPending

		err := r.updateNIMBuildState(ctx, nimBuild, appsv1alpha1.NimBuildStatusPending)
		if err != nil {
			logger.Error(err, "Failed to update NIMBuild state", "NIMBuild", nimBuild.Name)
			return ctrl.Result{}, err
		}
		return result, err
	}
	return result, nil
}

// GetScheme returns the scheme of the reconciler.
func (r *NIMBuildReconciler) GetScheme() *runtime.Scheme {
	return r.scheme
}

func (r *NIMBuildReconciler) GetLogger() logr.Logger {
	return r.log
}

func (r *NIMBuildReconciler) GetClient() client.Client {
	return r.Client
}

func (r *NIMBuildReconciler) GetUpdater() conditions.Updater {
	return r.updater
}

// GetDiscoveryClient returns the discovery client instance.
func (r *NIMBuildReconciler) GetDiscoveryClient() discovery.DiscoveryInterface {
	return nil
}

func (r *NIMBuildReconciler) GetRenderer() render.Renderer {
	return nil
}

func (r *NIMBuildReconciler) GetEventRecorder() record.EventRecorder {
	return r.recorder
}

func (r *NIMBuildReconciler) GetOrchestratorType(ctx context.Context) (k8sutil.OrchestratorType, error) {
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
func (r *NIMBuildReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor("nimbuild-controller")

	err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&appsv1alpha1.NIMBuild{},
		"spec.nimCache.name",
		func(rawObj client.Object) []string {
			nimBuild, ok := rawObj.(*appsv1alpha1.NIMBuild)
			if !ok {
				return []string{}
			}
			return []string{nimBuild.Spec.NIMCache.Name}
		},
	)
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.NIMBuild{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Type assert to NIMBuild
				if _, ok := e.ObjectOld.(*appsv1alpha1.NIMBuild); ok {
					newNIMBuild, ok := e.ObjectNew.(*appsv1alpha1.NIMBuild)
					if ok {
						// Handle case where object is marked for deletion
						if !newNIMBuild.ObjectMeta.DeletionTimestamp.IsZero() {
							return true
						}
						return false
					}
				}
				// For other types we watch, reconcile them
				return true
			},
		}).Watches(
		&appsv1alpha1.NIMCache{},
		handler.EnqueueRequestsFromMapFunc(r.mapNIMCacheToNIMBuild),
		builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
	).
		Complete(r)
}

func (r *NIMBuildReconciler) mapNIMCacheToNIMBuild(ctx context.Context, obj client.Object) []ctrl.Request {
	nimCache, ok := obj.(*appsv1alpha1.NIMCache)
	if !ok {
		return []ctrl.Request{}
	}

	// Get all NIMBuilds that reference this NIMCache
	var nimBuilds appsv1alpha1.NIMBuildList
	if err := r.List(ctx, &nimBuilds, client.MatchingFields{"spec.nimCacheRef": nimCache.GetName()}, client.InNamespace(nimCache.GetNamespace())); err != nil {
		return []ctrl.Request{}
	}

	// Enqueue reconciliation for each matching NIMBuild object
	requests := make([]ctrl.Request, len(nimBuilds.Items))
	for i, item := range nimBuilds.Items {
		requests[i] = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      item.Name,
				Namespace: item.Namespace,
			},
		}
	}
	return requests
}

func (r *NIMBuildReconciler) cleanupNIMBuild(ctx context.Context, nimBuild *appsv1alpha1.NIMBuild) error {
	var errList []error
	logger := r.GetLogger()

	// All owned objects are garbage collected

	// Fetch the pod
	podName := types.NamespacedName{Name: nimBuild.GetEngineBuildPodName(), Namespace: nimBuild.Namespace}
	pod := &corev1.Pod{}
	if err := r.Get(ctx, podName, pod); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		logger.Error(err, "unable to fetch the pod for cleanup", "pod", podName)
		return err
	}

	if err := r.Delete(ctx, pod); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Pod already deleted", "pod", podName)
			return nil
		}
		logger.Error(err, "unable to delete associated pods during cleanup", "job", podName, "pod", pod.Name)
		errList = append(errList, err)
	}

	if len(errList) > 0 {
		return fmt.Errorf("failed to cleanup resources: %v", errList)
	}

	return nil
}

func (r *NIMBuildReconciler) reconcileNIMBuild(ctx context.Context, nimBuild *appsv1alpha1.NIMBuild) (reconcile.Result, error) {
	logger := r.GetLogger()
	nimCacheNamespacedName := types.NamespacedName{Name: nimBuild.Spec.NIMCache.Name, Namespace: nimBuild.GetNamespace()}

	nimCache := &appsv1alpha1.NIMCache{}
	if err := r.Get(ctx, nimCacheNamespacedName, nimCache); err != nil {
		logger.Error(err, "unable to fetch NIMCache for NIMBuild", "NIMCache", nimCacheNamespacedName)
		conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionNIMCacheNotFound, metav1.ConditionTrue, "NIMCache not found", "Not able to get NIMCache for NIMBuild")
		return ctrl.Result{}, r.updateNIMBuildState(ctx, nimBuild, appsv1alpha1.NimBuildStatusFailed)
	}

	switch nimCache.Status.State {
	case appsv1alpha1.NimCacheStatusReady:
		logger.V(4).Info("NIMCache is ready", "nimcache", nimCache.Name, "nimservice", nimBuild.Name)
	case appsv1alpha1.NimCacheStatusFailed:
		conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionNimCacheFailed, metav1.ConditionTrue, "NIMCache failed", "NIMCache is failed. NIMBuild can not progress")
		return ctrl.Result{}, r.updateNIMBuildState(ctx, nimBuild, appsv1alpha1.NimBuildStatusFailed)
	default:
		conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionWaitForNimCache, metav1.ConditionTrue, "NIMCache not ready", "Waiting for NIMCache to be ready before building engines")
		return ctrl.Result{}, r.updateNIMBuildState(ctx, nimBuild, appsv1alpha1.NimBuildStatusFailed)
	}

	if !meta.IsStatusConditionTrue(nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionEngineBuildPodCompleted) {
		err := r.reconcileEngineBuild(ctx, nimBuild, nimCache)
		if err != nil {
			logger.Error(err, "reconciliation of nimbuild failed")
			return ctrl.Result{}, err
		}
	}

	if meta.IsStatusConditionTrue(nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionEngineBuildPodCompleted) && !meta.IsStatusConditionTrue(nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionModelManifestPodCompleted) {
		err := r.reconcileLocalModelManifest(ctx, nimBuild, nimCache)
		if err != nil {
			logger.Error(err, "reconciliation of nimbuild failed")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}
func (r *NIMBuildReconciler) updateNIMBuildState(ctx context.Context, nimBuild *appsv1alpha1.NIMBuild, state string) error {
	logger := r.GetLogger()
	nimBuild.Status.State = state
	err := r.updateNIMBuildStatus(ctx, nimBuild)
	if err != nil {
		logger.Error(err, "Failed to update NIMBuild status", "NIMBuild", nimBuild.Name)
		return err
	}
	return nil
}

func (r *NIMBuildReconciler) reconcileEngineBuild(ctx context.Context, nimBuild *appsv1alpha1.NIMBuild, nimCache *appsv1alpha1.NIMCache) error {
	logger := r.GetLogger()
	var buildableProfile appsv1alpha1.NIMProfile
	if nimBuild.Spec.NIMCache.Profile == "" {
		buildableProfiles := getBuildableProfiles(nimCache)
		if len(buildableProfiles) > 0 {
			switch {
			case len(buildableProfiles) > 1:
				logger.Info("Multiple buildable profiles found", "Profiles", buildableProfiles)
				conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionMultipleBuildableProfilesFound, metav1.ConditionTrue, "MultipleBuildableProfilesFound", "Multiple buildable profiles found in NIM Cache, please select one profile to build")
				return r.updateNIMBuildState(ctx, nimBuild, appsv1alpha1.NimBuildStatusFailed)
			case len(buildableProfiles) == 1:
				logger.Info("Selected buildable profile found", "Profile", buildableProfiles[0].Name)
				conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionSingleBuildableProfilesFound, metav1.ConditionTrue, "BuildableProfileFound", "Single buildable profile cached in NIM Cache")
				buildableProfile = buildableProfiles[0]
				nimBuild.Status.InputProfile = buildableProfile
			default:
				logger.Info("No buildable profiles found, skipping engine build")
				conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionNoBuildableProfilesFound, metav1.ConditionTrue, "NoBuildableProfilesFound", "No buildable profiles found in NIM Cache")
				return r.updateNIMBuildState(ctx, nimBuild, appsv1alpha1.NimBuildStatusFailed)
			}

		} else {
			logger.Info("No buildable profiles found, skipping engine build")
			conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionNoBuildableProfilesFound, metav1.ConditionTrue, "NoBuildableProfilesFound", "No buildable profiles found for NIM Cache")
			return r.updateNIMBuildState(ctx, nimBuild, appsv1alpha1.NimBuildStatusFailed)
		}
	} else {
		foundProfile := getBuildableProfileByName(nimCache, nimBuild.Spec.NIMCache.Profile)
		if foundProfile == nil {
			logger.Info("No buildable profiles found", "ProfileName", nimBuild.Spec.NIMCache.Profile)
			conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionNoBuildableProfilesFound, metav1.ConditionTrue, "NoBuildableProfilesFound", "No buildable profiles found, please select a valid profile from the NIM cache")
			return r.updateNIMBuildState(ctx, nimBuild, appsv1alpha1.NimBuildStatusFailed)
		} else {
			logger.Info("Selected buildable profile found", "Profile", foundProfile.Name)
			conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionSingleBuildableProfilesFound, metav1.ConditionTrue, "BuildableProfileFound", "Single buildable profile found")
			buildableProfile = *foundProfile
			nimBuild.Status.InputProfile = buildableProfile
		}
	}

	pod := &corev1.Pod{}
	podName := types.NamespacedName{Name: nimBuild.GetEngineBuildPodName(), Namespace: nimBuild.GetNamespace()}
	err := r.Get(ctx, podName, pod)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return err
	}
	// If pod does not exist and caching is not complete, create a new one
	if err != nil && !meta.IsStatusConditionTrue(nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionEngineBuildPodCreated) {
		pod, err = r.constructEngineBuildPod(nimBuild, nimCache, r.orchestratorType, buildableProfile)
		if err != nil {
			logger.Error(err, "Failed to construct job")
			return err
		}
		if err := controllerutil.SetControllerReference(nimBuild, pod, r.GetScheme()); err != nil {
			return err
		}

		err = r.createPod(ctx, pod)
		if err != nil {
			logger.Error(err, "failed to create", "pod", pod.Name)
			return err
		}

		logger.Info("Created pod for NIM Cache engine build", "pod", podName)
		conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionEngineBuildPodCreated, metav1.ConditionTrue, "EngineBuildPodCreated", "The pod to build engine has been created")
		return r.updateNIMBuildState(ctx, nimBuild, appsv1alpha1.NimBuildStatusInProgress)
	}

	// Reconcile the job status
	if err := r.reconcileEngineBuildPodStatus(ctx, nimBuild, pod); err != nil {
		return err
	}
	return nil
}

func getBuildableProfiles(cache *appsv1alpha1.NIMCache) []appsv1alpha1.NIMProfile {
	var buildableProfiles []appsv1alpha1.NIMProfile
	for _, profile := range cache.Status.Profiles {
		if profile.Config["trtllm_buildable"] == "true" {
			buildableProfiles = append(buildableProfiles, profile)
		}
	}
	return buildableProfiles
}

func getBuildableProfileByName(cache *appsv1alpha1.NIMCache, profileName string) *appsv1alpha1.NIMProfile {
	for _, profile := range cache.Status.Profiles {
		if profile.Config["trtllm_buildable"] == "true" && profile.Name == profileName {
			return &profile
		}
	}
	return nil
}

func (r *NIMBuildReconciler) reconcileEngineBuildPodStatus(ctx context.Context, nimBuild *appsv1alpha1.NIMBuild, pod *corev1.Pod) error {
	logger := log.FromContext(ctx)
	podName := pod.Name

	switch {
	case isPodReady(pod) && !meta.IsStatusConditionTrue(nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionEngineBuildPodCompleted):
		logger.Info("Pod Ready", "pod", podName)
		conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionEngineBuildPodCompleted, metav1.ConditionTrue, "PodReady", "The Pod to build engine has successfully completed")
		if err := r.deletePod(ctx, pod); err != nil {
			logger.Error(err, "Unable to delete NIM Cache build engine pod", "Name", pod.Name)
			return err
		}

	case pod.Status.Phase == corev1.PodFailed && !meta.IsStatusConditionTrue(nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionEngineBuildPodCompleted):
		logger.Info("Failed to cache NIM, build pod failed", "pod", pod)
		conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionEngineBuildPodCompleted, metav1.ConditionFalse, "PodFailed", "The pod to build engine has failed")
		return r.updateNIMBuildState(ctx, nimBuild, appsv1alpha1.NimBuildStatusFailed)

	case !isPodReady(pod) && !meta.IsStatusConditionTrue(nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionEngineBuildPodCompleted):
		logger.Info("Caching NIM is in progress, build engine pod running", "job", podName)
		conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionEngineBuildPodPending, metav1.ConditionTrue, "PodRunning", "The Pod to build engine is in progress")
		return r.updateNIMBuildState(ctx, nimBuild, appsv1alpha1.NimBuildStatusInProgress)

	}

	return nil
}

func (r *NIMBuildReconciler) updateNIMBuildStatus(ctx context.Context, nimBuild *appsv1alpha1.NIMBuild) error {
	logger := r.GetLogger()

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		obj := &appsv1alpha1.NIMBuild{}
		errGet := r.Get(ctx, types.NamespacedName{Name: nimBuild.Name, Namespace: nimBuild.GetNamespace()}, obj)
		if errGet != nil {
			logger.Error(errGet, "error getting NIMBuild", "name", nimBuild.Name)
			return errGet
		}
		obj.Status = nimBuild.Status
		if err := r.Status().Update(ctx, obj); err != nil {
			logger.Error(err, "Failed to update status", "NIMBuild", nimBuild.Name)
			return err
		}
		return nil
	})
}

func (r *NIMBuildReconciler) constructEngineBuildPod(nimBuild *appsv1alpha1.NIMBuild, nimCache *appsv1alpha1.NIMCache, platformType k8sutil.OrchestratorType, inputNimProfile appsv1alpha1.NIMProfile) (*corev1.Pod, error) {
	logger := r.GetLogger()
	pvcName := shared.GetPVCName(nimCache, nimCache.Spec.Storage.PVC)
	labels := map[string]string{
		"app":                          "k8s-nim-operator",
		"app.kubernetes.io/name":       nimBuild.Name,
		"app.kubernetes.io/managed-by": "k8s-nim-operator",
	}

	if nimBuild.GetLabels() != nil {
		labels = utils.MergeMaps(labels, nimBuild.GetLabels())
	}

	annotations := map[string]string{
		"sidecar.istio.io/inject": "false",
	}
	if nimBuild.GetAnnotations() != nil {
		annotations = utils.MergeMaps(annotations, nimBuild.GetAnnotations())
	}

	// Get tensorParallelism from the profile
	tensorParallelism, err := utils.GetTensorParallelismByProfileTags(inputNimProfile.Config)
	if err != nil {
		logger.Error(err, "Failed to retrieve tensorParallelism")
		return nil, err
	}

	// Ensure Resources, Requests, and Limits are initialized
	if nimBuild.Spec.Resources == nil {
		nimBuild.Spec.Resources = &appsv1alpha1.ResourceRequirements{}
	}
	if nimBuild.Spec.Resources.Requests == nil {
		nimBuild.Spec.Resources.Requests = corev1.ResourceList{}
	}
	if nimBuild.Spec.Resources.Limits == nil {
		nimBuild.Spec.Resources.Limits = corev1.ResourceList{}
	}
	if tensorParallelism == "" {
		if _, present := nimBuild.Spec.Resources.Requests["nvidia.com/gpu"]; !present {
			return nil, fmt.Errorf("tensorParallelism is not set in the profile tags or resources")
		}
	} else {
		if _, present := nimBuild.Spec.Resources.Requests["nvidia.com/gpu"]; !present {
			gpuQuantity, err := apiResource.ParseQuantity(tensorParallelism)
			if err != nil {
				return nil, fmt.Errorf("failed to parse tensorParallelism: %w", err)
			}
			nimBuild.Spec.Resources.Requests["nvidia.com/gpu"] = gpuQuantity
			nimBuild.Spec.Resources.Limits["nvidia.com/gpu"] = gpuQuantity

		} else {
			return nil, fmt.Errorf("tensorParallelism is set in the profile tags, but nvidia.com/gpu is already set in resources")
		}
	}

	if platformType == k8sutil.OpenShift {
		annotations["openshift.io/scc"] = "nonroot"
	}

	var imagePullSecrets []corev1.LocalObjectReference
	for _, name := range nimBuild.Spec.Image.PullSecrets {
		imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReference{Name: name})
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        nimBuild.GetEngineBuildPodName(),
			Namespace:   nimBuild.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{
			RuntimeClassName: nimCache.GetRuntimeClassName(),
			SecurityContext: &corev1.PodSecurityContext{
				RunAsUser:    nimCache.GetUserID(),
				FSGroup:      nimCache.GetGroupID(),
				RunAsNonRoot: ptr.To[bool](true),
			},
			Containers:    []corev1.Container{},
			RestartPolicy: corev1.RestartPolicyNever,
			Volumes: []corev1.Volume{
				{
					Name: "nim-cache-volume",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
			},
			ImagePullSecrets: imagePullSecrets,
			Tolerations:      nimBuild.GetTolerations(),
			NodeSelector:     nimBuild.GetNodeSelectors(),
		},
	}

	// SeccompProfile must be set for TKGS
	if platformType == k8sutil.TKGS {
		pod.Spec.SecurityContext.SeccompProfile = &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		}
	}

	pod.Spec.Containers = []corev1.Container{
		{
			Name:  NIMBuildContainerName,
			Image: nimBuild.GetImage(),
			Env: []corev1.EnvVar{
				{
					Name:  "NIM_CACHE_PATH",
					Value: "/model-store",
				},
				{
					Name:  "NIM_SERVER_PORT",
					Value: "8000",
				},
				{
					Name:  "NIM_HTTP_API_PORT",
					Value: "8000",
				},
				{
					Name:  "NIM_CUSTOM_MODEL_NAME",
					Value: nimBuild.GetModelName(),
				},
				{
					Name:  "NIM_MODEL_PROFILE",
					Value: inputNimProfile.Name,
				},
				{
					Name: "NGC_API_KEY",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: nimCache.Spec.Source.NGC.AuthSecret,
							},
							Key: "NGC_API_KEY",
						},
					},
				},
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "nim-cache-volume",
					MountPath: "/model-store",
					SubPath:   nimCache.Spec.Storage.PVC.SubPath,
				},
			},
			Resources:                corev1.ResourceRequirements{Limits: nimBuild.Spec.Resources.Limits, Requests: nimBuild.Spec.Resources.Requests},
			TerminationMessagePath:   "/dev/termination-log",
			TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
			Ports: []corev1.ContainerPort{{
				Name:          "api",
				ContainerPort: 8000,
				Protocol:      corev1.ProtocolTCP,
			}},
			ReadinessProbe: &corev1.Probe{
				InitialDelaySeconds: 15,
				TimeoutSeconds:      1,
				PeriodSeconds:       10,
				SuccessThreshold:    1,
				FailureThreshold:    3,
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/v1/health/ready",
						Port: intstr.FromString("api"),
					},
				},
			},
			LivenessProbe: &corev1.Probe{
				InitialDelaySeconds: 15,
				TimeoutSeconds:      1,
				PeriodSeconds:       10,
				SuccessThreshold:    1,
				FailureThreshold:    3,
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/v1/health/live",
						Port: intstr.FromString("api"),
					},
				},
			},
			StartupProbe: &corev1.Probe{
				InitialDelaySeconds: 30,
				TimeoutSeconds:      1,
				PeriodSeconds:       10,
				SuccessThreshold:    1,
				FailureThreshold:    30,
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/v1/health/ready",
						Port: intstr.FromString("api"),
					},
				},
			},
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: ptr.To[bool](false),
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
				},
				RunAsNonRoot: ptr.To[bool](true),
				RunAsGroup:   nimCache.GetGroupID(),
				RunAsUser:    nimCache.GetUserID(),
			},
		},
	}

	// Merge env with the user provided values
	pod.Spec.Containers[0].Env = utils.MergeEnvVars(pod.Spec.Containers[0].Env, nimBuild.Spec.Env)

	return pod, nil
}

func isPodReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

func (r *NIMBuildReconciler) deletePod(ctx context.Context, pod *corev1.Pod) error {
	logger := log.FromContext(ctx)
	logger.Info("Deleting Engine Build Pod", "name", pod.Name, "namespace", pod.Namespace)
	if err := r.Delete(ctx, pod); err != nil {
		logger.Error(err, "Failed to Delete Engine Build Pod", "name", pod.Name)
		return err
	}
	return nil
}

func (r *NIMBuildReconciler) reconcileLocalModelManifest(ctx context.Context, nimBuild *appsv1alpha1.NIMBuild, nimCache *appsv1alpha1.NIMCache) error {
	logger := r.GetLogger()

	// Model manifest is available only for NGC model pullers
	if nimCache.Spec.Source.NGC == nil {
		return nil
	}

	existingConfig := &corev1.ConfigMap{}
	cmName := getManifestConfigName(nimCache)
	err := r.Get(ctx, client.ObjectKey{Name: cmName, Namespace: nimBuild.Namespace}, existingConfig)
	if err != nil {
		logger.Error(err, "failed to get configmap of the local model manifest", "name", cmName)
		return err
	}

	// Create a temporary pod for parsing local model manifest
	pod := constructLocalManifestPodSpec(nimCache, nimBuild, r.orchestratorType)
	// Add nimBuild as owner for watching on status change
	if err := controllerutil.SetControllerReference(nimBuild, pod, r.GetScheme()); err != nil {
		return err
	}
	err = r.createPod(ctx, pod)
	if err != nil {
		logger.Error(err, "failed to create", "pod", pod.Name)
		return err
	}

	existingPod := &corev1.Pod{}
	err = r.Get(ctx, client.ObjectKey{Name: pod.Name, Namespace: nimBuild.Namespace}, existingPod)
	if err != nil {
		logger.Error(err, "failed to get pod for model selection", "pod", pod.Name)
		return err
	}

	if existingPod.Status.Phase != corev1.PodRunning {
		// requeue request with delay until the pod is ready
		return nil
	}

	// Extract manifest file
	output, err := k8sutil.GetPodLogs(ctx, existingPod, NIMBuildManifestContainerName)
	if err != nil {
		logger.Error(err, "failed to get pod logs for parsing model manifest file", "pod", pod.Name)
		return err
	}

	if output == "" {
		logger.Info("Requeuing to wait for the manifest to be copied from the container")
		return nil
	}

	parser := nimparserutils.GetNIMParser([]byte(output))
	// Parse the file
	manifest, err := parser.ParseModelManifestFromRawOutput([]byte(output))
	if err != nil {
		logger.Error(err, "Failed to parse model manifest from the pod")
		return err
	}
	logger.V(2).Info("local manifest file", "nimbuild", nimBuild.Name, "manifest", manifest)

	// Update a ConfigMap with the model manifest file for re-use
	err = r.updateManifestConfigMap(ctx, nimCache, &manifest)
	if err != nil {
		logger.Error(err, "Failed to create model manifest config map")
		return err
	}

	// Model manifest is successfully extracted, cleanup temporary pod
	err = r.Delete(ctx, existingPod)
	if err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to delete", "pod", pod.Name)
		// requeue request with delay until the pod is cleaned up
		// this is required as NIM containers are resource heavy
		return err
	}

	// To Do: Explore changing the profile list on NIMCache to a map for faster lookups
	// Update the NIMCache status with the details of the built profile
	builtProfileName := getBuiltProfileName(manifest, nimBuild)
	if builtProfileName != "" {
		builtProfile := appsv1alpha1.NIMProfile{
			Name:    builtProfileName,
			Model:   manifest.GetProfileModel(builtProfileName),
			Config:  manifest.GetProfileTags(builtProfileName),
			Release: manifest.GetProfileRelease(builtProfileName),
		}
		nimBuild.Status.OutputProfile = builtProfile

		presentOnNIMCache := isBuiltProfilePresentOnNIMCacheStatus(nimCache, builtProfileName)
		// If built profile is not present on NIMCache status, add it
		if !presentOnNIMCache {
			tagsMap := manifest.GetProfileTags(builtProfileName)
			tagsMap["input_profile"] = nimBuild.Status.InputProfile.Name
			logger.Info("Adding profile to NIMCache status", "profileName", builtProfileName)
			nimCache.Status.Profiles = append(nimCache.Status.Profiles, builtProfile)
		}
	}

	// Update the NIMCache status with the new profiles
	obj := &appsv1alpha1.NIMCache{}
	errGet := r.Get(ctx, types.NamespacedName{Name: nimCache.Name, Namespace: nimCache.GetNamespace()}, obj)
	if errGet != nil {
		logger.Error(errGet, "error getting NIMCache", "name", nimCache.Name)
		return errGet
	}
	obj.Status = nimCache.Status
	if err := r.Status().Update(ctx, obj); err != nil {
		logger.Error(err, "Failed to update status", "NIMCache", nimCache.Name)
		return err
	}
	// Update the NIMBuild status
	conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionModelManifestPodCompleted, metav1.ConditionTrue, "PodCompleted", "The Pod to read local model manifest is completed")
	return r.updateNIMBuildState(ctx, nimBuild, appsv1alpha1.NimBuildStatusReady)
}

func isBuiltProfilePresentOnNIMCacheStatus(nimCache *appsv1alpha1.NIMCache, builtProfileName string) bool {
	for _, selectedProfile := range nimCache.Status.Profiles {
		if selectedProfile.Name == builtProfileName {
			return true
		}
	}
	return false
}

// getBuiltProfileName retrieves the auto generated profile name assigned for the built engine.
func getBuiltProfileName(manifest nimparser.NIMManifestInterface, nimBuild *appsv1alpha1.NIMBuild) string {
	for _, profileName := range manifest.GetProfilesList() {
		tagsMap := manifest.GetProfileTags(profileName)
		if tagsMap["model_name"] == nimBuild.GetModelName() {
			return profileName
		}
	}
	return ""
}

func constructLocalManifestPodSpec(nimCache *appsv1alpha1.NIMCache, nimBuild *appsv1alpha1.NIMBuild, platformType k8sutil.OrchestratorType) *corev1.Pod {

	labels := map[string]string{
		"app":                          "k8s-nim-operator",
		"app.kubernetes.io/name":       nimBuild.Name,
		"app.kubernetes.io/managed-by": "k8s-nim-operator",
	}

	annotations := map[string]string{
		"sidecar.istio.io/inject": "false",
	}

	if platformType == k8sutil.OpenShift {
		annotations = map[string]string{
			"openshift.io/required-scc": "nonroot",
		}
	}
	pvcName := shared.GetPVCName(nimCache, nimCache.Spec.Storage.PVC)

	var imagePullSecrets []corev1.LocalObjectReference
	for _, name := range nimBuild.Spec.Image.PullSecrets {
		imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReference{Name: name})
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        nimBuild.GetLocalManifestReaderPodName(),
			Namespace:   nimBuild.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{
			ImagePullSecrets: imagePullSecrets,
			RuntimeClassName: nimCache.GetRuntimeClassName(),
			SecurityContext: &corev1.PodSecurityContext{
				RunAsUser:    nimCache.GetUserID(),
				FSGroup:      nimCache.GetGroupID(),
				RunAsNonRoot: ptr.To[bool](true),
			},
			Containers:    []corev1.Container{},
			RestartPolicy: corev1.RestartPolicyNever,
			Volumes: []corev1.Volume{
				{
					Name: "nim-cache-volume",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
			},
			Tolerations:  nimBuild.GetTolerations(),
			NodeSelector: nimBuild.GetNodeSelectors(),
		},
	}

	// SeccompProfile must be set for TKGS
	if platformType == k8sutil.TKGS {
		pod.Spec.SecurityContext.SeccompProfile = &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		}
	}

	pod.Spec.Containers = []corev1.Container{
		{
			Name:    NIMBuildManifestContainerName,
			Image:   nimBuild.GetImage(),
			Command: getNIMBuildLocalManifestPodCommand(),
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: ptr.To[bool](false),
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
				},
				RunAsNonRoot: ptr.To[bool](true),
				RunAsGroup:   nimCache.GetGroupID(),
				RunAsUser:    nimCache.GetUserID(),
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "nim-cache-volume",
					MountPath: "/model-store",
					SubPath:   nimCache.Spec.Storage.PVC.SubPath,
				},
			},
		},
	}

	return pod
}

// getNIMBuildLocalManifestPodCommand is the command to run in the local manifest pod.
func getNIMBuildLocalManifestPodCommand() []string {
	return []string{
		"sh",
		"-c",
		strings.Join([]string{
			"cat /model-store/local_cache/local_manifest.yaml;",
			"sleep infinity",
		}, " "),
	}
}

// createPod creates a pod in the cluster.
func (r *NIMBuildReconciler) createPod(ctx context.Context, pod *corev1.Pod) error {
	// Create pod
	err := r.Create(ctx, pod)
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// updateManifestConfigMap updates the manifest ConfigMap in the cluster with the local manifest data.
func (r *NIMBuildReconciler) updateManifestConfigMap(ctx context.Context, nimCache *appsv1alpha1.NIMCache, manifestData *nimparser.NIMManifestInterface) error {
	// Convert manifestData to YAML
	manifestBytes, err := yaml.Marshal(manifestData)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest data: %w", err)
	}

	// Pretty-print the YAML content
	var prettyYAML interface{}
	err = yaml.Unmarshal(manifestBytes, &prettyYAML)
	if err != nil {
		return fmt.Errorf("failed to unmarshal manifest data for pretty-printing: %w", err)
	}

	prettyManifestBytes, err := yaml.Marshal(prettyYAML)
	if err != nil {
		return fmt.Errorf("failed to re-marshal manifest data for pretty-printing: %w", err)
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getManifestConfigName(nimCache),
			Namespace: nimCache.GetNamespace(),
			Labels: map[string]string{
				"app": nimCache.GetName(),
			},
		},
	}

	// Fetch the existing ConfigMap if it exists
	err = r.Get(ctx, client.ObjectKey{Name: configMap.Name, Namespace: configMap.Namespace}, configMap)
	if err != nil {
		return fmt.Errorf("failed to get ConfigMap %s: %w", configMap.Name, err)
	}

	// Update the data
	configMap.Data["local_model_manifest.yaml"] = string(prettyManifestBytes)

	// Create the ConfigMap
	if err := r.Update(ctx, configMap); err != nil {
		return fmt.Errorf("failed to create manifest ConfigMap %s: %w", configMap.Name, err)
	}
	return nil
}
