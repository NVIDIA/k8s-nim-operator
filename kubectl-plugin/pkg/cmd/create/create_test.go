package create

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

// --- NIMService tests ---

func Test_FillOutNIMServiceSpec_Valid(t *testing.T) {
	options := &NIMServiceOptions{
		ImageRepository:     "nvcr.io/nim/meta/llama-3.1-8b-instruct",
		Tag:                 "1.3.3",
		PVCCreate:           true,
		PVCStorageName:      "nim-pvc",
		PVCStorageClass:     "standard",
		PVCSize:             "20Gi",
		PVCVolumeAccessMode: string(corev1.ReadWriteMany),
		AuthSecret:          "ngc-api-secret",
		PullPolicy:          string(corev1.PullIfNotPresent),
		PullSecrets:         []string{"pullsecret"},
		ServicePort:         8080,
		ServiceType:         string(corev1.ServiceTypeClusterIP),
		Replicas:            2,
		GPULimit:            "1",
		ScaleMaxReplicas:    -1,
		ScaleMinReplicas:    -1,
		InferencePlatform:   string(appsv1alpha1.PlatformTypeStandalone),
	}

	ns, err := FillOutNIMServiceSpec(options)
	if err != nil {
		t.Fatalf("FillOutNIMServiceSpec error: %v", err)
	}

	// Check image settings
	if ns.Spec.Image.Repository != options.ImageRepository || ns.Spec.Image.Tag != options.Tag {
		t.Fatalf("image not set correctly: %+v", ns.Spec.Image)
	}

	// Check pull policy and secrets
	if ns.Spec.Image.PullPolicy != options.PullPolicy {
		t.Fatalf("pull policy not set correctly: %s", ns.Spec.Image.PullPolicy)
	}
	if len(ns.Spec.Image.PullSecrets) != 1 || ns.Spec.Image.PullSecrets[0] != "pullsecret" {
		t.Fatalf("pull secrets not set correctly: %+v", ns.Spec.Image.PullSecrets)
	}

	// Check PVC storage settings
	if ns.Spec.Storage.PVC.Name != options.PVCStorageName || ns.Spec.Storage.PVC.Size != options.PVCSize {
		t.Fatalf("pvc not set correctly: %+v", ns.Spec.Storage.PVC)
	}
	if ns.Spec.Storage.PVC.Create == nil || !*ns.Spec.Storage.PVC.Create {
		t.Fatalf("pvc create flag not set")
	}
	if ns.Spec.Storage.PVC.StorageClass != options.PVCStorageClass {
		t.Fatalf("pvc storage class not set correctly: %s", ns.Spec.Storage.PVC.StorageClass)
	}
	if ns.Spec.Storage.PVC.VolumeAccessMode != corev1.PersistentVolumeAccessMode(options.PVCVolumeAccessMode) {
		t.Fatalf("volume access mode not set")
	}

	// Check auth secret
	if ns.Spec.AuthSecret != options.AuthSecret {
		t.Fatalf("auth secret not set correctly: %s", ns.Spec.AuthSecret)
	}

	// Check service settings
	if ns.Spec.Expose.Service.Type != corev1.ServiceType(options.ServiceType) || ns.Spec.Expose.Service.Port == nil || *ns.Spec.Expose.Service.Port != options.ServicePort {
		t.Fatalf("service expose not set correctly: %+v", ns.Spec.Expose.Service)
	}

	// Check resources
	if ns.Spec.Resources == nil || !ns.Spec.Resources.Limits[corev1.ResourceName("nvidia.com/gpu")].Equal(resource.MustParse(options.GPULimit)) {
		t.Fatalf("gpu limit not set: %+v", ns.Spec.Resources)
	}

	// Check replicas
	if ns.Spec.Replicas != options.Replicas {
		t.Fatalf("replicas not set correctly: %d", ns.Spec.Replicas)
	}

	// Check scaling (should be disabled since ScaleMaxReplicas is -1)
	if ns.Spec.Scale.Enabled != nil && *ns.Spec.Scale.Enabled {
		t.Fatalf("scaling should not be enabled when ScaleMaxReplicas is -1")
	}

	// Check inference platform
	if ns.Spec.InferencePlatform != appsv1alpha1.PlatformType(options.InferencePlatform) {
		t.Fatalf("inference platform not set")
	}
}

func Test_FillOutNIMServiceSpec_WithScaling(t *testing.T) {
	options := &NIMServiceOptions{
		ImageRepository:     "nvcr.io/nim/meta/llama-3.1-8b-instruct",
		Tag:                 "1.3.3",
		ServiceType:         string(corev1.ServiceTypeClusterIP),
		GPULimit:            "2",
		Replicas:            1,
		ScaleMinReplicas:    2,
		ScaleMaxReplicas:    10,
		InferencePlatform:   string(appsv1alpha1.PlatformTypeKServe),
		PVCVolumeAccessMode: string(corev1.ReadWriteOnce),
	}

	ns, err := FillOutNIMServiceSpec(options)
	if err != nil {
		t.Fatalf("FillOutNIMServiceSpec error: %v", err)
	}

	// Check scaling is enabled
	if ns.Spec.Scale.Enabled == nil || !*ns.Spec.Scale.Enabled {
		t.Fatalf("scaling should be enabled when ScaleMaxReplicas is set")
	}
	if ns.Spec.Scale.HPA.MaxReplicas != options.ScaleMaxReplicas {
		t.Fatalf("max replicas not set correctly: %d", ns.Spec.Scale.HPA.MaxReplicas)
	}
	if ns.Spec.Scale.HPA.MinReplicas == nil || *ns.Spec.Scale.HPA.MinReplicas != options.ScaleMinReplicas {
		t.Fatalf("min replicas not set correctly")
	}

	// Check KServe platform
	if ns.Spec.InferencePlatform != appsv1alpha1.PlatformTypeKServe {
		t.Fatalf("inference platform should be kserve")
	}
}

func Test_FillOutNIMServiceSpec_WithNIMCache(t *testing.T) {
	options := &NIMServiceOptions{
		ImageRepository:        "nvcr.io/nim/meta/llama-3.1-8b-instruct",
		Tag:                    "1.3.3",
		NIMCacheStorageName:    "my-nimcache",
		NIMCacheStorageProfile: "optimized-profile",
		ServiceType:            string(corev1.ServiceTypeNodePort),
		GPULimit:               "1",
		InferencePlatform:      string(appsv1alpha1.PlatformTypeStandalone),
	}

	ns, err := FillOutNIMServiceSpec(options)
	if err != nil {
		t.Fatalf("FillOutNIMServiceSpec error: %v", err)
	}

	// Check NIMCache storage settings
	if ns.Spec.Storage.NIMCache.Name != options.NIMCacheStorageName {
		t.Fatalf("nimcache name not set correctly: %s", ns.Spec.Storage.NIMCache.Name)
	}
	if ns.Spec.Storage.NIMCache.Profile != options.NIMCacheStorageProfile {
		t.Fatalf("nimcache profile not set correctly: %s", ns.Spec.Storage.NIMCache.Profile)
	}

	// Verify PVC settings are not set when using NIMCache
	if ns.Spec.Storage.PVC.Name != "" {
		t.Fatalf("PVC name should be empty when using NIMCache")
	}
}

func Test_FillOutNIMServiceSpec_HostPath(t *testing.T) {
	options := &NIMServiceOptions{
		ImageRepository:   "nvcr.io/nim/meta/llama-3.1-8b-instruct",
		Tag:               "1.3.3",
		HostPath:          "/mnt/nim-cache",
		ServiceType:       string(corev1.ServiceTypeClusterIP),
		GPULimit:          "1",
		InferencePlatform: string(appsv1alpha1.PlatformTypeStandalone),
	}

	ns, err := FillOutNIMServiceSpec(options)
	if err != nil {
		t.Fatalf("FillOutNIMServiceSpec error: %v", err)
	}

	if ns.Spec.Storage.HostPath == nil || *ns.Spec.Storage.HostPath != options.HostPath {
		t.Fatalf("hostPath not set correctly: %+v", ns.Spec.Storage.HostPath)
	}
}

func Test_FillOutNIMServiceSpec_InvalidPVCMode(t *testing.T) {
	options := &NIMServiceOptions{
		ImageRepository:     "repo",
		Tag:                 "v1",
		PVCVolumeAccessMode: "InvalidMode",
		ServiceType:         string(corev1.ServiceTypeClusterIP),
		GPULimit:            "1",
	}
	_, err := FillOutNIMServiceSpec(options)
	if err == nil {
		t.Fatalf("expected error for invalid pvc-volume-access-mode")
	}
}

func Test_FillOutNIMServiceSpec_InvalidServiceType(t *testing.T) {
	options := &NIMServiceOptions{
		ImageRepository:     "repo",
		Tag:                 "v1",
		PVCVolumeAccessMode: string(corev1.ReadWriteOnce),
		ServiceType:         "BogusType",
		GPULimit:            "1",
	}
	_, err := FillOutNIMServiceSpec(options)
	if err == nil {
		t.Fatalf("expected error for invalid service-type")
	}
}

func Test_FillOutNIMServiceSpec_SimpleEnvVars(t *testing.T) {
	options := &NIMServiceOptions{
		ImageRepository:     "repo",
		Tag:                 "v1",
		ServiceType:         string(corev1.ServiceTypeClusterIP),
		GPULimit:            "1",
		InferencePlatform:   string(appsv1alpha1.PlatformTypeStandalone),
		Env:                 []string{"ENV_NAME", "env_value"},
		PVCVolumeAccessMode: string(corev1.ReadWriteOnce),
	}

	ns, err := FillOutNIMServiceSpec(options)
	if err != nil {
		t.Fatalf("FillOutNIMServiceSpec error: %v", err)
	}

	if len(ns.Spec.Env) != 1 {
		t.Fatalf("expected 1 env var, got %d", len(ns.Spec.Env))
	}
	if ns.Spec.Env[0].Name != "ENV_NAME" || ns.Spec.Env[0].Value != "env_value" {
		t.Fatalf("env var not set correctly: %+v", ns.Spec.Env[0])
	}
}

func Test_FillOutNIMServiceSpec_HFTokenEnvVars(t *testing.T) {
	options := &NIMServiceOptions{
		ImageRepository:     "repo",
		Tag:                 "v1",
		ServiceType:         string(corev1.ServiceTypeClusterIP),
		GPULimit:            "1",
		InferencePlatform:   string(appsv1alpha1.PlatformTypeStandalone),
		Env:                 []string{"NIM_MODEL_NAME", "hf://meta-llama/Llama-2-7b-hf", "HF_TOKEN", "hf-api-secret"},
		PVCVolumeAccessMode: string(corev1.ReadWriteOnce),
	}

	ns, err := FillOutNIMServiceSpec(options)
	if err != nil {
		t.Fatalf("FillOutNIMServiceSpec error: %v", err)
	}

	if len(ns.Spec.Env) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(ns.Spec.Env))
	}

	// Check first env var
	if ns.Spec.Env[0].Name != "NIM_MODEL_NAME" || ns.Spec.Env[0].Value != "hf://meta-llama/Llama-2-7b-hf" {
		t.Fatalf("first env var not set correctly: %+v", ns.Spec.Env[0])
	}

	// Check second env var with SecretKeyRef
	if ns.Spec.Env[1].Name != "HF_TOKEN" {
		t.Fatalf("second env var name not correct: %s", ns.Spec.Env[1].Name)
	}
	if ns.Spec.Env[1].ValueFrom == nil || ns.Spec.Env[1].ValueFrom.SecretKeyRef == nil {
		t.Fatalf("second env var ValueFrom not set correctly")
	}
	if ns.Spec.Env[1].ValueFrom.SecretKeyRef.Name != "hf-api-secret" {
		t.Fatalf("secret name not correct: %s", ns.Spec.Env[1].ValueFrom.SecretKeyRef.Name)
	}
	if ns.Spec.Env[1].ValueFrom.SecretKeyRef.Key != "HF_TOKEN" {
		t.Fatalf("secret key not correct: %s", ns.Spec.Env[1].ValueFrom.SecretKeyRef.Key)
	}
	if ns.Spec.Env[1].ValueFrom.SecretKeyRef.Optional == nil || !*ns.Spec.Env[1].ValueFrom.SecretKeyRef.Optional {
		t.Fatalf("secret optional field not set to true")
	}
}

func Test_FillOutNIMServiceSpec_InvalidEnvVars(t *testing.T) {
	options := &NIMServiceOptions{
		ImageRepository:   "repo",
		Tag:               "v1",
		ServiceType:       string(corev1.ServiceTypeClusterIP),
		GPULimit:          "1",
		InferencePlatform: string(appsv1alpha1.PlatformTypeStandalone),
		Env:               []string{"ENV1", "val1", "ENV2"}, // Invalid: 3 elements
	}

	_, err := FillOutNIMServiceSpec(options)
	if err == nil {
		t.Fatalf("expected error for invalid env var count")
	}
}

func Test_FillOutNIMServiceSpec_InvalidInferencePlatform(t *testing.T) {
	options := &NIMServiceOptions{
		ImageRepository:   "repo",
		Tag:               "v1",
		ServiceType:       string(corev1.ServiceTypeClusterIP),
		GPULimit:          "1",
		InferencePlatform: "invalid-platform",
	}

	_, err := FillOutNIMServiceSpec(options)
	if err == nil {
		t.Fatalf("expected error for invalid inference platform")
	}
}

// --- NIMCache tests ---

func Test_ValidateNIMCacheOptions(t *testing.T) {
	good := &NIMCacheOptions{SourceConfiguration: "ngc"}
	if err := Validate(good); err != nil {
		t.Fatalf("Validate() good: %v", err)
	}
	bad := &NIMCacheOptions{SourceConfiguration: "something-else"}
	if err := Validate(bad); err == nil {
		t.Fatalf("Validate() expected error for invalid source")
	}
}

func Test_FillOutNIMCacheSpec_NGC(t *testing.T) {
	options := &NIMCacheOptions{
		SourceConfiguration: "ngc",
		AuthSecret:          "ngc-api-secret",
		ModelPuller:         "nvcr.io/nim/puller:latest",
		PullSecret:          "pull-secret",
		Profiles:            []string{"p1", "p2"},
		Precision:           "fp8",
		Engine:              "tensorrt_llm",
		TensorParallelism:   "2",
		QosProfile:          "throughput",
		Lora:                "true",
		Buildable:           "false",
		GPUs:                []string{"h100", "a100"},
		ResourcesCPU:        "2",
		ResourcesMemory:     "4Gi",
	}

	nc, err := FillOutNIMCacheSpec(options)
	if err != nil {
		t.Fatalf("FillOutNIMCacheSpec error: %v", err)
	}
	if nc.Spec.Source.NGC == nil {
		t.Fatalf("NGC source not set")
	}
	if nc.Spec.Source.NGC.Model == nil || nc.Spec.Source.NGC.Model.Precision != "fp8" || nc.Spec.Source.NGC.Model.Engine != "tensorrt_llm" {
		t.Fatalf("model fields not set: %+v", nc.Spec.Source.NGC.Model)
	}
	if len(nc.Spec.Source.NGC.Model.GPUs) != 2 || nc.Spec.Source.NGC.Model.GPUs[0].Product == "" {
		t.Fatalf("gpus not set: %+v", nc.Spec.Source.NGC.Model.GPUs)
	}
	if !nc.Spec.Resources.CPU.Equal(resource.MustParse("2")) || !nc.Spec.Resources.Memory.Equal(resource.MustParse("4Gi")) {
		t.Fatalf("resources not set: %+v", nc.Spec.Resources)
	}
}

func Test_FillOutNIMCacheSpec_NGC_WithModelEndpoint(t *testing.T) {
	options := &NIMCacheOptions{
		SourceConfiguration: "ngc",
		AuthSecret:          "ngc-api-secret",
		ModelPuller:         "nvcr.io/nim/puller:latest",
		PullSecret:          "pull-secret",
		ModelEndpoint:       "https://api.endpoint.com/v1/models",
	}

	nc, err := FillOutNIMCacheSpec(options)
	if err != nil {
		t.Fatalf("FillOutNIMCacheSpec error: %v", err)
	}
	if nc.Spec.Source.NGC == nil {
		t.Fatalf("NGC source not set")
	}
	if nc.Spec.Source.NGC.ModelEndpoint == nil || *nc.Spec.Source.NGC.ModelEndpoint != options.ModelEndpoint {
		t.Fatalf("model endpoint not set correctly")
	}
	// When ModelEndpoint is set, Model fields should not be set
	if nc.Spec.Source.NGC.Model != nil {
		t.Fatalf("Model fields should not be set when ModelEndpoint is provided")
	}
}

func Test_FillOutNIMCacheSpec_HF(t *testing.T) {
	options := &NIMCacheOptions{
		SourceConfiguration: "huggingface",
		AltEndpoint:         "https://hf.example",
		AltNamespace:        "main",
		ModelName:           "m1",
		AuthSecret:          "hf-secret",
		ModelPuller:         "hf-puller:latest",
		PullSecret:          "hf-pull",
		Revision:            "r1",
	}
	nc, err := FillOutNIMCacheSpec(options)
	if err != nil {
		t.Fatalf("FillOutNIMCacheSpec HF error: %v", err)
	}
	if nc.Spec.Source.HF == nil || nc.Spec.Source.HF.Endpoint == "" || nc.Spec.Source.HF.Namespace == "" {
		t.Fatalf("HF fields not set: %+v", nc.Spec.Source.HF)
	}
}

func Test_FillOutNIMCacheSpec_DataStore(t *testing.T) {
	options := &NIMCacheOptions{
		SourceConfiguration: "nemodatastore",
		AltEndpoint:         "https://ds.example/v1/hf",
		AltNamespace:        "default",
		DatasetName:         "dset",
		AuthSecret:          "ds-secret",
		ModelPuller:         "ds-puller:latest",
		PullSecret:          "ds-pull",
		Revision:            "r2",
	}
	nc, err := FillOutNIMCacheSpec(options)
	if err != nil {
		t.Fatalf("FillOutNIMCacheSpec DataStore error: %v", err)
	}
	if nc.Spec.Source.DataStore == nil || nc.Spec.Source.DataStore.Endpoint == "" || nc.Spec.Source.DataStore.Namespace == "" {
		t.Fatalf("DataStore fields not set: %+v", nc.Spec.Source.DataStore)
	}
}

func Test_FillOutNIMCacheSpec_InvalidBools(t *testing.T) {
	options := &NIMCacheOptions{
		SourceConfiguration: "ngc",
		ModelPuller:         "img",
		AuthSecret:          "s",
		Lora:                "not-bool",
	}
	_, err := FillOutNIMCacheSpec(options)
	if err == nil {
		t.Fatalf("expected error for invalid --lora bool")
	}
	options.Lora = "true"
	options.Buildable = "nope"
	_, err = FillOutNIMCacheSpec(options)
	if err == nil {
		t.Fatalf("expected error for invalid --buildable bool")
	}
}

func Test_FillOutNIMCacheSpec_InvalidResources(t *testing.T) {
	tests := []struct {
		name    string
		options *NIMCacheOptions
		wantErr bool
	}{
		{
			name: "invalid CPU resource",
			options: &NIMCacheOptions{
				SourceConfiguration: "ngc",
				ModelPuller:         "img",
				AuthSecret:          "s",
				ResourcesCPU:        "invalid-cpu",
			},
			wantErr: true,
		},
		{
			name: "invalid memory resource",
			options: &NIMCacheOptions{
				SourceConfiguration: "ngc",
				ModelPuller:         "img",
				AuthSecret:          "s",
				ResourcesMemory:     "invalid-memory",
			},
			wantErr: true,
		},
		{
			name: "valid resources",
			options: &NIMCacheOptions{
				SourceConfiguration: "ngc",
				ModelPuller:         "img",
				AuthSecret:          "s",
				ResourcesCPU:        "500m",
				ResourcesMemory:     "2Gi",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := FillOutNIMCacheSpec(tt.options)
			if (err != nil) != tt.wantErr {
				t.Errorf("FillOutNIMCacheSpec() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_FillOutNIMCacheSpec_InvalidTensorParallelism(t *testing.T) {
	options := &NIMCacheOptions{
		SourceConfiguration: "ngc",
		ModelPuller:         "img",
		AuthSecret:          "s",
		TensorParallelism:   "not-a-number",
	}

	nc, err := FillOutNIMCacheSpec(options)
	if err != nil {
		t.Fatalf("expected no error for invalid tensor parallelism, got: %v", err)
	}

	// Should store as string even if not a valid number
	if nc.Spec.Source.NGC.Model.TensorParallelism != "not-a-number" {
		t.Fatalf("expected tensor parallelism to be stored as-is")
	}
}
