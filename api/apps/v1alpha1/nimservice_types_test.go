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

func TestHTTpRoute(t *testing.T) {
	enabled := true
	prefixTypeMatch := gatewayv1.PathMatchPathPrefix
	root := "/"
	var port gatewayv1.PortNumber = DefaultAPIPort

	tests := []struct {
		name          string
		nimSpec       *HTTPRoute
		desiredGWSpec gatewayv1.HTTPRouteSpec
	}{
		{
			name: "should return empty gatewayv1.HTTPRouteSpec if HTTPRouteSpec is nil",
			nimSpec: &HTTPRoute{
				Enabled: &enabled,
				Spec:    nil,
			},
			desiredGWSpec: gatewayv1.HTTPRouteSpec{},
		},
		{
			name: "should correctly translate to gatewayv1.HTTPRouteSpec",
			nimSpec: &HTTPRoute{
				Enabled: &enabled,
				Spec: &HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name: "istio-gateway",
							},
						},
					},
					Host: "foobar.nim",
					Paths: []HTTPPathMatch{
						{
							Type:  &prefixTypeMatch,
							Value: &root,
						},
					},
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
					"foobar.nim",
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
			gwSpec := tt.nimSpec.GenerateGatewayHTTPRouteSpec("test")
			if !reflect.DeepEqual(gwSpec, tt.desiredGWSpec) {
				t.Errorf("GenerateGatewayHTTPRouteSpec() = %+v, want %+v", gwSpec, tt.desiredGWSpec)
			}
		})
	}
}
