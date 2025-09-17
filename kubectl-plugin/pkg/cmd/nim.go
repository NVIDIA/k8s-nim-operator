package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"k8s-nim-operator-cli/pkg/cmd/create"
	"k8s-nim-operator-cli/pkg/cmd/delete"
	"k8s-nim-operator-cli/pkg/cmd/deploy"
	"k8s-nim-operator-cli/pkg/cmd/get"
	"k8s-nim-operator-cli/pkg/cmd/log"
	"k8s-nim-operator-cli/pkg/cmd/status"
)

func init() {
	// Initialize the controller-runtime logger globally
	logger := zap.New(zap.UseDevMode(true))
	ctrl.SetLogger(logger)
}

func NewNIMCommand(streams genericiooptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "nim",
		Short:        "nim operator kubectl plugin",
		Long:         "Manage NIM Operator resources like NIMCache and NIMService on Kubernetes",
		SilenceUsage: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.HelpFunc()(cmd, args)
		},
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}

	configFlags := genericclioptions.NewConfigFlags(true)
	configFlags.AddFlags(cmd.PersistentFlags())

	// Hide inherited kubeconfig-related global flags from help output
	cmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		_ = cmd.PersistentFlags().MarkHidden(f.Name)
	})

	cmdFactory := cmdutil.NewFactory(configFlags)

	cmd.AddCommand(get.NewGetCommand(cmdFactory, streams))
	cmd.AddCommand(status.NewStatusCommand(cmdFactory, streams))
	cmd.AddCommand(log.NewLogCommand(cmdFactory, streams))
	cmd.AddCommand(delete.NewDeleteCommand(cmdFactory, streams))
	cmd.AddCommand(create.NewCreateCommand(cmdFactory, streams))
	cmd.AddCommand(deploy.NewDeployCommand(cmdFactory, streams))

	return cmd
}
