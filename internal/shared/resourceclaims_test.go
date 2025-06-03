package shared

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestUpdateContainerResourceClaims(t *testing.T) {
	tests := []struct {
		name           string
		containers     []corev1.Container
		resourceClaims []corev1.PodResourceClaim
		expected       []corev1.Container
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
			resourceClaims: []corev1.PodResourceClaim{
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
			resourceClaims: []corev1.PodResourceClaim{
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
			resourceClaims: []corev1.PodResourceClaim{
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
			resourceClaims: []corev1.PodResourceClaim{
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
			resourceClaims: []corev1.PodResourceClaim{
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a copy of the containers to avoid modifying the test data
			containers := make([]corev1.Container, len(tt.containers))
			copy(containers, tt.containers)

			UpdateContainerResourceClaims(containers, tt.resourceClaims)

			// Compare the results
			assert.Equal(t, tt.expected, containers)
		})
	}
}
