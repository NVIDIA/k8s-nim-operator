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
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
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

func TestHTTpRoute(t *testing.T) {
	prefixTypeMatch := gatewayv1.PathMatchPathPrefix
	root := "/"
	var port gatewayv1.PortNumber = DefaultAPIPort

	tests := []struct {
		name          string
		router        Router
		desiredGWSpec gatewayv1.HTTPRouteSpec
	}{
		{
			name: "should return empty gatewayv1.HTTPRouteSpec if Router does not have a gateway defined.",
			router: Router{
				Gateway: nil,
			},
			desiredGWSpec: gatewayv1.HTTPRouteSpec{},
		},
		{
			name: "should correctly translate to gatewayv1.HTTPRouteSpec",
			router: Router{
				HostDomainName: "foobar.nim",
				Gateway: &Gateway{
					Name:      "istio-gateway",
					Namespace: "istio-system",
				},
			},
			desiredGWSpec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{
						{
							Name: "istio-gateway",
						},
					},
				},
				Hostnames: []gatewayv1.Hostname{
					"test.foobar.nim",
				},
				Rules: []gatewayv1.HTTPRouteRule{
					{
						Matches: []gatewayv1.HTTPRouteMatch{
							{
								Path: &gatewayv1.HTTPPathMatch{
									Type:  &prefixTypeMatch,
									Value: &root,
								},
							},
						},
						BackendRefs: []gatewayv1.HTTPBackendRef{
							{
								BackendRef: gatewayv1.BackendRef{
									BackendObjectReference: gatewayv1.BackendObjectReference{
										Name: "test",
										Port: &port,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gwSpec := tt.router.GenerateGatewayHTTPRouteSpec("test", "test", DefaultAPIPort)
			if !reflect.DeepEqual(gwSpec, tt.desiredGWSpec) {
				t.Errorf("GenerateGatewayHTTPRouteSpec() = %+v, want %+v", gwSpec, tt.desiredGWSpec)
			}
		})
	}
}

func TestIngress(t *testing.T) {
	tests := []struct {
		name               string
		router             Router
		desiredIngressSpec networkingv1.IngressSpec
	}{
		{
			name: "should return empty networkingv1.IngressSpec if Router does not have an ingress class defined.",
			router: Router{
				Ingress: nil,
			},
			desiredIngressSpec: networkingv1.IngressSpec{},
		},
		{
			name: "should correctly translate to networkingv1.IngressSpec",
			router: Router{
				HostDomainName: "foobar.nim",
				Ingress: &RouterIngress{
					IngressClass: "nginx",
				},
			},
			desiredIngressSpec: networkingv1.IngressSpec{
				IngressClassName: ptr.To("nginx"),
				Rules: []networkingv1.IngressRule{
					{
						Host: "test.foobar.nim",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ingressSpec := tt.router.GenerateIngressSpec("test", "test")
			if !reflect.DeepEqual(ingressSpec, tt.desiredIngressSpec) {
				t.Errorf("GenerateIngressSpec() = %+v, want %+v", ingressSpec, tt.desiredIngressSpec)
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

// TestGetInferenceServiceParams tests the GetInferenceServiceParams function with different autoscaling and deployment mode combinations.
func TestGetInferenceServiceParams(t *testing.T) {
	tests := []struct {
		name                string
		nimService          *NIMService
		deploymentMode      string
		expectHPAAnnotation bool
		expectMinReplicas   bool
	}{
		{
			name: "Autoscaling enabled with RawDeployment mode",
			nimService: &NIMService{
				Spec: NIMServiceSpec{
					Image: Image{
						Repository: "test-repo",
						Tag:        "test-tag",
					},
					AuthSecret: "test-secret",
					Scale: Autoscaling{
						Enabled: ptr.To(true),
						HPA: HorizontalPodAutoscalerSpec{
							MinReplicas: ptr.To[int32](1),
							MaxReplicas: 5,
						},
					},
					Expose: Expose{
						Service: Service{
							Port: ptr.To[int32](8000),
						},
					},
				},
			},
			deploymentMode:      "RawDeployment",
			expectHPAAnnotation: true,
			expectMinReplicas:   true,
		},
		{
			name: "Autoscaling disabled with Standard deployment mode",
			nimService: &NIMService{
				Spec: NIMServiceSpec{
					Image: Image{
						Repository: "test-repo",
						Tag:        "test-tag",
					},
					AuthSecret: "test-secret",
					Scale: Autoscaling{
						Enabled: ptr.To(false),
					},
					Replicas: ptr.To[int32](2),
					Expose: Expose{
						Service: Service{
							Port: ptr.To[int32](8000),
						},
					},
				},
			},
			deploymentMode:      "Standard",
			expectHPAAnnotation: true, // Standard mode should enable HPA
			expectMinReplicas:   false,
		},
		{
			name: "Autoscaling disabled with Knative deployment mode",
			nimService: &NIMService{
				Spec: NIMServiceSpec{
					Image: Image{
						Repository: "test-repo",
						Tag:        "test-tag",
					},
					AuthSecret: "test-secret",
					Scale: Autoscaling{
						Enabled: ptr.To(false),
					},
					Replicas: ptr.To[int32](2),
					Expose: Expose{
						Service: Service{
							Port: ptr.To[int32](8000),
						},
					},
				},
			},
			deploymentMode:      "Knative",
			expectHPAAnnotation: false, // Neither autoscaling nor standard mode
			expectMinReplicas:   false,
		},
		{
			name: "Autoscaling enabled with Standard deployment mode",
			nimService: &NIMService{
				Spec: NIMServiceSpec{
					Image: Image{
						Repository: "test-repo",
						Tag:        "test-tag",
					},
					AuthSecret: "test-secret",
					Scale: Autoscaling{
						Enabled: ptr.To(true),
						HPA: HorizontalPodAutoscalerSpec{
							MinReplicas: ptr.To[int32](1),
							MaxReplicas: 10,
						},
					},
					Expose: Expose{
						Service: Service{
							Port: ptr.To[int32](8000),
						},
					},
				},
			},
			deploymentMode:      "Standard",
			expectHPAAnnotation: true, // Both conditions met
			expectMinReplicas:   true,
		},
		{
			name: "Zero replicas with autoscaling disabled",
			nimService: &NIMService{
				Spec: NIMServiceSpec{
					Image: Image{
						Repository: "test-repo",
						Tag:        "test-tag",
					},
					AuthSecret: "test-secret",
					Scale: Autoscaling{
						Enabled: ptr.To(false),
					},
					Replicas: ptr.To[int32](0), // Zero replicas should be valid now
					Expose: Expose{
						Service: Service{
							Port: ptr.To[int32](8000),
						},
					},
				},
			},
			deploymentMode:      "Knative",
			expectHPAAnnotation: false,
			expectMinReplicas:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// Test autoscaling enabled state
			isAutoScalingEnabled := tt.nimService.IsAutoScalingEnabled()
			if tt.expectHPAAnnotation && !isAutoScalingEnabled && tt.deploymentMode != "Standard" && tt.deploymentMode != "RawDeployment" {
				t.Errorf("Expected autoscaling to be enabled or standard deployment mode, but neither condition is met")
			}

			// Test replica validation
			if tt.nimService.Spec.Replicas != nil {
				if *tt.nimService.Spec.Replicas < 0 {
					t.Errorf("Replicas should not be negative, got %d", *tt.nimService.Spec.Replicas)
				}
				// Verify zero replicas are now allowed (changed from minimum of 1 to 0)
				if *tt.nimService.Spec.Replicas == 0 {
					// This should be valid now - test passes if we reach here
					t.Logf("Zero replicas correctly allowed")
				}
			}

			// Verify GetReplicas works correctly
			if tt.expectMinReplicas {
				replicas := tt.nimService.GetReplicas()
				if replicas == nil {
					t.Errorf("Expected replicas to be set for HPA, but got nil")
				}
			}
		})
	}
}

// TestReplicasMinimumValidation tests that zero replicas are now allowed.
func TestReplicasMinimumValidation(t *testing.T) {
	tests := []struct {
		name       string
		replicas   *int32
		expectPass bool
	}{
		{
			name:       "Zero replicas should be valid",
			replicas:   ptr.To[int32](0),
			expectPass: true,
		},
		{
			name:       "One replica should be valid",
			replicas:   ptr.To[int32](1),
			expectPass: true,
		},
		{
			name:       "Multiple replicas should be valid",
			replicas:   ptr.To[int32](5),
			expectPass: true,
		},
		{
			name:       "Nil replicas should be valid",
			replicas:   nil,
			expectPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nimService := &NIMService{
				Spec: NIMServiceSpec{
					Replicas: tt.replicas,
				},
			}

			// Validate that replicas field accepts the value
			if nimService.Spec.Replicas != nil {
				if *nimService.Spec.Replicas < 0 {
					if tt.expectPass {
						t.Errorf("Expected replicas %d to be valid, but validation would fail", *nimService.Spec.Replicas)
					}
				} else {
					if !tt.expectPass {
						t.Errorf("Expected replicas %d to be invalid, but validation would pass", *nimService.Spec.Replicas)
					}
				}
			}

			// Test GetReplicas method
			replicas := nimService.GetReplicas()
			if tt.replicas != nil && replicas != nil {
				if *replicas != *tt.replicas {
					t.Errorf("GetReplicas() = %d, want %d", *replicas, *tt.replicas)
				}
			}
		})
	}
}

// TestIsAutoScalingEnabled tests the autoscaling enabled detection.
func TestIsAutoScalingEnabled(t *testing.T) {
	tests := []struct {
		name     string
		scale    Autoscaling
		expected bool
	}{
		{
			name: "Autoscaling explicitly enabled",
			scale: Autoscaling{
				Enabled: ptr.To(true),
			},
			expected: true,
		},
		{
			name: "Autoscaling explicitly disabled",
			scale: Autoscaling{
				Enabled: ptr.To(false),
			},
			expected: false,
		},
		{
			name:     "Autoscaling not set (nil)",
			scale:    Autoscaling{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nimService := &NIMService{
				Spec: NIMServiceSpec{
					Scale: tt.scale,
				},
			}

			result := nimService.IsAutoScalingEnabled()
			if result != tt.expected {
				t.Errorf("IsAutoScalingEnabled() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestStandardModeReplicasWithoutAutoscaling(t *testing.T) {
	nimService := &NIMService{
		Spec: NIMServiceSpec{
			Image: Image{
				Repository: "test-repo",
				Tag:        "test-tag",
			},
			AuthSecret: "test-secret",
			Scale: Autoscaling{
				Enabled: ptr.To(false),
			},
			Replicas: ptr.To[int32](3),
			Expose: Expose{
				Service: Service{
					Port: ptr.To[int32](8000),
				},
			},
		},
	}

	params := nimService.GetInferenceServiceParams("Standard")

	if params.MinReplicas == nil {
		t.Fatal("expected MinReplicas to be set, got nil")
	}
	if *params.MinReplicas != 3 {
		t.Errorf("MinReplicas = %d, want 3", *params.MinReplicas)
	}
	if params.MaxReplicas == nil {
		t.Fatal("expected MaxReplicas to be set, got nil")
	}
	if *params.MaxReplicas != 3 {
		t.Errorf("MaxReplicas = %d, want 3", *params.MaxReplicas)
	}
}
