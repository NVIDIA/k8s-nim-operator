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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	// NIMPipelineConditionReady indicates that the NIM pipeline is ready.
	NIMPipelineConditionReady = "NIM_PIPELINE_READY"
	// NIMPipelineConditionFailed indicates that the NIM pipeline has failed.
	NIMPipelineConditionFailed = "NIM_PIPELINE_FAILED"

	// NIMPipelinetatusNotReady indicates that one or more services in the NIM pipeline are not ready
	NIMPipelinetatusNotReady = "NotReady"
	// NIMPipelineStatusReady indicates that NIM pipeline is ready
	NIMPipelineStatusReady = "Ready"
	// NIMPipelineStatusFailed indicates that one or more services in the NIM pipeline has failed
	NIMPipelineStatusFailed = "Failed"
)

// NIMPipelineSpec defines the desired state of NIMPipeline
type NIMPipelineSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// NIMService configures attributes to deploy a NIM service as part of the pipeline
	Services []NIMServicePipelineSpec `json:"services,omitempty"`
}

// NIMServicePipelineSpec defines the desired state of NIMService as part of the NIMPipeline
type NIMServicePipelineSpec struct {
	Name         string              `json:"name,omitempty"`
	Enabled      *bool               `json:"enabled,omitempty"`
	Spec         NIMServiceSpec      `json:"spec,omitempty"`
	Dependencies []ServiceDependency `json:"dependencies,omitempty"`
}

// ServiceDependency defines service dependencies
type ServiceDependency struct {
	// Name is the dependent service name
	Name string `json:"name"`
	// Port is the dependent service port
	Port int32 `json:"port"`
	// EnvName is the dependent service endpoint environment variable name
	EnvName string `json:"envName,omitempty"`
	// EnvValue is the dependent service endpoint environment variable value
	EnvValue string `json:"envValue,omitempty"`
}

// NIMPipelineStatus defines the observed state of NIMPipeline
type NIMPipelineStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// States indicate state of individual services in the pipeline
	States map[string]string `json:"states,omitempty"`
	// State indicates the overall state of the pipeline
	State string `json:"state,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.state`,priority=0
// +kubebuilder:printcolumn:name="Age",type=string,JSONPath=`.metadata.creationTimestamp`,priority=0

// NIMPipeline is the Schema for the nimpipelines API
type NIMPipeline struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NIMPipelineSpec   `json:"spec,omitempty"`
	Status NIMPipelineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NIMPipelineList contains a list of NIMPipeline
type NIMPipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NIMPipeline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NIMPipeline{}, &NIMPipelineList{})
}
