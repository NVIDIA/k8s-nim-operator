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

package v1alpha1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NIMCacheSpec defines the desired state of NIMCache
type NIMCacheSpec struct {
	// Source is the NIM model source to cache
	Source NIMSource `json:"source"`
	// Storage is the target storage for caching NIM model
	Storage Storage `json:"storage"`
	// Resources defines the minimum resources required for the caching job to run(cpu, memory, gpu).
	Resources Resources `json:"resources,omitempty"`
	// Tolerations for running the job to cache the NIM model
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// NodeSelectors are the node selector labels to schedule the caching job.
	NodeSelectors map[string]string `json:"gpuSelectors,omitempty"`
	UserID        *int64            `json:"userID,omitempty"`
	GroupID       *int64            `json:"groupID,omitempty"`
}

// NIMSource defines the source for caching NIM model
type NIMSource struct {
	// NGCSource represents models stored in NGC
	NGC *NGCSource `json:"ngc,omitempty"`

	// NGCSource represents models stored in NVIDIA DataStore service
	DataStore *DataStoreSource `json:"dataStore,omitempty"`
}

// NGCSource references a model stored on NVIDIA NGC
type NGCSource struct {
	// The name of an existing pull secret containing the NGC_API_KEY
	AuthSecret string `json:"authSecret"`
	// ModelPuller is the container image that can pull the model
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="modelPuller is an immutable field. Please create a new NIMCache resource instead when you want to change this container."
	ModelPuller string `json:"modelPuller"`
	// PullSecret to pull the model puller image
	PullSecret string `json:"pullSecret,omitempty"`
	// Model spec for caching
	Model ModelSpec `json:"model,omitempty"`
}

// ModelSpec is the spec required to cache selected models
type ModelSpec struct {
	// Profiles are the specific model profiles to cache
	Profiles []string `json:"profiles,omitempty"`
	// AutoDetect enables auto-detection of model profiles with specified model parameters
	AutoDetect *bool `json:"autoDetect,omitempty"`
	// Precision is the precision for model quantization
	Precision string `json:"precision,omitempty"`
	// Engine is the backend engine (tensort_llm, vllm)
	Engine string `json:"engine,omitempty"`
	// TensorParallelism is the minimum GPUs required for the model computations
	TensorParallelism string `json:"tensorParallelism,omitempty"`
	// QoSProfile is the supported QoS profile types for the models (throughput, latency)
	QoSProfile string `json:"qosProfile,omitempty"`
	// GPU is the spec for matching GPUs for caching optimized models
	GPUs []GPUSpec `json:"gpus,omitempty"`
	// Lora indicates a finetuned model with LoRa adapters
	Lora *bool `json:"lora,omitempty"`
}

// GPUSpec is the spec required to cache models for selected gpu type
type GPUSpec struct {
	// Product is the GPU product string (h100, a100, l40s)
	Product string `json:"product,omitempty"`
	// IDs are the device-ids for a specific GPU SKU
	IDs []string `json:"ids,omitempty"`
}

// DataStoreSource references a model stored on NVIDIA DataStore service
type DataStoreSource struct {
	// The endpoint for datastore
	Endpoint string `json:"endpoint"`
	// Name of either model/checkpoint or dataset to download
	ModelName      *string `json:"modelName,omitempty"`
	CheckpointName *string `json:"checkpointName,omitempty"`
	DatasetName    *string `json:"datasetName,omitempty"`
	// The name of an existing auth secret containing the AUTH_TOKEN"
	AuthSecret string `json:"authSecret"`
	// ModelPuller is the container image that can pull the model
	ModelPuller string `json:"modelPuller"`
	// PullSecret for the model puller image
	PullSecret string `json:"pullSecret,omitempty"`
}

// Storage defines the attributes of various storage targets used to store the model
type Storage struct {
	// PersistentVolumeClaim is the pvc volume used for caching NIM
	PVC PersistentVolumeClaim `json:"pvc,omitempty"`
	// HostPath is the host path volume for caching NIM
	HostPath *string `json:"hostPath,omitempty"`
}

// PersistentVolumeClaim defines the attributes of PVC used as a source for caching NIM model
type PersistentVolumeClaim struct {
	// Create indicates to create a new PVC
	Create *bool `json:"create,omitempty"`
	// Name is the name of the PVC
	Name string `json:"name,omitempty"`
	// StorageClass to be used for PVC creation. Leave it as empty if the PVC is already created.
	StorageClass string `json:"storageClass,omitempty"`
	// Size of the NIM cache in Gi, used during PVC creation
	Size string `json:"size,omitempty"`
	// VolumeAccessMode is the volume access mode of the PVC
	VolumeAccessMode corev1.PersistentVolumeAccessMode `json:"volumeAccessMode,omitempty"`
	SubPath          string                            `json:"subPath,omitempty"`
}

// NIMCacheStatus defines the observed state of NIMCache
type NIMCacheStatus struct {
	State      string             `json:"state,omitempty"`
	PVC        string             `json:"pvc,omitempty"`
	Profiles   []NIMProfile       `json:"profiles,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// NIMProfile defines the profiles that were cached
type NIMProfile struct {
	Name    string            `json:"name,omitempty"`
	Model   string            `json:"model,omitempty"`
	Release string            `json:"release,omitempty"`
	Config  map[string]string `json:"config,omitempty"`
}

// Resources defines the minimum resources required for caching NIM.
// +kubebuilder:validation:Required
type Resources struct {
	// GPUs specifies the number of GPUs to assign to the caching job.
	// +kubebuilder:validation:Minimum=0
	GPUs int `json:"gpus,omitempty"`
	// CPU indicates the minimum number of CPUs to use while caching NIM
	CPU resource.Quantity `json:"cpu,omitempty"`
	// Memory indicates the minimum amount of memory to use while caching NIM
	// Valid values are numbers followed by one of the suffixes Ki, Mi, Gi, or Ti (e.g. "4Gi", "4096Mi").
	Memory resource.Quantity `json:"memory,omitempty"`
}

const (
	// NimCacheConditionJobCreated indicates that the caching job is created.
	NimCacheConditionJobCreated = "NIM_CACHE_JOB_CREATED"
	// NimCacheConditionJobCompleted indicates that the caching job is completed.
	NimCacheConditionJobCompleted = "NIM_CACHE_JOB_COMPLETED"
	// NimCacheConditionJobPending indicates that the caching job is in pending state.
	NimCacheConditionJobPending = "NIM_CACHE_JOB_PENDING"
	// NimCacheConditionPVCCreated indicates that the caching pvc is created.
	NimCacheConditionPVCCreated = "NIM_CACHE_PVC_CREATED"

	// NimCacheStatusNotReady indicates that cache is not ready
	NimCacheStatusNotReady = "NotReady"
	// NimCacheStatusPVCCreated indicates that the pvc is created for caching
	NimCacheStatusPVCCreated = "PVC-Created"
	// NimCacheStatusStarted indicates that caching process is started
	NimCacheStatusStarted = "Started"
	// NimCacheStatusReady indicates that cache is ready
	NimCacheStatusReady = "Ready"
	// NimCacheStatusInProgress indicates that caching is in progress
	NimCacheStatusInProgress = "InProgress"
	// NimCacheStatusPending indicates that caching is not yet started
	NimCacheStatusPending = "Pending"
	// NimCacheStatusFailed indicates that caching is failed
	NimCacheStatusFailed = "Failed"
)

// EnvFromSecrets return the list of secrets that should be mounted as env vars
func (s *NIMSource) EnvFromSecrets() []v1.EnvFromSource {
	if s.NGC != nil && s.NGC.AuthSecret != "" {
		return []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: s.NGC.AuthSecret,
					},
				},
			},
		}
	} else if s.DataStore != nil && s.DataStore.AuthSecret != "" {
		return []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: s.DataStore.AuthSecret,
					},
				},
			},
		}
	}
	// no secrets to source the env variables
	return []corev1.EnvFromSource{}
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.state`,priority=0
// +kubebuilder:printcolumn:name="PVC",type=string,JSONPath=`.status.pvc`,priority=0
// +kubebuilder:printcolumn:name="Age",type=string,JSONPath=`.metadata.creationTimestamp`,priority=0

// NIMCache is the Schema for the nimcaches API
type NIMCache struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NIMCacheSpec   `json:"spec,omitempty"`
	Status NIMCacheStatus `json:"status,omitempty"`
}

// GetPVCName returns the name to be used for the PVC based on the custom spec
// Prefers pvc.Name if explicitly set by the user in the NIMCache instance
func (n *NIMCache) GetPVCName(pvc PersistentVolumeClaim) string {
	pvcName := fmt.Sprintf("%s-pvc", n.GetName())
	if pvc.Name != "" {
		pvcName = pvc.Name
	}
	return pvcName
}

// GetUserID returns user ID. Returns default value if not set on NimCache object.
func (n *NIMCache) GetUserID() *int64 {
	if n.Spec.UserID == nil {
		return ptr.To[int64](1000)
	}
	return n.Spec.UserID
}

// GetGroupID returns group ID. Returns default value if not set on NimCache object.
func (n *NIMCache) GetGroupID() *int64 {
	if n.Spec.GroupID == nil {
		return ptr.To[int64](2000)
	}
	return n.Spec.GroupID
}

// +kubebuilder:object:root=true

// NIMCacheList contains a list of NIMCache
type NIMCacheList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NIMCache `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NIMCache{}, &NIMCacheList{})
}
