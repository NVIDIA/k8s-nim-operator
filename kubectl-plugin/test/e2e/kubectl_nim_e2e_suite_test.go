package e2e

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestKubectlNimCommand(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kubectl NIM e2e Test Suite")
}
