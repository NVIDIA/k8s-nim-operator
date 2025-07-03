package nimsource

import "fmt"

type HFInterface interface {
	GetModelName() *string
	GetDatasetName() *string
	GetAuthSecret() string
	GetModelPuller() string
	GetPullSecret() string
	GetEndpoint() string
	GetNamespace() string
	GetRevision() string
}

func DownloadToCacheCommand(src HFInterface, outputPath string) []string {
	var command []string
	if src.GetModelName() != nil { // nolint:gocritic
		hfRepo := fmt.Sprintf("%s/%s", src.GetNamespace(), *src.GetModelName())
		command = []string{"huggingface-cli", "download", hfRepo, "--local-dir", outputPath, "--repo-type", "model"}
	} else if src.GetDatasetName() != nil {
		hfRepo := fmt.Sprintf("%s/%s", src.GetNamespace(), *src.GetDatasetName())
		command = []string{"huggingface-cli", "download", hfRepo, "--local-dir", outputPath, "--repo-type", "dataset"}
	}
	if src.GetRevision() != "" {
		command = append(command, "--revision", src.GetRevision())
	}
	return command
}
