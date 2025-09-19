package e2e

import (
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Calling kubectl nim `deploy` command", func() {
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

	Context("deploy with no caching", func() {
		It("should succeed in deploying a NIMService without caching", func() {
			serviceName := "test-deploy-service-" + randStringBytes(5)

			// Prepare input for interactive deploy command
			// Format: cache_model(no) + model_url + storage_class
			input := "no\nngc://nvidia/nemo/llama-3.1-8b-instruct:2.0\nstandard\n"

			cmd := exec.Command("kubectl", "nim", "deploy", serviceName, "--namespace="+namespace)
			cmd.Stdin = strings.NewReader(input)
			output, err := runCommandWithDebug(cmd)

			if err != nil {
				GinkgoWriter.Printf("Deploy command failed: %v\n", err)
				GinkgoWriter.Printf("Output: %s\n", string(output))
			}

			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify the NIMService was created
			nimService := getAndCheckNIMService(namespace, serviceName)
			Expect(nimService).NotTo(BeNil())
			Expect(nimService.Name).To(Equal(serviceName))
			Expect(nimService.Namespace).To(Equal(namespace))

			// Wait for readiness if verification is enabled
			if shouldWaitForReadiness() {
				waitForNIMServiceReady(namespace, serviceName)
			}

			// Verify the service has the expected configuration
			Expect(nimService.Spec.Image.Repository).To(Equal("nvcr.io/nim/nvidia/llm-nim"))
			Expect(nimService.Spec.Image.Tag).To(Equal("1.12"))

			// Verify environment variables are set correctly
			found := false
			for _, env := range nimService.Spec.Env {
				if env.Name == "NIM_MODEL_NAME" {
					found = true
					Expect(env.Value).To(Equal("ngc://nvidia/nemo/llama-3.1-8b-instruct:2.0"))
					break
				}
			}
			Expect(found).To(BeTrue(), "NIM_MODEL_NAME environment variable should be set")

			// Verify PVC configuration
			Expect(*nimService.Spec.Storage.PVC.Create).To(BeTrue())
			Expect(nimService.Spec.Storage.PVC.Size).To(Equal("20Gi"))
			Expect(string(nimService.Spec.Storage.PVC.VolumeAccessMode)).To(Equal("ReadWriteOnce"))
		})

		It("should succeed with HuggingFace model URL", func() {
			serviceName := "test-deploy-hf-" + randStringBytes(5)

			// Test with HuggingFace model URL
			input := "no\nhf://meta-llama/Llama-3.2-1B-Instruct\n\n" // empty storage class

			cmd := exec.Command("kubectl", "nim", "deploy", serviceName, "--namespace="+namespace)
			cmd.Stdin = strings.NewReader(input)
			output, err := runCommandWithDebug(cmd)

			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify the NIMService was created with HF model
			nimService := getAndCheckNIMService(namespace, serviceName)
			Expect(nimService).NotTo(BeNil())

			// Wait for readiness if verification is enabled
			if shouldWaitForReadiness() {
				waitForNIMServiceReady(namespace, serviceName)
			}

			// Verify HF model environment variable
			found := false
			for _, env := range nimService.Spec.Env {
				if env.Name == "NIM_MODEL_NAME" {
					found = true
					Expect(env.Value).To(Equal("hf://meta-llama/Llama-3.2-1B-Instruct"))
					break
				}
			}
			Expect(found).To(BeTrue(), "NIM_MODEL_NAME should contain HuggingFace model URL")
		})
	})

	Context("deploy with NGC caching", func() {
		It("should succeed in deploying with NGC model caching", func() {
			serviceName := "test-deploy-cached-" + randStringBytes(5)

			// Prepare input for caching scenario
			// Format: cache_model(yes) + source(ngc) + model_url + storage_class
			input := "yes\nngc\nngc://nvidia/nemo/llama-3.1-8b-instruct:2.0\nfast-ssd\n"

			cmd := exec.Command("kubectl", "nim", "deploy", serviceName, "--namespace="+namespace)
			cmd.Stdin = strings.NewReader(input)
			output, err := runCommandWithDebug(cmd)

			if err != nil {
				GinkgoWriter.Printf("Deploy with caching failed: %v\n", err)
				GinkgoWriter.Printf("Output: %s\n", string(output))
			}

			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify both NIMCache and NIMService were created
			cacheName := serviceName + "-cache"

			nimCache := getAndCheckNIMCache(namespace, cacheName)
			Expect(nimCache).NotTo(BeNil())
			Expect(nimCache.Name).To(Equal(cacheName))
			Expect(nimCache.Spec.Source.NGC).NotTo(BeNil())

			nimService := getAndCheckNIMService(namespace, serviceName)
			Expect(nimService).NotTo(BeNil())
			Expect(nimService.Name).To(Equal(serviceName))

			// Wait for readiness if verification is enabled
			if shouldWaitForReadiness() {
				waitForNIMCacheReady(namespace, cacheName)
				waitForNIMServiceReady(namespace, serviceName)
			}

			// Verify NIMService references the cache
			Expect(nimService.Spec.Storage.NIMCache.Name).To(Equal(cacheName))

			// Verify NIMCache has correct configuration
			Expect(*nimCache.Spec.Storage.PVC.Create).To(BeTrue())
			Expect(nimCache.Spec.Storage.PVC.Size).To(Equal("20Gi"))
			Expect(string(nimCache.Spec.Storage.PVC.VolumeAccessMode)).To(Equal("ReadWriteOnce"))
			Expect(nimCache.Spec.Storage.PVC.StorageClass).To(Equal("fast-ssd"))
		})
	})

	Context("deploy with HuggingFace caching", func() {
		It("should succeed in deploying with HuggingFace model caching", func() {
			serviceName := "test-deploy-hf-cached-" + randStringBytes(5)

			// Prepare input for HuggingFace caching scenario
			// Format: cache_model(yes) + source(huggingface) + endpoint + namespace + model_name + storage_class
			input := "yes\nhuggingface\nhttps://huggingface.co/api\nmeta-llama\nLlama-3.2-1B-Instruct\n\n" // empty storage class

			cmd := exec.Command("kubectl", "nim", "deploy", serviceName, "--namespace="+namespace)
			cmd.Stdin = strings.NewReader(input)
			output, err := runCommandWithDebug(cmd)

			if err != nil {
				GinkgoWriter.Printf("Deploy with HF caching failed: %v\n", err)
				GinkgoWriter.Printf("Output: %s\n", string(output))
			}

			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify both NIMCache and NIMService were created
			cacheName := serviceName + "-cache"

			nimCache := getAndCheckNIMCache(namespace, cacheName)
			Expect(nimCache).NotTo(BeNil())
			Expect(nimCache.Spec.Source.HF).NotTo(BeNil())
			Expect(nimCache.Spec.Source.HF.Endpoint).To(Equal("https://huggingface.co/api"))

			nimService := getAndCheckNIMService(namespace, serviceName)
			Expect(nimService).NotTo(BeNil())
			Expect(nimService.Spec.Storage.NIMCache.Name).To(Equal(cacheName))

			// Wait for readiness if verification is enabled
			if shouldWaitForReadiness() {
				waitForNIMCacheReady(namespace, cacheName)
				waitForNIMServiceReady(namespace, serviceName)
			}
		})
	})

	Context("deploy with nemoDataStore caching", func() {
		It("should succeed in deploying with NeMo DataStore model caching", func() {
			serviceName := "test-deploy-nemo-cached-" + randStringBytes(5)

			// Prepare input for NeMo DataStore caching scenario
			// NeMo DataStore endpoint must match pattern: ^https?://.*/v1/hf/?$
			input := "yes\nnemoDataStore\nhttps://nemo.example.com/v1/hf\nnemo-models\ncustom-model-v2\ngp3\n"

			cmd := exec.Command("kubectl", "nim", "deploy", serviceName, "--namespace="+namespace)
			cmd.Stdin = strings.NewReader(input)
			output, err := runCommandWithDebug(cmd)

			if err != nil {
				GinkgoWriter.Printf("Deploy with NeMo caching failed: %v\n", err)
				GinkgoWriter.Printf("Output: %s\n", string(output))
			}

			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify both NIMCache and NIMService were created
			cacheName := serviceName + "-cache"

			nimCache := getAndCheckNIMCache(namespace, cacheName)
			Expect(nimCache).NotTo(BeNil())
			Expect(nimCache.Spec.Source.DataStore).NotTo(BeNil())
			Expect(nimCache.Spec.Source.DataStore.Endpoint).To(Equal("https://nemo.example.com/v1/hf"))

			nimService := getAndCheckNIMService(namespace, serviceName)
			Expect(nimService).NotTo(BeNil())
			Expect(nimService.Spec.Storage.NIMCache.Name).To(Equal(cacheName))

			// Wait for readiness if verification is enabled
			if shouldWaitForReadiness() {
				waitForNIMCacheReady(namespace, cacheName)
				waitForNIMServiceReady(namespace, serviceName)
			}

			// Verify storage class is set correctly
			Expect(nimCache.Spec.Storage.PVC.StorageClass).To(Equal("gp3"))
		})
	})

	Context("deploy error handling", func() {
		It("should show help when no arguments provided", func() {
			cmd := exec.Command("kubectl", "nim", "deploy", "--namespace="+namespace)
			output, err := runCommandWithDebug(cmd)

			// The command should succeed but show help
			Expect(err).NotTo(HaveOccurred())
			// Help output should contain usage information
			Expect(string(output)).To(SatisfyAny(
				ContainSubstring("Usage:"),
				ContainSubstring("deploy NAME"),
				ContainSubstring("Interactively deploy"),
			))
		})

		It("should handle invalid responses gracefully", func() {
			serviceName := "test-deploy-invalid-" + randStringBytes(5)

			// Provide invalid response first, then valid ones
			input := "maybe\nno\nngc://nvidia/nemo/llama-3.1-8b-instruct:2.0\n\n"

			cmd := exec.Command("kubectl", "nim", "deploy", serviceName, "--namespace="+namespace)
			cmd.Stdin = strings.NewReader(input)
			output, err := runCommandWithDebug(cmd)

			// Should eventually succeed after retry
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Should contain the retry message
			Expect(string(output)).To(ContainSubstring("Invalid response"))
		})

		It("should handle invalid image source gracefully", func() {
			serviceName := "test-deploy-invalid-source-" + randStringBytes(5)

			// Provide invalid image source first, then valid ones
			input := "yes\ninvalid-source\nngc\nngc://nvidia/nemo/llama-3.1-8b-instruct:2.0\n\n"

			cmd := exec.Command("kubectl", "nim", "deploy", serviceName, "--namespace="+namespace)
			cmd.Stdin = strings.NewReader(input)
			output, err := runCommandWithDebug(cmd)

			// Should eventually succeed after retry
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify the resources were created correctly
			cacheName := serviceName + "-cache"
			nimCache := getAndCheckNIMCache(namespace, cacheName)
			Expect(nimCache).NotTo(BeNil())
			Expect(nimCache.Spec.Source.NGC).NotTo(BeNil())
		})
	})

	Context("deploy PVC configuration", func() {
		It("should create PVC with default storage class", func() {
			serviceName := "test-deploy-pvc-custom-" + randStringBytes(5)

			input := "no\nngc://nvidia/nemo/llama-3.1-8b-instruct:2.0\n\n" // Use default storage class

			cmd := exec.Command("kubectl", "nim", "deploy", serviceName, "--namespace="+namespace)
			cmd.Stdin = strings.NewReader(input)
			output, err := runCommandWithDebug(cmd)

			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			nimService := getAndCheckNIMService(namespace, serviceName)
			Expect(nimService).NotTo(BeNil())

			// Wait for readiness if verification is enabled
			if shouldWaitForReadiness() {
				waitForNIMServiceReady(namespace, serviceName)
			}

			// Verify PVC configuration with custom storage class
			Expect(*nimService.Spec.Storage.PVC.Create).To(BeTrue())
			Expect(nimService.Spec.Storage.PVC.Size).To(Equal("20Gi"))
			Expect(nimService.Spec.Storage.PVC.StorageClass).To(Equal("")) // Empty means default storage class
		})

		It("should create PVC with default storage class when empty", func() {
			serviceName := "test-deploy-pvc-default-" + randStringBytes(5)

			input := "no\nngc://nvidia/nemo/llama-3.1-8b-instruct:2.0\n\n" // empty storage class

			cmd := exec.Command("kubectl", "nim", "deploy", serviceName, "--namespace="+namespace)
			cmd.Stdin = strings.NewReader(input)
			output, err := runCommandWithDebug(cmd)

			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			nimService := getAndCheckNIMService(namespace, serviceName)
			Expect(nimService).NotTo(BeNil())

			// Wait for readiness if verification is enabled
			if shouldWaitForReadiness() {
				waitForNIMServiceReady(namespace, serviceName)
			}

			// Verify PVC configuration uses default storage class
			Expect(*nimService.Spec.Storage.PVC.Create).To(BeTrue())
			Expect(nimService.Spec.Storage.PVC.Size).To(Equal("20Gi"))
			// StorageClass should be empty when using default
			Expect(nimService.Spec.Storage.PVC.StorageClass).To(BeEmpty())
		})
	})
})
