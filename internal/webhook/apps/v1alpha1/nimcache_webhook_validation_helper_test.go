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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

// TestValidateNGCSource covers the edge-cases and success path of validateNGCSource.
func TestValidateNGCSource(t *testing.T) {
	fldPath := field.NewPath("spec").Child("source").Child("ngc")

	tests := []struct {
		name         string
		src          *appsv1alpha1.NGCSource
		wantErrs     int
		wantWarnings int
	}{
		{
			name:         "nil source",
			src:          nil,
			wantErrs:     0,
			wantWarnings: 0,
		},
		{
			name: "missing secrets and puller",
			src: &appsv1alpha1.NGCSource{
				Model: &appsv1alpha1.ModelSpec{},
			},
			wantErrs:     2,
			wantWarnings: 0,
		},
		{
			name: "ngcsource.model.profiles contains 'all' and more values",
			src: &appsv1alpha1.NGCSource{
				AuthSecret:  "sec",
				ModelPuller: "img",
				Model: &appsv1alpha1.ModelSpec{
					Profiles: []string{"all", "foo"},
				},
			},
			wantErrs:     1,
			wantWarnings: 0,
		},
		{
			name: "profiles with forbidden additional fields",
			src: &appsv1alpha1.NGCSource{
				AuthSecret:  "sec",
				ModelPuller: "img",
				Model: &appsv1alpha1.ModelSpec{
					Profiles:  []string{"foo"},
					Precision: "fp16",
					Engine:    "test",
				},
			},
			wantErrs:     2,
			wantWarnings: 0,
		},
		{
			name: "invalid qos profile",
			src: &appsv1alpha1.NGCSource{
				AuthSecret:  "sec",
				ModelPuller: "img",
				Model: &appsv1alpha1.ModelSpec{
					QoSProfile: "fast",
				},
			},
			wantErrs:     1,
			wantWarnings: 0,
		},
		{
			name: "valid source",
			src: &appsv1alpha1.NGCSource{
				AuthSecret:  "sec",
				ModelPuller: "img",
				Model: &appsv1alpha1.ModelSpec{
					Precision:  "fp16",
					QoSProfile: "throughput",
				},
			},
			wantErrs:     0,
			wantWarnings: 0,
		},
		{
			name: "valid source #2",
			src: &appsv1alpha1.NGCSource{
				AuthSecret:  "sec",
				ModelPuller: "img",
				Model: &appsv1alpha1.ModelSpec{
					Profiles: []string{"foo"},
				},
			},
			wantErrs:     0,
			wantWarnings: 0,
		},
		{
			name: "valid source #3",
			src: &appsv1alpha1.NGCSource{
				AuthSecret:  "sec",
				ModelPuller: "img",
				Model: &appsv1alpha1.ModelSpec{
					QoSProfile: "latency",
				},
			},
			wantErrs:     0,
			wantWarnings: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, errs := validateNGCSource(tc.src, fldPath)
			gotErrs := len(errs)
			gotWarnings := len(w)
			if gotErrs != tc.wantErrs || gotWarnings != tc.wantWarnings {
				t.Logf("Validation errors:")
				t.Fatalf("got %d errs, %d warnings, want %d errs, %d warnings", gotErrs, gotWarnings, tc.wantErrs, tc.wantWarnings)
			}
		})
	}
}

// TestValidateNIMCacheStorageConfiguration tests validateNIMCacheStorageConfiguration.
func TestValidateNIMCacheStorageConfiguration(t *testing.T) {
	fldPath := field.NewPath("spec").Child("storage")

	falseVal := false
	trueVal := true

	tests := []struct {
		name         string
		storage      *appsv1alpha1.NIMCacheStorage
		wantErrs     int
		wantWarnings int
	}{
		{
			name:         "empty storage",
			storage:      &appsv1alpha1.NIMCacheStorage{},
			wantErrs:     1,
			wantWarnings: 0,
		},
		{
			name: "create false name empty",
			storage: &appsv1alpha1.NIMCacheStorage{
				PVC: appsv1alpha1.PersistentVolumeClaim{
					Create: &falseVal,
				},
			},
			wantErrs:     1,
			wantWarnings: 0,
		},
		{
			name: "invalid volume access mode",
			storage: &appsv1alpha1.NIMCacheStorage{
				PVC: appsv1alpha1.PersistentVolumeClaim{
					Create:           &trueVal,
					VolumeAccessMode: "RandomMode",
				},
			},
			wantErrs:     1,
			wantWarnings: 0,
		},
		{
			name: "valid storage",
			storage: &appsv1alpha1.NIMCacheStorage{
				PVC: appsv1alpha1.PersistentVolumeClaim{
					Create:           &trueVal,
					StorageClass:     "standard",
					Size:             "10Gi",
					VolumeAccessMode: corev1.ReadWriteOnce,
				},
			},
			wantErrs:     0,
			wantWarnings: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, errs := validateNIMCacheStorageConfiguration(tc.storage, fldPath)
			gotErrs := len(errs)
			gotWarnings := len(w)
			if gotErrs != tc.wantErrs || gotWarnings != tc.wantWarnings {
				t.Logf("Validation errors:")
				t.Fatalf("got %d errs, %d warnings, want %d errs, %d warnings", gotErrs, gotWarnings, tc.wantErrs, tc.wantWarnings)
			}
		})
	}
}

// TestValidateProxyConfiguration tests validateProxyConfiguration.
func TestValidateProxyConfiguration(t *testing.T) {
	fldPath := field.NewPath("spec").Child("proxy")

	tests := []struct {
		name         string
		proxy        *appsv1alpha1.ProxySpec
		wantErrs     int
		wantWarnings int
	}{
		{
			name:         "nil proxy",
			proxy:        nil,
			wantErrs:     0,
			wantWarnings: 0,
		},
		{
			name:         "empty proxy",
			proxy:        &appsv1alpha1.ProxySpec{},
			wantErrs:     0,
			wantWarnings: 0,
		},
		{
			name: "invalid noProxy token",
			proxy: &appsv1alpha1.ProxySpec{
				NoProxy: "invalid_token$",
			},
			wantErrs:     1,
			wantWarnings: 0,
		},
		{
			name: "missing scheme http proxy",
			proxy: &appsv1alpha1.ProxySpec{
				HttpProxy: "proxy:8080",
			},
			wantErrs:     1,
			wantWarnings: 0,
		},
		{
			name: "missing scheme https proxy",
			proxy: &appsv1alpha1.ProxySpec{
				HttpsProxy: "proxy:8443",
			},
			wantErrs:     1,
			wantWarnings: 0,
		},
		{
			name: "valid proxy spec",
			proxy: &appsv1alpha1.ProxySpec{
				HttpProxy:  "http://proxy:8080",
				HttpsProxy: "https://proxy:8443",
				NoProxy:    "localhost,.example.com,10.1.2.3",
			},
			wantErrs:     0,
			wantWarnings: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, errs := validateProxyConfiguration(tc.proxy, fldPath)
			gotErrs := len(errs)
			gotWarnings := len(w)
			if gotErrs != tc.wantErrs || gotWarnings != tc.wantWarnings {
				t.Logf("Validation errors:")
				t.Fatalf("got %d errs, %d warnings, want %d errs, %d warnings", gotErrs, gotWarnings, tc.wantErrs, tc.wantWarnings)
			}
		})
	}
}

// TestValidateNIMSourceConfiguration ensures validateNIMSourceConfiguration delegates correctly.
func TestValidateNIMSourceConfiguration(t *testing.T) {
	fldPath := field.NewPath("spec").Child("source")

	tests := []struct {
		name         string
		source       *appsv1alpha1.NIMSource
		wantErrs     int
		wantWarnings int
	}{
		{
			name:         "empty NIMSource (no NGC)",
			source:       &appsv1alpha1.NIMSource{},
			wantErrs:     0,
			wantWarnings: 0,
		},
		{
			name: "NGC errors propagate",
			source: &appsv1alpha1.NIMSource{
				NGC: &appsv1alpha1.NGCSource{
					Model: &appsv1alpha1.ModelSpec{
						Profiles:   []string{"all", "wrong"},
						Precision:  "fp16",
						QoSProfile: "throughput",
					},
				},
			},
			wantErrs:     5, // missing authSecret & modelPuller, profiles should only have one entry. If profiles is defined, all other model fields must be empty
			wantWarnings: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, errs := validateNIMSourceConfiguration(tc.source, fldPath)
			gotErrs := len(errs)
			gotWarnings := len(w)
			if gotErrs != tc.wantErrs || gotWarnings != tc.wantWarnings {
				t.Fatalf("got %d errs, %d warnings, want %d errs, %d warnings", gotErrs, gotWarnings, tc.wantErrs, tc.wantWarnings)
			}
		})
	}
}

// TestValidateImmutableNIMCacheSpec checks immutability enforcement on NIMCache.Spec.
func TestValidateImmutableNIMCacheSpec(t *testing.T) {
	fldPath := field.NewPath("nimcache")

	// helper to build a simple NIMCache with specified PVC size
	buildCache := func(size string) *appsv1alpha1.NIMCache {
		return &appsv1alpha1.NIMCache{
			Spec: appsv1alpha1.NIMCacheSpec{
				Storage: appsv1alpha1.NIMCacheStorage{
					PVC: appsv1alpha1.PersistentVolumeClaim{
						Size: size,
					},
				},
			},
		}
	}

	tests := []struct {
		name         string
		oldObj       *appsv1alpha1.NIMCache
		newObj       *appsv1alpha1.NIMCache
		wantErrs     int
		wantWarnings int
	}{
		{
			name:         "spec unchanged",
			oldObj:       buildCache("10Gi"),
			newObj:       buildCache("10Gi"),
			wantErrs:     0,
			wantWarnings: 0,
		},
		{
			name:         "spec changed (PVC size)",
			oldObj:       buildCache("10Gi"),
			newObj:       buildCache("20Gi"),
			wantErrs:     1,
			wantWarnings: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, errs := validateImmutableNIMCacheSpec(tc.oldObj, tc.newObj, fldPath)
			gotErrs := len(errs)
			gotWarnings := len(w)
			if gotErrs != tc.wantErrs || gotWarnings != tc.wantWarnings {
				t.Logf("Validation errors:")
				t.Fatalf("got %d errs, %d warnings, want %d errs, %d warnings", gotErrs, gotWarnings, tc.wantErrs, tc.wantWarnings)
			}
		})
	}
}
