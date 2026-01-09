package k8sutil

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	resourcev1 "k8s.io/api/resource/v1"

	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

var _ = Describe("Test ResourceClaims k8s utils", func() {
	var (
		s          *runtime.Scheme
		fakeClient client.Client
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		s = scheme.Scheme
		err := resourcev1.AddToScheme(s)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("Test ListResourceClaimsByPodClaimName", func() {
		It("should return all matching claims", func() {
			claims := []resourcev1.ResourceClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "claim-1",
						Namespace: "default",
						Annotations: map[string]string{
							utils.DRAPodClaimNameAnnotationKey: "test-pod-claim",
						},
						CreationTimestamp: metav1.Now(),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "claim-2",
						Namespace: "default",
						Annotations: map[string]string{
							utils.DRAPodClaimNameAnnotationKey: "test-pod-claim",
						},
						CreationTimestamp: metav1.NewTime(time.Now().Add(time.Hour)),
					},
				},
			}
			objs := make([]client.Object, len(claims))
			for i := range claims {
				objs[i] = &claims[i]
			}
			fakeClient = fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()

			result, err := ListResourceClaimsByPodClaimName(ctx, fakeClient, "default", "test-pod-claim")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(2))
			Expect(result[0].Name).To(Equal("claim-1"))
			Expect(result[1].Name).To(Equal("claim-2"))
		})

		It("should return an empty list when no claims match", func() {
			claims := []resourcev1.ResourceClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "claim-1",
						Namespace: "default",
						Annotations: map[string]string{
							utils.DRAPodClaimNameAnnotationKey: "test-pod-claim",
						},
					},
				},
			}
			objs := make([]client.Object, len(claims))
			for i := range claims {
				objs[i] = &claims[i]
			}
			fakeClient = fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()

			result, err := ListResourceClaimsByPodClaimName(ctx, fakeClient, "default", "non-existent-pod-claim")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		It("should return an empty list when no claims exist", func() {
			fakeClient = fake.NewClientBuilder().WithScheme(s).Build()

			result, err := ListResourceClaimsByPodClaimName(ctx, fakeClient, "default", "test-pod-claim")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		It("should return claims sorted by creation timestamp", func() {
			claims := []resourcev1.ResourceClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "newer-claim",
						Namespace: "default",
						Annotations: map[string]string{
							utils.DRAPodClaimNameAnnotationKey: "test-pod-claim",
						},
						CreationTimestamp: metav1.NewTime(time.Now().Add(time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "older-claim",
						Namespace: "default",
						Annotations: map[string]string{
							utils.DRAPodClaimNameAnnotationKey: "test-pod-claim",
						},
						CreationTimestamp: metav1.Now(),
					},
				},
			}
			objs := make([]client.Object, len(claims))
			for i := range claims {
				objs[i] = &claims[i]
			}
			fakeClient = fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()

			result, err := ListResourceClaimsByPodClaimName(ctx, fakeClient, "default", "test-pod-claim")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(2))
			Expect(result[0].Name).To(Equal("older-claim"))
			Expect(result[1].Name).To(Equal("newer-claim"))
		})

		It("should return an empty list when podClaimName is empty", func() {
			claims := []resourcev1.ResourceClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "claim-1",
						Namespace: "default",
						Annotations: map[string]string{
							utils.DRAPodClaimNameAnnotationKey: "test-pod-claim",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "claim-2",
						Namespace: "default",
						Annotations: map[string]string{
							utils.DRAPodClaimNameAnnotationKey: "another-pod-claim",
						},
					},
				},
			}
			objs := make([]client.Object, len(claims))
			for i := range claims {
				objs[i] = &claims[i]
			}
			fakeClient = fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()

			result, err := ListResourceClaimsByPodClaimName(ctx, fakeClient, "default", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		It("should return matching claims from all namespaces when namespace is empty", func() {
			claims := []resourcev1.ResourceClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "claim-1",
						Namespace: "default",
						Annotations: map[string]string{
							utils.DRAPodClaimNameAnnotationKey: "test-pod-claim",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "claim-2",
						Namespace: "test-ns",
						Annotations: map[string]string{
							utils.DRAPodClaimNameAnnotationKey: "test-pod-claim",
						},
					},
				},
			}
			objs := make([]client.Object, len(claims))
			for i := range claims {
				objs[i] = &claims[i]
			}
			fakeClient = fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()

			result, err := ListResourceClaimsByPodClaimName(ctx, fakeClient, "", "test-pod-claim")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(2))
			Expect(result[0].Name).To(Equal("claim-1"))
			Expect(result[0].Namespace).To(Equal("default"))
			Expect(result[1].Name).To(Equal("claim-2"))
			Expect(result[1].Namespace).To(Equal("test-ns"))
		})
	})

	DescribeTable("getDRAResourceClaimState",
		func(claim *resourcev1.ResourceClaim, expectedState string) {
			state := GetResourceClaimState(claim)
			Expect(state).To(Equal(expectedState))
		},
		Entry("pending claim",
			&resourcev1.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-claim",
					Namespace:         "test-ns",
					CreationTimestamp: metav1.Time{},
				},
				Status: resourcev1.ResourceClaimStatus{},
			},
			"pending",
		),
		Entry("allocated and reserved claim",
			&resourcev1.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-claim",
					Namespace:         "test-ns",
					CreationTimestamp: metav1.Time{},
				},
				Status: resourcev1.ResourceClaimStatus{
					Allocation: &resourcev1.AllocationResult{},
					ReservedFor: []resourcev1.ResourceClaimConsumerReference{
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
			&resourcev1.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-claim",
					Namespace:         "test-ns",
					CreationTimestamp: metav1.Time{},
					DeletionTimestamp: &metav1.Time{},
					Finalizers: []string{
						resourcev1.Finalizer,
					},
				},
				Status: resourcev1.ResourceClaimStatus{},
			},
			"deleted",
		),
		Entry("deleted, allocated and reserved claim",
			&resourcev1.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-claim",
					Namespace:         "test-ns",
					CreationTimestamp: metav1.Time{},
					DeletionTimestamp: &metav1.Time{},
					Finalizers: []string{
						resourcev1.Finalizer,
					},
				},
				Status: resourcev1.ResourceClaimStatus{
					Allocation: &resourcev1.AllocationResult{},
					ReservedFor: []resourcev1.ResourceClaimConsumerReference{
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
