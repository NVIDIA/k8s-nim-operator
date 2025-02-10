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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	// NemoServiceConditionReady indicates that the all Nemo services are ready.
	NemoServiceConditionReady = "NEMO_SERVICES_READY"
	// NemoServiceConditionFailed indicates that one or more Nemo services have failed.
	NemoServiceConditionFailed = "NEMO_SERVICES_FAILED"

	// NemoServiceStatusNotReady indicates that one or more Nemo services are not ready
	NemoServiceStatusNotReady = "NotReady"
	// NemoServiceStatusReady indicates that all Nemo services are ready
	NemoServiceStatusReady = "Ready"
	// NemoServiceStatusFailed indicates that one or more Nemo services have failed
	NemoServiceStatusFailed = "Failed"
	// NemoServiceStatusPending indicates that one or more Nemo services are in pending state
	NemoServiceStatusPending = "Pending"
	// NemoServiceStatusUnknown indicates that one or more Nemo services are in unknown state
	NemoServiceStatusUnknown = "Unknown"
)

// NemoServiceSpec defines the desired state of NemoService
type NemoServiceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Customizer configures attributes to deploy a Nemo Customizer service
	Customizer NemoServiceCustomizerSpec `json:"customizer,omitempty"`
	// Datastore configures attributes to deploy a Nemo Datastore service
	Datastore NemoServiceDatastoreSpec `json:"datastore,omitempty"`
	// Evaluator configures attributes to deploy a Nemo Evaluator service
	Evaluator NemoServiceEvaluatorSpec `json:"evaluator,omitempty"`
	// Guardrails configures attributes to deploy a Nemo Guardrails service
	Guardrails NemoServiceGuardrailsSpec `json:"guardrails,omitempty"`
	// Entitystore configures attributes to deploy a Nemo Entitystore service
	Entitystore NemoServiceEntitystoreSpec `json:"entitystore,omitempty"`
}

// NemoServiceCustomizerSpec defines the desired state of NemoCustomizer as part of the NeMoServices
type NemoServiceCustomizerSpec struct {
	Name    string              `json:"name,omitempty"`
	Enabled *bool               `json:"enabled,omitempty"`
	Spec    *NemoCustomizerSpec `json:"spec,omitempty"`
}

// NemoServiceEvaluatorSpec defines the desired state of NemoEvaluator as part of the NeMoServices
type NemoServiceEvaluatorSpec struct {
	Name    string             `json:"name,omitempty"`
	Enabled *bool              `json:"enabled,omitempty"`
	Spec    *NemoEvaluatorSpec `json:"spec,omitempty"`
}

// NemoServiceGuardrailsSpec defines the desired state of NemoGuardrails as part of the NeMoServices
type NemoServiceGuardrailsSpec struct {
	Name    string             `json:"name,omitempty"`
	Enabled *bool              `json:"enabled,omitempty"`
	Spec    *NemoGuardrailSpec `json:"spec,omitempty"`
}

// NemoServiceDatastoreSpec defines the desired state of NemoDatastore as part of the NeMoServices
type NemoServiceDatastoreSpec struct {
	Name    string             `json:"name,omitempty"`
	Enabled *bool              `json:"enabled,omitempty"`
	Spec    *NemoDatastoreSpec `json:"spec,omitempty"`
}

// NemoServiceEntitystoreSpec defines the desired state of NemoEntitystore as part of the NeMoServices
type NemoServiceEntitystoreSpec struct {
	Name    string               `json:"name,omitempty"`
	Enabled *bool                `json:"enabled,omitempty"`
	Spec    *NemoEntitystoreSpec `json:"spec,omitempty"`
}

// NemoServiceStatus defines the observed state of NemoService
type NemoServiceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// States indicate state of individual Nemo services
	States map[string]string `json:"states,omitempty"`
	// State indicates the overall state of the Nemo services
	State string `json:"state,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.state`,priority=0
// +kubebuilder:printcolumn:name="Age",type="date",format="date-time",JSONPath=".metadata.creationTimestamp",priority=0

// NemoService is the Schema for the nemoservices API
type NemoService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NemoServiceSpec   `json:"spec,omitempty"`
	Status NemoServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NemoServiceList contains a list of NemoService
type NemoServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NemoService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NemoService{}, &NemoServiceList{})
}
