/*
Copyright 2026.

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
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
)

// TestNIMCacheGetInitContainers tests the GetInitContainers function on NIMCache.
func TestNIMCacheGetInitContainers(t *testing.T) {
	updateCaCertContainer := corev1.Container{
		Name:            "update-ca-certificates",
		Image:           "ngc-puller:latest",
		Command:         k8sutil.GetUpdateCaCertInitContainerCommand(),
		SecurityContext: k8sutil.GetUpdateCaCertInitContainerSecurityContext(),
		VolumeMounts:    k8sutil.GetUpdateCaCertInitContainerVolumeMounts(),
	}

	userInitContainer := &NIMContainerSpec{
		Name:  "user-init",
		Image: Image{Repository: "user-repo", Tag: "v1"},
	}
	userInitContainerDesired := corev1.Container{
		Name:  "user-init",
		Image: "user-repo:v1",
	}

	tests := []struct {
		name     string
		nimCache *NIMCache
		desired  []corev1.Container
	}{
		{
			name: "Proxy is nil - no update-ca-certificates init container",
			nimCache: &NIMCache{
				Spec: NIMCacheSpec{
					Source: NIMSource{
						NGC: &NGCSource{ModelPuller: "ngc-puller:latest"},
					},
				},
			},
			desired: nil,
		},
		{
			name: "Proxy is set but cert config map is empty - no update-ca-certificates init container",
			nimCache: &NIMCache{
				Spec: NIMCacheSpec{
					Source: NIMSource{
						NGC: &NGCSource{ModelPuller: "ngc-puller:latest"},
					},
					Proxy: &ProxySpec{
						HttpsProxy: "https://proxy.example.com:8443",
					},
				},
			},
			desired: nil,
		},
		{
			name: "Proxy is set with cert config map and NGC source - adds update-ca-certificates init container",
			nimCache: &NIMCache{
				Spec: NIMCacheSpec{
					Source: NIMSource{
						NGC: &NGCSource{ModelPuller: "ngc-puller:latest"},
					},
					Proxy: &ProxySpec{CertConfigMap: "proxy-ca-cert"},
				},
			},
			desired: []corev1.Container{updateCaCertContainer},
		},
		{
			name: "Proxy with cert config map combined with user init container",
			nimCache: &NIMCache{
				Spec: NIMCacheSpec{
					Source: NIMSource{
						NGC: &NGCSource{ModelPuller: "ngc-puller:latest"},
					},
					Proxy:          &ProxySpec{CertConfigMap: "proxy-ca-cert"},
					InitContainers: []*NIMContainerSpec{userInitContainer},
				},
			},
			desired: []corev1.Container{updateCaCertContainer, userInitContainerDesired},
		},
		{
			name: "Proxy with empty cert config map and user init container - only user init container",
			nimCache: &NIMCache{
				Spec: NIMCacheSpec{
					Source: NIMSource{
						NGC: &NGCSource{ModelPuller: "ngc-puller:latest"},
					},
					Proxy:          &ProxySpec{HttpsProxy: "https://proxy.example.com:8443"},
					InitContainers: []*NIMContainerSpec{userInitContainer},
				},
			},
			desired: []corev1.Container{userInitContainerDesired},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.nimCache.GetInitContainers()
			if !reflect.DeepEqual(got, tt.desired) {
				t.Errorf("GetInitContainers() = %+v, want %+v", got, tt.desired)
			}
		})
	}
}
