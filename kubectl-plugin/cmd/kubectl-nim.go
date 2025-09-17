package main

import (
	"os"

	flag "github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"k8s-nim-operator-cli/pkg/cmd"
)

func main() {
	flags := flag.NewFlagSet("kubectl-nim", flag.ExitOnError)
	flag.CommandLine = flags
	ioStreams := genericiooptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}

	root := cmd.NewNIMCommand(ioStreams)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
