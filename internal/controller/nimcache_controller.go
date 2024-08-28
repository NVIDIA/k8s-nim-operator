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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	platform "github.com/NVIDIA/k8s-nim-operator/internal/controller/platform"
	"github.com/NVIDIA/k8s-nim-operator/internal/nimparser"
	"github.com/NVIDIA/k8s-nim-operator/internal/render"
	"github.com/NVIDIA/k8s-nim-operator/internal/shared"
	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
	"github.com/go-logr/logr"
	"gopkg.in/yaml.v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apiResource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// SelectedNIMProfilesAnnotationKey is the annotation key for auto-selected model profiles
	SelectedNIMProfilesAnnotationKey = "nvidia.com/selected-profiles"

	// NIMCacheFinalizer is the finalizer annotation
	NIMCacheFinalizer = "finalizer.nimcache.apps.nvidia.com"

	// AllProfiles represents all profiles in the NIM manifest
	AllProfiles = "all"

	// NIMCacheRole is the name of the role for all NIMCache instances in the namespace
	NIMCacheRole = "nim-cache-role"

	// NIMCacheRoleBinding is the name of the rolebinding for all NIMCache instances in the namespace
	NIMCacheRoleBinding = "nim-cache-rolebinding"

	// NIMCacheServiceAccount is the name of the serviceaccount for all NIMCache instances in the namespace
	NIMCacheServiceAccount = "nim-cache-sa"
)

// NIMCacheReconciler reconciles a NIMCache object
type NIMCacheReconciler struct {
	client.Client
	scheme   *runtime.Scheme
	log      logr.Logger
	Platform platform.Platform
	updater  conditions.Updater
}

// Ensure NIMCacheReconciler implements the Reconciler interface
var _ shared.Reconciler = &NIMCacheReconciler{}

// NewNIMCacheReconciler creates a new reconciler for NIMCache with the given platform
func NewNIMCacheReconciler(client client.Client, scheme *runtime.Scheme, log logr.Logger, platform platform.Platform) *NIMCacheReconciler {
	return &NIMCacheReconciler{
		Client:   client,
		scheme:   scheme,
		log:      log,
		Platform: platform,
	}
}

// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimcaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimcaches/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimcaches/finalizers,verbs=update
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=use,resourceNames=nonroot
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",resources=pods/log,verbs=get
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;create;delete
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NIMCache object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *NIMCacheReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the NIMCache instance
	nimCache := &appsv1alpha1.NIMCache{}
	if err := r.Get(ctx, req.NamespacedName, nimCache); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "unable to fetch NIMCache", "name", req.NamespacedName)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	logger.Info("Reconciling", "NIMCache", nimCache.Name)

	// Check if the instance is marked for deletion
	if nimCache.ObjectMeta.DeletionTimestamp.IsZero() {
		// Add finalizer if not present
		if !controllerutil.ContainsFinalizer(nimCache, NIMCacheFinalizer) {
			controllerutil.AddFinalizer(nimCache, NIMCacheFinalizer)
			if err := r.Update(ctx, nimCache); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The instance is being deleted
		if controllerutil.ContainsFinalizer(nimCache, NIMCacheFinalizer) {
			// Perform cleanup of resources
			if err := r.cleanupNIMCache(ctx, nimCache); err != nil {
				return ctrl.Result{}, err
			}

			// Remove finalizer to allow for deletion
			controllerutil.RemoveFinalizer(nimCache, NIMCacheFinalizer)
			if err := r.Update(ctx, nimCache); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}
	// Handle nim-cache reconciliation
	result, err := r.reconcileNIMCache(ctx, nimCache)
	if err != nil {
		obj := &appsv1alpha1.NIMCache{}
		errGet := r.Get(ctx, types.NamespacedName{Name: nimCache.Name, Namespace: nimCache.GetNamespace()}, obj)
		if errGet != nil {
			logger.Error(errGet, "error getting NIMCache", "name", nimCache.Name)
			return result, errGet
		}
		logger.Error(err, "error reconciling NIMCache", "name", nimCache.Name)
		conditions.UpdateCondition(&obj.Status.Conditions, appsv1alpha1.NimCacheConditionPVCCreated, metav1.ConditionFalse, "Failed", err.Error())
		obj.Status.State = appsv1alpha1.NimCacheStatusNotReady
		if err := r.Status().Update(ctx, obj); err != nil {
			logger.Error(err, "Failed to update status", "NIMCache", nimCache.Name)
			return result, err
		}
		return result, err
	}

	return result, nil
}

// GetScheme returns the scheme of the reconciler
func (r *NIMCacheReconciler) GetScheme() *runtime.Scheme {
	return r.scheme
}

// GetLogger returns the logger of the reconciler
func (r *NIMCacheReconciler) GetLogger() logr.Logger {
	return r.log
}

// GetClient returns the client instance
func (r *NIMCacheReconciler) GetClient() client.Client {
	return r.Client
}

// GetUpdater returns the conditions updater instance
func (r *NIMCacheReconciler) GetUpdater() conditions.Updater {
	return r.updater
}

// GetRenderer returns the renderer instance
func (r *NIMCacheReconciler) GetRenderer() render.Renderer {
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NIMCacheReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.NIMCache{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Complete(r)
}

func (r *NIMCacheReconciler) cleanupNIMCache(ctx context.Context, nimCache *appsv1alpha1.NIMCache) error {
	var errList []error
	logger := r.GetLogger()

	// TODO: Check if the cache is in use (allocated) and prevent deletion

	// All owned objects are garbage collected

	// Fetch the job
	jobName := types.NamespacedName{Name: nimCache.Name + "-job", Namespace: nimCache.Namespace}
	job := &batchv1.Job{}
	if err := r.Get(ctx, jobName, job); client.IgnoreNotFound(err) != nil {
		logger.Error(err, "unable to fetch the job for cleanup", "job", jobName)
		return err
	}

	// Delete associated stale pods in error
	podList := &corev1.PodList{}
	if job.Spec.Selector != nil {
		if err := r.List(context.Background(), podList, client.MatchingLabels(job.Spec.Selector.MatchLabels)); err != nil {
			logger.Error(err, "unable to list associated pods during cleanup", "job", jobName)
			errList = append(errList, err)
		}
	}

	for _, pod := range podList.Items {
		if err := r.Delete(context.Background(), &pod); err != nil {
			logger.Error(err, "unable to delete associated pods during cleanup", "job", jobName, "pod", pod.Name)
			errList = append(errList, err)
		}
	}

	if len(errList) > 0 {
		return fmt.Errorf("failed to cleanup resources: %v", errList)
	}

	return nil
}

func (r *NIMCacheReconciler) reconcileRole(ctx context.Context, nimCache *appsv1alpha1.NIMCache) error {
	logger := r.GetLogger()
	roleName := NIMCacheRole
	roleNamespacedName := types.NamespacedName{Name: roleName, Namespace: nimCache.GetNamespace()}

	// Desired Role configuration
	desiredRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: nimCache.GetNamespace(),
			Labels: map[string]string{
				"app": "k8s-nim-operator",
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups:     []string{"security.openshift.io"},
				Resources:     []string{"securitycontextconstraints"},
				ResourceNames: []string{"nonroot"},
				Verbs:         []string{"use"},
			},
		},
	}

	// Check if the Role already exists
	existingRole := &rbacv1.Role{}
	err := r.Get(ctx, roleNamespacedName, existingRole)
	if err != nil && client.IgnoreNotFound(err) != nil {
		logger.Error(err, "Failed to get Role", "Name", roleName)
		return err
	}

	if err != nil {
		// Role does not exist, create a new one
		logger.Info("Creating a new Role", "Name", roleName)

		err = r.Create(ctx, desiredRole)
		if err != nil {
			logger.Error(err, "Failed to create Role", "Name", roleName)
			return err
		}

		logger.Info("Successfully created Role", "Name", roleName)
	} else {
		// Role exists, check if it needs to be updated
		if !roleEqual(existingRole, desiredRole) {
			logger.Info("Updating existing Role", "Name", roleName)
			existingRole.Rules = desiredRole.Rules

			err = r.Update(ctx, existingRole)
			if err != nil {
				logger.Error(err, "Failed to update Role", "Name", roleName)
				return err
			}

			logger.Info("Successfully updated Role", "Name", roleName)
		}
	}

	return nil
}

// Helper function to check if two Roles are equal
func roleEqual(existing, desired *rbacv1.Role) bool {
	return utils.IsEqual(existing, desired, "Rules")
}

func (r *NIMCacheReconciler) reconcileRoleBinding(ctx context.Context, nimCache *appsv1alpha1.NIMCache) error {
	logger := r.GetLogger()
	rbName := NIMCacheRoleBinding
	rbNamespacedName := types.NamespacedName{Name: rbName, Namespace: nimCache.GetNamespace()}

	// Desired RoleBinding configuration
	desiredRB := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rbName,
			Namespace: nimCache.GetNamespace(),
			Labels: map[string]string{
				"app": "k8s-nim-operator",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     NIMCacheRole,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      NIMCacheServiceAccount,
				Namespace: nimCache.GetNamespace(),
			},
		},
	}

	// Check if the RoleBinding already exists
	existingRB := &rbacv1.RoleBinding{}
	err := r.Get(ctx, rbNamespacedName, existingRB)
	if err != nil && client.IgnoreNotFound(err) != nil {
		logger.Error(err, "Failed to get RoleBinding", "Name", rbName)
		return err
	}

	if err != nil {
		// RoleBinding does not exist, create a new one
		logger.Info("Creating a new RoleBinding", "Name", rbName)

		err = r.Create(ctx, desiredRB)
		if err != nil {
			logger.Error(err, "Failed to create RoleBinding", "Name", rbName)
			return err
		}

		logger.Info("Successfully created RoleBinding", "Name", rbName)
	} else {
		// RoleBinding exists, check if it needs to be updated
		if !roleBindingEqual(existingRB, desiredRB) {
			logger.Info("Updating existing RoleBinding", "Name", rbName)
			existingRB.RoleRef = desiredRB.RoleRef
			existingRB.Subjects = desiredRB.Subjects

			err = r.Update(ctx, existingRB)
			if err != nil {
				logger.Error(err, "Failed to update RoleBinding", "Name", rbName)
				return err
			}

			logger.Info("Successfully updated RoleBinding", "Name", rbName)
		}
	}

	return nil
}

// Helper function to check if two RoleBindings are equal
func roleBindingEqual(existing, desired *rbacv1.RoleBinding) bool {
	return utils.IsEqual(existing, desired, "RoleRef", "Subjects")
}

func (r *NIMCacheReconciler) reconcileServiceAccount(ctx context.Context, nimCache *appsv1alpha1.NIMCache) error {
	logger := r.GetLogger()
	saName := NIMCacheServiceAccount
	saNamespacedName := types.NamespacedName{Name: saName, Namespace: nimCache.GetNamespace()}

	sa := &corev1.ServiceAccount{}
	err := r.Get(ctx, saNamespacedName, sa)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return err
	}

	// If ServiceAccount does not exist, create a new one
	if err != nil {
		logger.Info("Creating a new ServiceAccount", "Name", saName)

		newSA := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      saName,
				Namespace: nimCache.GetNamespace(),
				Labels:    map[string]string{"app": "k8s-nim-operator"},
			},
		}

		// Create the ServiceAccount
		err = r.Create(ctx, newSA)
		if err != nil {
			logger.Error(err, "Failed to create ServiceAccount", "Name", saName)
			return err
		}

		logger.Info("Successfully created ServiceAccount", "Name", saName)
	}

	// If the ServiceAccount already exists, no action is needed
	return nil
}

func (r *NIMCacheReconciler) reconcilePVC(ctx context.Context, nimCache *appsv1alpha1.NIMCache) error {
	logger := r.GetLogger()
	pvcName := getPvcName(nimCache, nimCache.Spec.Storage.PVC)
	pvcNamespacedName := types.NamespacedName{Name: pvcName, Namespace: nimCache.GetNamespace()}
	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, pvcNamespacedName, pvc)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return err
	}

	// If PVC does not exist, create a new one if creation flag is enabled
	if err != nil {
		if nimCache.Spec.Storage.PVC.Create != nil && *nimCache.Spec.Storage.PVC.Create {
			pvc, err = shared.ConstructPVC(nimCache.Spec.Storage.PVC, metav1.ObjectMeta{Name: pvcName, Namespace: nimCache.GetNamespace()})
			if err != nil {
				logger.Error(err, "Failed to construct pvc", "name", pvcName)
				return err
			}
			if err := controllerutil.SetControllerReference(nimCache, pvc, r.GetScheme()); err != nil {
				return err
			}
			err = r.Create(ctx, pvc)
			if err != nil {
				logger.Error(err, "Failed to create pvc", "name", pvcName)
				return err
			}
			logger.Info("Created PVC for NIM Cache", "pvc", pvc.Name)

			conditions.UpdateCondition(&nimCache.Status.Conditions, appsv1alpha1.NimCacheConditionPVCCreated, metav1.ConditionTrue, "PVCCreated", "The PVC has been created for caching NIM model")
			nimCache.Status.State = appsv1alpha1.NimCacheStatusPVCCreated
		} else {
			logger.Error(err, "PVC doesn't exist and auto-creation is not enabled", "name", pvcNamespacedName)
			return err
		}
	}
	return nil
}

// Model selection required when
// NGC source is set and
// Model auto-selection is enabled and
// Explicit model profiles are not provided by the user
func isModelSelectionRequired(nimCache *appsv1alpha1.NIMCache) bool {
	if nimCache.Spec.Source.NGC != nil &&
		nimCache.Spec.Source.NGC.Model.AutoDetect != nil &&
		*nimCache.Spec.Source.NGC.Model.AutoDetect &&
		len(nimCache.Spec.Source.NGC.Model.Profiles) == 0 {
		return true
	}
	return false
}

func isModelSelectionDone(nimCache *appsv1alpha1.NIMCache) bool {
	if nimCache.Annotations != nil {
		if _, exists := nimCache.Annotations[SelectedNIMProfilesAnnotationKey]; exists {
			return true
		}
	}
	return false
}

func getSelectedProfiles(nimCache *appsv1alpha1.NIMCache) ([]string, error) {
	// Return profiles explicitly specified by the user in the spec
	if len(nimCache.Spec.Source.NGC.Model.Profiles) > 0 {
		return nimCache.Spec.Source.NGC.Model.Profiles, nil
	} else if isModelSelectionRequired(nimCache) {
		// Retrieve the selected profiles from the annotation
		var selectedProfiles []string
		if annotation, exists := nimCache.Annotations[SelectedNIMProfilesAnnotationKey]; exists {
			if err := json.Unmarshal([]byte(annotation), &selectedProfiles); err != nil {
				return nil, err
			}
		}
		return selectedProfiles, nil
	}
	return nil, nil
}

func (r *NIMCacheReconciler) reconcileModelManifest(ctx context.Context, nimCache *appsv1alpha1.NIMCache) (requeue bool, err error) {
	logger := r.GetLogger()

	// Model manifest is available only for NGC model pullers
	if nimCache.Spec.Source.NGC == nil {
		return false, nil
	}

	existingConfig := &corev1.ConfigMap{}
	cmName := getManifestConfigName(nimCache)
	err = r.Get(ctx, client.ObjectKey{Name: cmName, Namespace: nimCache.Namespace}, existingConfig)
	if err != nil && client.IgnoreNotFound(err) != nil {
		logger.Error(err, "failed to get configmap of the model manifest", "name", cmName)
		return false, err
	}

	// No action if the configmap is already created
	if err == nil {
		return false, nil
	}

	// Create a configmap by extracting the model manifest
	// Create a temporary pod for parsing model manifest
	pod := constructPodSpec(nimCache)
	// Add nimCache as owner for watching on status change
	if err := controllerutil.SetControllerReference(nimCache, pod, r.GetScheme()); err != nil {
		return false, err
	}
	err = r.createPod(ctx, pod)
	if err != nil {
		logger.Error(err, "failed to create", "pod", pod.Name)
		return false, err
	}

	existingPod := &corev1.Pod{}
	err = r.Get(ctx, client.ObjectKey{Name: pod.Name, Namespace: nimCache.Namespace}, existingPod)
	if err != nil {
		logger.Error(err, "failed to get pod for model selection", "pod", pod.Name)
		return false, err
	}

	if existingPod.Status.Phase != corev1.PodRunning {
		// requeue request with delay until the pod is ready
		return true, nil
	}

	// Extract manifest file
	output, err := r.getPodLogs(ctx, existingPod)
	if err != nil {
		logger.Error(err, "failed to get pod logs for parsing model manifest file", "pod", pod.Name)
		return false, err
	}

	if output == "" {
		logger.Info("Requeuing to wait for the manifest to be copied from the container")
		return true, nil
	}

	// Parse the file
	manifest, err := nimparser.ParseModelManifestFromRawOutput([]byte(output))
	if err != nil {
		logger.Error(err, "Failed to parse model manifest from the pod")
		return false, err
	}
	logger.V(2).Info("manifest file", "nimcache", nimCache.Name, "manifest", manifest)

	// Create a ConfigMap with the model manifest file for re-use
	err = r.createManifestConfigMap(ctx, nimCache, manifest)
	if err != nil {
		logger.Error(err, "Failed to create model manifest config map")
		return false, err
	}

	// Model manifest is successfully extracted, cleanup temporary pod
	err = r.Delete(ctx, existingPod)
	if err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to delete", "pod", pod.Name)
		// requeue request with delay until the pod is cleaned up
		// this is required as NIM containers are resource heavy
		return true, err
	}
	return false, nil
}

func (r *NIMCacheReconciler) reconcileModelSelection(ctx context.Context, nimCache *appsv1alpha1.NIMCache) error {
	logger := r.GetLogger()

	// reconcile model selection pod
	if isModelSelectionRequired(nimCache) && !isModelSelectionDone(nimCache) {
		var discoveredGPUs []string
		// If no specific GPUs are provided, then auto-detect GPUs in the cluster for profile selection
		if len(nimCache.Spec.Source.NGC.Model.GPUs) == 0 {
			gpusByNode, err := r.GetNodeGPUProducts(ctx)
			if err != nil {
				logger.Error(err, "Failed to get gpus in the cluster")
				return err
			}
			discoveredGPUs = getUniqueGPUProducts(gpusByNode)
		}

		// Get the model manifest from the config
		nimManifest, err := r.extractNIMManifest(ctx, getManifestConfigName(nimCache), nimCache.GetNamespace())
		if err != nil {
			return fmt.Errorf("failed to get model manifest config file: %w", err)
		}

		// Match profiles with user input
		profiles, err := nimparser.MatchProfiles(nimCache.Spec.Source.NGC.Model, *nimManifest, discoveredGPUs)
		if err != nil {
			logger.Error(err, "Failed to match profiles for given model parameters")
			return err
		}

		// Add the annotation to the NIMCache object
		if nimCache.Annotations == nil {
			nimCache.Annotations = map[string]string{}
		}

		profilesJSON, err := json.Marshal(profiles)
		if err != nil {
			logger.Error(err, "unable to marshal profiles to JSON")
			return err
		}

		nimCache.Annotations[SelectedNIMProfilesAnnotationKey] = string(profilesJSON)
	}
	return nil
}

func (r *NIMCacheReconciler) reconcileJob(ctx context.Context, nimCache *appsv1alpha1.NIMCache) error {
	logger := r.GetLogger()

	// reconcile model caching job
	job := &batchv1.Job{}
	jobName := types.NamespacedName{Name: getJobName(nimCache), Namespace: nimCache.GetNamespace()}
	err := r.Get(ctx, jobName, job)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return err
	}

	// If Job does not exist and caching is not complete, create a new one
	if err != nil && nimCache.Status.State != appsv1alpha1.NimCacheStatusReady {
		job, err := constructJob(nimCache)
		if err != nil {
			logger.Error(err, "Failed to construct job")
			return err
		}
		if err := controllerutil.SetControllerReference(nimCache, job, r.GetScheme()); err != nil {
			return err
		}
		err = r.Create(ctx, job)
		if err != nil {
			logger.Error(err, "Failed to create job")
			return err
		}
		logger.Info("Created Job for NIM Cache", "job", jobName)
		conditions.UpdateCondition(&nimCache.Status.Conditions, appsv1alpha1.NimCacheConditionJobCreated, metav1.ConditionTrue, "JobCreated", "The Job to cache NIM has been created")
		nimCache.Status.State = appsv1alpha1.NimCacheStatusStarted
		nimCache.Status.Profiles = []v1alpha1.NIMProfile{}
		return nil
	}

	// Reconcile the job status
	if err := r.reconcileJobStatus(ctx, nimCache, job); err != nil {
		return err
	}

	return nil
}

func (r *NIMCacheReconciler) reconcileJobStatus(ctx context.Context, nimCache *appsv1alpha1.NIMCache, job *batchv1.Job) error {
	logger := log.FromContext(ctx)
	jobName := job.Name

	switch {
	case job.Status.Succeeded > 0 && nimCache.Status.State != appsv1alpha1.NimCacheStatusReady:
		logger.Info("Job completed", "job", jobName)
		conditions.UpdateCondition(&nimCache.Status.Conditions, appsv1alpha1.NimCacheConditionJobCompleted, metav1.ConditionTrue, "JobCompleted", "The Job to cache NIM has successfully completed")
		nimCache.Status.State = appsv1alpha1.NimCacheStatusReady
		nimCache.Status.PVC = getPvcName(nimCache, nimCache.Spec.Storage.PVC)

		selectedProfiles, err := getSelectedProfiles(nimCache)
		if err != nil {
			return fmt.Errorf("failed to get selected profiles: %w", err)
		}

		if len(selectedProfiles) > 0 && !utils.ContainsElement(selectedProfiles, AllProfiles) {
			nimManifest, err := r.extractNIMManifest(ctx, getManifestConfigName(nimCache), nimCache.GetNamespace())
			if err != nil {
				return fmt.Errorf("failed to get model manifest config file: %w", err)
			}

			logger.V(2).Info("model manifest config", "manifest", nimManifest)

			// for selected profiles, update relevant info for status
			for profileName, profileData := range *nimManifest {
				for _, selectedProfile := range selectedProfiles {
					if profileName == selectedProfile {
						nimCache.Status.Profiles = append(nimCache.Status.Profiles, appsv1alpha1.NIMProfile{
							Name:    profileName,
							Model:   profileData.Model,
							Config:  profileData.Tags,
							Release: profileData.Release,
						})
					}
				}
			}
		}

	case job.Status.Failed > 0 && nimCache.Status.State != appsv1alpha1.NimCacheStatusFailed:
		logger.Info("Failed to cache NIM, job failed", "job", jobName)
		conditions.UpdateCondition(&nimCache.Status.Conditions, appsv1alpha1.NimCacheConditionJobCompleted, metav1.ConditionFalse, "JobFailed", "The Job to cache NIM has failed")
		nimCache.Status.State = appsv1alpha1.NimCacheStatusFailed
		nimCache.Status.Profiles = []v1alpha1.NIMProfile{}

	case job.Status.Active > 0 && nimCache.Status.State != appsv1alpha1.NimCacheStatusInProgress:
		logger.Info("Caching NIM is in progress, job running", "job", jobName)
		conditions.UpdateCondition(&nimCache.Status.Conditions, appsv1alpha1.NimCacheConditionJobPending, metav1.ConditionFalse, "JobRunning", "The Job to cache NIM is in progress")
		nimCache.Status.State = appsv1alpha1.NimCacheStatusInProgress
		nimCache.Status.Profiles = []v1alpha1.NIMProfile{}

	case job.Status.Active == 0 && nimCache.Status.State != appsv1alpha1.NimCacheStatusReady && nimCache.Status.State != appsv1alpha1.NimCacheStatusPending:
		logger.Info("Caching NIM is in progress, job pending", "job", jobName)
		conditions.UpdateCondition(&nimCache.Status.Conditions, appsv1alpha1.NimCacheConditionJobPending, metav1.ConditionTrue, "JobPending", "The Job to cache NIM is in pending state")
		nimCache.Status.State = appsv1alpha1.NimCacheStatusPending
		nimCache.Status.Profiles = []v1alpha1.NIMProfile{}

	}

	return nil
}

func (r *NIMCacheReconciler) createPod(ctx context.Context, pod *corev1.Pod) error {
	// Create pod
	err := r.Create(ctx, pod)
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (r *NIMCacheReconciler) reconcileNIMCache(ctx context.Context, nimCache *appsv1alpha1.NIMCache) (ctrl.Result, error) {
	logger := r.GetLogger()

	// Reconcile ServiceAccount
	err := r.reconcileServiceAccount(ctx, nimCache)
	if err != nil {
		logger.Error(err, "reconciliation of serviceaccount failed")
		return ctrl.Result{}, err
	}

	// Reconcile Role
	err = r.reconcileRole(ctx, nimCache)
	if err != nil {
		logger.Error(err, "reconciliation of role failed")
		return ctrl.Result{}, err
	}

	// Reconcile RoleBinding
	err = r.reconcileRoleBinding(ctx, nimCache)
	if err != nil {
		logger.Error(err, "reconciliation of rolebinding failed")
		return ctrl.Result{}, err
	}

	// Reconcile PVC
	err = r.reconcilePVC(ctx, nimCache)
	if err != nil {
		logger.Error(err, "reconciliation of pvc failed", "pvc", getPvcName(nimCache, nimCache.Spec.Storage.PVC))
		return ctrl.Result{}, err
	}

	requeue, err := r.reconcileModelManifest(ctx, nimCache)
	if err != nil {
		logger.Error(err, "reconciliation to extract model manifest failed", "pod", getPodName(nimCache))
		return ctrl.Result{}, err
	}

	if requeue {
		logger.V(2).Info("requeueing for reconciliation for model selection", "pod", getPodName(nimCache))
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	// Reconcile NIM model selection
	err = r.reconcileModelSelection(ctx, nimCache)
	if err != nil {
		logger.Error(err, "reconciliation of model selection failed")
		return ctrl.Result{}, err
	}

	// Reconcile caching Job
	err = r.reconcileJob(ctx, nimCache)
	if err != nil {
		logger.Error(err, "reconciliation of caching job failed", "job", getJobName(nimCache))
		return ctrl.Result{}, err
	}

	obj := &appsv1alpha1.NIMCache{}
	errGet := r.Get(ctx, types.NamespacedName{Name: nimCache.Name, Namespace: nimCache.GetNamespace()}, obj)
	if errGet != nil {
		logger.Error(errGet, "error getting NIMCache", "name", nimCache.Name)
		return ctrl.Result{}, errGet
	}
	obj.Status = nimCache.Status
	if err := r.Status().Update(ctx, obj); err != nil {
		logger.Error(err, "Failed to update status", "NIMCache", nimCache.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func getJobName(nimCache *appsv1alpha1.NIMCache) string {
	return fmt.Sprintf("%s-job", nimCache.GetName())
}

func getPvcName(parent client.Object, pvc appsv1alpha1.PersistentVolumeClaim) string {
	pvcName := fmt.Sprintf("%s-pvc", parent.GetName())
	if pvc.Name != "" {
		pvcName = pvc.Name
	}
	return pvcName
}

func getPodName(nimCache *appsv1alpha1.NIMCache) string {
	return fmt.Sprintf("%s-pod", nimCache.GetName())
}

func getManifestConfigName(nimCache *appsv1alpha1.NIMCache) string {
	return fmt.Sprintf("%s-manifest", nimCache.GetName())
}

// constructPodSpec constructs a Pod specification
func constructPodSpec(nimCache *appsv1alpha1.NIMCache) *corev1.Pod {
	labels := map[string]string{
		"app":                          "k8s-nim-operator",
		"app.kubernetes.io/name":       nimCache.Name,
		"app.kubernetes.io/managed-by": "k8s-nim-operator",
	}

	annotations := map[string]string{
		"openshift.io/scc": "nonroot",
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getPodName(nimCache),
			Namespace:   nimCache.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "nim-cache",
					Image:   nimCache.Spec.Source.NGC.ModelPuller,
					Command: []string{"sh", "-c", "cat /etc/nim/config/model_manifest.yaml; sleep infinity"},
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
			},
			SecurityContext: &corev1.PodSecurityContext{
				RunAsUser:    nimCache.GetUserID(),
				FSGroup:      nimCache.GetGroupID(),
				RunAsNonRoot: ptr.To[bool](true),
			},
			ServiceAccountName: NIMCacheServiceAccount,
			Tolerations:        nimCache.GetTolerations(),
			NodeSelector:       nimCache.GetNodeSelectors(),
		},
	}

	pod.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
		{
			Name: nimCache.Spec.Source.NGC.PullSecret,
		},
	}

	return pod
}

func (r *NIMCacheReconciler) getPodLogs(ctx context.Context, pod *corev1.Pod) (string, error) {
	podLogOpts := corev1.PodLogOptions{}
	config, err := rest.InClusterConfig()
	if err != nil {
		return "", err
	}
	// create a clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", err
	}
	req := clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &podLogOpts)
	podLogs, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func constructJob(nimCache *appsv1alpha1.NIMCache) (*batchv1.Job, error) {
	pvcName := getPvcName(nimCache, nimCache.Spec.Storage.PVC)
	labels := map[string]string{
		"app":                          "k8s-nim-operator",
		"app.kubernetes.io/name":       nimCache.Name,
		"app.kubernetes.io/managed-by": "k8s-nim-operator",
	}

	annotations := map[string]string{
		"openshift.io/scc":        "nonroot",
		"sidecar.istio.io/inject": "false",
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nimCache.Name + "-job",
			Namespace: nimCache.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: annotations,
				},
				Spec: corev1.PodSpec{
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
			},
			BackoffLimit:            ptr.To[int32](5),   // retry max 5 times on failure
			TTLSecondsAfterFinished: ptr.To[int32](600), // cleanup automatically after job finishes
		},
	}
	if nimCache.Spec.Source.DataStore != nil {
		outputPath := "/output"
		if nimCache.Spec.Storage.HostPath != nil {
			outputPath = fmt.Sprintf("%v/%v", outputPath, *nimCache.Spec.Storage.HostPath)
		}
		var command []string
		if nimCache.Spec.Source.DataStore.ModelName != nil && nimCache.Spec.Source.DataStore.CheckpointName != nil {
			command = []string{"datastore-tools", "checkpoint", "download", "--model-name", *nimCache.Spec.Source.DataStore.ModelName, "--checkpoint-name", *nimCache.Spec.Source.DataStore.CheckpointName, "--path", outputPath, "--end-point", nimCache.Spec.Source.DataStore.Endpoint}
		} else if nimCache.Spec.Source.DataStore.DatasetName != nil {
			command = []string{"datastore-tools", "dataset", "download", "--dataset-name", *nimCache.Spec.Source.DataStore.DatasetName, "--path", outputPath, "--end-point", nimCache.Spec.Source.DataStore.Endpoint}
		} else {
			return nil, errors.NewBadRequest("either datasetName or (modelName and checkpointName) must be provided")
		}
		job.Spec.Template.Spec.Containers = []corev1.Container{
			{
				Name:    "nim-cache",
				Image:   nimCache.Spec.Source.DataStore.ModelPuller,
				EnvFrom: nimCache.Spec.Source.EnvFromSecrets(),
				Env:     []corev1.EnvVar{},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "nim-cache-volume",
						MountPath: "/output",
						SubPath:   nimCache.Spec.Storage.PVC.SubPath,
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
				Command: command,
			},
		}
		job.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: nimCache.Spec.Source.DataStore.PullSecret,
			},
		}
	} else if nimCache.Spec.Source.NGC != nil {
		job.Spec.Template.Spec.Containers = []corev1.Container{
			{
				Name:    "nim-cache",
				Image:   nimCache.Spec.Source.NGC.ModelPuller,
				Command: []string{"download-to-cache"},
				EnvFrom: nimCache.Spec.Source.EnvFromSecrets(),
				Env: []corev1.EnvVar{
					{
						Name:  "HF_HOME",
						Value: "/model-store/huggingface",
					},
					{
						Name:  "NIM_CACHE_PATH",
						Value: "/model-store",
					},
					{
						Name:  "NGC_HOME",
						Value: "/model-store/ngc",
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
						"nvidia.com/gpu": *apiResource.NewQuantity(int64(nimCache.Spec.Resources.GPUs), apiResource.DecimalExponent),
					},
					Requests: map[corev1.ResourceName]apiResource.Quantity{
						"cpu":            nimCache.Spec.Resources.CPU,
						"memory":         nimCache.Spec.Resources.Memory,
						"nvidia.com/gpu": *apiResource.NewQuantity(int64(nimCache.Spec.Resources.GPUs), apiResource.DecimalExponent),
					},
				},
				TerminationMessagePath:   "/dev/termination-log",
				TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
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
		job.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: nimCache.Spec.Source.NGC.PullSecret,
			},
		}
		// Pass specific profiles to download based on user selection or auto-selection
		selectedProfiles, err := getSelectedProfiles(nimCache)
		if err != nil {
			return nil, err
		}

		if len(selectedProfiles) == 0 && nimCache.Spec.Resources.GPUs == 0 {
			return nil, fmt.Errorf("No profiles are selected for caching and no GPUs are assigned to the pod for auto-selection")
		}

		if len(selectedProfiles) > 0 {
			if utils.ContainsElement(selectedProfiles, AllProfiles) {
				job.Spec.Template.Spec.Containers[0].Args = []string{"--all"}
			} else {
				job.Spec.Template.Spec.Containers[0].Args = []string{"--profiles"}
				job.Spec.Template.Spec.Containers[0].Args = append(job.Spec.Template.Spec.Containers[0].Args, selectedProfiles...)
			}
		}
	}
	return job, nil
}

// getConfigMap retrieves the given ConfigMap
func (r *NIMCacheReconciler) getConfigMap(ctx context.Context, name, namespace string) (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, configMap)
	return configMap, err
}

// extractNIMManifest extracts the NIMManifest from the ConfigMap data
func (r *NIMCacheReconciler) extractNIMManifest(ctx context.Context, configName, namespace string) (*nimparser.NIMManifest, error) {
	configMap, err := r.getConfigMap(ctx, configName, namespace)
	if err != nil {
		return nil, fmt.Errorf("unable to get ConfigMap %s: %w", configName, err)
	}

	data, ok := configMap.Data["model_manifest.yaml"]
	if !ok {
		return nil, fmt.Errorf("model_manifest.yaml not found in ConfigMap")
	}

	manifest, err := nimparser.ParseModelManifestFromRawOutput([]byte(data))
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal manifest data: %w", err)
	}
	return manifest, nil
}

// createManifestConfigMap creates a ConfigMap with the given model manifest data
func (r *NIMCacheReconciler) createManifestConfigMap(ctx context.Context, nimCache *appsv1alpha1.NIMCache, manifestData *nimparser.NIMManifest) error {
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
	if err != nil && client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to get ConfigMap %s: %w", configMap.Name, err)
	}

	if err == nil {
		// config map already exists, no need to update model manifest as it is immutable per NIM version
		return nil
	}

	if err := controllerutil.SetControllerReference(nimCache, configMap, r.GetScheme()); err != nil {
		return err
	}

	// Update the data
	configMap.Data = map[string]string{
		"model_manifest.yaml": string(prettyManifestBytes),
	}

	// Create the ConfigMap
	if err := r.Create(ctx, configMap); err != nil {
		return fmt.Errorf("failed to create manifest ConfigMap %s: %w", configMap.Name, err)
	}
	return nil
}

// GetNodeGPUProducts retrieves the value of the "nvidia.com/gpu.product" label from all nodes in the cluster,
// filtering nodes where this label is not empty.
func (r *NIMCacheReconciler) GetNodeGPUProducts(ctx context.Context) (map[string]string, error) {
	logger := r.GetLogger()

	// List all nodes
	nodeList := &corev1.NodeList{}
	err := r.Client.List(ctx, nodeList)
	if err != nil {
		logger.Error(err, "unable to list nodes to detect gpu types in the cluster")
		return nil, fmt.Errorf("unable to list gpu nodes: %w", err)
	}

	// Map to store node names and their GPU product labels
	nodeGPUProducts := make(map[string]string)

	// Iterate over the nodes and filter by the GPU product label
	for _, node := range nodeList.Items {
		if gpuProduct, ok := node.Labels["nvidia.com/gpu.product"]; ok && strings.TrimSpace(gpuProduct) != "" {
			nodeGPUProducts[node.Name] = gpuProduct
		}
	}

	return nodeGPUProducts, nil
}

// getUniqueGPUProducts extracts unique GPU product values from the map of node GPU products.
func getUniqueGPUProducts(nodeGPUProducts map[string]string) []string {
	gpuProductSet := make(map[string]struct{})
	for _, gpuProduct := range nodeGPUProducts {
		gpuProductSet[gpuProduct] = struct{}{}
	}

	uniqueGPUProducts := make([]string, 0, len(gpuProductSet))
	for gpuProduct := range gpuProductSet {
		uniqueGPUProducts = append(uniqueGPUProducts, gpuProduct)
	}

	return uniqueGPUProducts
}
