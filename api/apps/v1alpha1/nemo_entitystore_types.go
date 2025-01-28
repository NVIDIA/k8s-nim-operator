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

package v1alpha1

import (
	"fmt"
	"maps"
	"os"
	"strconv"

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
	// NemoEntitystoreConditionReady indicates that the NEMO EntitystoreService is ready.
	NemoEntitystoreConditionReady = "Ready"
	// NemoEntitystoreConditionFailed indicates that the NEMO EntitystoreService has failed.
	NemoEntitystoreConditionFailed = "Failed"

	// NemoEntitystoreStatusPending indicates that NEMO EntitystoreService is in pending state
	NemoEntitystoreStatusPending = "Pending"
	// NemoEntitystoreStatusNotReady indicates that NEMO EntitystoreService is not ready
	NemoEntitystoreStatusNotReady = "NotReady"
	// NemoEntitystoreStatusReady indicates that NEMO EntitystoreService is ready
	NemoEntitystoreStatusReady = "Ready"
	// NemoEntitystoreStatusFailed indicates that NEMO EntitystoreService has failed
	NemoEntitystoreStatusFailed = "Failed"
)

// NemoEntitystoreSpec defines the desired state of NemoEntitystore
type NemoEntitystoreSpec struct {
	Image          Image                        `json:"image,omitempty"`
	Command        []string                     `json:"command,omitempty"`
	Args           []string                     `json:"args,omitempty"`
	Env            []corev1.EnvVar              `json:"env,omitempty"`
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
	Replicas     int    `json:"replicas,omitempty"`
	UserID       *int64 `json:"userID,omitempty"`
	GroupID      *int64 `json:"groupID,omitempty"`
	RuntimeClass string `json:"runtimeClass,omitempty"`
	// DatabaseConfig stores the database configuration for NEMO entitystore.
	// Required, must not be nil.
	//
	// +kubebuilder:validation:Required
	DatabaseConfig *DatabaseConfig `json:"databaseConfig,omitempty"`
}

// NemoEntitystoreStatus defines the observed state of NemoEntitystore
type NemoEntitystoreStatus struct {
	Conditions        []metav1.Condition `json:"conditions,omitempty"`
	AvailableReplicas int32              `json:"availableReplicas,omitempty"`
	State             string             `json:"state,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.state`,priority=0
// +kubebuilder:printcolumn:name="Age",type="date",format="date-time",JSONPath=".metadata.creationTimestamp",priority=0

// NemoEntitystore is the Schema for the NemoEntitystore API
type NemoEntitystore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NemoEntitystoreSpec   `json:"spec,omitempty"`
	Status NemoEntitystoreStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NemoEntitystoreList contains a list of NemoEntitystore
type NemoEntitystoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NemoEntitystore `json:"items"`
}

// GetPVCName returns the name to be used for the PVC based on the custom spec
// Prefers pvc.Name if explicitly set by the user in the NemoEntitystore instance
func (n *NemoEntitystore) GetPVCName(pvc PersistentVolumeClaim) string {
	pvcName := fmt.Sprintf("%s-pvc", n.GetName())
	if pvc.Name != "" {
		pvcName = pvc.Name
	}
	return pvcName
}

// GetStandardSelectorLabels returns the standard selector labels for the NemoEntitystore deployment
func (n *NemoEntitystore) GetStandardSelectorLabels() map[string]string {
	return map[string]string{
		"app":                        n.Name,
		"app.kubernetes.io/name":     n.Name,
		"app.kubernetes.io/instance": n.Name,
	}
}

// GetStandardLabels returns the standard set of labels for NemoEntitystore resources
func (n *NemoEntitystore) GetStandardLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":             n.Name,
		"app.kubernetes.io/instance":         n.Name,
		"app.kubernetes.io/operator-version": os.Getenv("OPERATOR_VERSION"),
		"app.kubernetes.io/part-of":          "nemo-entitystore-service",
		"app.kubernetes.io/managed-by":       "k8s-nim-operator",
	}
}

// GetStandardEnv returns the standard set of env variables for the NemoEntitystore container
func (n *NemoEntitystore) GetStandardEnv() []corev1.EnvVar {
	// add standard env required for NIM service
	envVars := []corev1.EnvVar{
		{
			Name:  "APP_VERSION",
			Value: n.Spec.Image.Tag,
		},
		{
			Name: "POSTGRES_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: n.Spec.DatabaseConfig.Credentials.PasswordKey,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: n.Spec.DatabaseConfig.Credentials.SecretName,
					},
				},
			},
		},
		{
			Name:  "POSTGRES_USER",
			Value: n.Spec.DatabaseConfig.Credentials.User,
		},
		{
			Name:  "POSTGRES_HOST",
			Value: n.Spec.DatabaseConfig.Host,
		},
		{
			Name:  "POSTGRES_PORT",
			Value: strconv.FormatInt(int64(n.Spec.DatabaseConfig.Port), 10),
		},
		{
			Name:  "POSTGRES_DB",
			Value: n.Spec.DatabaseConfig.DatabaseName,
		},
	}

	return envVars
}

// GetStandardAnnotations returns default annotations to apply to the NemoEntitystore instance
func (n *NemoEntitystore) GetStandardAnnotations() map[string]string {
	standardAnnotations := map[string]string{
		"openshift.io/scc": "nonroot",
	}
	return standardAnnotations
}

// GetNemoEntitystoreAnnotations returns annotations to apply to the NemoEntitystore instance
func (n *NemoEntitystore) GetNemoEntitystoreAnnotations() map[string]string {
	standardAnnotations := n.GetStandardAnnotations()

	if n.Spec.Annotations != nil {
		return utils.MergeMaps(standardAnnotations, n.Spec.Annotations)
	}

	return standardAnnotations
}

// GetServiceLabels returns merged labels to apply to the NemoEntitystore instance
func (n *NemoEntitystore) GetServiceLabels() map[string]string {
	standardLabels := n.GetStandardLabels()

	if n.Spec.Labels != nil {
		return utils.MergeMaps(standardLabels, n.Spec.Labels)
	}
	return standardLabels
}

// GetSelectorLabels returns standard selector labels to apply to the NemoEntitystore instance
func (n *NemoEntitystore) GetSelectorLabels() map[string]string {
	// TODO: add custom ones
	return n.GetStandardSelectorLabels()
}

// GetNodeSelector returns node selector labels for the NemoEntitystore instance
func (n *NemoEntitystore) GetNodeSelector() map[string]string {
	return n.Spec.NodeSelector
}

// GetTolerations returns tolerations for the NemoEntitystore instance
func (n *NemoEntitystore) GetTolerations() []corev1.Toleration {
	return n.Spec.Tolerations
}

// GetPodAffinity returns pod affinity for the NemoEntitystore instance
func (n *NemoEntitystore) GetPodAffinity() *corev1.PodAffinity {
	return n.Spec.PodAffinity
}

// GetContainerName returns name of the container for NemoEntitystore deployment
func (n *NemoEntitystore) GetContainerName() string {
	return fmt.Sprintf("%s-ctr", n.Name)
}

// GetCommand return command to override for the NemoEntitystore container
func (n *NemoEntitystore) GetCommand() []string {
	return n.Spec.Command
}

// GetArgs return arguments for the NemoEntitystore container
func (n *NemoEntitystore) GetArgs() []string {
	return n.Spec.Args
}

// GetEnv returns merged slice of standard and user specified env variables
func (n *NemoEntitystore) GetEnv() []corev1.EnvVar {
	return utils.MergeEnvVars(n.GetStandardEnv(), n.Spec.Env)
}

// GetImage returns container image for the NemoEntitystore
func (n *NemoEntitystore) GetImage() string {
	return fmt.Sprintf("%s:%s", n.Spec.Image.Repository, n.Spec.Image.Tag)
}

// GetImagePullSecrets returns the image pull secrets for the NIM container
func (n *NemoEntitystore) GetImagePullSecrets() []string {
	return n.Spec.Image.PullSecrets
}

// GetImagePullPolicy returns the image pull policy for the NIM container
func (n *NemoEntitystore) GetImagePullPolicy() string {
	return n.Spec.Image.PullPolicy
}

// GetResources returns resources to allocate to the NemoEntitystore container
func (n *NemoEntitystore) GetResources() *corev1.ResourceRequirements {
	return n.Spec.Resources
}

// GetLivenessProbe returns liveness probe for the NemoEntitystore container
func (n *NemoEntitystore) GetLivenessProbe() *corev1.Probe {
	if n.Spec.LivenessProbe.Probe == nil {
		return n.GetDefaultLivenessProbe()
	}
	return n.Spec.LivenessProbe.Probe
}

// GetDefaultLivenessProbe returns the default liveness probe for the NemoEntitystore container
func (n *NemoEntitystore) GetDefaultLivenessProbe() *corev1.Probe {
	probe := corev1.Probe{
		InitialDelaySeconds: 3,
		TimeoutSeconds:      20,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		FailureThreshold:    10,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/health",
				Port: intstr.IntOrString{
					Type:   intstr.Type(0),
					IntVal: n.Spec.Expose.Service.Port,
				},
			},
		},
	}

	return &probe
}

// GetReadinessProbe returns readiness probe for the NemoEntitystore container
func (n *NemoEntitystore) GetReadinessProbe() *corev1.Probe {
	if n.Spec.ReadinessProbe.Probe == nil {
		return n.GetDefaultReadinessProbe()
	}
	return n.Spec.ReadinessProbe.Probe
}

// GetDefaultReadinessProbe returns the default readiness probe for the NemoEntitystore container
func (n *NemoEntitystore) GetDefaultReadinessProbe() *corev1.Probe {
	probe := corev1.Probe{
		InitialDelaySeconds: 10,
		TimeoutSeconds:      20,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		FailureThreshold:    20,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/health",
				Port: intstr.IntOrString{
					Type:   intstr.Type(0),
					IntVal: n.Spec.Expose.Service.Port,
				},
			},
		},
	}

	return &probe
}

// GetStartupProbe returns startup probe for the NemoEntitystore container
func (n *NemoEntitystore) GetStartupProbe() *corev1.Probe {
	if n.Spec.StartupProbe.Probe == nil {
		return n.GetDefaultStartupProbe()
	}
	return n.Spec.StartupProbe.Probe
}

// GetDefaultStartupProbe returns the default startup probe for the NemoEntitystore container
func (n *NemoEntitystore) GetDefaultStartupProbe() *corev1.Probe {
	probe := corev1.Probe{
		InitialDelaySeconds: 40,
		TimeoutSeconds:      1,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		FailureThreshold:    180,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/health",
				Port: intstr.IntOrString{
					Type:   intstr.Type(0),
					IntVal: n.Spec.Expose.Service.Port,
				},
			},
		},
	}

	return &probe
}

// GetVolumes returns volumes for the NemoEntitystore container
func (n *NemoEntitystore) GetVolumes() []corev1.Volume {
	return []corev1.Volume{}
}

// GetVolumeMounts returns volumes for the NemoEntitystore container
func (n *NemoEntitystore) GetVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{}
}

// GetServiceAccountName returns service account name for the NemoEntitystore deployment
func (n *NemoEntitystore) GetServiceAccountName() string {
	return n.Name
}

// GetRuntimeClass return the runtime class name for the NemoEntitystore deployment
func (n *NemoEntitystore) GetRuntimeClass() string {
	return n.Spec.RuntimeClass
}

// GetHPA returns the HPA spec for the NemoEntitystore deployment
func (n *NemoEntitystore) GetHPA() HorizontalPodAutoscalerSpec {
	return n.Spec.Scale.HPA
}

// GetServiceMonitor returns the Service Monitor details for the NemoEntitystore deployment
func (n *NemoEntitystore) GetServiceMonitor() ServiceMonitor {
	return n.Spec.Metrics.ServiceMonitor
}

// GetReplicas returns replicas for the NemoEntitystore deployment
func (n *NemoEntitystore) GetReplicas() int {
	if n.IsAutoScalingEnabled() {
		return 0
	}
	return n.Spec.Replicas
}

// GetDeploymentKind returns the kind of deployment for NemoEntitystore
func (n *NemoEntitystore) GetDeploymentKind() string {
	return "Deployment"
}

// IsAutoScalingEnabled returns true if autoscaling is enabled for NemoEntitystore deployment
func (n *NemoEntitystore) IsAutoScalingEnabled() bool {
	return n.Spec.Scale.Enabled != nil && *n.Spec.Scale.Enabled
}

// IsIngressEnabled returns true if ingress is enabled for NemoEntitystore deployment
func (n *NemoEntitystore) IsIngressEnabled() bool {
	return n.Spec.Expose.Ingress.Enabled != nil && *n.Spec.Expose.Ingress.Enabled
}

// GetIngressSpec returns the Ingress spec NemoEntitystore deployment
func (n *NemoEntitystore) GetIngressSpec() networkingv1.IngressSpec {
	return n.Spec.Expose.Ingress.Spec
}

// IsServiceMonitorEnabled returns true if servicemonitor is enabled for NemoEntitystore deployment
func (n *NemoEntitystore) IsServiceMonitorEnabled() bool {
	return n.Spec.Metrics.Enabled != nil && *n.Spec.Metrics.Enabled
}

// GetServicePort returns the service port for the NemoEntitystore deployment
func (n *NemoEntitystore) GetServicePort() int32 {
	return n.Spec.Expose.Service.Port
}

// GetServiceType returns the service type for the NemoEntitystore deployment
func (n *NemoEntitystore) GetServiceType() string {
	return string(n.Spec.Expose.Service.Type)
}

// GetUserID returns the user ID for the NemoEntitystore deployment
func (n *NemoEntitystore) GetUserID() *int64 {
	return n.Spec.UserID

}

// GetGroupID returns the group ID for the NemoEntitystore deployment
func (n *NemoEntitystore) GetGroupID() *int64 {
	return n.Spec.GroupID

}

// GetServiceAccountParams return params to render ServiceAccount from templates
func (n *NemoEntitystore) GetServiceAccountParams() *rendertypes.ServiceAccountParams {
	params := &rendertypes.ServiceAccountParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNemoEntitystoreAnnotations()
	return params
}

// GetDeploymentParams returns params to render Deployment from templates
func (n *NemoEntitystore) GetDeploymentParams() *rendertypes.DeploymentParams {
	params := &rendertypes.DeploymentParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNemoEntitystoreAnnotations()

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
	params.RuntimeClassName = n.GetRuntimeClass()
	return params
}

// GetStatefulSetParams returns params to render StatefulSet from templates
func (n *NemoEntitystore) GetStatefulSetParams() *rendertypes.StatefulSetParams {

	params := &rendertypes.StatefulSetParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNemoEntitystoreAnnotations()

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
	params.RuntimeClassName = n.GetRuntimeClass()
	return params
}

// GetServiceParams returns params to render Service from templates
func (n *NemoEntitystore) GetServiceParams() *rendertypes.ServiceParams {
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
func (n *NemoEntitystore) GetIngressParams() *rendertypes.IngressParams {
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
func (n *NemoEntitystore) GetRoleParams() *rendertypes.RoleParams {
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
func (n *NemoEntitystore) GetRoleBindingParams() *rendertypes.RoleBindingParams {
	params := &rendertypes.RoleBindingParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()

	params.ServiceAccountName = n.GetServiceAccountName()
	params.RoleName = n.GetName()
	return params
}

// GetHPAParams returns params to render HPA from templates
func (n *NemoEntitystore) GetHPAParams() *rendertypes.HPAParams {
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
func (n *NemoEntitystore) GetSCCParams() *rendertypes.SCCParams {
	params := &rendertypes.SCCParams{}
	// Set metadata
	params.Name = "nemo-entity-scc"

	params.ServiceAccountName = n.GetServiceAccountName()
	return params
}

// GetServiceMonitorParams return params to render Service Monitor from templates
func (n *NemoEntitystore) GetServiceMonitorParams() *rendertypes.ServiceMonitorParams {
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

func (n *NemoEntitystore) GetIngressAnnotations() map[string]string {
	NemoEntitystoreAnnotations := n.GetNemoEntitystoreAnnotations()

	if n.Spec.Expose.Ingress.Annotations != nil {
		return utils.MergeMaps(NemoEntitystoreAnnotations, n.Spec.Expose.Ingress.Annotations)
	}
	return NemoEntitystoreAnnotations
}

func (n *NemoEntitystore) GetServiceAnnotations() map[string]string {
	NemoEntitystoreAnnotations := n.GetNemoEntitystoreAnnotations()

	if n.Spec.Expose.Service.Annotations != nil {
		return utils.MergeMaps(NemoEntitystoreAnnotations, n.Spec.Expose.Service.Annotations)
	}
	return NemoEntitystoreAnnotations
}

func (n *NemoEntitystore) GetHPAAnnotations() map[string]string {
	NemoEntitystoreAnnotations := n.GetNemoEntitystoreAnnotations()

	if n.Spec.Scale.Annotations != nil {
		return utils.MergeMaps(NemoEntitystoreAnnotations, n.Spec.Scale.Annotations)
	}
	return NemoEntitystoreAnnotations
}

func (n *NemoEntitystore) GetServiceMonitorAnnotations() map[string]string {
	NemoEntitystoreAnnotations := n.GetNemoEntitystoreAnnotations()

	if n.Spec.Metrics.ServiceMonitor.Annotations != nil {
		return utils.MergeMaps(NemoEntitystoreAnnotations, n.Spec.Metrics.ServiceMonitor.Annotations)
	}
	return NemoEntitystoreAnnotations
}

func init() {
	SchemeBuilder.Register(&NemoEntitystore{}, &NemoEntitystoreList{})
}
