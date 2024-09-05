/*
Copyright 2024.

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

package nfdutil

import (
	"context"
	"fmt"
	"os"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

// CheckNodeFeatureRule checks if a NodeFeatureRule exists that is owned by the NIM operator
func CheckNodeFeatureRule(ctx context.Context, k8sClient client.Client) (bool, error) {
	// Get operator namespace from the environment
	namespace := os.Getenv("OPERATOR_NAMESPACE")
	if namespace == "" {
		return false, fmt.Errorf("OPERATOR_NAMESPACE is not set")
	}

	// Check if the NodeFeatureRule CRD exists
	crdExists, err := checkCRDExists(ctx, k8sClient, "nodefeaturerules.nfd.k8s.io")
	if err != nil {
		return false, err
	}

	if !crdExists {
		return false, nil
	}

	// List NodeFeatureRules in the namespace
	nfrList := &nfdv1alpha1.NodeFeatureRuleList{}
	labelSelector := client.MatchingLabels{"app.kubernetes.io/managed-by": "k8s-nim-operator"}
	// List NodeFeatureRules in the specified namespace with the matching label
	err = k8sClient.List(ctx, nfrList, client.InNamespace(namespace), labelSelector)
	if err != nil {
		if errors.IsNotFound(err) {
			// No NodeFeatureRule exists with this label
			return false, nil
		}
		return false, fmt.Errorf("failed to list NodeFeatureRules: %w", err)
	}

	// If we find any items in the list, return true
	if len(nfrList.Items) > 0 {
		return true, nil
	}

	return false, nil
}

// checkCRDExists checks if a CRD exists by querying the apiextensions.k8s.io/v1 API
func checkCRDExists(ctx context.Context, k8sClient client.Client, crdName string) (bool, error) {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	err := k8sClient.Get(ctx, client.ObjectKey{Name: crdName}, crd)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// CRD not found
			return false, nil
		}
		return false, fmt.Errorf("error checking for CRD %s: %w", crdName, err)
	}
	return true, nil
}
