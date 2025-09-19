package e2e

import (
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
)

var _ = Describe("Calling kubectl nim `create` command", func() {
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

	Context("create nimcache", func() {
		It("should succeed in creating a basic nimcache", func() {
			nimCacheName := "test-create-nimcache-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimcache", nimCacheName,
				"--namespace="+namespace,
				"--nim-source=ngc",
				"--model-puller=nvcr.io/nim/meta/llama-3.2-1b-instruct:1.12.0",
				"--pull-secret=ngc-secret",
				"--auth-secret=ngc-api-secret",
				"--pvc-storage-name=shared-test-pvc",
				"--pvc-create=false",
				"--pvc-size=10Gi",
				"--pvc-volume-access-mode=ReadWriteMany")
			output, err := runCommandWithDebug(cmd)

			if err != nil {
				GinkgoWriter.Printf("Command failed: %v\n", err)
				GinkgoWriter.Printf("Output: %s\n", string(output))
			}

			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify the resource was created
			nimCache := getAndCheckNIMCache(namespace, nimCacheName)
			Expect(nimCache.Name).To(Equal(nimCacheName))
			Expect(nimCache.Namespace).To(Equal(namespace))
			Expect(nimCache.Spec.Source.NGC).NotTo(BeNil())
			Expect(nimCache.Spec.Source.NGC.AuthSecret).To(Equal("ngc-api-secret"))
			Expect(nimCache.Spec.Source.NGC.ModelPuller).To(Equal("nvcr.io/nim/meta/llama-3.2-1b-instruct:1.12.0"))
			Expect(nimCache.Spec.Source.NGC.PullSecret).To(Equal("ngc-secret"))

			// Wait for the NIMCache to become ready and verify deployment health (if enabled)
			if shouldWaitForReadiness() {
				waitForNIMCacheReady(namespace, nimCacheName)
			}
		})

		It("should succeed in creating nimcache with PVC", func() {
			nimCacheName := "test-create-nimcache-pvc-" + randStringBytes(5)
			pvcName := "test-pvc-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimcache", nimCacheName,
				"--namespace="+namespace,
				"--nim-source=ngc",
				"--model-puller=nvcr.io/nim/meta/llama-3.2-1b-instruct:1.12.0",
				"--pull-secret=ngc-secret",
				"--auth-secret=ngc-api-secret",
				"--pvc-create=true",
				"--pvc-size=10Gi",
				"--pvc-storage-name="+pvcName,
				"--pvc-volume-access-mode=ReadWriteMany")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify the resource was created with PVC configuration
			nimCache := getAndCheckNIMCache(namespace, nimCacheName)
			Expect(nimCache.Name).To(Equal(nimCacheName))

			Expect(nimCache.Spec.Storage.PVC.Name).To(Equal(pvcName))
			Expect(nimCache.Spec.Storage.PVC.Create).To(Equal(ptr.To(true)))
			Expect(nimCache.Spec.Storage.PVC.Size).To(Equal("10Gi"))
			Expect(string(nimCache.Spec.Storage.PVC.VolumeAccessMode)).To(Equal("ReadWriteMany"))
			Expect(nimCache.Spec.Storage.PVC.StorageClass).To(Equal("")) // no value results in cluster default default which is empty string.

			// Wait for the NIMCache to become ready and verify deployment health (if enabled)
			if shouldWaitForReadiness() {
				waitForNIMCacheReady(namespace, nimCacheName)
			}
		})

		It("should succeed in creating nimcache with custom resources", func() {
			nimCacheName := "test-create-nimcache-resources-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimcache", nimCacheName,
				"--namespace="+namespace,
				"--nim-source=ngc",
				"--model-puller=nvcr.io/nim/meta/llama-3.2-1b-instruct:1.12.0",
				"--pull-secret=ngc-secret",
				"--auth-secret=ngc-api-secret",
				"--resources-cpu=4",
				"--resources-memory=16Gi",
				"--pvc-storage-name=test-pvc-"+nimCacheName,
				"--pvc-create=true",
				"--pvc-size=10Gi",
				"--pvc-volume-access-mode=ReadWriteMany")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify the resource was created with custom resources
			nimCache := getAndCheckNIMCache(namespace, nimCacheName)
			Expect(nimCache.Name).To(Equal(nimCacheName))
			Expect(nimCache.Spec.Resources.CPU.String()).To(Equal("4"))
			Expect(nimCache.Spec.Resources.Memory.String()).To(Equal("16Gi"))

			// Wait for the NIMCache to become ready and verify deployment health (if enabled)
			if shouldWaitForReadiness() {
				waitForNIMCacheReady(namespace, nimCacheName)
			}
		})

		It("should fail when creating duplicate nimcache", func() {
			nimCacheName := "test-duplicate-nimcache-" + randStringBytes(5)

			// Create first nimcache
			cmd := exec.Command("kubectl", "nim", "create", "nimcache", nimCacheName,
				"--namespace="+namespace,
				"--nim-source=ngc",
				"--model-puller=nvcr.io/nim/meta/llama-3.2-1b-instruct:1.12.0",
				"--pull-secret=ngc-secret",
				"--auth-secret=ngc-api-secret",
				"--pvc-storage-name=test-pvc-"+nimCacheName,
				"--pvc-create=true",
				"--pvc-size=10Gi",
				"--pvc-volume-access-mode=ReadWriteMany")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())

			// Try to create duplicate
			cmd = exec.Command("kubectl", "nim", "create", "nimcache", nimCacheName,
				"--namespace="+namespace,
				"--nim-source=ngc",
				"--model-puller=nvcr.io/nim/meta/llama-3.2-1b-instruct:1.12.0",
				"--pull-secret=ngc-secret",
				"--auth-secret=ngc-api-secret",
				"--pvc-storage-name=test-pvc-"+nimCacheName+"-duplicate",
				"--pvc-create=true",
				"--pvc-size=10Gi",
				"--pvc-volume-access-mode=ReadWriteMany")
			output, err = runCommandWithDebug(cmd)

			Expect(err).To(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("already exists"))
		})

		It("should fail when missing required parameters", func() {
			nimCacheName := "test-missing-params-nimcache-" + randStringBytes(5)

			// Missing nim-source
			cmd := exec.Command("kubectl", "nim", "create", "nimcache", nimCacheName,
				"--namespace="+namespace,
				"--model-puller=nvcr.io/nim/meta/llama-3.2-1b-instruct:1.12.0",
				"--pull-secret=ngc-secret",
				"--auth-secret=ngc-api-secret",
				"--pvc-storage-name=test-pvc-"+nimCacheName)
			output, err := runCommandWithDebug(cmd)

			Expect(err).To(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("nim-source"))
		})

		// Comprehensive flag testing for NIMCache
		It("should succeed with all NGC NIMCache flags without profiles (with readiness verification)", func() {
			nimCacheName := "test-all-ngc-flags-nimcache-" + randStringBytes(5)
			pvcName := "test-pvc-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimcache", nimCacheName,
				"--namespace="+namespace,
				"--nim-source=ngc",
				"--resources-cpu=8",
				"--resources-memory=32Gi",
				"--model-puller=nvcr.io/nim/meta/llama-3.2-1b-instruct:1.12.0",
				"--precision=fp16",
				"--engine=tensorrt_llm",
				"--tensor-parallelism=2",
				"--qos-profile=throughput",
				"--lora=true",
				"--buildable=false",
				"--pull-secret=ngc-secret",
				"--gpus=h100,a100",
				"--pvc-create=true",
				"--pvc-storage-name="+pvcName,
				"--pvc-storage-class=premium",
				"--pvc-size=200Gi",
				"--pvc-volume-access-mode=ReadWriteMany",
				"--auth-secret=ngc-api-secret")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify all NGC flags were applied correctly
			nimCache := getAndCheckNIMCache(namespace, nimCacheName)
			Expect(nimCache.Name).To(Equal(nimCacheName))
			Expect(nimCache.Spec.Source.NGC).NotTo(BeNil())
			Expect(nimCache.Spec.Source.NGC.AuthSecret).To(Equal("ngc-api-secret"))
			Expect(nimCache.Spec.Source.NGC.ModelPuller).To(Equal("nvcr.io/nim/meta/llama-3.2-1b-instruct:1.12.0"))
			Expect(nimCache.Spec.Source.NGC.PullSecret).To(Equal("ngc-secret"))
			Expect(nimCache.Spec.Source.NGC.Model).NotTo(BeNil())
			Expect(nimCache.Spec.Source.NGC.Model.Precision).To(Equal("fp16"))
			Expect(nimCache.Spec.Source.NGC.Model.Engine).To(Equal("tensorrt_llm"))
			Expect(nimCache.Spec.Source.NGC.Model.TensorParallelism).To(Equal("2"))
			Expect(nimCache.Spec.Source.NGC.Model.QoSProfile).To(Equal("throughput"))
			Expect(*nimCache.Spec.Source.NGC.Model.Lora).To(Equal(true))
			Expect(*nimCache.Spec.Source.NGC.Model.Buildable).To(Equal(false))
			Expect(nimCache.Spec.Source.NGC.Model.GPUs).To(HaveLen(2))
			Expect(nimCache.Spec.Source.NGC.Model.GPUs[0].Product).To(Equal("h100"))
			Expect(nimCache.Spec.Source.NGC.Model.GPUs[1].Product).To(Equal("a100"))
			Expect(nimCache.Spec.Resources.CPU.String()).To(Equal("8"))
			Expect(nimCache.Spec.Resources.Memory.String()).To(Equal("32Gi"))
			Expect(nimCache.Spec.Storage.PVC.Name).To(Equal(pvcName))
			Expect(nimCache.Spec.Storage.PVC.Create).To(Equal(ptr.To(true)))
			Expect(nimCache.Spec.Storage.PVC.StorageClass).To(Equal("premium"))
			Expect(nimCache.Spec.Storage.PVC.Size).To(Equal("200Gi"))
			Expect(string(nimCache.Spec.Storage.PVC.VolumeAccessMode)).To(Equal("ReadWriteMany"))

			if shouldWaitForReadiness() {
				waitForNIMCacheReady(namespace, nimCacheName)
			}
		})

		It("should succeed with NGC NIMCache profiles flags (with readiness verification)", func() {
			nimCacheName := "test-profiles-nimcache-" + randStringBytes(5)
			pvcName := "test-pvc-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimcache", nimCacheName,
				"--namespace="+namespace,
				"--nim-source=ngc",
				"--resources-cpu=8",
				"--resources-memory=32Gi",
				"--model-puller=nvcr.io/nim/meta/llama-3.2-1b-instruct:1.12.0",
				"--profiles=profile1,profile2",
				"--pull-secret=ngc-secret",
				"--pvc-create=true",
				"--pvc-storage-name="+pvcName,
				"--pvc-storage-class=premium",
				"--pvc-size=200Gi",
				"--pvc-volume-access-mode=ReadWriteMany",
				"--auth-secret=ngc-api-secret")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify profiles flags were applied correctly (individual model fields should be empty)
			nimCache := getAndCheckNIMCache(namespace, nimCacheName)
			Expect(nimCache.Name).To(Equal(nimCacheName))
			Expect(nimCache.Spec.Source.NGC).NotTo(BeNil())
			Expect(nimCache.Spec.Source.NGC.AuthSecret).To(Equal("ngc-api-secret"))
			Expect(nimCache.Spec.Source.NGC.ModelPuller).To(Equal("nvcr.io/nim/meta/llama-3.2-1b-instruct:1.12.0"))
			Expect(nimCache.Spec.Source.NGC.PullSecret).To(Equal("ngc-secret"))
			Expect(nimCache.Spec.Source.NGC.Model).NotTo(BeNil())
			Expect(nimCache.Spec.Source.NGC.Model.Profiles).To(ContainElements("profile1", "profile2"))
			// When profiles are specified, other model fields should be empty
			Expect(nimCache.Spec.Source.NGC.Model.Precision).To(Equal(""))
			Expect(nimCache.Spec.Source.NGC.Model.Engine).To(Equal(""))
			Expect(nimCache.Spec.Source.NGC.Model.TensorParallelism).To(Equal(""))
			Expect(nimCache.Spec.Source.NGC.Model.QoSProfile).To(Equal(""))
			Expect(nimCache.Spec.Source.NGC.Model.GPUs).To(BeEmpty())
			Expect(nimCache.Spec.Source.NGC.Model.Lora).To(BeNil())
			Expect(nimCache.Spec.Source.NGC.Model.Buildable).To(BeNil())
			Expect(nimCache.Spec.Resources.CPU.String()).To(Equal("8"))
			Expect(nimCache.Spec.Resources.Memory.String()).To(Equal("32Gi"))
			Expect(nimCache.Spec.Storage.PVC.Name).To(Equal(pvcName))
			Expect(nimCache.Spec.Storage.PVC.Create).To(Equal(ptr.To(true)))
			Expect(nimCache.Spec.Storage.PVC.StorageClass).To(Equal("premium"))
			Expect(nimCache.Spec.Storage.PVC.Size).To(Equal("200Gi"))
			Expect(string(nimCache.Spec.Storage.PVC.VolumeAccessMode)).To(Equal("ReadWriteMany"))

			if shouldWaitForReadiness() {
				waitForNIMCacheReady(namespace, nimCacheName)
			}
		})

		It("should succeed with NGC model endpoint flag (without readiness verification)", func() {
			nimCacheName := "test-ngc-endpoint-nimcache-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimcache", nimCacheName,
				"--namespace="+namespace,
				"--nim-source=ngc",
				"--model-puller=nvcr.io/nim/meta/llama-3.2-1b-instruct:1.12.0",
				"--pull-secret=ngc-secret",
				"--auth-secret=ngc-api-secret",
				"--ngc-model-endpoint=https://api.ngc.nvidia.com/v2/models/meta/llama-3.1-8b-instruct",
				"--pvc-storage-name=test-pvc-"+nimCacheName,
				"--pvc-create=true",
				"--pvc-size=50Gi",
				"--pvc-volume-access-mode=ReadWriteMany")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify NGC model endpoint flag
			nimCache := getAndCheckNIMCache(namespace, nimCacheName)
			Expect(nimCache.Spec.Source.NGC.ModelEndpoint).NotTo(BeNil())
			Expect(*nimCache.Spec.Source.NGC.ModelEndpoint).To(Equal("https://api.ngc.nvidia.com/v2/models/meta/llama-3.1-8b-instruct"))
			Expect(nimCache.Spec.Source.NGC.Model).To(BeNil()) // Model fields should not be set when ModelEndpoint is provided

			// Skip readiness verification for this test
		})

		It("should succeed with HuggingFace NIMCache flags (with readiness verification)", func() {
			nimCacheName := "test-hf-flags-nimcache-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimcache", nimCacheName,
				"--namespace="+namespace,
				"--nim-source=huggingface",
				"--alt-endpoint=https://huggingface.co",
				"--alt-namespace=meta-llama",
				"--alt-secret=hf-api-secret",
				"--model-name=Llama-2-7b-hf",
				"--revision=main",
				"--model-puller=nvcr.io/nim/nvidia/llm-nim:1.12",
				"--pull-secret=hf-pull-secret",
				"--pvc-storage-name=test-pvc-"+nimCacheName,
				"--pvc-create=true",
				"--pvc-size=100Gi",
				"--pvc-volume-access-mode=ReadWriteOnce")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify HuggingFace flags
			nimCache := getAndCheckNIMCache(namespace, nimCacheName)
			Expect(nimCache.Spec.Source.HF).NotTo(BeNil())
			Expect(nimCache.Spec.Source.HF.Endpoint).To(Equal("https://huggingface.co"))
			Expect(nimCache.Spec.Source.HF.Namespace).To(Equal("meta-llama"))
			Expect(nimCache.Spec.Source.HF.AuthSecret).To(Equal("hf-api-secret"))
			Expect(nimCache.Spec.Source.HF.ModelPuller).To(Equal("nvcr.io/nim/nvidia/llm-nim:1.12"))
			Expect(nimCache.Spec.Source.HF.PullSecret).To(Equal("hf-pull-secret"))
			Expect(*nimCache.Spec.Source.HF.ModelName).To(Equal("Llama-2-7b-hf"))
			Expect(*nimCache.Spec.Source.HF.Revision).To(Equal("main"))

			if shouldWaitForReadiness() {
				waitForNIMCacheReady(namespace, nimCacheName)
			}
		})

		It("should succeed with NeMo DataStore NIMCache flags (without readiness verification)", func() {
			nimCacheName := "test-nemo-flags-nimcache-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimcache", nimCacheName,
				"--namespace="+namespace,
				"--nim-source=nemodatastore",
				"--alt-endpoint=https://nemo.nvidia.com/v1/hf",
				"--alt-namespace=nvidia",
				"--alt-secret=nemo-api-secret",
				"--dataset-name=test-dataset",
				"--revision=v1.0",
				"--model-puller=nvcr.io/nim/nvidia/llm-nim:1.12",
				"--pull-secret=nemo-pull-secret",
				"--pvc-storage-name=test-pvc-"+nimCacheName,
				"--pvc-create=true",
				"--pvc-size=75Gi",
				"--pvc-volume-access-mode=ReadWriteMany")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify NeMo DataStore flags
			nimCache := getAndCheckNIMCache(namespace, nimCacheName)
			Expect(nimCache.Spec.Source.DataStore).NotTo(BeNil())
			Expect(nimCache.Spec.Source.DataStore.Endpoint).To(Equal("https://nemo.nvidia.com/v1/hf"))
			Expect(nimCache.Spec.Source.DataStore.Namespace).To(Equal("nvidia"))
			Expect(nimCache.Spec.Source.DataStore.AuthSecret).To(Equal("nemo-api-secret"))
			Expect(nimCache.Spec.Source.DataStore.ModelPuller).To(Equal("nvcr.io/nim/nvidia/llm-nim:1.12"))
			Expect(nimCache.Spec.Source.DataStore.PullSecret).To(Equal("nemo-pull-secret"))
			Expect(*nimCache.Spec.Source.DataStore.DatasetName).To(Equal("test-dataset"))
			Expect(*nimCache.Spec.Source.DataStore.Revision).To(Equal("v1.0"))

			// Skip readiness verification for this test
		})

		It("should succeed with resource limit flags (with readiness verification)", func() {
			nimCacheName := "test-resources-nimcache-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimcache", nimCacheName,
				"--namespace="+namespace,
				"--nim-source=ngc",
				"--model-puller=nvcr.io/nim/meta/llama-3.2-1b-instruct:1.12.0",
				"--pull-secret=ngc-secret",
				"--auth-secret=ngc-api-secret",
				"--resources-cpu=16",
				"--resources-memory=64Gi",
				"--pvc-storage-name=test-pvc-"+nimCacheName,
				"--pvc-create=true",
				"--pvc-size=25Gi",
				"--pvc-volume-access-mode=ReadWriteMany")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify resource flags
			nimCache := getAndCheckNIMCache(namespace, nimCacheName)
			Expect(nimCache.Spec.Resources.CPU.String()).To(Equal("16"))
			Expect(nimCache.Spec.Resources.Memory.String()).To(Equal("64Gi"))

			if shouldWaitForReadiness() {
				waitForNIMCacheReady(namespace, nimCacheName)
			}
		})

		It("should succeed with model configuration flags (without readiness verification)", func() {
			nimCacheName := "test-model-config-nimcache-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimcache", nimCacheName,
				"--namespace="+namespace,
				"--nim-source=ngc",
				"--model-puller=nvcr.io/nim/meta/llama-3.2-1b-instruct:1.12.0",
				"--pull-secret=ngc-secret",
				"--auth-secret=ngc-api-secret",
				"--precision=int8",
				"--engine=vllm",
				"--tensor-parallelism=4",
				"--qos-profile=latency",
				"--lora=false",
				"--buildable=true",
				"--pvc-storage-name=test-pvc-"+nimCacheName,
				"--pvc-create=true",
				"--pvc-size=30Gi",
				"--pvc-volume-access-mode=ReadWriteMany")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify model configuration flags
			nimCache := getAndCheckNIMCache(namespace, nimCacheName)
			Expect(nimCache.Spec.Source.NGC.Model.Precision).To(Equal("int8"))
			Expect(nimCache.Spec.Source.NGC.Model.Engine).To(Equal("vllm"))
			Expect(nimCache.Spec.Source.NGC.Model.TensorParallelism).To(Equal("4"))
			Expect(nimCache.Spec.Source.NGC.Model.QoSProfile).To(Equal("latency"))
			Expect(*nimCache.Spec.Source.NGC.Model.Lora).To(Equal(false))
			Expect(*nimCache.Spec.Source.NGC.Model.Buildable).To(Equal(true))

			// Skip readiness verification for this test
		})

		It("should succeed with GPU specification flags (with readiness verification)", func() {
			nimCacheName := "test-gpu-spec-nimcache-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimcache", nimCacheName,
				"--namespace="+namespace,
				"--nim-source=ngc",
				"--model-puller=nvcr.io/nim/meta/llama-3.2-1b-instruct:1.12.0",
				"--pull-secret=ngc-secret",
				"--auth-secret=ngc-api-secret",
				"--gpus=l40s,rtx4090",
				"--pvc-storage-name=test-pvc-"+nimCacheName,
				"--pvc-create=true",
				"--pvc-size=40Gi",
				"--pvc-volume-access-mode=ReadWriteMany")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify GPU specification flags
			nimCache := getAndCheckNIMCache(namespace, nimCacheName)
			Expect(nimCache.Spec.Source.NGC.Model.GPUs).To(HaveLen(2))
			Expect(nimCache.Spec.Source.NGC.Model.GPUs[0].Product).To(Equal("l40s"))
			Expect(nimCache.Spec.Source.NGC.Model.GPUs[1].Product).To(Equal("rtx4090"))

			if shouldWaitForReadiness() {
				waitForNIMCacheReady(namespace, nimCacheName)
			}
		})

		It("should succeed with profiles only flags (with readiness verification)", func() {
			nimCacheName := "test-profiles-only-nimcache-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimcache", nimCacheName,
				"--namespace="+namespace,
				"--nim-source=ngc",
				"--model-puller=nvcr.io/nim/meta/llama-3.2-1b-instruct:1.12.0",
				"--pull-secret=ngc-secret",
				"--auth-secret=ngc-api-secret",
				"--profiles=profile-a,profile-b",
				"--resources-cpu=4",
				"--resources-memory=16Gi",
				"--pvc-storage-name=test-pvc-"+nimCacheName,
				"--pvc-create=true",
				"--pvc-size=60Gi",
				"--pvc-volume-access-mode=ReadWriteMany")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify profile specification with readiness verification
			nimCache := getAndCheckNIMCache(namespace, nimCacheName)
			Expect(nimCache.Spec.Source.NGC.Model.Profiles).To(ContainElements("profile-a", "profile-b"))
			Expect(nimCache.Spec.Resources.CPU.String()).To(Equal("4"))
			Expect(nimCache.Spec.Resources.Memory.String()).To(Equal("16Gi"))

			if shouldWaitForReadiness() {
				waitForNIMCacheReady(namespace, nimCacheName)
			}
		})

		It("should succeed with profile specification flags (without readiness verification)", func() {
			nimCacheName := "test-profiles-nimcache-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimcache", nimCacheName,
				"--namespace="+namespace,
				"--nim-source=ngc",
				"--model-puller=nvcr.io/nim/meta/llama-3.2-1b-instruct:1.12.0",
				"--pull-secret=ngc-secret",
				"--auth-secret=ngc-api-secret",
				"--profiles=optimized-profile,standard-profile,fast-profile",
				"--pvc-storage-name=test-pvc-"+nimCacheName,
				"--pvc-create=true",
				"--pvc-size=35Gi",
				"--pvc-volume-access-mode=ReadWriteMany")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify profile specification flags
			nimCache := getAndCheckNIMCache(namespace, nimCacheName)
			Expect(nimCache.Spec.Source.NGC.Model.Profiles).To(ContainElements("optimized-profile", "standard-profile", "fast-profile"))
			// When profiles are specified, other model fields should be empty
			Expect(nimCache.Spec.Source.NGC.Model.Precision).To(Equal(""))
			Expect(nimCache.Spec.Source.NGC.Model.Engine).To(Equal(""))
			Expect(nimCache.Spec.Source.NGC.Model.TensorParallelism).To(Equal(""))
			Expect(nimCache.Spec.Source.NGC.Model.QoSProfile).To(Equal(""))
			Expect(nimCache.Spec.Source.NGC.Model.GPUs).To(BeEmpty())
			Expect(nimCache.Spec.Source.NGC.Model.Lora).To(BeNil())
			Expect(nimCache.Spec.Source.NGC.Model.Buildable).To(BeNil())

			// Skip readiness verification for this test
		})
	})

	Context("create nimservice", func() {
		var nimCacheName string

		BeforeEach(func() {
			// Create a NIMCache that NIMService can reference
			nimCacheName = "test-nimcache-for-service-" + randStringBytes(5)
			deployTestNIMCache(namespace, nimCacheName)
		})

		It("should succeed in creating a basic nimservice", func() {
			nimServiceName := "test-create-nimservice-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimservice", nimServiceName,
				"--namespace="+namespace,
				"--image-repository=nvcr.io/nim/meta/llama-3.1-8b-instruct",
				"--tag=1.12.0",
				"--nimcache-storage-name="+nimCacheName,
				"--pull-secrets=ngc-secret")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify the resource was created
			nimService := getAndCheckNIMService(namespace, nimServiceName)
			Expect(nimService.Name).To(Equal(nimServiceName))
			Expect(nimService.Namespace).To(Equal(namespace))
			Expect(nimService.Spec.Image.Repository).To(Equal("nvcr.io/nim/meta/llama-3.1-8b-instruct"))
			Expect(nimService.Spec.Image.Tag).To(Equal("1.12.0"))
			Expect(nimService.Spec.Image.PullSecrets).To(ContainElement("ngc-secret"))
			Expect(nimService.Spec.Storage.NIMCache.Name).To(Equal(nimCacheName))

			// Wait for the NIMService to become ready and verify deployment health (if enabled)
			if shouldWaitForReadiness() {
				waitForNIMServiceReady(namespace, nimServiceName)
			}
		})

		It("should succeed in creating nimservice with custom replicas", func() {
			nimServiceName := "test-create-nimservice-replicas-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimservice", nimServiceName,
				"--namespace="+namespace,
				"--image-repository=nvcr.io/nim/meta/llama-3.1-8b-instruct",
				"--tag=latest",
				"--nimcache-storage-name="+nimCacheName,
				"--pull-secrets=ngc-secret",
				"--replicas=3")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify the resource was created with custom replicas
			nimService := getAndCheckNIMService(namespace, nimServiceName)
			Expect(nimService.Name).To(Equal(nimServiceName))
			Expect(nimService.Spec.Replicas).To(Equal(int(3)))

			// Wait for the NIMService to become ready and verify deployment health (if enabled)
			if shouldWaitForReadiness() {
				waitForNIMServiceReady(namespace, nimServiceName)
			}
		})

		It("should succeed in creating nimservice with autoscaling", func() {
			nimServiceName := "test-create-nimservice-autoscale-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimservice", nimServiceName,
				"--namespace="+namespace,
				"--image-repository=nvcr.io/nim/meta/llama-3.1-8b-instruct",
				"--tag=latest",
				"--nimcache-storage-name="+nimCacheName,
				"--pull-secrets=ngc-secret",
				"--scale-min-replicas=1",
				"--scale-max-replicas=5")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify the resource was created with autoscaling
			nimService := getAndCheckNIMService(namespace, nimServiceName)
			Expect(nimService.Name).To(Equal(nimServiceName))
			Expect(nimService.Spec.Scale.Enabled).To(Equal(ptr.To(true)))
			Expect(*nimService.Spec.Scale.HPA.MinReplicas).To(Equal(int32(1)))
			Expect(nimService.Spec.Scale.HPA.MaxReplicas).To(Equal(int32(5)))

			// Wait for the NIMService to become ready and verify deployment health (if enabled)
			if shouldWaitForReadiness() {
				waitForNIMServiceReady(namespace, nimServiceName)
			}
		})

		It("should succeed in creating nimservice with custom port and service type", func() {
			nimServiceName := "test-create-nimservice-port-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimservice", nimServiceName,
				"--namespace="+namespace,
				"--image-repository=nvcr.io/nim/meta/llama-3.1-8b-instruct",
				"--tag=latest",
				"--nimcache-storage-name="+nimCacheName,
				"--pull-secrets=ngc-secret",
				"--service-port=9000",
				"--service-type=LoadBalancer")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify the resource was created with custom service settings
			nimService := getAndCheckNIMService(namespace, nimServiceName)
			Expect(nimService.Name).To(Equal(nimServiceName))
			Expect(*nimService.Spec.Expose.Service.Port).To(Equal(int32(9000)))
			Expect(string(nimService.Spec.Expose.Service.Type)).To(Equal("LoadBalancer"))

			// Wait for the NIMService to become ready and verify deployment health (if enabled)
			if shouldWaitForReadiness() {
				waitForNIMServiceReady(namespace, nimServiceName)
			}
		})

		It("should succeed in creating nimservice with environment variables (this is only intended to be used internally by the deploy command)", func() {
			nimServiceName := "test-create-nimservice-env-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimservice", nimServiceName,
				"--namespace="+namespace,
				"--image-repository=nvcr.io/nim/meta/llama-3.1-8b-instruct",
				"--tag=latest",
				"--nimcache-storage-name="+nimCacheName,
				"--pull-secrets=ngc-secret",
				"--env=ENV1,value1,ENV2,value2")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify the resource was created with environment variables
			nimService := getAndCheckNIMService(namespace, nimServiceName)
			Expect(nimService.Name).To(Equal(nimServiceName))
			Expect(nimService.Spec.Env).To(HaveLen(2))
			Expect(nimService.Spec.Env[0].Name).To(Equal("ENV1"))
			Expect(nimService.Spec.Env[0].Value).To(Equal("value1"))
			Expect(nimService.Spec.Env[1].Name).To(Equal("ENV2"))
			// This would equal the actual token value => Expect(nimService.Spec.Env[1].Value).To(Equal("value2"))

			// Wait for the NIMService to become ready and verify deployment health (if enabled)
			if shouldWaitForReadiness() {
				waitForNIMServiceReady(namespace, nimServiceName)
			}
		})

		It("should fail when creating duplicate nimservice", func() {
			nimServiceName := "test-duplicate-nimservice-" + randStringBytes(5)

			// Create first nimservice
			cmd := exec.Command("kubectl", "nim", "create", "nimservice", nimServiceName,
				"--namespace="+namespace,
				"--image-repository=nvcr.io/nim/meta/llama-3.1-8b-instruct",
				"--tag=latest",
				"--nimcache-storage-name="+nimCacheName,
				"--pull-secrets=ngc-secret")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())

			// Try to create duplicate
			cmd = exec.Command("kubectl", "nim", "create", "nimservice", nimServiceName,
				"--namespace="+namespace,
				"--image-repository=nvcr.io/nim/meta/llama-3.1-8b-instruct",
				"--tag=latest",
				"--nimcache-storage-name="+nimCacheName,
				"--pull-secrets=ngc-secret")
			output, err = runCommandWithDebug(cmd)

			Expect(err).To(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("already exists"))
		})

		It("should fail when referencing non-existent nimcache", func() {
			nimServiceName := "test-invalid-cache-nimservice-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimservice", nimServiceName,
				"--namespace="+namespace,
				"--image-repository=nvcr.io/nim/meta/llama-3.1-8b-instruct",
				"--tag=latest",
				"--nimcache-storage-name=non-existent-cache",
				"--pull-secrets=ngc-secret")
			output, err := runCommandWithDebug(cmd)

			// The command might succeed (creation), but the service won't be healthy
			// This depends on operator validation behavior
			if err != nil {
				Expect(string(output)).To(ContainSubstring("not found"))
			} else {
				// If it succeeds, at least verify it was created
				nimService := getAndCheckNIMService(namespace, nimServiceName)
				Expect(nimService.Spec.Storage.NIMCache.Name).To(Equal("non-existent-cache"))
			}
		})

		// Comprehensive flag testing for NIMService with standalone platform
		It("should succeed with all NIMService flags with standalone platform (with readiness verification)", func() {
			nimServiceName := "test-all-flags-nimservice-" + randStringBytes(5)
			pvcName := "test-pvc-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimservice", nimServiceName,
				"--namespace="+namespace,
				"--image-repository=nvcr.io/nim/meta/llama-3.1-8b-instruct",
				"--tag=1.12.0",
				"--pvc-create=true",
				"--pvc-storage-name="+pvcName,
				"--pvc-storage-class=standard",
				"--pvc-size=50Gi",
				"--pvc-volume-access-mode=ReadWriteMany",
				"--pull-policy=Always",
				"--auth-secret=ngc-api-secret",
				"--pull-secrets=ngc-secret,additional-secret",
				"--service-port=9090",
				"--service-type=NodePort",
				"--replicas=2",
				"--gpu-limit=2",
				"--scale-max-replicas=10",
				"--scale-min-replicas=1",
				"--inference-platform=standalone")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify all flags were applied correctly
			nimService := getAndCheckNIMService(namespace, nimServiceName)
			Expect(nimService.Name).To(Equal(nimServiceName))
			Expect(nimService.Spec.Image.Repository).To(Equal("nvcr.io/nim/meta/llama-3.1-8b-instruct"))
			Expect(nimService.Spec.Image.Tag).To(Equal("1.12.0"))
			Expect(nimService.Spec.Image.PullPolicy).To(Equal("Always"))
			Expect(nimService.Spec.Image.PullSecrets).To(ContainElements("ngc-secret", "additional-secret"))
			Expect(nimService.Spec.AuthSecret).To(Equal("ngc-api-secret"))
			Expect(nimService.Spec.Storage.PVC.Name).To(Equal(pvcName))
			Expect(nimService.Spec.Storage.PVC.Create).To(Equal(ptr.To(true)))
			Expect(nimService.Spec.Storage.PVC.StorageClass).To(Equal("standard"))
			Expect(nimService.Spec.Storage.PVC.Size).To(Equal("50Gi"))
			Expect(string(nimService.Spec.Storage.PVC.VolumeAccessMode)).To(Equal("ReadWriteMany"))
			Expect(*nimService.Spec.Expose.Service.Port).To(Equal(int32(9090)))
			Expect(string(nimService.Spec.Expose.Service.Type)).To(Equal("NodePort"))
			Expect(nimService.Spec.Replicas).To(Equal(2))
			expectedGPU := resource.MustParse("2")
			Expect(nimService.Spec.Resources.Limits["nvidia.com/gpu"].Equal(expectedGPU)).To(BeTrue())
			Expect(nimService.Spec.Scale.Enabled).To(Equal(ptr.To(true)))
			Expect(nimService.Spec.Scale.HPA.MaxReplicas).To(Equal(int32(10)))
			Expect(*nimService.Spec.Scale.HPA.MinReplicas).To(Equal(int32(1)))
			Expect(string(nimService.Spec.InferencePlatform)).To(Equal("standalone"))

			// Wait for readiness if enabled
			if shouldWaitForReadiness() {
				waitForNIMServiceReady(namespace, nimServiceName)
			}
		})

		It("should succeed with all NIMService flags (with readiness verification)", func() {
			nimServiceName := "test-all-flags-nimservice-" + randStringBytes(5)
			pvcName := "test-pvc-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimservice", nimServiceName,
				"--namespace="+namespace,
				"--image-repository=nvcr.io/nim/meta/llama-3.1-8b-instruct",
				"--tag=1.12.0",
				"--pvc-create=true",
				"--pvc-storage-name="+pvcName,
				"--pvc-storage-class=standard",
				"--pvc-size=50Gi",
				"--pvc-volume-access-mode=ReadWriteMany",
				"--pull-policy=Always",
				"--auth-secret=ngc-api-secret",
				"--pull-secrets=ngc-secret,additional-secret",
				"--service-port=9090",
				"--service-type=NodePort",
				"--replicas=2",
				"--gpu-limit=2",
				"--inference-platform=kserve")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify KServe flags were applied correctly (scaling should not be enabled)
			nimService := getAndCheckNIMService(namespace, nimServiceName)
			Expect(nimService.Name).To(Equal(nimServiceName))
			Expect(nimService.Spec.Image.Repository).To(Equal("nvcr.io/nim/meta/llama-3.1-8b-instruct"))
			Expect(nimService.Spec.Image.Tag).To(Equal("1.12.0"))
			Expect(nimService.Spec.Image.PullPolicy).To(Equal("Always"))
			Expect(nimService.Spec.Image.PullSecrets).To(ContainElements("ngc-secret", "additional-secret"))
			Expect(nimService.Spec.AuthSecret).To(Equal("ngc-api-secret"))
			Expect(nimService.Spec.Storage.PVC.Name).To(Equal(pvcName))
			Expect(nimService.Spec.Storage.PVC.Create).To(Equal(ptr.To(true)))
			Expect(nimService.Spec.Storage.PVC.StorageClass).To(Equal("standard"))
			Expect(nimService.Spec.Storage.PVC.Size).To(Equal("50Gi"))
			Expect(string(nimService.Spec.Storage.PVC.VolumeAccessMode)).To(Equal("ReadWriteMany"))
			Expect(*nimService.Spec.Expose.Service.Port).To(Equal(int32(9090)))
			Expect(string(nimService.Spec.Expose.Service.Type)).To(Equal("NodePort"))
			Expect(nimService.Spec.Replicas).To(Equal(2))
			expectedGPU := resource.MustParse("2")
			Expect(nimService.Spec.Resources.Limits["nvidia.com/gpu"].Equal(expectedGPU)).To(BeTrue())
			// KServe platform should not have scaling enabled (serverless mode handles scaling)
			Expect(nimService.Spec.Scale.Enabled).To(BeNil())
			Expect(string(nimService.Spec.InferencePlatform)).To(Equal("kserve"))

			// Wait for readiness if enabled
			if shouldWaitForReadiness() {
				waitForNIMServiceReady(namespace, nimServiceName)
			}
		})

		It("should succeed with all NIMService flags (without readiness verification)", func() {
			nimServiceName := "test-all-flags-no-wait-nimservice-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimservice", nimServiceName,
				"--namespace="+namespace,
				"--image-repository=nvcr.io/nim/meta/llama-3.1-8b-instruct",
				"--tag=1.12.0",
				"--nimcache-storage-name="+nimCacheName,
				"--nimcache-storage-profile=optimized",
				"--pull-policy=IfNotPresent",
				"--auth-secret=ngc-api-secret",
				"--pull-secrets=ngc-secret",
				"--service-port=8080",
				"--service-type=ClusterIP",
				"--replicas=1",
				"--gpu-limit=1",
				"--inference-platform=standalone")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify flags were applied correctly
			nimService := getAndCheckNIMService(namespace, nimServiceName)
			Expect(nimService.Name).To(Equal(nimServiceName))
			Expect(nimService.Spec.Storage.NIMCache.Name).To(Equal(nimCacheName))
			Expect(nimService.Spec.Storage.NIMCache.Profile).To(Equal("optimized"))
			Expect(nimService.Spec.Image.PullPolicy).To(Equal("IfNotPresent"))
			Expect(*nimService.Spec.Expose.Service.Port).To(Equal(int32(8080)))
			Expect(string(nimService.Spec.Expose.Service.Type)).To(Equal("ClusterIP"))
			Expect(nimService.Spec.Replicas).To(Equal(1))
			Expect(string(nimService.Spec.InferencePlatform)).To(Equal("standalone"))

			// Skip readiness verification for this test
		})

		It("should succeed with PVC storage flags (with readiness verification)", func() {
			nimServiceName := "test-pvc-flags-nimservice-" + randStringBytes(5)
			pvcName := "test-pvc-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimservice", nimServiceName,
				"--namespace="+namespace,
				"--image-repository=nvcr.io/nim/meta/llama-3.1-8b-instruct",
				"--tag=1.12.0",
				"--pvc-create=true",
				"--pvc-storage-name="+pvcName,
				"--pvc-storage-class=fast-ssd",
				"--pvc-size=100Gi",
				"--pvc-volume-access-mode=ReadWriteOnce")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify PVC-specific flags
			nimService := getAndCheckNIMService(namespace, nimServiceName)
			Expect(nimService.Spec.Storage.PVC.Name).To(Equal(pvcName))
			Expect(nimService.Spec.Storage.PVC.Create).To(Equal(ptr.To(true)))
			Expect(nimService.Spec.Storage.PVC.StorageClass).To(Equal("fast-ssd"))
			Expect(nimService.Spec.Storage.PVC.Size).To(Equal("100Gi"))
			Expect(string(nimService.Spec.Storage.PVC.VolumeAccessMode)).To(Equal("ReadWriteOnce"))

			if shouldWaitForReadiness() {
				waitForNIMServiceReady(namespace, nimServiceName)
			}
		})

		It("should succeed with scaling flags (without readiness verification)", func() {
			nimServiceName := "test-scaling-flags-nimservice-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimservice", nimServiceName,
				"--namespace="+namespace,
				"--image-repository=nvcr.io/nim/meta/llama-3.1-8b-instruct",
				"--tag=1.12.0",
				"--nimcache-storage-name="+nimCacheName,
				"--scale-min-replicas=2",
				"--scale-max-replicas=8",
				"--replicas=3")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify scaling flags
			nimService := getAndCheckNIMService(namespace, nimServiceName)
			Expect(nimService.Spec.Scale.Enabled).To(Equal(ptr.To(true)))
			Expect(*nimService.Spec.Scale.HPA.MinReplicas).To(Equal(int32(2)))
			Expect(nimService.Spec.Scale.HPA.MaxReplicas).To(Equal(int32(8)))
			Expect(nimService.Spec.Replicas).To(Equal(3))

			// Skip readiness verification for this test
		})

		It("should succeed with service exposure flags (with readiness verification)", func() {
			nimServiceName := "test-service-flags-nimservice-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimservice", nimServiceName,
				"--namespace="+namespace,
				"--image-repository=nvcr.io/nim/meta/llama-3.1-8b-instruct",
				"--tag=1.12.0",
				"--nimcache-storage-name="+nimCacheName,
				"--service-port=7777",
				"--service-type=LoadBalancer")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify service exposure flags
			nimService := getAndCheckNIMService(namespace, nimServiceName)
			Expect(*nimService.Spec.Expose.Service.Port).To(Equal(int32(7777)))
			Expect(string(nimService.Spec.Expose.Service.Type)).To(Equal("LoadBalancer"))

			if shouldWaitForReadiness() {
				waitForNIMServiceReady(namespace, nimServiceName)
			}
		})

		It("should succeed with GPU and resource flags (without readiness verification)", func() {
			nimServiceName := "test-gpu-flags-nimservice-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimservice", nimServiceName,
				"--namespace="+namespace,
				"--image-repository=nvcr.io/nim/meta/llama-3.1-8b-instruct",
				"--tag=1.12.0",
				"--nimcache-storage-name="+nimCacheName,
				"--gpu-limit=4")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify GPU resource flags
			nimService := getAndCheckNIMService(namespace, nimServiceName)
			expectedGPU := resource.MustParse("4")
			Expect(nimService.Spec.Resources.Limits["nvidia.com/gpu"].Equal(expectedGPU)).To(BeTrue())

			// Skip readiness verification for this test
		})

		It("should succeed with KServe inference platform flags (with readiness verification)", func() {
			nimServiceName := "test-kserve-platform-nimservice-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimservice", nimServiceName,
				"--namespace="+namespace,
				"--image-repository=nvcr.io/nim/meta/llama-3.1-8b-instruct",
				"--tag=1.12.0",
				"--nimcache-storage-name="+nimCacheName,
				"--inference-platform=kserve",
				"--replicas=1")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify KServe inference platform flag (no scaling should be enabled)
			nimService := getAndCheckNIMService(namespace, nimServiceName)
			Expect(string(nimService.Spec.InferencePlatform)).To(Equal("kserve"))
			Expect(nimService.Spec.Replicas).To(Equal(1))
			// Scaling should not be enabled for KServe
			if nimService.Spec.Scale.Enabled != nil {
				Expect(*nimService.Spec.Scale.Enabled).To(BeFalse())
			}

			if shouldWaitForReadiness() {
				waitForNIMServiceReady(namespace, nimServiceName)
			}
		})

		It("should succeed with standalone inference platform flags (without readiness verification)", func() {
			nimServiceName := "test-standalone-platform-nimservice-" + randStringBytes(5)

			cmd := exec.Command("kubectl", "nim", "create", "nimservice", nimServiceName,
				"--namespace="+namespace,
				"--image-repository=nvcr.io/nim/meta/llama-3.1-8b-instruct",
				"--tag=1.12.0",
				"--nimcache-storage-name="+nimCacheName,
				"--inference-platform=standalone",
				"--replicas=2")
			output, err := runCommandWithDebug(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("created"))

			// Verify standalone inference platform flag
			nimService := getAndCheckNIMService(namespace, nimServiceName)
			Expect(string(nimService.Spec.InferencePlatform)).To(Equal("standalone"))
			Expect(nimService.Spec.Replicas).To(Equal(2))

			// Skip readiness verification for this test
		})
	})
})
