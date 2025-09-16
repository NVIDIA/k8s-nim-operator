package create

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"context"
	util "k8s-nim-operator-cli/pkg/util"
	"k8s-nim-operator-cli/pkg/util/client"

	"k8s.io/utils/ptr"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

type NIMServiceOptions struct {
	cmdFactory             cmdutil.Factory
	IoStreams              *genericclioptions.IOStreams
	Namespace              string
	ResourceName           string
	ResourceType           util.ResourceType
	AllNamespaces          bool
	ImageRepository        string
	Tag                    string
	NIMCacheStorageName    string
	NIMCacheStorageProfile string
	PVCCreate              bool
	PVCStorageName         string
	PVCStorageClass        string
	PVCSize                string
	PVCVolumeAccessMode    string
	PullPolicy             string
	PullSecrets            []string
	Env                    []string
	AuthSecret             string
	ServicePort            int32
	ServiceType            string
	GPULimit               string
	Replicas               int
	ScaleMaxReplicas       int32
	ScaleMinReplicas       int32
	InferencePlatform      string
	HostPath               string
}

func NewNIMServiceOptions(cmdFactory cmdutil.Factory, streams genericclioptions.IOStreams) *NIMServiceOptions {
	return &NIMServiceOptions{
		cmdFactory: cmdFactory,
		IoStreams:  &streams,
	}
}

// Populates NIMServiceOptions with namespace and resource name (if present).
func (options *NIMServiceOptions) CompleteNamespace(args []string, cmd *cobra.Command) error {
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

func NewCreateNIMServiceCommand(cmdFactory cmdutil.Factory, streams genericclioptions.IOStreams) *cobra.Command {
	options := NewNIMServiceOptions(cmdFactory, streams)

	cmd := &cobra.Command{
		Use:   "nimservice [NAME]",
		Short: "Create new NIMService with specified information",
		Long: `Create new NIMService with specified parameters. 

Minimum required flags are --image-repository, --tag, and storage: reference either an existing NIMCache with --nimcache-storage-name, or reference an existing/create new PVC. 
	- If using existing PVC, minimum required flags are pvc-storage-name. 
	- If creating new PVC, minimum required flags are pvc-create, pvc-size, pvc-volume-access-mode, pvc-storage-class.`,
		SilenceUsage: true,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				cmd.HelpFunc()(cmd, args)
				return nil
			} else {
				if err := options.CompleteNamespace(args, cmd); err != nil {
					return err
				}
				// running cmd.Execute or cmd.ExecuteE sets the context, which will be done by root.
				k8sClient, err := client.NewClient(cmdFactory)
				if err != nil {
					return fmt.Errorf("failed to create client: %w", err)
				}
				options.ResourceType = util.NIMService
				return RunCreateNIMService(cmd.Context(), options, k8sClient)
			}
		},
	}

	cmd.Example = strings.Join([]string{
		"  Creating NIMService with existing PVC as storage.",
		"    kl nim create nimservice llama3-nimservice --image-repository=nvcr.io/nim/meta/llama-3.1-8b-instruct --tag=1.3.3 --pvc-storage-name=nim-pvc",
		"",
		"  Creating NIMService without existing PVC as storage.",
		"    kl nim create nimservice llama3-nimservice --image-repository=nvcr.io/nim/meta/llama-3.1-8b-instruct --tag=1.3.3 --pvc-create=true --pvc-size=20Gi --pvc-volume-access-mode=ReadWriteMany --pvc-storage-class=<storage-class-name>",
		"",
		"  Creating NIMService with existing NIMCache as storage.",
		"    kl nim create nimservice llama3-nimservice --image-repository=nvcr.io/nim/meta/llama-3.1-8b-instruct --tag=1.3.3 --nimcache-storage-name=<nimcache-name>",
	}, "\n")

	// The first argument will be name. Other arguments will be specified as flags.
	cmd.Flags().StringVar(&options.ImageRepository, "image-repository", util.ImageRepository, "Repository to pull image from. Required")
	cmd.Flags().StringVar(&options.Tag, "tag", util.Tag, "Image tag. Required")
	cmd.Flags().StringVar(&options.NIMCacheStorageName, "nimcache-storage-name", util.NIMCacheStorageName, "Nimcache name to use for storage.")
	cmd.Flags().StringVar(&options.NIMCacheStorageProfile, "nimcache-storage-profile", util.NIMCacheStorageProfile, "Nimcache profile to use for storage.")
	cmd.Flags().BoolVar(&options.PVCCreate, "pvc-create", util.PVCCreate, "Specify as true to create a new PVC. Default is false.")
	cmd.Flags().StringVar(&options.PVCStorageName, "pvc-storage-name", util.PVCStorageName, "PVC name to use for storage. Can be used to specify existing PVC as well as creating new PVC")
	cmd.Flags().StringVar(&options.PVCVolumeAccessMode, "pvc-volume-access-mode", util.PVCVolumeAccessMode, "Volume access mode for PVC creation. Must provide if creating new PVC.")
	cmd.Flags().StringVar(&options.PVCSize, "pvc-size", util.PVCSize, "Size for PVC creation. Must provide if creating new PVC.")
	cmd.Flags().StringVar(&options.PVCStorageClass, "pvc-storage-class", util.PVCStorageClass, "Storage class for PVC creation.")
	cmd.Flags().StringVar(&options.PullPolicy, "pull-policy", util.PullPolicy, "Pull policy to use while pulling image.")
	cmd.Flags().StringVar(&options.AuthSecret, "auth-secret", util.AuthSecret, "Auth secret to use for accessing NGC.")
	cmd.Flags().StringSliceVar(&options.PullSecrets, "pull-secrets", util.PullSecrets, "Comma-separated list of image pull secrets.")
	cmd.Flags().StringSliceVar(&options.Env, "env", util.Env, "Comma-separated list of env_name,env. Only supports one pair.")
	cmd.Flags().Int32Var(&options.ServicePort, "service-port", util.ServicePort, "Port to expose NIMService.")
	cmd.Flags().StringVar(&options.ServiceType, "service-type", util.ServiceType, "Service type to use in expose.")
	cmd.Flags().IntVar(&options.Replicas, "replicas", util.Replicas, "Number of replicas for the NIMService.")
	cmd.Flags().StringVar(&options.GPULimit, "gpu-limit", util.GPULimit, "Maximum number of GPUs the NIMService can use.")
	cmd.Flags().Int32Var(&options.ScaleMaxReplicas, "scale-max-replicas", util.ScaleMaxReplicas, "Maximum number of replicas for the NIMService's HorizontalPodAutoscaler.")
	cmd.Flags().Int32Var(&options.ScaleMinReplicas, "scale-min-replicas", util.ScaleMinReplicas, "Minimum number of replicas for the NIMService's HorizontalPodAutoscaler.")
	cmd.Flags().StringVar(&options.InferencePlatform, "inference-platform", util.InferencePlatform, "Inference platform to use for this service. Valid values are 'standalone' (default) and 'kserve.'")

	// add CPU/Memory resource limits?

	return cmd
}

// Will need different Run commands for NewCreateNIMCacheCommand and nimservice command.
func RunCreateNIMService(ctx context.Context, options *NIMServiceOptions, k8sClient client.Client) error {

	// Fill out NIMService Spec.
	nimservice, err := FillOutNIMServiceSpec(options)
	if err != nil {
		return err
	}

	// Set metadata.
	nimservice.Name = options.ResourceName
	nimservice.Namespace = options.Namespace

	// Create the NIMService CR.
	if _, err := k8sClient.NIMClient().AppsV1alpha1().NIMServices(options.Namespace).Create(ctx, nimservice, v1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create NIMService %s/%s: %w", options.Namespace, options.ResourceName, err)
	}

	fmt.Fprintf(options.IoStreams.Out, "NIMService %q created in namespace %q\n", options.ResourceName, options.Namespace)
	return nil
}

func FillOutNIMServiceSpec(options *NIMServiceOptions) (*appsv1alpha1.NIMService, error) {
	// Create a sample NIMService.
	nimservice := appsv1alpha1.NIMService{}

	// Fill out NIMService.Spec.
	// Complete Image.
	nimservice.Spec.Image.Repository = options.ImageRepository
	nimservice.Spec.Image.Tag = options.Tag

	// Complete Storage.
	if options.HostPath != "" {
		nimservice.Spec.Storage.HostPath = ptr.To(options.HostPath)
	} else if options.NIMCacheStorageName != "" {
		nimservice.Spec.Storage.NIMCache.Name = options.NIMCacheStorageName
		if options.NIMCacheStorageProfile != "" {
			nimservice.Spec.Storage.NIMCache.Profile = options.NIMCacheStorageProfile
		}
	} else {
		if options.PVCStorageName != "" {
			nimservice.Spec.Storage.PVC.Name = options.PVCStorageName
		}
		nimservice.Spec.Storage.PVC.Create = ptr.To(options.PVCCreate)

		switch options.PVCVolumeAccessMode {
		case string(corev1.ReadWriteOnce), string(corev1.ReadOnlyMany), string(corev1.ReadWriteMany), string(corev1.ReadWriteOncePod):
			nimservice.Spec.Storage.PVC.VolumeAccessMode = corev1.PersistentVolumeAccessMode(options.PVCVolumeAccessMode)
		default:
			return nil, fmt.Errorf("invalid pvc-volume-access-mode: %q", options.PVCVolumeAccessMode)
		}

		if options.PVCStorageClass != "" {
			nimservice.Spec.Storage.PVC.StorageClass = options.PVCStorageClass
		}

		if options.PVCSize != "" {
			nimservice.Spec.Storage.PVC.Size = options.PVCSize
		}
	}

	nimservice.Spec.AuthSecret = options.AuthSecret
	nimservice.Spec.Image.PullSecrets = options.PullSecrets
	nimservice.Spec.Image.PullPolicy = options.PullPolicy
	nimservice.Spec.Expose.Service.Port = ptr.To(options.ServicePort)

	switch options.ServiceType {
	case string(corev1.ServiceTypeClusterIP), string(corev1.ServiceTypeNodePort), string(corev1.ServiceTypeLoadBalancer):
		nimservice.Spec.Expose.Service.Type = corev1.ServiceType(options.ServiceType)
	default:
		return nil, fmt.Errorf("invalid service-type: %q", options.ServiceType)
	}

	parsedLimit, err := resource.ParseQuantity(options.GPULimit)
	if err != nil {
		return nil, err
	}
	requirements := &corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceName("nvidia.com/gpu"): parsedLimit,
		},
	}
	nimservice.Spec.Resources = requirements

	nimservice.Spec.Replicas = options.Replicas

	// If ScaleMaxReplicas is defined, autoscaling is enabled. ScaleMaxReplicas not being defined but ScaleMaxReplicas being defined will be taken care of by apiserver.
	if options.ScaleMaxReplicas != -1 {
		nimservice.Spec.Scale.Enabled = ptr.To(true)
		nimservice.Spec.Scale.HPA.MaxReplicas = int32(options.ScaleMaxReplicas)

		if options.ScaleMinReplicas != -1 {
			nimservice.Spec.Scale.HPA.MinReplicas = ptr.To(options.ScaleMinReplicas)
		}
	}

	switch options.InferencePlatform {
	case string(appsv1alpha1.PlatformTypeKServe), string(appsv1alpha1.PlatformTypeStandalone):
		nimservice.Spec.InferencePlatform = appsv1alpha1.PlatformType(options.InferencePlatform)
	default:
		return nil, fmt.Errorf("invalid inference-platform: %q, must be one of 'kserve,' 'standalone'", options.InferencePlatform)
	}

	// Expect max of two pairs of env name, value.
	if len(options.Env) == 2 {
		nimservice.Spec.Env = []corev1.EnvVar{{
			Name:  options.Env[0],
			Value: options.Env[1],
		}}
	} else if len(options.Env) == 4 {		  
		nimservice.Spec.Env = []corev1.EnvVar{
			{
			  Name:  options.Env[0],         // e.g. "NIM_MODEL_NAME"
			  Value: options.Env[1],         // e.g. "hf://..."
			},
			{
			  Name: options.Env[2],          // e.g. "HF_TOKEN"
			  ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
				  LocalObjectReference: corev1.LocalObjectReference{
					Name: options.Env[3],    // e.g. "hf-api-secret"
				  },
				  Key: "HF_TOKEN",
				  Optional: ptr.To(true),
				},
			  },
			},
		}
	} else if len(options.Env) != 0 {
		return &nimservice, fmt.Errorf("Only two env allowed, NIM_MODEL_NAME_ENV_VAR and HF_TOKEN.")
	}


	return &nimservice, nil
}
