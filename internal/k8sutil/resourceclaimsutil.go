package k8sutil

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

import (
	"context"
	"sort"
	"strings"

	resourcev1beta2 "k8s.io/api/resource/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

func ListResourceClaimsByPodClaimName(ctx context.Context, k8sclient client.Client, namespace string, podClaimName string) ([]resourcev1beta2.ResourceClaim, error) {
	resourceClaims := make([]resourcev1beta2.ResourceClaim, 0)
	var claimList resourcev1beta2.ResourceClaimList
	if err := k8sclient.List(ctx, &claimList, client.InNamespace(namespace)); err != nil {
		return nil, err
	}

	// Filter resource claims by pod claim name annotation.
	for _, claim := range claimList.Items {
		value, ok := claim.Annotations[utils.DRAPodClaimNameAnnotationKey]
		if ok && value == podClaimName {
			resourceClaims = append(resourceClaims, claim)
		}
	}
	// Sort by creationTimestamp (oldest first)
	sort.Slice(resourceClaims, func(i, j int) bool {
		return resourceClaims[i].CreationTimestamp.Time.Before(resourceClaims[j].CreationTimestamp.Time)
	})

	return resourceClaims, nil
}

func GetResourceClaim(ctx context.Context, k8sclient client.Client, name string, namespace string) (*resourcev1beta2.ResourceClaim, error) {
	claim := &resourcev1beta2.ResourceClaim{}
	if err := k8sclient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, claim); err != nil {
		return nil, err
	}
	return claim, nil
}

func GetResourceClaimState(claim *resourcev1beta2.ResourceClaim) string {
	var states []string
	if claim.GetDeletionTimestamp() != nil {
		states = append(states, "deleted")
	}
	if claim.Status.Allocation == nil {
		if claim.GetDeletionTimestamp() == nil {
			states = append(states, "pending")
		}
	} else {
		states = append(states, "allocated")
		if len(claim.Status.ReservedFor) > 0 {
			states = append(states, "reserved")
		}
	}
	return strings.Join(states, ",")
}

func GetResourceClaimTemplate(ctx context.Context, k8sclient client.Client, name string, namespace string) (*resourcev1beta2.ResourceClaimTemplate, error) {
	claimTemplate := &resourcev1beta2.ResourceClaimTemplate{}
	if err := k8sclient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, claimTemplate); err != nil {
		return nil, err
	}
	return claimTemplate, nil
}
