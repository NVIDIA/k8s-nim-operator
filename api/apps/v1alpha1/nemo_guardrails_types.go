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

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	rendertypes "github.com/NVIDIA/k8s-nim-operator/internal/render/types"
	utils "github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	// GuardrailAPIPort is the default port that the guardrail serves on.
	GuardrailAPIPort = 8000
	// NemoGuardrailConditionReady indicates that the NEMO GuardrailService is ready.
	NemoGuardrailConditionReady = "Ready"
	// NemoGuardrailConditionFailed indicates that the NEMO GuardrailService has failed.
	NemoGuardrailConditionFailed = "Failed"

	// NemoGuardrailStatusPending indicates that NEMO GuardrailService is in pending state.
	NemoGuardrailStatusPending = "Pending"
	// NemoGuardrailStatusNotReady indicates that NEMO GuardrailService is not ready.
	NemoGuardrailStatusNotReady = "NotReady"
	// NemoGuardrailStatusReady indicates that NEMO GuardrailService is ready.
	NemoGuardrailStatusReady = "Ready"
	// NemoGuardrailStatusFailed indicates that NEMO GuardrailService has failed.
	NemoGuardrailStatusFailed = "Failed"
)

// NemoGuardrailSpec defines the desired state of NemoGuardrail.
// +kubebuilder:validation:XValidation:rule="!(has(self.scale) && has(self.scale.enabled) && self.scale.enabled && has(self.replicas))",message="spec.replicas cannot be set when spec.scale.enabled is true"
type NemoGuardrailSpec struct {
	Image       Image           `json:"image"`
	Command     []string        `json:"command,omitempty"`
	Args        []string        `json:"args,omitempty"`
	Env         []corev1.EnvVar `json:"env,omitempty"`
	NIMEndpoint *NIMEndpoint    `json:"nimEndpoint,omitempty"`
	// ConfigStore stores the config of the guardrail service
	ConfigStore  GuardrailConfig     `json:"configStore,omitempty"`
	Labels       map[string]string   `json:"labels,omitempty"`
	Annotations  map[string]string   `json:"annotations,omitempty"`
	NodeSelector map[string]string   `json:"nodeSelector,omitempty"`
	Tolerations  []corev1.Toleration `json:"tolerations,omitempty"`
	Affinity     *corev1.Affinity    `json:"affinity,omitempty"`
	// Deprecated: Use Affinity instead.
	PodAffinity *corev1.PodAffinity          `json:"podAffinity,omitempty"`
	Resources   *corev1.ResourceRequirements `json:"resources,omitempty"`
	// +kubebuilder:validation:XValidation:rule="!(has(self.service.grpcPort))", message="unsupported field: spec.expose.service.grpcPort"
	// +kubebuilder:validation:XValidation:rule="!(has(self.service.metricsPort))", message="unsupported field: spec.expose.service.metricsPort"
	Expose  ExposeV1    `json:"expose,omitempty"`
	Scale   Autoscaling `json:"scale,omitempty"`
	Metrics Metrics     `json:"metrics,omitempty"`
	// +kubebuilder:validation:Minimum=1
	Replicas     *int32 `json:"replicas,omitempty"`
	UserID       *int64 `json:"userID,omitempty"`
	GroupID      *int64 `json:"groupID,omitempty"`
	RuntimeClass string `json:"runtimeClass,omitempty"`

	// DatabaseConfig stores the metadata for the guardrail service.
	DatabaseConfig *DatabaseConfig `json:"databaseConfig,omitempty"`
}

type NIMEndpoint struct {
	// The base URL for the NIM service. This can either be the endpoint for a single NIM or a NIM proxy.
	// A NIM proxy endpoint is needed if you need to run guardrail for serving multiple NIMs.
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^https?:\/\/[^\s]+\/v1\/?$`
	// +kubebuilder:validation:Format=uri
	BaseURL string `json:"baseURL"`
	// The name of the secret that contains the API key for accessing the base URL endpoint. This is needed if the base URL is for a NIM proxy.
	// When using NVIDIA's hosted NIM proxy `https://integrate.api.nvidia.com/v1` as the base URL, the API key can be retrieved from https://build.nvidia.com/explore/discover
	//
	// +kubebuilder:validation:Optional
	APIKeySecret string `json:"apiKeySecret,omitempty"`
	// The key in the secret that contains the API key for accessing the base URL endpoint. Defaults to `NIM_ENDPOINT_API_KEY`
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:="NIM_ENDPOINT_API_KEY"
	APIKeyKey string `json:"apiKeyKey,omitempty"`
}

// GuardrailConfig defines the source where the service config is made available.
//
// +kubebuilder:validation:XValidation:rule="!(has(self.configMap) && has(self.pvc))", message="Cannot set both ConfigMap and PVC in ConfigStore"
type GuardrailConfig struct {
	ConfigMap *ConfigMapRef          `json:"configMap,omitempty"`
	PVC       *PersistentVolumeClaim `json:"pvc,omitempty"`
}

// NemoGuardrailStatus defines the observed state of NemoGuardrail.
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

// NemoGuardrail is the Schema for the NemoGuardrail API.
type NemoGuardrail struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NemoGuardrailSpec   `json:"spec,omitempty"`
	Status NemoGuardrailStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NemoGuardrailList contains a list of NemoGuardrail.
type NemoGuardrailList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NemoGuardrail `json:"items"`
}

// GetPVCName returns the name to be used for the PVC based on the custom spec
// Prefers pvc.Name if explicitly set by the user in the NemoGuardrail instance.
func (n *NemoGuardrail) GetPVCName(pvc PersistentVolumeClaim) string {
	pvcName := fmt.Sprintf("%s-pvc", n.GetName())
	if pvc.Name != "" {
		pvcName = pvc.Name
	}
	return pvcName
}

// GetStandardSelectorLabels returns the standard selector labels for the NemoGuardrail deployment.
func (n *NemoGuardrail) GetStandardSelectorLabels() map[string]string {
	return map[string]string{
		"app":                        n.Name,
		"app.kubernetes.io/name":     n.Name,
		"app.kubernetes.io/instance": n.Name,
	}
}

// GetStandardLabels returns the standard set of labels for NemoGuardrail resources.
func (n *NemoGuardrail) GetStandardLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":             n.Name,
		"app.kubernetes.io/instance":         n.Name,
		"app.kubernetes.io/operator-version": os.Getenv("OPERATOR_VERSION"),
		"app.kubernetes.io/part-of":          "nemo-guardrail-service",
		"app.kubernetes.io/managed-by":       "k8s-nim-operator",
	}
}

// GetStandardEnv returns the standard set of env variables for the NemoGuardrail container.
func (n *NemoGuardrail) GetStandardEnv() []corev1.EnvVar {
	// add standard env required for NIM service
	envVars := []corev1.EnvVar{
		{
			Name:  "CONFIG_STORE_PATH",
			Value: "/config-store",
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
			Value: fmt.Sprintf("%d", GuardrailAPIPort),
		},
		{
			Name:  "DEMO",
			Value: "False",
		},
	}

	if n.Spec.NIMEndpoint != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "NIM_ENDPOINT_URL",
			Value: n.Spec.NIMEndpoint.BaseURL,
		})
		if len(n.Spec.NIMEndpoint.APIKeySecret) > 0 {
			envVars = append(envVars, corev1.EnvVar{
				Name: "NIM_ENDPOINT_API_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						Key: n.Spec.NIMEndpoint.APIKeyKey,
						LocalObjectReference: corev1.LocalObjectReference{
							Name: n.Spec.NIMEndpoint.APIKeySecret,
						},
					},
				},
			})
		}
	}

	if n.Spec.DatabaseConfig != nil {
		// Append the environment variables for Postgres
		envVars = append(envVars, n.GetPostgresEnv()...)
	}
	return envVars
}

// GetPostgresEnv returns the PostgreSQL environment variables for a Kubernetes pod.
func (n *NemoGuardrail) GetPostgresEnv() []corev1.EnvVar {
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
		{
			Name: "DB_URI", // guardrails requires this env for external DB
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
func (n *NemoGuardrail) GeneratePostgresConnString(secretValue string) string {
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

func (n *NemoGuardrail) GetSecretParams(secretMapData map[string]string) *rendertypes.SecretParams {
	params := &rendertypes.SecretParams{}

	// Set metadata
	params.Name = n.Name
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetLabels()
	params.Annotations = n.GetAnnotations()

	params.SecretMapData = secretMapData

	return params
}

// GetStandardAnnotations returns default annotations to apply to the NemoGuardrail instance.
func (n *NemoGuardrail) GetStandardAnnotations() map[string]string {
	standardAnnotations := map[string]string{
		"openshift.io/required-scc":             "nonroot",
		utils.NvidiaAnnotationParentSpecHashKey: utils.DeepHashObject(n.Spec),
	}
	return standardAnnotations
}

// GetNemoGuardrailAnnotations returns annotations to apply to the NemoGuardrail instance.
func (n *NemoGuardrail) GetNemoGuardrailAnnotations() map[string]string {
	standardAnnotations := n.GetStandardAnnotations()

	if n.Spec.Annotations != nil {
		return utils.MergeMaps(standardAnnotations, n.Spec.Annotations)
	}

	return standardAnnotations
}

// GetServiceLabels returns merged labels to apply to the NemoGuardrail instance.
func (n *NemoGuardrail) GetServiceLabels() map[string]string {
	standardLabels := n.GetStandardLabels()

	if n.Spec.Labels != nil {
		return utils.MergeMaps(standardLabels, n.Spec.Labels)
	}
	return standardLabels
}

// GetSelectorLabels returns standard selector labels to apply to the NemoGuardrail instance.
func (n *NemoGuardrail) GetSelectorLabels() map[string]string {
	// TODO: add custom ones
	return n.GetStandardSelectorLabels()
}

// GetNodeSelector returns node selector labels for the NemoGuardrail instance.
func (n *NemoGuardrail) GetNodeSelector() map[string]string {
	return n.Spec.NodeSelector
}

// GetTolerations returns tolerations for the NemoGuardrail instance.
func (n *NemoGuardrail) GetTolerations() []corev1.Toleration {
	return n.Spec.Tolerations
}

// GetAffinity returns affinity for the NemoGuardrail instance.
func (n *NemoGuardrail) GetAffinity() *corev1.Affinity {
	return n.Spec.Affinity
}

// GetContainerName returns name of the container for NemoGuardrail deployment.
func (n *NemoGuardrail) GetContainerName() string {
	return fmt.Sprintf("%s-ctr", n.Name)
}

// GetCommand return command to override for the NemoGuardrail container.
func (n *NemoGuardrail) GetCommand() []string {
	return n.Spec.Command
}

// GetArgs return arguments for the NemoGuardrail container.
func (n *NemoGuardrail) GetArgs() []string {
	return n.Spec.Args
}

// GetEnv returns merged slice of standard and user specified env variables.
func (n *NemoGuardrail) GetEnv() []corev1.EnvVar {
	return utils.MergeEnvVars(n.GetStandardEnv(), n.Spec.Env)
}

// GetImage returns container image for the NemoGuardrail.
func (n *NemoGuardrail) GetImage() string {
	return fmt.Sprintf("%s:%s", n.Spec.Image.Repository, n.Spec.Image.Tag)
}

// GetImagePullSecrets returns the image pull secrets for the NIM container.
func (n *NemoGuardrail) GetImagePullSecrets() []string {
	return n.Spec.Image.PullSecrets
}

// GetImagePullPolicy returns the image pull policy for the NIM container.
func (n *NemoGuardrail) GetImagePullPolicy() string {
	return n.Spec.Image.PullPolicy
}

// GetResources returns resources to allocate to the NemoGuardrail container.
func (n *NemoGuardrail) GetResources() *corev1.ResourceRequirements {
	return n.Spec.Resources
}

// GetLivenessProbe returns liveness probe for the NemoGuardrail container.
func (n *NemoGuardrail) GetLivenessProbe() *corev1.Probe {
	return &corev1.Probe{
		InitialDelaySeconds: 15,
		TimeoutSeconds:      1,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		FailureThreshold:    3,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/v1/health",
				Port: intstr.FromString(DefaultNamedPortAPI),
			},
		},
	}
}

// GetReadinessProbe returns readiness probe for the NemoGuardrail container.
func (n *NemoGuardrail) GetReadinessProbe() *corev1.Probe {
	return &corev1.Probe{
		InitialDelaySeconds: 15,
		TimeoutSeconds:      1,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		FailureThreshold:    3,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/v1/health",
				Port: intstr.FromString(DefaultNamedPortAPI),
			},
		},
	}
}

// GetStartupProbe returns startup probe for the NemoGuardrail container.
func (n *NemoGuardrail) GetStartupProbe() *corev1.Probe {
	return &corev1.Probe{
		InitialDelaySeconds: 30,
		TimeoutSeconds:      1,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		FailureThreshold:    30,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/v1/health",
				Port: intstr.FromString(DefaultNamedPortAPI),
			},
		},
	}
}

// GetVolumes returns volumes for the NemoGuardrail container.
func (n *NemoGuardrail) GetVolumes() []corev1.Volume {
	volumes := []corev1.Volume{}
	if n.Spec.ConfigStore.ConfigMap != nil {
		volumes = append(volumes, corev1.Volume{
			Name: "config-store",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: n.Spec.ConfigStore.ConfigMap.Name,
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

// GetVolumeMounts returns volumes for the NemoGuardrail container.
func (n *NemoGuardrail) GetVolumeMounts() []corev1.VolumeMount {
	volumeMount := corev1.VolumeMount{
		Name:      "config-store",
		MountPath: "/config-store",
	}
	if n.Spec.ConfigStore.PVC != nil {
		volumeMount.MountPath = "/config-store"
		subPath := n.Spec.ConfigStore.PVC.SubPath
		if subPath == "" {
			subPath = "guardrails-config-store"
		}
		volumeMount.SubPath = subPath
	}

	return []corev1.VolumeMount{volumeMount}
}

// GetServiceAccountName returns service account name for the NemoGuardrail deployment.
func (n *NemoGuardrail) GetServiceAccountName() string {
	return n.Name
}

// GetRuntimeClass return the runtime class name for the NemoGuardrail deployment.
func (n *NemoGuardrail) GetRuntimeClass() string {
	return n.Spec.RuntimeClass
}

// GetHPA returns the HPA spec for the NemoGuardrail deployment.
func (n *NemoGuardrail) GetHPA() HorizontalPodAutoscalerSpec {
	return n.Spec.Scale.HPA
}

// GetServiceMonitor returns the Service Monitor details for the NemoGuardrail deployment.
func (n *NemoGuardrail) GetServiceMonitor() ServiceMonitor {
	return n.Spec.Metrics.ServiceMonitor
}

// GetReplicas returns replicas for the NemoGuardrail deployment.
func (n *NemoGuardrail) GetReplicas() *int32 {
	if n.IsAutoScalingEnabled() {
		return n.Spec.Scale.HPA.MinReplicas
	}
	return n.Spec.Replicas
}

// GetDeploymentKind returns the kind of deployment for NemoGuardrail.
func (n *NemoGuardrail) GetDeploymentKind() string {
	return "Deployment"
}

// IsAutoScalingEnabled returns true if autoscaling is enabled for NemoGuardrail deployment.
func (n *NemoGuardrail) IsAutoScalingEnabled() bool {
	return n.Spec.Scale.Enabled != nil && *n.Spec.Scale.Enabled
}

// IsIngressEnabled returns true if ingress is enabled for NemoGuardrail deployment.
func (n *NemoGuardrail) IsIngressEnabled() bool {
	return n.Spec.Expose.Ingress.Enabled != nil && *n.Spec.Expose.Ingress.Enabled
}

// GetIngressSpec returns the Ingress spec NemoGuardrail deployment.
func (n *NemoGuardrail) GetIngressSpec() networkingv1.IngressSpec {
	return n.Spec.Expose.Ingress.GenerateNetworkingV1IngressSpec(n.GetName())
}

// IsServiceMonitorEnabled returns true if servicemonitor is enabled for NemoGuardrail deployment.
func (n *NemoGuardrail) IsServiceMonitorEnabled() bool {
	return n.Spec.Metrics.Enabled != nil && *n.Spec.Metrics.Enabled
}

// GetServicePort returns the service port for the NemoGuardrail deployment or default port.
func (n *NemoGuardrail) GetServicePort() int32 {
	if n.Spec.Expose.Service.Port == nil {
		return DefaultAPIPort
	}
	return *n.Spec.Expose.Service.Port
}

// GetServiceType returns the service type for the NemoGuardrail deployment.
func (n *NemoGuardrail) GetServiceType() string {
	return string(n.Spec.Expose.Service.Type)
}

// GetUserID returns the user ID for the NemoGuardrail deployment.
func (n *NemoGuardrail) GetUserID() *int64 {
	if n.Spec.UserID != nil {
		return n.Spec.UserID
	}
	return ptr.To[int64](1000)
}

// GetGroupID returns the group ID for the NemoGuardrail deployment.
func (n *NemoGuardrail) GetGroupID() *int64 {
	if n.Spec.GroupID != nil {
		return n.Spec.GroupID
	}
	return ptr.To[int64](2000)
}

// GetServiceAccountParams return params to render ServiceAccount from templates.
func (n *NemoGuardrail) GetServiceAccountParams() *rendertypes.ServiceAccountParams {
	params := &rendertypes.ServiceAccountParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNemoGuardrailAnnotations()
	return params
}

// GetDeploymentParams returns params to render Deployment from templates.
func (n *NemoGuardrail) GetDeploymentParams() *rendertypes.DeploymentParams {
	params := &rendertypes.DeploymentParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNemoGuardrailAnnotations()
	params.PodAnnotations = n.GetNemoGuardrailAnnotations()
	delete(params.PodAnnotations, utils.NvidiaAnnotationParentSpecHashKey)

	// Set template spec
	if !n.IsAutoScalingEnabled() {
		params.Replicas = n.GetReplicas()
	}
	params.NodeSelector = n.GetNodeSelector()
	params.Tolerations = n.GetTolerations()
	params.Affinity = n.GetAffinity()
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

	// Set security context
	params.UserID = n.GetUserID()
	params.GroupID = n.GetGroupID()

	// Set service account
	params.ServiceAccountName = n.GetServiceAccountName()

	// Set runtime class
	params.RuntimeClassName = n.GetRuntimeClass()

	// Setup container ports for guardrail
	params.Ports = []corev1.ContainerPort{
		{
			Name:          DefaultNamedPortAPI,
			Protocol:      corev1.ProtocolTCP,
			ContainerPort: GuardrailAPIPort,
		},
	}

	return params
}

// GetStatefulSetParams returns params to render StatefulSet from templates.
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
	params.Affinity = n.GetAffinity()
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

// GetServiceParams returns params to render Service from templates.
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

// GetIngressParams returns params to render Ingress from templates.
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

// GetRoleParams returns params to render Role from templates.
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

// GetRoleBindingParams returns params to render RoleBinding from templates.
func (n *NemoGuardrail) GetRoleBindingParams() *rendertypes.RoleBindingParams {
	params := &rendertypes.RoleBindingParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()

	params.ServiceAccountName = n.GetServiceAccountName()
	params.RoleName = n.GetName()
	return params
}

// GetHPAParams returns params to render HPA from templates.
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

// GetSCCParams return params to render SCC from templates.
func (n *NemoGuardrail) GetSCCParams() *rendertypes.SCCParams {
	params := &rendertypes.SCCParams{}
	// Set metadata
	params.Name = "nemo-guardrails-scc"

	params.ServiceAccountName = n.GetServiceAccountName()
	return params
}

// GetServiceMonitorParams return params to render Service Monitor from templates.
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

// GetInitContainers returns the init containers for the NemoGuardrail.
//
// It creates and returns a slice of corev1.Container.
// The init containers include a busybox container to wait for Postgres to start,
// and an guardrail-db-migration container to run the database migration.
//
// Returns a slice of corev1.Container.
func (n *NemoGuardrail) GetInitContainers() []corev1.Container {

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
			Name:  "PYTHONPATH",
			Value: "/app/services/guardrails",
		},
	}
	// Append the environment variables for Postgres
	envVars = append(envVars, n.GetPostgresEnv()...)

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
			Name:            "guardrail-db-migration",
			Image:           n.GetImage(),
			ImagePullPolicy: corev1.PullPolicy(n.GetImagePullPolicy()),
			Command: []string{
				"/app/.venv/bin/alembic",
			},
			Args: []string{
				"upgrade",
				"head",
			},
			Env:        envVars,
			WorkingDir: "/app/services/guardrails",
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
	SchemeBuilder.Register(&NemoGuardrail{}, &NemoGuardrailList{})
}
