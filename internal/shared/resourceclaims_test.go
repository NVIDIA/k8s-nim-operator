package shared

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

func TestUpdateContainerResourceClaims(t *testing.T) {
	tests := []struct {
		name         string
		containers   []corev1.Container
		draResources []appsv1alpha1.DRAResource
		expected     []corev1.Container
	}{
		{
			name: "add new resource claim to empty containers",
			containers: []corev1.Container{
				{
					Name: "container1",
					Resources: corev1.ResourceRequirements{
						Claims: []corev1.ResourceClaim{},
					},
				},
			},
			draResources: []appsv1alpha1.DRAResource{
				{
					Name: "claim1",
				},
			},
			expected: []corev1.Container{
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
		},
		{
			name: "add new resource claim to containers with existing claims",
			containers: []corev1.Container{
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
			draResources: []appsv1alpha1.DRAResource{
				{
					Name: "new-claim",
				},
			},
			expected: []corev1.Container{
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
		},
		{
			name: "do not add duplicate resource claims",
			containers: []corev1.Container{
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
			draResources: []appsv1alpha1.DRAResource{
				{
					Name: "existing-claim",
				},
			},
			expected: []corev1.Container{
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
		},
		{
			name: "add multiple resource claims to multiple containers",
			containers: []corev1.Container{
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
			draResources: []appsv1alpha1.DRAResource{
				{
					Name: "claim1",
				},
				{
					Name: "claim2",
				},
			},
			expected: []corev1.Container{
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
		},
		{
			name: "containers with different claims should not get duplicates",
			containers: []corev1.Container{
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
			draResources: []appsv1alpha1.DRAResource{
				{
					Name: "claim1",
				},
				{
					Name: "claim2",
				},
			},
			expected: []corev1.Container{
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
		},
		{
			name: "add resource claims with requests",
			containers: []corev1.Container{
				{
					Name: "container1",
					Resources: corev1.ResourceRequirements{
						Claims: []corev1.ResourceClaim{},
					},
				},
			},
			draResources: []appsv1alpha1.DRAResource{
				{
					Name: "claim1",
					Requests: []string{
						"request1",
						"request2",
					},
				},
			},
			expected: []corev1.Container{
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a copy of the containers to avoid modifying the test data
			containers := make([]corev1.Container, len(tt.containers))
			copy(containers, tt.containers)

			UpdateContainerResourceClaims(containers, tt.draResources)

			// Compare the results
			assert.Equal(t, tt.expected, containers)
		})
	}
}
