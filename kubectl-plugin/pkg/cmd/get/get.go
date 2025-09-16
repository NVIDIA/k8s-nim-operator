package get

import (
	"fmt"
	"strings"
	"context"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s-nim-operator-cli/pkg/util"
	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"k8s-nim-operator-cli/pkg/util/client"
)

func NewGetCommand(cmdFactory cmdutil.Factory, streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "get",
		Short:        "Display one or many NIM Operator custom resources",
		Long:         `Prints a table of the most important information about the specified NIM Operator resources`,
		Aliases:      []string{"list"},
		SilenceUsage: true,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) > 0 {
				fmt.Println(fmt.Errorf("unknown command(s) %q", strings.Join(args, " ")))
			}
			cmd.HelpFunc()(cmd, args)
		},
	}

	cmd.AddCommand(NewGetNIMCacheCommand(cmdFactory, streams))
	cmd.AddCommand(NewGetNIMServiceCommand(cmdFactory, streams))
	return cmd
}

// Common Run command for get's custom resources.
func Run(ctx context.Context, options *util.FetchResourceOptions, k8sClient client.Client) error {
	resourceList, err := util.FetchResources(ctx, options, k8sClient)
	if err != nil {
		return err
	}

	switch options.ResourceType {

	case util.NIMService:
		// Cast resourceList to NIMServiceList.
		nimServiceList, ok := resourceList.(*appsv1alpha1.NIMServiceList)
		if !ok {
			return fmt.Errorf("failed to cast resourceList to NIMServiceList")
		}
		return printNIMServices(nimServiceList, options.IoStreams.Out)

	case util.NIMCache:
		// Cast resourceList to NIMCacheList.
		nimCacheList, ok := resourceList.(*appsv1alpha1.NIMCacheList)
		if !ok {
			return fmt.Errorf("failed to cast resourceList to NIMCacheList")
		}
		return printNIMCaches(nimCacheList, options.IoStreams.Out)
	}

	return err
}