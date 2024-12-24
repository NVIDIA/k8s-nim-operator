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
	// NemoDatastoreV2ConditionReady indicates that the NEMO datastore service is ready.
	NemoDatastoreV2ConditionReady = "Ready"
	// NemoDatastoreV2ConditionFailed indicates that the NEMO datastore service has failed.
	NemoDatastoreV2ConditionFailed = "Failed"

	// NemoDatastoreV2StatusPending indicates that NEMO datastore service is in pending state
	NemoDatastoreV2StatusPending = "Pending"
	// NemoDatastoreV2StatusNotReady indicates that NEMO datastore service is not ready
	NemoDatastoreV2StatusNotReady = "NotReady"
	// NemoDatastoreV2StatusReady indicates that NEMO datastore service is ready
	NemoDatastoreV2StatusReady = "Ready"
	// NemoDatastoreV2StatusFailed indicates that NEMO datastore service has failed
	NemoDatastoreV2StatusFailed = "Failed"
)

// NemoDatastoreV2Spec defines the desired state of NemoDatastoreV2
type NemoDatastoreV2Spec struct {
	Image   Image           `json:"image,omitempty"`
	Command []string        `json:"command,omitempty"`
	Args    []string        `json:"args,omitempty"`
	Env     []corev1.EnvVar `json:"env,omitempty"`
	// The name of an secret that contains authn for the NGC NIM service API
	AuthSecret     string                       `json:"authSecret"`
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

	DataStoreParams NemoDatastoreV2Params `json:"dataStoreParams"`
}

// NemoDatastoreV2Status defines the observed state of NemoDatastoreV2
type NemoDatastoreV2Status struct {
	Conditions        []metav1.Condition `json:"conditions,omitempty"`
	AvailableReplicas int32              `json:"availableReplicas,omitempty"`
	State             string             `json:"state,omitempty"`
}

type NemoDatastoreV2Params struct {
	DBSecret         string `json:"dbSecret"`
	GiteaAdminSecret string `json:"giteaAdminSecret"`

	ObjectStoreSecret       string `json:"objStoreSecret"`
	DataStoreSettingsSecret string `json:"datastoreSettingsSecret"`
	LfsJwtSecret            string `json:"lfsJwtSecret"`

	DataStoreInitSecret         string `json:"datastoreInitSecret"`
	DataStoreConfigSecret       string `json:"datastoreConfigSecret"`
	DataStoreInlineConfigSecret string `json:"datastoreInlineConfigSecret"`

	SshEnabled bool `json:"sshEnabled"`

	PVCSharedData string `json:"pvcSharedData"`
	StorageClass  string `json:"storageClass"`
	Size          string `json:"size,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.state`,priority=0
// +kubebuilder:printcolumn:name="Age",type="date",format="date-time",JSONPath=".metadata.creationTimestamp",priority=0

// NemoDatastoreV2 is the Schema for the NemoDatastoreV2 API
type NemoDatastoreV2 struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NemoDatastoreV2Spec   `json:"spec,omitempty"`
	Status NemoDatastoreV2Status `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NemoDatastoreV2List contains a list of NemoDatastoreV2
type NemoDatastoreV2List struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NemoDatastoreV2 `json:"items"`
}

// GetPVCName returns the name to be used for the PVC based on the custom spec
// Prefers pvc.Name if explicitly set by the user in the NemoDatastoreV2 instance
func (n *NemoDatastoreV2) GetPVCName(pvc PersistentVolumeClaim) string {
	pvcName := fmt.Sprintf("%s-pvc", n.GetName())
	if pvc.Name != "" {
		pvcName = pvc.Name
	}
	return pvcName
}

// GetStandardSelectorLabels returns the standard selector labels for the NemoDatastoreV2 deployment
func (n *NemoDatastoreV2) GetStandardSelectorLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     n.Name,
		"app.kubernetes.io/instance": n.Name,
	}
}

// GetStandardLabels returns the standard set of labels for NemoDatastoreV2 resources
func (n *NemoDatastoreV2) GetStandardLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":             n.Name,
		"app.kubernetes.io/instance":         n.Name,
		"app.kubernetes.io/operator-version": os.Getenv("OPERATOR_VERSION"),
		"app.kubernetes.io/part-of":          "nemo-datastore-service",
		"app.kubernetes.io/managed-by":       "k8s-nim-operator",
	}
}

// GetMainContainerEnv returns the standard set of env variables for the NemoDatastoreV2 main container
func (n *NemoDatastoreV2) GetStandardEnv() []corev1.EnvVar {
	// add standard env required for NIM service
	envVars := []corev1.EnvVar{
		{
			Name:  "SSH_LISTEN_PORT",
			Value: "2222",
		},
		{
			Name:  "SSH_PORT",
			Value: "22",
		},
		{
			Name:  "GITEA_APP_INI",
			Value: "/data/gitea/conf/app.ini",
		},
		{
			Name:  "GITEA_CUSTOM",
			Value: "/data/gitea",
		},
		{
			Name:  "GITEA_WORK_DIR",
			Value: "/data",
		},
		{
			Name:  "TMPDIR",
			Value: "/tmp/gitea",
		},
		{
			Name:  "GITEA_TEMP",
			Value: "/tmp/gitea",
		},
		{
			Name:  "HOME",
			Value: "/data/gitea/git",
		},
		{
			Name: "GITEA__DATABASE__PASSWD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: "password",
					LocalObjectReference: corev1.LocalObjectReference{
						Name: n.Spec.DataStoreParams.DBSecret,
					},
				},
			},
		},
	}
	return envVars
}

func (n *NemoDatastoreV2) GetInitContainerEnv() []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  "GITEA_APP_INI",
			Value: "/data/gitea/conf/app.ini",
		},
		{
			Name:  "GITEA_CUSTOM",
			Value: "/data/gitea",
		},
		{
			Name:  "GITEA_WORK_DIR",
			Value: "/data",
		},
		{
			Name:  "TMPDIR",
			Value: "/tmp/gitea",
		},
		{
			Name:  "GITEA_TEMP",
			Value: "/tmp/gitea",
		},

		{
			Name:  "HOME",
			Value: "/data/gitea/git",
		},
		{
			Name: "GITEA__DATABASE__PASSWD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: "password",
					LocalObjectReference: corev1.LocalObjectReference{
						Name: n.Spec.DataStoreParams.DBSecret,
					},
				},
			},
		},
		{
			Name: "GITEA_ADMIN_USERNAME",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: "username",
					LocalObjectReference: corev1.LocalObjectReference{
						Name: n.Spec.DataStoreParams.GiteaAdminSecret,
					},
				},
			},
		},
		{
			Name: "GITEA_ADMIN_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: "password",
					LocalObjectReference: corev1.LocalObjectReference{
						Name: n.Spec.DataStoreParams.GiteaAdminSecret,
					},
				},
			},
		},
	}
	return envVars
}

// GetVolumes returns volumes for the NemoDatastoreV2 container
func (n *NemoDatastoreV2) GetVolumes() []corev1.Volume {
	/*volumes:
	  - name: init
	    secret:
	      defaultMode: 110
	      secretName: datastore-v2-nemo-datastore-init
	  - name: config
	    secret:
	      defaultMode: 110
	      secretName: datastore-v2-nemo-datastore
	  - name: inline-config-sources
	    secret:
	      defaultMode: 420
	      secretName: datastore-v2-nemo-datastore-inline-config
	  - emptyDir: {}
	    name: temp
	  - name: data
	    persistentVolumeClaim:
	      claimName: datastore-shared-storage
	*/
	var initMode = int32(110)
	var configMode = int32(420)

	volumes := []corev1.Volume{
		{
			Name: "init",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  n.Spec.DataStoreParams.DataStoreInitSecret,
					DefaultMode: &initMode,
				},
			},
		},
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  n.Spec.DataStoreParams.DataStoreConfigSecret,
					DefaultMode: &initMode,
				},
			},
		},
		{
			Name: "inline-config-sources",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  n.Spec.DataStoreParams.DataStoreInlineConfigSecret,
					DefaultMode: &configMode,
				},
			},
		},
		{
			Name: "temp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: n.Spec.DataStoreParams.PVCSharedData,
				},
			},
		},
	}
	return volumes
}

// GetStandardAnnotations returns default annotations to apply to the NemoDatastoreV2 instance
func (n *NemoDatastoreV2) GetEnvFrom() []corev1.EnvFromSource {
	return []corev1.EnvFromSource{}
}

// GetStandardAnnotations returns default annotations to apply to the NemoDatastoreV2 instance
func (n *NemoDatastoreV2) GetInitAppIniEnvFrom() []corev1.EnvFromSource {
	return []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: n.Spec.DataStoreParams.DataStoreSettingsSecret,
				},
			},
		},
	}
}

// GetStandardAnnotations returns default annotations to apply to the NemoDatastoreV2 instance
func (n *NemoDatastoreV2) GetStandardAnnotations() map[string]string {
	standardAnnotations := map[string]string{
		"openshift.io/scc": "nonroot",
	}
	return standardAnnotations
}

// GetNemoDatastoreV2Annotations returns annotations to apply to the NemoDatastoreV2 instance
func (n *NemoDatastoreV2) GetNemoDatastoreV2Annotations() map[string]string {
	standardAnnotations := n.GetStandardAnnotations()

	if n.Spec.Annotations != nil {
		return utils.MergeMaps(standardAnnotations, n.Spec.Annotations)
	}

	return standardAnnotations
}

// GetServiceLabels returns merged labels to apply to the NemoDatastoreV2 instance
func (n *NemoDatastoreV2) GetServiceLabels() map[string]string {
	standardLabels := n.GetStandardLabels()

	if n.Spec.Labels != nil {
		return utils.MergeMaps(standardLabels, n.Spec.Labels)
	}
	return standardLabels
}

// GetSelectorLabels returns standard selector labels to apply to the NemoDatastoreV2 instance
func (n *NemoDatastoreV2) GetSelectorLabels() map[string]string {
	// TODO: add custom ones
	return n.GetStandardSelectorLabels()
}

// GetNodeSelector returns node selector labels for the NemoDatastoreV2 instance
func (n *NemoDatastoreV2) GetNodeSelector() map[string]string {
	return n.Spec.NodeSelector
}

// GetTolerations returns tolerations for the NemoDatastoreV2 instance
func (n *NemoDatastoreV2) GetTolerations() []corev1.Toleration {
	return n.Spec.Tolerations
}

// GetPodAffinity returns pod affinity for the NemoDatastoreV2 instance
func (n *NemoDatastoreV2) GetPodAffinity() *corev1.PodAffinity {
	return n.Spec.PodAffinity
}

// GetContainerName returns name of the container for NemoDatastoreV2 deployment
func (n *NemoDatastoreV2) GetContainerName() string {
	return fmt.Sprintf("%s-ctr", n.Name)
}

// GetCommand return command to override for the NemoDatastoreV2 container
func (n *NemoDatastoreV2) GetCommand() []string {
	return n.Spec.Command
}

// GetArgs return arguments for the NemoDatastoreV2 container
func (n *NemoDatastoreV2) GetArgs() []string {
	return n.Spec.Args
}

// GetEnv returns merged slice of standard and user specified env variables
func (n *NemoDatastoreV2) GetEnv() []corev1.EnvVar {
	return utils.MergeEnvVars(n.GetStandardEnv(), n.Spec.Env)
}

// GetImage returns container image for the NemoDatastoreV2
func (n *NemoDatastoreV2) GetImage() string {
	return fmt.Sprintf("%s:%s", n.Spec.Image.Repository, n.Spec.Image.Tag)
}

// GetImagePullSecrets returns the image pull secrets for the NIM container
func (n *NemoDatastoreV2) GetImagePullSecrets() []string {
	return n.Spec.Image.PullSecrets
}

// GetImagePullPolicy returns the image pull policy for the NIM container
func (n *NemoDatastoreV2) GetImagePullPolicy() string {
	return n.Spec.Image.PullPolicy
}

// GetResources returns resources to allocate to the NemoDatastoreV2 container
func (n *NemoDatastoreV2) GetResources() *corev1.ResourceRequirements {
	return n.Spec.Resources
}

// GetLivenessProbe returns liveness probe for the NemoDatastoreV2 container
func (n *NemoDatastoreV2) GetLivenessProbe() *corev1.Probe {
	if n.Spec.LivenessProbe.Probe == nil {
		return n.GetDefaultLivenessProbe()
	}
	return n.Spec.LivenessProbe.Probe
}

// GetDefaultLivenessProbe returns the default liveness probe for the NemoDatastoreV2 container
func (n *NemoDatastoreV2) GetDefaultLivenessProbe() *corev1.Probe {
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

// GetReadinessProbe returns readiness probe for the NemoDatastoreV2 container
func (n *NemoDatastoreV2) GetReadinessProbe() *corev1.Probe {
	if n.Spec.ReadinessProbe.Probe == nil {
		return n.GetDefaultReadinessProbe()
	}
	return n.Spec.ReadinessProbe.Probe
}

// GetDefaultReadinessProbe returns the default readiness probe for the NemoDatastoreV2 container
func (n *NemoDatastoreV2) GetDefaultReadinessProbe() *corev1.Probe {
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

// GetStartupProbe returns startup probe for the NemoDatastoreV2 container
func (n *NemoDatastoreV2) GetStartupProbe() *corev1.Probe {
	return n.Spec.StartupProbe.Probe
}

// GetDefaultStartupProbe returns the default startup probe for the NemoDatastoreV2 container
func (n *NemoDatastoreV2) GetDefaultStartupProbe() *corev1.Probe {
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

// GetVolumeMounts returns volumes for the NemoDatastoreV2 container
func (n *NemoDatastoreV2) GetVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			MountPath: "/tmp",
			Name:      "temp",
		},
		{
			MountPath: "/data",
			Name:      "data",
		},
	}
}

func (n *NemoDatastoreV2) GetVolumeMountsInitContainer() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			MountPath: "/usr/sbin",
			Name:      "config",
		},
		{
			MountPath: "/tmp",
			Name:      "temp",
		},
		{
			MountPath: "/data",
			Name:      "data",
		},
		{
			MountPath: "/env-to-ini-mounts/inlines/",
			Name:      "inline-config-sources",
		},
		{
			MountPath: "/usr/sbin/init",
			Name:      "init",
		},
	}
}

func (n *NemoDatastoreV2) GetInitContainers() []corev1.Container {
	user := int64(1000)
	return []corev1.Container{
		{
			Name:            "init-datastore",
			Image:           n.GetImage(),
			ImagePullPolicy: corev1.PullPolicy(n.GetImagePullPolicy()),
			Command: []string{
				"/bin/sh", "-c",
			},
			Args: []string{
				"/usr/sbin/init/init_directory_structure.sh && /usr/sbin/config_environment.sh",
			},
			VolumeMounts: n.GetVolumeMountsInitContainer(),
			Env:          n.GetInitContainerEnv(),
			EnvFrom:      n.GetInitAppIniEnvFrom(),
		},
		{
			Name:            "configure-datastore",
			Image:           n.GetImage(),
			ImagePullPolicy: corev1.PullPolicy(n.GetImagePullPolicy()),
			Command: []string{
				"/bin/sh", "-c",
			},
			Args: []string{
				"/usr/sbin/init/configure_gitea.sh",
			},
			VolumeMounts: n.GetVolumeMountsInitContainer(),
			Env:          n.GetInitContainerEnv(),
			EnvFrom:      n.GetInitAppIniEnvFrom(),
			SecurityContext: &corev1.SecurityContext{
				RunAsUser: &user,
			},
		},
	}
}

// GetServiceAccountName returns service account name for the NemoDatastoreV2 deployment
func (n *NemoDatastoreV2) GetServiceAccountName() string {
	return n.Name
}

// GetRuntimeClass return the runtime class name for the NemoDatastoreV2 deployment
func (n *NemoDatastoreV2) GetRuntimeClass() string {
	return n.Spec.RuntimeClass
}

// GetHPA returns the HPA spec for the NemoDatastoreV2 deployment
func (n *NemoDatastoreV2) GetHPA() HorizontalPodAutoscalerSpec {
	return n.Spec.Scale.HPA
}

// GetServiceMonitor returns the Service Monitor details for the NemoDatastoreV2 deployment
func (n *NemoDatastoreV2) GetServiceMonitor() ServiceMonitor {
	return n.Spec.Metrics.ServiceMonitor
}

// GetReplicas returns replicas for the NemoDatastoreV2 deployment
func (n *NemoDatastoreV2) GetReplicas() int {
	if n.IsAutoScalingEnabled() {
		return 0
	}
	return n.Spec.Replicas
}

// GetDeploymentKind returns the kind of deployment for NemoDatastoreV2
func (n *NemoDatastoreV2) GetDeploymentKind() string {
	return "Deployment"
}

// IsAutoScalingEnabled returns true if autoscaling is enabled for NemoDatastoreV2 deployment
func (n *NemoDatastoreV2) IsAutoScalingEnabled() bool {
	return n.Spec.Scale.Enabled != nil && *n.Spec.Scale.Enabled
}

// IsIngressEnabled returns true if ingress is enabled for NemoDatastoreV2 deployment
func (n *NemoDatastoreV2) IsIngressEnabled() bool {
	return n.Spec.Expose.Ingress.Enabled != nil && *n.Spec.Expose.Ingress.Enabled
}

// GetIngressSpec returns the Ingress spec NemoDatastoreV2 deployment
func (n *NemoDatastoreV2) GetIngressSpec() networkingv1.IngressSpec {
	return n.Spec.Expose.Ingress.Spec
}

// IsServiceMonitorEnabled returns true if servicemonitor is enabled for NemoDatastoreV2 deployment
func (n *NemoDatastoreV2) IsServiceMonitorEnabled() bool {
	return n.Spec.Metrics.Enabled != nil && *n.Spec.Metrics.Enabled
}

// GetServicePort returns the service port for the NemoDatastoreV2 deployment
func (n *NemoDatastoreV2) GetServicePort() int32 {
	return n.Spec.Expose.Service.Port
}

// GetServiceType returns the service type for the NemoDatastoreV2 deployment
func (n *NemoDatastoreV2) GetServiceType() string {
	return string(n.Spec.Expose.Service.Type)
}

// GetUserID returns the user ID for the NemoDatastoreV2 deployment
func (n *NemoDatastoreV2) GetUserID() *int64 {
	return n.Spec.UserID

}

// GetGroupID returns the group ID for the NemoDatastoreV2 deployment
func (n *NemoDatastoreV2) GetGroupID() *int64 {
	return n.Spec.GroupID

}

// GetServiceAccountParams return params to render ServiceAccount from templates
func (n *NemoDatastoreV2) GetServiceAccountParams() *rendertypes.ServiceAccountParams {
	params := &rendertypes.ServiceAccountParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNemoDatastoreV2Annotations()
	return params
}

// GetDeploymentParams returns params to render Deployment from templates
func (n *NemoDatastoreV2) GetDeploymentParams() *rendertypes.DeploymentParams {
	params := &rendertypes.DeploymentParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNemoDatastoreV2Annotations()

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
func (n *NemoDatastoreV2) GetStatefulSetParams() *rendertypes.StatefulSetParams {

	params := &rendertypes.StatefulSetParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNemoDatastoreV2Annotations()

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
func (n *NemoDatastoreV2) GetServiceParams() *rendertypes.ServiceParams {
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
func (n *NemoDatastoreV2) GetIngressParams() *rendertypes.IngressParams {
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
func (n *NemoDatastoreV2) GetRoleParams() *rendertypes.RoleParams {
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
func (n *NemoDatastoreV2) GetRoleBindingParams() *rendertypes.RoleBindingParams {
	params := &rendertypes.RoleBindingParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()

	params.ServiceAccountName = n.GetServiceAccountName()
	params.RoleName = n.GetName()
	return params
}

// GetHPAParams returns params to render HPA from templates
func (n *NemoDatastoreV2) GetHPAParams() *rendertypes.HPAParams {
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
func (n *NemoDatastoreV2) GetSCCParams() *rendertypes.SCCParams {
	params := &rendertypes.SCCParams{}
	// Set metadata
	params.Name = "nemo-datastore-scc"

	params.ServiceAccountName = n.GetServiceAccountName()
	return params
}

// GetServiceMonitorParams return params to render Service Monitor from templates
func (n *NemoDatastoreV2) GetServiceMonitorParams() *rendertypes.ServiceMonitorParams {
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

func (n *NemoDatastoreV2) GetIngressAnnotations() map[string]string {
	NemoDatastoreV2Annotations := n.GetNemoDatastoreV2Annotations()

	if n.Spec.Expose.Ingress.Annotations != nil {
		return utils.MergeMaps(NemoDatastoreV2Annotations, n.Spec.Expose.Ingress.Annotations)
	}
	return NemoDatastoreV2Annotations
}

func (n *NemoDatastoreV2) GetServiceAnnotations() map[string]string {
	NemoDatastoreV2Annotations := n.GetNemoDatastoreV2Annotations()

	if n.Spec.Expose.Service.Annotations != nil {
		return utils.MergeMaps(NemoDatastoreV2Annotations, n.Spec.Expose.Service.Annotations)
	}
	return NemoDatastoreV2Annotations
}

func (n *NemoDatastoreV2) GetHPAAnnotations() map[string]string {
	NemoDatastoreV2Annotations := n.GetNemoDatastoreV2Annotations()

	if n.Spec.Scale.Annotations != nil {
		return utils.MergeMaps(NemoDatastoreV2Annotations, n.Spec.Scale.Annotations)
	}
	return NemoDatastoreV2Annotations
}

func (n *NemoDatastoreV2) GetServiceMonitorAnnotations() map[string]string {
	NemoDatastoreV2Annotations := n.GetNemoDatastoreV2Annotations()

	if n.Spec.Metrics.ServiceMonitor.Annotations != nil {
		return utils.MergeMaps(NemoDatastoreV2Annotations, n.Spec.Metrics.ServiceMonitor.Annotations)
	}
	return NemoDatastoreV2Annotations
}

func init() {
	SchemeBuilder.Register(&NemoDatastoreV2{}, &NemoDatastoreV2List{})
}
