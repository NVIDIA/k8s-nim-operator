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
			case 1:
				// Proceed as normal if one arg provided.
				if err := options.CompleteNamespace(args, cmd); err != nil {
					return err
				}
				// Overwrite this as it would be populated with "collect". There is also no ResourceName involved in this operation.
				options.ResourceName = ""
				return Run(cmd.Context(), options)
			default:
				fmt.Println(fmt.Errorf("unknown command(s) %q", strings.Join(args, " ")))
			}
			return nil
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
