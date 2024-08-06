/*
 * Copyright (c) 2024, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package e2e

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	helm "github.com/mittwald/go-helm-client"
	helmValues "github.com/mittwald/go-helm-client/values"
	extclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	"github.com/NVIDIA/k8s-test-infra/pkg/diagnostics"
	"github.com/NVIDIA/k8s-test-infra/pkg/framework"
)

// Actual test suite
var _ = NVDescribe("NIM Operator", func() {
	f := framework.NewFramework("k8s-nim-operator")

	Context("When deploying k8s-nim-operator", Ordered, func() {
		// helm-chart is required
		if *HelmChart == "" {
			Fail("No helm-chart for k8s-nim-operator specified")
		}

		// Init global suite vars vars
		var (
			crds      []string
			extClient *extclient.Clientset

			helmReleaseName string
			chartSpec       helm.ChartSpec

			collectLogsFrom      []string
			diagnosticsCollector *diagnostics.Diagnostic
		)

		defaultCollectorObjects := []string{
			"pods",
			"nodes",
			"namespaces",
			"deployments",
			"daemonsets",
		}

		crds = []string{
			"nimcaches.apps.nvidia.com",
			"nimservices.apps.nvidia.com",
			"nimpipelines.apps.nvidia.com",
		}

		values := helmValues.Options{
			Values: []string{
				fmt.Sprintf("operator.image.repository=%s", *ImageRepo),
				fmt.Sprintf("operator.image.tag=%s", *ImageTag),
				fmt.Sprintf("operator.image.pullPolicy=%s", *ImagePullPolicy),
			},
		}

		// check Collector objects
		collectLogsFrom = defaultCollectorObjects
		if *CollectLogsFrom != "" && *CollectLogsFrom != "default" {
			collectLogsFrom = strings.Split(*CollectLogsFrom, ",")
		}

		BeforeAll(func(ctx context.Context) {
			// Create clients for apiextensions and our CRD api
			extClient = extclient.NewForConfigOrDie(f.ClientConfig())
			helmReleaseName = "nim-op-e2e-test" + rand.String(5)
		})

		JustBeforeEach(func(ctx context.Context) {
			// reset Helm Client
			chartSpec = helm.ChartSpec{
				ReleaseName:   helmReleaseName,
				ChartName:     *HelmChart,
				Namespace:     f.Namespace.Name,
				Wait:          true,
				Timeout:       1 * time.Minute,
				ValuesOptions: values,
				CleanupOnFail: true,
			}

			By("Installing k8s-nim-operator Helm chart")
			_, err := f.HelmClient.InstallChart(ctx, &chartSpec, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func(ctx context.Context) {
			// Run diagnostic collector if test failed
			if CurrentSpecReport().Failed() {
				var err error
				diagnosticsCollector, err = diagnostics.New(
					diagnostics.WithNamespace(f.Namespace.Name),
					diagnostics.WithArtifactDir(*LogArtifactDir),
					diagnostics.WithKubernetesClient(f.ClientSet),
					diagnostics.WithObjects(collectLogsFrom...),
				)
				Expect(err).NotTo(HaveOccurred())

				err = diagnosticsCollector.Collect(ctx)
				Expect(err).NotTo(HaveOccurred())
			}
			// Cleanup before next test run
			// Delete Helm release
			err := f.HelmClient.UninstallReleaseByName(helmReleaseName)
			Expect(err).NotTo(HaveOccurred())
		})

		// Clean up
		AfterAll(func(ctx context.Context) {
			// Delete CRDs
			for _, crd := range crds {
				err := extClient.ApiextensionsV1().CustomResourceDefinitions().Delete(ctx, crd, metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())
			}
		})

		Context("and the NIM Operator is deployed", func() {
			It("it should create *.apps.nvidia.com CRD's", func(ctx context.Context) {
				crdl, err := f.ApiExtClient.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{})
				Expect(err).NotTo(HaveOccurred())

				// Look for the 3 NIMCache, NIMService and NIMPipeline CRDs
				nimCrds := map[string]bool{
					"nimcaches.apps.nvidia.com":    false,
					"nimservices.apps.nvidia.com":  false,
					"nimpipelines.apps.nvidia.com": false,
				}

				for _, crd := range crdl.Items {
					if _, ok := nimCrds[crd.Name]; ok {
						nimCrds[crd.Name] = true
					}
				}

				for crdName, found := range nimCrds {
					Expect(found).To(BeTrue(), "CRD %q not found", crdName)
				}
			})
		})
	})
})
