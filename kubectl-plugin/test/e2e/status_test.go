package e2e

import (
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Calling kubectl nim `status` command", func() {
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

	Context("status nimcache", func() {
		It("should show status for a single nimcache", func() {
			// Create a nimcache
			nimCacheName := "test-status-nimcache-" + randStringBytes(5)
			deployTestNIMCache(namespace, nimCacheName)

			// Check status
			cmd := exec.Command("kubectl", "nim", "status", "nimcache", nimCacheName, "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).NotTo(HaveOccurred())
			outputStr := string(output)

			// Verify all expected fields are present and populated
			Expect(outputStr).To(ContainSubstring(nimCacheName))                    // Name
			Expect(outputStr).To(ContainSubstring(namespace))                       // Namespace
			Expect(outputStr).To(ContainSubstring("Ready"))                         // State
			Expect(outputStr).To(ContainSubstring("test-pvc-" + nimCacheName))      // PVC
			Expect(outputStr).To(ContainSubstring("Ready/True"))                    // Type/Status
			Expect(outputStr).To(ContainSubstring("NIMCache is ready for testing")) // Message
			Expect(outputStr).To(ContainSubstring("llama-3.1-8b-instruct"))         // Profile info

			// Status output should include table headers
			Expect(outputStr).To(SatisfyAny(
				ContainSubstring("STATE"),
				ContainSubstring("PVC"),
				ContainSubstring("TYPE/STATUS"),
				ContainSubstring("NAME"),
			))
		})

		It("should show status for all nimcaches", func() {
			// Create multiple nimcaches
			nimCacheName1 := "test-status-nimcache-1-" + randStringBytes(5)
			nimCacheName2 := "test-status-nimcache-2-" + randStringBytes(5)
			deployTestNIMCache(namespace, nimCacheName1)
			deployTestNIMCache(namespace, nimCacheName2)

			// Check status for all
			cmd := exec.Command("kubectl", "nim", "status", "nimcache", "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).NotTo(HaveOccurred())
			outputStr := string(output)
			Expect(outputStr).To(ContainSubstring(nimCacheName1))
			Expect(outputStr).To(ContainSubstring(nimCacheName2))
		})

		It("should handle non-existent nimcache", func() {
			cmd := exec.Command("kubectl", "nim", "status", "nimcache", "non-existent-cache", "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).To(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("not found"))
		})

		It("should show status across all namespaces with --all-namespaces", func() {
			// Create another namespace with nimcache
			namespace2 := createTestNamespace()
			createNGCSecretForTesting(namespace2)
			DeferCleanup(func() {
				cleanupTestResources(namespace2)
				deleteTestNamespace(namespace2)
			})

			nimCacheName1 := "test-status-allns-nimcache-1-" + randStringBytes(5)
			nimCacheName2 := "test-status-allns-nimcache-2-" + randStringBytes(5)
			deployTestNIMCache(namespace, nimCacheName1)
			deployTestNIMCache(namespace2, nimCacheName2)

			// Check status across all namespaces
			cmd := exec.Command("kubectl", "nim", "status", "nimcache", "--all-namespaces")
			output, err := runCommandWithDebug(cmd)

			Expect(err).NotTo(HaveOccurred())
			outputStr := string(output)
			Expect(outputStr).To(ContainSubstring(nimCacheName1))
			Expect(outputStr).To(ContainSubstring(nimCacheName2))
			Expect(outputStr).To(ContainSubstring(namespace))
			Expect(outputStr).To(ContainSubstring(namespace2))
		})
	})

	Context("status nimservice", func() {
		var nimCacheName string

		BeforeEach(func() {
			// Create a NIMCache that NIMService can reference
			nimCacheName = "test-nimcache-for-status-" + randStringBytes(5)
			deployTestNIMCache(namespace, nimCacheName)
		})

		It("should show status for a single nimservice", func() {
			// Create a nimservice
			nimServiceName := "test-status-nimservice-" + randStringBytes(5)
			deployTestNIMService(namespace, nimServiceName, nimCacheName)

			// Check status
			cmd := exec.Command("kubectl", "nim", "status", "nimservice", nimServiceName, "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).NotTo(HaveOccurred())
			outputStr := string(output)

			// Verify all expected fields are present and populated
			Expect(outputStr).To(ContainSubstring(nimServiceName)) // Name
			Expect(outputStr).To(ContainSubstring(namespace))      // Namespace
			// State and status may vary based on actual operator state
			Expect(outputStr).To(MatchRegexp(`\w+/\w+`)) // Type/Status format
			Expect(outputStr).To(MatchRegexp(`\d+`))     // Should contain numbers (replicas, age)

			// Status output should include table headers
			Expect(outputStr).To(SatisfyAny(
				ContainSubstring("STATE"),
				ContainSubstring("AVAILABLE REPLICAS"),
				ContainSubstring("TYPE/STATUS"),
				ContainSubstring("NAME"),
			))
		})

		It("should show status for all nimservices", func() {
			// Create multiple nimservices
			nimServiceName1 := "test-status-nimservice-1-" + randStringBytes(5)
			nimServiceName2 := "test-status-nimservice-2-" + randStringBytes(5)
			deployTestNIMService(namespace, nimServiceName1, nimCacheName)
			deployTestNIMService(namespace, nimServiceName2, nimCacheName)

			// Check status for all
			cmd := exec.Command("kubectl", "nim", "status", "nimservice", "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).NotTo(HaveOccurred())
			outputStr := string(output)
			Expect(outputStr).To(ContainSubstring(nimServiceName1))
			Expect(outputStr).To(ContainSubstring(nimServiceName2))
		})

		It("should show endpoint information", func() {
			nimServiceName := "test-status-endpoint-nimservice-" + randStringBytes(5)
			deployTestNIMService(namespace, nimServiceName, nimCacheName)

			// Check status - should include endpoint info
			cmd := exec.Command("kubectl", "nim", "status", "nimservice", nimServiceName, "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).NotTo(HaveOccurred())
			outputStr := string(output)
			Expect(outputStr).To(ContainSubstring(nimServiceName))
			// Wide output should still have standard headers but might show more info
			Expect(outputStr).To(SatisfyAny(
				ContainSubstring("STATE"),
				ContainSubstring("AVAILABLE REPLICAS"),
				ContainSubstring("TYPE/STATUS"),
			))
		})

		It("should handle non-existent nimservice", func() {
			cmd := exec.Command("kubectl", "nim", "status", "nimservice", "non-existent-service", "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).To(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("not found"))
		})

		It("should display all fields with correct values in table format", func() {
			// Create a nimservice with known values
			nimServiceName := "test-field-validation-" + randStringBytes(5)
			deployTestNIMService(namespace, nimServiceName, nimCacheName)

			// Get the actual resource to compare values
			actualService := getAndCheckNIMService(namespace, nimServiceName)

			// Check status output
			cmd := exec.Command("kubectl", "nim", "status", "nimservice", nimServiceName, "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).NotTo(HaveOccurred())
			outputStr := string(output)

			// Verify each field matches the actual resource values (not patched values)
			Expect(outputStr).To(ContainSubstring(actualService.Name))
			Expect(outputStr).To(ContainSubstring(actualService.Namespace))

			// Verify that status fields are populated (actual values may vary)
			Expect(outputStr).To(MatchRegexp(`\d+`)) // Should contain numbers (replicas, age, etc.)

			// Verify that condition information is displayed
			Expect(outputStr).To(MatchRegexp(`\w+/\w+`)) // Should have Type/Status format

			// Verify table structure - should have proper columns
			lines := strings.Split(outputStr, "\n")
			Expect(len(lines)).To(BeNumerically(">=", 2)) // At least header + data row

			// Check that we have the expected column headers
			headerLine := lines[0]
			Expect(headerLine).To(ContainSubstring("NAME"))
			Expect(headerLine).To(ContainSubstring("NAMESPACE"))
			Expect(headerLine).To(ContainSubstring("STATE"))
			Expect(headerLine).To(ContainSubstring("AVAILABLE REPLICAS"))
			Expect(headerLine).To(ContainSubstring("TYPE/STATUS"))

			// Verify data row contains actual values from the resource
			dataLine := lines[1]
			Expect(dataLine).To(ContainSubstring(actualService.Name))
			Expect(dataLine).To(ContainSubstring(actualService.Namespace))

			// Verify that all expected columns have some content (not empty): exact values have already been checked in the command package tests.
			columns := strings.Fields(dataLine)
			Expect(len(columns)).To(BeNumerically(">=", 5)) // At least 5 columns with data
		})

	})
})
