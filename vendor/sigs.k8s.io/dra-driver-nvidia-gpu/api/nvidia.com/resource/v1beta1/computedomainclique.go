/*
Copyright The Kubernetes Authors.

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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true
// +kubebuilder:resource:scope=Namespaced

// ComputeDomainClique holds information about a specific clique within a ComputeDomain.
// It is created in the driver namespace and named as "<computeDomainUID>.<cliqueID>".
type ComputeDomainClique struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +listType=map
	// +listMapKey=nodeName
	Daemons []*ComputeDomainDaemonInfo `json:"daemons,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ComputeDomainCliqueList provides a list of ComputeDomainCliques.
type ComputeDomainCliqueList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ComputeDomainClique `json:"items"`
}

// ComputeDomainDaemonInfo provides information about each daemon in a ComputeDomainClique.
type ComputeDomainDaemonInfo struct {
	NodeName  string `json:"nodeName"`
	IPAddress string `json:"ipAddress"`
	CliqueID  string `json:"cliqueID"`
	// The Index field is used to ensure a consistent IP-to-DNS name
	// mapping across all machines within an IMEX domain. Each node's index
	// directly determines its DNS name within a given NVLink partition
	// (i.e. clique). In other words, the 2-tuple of (CliqueID, Index) will
	// always be unique. This field is marked as optional (but not
	// omitempty) in order to support downgrades and avoid an API bump.
	// +kubebuilder:validation:Optional
	Index int `json:"index"`
	// The Status field tracks the readiness of the IMEX daemon running on
	// this node. It gets switched to Ready whenever the IMEX daemon is
	// ready to broker GPU memory exchanges and switches to NotReady when
	// it is not. It is marked as optional in order to support downgrades
	// and avoid an API bump.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=Ready;NotReady
	// +kubebuilder:default:=NotReady
	Status string `json:"status,omitempty"`
}
