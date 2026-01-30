/**
# Copyright (c) NVIDIA CORPORATION.  All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	kservev1beta1 "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	kserveconstants "github.com/kserve/kserve/pkg/constants"
	kserveutils "github.com/kserve/kserve/pkg/utils"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	KServeControllerName = "kserve-controller-manager"
)

func IsKServeStandardDeploymentMode(deploymentMode kserveconstants.DeploymentModeType) bool {
	return deploymentMode == kserveconstants.Standard || deploymentMode == kserveconstants.LegacyRawDeployment
}

func IsKServeKnativeDeploymentMode(deploymentMode kserveconstants.DeploymentModeType) bool {
	return deploymentMode == kserveconstants.Knative || deploymentMode == kserveconstants.LegacyServerless
}

func GetKServeDeploymentMode(ctx context.Context, k8sClient client.Client,
	podAnnotations map[string]string, isvcNamespacedName *types.NamespacedName) (kserveconstants.DeploymentModeType, error) {
	logger := log.FromContext(ctx).WithName("KServe").WithName("Deployment Mode")

	var annotations map[string]string
	var deployConfig *kservev1beta1.DeployConfig
	var statusDeploymentMode string

	if k8sClient != nil {
		isvcConfigMap, err := getISVCConfigMap(ctx, k8sClient)
		if err != nil {
			return "", err
		}

		var isvcConfig *kservev1beta1.InferenceServicesConfig
		if isvcConfigMap != nil {
			isvcConfig, err = kservev1beta1.NewInferenceServicesConfig(isvcConfigMap)
			if err != nil {
				return "", err
			}
		}

		if isvcConfig != nil && podAnnotations != nil {
			annotations = kserveutils.Filter(podAnnotations, func(key string) bool {
				return !kserveutils.Includes(isvcConfig.ServiceAnnotationDisallowedList, key)
			})
		} else {
			annotations = podAnnotations
		}

		if isvcConfigMap != nil {
			deployConfig, err = newDeployConfig(isvcConfigMap)
			if err != nil {
				return "", err
			}
		} else {
			logger.V(1).Info("ConfigMap inferenceservice-config is not found in the cluster, skipping the " +
				"default deployment mode check from the ConfigMap")
		}

		// Try to get the deployment mode from the InferenceService status if it exists.
		// Note: The InferenceService doesn't exist yet, so this will return NotFound (which is handled gracefully).
		// Status-based mode detection only applies to existing InferenceServices. This ensures the status value takes
		// highest priority when available, maintaining consistency with KServe's behavior.
		if isvcNamespacedName != nil {
			isvc := &kservev1beta1.InferenceService{}
			err = k8sClient.Get(ctx, *isvcNamespacedName, isvc)
			if err != nil {
				if !k8serrors.IsNotFound(err) {
					return "", err
				}
				logger.V(1).Info("InferenceService not found, deployment mode from status is not available",
					"InferenceService", isvcNamespacedName.String())
			} else {
				statusDeploymentMode = isvc.Status.DeploymentMode
			}
		} else {
			logger.V(1).Info("InferenceService name is not set, skipping deployment mode check from InferenceService status")
		}
	} else {
		logger.V(1).Info("k8sClient is nil, skipping default deployment mode retrieval from the inferenceservice-config ConfigMap, " +
			"skipping deployment mode check from InferenceService status, using the annotations only")
		annotations = podAnnotations
	}

	return getDeploymentMode(ctx, statusDeploymentMode, annotations, deployConfig)
}

/*
GetDeploymentMode returns the current deployment mode, supports Knative and Standard
case 1: no serving.kserve.org/deploymentMode annotation

	return config.deploy.defaultDeploymentMode

case 2: serving.kserve.org/deploymentMode is set

	        if the mode is "Standard", "Knative", "ModelMesh" or "RawDeployment", "Serverless", return it.
			else return config.deploy.defaultDeploymentMode

ODH 3.0 supports "RawDeployment", "Serverless", and doesn't accept "Standard", "Knative"
Ref: https://github.com/opendatahub-io/kserve/blob/cf2920b4276d97fc2d2d700efe4879749a56b418/pkg/controller/v1beta1/inferenceservice/utils/utils.go#L220
*/
func getDeploymentMode(ctx context.Context, statusDeploymentMode string, annotations map[string]string,
	deployConfig *kservev1beta1.DeployConfig) (kserveconstants.DeploymentModeType, error) {
	logger := log.FromContext(ctx).WithName("KServe").WithName("Deployment Mode")

	// First priority is the deploymentMode recorded in the status
	if len(statusDeploymentMode) != 0 {
		logger.Info("using deployment mode from InferenceService status",
			"Deployment Mode", statusDeploymentMode)
		return kserveconstants.DeploymentModeType(statusDeploymentMode), nil
	}

	if annotations != nil {
		// Second priority, if the status doesn't have the deploymentMode recorded, is explicit annotations
		deploymentMode, ok := annotations[kserveconstants.DeploymentMode]

		// Note: ODH 3.0 requires using "RawDeployment" and "Serverless" directly
		// without conversion to "Standard" and "Knative". Do not convert legacy modes.

		if ok && deploymentMode != "" { // Explicitly check for non-empty
			if deploymentMode == string(kserveconstants.Standard) ||
				deploymentMode == string(kserveconstants.Knative) ||
				deploymentMode == string(kserveconstants.ModelMeshDeployment) ||
				deploymentMode == string(kserveconstants.LegacyRawDeployment) ||
				deploymentMode == string(kserveconstants.LegacyServerless) {
				logger.Info("using deployment mode from annotations",
					"Deployment Mode", deploymentMode)
				return kserveconstants.DeploymentModeType(deploymentMode), nil
			}
			// Only error if annotation exists AND is non-empty AND is invalid
			logger.Error(nil, "deployment mode annotation found but value is invalid",
				"Deployment Mode", deploymentMode)
			return "", fmt.Errorf("deployment mode annotation found but value is invalid: %s", deploymentMode)
		}

		logger.V(1).Info("warning: deployment mode annotation not found or empty in annotations")
	}

	if deployConfig != nil {
		// Finally, if an InferenceService is being created and does not explicitly specify a DeploymentMode
		logger.Info("using the default deployment mode from the inferenceservice-config ConfigMap",
			"Deployment Mode", deployConfig.DefaultDeploymentMode)
		return kserveconstants.DeploymentModeType(deployConfig.DefaultDeploymentMode), nil
	}

	// If InferenceServicesConfig doesn't exist, use the default deployment mode
	// For ODH, InferenceServicesConfig is always bundled, and this line should not be reached
	logger.Info("deployment mode is not found in any configurations, using default deployment mode",
		"Deployment Mode", kserveconstants.DefaultDeployment)
	return kserveconstants.DefaultDeployment, nil
}

/*
ODH 3.0 supports "RawDeployment", "Serverless", and doesn't accept "Standard", "Knative"
Ref: https://github.com/opendatahub-io/kserve/blob/cf2920b4276d97fc2d2d700efe4879749a56b418/pkg/apis/serving/v1beta1/configmap.go#L359
*/
func newDeployConfig(isvcConfigMap *corev1.ConfigMap) (*kservev1beta1.DeployConfig, error) {
	deploy, ok := isvcConfigMap.Data[kservev1beta1.DeployConfigName]
	if !ok {
		// No deploy config in ConfigMap, return nil so caller falls back to default
		return nil, nil
	}

	deployConfig := &kservev1beta1.DeployConfig{}
	err := json.Unmarshal([]byte(deploy), &deployConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to parse deploy config json: %w", err)
	}

	if deployConfig.DefaultDeploymentMode == "" {
		return nil, errors.New("invalid deploy config, defaultDeploymentMode is required")
	}

	// Note: ODH 3.0 requires using "RawDeployment" and "Serverless" directly
	// without conversion to "Standard" and "Knative". Do not convert legacy modes.

	if deployConfig.DefaultDeploymentMode != string(kserveconstants.Knative) &&
		deployConfig.DefaultDeploymentMode != string(kserveconstants.Standard) &&
		deployConfig.DefaultDeploymentMode != string(kserveconstants.ModelMeshDeployment) &&
		deployConfig.DefaultDeploymentMode != string(kserveconstants.LegacyRawDeployment) &&
		deployConfig.DefaultDeploymentMode != string(kserveconstants.LegacyServerless) {
		return nil, errors.New("invalid deployment mode. Supported modes are Knative," +
			" Standard, ModelMesh and RawDeployment, Serverless")
	}

	return deployConfig, nil
}

func getISVCConfigMap(ctx context.Context, k8sClient client.Client) (*corev1.ConfigMap, error) {
	var namespace string
	// Find the namespace of the KServe controller
	deploymentList := &appsv1.DeploymentList{}
	if err := k8sClient.List(ctx, deploymentList, client.MatchingLabels{"app.kubernetes.io/name": KServeControllerName}); err != nil {
		return nil, err
	}
	for _, deployment := range deploymentList.Items {
		if deployment.GetName() == KServeControllerName {
			namespace = deployment.Namespace
			break
		}
	}

	if namespace == "" {
		return nil, fmt.Errorf("failed to find the namespace of KServe Deployment")
	}

	// Get the inferenceservice-config ConfigMap in the namespace
	isvcConfigMap := &corev1.ConfigMap{}
	err := k8sClient.Get(ctx,
		types.NamespacedName{
			Name:      kserveconstants.InferenceServiceConfigMapName,
			Namespace: namespace},
		isvcConfigMap)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return isvcConfigMap, nil
}
