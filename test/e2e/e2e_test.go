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
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/NVIDIA/k8s-test-infra/pkg/diagnostics"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	helm "github.com/mittwald/go-helm-client"
	helmValues "github.com/mittwald/go-helm-client/values"
	"helm.sh/helm/v3/pkg/repo"
	corev1 "k8s.io/api/core/v1"
	extclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	nvidiaHelm                  = "https://helm.ngc.nvidia.com/nvidia"
	rancherLocalPathProvisioner = "https://github.com/rancher/local-path-provisioner.git"
	bitnamiHelm                 = "https://charts.bitnami.com/bitnami"
	nfd                         = "nfd"
	gpuOperator                 = "gpu-operator"
	localPathProvisioner        = "local-path-provisioner"
	postgresql                  = "postgresql"

	// DefaultNamespaceDeletionTimeout is timeout duration for waiting for a namespace deletion.
	DefaultNamespaceDeletionTimeout = 10 * time.Minute

	// PollInterval is how often to Poll pods, nodes and claims.
	PollInterval = 2 * time.Second
)

// Test context.
var (
	KUBECONFIG string

	EnableNFD                  bool
	EnableGPUOperator          bool
	EnableLocalPathProvisioner bool
	EnableNemoMicroservices    bool
	LogArtifactDir             string
	ImageRepo                  string
	ImageTag                   string
	ImagePullPolicy            string
	CollectLogsFrom            string
	AdmissionControllerEnabled bool
	Timeout                    time.Duration

	// k8s clients.
	clientConfig *rest.Config
	clientSet    clientset.Interface
	extClient    *extclient.Clientset

	testNamespace *corev1.Namespace // Every test has at least one namespace unless creation is skipped

	// Helm.
	helmChart       string
	helmClient      helm.Client
	helmLogFile     *os.File
	helmArtifactDir string
	helmLogger      *log.Logger
	helmReleaseName string

	ctx context.Context
	cwd string

	diagnosticsCollector    *diagnostics.Diagnostic
	collectLogsFrom         []string
	defaultCollectorObjects = []string{
		"pods",
		"nodes",
		"namespaces",
		"deployments",
		"daemonsets",
		"jobs",
	}

	// NEMO microservice variables.
	NemoEntityStoreRepo    string
	NemoEntityStoreVersion string
)

func TestMain(t *testing.T) {
	suiteName := "E2E K8s NIM Operator"

	RegisterFailHandler(Fail)
	RunSpecs(t,
		suiteName,
	)
}

// BeforeSuite runs before the test suite.
var _ = BeforeSuite(func() {
	var err error
	ctx = context.Background()

	// Init
	getTestEnv()

	// Get k8s clients
	getK8sClients()

	// Create clients for apiextensions and our CRD api
	extClient = extclient.NewForConfigOrDie(clientConfig)

	// Create a namespace for the test
	testNamespace, err = CreateTestingNS("k8s-nim-operator", clientSet, nil)
	Expect(err).NotTo(HaveOccurred())

	// Get Helm client
	helmReleaseName = "nim-op-e2e-test" + rand.String(5)
	getHelmClient()

	// check Collector objects
	collectLogsFrom = defaultCollectorObjects
	if CollectLogsFrom != "" && CollectLogsFrom != "default" {
		collectLogsFrom = strings.Split(CollectLogsFrom, ",")
	}

	createPullSecrets()
	deployDependencies(ctx)
})

// AfterSuite runs after the test suite.
var _ = AfterSuite(func() {
	// Clean up CRs so they are garbage collected by their controllers
	cleanupNIMCRs()
	cleanupNEMOCRs()
	// Clean up Helm deployments
	cleanup()
	// Remove CRDs
	cleanupCRDs()
	// Delete Test Namespace
	DeleteNamespace()
})

// deployDependencies installs all the dependent helm charts.
func deployDependencies(ctx context.Context) {
	// Install dependencies if needed
	if EnableNFD {
		// Add or Update Helm repo
		helmRepo := repo.Entry{
			Name: nfd,
			URL:  "https://kubernetes-sigs.github.io/node-feature-discovery/charts",
		}
		err := helmClient.AddOrUpdateChartRepo(helmRepo)
		Expect(err).NotTo(HaveOccurred())

		err = helmClient.UpdateChartRepos()
		Expect(err).NotTo(HaveOccurred())

		// Install NFD
		chartSpec := &helm.ChartSpec{
			ReleaseName:     nfd,
			ChartName:       "nfd/node-feature-discovery",
			Namespace:       testNamespace.Name,
			CreateNamespace: false,
			Wait:            true,
			Timeout:         5 * time.Minute,
			CleanupOnFail:   true,
		}
		_, err = helmClient.InstallOrUpgradeChart(ctx, chartSpec, nil)
		Expect(err).NotTo(HaveOccurred())
	}

	if EnableGPUOperator {
		// Add or Update Helm repo
		helmRepo := repo.Entry{
			Name: "nvidia",
			URL:  nvidiaHelm,
		}
		err := helmClient.AddOrUpdateChartRepo(helmRepo)
		Expect(err).NotTo(HaveOccurred())

		err = helmClient.UpdateChartRepos()
		Expect(err).NotTo(HaveOccurred())

		// Install GPU Operator
		values := helmValues.Options{
			Values: []string{
				fmt.Sprintf("nfd.enabled=%v", false), // we install NFD separately
			},
		}
		chartSpec := &helm.ChartSpec{
			ReleaseName:     gpuOperator,
			ChartName:       "nvidia/gpu-operator",
			Namespace:       testNamespace.Name,
			CreateNamespace: false,
			Wait:            true,
			WaitForJobs:     true,
			Timeout:         10 * time.Minute, // Give it time to build the driver
			CleanupOnFail:   true,
			ValuesOptions:   values}
		_, err = helmClient.InstallOrUpgradeChart(ctx, chartSpec, nil)
		Expect(err).NotTo(HaveOccurred())
	}

	if EnableLocalPathProvisioner {
		// Clone the repository
		if _, err := os.Stat(filepath.Join(cwd, localPathProvisioner)); os.IsNotExist(err) {
			tag := "v0.0.28"
			_, err := git.PlainClone(filepath.Join(cwd, localPathProvisioner), false, &git.CloneOptions{
				URL:           rancherLocalPathProvisioner,
				ReferenceName: plumbing.NewTagReferenceName(tag),
				Progress:      nil,
			})
			Expect(err).NotTo(HaveOccurred())
		}
		// Set chartPath to the correct location
		chartPath := filepath.Join(localPathProvisioner, "deploy", "chart", localPathProvisioner)
		// Load the chart
		chartSpec := &helm.ChartSpec{
			ReleaseName:     localPathProvisioner,
			ChartName:       chartPath,
			Namespace:       testNamespace.Name,
			Wait:            true,
			Timeout:         1 * time.Minute,
			CleanupOnFail:   true,
			CreateNamespace: false,
		}
		// Install the local-path-provisioner
		_, err := helmClient.InstallChart(ctx, chartSpec, nil)
		Expect(err).NotTo(HaveOccurred())
	}
}

// cleanup cleans up the test environment.
func getK8sClients() {
	var err error

	// get config from kubeconfig
	c, err := clientcmd.LoadFromFile(KUBECONFIG)
	Expect(err).NotTo(HaveOccurred())

	// get client config
	clientConfig, err = clientcmd.NewDefaultClientConfig(*c, &clientcmd.ConfigOverrides{}).ClientConfig()
	Expect(err).NotTo(HaveOccurred())

	clientSet, err = clientset.NewForConfig(clientConfig)
	Expect(err).NotTo(HaveOccurred())
}

// cleanup cleans up the test environment.
func getHelmClient() {
	var err error

	// Set Helm log file
	helmArtifactDir = filepath.Join(LogArtifactDir, "helm")

	// Create a Helm client
	err = os.MkdirAll(helmArtifactDir, 0755)
	Expect(err).NotTo(HaveOccurred())

	helmLogFile, err = os.OpenFile(filepath.Join(LogArtifactDir, "helm_logs"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	Expect(err).NotTo(HaveOccurred())

	helmLogger = log.New(helmLogFile, fmt.Sprintf("%s\t", testNamespace.Name), log.Ldate|log.Ltime)

	helmRestConf := &helm.RestConfClientOptions{
		Options: &helm.Options{
			Namespace:        testNamespace.Name,
			RepositoryCache:  "/tmp/.helmcache",
			RepositoryConfig: "/tmp/.helmrepo",
			Debug:            true,
			DebugLog:         helmLogger.Printf,
		},
		RestConfig: clientConfig,
	}

	helmClient, err = helm.NewClientFromRestConf(helmRestConf)
	Expect(err).NotTo(HaveOccurred())
}

// cleanup cleans up the test environment.
func getTestEnv() {
	var err error

	KUBECONFIG = os.Getenv("KUBECONFIG")
	Expect(KUBECONFIG).NotTo(BeEmpty(), "KUBECONFIG must be set")

	EnableNFD = getBoolEnvVar("ENABLE_NFD", false)
	EnableGPUOperator = getBoolEnvVar("ENABLE_GPU_OPERATOR", false)
	EnableLocalPathProvisioner = getBoolEnvVar("ENABLE_LOCAL_PATH_PROVISIONER", false)
	EnableNemoMicroservices = getBoolEnvVar("ENABLE_NEMO_MICROSERVICES", false)
	Timeout = time.Duration(getIntEnvVar("E2E_TIMEOUT_SECONDS", 1800)) * time.Second

	helmChart = os.Getenv("HELM_CHART")
	Expect(helmChart).NotTo(BeEmpty(), "HELM_CHART must be set")

	LogArtifactDir = os.Getenv("LOG_ARTIFACT_DIR")

	ImageRepo = os.Getenv("E2E_IMAGE_REPO")
	Expect(ImageRepo).NotTo(BeEmpty(), "IMAGE_REPO must be set")

	ImageTag = os.Getenv("E2E_IMAGE_TAG")
	Expect(ImageTag).NotTo(BeEmpty(), "IMAGE_TAG must be set")

	ImagePullPolicy = os.Getenv("E2E_IMAGE_PULL_POLICY")
	Expect(ImagePullPolicy).NotTo(BeEmpty(), "IMAGE_PULL_POLICY must be set")

	CollectLogsFrom = os.Getenv("COLLECT_LOGS_FROM")

	AdmissionControllerEnabled = getBoolEnvVar("ADMISSION_CONTROLLER_ENABLED", false)

	if EnableNemoMicroservices {
		// Entitystore env variables.
		NemoEntityStoreRepo = os.Getenv("NEMO_ENTITYSTORE_REPO")
		Expect(NemoEntityStoreRepo).NotTo(BeEmpty(), "NEMO_ENTITYSTORE_REPO must be set")
		NemoEntityStoreVersion = os.Getenv("NEMO_ENTITYSTORE_VERSION")
		Expect(NemoEntityStoreVersion).NotTo(BeEmpty(), "NEMO_ENTITYSTORE_VERSION must be set")
	}

	// Get current working directory
	cwd, err = os.Getwd()
	Expect(err).NotTo(HaveOccurred())
}

// getBoolEnvVar returns the boolean value of the environment variable or the default value if not set.
func getBoolEnvVar(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	boolValue, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return boolValue
}

func getIntEnvVar(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	intValue, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return intValue
}

// CreateTestingNS should be used by every test, note that we append a common prefix to the provided test name.
// Please see NewFramework instead of using this directly.
func CreateTestingNS(baseName string, c clientset.Interface, labels map[string]string) (*corev1.Namespace, error) {
	uid := RandomSuffix()
	if labels == nil {
		labels = map[string]string{}
	}
	labels["e2e-run"] = uid

	// We don't use ObjectMeta.GenerateName feature, as in case of API call
	// failure we don't know whether the namespace was created and what is its
	// name.
	name := fmt.Sprintf("%v-%v", baseName, uid)

	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "",
			Labels:    labels,
		},
		Status: corev1.NamespaceStatus{},
	}
	// Be robust about making the namespace creation call.
	var got *corev1.Namespace
	if err := wait.PollUntilContextTimeout(ctx, PollInterval, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		var err error
		got, err = c.CoreV1().Namespaces().Create(ctx, namespaceObj, metav1.CreateOptions{})
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				// regenerate on conflict
				namespaceObj.Name = fmt.Sprintf("%v-%v", baseName, uid)
			}
			return false, nil
		}
		return true, nil
	}); err != nil {
		return nil, err
	}

	return got, nil
}

// DeleteNamespace can be used to delete a namespace.
func DeleteNamespace() {
	defer func() {
		err := clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			Expect(err).NotTo(HaveOccurred())
		}
		err = WaitForNamespacesDeleted(clientSet, []string{testNamespace.Name}, DefaultNamespaceDeletionTimeout)
		Expect(err).NotTo(HaveOccurred())
	}()
}

// WaitForNamespacesDeleted waits for the namespaces to be deleted.
func WaitForNamespacesDeleted(c clientset.Interface, namespaces []string, timeout time.Duration) error {
	By(fmt.Sprintf("Waiting for namespace %+v deletion", namespaces))
	nsMap := map[string]bool{}
	for _, ns := range namespaces {
		nsMap[ns] = true
	}
	// Now POLL until all namespaces have been eradicated.
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			nsList, err := c.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
			if err != nil {
				return false, err
			}
			for _, item := range nsList.Items {
				if _, ok := nsMap[item.Name]; ok {
					return false, nil
				}
			}
			return true, nil
		})
}

// RandomSuffix provides a random sequence to append to pods,services,rcs.
func RandomSuffix() string {
	return strconv.Itoa(rand.Intn(10000))
}

func createPullSecrets() {
	// Get the NGC_API_KEY
	NGC_API_KEY := os.Getenv("NGC_API_KEY")

	// Create a secret for pulling the image
	ngcAPIsecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ngc-api-secret",
			Namespace: testNamespace.Name,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"NGC_API_KEY": NGC_API_KEY,
		},
	}

	_, err := clientSet.CoreV1().Secrets(testNamespace.Name).Create(ctx, ngcAPIsecret, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	// Get the HF_TOKEN
	HF_TOKEN := os.Getenv("HF_TOKEN")

	// Create a secret for pulling the image
	hfTokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hf-token-secret",
			Namespace: testNamespace.Name,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"HF_TOKEN": HF_TOKEN,
		},
	}

	_, err = clientSet.CoreV1().Secrets(testNamespace.Name).Create(ctx, hfTokenSecret, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	// Create the dockerconfigjson type secret
	dockerServer := "nvcr.io"
	dockerUsername := `$oauthtoken`
	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", dockerUsername, NGC_API_KEY)))

	dockerConfigJson := `{
		"auths": {
			"` + dockerServer + `": {
				"username": "` + dockerUsername + `",
				"password": "` + NGC_API_KEY + `",
				"auth": "` + auth + `"
			}
		}
	}`

	ngcSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "ngc-secret",
			Namespace:         testNamespace.Name,
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: []byte(dockerConfigJson),
		},
		Type: corev1.SecretTypeDockerConfigJson,
	}
	_, err = clientSet.CoreV1().Secrets(testNamespace.Name).Create(ctx, ngcSecret, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
}
