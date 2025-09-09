package config

import "github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"

var (
	EnableWebhooks     bool
	OperatorNamePrefix string
	OperatorNamespace  string
	OrchestratorType   k8sutil.OrchestratorType
)
