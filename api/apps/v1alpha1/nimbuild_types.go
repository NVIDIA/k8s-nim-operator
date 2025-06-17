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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NIMBuildSpec to build optimized trtllm engines with given model config and weights.
type NIMBuildSpec struct {
	// Required: Reference to the model weights from NIMCache
	NIMCacheRef string `json:"nimCacheRef"`
	// Profile name for this engine build
	Profile string `json:"profile,omitempty"`
	// Any user-defined tags for tracking builds
	Tags map[string]string `json:"tags,omitempty"`
	// Additional build params
	BuildParams []string `json:"additionalBuildParams,omitempty"`
	// Resources is the resource requirements for the NIMBuild pod.
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// Tolerations for running the job to cache the NIM model
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// NodeSelector is the node selector labels to schedule the caching job.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

// NIMBuildStatus defines the observed state of NIMBuild.
type NIMBuildStatus struct {
	State      string             `json:"state,omitempty"`
	Profiles   []NIMProfile       `json:"profiles,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.state`,priority=0
// +kubebuilder:printcolumn:name="Age",type="date",format="date-time",JSONPath=".metadata.creationTimestamp",priority=0

// NIMBuild is the Schema for the nimcaches API.
type NIMBuild struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NIMBuildSpec   `json:"spec,omitempty"`
	Status NIMBuildStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// NIMBuildList contains a list of NIMBuild.
type NIMBuildList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NIMBuild `json:"items"`
}

const (
	// NimBuildConditionWaitForNimCache indicates NIMBuild progress is blocked until that the caching is complete.
	NimBuildConditionWaitForNimCache = "NIM_BUILD_WAIT_FOR_NIM_CACHE_READY"
	// NimBuildConditionReconcileFailed indicated that error occurred while reconciling NIMBuild object.
	NimBuildConditionReconcileFailed = "NIM_BUILD_RECONCILE_FAILED"
	// NimBuildConditionMultipleBuildableProfilesFound indicates that multiple buildable profiles are found for the NIMCache object.
	NimBuildConditionMultipleBuildableProfilesFound = "NIM_BUILD_MULTIPLE_BUILDABLE_PROFILES_FOUND"
	// NimBuildConditionSingleBuildableProfilesFound indicates that only one buildable profile is found for the NIMCache object.
	NimBuildConditionSingleBuildableProfilesFound = "NIM_BUILD_SINGLE_BUILDABLE_PROFILE_FOUND"
	// NimBuildConditionNoBuildableProfilesFound indicates that no buildable profiles are found for the NIMCache object.
	NimBuildConditionNoBuildableProfilesFound = "NIM_BUILD_NO_BUILDABLE_PROFILE_FOUND"

	// NimBuildConditionEngineBuildPodCreated indicates that the engine build pod is created.
	NimBuildConditionEngineBuildPodCreated = "NIM_BUILD_ENGINE_BUILD_POD_CREATED"
	// NimBuildConditionEngineBuildJobCompleted indicates that the engine build pod is completed.
	NimBuildConditionEngineBuildPodCompleted = "NIM_BUILD_ENGINE_BUILD_POD_COMPLETED"
	// NimBuildConditionEngineBuildPodPending indicates that the engine build pod is in pending state.
	NimBuildConditionEngineBuildPodPending = "NIM_BUILD_ENGINE_BUILD_POD_PENDING"
	// NimBuildConditionModelManifestPodCompleted indicates that the model manifest pod is in completed state.
	NimBuildConditionModelManifestPodCompleted = "NIM_BUILD_MODEL_MANIFEST_POD_COMPLETED"

	NimBuildConditionNIMCacheNotFound = "NIM_BUILD_NIM_CACHE_NOT_FOUND"

	NimBuildConditionNimCacheFailed = "NIM_BUILD_NIM_CACHE_FAILED"

	// NimBuildStatusNotReady indicates that build is not ready.
	NimBuildStatusNotReady = "NotReady"

	// NimBuildStatusStarted indicates that caching process is started.
	NimBuildStatusStarted = "Started"
	// NimBuildStatusReady indicates that cache is ready.
	NimBuildStatusReady = "Ready"
	// NimBuildStatusInProgress indicates that caching is in progress.
	NimBuildStatusInProgress = "InProgress"
	// NimBuildStatusPending indicates that building is not yet started.
	NimBuildStatusPending = "Pending"
	// NimBuildStatusFailed indicates that caching is failed.
	NimBuildStatusFailed = "Failed"
)

func init() {
	SchemeBuilder.Register(&NIMBuild{}, &NIMBuildList{})
}

// GetTolerations returns tolerations configured for the NIMBuild Pod.
func (n *NIMBuild) GetTolerations() []corev1.Toleration {
	return n.Spec.Tolerations
}

// GetNodeSelectors returns nodeselectors configured for the NIMBuild Pod.
func (n *NIMBuild) GetNodeSelectors() map[string]string {
	return n.Spec.NodeSelector
}
