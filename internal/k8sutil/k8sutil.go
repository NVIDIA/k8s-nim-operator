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
	"bytes"
	"context"
	goerrors "errors"
	"fmt"
	"io"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

// ErrConfigMapKeyNotFound indicates an error that the given key is missing from the config map.
var ErrConfigMapKeyNotFound = goerrors.New("configmap key not found")

// OrchestratorType is the underlying container orchestrator type.
type OrchestratorType string

const (
	// TKGS is the VMware Tanzu Kubernetes Grid Service.
	TKGS OrchestratorType = "TKGS"
	// OpenShift is the RedHat Openshift Container Platform.
	OpenShift OrchestratorType = "OpenShift"
	// GKE is the Google Kubernetes Engine Service.
	GKE OrchestratorType = "GKE"
	// EKS is the Amazon Elastic Kubernetes Service.
	EKS OrchestratorType = "EKS"
	// AKS is the Azure Kubernetes Service.
	AKS OrchestratorType = "AKS"
	// OKE is the Oracle Kubernetes Service.
	OKE OrchestratorType = "OKE"
	// Ezmeral is the HPE Ezmeral Data Fabric.
	Ezmeral OrchestratorType = "Ezmeral"
	// RKE is the Rancker Kubernetes Engine.
	RKE OrchestratorType = "RKE"
	// K8s is the upstream Kubernetes Distribution.
	K8s OrchestratorType = "Kubernetes"
	// Unknown distribution type.
	Unknown OrchestratorType = "Unknown"
)

// GetOrchestratorType checks the container orchestrator by looking for specific node labels that identify
// TKGS, OpenShift, or CSP-specific Kubernetes distributions.
func GetOrchestratorType(ctx context.Context, k8sClient client.Client) (OrchestratorType, error) {
	nodes := &corev1.NodeList{}
	err := k8sClient.List(ctx, nodes)
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

// CleanupResource deletes the given Kubernetes resource if it exists.
// If the resource does not exist or an error occurs during deletion, the function returns nil or the error.
//
// Parameters:
// ctx (context.Context): The context for the operation.
// obj (client.Object): The Kubernetes resource to delete.
// namespacedName (types.NamespacedName): The namespaced name of the resource.
//
// Returns:
// error: An error if the resource deletion fails, or nil if the resource is not found or deletion is successful.
func CleanupResource(ctx context.Context, k8sClient client.Client, obj client.Object, namespacedName types.NamespacedName) error {

	logger := log.FromContext(ctx)

	err := k8sClient.Get(ctx, namespacedName, obj)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if errors.IsNotFound(err) {
		return nil
	}

	err = k8sClient.Delete(ctx, obj)
	if err != nil {
		return err
	}
	logger.V(2).Info("object deleted", "obj", obj)
	return nil
}

// SyncResource sync the current object with the desired object spec.
func SyncResource(ctx context.Context, k8sClient client.Client, obj client.Object, desired client.Object) error {
	logger := log.FromContext(ctx)

	if !utils.IsSpecChanged(obj, desired) {
		logger.V(2).Info("Object spec has not changed, skipping update", "obj", obj)
		return nil
	}
	logger.V(2).Info("Object spec has changed, updating")
	if obj == nil || obj.GetName() == "" || obj.GetNamespace() == "" {
		err := k8sClient.Create(ctx, desired)
		if err != nil {
			return err
		}
	} else {
		err := k8sClient.Update(ctx, utils.UpdateObject(obj, desired))
		if err != nil {
			return err
		}
	}
	return nil
}

// IsDeploymentReady checks if the Deployment is ready.
func IsDeploymentReady(ctx context.Context, k8sClient client.Client, namespacedName *types.NamespacedName) (string, bool, error) {
	deployment := &appsv1.Deployment{}
	err := k8sClient.Get(ctx, client.ObjectKey{Name: namespacedName.Name, Namespace: namespacedName.Namespace}, deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}

	cond := getDeploymentCondition(deployment.Status, appsv1.DeploymentProgressing)
	if cond != nil && cond.Reason == "ProgressDeadlineExceeded" {
		return fmt.Sprintf("deployment %q exceeded its progress deadline", deployment.Name), false, nil
	}
	if deployment.Spec.Replicas != nil && deployment.Status.UpdatedReplicas < *deployment.Spec.Replicas {
		return fmt.Sprintf("Waiting for deployment %q rollout to finish: %d out of %d new replicas have been updated...\n", deployment.Name, deployment.Status.UpdatedReplicas, *deployment.Spec.Replicas), false, nil
	}
	if deployment.Status.Replicas > deployment.Status.UpdatedReplicas {
		return fmt.Sprintf("Waiting for deployment %q rollout to finish: %d old replicas are pending termination...\n", deployment.Name, deployment.Status.Replicas-deployment.Status.UpdatedReplicas), false, nil
	}
	if deployment.Status.AvailableReplicas < deployment.Status.UpdatedReplicas {
		return fmt.Sprintf("Waiting for deployment %q rollout to finish: %d of %d updated replicas are available...\n", deployment.Name, deployment.Status.AvailableReplicas, deployment.Status.UpdatedReplicas), false, nil
	}
	return fmt.Sprintf("deployment %q successfully rolled out\n", deployment.Name), true, nil
}

func getDeploymentCondition(status appsv1.DeploymentStatus, condType appsv1.DeploymentConditionType) *appsv1.DeploymentCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

// GetUpdateCaCertInitContainerCommand returns the command to update CA certificates in the init container.
func GetUpdateCaCertInitContainerCommand() []string {
	return []string{
		"/bin/sh",
		"-c",
		"echo 'Copying CA certs from Config Map'; " +
			"ls -l /custom; " +
			"cp /custom/*.crt /usr/local/share/ca-certificates/; " +
			"for f in /custom/*.pem; do cp \"$f\" \"/usr/local/share/ca-certificates/$(basename \"${f%.pem}\")-pem.crt\"; done;" +
			"ls -l  /usr/local/share/ca-certificates/;" +
			"echo 'Updating CA certs'; " +
			"update-ca-certificates; " +
			"echo 'CA certs updated'; " +
			"cp -r /etc/ssl/certs/ /ca-certs;" +
			"echo 'Exiting update-ca-certificates init container'",
	}
}

// GetUpdateCaCertInitContainerSecurityContext returns the security context for init container for updating CA certificates.
func GetUpdateCaCertInitContainerSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		RunAsUser:    ptr.To(int64(0)),
		RunAsNonRoot: ptr.To(false),
	}
}

// GetUpdateCaCertInitContainerVolumeMounts returns the volume mounts for the init container for updating CA certificates.
func GetUpdateCaCertInitContainerVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "ca-cert-volume",
			MountPath: "/ca-certs",
		},
		{
			Name:      "custom-ca",
			MountPath: "/custom",
		},
	}
}

// GetVolumesForUpdatingCaCert returns the volumes required for updating CA certificates.
func GetVolumesForUpdatingCaCert(configMapName string) []corev1.Volume {
	return []corev1.Volume{
		{
			Name: "ca-cert-volume",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "custom-ca",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: configMapName}},
			},
		},
	}
}

// GetVolumesMountsForUpdatingCaCert returns the volume mounts required for updating CA certificates.
func GetVolumesMountsForUpdatingCaCert() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "ca-cert-volume",
			MountPath: "/etc/ssl",
		},
	}
}

// GetRawYAMLFromConfigMap extracts yaml content from given configmap and key.
func GetRawYAMLFromConfigMap(ctx context.Context, k8sClient client.Client, namespace string, configMapName string, configMapKey string) (string, error) {
	var cm corev1.ConfigMap
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, &cm); err != nil {
		return "", err
	}

	raw, ok := cm.Data[configMapKey]
	if !ok {
		return "", fmt.Errorf("%w: key %q not found in ConfigMap %q", ErrConfigMapKeyNotFound, configMapKey, configMapName)
	}

	return raw, nil
}

func GetPodLogs(ctx context.Context, pod *corev1.Pod, containerName string) (string, error) {
	podLogOpts := corev1.PodLogOptions{Container: containerName}
	config, err := rest.InClusterConfig()
	if err != nil {
		return "", err
	}
	// create a clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", err
	}
	req := clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &podLogOpts)
	podLogs, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer func(podLogs io.ReadCloser) {
		_ = podLogs.Close()
	}(podLogs)

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func GetClusterVersion(discoveryClient discovery.DiscoveryInterface) (string, error) {
	if discoveryClient == nil {
		return "", fmt.Errorf("discoveryClient not provided")
	}
	versionInfo, err := discoveryClient.ServerVersion()
	if err != nil {
		return "", err
	}
	return versionInfo.GitVersion, nil
}

func CRDExists(discoveryClient discovery.DiscoveryInterface, gvr schema.GroupVersionResource) (bool, error) {
	if discoveryClient == nil {
		return false, fmt.Errorf("discoveryClient not provided")
	}
	resources, err := discoveryClient.ServerResourcesForGroupVersion(gvr.GroupVersion().String())
	if err != nil {
		// If the resource is not found, then the CRD is not enabled.
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	for _, resource := range resources.APIResources {
		if resource.Name == gvr.Resource {
			return true, nil
		}
	}
	return false, nil
}
