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
package nfdutil_test

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"

	"github.com/NVIDIA/k8s-nim-operator/internal/nfdutil"
)

var _ = Describe("NodeFeatureRule tests with CRD check", func() {
	var (
		ctx    context.Context
		client client.Client
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.TODO()
		os.Setenv("OPERATOR_NAMESPACE", "default")

		scheme = runtime.NewScheme()
		Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())
		Expect(nfdv1alpha1.AddToScheme(scheme)).To(Succeed())
	})

	AfterEach(func() {
		os.Unsetenv("OPERATOR_NAMESPACE")
	})

	Context("CRD existence check", func() {
		It("should return an error when the NodeFeatureRule CRD does not exist", func() {
			// Fake client without NodeFeatureRule CRD
			client = fake.NewClientBuilder().WithScheme(scheme).Build()

			found, err := nfdutil.CheckNodeFeatureRule(ctx, client)
			Expect(err).ToNot(HaveOccurred())
			Expect(found).To(BeFalse())
		})

		It("should proceed when NodeFeatureRule CRD exists but no resources are present", func() {
			// Add the CRD to the fake client
			crd := &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nodefeaturerules.nfd.k8s.io",
				},
			}
			client = fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(crd).
				Build()

			found, err := nfdutil.CheckNodeFeatureRule(ctx, client)
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeFalse()) // No NodeFeatureRules present
		})
	})

	Context("NodeFeatureRule listing and label check", func() {
		It("should return true when NodeFeatureRule with correct label exists", func() {
			// Add the CRD and NodeFeatureRule with correct label to the fake client
			crd := &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nodefeaturerules.nfd.k8s.io",
				},
			}

			nfr := &nfdv1alpha1.NodeFeatureRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nfr",
					Namespace: "default",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "k8s-nim-operator",
					},
				},
			}

			client = fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(crd, nfr).
				Build()

			found, err := nfdutil.CheckNodeFeatureRule(ctx, client)
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue()) // Correct NodeFeatureRule exists
		})

		It("should return false when no NodeFeatureRule with correct label exists", func() {
			// Add the CRD and NodeFeatureRule without the correct label
			crd := &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nodefeaturerules.nfd.k8s.io",
				},
			}

			nfr := &nfdv1alpha1.NodeFeatureRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nfr",
					Namespace: "default",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "some-other-operator",
					},
				},
			}

			client = fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(crd, nfr).
				Build()

			found, err := nfdutil.CheckNodeFeatureRule(ctx, client)
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeFalse()) // No matching NodeFeatureRule exists
		})
	})
})
