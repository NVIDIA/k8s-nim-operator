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
	"strings"

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

const (
	// CustomizerAPIPort is the default port that customizer serves on
	CustomizerAPIPort = 8000
	// DefaultNamedPortInternal is the default name for customizer internal port
	DefaultNamedPortInternal = "internal"
	// CustomizerInternalPort is the default port used for syncing training progress
	CustomizerInternalPort = 9009
	// NemoCustomizerConditionReady indicates that the NEMO CustomizerService is ready.
	NemoCustomizerConditionReady = "Ready"
	// NemoCustomizerConditionFailed indicates that the NEMO CustomizerService has failed.
	NemoCustomizerConditionFailed = "Failed"

	// NemoCustomizerStatusPending indicates that NEMO CustomizerService is in pending state
	NemoCustomizerStatusPending = "Pending"
	// NemoCustomizerStatusNotReady indicates that NEMO CustomizerService is not ready
	NemoCustomizerStatusNotReady = "NotReady"
	// NemoCustomizerStatusReady indicates that NEMO CustomizerService is ready
	NemoCustomizerStatusReady = "Ready"
	// NemoCustomizerStatusFailed indicates that NEMO CustomizerService has failed
	NemoCustomizerStatusFailed = "Failed"

	// SchedulerTypeVolcano indicates if the scheduler is volcano
	SchedulerTypeVolcano = "volcano"
	// SchedulerTypeRunAI indicates if the scheduler is run.ai
	SchedulerTypeRunAI = "runai"
)

// NemoCustomizerSpec defines the desired state of NemoCustomizer
type NemoCustomizerSpec struct {
	Image          Image                        `json:"image"`
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

	// CustomizerConfig stores the customizer configuration for training and models
	// +kubebuilder:validation:MinLength=1
	CustomizerConfig string `json:"customizerConfig"`

	// Scheduler Configuration
	Scheduler Scheduler `json:"scheduler,omitempty"`

	// OpenTelemetry Settings
	OpenTelemetry OTelSpec `json:"otel"`

	// DatabaseConfig stores the database configuration
	DatabaseConfig DatabaseConfig `json:"databaseConfig"`

	// WandBSecret stores the secret and encryption key for the Weights and Biases service.
	WandBSecret WandBSecret `json:"wandbSecret"`
}

// Scheduler defines the configuration for the scheduler
type Scheduler struct {
	// Type is the scheduler type (volcano, runai)
	// +kubebuilder:validation:Enum=volcano;runai
	// +kubebuilder:default:=volcano
	Type string `json:"type,omitempty"`
}

// NemoCustomizerStatus defines the observed state of NemoCustomizer
type NemoCustomizerStatus struct {
	Conditions        []metav1.Condition `json:"conditions,omitempty"`
	AvailableReplicas int32              `json:"availableReplicas,omitempty"`
	State             string             `json:"state,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.state`,priority=0
// +kubebuilder:printcolumn:name="Age",type="date",format="date-time",JSONPath=".metadata.creationTimestamp",priority=0

// NemoCustomizer is the Schema for the NemoCustomizer API
type NemoCustomizer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NemoCustomizerSpec   `json:"spec,omitempty"`
	Status NemoCustomizerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NemoCustomizerList contains a list of NemoCustomizer
type NemoCustomizerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NemoCustomizer `json:"items"`
}

// GetStandardSelectorLabels returns the standard selector labels for the NemoCustomizer deployment
func (n *NemoCustomizer) GetStandardSelectorLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name": n.Name,
	}
}

// GetStandardLabels returns the standard set of labels for NemoCustomizer resources
func (n *NemoCustomizer) GetStandardLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":             n.Name,
		"app.kubernetes.io/instance":         n.Name,
		"app.kubernetes.io/operator-version": os.Getenv("OPERATOR_VERSION"),
		"app.kubernetes.io/part-of":          "nemo-customizer-service",
		"app.kubernetes.io/managed-by":       "k8s-nim-operator",
	}
}

// GetStandardEnv returns the standard set of env variables for the NemoCustomizer container
func (n *NemoCustomizer) GetStandardEnv() []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  "NAMESPACE",
			Value: n.Namespace,
		},
		{
			Name: "HOST_IP",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "status.hostIP",
				},
			},
		},
		{
			Name:  "CONFIG_PATH",
			Value: "/app/config/config.yaml",
		},
		{
			Name:  "CUSTOMIZATIONS_CALLBACK_URL",
			Value: fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", n.GetName(), n.GetNamespace(), CustomizerInternalPort),
		},
		{
			Name:  "LOG_LEVEL",
			Value: "INFO",
		},
		{
			Name: "WANDB_ENCRYPTION_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: n.Spec.WandBSecret.APIKeyKey,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: n.Spec.WandBSecret.Name,
					},
				},
			},
		},
		{
			Name:  "WANDB_SECRET_NAME",
			Value: n.Spec.WandBSecret.Name,
		},
		{
			Name:  "WANDB_SECRET_KEY",
			Value: n.Spec.WandBSecret.APIKeyKey,
		},
	}

	// Append the environment variables for Postgres
	envVars = append(envVars, n.GetPostgresEnv()...)

	// Append the environment variables for OTel
	if n.IsOtelEnabled() {
		envVars = append(envVars, n.GetOtelEnv()...)
	}

	return envVars
}

// IsOtelEnabled returns true if Open Telemetry Collector is enabled
func (n *NemoCustomizer) IsOtelEnabled() bool {
	return n.Spec.OpenTelemetry.Enabled != nil && *n.Spec.OpenTelemetry.Enabled
}

// GetOtelEnv generates OpenTelemetry-related environment variables.
func (n *NemoCustomizer) GetOtelEnv() []corev1.EnvVar {
	var otelEnvVars []corev1.EnvVar

	otelEnvVars = append(otelEnvVars,
		corev1.EnvVar{Name: "OTEL_EXPORTER_OTLP_ENDPOINT", Value: n.Spec.OpenTelemetry.ExporterOtlpEndpoint},
		corev1.EnvVar{Name: "OTEL_TRACES_EXPORTER", Value: n.Spec.OpenTelemetry.ExporterConfig.TracesExporter},
		corev1.EnvVar{Name: "OTEL_METRICS_EXPORTER", Value: n.Spec.OpenTelemetry.ExporterConfig.MetricsExporter},
		corev1.EnvVar{Name: "OTEL_LOGS_EXPORTER", Value: n.Spec.OpenTelemetry.ExporterConfig.LogsExporter},
		corev1.EnvVar{Name: "OTEL_LOG_LEVEL", Value: n.Spec.OpenTelemetry.LogLevel},
	)

	if len(n.Spec.OpenTelemetry.ExcludedUrls) > 0 {
		otelEnvVars = append(otelEnvVars, corev1.EnvVar{
			Name:  "OTEL_PYTHON_EXCLUDED_URLS",
			Value: strings.Join(n.Spec.OpenTelemetry.ExcludedUrls, ","),
		})
	}

	var enableLog bool = true
	if n.Spec.OpenTelemetry.DisableLogging != nil {
		enableLog = !*n.Spec.OpenTelemetry.DisableLogging
	}
	otelEnvVars = append(otelEnvVars, corev1.EnvVar{
		Name:  "OTEL_PYTHON_LOGGING_AUTO_INSTRUMENTATION_ENABLED",
		Value: strconv.FormatBool(enableLog),
	})

	return otelEnvVars
}

// GetPostgresEnv returns the PostgreSQL environment variables for a Kubernetes pod.
func (n *NemoCustomizer) GetPostgresEnv() []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name: "POSTGRES_DB_PASSWORD",
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
			Name: "POSTGRES_DB_DSN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: "dsn",
					LocalObjectReference: corev1.LocalObjectReference{
						Name: n.Name,
					},
				},
			},
		},
	}

	return envVars
}

// GeneratePostgresConnString generates a PostgreSQL connection string using the database config.
func (n *NemoCustomizer) GeneratePostgresConnString(secretValue string) string {
	// Construct the connection string
	connString := fmt.Sprintf(
		"postgresql://%s:%s@%s:%d/%s",
		n.Spec.DatabaseConfig.Credentials.User,
		secretValue,
		n.Spec.DatabaseConfig.Host,
		n.Spec.DatabaseConfig.Port,
		n.Spec.DatabaseConfig.DatabaseName,
	)

	return connString
}

// GetVolumes generates the volumes required for the customizer.
func (n *NemoCustomizer) GetVolumes() []corev1.Volume {
	return []corev1.Volume{
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: n.GetName(),
					},
					Items: []corev1.KeyToPath{
						{
							Key:  "config.yaml",
							Path: "config.yaml",
						},
					},
				},
			},
		},
	}
}

// GetVolumeMounts generates the volume mounts required for the customizer.
func (n *NemoCustomizer) GetVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "config",
			MountPath: "/app/config",
			ReadOnly:  true,
		},
	}
}

// GetStandardAnnotations returns default annotations to apply to the NemoCustomizer instance
func (n *NemoCustomizer) GetStandardAnnotations() map[string]string {
	standardAnnotations := map[string]string{
		"openshift.io/scc":                      "nonroot",
		utils.NvidiaAnnotationParentSpecHashKey: utils.DeepHashObject(n.Spec),
	}
	return standardAnnotations
}

// GetNemoCustomizerAnnotations returns annotations to apply to the NemoCustomizer instance
func (n *NemoCustomizer) GetNemoCustomizerAnnotations() map[string]string {
	standardAnnotations := n.GetStandardAnnotations()

	if n.Spec.Annotations != nil {
		return utils.MergeMaps(standardAnnotations, n.Spec.Annotations)
	}

	return standardAnnotations
}

// GetServiceLabels returns merged labels to apply to the NemoCustomizer instance
func (n *NemoCustomizer) GetServiceLabels() map[string]string {
	standardLabels := n.GetStandardLabels()

	if n.Spec.Labels != nil {
		return utils.MergeMaps(standardLabels, n.Spec.Labels)
	}
	return standardLabels
}

// GetSelectorLabels returns standard selector labels to apply to the NemoCustomizer instance
func (n *NemoCustomizer) GetSelectorLabels() map[string]string {
	// TODO: add custom ones
	return n.GetStandardSelectorLabels()
}

// GetNodeSelector returns node selector labels for the NemoCustomizer instance
func (n *NemoCustomizer) GetNodeSelector() map[string]string {
	return n.Spec.NodeSelector
}

// GetTolerations returns tolerations for the NemoCustomizer instance
func (n *NemoCustomizer) GetTolerations() []corev1.Toleration {
	return n.Spec.Tolerations
}

// GetPodAffinity returns pod affinity for the NemoCustomizer instance
func (n *NemoCustomizer) GetPodAffinity() *corev1.PodAffinity {
	return n.Spec.PodAffinity
}

// GetContainerName returns name of the container for NemoCustomizer deployment
func (n *NemoCustomizer) GetContainerName() string {
	return fmt.Sprintf("%s-ctr", n.Name)
}

// GetCommand return command to override for the NemoCustomizer container
func (n *NemoCustomizer) GetCommand() []string {
	return n.Spec.Command
}

// GetArgs return arguments for the NemoCustomizer container
func (n *NemoCustomizer) GetArgs() []string {
	return n.Spec.Args
}

// GetEnv returns merged slice of standard and user specified env variables
func (n *NemoCustomizer) GetEnv() []corev1.EnvVar {
	return utils.MergeEnvVars(n.GetStandardEnv(), n.Spec.Env)
}

// GetImage returns container image for the NemoCustomizer
func (n *NemoCustomizer) GetImage() string {
	return fmt.Sprintf("%s:%s", n.Spec.Image.Repository, n.Spec.Image.Tag)
}

// GetImagePullSecrets returns the image pull secrets for the NIM container
func (n *NemoCustomizer) GetImagePullSecrets() []string {
	return n.Spec.Image.PullSecrets
}

// GetImagePullPolicy returns the image pull policy for the NIM container
func (n *NemoCustomizer) GetImagePullPolicy() string {
	return n.Spec.Image.PullPolicy
}

// GetResources returns resources to allocate to the NemoCustomizer container
func (n *NemoCustomizer) GetResources() *corev1.ResourceRequirements {
	return n.Spec.Resources
}

// GetStartupProbe returns startup probe for the NemoCustomizer container
func (n *NemoCustomizer) GetStartupProbe() *corev1.Probe {
	if n.Spec.StartupProbe.Probe == nil {
		return n.GetDefaultStartupProbe()
	}
	return n.Spec.StartupProbe.Probe
}

// GetDefaultStartupProbe returns the default startup probe for the NemoCustomizer container
func (n *NemoCustomizer) GetDefaultStartupProbe() *corev1.Probe {
	probe := corev1.Probe{
		FailureThreshold:    30,
		InitialDelaySeconds: 30,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		TimeoutSeconds:      1,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/v1/health/ready",
				Port: intstr.FromString(DefaultNamedPortAPI),
			},
		},
	}

	return &probe
}

// GetLivenessProbe returns liveness probe for the NemoCustomizer container
func (n *NemoCustomizer) GetLivenessProbe() *corev1.Probe {
	if n.Spec.LivenessProbe.Probe == nil {
		return n.GetDefaultLivenessProbe()
	}
	return n.Spec.LivenessProbe.Probe
}

// GetDefaultLivenessProbe returns the default liveness probe for the NemoCustomizer container
func (n *NemoCustomizer) GetDefaultLivenessProbe() *corev1.Probe {
	probe := corev1.Probe{
		FailureThreshold:    5,
		InitialDelaySeconds: 10,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		TimeoutSeconds:      10,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/v1/health/live",
				Port: intstr.FromString(DefaultNamedPortAPI),
			},
		},
	}
	return &probe
}

// GetReadinessProbe returns readiness probe for the NemoCustomizer container
func (n *NemoCustomizer) GetReadinessProbe() *corev1.Probe {
	if n.Spec.ReadinessProbe.Probe == nil {
		return n.GetDefaultReadinessProbe()
	}
	return n.Spec.ReadinessProbe.Probe
}

// GetDefaultReadinessProbe returns the default readiness probe for the NemoCustomizer container
func (n *NemoCustomizer) GetDefaultReadinessProbe() *corev1.Probe {
	probe := corev1.Probe{
		FailureThreshold:    3,
		InitialDelaySeconds: 10,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		TimeoutSeconds:      1,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/v1/health/ready",
				Port: intstr.FromString(DefaultNamedPortAPI),
			},
		},
	}

	return &probe
}

// GetServiceAccountName returns service account name for the NemoCustomizer deployment
func (n *NemoCustomizer) GetServiceAccountName() string {
	return n.Name
}

// GetRuntimeClass return the runtime class name for the NemoCustomizer deployment
func (n *NemoCustomizer) GetRuntimeClass() string {
	return n.Spec.RuntimeClass
}

// GetHPA returns the HPA spec for the NemoCustomizer deployment
func (n *NemoCustomizer) GetHPA() HorizontalPodAutoscalerSpec {
	return n.Spec.Scale.HPA
}

// GetServiceMonitor returns the Service Monitor details for the NemoCustomizer deployment
func (n *NemoCustomizer) GetServiceMonitor() ServiceMonitor {
	return n.Spec.Metrics.ServiceMonitor
}

// GetReplicas returns replicas for the NemoCustomizer deployment
func (n *NemoCustomizer) GetReplicas() int {
	if n.IsAutoScalingEnabled() {
		return 0
	}
	return n.Spec.Replicas
}

// GetDeploymentKind returns the kind of deployment for NemoCustomizer
func (n *NemoCustomizer) GetDeploymentKind() string {
	return "Deployment"
}

// IsAutoScalingEnabled returns true if autoscaling is enabled for NemoCustomizer deployment
func (n *NemoCustomizer) IsAutoScalingEnabled() bool {
	return n.Spec.Scale.Enabled != nil && *n.Spec.Scale.Enabled
}

// IsIngressEnabled returns true if ingress is enabled for NemoCustomizer deployment
func (n *NemoCustomizer) IsIngressEnabled() bool {
	return n.Spec.Expose.Ingress.Enabled != nil && *n.Spec.Expose.Ingress.Enabled
}

// GetIngressSpec returns the Ingress spec NemoCustomizer deployment
func (n *NemoCustomizer) GetIngressSpec() networkingv1.IngressSpec {
	return n.Spec.Expose.Ingress.Spec
}

// IsServiceMonitorEnabled returns true if servicemonitor is enabled for NemoCustomizer deployment
func (n *NemoCustomizer) IsServiceMonitorEnabled() bool {
	return n.Spec.Metrics.Enabled != nil && *n.Spec.Metrics.Enabled
}

// GetServiceType returns the service type for the NemoCustomizer deployment
func (n *NemoCustomizer) GetServiceType() string {
	return string(n.Spec.Expose.Service.Type)
}

// GetUserID returns the user ID for the NemoCustomizer deployment
func (n *NemoCustomizer) GetUserID() *int64 {
	return n.Spec.UserID

}

// GetGroupID returns the group ID for the NemoCustomizer deployment
func (n *NemoCustomizer) GetGroupID() *int64 {
	return n.Spec.GroupID

}

// GetServiceAccountParams return params to render ServiceAccount from templates
func (n *NemoCustomizer) GetServiceAccountParams() *rendertypes.ServiceAccountParams {
	params := &rendertypes.ServiceAccountParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNemoCustomizerAnnotations()
	return params
}

// GetDeploymentParams returns params to render Deployment from templates
func (n *NemoCustomizer) GetDeploymentParams() *rendertypes.DeploymentParams {
	params := &rendertypes.DeploymentParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNemoCustomizerAnnotations()
	params.PodAnnotations = n.GetNemoCustomizerAnnotations()
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
	params.RuntimeClassName = n.GetRuntimeClass()

	// Setup container ports for customizer
	params.Ports = []corev1.ContainerPort{
		{
			Name:          DefaultNamedPortAPI,
			Protocol:      corev1.ProtocolTCP,
			ContainerPort: CustomizerAPIPort,
		},
		{
			Name:          DefaultNamedPortInternal,
			Protocol:      corev1.ProtocolTCP,
			ContainerPort: CustomizerInternalPort,
		},
	}
	return params
}

// GetStatefulSetParams returns params to render StatefulSet from templates
func (n *NemoCustomizer) GetStatefulSetParams() *rendertypes.StatefulSetParams {

	params := &rendertypes.StatefulSetParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNemoCustomizerAnnotations()

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
func (n *NemoCustomizer) GetServiceParams() *rendertypes.ServiceParams {
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
		{
			Name:       DefaultNamedPortInternal,
			Port:       CustomizerInternalPort,
			TargetPort: intstr.FromString((DefaultNamedPortInternal)),
			Protocol:   corev1.ProtocolTCP,
		},
	}
	return params
}

// GetIngressParams returns params to render Ingress from templates
func (n *NemoCustomizer) GetIngressParams() *rendertypes.IngressParams {
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
func (n *NemoCustomizer) GetRoleParams() *rendertypes.RoleParams {
	params := &rendertypes.RoleParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()

	// Set rules for customizer
	params.Rules = []rbacv1.PolicyRule{
		{
			APIGroups:     []string{"security.openshift.io"},
			Resources:     []string{"securitycontextconstraints"},
			ResourceNames: []string{"nonroot"},
			Verbs:         []string{"use"},
		},
		{
			APIGroups: []string{"batch"},
			Resources: []string{"jobs"},
			Verbs:     []string{"create", "get", "list", "watch", "update", "delete", "patch"},
		},
		{
			APIGroups: []string{"batch"},
			Resources: []string{"jobs/status"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods", "persistentvolumeclaims", "services", "configmaps"},
			Verbs:     []string{"create", "get", "list", "watch", "delete"},
		},
		{
			APIGroups: []string{"nvidia.com"},
			Resources: []string{"nemotrainingjobs", "nemotrainingjobs/status", "nemoentityhandlers"},
			Verbs:     []string{"create", "get", "list", "watch", "update", "delete", "patch"},
		},
	}

	volcanoRules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{"batch.volcano.sh"},
			Resources: []string{"jobs"},
			Verbs:     []string{"create", "get", "list", "watch", "update", "delete", "patch"},
		},
		{
			APIGroups: []string{"batch.volcano.sh"},
			Resources: []string{"jobs/status"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"nodeinfo.volcano.sh"},
			Resources: []string{"numatopologies"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"scheduling.incubator.k8s.io", "scheduling.volcano.sh"},
			Resources: []string{"queues", "queues/status", "podgroups"},
			Verbs:     []string{"get", "list", "watch"},
		},
	}

	if n.Spec.Scheduler.Type == SchedulerTypeVolcano {
		params.Rules = append(params.Rules, volcanoRules...)
	}

	return params
}

// GetRoleBindingParams returns params to render RoleBinding from templates
func (n *NemoCustomizer) GetRoleBindingParams() *rendertypes.RoleBindingParams {
	params := &rendertypes.RoleBindingParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()

	params.ServiceAccountName = n.GetServiceAccountName()
	params.RoleName = n.GetName()
	return params
}

// GetHPAParams returns params to render HPA from templates
func (n *NemoCustomizer) GetHPAParams() *rendertypes.HPAParams {
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
func (n *NemoCustomizer) GetSCCParams() *rendertypes.SCCParams {
	params := &rendertypes.SCCParams{}
	// Set metadata
	params.Name = "nemo-customizer-scc"

	params.ServiceAccountName = n.GetServiceAccountName()
	return params
}

// GetServiceMonitorParams return params to render Service Monitor from templates
func (n *NemoCustomizer) GetServiceMonitorParams() *rendertypes.ServiceMonitorParams {
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
				Port:          DefaultNamedPortAPI,
				ScrapeTimeout: serviceMonitor.ScrapeTimeout,
				Interval:      serviceMonitor.Interval,
			},
		},
	}
	params.SMSpec = smSpec
	return params
}

// GetServicePort returns the service port for the NemoCustomizer deployment or default port
func (n *NemoCustomizer) GetServicePort() int32 {
	if n.Spec.Expose.Service.Port == nil {
		return DefaultAPIPort
	}

	return *n.Spec.Expose.Service.Port
}

// GetIngressAnnotations return standard and customized ingress annotations
func (n *NemoCustomizer) GetIngressAnnotations() map[string]string {
	NemoCustomizerAnnotations := n.GetNemoCustomizerAnnotations()

	if n.Spec.Expose.Ingress.Annotations != nil {
		return utils.MergeMaps(NemoCustomizerAnnotations, n.Spec.Expose.Ingress.Annotations)
	}
	return NemoCustomizerAnnotations
}

// GetServiceAnnotations return standard and customized service annotations
func (n *NemoCustomizer) GetServiceAnnotations() map[string]string {
	NemoCustomizerAnnotations := n.GetNemoCustomizerAnnotations()

	if n.Spec.Expose.Service.Annotations != nil {
		return utils.MergeMaps(NemoCustomizerAnnotations, n.Spec.Expose.Service.Annotations)
	}
	return NemoCustomizerAnnotations
}

// GetHPAAnnotations return standard and customized hpa annotations
func (n *NemoCustomizer) GetHPAAnnotations() map[string]string {
	NemoCustomizerAnnotations := n.GetNemoCustomizerAnnotations()

	if n.Spec.Scale.Annotations != nil {
		return utils.MergeMaps(NemoCustomizerAnnotations, n.Spec.Scale.Annotations)
	}
	return NemoCustomizerAnnotations
}

// GetServiceMonitorAnnotations return standard and customized servicemonitor annotations
func (n *NemoCustomizer) GetServiceMonitorAnnotations() map[string]string {
	NemoCustomizerAnnotations := n.GetNemoCustomizerAnnotations()

	if n.Spec.Metrics.ServiceMonitor.Annotations != nil {
		return utils.MergeMaps(NemoCustomizerAnnotations, n.Spec.Metrics.ServiceMonitor.Annotations)
	}
	return NemoCustomizerAnnotations
}

// GetConfigMapParams returns params to render NemoCustomizer config from templates
func (n *NemoCustomizer) GetConfigMapParams() *rendertypes.ConfigMapParams {
	params := &rendertypes.ConfigMapParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetLabels()
	params.Annotations = n.GetAnnotations()

	// Initialize the ConfigMap data
	params.ConfigMapData = map[string]string{
		"config.yaml": n.Spec.CustomizerConfig,
	}

	return params
}

func (n *NemoCustomizer) GetSecretParams(secretMapData map[string]string) *rendertypes.SecretParams {
	params := &rendertypes.SecretParams{}

	// Set metadata
	params.Name = n.Name
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetLabels()
	params.Annotations = n.GetAnnotations()

	params.SecretMapData = secretMapData

	return params
}

func init() {
	SchemeBuilder.Register(&NemoCustomizer{}, &NemoCustomizerList{})
}
