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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"maps"
	"os"

	kserveconstants "github.com/kserve/kserve/pkg/constants"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
	rendertypes "github.com/NVIDIA/k8s-nim-operator/internal/render/types"
	utils "github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	// NIMServiceConditionReady indicates that the NIM deployment is ready.
	NIMServiceConditionReady = "NIM_SERVICE_READY"
	// NIMServiceConditionFailed indicates that the NIM deployment has failed.
	NIMServiceConditionFailed = "NIM_SERVICE_FAILED"

	// NIMServiceStatusPending indicates that NIM deployment is in pending state.
	NIMServiceStatusPending = "Pending"
	// NIMServiceStatusNotReady indicates that NIM deployment is not ready.
	NIMServiceStatusNotReady = "NotReady"
	// NIMServiceStatusReady indicates that NIM deployment is ready.
	NIMServiceStatusReady = "Ready"
	// NIMServiceStatusFailed indicates that NIM deployment has failed.
	NIMServiceStatusFailed = "Failed"

	DefaultMPIStartScript = `#!/bin/bash
NIM_JSONL_LOGGING="${NIM_JSONL_LOGGING:-0}"
if [ "${NIM_JSONL_LOGGING}" -eq "1" ] && /opt/nim/llm/.venv/bin/python3 -c "import nim_llm_sdk.logging.pack_all_logs_into_json" 2> /dev/null; then
  /opt/nim/llm/.venv/bin/python3 -m nim_llm_sdk.entrypoints.openai.api_server |& /opt/nim/llm/.venv/bin/python3 -m nim_llm_sdk.logging.pack_all_logs_into_json
else
  /opt/nim/llm/.venv/bin/python3 -m nim_llm_sdk.entrypoints.openai.api_server
fi`

	DefaultMPITimeout = 300
)

type NIMBackendType string

const (
	NIMBackendTypeLWS NIMBackendType = "lws"
)

// NIMServiceSpec defines the desired state of NIMService.
// +kubebuilder:validation:XValidation:rule="!(has(self.multiNode) && has(self.scale) && has(self.scale.enabled) && self.scale.enabled)", message="autoScaling must be nil or disabled when multiNode is set"
type NIMServiceSpec struct {
	Image   Image           `json:"image"`
	Command []string        `json:"command,omitempty"`
	Args    []string        `json:"args,omitempty"`
	Env     []corev1.EnvVar `json:"env,omitempty"`
	// The name of an existing pull secret containing the NGC_API_KEY
	AuthSecret string `json:"authSecret"`
	// Storage is the target storage for caching NIM model if NIMCache is not provided
	Storage      NIMServiceStorage   `json:"storage,omitempty"`
	Labels       map[string]string   `json:"labels,omitempty"`
	Annotations  map[string]string   `json:"annotations,omitempty"`
	NodeSelector map[string]string   `json:"nodeSelector,omitempty"`
	Tolerations  []corev1.Toleration `json:"tolerations,omitempty"`
	PodAffinity  *corev1.PodAffinity `json:"podAffinity,omitempty"`
	// Resources is the resource requirements for the NIMService deployment or leader worker set.
	//
	// Note: Only traditional resources like cpu/memory and custom device plugin resources are supported here.
	// Any DRA claim references are ignored. Use DRAResources instead for those.
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// DRAResources is the list of DRA resource claims to be used for the NIMService deployment or leader worker set.
	DRAResources   []DRAResource `json:"draResources,omitempty"`
	Expose         Expose        `json:"expose,omitempty"`
	LivenessProbe  Probe         `json:"livenessProbe,omitempty"`
	ReadinessProbe Probe         `json:"readinessProbe,omitempty"`
	StartupProbe   Probe         `json:"startupProbe,omitempty"`
	Scale          Autoscaling   `json:"scale,omitempty"`
	SchedulerName  string        `json:"schedulerName,omitempty"`
	Metrics        Metrics       `json:"metrics,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default:=1
	Replicas         int                        `json:"replicas,omitempty"`
	UserID           *int64                     `json:"userID,omitempty"`
	GroupID          *int64                     `json:"groupID,omitempty"`
	RuntimeClassName string                     `json:"runtimeClassName,omitempty"`
	Proxy            *ProxySpec                 `json:"proxy,omitempty"`
	MultiNode        *NimServiceMultiNodeConfig `json:"multiNode,omitempty"`
}

// NimServiceMultiNodeConfig defines the configuration for multi-node NIMService.
type NimServiceMultiNodeConfig struct {
	// +kubebuilder:validation:Enum=lws
	// +kubebuilder:default:=lws
	// BackendType specifies the backend type for the multi-node NIMService. Currently only LWS is supported.
	BackendType NIMBackendType `json:"backendType,omitempty"`

	// +kubebuilder:default:=1
	// Size specifies the number of pods to create for the multi-node NIMService.
	// +kubebuilder:validation:Minimum=1
	Size int `json:"size,omitempty"`

	// +kubebuilder:default:=1
	// GPUSPerPod specifies the number of GPUs for each instance. In most cases, this should match `resources.limits.nvidia.com/gpu`.
	GPUSPerPod int `json:"gpusPerPod,omitempty"`

	// MPI config for NIMService using LeaderWorkerSet
	MPI *MultiNodeMPIConfig `json:"mpi,omitempty"`
}

type MultiNodeMPIConfig struct {
	// +kubebuilder:default:=300
	// MPIStartTimeout specifies the timeout in seconds for starting the cluster.
	MPIStartTimeout int `json:"mpiStartTimeout"`
}

// NIMCacheVolSpec defines the spec to use NIMCache volume.
type NIMCacheVolSpec struct {
	Name    string `json:"name,omitempty"`
	Profile string `json:"profile,omitempty"`
}

// NIMServiceStatus defines the observed state of NIMService.
type NIMServiceStatus struct {
	Conditions        []metav1.Condition `json:"conditions,omitempty"`
	AvailableReplicas int32              `json:"availableReplicas,omitempty"`
	State             string             `json:"state,omitempty"`
	Model             *ModelStatus       `json:"model,omitempty"`
	// DRAResourceStatuses is the status of the DRA resources.
	// +listType=map
	// +listMapKey=name
	DRAResourceStatuses []DRAResourceStatus `json:"draResourceStatuses,omitempty"`
}

// ModelStatus defines the configuration of the NIMService model.
type ModelStatus struct {
	Name             string `json:"name"`
	ClusterEndpoint  string `json:"clusterEndpoint"`
	ExternalEndpoint string `json:"externalEndpoint"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.state`,priority=0
// +kubebuilder:printcolumn:name="Age",type="date",format="date-time",JSONPath=".metadata.creationTimestamp",priority=0

// NIMService is the Schema for the nimservices API.
type NIMService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NIMServiceSpec   `json:"spec,omitempty"`
	Status NIMServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NIMServiceList contains a list of NIMService.
type NIMServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NIMService `json:"items"`
}

// NIMServiceStorage defines the attributes of various storage targets used to store the model.
type NIMServiceStorage struct {
	NIMCache NIMCacheVolSpec `json:"nimCache,omitempty"`
	// SharedMemorySizeLimit sets the max size of the shared memory volume (emptyDir) used by NIMs for fast model runtime I/O.
	SharedMemorySizeLimit *resource.Quantity `json:"sharedMemorySizeLimit,omitempty"`
	// PersistentVolumeClaim is the pvc volume used for caching NIM
	PVC PersistentVolumeClaim `json:"pvc,omitempty"`
	// HostPath is the host path volume for caching NIM
	HostPath *string `json:"hostPath,omitempty"`
	// ReadOnly mode indicates if the volume should be mounted as read-only
	ReadOnly *bool `json:"readOnly,omitempty"`
}

// GetLWSName returns the name to be used for the LeaderWorkerSet based on the custom spec.
func (n *NIMService) GetLWSName() string {
	return fmt.Sprintf("%s-lws", n.GetName())
}

// GetPVCName returns the name to be used for the PVC based on the custom spec
// Prefers pvc.Name if explicitly set by the user in the NIMService instance.
func (n *NIMService) GetPVCName(pvc PersistentVolumeClaim) string {
	pvcName := fmt.Sprintf("%s-pvc", n.GetName())
	if pvc.Name != "" {
		pvcName = pvc.Name
	}
	return pvcName
}

// GetStandardSelectorLabels returns the standard selector labels for the NIMService deployment.
func (n *NIMService) GetStandardSelectorLabels() map[string]string {
	appName := n.GetName()
	if n.Spec.MultiNode != nil {
		appName = n.GetLWSName()
	}
	return map[string]string{
		"app": appName,
	}
}

// GetStandardLabels returns the standard set of labels for NIMService resources.
func (n *NIMService) GetStandardLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":             n.Name,
		"app.kubernetes.io/instance":         n.Name,
		"app.kubernetes.io/operator-version": os.Getenv("OPERATOR_VERSION"),
		"app.kubernetes.io/part-of":          "nim-service",
		"app.kubernetes.io/managed-by":       "k8s-nim-operator",
	}
}

// GetStandardEnv returns the standard set of env variables for the NIMService container.
func (n *NIMService) GetStandardEnv() []corev1.EnvVar {
	// add standard env required for NIM service
	envVars := []corev1.EnvVar{
		{
			Name:  "NIM_CACHE_PATH",
			Value: utils.DefaultModelStorePath,
		},
		{
			Name: "NGC_API_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: n.Spec.AuthSecret,
					},
					Key: "NGC_API_KEY",
				},
			},
		},
		{
			Name:  "OUTLINES_CACHE_DIR",
			Value: "/tmp/outlines",
		},
		{
			Name:  "NIM_SERVER_PORT",
			Value: fmt.Sprintf("%d", *n.Spec.Expose.Service.Port),
		},
		{
			Name:  "NIM_HTTP_API_PORT",
			Value: fmt.Sprintf("%d", *n.Spec.Expose.Service.Port),
		},
		{
			Name:  "NIM_JSONL_LOGGING",
			Value: "1",
		},
		{
			Name:  "NIM_LOG_LEVEL",
			Value: "INFO",
		},
	}
	if n.Spec.Expose.Service.GRPCPort != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "NIM_GRPC_API_PORT",
			Value: fmt.Sprintf("%d", *n.Spec.Expose.Service.GRPCPort),
		}, corev1.EnvVar{
			Name:  "NIM_TRITON_GRPC_PORT",
			Value: fmt.Sprintf("%d", *n.Spec.Expose.Service.GRPCPort),
		})
	}
	if n.Spec.Expose.Service.MetricsPort != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "NIM_TRITON_METRICS_PORT",
			Value: fmt.Sprintf("%d", *n.Spec.Expose.Service.MetricsPort),
		})
	}

	return envVars
}

func (n *NIMService) GetLWSLeaderEnv() []corev1.EnvVar {
	env := n.GetEnv()

	mpiTimeout := DefaultMPITimeout
	if n.Spec.MultiNode.MPI != nil && n.Spec.MultiNode.MPI.MPIStartTimeout != 0 {
		mpiTimeout = n.Spec.MultiNode.MPI.MPIStartTimeout
	}

	env = append(env,
		corev1.EnvVar{
			Name:  "NIM_LEADER_ROLE",
			Value: "1",
		},
		corev1.EnvVar{
			Name:  "NIM_MPI_ALLOW_RUN_AS_ROOT",
			Value: "0",
		},
		corev1.EnvVar{
			Name:  "OMPI_MCA_orte_keep_fqdn_hostnames",
			Value: "true",
		},
		corev1.EnvVar{
			Name:  "OMPI_MCA_plm_rsh_args",
			Value: "-o ConnectionAttempts=20",
		},
		corev1.EnvVar{
			Name:  "NIM_NUM_COMPUTE_NODES",
			Value: fmt.Sprintf("%d", n.Spec.MultiNode.Size),
		},
		corev1.EnvVar{
			Name:  "GPUS_PER_NODE",
			Value: fmt.Sprintf("%d", n.Spec.MultiNode.GPUSPerPod),
		},
		corev1.EnvVar{
			Name:  "CLUSTER_START_TIMEOUT",
			Value: fmt.Sprintf("%d", mpiTimeout),
		},
		corev1.EnvVar{
			Name: "CLUSTER_SIZE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.annotations['leaderworkerset.sigs.k8s.io/size']",
				},
			},
		},
		corev1.EnvVar{
			Name: "GROUP_INDEX",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.labels['leaderworkerset.sigs.k8s.io/group-index']",
				},
			},
		},
	)
	return env
}

func (n *NIMService) GetLWSWorkerEnv() []corev1.EnvVar {
	env := n.GetEnv()
	env = append(env,
		corev1.EnvVar{
			Name:  "NIM_LEADER_ROLE",
			Value: "0",
		},
		corev1.EnvVar{
			Name:  "NIM_MPI_ALLOW_RUN_AS_ROOT",
			Value: "0",
		},
		corev1.EnvVar{
			Name:  "NIM_NUM_COMPUTE_NODES",
			Value: fmt.Sprintf("%d", n.Spec.MultiNode.Size),
		},
		corev1.EnvVar{
			Name: "LEADER_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.annotations['leaderworkerset.sigs.k8s.io/leader-name']",
				},
			},
		},
		corev1.EnvVar{
			Name: "NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
		corev1.EnvVar{
			Name: "LWS_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.labels['leaderworkerset.sigs.k8s.io/name']",
				},
			},
		},
	)
	return env
}

// GetProxySpec returns the proxy spec for the NIMService deployment.
func (n *NIMService) GetProxyEnv() []corev1.EnvVar {

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

// GetStandardAnnotations returns default annotations to apply to the NIMService instance.
func (n *NIMService) GetStandardAnnotations() map[string]string {
	standardAnnotations := map[string]string{
		"openshift.io/required-scc":             "nonroot",
		utils.NvidiaAnnotationParentSpecHashKey: utils.DeepHashObject(n.Spec),
	}
	if n.GetProxySpec() != nil {
		standardAnnotations["openshift.io/required-scc"] = "anyuid"
	}
	return standardAnnotations
}

// GetNIMServiceAnnotations returns annotations to apply to the NIMService instance.
func (n *NIMService) GetNIMServiceAnnotations() map[string]string {
	standardAnnotations := n.GetStandardAnnotations()

	if n.Spec.Annotations != nil {
		return utils.MergeMaps(standardAnnotations, n.Spec.Annotations)
	}

	return standardAnnotations
}

// GetServiceLabels returns merged labels to apply to the NIMService instance.
func (n *NIMService) GetServiceLabels() map[string]string {
	standardLabels := n.GetStandardLabels()

	if n.Spec.Labels != nil {
		return utils.MergeMaps(standardLabels, n.Spec.Labels)
	}
	return standardLabels
}

// GetSelectorLabels returns standard selector labels to apply to the NIMService instance.
func (n *NIMService) GetSelectorLabels() map[string]string {
	// TODO: add custom ones
	selector := n.GetStandardSelectorLabels()
	if n.Spec.MultiNode != nil {
		selector["nim-llm-role"] = "leader"
	}
	return selector
}

// GetNodeSelector returns node selector labels for the NIMService instance.
func (n *NIMService) GetNodeSelector() map[string]string {
	return n.Spec.NodeSelector
}

// GetTolerations returns tolerations for the NIMService instance.
func (n *NIMService) GetTolerations() []corev1.Toleration {
	return n.Spec.Tolerations
}

// GetPodAffinity returns pod affinity for the NIMService instance.
func (n *NIMService) GetPodAffinity() *corev1.PodAffinity {
	return n.Spec.PodAffinity
}

// GetContainerName returns name of the container for NIMService deployment.
func (n *NIMService) GetContainerName() string {
	return fmt.Sprintf("%s-ctr", n.Name)
}

// GetCommand return command to override for the NIMService container.
func (n *NIMService) GetCommand() []string {
	return n.Spec.Command
}

// GetArgs return arguments for the NIMService container.
func (n *NIMService) GetArgs() []string {
	return n.Spec.Args
}

// GetEnv returns merged slice of standard and user specified env variables.
func (n *NIMService) GetEnv() []corev1.EnvVar {
	envVarList := utils.MergeEnvVars(n.GetStandardEnv(), n.Spec.Env)
	if n.GetProxySpec() != nil {
		envVarList = utils.MergeEnvVars(envVarList, n.GetProxyEnv())
	}
	return envVarList
}

// GetImage returns container image for the NIMService.
func (n *NIMService) GetImage() string {
	return fmt.Sprintf("%s:%s", n.Spec.Image.Repository, n.Spec.Image.Tag)
}

// GetImagePullSecrets returns the image pull secrets for the NIM container.
func (n *NIMService) GetImagePullSecrets() []string {
	return n.Spec.Image.PullSecrets
}

// GetImagePullPolicy returns the image pull policy for the NIM container.
func (n *NIMService) GetImagePullPolicy() string {
	return n.Spec.Image.PullPolicy
}

// GetResources returns resources to allocate to the NIMService container.
func (n *NIMService) GetResources() *corev1.ResourceRequirements {
	if n.Spec.Resources == nil {
		return nil
	}

	return &corev1.ResourceRequirements{
		Requests: n.Spec.Resources.Requests,
		Limits:   n.Spec.Resources.Limits,
	}
}

// IsProbeEnabled returns true if a given liveness/readiness/startup probe is enabled.
func IsProbeEnabled(probe Probe) bool {
	if probe.Enabled == nil {
		return true
	}
	return *probe.Enabled
}

// GetLivenessProbe returns liveness probe for the NIMService container.
func (n *NIMService) GetLivenessProbe() *corev1.Probe {
	if n.Spec.LivenessProbe.Probe == nil {
		return n.GetDefaultLivenessProbe()
	}
	return n.Spec.LivenessProbe.Probe
}

// GetDefaultLivenessProbe returns the default liveness probe for the NIMService container.
func (n *NIMService) GetDefaultLivenessProbe() *corev1.Probe {
	probe := corev1.Probe{
		InitialDelaySeconds: 15,
		TimeoutSeconds:      1,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		FailureThreshold:    3,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/v1/health/live",
				Port: intstr.FromString(DefaultNamedPortAPI),
			},
		},
	}

	return &probe
}

// GetReadinessProbe returns readiness probe for the NIMService container.
func (n *NIMService) GetReadinessProbe() *corev1.Probe {
	if n.Spec.ReadinessProbe.Probe == nil {
		return n.GetDefaultReadinessProbe()
	}
	return n.Spec.ReadinessProbe.Probe
}

// GetDefaultReadinessProbe returns the default readiness probe for the NIMService container.
func (n *NIMService) GetDefaultReadinessProbe() *corev1.Probe {
	probe := corev1.Probe{
		InitialDelaySeconds: 15,
		TimeoutSeconds:      1,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		FailureThreshold:    3,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/v1/health/ready",
				Port: intstr.FromString(DefaultNamedPortAPI),
			},
		},
	}

	return &probe
}

// GetStartupProbe returns startup probe for the NIMService container.
func (n *NIMService) GetStartupProbe() *corev1.Probe {
	if n.Spec.StartupProbe.Probe == nil {
		return n.GetDefaultStartupProbe()
	}
	return n.Spec.StartupProbe.Probe
}

// GetDefaultStartupProbe returns the default startup probe for the NIMService container.
func (n *NIMService) GetDefaultStartupProbe() *corev1.Probe {
	probe := corev1.Probe{
		InitialDelaySeconds: 30,
		TimeoutSeconds:      1,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		FailureThreshold:    30,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/v1/health/ready",
				Port: intstr.FromString(DefaultNamedPortAPI),
			},
		},
	}

	return &probe
}

// GetVolumesMounts returns volume mounts for the NIMService container.
func (n *NIMService) GetVolumesMounts() []corev1.Volume {
	// TODO: setup volume mounts required for NIM
	return nil
}

// GetVolumes returns volumes for the NIMService container.
func (n *NIMService) GetVolumes(modelPVC PersistentVolumeClaim) []corev1.Volume {
	// TODO: Fetch actual PVC name from associated NIMCache obj
	volumes := []corev1.Volume{
		{
			Name: "dshm",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium:    corev1.StorageMediumMemory,
					SizeLimit: n.Spec.Storage.SharedMemorySizeLimit,
				},
			},
		},
		{
			Name: "model-store",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: modelPVC.Name,
					ReadOnly:  n.GetStorageReadOnly(),
				},
			},
		},
	}

	if n.GetProxySpec() != nil {
		volumes = append(volumes, k8sutil.GetVolumesForUpdatingCaCert(n.Spec.Proxy.CertConfigMap)...)
	}
	return volumes
}

func (n *NIMService) GetLeaderVolumes(modelPVC PersistentVolumeClaim) []corev1.Volume {
	volumes := n.GetVolumes(modelPVC)

	volumes = append(volumes,
		corev1.Volume{
			Name: "mpi-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: ptr.To[int32](365),
					LocalObjectReference: corev1.LocalObjectReference{
						Name: fmt.Sprintf("%s-mpi-config", n.GetName()),
					},
				},
			},
		},
		corev1.Volume{
			Name: "ssh-dotfiles",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		corev1.Volume{
			Name: "ssh-pk",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  fmt.Sprintf("%s-ssh-pk", n.GetName()),
					DefaultMode: ptr.To[int32](256),
				},
			},
		},
		corev1.Volume{
			Name: "start-mpi-script",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: ptr.To[int32](365),
					LocalObjectReference: corev1.LocalObjectReference{
						Name: fmt.Sprintf("%s-mpi-start-script", n.GetName()),
					},
				},
			},
		},
	)
	return volumes
}

func (n *NIMService) GetWorkerVolumes(modelPVC PersistentVolumeClaim) []corev1.Volume {
	volumes := n.GetVolumes(modelPVC)
	volumes = append(volumes,
		corev1.Volume{
			Name: "ssh-confs",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		corev1.Volume{
			Name: "ssh-dotfiles",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		corev1.Volume{
			Name: "ssh-pk",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  fmt.Sprintf("%s-ssh-pk", n.GetName()),
					DefaultMode: ptr.To[int32](256),
				},
			},
		},
		corev1.Volume{
			Name: "start-mpi-script",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: ptr.To[int32](365),
					LocalObjectReference: corev1.LocalObjectReference{
						Name: fmt.Sprintf("%s-mpi-start-script", n.GetName()),
					},
				},
			},
		},
	)
	return volumes
}

// GetVolumeMounts returns volumes for the NIMService container.
func (n *NIMService) GetVolumeMounts(modelPVC PersistentVolumeClaim) []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "model-store",
			MountPath: "/model-store",
			SubPath:   modelPVC.SubPath,
		},
		{
			Name:      "dshm",
			MountPath: "/dev/shm",
		},
	}

	if n.GetProxySpec() != nil {
		volumeMounts = append(volumeMounts, k8sutil.GetVolumesMountsForUpdatingCaCert()...)
	}
	return volumeMounts
}

func (n *NIMService) GetWorkerVolumeMounts(modelPVC PersistentVolumeClaim) []corev1.VolumeMount {
	volumeMounts := n.GetVolumeMounts(modelPVC)

	volumeMounts = append(volumeMounts,
		corev1.VolumeMount{
			Name:      "ssh-confs",
			MountPath: "/ssh-confs",
		},
		corev1.VolumeMount{
			Name:      "ssh-dotfiles",
			MountPath: "/opt/nim/llm/.ssh",
		},
		corev1.VolumeMount{
			Name:      "ssh-pk",
			MountPath: "/opt/nim/llm/pk",
		},
		corev1.VolumeMount{
			Name:      "start-mpi-script",
			MountPath: "/opt/nim/start-mpi-cluster.sh",
			SubPath:   "start-mpi-cluster.sh",
		},
	)
	return volumeMounts
}

func (n *NIMService) GetLeaderVolumeMounts(modelPVC PersistentVolumeClaim) []corev1.VolumeMount {
	volumeMounts := n.GetVolumeMounts(modelPVC)
	volumeMounts = append(volumeMounts,
		corev1.VolumeMount{
			Name:      "mpi-config",
			MountPath: "/etc/mpi",
		},
		corev1.VolumeMount{
			Name:      "ssh-dotfiles",
			MountPath: "/opt/nim/llm/.ssh",
		},
		corev1.VolumeMount{
			Name:      "ssh-pk",
			MountPath: "/opt/nim/llm/pk",
		},
		corev1.VolumeMount{
			Name:      "start-mpi-script",
			MountPath: "/opt/nim/start-mpi-cluster.sh",
			SubPath:   "start-mpi-cluster.sh",
		},
	)
	return volumeMounts
}

// GetServiceAccountName returns service account name for the NIMService deployment.
func (n *NIMService) GetServiceAccountName() string {
	return n.Name
}

// GetRuntimeClassName return the runtime class name for the NIMService deployment.
func (n *NIMService) GetRuntimeClassName() string {
	return n.Spec.RuntimeClassName
}

// GetNIMCacheName returns the NIMCache name to use for the NIMService deployment.
func (n *NIMService) GetNIMCacheName() string {
	return n.Spec.Storage.NIMCache.Name
}

// GetNIMCacheProfile returns the explicit profile to use for the NIMService deployment.
func (n *NIMService) GetNIMCacheProfile() string {
	return n.Spec.Storage.NIMCache.Profile
}

// GetHPA returns the HPA spec for the NIMService deployment.
func (n *NIMService) GetHPA() HorizontalPodAutoscalerSpec {
	return n.Spec.Scale.HPA
}

// GetServiceMonitor returns the Service Monitor details for the NIMService deployment.
func (n *NIMService) GetServiceMonitor() ServiceMonitor {
	return n.Spec.Metrics.ServiceMonitor
}

// GetReplicas returns replicas for the NIMService deployment.
func (n *NIMService) GetReplicas() int {
	if n.IsAutoScalingEnabled() {
		return 0
	}
	return n.Spec.Replicas
}

func (n *NIMService) GetLWSSize() int {
	if n.Spec.MultiNode == nil {
		return 0
	}
	return n.Spec.MultiNode.Size
}

// GetDeploymentKind returns the kind of deployment for NIMService.
func (n *NIMService) GetDeploymentKind() string {
	return "Deployment"
}

// GetInitContainers returns the init containers for the NIMService deployment.
func (n *NIMService) GetInitContainers() []corev1.Container {
	if n.Spec.Proxy != nil {
		return []corev1.Container{
			{
				Name:            "update-ca-certificates",
				Image:           n.GetImage(),
				ImagePullPolicy: corev1.PullPolicy(n.GetImagePullPolicy()),
				Command:         k8sutil.GetUpdateCaCertInitContainerCommand(),
				SecurityContext: k8sutil.GetUpdateCaCertInitContainerSecurityContext(),
				VolumeMounts:    k8sutil.GetUpdateCaCertInitContainerVolumeMounts(),
			},
		}
	}
	return []corev1.Container{}
}

// IsAutoScalingEnabled returns true if autoscaling is enabled for NIMService deployment.
func (n *NIMService) IsAutoScalingEnabled() bool {
	return n.Spec.Scale.Enabled != nil && *n.Spec.Scale.Enabled
}

// IsIngressEnabled returns true if ingress is enabled for NIMService deployment.
func (n *NIMService) IsIngressEnabled() bool {
	return n.Spec.Expose.Ingress.Enabled != nil && *n.Spec.Expose.Ingress.Enabled
}

// GetIngressSpec returns the Ingress spec NIMService deployment.
func (n *NIMService) GetIngressSpec() networkingv1.IngressSpec {
	return n.Spec.Expose.Ingress.Spec
}

// IsServiceMonitorEnabled returns true if servicemonitor is enabled for NIMService deployment.
func (n *NIMService) IsServiceMonitorEnabled() bool {
	return n.Spec.Metrics.Enabled != nil && *n.Spec.Metrics.Enabled
}

// GetServicePort returns the service port for the NIMService deployment or default port.
func (n *NIMService) GetServicePort() int32 {
	if n.Spec.Expose.Service.Port == nil {
		return DefaultAPIPort
	}
	return *n.Spec.Expose.Service.Port
}

// GetServiceType returns the service type for the NIMService deployment.
func (n *NIMService) GetServiceType() string {
	return string(n.Spec.Expose.Service.Type)
}

// GetUserID returns the user ID for the NIMService deployment.
func (n *NIMService) GetUserID() *int64 {
	if n.Spec.UserID != nil {
		return n.Spec.UserID
	}
	return ptr.To[int64](1000)
}

// GetGroupID returns the group ID for the NIMService deployment.
func (n *NIMService) GetGroupID() *int64 {
	if n.Spec.GroupID != nil {
		return n.Spec.GroupID
	}
	return ptr.To[int64](2000)
}

// GetStorageReadOnly returns true if the volume have to be mounted as read-only for the NIMService deployment.
func (n *NIMService) GetStorageReadOnly() bool {
	if n.Spec.Storage.ReadOnly == nil {
		return false
	}
	return *n.Spec.Storage.ReadOnly
}

// GetServiceAccountParams return params to render ServiceAccount from templates.
func (n *NIMService) GetServiceAccountParams() *rendertypes.ServiceAccountParams {
	params := &rendertypes.ServiceAccountParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNIMServiceAnnotations()
	return params
}

// GetDeploymentParams returns params to render Deployment from templates.
func (n *NIMService) GetDeploymentParams() *rendertypes.DeploymentParams {
	params := &rendertypes.DeploymentParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNIMServiceAnnotations()
	params.PodAnnotations = n.GetNIMServiceAnnotations()
	delete(params.PodAnnotations, utils.NvidiaAnnotationParentSpecHashKey)

	// Set template spec
	if !n.IsAutoScalingEnabled() {
		params.Replicas = n.GetReplicas()
	}
	params.NodeSelector = n.GetNodeSelector()
	params.Tolerations = n.GetTolerations()
	params.Affinity = n.GetPodAffinity()
	params.ImagePullSecrets = n.GetImagePullSecrets()
	params.ImagePullPolicy = n.GetImagePullPolicy()

	// Set labels and selectors
	params.SelectorLabels = n.GetSelectorLabels()

	// Set container spec
	params.ContainerName = n.GetContainerName()
	params.Env = n.GetEnv()
	params.Args = n.GetArgs()
	params.Command = n.GetCommand()
	params.Resources = n.GetResources()
	params.Image = n.GetImage()

	// Set container probes
	if IsProbeEnabled(n.Spec.LivenessProbe) {
		params.LivenessProbe = n.GetLivenessProbe()
	}
	if IsProbeEnabled(n.Spec.ReadinessProbe) {
		params.ReadinessProbe = n.GetReadinessProbe()
	}
	if IsProbeEnabled(n.Spec.StartupProbe) {
		params.StartupProbe = n.GetStartupProbe()
	}
	params.UserID = n.GetUserID()
	params.GroupID = n.GetGroupID()

	// Set service account
	params.ServiceAccountName = n.GetServiceAccountName()

	// Set runtime class
	params.RuntimeClassName = n.GetRuntimeClassName()

	// Set scheduler
	params.SchedulerName = n.GetSchedulerName()

	// Setup container ports for nimservice
	params.Ports = []corev1.ContainerPort{
		{
			Name:          DefaultNamedPortAPI,
			Protocol:      corev1.ProtocolTCP,
			ContainerPort: *n.Spec.Expose.Service.Port,
		},
	}
	if n.Spec.Expose.Service.GRPCPort != nil {
		params.Ports = append(params.Ports, corev1.ContainerPort{
			Name:          DefaultNamedPortGRPC,
			Protocol:      corev1.ProtocolTCP,
			ContainerPort: *n.Spec.Expose.Service.GRPCPort,
		})
	}
	if n.Spec.Expose.Service.MetricsPort != nil {
		params.Ports = append(params.Ports, corev1.ContainerPort{
			Name:          DefaultNamedPortMetrics,
			Protocol:      corev1.ProtocolTCP,
			ContainerPort: *n.Spec.Expose.Service.MetricsPort,
		})
	}
	return params
}

func (n *NIMService) GetLWSParams() *rendertypes.LeaderWorkerSetParams {
	params := &rendertypes.LeaderWorkerSetParams{}

	// Set metadata
	params.Name = n.GetLWSName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetLabels()
	params.Annotations = n.GetNIMServiceAnnotations()
	params.PodAnnotations = n.GetNIMServiceAnnotations()
	delete(params.PodAnnotations, utils.NvidiaAnnotationParentSpecHashKey)

	// Set template spec
	params.Replicas = n.GetReplicas()
	params.Size = n.GetLWSSize()
	params.ContainerName = n.GetContainerName()
	params.Args = n.GetArgs()
	params.Command = n.GetCommand()
	params.LeaderEnvs = n.GetLWSLeaderEnv()
	params.WorkerEnvs = n.GetLWSWorkerEnv()
	params.UserID = n.GetUserID()
	params.GroupID = n.GetGroupID()
	params.Image = n.GetImage()
	params.ImagePullSecrets = n.GetImagePullSecrets()
	params.ImagePullPolicy = n.GetImagePullPolicy()
	params.Resources = n.GetResources()
	params.NodeSelector = n.GetNodeSelector()
	params.Tolerations = n.GetTolerations()
	params.Affinity = n.GetPodAffinity()
	params.Ports = []corev1.ContainerPort{
		{
			Name:          DefaultNamedPortAPI,
			ContainerPort: n.GetServicePort(),
			Protocol:      corev1.ProtocolTCP,
		},
	}

	// Set container probes
	if IsProbeEnabled(n.Spec.LivenessProbe) {
		params.LivenessProbe = n.GetLivenessProbe()
	}

	if IsProbeEnabled(n.Spec.ReadinessProbe) {
		params.ReadinessProbe = n.GetReadinessProbe()
	}

	if IsProbeEnabled(n.Spec.StartupProbe) {
		params.StartupProbe = n.GetStartupProbe()
	}

	params.ServiceAccountName = n.GetServiceAccountName()
	params.SchedulerName = n.GetSchedulerName()
	params.RuntimeClassName = n.GetRuntimeClassName()
	params.InitContainers = n.GetInitContainers()
	return params
}

// GetSchedulerName returns the scheduler name for the NIMService deployment.
func (n *NIMService) GetSchedulerName() string {
	return n.Spec.SchedulerName
}

// GetStatefulSetParams returns params to render StatefulSet from templates.
func (n *NIMService) GetStatefulSetParams() *rendertypes.StatefulSetParams {

	params := &rendertypes.StatefulSetParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNIMServiceAnnotations()

	// Set template spec
	if !n.IsAutoScalingEnabled() {
		params.Replicas = n.GetReplicas()
	}
	params.ServiceName = n.GetName()
	params.NodeSelector = n.GetNodeSelector()
	params.Tolerations = n.GetTolerations()
	params.Affinity = n.GetPodAffinity()
	params.ImagePullSecrets = n.GetImagePullSecrets()
	params.ImagePullPolicy = n.GetImagePullPolicy()

	// Set labels and selectors
	params.SelectorLabels = n.GetSelectorLabels()

	// Set container spec
	params.ContainerName = n.GetContainerName()
	params.Env = n.GetEnv()
	params.Args = n.GetArgs()
	params.Command = n.GetCommand()
	params.Resources = n.GetResources()
	params.Image = n.GetImage()

	// Set container probes
	params.LivenessProbe = n.GetLivenessProbe()
	params.ReadinessProbe = n.GetReadinessProbe()
	params.StartupProbe = n.GetStartupProbe()

	// Set service account
	params.ServiceAccountName = n.GetServiceAccountName()

	// Set runtime class
	params.RuntimeClassName = n.GetRuntimeClassName()
	return params
}

func (n *NIMService) generateMPIConfigData() map[string]string {
	// Construct ConfigMap data
	data := make(map[string]string)
	for i := 0; i < n.Spec.Replicas; i++ {
		hostfile := fmt.Sprintf("localhost slots=%d\n", n.Spec.MultiNode.GPUSPerPod)
		for j := 1; j < n.Spec.MultiNode.Size; j++ {
			workerHostname := fmt.Sprintf("%s-%d-%d.%s.%s.svc slots=%d",
				n.GetLWSName(), i, j, n.GetLWSName(), n.GetNamespace(), n.Spec.MultiNode.GPUSPerPod)
			hostfile += workerHostname + "\n"
		}
		dataKey := fmt.Sprintf("hostfile-%d", i)
		data[dataKey] = hostfile
	}
	return data
}

func (n *NIMService) GetMPIConfigParams() *rendertypes.ConfigMapParams {
	if n.Spec.MultiNode == nil {
		return nil
	}
	return &rendertypes.ConfigMapParams{
		Name:          fmt.Sprintf("%s-mpi-config", n.GetName()),
		Namespace:     n.GetNamespace(),
		ConfigMapData: n.generateMPIConfigData(),
		Labels:        n.GetLabels(),
		Annotations:   n.GetAnnotations(),
	}
}

func (n *NIMService) GetDefaultMPIScriptConfigParams() *rendertypes.ConfigMapParams {
	if n.Spec.MultiNode == nil {
		return nil
	}

	return &rendertypes.ConfigMapParams{
		Name:      fmt.Sprintf("%s-mpi-start-script", n.GetName()),
		Namespace: n.GetNamespace(),
		ConfigMapData: map[string]string{
			"start-mpi-cluster.sh": DefaultMPIStartScript,
		},
		Labels:      n.GetLabels(),
		Annotations: n.GetAnnotations(),
	}
}

func (n *NIMService) GetMPISSHSecretParams() (*rendertypes.SecretParams, error) {
	if n.Spec.MultiNode == nil {
		return nil, nil
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate MPI SSH private key: %v", err)
	}

	privateKeyPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
		},
	)

	return &rendertypes.SecretParams{
		Name:        fmt.Sprintf("%s-ssh-pk", n.GetName()),
		Namespace:   n.GetNamespace(),
		Labels:      n.GetLabels(),
		Annotations: n.GetAnnotations(),
		SecretMapData: map[string]string{
			"private.key": base64.StdEncoding.EncodeToString(privateKeyPEM),
		},
	}, nil
}

// GetServiceParams returns params to render Service from templates.
func (n *NIMService) GetServiceParams() *rendertypes.ServiceParams {
	params := &rendertypes.ServiceParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetServiceAnnotations()

	// Set service selector labels
	params.SelectorLabels = n.GetSelectorLabels()

	// Set service type
	params.Type = n.GetServiceType()

	// Set service ports
	params.Ports = []corev1.ServicePort{
		{
			Name:       DefaultNamedPortAPI,
			Port:       n.GetServicePort(),
			TargetPort: intstr.FromString((DefaultNamedPortAPI)),
			Protocol:   corev1.ProtocolTCP,
		},
	}
	if n.Spec.Expose.Service.GRPCPort != nil {
		params.Ports = append(params.Ports, corev1.ServicePort{
			Name:       DefaultNamedPortGRPC,
			Port:       *n.Spec.Expose.Service.GRPCPort,
			TargetPort: intstr.FromString(DefaultNamedPortGRPC),
			Protocol:   corev1.ProtocolTCP,
		})
	}
	if n.Spec.Expose.Service.MetricsPort != nil {
		params.Ports = append(params.Ports, corev1.ServicePort{
			Name:       DefaultNamedPortMetrics,
			Port:       *n.Spec.Expose.Service.MetricsPort,
			TargetPort: intstr.FromString(DefaultNamedPortMetrics),
			Protocol:   corev1.ProtocolTCP,
		})
	}

	return params
}

// GetIngressParams returns params to render Ingress from templates.
func (n *NIMService) GetIngressParams() *rendertypes.IngressParams {
	params := &rendertypes.IngressParams{}

	params.Enabled = n.IsIngressEnabled()
	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetIngressAnnotations()
	params.Spec = n.GetIngressSpec()
	return params
}

// GetRoleParams returns params to render Role from templates.
func (n *NIMService) GetRoleParams() *rendertypes.RoleParams {
	params := &rendertypes.RoleParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()

	// Set rules to use SCC
	if n.GetProxySpec() != nil {
		params.Rules = []rbacv1.PolicyRule{
			{
				APIGroups:     []string{"security.openshift.io"},
				Resources:     []string{"securitycontextconstraints"},
				ResourceNames: []string{"anyuid"},
				Verbs:         []string{"use"},
			},
		}
	} else {
		params.Rules = []rbacv1.PolicyRule{
			{
				APIGroups:     []string{"security.openshift.io"},
				Resources:     []string{"securitycontextconstraints"},
				ResourceNames: []string{"nonroot"},
				Verbs:         []string{"use"},
			},
		}
	}

	return params
}

// GetRoleBindingParams returns params to render RoleBinding from templates.
func (n *NIMService) GetRoleBindingParams() *rendertypes.RoleBindingParams {
	params := &rendertypes.RoleBindingParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()

	params.ServiceAccountName = n.GetServiceAccountName()
	params.RoleName = n.GetName()
	return params
}

// GetHPAParams returns params to render HPA from templates.
func (n *NIMService) GetHPAParams() *rendertypes.HPAParams {
	params := &rendertypes.HPAParams{}

	params.Enabled = n.IsAutoScalingEnabled()

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetHPAAnnotations()

	// Set HPA spec
	hpa := n.GetHPA()
	hpaSpec := autoscalingv2.HorizontalPodAutoscalerSpec{
		ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
			Kind:       n.GetDeploymentKind(),
			Name:       n.GetName(),
			APIVersion: "apps/v1",
		},
		MinReplicas: hpa.MinReplicas,
		MaxReplicas: hpa.MaxReplicas,
		Metrics:     hpa.Metrics,
		Behavior:    hpa.Behavior,
	}
	params.HPASpec = hpaSpec
	return params
}

// GetSCCParams return params to render SCC from templates.
func (n *NIMService) GetSCCParams() *rendertypes.SCCParams {
	params := &rendertypes.SCCParams{}
	// Set metadata
	params.Name = "nim-service-scc"

	params.ServiceAccountName = n.GetServiceAccountName()
	return params
}

// GetServiceMonitorParams return params to render Service Monitor from templates.
func (n *NIMService) GetServiceMonitorParams() *rendertypes.ServiceMonitorParams {
	params := &rendertypes.ServiceMonitorParams{}
	serviceMonitor := n.GetServiceMonitor()
	params.Enabled = n.IsServiceMonitorEnabled()
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	svcLabels := n.GetServiceLabels()
	maps.Copy(svcLabels, serviceMonitor.AdditionalLabels)
	params.Labels = svcLabels
	params.Annotations = n.GetServiceMonitorAnnotations()

	// Set Service Monitor spec
	smSpec := monitoringv1.ServiceMonitorSpec{
		NamespaceSelector: monitoringv1.NamespaceSelector{MatchNames: []string{n.Namespace}},
		Selector:          metav1.LabelSelector{MatchLabels: n.GetServiceLabels()},
		Endpoints: []monitoringv1.Endpoint{
			{
				Path:          "/v1/metrics",
				Port:          DefaultNamedPortAPI,
				ScrapeTimeout: serviceMonitor.ScrapeTimeout,
				Interval:      serviceMonitor.Interval,
			},
		},
	}
	if n.Spec.Expose.Service.MetricsPort != nil {
		smSpec.Endpoints = append(smSpec.Endpoints, monitoringv1.Endpoint{
			Path:          "/metrics",
			Port:          DefaultNamedPortMetrics,
			ScrapeTimeout: serviceMonitor.ScrapeTimeout,
			Interval:      serviceMonitor.Interval,
		})
	}
	params.SMSpec = smSpec
	return params
}

func (n *NIMService) GetIngressAnnotations() map[string]string {
	nimServiceAnnotations := n.GetNIMServiceAnnotations()

	if n.Spec.Expose.Ingress.Annotations != nil {
		return utils.MergeMaps(nimServiceAnnotations, n.Spec.Expose.Ingress.Annotations)
	}
	return nimServiceAnnotations
}

func (n *NIMService) GetServiceAnnotations() map[string]string {
	nimServiceAnnotations := n.GetNIMServiceAnnotations()

	if n.Spec.Expose.Service.Annotations != nil {
		return utils.MergeMaps(nimServiceAnnotations, n.Spec.Expose.Service.Annotations)
	}
	return nimServiceAnnotations
}

func (n *NIMService) GetHPAAnnotations() map[string]string {
	nimServiceAnnotations := n.GetNIMServiceAnnotations()

	if n.Spec.Scale.Annotations != nil {
		return utils.MergeMaps(nimServiceAnnotations, n.Spec.Scale.Annotations)
	}
	return nimServiceAnnotations
}

func (n *NIMService) GetServiceMonitorAnnotations() map[string]string {
	nimServiceAnnotations := n.GetNIMServiceAnnotations()

	if n.Spec.Metrics.ServiceMonitor.Annotations != nil {
		return utils.MergeMaps(nimServiceAnnotations, n.Spec.Metrics.ServiceMonitor.Annotations)
	}
	return nimServiceAnnotations
}

// GetProxySpec returns the proxy spec for the NIMService deployment.
func (n *NIMService) GetProxySpec() *ProxySpec {
	return n.Spec.Proxy
}

// GetInferenceServiceParams returns params to render InferenceService from templates.
func (n *NIMService) GetInferenceServiceParams(
	deploymentMode kserveconstants.DeploymentModeType) *rendertypes.InferenceServiceParams {

	params := &rendertypes.InferenceServiceParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNIMServiceAnnotations()
	params.PodAnnotations = n.GetNIMServiceAnnotations()
	delete(params.PodAnnotations, utils.NvidiaAnnotationParentSpecHashKey)

	// Set template spec
	if !n.IsAutoScalingEnabled() || deploymentMode != kserveconstants.RawDeployment {
		params.MinReplicas = ptr.To[int32](int32(n.GetReplicas()))
	} else {
		params.Annotations[kserveconstants.AutoscalerClass] = string(kserveconstants.AutoscalerClassHPA)

		minReplicas, maxReplicas, metric, metricType, target := n.GetInferenceServiceHPAParams()
		if minReplicas != nil {
			params.MinReplicas = minReplicas
		}
		if maxReplicas > 0 {
			params.MaxReplicas = ptr.To[int32](maxReplicas)
		}
		if metric != "" {
			params.ScaleMetric = metric
		}
		if metricType != "" {
			params.ScaleMetricType = metricType
		}
		if target > 0 {
			params.ScaleTarget = ptr.To(target)
		}
	}

	params.NodeSelector = n.GetNodeSelector()
	params.Tolerations = n.GetTolerations()
	params.Affinity = n.GetPodAffinity()
	params.ImagePullSecrets = n.GetImagePullSecrets()
	params.ImagePullPolicy = n.GetImagePullPolicy()

	// Set labels and selectors
	params.SelectorLabels = n.GetSelectorLabels()

	// Set container spec
	params.ContainerName = n.GetContainerName()
	params.Env = n.GetEnv()
	params.Args = n.GetArgs()
	params.Command = n.GetCommand()
	params.Resources = n.GetResources()
	params.Image = n.GetImage()

	// Set container probes
	if IsProbeEnabled(n.Spec.LivenessProbe) {
		params.LivenessProbe = n.GetInferenceServiceLivenessProbe(deploymentMode)
	}
	if IsProbeEnabled(n.Spec.ReadinessProbe) {
		params.ReadinessProbe = n.GetInferenceServiceReadinessProbe(deploymentMode)
	}
	if IsProbeEnabled(n.Spec.StartupProbe) {
		params.StartupProbe = n.GetInferenceServiceStartupProbe(deploymentMode)
	}

	params.UserID = n.GetUserID()
	params.GroupID = n.GetGroupID()

	// Set service account
	params.ServiceAccountName = n.GetServiceAccountName()

	// Set runtime class
	params.RuntimeClassName = n.GetRuntimeClassName()

	// Set scheduler
	params.SchedulerName = n.GetSchedulerName()

	params.Ports = n.GetInferenceServicePorts(deploymentMode)

	return params
}

// GetInferenceServiceLivenessProbe returns liveness probe for the NIMService container.
func (n *NIMService) GetInferenceServiceLivenessProbe(modeType kserveconstants.DeploymentModeType) *corev1.Probe {
	if modeType == kserveconstants.RawDeployment {
		if n.Spec.LivenessProbe.Probe == nil {
			return n.GetDefaultLivenessProbe()
		}
	} else {
		if n.Spec.LivenessProbe.Probe == nil {
			probe := &corev1.Probe{
				InitialDelaySeconds: 15,
				TimeoutSeconds:      1,
				PeriodSeconds:       10,
				SuccessThreshold:    1,
				FailureThreshold:    3,
			}
			if n.Spec.Expose.Service.GRPCPort != nil {
				probe.ProbeHandler = corev1.ProbeHandler{
					GRPC: &corev1.GRPCAction{
						Port: *n.Spec.Expose.Service.GRPCPort,
					},
				}
			} else {
				probe.ProbeHandler = corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/v1/health/live",
						Port: intstr.FromInt32(*n.Spec.Expose.Service.Port),
					},
				}
			}
			return probe
		}
	}

	return n.Spec.LivenessProbe.Probe
}

// GetInferenceServiceReadinessProbe returns readiness probe for the NIMService container.
func (n *NIMService) GetInferenceServiceReadinessProbe(modeType kserveconstants.DeploymentModeType) *corev1.Probe {
	if modeType == kserveconstants.RawDeployment {
		if n.Spec.ReadinessProbe.Probe == nil {
			return n.GetDefaultReadinessProbe()
		}
	} else {
		if n.Spec.ReadinessProbe.Probe == nil {
			probe := &corev1.Probe{
				InitialDelaySeconds: 15,
				TimeoutSeconds:      1,
				PeriodSeconds:       10,
				SuccessThreshold:    1,
				FailureThreshold:    3,
			}
			if n.Spec.Expose.Service.GRPCPort != nil {
				probe.ProbeHandler = corev1.ProbeHandler{
					GRPC: &corev1.GRPCAction{
						Port: *n.Spec.Expose.Service.GRPCPort,
					},
				}
			} else {
				probe.ProbeHandler = corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/v1/health/ready",
						Port: intstr.FromInt32(*n.Spec.Expose.Service.Port),
					},
				}
			}
			return probe
		}
	}

	return n.Spec.ReadinessProbe.Probe
}

// GetInferenceServiceStartupProbe returns startup probe for the NIMService container.
func (n *NIMService) GetInferenceServiceStartupProbe(modeType kserveconstants.DeploymentModeType) *corev1.Probe {
	if modeType == kserveconstants.RawDeployment {
		if n.Spec.StartupProbe.Probe == nil {
			return n.GetDefaultStartupProbe()
		}
	} else {
		if n.Spec.StartupProbe.Probe == nil {
			probe := &corev1.Probe{
				InitialDelaySeconds: 30,
				TimeoutSeconds:      1,
				PeriodSeconds:       10,
				SuccessThreshold:    1,
				FailureThreshold:    30,
			}
			if n.Spec.Expose.Service.GRPCPort != nil {
				probe.ProbeHandler = corev1.ProbeHandler{
					GRPC: &corev1.GRPCAction{
						Port: *n.Spec.Expose.Service.GRPCPort,
					},
				}
			} else {
				probe.ProbeHandler = corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/v1/health/ready",
						Port: intstr.FromInt32(*n.Spec.Expose.Service.Port),
					},
				}
			}
			return probe
		}
	}

	return n.Spec.StartupProbe.Probe
}

// GetInferenceServicePorts returns ports for the NIMService container.
func (n *NIMService) GetInferenceServicePorts(modeType kserveconstants.DeploymentModeType) []corev1.ContainerPort {
	ports := []corev1.ContainerPort{}

	// Setup container ports for nimservice
	if modeType == kserveconstants.RawDeployment {
		ports = append(ports, corev1.ContainerPort{
			Name:          DefaultNamedPortAPI,
			Protocol:      corev1.ProtocolTCP,
			ContainerPort: *n.Spec.Expose.Service.Port,
		})
		if n.Spec.Expose.Service.GRPCPort != nil {
			ports = append(ports, corev1.ContainerPort{
				Name:          DefaultNamedPortGRPC,
				Protocol:      corev1.ProtocolTCP,
				ContainerPort: *n.Spec.Expose.Service.GRPCPort,
			})
		}
		if n.Spec.Expose.Service.MetricsPort != nil {
			ports = append(ports, corev1.ContainerPort{
				Name:          DefaultNamedPortMetrics,
				Protocol:      corev1.ProtocolTCP,
				ContainerPort: *n.Spec.Expose.Service.MetricsPort,
			})
		}
	} else {
		ports = append(ports, corev1.ContainerPort{
			Protocol:      corev1.ProtocolTCP,
			ContainerPort: *n.Spec.Expose.Service.Port,
		})
		if n.Spec.Expose.Service.GRPCPort != nil {
			ports = append(ports, corev1.ContainerPort{
				Protocol:      corev1.ProtocolTCP,
				ContainerPort: *n.Spec.Expose.Service.GRPCPort,
			})
		}
	}

	return ports
}

// GetInferenceServiceHPAParams returns the HPA spec for the NIMService deployment.
func (n *NIMService) GetInferenceServiceHPAParams() (*int32, int32, string, string, int32) {
	hpa := n.GetHPA()

	var minReplicas *int32
	var maxReplicas int32
	var metric string
	var metricType string
	var target int32

	if hpa.MinReplicas != nil {
		minReplicas = hpa.MinReplicas
	}
	maxReplicas = hpa.MaxReplicas

	for _, m := range hpa.Metrics {
		if m.Type == autoscalingv2.ResourceMetricSourceType && m.Resource != nil {
			if m.Resource.Name == corev1.ResourceCPU || m.Resource.Name == corev1.ResourceMemory {
				metric = string(m.Resource.Name)
				metricType = string(m.Resource.Target.Type)

				switch m.Resource.Target.Type {
				case autoscalingv2.UtilizationMetricType:
					if m.Resource.Target.AverageUtilization != nil {
						target = *m.Resource.Target.AverageUtilization
					}
				case autoscalingv2.ValueMetricType:
					if m.Resource.Target.Value != nil {
						target = int32((*m.Resource.Target.Value).Value())
					}
				case autoscalingv2.AverageValueMetricType:
					if m.Resource.Target.AverageValue != nil {
						target = int32((*m.Resource.Target.AverageValue).Value())
					}
				}
				break
			}
		}
	}

	return minReplicas, maxReplicas, metric, metricType, target
}

func init() {
	SchemeBuilder.Register(&NIMService{}, &NIMServiceList{})
}
