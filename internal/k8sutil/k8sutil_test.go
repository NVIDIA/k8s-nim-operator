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
	"context"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	discoveryfake "k8s.io/client-go/discovery/fake"
	testing "k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("K8s util tests", func() {
	DescribeTable("DetectPlatform tests",
		func(labels map[string]string, expected OrchestratorType) {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-node",
					Labels: labels,
				},
			}
			fakeClient := fake.NewClientBuilder().WithObjects(node).Build()

			platform, err := GetOrchestratorType(context.TODO(), fakeClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(platform).To(Equal(expected))
		},
		Entry("TKGS platform detected", map[string]string{"node.vmware.com/tkg": "true"}, TKGS),
		Entry("OpenShift platform detected", map[string]string{"node.openshift.io/os_id": "rhcos"}, OpenShift),
		Entry("GKE platform detected", map[string]string{"cloud.google.com/gke-nodepool": "default-pool"}, GKE),
		Entry("EKS platform detected", map[string]string{"eks.amazonaws.com/nodegroup": "my-node-group"}, EKS),
		Entry("AKS platform detected", map[string]string{"kubernetes.azure.com/cluster": "my-aks-cluster"}, AKS),
		Entry("OKE platform detected", map[string]string{"oke.oraclecloud.com/cluster": "my-oke-cluster"}, OKE),
		Entry("HPE Ezmeral platform detected", map[string]string{"ezmeral.hpe.com/cluster": "my-ezmeral-cluster"}, Ezmeral),
		Entry("Rancher RKE platform detected", map[string]string{"rke.cattle.io/version": "v1.2.3"}, RKE),
		Entry("Unknown platform", map[string]string{}, K8s),
	)

	Context("GetClusterVersion", func() {
		var fakeDiscovery *discoveryfake.FakeDiscovery

		BeforeEach(func() {
			fakeDiscovery = &discoveryfake.FakeDiscovery{
				Fake: &testing.Fake{
					RWMutex: sync.RWMutex{},
				},
				FakedServerVersion: &version.Info{
					GitVersion: "v1.33.0",
				},
			}
		})

		It("should return error when discovery client is nil", func() {
			version, err := GetClusterVersion(nil)
			Expect(err).To(HaveOccurred())
			Expect(version).To(BeEmpty())
		})

		It("should return cluster version when server version is available", func() {
			version, err := GetClusterVersion(fakeDiscovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(version).To(Equal("v1.33.0"))
		})
	})
})
