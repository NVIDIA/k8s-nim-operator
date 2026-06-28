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
)

func TestNemoGuardrailValidateConfigStore(t *testing.T) {
	tests := []struct {
		name          string
		nemoGuardrail *NemoGuardrail
		wantErr       bool
	}{
		{
			name: "should accept config map if it is the only config source",
			nemoGuardrail: &NemoGuardrail{
				Spec: NemoGuardrailSpec{
					ConfigStore: GuardrailConfig{
						ConfigMap: &ConfigMapRef{Name: "guardrail-config"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "should accept pvc if it is the only config source",
			nemoGuardrail: &NemoGuardrail{
				Spec: NemoGuardrailSpec{
					ConfigStore: GuardrailConfig{
						PVC: &PersistentVolumeClaim{Name: "guardrail-pvc"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "should reject config store if both config map and pvc are set",
			nemoGuardrail: &NemoGuardrail{
				Spec: NemoGuardrailSpec{
					ConfigStore: GuardrailConfig{
						ConfigMap: &ConfigMapRef{Name: "guardrail-config"},
						PVC:       &PersistentVolumeClaim{Name: "guardrail-pvc"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "should reject config store if both config map and pvc are omitted",
			nemoGuardrail: &NemoGuardrail{
				Spec: NemoGuardrailSpec{
					ConfigStore: GuardrailConfig{},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.nemoGuardrail.ValidateConfigStore()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfigStore() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNemoGuardrailGetVolumes(t *testing.T) {
	tests := []struct {
		name          string
		nemoGuardrail *NemoGuardrail
		want          []corev1.Volume
	}{
		{
			name: "should return config map volume if config map is set",
			nemoGuardrail: &NemoGuardrail{
				Spec: NemoGuardrailSpec{
					ConfigStore: GuardrailConfig{
						ConfigMap: &ConfigMapRef{Name: "guardrail-config"},
					},
				},
			},
			want: []corev1.Volume{
				{
					Name: "config-store",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "guardrail-config"},
						},
					},
				},
			},
		},
		{
			name: "should return pvc volume if pvc is set",
			nemoGuardrail: &NemoGuardrail{
				Spec: NemoGuardrailSpec{
					ConfigStore: GuardrailConfig{
						PVC: &PersistentVolumeClaim{Name: "guardrail-pvc"},
					},
				},
			},
			want: []corev1.Volume{
				{
					Name: "config-store",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "guardrail-pvc",
						},
					},
				},
			},
		},
		{
			name: "should return no volumes if config store is invalid",
			nemoGuardrail: &NemoGuardrail{
				Spec: NemoGuardrailSpec{
					ConfigStore: GuardrailConfig{},
				},
			},
			want: []corev1.Volume{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.nemoGuardrail.GetVolumes()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetVolumes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNemoGuardrailGetVolumeMounts(t *testing.T) {
	tests := []struct {
		name          string
		nemoGuardrail *NemoGuardrail
		want          []corev1.VolumeMount
	}{
		{
			name: "should return config map mount if config map is set",
			nemoGuardrail: &NemoGuardrail{
				Spec: NemoGuardrailSpec{
					ConfigStore: GuardrailConfig{
						ConfigMap: &ConfigMapRef{Name: "guardrail-config"},
					},
				},
			},
			want: []corev1.VolumeMount{
				{
					Name:      "config-store",
					MountPath: "/config-store",
				},
			},
		},
		{
			name: "should return pvc mount with default subpath if pvc is set",
			nemoGuardrail: &NemoGuardrail{
				Spec: NemoGuardrailSpec{
					ConfigStore: GuardrailConfig{
						PVC: &PersistentVolumeClaim{Name: "guardrail-pvc"},
					},
				},
			},
			want: []corev1.VolumeMount{
				{
					Name:      "config-store",
					MountPath: "/config-store",
					SubPath:   "guardrails-config-store",
				},
			},
		},
		{
			name: "should return no mounts if config store is invalid",
			nemoGuardrail: &NemoGuardrail{
				Spec: NemoGuardrailSpec{
					ConfigStore: GuardrailConfig{},
				},
			},
			want: []corev1.VolumeMount{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.nemoGuardrail.GetVolumeMounts()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetVolumeMounts() = %v, want %v", got, tt.want)
			}
		})
	}
}
