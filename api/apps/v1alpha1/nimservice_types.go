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
	"os"

	rendertypes "github.com/NVIDIA/k8s-nim-operator/internal/render/types"
	utils "github.com/NVIDIA/k8s-nim-operator/internal/utils"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	// NIMServiceConditionReady indicates that the NIM deployment is ready.
	NIMServiceConditionReady = "NIM_SERVICE_READY"
	// NIMServiceConditionFailed indicates that the NIM deployment has failed.
	NIMServiceConditionFailed = "NIM_SERVICE_FAILED"

	// NIMServiceStatusPending indicates that NIM deployment is in pending state
	NIMServiceStatusPending = "Pending"
	// NIMServiceStatusNotReady indicates that NIM deployment is not ready
	NIMServiceStatusNotReady = "NotReady"
	// NIMServiceStatusReady indicates that NIM deployment is ready
	NIMServiceStatusReady = "Ready"
	// NIMServiceStatusFailed indicates that NIM deployment has failed
	NIMServiceStatusFailed = "Failed"
)

// NIMServiceSpec defines the desired state of NIMService
type NIMServiceSpec struct {
	// +kubebuilder:validation:Enum=llm;embedding;reranking
	// +kubebuilder:default=llm
	Type    string          `json:"type,omitempty"`
	Image   Image           `json:"image,omitempty"`
	Command []string        `json:"command,omitempty"`
	Args    []string        `json:"args,omitempty"`
	Env     []corev1.EnvVar `json:"env,omitempty"`
	// The name of an existing pull secret containing the NGC_API_KEY
	AuthSecret string          `json:"authSecret"`
	NIMCache   NIMCacheVolSpec `json:"nimCache,omitempty"`
	// Storage is the target storage for caching NIM model if NIMCache is not provided
	Storage        Storage                      `json:"storage,omitempty"`
	Labels         map[string]string            `json:"labels,omitempty"`
	Annotations    map[string]string            `json:"annotations,omitempty"`
	NodeSelector   map[string]string            `json:"nodeSelector,omitempty"`
	Tolerations    []corev1.Toleration          `json:"tolerations,omitempty"`
	PodAffinity    *corev1.PodAffinity          `json:"podAffinity,omitempty"`
	Resources      *corev1.ResourceRequirements `json:"resources,omitempty"`
	Expose         Expose                       `json:"expose,omitempty"`
	LivenessProbe  Probe                        `json:"livenessProbe,omitempty"`
	ReadinessProbe Probe                        `json:"readinessProbe,omitempty"`
	StartupProbe   Probe                        `json:"startupProbe,omitempty"`
	Scale          Autoscaling                  `json:"scale,omitempty"`
	Metrics        Metrics                      `json:"metrics,omitempty"`
	Replicas       int                          `json:"replicas,omitempty"`
	UserID         *int64                       `json:"userID,omitempty"`
	GroupID        *int64                       `json:"groupID,omitempty"`
}

// NIMCacheVolSpec defines the spec to use NIMCache volume
type NIMCacheVolSpec struct {
	Name    string `json:"name,omitempty"`
	Profile string `json:"profile,omitempty"`
}

// NIMServiceStatus defines the observed state of NIMService
type NIMServiceStatus struct {
	Conditions        []metav1.Condition `json:"conditions,omitempty"`
	AvailableReplicas int32              `json:"availableReplicas,omitempty"`
	State             string             `json:"state,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.state`,priority=0
// +kubebuilder:printcolumn:name="Age",type=string,JSONPath=`.metadata.creationTimestamp`,priority=0

// NIMService is the Schema for the nimservices API
type NIMService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NIMServiceSpec   `json:"spec,omitempty"`
	Status NIMServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NIMServiceList contains a list of NIMService
type NIMServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NIMService `json:"items"`
}

// GetPVCName returns the name to be used for the PVC based on the custom spec
// Prefers pvc.Name if explicitly set by the user in the NIMService instance
func (n *NIMService) GetPVCName(pvc PersistentVolumeClaim) string {
	pvcName := fmt.Sprintf("%s-pvc", n.GetName())
	if pvc.Name != "" {
		pvcName = pvc.Name
	}
	return pvcName
}

// GetStandardSelectorLabels returns the standard selector labels for the NIMService deployment
func (n *NIMService) GetStandardSelectorLabels() map[string]string {
	return map[string]string{
		"app": n.Name,
	}
}

// GetStandardLabels returns the standard set of labels for NIMService resources
func (n *NIMService) GetStandardLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":             n.Name,
		"app.kubernetes.io/instance":         n.Name,
		"app.kubernetes.io/operator-version": os.Getenv("OPERATOR_VERSION"),
		"app.kubernetes.io/part-of":          "nim-service",
		"app.kubernetes.io/managed-by":       "k8s-nim-operator",
	}
}

// GetStandardEnv returns the standard set of env variables for the NIMService container
func (n *NIMService) GetStandardEnv() []corev1.EnvVar {
	// add standard env required for NIM service
	envVars := []corev1.EnvVar{
		{
			Name:  "NIM_CACHE_PATH",
			Value: "/model-store",
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
			Value: fmt.Sprint(n.Spec.Expose.Service.OpenAIPort),
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

	return envVars
}

// GetStandardAnnotations returns default annotations to apply to the NIMService instance
func (n *NIMService) GetStandardAnnotations() map[string]string {
	standardAnnotations := map[string]string{
		"openshift.io/scc": "anyuid",
	}
	return standardAnnotations
}

// GetServiceAnnotations returns annotations to apply to the NIMService instance
func (n *NIMService) GetServiceAnnotations() map[string]string {
	standardAnnotations := n.GetStandardAnnotations()

	if n.Spec.Annotations != nil {
		return utils.MergeMaps(standardAnnotations, n.Spec.Annotations)
	}

	return standardAnnotations
}

// GetServiceLabels returns merged labels to apply to the NIMService instance
func (n *NIMService) GetServiceLabels() map[string]string {
	standardLabels := n.GetStandardLabels()

	if n.Spec.Labels != nil {
		return utils.MergeMaps(standardLabels, n.Spec.Labels)
	}
	return standardLabels
}

// GetSelectorLabels returns standard selector labels to apply to the NIMService instance
func (n *NIMService) GetSelectorLabels() map[string]string {
	// TODO: add custom ones
	return n.GetStandardSelectorLabels()
}

// GetNodeSelector returns node selector labels for the NIMService instance
func (n *NIMService) GetNodeSelector() map[string]string {
	return n.Spec.NodeSelector
}

// GetTolerations returns tolerations for the NIMService instance
func (n *NIMService) GetTolerations() []corev1.Toleration {
	return n.Spec.Tolerations
}

// GetPodAffinity returns pod affinity for the NIMService instance
func (n *NIMService) GetPodAffinity() *corev1.PodAffinity {
	return n.Spec.PodAffinity
}

// GetContainerName returns name of the container for NIMService deployment
func (n *NIMService) GetContainerName() string {
	return fmt.Sprintf("%s-ctr", n.Name)
}

// GetCommand return command to override for the NIMService container
func (n *NIMService) GetCommand() []string {
	return n.Spec.Command
}

// GetArgs return arguments for the NIMService container
func (n *NIMService) GetArgs() []string {
	return n.Spec.Args
}

// GetEnv returns merged slice of standard and user specified env variables
func (n *NIMService) GetEnv() []corev1.EnvVar {
	return utils.MergeEnvVars(n.GetStandardEnv(), n.Spec.Env)
}

// GetImage returns container image for the NIMService
func (n *NIMService) GetImage() string {
	return fmt.Sprintf("%s:%s", n.Spec.Image.Repository, n.Spec.Image.Tag)
}

// GetImagePullSecrets returns the image pull secrets for the NIM container
func (n *NIMService) GetImagePullSecrets() []string {
	return n.Spec.Image.PullSecrets
}

// GetImagePullPolicy returns the image pull policy for the NIM container
func (n *NIMService) GetImagePullPolicy() string {
	return n.Spec.Image.PullPolicy
}

// GetResources returns resources to allocate to the NIMService container
func (n *NIMService) GetResources() *corev1.ResourceRequirements {
	return n.Spec.Resources
}

func IsProbeEnabled(probe Probe) bool {
	if probe.Enabled == nil {
		return true
	}
	return *probe.Enabled
}

// GetLivenessProbe returns liveness probe for the NIMService container
func (n *NIMService) GetLivenessProbe() *corev1.Probe {
	if n.Spec.LivenessProbe.Probe == nil {
		return GetDefaultLivenessProbe()
	}
	return n.Spec.LivenessProbe.Probe
}

// GetDefaultLivenessProbe returns the default liveness probe for the NIMService container
func GetDefaultLivenessProbe() *corev1.Probe {
	probe := corev1.Probe{
		InitialDelaySeconds: 15,
		TimeoutSeconds:      1,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		FailureThreshold:    3,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/v1/health/live",
				Port: intstr.IntOrString{
					Type:   intstr.Type(0),
					IntVal: 8000,
				},
			},
		},
	}

	return &probe
}

// GetReadinessProbe returns readiness probe for the NIMService container
func (n *NIMService) GetReadinessProbe() *corev1.Probe {
	if n.Spec.ReadinessProbe.Probe == nil {
		return GetDefaultReadinessProbe()
	}
	return n.Spec.ReadinessProbe.Probe
}

// GetDefaultReadinessProbe returns the default readiness probe for the NIMService container
func GetDefaultReadinessProbe() *corev1.Probe {
	probe := corev1.Probe{
		InitialDelaySeconds: 15,
		TimeoutSeconds:      1,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		FailureThreshold:    3,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/v1/health/ready",
				Port: intstr.IntOrString{
					Type:   intstr.Type(0),
					IntVal: 8000,
				},
			},
		},
	}

	return &probe
}

// GetStartupProbe returns startup probe for the NIMService container
func (n *NIMService) GetStartupProbe() *corev1.Probe {
	if n.Spec.StartupProbe.Probe == nil {
		return GetDefaultStartupProbe()
	}
	return n.Spec.StartupProbe.Probe
}

// GetDefaultStartupProbe returns the default startup probe for the NIMService container
func GetDefaultStartupProbe() *corev1.Probe {
	probe := corev1.Probe{
		InitialDelaySeconds: 40,
		TimeoutSeconds:      1,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		FailureThreshold:    180,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/v1/health/ready",
				Port: intstr.IntOrString{
					Type:   intstr.Type(0),
					IntVal: 8000,
				},
			},
		},
	}

	return &probe
}

// GetVolumesMounts returns volume mounts for the NIMService container
func (n *NIMService) GetVolumesMounts() []corev1.Volume {
	// TODO: setup volume mounts required for NIM
	return nil
}

// GetVolumes returns volumes for the NIMService container
func (n *NIMService) GetVolumes(modelPVC PersistentVolumeClaim) []corev1.Volume {
	// TODO: Fetch actual PVC name from associated NIMCache obj
	volumes := []corev1.Volume{
		{
			Name: "dshm",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium: corev1.StorageMediumMemory,
				},
			},
		},
		{
			Name: "model-store",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: modelPVC.Name,
				},
			},
		},
	}

	return volumes
}

// GetVolumeMounts returns volumes for the NIMService container
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

	return volumeMounts
}

// GetServiceAccountName returns service account name for the NIMService deployment
func (n *NIMService) GetServiceAccountName() string {
	return n.Name
}

// GetNIMCacheName returns the NIMCache name to use for the NIMService deployment
func (n *NIMService) GetNIMCacheName() string {
	return n.Spec.NIMCache.Name
}

// GetNIMCacheProfile returns the explicit profile to use for the NIMService deployment
func (n *NIMService) GetNIMCacheProfile() string {
	return n.Spec.NIMCache.Profile
}

// GetHPA returns the HPA spec for the NIMService deployment
func (n *NIMService) GetHPA() HorizontalPodAutoscalerSpec {
	return n.Spec.Scale.HPA
}

// GetReplicas returns replicas for the NIMService deployment
func (n *NIMService) GetReplicas() int {
	if n.IsAutoScalingEnabled() {
		return 0
	}
	return n.Spec.Replicas
}

// GetDeploymentKind returns the kind of deployment for NIMService
func (n *NIMService) GetDeploymentKind() string {
	return "Deployment"
}

// IsAutoScalingEnabled returns true if autoscaling is enabled for NIMService deployment
func (n *NIMService) IsAutoScalingEnabled() bool {
	return n.Spec.Scale.Enabled != nil && *n.Spec.Scale.Enabled
}

// IsIngressEnabled returns true if ingress is enabled for NIMService deployment
func (n *NIMService) IsIngressEnabled() bool {
	return n.Spec.Expose.Ingress.Enabled != nil && *n.Spec.Expose.Ingress.Enabled
}

// GetIngressSpec returns the Ingress spec NIMService deployment
func (n *NIMService) GetIngressSpec() networkingv1.IngressSpec {
	return n.Spec.Expose.Ingress.Spec
}

// IsServiceMonitorEnabled returns true if servicemonitor is enabled for NIMService deployment
func (n *NIMService) IsServiceMonitorEnabled() bool {
	return n.Spec.Metrics.Enabled != nil && *n.Spec.Metrics.Enabled
}

// GetServicePort returns the service port for the NIMService deployment
func (n *NIMService) GetServicePort() int32 {
	return n.Spec.Expose.Service.OpenAIPort
}

// GetUserID returns the user ID for the NIMService deployment
func (n *NIMService) GetUserID() *int64 {
	return n.Spec.UserID

}

// GetGroupID returns the group ID for the NIMService deployment
func (n *NIMService) GetGroupID() *int64 {
	return n.Spec.GroupID

}

// GetServiceAccountParams return params to render ServiceAccount from templates
func (n *NIMService) GetServiceAccountParams() *rendertypes.ServiceAccountParams {
	params := &rendertypes.ServiceAccountParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetServiceAnnotations()
	return params
}

// GetDeploymentParams returns params to render Deployment from templates
func (n *NIMService) GetDeploymentParams() *rendertypes.DeploymentParams {
	params := &rendertypes.DeploymentParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetServiceAnnotations()

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
	return params
}

// GetStatefulSetParams returns params to render StatefulSet from templates
func (n *NIMService) GetStatefulSetParams() *rendertypes.StatefulSetParams {

	params := &rendertypes.StatefulSetParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetServiceAnnotations()

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
	return params
}

// GetServiceParams returns params to render Service from templates
func (n *NIMService) GetServiceParams() *rendertypes.ServiceParams {
	params := &rendertypes.ServiceParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetServiceAnnotations()

	// Set service selector labels
	params.SelectorLabels = n.GetSelectorLabels()

	// Set service ports
	params.Port = n.GetServicePort()
	return params
}

// GetIngressParams returns params to render Ingress from templates
func (n *NIMService) GetIngressParams() *rendertypes.IngressParams {
	params := &rendertypes.IngressParams{}

	params.Enabled = n.IsIngressEnabled()
	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetServiceAnnotations()
	params.Spec = n.GetIngressSpec()
	return params
}

// GetRoleParams returns params to render Role from templates
func (n *NIMService) GetRoleParams() *rendertypes.RoleParams {
	params := &rendertypes.RoleParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()

	// TODO: set rules
	return params
}

// GetRoleBindingParams returns params to render RoleBinding from templates
func (n *NIMService) GetRoleBindingParams() *rendertypes.RoleBindingParams {
	params := &rendertypes.RoleBindingParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()

	params.ServiceAccountName = n.GetServiceAccountName()
	params.RoleName = n.GetName()
	return params
}

// GetHPAParams returns params to render HPA from templates
func (n *NIMService) GetHPAParams() *rendertypes.HPAParams {
	params := &rendertypes.HPAParams{}

	params.Enabled = n.IsAutoScalingEnabled()

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetServiceAnnotations()

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

// GetSCCParams return params to render SCC from templates
func (n *NIMService) GetSCCParams() *rendertypes.SCCParams {
	params := &rendertypes.SCCParams{}
	// Set metadata
	params.Name = "nim-service-scc"

	params.ServiceAccountName = n.GetServiceAccountName()
	return params
}

func init() {
	SchemeBuilder.Register(&NIMService{}, &NIMServiceList{})
}
