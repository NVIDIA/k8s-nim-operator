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
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// TestGetVolumes tests the GetVolumes function.
func TestGetVolumes(t *testing.T) {
	tests := []struct {
		name       string
		modelPVC   PersistentVolumeClaim
		desired    []corev1.Volume
		nimService *NIMService
	}{
		{
			name:       "Storage read only is nil",
			nimService: &NIMService{Spec: NIMServiceSpec{Storage: NIMServiceStorage{}}},
			modelPVC:   PersistentVolumeClaim{Name: "test-pvc"},
			desired: []corev1.Volume{
				{
					Name: "dshm",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							Medium: corev1.StorageMediumMemory,
						},
					},
				},
				{
					Name: "model-store",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "test-pvc",
							ReadOnly:  false,
						},
					},
				},
			},
		},
		{
			name:       "Storage read only is false",
			nimService: &NIMService{Spec: NIMServiceSpec{Storage: NIMServiceStorage{ReadOnly: &[]bool{false}[0]}}},
			modelPVC:   PersistentVolumeClaim{Name: "test-pvc"},
			desired: []corev1.Volume{
				{
					Name: "dshm",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							Medium: corev1.StorageMediumMemory,
						},
					},
				},
				{
					Name: "model-store",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "test-pvc",
							ReadOnly:  false,
						},
					},
				},
			},
		},
		{
			name:       "Storage read only is true",
			nimService: &NIMService{Spec: NIMServiceSpec{Storage: NIMServiceStorage{ReadOnly: &[]bool{true}[0]}}},
			modelPVC:   PersistentVolumeClaim{Name: "test-pvc"},
			desired: []corev1.Volume{
				{
					Name: "dshm",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							Medium: corev1.StorageMediumMemory,
						},
					},
				},
				{
					Name: "model-store",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "test-pvc",
							ReadOnly:  true,
						},
					},
				},
			},
		},
		{
			name:       "Proxy Spec is set and cert config map is set",
			nimService: &NIMService{Spec: NIMServiceSpec{Proxy: &ProxySpec{CertConfigMap: "proxy-ca-cert"}}},
			desired: []corev1.Volume{
				{
					Name: "proxy-ca-cert",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "proxy-ca-cert",
							},
						},
					},
				},
			},
		},
		{
			name:       "Proxy Spec is set and cert config map is empty",
			nimService: &NIMService{Spec: NIMServiceSpec{Proxy: &ProxySpec{CertConfigMap: ""}}},
			desired:    []corev1.Volume{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vols := tt.nimService.GetVolumes(&tt.modelPVC)
			if !reflect.DeepEqual(vols, tt.desired) {
				t.Errorf("GetVolumes() = %v, want %v", vols, tt.desired)
			}
		})
	}

}

// TestGetVolumesWithEmptyDir tests the GetVolumes function with EmptyDir storage.
func TestGetVolumesWithEmptyDir(t *testing.T) {
	sizeLimit := resource.MustParse("10Gi")
	tests := []struct {
		name       string
		nimService *NIMService
		modelPVC   *PersistentVolumeClaim
		desired    []corev1.Volume
	}{
		{
			name: "EmptyDir storage with size limit",
			nimService: &NIMService{
				Spec: NIMServiceSpec{
					Storage: NIMServiceStorage{
						EmptyDir: &EmptyDirSpec{
							SizeLimit: &sizeLimit,
						},
					},
				},
			},
			modelPVC: nil,
			desired: []corev1.Volume{
				{
					Name: "dshm",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							Medium: corev1.StorageMediumMemory,
						},
					},
				},
				{
					Name: "model-store",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							SizeLimit: &sizeLimit,
						},
					},
				},
			},
		},
		{
			name: "EmptyDir storage without size limit",
			nimService: &NIMService{
				Spec: NIMServiceSpec{
					Storage: NIMServiceStorage{
						EmptyDir: &EmptyDirSpec{},
					},
				},
			},
			modelPVC: nil,
			desired: []corev1.Volume{
				{
					Name: "dshm",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							Medium: corev1.StorageMediumMemory,
						},
					},
				},
				{
					Name: "model-store",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vols := tt.nimService.GetVolumes(tt.modelPVC)
			if !reflect.DeepEqual(vols, tt.desired) {
				t.Errorf("GetVolumes() = %v, want %v", vols, tt.desired)
			}
		})
	}
}

// TestGetVolumeMounts tests the GetVolumeMounts function.
func TestGetVolumeMounts(t *testing.T) {
	tests := []struct {
		name       string
		nimService *NIMService
		modelPVC   *PersistentVolumeClaim
		desired    []corev1.VolumeMount
	}{
		{
			name: "Volume mounts without subpath",
			nimService: &NIMService{
				Spec: NIMServiceSpec{
					Storage: NIMServiceStorage{},
				},
			},
			modelPVC: &PersistentVolumeClaim{Name: "test-pvc"},
			desired: []corev1.VolumeMount{
				{
					Name:      "model-store",
					MountPath: "/model-store",
				},
				{
					Name:      "dshm",
					MountPath: "/dev/shm",
				},
			},
		},
		{
			name: "Volume mounts with subpath",
			nimService: &NIMService{
				Spec: NIMServiceSpec{
					Storage: NIMServiceStorage{},
				},
			},
			modelPVC: &PersistentVolumeClaim{
				Name:    "test-pvc",
				SubPath: "models",
			},
			desired: []corev1.VolumeMount{
				{
					Name:      "model-store",
					MountPath: "/model-store",
					SubPath:   "models",
				},
				{
					Name:      "dshm",
					MountPath: "/dev/shm",
				},
			},
		},
		{
			name: "Volume mounts with proxy cert config map",
			nimService: &NIMService{
				Spec: NIMServiceSpec{
					Proxy: &ProxySpec{CertConfigMap: "proxy-ca-cert"},
				},
			},
			modelPVC: &PersistentVolumeClaim{Name: "test-pvc"},
			desired: []corev1.VolumeMount{
				{
					Name:      "proxy-ca-cert",
					MountPath: "/etc/ssl/certs",
				},
			},
		},
		{
			name: "Volume mounts without proxy cert config map",
			nimService: &NIMService{
				Spec: NIMServiceSpec{
					Proxy: &ProxySpec{CertConfigMap: ""},
				},
			},
			modelPVC: &PersistentVolumeClaim{Name: "test-pvc"},
			desired: []corev1.VolumeMount{
				{
					Name:      "model-store",
					MountPath: "/model-store",
				},
				{
					Name:      "dshm",
					MountPath: "/dev/shm",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mounts := tt.nimService.GetVolumeMounts(tt.modelPVC)
			if !reflect.DeepEqual(mounts, tt.desired) {
				t.Errorf("GetVolumeMounts() = %v, want %v", mounts, tt.desired)
			}
		})
	}
}

// TestGetProxyEnv tests the GetProxyEnv function.
func TestGetProxyEnv(t *testing.T) {
	tests := []struct {
		name       string
		nimService *NIMService
		wantEnvs   map[string]string
	}{
		{
			name: "Proxy environment variables with cert config map",
			nimService: &NIMService{
				Spec: NIMServiceSpec{
					Proxy: &ProxySpec{
						HttpsProxy:    "https://proxy.example.com:8443",
						HttpProxy:     "http://proxy.example.com:8080",
						NoProxy:       "localhost,127.0.0.1",
						CertConfigMap: "proxy-ca-cert",
					},
				},
			},
			wantEnvs: map[string]string{
				"HTTPS_PROXY":            "https://proxy.example.com:8443",
				"HTTP_PROXY":             "http://proxy.example.com:8080",
				"NO_PROXY":               "localhost,127.0.0.1",
				"https_proxy":            "https://proxy.example.com:8443",
				"http_proxy":             "http://proxy.example.com:8080",
				"no_proxy":               "localhost,127.0.0.1",
				"NIM_SDK_USE_NATIVE_TLS": "1",
			},
		},
		{
			name: "Proxy environment variables without cert config map",
			nimService: &NIMService{
				Spec: NIMServiceSpec{
					Proxy: &ProxySpec{
						HttpsProxy:    "https://proxy.example.com:8443",
						HttpProxy:     "http://proxy.example.com:8080",
						NoProxy:       "localhost,127.0.0.1",
						CertConfigMap: "",
					},
				},
			},
			wantEnvs: map[string]string{
				"HTTPS_PROXY":            "https://proxy.example.com:8443",
				"HTTP_PROXY":             "http://proxy.example.com:8080",
				"NO_PROXY":               "localhost,127.0.0.1",
				"https_proxy":            "https://proxy.example.com:8443",
				"http_proxy":             "http://proxy.example.com:8080",
				"no_proxy":               "localhost,127.0.0.1",
				"NIM_SDK_USE_NATIVE_TLS": "1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envs := tt.nimService.GetProxyEnv()
			foundEnvs := make(map[string]string)
			for _, env := range envs {
				foundEnvs[env.Name] = env.Value
			}
			for wantKey, wantValue := range tt.wantEnvs {
				if gotValue, ok := foundEnvs[wantKey]; !ok {
					t.Errorf("GetProxyEnv() missing env var: %s", wantKey)
				} else if gotValue != wantValue {
					t.Errorf("GetProxyEnv() %s = %s, want %s", wantKey, gotValue, wantValue)
				}
			}
		})
	}
}

// TestGetProxySpec tests the GetProxySpec function.
func TestGetProxySpec(t *testing.T) {
	tests := []struct {
		name       string
		nimService *NIMService
		wantNil    bool
	}{
		{
			name: "With proxy spec",
			nimService: &NIMService{
				Spec: NIMServiceSpec{
					Proxy: &ProxySpec{
						HttpsProxy: "https://proxy.example.com:8443",
					},
				},
			},
			wantNil: false,
		},
		{
			name: "Without proxy spec",
			nimService: &NIMService{
				Spec: NIMServiceSpec{},
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.nimService.GetProxySpec()
			if (got == nil) != tt.wantNil {
				t.Errorf("GetProxySpec() nil = %v, want nil = %v", got == nil, tt.wantNil)
			}
		})
	}
}

// TestGetProxyCertConfigMap tests the GetProxyCertConfigMap function.
func TestGetProxyCertConfigMap(t *testing.T) {
	tests := []struct {
		name       string
		nimService *NIMService
		want       string
	}{
		{
			name: "With proxy spec and cert configmap",
			nimService: &NIMService{
				Spec: NIMServiceSpec{
					Proxy: &ProxySpec{
						HttpsProxy:    "https://proxy.example.com:8443",
						CertConfigMap: "proxy-ca-cert",
					},
				},
			},
			want: "proxy-ca-cert",
		},
		{
			name: "With proxy spec but empty cert configmap",
			nimService: &NIMService{
				Spec: NIMServiceSpec{
					Proxy: &ProxySpec{
						HttpsProxy:    "https://proxy.example.com:8443",
						CertConfigMap: "",
					},
				},
			},
			want: "",
		},
		{
			name: "Without proxy spec",
			nimService: &NIMService{
				Spec: NIMServiceSpec{},
			},
			want: "",
		},
		{
			name: "With nil proxy spec",
			nimService: &NIMService{
				Spec: NIMServiceSpec{
					Proxy: nil,
				},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.nimService.GetProxyCertConfigMap()
			if got != tt.want {
				t.Errorf("GetProxyCertConfigMap() = %s, want %s", got, tt.want)
			}
		})
	}
}
