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
	"k8s.io/utils/ptr"
)

const (
	// DefaultAPIPort is the default api  port
	DefaultAPIPort = 8000
	// DefaultNamedPortAPI is the default name for api port
	DefaultNamedPortAPI = "api"
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
	// Port is the main api serving port (default: 8000)
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:default:=8000
	Port        *int32            `json:"port,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ExposeV1 defines attributes to expose the service
type ExposeV1 struct {
	Service Service   `json:"service,omitempty"`
	Ingress IngressV1 `json:"ingress,omitempty"`
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
	Repository  string   `json:"repository"`
	PullPolicy  string   `json:"pullPolicy,omitempty"`
	Tag         string   `json:"tag"`
	PullSecrets []string `json:"pullSecrets,omitempty"`
}

// Ingress defines attributes to enable ingress for the service
type Ingress struct {
	// ingress, or virtualService - not both
	Enabled     *bool                    `json:"enabled,omitempty"`
	Annotations map[string]string        `json:"annotations,omitempty"`
	Spec        networkingv1.IngressSpec `json:"spec,omitempty"`
}

// IngressV1 defines attributes for ingress
//
// +kubebuilder:validation:XValidation:rule="(has(self.spec) && has(self.enabled) && self.enabled) || !has(self.enabled) || !self.enabled", message="spec cannot be nil when ingress is enabled"
type IngressV1 struct {
	Enabled     *bool             `json:"enabled,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Spec        *IngressSpec      `json:"spec,omitempty"`
}

func (i *IngressV1) GenerateNetworkingV1IngressSpec(name string) networkingv1.IngressSpec {
	if i.Spec == nil {
		return networkingv1.IngressSpec{}
	}

	ingressSpec := networkingv1.IngressSpec{
		IngressClassName: &i.Spec.IngressClassName,
		Rules: []networkingv1.IngressRule{
			{
				Host: i.Spec.Host,
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{},
				},
			},
		},
	}

	svcBackend := networkingv1.IngressBackend{
		Service: &networkingv1.IngressServiceBackend{
			Name: name,
			Port: networkingv1.ServiceBackendPort{
				Name: DefaultNamedPortAPI,
			},
		},
	}
	if len(i.Spec.Paths) == 0 {
		ingressSpec.Rules[0].HTTP.Paths = append(ingressSpec.Rules[0].HTTP.Paths, networkingv1.HTTPIngressPath{
			Path:     "/",
			PathType: ptr.To(networkingv1.PathTypePrefix),
			Backend:  svcBackend,
		})
	}
	for _, path := range i.Spec.Paths {
		ingressSpec.Rules[0].HTTP.Paths = append(ingressSpec.Rules[0].HTTP.Paths, networkingv1.HTTPIngressPath{
			Path:     path.Path,
			PathType: path.PathType,
			Backend:  svcBackend,
		})
	}
	return ingressSpec
}

type IngressSpec struct {
	// +kubebuilder:validation:Pattern=`[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*`
	IngressClassName string        `json:"ingressClassName"`
	Host             string        `json:"host,omitempty"`
	Paths            []IngressPath `json:"paths,omitempty"`
}

// IngressPath defines attributes for ingress paths
type IngressPath struct {
	// +kubebuilder:default="/"
	Path string `json:"path,omitempty"`
	// +kubebuilder:default=Prefix
	PathType *networkingv1.PathType `json:"pathType,omitempty"`
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
