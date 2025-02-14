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
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// assertEqual checks if the actual result matches the expected result.
func assertEqual(t *testing.T, actual, expected intstr.IntOrString) {
	if actual != expected {
		t.Errorf("Got %v, expected %v", actual, expected)
	}
}

func TestGetProbePort(t *testing.T) {
	tests := []struct {
		name        string
		serviceSpec Service
		expected    intstr.IntOrString
	}{
		{"Single named port", Service{Ports: []corev1.ServicePort{{Name: "api", Port: 8080}}}, intstr.FromString("api")},
		{"Single unnamed port", Service{Ports: []corev1.ServicePort{{Port: 9090}}}, intstr.FromInt(9090)},
		{"Multiple ports - prefers 'api'", Service{Ports: []corev1.ServicePort{{Name: "metrics", Port: 9090}, {Name: "api", Port: 8080}}}, intstr.FromString("api")},
		{"No ports - uses legacy Port field", Service{Port: 7070}, intstr.FromString("api")},
		{"No ports at all - defaults to 'api'", Service{}, intstr.FromString("api")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertEqual(t, getProbePort(tt.serviceSpec), tt.expected)
		})
	}
}

func TestGetMetricsPort(t *testing.T) {
	tests := []struct {
		name        string
		serviceSpec Service
		expected    intstr.IntOrString
	}{
		{"Single named port", Service{Ports: []corev1.ServicePort{{Name: "metrics", Port: 8081}}}, intstr.FromString("metrics")},
		{"Single unnamed port", Service{Ports: []corev1.ServicePort{{Port: 8181}}}, intstr.FromInt(8181)},
		{"Multiple ports - prefers 'metrics'", Service{Ports: []corev1.ServicePort{{Name: "api", Port: 8080}, {Name: "metrics", Port: 9090}}}, intstr.FromString("metrics")},
		{"Multiple ports - no 'metrics', uses 'api'", Service{Ports: []corev1.ServicePort{{Name: "grpc", Port: 5050}, {Name: "api", Port: 8080}}}, intstr.FromString("api")},
		{"No ports - uses legacy Port field", Service{Port: 6060}, intstr.FromString("api")},
		{"No ports at all - defaults to 'metrics'", Service{}, intstr.FromString("api")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertEqual(t, getMetricsPort(tt.serviceSpec), tt.expected)
		})
	}
}
