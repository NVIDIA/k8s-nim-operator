package create

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"context"
	util "k8s-nim-operator-cli/pkg/util"
	"k8s-nim-operator-cli/pkg/util/client"

	"strconv"

	"k8s.io/utils/ptr"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

type NIMCacheOptions struct {
	cmdFactory    cmdutil.Factory
	IoStreams     *genericclioptions.IOStreams
	Namespace     string
	ResourceName  string
	ResourceType  util.ResourceType
	AllNamespaces bool
	// All PVC flags are reused.
	PVCCreate           bool
	PVCStorageName      string
	PVCStorageClass     string
	PVCSize             string
	PVCVolumeAccessMode string
	// All PVC flags are reused.
	PullSecret string
	AuthSecret string
	AltSecret  string

	SourceConfiguration string
	ResourcesCPU        string
	ResourcesMemory     string
	ModelPuller         string
	ModelEndpoint       string
	Precision           string
	Engine              string
	TensorParallelism   string
	QosProfile          string
	Lora                string
	Buildable           string
	Profiles            []string
	GPUs                []string
	AltEndpoint         string
	AltNamespace        string
	ModelName           string
	DatasetName         string
	Revision            string
}

func NewNIMCacheOptions(cmdFactory cmdutil.Factory, streams genericclioptions.IOStreams) *NIMCacheOptions {
	return &NIMCacheOptions{
		cmdFactory: cmdFactory,
		IoStreams:  &streams,
	}
}

// Populates NIMServiceOptions with namespace and resource name (if present).
func (options *NIMCacheOptions) CompleteNamespace(args []string, cmd *cobra.Command) error {
	namespace, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return fmt.Errorf("failed to get namespace: %w", err)
	}
	options.Namespace = namespace
	if options.Namespace == "" {
		options.Namespace = "default"
	}

	options.ResourceName = args[0]

	return nil
}

func NewCreateNIMCacheCommand(cmdFactory cmdutil.Factory, streams genericclioptions.IOStreams) *cobra.Command {
	options := NewNIMCacheOptions(cmdFactory, streams)

	cmd := &cobra.Command{
		Use:   "nimcache [NAME]",
		Short: "Create new NIMCache with specified information",
		Long: `Create new NIMCache with specified parameters.
Must specify --nim-source and storage: reference an existing/create new PVC.`,
		SilenceUsage: true,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				cmd.HelpFunc()(cmd, args)
				return nil
			} else {
				if err := Validate(options); err != nil {
					return err
				}
				if err := options.CompleteNamespace(args, cmd); err != nil {
					return err
				}
				// running cmd.Execute or cmd.ExecuteE sets the context, which will be done by root.
				k8sClient, err := client.NewClient(cmdFactory)
				if err != nil {
					return fmt.Errorf("failed to create client: %w", err)
				}
				options.ResourceType = util.NIMCache
				return RunCreateNIMCache(cmd.Context(), options, k8sClient)
			}
		},
	}

	cmd.Example = strings.Join([]string{
		"  kl nim create nimcache my-nimcache --nim-source=ngc --model-puller=nvcr.io/nim/meta/llama-3.1-8b-instruct:1.3.3 --pull-secret=ngc-secret --auth-secret=ngc-api-secret --engine=tensorrt_llm --tensorParallelism=1 --pvc-storage-name=nim-pvc",
		"",
		"  kl nim create nimcache my-nimcache  --alt-endpoint=<hf-endpoint> --alt-namespace=main --alt-secret=<hf-secret> model-puller=<model-puller> --pull-secret=<hf-pullsecret> --pvc-create=true --pvc-size=20Gi --pvc-volume-access-mode=ReadWriteMany --pvc-storage-class=<storage-class-name>",
	}, "\n")

	// The first argument will be name. Other arguments will be specified as flags.
	cmd.Flags().StringVar(&options.SourceConfiguration, "nim-source", util.SourceConfiguration, "The NIM model source to cache. Must be one of 'ngc', 'huggingface', 'nemodatastore'.")
	cmd.Flags().StringVar(&options.ResourcesCPU, "resources-cpu", util.ResourcesCPU, "Minimum CPU resources required for caching job to run.")
	cmd.Flags().StringVar(&options.ResourcesMemory, "resources-memory", util.ResourcesMemory, "Minimum memory resources required for caching job to run.")
	cmd.Flags().StringVar(&options.ModelPuller, "model-puller", util.ModelPuller, "Container image that can pull the model. If nim-source=huggingface, is the containerized huggingface-cli image to pull the data.")
	cmd.Flags().StringVar(&options.ModelEndpoint, "ngc-model-endpoint", util.ModelEndpoint, "Endpoint for the model to be cached for Universal NIM. Used when nim-source=ngc.")
	cmd.Flags().StringVar(&options.Precision, "precision", util.Precision, "Precision for model quantization.")
	cmd.Flags().StringVar(&options.Engine, "engine", util.Engine, "Backend engine (tensorrt_llm, vllm).")
	cmd.Flags().StringVar(&options.TensorParallelism, "tensor-parallelism", util.TensorParallelism, "Minimum GPUs required for the model computations.")
	cmd.Flags().StringVar(&options.QosProfile, "qos-profile", util.QosProfile, "Supported QoS profile types for the models. Can be either 'throughput' or 'latency.'")
	cmd.Flags().StringVar(&options.Lora, "lora", util.Lora, "Indicates a finetuned model with LoRa adapters. Can be either 'true' or 'false'.")
	cmd.Flags().StringVar(&options.Buildable, "buildable", util.Buildable, "Indicates a finetuned model with LoRa adapters. Can be either 'true' or 'false'.")
	cmd.Flags().StringVar(&options.PullSecret, "pull-secret", util.PullSecret, "Image pull secret.")
	cmd.Flags().StringSliceVar(&options.Profiles, "profiles", util.Profiles, "Comma-separated list of the specific model profiles to Cache. When provided, rest of model parameters for profile selection (precision, engine, etc.) are ignored.")
	cmd.Flags().StringSliceVar(&options.GPUs, "gpus", util.GPUs, "Comma-separated list of GPU product strings for matching GPUs to cache optimized models. Eg: h100, a100, l40s.")
	cmd.Flags().StringVar(&options.AltEndpoint, "alt-endpoint", util.AltEndpoint, "Endpoint for HuggingFace/NeMo DataStore. If source is NeMo DataStore, this is the HuggingFace endpoint from NeMo DataStore.")
	cmd.Flags().StringVar(&options.AltNamespace, "alt-namespace", util.AltNamespace, "Namespace within the HuggingFace Hub/NeMo DataStore.")
	cmd.Flags().StringVar(&options.ModelName, "model-name", util.ModelName, "Name of the model when nim source is HF/Nemo.")
	cmd.Flags().StringVar(&options.DatasetName, "dataset-name", util.DatasetName, "Name of the dataset when nim source is HF/Nemo.")
	cmd.Flags().StringVar(&options.Revision, "revision", util.Revision, "Revision of object to be cached when nim source is HF/Nemo. Either a commit hash, branch name or tag.")

	// Common flags.
	cmd.Flags().BoolVar(&options.PVCCreate, "pvc-create", util.PVCCreate, "Specify as true to create a new PVC. Default is false.")
	cmd.Flags().StringVar(&options.PVCStorageName, "pvc-storage-name", util.PVCStorageName, "PVC name to use for storage. Can be used to specify existing PVC as well as creating new PVC")
	cmd.Flags().StringVar(&options.PVCVolumeAccessMode, "pvc-volume-access-mode", util.PVCVolumeAccessMode, "Volume access mode for PVC creation. Must provide if creating new PVC.")
	cmd.Flags().StringVar(&options.PVCSize, "pvc-size", util.PVCSize, "Size for PVC creation. Must provide if creating new PVC.")
	cmd.Flags().StringVar(&options.PVCStorageClass, "pvc-storage-class", util.PVCStorageClass, "Storage class for PVC creation. Optional.")
	cmd.Flags().StringVar(&options.AuthSecret, "auth-secret", util.AuthSecret, "Auth secret to use for accessing NGC.")
	cmd.Flags().StringVar(&options.AltSecret, "alt-secret", util.AltSecret, "Auth secret to use for accessing HF/NemoDataStore.")

	return cmd
}

// Ensure the source is defined. This is because there are common fields across all three sources, making it challenging to decide which one the user intends to set.
func Validate(options *NIMCacheOptions) error {
	if strings.ToLower(options.SourceConfiguration) != "ngc" && strings.ToLower(options.SourceConfiguration) != "huggingface" && strings.ToLower(options.SourceConfiguration) != "nemodatastore" {
		print(options.SourceConfiguration)
		return fmt.Errorf("--nim-source must be set to one of 'ngc', 'huggingface', 'nemodatastore'. is %q", options.SourceConfiguration)
	}
	return nil
}

// Will need different Run commands for NewCreateNIMCacheCommand and nimservice command.
func RunCreateNIMCache(ctx context.Context, options *NIMCacheOptions, k8sClient client.Client) error {

	// Fill out NIMCache Spec.
	nimcache, err := FillOutNIMCacheSpec(options)
	if err != nil {
		return err
	}

	// Set metadata.
	nimcache.Name = options.ResourceName
	nimcache.Namespace = options.Namespace

	// Create the NIMCache CR.
	if _, err := k8sClient.NIMClient().AppsV1alpha1().NIMCaches(options.Namespace).Create(ctx, nimcache, v1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create NIMCache %s/%s: %w", options.Namespace, options.ResourceName, err)
	}

	fmt.Fprintf(options.IoStreams.Out, "NIMCache %q created in namespace %q\n", options.ResourceName, options.Namespace)
	return nil
}

func FillOutNIMCacheSpec(options *NIMCacheOptions) (*appsv1alpha1.NIMCache, error) {
	// Create a sample NIMService.
	nimcache := appsv1alpha1.NIMCache{}

	source := strings.ToLower(options.SourceConfiguration)

	// Fill out Source
	switch source {
	case "ngc":
		// Empty pointer, so initialize
		nimcache.Spec.Source.NGC = &appsv1alpha1.NGCSource{}
		nimcache.Spec.Source.NGC.AuthSecret = options.AuthSecret
		nimcache.Spec.Source.NGC.ModelPuller = options.ModelPuller
		nimcache.Spec.Source.NGC.PullSecret = options.PullSecret
		if options.ModelEndpoint != "" {
			nimcache.Spec.Source.NGC.ModelEndpoint = ptr.To(options.ModelEndpoint)
		} else {
			// Model fields
			nimcache.Spec.Source.NGC.Model = &appsv1alpha1.ModelSpec{}
			if len(options.Profiles) > 0 {
				nimcache.Spec.Source.NGC.Model.Profiles = options.Profiles
			}
			if options.Precision != "" {
				nimcache.Spec.Source.NGC.Model.Precision = options.Precision
			}
			if options.Engine != "" {
				nimcache.Spec.Source.NGC.Model.Engine = options.Engine
			}
			if options.TensorParallelism != "" {
				nimcache.Spec.Source.NGC.Model.TensorParallelism = options.TensorParallelism
			}
			if options.QosProfile != "" {
				nimcache.Spec.Source.NGC.Model.QoSProfile = options.QosProfile
			}
			if options.QosProfile != "" {
				nimcache.Spec.Source.NGC.Model.QoSProfile = options.QosProfile
			}
			if len(options.GPUs) > 0 {
				gpuSpecs := make([]appsv1alpha1.GPUSpec, 0, len(options.GPUs))
				for _, product := range options.GPUs {
					gpuSpecs = append(gpuSpecs, appsv1alpha1.GPUSpec{Product: product})
				}
				nimcache.Spec.Source.NGC.Model.GPUs = gpuSpecs
			}
			if options.Lora != "" {
				parsed, err := strconv.ParseBool(options.Lora)
				if err != nil {
					return nil, fmt.Errorf("--lora must be either 'true' or 'false.'")
				}
				nimcache.Spec.Source.NGC.Model.Lora = ptr.To(parsed)
			}
			if options.Buildable != "" {
				parsed, err := strconv.ParseBool(options.Buildable)
				if err != nil {
					return nil, fmt.Errorf("--buildable must be either 'true' or 'false.'")
				}
				nimcache.Spec.Source.NGC.Model.Buildable = ptr.To(parsed)
			}
		}

	case "huggingface":
		nimcache.Spec.Source.HF = &appsv1alpha1.HuggingFaceHubSource{}
		nimcache.Spec.Source.HF.Endpoint = options.AltEndpoint
		nimcache.Spec.Source.HF.Namespace = options.AltNamespace
		nimcache.Spec.Source.HF.AuthSecret = options.AltSecret
		nimcache.Spec.Source.HF.ModelPuller = options.ModelPuller
		nimcache.Spec.Source.HF.PullSecret = options.PullSecret

		fillOutDSHF(&nimcache, options)
	default:
		//NeMo DataStore
		nimcache.Spec.Source.DataStore = &appsv1alpha1.NemoDataStoreSource{}
		nimcache.Spec.Source.DataStore.Endpoint = options.AltEndpoint
		nimcache.Spec.Source.DataStore.Namespace = options.AltNamespace
		nimcache.Spec.Source.DataStore.AuthSecret = options.AltSecret
		nimcache.Spec.Source.DataStore.ModelPuller = options.ModelPuller
		nimcache.Spec.Source.DataStore.PullSecret = options.PullSecret
		fillOutDSHF(&nimcache, options)
	}

	if options.ResourcesCPU != "" {
		parsedCPU, err := resource.ParseQuantity(options.ResourcesCPU)
		if err != nil {
			return nil, err
		}
		nimcache.Spec.Resources.CPU = parsedCPU
	}

	if options.ResourcesMemory != "" {
		parsedMem, err := resource.ParseQuantity(options.ResourcesMemory)
		if err != nil {
			return nil, err
		}
		nimcache.Spec.Resources.Memory = parsedMem
	}

	if options.PVCStorageName != "" {
		nimcache.Spec.Storage.PVC.Name = options.PVCStorageName
	}
	nimcache.Spec.Storage.PVC.Create = ptr.To(options.PVCCreate)

	if *nimcache.Spec.Storage.PVC.Create {
		switch options.PVCVolumeAccessMode {
		case string(corev1.ReadWriteOnce), string(corev1.ReadOnlyMany), string(corev1.ReadWriteMany), string(corev1.ReadWriteOncePod):
			nimcache.Spec.Storage.PVC.VolumeAccessMode = corev1.PersistentVolumeAccessMode(options.PVCVolumeAccessMode)
		default:
			return nil, fmt.Errorf("invalid pvc-volume-access-mode: %q", options.PVCVolumeAccessMode)
		}
	}

	if options.PVCStorageClass != "" {
		nimcache.Spec.Storage.PVC.StorageClass = options.PVCStorageClass
	}

	if options.PVCSize != "" {
		nimcache.Spec.Storage.PVC.Size = options.PVCSize
	}

	return &nimcache, nil
}

func fillOutDSHF(nimcache *appsv1alpha1.NIMCache, options *NIMCacheOptions) {
	if options.SourceConfiguration == "huggingface" {
		if options.ModelName != "" {
			nimcache.Spec.Source.HF.ModelName = ptr.To(options.ModelName)
		}
		if options.DatasetName != "" {
			nimcache.Spec.Source.HF.DatasetName = ptr.To(options.DatasetName)
		}
		if options.Revision != "" {
			nimcache.Spec.Source.HF.Revision = ptr.To(options.Revision)
		}
	} else {
		// NeMo DataStore
		if options.ModelName != "" {
			nimcache.Spec.Source.DataStore.ModelName = ptr.To(options.ModelName)
		}
		if options.DatasetName != "" {
			nimcache.Spec.Source.DataStore.DatasetName = ptr.To(options.DatasetName)
		}
		if options.Revision != "" {
			nimcache.Spec.Source.DataStore.Revision = ptr.To(options.Revision)
		}
	}
}
