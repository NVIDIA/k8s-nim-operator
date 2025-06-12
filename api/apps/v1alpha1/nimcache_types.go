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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NIMCacheSpec defines the desired state of NIMCache.
type NIMCacheSpec struct {
	// Source is the NIM model source to cache
	Source NIMSource `json:"source"`
	// Storage is the target storage for caching NIM model
	Storage NIMCacheStorage `json:"storage"`
	// Resources defines the minimum resources required for the caching job to run(cpu, memory, gpu).
	Resources Resources `json:"resources,omitempty"`
	// Tolerations for running the job to cache the NIM model
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// NodeSelector is the node selector labels to schedule the caching job.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// UserID is the user ID for the caching job
	UserID *int64 `json:"userID,omitempty"`
	// GroupID is the group ID for the caching job
	GroupID *int64 `json:"groupID,omitempty"`
	// CertConfig is the name of the ConfigMap containing the custom certificates.
	// for secure communication.
	// Deprecated: use `Proxy` instead to configure custom certificates for using proxy.
	// +optional
	CertConfig *CertConfig `json:"certConfig,omitempty"`
	// Env are the additional custom environment variabes for the caching job
	Env []corev1.EnvVar `json:"env,omitempty"`
	// RuntimeClassName is the runtimeclass for the caching job
	RuntimeClassName string     `json:"runtimeClassName,omitempty"`
	Proxy            *ProxySpec `json:"proxy,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="(has(self.ngc) ? 1 : 0) + (has(self.dataStore) ? 1 : 0) + (has(self.hf) ? 1 : 0) == 1",message="Exactly one of ngc, dataStore, or hf must be defined"
// NIMSource defines the source for caching NIM model.
type NIMSource struct {
	// NGCSource represents models stored in NGC
	NGC *NGCSource `json:"ngc,omitempty"`

	// DataStore represents models stored in NVIDIA NeMo DataStore service
	DataStore *NemoDataStoreSource `json:"dataStore,omitempty"`
	// HuggingFaceHub represents models stored in HuggingFace Hub
	HF *HuggingFaceHubSource `json:"hf,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="(has(self.modelName) ? 1 : 0) + (has(self.datasetName) ? 1 : 0) == 1",message="Exactly one of modelName or datasetName must be defined"
type DSHFCommonFields struct {
	// modelName is the name of the model
	ModelName *string `json:"modelName,omitempty"`
	// datasetName is the name of the dataset
	DatasetName *string `json:"datasetName,omitempty"`
	// authSecret is the name of the secret containing the "HF_TOKEN" token
	// +kubebuilder:validation:MinLength=1
	AuthSecret string `json:"authSecret"`
	// modelPuller is the containerized huggingface-cli image to pull the data
	// +kubebuilder:validation:MinLength=1
	ModelPuller string `json:"modelPuller"`
	// pullSecret is the name of the image pull secret for the modelPuller image
	// +kubebuilder:validation:MinLength=1
	PullSecret string `json:"pullSecret"`
}

type NemoDataStoreSource struct {
	// Endpoint is the HuggingFace endpoint from NeMo DataStore
	// +kubebuilder:validation:Pattern=`^https?://.*/v1/hf/?$`
	Endpoint string `json:"endpoint"`
	// Namespace is the namespace within NeMo DataStore
	// +kubebuilder:default="default"
	Namespace        string `json:"namespace"`
	DSHFCommonFields `json:",inline"`
}

type HuggingFaceHubSource struct {
	// Endpoint is the HuggingFace endpoint
	// +kubebuilder:validation:Pattern=`^https?://.*$`
	Endpoint string `json:"endpoint"`
	// Namespace is the namespace within the HuggingFace Hub
	// +kubebuilder:validation:MinLength=1
	Namespace        string `json:"namespace"`
	DSHFCommonFields `json:",inline"`
}

// NGCSource references a model stored on NVIDIA NGC.
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

// ModelSpec is the spec required to cache selected models.
type ModelSpec struct {
	// Profiles are the specific model profiles to cache. When these are provided, rest of the model parameters for profile selection are ignored
	Profiles []string `json:"profiles,omitempty"`
	// Precision is the precision for model quantization
	Precision string `json:"precision,omitempty"`
	// Engine is the backend engine (tensorrt_llm, vllm)
	Engine string `json:"engine,omitempty"`
	// TensorParallelism is the minimum GPUs required for the model computations
	TensorParallelism string `json:"tensorParallelism,omitempty"`
	// QoSProfile is the supported QoS profile types for the models (throughput, latency)
	QoSProfile string `json:"qosProfile,omitempty"`
	// GPU is the spec for matching GPUs for caching optimized models
	GPUs []GPUSpec `json:"gpus,omitempty"`
	// Lora indicates a finetuned model with LoRa adapters
	Lora *bool `json:"lora,omitempty"`
	// Buildable indicates generic model profiles that can be optimized with an NVIDIA engine for any GPUs
	Buildable *bool `json:"buildable,omitempty"`
}

// GPUSpec is the spec required to cache models for selected gpu type.
type GPUSpec struct {
	// Product is the GPU product string (h100, a100, l40s)
	Product string `json:"product,omitempty"`
	// IDs are the device-ids for a specific GPU SKU
	IDs []string `json:"ids,omitempty"`
}

// NIMCacheStorage defines the attributes of various storage targets used to store the model.
type NIMCacheStorage struct {
	// PersistentVolumeClaim is the pvc volume used for caching NIM
	PVC PersistentVolumeClaim `json:"pvc,omitempty"`
	// HostPath is the host path volume for caching NIM
	HostPath *string `json:"hostPath,omitempty"`
}

// NIMCacheStatus defines the observed state of NIMCache.
type NIMCacheStatus struct {
	State      string             `json:"state,omitempty"`
	PVC        string             `json:"pvc,omitempty"`
	Profiles   []NIMProfile       `json:"profiles,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// NIMProfile defines the profiles that were cached.
type NIMProfile struct {
	Name    string            `json:"name,omitempty"`
	Model   string            `json:"model,omitempty"`
	Release string            `json:"release,omitempty"`
	Config  map[string]string `json:"config,omitempty"`
}

// Resources defines the minimum resources required for caching NIM.
type Resources struct {
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
	// NimCacheConditionReconcileFailed indicated that error occurred while reconciling NIMCache object.
	NimCacheConditionReconcileFailed = "NIM_CACHE_RECONCILE_FAILED"

	// NimCacheStatusNotReady indicates that cache is not ready.
	NimCacheStatusNotReady = "NotReady"
	// NimCacheStatusPVCCreated indicates that the pvc is created for caching.
	NimCacheStatusPVCCreated = "PVC-Created"
	// NimCacheStatusStarted indicates that caching process is started.
	NimCacheStatusStarted = "Started"
	// NimCacheStatusReady indicates that cache is ready.
	NimCacheStatusReady = "Ready"
	// NimCacheStatusInProgress indicates that caching is in progress.
	NimCacheStatusInProgress = "InProgress"
	// NimCacheStatusPending indicates that caching is not yet started.
	NimCacheStatusPending = "Pending"
	// NimCacheStatusFailed indicates that caching is failed.
	NimCacheStatusFailed = "Failed"
)

// EnvFromSecrets return the list of secrets that should be mounted as env vars.
func (s *NIMSource) EnvFromSecrets() []corev1.EnvFromSource {
	if s.NGC != nil && s.NGC.AuthSecret != "" { // nolint:gocritic
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
	} else if s.HF != nil && s.HF.AuthSecret != "" {
		return []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: s.HF.AuthSecret,
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
// +kubebuilder:printcolumn:name="Age",type="date",format="date-time",JSONPath=".metadata.creationTimestamp",priority=0

// NIMCache is the Schema for the nimcaches API.
type NIMCache struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NIMCacheSpec   `json:"spec,omitempty"`
	Status NIMCacheStatus `json:"status,omitempty"`
}

// GetPVCName returns the name to be used for the PVC based on the custom spec
// Prefers pvc.Name if explicitly set by the user in the NIMCache instance.
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

// GetTolerations returns tolerations configured for the NIMCache Job.
func (n *NIMCache) GetTolerations() []corev1.Toleration {
	return n.Spec.Tolerations
}

// GetNodeSelectors returns nodeselectors configured for the NIMCache Job.
func (n *NIMCache) GetNodeSelectors() map[string]string {
	if n.Spec.NodeSelector == nil {
		return map[string]string{"feature.node.kubernetes.io/pci-10de.present": "true"}
	}
	return n.Spec.NodeSelector
}

// GetRuntimeClassName return the runtime class name for the NIMCache Job.
func (n *NIMCache) GetRuntimeClassName() *string {
	if n.Spec.RuntimeClassName == "" {
		return nil
	}
	return &n.Spec.RuntimeClassName
}

// GetProxySpec returns the proxy spec for the NIMService deployment.
func (n *NIMCache) GetProxySpec() *ProxySpec {
	return n.Spec.Proxy
}

func (n *NIMCache) GetEnvWithProxy() []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  "NIM_SDK_USE_NATIVE_TLS",
			Value: "1",
		},
		{
			Name:  "HTTPS_PROXY",
			Value: n.Spec.Proxy.HttpsProxy,
		},
		{
			Name:  "HTTP_PROXY",
			Value: n.Spec.Proxy.HttpProxy,
		},
		{
			Name:  "NO_PROXY",
			Value: n.Spec.Proxy.NoProxy,
		},
		{
			Name:  "https_proxy",
			Value: n.Spec.Proxy.HttpsProxy,
		},
		{
			Name:  "http_proxy",
			Value: n.Spec.Proxy.HttpProxy,
		},
		{
			Name:  "no_proxy",
			Value: n.Spec.Proxy.NoProxy,
		},
	}
	return envVars
}

func (n *NIMCache) GetInitContainers() []corev1.Container {
	if n.Spec.Proxy != nil {
		initContainerList := []corev1.Container{
			{
				Name:            "update-ca-certificates",
				Command:         k8sutil.GetUpdateCaCertInitContainerCommand(),
				SecurityContext: k8sutil.GetUpdateCaCertInitContainerSecurityContext(),
				VolumeMounts:    k8sutil.GetUpdateCaCertInitContainerVolumeMounts(),
			},
		}
		if n.Spec.Source.NGC != nil { // nolint:gocritic
			initContainerList[0].Image = n.Spec.Source.NGC.ModelPuller
		} else if n.Spec.Source.DataStore != nil {
			initContainerList[0].Image = n.Spec.Source.DataStore.ModelPuller
		} else if n.Spec.Source.HF != nil {
			initContainerList[0].Image = n.Spec.Source.HF.ModelPuller
		}
		return initContainerList
	}
	return []corev1.Container{}
}

func (d *DSHFCommonFields) GetModelName() *string {
	return d.ModelName
}

func (d *DSHFCommonFields) GetDatasetName() *string {
	return d.DatasetName
}

func (d *DSHFCommonFields) GetAuthSecret() string {
	return d.AuthSecret
}

func (d *DSHFCommonFields) GetModelPuller() string {
	return d.ModelPuller
}

func (d *DSHFCommonFields) GetPullSecret() string {
	return d.PullSecret
}

func (d *HuggingFaceHubSource) GetEndpoint() string {
	return d.Endpoint
}

func (d *HuggingFaceHubSource) GetNamespace() string {
	return d.Namespace
}

func (d *NemoDataStoreSource) GetEndpoint() string {
	return d.Endpoint
}

func (d *NemoDataStoreSource) GetNamespace() string {
	return d.Namespace
}

// +kubebuilder:object:root=true

// NIMCacheList contains a list of NIMCache.
type NIMCacheList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NIMCache `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NIMCache{}, &NIMCacheList{})
}
