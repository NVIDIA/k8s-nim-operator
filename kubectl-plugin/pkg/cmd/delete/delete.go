package delete

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"

	"k8s-nim-operator-cli/pkg/util"
	"k8s-nim-operator-cli/pkg/util/client"
)

func NewDeleteCommand(cmdFactory cmdutil.Factory, streams genericclioptions.IOStreams) *cobra.Command {
	options := util.NewFetchResourceOptions(cmdFactory, streams)

	cmd := &cobra.Command{
		Use:   "delete RESOURCE_TYPE RESOURCE_NAME",
		Short: "Delete a custom resource deployment",
		Long:  "Delete a NIM Operator custom resource's deployment",
		Example: `  nim delete nimcache my-cache
  nim delete nimservice my-service`,
		Aliases:      []string{"remove"},
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
				// Parse resource type and name from args
				resource := strings.ToLower(args[0])
				name := args[1]
				switch resource {
				case "nimservice", "nimservices":
					options.ResourceType = util.NIMService
				case "nimcache", "nimcaches":
					options.ResourceType = util.NIMCache
				default:
					return fmt.Errorf("unsupported resource type %q", resource)
				}
				options.ResourceName = name
				k8sClient, err := client.NewClient(cmdFactory)
				if err != nil {
					return fmt.Errorf("failed to create client: %w", err)
				}
				return Run(cmd.Context(), options, k8sClient)
			default:
				fmt.Println(fmt.Errorf("unknown command(s) %q", strings.Join(args, " ")))
			}
			return nil
		},
	}

	cmd.SetHelpTemplate(helpTemplate)

	return cmd
}

func Run(ctx context.Context, options *util.FetchResourceOptions, k8sClient client.Client) error {
	resourceList, err := util.FetchResources(ctx, options, k8sClient)
	if err != nil {
		return err
	}

	var (
		ns   string
		name string = options.ResourceName
	)

	switch options.ResourceType {
	case util.NIMService:
		nl, ok := resourceList.(*appsv1alpha1.NIMServiceList)
		if !ok || len(nl.Items) == 0 {
			return fmt.Errorf("NIMService %q not found", name)
		}
		ns = nl.Items[0].Namespace

		if err := k8sClient.NIMClient().AppsV1alpha1().NIMServices(ns).Delete(ctx, name, v1.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete NIMService %s/%s: %w", ns, name, err)
		}
		fmt.Fprintf(options.IoStreams.Out, "NIMService %q deleted in namespace %q\n", name, ns)

	case util.NIMCache:
		cl, ok := resourceList.(*appsv1alpha1.NIMCacheList)
		if !ok || len(cl.Items) == 0 {
			return fmt.Errorf("NIMCache %q not found", name)
		}
		ns = cl.Items[0].Namespace

		if err := k8sClient.NIMClient().AppsV1alpha1().NIMCaches(ns).Delete(ctx, name, v1.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete NIMCache %s/%s: %w", ns, name, err)
		}
		fmt.Fprintf(options.IoStreams.Out, "NIMCache %q deleted in namespace %q\n", name, ns)

	default:
		return fmt.Errorf("unsupported resource type %q", options.ResourceType)
	}

	return nil
}

// Custom help message template. Needed to show supported resource types as a custom category to be consistent with "Available Commands" for get and status.
const helpTemplate = `{{- if .Long }}{{ .Long }}{{- else }}{{ .Short }}{{- end }}

Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}

{{if gt (len .Aliases) 0}}Aliases:
  {{.NameAndAliases}}

Supported RESOURCE types:
  nimcache     Delete a NIMCache deployment.
  nimservice   Delete a NIMService deployment.

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
