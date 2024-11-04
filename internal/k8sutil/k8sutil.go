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

package k8sutil

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// OrchestratorType is the underlying container orchestrator type
type OrchestratorType string

const (
	// TKGS is the VMware Tanzu Kubernetes Grid Service
	TKGS OrchestratorType = "TKGS"
	// OpenShift is the RedHat Openshift Container Platform
	OpenShift OrchestratorType = "OpenShift"
	// GKE is the Google Kubernetes Engine Service
	GKE OrchestratorType = "GKE"
	// EKS is the Amazon Elastic Kubernetes Service
	EKS OrchestratorType = "EKS"
	// AKS is the Azure Kubernetes Service
	AKS OrchestratorType = "AKS"
	// OKE is the Oracle Kubernetes Service
	OKE OrchestratorType = "OKE"
	// Ezmeral is the HPE Ezmeral Data Fabric
	Ezmeral OrchestratorType = "Ezmeral"
	// RKE is the Rancker Kubernetes Engine
	RKE OrchestratorType = "RKE"
	// K8s is the upstream Kubernetes Distribution
	K8s OrchestratorType = "Kubernetes"
	// Unknown distribution type
	Unknown OrchestratorType = "Unknown"
)

// GetOrchestratorType checks the container orchestrator by looking for specific node labels that identify
// TKGS, OpenShift, or CSP-specific Kubernetes distributions.
func GetOrchestratorType(k8sClient client.Client) (OrchestratorType, error) {
	nodes := &corev1.NodeList{}
	err := k8sClient.List(context.TODO(), nodes)
	if err != nil {
		return Unknown, fmt.Errorf("error listing nodes: %v", err)
	}

	for _, node := range nodes.Items {
		// Detect TKGS
		if _, isTKGS := node.Labels["node.vmware.com/tkg"]; isTKGS {
			return TKGS, nil
		}
		if _, isTKGS := node.Labels["vsphere-tanzu"]; isTKGS {
			return TKGS, nil
		}

		// Detect OpenShift
		if _, isOpenShift := node.Labels["node.openshift.io/os_id"]; isOpenShift {
			return OpenShift, nil
		}

		// Detect Google GKE
		if _, isGKE := node.Labels["cloud.google.com/gke-nodepool"]; isGKE {
			return GKE, nil
		}

		// Detect Amazon EKS
		if _, isEKS := node.Labels["eks.amazonaws.com/nodegroup"]; isEKS {
			return EKS, nil
		}

		// Detect Azure AKS
		if _, isAKS := node.Labels["kubernetes.azure.com/cluster"]; isAKS {
			return AKS, nil
		}

		// Detect Oracle OKE
		if _, isOKE := node.Labels["oke.oraclecloud.com/cluster"]; isOKE {
			return OKE, nil
		}

		// Detect HPE Ezmeral
		if _, isHPE := node.Labels["ezmeral.hpe.com/cluster"]; isHPE {
			return Ezmeral, nil
		}

		// Detect Rancher RKE
		if _, isRKE := node.Labels["rke.cattle.io/version"]; isRKE {
			return RKE, nil
		}
	}

	// Default to Upstream Kubernetes if no specific platform labels are found
	return K8s, nil
}
