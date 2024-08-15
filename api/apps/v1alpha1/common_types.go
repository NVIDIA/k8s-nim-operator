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
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

// Expose defines attributes to expose the service
type Expose struct {
	Service Service `json:"service,omitempty"`
	Ingress Ingress `json:"ingress,omitempty"`
}

// Service defines attributes to create a service
type Service struct {
	Type corev1.ServiceType `json:"type,omitempty"`
	// override the default service name
	Name       string `json:"name,omitempty"`
	OpenAIPort int32  `json:"openaiPort"`
}

// Metrics defines attributes to setup metrics collection
type Metrics struct {
	Enabled *bool `json:"enabled,omitempty"`
	// for use with the Prometheus Operator and the primary service object
	ServiceMonitor ServiceMonitor `json:"serviceMonitor,omitempty"`
}

// ServiceMonitor defines attributes to create a service monitor
type ServiceMonitor struct {
	Create           *bool             `json:"enabled,omitempty"`
	AdditionalLabels map[string]string `json:"additionalLabels,omitempty"`
}

// Autoscaling defines attributes to automatically scale the service based on metrics
type Autoscaling struct {
	Enabled *bool                       `json:"enabled,omitempty"`
	HPA     HorizontalPodAutoscalerSpec `json:"hpa,omitempty"`
}

type HorizontalPodAutoscalerSpec struct {
	MinReplicas *int32                                         `json:"minReplicas,omitempty"`
	MaxReplicas int32                                          `json:"maxReplicas"`
	Metrics     []autoscalingv2.MetricSpec                     `json:"metrics,omitempty"`
	Behavior    *autoscalingv2.HorizontalPodAutoscalerBehavior `json:"behavior,omitempty" `
}

// Image defines image attributes
type Image struct {
	Repository  string   `json:"repository,omitempty"`
	PullPolicy  string   `json:"pullPolicy,omitempty"`
	Tag         string   `json:"tag,omitempty"`
	PullSecrets []string `json:"pullSecrets,omitempty"`
}

// Ingress defines attributes to enable ingress for the service
type Ingress struct {
	// ingress, or virtualService - not both
	Enabled *bool                    `json:"enabled,omitempty"`
	Spec    networkingv1.IngressSpec `json:"spec,omitempty"`
}

// IngressHost defines attributes for ingress host
type IngressHost struct {
	Host  string        `json:"host,omitempty"`
	Paths []IngressPath `json:"paths,omitempty"`
}

// IngressPath defines attributes for ingress paths
type IngressPath struct {
	Path        string                `json:"path,omitempty"`
	PathType    networkingv1.PathType `json:"pathType,omitempty"`
	ServiceType string                `json:"serviceType,omitempty"`
}

type Probe struct {
	Enabled *bool         `json:"enabled,omitempty"`
	Probe   *corev1.Probe `json:"probe,omitempty"`
}
