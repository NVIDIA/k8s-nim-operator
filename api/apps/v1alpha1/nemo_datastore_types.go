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

	rendertypes "github.com/NVIDIA/k8s-nim-operator/internal/render/types"
	utils "github.com/NVIDIA/k8s-nim-operator/internal/utils"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	// NemoDatastoreConditionReady indicates that the NEMO datastore service is ready.
	NemoDatastoreConditionReady = "Ready"
	// NemoDatastoreConditionFailed indicates that the NEMO datastore service has failed.
	NemoDatastoreConditionFailed = "Failed"

	// NemoDatastoreStatusPending indicates that NEMO datastore service is in pending state
	NemoDatastoreStatusPending = "Pending"
	// NemoDatastoreStatusNotReady indicates that NEMO datastore service is not ready
	NemoDatastoreStatusNotReady = "NotReady"
	// NemoDatastoreStatusReady indicates that NEMO datastore service is ready
	NemoDatastoreStatusReady = "Ready"
	// NemoDatastoreStatusFailed indicates that NEMO datastore service has failed
	NemoDatastoreStatusFailed = "Failed"
)

// NemoDatastoreSpec defines the desired state of NemoDatastore
type NemoDatastoreSpec struct {
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

	// ObjectStore specifies the location and credentials for accessing the external Object Storage
	ObjectStoreConfig ObjectStoreConfig `json:"objectStoreConfig"` // e.g. minio
	// ExternalDatabase contains external PostgreSQL configuration
	DatabaseConfig DatabaseConfig `json:"databaseConfig"` // e.g. postgres
	// secrets contains the pre-requisite secrets that must be created before deploying the datastore CR
	Secrets Secrets `json:"secrets"`
	// PVC defines the PersistentVolumeClaim for the datastore
	PVC *PersistentVolumeClaim `json:"pvc,omitempty"`
}

type Secrets struct {
	GiteaAdminSecret            string `json:"giteaAdminSecret"`
	LfsJwtSecret                string `json:"lfsJwtSecret"`
	DataStoreInitSecret         string `json:"datastoreInitSecret"`
	DataStoreConfigSecret       string `json:"datastoreConfigSecret"` // config_environment.sh
	DataStoreInlineConfigSecret string `json:"datastoreInlineConfigSecret"`
}

// NemoDatastoreStatus defines the observed state of NemoDatastore
type NemoDatastoreStatus struct {
	Conditions        []metav1.Condition `json:"conditions,omitempty"`
	AvailableReplicas int32              `json:"availableReplicas,omitempty"`
	State             string             `json:"state,omitempty"`
}

type ObjectStoreConfig struct { // e.g. Minio, s3
	// ObjectStoreCredentials stores the configuration to retrieve the object store credentials
	Credentials ObjectStoreCredentials `json:"credentials"`

	// +kubebuilder:default:=true
	ServeDirect bool `json:"serveDirect,omitempty"`

	// endpoint is the fully qualidfied object store endpoint
	Endpoint string `json:"endpoint"`
	// BucketName is the bucket where LFS files will be stored
	BucketName string `json:"bucketName"`
	// Region is the region where bucket is hosted
	Region string `json:"region"`
	// SSL enable ssl for object store transport
	SSL bool `json:"ssl"`
}

type ObjectStoreCredentials struct {
	// User is the non-root username for a NEMO Service in the object store.
	User string `json:"user"`

	// SecretName is the name of the secret which has the object credentials for a NEMO service user.
	SecretName string `json:"secretName"`

	// PasswordKey is the name of the key in the `CredentialsSecret` secret for the object store credentials.
	PasswordKey string `json:"passwordKey"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.state`,priority=0
// +kubebuilder:printcolumn:name="Age",type="date",format="date-time",JSONPath=".metadata.creationTimestamp",priority=0

// NemoDatastore is the Schema for the NemoDatastore API
type NemoDatastore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NemoDatastoreSpec   `json:"spec,omitempty"`
	Status NemoDatastoreStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NemoDatastoreList contains a list of NemoDatastore
type NemoDatastoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NemoDatastore `json:"items"`
}

// GetPVCName returns the name to be used for the PVC based on the custom spec
// Prefers pvc.Name if explicitly set by the user in the NemoDatastore instance
func (n *NemoDatastore) GetPVCName() string {
	pvcName := fmt.Sprintf("%s-pvc", n.GetName())
	if n.Spec.PVC != nil && n.Spec.PVC.Name != "" {
		pvcName = n.Spec.PVC.Name
	}
	return pvcName
}

// GetStandardSelectorLabels returns the standard selector labels for the NemoDatastore deployment
func (n *NemoDatastore) GetStandardSelectorLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     n.Name,
		"app.kubernetes.io/instance": n.Name,
	}
}

// GetStandardLabels returns the standard set of labels for NemoDatastore resources
func (n *NemoDatastore) GetStandardLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":             n.Name,
		"app.kubernetes.io/instance":         n.Name,
		"app.kubernetes.io/operator-version": os.Getenv("OPERATOR_VERSION"),
		"app.kubernetes.io/part-of":          "nemo-datastore-service",
		"app.kubernetes.io/managed-by":       "k8s-nim-operator",
	}
}

// GetMainContainerEnv returns the standard set of env variables for the NemoDatastore main container
func (n *NemoDatastore) GetStandardEnv() []corev1.EnvVar {
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
			Name:  "GITEA__LFS__MINIO_ACCESS_KEY_ID",
			Value: n.Spec.ObjectStoreConfig.Credentials.User,
		},
		{
			Name: "GITEA__LFS__MINIO_SECRET_ACCESS_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: n.Spec.ObjectStoreConfig.Credentials.PasswordKey,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: n.Spec.ObjectStoreConfig.Credentials.SecretName,
					},
				},
			},
		},
		{
			Name: "GITEA__SERVER__LFS_JWT_SECRET",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: "jwtSecret",
					LocalObjectReference: corev1.LocalObjectReference{
						Name: n.Spec.Secrets.LfsJwtSecret,
					},
				},
			},
		},
		{
			Name: "GITEA__DATABASE__PASSWD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: n.Spec.DatabaseConfig.Credentials.PasswordKey,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: n.Spec.DatabaseConfig.Credentials.SecretName,
					},
				},
			},
		},
	}
	return envVars
}

func (n *NemoDatastore) GetInitContainerEnv() []corev1.EnvVar {
	objStoreSetting := n.Spec.ObjectStoreConfig
	dbSetting := n.Spec.DatabaseConfig

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
			Name:  "GITEA__LFS__MINIO_ACCESS_KEY_ID",
			Value: objStoreSetting.Credentials.User,
		},
		{
			Name: "GITEA__LFS__MINIO_SECRET_ACCESS_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: objStoreSetting.Credentials.PasswordKey,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: objStoreSetting.Credentials.SecretName,
					},
				},
			},
		},
		{
			Name: "GITEA__SERVER__LFS_JWT_SECRET",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: "jwtSecret",
					LocalObjectReference: corev1.LocalObjectReference{
						Name: n.Spec.Secrets.LfsJwtSecret,
					},
				},
			},
		},
		{
			Name: "GITEA__DATABASE__PASSWD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: dbSetting.Credentials.PasswordKey,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: dbSetting.Credentials.SecretName,
					},
				},
			},
		},
		{
			Name: "GITEA_ADMIN_USERNAME",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: "GITEA_ADMIN_USERNAME",
					LocalObjectReference: corev1.LocalObjectReference{
						Name: n.Spec.Secrets.GiteaAdminSecret,
					},
				},
			},
		},
		{
			Name: "GITEA_ADMIN_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: "GITEA_ADMIN_PASSWORD",
					LocalObjectReference: corev1.LocalObjectReference{
						Name: n.Spec.Secrets.GiteaAdminSecret,
					},
				},
			},
		},
		{
			Name:  "GITEA__LFS__SERVE_DIRECT",
			Value: strconv.FormatBool(objStoreSetting.ServeDirect),
		},
		{
			Name:  "GITEA__LFS__STORAGE_TYPE",
			Value: "minio",
		},
		{
			Name:  "GITEA__LFS__MINIO_ENDPOINT",
			Value: objStoreSetting.Endpoint,
		},
		{
			Name:  "GITEA__LFS__MINIO_BUCKET",
			Value: objStoreSetting.BucketName,
		},
		{
			Name:  "GITEA__LFS__MINIO_LOCATION",
			Value: objStoreSetting.Region,
		},
		{
			Name:  "GITEA__LFS__MINIO_LOCATION",
			Value: objStoreSetting.Region,
		},
		{
			Name:  "GITEA__LFS__MINIO_USE_SSL",
			Value: strconv.FormatBool(objStoreSetting.SSL),
		},
		{
			Name:  "GITEA__DATABASE__SSL_MODE",
			Value: "disable",
		},
		{
			Name:  "GITEA__DATABASE__NAME",
			Value: dbSetting.DatabaseName,
		},
		{
			Name:  "GITEA__DATABASE__HOST",
			Value: fmt.Sprintf("%s:%d", dbSetting.Host, dbSetting.Port),
		},
		{
			Name:  "GITEA__DATABASE__USER",
			Value: dbSetting.Credentials.User,
		},
	}
	return envVars
}

// GetVolumes returns volumes for the NemoDatastore container
func (n *NemoDatastore) GetVolumes() []corev1.Volume {
	/*volumes:
	  - name: init
	    secret:
	      defaultMode: 110
	      secretName: datastore-nemo-datastore-init
	  - name: config
	    secret:
	      defaultMode: 110
	      secretName: datastore-nemo-datastore
	  - name: inline-config-sources
	    secret:
	      defaultMode: 420
	      secretName: datastore-nemo-datastore-inline-config
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
					SecretName:  n.Spec.Secrets.DataStoreInitSecret,
					DefaultMode: &initMode,
				},
			},
		},
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  n.Spec.Secrets.DataStoreConfigSecret,
					DefaultMode: &initMode,
				},
			},
		},
		{
			Name: "inline-config-sources",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  n.Spec.Secrets.DataStoreInlineConfigSecret,
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
	}

	if n.Spec.PVC != nil {
		volumes = append(volumes, corev1.Volume{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: n.GetPVCName(),
				},
			},
		})
	} else {
		volumes = append(volumes, corev1.Volume{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}
	return volumes
}

func (n *NemoDatastore) ShouldCreatePersistentStorage() bool {
	return n.Spec.PVC != nil && n.Spec.PVC.Create != nil && *n.Spec.PVC.Create
}

// GetStandardAnnotations returns default annotations to apply to the NemoDatastore instance
func (n *NemoDatastore) GetStandardAnnotations() map[string]string {
	standardAnnotations := map[string]string{
		"openshift.io/scc": "nonroot",
	}
	return standardAnnotations
}

// GetNemoDatastoreAnnotations returns annotations to apply to the NemoDatastore instance
func (n *NemoDatastore) GetNemoDatastoreAnnotations() map[string]string {
	standardAnnotations := n.GetStandardAnnotations()

	if n.Spec.Annotations != nil {
		return utils.MergeMaps(standardAnnotations, n.Spec.Annotations)
	}

	return standardAnnotations
}

// GetServiceLabels returns merged labels to apply to the NemoDatastore instance
func (n *NemoDatastore) GetServiceLabels() map[string]string {
	standardLabels := n.GetStandardLabels()

	if n.Spec.Labels != nil {
		return utils.MergeMaps(standardLabels, n.Spec.Labels)
	}
	return standardLabels
}

// GetSelectorLabels returns standard selector labels to apply to the NemoDatastore instance
func (n *NemoDatastore) GetSelectorLabels() map[string]string {
	// TODO: add custom ones
	return n.GetStandardSelectorLabels()
}

// GetNodeSelector returns node selector labels for the NemoDatastore instance
func (n *NemoDatastore) GetNodeSelector() map[string]string {
	return n.Spec.NodeSelector
}

// GetTolerations returns tolerations for the NemoDatastore instance
func (n *NemoDatastore) GetTolerations() []corev1.Toleration {
	return n.Spec.Tolerations
}

// GetPodAffinity returns pod affinity for the NemoDatastore instance
func (n *NemoDatastore) GetPodAffinity() *corev1.PodAffinity {
	return n.Spec.PodAffinity
}

// GetContainerName returns name of the container for NemoDatastore deployment
func (n *NemoDatastore) GetContainerName() string {
	return fmt.Sprintf("%s-ctr", n.Name)
}

// GetCommand return command to override for the NemoDatastore container
func (n *NemoDatastore) GetCommand() []string {
	return n.Spec.Command
}

// GetArgs return arguments for the NemoDatastore container
func (n *NemoDatastore) GetArgs() []string {
	return n.Spec.Args
}

// GetEnv returns merged slice of standard and user specified env variables
func (n *NemoDatastore) GetEnv() []corev1.EnvVar {
	return utils.MergeEnvVars(n.GetStandardEnv(), n.Spec.Env)
}

// GetImage returns container image for the NemoDatastore
func (n *NemoDatastore) GetImage() string {
	return fmt.Sprintf("%s:%s", n.Spec.Image.Repository, n.Spec.Image.Tag)
}

// GetImagePullSecrets returns the image pull secrets for the NIM container
func (n *NemoDatastore) GetImagePullSecrets() []string {
	return n.Spec.Image.PullSecrets
}

// GetImagePullPolicy returns the image pull policy for the NIM container
func (n *NemoDatastore) GetImagePullPolicy() string {
	return n.Spec.Image.PullPolicy
}

// GetResources returns resources to allocate to the NemoDatastore container
func (n *NemoDatastore) GetResources() *corev1.ResourceRequirements {
	return n.Spec.Resources
}

// GetLivenessProbe returns liveness probe for the NemoDatastore container
func (n *NemoDatastore) GetLivenessProbe() *corev1.Probe {
	if n.Spec.LivenessProbe.Probe == nil {
		return n.GetDefaultLivenessProbe()
	}
	return n.Spec.LivenessProbe.Probe
}

// GetDefaultLivenessProbe returns the default liveness probe for the NemoDatastore container
func (n *NemoDatastore) GetDefaultLivenessProbe() *corev1.Probe {
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

// GetReadinessProbe returns readiness probe for the NemoDatastore container
func (n *NemoDatastore) GetReadinessProbe() *corev1.Probe {
	if n.Spec.ReadinessProbe.Probe == nil {
		return n.GetDefaultReadinessProbe()
	}
	return n.Spec.ReadinessProbe.Probe
}

// GetDefaultReadinessProbe returns the default readiness probe for the NemoDatastore container
func (n *NemoDatastore) GetDefaultReadinessProbe() *corev1.Probe {
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

// GetStartupProbe returns startup probe for the NemoDatastore container
func (n *NemoDatastore) GetStartupProbe() *corev1.Probe {
	return n.Spec.StartupProbe.Probe
}

// GetDefaultStartupProbe returns the default startup probe for the NemoDatastore container
func (n *NemoDatastore) GetDefaultStartupProbe() *corev1.Probe {
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

// GetVolumeMounts returns volumes for the NemoDatastore container
func (n *NemoDatastore) GetVolumeMounts() []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{
		{
			MountPath: "/tmp",
			Name:      "temp",
		},
	}

	dataMount := corev1.VolumeMount{
		MountPath: "/data",
		Name:      "data",
	}

	if n.Spec.PVC != nil {
		dataMount.SubPath = n.Spec.PVC.SubPath
	}
	mounts = append(mounts, dataMount)
	return mounts
}

func (n *NemoDatastore) GetVolumeMountsInitContainer() []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{
		{
			MountPath: "/usr/sbin",
			Name:      "config",
		},
		{
			MountPath: "/tmp",
			Name:      "temp",
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
	dataMount := corev1.VolumeMount{
		MountPath: "/data",
		Name:      "data",
	}

	if n.Spec.PVC != nil {
		dataMount.SubPath = n.Spec.PVC.SubPath
	}
	mounts = append(mounts, dataMount)
	return mounts
}

func (n *NemoDatastore) GetInitContainers() []corev1.Container {
	return []corev1.Container{
		{
			Name:            "init-directories",
			Image:           n.GetImage(),
			ImagePullPolicy: corev1.PullPolicy(n.GetImagePullPolicy()),
			Command: []string{
				"/usr/sbin/init/init_directory_structure.sh",
			},
			VolumeMounts: n.GetVolumeMountsInitContainer(),
			Env:          n.GetInitContainerEnv(),
		},
		{
			Name:            "init-app-ini",
			Image:           n.GetImage(),
			ImagePullPolicy: corev1.PullPolicy(n.GetImagePullPolicy()),
			Command: []string{
				"/usr/sbin/config_environment.sh",
			},
			VolumeMounts: n.GetVolumeMountsInitContainer(),
			Env:          n.GetInitContainerEnv(),
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
			SecurityContext: &corev1.SecurityContext{
				RunAsUser: n.GetUserID(),
			},
		},
	}
}

// GetServiceAccountName returns service account name for the NemoDatastore deployment
func (n *NemoDatastore) GetServiceAccountName() string {
	return n.Name
}

// GetRuntimeClass return the runtime class name for the NemoDatastore deployment
func (n *NemoDatastore) GetRuntimeClass() string {
	return n.Spec.RuntimeClass
}

// GetHPA returns the HPA spec for the NemoDatastore deployment
func (n *NemoDatastore) GetHPA() HorizontalPodAutoscalerSpec {
	return n.Spec.Scale.HPA
}

// GetServiceMonitor returns the Service Monitor details for the NemoDatastore deployment
func (n *NemoDatastore) GetServiceMonitor() ServiceMonitor {
	return n.Spec.Metrics.ServiceMonitor
}

// GetReplicas returns replicas for the NemoDatastore deployment
func (n *NemoDatastore) GetReplicas() int {
	if n.IsAutoScalingEnabled() {
		return 0
	}
	return n.Spec.Replicas
}

// GetDeploymentKind returns the kind of deployment for NemoDatastore
func (n *NemoDatastore) GetDeploymentKind() string {
	return "Deployment"
}

// IsAutoScalingEnabled returns true if autoscaling is enabled for NemoDatastore deployment
func (n *NemoDatastore) IsAutoScalingEnabled() bool {
	return n.Spec.Scale.Enabled != nil && *n.Spec.Scale.Enabled
}

// IsIngressEnabled returns true if ingress is enabled for NemoDatastore deployment
func (n *NemoDatastore) IsIngressEnabled() bool {
	return n.Spec.Expose.Ingress.Enabled != nil && *n.Spec.Expose.Ingress.Enabled
}

// GetIngressSpec returns the Ingress spec NemoDatastore deployment
func (n *NemoDatastore) GetIngressSpec() networkingv1.IngressSpec {
	return n.Spec.Expose.Ingress.Spec
}

// IsServiceMonitorEnabled returns true if servicemonitor is enabled for NemoDatastore deployment
func (n *NemoDatastore) IsServiceMonitorEnabled() bool {
	return n.Spec.Metrics.Enabled != nil && *n.Spec.Metrics.Enabled
}

// GetServicePort returns the service port for the NemoDatastore deployment
func (n *NemoDatastore) GetServicePort() int32 {
	return n.Spec.Expose.Service.Port
}

// GetServiceType returns the service type for the NemoDatastore deployment
func (n *NemoDatastore) GetServiceType() string {
	return string(n.Spec.Expose.Service.Type)
}

// GetUserID returns the user ID for the NemoDatastore deployment
func (n *NemoDatastore) GetUserID() *int64 {
	if n.Spec.UserID != nil {
		return n.Spec.UserID
	}
	return ptr.To[int64](1000)
}

// GetGroupID returns the group ID for the NemoDatastore deployment
func (n *NemoDatastore) GetGroupID() *int64 {
	if n.Spec.GroupID != nil {
		return n.Spec.GroupID
	}
	return ptr.To[int64](2000)
}

// GetServiceAccountParams return params to render ServiceAccount from templates
func (n *NemoDatastore) GetServiceAccountParams() *rendertypes.ServiceAccountParams {
	params := &rendertypes.ServiceAccountParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNemoDatastoreAnnotations()
	return params
}

// GetDeploymentParams returns params to render Deployment from templates
func (n *NemoDatastore) GetDeploymentParams() *rendertypes.DeploymentParams {
	params := &rendertypes.DeploymentParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNemoDatastoreAnnotations()

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
func (n *NemoDatastore) GetStatefulSetParams() *rendertypes.StatefulSetParams {

	params := &rendertypes.StatefulSetParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()
	params.Labels = n.GetServiceLabels()
	params.Annotations = n.GetNemoDatastoreAnnotations()

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
func (n *NemoDatastore) GetServiceParams() *rendertypes.ServiceParams {
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
func (n *NemoDatastore) GetIngressParams() *rendertypes.IngressParams {
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
func (n *NemoDatastore) GetRoleParams() *rendertypes.RoleParams {
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
func (n *NemoDatastore) GetRoleBindingParams() *rendertypes.RoleBindingParams {
	params := &rendertypes.RoleBindingParams{}

	// Set metadata
	params.Name = n.GetName()
	params.Namespace = n.GetNamespace()

	params.ServiceAccountName = n.GetServiceAccountName()
	params.RoleName = n.GetName()
	return params
}

// GetHPAParams returns params to render HPA from templates
func (n *NemoDatastore) GetHPAParams() *rendertypes.HPAParams {
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
func (n *NemoDatastore) GetSCCParams() *rendertypes.SCCParams {
	params := &rendertypes.SCCParams{}
	// Set metadata
	params.Name = "nemo-datastore-scc"

	params.ServiceAccountName = n.GetServiceAccountName()
	return params
}

// GetServiceMonitorParams return params to render Service Monitor from templates
func (n *NemoDatastore) GetServiceMonitorParams() *rendertypes.ServiceMonitorParams {
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

func (n *NemoDatastore) GetIngressAnnotations() map[string]string {
	NemoDatastoreAnnotations := n.GetNemoDatastoreAnnotations()

	if n.Spec.Expose.Ingress.Annotations != nil {
		return utils.MergeMaps(NemoDatastoreAnnotations, n.Spec.Expose.Ingress.Annotations)
	}
	return NemoDatastoreAnnotations
}

func (n *NemoDatastore) GetServiceAnnotations() map[string]string {
	NemoDatastoreAnnotations := n.GetNemoDatastoreAnnotations()

	if n.Spec.Expose.Service.Annotations != nil {
		return utils.MergeMaps(NemoDatastoreAnnotations, n.Spec.Expose.Service.Annotations)
	}
	return NemoDatastoreAnnotations
}

func (n *NemoDatastore) GetHPAAnnotations() map[string]string {
	NemoDatastoreAnnotations := n.GetNemoDatastoreAnnotations()

	if n.Spec.Scale.Annotations != nil {
		return utils.MergeMaps(NemoDatastoreAnnotations, n.Spec.Scale.Annotations)
	}
	return NemoDatastoreAnnotations
}

func (n *NemoDatastore) GetServiceMonitorAnnotations() map[string]string {
	NemoDatastoreAnnotations := n.GetNemoDatastoreAnnotations()

	if n.Spec.Metrics.ServiceMonitor.Annotations != nil {
		return utils.MergeMaps(NemoDatastoreAnnotations, n.Spec.Metrics.ServiceMonitor.Annotations)
	}
	return NemoDatastoreAnnotations
}

func init() {
	SchemeBuilder.Register(&NemoDatastore{}, &NemoDatastoreList{})
}
