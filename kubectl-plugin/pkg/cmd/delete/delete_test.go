package delete

import (
	"bytes"
	"strings"
	"testing"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// There are no internal helper methods to test. Hence, testing the command itself.

func Test_NewDeleteCommand_Wiring(t *testing.T) {
	streams, _, _, _ := genericTestIOStreams()
	cmd := NewDeleteCommand(nil, streams)
	if !strings.HasPrefix(cmd.Use, "delete ") {
		t.Fatalf("Use = %q", cmd.Use)
	}
	if len(cmd.Aliases) == 0 || cmd.Aliases[0] != "remove" {
		t.Fatalf("aliases = %v", cmd.Aliases)
	}
}

// helpers.
func genericTestIOStreams() (s genericclioptions.IOStreams, in *bytes.Buffer, out *bytes.Buffer, errOut *bytes.Buffer) {
	in = &bytes.Buffer{}
	out = &bytes.Buffer{}
	errOut = &bytes.Buffer{}
	return genericclioptions.IOStreams{In: in, Out: out, ErrOut: errOut}, in, out, errOut
}
