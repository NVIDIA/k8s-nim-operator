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
	"maps"
	"os"

	rendertypes "github.com/NVIDIA/k8s-nim-operator/internal/render/types"
	utils "github.com/NVIDIA/k8s-nim-operator/internal/utils"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	// NemoGuardrailConditionReady indicates that the NEMO GuardrailService is ready.
	NemoGuardrailConditionReady = "Ready"
	// NemoGuardrailConditionFailed indicates that the NEMO GuardrailService has failed.
	NemoGuardrailConditionFailed = "Failed"

	// NemoGuardrailStatusPending indicates that NEMO GuardrailService is in pending state
	NemoGuardrailStatusPending = "Pending"
	// NemoGuardrailStatusNotReady indicates that NEMO GuardrailService is not ready
	NemoGuardrailStatusNotReady = "NotReady"
	// NemoGuardrailStatusReady indicates that NEMO GuardrailService is ready
	NemoGuardrailStatusReady = "Ready"
	// NemoGuardrailStatusFailed indicates that NEMO GuardrailService has failed
	NemoGuardrailStatusFailed = "Failed"
)

// NemoGuardrailSpec defines the desired state of NemoGuardrail
type NemoGuardrailSpec struct {
	Image   Image           `json:"image,omitempty"`
	Command []string        `json:"command,omitempty"`
	Args    []string        `json:"args,omitempty"`
	Env     []corev1.EnvVar `json:"env,omitempty"`
	// The name of an secret that contains authn for the NGC NIM service API
	AuthSecret string `json:"authSecret"`
	// ConfigStore stores the config of the guardrail service
	ConfigStore    GuardrailConfig              `json:"configStore,omitempty"`
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
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default:=1
	Replicas int    `json:"replicas,omitempty"`
	UserID   *int64 `json:"userID,omitempty"`
	GroupID  *int64 `json:"groupID,omitempty"`
}

type GuardrailConfig struct {
	ConfigMap string                 `json:"configMap,omitempty"`
	PVC       *PersistentVolumeClaim `json:"pvc,omitempty"`
}

// NemoGuardrailStatus defines the observed state of NemoGuardrail
type NemoGuardrailStatus struct {
	Conditions        []metav1.Condition `json:"conditions,omitempty"`
	AvailableReplicas int32              `json:"availableReplicas,omitempty"`
	State             string             `json:"state,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.state`,priority=0
// +kubebuilder:printcolumn:name="Age",type="date",format="date-time",JSONPath=".metadata.creationTimestamp",priority=0

// NemoGuardrail is the Schema for the NemoGuardrail API
type NemoGuardrail struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NemoGuardrailSpec   `json:"spec,omitempty"`
	Status NemoGuardrailStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NemoGuardrailList contains a list of NemoGuardrail
type NemoGuardrailList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NemoGuardrail `json:"items"`
}

// GetPVCName returns the name to be used for the PVC based on the custom spec
// Prefers pvc.Name if explicitly set by the user in the NemoGuardrail instance
func (n *NemoGuardrail) GetPVCName(pvc PersistentVolumeClaim) string {
	pvcName := fmt.Sprintf("%s-pvc", n.GetName())
	if pvc.Name != "" {
		pvcName = pvc.Name
	}
	return pvcName
}

// GetStandardSelectorLabels returns the standard selector labels for the NemoGuardrail deployment
func (n *NemoGuardrail) GetStandardSelectorLabels() map[string]string {
	return map[string]string{
		"app": n.Name,
	}
}

// GetStandardLabels returns the standard set of labels for NemoGuardrail resources
func (n *NemoGuardrail) GetStandardLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":             n.Name,
		"app.kubernetes.io/instance":         n.Name,
		"app.kubernetes.io/operator-version": os.Getenv("OPERATOR_VERSION"),
		"app.kubernetes.io/part-of":          "nemo-guardrail-service",
		"app.kubernetes.io/managed-by":       "k8s-nim-operator",
	}
}

// GetStandardEnv returns the standard set of env variables for the NemoGuardrail container
func (n *NemoGuardrail) GetStandardEnv() []corev1.EnvVar {
	// add standard env required for NIM service
	envVars := []corev1.EnvVar{
		{
			Name:  "CONFIG_STORE_PATH",
			Value: "/config-store",
		},
		{
			Name:  "NIM_ENDPOINT_URL",
			Value: "https://integrate.api.nvidia.com/v1",
		},
		{
			Name:  "DEFAULT_CONFIG_ID",
			Value: "default",
		},
		{
			Name:  "DEFAULT_LLM_PROVIDER",
			Value: "nim",
		},
		{
			Name:  "NEMO_GUARDRAILS_SERVER_ENABLE_CORS",
			Value: "False",
		},
		{
			Name:  "NEMO_GUARDRAILS_SERVER_ALLOWED_ORIGINS",
			Value: "*",
		},
		{
			Name:  "GUARDRAILS_HOST",
			Value: "0.0.0.0",
		},
		{
			Name:  "GUARDRAILS_PORT",
			Value: "7331",
		},
		{
			Name:  "DEMO",
			Value: "False",
		},
	}

	return envVars
}

// GetStandardAnnotations returns default annotations to apply to the NemoGuardrail instance
func (n *NemoGuardrail) GetStandardAnnotations() map[string]string {
	standardAnnotations := map[string]string{
		"openshift.io/scc": "nonroot",
	}
	return standardAnnotations
}

// GetNemoGuardrailAnnotations returns annotations to apply to the NemoGuardrail instance
func (n *NemoGuardrail) GetNemoGuardrailAnnotations() map[string]string {
	standardAnnotations := n.GetStandardAnnotations()

	if n.Spec.Annotations != nil {
		return utils.MergeMaps(standardAnnotations, n.Spec.Annotations)
	}

	return standardAnnotations
}

// GetServiceLabels returns merged labels to apply to the NemoGuardrail instance
func (n *NemoGuardrail) GetServiceLabels() map[string]string {
	standardLabels := n.GetStandardLabels()

	if n.Spec.Labels != nil {
		return utils.MergeMaps(standardLabels, n.Spec.Labels)
	}
	return standardLabels
}

// GetSelectorLabels returns standard selector labels to apply to the NemoGuardrail instance
func (n *NemoGuardrail) GetSelectorLabels() map[string]string {
	// TODO: add custom ones
	return n.GetStandardSelectorLabels()
}

// GetNodeSelector returns node selector labels for the NemoGuardrail instance
func (n *NemoGuardrail) GetNodeSelector() map[string]string {
	return n.Spec.NodeSelector
}

// GetTolerations returns tolerations for the NemoGuardrail instance
func (n *NemoGuardrail) GetTolerations() []corev1.Toleration {
	return n.Spec.Tolerations
}

// GetPodAffinity returns pod affinity for the NemoGuardrail instance
func (n *NemoGuardrail) GetPodAffinity() *corev1.PodAffinity {
	return n.Spec.PodAffinity
}

// GetContainerName returns name of the container for NemoGuardrail deployment
func (n *NemoGuardrail) GetContainerName() string {
	return fmt.Sprintf("%s-ctr", n.Name)
}

// GetCommand return command to override for the NemoGuardrail container
func (n *NemoGuardrail) GetCommand() []string {
	return n.Spec.Command
}

// GetArgs return arguments for the NemoGuardrail container
func (n *NemoGuardrail) GetArgs() []string {
	return n.Spec.Args
}

// GetEnv returns merged slice of standard and user specified env variables
func (n *NemoGuardrail) GetEnv() []corev1.EnvVar {
	return utils.MergeEnvVars(n.GetStandardEnv(), n.Spec.Env)
}

// GetImage returns container image for the NemoGuardrail
func (n *NemoGuardrail) GetImage() string {
	return fmt.Sprintf("%s:%s", n.Spec.Image.Repository, n.Spec.Image.Tag)
}

// GetImagePullSecrets returns the image pull secrets for the NIM container
func (n *NemoGuardrail) GetImagePullSecrets() []string {
	return n.Spec.Image.PullSecrets
}

// GetImagePullPolicy returns the image pull policy for the NIM container
func (n *NemoGuardrail) GetImagePullPolicy() string {
	return n.Spec.Image.PullPolicy
}

// GetResources returns resources to allocate to the NemoGuardrail container
func (n *NemoGuardrail) GetResources() *corev1.ResourceRequirements {
	return n.Spec.Resources
}

// GetLivenessProbe returns liveness probe for the NemoGuardrail container
func (n *NemoGuardrail) GetLivenessProbe() *corev1.Probe {
	if n.Spec.LivenessProbe.Probe == nil {
		return n.GetDefaultLivenessProbe()
	}
	return n.Spec.LivenessProbe.Probe
}

// GetDefaultLivenessProbe returns the default liveness probe for the NemoGuardrail container
func (n *NemoGuardrail) GetDefaultLivenessProbe() *corev1.Probe {
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
					IntVal: n.Spec.Expose.Service.Port,
				},
			},
		},
	}

	return &probe
}

// GetReadinessProbe returns readiness probe for the NemoGuardrail container
func (n *NemoGuardrail) GetReadinessProbe() *corev1.Probe {
	if n.Spec.ReadinessProbe.Probe == nil {
		return n.GetDefaultReadinessProbe()
	}
	return n.Spec.ReadinessProbe.Probe
}

// GetDefaultReadinessProbe returns the default readiness probe for the NemoGuardrail container
func (n *NemoGuardrail) GetDefaultReadinessProbe() *corev1.Probe {
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
					IntVal: n.Spec.Expose.Service.Port,
				},
			},
		},
	}

	return &probe
}

// GetStartupProbe returns startup probe for the NemoGuardrail container
func (n *NemoGuardrail) GetStartupProbe() *corev1.Probe {
	if n.Spec.StartupProbe.Probe == nil {
		return n.GetDefaultStartupProbe()
	}
	return n.Spec.StartupProbe.Probe
}

// GetDefaultStartupProbe returns the default startup probe for the NemoGuardrail container
func (n *NemoGuardrail) GetDefaultStartupProbe() *corev1.Probe {
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
					IntVal: n.Spec.Expose.Service.Port,
				},
			},
		},
	}

	return &probe
}

// GetVolumes returns volumes for the NemoGuardrail container
func (n *NemoGuardrail) GetVolumes() []corev1.Volume {
	volumes := []corev1.Volume{}
	if n.Spec.ConfigStore.ConfigMap != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "config-store",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: n.Spec.ConfigStore.ConfigMap,
					},
				},
			},
		})
	} else {
		volumes = append(volumes, corev1.Volume{
			Name: "config-store",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: n.Spec.ConfigStore.PVC.Name,
				},
			},
		})
	}
	return volumes
}

// GetVolumeMounts returns volumes for the NemoGuardrail container
func (n *NemoGuardrail) GetVolumeMounts() []corev1.VolumeMount {
	volumeMount := corev1.VolumeMount{
		Name:      "config-store",
		MountPath: "/config-store",
	}
	if n.Spec.ConfigStore.PVC != nil {
		volumeMount.SubPath = n.Spec.ConfigStore.PVC.SubPath
	}

	return []corev1.VolumeMount{volumeMount}
}

// GetServiceAccountName returns service account name for the NemoGuardrail deployment
func (n *NemoGuardrail) GetServiceAccountName() string {
	return n.Name
}

// GetHPA returns the HPA spec for the NemoGuardrail deployment
func (n *NemoGuardrail) GetHPA() HorizontalPodAutoscalerSpec {
	return n.Spec.Scale.HPA
}

// GetServiceMonitor returns the Service Monitor details for the NemoGuardrail deployment
func (n *NemoGuardrail) GetServiceMonitor() ServiceMonitor {
	return n.Spec.Metrics.ServiceMonitor
}

// GetReplicas returns replicas for the NemoGuardrail deployment
func (n *NemoGuardrail) GetReplicas() int {
	if n.IsAutoScalingEnabled() {
		return 0
	}
	return n.Spec.Replicas
}

// GetDeploymentKind returns the kind of deployment for NemoGuardrail
func (n *NemoGuardrail) GetDeploymentKind() string {
	return "Deployment"
}

// IsAutoScalingEnabled returns true if autoscaling is enabled for NemoGuardrail deployment
func (n *NemoGuardrail) IsAutoScalingEnabled() bool {
	return n.Spec.Scale.Enabled != nil && *n.Spec.Scale.Enabled
}

// IsIngressEnabled returns true if ingress is enabled for NemoGuardrail deployment
func (n *NemoGuardrail) IsIngressEnabled() bool {
	return n.Spec.Expose.Ingress.Enabled != nil && *n.Spec.Expose.Ingress.Enabled
}

// GetIngressSpec returns the Ingress spec NemoGuardrail deployment
func (n *NemoGuardrail) GetIngressSpec() networkingv1.IngressSpec {
	return n.Spec.Expose.Ingress.Spec
}

// IsServiceMonitorEnabled returns true if servicemonitor is enabled for NemoGuardrail deployment
func (n *NemoGuardrail) IsServiceMonitorEnabled() bool {
	return n.Spec.Metrics.Enabled != nil && *n.Spec.Metrics.Enabled
}

// GetServicePort returns the service port for the NemoGuardrail deployment
func (n *NemoGuardrail) GetServicePort() int32 {
	return n.Spec.Expose.Service.Port
}

// GetServiceType returns the service type for the NemoGuardrail deployment
func (n *NemoGuardrail) GetServiceType() string {
	return string(n.Spec.Expose.Service.Type)
}

// GetUserID returns the user ID for the NemoGuardrail deployment
func (n *NemoGuardrail) GetUserID() *int64 {
	return n.Spec.UserID

}

// GetGroupID returns the group ID for the NemoGuardrail deployment
func (n *NemoGuardrail) GetGroupID() *int64 {
	return n.Spec.GroupID

}

// GetServiceAccountParams return params to render ServiceAccount from templates
func (n *NemoGuardrail) GetServiceAccountParams() *rendertypes.ServiceAccountParams {
	params := &rendertypes.ServiceAccountParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNemoGuardrailAnnotations()
	return params
}

// GetDeploymentParams returns params to render Deployment from templates
func (n *NemoGuardrail) GetDeploymentParams() *rendertypes.DeploymentParams {
	params := &rendertypes.DeploymentParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNemoGuardrailAnnotations()

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
func (n *NemoGuardrail) GetStatefulSetParams() *rendertypes.StatefulSetParams {

	params := &rendertypes.StatefulSetParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNemoGuardrailAnnotations()

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
func (n *NemoGuardrail) GetServiceParams() *rendertypes.ServiceParams {
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
	params.Port = n.GetServicePort()
	params.PortName = "service-port"
	return params
}

// GetIngressParams returns params to render Ingress from templates
func (n *NemoGuardrail) GetIngressParams() *rendertypes.IngressParams {
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

// GetRoleParams returns params to render Role from templates
func (n *NemoGuardrail) GetRoleParams() *rendertypes.RoleParams {
	params := &rendertypes.RoleParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()

	// Set rules to use SCC
	params.Rules = []rbacv1.PolicyRule{
		{
			APIGroups:     []string{"security.openshift.io"},
			Resources:     []string{"securitycontextconstraints"},
			ResourceNames: []string{"nonroot"},
			Verbs:         []string{"use"},
		},
	}

	return params
}

// GetRoleBindingParams returns params to render RoleBinding from templates
func (n *NemoGuardrail) GetRoleBindingParams() *rendertypes.RoleBindingParams {
	params := &rendertypes.RoleBindingParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()

	params.ServiceAccountName = n.GetServiceAccountName()
	params.RoleName = n.GetName()
	return params
}

// GetHPAParams returns params to render HPA from templates
func (n *NemoGuardrail) GetHPAParams() *rendertypes.HPAParams {
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

// GetSCCParams return params to render SCC from templates
func (n *NemoGuardrail) GetSCCParams() *rendertypes.SCCParams {
	params := &rendertypes.SCCParams{}
	// Set metadata
	params.Name = "nemo-guardrails-scc"

	params.ServiceAccountName = n.GetServiceAccountName()
	return params
}

// GetServiceMonitorParams return params to render Service Monitor from templates
func (n *NemoGuardrail) GetServiceMonitorParams() *rendertypes.ServiceMonitorParams {
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
		Endpoints:         []monitoringv1.Endpoint{{Port: "service-port", ScrapeTimeout: serviceMonitor.ScrapeTimeout, Interval: serviceMonitor.Interval}},
	}
	params.SMSpec = smSpec
	return params
}

func (n *NemoGuardrail) GetIngressAnnotations() map[string]string {
	NemoGuardrailAnnotations := n.GetNemoGuardrailAnnotations()

	if n.Spec.Expose.Ingress.Annotations != nil {
		return utils.MergeMaps(NemoGuardrailAnnotations, n.Spec.Expose.Ingress.Annotations)
	}
	return NemoGuardrailAnnotations
}

func (n *NemoGuardrail) GetServiceAnnotations() map[string]string {
	NemoGuardrailAnnotations := n.GetNemoGuardrailAnnotations()

	if n.Spec.Expose.Service.Annotations != nil {
		return utils.MergeMaps(NemoGuardrailAnnotations, n.Spec.Expose.Service.Annotations)
	}
	return NemoGuardrailAnnotations
}

func (n *NemoGuardrail) GetHPAAnnotations() map[string]string {
	NemoGuardrailAnnotations := n.GetNemoGuardrailAnnotations()

	if n.Spec.Scale.Annotations != nil {
		return utils.MergeMaps(NemoGuardrailAnnotations, n.Spec.Scale.Annotations)
	}
	return NemoGuardrailAnnotations
}

func (n *NemoGuardrail) GetServiceMonitorAnnotations() map[string]string {
	NemoGuardrailAnnotations := n.GetNemoGuardrailAnnotations()

	if n.Spec.Metrics.ServiceMonitor.Annotations != nil {
		return utils.MergeMaps(NemoGuardrailAnnotations, n.Spec.Metrics.ServiceMonitor.Annotations)
	}
	return NemoGuardrailAnnotations
}

func init() {
	SchemeBuilder.Register(&NemoGuardrail{}, &NemoGuardrailList{})
}
