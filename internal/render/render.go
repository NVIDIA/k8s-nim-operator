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

package render

/*
 Render package renders k8s API objects from a given set of template .yaml files
 provided in a source directory and a RenderData struct to be used in the rendering process

 The objects are rendered in `Unstructured` format provided by
 k8s.io/apimachinery/pkg/apis/meta/v1/unstructured package.
*/

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	kservev1beta1 "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	securityv1 "github.com/openshift/api/security/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	resourcev1beta2 "k8s.io/api/resource/v1beta2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	yamlDecoder "k8s.io/apimachinery/pkg/util/yaml"
	lws "sigs.k8s.io/lws/api/leaderworkerset/v1"
	yamlConverter "sigs.k8s.io/yaml"

	"github.com/NVIDIA/k8s-nim-operator/internal/render/types"
	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

const (
	maxBufSizeForYamlDecode = 4096
)

// ManifestFileSuffix indictes common suffixes of files to render.
var ManifestFileSuffix = []string{"yaml", "yml", "json"}

// Renderer renders k8s objects from a manifest source dir and TemplateData used by the templating engine.
type Renderer interface {
	// RenderObjects renders kubernetes objects using provided TemplateData
	RenderObjects(data *TemplateData) ([]*unstructured.Unstructured, error)
	DaemonSet(params *types.DaemonsetParams) (*appsv1.DaemonSet, error)
	Deployment(params *types.DeploymentParams) (*appsv1.Deployment, error)
	LeaderWorkerSet(params *types.LeaderWorkerSetParams) (*lws.LeaderWorkerSet, error)
	StatefulSet(params *types.StatefulSetParams) (*appsv1.StatefulSet, error)
	Service(params *types.ServiceParams) (*corev1.Service, error)
	ServiceAccount(params *types.ServiceAccountParams) (*corev1.ServiceAccount, error)
	Role(params *types.RoleParams) (*rbacv1.Role, error)
	RoleBinding(params *types.RoleBindingParams) (*rbacv1.RoleBinding, error)
	SCC(params *types.SCCParams) (*securityv1.SecurityContextConstraints, error)
	Ingress(params *types.IngressParams) (*networkingv1.Ingress, error)
	HPA(params *types.HPAParams) (*autoscalingv2.HorizontalPodAutoscaler, error)
	ServiceMonitor(params *types.ServiceMonitorParams) (*monitoringv1.ServiceMonitor, error)
	ConfigMap(params *types.ConfigMapParams) (*corev1.ConfigMap, error)
	Secret(params *types.SecretParams) (*corev1.Secret, error)
	InferenceService(params *types.InferenceServiceParams) (*kservev1beta1.InferenceService, error)
	ResourceClaimTemplate(params *types.ResourceClaimTemplateParams) (*resourcev1beta2.ResourceClaimTemplate, error)
}

// TemplateData is used by the templating engine to render templates.
type TemplateData struct {
	// Funcs are additional Functions used during the templating process
	Funcs template.FuncMap
	// Data used for the rendering process
	Data interface{}
}

// NewRenderer creates a Renderer object, that will render all template files provided.
// file format needs to be either json or yaml.
func NewRenderer(directory string) Renderer {
	return &textTemplateRenderer{
		directory: directory,
	}
}

// textTemplateRenderer is an implementation of the Renderer interface using golang builtin text/template package
// as its templating engine.
type textTemplateRenderer struct {
	directory string
}

// RenderObjects renders kubernetes objects utilizing the provided TemplateData.
func (r *textTemplateRenderer) RenderObjects(data *TemplateData) ([]*unstructured.Unstructured, error) {
	var objs []*unstructured.Unstructured

	files, err := utils.GetFilesWithSuffix(r.directory, "json", "yaml", "yml")
	if err != nil {
		return nil, fmt.Errorf("error getting files %s: %w", r.directory, err)
	}

	for _, file := range files {
		out, err := r.renderFile(file, data)
		if err != nil {
			return nil, fmt.Errorf("error rendering file %s: %w", file, err)
		}
		objs = append(objs, out...)
	}
	return objs, nil
}

// renderFile renders a single file to a list of k8s unstructured objects.
func (r *textTemplateRenderer) renderFile(filePath string, data *TemplateData) ([]*unstructured.Unstructured, error) {
	// Read file
	txt, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file %s: %w", filePath, err)
	}

	// Create a new template
	tmpl := template.New(path.Base(filePath)).Funcs(sprig.FuncMap()).Option("missingkey=error")

	tmpl.Funcs(template.FuncMap{
		"yaml": func(obj interface{}) (string, error) {
			yamlBytes, err := yamlConverter.Marshal(obj)
			return string(yamlBytes), err
		},
		"deref": func(b *bool) bool {
			if b == nil {
				return false
			}
			return *b
		},
	})

	if data.Funcs != nil {
		tmpl.Funcs(data.Funcs)
	}

	if _, err := tmpl.Parse(string(txt)); err != nil {
		return nil, fmt.Errorf("failed to parse manifest file %s: %w", filePath, err)
	}
	rendered := bytes.Buffer{}

	if err := tmpl.Execute(&rendered, data.Data); err != nil {
		return nil, fmt.Errorf("failed to render manifest %s: %w", filePath, err)
	}

	out := []*unstructured.Unstructured{}

	// special case - if the entire file is whitespace, skip
	if strings.TrimSpace(rendered.String()) == "" {
		return out, nil
	}

	decoder := yamlDecoder.NewYAMLOrJSONDecoder(&rendered, maxBufSizeForYamlDecode)
	for {
		u := unstructured.Unstructured{}
		if err := decoder.Decode(&u); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to unmarshal manifest %s: %w", filePath, err)
		}
		// Ensure object is not empty by checking the object kind
		if u.GetKind() == "" {
			continue
		}
		out = append(out, &u)
	}

	return out, nil
}

// Daemonset renders a DaemonSet spec with given templating data.
func (r *textTemplateRenderer) DaemonSet(params *types.DaemonsetParams) (*appsv1.DaemonSet, error) {
	objs, err := r.renderFile(path.Join(r.directory, "daemonset.yaml"), &TemplateData{Data: params})
	if err != nil {
		return nil, err
	}

	if len(objs) == 0 {
		return nil, nil
	}

	daemonset := &appsv1.DaemonSet{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(objs[0].Object, daemonset)
	if err != nil {
		return nil, fmt.Errorf("error converting unstructured object to DaemonSet: %w", err)
	}
	return daemonset, nil
}

// Deployment renders a Deployment spec with given templating data.
func (r *textTemplateRenderer) Deployment(params *types.DeploymentParams) (*appsv1.Deployment, error) {
	objs, err := r.renderFile(path.Join(r.directory, "deployment.yaml"), &TemplateData{Data: params})
	if err != nil {
		return nil, err
	}

	if len(objs) == 0 {
		return nil, nil
	}

	deployment := &appsv1.Deployment{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(objs[0].Object, deployment)
	if err != nil {
		return nil, fmt.Errorf("error converting unstructured object to Deployment: %w", err)
	}
	return deployment, nil
}

func (r *textTemplateRenderer) LeaderWorkerSet(params *types.LeaderWorkerSetParams) (*lws.LeaderWorkerSet, error) {
	objs, err := r.renderFile(path.Join(r.directory, "lws.yaml"), &TemplateData{Data: params})
	if err != nil {
		return nil, err
	}
	if len(objs) == 0 {
		return nil, nil
	}

	leaderworkerset := &lws.LeaderWorkerSet{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(objs[0].Object, leaderworkerset)
	if err != nil {
		return nil, fmt.Errorf("error converting unstructured object to LeaderWorkerSet: %w", err)
	}
	return leaderworkerset, nil
}

// StatefulSet renders spec for a StatefulSet with the given templating data.
func (r *textTemplateRenderer) StatefulSet(params *types.StatefulSetParams) (*appsv1.StatefulSet, error) {
	objs, err := r.renderFile(path.Join(r.directory, "statefulset.yaml"), &TemplateData{Data: params})
	if err != nil {
		return nil, err
	}
	if len(objs) == 0 {
		return nil, nil
	}
	statefulset := &appsv1.StatefulSet{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(objs[0].Object, statefulset)
	if err != nil {
		return nil, fmt.Errorf("error converting unstructured object to StatefulSet: %w", err)
	}
	return statefulset, nil
}

// Service renders a Service spec with given templating data.
func (r *textTemplateRenderer) Service(params *types.ServiceParams) (*corev1.Service, error) {
	objs, err := r.renderFile(path.Join(r.directory, "service.yaml"), &TemplateData{Data: params})
	if err != nil {
		return nil, err
	}
	if len(objs) == 0 {
		return nil, nil
	}
	service := &corev1.Service{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(objs[0].Object, service)
	if err != nil {
		return nil, fmt.Errorf("error converting unstructured object to Service: %w", err)
	}
	return service, nil
}

// ServiceAccount renders a ServiceAccount spec with the given templating data.
func (r *textTemplateRenderer) ServiceAccount(params *types.ServiceAccountParams) (*corev1.ServiceAccount, error) {
	objs, err := r.renderFile(path.Join(r.directory, "serviceaccount.yaml"), &TemplateData{Data: params})
	if err != nil {
		return nil, err
	}
	if len(objs) == 0 {
		return nil, nil
	}
	serviceAccount := &corev1.ServiceAccount{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(objs[0].Object, serviceAccount)
	if err != nil {
		return nil, fmt.Errorf("error converting unstructured object to ServiceAccount: %w", err)
	}
	return serviceAccount, nil
}

// Role renders a Role spec with the given templating data.
func (r *textTemplateRenderer) Role(params *types.RoleParams) (*rbacv1.Role, error) {
	objs, err := r.renderFile(path.Join(r.directory, "role.yaml"), &TemplateData{Data: params})
	if err != nil {
		return nil, err
	}
	if len(objs) == 0 {
		return nil, nil
	}
	role := &rbacv1.Role{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(objs[0].Object, role)
	if err != nil {
		return nil, fmt.Errorf("error converting unstructured object to Role: %w", err)
	}
	return role, nil
}

// RoleBinding renders a RoleBinding spec with the given templating data.
func (r *textTemplateRenderer) RoleBinding(params *types.RoleBindingParams) (*rbacv1.RoleBinding, error) {
	objs, err := r.renderFile(path.Join(r.directory, "rolebinding.yaml"), &TemplateData{Data: params})
	if err != nil {
		return nil, err
	}
	if len(objs) == 0 {
		return nil, nil
	}
	roleBinding := &rbacv1.RoleBinding{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(objs[0].Object, roleBinding)
	if err != nil {
		return nil, fmt.Errorf("error converting unstructured object to RoleBinding: %w", err)
	}
	return roleBinding, nil
}

// SCC renders a SecurityContextConstraints spec with the given templating data.
func (r *textTemplateRenderer) SCC(params *types.SCCParams) (*securityv1.SecurityContextConstraints, error) {
	objs, err := r.renderFile(path.Join(r.directory, "scc.yaml"), &TemplateData{Data: params})
	if err != nil {
		return nil, err
	}
	if len(objs) == 0 {
		return nil, nil
	}
	scc := &securityv1.SecurityContextConstraints{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(objs[0].Object, scc)
	if err != nil {
		return nil, fmt.Errorf("error converting unstructured object to SCC: %w", err)
	}
	return scc, nil
}

// Ingress renders an Ingress spec with the given templating data.
func (r *textTemplateRenderer) Ingress(params *types.IngressParams) (*networkingv1.Ingress, error) {
	objs, err := r.renderFile(path.Join(r.directory, "ingress.yaml"), &TemplateData{Data: params})
	if err != nil {
		return nil, err
	}
	if len(objs) == 0 {
		return nil, nil
	}
	ingress := &networkingv1.Ingress{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(objs[0].Object, ingress)
	if err != nil {
		return nil, fmt.Errorf("error converting unstructured object to Ingress: %w", err)
	}
	return ingress, nil
}

// HPA renders spec for HPA with the given templating data.
func (r *textTemplateRenderer) HPA(params *types.HPAParams) (*autoscalingv2.HorizontalPodAutoscaler, error) {
	objs, err := r.renderFile(path.Join(r.directory, "hpa.yaml"), &TemplateData{Data: params})
	if err != nil {
		return nil, err
	}
	if len(objs) == 0 {
		return nil, nil
	}
	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(objs[0].Object, hpa)
	if err != nil {
		return nil, fmt.Errorf("error converting unstructured object to HPA: %w", err)
	}
	// Sorting to avoid unnecessary object updates
	hpa.Spec.Metrics = utils.SortHPAMetricsSpec(hpa.Spec.Metrics)
	return hpa, nil
}

// ServiceMonitor renders spec for a ServiceMonitor with the given templating data.
func (r *textTemplateRenderer) ServiceMonitor(params *types.ServiceMonitorParams) (*monitoringv1.ServiceMonitor, error) {
	objs, err := r.renderFile(path.Join(r.directory, "servicemonitor.yaml"), &TemplateData{Data: params})
	if err != nil {
		return nil, err
	}
	if len(objs) == 0 {
		return nil, nil
	}
	serviceMonitor := &monitoringv1.ServiceMonitor{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(objs[0].Object, serviceMonitor)
	if err != nil {
		return nil, fmt.Errorf("error converting unstructured object to ServiceMonitor: %w", err)
	}
	return serviceMonitor, nil
}

// ConfigMap renders a ConfigMap spec with given templating data.
func (r *textTemplateRenderer) ConfigMap(params *types.ConfigMapParams) (*corev1.ConfigMap, error) {
	objs, err := r.renderFile(path.Join(r.directory, "configmap.yaml"), &TemplateData{Data: params})
	if err != nil {
		return nil, err
	}
	if len(objs) == 0 {
		return nil, nil
	}
	cm := &corev1.ConfigMap{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(objs[0].Object, cm)
	if err != nil {
		return nil, fmt.Errorf("error converting unstructured object to ConfigMap: %w", err)
	}
	return cm, nil
}

// Secret renders a Secret spec with given templating data.
func (r *textTemplateRenderer) Secret(params *types.SecretParams) (*corev1.Secret, error) {
	objs, err := r.renderFile(path.Join(r.directory, "secret.yaml"), &TemplateData{Data: params})
	if err != nil {
		return nil, err
	}
	if len(objs) == 0 {
		return nil, nil
	}
	secret := &corev1.Secret{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(objs[0].Object, secret)
	if err != nil {
		return nil, fmt.Errorf("error converting unstructured object to Secret: %w", err)
	}
	return secret, nil
}

// InferenceService renders a InferenceService spec with given templating data.
func (r *textTemplateRenderer) InferenceService(params *types.InferenceServiceParams) (*kservev1beta1.InferenceService, error) {
	objs, err := r.renderFile(path.Join(r.directory, "inferenceservice.yaml"), &TemplateData{Data: params})
	if err != nil {
		return nil, err
	}

	if len(objs) == 0 {
		return nil, nil
	}

	inferenceService := &kservev1beta1.InferenceService{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(objs[0].Object, inferenceService)
	if err != nil {
		return nil, fmt.Errorf("error converting unstructured object to InferenceService: %w", err)
	}
	return inferenceService, nil
}

// ResourceClaimTemplate renders a ResourceClaimTemplate spec with given templating data.
func (r *textTemplateRenderer) ResourceClaimTemplate(params *types.ResourceClaimTemplateParams) (*resourcev1beta2.ResourceClaimTemplate, error) {
	objs, err := r.renderFile(path.Join(r.directory, "resourceclaimtemplate.yaml"), &TemplateData{Data: params})
	if err != nil {
		return nil, err
	}
	if len(objs) == 0 {
		return nil, nil
	}
	resourceClaimTemplate := &resourcev1beta2.ResourceClaimTemplate{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(objs[0].Object, resourceClaimTemplate)
	if err != nil {
		return nil, fmt.Errorf("error converting unstructured object to ResourceClaimTemplate: %w", err)
	}
	return resourceClaimTemplate, nil
}
