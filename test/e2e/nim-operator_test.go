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
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	helm "github.com/mittwald/go-helm-client"
	helmValues "github.com/mittwald/go-helm-client/values"
	"helm.sh/helm/v3/pkg/repo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/NVIDIA/k8s-test-infra/pkg/diagnostics"

	"github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/api/versioned"
)

// Regex patterns for string substitution in CRs.
const (
	namespacePattern = "{TEST_NAMESPACE}"
	esRepoPattern    = "{NEMO_ENTITYSTORE_REPO}"
	esVersionPattern = "{NEMO_ENTITYSTORE_VERSION}"
)

// Actual test suite.
var _ = Describe("NIM Operator", Ordered, func() {

	AfterEach(func(ctx context.Context) {
		// Run diagnostic collector if test failed
		if CurrentSpecReport().Failed() {
			var err error
			diagnosticsCollector, err = diagnostics.New(
				diagnostics.WithNamespace(testNamespace.Name),
				diagnostics.WithArtifactDir(LogArtifactDir),
				diagnostics.WithKubernetesClient(clientSet),
				diagnostics.WithObjects(collectLogsFrom...),
			)
			Expect(err).NotTo(HaveOccurred())

			err = diagnosticsCollector.Collect(ctx)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	BeforeAll(func() {
		// Add or Update Helm repo
		helmRepo := repo.Entry{
			Name: "nvidia",
			URL:  nvidiaHelm,
		}
		err := helmClient.AddOrUpdateChartRepo(helmRepo)
		Expect(err).NotTo(HaveOccurred())

		err = helmClient.UpdateChartRepos()
		Expect(err).NotTo(HaveOccurred())

		pullSecrets := []string{"ngc-secret"}
		// Values
		values := helmValues.Options{
			Values: []string{
				fmt.Sprintf("operator.image.repository=%s", ImageRepo),
				fmt.Sprintf("operator.image.tag=%s", ImageTag),
				fmt.Sprintf("operator.image.pullPolicy=%s", ImagePullPolicy),
				fmt.Sprintf("operator.image.pullSecrets={%s}", strings.Join(pullSecrets, ",")),
				fmt.Sprintf("operator.admissionController.enabled=%t", AdmissionControllerEnabled),
			},
		}

		// Chart spec
		chartSpec := &helm.ChartSpec{
			ReleaseName:     helmReleaseName,
			ChartName:       helmChart,
			Namespace:       testNamespace.Name,
			CreateNamespace: true,
			Wait:            true,
			Timeout:         10 * time.Minute, // pull time is long
			ValuesOptions:   values,
			CleanupOnFail:   true,
		}

		By("Installing k8s-nim-operator Helm chart")
		_, err = helmClient.InstallOrUpgradeChart(ctx, chartSpec, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	When("deploying NIMCache and NIMService", Ordered, func() {
		AfterEach(func() {
			// Clean up
			cleanupNIMCRs()
		})

		It("should go to READY state", func(ctx context.Context) {
			// Create a NIMCache object
			By("Creating a NIMCache object")
			cli, err := versioned.NewForConfig(clientConfig)
			Expect(err).NotTo(HaveOccurred())

			nimCache := &v1alpha1.NIMCache{}
			data, err := os.ReadFile(filepath.Join(cwd, "data", "nimcache.yml"))
			Expect(err).NotTo(HaveOccurred())

			err = yaml.Unmarshal(data, nimCache)
			Expect(err).NotTo(HaveOccurred())

			_, err = cli.AppsV1alpha1().NIMCaches(testNamespace.Name).Create(ctx, nimCache, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the NIMCache object state is ready")
			Eventually(func() bool {
				nimCacheObject, _ := cli.AppsV1alpha1().NIMCaches(testNamespace.Name).Get(ctx, nimCache.Name, metav1.GetOptions{})
				return nimCacheObject.Status.State == v1alpha1.NimCacheStatusReady
			}, Timeout, 5*time.Second).Should(BeTrue())

			// Create a NIMService object
			By("Creating a NIMService object")
			nimService := &v1alpha1.NIMService{}
			data, err = os.ReadFile(filepath.Join(cwd, "data", "nimservice.yml"))
			Expect(err).NotTo(HaveOccurred())

			err = yaml.Unmarshal(data, nimService)
			Expect(err).NotTo(HaveOccurred())

			_, err = cli.AppsV1alpha1().NIMServices(testNamespace.Name).Create(ctx, nimService, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the NIMService object state is ready")
			Eventually(func() bool {
				nimServiceObject, _ := cli.AppsV1alpha1().NIMServices(testNamespace.Name).Get(ctx, nimService.Name, metav1.GetOptions{})
				return nimServiceObject.Status.State == v1alpha1.NIMServiceStatusReady
			}, Timeout, 5*time.Second).Should(BeTrue())
		})
	})

	When("deploying NEMO microservices", func() {
		BeforeEach(func() {
			if !EnableNemoMicroservices {
				Skip("NEMO microservies not requested to be tested")
			}

			// Install dependencies
			By("Installing a postgres database")
			installEntitystoreDependencies()

		})
		AfterEach(func() {
			if !EnableNemoMicroservices {
				return
			}

			// Clean up CRs
			cleanupNEMOCRs()

			// Cleanup dependencies
			err := helmClient.UninstallReleaseByName(fmt.Sprintf("es-%s", postgresql))
			Expect(err).NotTo(HaveOccurred())
		})

		It("NEMO Entitystore CR should go to READY state", func() {
			// Create a NemoEntitystore object
			By("Creating a NemoEntitystore object")
			cli, err := versioned.NewForConfig(clientConfig)
			Expect(err).NotTo(HaveOccurred())

			nemoEntitystore := &v1alpha1.NemoEntitystore{}
			data, err := os.ReadFile(filepath.Join(cwd, "data", "nemoentitystore.yml"))
			Expect(err).NotTo(HaveOccurred())
			dataStr := string(data)
			dataStr = strings.ReplaceAll(dataStr, namespacePattern, testNamespace.Name)
			dataStr = strings.ReplaceAll(dataStr, esRepoPattern, NemoEntityStoreRepo)
			dataStr = strings.ReplaceAll(dataStr, esVersionPattern, NemoEntityStoreVersion)

			err = yaml.Unmarshal([]byte(dataStr), nemoEntitystore)
			Expect(err).NotTo(HaveOccurred())

			_, err = cli.AppsV1alpha1().NemoEntitystores(testNamespace.Name).Create(ctx, nemoEntitystore, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the NemoEntitystore object state is ready")
			Eventually(func() bool {
				nemoEntitystoreObject, _ := cli.AppsV1alpha1().NemoEntitystores(testNamespace.Name).Get(ctx, nemoEntitystore.Name, metav1.GetOptions{})
				return nemoEntitystoreObject.Status.State == v1alpha1.NemoEntitystoreStatusReady
			}, Timeout, 5*time.Second).Should(BeTrue())

		})
	})
})

func cleanup() {
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())

	deployed, err := helmClient.ListDeployedReleases()
	Expect(err).NotTo(HaveOccurred())

	for _, release := range deployed {
		switch release.Name {
		case nfd:
			if EnableNFD {
				err := helmClient.UninstallReleaseByName(release.Name)
				Expect(err).NotTo(HaveOccurred())
			} // else skip
		case gpuOperator:
			if EnableGPUOperator {
				err := helmClient.UninstallReleaseByName(release.Name)
				Expect(err).NotTo(HaveOccurred())
			} // else skip
		case localPathProvisioner:
			if EnableLocalPathProvisioner {
				err := os.RemoveAll(filepath.Join(cwd, localPathProvisioner))
				Expect(err).NotTo(HaveOccurred())

				err = helmClient.UninstallReleaseByName(release.Name)
				Expect(err).NotTo(HaveOccurred())
			} // else skip
		case helmReleaseName:
			err := helmClient.UninstallReleaseByName(helmReleaseName)
			Expect(err).NotTo(HaveOccurred())
		}
	}
}

// cleanupNIMCRs deletes all NIMCache, NIMService and NIMPipeline CRs deployed on the test namespace.
func cleanupNIMCRs() {
	cli, err := versioned.NewForConfig(clientConfig)
	Expect(err).NotTo(HaveOccurred())

	// List all NIMCache CRs
	nimCacheList, err := cli.AppsV1alpha1().NIMCaches(testNamespace.Name).List(ctx, metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())

	// Delete all NIMCache CRs
	if len(nimCacheList.Items) != 0 {
		By("Deleting all NIMCache CRs")
		for _, nimCache := range nimCacheList.Items {
			err := cli.AppsV1alpha1().NIMCaches(testNamespace.Name).Delete(ctx, nimCache.Name, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())
		}
	}

	// List all NIMService CRs
	nimServiceList, err := cli.AppsV1alpha1().NIMServices(testNamespace.Name).List(ctx, metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())

	// Delete all NIMService CRs
	if len(nimServiceList.Items) != 0 {
		By("Deleting all NIMService CRs")
		for _, nimService := range nimServiceList.Items {
			err := cli.AppsV1alpha1().NIMServices(testNamespace.Name).Delete(ctx, nimService.Name, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())
		}
	}

	// List all NIMPipeline CRs
	nimPipelineList, err := cli.AppsV1alpha1().NIMPipelines(testNamespace.Name).List(ctx, metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())

	// Delete all NIMPipeline CRs
	if len(nimPipelineList.Items) != 0 {
		By("Deleting all NIMPipeline CRs")
		for _, nimPipeline := range nimPipelineList.Items {
			err := cli.AppsV1alpha1().NIMPipelines(testNamespace.Name).Delete(ctx, nimPipeline.Name, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())
		}
	}

	// List all NemoEntitystore CRs
	nemoEntitystoreList, err := cli.AppsV1alpha1().NemoEntitystores(testNamespace.Name).List(ctx, metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())

	// Delete all NemoEntitystore CRs
	if len(nemoEntitystoreList.Items) != 0 {
		By("Deleting all NemoEntitystore CRs")
		for _, nemoEntitystore := range nemoEntitystoreList.Items {
			err := cli.AppsV1alpha1().NemoEntitystores(testNamespace.Name).Delete(ctx, nemoEntitystore.Name, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())
		}
	}
}

// cleanupNEMOCRs deletes all NEMO microservice CRs deployed on the test namespace.
func cleanupNEMOCRs() {
	cli, err := versioned.NewForConfig(clientConfig)
	Expect(err).NotTo(HaveOccurred())

	// List all NemoEntitystore CRs
	nemoEntitystoreList, err := cli.AppsV1alpha1().NemoEntitystores(testNamespace.Name).List(ctx, metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())

	// Delete all NemoEntitystore CRs
	if len(nemoEntitystoreList.Items) != 0 {
		By("Deleting all NemoEntitystore CRs")
		for _, nemoEntitystore := range nemoEntitystoreList.Items {
			err := cli.AppsV1alpha1().NemoEntitystores(testNamespace.Name).Delete(ctx, nemoEntitystore.Name, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())
		}
	}
}

func cleanupCRDs() {
	crds := []string{
		"nimcaches.apps.nvidia.com",
		"nimservices.apps.nvidia.com",
		"nimpipelines.apps.nvidia.com",
	}

	if EnableNFD {
		crds = append(crds, "nodefeatures.nfd.k8s-sigs.io", "nodefeaturerules.nfd.k8s-sigs.io", "nodefeaturegroups.nfd.k8s-sigs.io")
	}

	if EnableGPUOperator {
		crds = append(crds, "clusterpolicies.nvidia.com", "nvidiadrivers.nvidia.com")
	}

	if EnableNemoMicroservices {
		crds = append(crds, "nemoentitystores.apps.nvidia.com")
	}

	// Delete CRDs
	for _, crd := range crds {
		err := extClient.ApiextensionsV1().CustomResourceDefinitions().Delete(ctx, crd, metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())
	}
}

func installEntitystoreDependencies() {
	// Add or Update Helm repo
	helmRepo := repo.Entry{
		Name: "bitnami",
		URL:  bitnamiHelm,
	}
	err := helmClient.AddOrUpdateChartRepo(helmRepo)
	Expect(err).NotTo(HaveOccurred())

	err = helmClient.UpdateChartRepos()
	Expect(err).NotTo(HaveOccurred())

	// Install Database dependencies for entitystore.
	values := helmValues.Options{
		Values: []string{
			"architecture=standalone",
			fmt.Sprintf("auth.username=%s", "esuser"),
			fmt.Sprintf("auth.database=%s", "gateway"),
			fmt.Sprintf("auth.exitingSecret=%s", "es-postgresql"),
			fmt.Sprintf("global.storageClass=%s", "local-path"),
			fmt.Sprintf("global.size=%s", "10GB"),
		},
	}
	chartSpec := &helm.ChartSpec{
		ReleaseName:     fmt.Sprintf("es-%s", postgresql),
		ChartName:       "bitnami/postgresql",
		Namespace:       testNamespace.Name,
		CreateNamespace: false,
		Wait:            true,
		WaitForJobs:     true,
		Timeout:         10 * time.Minute,
		CleanupOnFail:   true,
		ValuesOptions:   values}
	_, err = helmClient.InstallOrUpgradeChart(ctx, chartSpec, nil)
	Expect(err).NotTo(HaveOccurred())
}
