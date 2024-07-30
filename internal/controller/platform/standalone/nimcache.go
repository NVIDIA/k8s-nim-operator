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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/NVIDIA/k8s-nim-operator/internal/nimparser"
	"github.com/NVIDIA/k8s-nim-operator/internal/render"
	"github.com/go-logr/logr"
	"gopkg.in/yaml.v2"

	"github.com/NVIDIA/k8s-nim-operator/api/v1alpha1"
	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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
)

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

// GetRenderer returns the renderer instance
func (r *NIMCacheReconciler) GetRenderer() render.Renderer {
	return nil
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
			pvc, err = constructPVC(nimCache.Spec.Storage.PVC, metav1.ObjectMeta{Name: pvcName, Namespace: nimCache.GetNamespace()})
			if err != nil {
				logger.Error(err, "Failed to construct pvc", "name", pvc.Name)
				return err
			}
			if err := controllerutil.SetControllerReference(nimCache, pvc, r.GetScheme()); err != nil {
				return err
			}
			err = r.Create(ctx, pvc)
			if err != nil {
				logger.Error(err, "Failed to create pvc", "name", pvc.Name)
				return err
			}
			logger.Info("Created PVC for NIM Cache", "pvc", pvcName)

			updateCondition(&nimCache.Status.Conditions, appsv1alpha1.NimCacheConditionPVCCreated, metav1.ConditionTrue, "PVCCreated", "The PVC has been created for caching NIM")
			nimCache.Status.State = appsv1alpha1.NimCacheStatusPVCCreated
			if err := r.Status().Update(ctx, nimCache); err != nil {
				logger.Error(err, "Failed to update status", "NIMCache", nimCache.Name)
				return err
			}
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

func (r *NIMCacheReconciler) reconcileModelSelection(ctx context.Context, nimCache *appsv1alpha1.NIMCache) (requeue bool, err error) {
	logger := r.GetLogger()

	// reconcile model selection pod
	if isModelSelectionRequired(nimCache) && !isModelSelectionDone(nimCache) {
		// Create a temporary pod for parsing model manifest
		pod := constructPodSpec(nimCache)
		// Add nimCache as owner for watching on status change
		if err := controllerutil.SetControllerReference(nimCache, pod, r.GetScheme()); err != nil {
			return false, err
		}
		err := r.createPod(ctx, pod)
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

		var discoveredGPUs []string
		// If no specific GPUs are provided, then auto-detect GPUs in the cluster for profile selection
		if len(nimCache.Spec.Source.NGC.Model.GPUs) == 0 {
			gpusByNode, err := r.GetNodeGPUProducts(ctx)
			if err != nil {
				logger.Error(err, "Failed to get gpus in the cluster")
				return false, err
			}
			discoveredGPUs = getUniqueGPUProducts(gpusByNode)
		}

		// Match profiles with user input
		profiles, err := nimparser.MatchProfiles(nimCache.Spec.Source.NGC.Model, *manifest, discoveredGPUs)
		if err != nil {
			logger.Error(err, "Failed to match profiles for given model parameters")
			return false, err
		}

		// Add the annotation to the NIMCache object
		if nimCache.Annotations == nil {
			nimCache.Annotations = map[string]string{}
		}

		profilesJSON, err := json.Marshal(profiles)
		if err != nil {
			logger.Error(err, "unable to marshal profiles to JSON")
			return false, err
		}

		nimCache.Annotations[SelectedNIMProfilesAnnotationKey] = string(profilesJSON)
		if err := r.Update(ctx, nimCache); err != nil {
			logger.Error(err, "unable to update NIMCache with selected profiles annotation")
			return false, err
		}

		// Selected profiles updated, cleanup temporary pod
		err = r.Delete(ctx, existingPod)
		if err != nil && !errors.IsNotFound(err) {
			logger.Error(err, "failed to delete", "pod", pod.Name)
			// requeue request with delay until the pod is cleaned up
			// this is required as NIM containers are resource heavy
			return true, err
		}
	}
	return false, nil
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
			logger.Error(err, "Failed to construct job", "name", getPvcName(nimCache, nimCache.Spec.Storage.PVC))
			return err
		}
		if err := controllerutil.SetControllerReference(nimCache, job, r.GetScheme()); err != nil {
			return err
		}
		err = r.Create(ctx, job)
		if err != nil {
			logger.Error(err, "Failed to create job", "name", getPvcName(nimCache, nimCache.Spec.Storage.PVC))
			return err
		}
		logger.Info("Created Job for NIM Cache", "job", jobName)
		updateCondition(&nimCache.Status.Conditions, appsv1alpha1.NimCacheConditionJobCreated, metav1.ConditionTrue, "JobCreated", "The Job to cache NIM has been created")
		nimCache.Status.State = appsv1alpha1.NimCacheStatusStarted
		nimCache.Status.Profiles = []v1alpha1.NIMProfile{}
		if err := r.Status().Update(ctx, nimCache); err != nil {
			return err
		}
		// return to reconcile later on job status update
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
		updateCondition(&nimCache.Status.Conditions, appsv1alpha1.NimCacheConditionJobCompleted, metav1.ConditionTrue, "JobCompleted", "The Job to cache NIM has successfully completed")
		nimCache.Status.State = appsv1alpha1.NimCacheStatusReady
		nimCache.Status.PVC = getPvcName(nimCache, nimCache.Spec.Storage.PVC)

		selectedProfiles, err := getSelectedProfiles(nimCache)
		if err != nil {
			return fmt.Errorf("failed to get selected profiles: %w", err)
		}

		if len(selectedProfiles) > 0 {
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

		if err := r.Status().Update(ctx, nimCache); err != nil {
			return fmt.Errorf("failed to update status: %w", err)
		}

	case job.Status.Failed > 0 && nimCache.Status.State != appsv1alpha1.NimCacheStatusFailed:
		logger.Info("Failed to cache NIM, job failed", "job", jobName)
		updateCondition(&nimCache.Status.Conditions, appsv1alpha1.NimCacheConditionJobCompleted, metav1.ConditionFalse, "JobFailed", "The Job to cache NIM has failed")
		nimCache.Status.State = appsv1alpha1.NimCacheStatusFailed
		nimCache.Status.Profiles = []v1alpha1.NIMProfile{}

		if err := r.Status().Update(ctx, nimCache); err != nil {
			return fmt.Errorf("failed to update status: %w", err)
		}

	case job.Status.Active > 0 && nimCache.Status.State != appsv1alpha1.NimCacheStatusInProgress:
		logger.Info("Caching NIM is in progress, job running", "job", jobName)
		updateCondition(&nimCache.Status.Conditions, appsv1alpha1.NimCacheConditionJobPending, metav1.ConditionFalse, "JobRunning", "The Job to cache NIM is in progress")
		nimCache.Status.State = appsv1alpha1.NimCacheStatusInProgress
		nimCache.Status.Profiles = []v1alpha1.NIMProfile{}

		if err := r.Status().Update(ctx, nimCache); err != nil {
			return fmt.Errorf("failed to update status: %w", err)
		}

	case job.Status.Active == 0 && nimCache.Status.State != appsv1alpha1.NimCacheStatusReady && nimCache.Status.State != appsv1alpha1.NimCacheStatusPending:
		logger.Info("Caching NIM is in progress, job pending", "job", jobName)
		updateCondition(&nimCache.Status.Conditions, appsv1alpha1.NimCacheConditionJobPending, metav1.ConditionTrue, "JobPending", "The Job to cache NIM is in pending state")
		nimCache.Status.State = appsv1alpha1.NimCacheStatusPending
		nimCache.Status.Profiles = []v1alpha1.NIMProfile{}

		if err := r.Status().Update(ctx, nimCache); err != nil {
			return fmt.Errorf("failed to update status: %w", err)
		}
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

	// Reconcile PVC
	err := r.reconcilePVC(ctx, nimCache)
	if err != nil {
		logger.Error(err, "reconciliation of pvc failed", "pvc", getPvcName(nimCache, nimCache.Spec.Storage.PVC))
		return ctrl.Result{}, err
	}

	// Reconcile NIM model selection
	requeue, err := r.reconcileModelSelection(ctx, nimCache)
	if err != nil {
		logger.Error(err, "reconciliation of model selection failed", "pod", getPodName(nimCache))
		return ctrl.Result{}, err
	}

	if requeue {
		logger.Info("requeueing for reconciliation for model selection", "pod", getPodName(nimCache))
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	// Reconcile caching Job
	err = r.reconcileJob(ctx, nimCache)
	if err != nil {
		logger.Error(err, "reconciliation of caching job failed", "job", getJobName(nimCache))
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func getJobName(nimCache *appsv1alpha1.NIMCache) string {
	return fmt.Sprintf("%s-job", nimCache.GetName())
}

func getPvcName(parent client.Object, pvc appsv1alpha1.PersistentVolumeClaim) string {
	pvcName := fmt.Sprintf("%s-pvc", parent.GetName())
	if pvc.Name != nil {
		pvcName = *pvc.Name
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
		"openshift.io/scc": "anyuid",
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
						RunAsGroup:   ptr.To[int64](2000),
					},
				},
			},
			SecurityContext: &corev1.PodSecurityContext{
				RunAsUser:    ptr.To[int64](1000),
				FSGroup:      ptr.To[int64](2000),
				RunAsNonRoot: ptr.To[bool](true),
			},
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
		"openshift.io/scc":        "anyuid",
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
						RunAsUser:    ptr.To[int64](1000),
						FSGroup:      ptr.To[int64](2000),
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
					ImagePullSecrets: []corev1.LocalObjectReference{},
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
					},
				},
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: ptr.To[bool](false),
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{"ALL"},
					},
					RunAsNonRoot: ptr.To[bool](true),
					RunAsGroup:   ptr.To[int64](2000),
					RunAsUser:    ptr.To[int64](1000),
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
						Value: "/model-store", // Need to be set to a writable directory by non-root user
					},
					{
						Name:  "NIM_CACHE_PATH", // Note: in the download mode, NIM_CACHE_PATH is not used
						Value: "/model-store",
					},
					{
						Name:  "NGC_HOME", // Note: NGC_HOME is required and handled as NIM_CACHE_PATH in the download mode
						Value: "/model-store",
					},
				},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "nim-cache-volume",
						MountPath: "/model-store",
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
					RunAsGroup:   ptr.To[int64](2000),
					RunAsUser:    ptr.To[int64](1000),
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
		if selectedProfiles != nil {
			job.Spec.Template.Spec.Containers[0].Args = []string{"--profiles"}
			job.Spec.Template.Spec.Containers[0].Args = append(job.Spec.Template.Spec.Containers[0].Args, selectedProfiles...)
		}
	}
	return job, nil
}

func constructPVC(pvc appsv1alpha1.PersistentVolumeClaim, pvcMeta metav1.ObjectMeta) (*corev1.PersistentVolumeClaim, error) {
	storageClassName := pvc.StorageClass
	size, err := resource.ParseQuantity(pvc.Size)
	if err != nil {
		return nil, fmt.Errorf("failed to parse size for pvc creation %s, err %v", pvcMeta.Name, err)
	}
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: pvcMeta,
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{pvc.VolumeAccessMode},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: size,
				},
			},
			StorageClassName: &storageClassName,
		},
	}, nil
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

func updateCondition(conditions *[]metav1.Condition, conditionType string, status metav1.ConditionStatus, reason, message string) {
	for i := range *conditions {
		if (*conditions)[i].Type == conditionType {
			// existing condition
			(*conditions)[i].Status = status
			(*conditions)[i].LastTransitionTime = metav1.Now()
			(*conditions)[i].Reason = reason
			(*conditions)[i].Message = message
			// condition updated
			return
		}
	}
	// new condition
	*conditions = append(*conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
	// condition updated
}
