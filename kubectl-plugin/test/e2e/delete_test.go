package e2e

import (
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Calling kubectl nim `delete` command", func() {
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

	Context("delete nimcache", func() {
		It("should succeed in deleting a nimcache", func() {
			// First create a nimcache
			nimCacheName := "test-delete-nimcache-" + randStringBytes(5)
			deployTestNIMCache(namespace, nimCacheName)

			// Verify it exists
			cmd := exec.Command("kubectl", "get", "nimcache", nimCacheName, "--namespace="+namespace)
			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Delete the nimcache
			cmd = exec.Command("kubectl", "nim", "delete", "nimcache", nimCacheName, "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("deleted"))

			// Verify it's gone
			cmd = exec.Command("kubectl", "get", "nimcache", nimCacheName, "--namespace="+namespace)
			err = cmd.Run()
			Expect(err).To(HaveOccurred())
		})

		It("should succeed in deleting multiple nimcaches", func() {
			// Create multiple nimcaches
			nimCacheName1 := "test-delete-nimcache-1-" + randStringBytes(5)
			nimCacheName2 := "test-delete-nimcache-2-" + randStringBytes(5)
			deployTestNIMCache(namespace, nimCacheName1)
			deployTestNIMCache(namespace, nimCacheName2)

			// Delete first nimcache
			cmd := exec.Command("kubectl", "nim", "delete", "nimcache", nimCacheName1, "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("deleted"))

			// Delete second nimcache
			cmd = exec.Command("kubectl", "nim", "delete", "nimcache", nimCacheName2, "--namespace="+namespace)
			output, err = runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("deleted"))

			// Verify both are gone
			cmd = exec.Command("kubectl", "get", "nimcache", nimCacheName1, "--namespace="+namespace)
			err = cmd.Run()
			Expect(err).To(HaveOccurred())

			cmd = exec.Command("kubectl", "get", "nimcache", nimCacheName2, "--namespace="+namespace)
			err = cmd.Run()
			Expect(err).To(HaveOccurred())
		})

		It("should succeed deleting all nimcaches individually", func() {
			// Create multiple nimcaches
			nimCacheName1 := "test-delete-all-nimcache-1-" + randStringBytes(5)
			nimCacheName2 := "test-delete-all-nimcache-2-" + randStringBytes(5)
			deployTestNIMCache(namespace, nimCacheName1)
			deployTestNIMCache(namespace, nimCacheName2)

			// Delete each nimcache individually
			cmd := exec.Command("kubectl", "nim", "delete", "nimcache", nimCacheName1, "--namespace="+namespace)
			_, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("kubectl", "nim", "delete", "nimcache", nimCacheName2, "--namespace="+namespace)
			_, err = runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())

			// Verify all are gone
			cmd = exec.Command("kubectl", "get", "nimcache", "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("No resources found"))
		})

		It("should return appropriate message when deleting non-existent nimcache", func() {
			cmd := exec.Command("kubectl", "nim", "delete", "nimcache", "non-existent-cache", "--namespace="+namespace)
			output, _ := runCommandWithDebug(cmd)

			// kubectl typically doesn't error on deleting non-existent resources
			// but shows a not found message
			outputStr := string(output)
			Expect(outputStr).To(SatisfyAny(
				ContainSubstring("not found"),
				ContainSubstring("No resources found"),
			))
		})

		It("should handle basic delete", func() {
			nimCacheName := "test-delete-basic-nimcache-" + randStringBytes(5)
			deployTestNIMCache(namespace, nimCacheName)

			// Delete normally
			cmd := exec.Command("kubectl", "nim", "delete", "nimcache", nimCacheName,
				"--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("deleted"))
		})
	})

	Context("delete nimservice", func() {
		var nimCacheName string

		BeforeEach(func() {
			// Create a NIMCache that NIMService can reference
			nimCacheName = "test-nimcache-for-delete-" + randStringBytes(5)
			deployTestNIMCache(namespace, nimCacheName)
		})

		It("should succeed in deleting a nimservice", func() {
			// First create a nimservice
			nimServiceName := "test-delete-nimservice-" + randStringBytes(5)
			deployTestNIMService(namespace, nimServiceName, nimCacheName)

			// Verify it exists
			cmd := exec.Command("kubectl", "get", "nimservice", nimServiceName, "--namespace="+namespace)
			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			// Delete the nimservice
			cmd = exec.Command("kubectl", "nim", "delete", "nimservice", nimServiceName, "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("deleted"))

			// Verify it's gone
			cmd = exec.Command("kubectl", "get", "nimservice", nimServiceName, "--namespace="+namespace)
			err = cmd.Run()
			Expect(err).To(HaveOccurred())
		})

		It("should succeed in deleting multiple nimservices", func() {
			// Create multiple nimservices
			nimServiceName1 := "test-delete-nimservice-1-" + randStringBytes(5)
			nimServiceName2 := "test-delete-nimservice-2-" + randStringBytes(5)
			deployTestNIMService(namespace, nimServiceName1, nimCacheName)
			deployTestNIMService(namespace, nimServiceName2, nimCacheName)

			// Delete first nimservice
			cmd := exec.Command("kubectl", "nim", "delete", "nimservice", nimServiceName1, "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("deleted"))

			// Delete second nimservice
			cmd = exec.Command("kubectl", "nim", "delete", "nimservice", nimServiceName2, "--namespace="+namespace)
			output, err = runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("deleted"))

			// Verify both are gone
			cmd = exec.Command("kubectl", "get", "nimservice", nimServiceName1, "--namespace="+namespace)
			err = cmd.Run()
			Expect(err).To(HaveOccurred())

			cmd = exec.Command("kubectl", "get", "nimservice", nimServiceName2, "--namespace="+namespace)
			err = cmd.Run()
			Expect(err).To(HaveOccurred())
		})

		It("should succeed deleting all nimservices individually", func() {
			// Create multiple nimservices
			nimServiceName1 := "test-delete-all-nimservice-1-" + randStringBytes(5)
			nimServiceName2 := "test-delete-all-nimservice-2-" + randStringBytes(5)
			deployTestNIMService(namespace, nimServiceName1, nimCacheName)
			deployTestNIMService(namespace, nimServiceName2, nimCacheName)

			// Delete each nimservice individually
			cmd := exec.Command("kubectl", "nim", "delete", "nimservice", nimServiceName1, "--namespace="+namespace)
			_, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("kubectl", "nim", "delete", "nimservice", nimServiceName2, "--namespace="+namespace)
			_, err = runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())

			// Verify all are gone
			cmd = exec.Command("kubectl", "get", "nimservice", "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("No resources found"))
		})

		It("should handle basic delete", func() {
			nimServiceName := "test-basic-delete-nimservice-" + randStringBytes(5)
			deployTestNIMService(namespace, nimServiceName, nimCacheName)

			// Normal delete
			cmd := exec.Command("kubectl", "nim", "delete", "nimservice", nimServiceName,
				"--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("deleted"))

			// Verify it's gone
			Eventually(func() error {
				cmd = exec.Command("kubectl", "get", "nimservice", nimServiceName, "--namespace="+namespace)
				return cmd.Run()
			}, 10*time.Second, 1*time.Second).Should(HaveOccurred())
		})
	})

	Context("delete resources individually", func() {
		It("should delete both nimcache and nimservice individually", func() {
			// Create resources
			nimCacheName := "test-delete-all-cache-" + randStringBytes(5)
			nimServiceName := "test-delete-all-service-" + randStringBytes(5)
			deployTestNIMCache(namespace, nimCacheName)
			deployTestNIMService(namespace, nimServiceName, nimCacheName)

			// Delete nimservice first
			cmd := exec.Command("kubectl", "nim", "delete", "nimservice", nimServiceName, "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("deleted"))

			// Delete nimcache
			cmd = exec.Command("kubectl", "nim", "delete", "nimcache", nimCacheName, "--namespace="+namespace)
			output, err = runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("deleted"))

			// Verify all are gone
			cmd = exec.Command("kubectl", "get", "nimcache", "--namespace="+namespace)
			output, err = runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("No resources found"))

			cmd = exec.Command("kubectl", "get", "nimservice", "--namespace="+namespace)
			output, err = runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("No resources found"))
		})
	})

	Context("delete nimcache and verify", func() {
		It("should delete and verify deletion", func() {
			nimCacheName := "test-delete-verify-nimcache-" + randStringBytes(5)
			deployTestNIMCache(namespace, nimCacheName)

			// Delete normally
			cmd := exec.Command("kubectl", "nim", "delete", "nimcache", nimCacheName,
				"--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("deleted"))

			// Resource should be gone
			cmd = exec.Command("kubectl", "get", "nimcache", nimCacheName, "--namespace="+namespace)
			err = cmd.Run()
			Expect(err).To(HaveOccurred())
		})
	})
})
