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
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	apiResource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	"github.com/NVIDIA/k8s-nim-operator/internal/controller/platform"
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
)

// NIMBuildReconciler reconciles a NIMBuild object.
type NIMBuildReconciler struct {
	client.Client
	scheme           *runtime.Scheme
	log              logr.Logger
	Platform         platform.Platform
	orchestratorType k8sutil.OrchestratorType
	updater          conditions.Updater
	recorder         record.EventRecorder
}

// Ensure NIMBuildReconciler implements the Reconciler interface.
var _ shared.Reconciler = &NIMBuildReconciler{}

// NewNIMBuildReconciler creates a new reconciler for NIMBuild with the given platform.
func NewNIMBuildReconciler(client client.Client, scheme *runtime.Scheme, log logr.Logger, platform platform.Platform) *NIMBuildReconciler {
	return &NIMBuildReconciler{
		Client:   client,
		scheme:   scheme,
		log:      log,
		Platform: platform,
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
	previousStatusState := nimBuild.Status.State

	defer func() {
		if err != nil {
			r.GetEventRecorder().Eventf(nimBuild, corev1.EventTypeWarning, "ReconcileFailed",
				"NIMBuild %s reconcile failed, msg: %s", nimBuild.Name, err.Error())
		} else if previousStatusState != nimBuild.Status.State {
			r.GetEventRecorder().Eventf(nimBuild, corev1.EventTypeNormal, nimBuild.Status.State,
				"NIMBuild %s reconcile success, new state: %s", nimBuild.Name, nimBuild.Status.State)
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
			return ctrl.Result{}, nil
		}
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
		nimBuild.Status.State = appsv1alpha1.NimBuildStatusNotReady

		err := r.updateNIMBuildStatus(ctx, nimBuild)
		if err != nil {
			logger.Error(err, "Failed to update NIMBuild status", "NIMBuild", nimBuild.Name)
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
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.NIMBuild{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Type assert to NIMBuild
				if oldNIMBuild, ok := e.ObjectOld.(*appsv1alpha1.NIMBuild); ok {
					newNIMBuild, ok := e.ObjectNew.(*appsv1alpha1.NIMBuild)
					if ok {
						// Handle case where object is marked for deletion
						if !newNIMBuild.ObjectMeta.DeletionTimestamp.IsZero() {
							return true
						}

						// Handle only spec updates
						return !reflect.DeepEqual(oldNIMBuild.Spec, newNIMBuild.Spec)
					}
				}
				// For other types we watch, reconcile them
				return true
			},
		}).
		Complete(r)
}

func (r *NIMBuildReconciler) cleanupNIMBuild(ctx context.Context, nimBuild *appsv1alpha1.NIMBuild) error {
	var errList []error
	logger := r.GetLogger()

	// All owned objects are garbage collected

	// Fetch the pod
	podName := types.NamespacedName{Name: nimBuild.Name + "-engine-build-pod", Namespace: nimBuild.Namespace}
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
	nimCacheNamespacedName := types.NamespacedName{Name: nimBuild.Spec.NIMCacheRef, Namespace: nimBuild.GetNamespace()}

	nimCache := &appsv1alpha1.NIMCache{}
	if err := r.Get(ctx, nimCacheNamespacedName, nimCache); err != nil {
		logger.Error(err, "unable to fetch NIMCache for NIMBuild", "NIMCache", nimCacheNamespacedName)
		return ctrl.Result{}, err
	}

	if nimCache.Status.State == appsv1alpha1.NimCacheStatusReady {
		err := r.reconcileEngineBuild(ctx, nimBuild, nimCache)
		if err != nil {
			logger.Error(err, "reconciliation of nimbuild failed")
			return ctrl.Result{}, err
		}
	} else {
		conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionWaitForNimCache, metav1.ConditionTrue, "NIMCache not ready", "Waiting for NIMCache to be ready before building engines")
		nimBuild.Status.State = appsv1alpha1.NimBuildStatusPending
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil

	}

	err := r.updateNIMBuildStatus(ctx, nimBuild)
	if err != nil {
		logger.Error(err, "Failed to update NIMBuild status", "NIMBuild", nimBuild.Name)
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *NIMBuildReconciler) reconcileEngineBuild(ctx context.Context, nimBuild *appsv1alpha1.NIMBuild, nimCache *appsv1alpha1.NIMCache) error {
	logger := r.GetLogger()
	// If the NIMCache is not in a state to build engines, return early
	// Wait for caching to complete before building engines
	buildableProfile := appsv1alpha1.NIMProfile{}
	if nimBuild.Spec.ProfileName == "" {
		buildableProfiles := getBuildableProfiles(nimCache)
		if buildableProfiles != nil {
			switch {
			case len(buildableProfiles) > 1:
				logger.Info("Multiple buildable profiles found", "Profiles", buildableProfiles)
				conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionMultipleBuildableProfilesFound, metav1.ConditionTrue, "MultipleBuildableProfilesFound", "Multiple buildable profiles found, please select one profile to build")
				nimBuild.Status.State = appsv1alpha1.NimBuildStatusFailed
				return nil
			case len(buildableProfiles) == 1:
				logger.Info("Single buildable profile found", "Profile", buildableProfiles[0].Name)
				conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionSingleBuildableProfilesFound, metav1.ConditionTrue, "BuildableProfileFound", "Single buildable profile found")
				buildableProfile = buildableProfiles[0]
			default:
				logger.Info("No buildable profiles found, skipping engine build")
				conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionNoBuildableProfilesFound, metav1.ConditionTrue, "NoBuildableProfilesFound", "No buildable profiles found for NIM Cache")
				nimBuild.Status.State = appsv1alpha1.NimBuildStatusFailed
				return nil
			}

		} else {
			logger.Info("No buildable profiles found, skipping engine build")
			conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionNoBuildableProfilesFound, metav1.ConditionTrue, "NoBuildableProfilesFound", "No buildable profiles found for NIM Cache")
			nimBuild.Status.State = appsv1alpha1.NimBuildStatusFailed
		}
	} else {
		foundProfile := getBuildableProfileByName(nimCache, nimBuild.Spec.ProfileName)
		if foundProfile == nil {
			logger.Info("No buildable profiles found", "ProfileName", nimBuild.Spec.ProfileName)
			conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionMultipleBuildableProfilesFound, metav1.ConditionTrue, "MultipleBuildableProfilesFound", "Multiple buildable profiles found, please select one profile to build")
			nimBuild.Status.State = appsv1alpha1.NimBuildStatusFailed
			return nil
		} else {
			logger.Info("Single buildable profile found", "Profile", foundProfile.Name)
			conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionSingleBuildableProfilesFound, metav1.ConditionTrue, "BuildableProfileFound", "Single buildable profile found")
			buildableProfile = *foundProfile
		}
	}

	pod := &corev1.Pod{}
	podName := types.NamespacedName{Name: nimBuild.Name + "-engine-build-pod", Namespace: nimBuild.GetNamespace()}
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
		err = r.Create(ctx, pod)
		if err != nil {
			logger.Error(err, "Failed to create pod")
			return err
		}
		logger.Info("Created pod for NIM Cache engine build", "pod", podName)
		conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionEngineBuildPodCreated, metav1.ConditionTrue, "EngineBuildPodCreated", "The pod to build engine has been created")
		return nil
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
		conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionEngineBuildPodCompleted, metav1.ConditionTrue, "PodReady", "The Pod to cache NIM has successfully completed")
		nimBuild.Status.State = appsv1alpha1.NimBuildStatusReady
		if err := r.deletePod(ctx, pod); err != nil {
			logger.Error(err, "Unable to delete NIM Cache build engine pod", "Name", pod.Name)
			return err
		}

	case pod.Status.Phase == corev1.PodFailed && !meta.IsStatusConditionTrue(nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionEngineBuildPodCompleted):
		logger.Info("Failed to cache NIM, build pod failed", "pod", pod)
		conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionEngineBuildPodCreated, metav1.ConditionFalse, "PodFailed", "The pod to build engine has failed")
		nimBuild.Status.State = appsv1alpha1.NimBuildStatusFailed

	case !isPodReady(pod) && !meta.IsStatusConditionTrue(nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionEngineBuildPodCompleted):
		logger.Info("Caching NIM is in progress, build engine pod running", "job", podName)
		conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionEngineBuildPodPending, metav1.ConditionTrue, "PodRunning", "The Pod to cache NIM is in progress")
		nimBuild.Status.State = appsv1alpha1.NimBuildStatusInProgress

	}

	return nil
}

func (r *NIMBuildReconciler) updateNIMBuildStatus(ctx context.Context, nimBuild *appsv1alpha1.NIMBuild) error {
	logger := r.GetLogger()
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
}

func (r *NIMBuildReconciler) constructEngineBuildPod(nimBuild *appsv1alpha1.NIMBuild, nimCache *appsv1alpha1.NIMCache, platformType k8sutil.OrchestratorType, nimProfile appsv1alpha1.NIMProfile) (*corev1.Pod, error) {
	logger := r.GetLogger()
	pvcName := shared.GetPVCName(nimCache, nimCache.Spec.Storage.PVC)
	labels := map[string]string{
		"app":                          "k8s-nim-operator",
		"app.kubernetes.io/name":       nimBuild.Name,
		"app.kubernetes.io/managed-by": "k8s-nim-operator",
	}

	annotations := map[string]string{
		"sidecar.istio.io/inject": "false",
	}

	// If no user-provided GPU resource is found, proceed with auto-assignment
	// Get tensorParallelism from the profile
	tensorParallelism, err := utils.GetTensorParallelismByProfileTags(nimProfile.Config)
	if err != nil {
		logger.Error(err, "Failed to retrieve tensorParallelism")
		return nil, err
	}

	gpuQuantity, err := apiResource.ParseQuantity(tensorParallelism)
	if err != nil {
		return nil, fmt.Errorf("failed to parse tensorParallelism: %w", err)
	}

	if platformType == k8sutil.OpenShift {
		if nimCache.GetProxySpec() != nil {
			annotations["openshift.io/scc"] = "anyuid"
		} else {
			annotations["openshift.io/scc"] = "nonroot"
		}
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        nimBuild.Name + "-engine-build-pod",
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
			ImagePullSecrets:   []corev1.LocalObjectReference{},
			ServiceAccountName: NIMCacheServiceAccount,
			Tolerations:        nimCache.GetTolerations(),
			NodeSelector:       nimCache.GetNodeSelectors(),
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
			Name:    NIMBuildContainerName,
			Image:   nimCache.Spec.Source.NGC.ModelPuller,
			EnvFrom: nimCache.Spec.Source.EnvFromSecrets(),
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
					Value: nimCache.Name + "-" + nimProfile.Name,
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
			Resources: corev1.ResourceRequirements{
				Limits: map[corev1.ResourceName]apiResource.Quantity{
					"cpu":            nimCache.Spec.Resources.CPU,
					"memory":         nimCache.Spec.Resources.Memory,
					"nvidia.com/gpu": gpuQuantity,
				},
				Requests: map[corev1.ResourceName]apiResource.Quantity{
					"cpu":            nimCache.Spec.Resources.CPU,
					"memory":         nimCache.Spec.Resources.Memory,
					"nvidia.com/gpu": gpuQuantity,
				},
			},
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
	pod.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
		{
			Name: nimCache.Spec.Source.NGC.PullSecret,
		},
	}

	// Merge env with the user provided values
	pod.Spec.Containers[0].Env = utils.MergeEnvVars(pod.Spec.Containers[0].Env, nimCache.Spec.Env)

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
