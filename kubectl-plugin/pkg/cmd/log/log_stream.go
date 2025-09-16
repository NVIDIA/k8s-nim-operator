package log

import (
	"context"
	"fmt"
	"strings"

	"k8s-nim-operator-cli/pkg/util"
	"k8s-nim-operator-cli/pkg/util/client"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
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
				fmt.Println(fmt.Errorf("unknown command(s) %q", strings.Join(args, " ")))
			}
			return nil
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

// Custom help message template. Needed to show supported resource types as a custom category to be consistent with "Available Commands" for get and status.
const streamHelpTemplate = `{{- if .Long }}{{ .Long }}{{- else }}{{ .Short }}{{- end }}

Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}

{{if gt (len .Aliases) 0}}Aliases:
  {{.NameAndAliases}}

Supported RESOURCE types:
  nimcache     Stream NIMCache logs.
  nimservice   Stream NIMService logs.

{{end}}{{if .HasExample}}Examples:
{{ .Example }}

{{end}}{{if .HasAvailableLocalFlags}}Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}

{{end}}{{if .HasAvailableInheritedFlags}}Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}

{{end}}{{if .HasHelpSubCommands}}Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{.CommandPath}} {{.Short}}{{end}}{{end}}

{{end}}{{if .HasAvailableSubCommands}}Available Commands:{{range .Commands}}{{if (and .IsAvailableCommand (not .IsAdditionalHelpTopicCommand))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}

{{end}}`
