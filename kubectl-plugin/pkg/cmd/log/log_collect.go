package log

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"k8s-nim-operator-cli/pkg/util"
	"k8s-nim-operator-cli/pkg/util/client"

	"k8s-nim-operator-cli/scripts"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func NewLogCollectCommand(cmdFactory cmdutil.Factory, streams genericclioptions.IOStreams) *cobra.Command {
	options := util.NewFetchResourceOptions(cmdFactory, streams)

	cmd := &cobra.Command{
		Use:   "collect",
		Short: "Get custom resource logs in a namespace",
		Long:  "Gather the logs of all NIM Operator CRs in a namespace and create a diagnostic bundle",
		Example: `  nim logs collect
  nim logs collect -n nim-service`,
		Aliases:      []string{"gather"},
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch len(args) {
			case 0:
				// Proceed as normal if no args provided.
				if err := options.CompleteNamespace(args, cmd); err != nil {
					return err
				}

				// Validate namespace exists before proceeding
				k8sClient, err := client.NewClient(cmdFactory)
				if err != nil {
					return fmt.Errorf("failed to create client: %w", err)
				}

				if err := validateNamespaceExists(cmd.Context(), k8sClient, options.Namespace); err != nil {
					return err
				}

				return RunCollect(cmd.Context(), options)
			default:
				return fmt.Errorf("too many arguments provided: %q. Expected no arguments", strings.Join(args, " "))
			}
		},
	}

	cmd.SetHelpTemplate(helpTemplate)

	return cmd
}

// validateNamespaceExists checks if the specified namespace exists in the cluster
func validateNamespaceExists(ctx context.Context, k8sClient client.Client, namespace string) error {
	_, err := k8sClient.KubernetesClient().CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("namespace %q not found", namespace)
		}
		return fmt.Errorf("failed to validate namespace %q: %w", namespace, err)
	}
	return nil
}

func RunCollect(ctx context.Context, options *util.FetchResourceOptions) error {
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

// parse ARTIFACT_DIR=... from either stream (handles xtrace on stderr).
func parseArtifactDir(out1, out2 []byte) string {
	if v := findArtifactDirIn(out1); v != "" {
		return v
	}
	if v := findArtifactDirIn(out2); v != "" {
		return v
	}
	return ""
}
func findArtifactDirIn(b []byte) string {
	re := regexp.MustCompile(`\bARTIFACT_DIR=([^\s]+)`)
	if m := re.FindSubmatch(b); len(m) == 2 {
		return string(m[1])
	}
	// fallback for explicit xtrace line
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "+ export ARTIFACT_DIR=") {
			return strings.TrimPrefix(line, "+ export ARTIFACT_DIR=")
		}
	}
	return ""
}

// returns full paths to files like "<artifactDir>/nim/<resourceName>-*.log".
func listResourceLogPaths(artifactDir string) ([]string, error) {
	nimDir := filepath.Join(artifactDir, "nim")
	ents, err := os.ReadDir(nimDir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", nimDir, err)
	}

	var paths []string
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".log") {
			paths = append(paths, filepath.Join(nimDir, name))
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no .log files found in %s", nimDir)
	}

	// newest first by mtime
	sort.Slice(paths, func(i, j int) bool {
		ai, _ := os.Stat(paths[i])
		aj, _ := os.Stat(paths[j])
		if ai != nil && aj != nil && !ai.ModTime().Equal(aj.ModTime()) {
			return ai.ModTime().After(aj.ModTime())
		}
		return paths[i] > paths[j]
	})

	return paths, nil
}

// Custom help message template. Needed to show supported resource types as a custom category to be consistent with "Available Commands" for get and status.
const helpTemplate = `{{- if .Long }}{{ .Long }}{{- else }}{{ .Short }}{{- end }}

Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}

{{if gt (len .Aliases) 0}}Aliases:
  {{.NameAndAliases}}

Supported COMMANDS:
  collect     Collect logs of all NIM Operator custom resources in a namespace and write them out to a diagnostic bundle.

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
