/*
Copyright 2025.

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

package v1alpha1

import (
	"testing"

	"k8s.io/utils/ptr"
)

func TestDRADeviceAttributeSelectorGetCELExpression(t *testing.T) {
	tests := []struct {
		name       string
		selector   *DRADeviceAttributeSelector
		driverName string
		wantExpr   string
		wantErr    string
	}{
		{
			name: "should return cel expression if selector value is set",
			selector: &DRADeviceAttributeSelector{
				Key: "memory",
				Op:  DRADeviceAttributeSelectorOpGreaterThanOrEqual,
				Value: &DRADeviceAttributeSelectorValue{
					IntValue: ptr.To(int32(8)),
				},
			},
			driverName: "gpu.nvidia.com",
			wantExpr:   `device.attributes["gpu.nvidia.com"].memory >= 8`,
		},
		{
			name: "should return error if selector value is missing",
			selector: &DRADeviceAttributeSelector{
				Key:   "memory",
				Op:    DRADeviceAttributeSelectorOpEqual,
				Value: nil,
			},
			driverName: "gpu.nvidia.com",
			wantErr:    "attribute selector value is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotExpr, err := tt.selector.GetCELExpression(tt.driverName)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("GetCELExpression() error = nil, want %q", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("GetCELExpression() error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("GetCELExpression() error = %v", err)
			}
			if gotExpr != tt.wantExpr {
				t.Fatalf("GetCELExpression() = %q, want %q", gotExpr, tt.wantExpr)
			}
		})
	}
}
