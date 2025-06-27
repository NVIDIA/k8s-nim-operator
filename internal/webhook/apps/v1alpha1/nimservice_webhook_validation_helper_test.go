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
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

// TestValidateImageConfiguration covers required field checks on Image.
func TestValidateImageConfiguration(t *testing.T) {
	fldPath := field.NewPath("spec").Child("image")

	tests := []struct {
		name     string
		image    *appsv1alpha1.Image
		wantErrs int
	}{
		{
			name:     "valid image",
			image:    &appsv1alpha1.Image{Repository: "repo", Tag: "latest"},
			wantErrs: 0,
		},
		{
			name:     "missing repository",
			image:    &appsv1alpha1.Image{Tag: "v1"},
			wantErrs: 1,
		},
		{
			name:     "missing tag",
			image:    &appsv1alpha1.Image{Repository: "repo"},
			wantErrs: 1,
		},
		{
			name:     "missing both",
			image:    &appsv1alpha1.Image{},
			wantErrs: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateImageConfiguration(tc.image, fldPath)
			if got := len(errs); got != tc.wantErrs {
				t.Logf("Validation errors:")
				for i, err := range errs {
					t.Logf("  %d: %s", i+1, err.Error())
				}
				t.Fatalf("got %d errs, want %d", got, tc.wantErrs)
			}
		})
	}
}

// Utility to build a minimal valid NIMService object for storage tests.
func baseNIMService() *appsv1alpha1.NIMService {
	return &appsv1alpha1.NIMService{
		Spec: appsv1alpha1.NIMServiceSpec{
			Image:      appsv1alpha1.Image{Repository: "repo", Tag: "latest"},
			AuthSecret: "dummy-secret",
		},
	}
}

// TestValidateServiceStorageConfiguration covers storage validation rules.
func TestValidateServiceStorageConfiguration(t *testing.T) {
	fldPath := field.NewPath("spec").Child("storage")

	trueVal := true
	falseVal := false
	sizeNeg := resource.MustParse("0")

	tests := []struct {
		name     string
		modify   func(*appsv1alpha1.NIMService)
		wantErrs int
	}{
		{
			name:     "missing both nimCache and pvc",
			modify:   func(ns *appsv1alpha1.NIMService) {},
			wantErrs: 1,
		},
		{
			name: "both nimCache and pvc defined",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.Storage.NIMCache = appsv1alpha1.NIMCacheVolSpec{Name: "cache", Profile: "p"}
				ns.Spec.Storage.PVC = appsv1alpha1.PersistentVolumeClaim{Name: "pvc"}
			},
			wantErrs: 1,
		},
		{
			name: "nimCache missing name",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.Storage.NIMCache = appsv1alpha1.NIMCacheVolSpec{Profile: "p"}
			},
			wantErrs: 1,
		},
		{
			name: "pvc create false name empty",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.Storage.PVC = appsv1alpha1.PersistentVolumeClaim{
					Create: &falseVal,
				}
			},
			wantErrs: 1,
		},
		{
			name: "pvc invalid volumeaccessmode",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.Storage.PVC = appsv1alpha1.PersistentVolumeClaim{
					Create:           &trueVal,
					VolumeAccessMode: "BadMode",
				}
			},
			wantErrs: 1,
		},
		{
			name: "shared memory size <=0",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.Storage.PVC = appsv1alpha1.PersistentVolumeClaim{Name: "pvc"}
				ns.Spec.Storage.SharedMemorySizeLimit = &sizeNeg
			},
			wantErrs: 1,
		},
		{
			name: "valid nimCache",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.Storage.NIMCache = appsv1alpha1.NIMCacheVolSpec{Name: "cache", Profile: "default"}
			},
			wantErrs: 0,
		},
		{
			name: "valid pvc",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.Storage.PVC = appsv1alpha1.PersistentVolumeClaim{
					Create:           &trueVal,
					Name:             "",
					StorageClass:     "standard",
					Size:             "10Gi",
					VolumeAccessMode: corev1.ReadWriteOnce,
				}
			},
			wantErrs: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ns := baseNIMService()
			tc.modify(ns)
			errs := validateServiceStorageConfiguration(&ns.Spec.Storage, fldPath)
			if got := len(errs); got != tc.wantErrs {
				t.Logf("Validation errors:")
				for i, err := range errs {
					t.Logf("  %d: %s", i+1, err.Error())
				}
				t.Fatalf("got %d errs, want %d", got, tc.wantErrs)
			}
		})
	}
}

// TestValidateDRAResourcesConfiguration covers DRA resource validation rules and
// Kubernetes-version compatibility in a single table-driven test.
func TestValidateDRAResourcesConfiguration(t *testing.T) {
	fld := field.NewPath("spec")

	cases := []struct {
		name       string
		modify     func(*appsv1alpha1.NIMService)
		k8sVersion string
		wantErrs   int
	}{
		{
			name:       "no dra resources",
			modify:     func(ns *appsv1alpha1.NIMService) {},
			k8sVersion: "v1.34.0",
			wantErrs:   0,
		},
		{
			name: "unsupported k8s version",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ResourceClaimTemplateName: ptr.To("tmpl1"),
				}}
			},
			k8sVersion: "v1.32.0", // below MinSupportedClusterVersionForDRA
			wantErrs:   1,
		},
		{
			name: "both name and template provided",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ResourceClaimName:         ptr.To("claim1"),
					ResourceClaimTemplateName: ptr.To("tmpl1"),
				}}
			},
			k8sVersion: "v1.34.0",
			wantErrs:   1,
		},
		{
			name: "neither name nor template provided",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{}}
			},
			k8sVersion: "v1.34.0",
			wantErrs:   1,
		},
		{
			name: "resourceClaimName with replicas>1",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.Replicas = 2
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ResourceClaimName: ptr.To("claim1"),
				}}
			},
			k8sVersion: "v1.34.0",
			wantErrs:   1,
		},
		{
			name: "resourceClaimName with autoscaling enabled",
			modify: func(ns *appsv1alpha1.NIMService) {
				enabled := true
				ns.Spec.Scale.Enabled = &enabled
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ResourceClaimName: ptr.To("claim1"),
				}}
			},
			k8sVersion: "v1.34.0",
			wantErrs:   1,
		},
		{
			name: "duplicate resourceClaimNames",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{
					{ResourceClaimName: ptr.To("dup")},
					{ResourceClaimName: ptr.To("dup")},
				}
			},
			k8sVersion: "v1.34.0",
			wantErrs:   1,
		},
		{
			name: "valid template",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ResourceClaimTemplateName: ptr.To("tmpl1"),
				}}
			},
			k8sVersion: "v1.34.0",
			wantErrs:   0,
		},
		{
			name: "valid multiple templates",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ResourceClaimTemplateName: ptr.To("tmpl1"),
				}, {
					ResourceClaimTemplateName: ptr.To("tmpl2"),
				}}
			},
			k8sVersion: "v1.34.0",
			wantErrs:   0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ns := baseNIMService()
			tc.modify(ns)
			errs := validateDRAResourcesConfiguration(&ns.Spec, fld, tc.k8sVersion)
			if got := len(errs); got != tc.wantErrs {
				t.Logf("Validation errors:")
				for i, err := range errs {
					t.Logf("  %d: %s", i+1, err.Error())
				}
				t.Fatalf("got %d errs, want %d", got, tc.wantErrs)
			}
		})
	}
}

// TestValidateAuthSecret verifies required secret enforcement using table-driven cases.
func TestValidateAuthSecret(t *testing.T) {
	fld := field.NewPath("spec").Child("authSecret")
	cases := []struct {
		name     string
		value    string
		wantErrs int
	}{
		{"non-empty", "secret", 0},
		{"empty", "", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateAuthSecret(&tc.value, fld)
			if got := len(errs); got != tc.wantErrs {
				t.Logf("Validation errors:")
				for i, err := range errs {
					t.Logf("  %d: %s", i+1, err.Error())
				}
				t.Fatalf("got %d errs, want %d", got, tc.wantErrs)
			}
		})
	}
}

// TestValidateExposeIngressConfiguration table-driven.
func TestValidateExposeIngressConfiguration(t *testing.T) {
	fld := field.NewPath("spec").Child("expose").Child("ingress")
	enabled := true
	class := "nginx"

	cases := []struct {
		name     string
		modify   func(*appsv1alpha1.NIMService)
		wantErrs int
	}{
		{
			name:     "ingress disabled",
			modify:   func(ns *appsv1alpha1.NIMService) {},
			wantErrs: 0,
		},
		{
			name: "enabled empty spec",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.Expose.Ingress.Enabled = &enabled
			},
			wantErrs: 1,
		},
		{
			name: "enabled with spec",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.Expose.Ingress.Enabled = &enabled
				ns.Spec.Expose.Ingress.Spec.IngressClassName = &class
			},
			wantErrs: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ns := baseNIMService()
			tc.modify(ns)
			errs := validateExposeConfiguration(&ns.Spec.Expose, fld)
			if got := len(errs); got != tc.wantErrs {
				t.Logf("Validation errors:")
				for i, err := range errs {
					t.Logf("  %d: %s", i+1, err.Error())
				}
				t.Fatalf("got %d errs, want %d", got, tc.wantErrs)
			}
		})
	}
}

// TestValidateMetricsConfiguration table-driven.
func TestValidateMetricsConfiguration(t *testing.T) {
	fld := field.NewPath("spec").Child("metrics")
	enabled := true

	cases := []struct {
		name     string
		modify   func(*appsv1alpha1.NIMService)
		wantErrs int
	}{
		{
			name:     "metrics disabled",
			modify:   func(ns *appsv1alpha1.NIMService) {},
			wantErrs: 0,
		},
		{
			name: "enabled empty monitor",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.Metrics.Enabled = &enabled
			},
			wantErrs: 1,
		},
		{
			name: "enabled valid",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.Metrics.Enabled = &enabled
				ns.Spec.Metrics.ServiceMonitor.Interval = "30s"
			},
			wantErrs: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ns := baseNIMService()
			tc.modify(ns)
			errs := validateMetricsConfiguration(&ns.Spec.Metrics, fld)
			if got := len(errs); got != tc.wantErrs {
				t.Logf("Validation errors:")
				for i, err := range errs {
					t.Logf("  %d: %s", i+1, err.Error())
				}
				t.Fatalf("got %d errs, want %d", got, tc.wantErrs)
			}
		})
	}
}

// TestValidateScaleConfiguration table-driven.
func TestValidateScaleConfiguration(t *testing.T) {
	fld := field.NewPath("spec").Child("scale")
	enabled := true

	cases := []struct {
		name     string
		modify   func(*appsv1alpha1.NIMService)
		wantErrs int
	}{
		{"autoscaling disabled", func(ns *appsv1alpha1.NIMService) {}, 0},
		{"enabled empty HPA", func(ns *appsv1alpha1.NIMService) { ns.Spec.Scale.Enabled = &enabled }, 1},
		{"enabled valid HPA", func(ns *appsv1alpha1.NIMService) { ns.Spec.Scale.Enabled = &enabled; ns.Spec.Scale.HPA.MaxReplicas = 3 }, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ns := baseNIMService()
			tc.modify(ns)
			errs := validateScaleConfiguration(&ns.Spec.Scale, fld)
			if got := len(errs); got != tc.wantErrs {
				t.Logf("Validation errors:")
				for i, err := range errs {
					t.Logf("  %d: %s", i+1, err.Error())
				}
				t.Fatalf("got %d errs, want %d", got, tc.wantErrs)
			}
		})
	}
}

// TestValidateResourcesConfiguration table-driven.
func TestValidateResourcesConfiguration(t *testing.T) {
	fld := field.NewPath("spec").Child("resources")

	cases := []struct {
		name     string
		modify   func(*appsv1alpha1.NIMService)
		wantErrs int
	}{
		{"nil resources", func(ns *appsv1alpha1.NIMService) {}, 0},
		{"with claims", func(ns *appsv1alpha1.NIMService) {
			ns.Spec.Resources = &corev1.ResourceRequirements{Claims: []corev1.ResourceClaim{{Name: "c1"}}}
		}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ns := baseNIMService()
			tc.modify(ns)
			errs := validateResourcesConfiguration(ns.Spec.Resources, fld)
			if got := len(errs); got != tc.wantErrs {
				t.Logf("Validation errors:")
				for i, err := range errs {
					t.Logf("  %d: %s", i+1, err.Error())
				}
				t.Fatalf("got %d errs, want %d", got, tc.wantErrs)
			}
		})
	}
}

// TestValidateMultiNodeImmutability table-driven.
func TestValidateMultiNodeImmutability(t *testing.T) {
	fld := field.NewPath("spec").Child("multiNode")
	old := baseNIMService()
	old.Spec.MultiNode = &appsv1alpha1.NimServiceMultiNodeConfig{Size: 1}

	cases := []struct {
		name     string
		newObj   *appsv1alpha1.NIMService
		wantErrs int
	}{
		{"unchanged", func() *appsv1alpha1.NIMService {
			n := baseNIMService()
			n.Spec.MultiNode = &appsv1alpha1.NimServiceMultiNodeConfig{Size: 1}
			return n
		}(), 0},
		{"changed", func() *appsv1alpha1.NIMService {
			n := baseNIMService()
			n.Spec.MultiNode = &appsv1alpha1.NimServiceMultiNodeConfig{Size: 2}
			return n
		}(), 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			errs := validateMultiNodeImmutability(old, c.newObj, fld)
			if got := len(errs); got != c.wantErrs {
				t.Fatalf("got %d errs, want %d", got, c.wantErrs)
			}
		})
	}
}

// TestValidatePVCImmutability table-driven.
func TestValidatePVCImmutability(t *testing.T) {
	fld := field.NewPath("spec").Child("storage").Child("pvc")
	trueVal := true
	old := baseNIMService()
	old.Spec.Storage.PVC = appsv1alpha1.PersistentVolumeClaim{Create: &trueVal, Size: "10Gi"}

	cases := []struct {
		name     string
		newObj   *appsv1alpha1.NIMService
		wantErrs int
	}{
		{"no change", func() *appsv1alpha1.NIMService {
			n := baseNIMService()
			n.Spec.Storage.PVC = old.Spec.Storage.PVC
			return n
		}(), 0},
		{"changed size", func() *appsv1alpha1.NIMService {
			n := baseNIMService()
			n.Spec.Storage.PVC = appsv1alpha1.PersistentVolumeClaim{Create: &trueVal, Size: "20Gi"}
			return n
		}(), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validatePVCImmutability(old, tc.newObj, fld)
			if got := len(errs); got != tc.wantErrs {
				t.Logf("Validation errors:")
				for i, err := range errs {
					t.Logf("  %d: %s", i+1, err.Error())
				}
				t.Fatalf("got %d errs, want %d", got, tc.wantErrs)
			}
		})
	}
}
