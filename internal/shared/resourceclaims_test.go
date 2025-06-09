package shared

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
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
			return fmt.Sprintf("%s%s-%d-%s", podClaimNamePrefix, nimServiceNameHash, fieldIdx, claimHash)
		}

		It("should generate unique name for ResourceClaimName", func() {
			resource := &appsv1alpha1.DRAResource{
				ResourceClaimName: ptr.To("test-claim"),
			}

			name1 := generateUniquePodClaimName(nameCache, nimServiceName, resource)
			Expect(name1).To(HavePrefix(expectedPrefix(testClaimHash, 0)))
			Expect(name1).To(HaveSuffix("-1"))
		})

		It("should generate unique name for ResourceClaimTemplateName", func() {
			resource := &appsv1alpha1.DRAResource{
				ResourceClaimTemplateName: ptr.To("test-template"),
			}

			name1 := generateUniquePodClaimName(nameCache, nimServiceName, resource)
			Expect(name1).To(HavePrefix(expectedPrefix(testTemplateHash, 1)))
			Expect(name1).To(HaveSuffix("-1"))
		})

		It("should handle different templateswith same name", func() {
			resource1 := &appsv1alpha1.DRAResource{
				ResourceClaimTemplateName: ptr.To("test-template"),
			}
			resource2 := &appsv1alpha1.DRAResource{
				ResourceClaimTemplateName: ptr.To("test-template"),
			}

			name1 := generateUniquePodClaimName(nameCache, nimServiceName, resource1)
			name2 := generateUniquePodClaimName(nameCache, nimServiceName, resource2)

			Expect(name1).ToNot(Equal(name2))
			Expect(name1).To(HavePrefix(expectedPrefix(testTemplateHash, 1)))
			Expect(name1).To(HaveSuffix("-1"))
			Expect(name2).To(HavePrefix(expectedPrefix(testTemplateHash, 1)))
			Expect(name2).To(HaveSuffix("-2"))
		})

		It("should handle claims and templates with different names", func() {
			resource1 := &appsv1alpha1.DRAResource{
				ResourceClaimName: ptr.To("test-claim-1"),
			}
			resource2 := &appsv1alpha1.DRAResource{
				ResourceClaimTemplateName: ptr.To("test-template"),
			}

			name1 := generateUniquePodClaimName(nameCache, nimServiceName, resource1)
			name2 := generateUniquePodClaimName(nameCache, nimServiceName, resource2)

			Expect(name1).ToNot(Equal(name2))
			Expect(name1).To(HavePrefix(expectedPrefix(testClaim1Hash, 0)))
			Expect(name1).To(HaveSuffix("-1"))
			Expect(name2).To(HavePrefix(expectedPrefix(testTemplateHash, 1)))
			Expect(name2).To(HaveSuffix("-1"))
		})

		It("should handle claims and templates with same name", func() {
			resource1 := &appsv1alpha1.DRAResource{
				ResourceClaimName: ptr.To("test-claim"),
			}
			resource2 := &appsv1alpha1.DRAResource{
				ResourceClaimTemplateName: ptr.To("test-claim"),
			}

			name1 := generateUniquePodClaimName(nameCache, nimServiceName, resource1)
			name2 := generateUniquePodClaimName(nameCache, nimServiceName, resource2)

			// The names should be different due to the counter, but the prefix should be the same
			Expect(name1).ToNot(Equal(name2))
			Expect(name1).To(HavePrefix(expectedPrefix(testClaimHash, 0)))
			Expect(name1).To(HaveSuffix("-1"))
			Expect(name2).To(HavePrefix(expectedPrefix(testClaimHash, 1)))
			Expect(name2).To(HaveSuffix("-1"))
		})
	})
})
