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
			vols := tt.nimService.GetVolumes(&tt.modelPVC)
			if !reflect.DeepEqual(vols, tt.desired) {
				t.Errorf("GetVolumes() = %v, want %v", vols, tt.desired)
			}
		})
	}

}
