package log

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"

	"k8s-nim-operator-cli/pkg/util"
	"k8s-nim-operator-cli/pkg/util/client"
)

func NewLogStreamCommand(cmdFactory cmdutil.Factory, streams genericclioptions.IOStreams) *cobra.Command {
	options := util.NewFetchResourceOptions(cmdFactory, streams)

	cmd := &cobra.Command{
		Use:   "stream RESOURCE NAME",
		Short: "Stream custom resource logs",
		Long:  "Stream the logs of all pods of a specified NIM Operator custom resource",
		Example: `  nim log stream nimcache my-cache -n nim-cache
  nim log stream nimservice my-service`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch len(args) {
			case 0:
				// Show help if no args provided.
				cmd.HelpFunc()(cmd, args)
				return fmt.Errorf("missing required arguments: RESOURCE and NAME")
			case 2:
				// Proceed as normal if two args provided.
				if err := options.CompleteNamespace(args, cmd); err != nil {
					return err
				}
				k8sClient, err := client.NewClient(cmdFactory)
				if err != nil {
					return fmt.Errorf("failed to create client: %w", err)
				}
				return RunStream(cmd.Context(), options, k8sClient)
			default:
				return fmt.Errorf("too many arguments provided: %q. Expected: RESOURCE NAME", strings.Join(args, " "))
			}
		},
	}

	cmd.SetHelpTemplate(helpTemplate)

	return cmd
}

func RunStream(ctx context.Context, options *util.FetchResourceOptions, k8sClient client.Client) error {
	resourceList, err := util.FetchResources(ctx, options, k8sClient)
	if err != nil {
		return err
	}

	var (
		ns       = options.Namespace
		name     = options.ResourceName
		selector string
	)

	// Get the selector.
	switch options.ResourceType {

	case util.NIMService:
		nl, ok := resourceList.(*appsv1alpha1.NIMServiceList)
		if !ok || len(nl.Items) == 0 {
			return fmt.Errorf("NIMService %q not found", name)
		}
		ns = nl.Items[0].Namespace

		if selector == "" {
			selector = fmt.Sprintf("app.kubernetes.io/instance=%s", name)
		}

	case util.NIMCache:
		cl, ok := resourceList.(*appsv1alpha1.NIMCacheList)
		if !ok || len(cl.Items) == 0 {
			return fmt.Errorf("NIMCache %q not found", name)
		}
		ns = cl.Items[0].Namespace

		// Same selector logic as above
		if selector == "" {
			selector = fmt.Sprintf("app.kubernetes.io/instance=%s", name)
		}
	}

	return util.StreamResourceLogs(ctx, options, k8sClient, ns, name, selector)
}
