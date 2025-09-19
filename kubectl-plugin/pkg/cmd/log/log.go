package log

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"k8s-nim-operator-cli/pkg/util"

	"k8s-nim-operator-cli/scripts"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func NewLogCommand(cmdFactory cmdutil.Factory, streams genericclioptions.IOStreams) *cobra.Command {
	options := util.NewFetchResourceOptions(cmdFactory, streams)

	cmd := &cobra.Command{
		Use:   "log",
		Short: "Get custom resource logs in a namespace",
		Long:  "Gather the logs of all NIM Operator CRs in a namespace and create a diagnostic bundle",
		Example: `  nim logs collect -n nim-resources
  nim logs stream nimservice llama3b-instruct`,
		Aliases:      []string{"logs"},
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch len(args) {
			case 0:
				// Show help if no args provided.
				cmd.HelpFunc()(cmd, args)
				return nil
			case 1:
				// Check if the single argument is a valid subcommand
				validSubcommands := []string{"collect", "stream"}
				isValidSubcommand := false
				for _, validCmd := range validSubcommands {
					if args[0] == validCmd {
						isValidSubcommand = true
						break
					}
				}

				if !isValidSubcommand {
					return fmt.Errorf("unknown subcommand %q. Available subcommands: %s", args[0], strings.Join(validSubcommands, ", "))
				}

				// If it's "collect", treat it as the old behavior (namespace)
				if args[0] == "collect" {
					if err := options.CompleteNamespace([]string{}, cmd); err != nil {
						return err
					}
					// Overwrite this as it would be populated with "collect". There is also no ResourceName involved in this operation.
					options.ResourceName = ""
					return Run(cmd.Context(), options)
				}

				// For "stream", show help since it needs more arguments
				if args[0] == "stream" {
					return fmt.Errorf("stream subcommand requires additional arguments: RESOURCE NAME")
				}

				return nil
			default:
				return fmt.Errorf("too many arguments provided: %q", strings.Join(args, " "))
			}
		},
	}

	cmd.AddCommand(NewLogStreamCommand(cmdFactory, streams))
	cmd.AddCommand(NewLogCollectCommand(cmdFactory, streams))

	return cmd
}

func Run(ctx context.Context, options *util.FetchResourceOptions) error {
	// Materialize the embedded script.
	tmp, err := os.CreateTemp("", "must-gather-*.sh")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := os.WriteFile(tmp.Name(), scripts.MustGather, 0o755); err != nil {
		return fmt.Errorf("write script: %w", err)
	}

	// Run it and capture BOTH streams (xtrace is on stderr).
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "/bin/bash", tmp.Name())
	cmd.Env = append(os.Environ(),
		"OPERATOR_NAMESPACE=nim-operator",
		"NIM_NAMESPACE="+options.Namespace,
	)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("must-gather failed: %w\nstderr:\n%s", err, stderr.String())
	}

	// Parse ARTIFACT_DIR from the script output.
	artifactDir := parseArtifactDir(stdout.Bytes(), stderr.Bytes())
	if artifactDir == "" {
		return fmt.Errorf("could not find ARTIFACT_DIR in script output")
	}

	// Collect matching log file paths (prefix "<resourceName>-*.log").
	paths, err := listResourceLogPaths(artifactDir)
	if err != nil {
		return err
	}

	nimDir := filepath.Join(artifactDir, "nim")
	fmt.Printf("\nDiagnostic bundle created at  %s.\n", nimDir)
	fmt.Printf("Saved %d log file(s) in:\n", len(paths))
	for _, p := range paths {
		fmt.Printf("  %s\n", p)
	}
	return nil
}
