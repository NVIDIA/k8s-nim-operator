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
	"fmt"

	apiresource "k8s.io/apimachinery/pkg/api/resource"

	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
	k8sutilcel "github.com/NVIDIA/k8s-nim-operator/internal/k8sutil/cel"
)

// DRAResource references exactly one ResourceClaim, either directly
// or by naming a ResourceClaimTemplate which is then turned into a ResourceClaim.
//
// When creating the NIMService pods, it adds a name (`DNS_LABEL` format) to it
// that uniquely identifies the DRA resource.
// +kubebuilder:validation:XValidation:rule="(has(self.resourceClaimName) ? 1 : 0) + (has(self.resourceClaimTemplateName) ? 1 : 0) + (has(self.claimSpec) ? 1 : 0) == 1",message="exactly one of spec.resourceClaimName, spec.resourceClaimTemplateName, or spec.claimSpec must be set."
type DRAResource struct {
	// ResourceClaimName is the name of a DRA resource claim object in the same
	// namespace as the NIMService.
	//
	// Exactly one of ResourceClaimName and ResourceClaimTemplateName must
	// be set.
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*`
	ResourceClaimName *string `json:"resourceClaimName,omitempty"`

	// ResourceClaimTemplateName is the name of a DRA resource claim template
	// object in the same namespace as the pods for this NIMService.
	//
	// The template will be used to create a new DRA resource claim, which will
	// be bound to the pods created for this NIMService.
	//
	// Exactly one of ResourceClaimName and ResourceClaimTemplateName must
	// be set.
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*`
	ResourceClaimTemplateName *string `json:"resourceClaimTemplateName,omitempty"`

	// ClaimSpec is the spec to auto-generate a DRA resource claim/resource claim template. Only one of ClaimSpec, ResourceClaimName or ResourceClaimTemplateName must be specified.
	ClaimSpec *DRAClaimSpec `json:"claimSpec,omitempty"`

	// Requests is the list of requests in the referenced DRA resource claim/resource claim template
	// to be made available to the model container of the NIMService pods.
	//
	// If empty, everything from the claim is made available, otherwise
	// only the result of this subset of requests.
	//
	// +kubebuilder:validation:items:MinLength=1
	Requests []string `json:"requests,omitempty"`
}

// DRADeviceAttributeMatcherValue defines the value of a device attribute to match.
// Exactly one of the fields must be set.
type DRADeviceAttributeMatcherValue struct {
	// BoolValue is a true/false value.
	BoolValue *bool `json:"boolValue,omitempty"`
	// IntValue is a number.
	IntValue *int32 `json:"intValue,omitempty"`
	// StringValue is a string value.
	// +kubebuilder:validation:MaxLength=64
	StringValue *string `json:"stringValue,omitempty"`
	// VersionValue is a semantic version according to semver.org spec 2.0.0.
	// +kubebuilder:validation:MaxLength=64
	VersionValue *string `json:"versionValue,omitempty"`
}

func (d *DRADeviceAttributeMatcherValue) GetValue() any {
	switch {
	case d.BoolValue != nil:
		return *d.BoolValue
	case d.IntValue != nil:
		return int(*d.IntValue)
	case d.StringValue != nil:
		return *d.StringValue
	case d.VersionValue != nil:
		return *d.VersionValue
	}
	return nil
}

func (d *DRADeviceAttributeMatcherValue) GetValueType() k8sutilcel.ValueType {
	switch {
	case d.BoolValue != nil:
		return k8sutilcel.TypeBool
	case d.IntValue != nil:
		return k8sutilcel.TypeInt
	case d.StringValue != nil:
		return k8sutilcel.TypeString
	case d.VersionValue != nil:
		return k8sutilcel.TypeSemver
	default:
		return k8sutilcel.TypeUnknown
	}
}

// DRADeviceAttributeMatcherOp defines the operator to use for matching a device attribute.
type DRADeviceAttributeMatcherOp string

const (
	DRADeviceAttributeMatcherOpEqual              DRADeviceAttributeMatcherOp = "Equal"
	DRADeviceAttributeMatcherOpNotEqual           DRADeviceAttributeMatcherOp = "NotEqual"
	DRADeviceAttributeMatcherOpGreaterThan        DRADeviceAttributeMatcherOp = "GreaterThan"
	DRADeviceAttributeMatcherOpGreaterThanOrEqual DRADeviceAttributeMatcherOp = "GreaterThanOrEqual"
	DRADeviceAttributeMatcherOpLessThan           DRADeviceAttributeMatcherOp = "LessThan"
	DRADeviceAttributeMatcherOpLessThanOrEqual    DRADeviceAttributeMatcherOp = "LessThanOrEqual"
)

func (d DRADeviceAttributeMatcherOp) GetCELOperator() k8sutilcel.ComparisonOperator {
	switch d {
	case DRADeviceAttributeMatcherOpEqual:
		return k8sutilcel.OpEqual
	case DRADeviceAttributeMatcherOpNotEqual:
		return k8sutilcel.OpNotEqual
	case DRADeviceAttributeMatcherOpGreaterThan:
		return k8sutilcel.OpGreater
	case DRADeviceAttributeMatcherOpGreaterThanOrEqual:
		return k8sutilcel.OpGreaterOrEqual
	case DRADeviceAttributeMatcherOpLessThan:
		return k8sutilcel.OpLess
	case DRADeviceAttributeMatcherOpLessThanOrEqual:
		return k8sutilcel.OpLessOrEqual
	default:
		return k8sutilcel.OpEqual
	}
}

// DRADeviceAttributeMatcher defines the matcher expression for a DRA device attribute.
type DRADeviceAttributeMatcher struct {
	// Key is the name of the device attribute to match.
	// This is either a qualified name or a simple name.
	// If it is a simple name, then it is assumed to be prefixed with the DRA driver name.
	// Eg: "gpu.nvidia.com/productName" is equivalent to "productName" if the driver name is "gpu.nvidia.com". Otherwise they're treated as 2 different attributes.
	// +kubebuilder:validation:MaxLength=64
	Key string `json:"key"`
	// Op is the operator to use for matching the device attribute. Supported operators are:
	// * Equal: The device attribute value must be equal to the value specified in the matcher.
	// * NotEqual: The device attribute value must not be equal to the value specified in the matcher.
	// * GreaterThan: The device attribute value must be greater than the value specified in the matcher.
	// * GreaterThanOrEqual: The device attribute value must be greater than or equal to the value specified in the matcher.
	// * LessThan: The device attribute value must be less than the value specified in the matcher.
	// * LessThanOrEqual: The device attribute value must be less than or equal to the value specified in the matcher.
	//
	// +kubebuilder:validation:Enum=Equal;NotEqual;GreaterThan;GreaterThanOrEqual;LessThan;LessThanOrEqual
	// +kubebuilder:default=Equal
	Op DRADeviceAttributeMatcherOp `json:"op"`
	// Value is the value to match the device attribute against.
	Value *DRADeviceAttributeMatcherValue `json:"value,omitempty"`
}

func (d *DRADeviceAttributeMatcher) GetCELExpression(driverName string) (string, error) {
	domain, name := k8sutil.SplitQualifiedName(d.Key, driverName)
	attrKey := fmt.Sprintf("device.attributes[%q].%s", domain, name)
	return k8sutilcel.BuildExpr(attrKey, d.Op.GetCELOperator(), d.Value.GetValue(), d.Value.GetValueType())
}

// DRAResourceQuantityMatcherOp defines the operator to use for matching a resource quantity.
type DRAResourceQuantityMatcherOp string

const (
	DRAResourceQuantityMatcherOpEqual DRAResourceQuantityMatcherOp = "Equal"
)

func (d DRAResourceQuantityMatcherOp) GetCELOperator() k8sutilcel.ComparisonOperator {
	switch d {
	case DRAResourceQuantityMatcherOpEqual:
		return k8sutilcel.OpEqual
	default:
		return k8sutilcel.OpEqual
	}
}

// DRAResourceQuantityMatcher defines the matcher expression for a DRA device capacity.
type DRAResourceQuantityMatcher struct {
	// Key is the name of the resource quantity to match.
	// This is either a qualified name or a simple name.
	// If it is a simple name, then it is assumed to be prefixed with the DRA driver name.
	// Eg: "gpu.nvidia.com/memory" is equivalent to "memory" if the driver name is "gpu.nvidia.com". Otherwise they're treated as 2 different attributes.
	// +kubebuilder:validation:MaxLength=64
	Key string `json:"key"`
	// Op is the operator to use for matching the device capacity. Supported operators are:
	// * Equal: The resource quantity value must be equal to the value specified in the matcher.
	//
	// +kubebuilder:validation:Enum=Equal
	// +kubebuilder:default=Equal
	Op DRAResourceQuantityMatcherOp `json:"op"`
	// Value is the quantity to match the device capacity against.
	//
	// +kubebuilder:validation:Required
	Value *apiresource.Quantity `json:"value,omitempty"`
}

func (d *DRAResourceQuantityMatcher) GetCELExpression(driverName string) (string, error) {
	domain, name := k8sutil.SplitQualifiedName(d.Key, driverName)
	attrKey := fmt.Sprintf("device.capacity[%q].%s", domain, name)
	return k8sutilcel.BuildExpr(attrKey, d.Op.GetCELOperator(), d.Value, k8sutilcel.TypeQuantity)
}

type DRADeviceSpec struct {
	// Name is the name of the device request to use in the generated claim spec.
	// Must be a valid DNS_LABEL.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*`
	Name string `json:"name"`
	// Count is the number of devices to request.
	// +kubebuilder:default=1
	Count uint32 `json:"count"`
	// DeviceClassName references a specific DeviceClass to inherit configuration and selectors from.
	// +kubebuilder:default=gpu.nvidia.com
	DeviceClassName string `json:"deviceClassName"`
	// DriverName is the name of the DRA driver providing the capacity information.
	// Must be a DNS subdomain.
	//
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*`
	// +kubebuilder:default=gpu.nvidia.com
	DriverName string `json:"driverName,omitempty"`
	// MatchAttributes defines the criteria which must be satisfied by the device attributes of a device.
	// +kubebuilder:validation:MaxSize=20
	MatchAttributes []DRADeviceAttributeMatcher `json:"matchAttributes,omitempty"`
	// MatchCapacity defines the criteria which must be satisfied by the device capacity of a device.
	// +kubebuilder:validation:MaxSize=12
	MatchCapacity []DRAResourceQuantityMatcher `json:"matchCapacity,omitempty"`
}

// DRAClaimSpec defines the spec for generating a DRA resource claim/resource claim template.
type DRAClaimSpec struct {
	// GenerateName is an optional name prefix to use for generating the resource claim/resource claim template.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=16
	GenerateName string `json:"generateName,omitempty"`
	// +kubebuilder:validation:MinSize=1
	Devices []DRADeviceSpec `json:"devices"`
	// TODO: Warn that if set to false, then this NIMService cannot be scaled up.
	IsTemplate *bool `json:"isTemplate,omitempty"`
}

func (d *DRAClaimSpec) IsTemplateSpec() bool {
	return d.IsTemplate != nil && *d.IsTemplate
}

func (d *DRAClaimSpec) GetNamePrefix() string {
	namePrefix := d.GenerateName
	if namePrefix != "" {
		return namePrefix
	}
	if d.IsTemplateSpec() {
		return "claimtemplate"
	}
	return "claim"
}

// DRAResourceStatus defines the status of the DRAResource.
// +kubebuilder:validation:XValidation:rule="has(self.resourceClaimStatus) != has(self.resourceClaimTemplateStatus)",message="exactly one of resourceClaimStatus and resourceClaimTemplateStatus must be set."
type DRAResourceStatus struct {
	// Name is the pod claim name referenced in the pod spec as `spec.resourceClaims[].name` for this DRA resource.
	Name string `json:"name"`
	// ResourceClaimStatus is the status of the resource claim in this DRA resource.
	//
	// Exactly one of resourceClaimStatus and resourceClaimTemplateStatus will be set.
	ResourceClaimStatus *DRAResourceClaimStatusInfo `json:"resourceClaimStatus,omitempty"`
	// ResourceClaimTemplateStatus is the status of the resource claim template in this DRA resource.
	//
	// Exactly one of resourceClaimStatus and resourceClaimTemplateStatus will be set.
	ResourceClaimTemplateStatus *DRAResourceClaimTemplateStatusInfo `json:"resourceClaimTemplateStatus,omitempty"`
}

// DRAResourceClaimStatusInfo defines the status of a ResourceClaim referenced in the DRAResource.
type DRAResourceClaimStatusInfo struct {
	// Name is the name of the ResourceClaim.
	Name string `json:"name"`
	// State is the state of the ResourceClaim.
	// * pending: the resource claim is pending allocation.
	// * deleted: the resource claim has a deletion timestamp set but is not yet finalized.
	// * allocated: the resource claim is allocated to a pod.
	// * reserved: the resource claim is consumed by a pod.
	// This field will have one or more of the above values depending on the status of the resource claim.
	//
	// +kubebuilder:validation:default=pending
	State string `json:"state"`
}

// DRAResourceClaimTemplateStatusInfo defines the status of a ResourceClaimTemplate referenced in the DRAResource.
type DRAResourceClaimTemplateStatusInfo struct {
	// Name is the name of the resource claim template.
	Name string `json:"name"`
	// ResourceClaimStatuses is the statuses of the generated resource claims from this resource claim template.
	ResourceClaimStatuses []DRAResourceClaimStatusInfo `json:"resourceClaimStatuses,omitempty"`
}
