package e2e

import (
	"fmt"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Calling kubectl nim `get` command", func() {
	var namespace string

	BeforeEach(func() {
		namespace = createTestNamespace()
		createNGCSecretForTesting(namespace)
		DeferCleanup(func() {
			cleanupTestResources(namespace)
			deleteTestNamespace(namespace)
			namespace = ""
		})
	})

	Context("get nimcache", func() {
		It("should succeed in getting nimcache information", func() {
			// Deploy a test NIMCache
			nimCacheName := "test-nimcache-" + randStringBytes(5)
			deployTestNIMCache(namespace, nimCacheName)

			// Test getting the specific nimcache
			cmd := exec.Command("kubectl", "nim", "get", "nimcache", nimCacheName, "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring(nimCacheName))
		})

		It("should list all nimcaches in namespace", func() {
			// Deploy multiple test NIMCaches
			nimCacheName1 := "test-nimcache-1-" + randStringBytes(5)
			nimCacheName2 := "test-nimcache-2-" + randStringBytes(5)
			deployTestNIMCache(namespace, nimCacheName1)
			deployTestNIMCache(namespace, nimCacheName2)

			// Test listing all nimcaches
			cmd := exec.Command("kubectl", "nim", "get", "nimcache", "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring(nimCacheName1))
			Expect(string(output)).To(ContainSubstring(nimCacheName2))
		})

		It("should return error for non-existent nimcache", func() {
			cmd := exec.Command("kubectl", "nim", "get", "nimcache", "non-existent-cache", "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).To(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("not found"))
		})

	})

	Context("get nimservice", func() {
		var nimCacheName string

		BeforeEach(func() {
			// Create a NIMCache that NIMService can reference
			nimCacheName = "test-nimcache-for-service-" + randStringBytes(5)
			deployTestNIMCache(namespace, nimCacheName)
		})

		It("should succeed in getting nimservice information", func() {
			// Deploy a test NIMService
			nimServiceName := "test-nimservice-" + randStringBytes(5)
			deployTestNIMService(namespace, nimServiceName, nimCacheName)

			// Test getting the specific nimservice
			cmd := exec.Command("kubectl", "nim", "get", "nimservice", nimServiceName, "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring(nimServiceName))
		})

		It("should list all nimservices in namespace", func() {
			// Deploy multiple test NIMServices
			nimServiceName1 := "test-nimservice-1-" + randStringBytes(5)
			nimServiceName2 := "test-nimservice-2-" + randStringBytes(5)
			deployTestNIMService(namespace, nimServiceName1, nimCacheName)
			deployTestNIMService(namespace, nimServiceName2, nimCacheName)

			// Test listing all nimservices
			cmd := exec.Command("kubectl", "nim", "get", "nimservice", "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring(nimServiceName1))
			Expect(string(output)).To(ContainSubstring(nimServiceName2))
		})

		It("should return error for non-existent nimservice", func() {
			cmd := exec.Command("kubectl", "nim", "get", "nimservice", "non-existent-service", "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).To(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("not found"))
		})

		It("should display ExternalEndpoint in endpoint column", func() {
			// Deploy a test NIMService
			nimServiceName := "test-nimservice-endpoint-" + randStringBytes(5)
			nimCacheName := "test-nimcache-endpoint-" + randStringBytes(5)
			deployTestNIMCache(namespace, nimCacheName)
			deployTestNIMService(namespace, nimServiceName, nimCacheName)

			// Patch the NIMService to set the ExternalEndpoint
			externalEndpoint := "https://test-endpoint.example.com"
			patchJSON := fmt.Sprintf(`{"status":{"model":{"name":"test-model","clusterEndpoint":"http://test-service:8080","externalEndpoint":"%s"}}}`, externalEndpoint)
			patchCmd := exec.Command("kubectl", "patch", "nimservice", nimServiceName,
				"--namespace="+namespace,
				"--subresource=status",
				"--type=merge",
				"-p", patchJSON)
			patchOutput, patchErr := patchCmd.CombinedOutput()
			if patchErr != nil {
				GinkgoWriter.Printf("Patch command failed: %v\nOutput: %s\n", patchErr, string(patchOutput))
			}
			Expect(patchErr).NotTo(HaveOccurred())

			// Verify the patch was applied by checking the resource directly
			verifyCmd := exec.Command("kubectl", "get", "nimservice", nimServiceName,
				"--namespace="+namespace,
				"-o", "jsonpath={.status.model.externalEndpoint}")
			verifyOutput, verifyErr := verifyCmd.CombinedOutput()
			GinkgoWriter.Printf("Verify patch output: %s\n", string(verifyOutput))
			Expect(verifyErr).NotTo(HaveOccurred())

			// Get the NIMService and verify the endpoint is displayed
			cmd := exec.Command("kubectl", "nim", "get", "nimservice", nimServiceName, "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring(nimServiceName))
			Expect(string(output)).To(ContainSubstring(externalEndpoint))
		})

	})
})
