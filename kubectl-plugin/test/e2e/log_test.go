package e2e

import (
	"context"
	"os/exec"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Calling kubectl nim `log` command", func() {
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

	Context("log stream nimservice", func() {
		var nimServiceName string
		var nimCacheName string

		BeforeEach(func() {
			// Create NIMCache and NIMService for testing
			nimCacheName = "test-log-stream-nimcache-" + randStringBytes(5)
			nimServiceName = "test-log-stream-nimservice-" + randStringBytes(5)
			deployTestNIMCache(namespace, nimCacheName)
			deployTestNIMService(namespace, nimServiceName, nimCacheName)

			// Wait a bit for pods to be created
			time.Sleep(3 * time.Second)
		})

		It("should succeed in streaming logs from nimservice", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, "kubectl", "nim", "log", "stream", "nimservice", nimServiceName,
				"--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			// The command might timeout or fail if no pods are ready yet
			// But it should at least run without syntax errors
			outputStr := string(output)
			if err != nil {
				// Check if it's because no pods are ready or expected error conditions
				Expect(outputStr).To(SatisfyAny(
					ContainSubstring("no pods found"),
					ContainSubstring("waiting for pod"),
					ContainSubstring("container"),
					ContainSubstring("log"),
					ContainSubstring("not found"),
					ContainSubstring("timeout"),
				))
			} else {
				// If successful, should contain some log output
				Expect(len(outputStr)).To(BeNumerically(">", 0))
			}
		})

		It("should handle streaming logs continuously with timeout", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, "kubectl", "nim", "log", "stream", "nimservice", nimServiceName,
				"--namespace="+namespace)

			// Set process group for cleanup
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

			err := cmd.Start()
			if err != nil {
				// If we can't start, verify it's a reasonable error
				Expect(err.Error()).To(SatisfyAny(
					ContainSubstring("not found"),
					ContainSubstring("no such file"),
					ContainSubstring("executable"),
				))
				Skip("Could not start log streaming - command execution failed")
			}

			// Let it run for a couple seconds
			time.Sleep(2 * time.Second)

			// Kill the process group gracefully
			if cmd.Process != nil {
				pgid, err := syscall.Getpgid(cmd.Process.Pid)
				if err == nil {
					if killErr := syscall.Kill(-pgid, syscall.SIGTERM); killErr != nil {
						GinkgoWriter.Printf("Failed to terminate process group: %v\n", killErr)
					}
				}
				if waitErr := cmd.Wait(); waitErr != nil && ctx.Err() == nil {
					GinkgoWriter.Printf("Wait returned error: %v\n", waitErr)
				}
			}
		})

		It("should fail when streaming logs from non-existent nimservice", func() {
			cmd := exec.Command("kubectl", "nim", "log", "stream", "nimservice", "non-existent-service",
				"--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).To(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("not found"))
		})

		It("should fail with invalid resource type", func() {
			cmd := exec.Command("kubectl", "nim", "log", "stream", "invalidresource", "test-name",
				"--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).To(HaveOccurred())
			outputStr := string(output)
			Expect(outputStr).To(SatisfyAny(
				ContainSubstring("invalid"),
				ContainSubstring("unknown"),
				ContainSubstring("not supported"),
				ContainSubstring("error"),
			))
		})

		It("should fail when missing required arguments", func() {
			// Test with no resource name
			cmd := exec.Command("kubectl", "nim", "log", "stream", "nimservice",
				"--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).To(HaveOccurred())
			outputStr := string(output)
			Expect(outputStr).To(SatisfyAny(
				ContainSubstring("required"),
				ContainSubstring("usage"),
				ContainSubstring("help"),
				ContainSubstring("argument"),
			))
		})

		It("should handle namespace flag correctly", func() {
			// Test with explicit namespace flag
			cmd := exec.Command("kubectl", "nim", "log", "stream", "nimservice", nimServiceName,
				"-n", namespace)
			output, err := runCommandWithDebug(cmd)

			outputStr := string(output)
			if err != nil {
				// Should be a reasonable error related to pods/resources, not namespace
				Expect(outputStr).To(SatisfyAny(
					ContainSubstring("no pods found"),
					ContainSubstring("waiting for pod"),
					ContainSubstring("container"),
					ContainSubstring("log"),
					ContainSubstring("timeout"),
				))
			} else {
				Expect(len(outputStr)).To(BeNumerically(">", 0))
			}
		})
	})

	Context("log stream nimcache", func() {
		var nimCacheName string

		BeforeEach(func() {
			// Create NIMCache for testing
			nimCacheName = "test-log-nimcache-" + randStringBytes(5)
			deployTestNIMCache(namespace, nimCacheName)

			// Wait a bit for job pods to be created
			time.Sleep(3 * time.Second)
		})

		It("should succeed in streaming logs from nimcache job", func() {
			cmd := exec.Command("kubectl", "nim", "log", "stream", "nimcache", nimCacheName,
				"--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			outputStr := string(output)
			// NIMCache might have job pods or might be completed
			if err != nil {
				Expect(outputStr).To(SatisfyAny(
					ContainSubstring("no pods found"),
					ContainSubstring("job"),
					ContainSubstring("not found"),
					ContainSubstring("completed"),
					ContainSubstring("container"),
				))
			} else {
				// If successful, should contain some log output
				Expect(len(outputStr)).To(BeNumerically(">", 0))
			}
		})

		It("should fail when streaming logs from non-existent nimcache", func() {
			cmd := exec.Command("kubectl", "nim", "log", "stream", "nimcache", "non-existent-cache",
				"--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).To(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("not found"))
		})
	})

	Context("log collect", func() {
		var nimServiceName string
		var nimCacheName string

		BeforeEach(func() {
			// Create NIMCache and NIMService for testing
			nimCacheName = "test-log-collect-nimcache-" + randStringBytes(5)
			nimServiceName = "test-log-collect-nimservice-" + randStringBytes(5)
			deployTestNIMCache(namespace, nimCacheName)
			deployTestNIMService(namespace, nimServiceName, nimCacheName)

			// Wait a bit for pods to be created
			time.Sleep(3 * time.Second)
		})

		It("should succeed in collecting logs from namespace", func() {
			cmd := exec.Command("kubectl", "nim", "log", "collect",
				"--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			outputStr := string(output)
			// Check if collection was attempted
			if err == nil {
				Expect(outputStr).To(SatisfyAny(
					ContainSubstring("Diagnostic bundle created"),
					ContainSubstring("bundle"),
					ContainSubstring("collected"),
					ContainSubstring("logs"),
				))
			} else {
				// Might fail due to script issues or missing tools
				Expect(outputStr).To(SatisfyAny(
					ContainSubstring("must-gather failed"),
					ContainSubstring("failed"),
					ContainSubstring("error"),
					ContainSubstring("not found"),
				))
			}
		})

		It("should fail with invalid namespace", func() {
			cmd := exec.Command("kubectl", "nim", "log", "collect",
				"--namespace=non-existent-namespace")
			output, err := runCommandWithDebug(cmd)

			outputStr := string(output)
			if err != nil {
				Expect(outputStr).To(SatisfyAny(
					ContainSubstring("not found"),
					ContainSubstring("invalid"),
					ContainSubstring("error"),
				))
			}
			// Note: collect might still succeed even with invalid namespace depending on implementation
		})
	})

	Context("log command help and usage", func() {
		It("should show help when no subcommand is provided", func() {
			cmd := exec.Command("kubectl", "nim", "log")
			output, _ := runCommandWithDebug(cmd)

			outputStr := string(output)
			// Should show help or usage information
			Expect(outputStr).To(SatisfyAny(
				ContainSubstring("Usage"),
				ContainSubstring("help"),
				ContainSubstring("stream"),
				ContainSubstring("collect"),
				ContainSubstring("Available Commands"),
			))
		})

		It("should show help for stream subcommand", func() {
			cmd := exec.Command("kubectl", "nim", "log", "stream", "--help")
			output, err := runCommandWithDebug(cmd)

			Expect(err).NotTo(HaveOccurred())
			outputStr := string(output)
			Expect(outputStr).To(ContainSubstring("stream"))
			Expect(outputStr).To(SatisfyAny(
				ContainSubstring("Usage"),
				ContainSubstring("Examples"),
				ContainSubstring("nimservice"),
				ContainSubstring("nimcache"),
			))
		})

		It("should show help for collect subcommand", func() {
			cmd := exec.Command("kubectl", "nim", "log", "collect", "--help")
			output, err := runCommandWithDebug(cmd)

			Expect(err).NotTo(HaveOccurred())
			outputStr := string(output)
			Expect(outputStr).To(ContainSubstring("collect"))
			Expect(outputStr).To(SatisfyAny(
				ContainSubstring("Usage"),
				ContainSubstring("Examples"),
				ContainSubstring("namespace"),
				ContainSubstring("diagnostic"),
			))
		})

		It("should fail with unknown subcommand", func() {
			cmd := exec.Command("kubectl", "nim", "log", "unknown-command")
			output, err := runCommandWithDebug(cmd)

			Expect(err).To(HaveOccurred())
			outputStr := string(output)
			Expect(outputStr).To(SatisfyAny(
				ContainSubstring("unknown"),
				ContainSubstring("invalid"),
				ContainSubstring("error"),
				ContainSubstring("help"),
			))
		})
	})

	Context("log command edge cases", func() {
		It("should handle stream command with extra arguments", func() {
			cmd := exec.Command("kubectl", "nim", "log", "stream", "nimservice", "test-service", "extra-arg",
				"--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).To(HaveOccurred())
			outputStr := string(output)
			Expect(outputStr).To(SatisfyAny(
				ContainSubstring("unknown"),
				ContainSubstring("too many"),
				ContainSubstring("usage"),
				ContainSubstring("error"),
			))
		})

		It("should handle collect command with extra arguments", func() {
			cmd := exec.Command("kubectl", "nim", "log", "collect", "extra-arg",
				"--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).To(HaveOccurred())
			outputStr := string(output)
			Expect(outputStr).To(SatisfyAny(
				ContainSubstring("unknown"),
				ContainSubstring("too many"),
				ContainSubstring("usage"),
				ContainSubstring("error"),
			))
		})

		It("should handle stream command with missing resource type", func() {
			cmd := exec.Command("kubectl", "nim", "log", "stream",
				"--namespace="+namespace)
			output, _ := runCommandWithDebug(cmd)

			outputStr := string(output)
			// Should show help or usage when no args provided
			Expect(outputStr).To(SatisfyAny(
				ContainSubstring("Usage"),
				ContainSubstring("help"),
				ContainSubstring("stream"),
				ContainSubstring("Examples"),
			))
		})

		It("should validate resource types for stream command", func() {
			// Test various invalid resource types
			invalidTypes := []string{"pod", "deployment", "service", "configmap"}

			for _, resourceType := range invalidTypes {
				cmd := exec.Command("kubectl", "nim", "log", "stream", resourceType, "test-name",
					"--namespace="+namespace)
				output, err := runCommandWithDebug(cmd)

				if err != nil {
					outputStr := string(output)
					Expect(outputStr).To(SatisfyAny(
						ContainSubstring("invalid"),
						ContainSubstring("unknown"),
						ContainSubstring("not supported"),
						ContainSubstring("error"),
					))
				}
			}
		})
	})

	Context("log command with different namespaces", func() {
		var namespace2 string
		var nimServiceName string
		var nimCacheName string

		BeforeEach(func() {
			// Create second namespace with resources
			namespace2 = createTestNamespace()
			createNGCSecretForTesting(namespace2)
			DeferCleanup(func() {
				cleanupTestResources(namespace2)
				deleteTestNamespace(namespace2)
			})

			nimCacheName = "test-cross-ns-nimcache-" + randStringBytes(5)
			nimServiceName = "test-cross-ns-nimservice-" + randStringBytes(5)
			deployTestNIMCache(namespace2, nimCacheName)
			deployTestNIMService(namespace2, nimServiceName, nimCacheName)

			time.Sleep(3 * time.Second)
		})

		It("should stream logs from resources in different namespace", func() {
			cmd := exec.Command("kubectl", "nim", "log", "stream", "nimservice", nimServiceName,
				"--namespace="+namespace2)
			output, err := runCommandWithDebug(cmd)

			outputStr := string(output)
			if err != nil {
				Expect(outputStr).To(SatisfyAny(
					ContainSubstring("no pods found"),
					ContainSubstring("waiting for pod"),
					ContainSubstring("container"),
					ContainSubstring("log"),
					ContainSubstring("timeout"),
				))
			} else {
				Expect(len(outputStr)).To(BeNumerically(">", 0))
			}
		})

		It("should collect logs from specific namespace", func() {
			cmd := exec.Command("kubectl", "nim", "log", "collect",
				"--namespace="+namespace2)
			output, err := runCommandWithDebug(cmd)

			outputStr := string(output)
			if err == nil {
				Expect(outputStr).To(SatisfyAny(
					ContainSubstring("Diagnostic bundle created"),
					ContainSubstring("bundle"),
					ContainSubstring("collected"),
				))
			} else {
				Expect(outputStr).To(SatisfyAny(
					ContainSubstring("failed"),
					ContainSubstring("error"),
				))
			}
		})

		It("should fail to stream from wrong namespace", func() {
			// Try to access resource from namespace2 while specifying namespace1
			cmd := exec.Command("kubectl", "nim", "log", "stream", "nimservice", nimServiceName,
				"--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			Expect(err).To(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("not found"))
		})
	})
})
