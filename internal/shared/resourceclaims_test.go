package shared

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	resourcev1beta2 "k8s.io/api/resource/v1beta2"
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

	Describe("GenerateUniquePodClaimName", func() {
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

			name1 := GenerateUniquePodClaimName(nameCache, nimServiceName, resource)
			Expect(name1).To(HavePrefix(expectedPrefix(testClaimHash, 0)))
			Expect(name1).To(HaveSuffix("-0"))
		})

		It("should generate unique name for ResourceClaimTemplateName", func() {
			resource := &appsv1alpha1.DRAResource{
				ResourceClaimTemplateName: ptr.To("test-template"),
			}

			name1 := GenerateUniquePodClaimName(nameCache, nimServiceName, resource)
			Expect(name1).To(HavePrefix(expectedPrefix(testTemplateHash, 1)))
			Expect(name1).To(HaveSuffix("-0"))
		})

		It("should handle different templateswith same name", func() {
			resource1 := &appsv1alpha1.DRAResource{
				ResourceClaimTemplateName: ptr.To("test-template"),
			}
			resource2 := &appsv1alpha1.DRAResource{
				ResourceClaimTemplateName: ptr.To("test-template"),
			}

			name1 := GenerateUniquePodClaimName(nameCache, nimServiceName, resource1)
			name2 := GenerateUniquePodClaimName(nameCache, nimServiceName, resource2)

			Expect(name1).ToNot(Equal(name2))
			Expect(name1).To(HavePrefix(expectedPrefix(testTemplateHash, 1)))
			Expect(name1).To(HaveSuffix("-0"))
			Expect(name2).To(HavePrefix(expectedPrefix(testTemplateHash, 1)))
			Expect(name2).To(HaveSuffix("-1"))
		})

		It("should handle claims and templates with different names", func() {
			resource1 := &appsv1alpha1.DRAResource{
				ResourceClaimName: ptr.To("test-claim-1"),
			}
			resource2 := &appsv1alpha1.DRAResource{
				ResourceClaimTemplateName: ptr.To("test-template"),
			}

			name1 := GenerateUniquePodClaimName(nameCache, nimServiceName, resource1)
			name2 := GenerateUniquePodClaimName(nameCache, nimServiceName, resource2)

			Expect(name1).ToNot(Equal(name2))
			Expect(name1).To(HavePrefix(expectedPrefix(testClaim1Hash, 0)))
			Expect(name1).To(HaveSuffix("-0"))
			Expect(name2).To(HavePrefix(expectedPrefix(testTemplateHash, 1)))
			Expect(name2).To(HaveSuffix("-0"))
		})

		It("should handle claims and templates with same name", func() {
			resource1 := &appsv1alpha1.DRAResource{
				ResourceClaimName: ptr.To("test-claim"),
			}
			resource2 := &appsv1alpha1.DRAResource{
				ResourceClaimTemplateName: ptr.To("test-claim"),
			}

			name1 := GenerateUniquePodClaimName(nameCache, nimServiceName, resource1)
			name2 := GenerateUniquePodClaimName(nameCache, nimServiceName, resource2)

			// The names should be different due to the counter, but the prefix should be the same
			Expect(name1).ToNot(Equal(name2))
			Expect(name1).To(HavePrefix(expectedPrefix(testClaimHash, 0)))
			Expect(name1).To(HaveSuffix("-0"))
			Expect(name2).To(HavePrefix(expectedPrefix(testClaimHash, 1)))
			Expect(name2).To(HaveSuffix("-0"))
		})
	})

	Describe("generateDRAResourceClaimStatus", func() {
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
			}

			statuses, err := generateDRAResourceClaimStatus(ctx, client, ns, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(statuses).To(Equal([]appsv1alpha1.DRAResourceClaimStatus{
				{
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
						utils.DRAPodClaimNameAnnotationKey: "claim-8568b4fb55-0-5cb744997d-0",
					},
				},
			}
			claim2 := &resourcev1beta2.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "template-generated-claim-2",
					Namespace: ns,
					Annotations: map[string]string{
						utils.DRAPodClaimNameAnnotationKey: "claim-8568b4fb55-0-5cb744997d-0",
					},
				},
			}
			Expect(client.Create(ctx, claim1)).To(Succeed())
			Expect(client.Create(ctx, claim2)).To(Succeed())

			resource := &NamedDRAResource{
				Name: "claim-8568b4fb55-0-5cb744997d-0",
				DRAResource: appsv1alpha1.DRAResource{
					ResourceClaimTemplateName: ptr.To("test-template"),
				},
			}

			statuses, err := generateDRAResourceClaimStatus(ctx, client, ns, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(statuses).To(Equal([]appsv1alpha1.DRAResourceClaimStatus{
				{
					Name:  "template-generated-claim-1",
					State: "pending",
				},
				{
					Name:  "template-generated-claim-2",
					State: "pending",
				},
			}))
		})

		It("should ignore claims for a resource claim template that are being deleted", func() {
			// Setup test claims
			claim1 := &resourcev1beta2.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "template-generated-claim-1",
					Namespace: ns,
					Annotations: map[string]string{
						utils.DRAPodClaimNameAnnotationKey: "claim-8568b4fb55-0-5cb744997d-0",
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
						utils.DRAPodClaimNameAnnotationKey: "claim-8568b4fb55-0-5cb744997d-0",
					},
				},
			}
			Expect(client.Create(ctx, claim1)).To(Succeed())
			Expect(client.Create(ctx, claim2)).To(Succeed())
			// Delete the claim to set the deletion timestamp (protected from going away by finalizer)
			Expect(client.Delete(ctx, claim2)).To(Succeed())

			resource := &NamedDRAResource{
				Name: "claim-8568b4fb55-0-5cb744997d-0",
				DRAResource: appsv1alpha1.DRAResource{
					ResourceClaimTemplateName: ptr.To("test-template"),
				},
			}
			statuses, err := generateDRAResourceClaimStatus(ctx, client, ns, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(statuses).To(Equal([]appsv1alpha1.DRAResourceClaimStatus{
				{
					Name:  "template-generated-claim-1",
					State: "pending",
				},
			}))
		})

		It("should return empty status list for non-matching claims for a resource claim template", func() {
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
				Name: "claim-8568b4fb55-0-5cb744997d-0",
				DRAResource: appsv1alpha1.DRAResource{
					ResourceClaimTemplateName: ptr.To("test-template"),
				},
			}

			statuses, err := generateDRAResourceClaimStatus(ctx, client, ns, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(statuses).To(BeEmpty())
		})

		It("should return an error when the LIST call fails", func() {
			resource := &NamedDRAResource{
				Name: "claim-8568b4fb55-0-5cb744997d-0",
				DRAResource: appsv1alpha1.DRAResource{
					ResourceClaimTemplateName: ptr.To("test-template"),
				},
			}

			// Fake client with no resourcev1beta2scheme
			errorClient := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()

			_, err := generateDRAResourceClaimStatus(ctx, errorClient, ns, resource)
			Expect(err).To(HaveOccurred())
		})
	})

	DescribeTable("getDRAResourceClaimState",
		func(claim *resourcev1beta2.ResourceClaim, expectedState string) {
			state := getDRAResourceClaimState(claim)
			Expect(state).To(Equal(expectedState))
		},
		Entry("pending claim",
			&resourcev1beta2.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-claim",
					Namespace:         "test-ns",
					CreationTimestamp: metav1.Time{},
				},
				Status: resourcev1beta2.ResourceClaimStatus{},
			},
			"pending",
		),
		Entry("allocated and reserved claim",
			&resourcev1beta2.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-claim",
					Namespace:         "test-ns",
					CreationTimestamp: metav1.Time{},
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
			},
			"allocated,reserved",
		),
		Entry("deleted claim",
			&resourcev1beta2.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-claim",
					Namespace:         "test-ns",
					CreationTimestamp: metav1.Time{},
					DeletionTimestamp: &metav1.Time{},
					Finalizers: []string{
						resourcev1beta2.Finalizer,
					},
				},
				Status: resourcev1beta2.ResourceClaimStatus{},
			},
			"deleted",
		),
		Entry("deleted, allocated and reserved claim",
			&resourcev1beta2.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-claim",
					Namespace:         "test-ns",
					CreationTimestamp: metav1.Time{},
					DeletionTimestamp: &metav1.Time{},
					Finalizers: []string{
						resourcev1beta2.Finalizer,
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
			},
			"deleted,allocated,reserved",
		),
	)
})
