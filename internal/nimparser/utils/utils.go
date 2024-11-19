/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"strings"

	nimparser "github.com/NVIDIA/k8s-nim-operator/internal/nimparser"
	nimparserv1 "github.com/NVIDIA/k8s-nim-operator/internal/nimparser/v1"
	nimparserv2 "github.com/NVIDIA/k8s-nim-operator/internal/nimparser/v2"
	"gopkg.in/yaml.v2"
)

// GetNIMParser unmarshals the provided byte slice into a NIMSchemaManifest struct and returns
// the corresponding NIMParserInterface implementation based on the schema version.
// If the unmarshalling fails or the schema version is not recognized, it returns a NIMParser from v1.
//
// Parameters:
// - data: A byte slice containing the YAML data to be unmarshaled.
//
// Returns:
// - A NIMParserInterface implementation based on the schema version.
func GetNIMParser(data []byte) nimparser.NIMParserInterface {
	var config nimparser.NIMSchemaManifest
	err := yaml.Unmarshal(data, &config)
	if err != nil {
		return nimparserv1.NIMParser{}
	} else {
		schemaVersion := strings.TrimSpace(config.SchemaVersion)
		if schemaVersion == "2.0" {
			return nimparserv2.NIMParser{}
		}
	}
	return nimparserv1.NIMParser{}
}
