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
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
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
	Name string `json:"name,omitempty"`
	// Deprecated: Use Ports instead.
	// +kubebuilder:deprecatedversion
	// +kubebuilder:default=8000
	Port int32 `json:"port,omitempty"`
	// Defines multiple ports for the service
	Ports       []corev1.ServicePort `json:"ports,omitempty"`
	Annotations map[string]string    `json:"annotations,omitempty"`
}

// Metrics defines attributes to setup metrics collection
type Metrics struct {
	Enabled *bool `json:"enabled,omitempty"`
	// for use with the Prometheus Operator and the primary service object
	ServiceMonitor ServiceMonitor `json:"serviceMonitor,omitempty"`
}

// ServiceMonitor defines attributes to create a service monitor
type ServiceMonitor struct {
	AdditionalLabels map[string]string `json:"additionalLabels,omitempty"`
	Annotations      map[string]string `json:"annotations,omitempty"`
	Interval         promv1.Duration   `json:"interval,omitempty"`
	ScrapeTimeout    promv1.Duration   `json:"scrapeTimeout,omitempty"`
}

// Autoscaling defines attributes to automatically scale the service based on metrics
type Autoscaling struct {
	Enabled     *bool                       `json:"enabled,omitempty"`
	HPA         HorizontalPodAutoscalerSpec `json:"hpa,omitempty"`
	Annotations map[string]string           `json:"annotations,omitempty"`
}

// HorizontalPodAutoscalerSpec defines the parameters required to setup HPA
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
	Enabled     *bool                    `json:"enabled,omitempty"`
	Annotations map[string]string        `json:"annotations,omitempty"`
	Spec        networkingv1.IngressSpec `json:"spec,omitempty"`
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

// Probe defines attributes for startup/liveness/readiness probes
type Probe struct {
	Enabled *bool         `json:"enabled,omitempty"`
	Probe   *corev1.Probe `json:"probe,omitempty"`
}

// CertConfig defines the configuration for custom certificates.
type CertConfig struct {
	// Name of the ConfigMap containing the certificate data.
	Name string `json:"name"`
	// MountPath is the path where the certificates should be mounted in the container.
	MountPath string `json:"mountPath"`
}
