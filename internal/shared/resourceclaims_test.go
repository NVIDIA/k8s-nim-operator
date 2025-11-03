package shared

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	resourcev1beta2 "k8s.io/api/resource/v1beta2"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

var _ = Describe("DRA resourceclaim tests", func() {
	DescribeTable("should handle resource claims correctly",
		func(containers []corev1.Container, draResources []NamedDRAResource, expected []corev1.Container) {
			// Create a copy of the containers to avoid modifying the test data
			containersCopy := make([]corev1.Container, len(containers))
			copy(containersCopy, containers)

			UpdateContainerResourceClaims(containersCopy, draResources)
			Expect(containersCopy).To(Equal(expected))
		},
		Entry("add new resource claim to empty containers",
			[]corev1.Container{
				{
					Name: "container1",
					Resources: corev1.ResourceRequirements{
						Claims: []corev1.ResourceClaim{},
					},
				},
			},
			[]NamedDRAResource{
				{
					Name: "claim1",
				},
			},
			[]corev1.Container{
				{
					Name: "container1",
					Resources: corev1.ResourceRequirements{
						Claims: []corev1.ResourceClaim{
							{
								Name: "claim1",
							},
						},
					},
				},
			},
		),
		Entry("add new resource claim to containers with existing claims",
			[]corev1.Container{
				{
					Name: "container1",
					Resources: corev1.ResourceRequirements{
						Claims: []corev1.ResourceClaim{
							{
								Name: "existing-claim",
							},
						},
					},
				},
			},
			[]NamedDRAResource{
				{
					Name: "new-claim",
				},
			},
			[]corev1.Container{
				{
					Name: "container1",
					Resources: corev1.ResourceRequirements{
						Claims: []corev1.ResourceClaim{
							{
								Name: "existing-claim",
							},
							{
								Name: "new-claim",
							},
						},
					},
				},
			},
		),
		Entry("do not add duplicate resource claims",
			[]corev1.Container{
				{
					Name: "container1",
					Resources: corev1.ResourceRequirements{
						Claims: []corev1.ResourceClaim{
							{
								Name: "existing-claim",
							},
						},
					},
				},
			},
			[]NamedDRAResource{
				{
					Name: "existing-claim",
				},
			},
			[]corev1.Container{
				{
					Name: "container1",
					Resources: corev1.ResourceRequirements{
						Claims: []corev1.ResourceClaim{
							{
								Name: "existing-claim",
							},
						},
					},
				},
			},
		),
		Entry("add multiple resource claims to multiple containers",
			[]corev1.Container{
				{
					Name: "container1",
					Resources: corev1.ResourceRequirements{
						Claims: []corev1.ResourceClaim{},
					},
				},
				{
					Name: "container2",
					Resources: corev1.ResourceRequirements{
						Claims: []corev1.ResourceClaim{},
					},
				},
			},
			[]NamedDRAResource{
				{
					Name: "claim1",
				},
				{
					Name: "claim2",
				},
			},
			[]corev1.Container{
				{
					Name: "container1",
					Resources: corev1.ResourceRequirements{
						Claims: []corev1.ResourceClaim{
							{
								Name: "claim1",
							},
							{
								Name: "claim2",
							},
						},
					},
				},
				{
					Name: "container2",
					Resources: corev1.ResourceRequirements{
						Claims: []corev1.ResourceClaim{
							{
								Name: "claim1",
							},
							{
								Name: "claim2",
							},
						},
					},
				},
			},
		),
		Entry("containers with different claims should not get duplicates",
			[]corev1.Container{
				{
					Name: "container1",
					Resources: corev1.ResourceRequirements{
						Claims: []corev1.ResourceClaim{
							{
								Name: "claim1",
							},
						},
					},
				},
				{
					Name: "container2",
					Resources: corev1.ResourceRequirements{
						Claims: []corev1.ResourceClaim{
							{
								Name: "claim2",
							},
						},
					},
				},
			},
			[]NamedDRAResource{
				{
					Name: "claim1",
				},
				{
					Name: "claim2",
				},
			},
			[]corev1.Container{
				{
					Name: "container1",
					Resources: corev1.ResourceRequirements{
						Claims: []corev1.ResourceClaim{
							{
								Name: "claim1",
							},
						},
					},
				},
				{
					Name: "container2",
					Resources: corev1.ResourceRequirements{
						Claims: []corev1.ResourceClaim{
							{
								Name: "claim2",
							},
						},
					},
				},
			},
		),
		Entry("add resource claims with requests",
			[]corev1.Container{
				{
					Name: "container1",
					Resources: corev1.ResourceRequirements{
						Claims: []corev1.ResourceClaim{},
					},
				},
			},
			[]NamedDRAResource{
				{
					Name: "claim1",
					DRAResource: appsv1alpha1.DRAResource{
						Requests: []string{
							"request1",
							"request2",
						},
					},
				},
			},
			[]corev1.Container{
				{
					Name: "container1",
					Resources: corev1.ResourceRequirements{
						Claims: []corev1.ResourceClaim{
							{
								Name:    "claim1",
								Request: "request1",
							},
							{
								Name:    "claim1",
								Request: "request2",
							},
						},
					},
				},
			},
		),
	)

	Describe("generateUniquePodClaimName", func() {
		var nameCache map[string]int
		const nimServiceName = "test-service"
		const nimServiceNameHash = "8568b4fb55" // hash of "test-service"
		const testClaimHash = "9f8c8d9fb"       // hash of "test-claim"
		const testTemplateHash = "5cb744997d"   // hash of "test-template"
		const testClaim1Hash = "6454fc76cd"     // hash of "test-claim-1"

		BeforeEach(func() {
			nameCache = make(map[string]int)
		})

		// Helper function to calculate expected prefix
		expectedPrefix := func(claimHash string, fieldIdx int) string {
			return fmt.Sprintf("%s-%s-%d-%s", podClaimNamePrefix, nimServiceNameHash, fieldIdx, claimHash)
		}

		It("should generate unique name for ResourceClaimName", func() {
			resourceName := "test-claim"

			name1 := generateUniquePodClaimName(nameCache, nimServiceName, resourceName, DRAResourceFieldTypeClaim)
			Expect(name1).To(HavePrefix(expectedPrefix(testClaimHash, 0)))
			Expect(name1).To(HaveSuffix("-0"))
		})

		It("should generate unique name for ResourceClaimTemplateName", func() {
			resourceName := "test-template"

			name1 := generateUniquePodClaimName(nameCache, nimServiceName, resourceName, DRAResourceFieldTypeClaimTemplate)
			Expect(name1).To(HavePrefix(expectedPrefix(testTemplateHash, 1)))
			Expect(name1).To(HaveSuffix("-0"))
		})

		It("should handle different templates with same name", func() {
			resourceName := "test-template"

			name1 := generateUniquePodClaimName(nameCache, nimServiceName, resourceName, DRAResourceFieldTypeClaimTemplate)
			name2 := generateUniquePodClaimName(nameCache, nimServiceName, resourceName, DRAResourceFieldTypeClaimTemplate)

			Expect(name1).ToNot(Equal(name2))
			Expect(name1).To(HavePrefix(expectedPrefix(testTemplateHash, 1)))
			Expect(name1).To(HaveSuffix("-0"))
			Expect(name2).To(HavePrefix(expectedPrefix(testTemplateHash, 1)))
			Expect(name2).To(HaveSuffix("-1"))
		})

		It("should handle claims and templates with different names", func() {
			resourceName1 := "test-claim-1"
			resourceName2 := "test-template"

			name1 := generateUniquePodClaimName(nameCache, nimServiceName, resourceName1, DRAResourceFieldTypeClaim)
			name2 := generateUniquePodClaimName(nameCache, nimServiceName, resourceName2, DRAResourceFieldTypeClaimTemplate)

			Expect(name1).ToNot(Equal(name2))
			Expect(name1).To(HavePrefix(expectedPrefix(testClaim1Hash, 0)))
			Expect(name1).To(HaveSuffix("-0"))
			Expect(name2).To(HavePrefix(expectedPrefix(testTemplateHash, 1)))
			Expect(name2).To(HaveSuffix("-0"))
		})

		It("should handle claims and templates with same name", func() {
			resourceName := "test-claim"

			name1 := generateUniquePodClaimName(nameCache, nimServiceName, resourceName, DRAResourceFieldTypeClaim)
			name2 := generateUniquePodClaimName(nameCache, nimServiceName, resourceName, DRAResourceFieldTypeClaimTemplate)

			// The names should be different due to the counter, but the prefix should be the same
			Expect(name1).ToNot(Equal(name2))
			Expect(name1).To(HavePrefix(expectedPrefix(testClaimHash, 0)))
			Expect(name1).To(HaveSuffix("-0"))
			Expect(name2).To(HavePrefix(expectedPrefix(testClaimHash, 1)))
			Expect(name2).To(HaveSuffix("-0"))
		})
	})

	Describe("generateUniqueDRAResourceName", func() {
		const nimServiceName = "test-service"
		const nimServiceNameHash = "8568b4fb55" // hash of "test-service"

		It("should generate correct DRA resource names", func() {
			namePrefix := "claim"
			result := generateUniqueDRAResourceName(nimServiceName, namePrefix, 0)
			expected := fmt.Sprintf("%s-%s-0", namePrefix, nimServiceNameHash)
			Expect(result).To(Equal(expected))
		})

		It("should handle different NIM service names", func() {
			namePrefix := "claim"
			result1 := generateUniqueDRAResourceName("service-1", namePrefix, 0)
			result2 := generateUniqueDRAResourceName("service-2", namePrefix, 0)

			// Should have different hashes but same structure
			Expect(result1).To(HavePrefix("claim-"))
			Expect(result2).To(HavePrefix("claim-"))
			Expect(result1).ToNot(Equal(result2))
			Expect(result1).To(HaveSuffix("-0"))
			Expect(result2).To(HaveSuffix("-0"))
		})

		It("should handle different indices", func() {
			namePrefix := "template"
			result1 := generateUniqueDRAResourceName(nimServiceName, namePrefix, 0)
			result2 := generateUniqueDRAResourceName(nimServiceName, namePrefix, 1)

			Expect(result1).To(HavePrefix("template-"))
			Expect(result2).To(HavePrefix("template-"))
			Expect(result1).ToNot(Equal(result2))
			Expect(result1).To(HaveSuffix("-0"))
			Expect(result2).To(HaveSuffix("-1"))
		})

		It("should generate correct compute domain resource name", func() {
			name := generateUniqueDRAResourceName(nimServiceName, "compute-domain", noIndexSuffix)
			Expect(name).To(Equal("compute-domain-8568b4fb55"))
		})
	})

	Describe("generateDRAResourceStatus", func() {
		var (
			ctx    context.Context
			client client.Client
			ns     string
		)

		BeforeEach(func() {
			ctx = context.Background()
			ns = "test-ns"
			scheme := runtime.NewScheme()
			Expect(resourcev1beta2.AddToScheme(scheme)).To(Succeed())
			client = fake.NewClientBuilder().WithScheme(scheme).Build()
		})

		It("should return a single claim status when a resource claim name is provided", func() {
			// Setup test claims
			claim := &resourcev1beta2.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-claim",
					Namespace: ns,
				},
			}
			Expect(client.Create(ctx, claim)).To(Succeed())

			resource := &NamedDRAResource{
				Name: "claim-8568b4fb55-0-9f8c8d9fb-0",
				DRAResource: appsv1alpha1.DRAResource{
					ResourceClaimName: ptr.To("test-claim"),
				},
				FieldType:    DRAResourceFieldTypeClaim,
				ResourceName: "test-claim",
			}

			status, err := generateDRAResourceStatus(ctx, client, ns, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal(&appsv1alpha1.DRAResourceStatus{
				Name: "claim-8568b4fb55-0-9f8c8d9fb-0",
				ResourceClaimStatus: &appsv1alpha1.DRAResourceClaimStatusInfo{
					Name:  "test-claim",
					State: "pending",
				},
			}))
		})

		It("should return status for all matching claims for a resource claim template", func() {
			// Setup test claims
			claim1 := &resourcev1beta2.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "template-generated-claim-1",
					Namespace: ns,
					Annotations: map[string]string{
						utils.DRAPodClaimNameAnnotationKey: "claim-8568b4fb55-1-5cb744997d-0",
					},
				},
			}
			claim2 := &resourcev1beta2.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "template-generated-claim-2",
					Namespace: ns,
					Annotations: map[string]string{
						utils.DRAPodClaimNameAnnotationKey: "claim-8568b4fb55-1-5cb744997d-0",
					},
				},
			}
			Expect(client.Create(ctx, claim1)).To(Succeed())
			Expect(client.Create(ctx, claim2)).To(Succeed())

			resource := &NamedDRAResource{
				Name: "claim-8568b4fb55-1-5cb744997d-0",
				DRAResource: appsv1alpha1.DRAResource{
					ResourceClaimTemplateName: ptr.To("test-template"),
				},
				FieldType:    DRAResourceFieldTypeClaimTemplate,
				ResourceName: "test-template",
			}

			status, err := generateDRAResourceStatus(ctx, client, ns, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal(&appsv1alpha1.DRAResourceStatus{
				Name: "claim-8568b4fb55-1-5cb744997d-0",
				ResourceClaimTemplateStatus: &appsv1alpha1.DRAResourceClaimTemplateStatusInfo{
					Name: "test-template",
					ResourceClaimStatuses: []appsv1alpha1.DRAResourceClaimStatusInfo{
						{
							Name:  "template-generated-claim-1",
							State: "pending",
						},
						{
							Name:  "template-generated-claim-2",
							State: "pending",
						},
					},
				},
			}))
		})

		It("should return appropriate state for claims of a resource claim template that are being deleted", func() {
			// Setup test claims
			claim1 := &resourcev1beta2.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "template-generated-claim-1",
					Namespace: ns,
					Annotations: map[string]string{
						utils.DRAPodClaimNameAnnotationKey: "claim-8568b4fb55-1-5cb744997d-0",
					},
				},
			}
			claim2 := &resourcev1beta2.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "template-generated-claim-2",
					Namespace: ns,
					Finalizers: []string{
						resourcev1beta2.Finalizer,
					},
					Annotations: map[string]string{
						utils.DRAPodClaimNameAnnotationKey: "claim-8568b4fb55-1-5cb744997d-0",
					},
				},
			}
			Expect(client.Create(ctx, claim1)).To(Succeed())
			Expect(client.Create(ctx, claim2)).To(Succeed())
			// Delete the claim to set the deletion timestamp (protected from going away by finalizer)
			Expect(client.Delete(ctx, claim2)).To(Succeed())

			resource := &NamedDRAResource{
				Name: "claim-8568b4fb55-1-5cb744997d-0",
				DRAResource: appsv1alpha1.DRAResource{
					ResourceClaimTemplateName: ptr.To("test-template"),
				},
				FieldType:    DRAResourceFieldTypeClaimTemplate,
				ResourceName: "test-template",
			}
			status, err := generateDRAResourceStatus(ctx, client, ns, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal(&appsv1alpha1.DRAResourceStatus{
				Name: "claim-8568b4fb55-1-5cb744997d-0",
				ResourceClaimTemplateStatus: &appsv1alpha1.DRAResourceClaimTemplateStatusInfo{
					Name: "test-template",
					ResourceClaimStatuses: []appsv1alpha1.DRAResourceClaimStatusInfo{
						{
							Name:  "template-generated-claim-1",
							State: "pending",
						},
						{
							Name:  "template-generated-claim-2",
							State: "deleted",
						},
					},
				},
			}))
		})

		It("should return appropriate state for claims of a resource claim template that is allocated and in-use", func() {
			// Setup test claims
			claim1 := &resourcev1beta2.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "template-generated-claim-1",
					Namespace: ns,
					Annotations: map[string]string{
						utils.DRAPodClaimNameAnnotationKey: "claim-8568b4fb55-1-5cb744997d-0",
					},
				},
				Status: resourcev1beta2.ResourceClaimStatus{
					Allocation: &resourcev1beta2.AllocationResult{},
					ReservedFor: []resourcev1beta2.ResourceClaimConsumerReference{
						{
							Name:     "pod-test",
							Resource: "pods",
							UID:      types.UID("pod-test"),
						},
					},
				},
			}
			Expect(client.Create(ctx, claim1)).To(Succeed())

			resource := &NamedDRAResource{
				Name: "claim-8568b4fb55-1-5cb744997d-0",
				DRAResource: appsv1alpha1.DRAResource{
					ResourceClaimTemplateName: ptr.To("test-template"),
				},
				FieldType:    DRAResourceFieldTypeClaimTemplate,
				ResourceName: "test-template",
			}
			status, err := generateDRAResourceStatus(ctx, client, ns, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal(&appsv1alpha1.DRAResourceStatus{
				Name: "claim-8568b4fb55-1-5cb744997d-0",
				ResourceClaimTemplateStatus: &appsv1alpha1.DRAResourceClaimTemplateStatusInfo{
					Name: "test-template",
					ResourceClaimStatuses: []appsv1alpha1.DRAResourceClaimStatusInfo{
						{
							Name:  "template-generated-claim-1",
							State: "allocated,reserved",
						},
					},
				},
			}))
		})

		It("should return nil status for non-matching claims for a resource claim template", func() {
			// Setup test claim with different pod claim name
			claim := &resourcev1beta2.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "template-generated-claim-1",
					Namespace: ns,
					Annotations: map[string]string{
						utils.DRAPodClaimNameAnnotationKey: "different-nimservice-claim",
					},
				},
			}
			Expect(client.Create(ctx, claim)).To(Succeed())

			resource := &NamedDRAResource{
				Name: "claim-8568b4fb55-1-5cb744997d-0",
				DRAResource: appsv1alpha1.DRAResource{
					ResourceClaimTemplateName: ptr.To("test-template"),
				},
				FieldType:    DRAResourceFieldTypeClaimTemplate,
				ResourceName: "test-template",
			}

			status, err := generateDRAResourceStatus(ctx, client, ns, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal(&appsv1alpha1.DRAResourceStatus{
				Name: "claim-8568b4fb55-1-5cb744997d-0",
				ResourceClaimTemplateStatus: &appsv1alpha1.DRAResourceClaimTemplateStatusInfo{
					Name: "test-template",
				},
			}))
		})

		It("should return an error when the LIST call fails", func() {
			resource := &NamedDRAResource{
				Name: "claim-8568b4fb55-1-5cb744997d-0",
				DRAResource: appsv1alpha1.DRAResource{
					ResourceClaimTemplateName: ptr.To("test-template"),
				},
				FieldType:    DRAResourceFieldTypeClaimTemplate,
				ResourceName: "test-template",
			}

			// Fake client with no resourcev1beta2scheme
			errorClient := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()

			status, err := generateDRAResourceStatus(ctx, errorClient, ns, resource)
			Expect(err).To(HaveOccurred())
			Expect(status).To(BeNil())
		})
	})

	Describe("GetDRADeviceCELExpressions", func() {
		It("should return driver expression when only driver is specified", func() {
			device := appsv1alpha1.DRADeviceSpec{
				DriverName: "gpu.nvidia.com",
			}

			expressions, err := GetDRADeviceCELExpressions(device)
			Expect(err).NotTo(HaveOccurred())
			Expect(expressions).To(HaveLen(1))
			Expect(expressions[0]).To(Equal(`device.driver == "gpu.nvidia.com"`))
		})

		It("should return driver expression and custom CEL expressions when provided", func() {
			device := appsv1alpha1.DRADeviceSpec{
				DriverName: "gpu.nvidia.com",
				CELExpressions: []string{
					`device.attributes["gpu.nvidia.com"].foo >= 8`,
					`device.attributes["gpu.nvidia.com"].bar.compareTo(semver('7.0')) >= 0`,
				},
			}

			expressions, err := GetDRADeviceCELExpressions(device)
			Expect(err).NotTo(HaveOccurred())
			Expect(expressions).To(HaveLen(3))
			Expect(expressions[0]).To(Equal(`device.driver == "gpu.nvidia.com"`))
			Expect(expressions[1]).To(Equal(`device.attributes["gpu.nvidia.com"].foo >= 8`))
			Expect(expressions[2]).To(Equal(`device.attributes["gpu.nvidia.com"].bar.compareTo(semver('7.0')) >= 0`))
		})

		It("should return error when CEL expressions and attribute selectors are both set", func() {
			device := appsv1alpha1.DRADeviceSpec{
				DriverName: "gpu.nvidia.com",
				CELExpressions: []string{
					`device.attributes["gpu.nvidia.com"].foo >= 8`,
				},
				AttributeSelectors: []appsv1alpha1.DRADeviceAttributeSelector{
					{
						Key: "bar",
						Op:  appsv1alpha1.DRADeviceAttributeSelectorOpGreaterThanOrEqual,
						Value: &appsv1alpha1.DRADeviceAttributeSelectorValue{
							VersionValue: ptr.To("7.0"),
						},
					},
				},
			}

			expressions, err := GetDRADeviceCELExpressions(device)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("CELExpressions must not be set if attributeSelectors or capacitySelectors are set"))
			Expect(expressions).To(BeNil())
		})

		It("should return error when CEL expressions and capacity selectors are both set", func() {
			device := appsv1alpha1.DRADeviceSpec{
				DriverName: "gpu.nvidia.com",
				CELExpressions: []string{
					`device.attributes["gpu.nvidia.com"].foo >= 8`,
				},
				CapacitySelectors: []appsv1alpha1.DRAResourceQuantitySelector{
					{
						Key:   "memory",
						Op:    appsv1alpha1.DRAResourceQuantitySelectorOpGreaterThanOrEqual,
						Value: &apiresource.Quantity{},
					},
				},
			}

			expressions, err := GetDRADeviceCELExpressions(device)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("CELExpressions must not be set if attributeSelectors or capacitySelectors are set"))
			Expect(expressions).To(BeNil())
		})

		It("should return error when CEL expressions, attribute selectors, and capacity selectors are all set", func() {
			device := appsv1alpha1.DRADeviceSpec{
				DriverName: "gpu.nvidia.com",
				CELExpressions: []string{
					`device.attributes["gpu.nvidia.com"].foo >= 8`,
				},
				AttributeSelectors: []appsv1alpha1.DRADeviceAttributeSelector{
					{
						Key: "bar",
						Op:  appsv1alpha1.DRADeviceAttributeSelectorOpGreaterThanOrEqual,
						Value: &appsv1alpha1.DRADeviceAttributeSelectorValue{
							VersionValue: ptr.To("7.0"),
						},
					},
				},
				CapacitySelectors: []appsv1alpha1.DRAResourceQuantitySelector{
					{
						Key:   "memory",
						Op:    appsv1alpha1.DRAResourceQuantitySelectorOpGreaterThanOrEqual,
						Value: &apiresource.Quantity{},
					},
				},
			}

			expressions, err := GetDRADeviceCELExpressions(device)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("CELExpressions must not be set if attributeSelectors or capacitySelectors are set"))
			Expect(expressions).To(BeNil())
		})

		It("should return driver expression and attribute selector expressions when only attribute selectors are set", func() {
			device := appsv1alpha1.DRADeviceSpec{
				DriverName: "gpu.nvidia.com",
				AttributeSelectors: []appsv1alpha1.DRADeviceAttributeSelector{
					{
						Key: "foo",
						Op:  appsv1alpha1.DRADeviceAttributeSelectorOpGreaterThanOrEqual,
						Value: &appsv1alpha1.DRADeviceAttributeSelectorValue{
							IntValue: ptr.To[int32](8),
						},
					},
					{
						Key: "bar",
						Op:  appsv1alpha1.DRADeviceAttributeSelectorOpGreaterThanOrEqual,
						Value: &appsv1alpha1.DRADeviceAttributeSelectorValue{
							VersionValue: ptr.To("7.0"),
						},
					},
				},
			}

			expressions, err := GetDRADeviceCELExpressions(device)
			Expect(err).NotTo(HaveOccurred())
			Expect(expressions).To(HaveLen(3))
			Expect(expressions[0]).To(Equal(`device.driver == "gpu.nvidia.com"`))
			Expect(expressions[1]).To(Equal(`device.attributes["gpu.nvidia.com"].foo >= 8`))
			Expect(expressions[2]).To(Equal(`(device.attributes["gpu.nvidia.com"].bar).compareTo(semver("7.0")) >= 0`))
		})

		It("should return driver expression and capacity selector expressions when only capacity selectors are set", func() {
			device := appsv1alpha1.DRADeviceSpec{
				DriverName: "gpu.nvidia.com",
				CapacitySelectors: []appsv1alpha1.DRAResourceQuantitySelector{
					{
						Key:   "foo",
						Op:    appsv1alpha1.DRAResourceQuantitySelectorOpGreaterThanOrEqual,
						Value: apiresource.NewQuantity(8, apiresource.DecimalSI),
					},
					{
						Key:   "bar",
						Op:    appsv1alpha1.DRAResourceQuantitySelectorOpGreaterThanOrEqual,
						Value: apiresource.NewQuantity(7, apiresource.DecimalSI),
					},
				},
			}

			expressions, err := GetDRADeviceCELExpressions(device)
			Expect(err).NotTo(HaveOccurred())
			Expect(expressions).To(HaveLen(3))
			Expect(expressions[0]).To(Equal(`device.driver == "gpu.nvidia.com"`))
			Expect(expressions[1]).To(Equal(`(device.capacity["gpu.nvidia.com"].foo).compareTo(quantity("8")) >= 0`))
			Expect(expressions[2]).To(Equal(`(device.capacity["gpu.nvidia.com"].bar).compareTo(quantity("7")) >= 0`))
		})

		It("should return driver expression and both attribute and capacity selector expressions when both are set", func() {
			device := appsv1alpha1.DRADeviceSpec{
				DriverName: "gpu.nvidia.com",
				AttributeSelectors: []appsv1alpha1.DRADeviceAttributeSelector{
					{
						Key: "foo",
						Op:  appsv1alpha1.DRADeviceAttributeSelectorOpGreaterThanOrEqual,
						Value: &appsv1alpha1.DRADeviceAttributeSelectorValue{
							IntValue: ptr.To[int32](8),
						},
					},
				},
				CapacitySelectors: []appsv1alpha1.DRAResourceQuantitySelector{
					{
						Key:   "bar",
						Op:    appsv1alpha1.DRAResourceQuantitySelectorOpGreaterThanOrEqual,
						Value: apiresource.NewQuantity(8, apiresource.DecimalSI),
					},
				},
			}

			expressions, err := GetDRADeviceCELExpressions(device)
			Expect(err).NotTo(HaveOccurred())
			Expect(expressions).To(HaveLen(3))
			Expect(expressions[0]).To(Equal(`device.driver == "gpu.nvidia.com"`))
			Expect(expressions[1]).To(Equal(`device.attributes["gpu.nvidia.com"].foo >= 8`))
			Expect(expressions[2]).To(Equal(`(device.capacity["gpu.nvidia.com"].bar).compareTo(quantity("8")) >= 0`))
		})
	})
})
