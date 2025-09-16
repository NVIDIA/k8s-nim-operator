package deploy

// TODO: implement autocompletion
// TODO: remove unnecessayr packages like raycluster from go.sum/mod

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"strings"

	"k8s-nim-operator-cli/pkg/cmd/create"
	"k8s-nim-operator-cli/pkg/util"
	"k8s-nim-operator-cli/pkg/util/client"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

const (
	MULTI_LLM_NIM_REPO     = "nvcr.io/nim/nvidia/llm-nim"
	MULTI_LLM_TAG          = "1.12"
	NIM_MODEL_NAME_ENV_VAR = "NIM_MODEL_NAME"
)

func NewDeployCommand(cmdFactory cmdutil.Factory, streams genericclioptions.IOStreams) *cobra.Command {
	options := util.NewFetchResourceOptions(cmdFactory, streams)

	cmd := &cobra.Command{
		Use:          "deploy NAME",
		Short:        "Interactively deploy a NIMService custom resource.",
		Long:         `Given an image name and some more information, deploys a NIMService running a universal nim for the user (with/without NIMCache).
Note: ngc-secret, ngc-api-secret, and hf-api-secret (depending on model source) must exist and be defined pull secrets.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch len(args) {
			case 0:
				// Show help if no args provided.
				cmd.HelpFunc()(cmd, args)
			case 1:
				// Proceed as normal if one arg provided.
				if err := options.CompleteNamespace(args, cmd); err != nil {
					return err
				}
				k8sClient, err := client.NewClient(cmdFactory)
				if err != nil {
					return fmt.Errorf("failed to create client: %w", err)
				}
				createServiceCmd := create.NewCreateNIMServiceCommand(cmdFactory, streams)
				createCacheCmd := create.NewCreateNIMCacheCommand(cmdFactory, streams)
				errService, errCache := Run(cmd.Context(), options, k8sClient, createServiceCmd, createCacheCmd)
				if errCache != nil {
					return errCache
				} else if errService != nil {
					return errService
				}
				return nil
			default:
				fmt.Println(fmt.Errorf("unknown command(s) %q", strings.Join(args, " ")))
			}
			return nil
		},
	}

	return cmd
}

func Run(ctx context.Context, options *util.FetchResourceOptions, k8sClient client.Client, serviceCmd *cobra.Command, cacheCmd *cobra.Command) (error, error) {
	var cacheModel bool
	var imgSource string
	var endPoint string
	var altNamespace string
	var hfModelName string
	var pvcStorageClass string
	var pvcFlags []string

	reader := bufio.NewReader(options.IoStreams.In)
	// 1) Do you want to cache the model?
	for {
		fmt.Fprint(options.IoStreams.Out, "Do you want to cache the model? (yes/no): ")
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err), nil
		}

		response = strings.TrimSpace(strings.ToLower(response))
		switch response {
		case "yes":
			cacheModel = true
		case "no":
			cacheModel = false
		default:
			fmt.Fprintln(options.IoStreams.Out, "Invalid response. Please enter 'yes' or 'no'.")
			continue
		}
		break
	}

	// No: ask for NIM_MODEL_NAME. proceed to PVC creation steps (skip to step 3).
	if !cacheModel {
		fmt.Fprint(options.IoStreams.Out, "Enter the Model URL (eg: ngc://nvidia/nemo/<model_name>:2.0; hf://meta-llama/Llama-3.2-1B-Instruct): ")
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read model name: %w", err), nil
		}
		endPoint = strings.TrimSpace(response)
	} else {
		// Yes, continue below.
		// 2) Ask for image source.
		for {
			fmt.Fprint(options.IoStreams.Out, "Enter the image source for caching (ngc, huggingface, nemoDataStore): ")
			response, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read image source: %w", err), nil
			}
			imgSource = strings.TrimSpace(response)

			if imgSource == "ngc" || imgSource == "huggingface" || imgSource == "nemoDataStore" {
				break
			}
		}
		switch imgSource {
		case "ngc":
			// If ngc, ask for endpoint.
			fmt.Fprint(options.IoStreams.Out, "Enter the Model URL (eg: ngc://nvidia/nemo/<model_name>:2.0): ")
			response, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read model puller: %w", err), nil
			}
			endPoint = strings.TrimSpace(response)
	
		default:
			// Ask for endpoint & modelName.
			fmt.Fprint(options.IoStreams.Out, "Enter endpoint: ")
			response, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read endpoint: %w", err), nil
			}
			endPoint = strings.TrimSpace(response)

			fmt.Fprint(options.IoStreams.Out, "Enter namespace: ")
			response, err = reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read namespace: %w", err), nil
			}
			altNamespace = strings.TrimSpace(response)

			fmt.Fprint(options.IoStreams.Out, "Enter model name: ")
			response, err = reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read model name: %w", err), nil
			}
			hfModelName = strings.TrimSpace(response)
		}
	}

	// 3) Handle PVC creation
	// Ask for StorageClass to use for the PVC. Leaving blank will use the cluster's default.
	fmt.Fprint(options.IoStreams.Out, "Enter StorageClass for PVC (leave blank to use cluster default): ")
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read storage class: %w", err), nil
	}
	pvcStorageClass = strings.TrimSpace(response)

	// Create common PVC flags.
	pvcFlags = []string{
		"--pvc-create=true",
		"--pvc-size=20Gi",
		"--pvc-volume-access-mode=ReadWriteOnce",
	}
	if pvcStorageClass != "" {
		pvcFlags = append(pvcFlags, "--pvc-storage-class="+pvcStorageClass)
	}

	// Ensure subcommands have a namespace flag since they aren't attached to root
	serviceCmd.Flags().String("namespace", options.Namespace, "Namespace")
	cacheCmd.Flags().String("namespace", options.Namespace, "Namespace")

	// If caching is not desired, assemble the NIMService.
	if !cacheModel {
		serviceCmdArgs := []string{
			options.ResourceName,
			"--image-repository=" + MULTI_LLM_NIM_REPO,
			"--tag=" + MULTI_LLM_TAG,
			"--env=" + NIM_MODEL_NAME_ENV_VAR + "," + endPoint + ",HF_TOKEN," + "hf-api-secret",
		}

		serviceCmdArgs = append(serviceCmdArgs, pvcFlags...)
		serviceCmd.SetArgs(serviceCmdArgs)
		return serviceCmd.ExecuteContext(ctx), nil
	} else {
		// Assemble NIMCache flags.
		cacheCmdArgs := []string{
			options.ResourceName + "-cache",
			"--nim-source=" + imgSource,
		}

		cacheCmdArgs = append(cacheCmdArgs, "--model-puller=" + MULTI_LLM_NIM_REPO + ":" + MULTI_LLM_TAG)

		if imgSource == "ngc" {
			cacheCmdArgs = append(cacheCmdArgs, "--ngc-model-endpoint=" + endPoint)
		} else {
			cacheCmdArgs = append(cacheCmdArgs, "--alt-endpoint="+endPoint)
			cacheCmdArgs = append(cacheCmdArgs, "--alt-namespace="+altNamespace)
			cacheCmdArgs = append(cacheCmdArgs, "--model-name="+hfModelName)
			cacheCmdArgs = append(cacheCmdArgs, "--alt-secret=hf-api-secret")
		}
		cacheCmdArgs = append(cacheCmdArgs, pvcFlags...)
		cacheCmd.SetArgs(cacheCmdArgs)
		
		// Assemble NIMService flags.
		serviceCmdArgs := []string{
			options.ResourceName,
			"--image-repository=" + MULTI_LLM_NIM_REPO,
			"--tag=" + MULTI_LLM_TAG,
			"--nimcache-storage-name=" + options.ResourceName + "-cache",
		}
		serviceCmd.SetArgs(serviceCmdArgs)

		return serviceCmd.ExecuteContext(ctx), cacheCmd.ExecuteContext(ctx)
	}
}

/*
Control Flow: first make nimservice, the nimcache.

Questions:
1) Do you want to cache the model?
   - no: ask for NIM_MODEL_NAME. then proceed to PVC creation steps (skip to step 3)
   - yes: continue below:
2) Image source? NGC, or HF/NeMo DataStore
   - if ngc, ask for modelPuller
   - if hf/nemodatastore, ask for endpoint & hfModelName
3) Handle PVC creation
	- create new pvc of 20gb and use.
4) Assemble everything.
*/
