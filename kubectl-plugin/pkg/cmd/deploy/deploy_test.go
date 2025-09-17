package deploy

import (
	"bytes"
	"context"
	"fmt"
	"k8s-nim-operator-cli/pkg/util"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// --- Command structure tests ---

func Test_NewDeployCommand_Structure(t *testing.T) {
	streams := genericTestIOStreams()
	cmd := NewDeployCommand(nil, *streams)

	// Test basic command properties
	if cmd.Use != "deploy NAME" {
		t.Errorf("expected Use to be 'deploy NAME', got %s", cmd.Use)
	}
	if cmd.Short != "Interactively deploy a NIMService custom resource." {
		t.Errorf("unexpected Short description")
	}

	// Verify SilenceUsage is set
	if !cmd.SilenceUsage {
		t.Errorf("expected SilenceUsage to be true")
	}
}

func Test_NewDeployCommand_NoArgs(t *testing.T) {
	streams := genericTestIOStreams()
	cmd := NewDeployCommand(nil, *streams)

	// Test with no arguments - should show help
	cmd.SetArgs([]string{})
	err := cmd.Execute()

	if err != nil {
		t.Fatalf("expected no error for no args, got: %v", err)
	}

	// Since we show help on no args, check that execute succeeded
	// The actual help output goes to the command's output, not our test streams
}

func Test_NewDeployCommand_MultipleArgs(t *testing.T) {
	streams := genericTestIOStreams()
	cmd := NewDeployCommand(nil, *streams)

	// Test with multiple arguments - should show error
	cmd.SetArgs([]string{"arg1", "arg2", "arg3"})
	err := cmd.Execute()

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// The error message is printed but the command doesn't return an error
	// This is by design in the original code
}

// --- Run function tests ---

func Test_Run_NoCaching(t *testing.T) {
	tests := []struct {
		name            string
		userInput       string
		expectedError   bool
		checkServiceCmd func(*testing.T, *cobra.Command)
	}{
		{
			name: "no caching with ngc model",
			userInput: "no\n" +
				"ngc://nvidia/nemo/llama-8b:2.0\n" +
				"standard\n",
			expectedError: false,
			checkServiceCmd: func(t *testing.T, cmd *cobra.Command) {
				// Get the stored args from the command's annotation
				args := cmd.Annotations["setargs"]

				// Check that NIM_MODEL_NAME env var is set
				if !strings.Contains(args, NIM_MODEL_NAME_ENV_VAR) || !strings.Contains(args, "ngc://nvidia/nemo/llama-8b:2.0") {
					t.Errorf("expected NIM_MODEL_NAME env var with model URL in args: %s", args)
				}
			},
		},
		{
			name: "no caching with hf model",
			userInput: "no\n" +
				"hf://meta-llama/Llama-3.2-1B-Instruct\n" +
				"\n", // empty storage class
			expectedError: false,
			checkServiceCmd: func(t *testing.T, cmd *cobra.Command) {
				// Get the stored args from the command's annotation
				args := cmd.Annotations["setargs"]

				// Check that NIM_MODEL_NAME env var is set with HF model
				if !strings.Contains(args, NIM_MODEL_NAME_ENV_VAR) || !strings.Contains(args, "hf://meta-llama/Llama-3.2-1B-Instruct") {
					t.Errorf("expected NIM_MODEL_NAME env var with HF model URL in args: %s", args)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams := genericTestIOStreamsWithInput(tt.userInput)
			options := mockFetchResourceOptions(streams, "test-service", "default")

			serviceCmd := mockCommand()
			cacheCmd := mockCommand()

			errService, errCache := Run(context.Background(), options, nil, serviceCmd, cacheCmd)

			if tt.expectedError {
				if errService == nil && errCache == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if errService != nil {
					t.Errorf("unexpected service error: %v", errService)
				}
				if errCache != nil {
					t.Errorf("unexpected cache error: %v", errCache)
				}
			}

			// Check service command was configured correctly
			if tt.checkServiceCmd != nil {
				tt.checkServiceCmd(t, serviceCmd)
			}

			// Verify cache command was not configured
			if cacheArgs, ok := cacheCmd.Annotations["setargs"]; ok && cacheArgs != "" {
				t.Errorf("cache command should not be configured when caching is disabled, but got: %s", cacheArgs)
			}
		})
	}
}

func Test_Run_WithNGCCaching(t *testing.T) {
	tests := []struct {
		name          string
		userInput     string
		expectedError bool
		checkCommands func(*testing.T, *cobra.Command, *cobra.Command)
	}{
		{
			name: "ngc caching with valid input",
			userInput: "yes\n" +
				"ngc\n" +
				"ngc://nvidia/nemo/llama-8b:2.0\n" +
				"fast-ssd\n",
			expectedError: false,
			checkCommands: func(t *testing.T, serviceCmd, cacheCmd *cobra.Command) {
				// Check cache command
				cacheArgs := cacheCmd.Annotations["setargs"]

				// Should have cache name ending with -cache
				cacheArgsList := strings.Split(cacheArgs, " ")
				if len(cacheArgsList) == 0 || !strings.HasSuffix(cacheArgsList[0], "-cache") {
					t.Errorf("expected cache name to end with -cache, got args: %s", cacheArgs)
				}

				// Should have ngc source
				if !strings.Contains(cacheArgs, "--nim-source=ngc") {
					t.Errorf("expected --nim-source=ngc flag in cache args: %s", cacheArgs)
				}

				// Should have model endpoint
				if !strings.Contains(cacheArgs, "--ngc-model-endpoint=") {
					t.Errorf("expected --ngc-model-endpoint flag in cache args: %s", cacheArgs)
				}

				// Check service command uses nimcache
				serviceArgs := serviceCmd.Annotations["setargs"]
				if !strings.Contains(serviceArgs, "--nimcache-storage-name=") {
					t.Errorf("expected --nimcache-storage-name flag in service args: %s", serviceArgs)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams := genericTestIOStreamsWithInput(tt.userInput)
			options := mockFetchResourceOptions(streams, "test-model", "default")

			serviceCmd := mockCommand()
			cacheCmd := mockCommand()

			errService, errCache := Run(context.Background(), options, nil, serviceCmd, cacheCmd)

			if tt.expectedError {
				if errService == nil && errCache == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if errService != nil {
					t.Errorf("unexpected service error: %v", errService)
				}
				if errCache != nil {
					t.Errorf("unexpected cache error: %v", errCache)
				}
			}

			if tt.checkCommands != nil {
				tt.checkCommands(t, serviceCmd, cacheCmd)
			}
		})
	}
}

func Test_Run_WithHuggingFaceCaching(t *testing.T) {
	tests := []struct {
		name          string
		userInput     string
		expectedError bool
		checkCommands func(*testing.T, *cobra.Command, *cobra.Command)
	}{
		{
			name: "huggingface caching with valid input",
			userInput: "yes\n" +
				"huggingface\n" +
				"https://huggingface.co/api\n" +
				"meta-llama\n" +
				"Llama-3.2-1B-Instruct\n" +
				"gp3\n",
			expectedError: false,
			checkCommands: func(t *testing.T, serviceCmd, cacheCmd *cobra.Command) {
				cacheArgs := cacheCmd.Annotations["setargs"]

				// Check source is huggingface
				if !strings.Contains(cacheArgs, "--nim-source=huggingface") {
					t.Errorf("expected --nim-source=huggingface flag in cache args: %s", cacheArgs)
				}

				// Check alt-endpoint
				if !strings.Contains(cacheArgs, "--alt-endpoint=") {
					t.Errorf("expected --alt-endpoint flag in cache args: %s", cacheArgs)
				}

				// Check model name
				if !strings.Contains(cacheArgs, "--model-name=") || !strings.Contains(cacheArgs, "Llama-3.2-1B-Instruct") {
					t.Errorf("expected --model-name flag with model name in cache args: %s", cacheArgs)
				}
			},
		},
		{
			name: "nemoDataStore caching with valid input",
			userInput: "yes\n" +
				"nemoDataStore\n" +
				"https://nemo.example.com\n" +
				"nemo-models\n" +
				"custom-model-v2\n" +
				"\n", // empty storage class
			expectedError: false,
			checkCommands: func(t *testing.T, serviceCmd, cacheCmd *cobra.Command) {
				cacheArgs := cacheCmd.Annotations["setargs"]

				// Check source is nemoDataStore
				if !strings.Contains(cacheArgs, "--nim-source=nemoDataStore") {
					t.Errorf("expected --nim-source=nemoDataStore flag in cache args: %s", cacheArgs)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams := genericTestIOStreamsWithInput(tt.userInput)
			options := mockFetchResourceOptions(streams, "test-model", "default")

			serviceCmd := mockCommand()
			cacheCmd := mockCommand()

			errService, errCache := Run(context.Background(), options, nil, serviceCmd, cacheCmd)

			if tt.expectedError {
				if errService == nil && errCache == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if errService != nil {
					t.Errorf("unexpected service error: %v", errService)
				}
				if errCache != nil {
					t.Errorf("unexpected cache error: %v", errCache)
				}
			}

			if tt.checkCommands != nil {
				tt.checkCommands(t, serviceCmd, cacheCmd)
			}
		})
	}
}

func Test_Run_InvalidInputs(t *testing.T) {
	tests := []struct {
		name          string
		userInput     string
		expectedError bool
		errorContains string
	}{
		{
			name: "invalid yes/no response",
			userInput: "maybe\n" + // invalid
				"no\n" + // valid retry
				"ngc://nvidia/nemo/llama-8b:2.0\n" +
				"\n",
			expectedError: false,
		},
		{
			name: "invalid image source",
			userInput: "yes\n" +
				"invalid-source\n" + // invalid
				"ngc\n" + // valid retry
				"ngc://nvidia/nemo/llama-8b:2.0\n" +
				"\n",
			expectedError: false,
		},
		{
			name:          "EOF during cache question",
			userInput:     "", // EOF immediately
			expectedError: true,
			errorContains: "failed to read input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams := genericTestIOStreamsWithInput(tt.userInput)
			options := mockFetchResourceOptions(streams, "test", "default")

			serviceCmd := mockCommand()
			cacheCmd := mockCommand()

			errService, errCache := Run(context.Background(), options, nil, serviceCmd, cacheCmd)

			if tt.expectedError {
				if errService == nil && errCache == nil {
					t.Errorf("expected error but got none")
				}
				if tt.errorContains != "" {
					// Check either error contains the expected string
					gotError := false
					if errService != nil && strings.Contains(errService.Error(), tt.errorContains) {
						gotError = true
					}
					if errCache != nil && strings.Contains(errCache.Error(), tt.errorContains) {
						gotError = true
					}
					if !gotError {
						t.Errorf("expected error containing %q, got service: %v, cache: %v", tt.errorContains, errService, errCache)
					}
				}
			} else {
				if errService != nil {
					t.Errorf("unexpected service error: %v", errService)
				}
				if errCache != nil {
					t.Errorf("unexpected cache error: %v", errCache)
				}
			}
		})
	}
}

func Test_Run_PVCConfiguration(t *testing.T) {
	tests := []struct {
		name            string
		storageClass    string
		expectedPVCArgs []string
	}{
		{
			name:         "with custom storage class",
			storageClass: "fast-ssd",
			expectedPVCArgs: []string{
				"--pvc-create=true",
				"--pvc-size=20Gi",
				"--pvc-volume-access-mode=ReadWriteOnce",
				"--pvc-storage-class=fast-ssd",
			},
		},
		{
			name:         "with default storage class",
			storageClass: "", // empty means use default
			expectedPVCArgs: []string{
				"--pvc-create=true",
				"--pvc-size=20Gi",
				"--pvc-volume-access-mode=ReadWriteOnce",
				// no storage class flag
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userInput := fmt.Sprintf("no\nngc://nvidia/nemo/llama-8b:2.0\n%s\n", tt.storageClass)
			streams := genericTestIOStreamsWithInput(userInput)
			options := mockFetchResourceOptions(streams, "test", "default")

			serviceCmd := mockCommand()
			cacheCmd := mockCommand()

			_, _ = Run(context.Background(), options, nil, serviceCmd, cacheCmd)

			// Check PVC args in service command
			serviceArgs := serviceCmd.Annotations["setargs"]
			for _, expectedArg := range tt.expectedPVCArgs {
				if !strings.Contains(serviceArgs, expectedArg) {
					t.Errorf("expected PVC arg %q not found in service args: %s", expectedArg, serviceArgs)
				}
			}
		})
	}
}

// --- Helper functions ---

func genericTestIOStreams() *genericclioptions.IOStreams {
	return &genericclioptions.IOStreams{
		In:     &bytes.Buffer{},
		Out:    &bytes.Buffer{},
		ErrOut: &bytes.Buffer{},
	}
}

func genericTestIOStreamsWithInput(input string) *genericclioptions.IOStreams {
	return &genericclioptions.IOStreams{
		In:     strings.NewReader(input),
		Out:    &bytes.Buffer{},
		ErrOut: &bytes.Buffer{},
	}
}

func mockFetchResourceOptions(streams *genericclioptions.IOStreams, name, namespace string) *util.FetchResourceOptions {
	return &util.FetchResourceOptions{
		IoStreams:    streams,
		ResourceName: name,
		Namespace:    namespace,
	}
}

func mockCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "mock",
		RunE: func(cmd *cobra.Command, args []string) error {
			// When DisableFlagParsing is true, args contains all arguments
			if cmd.Annotations == nil {
				cmd.Annotations = make(map[string]string)
			}
			cmd.Annotations["setargs"] = strings.Join(args, " ")

			return nil
		},
	}

	// Initialize annotations
	cmd.Annotations = make(map[string]string)

	// Initialize flags
	cmd.Flags()
	cmd.DisableFlagParsing = true

	return cmd
}
