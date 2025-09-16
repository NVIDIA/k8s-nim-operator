package create

// TODO: implement autocompletion
// TODO: remove unnecessary packages like raycluster from go.sum/mod

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func NewCreateCommand(cmdFactory cmdutil.Factory, streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "create",
		Short:        "Create a NIM Operator custom resource",
		Long:         `Creates the specified NIM Operator resource with the parameters specified through flags.`,
		SilenceUsage: true,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) > 0 {
				fmt.Println(fmt.Errorf("unknown command(s) %q", strings.Join(args, " ")))
			}
			cmd.HelpFunc()(cmd, args)
		},
	}

	cmd.AddCommand(NewCreateNIMCacheCommand(cmdFactory, streams))
	cmd.AddCommand(NewCreateNIMServiceCommand(cmdFactory, streams))
	return cmd
}