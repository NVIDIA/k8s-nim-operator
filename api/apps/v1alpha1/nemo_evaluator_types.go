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
	"strconv"
	"strings"

	rendertypes "github.com/NVIDIA/k8s-nim-operator/internal/render/types"
	utils "github.com/NVIDIA/k8s-nim-operator/internal/utils"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// EvaluatorAPIPort is the default port that the evaluator serves on
	EvaluatorAPIPort = 7331
	// NemoEvaluatorConditionReady indicates that the NEMO EvaluatorService is ready.
	NemoEvaluatorConditionReady = "Ready"
	// NemoEvaluatorConditionFailed indicates that the NEMO EvaluatorService has failed.
	NemoEvaluatorConditionFailed = "Failed"

	// NemoEvaluatorStatusPending indicates that NEMO EvaluatorService is in pending state
	NemoEvaluatorStatusPending = "Pending"
	// NemoEvaluatorStatusNotReady indicates that NEMO EvaluatorService is not ready
	NemoEvaluatorStatusNotReady = "NotReady"
	// NemoEvaluatorStatusReady indicates that NEMO EvaluatorService is ready
	NemoEvaluatorStatusReady = "Ready"
	// NemoEvaluatorStatusFailed indicates that NEMO EvaluatorService has failed
	NemoEvaluatorStatusFailed = "Failed"
)

// NemoEvaluatorSpec defines the desired state of NemoEvaluator
type NemoEvaluatorSpec struct {
	Image   Image           `json:"image"`
	Command []string        `json:"command,omitempty"`
	Args    []string        `json:"args,omitempty"`
	Env     []corev1.EnvVar `json:"env,omitempty"`
	// The name of an secret that contains authn for the NGC NIM service API
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

	// DatabaseConfig stores the database configuration for NeMo entitystore.
	DatabaseConfig *DatabaseConfig `json:"databaseConfig"`
	// ArgoWorkflows stores the argo workflow service endpoint.
	ArgoWorkflows ArgoWorkflows `json:"argoWorkflows"`
	// VectorDB stores the vector db endpoint.
	VectorDB VectorDB `json:"vectorDB"`
	// Datastore stores the datastore endpoint.
	Datastore Datastore `json:"datastore"`
	// Entitystore stores the entitystore endpoint.
	Entitystore Entitystore `json:"entitystore"`
	// OpenTelemetry Settings
	// +kubebuilder:validation:Optional
	OpenTelemetry OTelSpec `json:"otel,omitempty"`

	// EvalLogLevel defines the evaluator log level (e.g., INFO, DEBUG).
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=INFO;DEBUG
	// +kubebuilder:default="INFO"
	EvalLogLevel string `json:"evalLogLevel,omitempty"`

	// LogHandlers defines the log sink handlers (e.g., INFO, DEBUG).
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=console;file
	// +kubebuilder:default="console"
	LogHandlers string `json:"logHandlers,omitempty"`

	// ConsoleLogLevel defines the console log level (e.g., INFO, DEBUG).
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=INFO;DEBUG
	// +kubebuilder:default="INFO"
	ConsoleLogLevel string `json:"consoleLogLevel,omitempty"`

	// EnableValidation indicates that the validation jobs to be enabled
	EnableValidation *bool `json:"enableValidation,omitempty"`

	// EvaluationImages defines the external images used for evaluation
	EvaluationImages EvaluationImages `json:"evaluationImages"`
}

type EvaluationImages struct {
	// +kubebuilder:validation:MinLength=1
	BigcodeEvalHarness string `json:"bigcodeEvalHarness"`
	// +kubebuilder:validation:MinLength=1
	LmEvalHarness string `json:"lmEvalHarness"`
	// +kubebuilder:validation:MinLength=1
	SimilarityMetrics string `json:"similarityMetrics"`
	// +kubebuilder:validation:MinLength=1
	LlmAsJudge string `json:"llmAsJudge"`
	// +kubebuilder:validation:MinLength=1
	MtBench string `json:"mtBench"`
	// +kubebuilder:validation:MinLength=1
	Retriever string `json:"retriever"`
	// +kubebuilder:validation:MinLength=1
	Rag string `json:"rag"`
}

// NemoEvaluatorStatus defines the observed state of NemoEvaluator
type NemoEvaluatorStatus struct {
	Conditions        []metav1.Condition `json:"conditions,omitempty"`
	AvailableReplicas int32              `json:"availableReplicas,omitempty"`
	State             string             `json:"state,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.state`,priority=0
// +kubebuilder:printcolumn:name="Age",type="date",format="date-time",JSONPath=".metadata.creationTimestamp",priority=0

// NemoEvaluator is the Schema for the NemoEvaluator API
type NemoEvaluator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NemoEvaluatorSpec   `json:"spec,omitempty"`
	Status NemoEvaluatorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NemoEvaluatorList contains a list of NemoEvaluator
type NemoEvaluatorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NemoEvaluator `json:"items"`
}

func (ei EvaluationImages) GetEvaluationImageEnv() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "BIGCODE_EVALUATION_HARNESS",
			Value: ei.BigcodeEvalHarness,
		},
		{
			Name:  "LM_EVAL_HARNESS",
			Value: ei.LmEvalHarness,
		},
		{
			Name:  "SIMILARITY_METRICS",
			Value: ei.SimilarityMetrics,
		},
		{
			Name:  "LLM_AS_A_JUDGE",
			Value: ei.LlmAsJudge,
		},
		{
			Name:  "MT_BENCH",
			Value: ei.MtBench,
		},
		{
			Name:  "RETRIEVER",
			Value: ei.Retriever,
		},
		{
			Name:  "RAG",
			Value: ei.Rag,
		},
	}
}

// GetStandardSelectorLabels returns the standard selector labels for the NemoEvaluator deployment
func (n *NemoEvaluator) GetStandardSelectorLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name": n.Name,
	}
}

// GetStandardLabels returns the standard set of labels for NemoEvaluator resources
func (n *NemoEvaluator) GetStandardLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":             n.Name,
		"app.kubernetes.io/instance":         n.Name,
		"app.kubernetes.io/operator-version": os.Getenv("OPERATOR_VERSION"),
		"app.kubernetes.io/part-of":          "nemo-evaluator-service",
		"app.kubernetes.io/managed-by":       "k8s-nim-operator",
	}
}

// GetStandardEnv returns the standard set of env variables for the NemoEvaluator container
func (n *NemoEvaluator) GetStandardEnv() []corev1.EnvVar {
	// add standard env required for NIM service

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
			Name:  "EVALUATOR_HOST",
			Value: "0.0.0.0",
		},
		{
			Name:  "EVALUATOR_PORT",
			Value: fmt.Sprintf("%d", EvaluatorAPIPort),
		},
		{
			Name:  "ARGO_HOST",
			Value: n.Spec.ArgoWorkflows.Endpoint,
		},
		{
			Name:  "MILVUS_URL",
			Value: n.Spec.VectorDB.Endpoint,
		},
		{
			Name:  "SERVICE_ACCOUNT",
			Value: n.Spec.ArgoWorkflows.ServiceAccount,
		},
		{
			Name:  "DATA_STORE_URL",
			Value: n.Spec.Datastore.Endpoint,
		},
		{
			Name:  "ENTITY_STORE_URL",
			Value: n.Spec.Entitystore.Endpoint,
		},
		{
			Name:  "EVAL_CONTAINER",
			Value: n.GetImage(),
		},
		{
			Name:  "LOG_HANDLERS",
			Value: n.Spec.LogHandlers,
		},
		{
			Name:  "CONSOLE_LOG_LEVEL",
			Value: n.Spec.ConsoleLogLevel,
		},
		{
			Name:  "EVAL_LOG_LEVEL",
			Value: n.Spec.EvalLogLevel,
		},
	}

	if n.IsValidationEnabled() {
		envVars = append(envVars,
			corev1.EnvVar{Name: "EVAL_ENABLE_VALIDATION", Value: "True"})
	}

	// Append the environment variables for Postgres
	envVars = append(envVars, n.GetPostgresEnv()...)

	// Append the environment variables for EvaluationImages
	envVars = append(envVars, n.Spec.EvaluationImages.GetEvaluationImageEnv()...)

	// Append the environment variables for OTel
	envVars = append(envVars, n.GetOtelEnv()...)

	return envVars
}

// IsValidationEnabled returns if the validation jobs are enabled by default
func (n *NemoEvaluator) IsValidationEnabled() bool {
	if n.Spec.EnableValidation == nil {
		// validation jobs are enabled by default
		return true
	}
	return *n.Spec.EnableValidation
}

// IsOtelEnabled returns true if Open Telemetry Collector is enabled
func (n *NemoEvaluator) IsOtelEnabled() bool {
	return n.Spec.OpenTelemetry.Enabled != nil && *n.Spec.OpenTelemetry.Enabled
}

// GetOtelEnv generates OpenTelemetry-related environment variables.
func (n *NemoEvaluator) GetOtelEnv() []corev1.EnvVar {
	if !n.IsOtelEnabled() {
		return []corev1.EnvVar{
			{
				Name:  "OTEL_SDK_DISABLED",
				Value: "true",
			},
		}
	}

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
func (n *NemoEvaluator) GetPostgresEnv() []corev1.EnvVar {
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
			Name: "POSTGRES_URI",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: "uri",
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
func (n *NemoEvaluator) GeneratePostgresConnString(secretValue string) string {
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

// GetStandardAnnotations returns default annotations to apply to the NemoEvaluator instance
func (n *NemoEvaluator) GetStandardAnnotations() map[string]string {
	standardAnnotations := map[string]string{
		"openshift.io/scc":                      "nonroot",
		utils.NvidiaAnnotationParentSpecHashKey: utils.DeepHashObject(n.Spec),
	}
	return standardAnnotations
}

// GetNemoEvaluatorAnnotations returns annotations to apply to the NemoEvaluator instance
func (n *NemoEvaluator) GetNemoEvaluatorAnnotations() map[string]string {
	standardAnnotations := n.GetStandardAnnotations()

	if n.Spec.Annotations != nil {
		return utils.MergeMaps(standardAnnotations, n.Spec.Annotations)
	}

	return standardAnnotations
}

// GetServiceLabels returns merged labels to apply to the NemoEvaluator instance
func (n *NemoEvaluator) GetServiceLabels() map[string]string {
	standardLabels := n.GetStandardLabels()

	if n.Spec.Labels != nil {
		return utils.MergeMaps(standardLabels, n.Spec.Labels)
	}
	return standardLabels
}

// GetSelectorLabels returns standard selector labels to apply to the NemoEvaluator instance
func (n *NemoEvaluator) GetSelectorLabels() map[string]string {
	// TODO: add custom ones
	return n.GetStandardSelectorLabels()
}

// GetNodeSelector returns node selector labels for the NemoEvaluator instance
func (n *NemoEvaluator) GetNodeSelector() map[string]string {
	return n.Spec.NodeSelector
}

// GetTolerations returns tolerations for the NemoEvaluator instance
func (n *NemoEvaluator) GetTolerations() []corev1.Toleration {
	return n.Spec.Tolerations
}

// GetPodAffinity returns pod affinity for the NemoEvaluator instance
func (n *NemoEvaluator) GetPodAffinity() *corev1.PodAffinity {
	return n.Spec.PodAffinity
}

// GetContainerName returns name of the container for NemoEvaluator deployment
func (n *NemoEvaluator) GetContainerName() string {
	return fmt.Sprintf("%s-ctr", n.Name)
}

// GetCommand return command to override for the NemoEvaluator container
func (n *NemoEvaluator) GetCommand() []string {
	return n.Spec.Command
}

// GetArgs return arguments for the NemoEvaluator container
func (n *NemoEvaluator) GetArgs() []string {
	return n.Spec.Args
}

// GetEnv returns merged slice of standard and user specified env variables
func (n *NemoEvaluator) GetEnv() []corev1.EnvVar {
	return utils.MergeEnvVars(n.GetStandardEnv(), n.Spec.Env)
}

// GetImage returns container image for the NemoEvaluator
func (n *NemoEvaluator) GetImage() string {
	return fmt.Sprintf("%s:%s", n.Spec.Image.Repository, n.Spec.Image.Tag)
}

// GetImagePullSecrets returns the image pull secrets for the NIM container
func (n *NemoEvaluator) GetImagePullSecrets() []string {
	return n.Spec.Image.PullSecrets
}

// GetImagePullPolicy returns the image pull policy for the NIM container
func (n *NemoEvaluator) GetImagePullPolicy() string {
	return n.Spec.Image.PullPolicy
}

// GetResources returns resources to allocate to the NemoEvaluator container
func (n *NemoEvaluator) GetResources() *corev1.ResourceRequirements {
	return n.Spec.Resources
}

// GetLivenessProbe returns liveness probe for the NemoEvaluator container
func (n *NemoEvaluator) GetLivenessProbe() *corev1.Probe {
	if n.Spec.LivenessProbe.Probe == nil {
		return n.GetDefaultLivenessProbe()
	}
	return n.Spec.LivenessProbe.Probe
}

// GetDefaultLivenessProbe returns the default liveness probe for the NemoEvaluator container
func (n *NemoEvaluator) GetDefaultLivenessProbe() *corev1.Probe {
	probe := corev1.Probe{
		FailureThreshold: 3,
		PeriodSeconds:    10,
		SuccessThreshold: 1,
		TimeoutSeconds:   1,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/health",
				Port: intstr.FromString(DefaultNamedPortAPI),
			},
		},
	}
	return &probe
}

// GetReadinessProbe returns readiness probe for the NemoEvaluator container
func (n *NemoEvaluator) GetReadinessProbe() *corev1.Probe {
	if n.Spec.ReadinessProbe.Probe == nil {
		return n.GetDefaultReadinessProbe()
	}
	return n.Spec.ReadinessProbe.Probe
}

// GetDefaultReadinessProbe returns the default readiness probe for the NemoEvaluator container
func (n *NemoEvaluator) GetDefaultReadinessProbe() *corev1.Probe {
	probe := corev1.Probe{
		FailureThreshold: 3,
		PeriodSeconds:    10,
		SuccessThreshold: 1,
		TimeoutSeconds:   1,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/health",
				Port: intstr.FromString(DefaultNamedPortAPI),
			},
		},
	}

	return &probe
}

// GetStartupProbe returns startup probe for the NemoEvaluator container
func (n *NemoEvaluator) GetStartupProbe() *corev1.Probe {
	if n.Spec.StartupProbe.Probe == nil {
		return n.GetDefaultStartupProbe()
	}
	return n.Spec.StartupProbe.Probe
}

// GetDefaultStartupProbe returns the default startup probe for the NemoEntitystore container
func (n *NemoEvaluator) GetDefaultStartupProbe() *corev1.Probe {
	probe := corev1.Probe{
		InitialDelaySeconds: 30,
		TimeoutSeconds:      1,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		FailureThreshold:    30,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/health",
				Port: intstr.FromString(DefaultNamedPortAPI),
			},
		},
	}

	return &probe
}

// GetServiceAccountName returns service account name for the NemoEvaluator deployment
func (n *NemoEvaluator) GetServiceAccountName() string {
	return n.Name
}

// GetRuntimeClass return the runtime class name for the NemoEvaluator deployment
func (n *NemoEvaluator) GetRuntimeClass() string {
	return n.Spec.RuntimeClass
}

// GetHPA returns the HPA spec for the NemoEvaluator deployment
func (n *NemoEvaluator) GetHPA() HorizontalPodAutoscalerSpec {
	return n.Spec.Scale.HPA
}

// GetServiceMonitor returns the Service Monitor details for the NemoEvaluator deployment
func (n *NemoEvaluator) GetServiceMonitor() ServiceMonitor {
	return n.Spec.Metrics.ServiceMonitor
}

// GetReplicas returns replicas for the NemoEvaluator deployment
func (n *NemoEvaluator) GetReplicas() int {
	if n.IsAutoScalingEnabled() {
		return 0
	}
	return n.Spec.Replicas
}

// GetDeploymentKind returns the kind of deployment for NemoEvaluator
func (n *NemoEvaluator) GetDeploymentKind() string {
	return "Deployment"
}

// IsAutoScalingEnabled returns true if autoscaling is enabled for NemoEvaluator deployment
func (n *NemoEvaluator) IsAutoScalingEnabled() bool {
	return n.Spec.Scale.Enabled != nil && *n.Spec.Scale.Enabled
}

// IsIngressEnabled returns true if ingress is enabled for NemoEvaluator deployment
func (n *NemoEvaluator) IsIngressEnabled() bool {
	return n.Spec.Expose.Ingress.Enabled != nil && *n.Spec.Expose.Ingress.Enabled
}

// GetIngressSpec returns the Ingress spec NemoEvaluator deployment
func (n *NemoEvaluator) GetIngressSpec() networkingv1.IngressSpec {
	return n.Spec.Expose.Ingress.Spec
}

// IsServiceMonitorEnabled returns true if servicemonitor is enabled for NemoEvaluator deployment
func (n *NemoEvaluator) IsServiceMonitorEnabled() bool {
	return n.Spec.Metrics.Enabled != nil && *n.Spec.Metrics.Enabled
}

// GetServicePort returns the service port for the NemoEvaluator deployment or default port
func (n *NemoEvaluator) GetServicePort() int32 {
	if n.Spec.Expose.Service.Port == nil {
		return DefaultAPIPort
	}
	return *n.Spec.Expose.Service.Port
}

// GetServiceType returns the service type for the NemoEvaluator deployment
func (n *NemoEvaluator) GetServiceType() string {
	return string(n.Spec.Expose.Service.Type)
}

// GetUserID returns the user ID for the NemoEvaluator deployment
func (n *NemoEvaluator) GetUserID() *int64 {
	return n.Spec.UserID

}

// GetGroupID returns the group ID for the NemoEvaluator deployment
func (n *NemoEvaluator) GetGroupID() *int64 {
	return n.Spec.GroupID

}

// GetServiceAccountParams return params to render ServiceAccount from templates
func (n *NemoEvaluator) GetServiceAccountParams() *rendertypes.ServiceAccountParams {
	params := &rendertypes.ServiceAccountParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNemoEvaluatorAnnotations()
	return params
}

// GetDeploymentParams returns params to render Deployment from templates
func (n *NemoEvaluator) GetDeploymentParams() *rendertypes.DeploymentParams {
	params := &rendertypes.DeploymentParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNemoEvaluatorAnnotations()
	params.PodAnnotations = n.GetNemoEvaluatorAnnotations()
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

	// Setup container ports for evaluator
	params.Ports = []corev1.ContainerPort{
		{
			Name:          DefaultNamedPortAPI,
			Protocol:      corev1.ProtocolTCP,
			ContainerPort: EvaluatorAPIPort,
		},
	}

	return params
}

// GetStatefulSetParams returns params to render StatefulSet from templates
func (n *NemoEvaluator) GetStatefulSetParams() *rendertypes.StatefulSetParams {

	params := &rendertypes.StatefulSetParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNemoEvaluatorAnnotations()

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
func (n *NemoEvaluator) GetServiceParams() *rendertypes.ServiceParams {
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

	return params
}

// GetIngressParams returns params to render Ingress from templates
func (n *NemoEvaluator) GetIngressParams() *rendertypes.IngressParams {
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
func (n *NemoEvaluator) GetRoleParams() *rendertypes.RoleParams {
	params := &rendertypes.RoleParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()

	// Set rules to use SCC
	params.Rules = []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"secrets", "pods"},
			Verbs:     []string{"get", "watch", "list", "create"},
		},
	}

	return params
}

// GetRoleBindingParams returns params to render RoleBinding from templates
func (n *NemoEvaluator) GetRoleBindingParams() *rendertypes.RoleBindingParams {
	params := &rendertypes.RoleBindingParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()

	params.ServiceAccountName = n.GetServiceAccountName()
	params.RoleName = n.GetName()
	return params
}

// GetHPAParams returns params to render HPA from templates
func (n *NemoEvaluator) GetHPAParams() *rendertypes.HPAParams {
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
func (n *NemoEvaluator) GetSCCParams() *rendertypes.SCCParams {
	params := &rendertypes.SCCParams{}
	// Set metadata
	params.Name = "nemo-evaluators-scc"

	params.ServiceAccountName = n.GetServiceAccountName()
	return params
}

// GetServiceMonitorParams return params to render Service Monitor from templates
func (n *NemoEvaluator) GetServiceMonitorParams() *rendertypes.ServiceMonitorParams {
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

func (n *NemoEvaluator) GetIngressAnnotations() map[string]string {
	NemoEvaluatorAnnotations := n.GetNemoEvaluatorAnnotations()

	if n.Spec.Expose.Ingress.Annotations != nil {
		return utils.MergeMaps(NemoEvaluatorAnnotations, n.Spec.Expose.Ingress.Annotations)
	}
	return NemoEvaluatorAnnotations
}

func (n *NemoEvaluator) GetServiceAnnotations() map[string]string {
	NemoEvaluatorAnnotations := n.GetNemoEvaluatorAnnotations()

	if n.Spec.Expose.Service.Annotations != nil {
		return utils.MergeMaps(NemoEvaluatorAnnotations, n.Spec.Expose.Service.Annotations)
	}
	return NemoEvaluatorAnnotations
}

func (n *NemoEvaluator) GetHPAAnnotations() map[string]string {
	NemoEvaluatorAnnotations := n.GetNemoEvaluatorAnnotations()

	if n.Spec.Scale.Annotations != nil {
		return utils.MergeMaps(NemoEvaluatorAnnotations, n.Spec.Scale.Annotations)
	}
	return NemoEvaluatorAnnotations
}

func (n *NemoEvaluator) GetServiceMonitorAnnotations() map[string]string {
	NemoEvaluatorAnnotations := n.GetNemoEvaluatorAnnotations()

	if n.Spec.Metrics.ServiceMonitor.Annotations != nil {
		return utils.MergeMaps(NemoEvaluatorAnnotations, n.Spec.Metrics.ServiceMonitor.Annotations)
	}
	return NemoEvaluatorAnnotations
}

func (n *NemoEvaluator) GetSecretParams(secretMapData map[string]string) *rendertypes.SecretParams {
	params := &rendertypes.SecretParams{}

	// Set metadata
	params.Name = n.Name
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetLabels()
	params.Annotations = n.GetAnnotations()

	params.SecretMapData = secretMapData

	return params
}

// GetInitContainers returns the init containers for the NemoEvaluator.
//
// It creates and returns a slice of corev1.Container.
// The init containers include a busybox container to wait for Postgres to start,
// and an evaluator-db-migration container to run the database migration.
//
// Returns a slice of corev1.Container.
func (n *NemoEvaluator) GetInitContainers() []corev1.Container {

	connCmd := fmt.Sprintf(
		"until nc -z %s %d; do echo \"Waiting for Postgres to start \"; sleep 5; done",
		n.Spec.DatabaseConfig.Host,
		n.Spec.DatabaseConfig.Port)

	envVars := []corev1.EnvVar{
		{
			Name:  "NAMESPACE",
			Value: n.Namespace,
		},
		{
			Name:  "ARGO_HOST",
			Value: n.Spec.ArgoWorkflows.Endpoint,
		},
		{
			Name:  "EVAL_CONTAINER",
			Value: n.GetImage(),
		},
		{
			Name:  "DATA_STORE_URL",
			Value: n.Spec.Datastore.Endpoint,
		},
	}
	// Append the environment variables for Postgres
	envVars = append(envVars, n.GetPostgresEnv()...)

	// Append the environment variables for EvaluationImages
	envVars = append(envVars, n.Spec.EvaluationImages.GetEvaluationImageEnv()...)

	return []corev1.Container{
		{
			Name:            "wait-for-postgres",
			Image:           "busybox",
			ImagePullPolicy: corev1.PullPolicy(n.GetImagePullPolicy()),
			Command: []string{
				"sh", "-c", connCmd,
			},
		},
		{
			Name:            "evaluator-db-migration",
			Image:           n.GetImage(),
			ImagePullPolicy: corev1.PullPolicy(n.GetImagePullPolicy()),
			Command: []string{
				"sh", "-c", "/app/scripts/run-db-migration.sh",
			},
			Env: envVars,
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
		},
	}
}

func init() {
	SchemeBuilder.Register(&NemoEvaluator{}, &NemoEvaluatorList{})
}
