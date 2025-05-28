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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/dump"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// NvidiaAnnotationHashKey indicates annotation name for last applied hash by the operator.
	NvidiaAnnotationHashKey = "nvidia.com/last-applied-hash"

	// NvidiaAnnotationParentSpecHashKey indicates annotation name for applied hash by the operator.
	NvidiaAnnotationParentSpecHashKey = "nvidia.com/parent-spec-hash"
)

// GetFilesWithSuffix returns all files under a given base directory that have a specific suffix
// The operation is performed recursively on subdirectories as well.
func GetFilesWithSuffix(baseDir string, suffixes ...string) ([]string, error) {
	var files []string
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		// Error during traversal
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Skip non suffix files
		base := info.Name()
		for _, s := range suffixes {
			if strings.HasSuffix(base, s) {
				files = append(files, path)
			}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error traversing directory tree: %w", err)
	}
	return files, nil
}

// BoolPtr returns a pointer to the bool value passed in.
func BoolPtr(v bool) *bool {
	return &v
}

func GetStringHash(s string) string {
	hasher := fnv.New32a()
	if _, err := hasher.Write([]byte(s)); err != nil {
		panic(err)
	}
	return rand.SafeEncodeString(fmt.Sprint(hasher.Sum32()))
}

// MergeMaps merges two maps and ensures no duplicate key-value pairs.
func MergeMaps(m1, m2 map[string]string) map[string]string {
	merged := make(map[string]string)

	for k, v := range m1 {
		merged[k] = v
	}

	for k, v := range m2 {
		if _, exists := merged[k]; !exists {
			merged[k] = v
		}
	}

	return merged
}

// MergeEnvVars merges two slices of environment variables, giving precedence to the second slice in case of duplicates.
func MergeEnvVars(env1, env2 []corev1.EnvVar) []corev1.EnvVar {
	var mergedEnv []corev1.EnvVar
	envMap := make(map[string]bool)
	for _, env := range env2 {
		mergedEnv = append(mergedEnv, env)
		envMap[env.Name] = true
	}

	for _, env := range env1 {
		if _, found := envMap[env.Name]; !found {
			mergedEnv = append(mergedEnv, env)
		}
	}

	return mergedEnv
}

// SortKeys recursively sorts the keys of a map to ensure consistent serialization.
func SortKeys(obj interface{}) interface{} {
	switch obj := obj.(type) {
	case map[string]interface{}:
		sortedMap := make(map[string]interface{})
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sortedMap[k] = SortKeys(obj[k])
		}
		return sortedMap
	case []interface{}:
		// Check if the slice contains maps and sort them by the "name" field or the first available field
		if len(obj) > 0 {

			if _, ok := obj[0].(map[string]interface{}); ok {
				sort.SliceStable(obj, func(i, j int) bool {
					iMap, iOk := obj[i].(map[string]interface{})
					jMap, jOk := obj[j].(map[string]interface{})
					if iOk && jOk {
						// Try to sort by "name" if present
						iName, iNameOk := iMap["name"].(string)
						jName, jNameOk := jMap["name"].(string)
						if iNameOk && jNameOk {
							return iName < jName
						}

						// If "name" is not available, sort by the first key in each map
						if len(iMap) > 0 && len(jMap) > 0 {
							iFirstKey := firstKey(iMap)
							jFirstKey := firstKey(jMap)
							return iFirstKey < jFirstKey
						}
					}
					// If no valid comparison is possible, maintain the original order
					return false
				})
			}
		}
		for i, v := range obj {
			obj[i] = SortKeys(v)
		}
	}
	return obj
}

// Helper function to get the first key of a map (alphabetically sorted).
func firstKey(m map[string]interface{}) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys[0]
}

// GetResourceHash returns a consistent hash for the given object spec.
func GetResourceHash(obj client.Object) string {
	parentSpecHash, ok := obj.GetAnnotations()[NvidiaAnnotationParentSpecHashKey]
	if ok {
		// remove CR spec hash before generating obj hash.
		delete(obj.GetAnnotations(), NvidiaAnnotationParentSpecHashKey)
	}
	objHash := DeepHashObject(obj)
	// re-add CR spec hash.
	if ok {
		obj.GetAnnotations()[NvidiaAnnotationParentSpecHashKey] = parentSpecHash
	}

	return objHash
}

// IsSpecChanged returns true if the spec has changed between the existing one
// and the new resource spec compared by hash.
func IsSpecChanged(current client.Object, desired client.Object) bool {
	if current == nil && desired != nil {
		return true
	}

	hashStr := GetResourceHash(desired)
	foundHashAnnotation := false

	currentAnnotations := current.GetAnnotations()
	desiredAnnotations := desired.GetAnnotations()

	if currentAnnotations == nil {
		currentAnnotations = map[string]string{}
	}
	if desiredAnnotations == nil {
		desiredAnnotations = map[string]string{}
	}

	for annotation, value := range currentAnnotations {
		if annotation == NvidiaAnnotationHashKey {
			if value != hashStr {
				// Update annotation to be added to resource as per new spec and indicate spec update is required
				desiredAnnotations[NvidiaAnnotationHashKey] = hashStr
				desired.SetAnnotations(desiredAnnotations)
				return true
			}
			foundHashAnnotation = true
			break
		}
	}

	if !foundHashAnnotation {
		// Update annotation to be added to resource as per new spec and indicate spec update is required
		desiredAnnotations[NvidiaAnnotationHashKey] = hashStr
		desired.SetAnnotations(desiredAnnotations)
		return true
	}

	return false
}

func IsParentSpecChanged(obj client.Object, desiredHash string) bool {
	if obj == nil {
		return true
	}

	currentHash, ok := obj.GetAnnotations()[NvidiaAnnotationParentSpecHashKey]
	if !ok {
		return true
	}

	return currentHash != desiredHash
}

// IsEqual compares two Kubernetes objects based on their relevant fields.
func IsEqual[T client.Object](existing, desired T, fieldsToCompare ...string) bool {
	for _, field := range fieldsToCompare {
		existingValue := reflect.ValueOf(existing).Elem().FieldByName(field)
		desiredValue := reflect.ValueOf(desired).Elem().FieldByName(field)

		if !reflect.DeepEqual(existingValue.Interface(), desiredValue.Interface()) {
			return false
		}
	}

	return true
}

func SortHPAMetricsSpec(metrics []autoscalingv2.MetricSpec) []autoscalingv2.MetricSpec {
	sort.Slice(metrics, func(i, j int) bool {
		iMetricsType := metrics[i].Type
		jMetricsType := metrics[j].Type
		return iMetricsType < jMetricsType
	})
	return metrics
}

// CalculateSHA256 calculates a SHA256 hash for the given data.
func CalculateSHA256(data string) string {
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

func FormatEndpoint(ip string, port int32) string {
	if ip == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d", ip, port)
}

func DeepHashObject(objToWrite any) string {
	hasher := fnv.New32a()
	hasher.Reset()
	_, _ = fmt.Fprintf(hasher, "%v", dump.ForHash(objToWrite))
	return rand.SafeEncodeString(fmt.Sprint(hasher.Sum32()))
}

func UpdateObject(obj client.Object, desired client.Object) client.Object {
	if obj == nil || desired == nil || !reflect.DeepEqual(obj.GetObjectKind(), desired.GetObjectKind()) || obj.GetName() != desired.GetName() || obj.GetNamespace() != desired.GetNamespace() {
		panic("invalid input to UpdateObject")
	}

	switch castedObj := obj.(type) {
	case *appsv1.Deployment:
		return updateDeployment(castedObj, desired.(*appsv1.Deployment)) //nolint:forcetypeassert
	case *appsv1.StatefulSet:
		return updateStatefulSet(castedObj, desired.(*appsv1.StatefulSet)) //nolint:forcetypeassert
	case *autoscalingv2.HorizontalPodAutoscaler:
		return updateHPA(castedObj, desired.(*autoscalingv2.HorizontalPodAutoscaler)) //nolint:forcetypeassert
	case *corev1.ConfigMap:
		return updateConfigMap(castedObj, desired.(*corev1.ConfigMap)) //nolint:forcetypeassert
	case *corev1.ServiceAccount:
		return updateServiceAccount(castedObj, desired.(*corev1.ServiceAccount)) //nolint:forcetypeassert
	case *corev1.Secret:
		return updateSecret(castedObj, desired.(*corev1.Secret)) //nolint:forcetypeassert
	case *corev1.Service:
		return updateService(castedObj, desired.(*corev1.Service)) //nolint:forcetypeassert
	case *monitoringv1.ServiceMonitor:
		return updateServiceMonitor(castedObj, desired.(*monitoringv1.ServiceMonitor)) //nolint:forcetypeassert
	case *networkingv1.Ingress:
		return updateIngress(castedObj, desired.(*networkingv1.Ingress)) //nolint:forcetypeassert
	case *rbacv1.Role:
		return updateRole(castedObj, desired.(*rbacv1.Role)) //nolint:forcetypeassert
	case *rbacv1.RoleBinding:
		return updateRoleBinding(castedObj, desired.(*rbacv1.RoleBinding)) //nolint:forcetypeassert
	case *corev1.PersistentVolumeClaim:
		return updatePersistentVolumeClaim(castedObj, desired.(*corev1.PersistentVolumeClaim)) //nolint:forcetypeassert
	default:
		panic("unsupported obj type")
	}
}

func updateDeployment(obj, desired *appsv1.Deployment) *appsv1.Deployment {
	obj.SetAnnotations(desired.GetAnnotations())
	obj.SetLabels(desired.GetLabels())
	obj.Spec = *desired.Spec.DeepCopy()
	return obj
}

func updateStatefulSet(obj, desired *appsv1.StatefulSet) *appsv1.StatefulSet {
	obj.SetAnnotations(desired.GetAnnotations())
	obj.SetLabels(desired.GetLabels())
	obj.Spec = *desired.Spec.DeepCopy()
	return obj
}

func updateHPA(obj, desired *autoscalingv2.HorizontalPodAutoscaler) *autoscalingv2.HorizontalPodAutoscaler {
	obj.SetAnnotations(desired.GetAnnotations())
	obj.SetLabels(desired.GetLabels())
	obj.Spec = *desired.Spec.DeepCopy()
	return obj
}

func updateConfigMap(obj, desired *corev1.ConfigMap) *corev1.ConfigMap {
	obj.SetAnnotations(desired.GetAnnotations())
	obj.SetLabels(desired.GetLabels())
	obj.Immutable = desired.Immutable
	obj.Data = desired.Data
	obj.BinaryData = desired.BinaryData
	return obj
}

func updateSecret(obj, desired *corev1.Secret) *corev1.Secret {
	obj.SetAnnotations(desired.GetAnnotations())
	obj.SetLabels(desired.GetLabels())
	obj.Immutable = desired.Immutable
	obj.Data = desired.Data
	obj.StringData = desired.StringData
	return obj
}

func updateService(obj, desired *corev1.Service) *corev1.Service {
	obj.SetAnnotations(desired.GetAnnotations())
	obj.SetLabels(desired.GetLabels())
	obj.Spec = *desired.Spec.DeepCopy()
	return obj
}

func updateServiceAccount(obj, desired *corev1.ServiceAccount) *corev1.ServiceAccount {
	obj.SetAnnotations(desired.GetAnnotations())
	obj.SetLabels(desired.GetLabels())
	obj.Secrets = desired.Secrets
	obj.ImagePullSecrets = desired.ImagePullSecrets
	obj.AutomountServiceAccountToken = desired.AutomountServiceAccountToken
	return obj
}

func updateServiceMonitor(obj, desired *monitoringv1.ServiceMonitor) *monitoringv1.ServiceMonitor {
	obj.SetAnnotations(desired.GetAnnotations())
	obj.SetLabels(desired.GetLabels())
	obj.Spec = *desired.Spec.DeepCopy()
	return obj
}

func updateIngress(obj, desired *networkingv1.Ingress) *networkingv1.Ingress {
	obj.SetAnnotations(desired.GetAnnotations())
	obj.SetLabels(desired.GetLabels())
	obj.Spec = *desired.Spec.DeepCopy()
	return obj
}

func updateRole(obj, desired *rbacv1.Role) *rbacv1.Role {
	obj.SetAnnotations(desired.GetAnnotations())
	obj.SetLabels(desired.GetLabels())
	rules := make([]rbacv1.PolicyRule, 0)
	for _, rule := range desired.Rules {
		rules = append(rules, *rule.DeepCopy())
	}
	obj.Rules = rules
	return obj
}

func updateRoleBinding(obj, desired *rbacv1.RoleBinding) *rbacv1.RoleBinding {
	obj.SetAnnotations(desired.GetAnnotations())
	obj.SetLabels(desired.GetLabels())
	obj.Subjects = desired.Subjects
	obj.RoleRef = desired.RoleRef
	return obj
}

func updatePersistentVolumeClaim(current, desired *corev1.PersistentVolumeClaim) *corev1.PersistentVolumeClaim {
	// Copy the current object (patch target)
	patchObj := current.DeepCopy()

	// Metadata updates
	patchObj.SetAnnotations(desired.GetAnnotations())
	patchObj.SetLabels(desired.GetLabels())

	// Update only the allowed mutable field: resources.requests
	if patchObj.Spec.Resources.Requests == nil {
		patchObj.Spec.Resources.Requests = corev1.ResourceList{}
	}
	for k, v := range desired.Spec.Resources.Requests {
		patchObj.Spec.Resources.Requests[k] = v
	}

	// Leave all other fields (like volumeName, accessModes, etc.) unchanged
	return patchObj
}
