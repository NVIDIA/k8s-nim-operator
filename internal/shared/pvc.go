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

package shared

import (
	"fmt"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConstructPVC constructs a PVC from the custom spec from the user
func ConstructPVC(pvc appsv1alpha1.PersistentVolumeClaim, pvcMeta metav1.ObjectMeta) (*corev1.PersistentVolumeClaim, error) {
	size, err := resource.ParseQuantity(pvc.Size)
	if err != nil {
		return nil, fmt.Errorf("failed to parse size for pvc creation %s, err %v", pvcMeta.Name, err)
	}
	claim := &corev1.PersistentVolumeClaim{
		ObjectMeta: pvcMeta,
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{pvc.VolumeAccessMode},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: size,
				},
			},
		},
	}

	if pvc.StorageClass != "" {
		claim.Spec.StorageClassName = &pvc.StorageClass
	}

	return claim, nil
}
