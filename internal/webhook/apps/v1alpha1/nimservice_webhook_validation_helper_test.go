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
	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

// TestValidateImageConfiguration covers required field checks on Image.
func TestValidateImageConfiguration(t *testing.T) {
	fldPath := field.NewPath("spec").Child("image")

	tests := []struct {
		name         string
		image        *appsv1alpha1.Image
		wantErrs     int
		wantWarnings int
	}{
		{
			name:         "valid image",
			image:        &appsv1alpha1.Image{Repository: "repo", Tag: "latest"},
			wantErrs:     0,
			wantWarnings: 0,
		},
		{
			name:         "missing repository",
			image:        &appsv1alpha1.Image{Tag: "v1"},
			wantErrs:     1,
			wantWarnings: 0,
		},
		{
			name:         "missing tag",
			image:        &appsv1alpha1.Image{Repository: "repo"},
			wantErrs:     1,
			wantWarnings: 0,
		},
		{
			name:         "missing both",
			image:        &appsv1alpha1.Image{},
			wantErrs:     2,
			wantWarnings: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, errs := validateImageConfiguration(tc.image, fldPath)
			gotErrs := len(errs)
			gotWarnings := len(w)
			if gotErrs != tc.wantErrs || gotWarnings != tc.wantWarnings {
				t.Logf("Validation errors:")
				t.Fatalf("got %d errs, %d warnings, want %d errs, %d warnings", gotErrs, gotWarnings, tc.wantErrs, tc.wantWarnings)
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
		name         string
		modify       func(*appsv1alpha1.NIMService)
		wantErrs     int
		wantWarnings int
	}{
		{
			name:         "missing both nimCache and pvc",
			modify:       func(ns *appsv1alpha1.NIMService) {},
			wantErrs:     1,
			wantWarnings: 0,
		},
		{
			name: "both nimCache and pvc defined",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.Storage.NIMCache = appsv1alpha1.NIMCacheVolSpec{Name: "cache", Profile: "p"}
				ns.Spec.Storage.PVC = appsv1alpha1.PersistentVolumeClaim{Name: "pvc"}
			},
			wantErrs:     1,
			wantWarnings: 0,
		},
		{
			name: "nimCache missing name",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.Storage.NIMCache = appsv1alpha1.NIMCacheVolSpec{Profile: "p"}
			},
			wantErrs:     1,
			wantWarnings: 0,
		},
		{
			name: "pvc create false name empty",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.Storage.PVC = appsv1alpha1.PersistentVolumeClaim{
					Create: &falseVal,
				}
			},
			wantErrs:     1,
			wantWarnings: 0,
		},
		{
			name: "pvc invalid volumeaccessmode",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.Storage.PVC = appsv1alpha1.PersistentVolumeClaim{
					Create:           &trueVal,
					VolumeAccessMode: "BadMode",
				}
			},
			wantErrs:     1,
			wantWarnings: 0,
		},
		{
			name: "shared memory size <=0",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.Storage.PVC = appsv1alpha1.PersistentVolumeClaim{Name: "pvc"}
				ns.Spec.Storage.SharedMemorySizeLimit = &sizeNeg
			},
			wantErrs:     1,
			wantWarnings: 0,
		},
		{
			name: "valid nimCache",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.Storage.NIMCache = appsv1alpha1.NIMCacheVolSpec{Name: "cache", Profile: "default"}
			},
			wantErrs:     0,
			wantWarnings: 0,
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
			wantErrs:     0,
			wantWarnings: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ns := baseNIMService()
			tc.modify(ns)
			w, errs := validateServiceStorageConfiguration(&ns.Spec.Storage, fldPath)
			gotErrs := len(errs)
			gotWarnings := len(w)
			if gotErrs != tc.wantErrs || gotWarnings != tc.wantWarnings {
				t.Logf("Validation errors:")
				t.Fatalf("got %d errs, %d warnings, want %d errs, %d warnings", gotErrs, gotWarnings, tc.wantErrs, tc.wantWarnings)
			}
		})
	}
}

// TestValidateDRAResourcesConfiguration covers DRA resource validation rules and
// Kubernetes-version compatibility in a comprehensive table-driven test.
func TestValidateDRAResourcesConfiguration(t *testing.T) {
	fld := field.NewPath("spec")

	cases := []struct {
		name            string
		modify          func(*appsv1alpha1.NIMService)
		k8sVersion      string
		wantErrs        int
		wantWarnings    int
		wantErrMsgs     []string
		wantWarningMsgs []string
	}{
		{
			name:            "no dra resources",
			modify:          func(ns *appsv1alpha1.NIMService) {},
			k8sVersion:      "v1.34.0",
			wantErrs:        0,
			wantWarnings:    0,
			wantErrMsgs:     nil,
			wantWarningMsgs: nil,
		},
		{
			name: "unsupported k8s version",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ResourceClaimTemplateName: ptr.To("tmpl1"),
				}}
			},
			k8sVersion:      "v1.33.0", // below MinSupportedClusterVersionForDRA
			wantErrs:        1,
			wantWarnings:    0,
			wantErrMsgs:     []string{"spec.draResources: Forbidden: is not supported by NIM-Operator on this cluster, please upgrade to k8s version 'v1.34.0' or higher"},
			wantWarningMsgs: nil,
		},
		{
			name: "both name and template provided",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ResourceClaimName:         ptr.To("claim1"),
					ResourceClaimTemplateName: ptr.To("tmpl1"),
				}}
			},
			k8sVersion:      "v1.34.0",
			wantErrs:        1,
			wantWarnings:    0,
			wantErrMsgs:     []string{"spec.draResources[0]: Invalid value: \"multiple dra resource sources defined\": must specify exactly one of spec.resourceClaimName, spec.resourceClaimTemplateName, or spec.claimCreationSpec"},
			wantWarningMsgs: nil,
		},
		{
			name: "both name and claimCreationSpec provided",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ResourceClaimName: ptr.To("claim1"),
					ClaimCreationSpec: &appsv1alpha1.DRAClaimCreationSpec{
						Devices: []appsv1alpha1.DRADeviceSpec{{
							Name:            "gpu",
							Count:           1,
							DeviceClassName: "gpu.nvidia.com",
							DriverName:      "gpu.nvidia.com",
						}},
					},
				}}
			},
			k8sVersion:      "v1.34.0",
			wantErrs:        1,
			wantErrMsgs:     []string{"spec.draResources[0]: Invalid value: \"multiple dra resource sources defined\": must specify exactly one of spec.resourceClaimName, spec.resourceClaimTemplateName, or spec.claimCreationSpec"},
			wantWarningMsgs: nil,
		},
		{
			name: "both template and claimCreationSpec provided",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ResourceClaimTemplateName: ptr.To("tmpl1"),
					ClaimCreationSpec: &appsv1alpha1.DRAClaimCreationSpec{
						Devices: []appsv1alpha1.DRADeviceSpec{{
							Name:            "gpu",
							Count:           1,
							DeviceClassName: "gpu.nvidia.com",
							DriverName:      "gpu.nvidia.com",
						}},
					},
				}}
			},
			k8sVersion:      "v1.34.0",
			wantErrs:        1,
			wantErrMsgs:     []string{"spec.draResources[0]: Invalid value: \"multiple dra resource sources defined\": must specify exactly one of spec.resourceClaimName, spec.resourceClaimTemplateName, or spec.claimCreationSpec"},
			wantWarningMsgs: nil,
			wantWarnings:    0,
		},
		{
			name: "all three fields provided",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ResourceClaimName:         ptr.To("claim1"),
					ResourceClaimTemplateName: ptr.To("tmpl1"),
					ClaimCreationSpec: &appsv1alpha1.DRAClaimCreationSpec{
						Devices: []appsv1alpha1.DRADeviceSpec{{
							Name:            "gpu",
							Count:           1,
							DeviceClassName: "gpu.nvidia.com",
							DriverName:      "gpu.nvidia.com",
						}},
					},
				}}
			},
			k8sVersion:      "v1.34.0",
			wantErrs:        1,
			wantErrMsgs:     []string{"spec.draResources[0]: Invalid value: \"multiple dra resource sources defined\": must specify exactly one of spec.resourceClaimName, spec.resourceClaimTemplateName, or spec.claimCreationSpec"},
			wantWarningMsgs: nil,
			wantWarnings:    0,
		},
		{
			name: "neither name nor template nor claimCreationSpec provided",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{}}
			},
			k8sVersion:      "v1.34.0",
			wantErrs:        1,
			wantWarnings:    0,
			wantWarningMsgs: nil,
			wantErrMsgs:     []string{"spec.draResources[0]: Required value: one of spec.resourceClaimName, spec.resourceClaimTemplateName, or spec.claimCreationSpec must be provided"},
		},
		{
			name: "resourceClaimName with replicas>1",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.Replicas = ptr.To(int32(2))
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ResourceClaimName: ptr.To("claim1"),
				}}
			},
			k8sVersion:      "v1.34.0",
			wantErrs:        1,
			wantWarnings:    0,
			wantErrMsgs:     []string{"spec.draResources[0].resourceClaimName: Forbidden: must not be set when spec.replicas > 1, use spec.draResources[0].resourceClaimTemplateName instead"},
			wantWarningMsgs: nil,
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
			k8sVersion:      "v1.34.0",
			wantErrs:        1,
			wantWarnings:    0,
			wantErrMsgs:     []string{"spec.draResources[0].resourceClaimName: Forbidden: must not be set when spec.scale.enabled is true, use spec.draResources[0].resourceClaimTemplateName instead"},
			wantWarningMsgs: nil,
		},
		{
			name: "duplicate resourceClaimNames",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{
					{ResourceClaimName: ptr.To("dup")},
					{ResourceClaimName: ptr.To("dup")},
				}
			},
			k8sVersion:      "v1.34.0",
			wantErrs:        1,
			wantWarnings:    0,
			wantErrMsgs:     []string{"spec.draResources[1].resourceClaimName: Duplicate value: \"dup\""},
			wantWarningMsgs: nil,
		},
		{
			name: "claimCreationSpec with empty devices",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ClaimCreationSpec: &appsv1alpha1.DRAClaimCreationSpec{
						Devices: []appsv1alpha1.DRADeviceSpec{},
					},
				}}
			},
			k8sVersion:      "v1.34.0",
			wantErrs:        1,
			wantErrMsgs:     []string{"spec.draResources[0].claimCreationSpec.devices: Required value: must be non-empty"},
			wantWarningMsgs: nil,
			wantWarnings:    0,
		},
		{
			name: "claimCreationSpec with invalid device - missing name",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ClaimCreationSpec: &appsv1alpha1.DRAClaimCreationSpec{
						Devices: []appsv1alpha1.DRADeviceSpec{{
							Count:           1,
							DeviceClassName: "gpu.nvidia.com",
							DriverName:      "gpu.nvidia.com",
						}},
					},
				}}
			},
			k8sVersion:      "v1.34.0",
			wantErrs:        1,
			wantErrMsgs:     []string{"spec.draResources[0].claimCreationSpec.devices[0].name: Required value: is required"},
			wantWarningMsgs: nil,
			wantWarnings:    0,
		},
		{
			name: "claimCreationSpec with invalid device - zero count",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ClaimCreationSpec: &appsv1alpha1.DRAClaimCreationSpec{
						Devices: []appsv1alpha1.DRADeviceSpec{{
							Name:            "gpu",
							Count:           0,
							DeviceClassName: "gpu.nvidia.com",
							DriverName:      "gpu.nvidia.com",
						}},
					},
				}}
			},
			k8sVersion:      "v1.34.0",
			wantErrs:        1,
			wantErrMsgs:     []string{"spec.draResources[0].claimCreationSpec.devices[0].count: Invalid value: 0: must be > 0"},
			wantWarningMsgs: nil,
			wantWarnings:    0,
		},
		{
			name: "claimCreationSpec with invalid device - missing deviceClassName",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ClaimCreationSpec: &appsv1alpha1.DRAClaimCreationSpec{
						Devices: []appsv1alpha1.DRADeviceSpec{{
							Name:       "gpu",
							Count:      1,
							DriverName: "gpu.nvidia.com",
						}},
					},
				}}
			},
			k8sVersion:      "v1.34.0",
			wantErrs:        1,
			wantErrMsgs:     []string{"spec.draResources[0].claimCreationSpec.devices[0].deviceClassName: Required value: is required"},
			wantWarningMsgs: nil,
			wantWarnings:    0,
		},
		{
			name: "claimCreationSpec with invalid device - missing driverName",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ClaimCreationSpec: &appsv1alpha1.DRAClaimCreationSpec{
						Devices: []appsv1alpha1.DRADeviceSpec{{
							Name:            "gpu",
							Count:           1,
							DeviceClassName: "gpu.nvidia.com",
						}},
					},
				}}
			},
			k8sVersion:      "v1.34.0",
			wantErrs:        1,
			wantErrMsgs:     []string{"spec.draResources[0].claimCreationSpec.devices[0].driverName: Required value: is required"},
			wantWarningMsgs: nil,
			wantWarnings:    0,
		},
		{
			name: "claimCreationSpec with duplicate attributeSelectors keys",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ClaimCreationSpec: &appsv1alpha1.DRAClaimCreationSpec{
						Devices: []appsv1alpha1.DRADeviceSpec{{
							Name:            "gpu",
							Count:           1,
							DeviceClassName: "gpu.nvidia.com",
							DriverName:      "gpu.nvidia.com",
							AttributeSelectors: []appsv1alpha1.DRADeviceAttributeSelector{
								{
									Key: "memory", // This normalizes to the same as "gpu.nvidia.com/memory"
									Op:  appsv1alpha1.DRADeviceAttributeSelectorOpEqual,
									Value: &appsv1alpha1.DRADeviceAttributeSelectorValue{
										StringValue: ptr.To("8Gi"),
									},
								},
								{
									Key: "gpu.nvidia.com/memory",
									Op:  appsv1alpha1.DRADeviceAttributeSelectorOpEqual,
									Value: &appsv1alpha1.DRADeviceAttributeSelectorValue{
										StringValue: ptr.To("16Gi"),
									},
								},
							},
						}},
					},
				}}
			},
			k8sVersion:      "v1.34.0",
			wantErrs:        1,
			wantErrMsgs:     []string{"spec.draResources[0].claimCreationSpec.devices[0].attributeSelectors[1]: Duplicate value: \"gpu.nvidia.com/memory\""},
			wantWarningMsgs: nil,
			wantWarnings:    0,
		},
		{
			name: "claimCreationSpec with duplicate capacitySelectors keys",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ClaimCreationSpec: &appsv1alpha1.DRAClaimCreationSpec{
						Devices: []appsv1alpha1.DRADeviceSpec{{
							Name:            "gpu",
							Count:           1,
							DeviceClassName: "gpu.nvidia.com",
							DriverName:      "gpu.nvidia.com",
							CapacitySelectors: []appsv1alpha1.DRAResourceQuantitySelector{
								{
									Key:   "memory", // This normalizes to the same as "gpu.nvidia.com/memory"
									Op:    appsv1alpha1.DRAResourceQuantitySelectorOpEqual,
									Value: resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
								},
								{
									Key:   "gpu.nvidia.com/memory",
									Op:    appsv1alpha1.DRAResourceQuantitySelectorOpEqual,
									Value: resource.NewQuantity(16*1024*1024*1024, resource.BinarySI),
								},
							},
						}},
					},
				}}
			},
			k8sVersion:      "v1.34.0",
			wantErrs:        1,
			wantErrMsgs:     []string{"spec.draResources[0].claimCreationSpec.devices[0].capacitySelectors[1]: Duplicate value: \"gpu.nvidia.com/memory\""},
			wantWarningMsgs: nil,
			wantWarnings:    0,
		},
		{
			name: "claimCreationSpec with invalid attribute selector - missing op",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ClaimCreationSpec: &appsv1alpha1.DRAClaimCreationSpec{
						Devices: []appsv1alpha1.DRADeviceSpec{{
							Name:            "gpu",
							Count:           1,
							DeviceClassName: "gpu.nvidia.com",
							DriverName:      "gpu.nvidia.com",
							AttributeSelectors: []appsv1alpha1.DRADeviceAttributeSelector{
								{
									Key: "memory",
									// Op is intentionally missing
									Value: &appsv1alpha1.DRADeviceAttributeSelectorValue{
										StringValue: ptr.To("8Gi"),
									},
								},
							},
						}},
					},
				}}
			},
			k8sVersion:      "v1.34.0",
			wantErrs:        1,
			wantErrMsgs:     []string{"spec.draResources[0].claimCreationSpec.devices[0].attributeSelectors[0].op: Required value: is required"},
			wantWarningMsgs: nil,
			wantWarnings:    0,
		},
		{
			name: "claimCreationSpec with invalid attribute selector - invalid op",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ClaimCreationSpec: &appsv1alpha1.DRAClaimCreationSpec{
						Devices: []appsv1alpha1.DRADeviceSpec{{
							Name:            "gpu",
							Count:           1,
							DeviceClassName: "gpu.nvidia.com",
							DriverName:      "gpu.nvidia.com",
							AttributeSelectors: []appsv1alpha1.DRADeviceAttributeSelector{
								{
									Key: "memory",
									Op:  "InvalidOp", // Invalid op
									Value: &appsv1alpha1.DRADeviceAttributeSelectorValue{
										StringValue: ptr.To("8Gi"),
									},
								},
							},
						}},
					},
				}}
			},
			k8sVersion:      "v1.34.0",
			wantErrs:        1,
			wantErrMsgs:     []string{"spec.draResources[0].claimCreationSpec.devices[0].attributeSelectors[0].op: Invalid value: \"InvalidOp\": must be one of [Equal NotEqual GreaterThan GreaterThanOrEqual LessThan LessThanOrEqual]"},
			wantWarningMsgs: nil,
			wantWarnings:    0,
		},
		{
			name: "claimCreationSpec with invalid attribute selector - no value",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ClaimCreationSpec: &appsv1alpha1.DRAClaimCreationSpec{
						Devices: []appsv1alpha1.DRADeviceSpec{{
							Name:            "gpu",
							Count:           1,
							DeviceClassName: "gpu.nvidia.com",
							DriverName:      "gpu.nvidia.com",
							AttributeSelectors: []appsv1alpha1.DRADeviceAttributeSelector{
								{
									Key:   "memory",
									Op:    appsv1alpha1.DRADeviceAttributeSelectorOpEqual,
									Value: &appsv1alpha1.DRADeviceAttributeSelectorValue{},
								},
							},
						}},
					},
				}}
			},
			k8sVersion:      "v1.34.0",
			wantErrs:        1,
			wantErrMsgs:     []string{"spec.draResources[0].claimCreationSpec.devices[0].attributeSelectors[0].value: Required value: must specify exactly one of spec.draResources[0].claimCreationSpec.devices[0].attributeSelectors[0].value.boolValue, spec.draResources[0].claimCreationSpec.devices[0].attributeSelectors[0].value.intValue, spec.draResources[0].claimCreationSpec.devices[0].attributeSelectors[0].value.stringValue, or spec.draResources[0].claimCreationSpec.devices[0].attributeSelectors[0].value.versionValue"},
			wantWarningMsgs: nil,
			wantWarnings:    0,
		},
		{
			name: "claimCreationSpec with invalid attribute selector - multiple values",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ClaimCreationSpec: &appsv1alpha1.DRAClaimCreationSpec{
						Devices: []appsv1alpha1.DRADeviceSpec{{
							Name:            "gpu",
							Count:           1,
							DeviceClassName: "gpu.nvidia.com",
							DriverName:      "gpu.nvidia.com",
							AttributeSelectors: []appsv1alpha1.DRADeviceAttributeSelector{
								{
									Key: "memory",
									Op:  appsv1alpha1.DRADeviceAttributeSelectorOpEqual,
									Value: &appsv1alpha1.DRADeviceAttributeSelectorValue{
										StringValue: ptr.To("8Gi"),
										IntValue:    ptr.To(int32(8)),
									},
								},
							},
						}},
					},
				}}
			},
			k8sVersion:      "v1.34.0",
			wantErrs:        1,
			wantErrMsgs:     []string{"spec.draResources[0].claimCreationSpec.devices[0].attributeSelectors[0].value: Invalid value: \"multiple attribute values defined\": must specify exactly one of spec.draResources[0].claimCreationSpec.devices[0].attributeSelectors[0].value.boolValue, spec.draResources[0].claimCreationSpec.devices[0].attributeSelectors[0].value.intValue, spec.draResources[0].claimCreationSpec.devices[0].attributeSelectors[0].value.stringValue, or spec.draResources[0].claimCreationSpec.devices[0].attributeSelectors[0].value.versionValue"},
			wantWarningMsgs: nil,
			wantWarnings:    0,
		},
		{
			name: "claimCreationSpec with invalid version value",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ClaimCreationSpec: &appsv1alpha1.DRAClaimCreationSpec{
						Devices: []appsv1alpha1.DRADeviceSpec{{
							Name:            "gpu",
							Count:           1,
							DeviceClassName: "gpu.nvidia.com",
							DriverName:      "gpu.nvidia.com",
							AttributeSelectors: []appsv1alpha1.DRADeviceAttributeSelector{
								{
									Key: "driver-version",
									Op:  appsv1alpha1.DRADeviceAttributeSelectorOpEqual,
									Value: &appsv1alpha1.DRADeviceAttributeSelectorValue{
										VersionValue: ptr.To("550.127.08"),
									},
								},
							},
						}},
					},
				}}
			},
			k8sVersion:      "v1.34.0",
			wantErrs:        1,
			wantErrMsgs:     []string{"spec.draResources[0].claimCreationSpec.devices[0].attributeSelectors[0].value: Invalid value: \"550.127.08\": must be a valid semantic version: Patch number must not contain leading zeroes \"08\""},
			wantWarningMsgs: nil,
			wantWarnings:    0,
		},
		{
			name: "claimCreationSpec with invalid quantity selector - invalid op",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ClaimCreationSpec: &appsv1alpha1.DRAClaimCreationSpec{
						Devices: []appsv1alpha1.DRADeviceSpec{{
							Name:            "gpu",
							Count:           1,
							DeviceClassName: "gpu.nvidia.com",
							DriverName:      "gpu.nvidia.com",
							CapacitySelectors: []appsv1alpha1.DRAResourceQuantitySelector{
								{
									Key:   "memory",
									Op:    "InvalidOp",
									Value: resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
								},
							},
						}},
					},
				}}
			},
			k8sVersion:      "v1.34.0",
			wantErrs:        1,
			wantErrMsgs:     []string{"spec.draResources[0].claimCreationSpec.devices[0].capacitySelectors[0].op: Invalid value: \"InvalidOp\": must be one of [Equal NotEqual GreaterThan GreaterThanOrEqual LessThan LessThanOrEqual]"},
			wantWarningMsgs: nil,
			wantWarnings:    0,
		},
		{
			name: "claimCreationSpec with invalid quantity selector - missing value",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ClaimCreationSpec: &appsv1alpha1.DRAClaimCreationSpec{
						Devices: []appsv1alpha1.DRADeviceSpec{{
							Name:            "gpu",
							Count:           1,
							DeviceClassName: "gpu.nvidia.com",
							DriverName:      "gpu.nvidia.com",
							CapacitySelectors: []appsv1alpha1.DRAResourceQuantitySelector{
								{
									Key: "memory",
									Op:  appsv1alpha1.DRAResourceQuantitySelectorOpEqual,
									// Value is intentionally missing
								},
							},
						}},
					},
				}}
			},
			k8sVersion:      "v1.34.0",
			wantErrs:        1,
			wantErrMsgs:     []string{"spec.draResources[0].claimCreationSpec.devices[0].capacitySelectors[0].value: Required value: is required"},
			wantWarningMsgs: nil,
			wantWarnings:    0,
		},
		{
			name: "valid template",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.DRAResources = []appsv1alpha1.DRAResource{{
					ResourceClaimTemplateName: ptr.To("tmpl1"),
				}}
			},
			k8sVersion:      "v1.34.0",
			wantErrs:        0,
			wantWarnings:    0,
			wantWarningMsgs: nil,
			wantErrMsgs:     nil,
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
			k8sVersion:      "v1.34.0",
			wantErrs:        0,
			wantWarningMsgs: nil,
			wantWarnings:    0,
			wantErrMsgs:     nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ns := baseNIMService()
			tc.modify(ns)
			w, errs := validateDRAResourcesConfiguration(&ns.Spec, fld, tc.k8sVersion)
			gotErrs := len(errs)
			gotWarnings := len(w)
			if gotErrs != tc.wantErrs || gotWarnings != tc.wantWarnings {
				t.Logf("Validation errors:")
				t.Fatalf("got %d errs, %d warnings, want %d errs, %d warnings", gotErrs, gotWarnings, tc.wantErrs, tc.wantWarnings)
			}

			// Check exact error messages if expected
			if tc.wantErrMsgs != nil {
				if len(errs) != len(tc.wantErrMsgs) {
					t.Fatalf("got %d errors, want %d", len(errs), len(tc.wantErrMsgs))
				}
				for i, expectedMsg := range tc.wantErrMsgs {
					if got := errs[i].Error(); got != expectedMsg {
						t.Errorf("\n  got:  %q\n  want: %q", got, expectedMsg)
					}
				}
			}
		})
	}
}

// TestValidateAuthSecret verifies required secret enforcement using table-driven cases.
func TestValidateAuthSecret(t *testing.T) {
	fld := field.NewPath("spec").Child("authSecret")
	cases := []struct {
		name         string
		value        string
		wantErrs     int
		wantWarnings int
	}{
		{"non-empty", "secret", 0, 0},
		{"empty", "", 1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, errs := validateAuthSecret(&tc.value, fld)
			gotErrs := len(errs)
			gotWarnings := len(w)
			if gotErrs != tc.wantErrs || gotWarnings != tc.wantWarnings {
				t.Logf("Validation errors:")
				t.Fatalf("got %d errs, %d warnings, want %d errs, %d warnings", gotErrs, gotWarnings, tc.wantErrs, tc.wantWarnings)
			}
		})
	}
}

// TestValidateMetricsConfiguration table-driven.
func TestValidateMetricsConfiguration(t *testing.T) {
	fld := field.NewPath("spec").Child("metrics")
	enabled := true

	cases := []struct {
		name         string
		modify       func(*appsv1alpha1.NIMService)
		wantErrs     int
		wantWarnings int
	}{
		{
			name:         "metrics disabled",
			modify:       func(ns *appsv1alpha1.NIMService) {},
			wantErrs:     0,
			wantWarnings: 0,
		},
		{
			name: "enabled empty monitor",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.Metrics.Enabled = &enabled
			},
			wantErrs:     1,
			wantWarnings: 0,
		},
		{
			name: "enabled valid",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.Metrics.Enabled = &enabled
				ns.Spec.Metrics.ServiceMonitor.Interval = "30s"
			},
			wantErrs:     0,
			wantWarnings: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ns := baseNIMService()
			tc.modify(ns)
			w, errs := validateMetricsConfiguration(&ns.Spec.Metrics, fld)
			gotErrs := len(errs)
			gotWarnings := len(w)
			if gotErrs != tc.wantErrs || gotWarnings != tc.wantWarnings {
				t.Logf("Validation errors:")
				t.Fatalf("got %d errs, %d warnings, want %d errs, %d warnings", gotErrs, gotWarnings, tc.wantErrs, tc.wantWarnings)
			}
		})
	}
}

// TestValidateScaleConfiguration table-driven.
func TestValidateScaleConfiguration(t *testing.T) {
	fld := field.NewPath("spec").Child("scale")
	enabled := true

	cases := []struct {
		name         string
		modify       func(*appsv1alpha1.NIMService)
		wantErrs     int
		wantWarnings int
	}{
		{"autoscaling disabled", func(ns *appsv1alpha1.NIMService) {}, 0, 0},
		{"enabled empty HPA", func(ns *appsv1alpha1.NIMService) { ns.Spec.Scale.Enabled = &enabled }, 1, 0},
		{"enabled valid HPA", func(ns *appsv1alpha1.NIMService) { ns.Spec.Scale.Enabled = &enabled; ns.Spec.Scale.HPA.MaxReplicas = 3 }, 0, 0},
		{"enabled both HPA and replicas", func(ns *appsv1alpha1.NIMService) {
			ns.Spec.Scale.Enabled = &enabled
			ns.Spec.Scale.HPA.MaxReplicas = 3
			ns.Spec.Replicas = ptr.To(int32(2))
		}, 1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ns := baseNIMService()
			tc.modify(ns)
			w, errs := validateScaleConfiguration(&ns.Spec.Scale, ns.Spec.Replicas, fld)
			gotErrs := len(errs)
			gotWarnings := len(w)
			if got := len(errs); got != tc.wantErrs {
				t.Logf("Validation errors:")
				t.Fatalf("got %d errs, %d warnings, want %d errs, %d warnings", gotErrs, gotWarnings, tc.wantErrs, tc.wantWarnings)
				t.Fatalf("got %d errs, %d warnings, want %d errs, %d warnings", gotErrs, gotWarnings, tc.wantErrs, tc.wantWarnings)
			}
		})
	}
}

// TestValidateResourcesConfiguration table-driven.
func TestValidateResourcesConfiguration(t *testing.T) {
	fld := field.NewPath("spec").Child("resources")

	cases := []struct {
		name         string
		modify       func(*appsv1alpha1.NIMService)
		wantErrs     int
		wantWarnings int
	}{
		{"nil resources", func(ns *appsv1alpha1.NIMService) {}, 0, 0},
		{"with claims", func(ns *appsv1alpha1.NIMService) {
			ns.Spec.Resources = &corev1.ResourceRequirements{Claims: []corev1.ResourceClaim{{Name: "c1"}}}
		}, 1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ns := baseNIMService()
			tc.modify(ns)
			w, errs := validateResourcesConfiguration(ns.Spec.Resources, fld)
			gotErrs := len(errs)
			gotWarnings := len(w)
			if gotErrs != tc.wantErrs || gotWarnings != tc.wantWarnings {
				t.Logf("Validation errors:")
				t.Fatalf("got %d errs, %d warnings, want %d errs, %d warnings", gotErrs, gotWarnings, tc.wantErrs, tc.wantWarnings)
			}
		})
	}
}

// TestValidateMultiNodeImmutability table-driven.
func TestValidateMultiNodeImmutability(t *testing.T) {
	fld := field.NewPath("spec").Child("multiNode")
	old := baseNIMService()
	old.Spec.MultiNode = &appsv1alpha1.NimServiceMultiNodeConfig{Parallelism: &appsv1alpha1.ParallelismSpec{Pipeline: ptr.To(uint32(1))}}

	cases := []struct {
		name         string
		newObj       *appsv1alpha1.NIMService
		wantErrs     int
		wantWarnings int
	}{
		{"unchanged", func() *appsv1alpha1.NIMService {
			n := baseNIMService()
			n.Spec.MultiNode = &appsv1alpha1.NimServiceMultiNodeConfig{Parallelism: &appsv1alpha1.ParallelismSpec{Pipeline: ptr.To(uint32(1))}}
			return n
		}(), 0, 0},
		{"changed", func() *appsv1alpha1.NIMService {
			n := baseNIMService()
			n.Spec.MultiNode = &appsv1alpha1.NimServiceMultiNodeConfig{Parallelism: &appsv1alpha1.ParallelismSpec{Pipeline: ptr.To(uint32(2))}}
			return n
		}(), 1, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w, errs := validateMultiNodeImmutability(old, c.newObj, fld)
			gotErrs := len(errs)
			gotWarnings := len(w)
			if gotErrs != c.wantErrs || gotWarnings != c.wantWarnings {
				t.Logf("Validation errors:")
				t.Fatalf("got %d errs, %d warnings, want %d errs, %d warnings", gotErrs, gotWarnings, c.wantErrs, c.wantWarnings)
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
		name         string
		newObj       *appsv1alpha1.NIMService
		wantErrs     int
		wantWarnings int
	}{
		{"no change", func() *appsv1alpha1.NIMService {
			n := baseNIMService()
			n.Spec.Storage.PVC = old.Spec.Storage.PVC
			return n
		}(), 0, 0},
		{"changed size", func() *appsv1alpha1.NIMService {
			n := baseNIMService()
			n.Spec.Storage.PVC = appsv1alpha1.PersistentVolumeClaim{Create: &trueVal, Size: "20Gi"}
			return n
		}(), 1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, errs := validatePVCImmutability(old, tc.newObj, fld)
			gotErrs := len(errs)
			gotWarnings := len(w)
			if gotErrs != tc.wantErrs || gotWarnings != tc.wantWarnings {
				t.Logf("Validation errors:")
				t.Fatalf("got %d errs, %d warnings, want %d errs, %d warnings", gotErrs, gotWarnings, tc.wantErrs, tc.wantWarnings)
			}
		})
	}
}

// TestValidateKServeConfiguration covers all KServe-specific validation rules.
func TestValidateKServeConfiguration(t *testing.T) {
	fld := field.NewPath("spec")

	trueVal := true

	tests := []struct {
		name         string
		modify       func(*appsv1alpha1.NIMService)
		wantErrs     int
		wantWarnings int
	}{
		{
			name: "standalone platform – no errors",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.InferencePlatform = appsv1alpha1.PlatformTypeStandalone
			},
			wantErrs:     0,
			wantWarnings: 0,
		},
		{
			name: "kserve knative (annotation absent) – valid",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.InferencePlatform = appsv1alpha1.PlatformTypeKServe
				// No annotation ⇒ knative by default.
			},
			wantErrs:     0,
			wantWarnings: 0,
		},
		{
			name: "kserve knative (annotation present) – autoscaling set",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.InferencePlatform = appsv1alpha1.PlatformTypeKServe
				ns.Spec.Annotations = map[string]string{utils.KServeDeploymentModeAnnotationKey: "Knative"}
				ns.Spec.Scale.Enabled = &trueVal
			},
			wantErrs:     1,
			wantWarnings: 0,
		},
		{
			name: "kserve knative – ingress set",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.InferencePlatform = appsv1alpha1.PlatformTypeKServe
				ns.Spec.Expose.Router.Ingress = &appsv1alpha1.RouterIngress{
					IngressClass: "nginx",
				}
			},
			wantErrs:     1,
			wantWarnings: 0,
		},
		{
			name: "kserve knative – servicemonitor set",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.InferencePlatform = appsv1alpha1.PlatformTypeKServe
				ns.Spec.Metrics.Enabled = &trueVal
			},
			wantErrs:     1,
			wantWarnings: 0,
		},
		{
			name: "kserve knative – all prohibited set",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.InferencePlatform = appsv1alpha1.PlatformTypeKServe
				ns.Spec.Scale.Enabled = &trueVal
				ns.Spec.Expose.Router.Ingress = &appsv1alpha1.RouterIngress{
					IngressClass: "nginx",
				}
				ns.Spec.Metrics.Enabled = &trueVal
			},
			wantErrs:     3,
			wantWarnings: 0,
		},
		{
			name: "kserve rawdeployment – allowed autoscaling, but multidnode forbidden",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.InferencePlatform = appsv1alpha1.PlatformTypeKServe
				ns.Spec.Annotations = map[string]string{utils.KServeDeploymentModeAnnotationKey: "RawDeployment"}
				ns.Spec.Scale.Enabled = &trueVal // should be fine
				ns.Spec.MultiNode = &appsv1alpha1.NimServiceMultiNodeConfig{Parallelism: &appsv1alpha1.ParallelismSpec{Pipeline: ptr.To(uint32(1))}}
			},
			wantErrs:     1, // only multiNode should trigger
			wantWarnings: 0,
		},
		{
			name: "kserve – multidnode alone",
			modify: func(ns *appsv1alpha1.NIMService) {
				ns.Spec.InferencePlatform = appsv1alpha1.PlatformTypeKServe
				ns.Spec.MultiNode = &appsv1alpha1.NimServiceMultiNodeConfig{Parallelism: &appsv1alpha1.ParallelismSpec{Pipeline: ptr.To(uint32(2))}}
			},
			wantErrs:     1,
			wantWarnings: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ns := baseNIMService()
			// Ensure nested structs are initialised to avoid nil panics when we set sub-fields.
			ns.Spec.Scale = appsv1alpha1.Autoscaling{}
			ns.Spec.Expose = appsv1alpha1.Expose{}
			ns.Spec.Expose.Router = appsv1alpha1.Router{}
			ns.Spec.Metrics = appsv1alpha1.Metrics{}

			tc.modify(ns)

			w, errs := validateKServeConfiguration(&ns.Spec, fld)
			gotErrs := len(errs)
			gotWarnings := len(w)
			if gotErrs != tc.wantErrs || gotWarnings != tc.wantWarnings {
				t.Logf("Validation errors:")
				t.Fatalf("got %d errs, %d warnings, want %d errs, %d warnings", gotErrs, gotWarnings, tc.wantErrs, tc.wantWarnings)
			}
		})
	}
}
