/**
# Copyright (c) NVIDIA CORPORATION.  All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestGetStringHash(t *testing.T) {
	type test struct {
		input    string
		expected string
	}

	testcases := []test{
		{
			input:    "2269c984-db9a-4b0e-9fd5-86df0ad269f7",
			expected: "7c6d7bd86b",
		},
		{
			input:    "2269c984-db9a-4b0e-9fd5-86df0ad269f7-5.15.0-1041-azure",
			expected: "79d6bd954f",
		},
		{
			input:    "2269c984-db9a-4b0e-9fd5-86df0ad269f7-rhcos4.14-414.92.202309282257",
			expected: "646cdfdb96",
		},
		{
			input:    "rhcos4.14-414.92.202309282257",
			expected: "5bbdb464cb",
		},
		{
			input:    "nvidia-gpu-driver-2269c984-db9a-4b0e-9fd5-86df0ad269f7-rhcos4.14-414.92.202309282257",
			expected: "7bf6859b6d",
		},
		{
			input:    "nvidia-vgpu-driver-2269c984-db9a-4b0e-9fd5-868df0ad269f7-rhcos4.14-414.92.202309282257",
			expected: "7469f59898",
		},
	}

	for _, tc := range testcases {
		actual := GetStringHash(tc.input)
		assert.Equal(t, tc.expected, actual)
	}
}

// TestGetFilesWithSuffix tests the GetFilesWithSuffix function.
func TestGetFilesWithSuffix(t *testing.T) {
	dir := t.TempDir()

	// Create test files
	testFiles := []string{"file1.txt", "file2.yaml", "file3.json"}
	for _, file := range testFiles {
		f, err := os.Create(filepath.Join(dir, file))
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		f.Close()
	}

	files, err := GetFilesWithSuffix(dir, ".txt", ".yaml")
	if err != nil {
		t.Fatalf("GetFilesWithSuffix returned error: %v", err)
	}

	expectedFiles := map[string]bool{
		filepath.Join(dir, "file1.txt"):  true,
		filepath.Join(dir, "file2.yaml"): true,
	}

	if len(files) != len(expectedFiles) {
		t.Fatalf("Expected %d files, but got %d", len(expectedFiles), len(files))
	}

	for _, file := range files {
		if !expectedFiles[file] {
			t.Errorf("Unexpected file found: %s", file)
		}
	}
}

// TestBoolPtr tests the BoolPtr function.
func TestBoolPtr(t *testing.T) {
	val := true
	ptr := BoolPtr(val)

	if ptr == nil || *ptr != val {
		t.Errorf("Expected pointer to %v, but got %v", val, ptr)
	}
}

// TestMergeEnvVars tests the MergeEnvVars function.
func TestMergeEnvVars(t *testing.T) {
	env1 := []corev1.EnvVar{
		{Name: "VAR1", Value: "value1"},
		{Name: "VAR2", Value: "value2"},
	}
	env2 := []corev1.EnvVar{
		{Name: "VAR2", Value: "new_value2"},
		{Name: "VAR3", Value: "value3"},
	}

	expectedEnv := []corev1.EnvVar{
		{Name: "VAR1", Value: "value1"},
		{Name: "VAR2", Value: "new_value2"},
		{Name: "VAR3", Value: "value3"},
	}

	mergedEnv := MergeEnvVars(env1, env2)
	if len(mergedEnv) != len(expectedEnv) {
		t.Fatalf("Expected %d env vars, but got %d", len(expectedEnv), len(mergedEnv))
	}

	envMap := make(map[string]string)
	for _, env := range mergedEnv {
		envMap[env.Name] = env.Value
	}

	for _, env := range expectedEnv {
		if val, exists := envMap[env.Name]; !exists || val != env.Value {
			t.Errorf("Expected env var %s to have value %s, but got %s", env.Name, env.Value, val)
		}
	}
}

// TestGetResourceHash tests the GetResourceHash function.
func TestGetResourceHash(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	})
	obj.SetName("test-deployment")
	obj.SetNamespace("default")

	hash := GetResourceHash(obj)
	if hash == "" {
		t.Errorf("Expected non-empty hash for object")
	}
}

// TestIsSpecChanged tests the IsSpecChanged function.
func TestIsSpecChanged(t *testing.T) {
	tests := []struct {
		name     string
		current  client.Object
		desired  client.Object
		expected bool
	}{
		{
			name: "no change in hash with deployment spec and env variables",
			current: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "nim-deployment",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"replicas": 2,
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"app": "nim",
							},
						},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"app": "nim",
								},
							},
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "nim",
										"image": "nim:v0.1.0",
										"ports": []interface{}{
											map[string]interface{}{
												"containerPort": 80,
											},
										},
										"env": []interface{}{
											map[string]interface{}{"name": "ENV_VAR1", "value": "value1"},
											map[string]interface{}{"name": "ENV_VAR2", "value": "value2"},
										},
									},
								},
							},
						},
					},
				},
			},
			desired: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "nim-deployment",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"replicas": 2,
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"app": "nim",
							},
						},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"app": "nim",
								},
							},
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "nim",
										"image": "nim:v0.1.0",
										"ports": []interface{}{
											map[string]interface{}{
												"containerPort": 80,
											},
										},
										"env": []interface{}{
											map[string]interface{}{"name": "ENV_VAR1", "value": "value1"},
											map[string]interface{}{"name": "ENV_VAR2", "value": "value2"},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "change in hash with change in value of elements",
			current: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "nim-deployment",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"replicas": 2,
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"app": "nim",
							},
						},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"app": "nim",
								},
							},
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "nim",
										"image": "nim:v0.1.0",
										"ports": []interface{}{
											map[string]interface{}{
												"containerPort": 80,
											},
										},
										"env": []interface{}{
											map[string]interface{}{"name": "ENV_VAR1", "value": "value2"},
											map[string]interface{}{"name": "ENV_VAR2", "value": "value1"},
										},
									},
								},
							},
						},
					},
				},
			},
			desired: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "nim-deployment",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"replicas": 3,
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"app": "nim",
							},
						},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"app": "nim",
								},
							},
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "nim",
										"image": "nim:v0.1.0",
										"ports": []interface{}{
											map[string]interface{}{
												"containerPort": 80,
											},
										},
										"env": []interface{}{
											map[string]interface{}{"name": "ENV_VAR1", "value": "asdf"},
											map[string]interface{}{"name": "ENV_VAR2", "value": "jljl"},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: true,
		},
	}

	for idx, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.current.SetAnnotations(map[string]string{
				NvidiaAnnotationHashKey: GetResourceHash(tt.current),
			})
			if got := IsSpecChanged(tt.current, tt.desired); got != tt.expected {
				t.Errorf("[Testcase %d] IsSpecChanged() = %v, want %v, hash current %s vs desired %s", (idx + 1), got, tt.expected, GetResourceHash(tt.current), GetResourceHash(tt.desired))
			}
		})
	}
}
