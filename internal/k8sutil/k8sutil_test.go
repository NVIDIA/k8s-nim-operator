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

package k8sutil

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDetectPlatform(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected PlatformType
	}{
		{"TKGS platform detected", map[string]string{"node.vmware.com/tkg": "true"}, TKGS},
		{"OpenShift platform detected", map[string]string{"node.openshift.io/os_id": "rhcos"}, OpenShift},
		{"GKE platform detected", map[string]string{"cloud.google.com/gke-nodepool": "default-pool"}, GKE},
		{"EKS platform detected", map[string]string{"eks.amazonaws.com/nodegroup": "my-node-group"}, EKS},
		{"AKS platform detected", map[string]string{"kubernetes.azure.com/cluster": "my-aks-cluster"}, AKS},
		{"OKE platform detected", map[string]string{"oke.oraclecloud.com/cluster": "my-oke-cluster"}, OKE},
		{"HPE Ezmeral platform detected", map[string]string{"ezmeral.hpe.com/cluster": "my-ezmeral-cluster"}, Ezmeral},
		{"Rancher RKE platform detected", map[string]string{"rke.cattle.io/version": "v1.2.3"}, RKE},
		{"Unknown platform", map[string]string{}, K8s},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fake client with nodes containing the given labels
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-node",
					Labels: tt.labels,
				},
			}
			fakeClient := fake.NewClientBuilder().WithObjects(node).Build()

			// Detect container platform
			platform, err := GetContainerPlatform(fakeClient)
			if err != nil {
				t.Fatalf("GetContainerPlatform failed: %v", err)
			}

			// Verify if the detected platform matches the expected type
			if platform != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, platform)
			}
		})
	}
}
