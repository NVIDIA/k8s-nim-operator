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
	"testing"

	nimparserv1 "github.com/NVIDIA/k8s-nim-operator/internal/nimparser/v1"
	nimparserv2 "github.com/NVIDIA/k8s-nim-operator/internal/nimparser/v2"
)

func TestGetNIMParserUsesMajorVersion(t *testing.T) {
	cases := []struct {
		version string
		v2      bool
	}{
		{version: "2.0.0", v2: true},
		{version: "2", v2: true},
		{version: "1.0.0", v2: false},
		{version: "1.2.0", v2: false},
		{version: "12.0.0", v2: false},
		{version: "1.0.2", v2: false},
	}

	for _, tc := range cases {
		parser := GetNIMParser([]byte("schema_version: " + tc.version + "\n"))
		_, isV2 := parser.(nimparserv2.NIMParser)
		_, isV1 := parser.(nimparserv1.NIMParser)
		if tc.v2 && !isV2 {
			t.Errorf("schema %s: got %T, want v2", tc.version, parser)
		}
		if !tc.v2 && !isV1 {
			t.Errorf("schema %s: got %T, want v1", tc.version, parser)
		}
	}
}
