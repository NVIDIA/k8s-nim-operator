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

package kserve

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
	"github.com/NVIDIA/k8s-nim-operator/internal/render"
	"github.com/NVIDIA/k8s-nim-operator/internal/utils"

	kservev1beta1 "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	kserveconstants "github.com/kserve/kserve/pkg/constants"
	knativeapis "knative.dev/pkg/apis"
	knativeduckv1 "knative.dev/pkg/apis/duck/v1"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/version"
	discoveryfake "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func sortEnvVars(envVars []corev1.EnvVar) {
	sort.SliceStable(envVars, func(i, j int) bool {
		return envVars[i].Name < envVars[j].Name
	})
}

func sortVolumeMounts(volumeMounts []corev1.VolumeMount) {
	sort.SliceStable(volumeMounts, func(i, j int) bool {
		return volumeMounts[i].Name < volumeMounts[j].Name
	})
}

func sortVolumes(volumes []corev1.Volume) {
	sort.SliceStable(volumes, func(i, j int) bool {
		return volumes[i].Name < volumes[j].Name
	})
}

func getCondition(obj *appsv1alpha1.NIMService, conditionType string) *metav1.Condition {
	for _, condition := range obj.Status.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}

// Custom transport that redirects requests to a specific host.
type mockTransport struct {
	targetHost        string
	testServer        *httptest.Server
	originalTransport http.RoundTripper
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Check if this request is going to our target IP
	hostname := strings.Split(req.URL.Host, ":")[0]
	if hostname == "" || req.URL.Host == m.targetHost {
		// Create a new URL pointing to our test server
		testURL, _ := url.Parse(m.testServer.URL)
		testURL.Path = req.URL.Path
		testURL.RawQuery = req.URL.RawQuery

		// Create a new request to our test server
		newReq := req.Clone(req.Context())
		newReq.URL = testURL
		newReq.Host = req.URL.Host // Preserve the original Host header

		// Send the request to our test server
		return http.DefaultClient.Do(newReq)
	}

	// For all other requests, use the original transport
	return m.originalTransport.RoundTrip(req)
}

var _ = Describe("NIMServiceReconciler for a KServe platform", func() {
	var (
		client            client.Client
		reconciler        *NIMServiceReconciler
		scheme            *runtime.Scheme
		nimService        *appsv1alpha1.NIMService
		nimCache          *appsv1alpha1.NIMCache
		volumeMounts      []corev1.VolumeMount
		volumes           []corev1.Volume
		testServerHandler http.HandlerFunc
		testServer        *httptest.Server
		originalTransport = http.DefaultTransport
		discoveryClient   *discoveryfake.FakeDiscovery
	)
	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(appsv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())
		Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
		Expect(autoscalingv2.AddToScheme(scheme)).To(Succeed())
		Expect(networkingv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(monitoringv1.AddToScheme(scheme)).To(Succeed())
		Expect(kservev1beta1.AddToScheme(scheme)).To(Succeed())
		Expect(gatewayv1.Install(scheme)).To(Succeed())

		client = fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&appsv1alpha1.NIMService{}).
			WithStatusSubresource(&appsv1alpha1.NIMCache{}).
			Build()

		boolTrue := true
		cwd, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		// Create mock discovery client
		discoveryClient = &discoveryfake.FakeDiscovery{Fake: &testing.Fake{}}
		discoveryClient.Resources = []*metav1.APIResourceList{
			{
				GroupVersion: resourcev1.SchemeGroupVersion.String(),
				APIResources: []metav1.APIResource{
					{Name: "resourceclaims"},
				},
			},
		}
		discoveryClient.FakedServerVersion = &version.Info{
			GitVersion: "v1.34.0",
		}

		reconciler = &NIMServiceReconciler{
			Client:          client,
			scheme:          scheme,
			updater:         conditions.NewUpdater(client),
			renderer:        render.NewRenderer(path.Join(strings.TrimSuffix(cwd, "internal/controller/platform/kserve"), "manifests")),
			recorder:        record.NewFakeRecorder(1000),
			discoveryClient: discoveryClient,
		}
		pvcName := "test-pvc"
		nimService = &appsv1alpha1.NIMService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-nimservice",
				Namespace: "default",
			},
			Spec: appsv1alpha1.NIMServiceSpec{
				Labels:      map[string]string{"app": "test-app"},
				Annotations: map[string]string{"annotation-key": "annotation-value"},
				Image:       appsv1alpha1.Image{Repository: "nvcr.io/nvidia/nim-llm", PullPolicy: "IfNotPresent", Tag: "v0.1.0", PullSecrets: []string{"ngc-secret"}},
				Storage: appsv1alpha1.NIMServiceStorage{
					PVC: appsv1alpha1.PersistentVolumeClaim{
						Name: pvcName,
					},
					NIMCache: appsv1alpha1.NIMCacheVolSpec{
						Name: "test-nimcache",
					},
				},
				Replicas: ptr.To(int32(1)),
				Env: []corev1.EnvVar{
					{
						Name:  "custom-env",
						Value: "custom-value",
					},
					{
						Name:  "NIM_CACHE_PATH",
						Value: "/model-store",
					},
					{
						Name: "NGC_API_KEY",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "ngc-api-secret",
								},
								Key: "NGC_API_KEY",
							},
						},
					},
					{
						Name:  "OUTLINES_CACHE_DIR",
						Value: "/tmp/outlines",
					},
					{
						Name:  "NIM_SERVER_PORT",
						Value: "9000",
					},
					{
						Name:  "NIM_HTTP_API_PORT",
						Value: "9000",
					},
					{
						Name:  "NIM_JSONL_LOGGING",
						Value: "1",
					},
					{
						Name:  "NIM_LOG_LEVEL",
						Value: "info",
					},
				},
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
				},
				NodeSelector: map[string]string{"disktype": "ssd"},
				Tolerations: []corev1.Toleration{
					{
						Key:      "key1",
						Operator: corev1.TolerationOpEqual,
						Value:    "value1",
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
				Expose: appsv1alpha1.Expose{
					Service: appsv1alpha1.Service{Type: corev1.ServiceTypeLoadBalancer, Port: ptr.To[int32](8123), Annotations: map[string]string{"annotation-key-specific": "service"}},
					Router: appsv1alpha1.Router{
						Ingress: &appsv1alpha1.RouterIngress{
							IngressClass: "nginx",
						},
					},
				},
				Scale: appsv1alpha1.Autoscaling{
					Enabled:     ptr.To[bool](true),
					Annotations: map[string]string{"annotation-key-specific": "HPA"},
					HPA: appsv1alpha1.HorizontalPodAutoscalerSpec{
						MinReplicas: ptr.To[int32](1),
						MaxReplicas: 10,
						Metrics: []autoscalingv2.MetricSpec{
							{
								Type: autoscalingv2.ResourceMetricSourceType,
								Resource: &autoscalingv2.ResourceMetricSource{
									Name: corev1.ResourceCPU,
									Target: autoscalingv2.MetricTarget{
										Type:               autoscalingv2.UtilizationMetricType,
										AverageUtilization: ptr.To[int32](80),
									},
								},
							},
						},
					},
				},
				Metrics: appsv1alpha1.Metrics{
					Enabled: &boolTrue,
					ServiceMonitor: appsv1alpha1.ServiceMonitor{
						Annotations:   map[string]string{"annotation-key-specific": "service-monitor"},
						Interval:      "1m",
						ScrapeTimeout: "30s",
					},
				},
				ReadinessProbe: appsv1alpha1.Probe{
					Enabled: &boolTrue,
					Probe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/ready",
								Port: intstr.IntOrString{IntVal: 8000},
							},
						},
					},
				},
				LivenessProbe: appsv1alpha1.Probe{
					Enabled: &boolTrue,
					Probe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/live",
								Port: intstr.IntOrString{IntVal: 8000},
							},
						},
					},
				},
				StartupProbe: appsv1alpha1.Probe{
					Enabled: &boolTrue,
					Probe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/start",
								Port: intstr.IntOrString{IntVal: 8000},
							},
						},
					},
				},
				RuntimeClassName: "nvidia",
			},
			Status: appsv1alpha1.NIMServiceStatus{
				State: conditions.NotReady,
			},
		}

		volumes = []corev1.Volume{
			{
				Name: "dshm",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{
						Medium: corev1.StorageMediumMemory,
					},
				},
			},
			{
				Name: "scratch",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{
						Medium: corev1.StorageMediumDefault,
					},
				},
			},
			{
				Name: "model-store",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "test-pvc",
						ReadOnly:  false,
					},
				},
			},
		}

		volumeMounts = []corev1.VolumeMount{
			{
				Name:      "model-store",
				MountPath: "/model-store",
				SubPath:   "subPath",
			},
			{
				Name:      "dshm",
				MountPath: "/dev/shm",
			},
			{
				Name:      "scratch",
				MountPath: "/scratch",
			},
		}
		nimCache = &appsv1alpha1.NIMCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-nimcache",
				Namespace: "default",
			},
			Spec: appsv1alpha1.NIMCacheSpec{
				Source: appsv1alpha1.NIMSource{
					NGC: &appsv1alpha1.NGCSource{
						ModelPuller: "test-container",
						PullSecret:  "my-secret",
					},
				},
				Storage: appsv1alpha1.NIMCacheStorage{
					PVC: appsv1alpha1.PersistentVolumeClaim{
						Create:       ptr.To[bool](true),
						StorageClass: "standard", Size: "1Gi", SubPath: "subPath",
					},
				},
			},
			Status: appsv1alpha1.NIMCacheStatus{
				State: appsv1alpha1.NimCacheStatusReady,
				PVC:   pvcName,
				Profiles: []appsv1alpha1.NIMProfile{{
					Name:   "test-profile",
					Config: map[string]string{"tp": "4"}},
				},
			},
		}
		err = client.Create(context.TODO(), nimCache)
		Expect(err).ShouldNot(HaveOccurred())
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: "default",
			},
		}
		err = client.Create(context.TODO(), pvc)
		Expect(err).ShouldNot(HaveOccurred())

		kserveDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kserve-controller-manager",
				Namespace: "default",
				Labels: map[string]string{
					"app.kubernetes.io/name": "kserve-controller-manager",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](1),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "nim-test-kserve",
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app": "nim-test-kserve",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "nim-test-kserve-container",
								Image: "nim-test-kserve-image",
								Ports: []corev1.ContainerPort{
									{
										ContainerPort: 80,
									},
								},
							},
						},
					},
				},
			},
		}
		err = client.Create(context.TODO(), kserveDeployment)
		Expect(err).NotTo(HaveOccurred())

		isvcConfig := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      kserveconstants.InferenceServiceConfigMapName,
				Namespace: "default",
			},
			Data: map[string]string{
				"deploy": "{\"defaultDeploymentMode\": \"RawDeployment\"}",
			},
		}
		err = client.Create(context.TODO(), isvcConfig)
		Expect(err).NotTo(HaveOccurred())

		var buf bytes.Buffer
		log.SetOutput(&buf)
		defer func() {
			log.SetOutput(os.Stderr)
		}()

		// Start mock test server to serve nimservice endpoint.
		testServerHandler = func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/models" {
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte(`{"object": "list", "data":[{"id": "dummy-model", "object": "model", "root": "dummy-model", "parent": null}]}`))
				Expect(err).ToNot(HaveOccurred())
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}
		testServer = httptest.NewServer(testServerHandler)
		http.DefaultTransport = &mockTransport{
			targetHost:        "127.0.0.1:8123",
			testServer:        testServer,
			originalTransport: originalTransport,
		}
	})

	AfterEach(func() {
		defer func() { http.DefaultTransport = originalTransport }()
		defer testServer.Close()
		// Clean up the NIMService instance
		nimService := &appsv1alpha1.NIMService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-nimservice",
				Namespace: "default",
			},
		}
		_ = client.Delete(context.TODO(), nimService)

		// Ensure that nimCache status is ready before each test
		nimCache.Status = appsv1alpha1.NIMCacheStatus{
			State: appsv1alpha1.NimCacheStatusReady,
		}

		// Update nimCache status
		Expect(client.Status().Update(context.TODO(), nimCache)).To(Succeed())

		By("delete the KServe Deployment")
		kserveDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kserve-controller-manager",
				Namespace: "default",
			},
		}
		err := client.Delete(context.TODO(), kserveDeployment)
		Expect(err).NotTo(HaveOccurred())

		By("delete the isvc ConfigMap")
		isvcConfig := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      kserveconstants.InferenceServiceConfigMapName,
				Namespace: "default",
			},
		}
		err = client.Delete(context.TODO(), isvcConfig)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Reconcile", func() {
		It("should create all resources for the NIMService", func() {
			namespacedName := types.NamespacedName{Name: nimService.Name, Namespace: nimService.Namespace}
			err := client.Create(context.TODO(), nimService)
			Expect(err).NotTo(HaveOccurred())

			result, err := reconciler.reconcileNIMService(context.TODO(), nimService)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Role should be created
			role := &rbacv1.Role{}
			err = client.Get(context.TODO(), namespacedName, role)
			Expect(err).NotTo(HaveOccurred())
			Expect(role.Name).To(Equal(nimService.GetName()))
			Expect(role.Namespace).To(Equal(nimService.GetNamespace()))

			// RoleBinding should be created
			roleBinding := &rbacv1.RoleBinding{}
			err = client.Get(context.TODO(), namespacedName, roleBinding)
			Expect(err).NotTo(HaveOccurred())
			Expect(roleBinding.Name).To(Equal(nimService.GetName()))
			Expect(roleBinding.Namespace).To(Equal(nimService.GetNamespace()))

			// Service Account should be created
			serviceAccount := &corev1.ServiceAccount{}
			err = client.Get(context.TODO(), namespacedName, serviceAccount)
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceAccount.Name).To(Equal(nimService.GetName()))
			Expect(serviceAccount.Namespace).To(Equal(nimService.GetNamespace()))

			// Service Monitor should be created
			sm := &monitoringv1.ServiceMonitor{}
			err = client.Get(context.TODO(), namespacedName, sm)
			Expect(err).NotTo(HaveOccurred())
			Expect(sm.Name).To(Equal(nimService.GetName()))
			Expect(sm.Namespace).To(Equal(nimService.GetNamespace()))
			Expect(sm.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(sm.Annotations["annotation-key-specific"]).To(Equal("service-monitor"))
			Expect(sm.Spec.Endpoints[0].Port).To(Equal("api"))
			Expect(sm.Spec.Endpoints[0].ScrapeTimeout).To(Equal(monitoringv1.Duration("30s")))
			Expect(sm.Spec.Endpoints[0].Interval).To(Equal(monitoringv1.Duration("1m")))

			// InferenceService should be created
			isvc := &kservev1beta1.InferenceService{}
			err = client.Get(context.TODO(), namespacedName, isvc)
			Expect(err).NotTo(HaveOccurred())
			Expect(isvc.Name).To(Equal(nimService.GetName()))
			Expect(isvc.Namespace).To(Equal(nimService.GetNamespace()))
			Expect(isvc.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(isvc.Spec.Predictor.Containers[0].Name).To(Equal(nimService.GetContainerName()))
			Expect(isvc.Spec.Predictor.Containers[0].Image).To(Equal(nimService.GetImage()))
			Expect(isvc.Spec.Predictor.Containers[0].ReadinessProbe).To(Equal(nimService.Spec.ReadinessProbe.Probe))
			Expect(isvc.Spec.Predictor.Containers[0].LivenessProbe).To(Equal(nimService.Spec.LivenessProbe.Probe))
			Expect(isvc.Spec.Predictor.Containers[0].StartupProbe).To(Equal(nimService.Spec.StartupProbe.Probe))
			Expect(*isvc.Spec.Predictor.RuntimeClassName).To(Equal(nimService.Spec.RuntimeClassName))

			sortEnvVars(isvc.Spec.Predictor.Containers[0].Env)
			sortEnvVars(nimService.Spec.Env)
			Expect(isvc.Spec.Predictor.Containers[0].Env).To(Equal(nimService.Spec.Env))

			sortVolumes(isvc.Spec.Predictor.Volumes)
			sortVolumes(volumes)
			Expect(isvc.Spec.Predictor.Volumes).To(Equal(volumes))

			sortVolumeMounts(isvc.Spec.Predictor.Containers[0].VolumeMounts)
			sortVolumeMounts(volumeMounts)
			Expect(isvc.Spec.Predictor.Containers[0].VolumeMounts).To(Equal(volumeMounts))

			Expect(isvc.Spec.Predictor.NodeSelector).To(Equal(nimService.Spec.NodeSelector))
			Expect(isvc.Spec.Predictor.Tolerations).To(Equal(nimService.Spec.Tolerations))

			Expect(*isvc.Spec.Predictor.MinReplicas).To(Equal(int32(1)))
			Expect(isvc.Spec.Predictor.MaxReplicas).To(Equal(int32(10)))
			Expect(*isvc.Spec.Predictor.ScaleMetricType).To(Equal(kservev1beta1.UtilizationMetricType))
			Expect(*isvc.Spec.Predictor.ScaleMetric).To(Equal(kservev1beta1.MetricCPU))
			Expect(*isvc.Spec.Predictor.ScaleTarget).To(Equal(int32(80)))

			// Verify the named ports
			expectedPorts := map[string]int32{
				"api": 8123,
			}
			foundPorts := make(map[string]int32)
			for _, port := range isvc.Spec.Predictor.Containers[0].Ports {
				foundPorts[port.Name] = port.ContainerPort
			}
			for name, expectedPort := range expectedPorts {
				Expect(foundPorts).To(HaveKeyWithValue(name, expectedPort),
					fmt.Sprintf("Expected service to have named port %q with port %d", name, expectedPort))
			}
		})

		Context("spec reconciliation with DRAResources", func() {
			It("should request resource claims", func() {
				nimService.Spec.DRAResources = []appsv1alpha1.DRAResource{
					{
						ResourceClaimName: ptr.To("test-resource-claim"),
					},
				}
				nimServiceKey := types.NamespacedName{Name: nimService.Name, Namespace: nimService.Namespace}
				err := client.Create(context.TODO(), nimService)
				Expect(err).NotTo(HaveOccurred())

				_, err = reconciler.reconcileNIMService(context.TODO(), nimService)
				Expect(err).NotTo(HaveOccurred())
				isvc := &kservev1beta1.InferenceService{}
				err = client.Get(context.TODO(), nimServiceKey, isvc)
				Expect(err).NotTo(HaveOccurred())

				// Pod spec validations.
				podSpec := isvc.Spec.Predictor.PodSpec
				Expect(podSpec.ResourceClaims).To(HaveLen(1))
				Expect(podSpec.ResourceClaims[0].ResourceClaimName).To(Equal(ptr.To("test-resource-claim")))

				// Container spec validations.
				Expect(podSpec.Containers[0].Resources.Claims).To(HaveLen(1))
				Expect(podSpec.Containers[0].Resources.Claims[0].Name).To(Equal(podSpec.ResourceClaims[0].Name))
			})

			It("should request resource claims templates", func() {
				nimService.Spec.DRAResources = []appsv1alpha1.DRAResource{
					{
						ResourceClaimTemplateName: ptr.To("test-resource-claim-template"),
					},
				}
				nimServiceKey := types.NamespacedName{Name: nimService.Name, Namespace: nimService.Namespace}
				err := client.Create(context.TODO(), nimService)
				Expect(err).NotTo(HaveOccurred())

				_, err = reconciler.reconcileNIMService(context.TODO(), nimService)
				Expect(err).NotTo(HaveOccurred())
				isvc := &kservev1beta1.InferenceService{}
				err = client.Get(context.TODO(), nimServiceKey, isvc)
				Expect(err).NotTo(HaveOccurred())

				// Pod spec validations.
				podSpec := isvc.Spec.Predictor.PodSpec
				Expect(podSpec.ResourceClaims).To(HaveLen(1))
				Expect(podSpec.ResourceClaims[0].ResourceClaimTemplateName).To(Equal(ptr.To("test-resource-claim-template")))

				// Container spec validations.
				Expect(podSpec.Containers[0].Resources.Claims).To(HaveLen(1))
				Expect(podSpec.Containers[0].Resources.Claims[0].Name).To(Equal(podSpec.ResourceClaims[0].Name))
			})

			It("should only contain the requests from the resource claims", func() {
				nimService.Spec.DRAResources = []appsv1alpha1.DRAResource{
					{
						ResourceClaimName: ptr.To("test-resource-claim"),
						Requests:          []string{"test-request-1", "test-request-2"},
					},
				}
				nimServiceKey := types.NamespacedName{Name: nimService.Name, Namespace: nimService.Namespace}
				err := client.Create(context.TODO(), nimService)
				Expect(err).NotTo(HaveOccurred())

				_, err = reconciler.reconcileNIMService(context.TODO(), nimService)
				Expect(err).NotTo(HaveOccurred())

				isvc := &kservev1beta1.InferenceService{}
				err = client.Get(context.TODO(), nimServiceKey, isvc)
				Expect(err).NotTo(HaveOccurred())

				// Pod spec validations.
				podSpec := isvc.Spec.Predictor.PodSpec
				Expect(podSpec.ResourceClaims).To(HaveLen(1))
				Expect(podSpec.ResourceClaims[0].ResourceClaimName).To(Equal(ptr.To("test-resource-claim")))

				// Container spec validations.
				Expect(podSpec.Containers[0].Resources.Claims).To(HaveLen(2))
				Expect(podSpec.Containers[0].Resources.Claims[0].Name).To(Equal(podSpec.ResourceClaims[0].Name))
				Expect(podSpec.Containers[0].Resources.Claims[0].Request).To(Equal("test-request-1"))
				Expect(podSpec.Containers[0].Resources.Claims[1].Name).To(Equal(podSpec.ResourceClaims[0].Name))
				Expect(podSpec.Containers[0].Resources.Claims[1].Request).To(Equal("test-request-2"))
			})

			It("should mark NIMService as failed when cluster version is less than v1.34.0", func() {
				reconciler.discoveryClient = &discoveryfake.FakeDiscovery{
					Fake: &testing.Fake{},
					FakedServerVersion: &version.Info{
						GitVersion: "v1.30.5",
					},
				}

				nimService.Spec.DRAResources = []appsv1alpha1.DRAResource{
					{
						ResourceClaimName: ptr.To("test-resource-claim"),
					},
				}

				nimServiceKey := types.NamespacedName{Name: nimService.Name, Namespace: nimService.Namespace}
				err := client.Create(context.TODO(), nimService)
				Expect(err).NotTo(HaveOccurred())

				_, err = reconciler.reconcileNIMService(context.TODO(), nimService)
				Expect(err).NotTo(HaveOccurred())

				obj := &appsv1alpha1.NIMService{}
				err = client.Get(context.TODO(), nimServiceKey, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Status.State).To(Equal(appsv1alpha1.NIMServiceStatusFailed))
				failedCondition := getCondition(obj, conditions.Failed)
				Expect(failedCondition).NotTo(BeNil())
				Expect(failedCondition.Status).To(Equal(metav1.ConditionTrue))
				Expect(failedCondition.Reason).To(Equal(conditions.ReasonDRAResourcesUnsupported))
				Expect(failedCondition.Message).To(Equal("DRA resources are not supported by NIM-Operator on this cluster, please upgrade to k8s version 'v1.34.0' or higher"))
			})

			It("should mark NIMService as failed when resource claim name is duplicated", func() {
				nimService.Spec.DRAResources = []appsv1alpha1.DRAResource{
					{
						ResourceClaimName: ptr.To("test-resource-claim"),
					},
					{
						ResourceClaimName: ptr.To("test-resource-claim"),
					},
				}
				nimServiceKey := types.NamespacedName{Name: nimService.Name, Namespace: nimService.Namespace}
				err := client.Create(context.TODO(), nimService)
				Expect(err).NotTo(HaveOccurred())

				_, err = reconciler.reconcileNIMService(context.TODO(), nimService)
				Expect(err).NotTo(HaveOccurred())

				obj := &appsv1alpha1.NIMService{}
				err = client.Get(context.TODO(), nimServiceKey, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.Status.State).To(Equal(appsv1alpha1.NIMServiceStatusFailed))
				failedCondition := getCondition(obj, conditions.Failed)
				Expect(failedCondition).NotTo(BeNil())
				Expect(failedCondition.Status).To(Equal(metav1.ConditionTrue))
				Expect(failedCondition.Reason).To(Equal(conditions.ReasonDRAResourcesUnsupported))
				Expect(failedCondition.Message).To(Equal("spec.draResources[1].resourceClaimName: duplicate resource claim name: 'test-resource-claim'"))
			})
		})

		It("should delete InferenceService when the NIMService is deleted", func() {
			nimServiceKey := types.NamespacedName{Name: nimService.Name, Namespace: nimService.Namespace}
			err := client.Create(context.TODO(), nimService)
			Expect(err).NotTo(HaveOccurred())

			_, err = reconciler.reconcileNIMService(context.TODO(), nimService)
			Expect(err).NotTo(HaveOccurred())

			isvc := &kservev1beta1.InferenceService{}
			err = client.Get(context.TODO(), nimServiceKey, isvc)
			Expect(err).NotTo(HaveOccurred())

			err = client.Delete(context.TODO(), nimService)
			Expect(err).NotTo(HaveOccurred())

			// Simulate the finalizer logic
			err = reconciler.cleanupNIMService(context.TODO(), nimService)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should update HPA when NIMService is updated", func() {
			namespacedName := types.NamespacedName{Name: nimService.Name, Namespace: nimService.Namespace}
			err := client.Create(context.TODO(), nimService)
			Expect(err).NotTo(HaveOccurred())

			result, err := reconciler.reconcileNIMService(context.TODO(), nimService)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			isvc := &kservev1beta1.InferenceService{}
			err = client.Get(context.TODO(), namespacedName, isvc)
			Expect(err).NotTo(HaveOccurred())

			// HPA should be specified
			Expect(*isvc.Spec.Predictor.MinReplicas).To(Equal(int32(1)))
			Expect(isvc.Spec.Predictor.MaxReplicas).To(Equal(int32(10)))
			Expect(*isvc.Spec.Predictor.ScaleMetricType).To(Equal(kservev1beta1.UtilizationMetricType))
			Expect(*isvc.Spec.Predictor.ScaleMetric).To(Equal(kservev1beta1.MetricCPU))
			Expect(*isvc.Spec.Predictor.ScaleTarget).To(Equal(int32(80)))

			// Ingress not disabled
			_, found := isvc.Labels[kserveconstants.NetworkVisibility]
			Expect(found).Should(BeFalse())

			nimService := &appsv1alpha1.NIMService{}
			err = client.Get(context.TODO(), namespacedName, nimService)
			Expect(err).NotTo(HaveOccurred())
			nimService.Spec.Scale.Enabled = ptr.To(false)
			nimService.Spec.Expose.Router.Ingress = nil
			err = client.Update(context.TODO(), nimService)
			Expect(err).NotTo(HaveOccurred())

			result, err = reconciler.reconcileNIMService(context.TODO(), nimService)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			err = client.Get(context.TODO(), namespacedName, isvc)
			Expect(err).NotTo(HaveOccurred())

			Expect(*isvc.Spec.Predictor.MinReplicas).To(Equal(int32(1)))
			Expect(isvc.Spec.Predictor.MaxReplicas).To(Equal(int32(0)))
			Expect(isvc.Spec.Predictor.ScaleMetricType).To(BeNil())
			Expect(isvc.Spec.Predictor.ScaleMetric).To(BeNil())
			Expect(isvc.Spec.Predictor.ScaleTarget).To(BeNil())

			visibility, found := isvc.Labels[kserveconstants.NetworkVisibility]
			Expect(found).Should(BeTrue())
			Expect(visibility).Should(Equal(kserveconstants.ClusterLocalVisibility))
		})
	})

	It("should be NotReady when nimcache is not ready", func() {
		nimCache.Status = appsv1alpha1.NIMCacheStatus{
			State: appsv1alpha1.NimCacheStatusNotReady,
		}
		Expect(client.Status().Update(context.TODO(), nimCache)).To(Succeed())
		err := client.Create(context.TODO(), nimService)
		Expect(err).NotTo(HaveOccurred())

		result, err := reconciler.reconcileNIMService(context.TODO(), nimService)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))

		// Check that the NIMService is not ready.
		namespacedName := types.NamespacedName{Name: nimService.Name, Namespace: nimService.Namespace}
		obj := &appsv1alpha1.NIMService{}
		err = client.Get(context.TODO(), namespacedName, obj)
		Expect(err).NotTo(HaveOccurred())
		Expect(obj.Status.State).To(Equal(appsv1alpha1.NIMServiceStatusNotReady))
		readyCondition := getCondition(obj, conditions.Ready)
		Expect(readyCondition).NotTo(BeNil())
		Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
		Expect(readyCondition.Reason).To(Equal(conditions.ReasonNIMCacheNotReady))
	})

	It("should be Failed when nimcache is not found", func() {
		testNimService := nimService.DeepCopy()
		testNimService.Spec.Storage.NIMCache.Name = "invalid-nimcache"
		err := client.Create(context.TODO(), testNimService)
		Expect(err).NotTo(HaveOccurred())

		result, err := reconciler.reconcileNIMService(context.TODO(), testNimService)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))

		// Check that the NIMService is in failed state.
		namespacedName := types.NamespacedName{Name: testNimService.Name, Namespace: testNimService.Namespace}
		obj := &appsv1alpha1.NIMService{}
		err = client.Get(context.TODO(), namespacedName, obj)
		Expect(err).NotTo(HaveOccurred())
		Expect(obj.Status.State).To(Equal(appsv1alpha1.NIMServiceStatusFailed))
		failedCondition := getCondition(obj, conditions.Failed)
		Expect(failedCondition).NotTo(BeNil())
		Expect(failedCondition.Status).To(Equal(metav1.ConditionTrue))
		Expect(failedCondition.Reason).To(Equal(conditions.ReasonNIMCacheNotFound))
	})

	It("should be Failed when nimcache is in failed state", func() {
		nimCache.Status = appsv1alpha1.NIMCacheStatus{
			State: appsv1alpha1.NimCacheStatusFailed,
			Conditions: []metav1.Condition{
				{
					Type:    conditions.Failed,
					Status:  metav1.ConditionTrue,
					Reason:  conditions.Failed,
					Message: "NIMCache failed",
				},
			},
		}
		Expect(client.Status().Update(context.TODO(), nimCache)).To(Succeed())

		err := client.Create(context.TODO(), nimService)
		Expect(err).NotTo(HaveOccurred())

		result, err := reconciler.reconcileNIMService(context.TODO(), nimService)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))

		// Check that the NIMService is in failed state.
		namespacedName := types.NamespacedName{Name: nimService.Name, Namespace: nimService.Namespace}
		obj := &appsv1alpha1.NIMService{}
		err = client.Get(context.TODO(), namespacedName, obj)
		Expect(err).NotTo(HaveOccurred())
		Expect(obj.Status.State).To(Equal(appsv1alpha1.NIMServiceStatusFailed))
		failedCondition := getCondition(obj, conditions.Failed)
		Expect(failedCondition).NotTo(BeNil())
		Expect(failedCondition.Status).To(Equal(metav1.ConditionTrue))
		Expect(failedCondition.Reason).To(Equal(conditions.ReasonNIMCacheFailed))
	})

	Describe("isInferenceServiceReady for setting status on NIMService", func() {
		var isvc *kservev1beta1.InferenceService
		BeforeEach(func() {
			isvc = &kservev1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimservice",
					Namespace: "default",
				},
				Spec:   kservev1beta1.InferenceServiceSpec{},
				Status: kservev1beta1.InferenceServiceStatus{},
			}
			_ = client.Create(context.TODO(), isvc)
		})
		AfterEach(func() {
			// Clean up the InferenceService instance
			_ = client.Delete(context.TODO(), isvc)
		})

		It("InferenceService not created", func() {
			_ = client.Delete(context.TODO(), isvc)
			msg, ready, err := reconciler.isInferenceServiceReady(context.TODO(), nimService)
			Expect(err).ToNot(HaveOccurred())
			Expect(ready).To(Equal(false))
			Expect(msg).To(Equal("Waiting for InferenceService \"test-nimservice\" creation"))
		})

		It("Predictor status not created", func() {
			msg, ready, err := reconciler.isInferenceServiceReady(context.TODO(), nimService)
			Expect(err).ToNot(HaveOccurred())
			Expect(ready).To(Equal(false))
			Expect(msg).To(Equal("Waiting for InferenceService \"test-nimservice\" reporting Predictor readiness"))
		})

		It("Predictor status successfully reported", func() {
			isvc.Status.Conditions = knativeduckv1.Conditions{
				{
					Type:    kservev1beta1.PredictorReady,
					Message: "Predictor Ready",
					Status:  corev1.ConditionTrue,
				},
			}
			err := client.Update(context.TODO(), isvc)
			Expect(err).ToNot(HaveOccurred())
			msg, ready, err := reconciler.isInferenceServiceReady(context.TODO(), nimService)
			Expect(err).ToNot(HaveOccurred())
			Expect(ready).To(Equal(true))
			Expect(msg).To(Equal("Predictor Ready"))
		})
	})

	Describe("update model status on NIMService", func() {
		var isvc *kservev1beta1.InferenceService
		BeforeEach(func() {
			isvc = &kservev1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimservice",
					Namespace: "default",
				},
				Spec:   kservev1beta1.InferenceServiceSpec{},
				Status: kservev1beta1.InferenceServiceStatus{},
			}
			_ = client.Create(context.TODO(), isvc)
		})
		AfterEach(func() {
			// Clean up the InferenceService instance
			_ = client.Delete(context.TODO(), isvc)
		})

		It("should fail when NIMService is unreachable", func() {
			var err error
			isvc.Status.URL, err = knativeapis.ParseURL("external.example.com")
			Expect(err).ToNot(HaveOccurred())
			isvc.Status.Address = &knativeduckv1.Addressable{}
			isvc.Status.Address.URL, err = knativeapis.ParseURL("cluster.example.com")
			Expect(err).ToNot(HaveOccurred())
			err = client.Update(context.TODO(), isvc)
			Expect(err).ToNot(HaveOccurred())

			err = reconciler.updateModelStatus(context.Background(), nimService, kserveconstants.LegacyServerless)
			Expect(err).To(HaveOccurred())
			Expect(nimService.Status.Model).To(BeNil())
		})

		It("should fail when models response is unmarshallable", func() {
			testServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/models" {
					w.WriteHeader(http.StatusOK)
					_, err := w.Write([]byte(`{"data": "invalid response"}`))
					Expect(err).ToNot(HaveOccurred())
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
			})

			var err error
			isvc.Status.URL, err = knativeapis.ParseURL("http://127.0.0.1:8123")
			Expect(err).ToNot(HaveOccurred())
			isvc.Status.Address = &knativeduckv1.Addressable{}
			isvc.Status.Address.URL, err = knativeapis.ParseURL("http://127.0.0.1:8123")
			Expect(err).ToNot(HaveOccurred())
			err = client.Update(context.TODO(), isvc)
			Expect(err).ToNot(HaveOccurred())

			err = reconciler.updateModelStatus(context.Background(), nimService, kserveconstants.LegacyServerless)
			Expect(err).To(HaveOccurred())
			Expect(nimService.Status.Model).To(BeNil())
		})

		It("should have empty model name when it cannot be inferred", func() {
			testServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/models" {
					w.WriteHeader(http.StatusOK)
					// Set dummy object type for model.
					_, err := w.Write([]byte(`{"object": "list", "data":[{"id": "dummy-model", "object": "dummy", "root": "dummy-model", "parent": null}]}`))
					Expect(err).ToNot(HaveOccurred())
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
			})

			var err error
			isvc.Status.URL, err = knativeapis.ParseURL("http://127.0.0.1:8123")
			Expect(err).ToNot(HaveOccurred())
			isvc.Status.Address = &knativeduckv1.Addressable{}
			isvc.Status.Address.URL, err = knativeapis.ParseURL("http://127.0.0.1:8123")
			Expect(err).ToNot(HaveOccurred())
			err = client.Update(context.TODO(), isvc)
			Expect(err).ToNot(HaveOccurred())

			err = reconciler.updateModelStatus(context.Background(), nimService, kserveconstants.LegacyServerless)
			Expect(err).ToNot(HaveOccurred())
			Expect(nimService.Status.Model).ToNot(BeNil())
			Expect(nimService.Status.Model.Name).ToNot(BeEmpty())
		})

		It("should set model status on NIMService", func() {
			var err error
			isvc.Status.URL, err = knativeapis.ParseURL("http://127.0.0.1:8123")
			Expect(err).ToNot(HaveOccurred())
			isvc.Status.Address = &knativeduckv1.Addressable{}
			isvc.Status.Address.URL, err = knativeapis.ParseURL("http://127.0.0.1:8123")
			Expect(err).ToNot(HaveOccurred())
			err = client.Update(context.TODO(), isvc)
			Expect(err).ToNot(HaveOccurred())

			err = reconciler.updateModelStatus(context.Background(), nimService, kserveconstants.LegacyServerless)
			Expect(err).ToNot(HaveOccurred())
			modelStatus := nimService.Status.Model
			Expect(modelStatus).ToNot(BeNil())
			Expect(modelStatus.ClusterEndpoint).To(Equal("http://127.0.0.1:8123"))
			Expect(modelStatus.ExternalEndpoint).To(Equal("http://127.0.0.1:8123"))
			Expect(modelStatus.Name).To(Equal("dummy-model"))
		})

		It("should succeed when nimservice has lora adapter models attached", func() {
			testServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/models" {
					w.WriteHeader(http.StatusOK)
					// Set dummy object type for model.
					_, err := w.Write([]byte(`{"object": "list", "data":[{"id": "dummy-model-adapter1", "object": "model", "root": "dummy-model", "parent": null}, {"id": "dummy-model-adapter2", "object": "model", "root": "dummy-model", "parent": null}, {"id": "dummy-model", "object": "model", "root": "dummy-model", "parent": null}]}`))
					Expect(err).ToNot(HaveOccurred())
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
			})

			var err error
			isvc.Status.URL, err = knativeapis.ParseURL("test-nimservice.default.example.com")
			Expect(err).ToNot(HaveOccurred())
			isvc.Status.Address = &knativeduckv1.Addressable{}
			isvc.Status.Address.URL, err = knativeapis.ParseURL("http://127.0.0.1:8123")
			Expect(err).ToNot(HaveOccurred())
			err = client.Update(context.TODO(), isvc)
			Expect(err).ToNot(HaveOccurred())

			err = reconciler.updateModelStatus(context.Background(), nimService, kserveconstants.LegacyRawDeployment)
			Expect(err).ToNot(HaveOccurred())
			modelStatus := nimService.Status.Model
			Expect(modelStatus).ToNot(BeNil())
			Expect(modelStatus.ClusterEndpoint).To(Equal("http://127.0.0.1:8123"))
			Expect(modelStatus.ExternalEndpoint).To(Equal("test-nimservice.default.example.com"))
			Expect(modelStatus.Name).To(Equal("dummy-model"))
		})

		Context("when nimservice only supports /v1/metadata", func() {
			It("should succeed when nimservice only supports /v1/metadata", func() {
				testServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/v1/models":
						w.WriteHeader(http.StatusNotFound)
					case "/v1/metadata":
						w.WriteHeader(http.StatusOK)
						_, err := w.Write([]byte(`{"modelInfo": [{"shortName": "dummy-model:dummy-version", "modelUrl": "ngc://org/team/dummy-model:dummy-version"}]}`))
						Expect(err).ToNot(HaveOccurred())
					default:
						w.WriteHeader(http.StatusNotFound)
					}
				})

				var err error
				isvc.Status.URL, err = knativeapis.ParseURL("test-nimservice.default.example.com")
				Expect(err).ToNot(HaveOccurred())
				isvc.Status.Address = &knativeduckv1.Addressable{}
				isvc.Status.Address.URL, err = knativeapis.ParseURL("http://127.0.0.1:8123")
				Expect(err).ToNot(HaveOccurred())
				err = client.Update(context.TODO(), isvc)
				Expect(err).ToNot(HaveOccurred())

				err = reconciler.updateModelStatus(context.Background(), nimService, kserveconstants.LegacyRawDeployment)
				Expect(err).ToNot(HaveOccurred())
				modelStatus := nimService.Status.Model
				Expect(modelStatus).ToNot(BeNil())
				Expect(modelStatus.ClusterEndpoint).To(Equal("http://127.0.0.1:8123"))
				Expect(modelStatus.ExternalEndpoint).To(Equal("test-nimservice.default.example.com"))
				Expect(modelStatus.Name).To(Equal("dummy-model"))
			})
		})

		It("should add NIM_MODEL_NAME environment variable when NIMCache is Universal NIM", func() {
			// Create a NIMCache with ModelEndpoint to make it Universal NIM
			modelEndpoint := "https://api.ngc.nvidia.com/v2/models/nvidia/nim-llama2-7b/versions/1.0.0"
			universalNimCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-universal-nimcache",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source: appsv1alpha1.NIMSource{
						NGC: &appsv1alpha1.NGCSource{
							ModelPuller:   "test-container",
							PullSecret:    "my-secret",
							ModelEndpoint: &modelEndpoint,
						},
					},
					Storage: appsv1alpha1.NIMCacheStorage{
						PVC: appsv1alpha1.PersistentVolumeClaim{
							Create:       ptr.To[bool](true),
							StorageClass: "standard",
							Size:         "1Gi",
							SubPath:      "subPath",
						},
					},
				},
				Status: appsv1alpha1.NIMCacheStatus{
					State: appsv1alpha1.NimCacheStatusReady,
					PVC:   "test-universal-nimcache-pvc",
					Profiles: []appsv1alpha1.NIMProfile{{
						Name:   "test-profile",
						Config: map[string]string{"tp": "4"}},
					},
				},
			}
			Expect(client.Create(context.TODO(), universalNimCache)).To(Succeed())

			// Create PVC for the universal NIMCache
			universalPVC := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-universal-nimcache-pvc",
					Namespace: "default",
				},
			}
			Expect(client.Create(context.TODO(), universalPVC)).To(Succeed())

			// Create a new NIMService instance for this test
			testNimService := &appsv1alpha1.NIMService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-universal-nimservice",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMServiceSpec{
					Labels:      map[string]string{"app": "test-app"},
					Annotations: map[string]string{"annotation-key": "annotation-value"},
					Image: appsv1alpha1.Image{
						Repository:  "nvcr.io/nvidia/nim-llm",
						Tag:         "v0.1.0",
						PullPolicy:  "IfNotPresent",
						PullSecrets: []string{"ngc-secret"},
					},
					Storage: appsv1alpha1.NIMServiceStorage{
						PVC: appsv1alpha1.PersistentVolumeClaim{
							Name:         "test-pvc",
							StorageClass: "standard",
							Size:         "1Gi",
							Create:       ptr.To[bool](true),
						},
						NIMCache: appsv1alpha1.NIMCacheVolSpec{
							Name: "test-universal-nimcache",
						},
					},
					Env: []corev1.EnvVar{
						{
							Name:  "custom-env",
							Value: "custom-value",
						},
					},
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("250m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
					},
					NodeSelector: map[string]string{"disktype": "ssd"},
					Tolerations: []corev1.Toleration{{
						Key:      "key1",
						Operator: corev1.TolerationOpEqual,
						Value:    "value1",
						Effect:   corev1.TaintEffectNoSchedule,
					}},
					Expose: appsv1alpha1.Expose{
						Service: appsv1alpha1.Service{Type: corev1.ServiceTypeLoadBalancer, Port: ptr.To[int32](8123), Annotations: map[string]string{"annotation-key-specific": "service"}},
					},
					RuntimeClassName: "nvidia",
				},
			}

			namespacedName := types.NamespacedName{Name: testNimService.Name, Namespace: testNimService.Namespace}
			Expect(client.Create(context.TODO(), testNimService)).To(Succeed())

			result, err := reconciler.reconcileNIMService(context.TODO(), testNimService)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// InferenceService should be created
			isvc := &kservev1beta1.InferenceService{}
			err = client.Get(context.TODO(), namespacedName, isvc)
			Expect(err).NotTo(HaveOccurred())
			Expect(isvc.Name).To(Equal(testNimService.GetName()))
			Expect(isvc.Namespace).To(Equal(testNimService.GetNamespace()))

			// Verify that NIM_MODEL_NAME environment variable is added
			container := isvc.Spec.Predictor.Containers[0]
			Expect(container.Env).To(ContainElement(corev1.EnvVar{
				Name:  "NIM_MODEL_NAME",
				Value: "/model-store",
			}), "NIM_MODEL_NAME environment variable should be present and set to /model-store")

			// Verify that other environment variables are still present
			var customEnv *corev1.EnvVar
			for _, env := range container.Env {
				if env.Name == "custom-env" {
					customEnv = &env
					break
				}
			}
			Expect(customEnv).NotTo(BeNil(), "Custom environment variables should still be present")
			Expect(customEnv.Value).To(Equal("custom-value"))
		})

		It("should not add NIM_MODEL_NAME environment variable when NIMCache is not Universal NIM", func() {
			// Create a regular NIMCache (not Universal NIM)
			regularNimCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-regular-nimcache",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source: appsv1alpha1.NIMSource{
						NGC: &appsv1alpha1.NGCSource{
							ModelPuller: "test-container",
							PullSecret:  "my-secret",
							// No ModelEndpoint, so it's not Universal NIM
						},
					},
					Storage: appsv1alpha1.NIMCacheStorage{
						PVC: appsv1alpha1.PersistentVolumeClaim{
							Create:       ptr.To[bool](true),
							StorageClass: "standard",
							Size:         "1Gi",
							SubPath:      "subPath",
						},
					},
				},
				Status: appsv1alpha1.NIMCacheStatus{
					State: appsv1alpha1.NimCacheStatusReady,
					PVC:   "test-regular-nimcache-pvc",
					Profiles: []appsv1alpha1.NIMProfile{{
						Name:   "test-profile",
						Config: map[string]string{"tp": "4"}},
					},
				},
			}
			Expect(client.Create(context.TODO(), regularNimCache)).To(Succeed())

			// Create PVC for the regular NIMCache
			regularPVC := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-regular-nimcache-pvc",
					Namespace: "default",
				},
			}
			Expect(client.Create(context.TODO(), regularPVC)).To(Succeed())

			// Create a new NIMService instance for this test
			testNimService := &appsv1alpha1.NIMService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-regular-nimservice",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMServiceSpec{
					Labels:      map[string]string{"app": "test-app"},
					Annotations: map[string]string{"annotation-key": "annotation-value"},
					Image: appsv1alpha1.Image{
						Repository:  "nvcr.io/nvidia/nim-llm",
						Tag:         "v0.1.0",
						PullPolicy:  "IfNotPresent",
						PullSecrets: []string{"ngc-secret"},
					},
					Storage: appsv1alpha1.NIMServiceStorage{
						PVC: appsv1alpha1.PersistentVolumeClaim{
							Name:         "test-pvc",
							StorageClass: "standard",
							Size:         "1Gi",
							Create:       ptr.To[bool](true),
						},
						NIMCache: appsv1alpha1.NIMCacheVolSpec{
							Name: "test-regular-nimcache",
						},
					},
					Env: []corev1.EnvVar{
						{
							Name:  "custom-env",
							Value: "custom-value",
						},
					},
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("250m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
					},
					NodeSelector: map[string]string{"disktype": "ssd"},
					Tolerations: []corev1.Toleration{{
						Key:      "key1",
						Operator: corev1.TolerationOpEqual,
						Value:    "value1",
						Effect:   corev1.TaintEffectNoSchedule,
					}},
					Expose: appsv1alpha1.Expose{
						Service: appsv1alpha1.Service{Type: corev1.ServiceTypeLoadBalancer, Port: ptr.To[int32](8123), Annotations: map[string]string{"annotation-key-specific": "service"}},
					},
					RuntimeClassName: "nvidia",
				},
			}

			namespacedName := types.NamespacedName{Name: testNimService.Name, Namespace: testNimService.Namespace}
			Expect(client.Create(context.TODO(), testNimService)).To(Succeed())

			result, err := reconciler.reconcileNIMService(context.TODO(), testNimService)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// InferenceService should be created
			isvc := &kservev1beta1.InferenceService{}
			err = client.Get(context.TODO(), namespacedName, isvc)
			Expect(err).NotTo(HaveOccurred())
			Expect(isvc.Name).To(Equal(testNimService.GetName()))
			Expect(isvc.Namespace).To(Equal(testNimService.GetNamespace()))

			// Verify that NIM_MODEL_NAME environment variable is NOT added
			container := isvc.Spec.Predictor.Containers[0]
			Expect(container.Env).NotTo(ContainElement(corev1.EnvVar{Name: "NIM_MODEL_NAME"}), "NIM_MODEL_NAME environment variable should not be present for non-Universal NIM")

			// Verify that other environment variables are still present
			var customEnv *corev1.EnvVar
			for _, env := range container.Env {
				if env.Name == "custom-env" {
					customEnv = &env
					break
				}
			}
			Expect(customEnv).NotTo(BeNil(), "Custom environment variables should still be present")
			Expect(customEnv.Value).To(Equal("custom-value"))
		})

		It("should respect user-provided NIM_MODEL_NAME environment variable over default for Universal NIM", func() {
			// Create a Universal NIM NIMCache
			modelEndpoint := "https://api.ngc.nvidia.com/v2/models/nvidia/nim-llama2-7b/versions/1q.0.0"
			universalNimCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-custom-universal-nimcache",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source: appsv1alpha1.NIMSource{
						NGC: &appsv1alpha1.NGCSource{
							ModelPuller:   "test-container",
							PullSecret:    "my-secret",
							ModelEndpoint: &modelEndpoint,
						},
					},
					Storage: appsv1alpha1.NIMCacheStorage{
						PVC: appsv1alpha1.PersistentVolumeClaim{
							Create:       ptr.To[bool](true),
							StorageClass: "standard",
							Size:         "1Gi",
							SubPath:      "subPath",
						},
					},
				},
				Status: appsv1alpha1.NIMCacheStatus{
					State: appsv1alpha1.NimCacheStatusReady,
					PVC:   "test-custom-universal-nimcache-pvc",
					Profiles: []appsv1alpha1.NIMProfile{{
						Name:   "test-profile",
						Config: map[string]string{"tp": "4"}},
					},
				},
			}
			Expect(client.Create(context.TODO(), universalNimCache)).To(Succeed())

			// Create PVC for the Universal NIMCache
			universalPVC := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-custom-universal-nimcache-pvc",
					Namespace: "default",
				},
			}
			Expect(client.Create(context.TODO(), universalPVC)).To(Succeed())

			// Create a new NIMService instance with custom NIM_MODEL_NAME
			testNimService := &appsv1alpha1.NIMService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-custom-universal-nimservice",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMServiceSpec{
					Env: []corev1.EnvVar{
						{
							Name:  "NIM_MODEL_NAME",
							Value: "/custom-model-path",
						},
						{
							Name:  "CUSTOM_ENV",
							Value: "custom-value",
						},
					},
					Image: appsv1alpha1.Image{
						Repository:  "nvcr.io/nvidia/nim-llm",
						Tag:         "v0.1.0",
						PullPolicy:  "IfNotPresent",
						PullSecrets: []string{"ngc-secret"},
					},
					Storage: appsv1alpha1.NIMServiceStorage{
						PVC: appsv1alpha1.PersistentVolumeClaim{
							Name:         "test-pvc",
							StorageClass: "standard",
							Size:         "1Gi",
							Create:       ptr.To[bool](true),
						},
						NIMCache: appsv1alpha1.NIMCacheVolSpec{
							Name: "test-custom-universal-nimcache",
						},
					},
					Expose: appsv1alpha1.Expose{
						Service: appsv1alpha1.Service{Type: corev1.ServiceTypeLoadBalancer, Port: ptr.To[int32](8123)},
					},
				},
			}

			namespacedName := types.NamespacedName{Name: testNimService.Name, Namespace: testNimService.Namespace}
			Expect(client.Create(context.TODO(), testNimService)).To(Succeed())

			result, err := reconciler.reconcileNIMService(context.TODO(), testNimService)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// InferenceService should be created
			isvc := &kservev1beta1.InferenceService{}
			err = client.Get(context.TODO(), namespacedName, isvc)
			Expect(err).NotTo(HaveOccurred())

			// Verify that user-provided NIM_MODEL_NAME takes precedence over default
			container := isvc.Spec.Predictor.Containers[0]
			Expect(container.Env).To(ContainElement(corev1.EnvVar{
				Name:  "NIM_MODEL_NAME",
				Value: "/custom-model-path",
			}), "User-provided NIM_MODEL_NAME environment variable should take precedence over default")

			// Verify that other user-provided environment variables are still present
			Expect(container.Env).To(ContainElement(corev1.EnvVar{
				Name:  "CUSTOM_ENV",
				Value: "custom-value",
			}), "Other user-provided environment variables should be present")

			// Verify that the default value is NOT present
			Expect(container.Env).NotTo(ContainElement(corev1.EnvVar{
				Name:  "NIM_MODEL_NAME",
				Value: "/model-store",
			}), "Default NIM_MODEL_NAME value should not be present when user provides custom value")
		})
	})

	Describe("getNIMModelEndpoints", func() {
		var isvc *kservev1beta1.InferenceService
		BeforeEach(func() {
			isvc = &kservev1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimservice",
					Namespace: "default",
				},
				Spec:   kservev1beta1.InferenceServiceSpec{},
				Status: kservev1beta1.InferenceServiceStatus{},
			}
			_ = client.Create(context.TODO(), isvc)
		})
		AfterEach(func() {
			// Clean up the InferenceService instance
			_ = client.Delete(context.TODO(), isvc)
		})

		It("should return err when InferenceService is missing", func() {
			_ = client.Delete(context.TODO(), isvc)
			_, _, err := reconciler.getNIMModelEndpoints(context.TODO(), nimService, kserveconstants.LegacyRawDeployment)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsNotFound(err)).To(BeTrue())
			Expect(err).Should(MatchError("inferenceservices.serving.kserve.io \"test-nimservice\" not found"))
		})

		It("should return error when external endpoint is not set", func() {
			_, _, err := reconciler.getNIMModelEndpoints(context.TODO(), nimService, kserveconstants.LegacyRawDeployment)
			Expect(err).To(HaveOccurred())
			Expect(err).Should(MatchError("external endpoint not available, nimservice test-nimservice"))
		})

		It("should return error when cluster endpoint is not set for RawDeployment", func() {
			var err error
			isvc.Status.URL, err = knativeapis.ParseURL("external.example.com")
			Expect(err).ToNot(HaveOccurred())
			_ = client.Update(context.TODO(), isvc)
			_, _, err = reconciler.getNIMModelEndpoints(context.TODO(), nimService, kserveconstants.LegacyRawDeployment)
			Expect(err).To(HaveOccurred())
			Expect(err).Should(MatchError("cluster endpoint not available, nimservice test-nimservice"))
		})

		It("should return cluster endpoint for Serverless mode", func() {
			var err error
			isvc.Status.URL, err = knativeapis.ParseURL("external.example.com")
			Expect(err).ToNot(HaveOccurred())
			isvc.Status.Address = &knativeduckv1.Addressable{}
			isvc.Status.Address.URL, err = knativeapis.ParseURL("cluster.example.com")
			Expect(err).ToNot(HaveOccurred())
			_ = client.Update(context.TODO(), isvc)
			internal, external, err := reconciler.getNIMModelEndpoints(context.TODO(), nimService, kserveconstants.LegacyServerless)
			Expect(err).ToNot(HaveOccurred())
			Expect(internal).To(Equal("cluster.example.com"))
			Expect(external).To(Equal("external.example.com"))
		})

		It("should return cluster endpoint for RawDeployment", func() {
			var err error
			isvc.Status.URL, err = knativeapis.ParseURL("external.example.com")
			Expect(err).ToNot(HaveOccurred())
			isvc.Status.Address = &knativeduckv1.Addressable{}
			isvc.Status.Address.URL, err = knativeapis.ParseURL("cluster.example.com")
			Expect(err).ToNot(HaveOccurred())
			_ = client.Update(context.TODO(), isvc)
			internal, external, err := reconciler.getNIMModelEndpoints(context.TODO(), nimService, kserveconstants.LegacyRawDeployment)
			Expect(err).ToNot(HaveOccurred())
			Expect(internal).To(Equal("cluster.example.com"))
			Expect(external).To(Equal("external.example.com"))
		})
	})

	Describe("getNIMCacheProfile", func() {
		It("should return nil when NIMCache is not used", func() {
			nimService.Spec.Storage.NIMCache.Name = ""
			profile, err := reconciler.getNIMCacheProfile(context.TODO(), nimService, "some-profile")
			Expect(err).To(BeNil())
			Expect(profile).To(BeNil())
		})

		It("should return an error when NIMCache is not found", func() {
			nimService.Spec.Storage.NIMCache.Name = "non-existent-cache"
			_, err := reconciler.getNIMCacheProfile(context.TODO(), nimService, "some-profile")
			Expect(err).To(HaveOccurred())
		})

		It("should return an error when NIMCache is not ready", func() {
			nimService.Spec.Storage.NIMCache.Name = "test-nimcache"
			nimCache.Status = appsv1alpha1.NIMCacheStatus{
				State: appsv1alpha1.NimCacheStatusPending,
			}

			// Update nimCache status
			Expect(reconciler.Client.Status().Update(context.TODO(), nimCache)).To(Succeed())
			_, err := reconciler.getNIMCacheProfile(context.TODO(), nimService, "test-profile")
			Expect(err).To(HaveOccurred())
		})

		It("should return nil when NIMCache profile is not found", func() {
			nimService.Spec.Storage.NIMCache.Name = "test-nimcache"
			profile, err := reconciler.getNIMCacheProfile(context.TODO(), nimService, "non-existent-profile")
			Expect(err).To(BeNil())
			Expect(profile).To(BeNil())
		})

		It("should return the profile if found in NIMCache", func() {
			nimService.Spec.Storage.NIMCache.Name = "test-nimcache"
			profile, err := reconciler.getNIMCacheProfile(context.TODO(), nimService, "test-profile")
			Expect(err).To(BeNil())
			Expect(profile.Name).To(Equal("test-profile"))
		})
	})

	Describe("getTensorParallelismByProfile", func() {
		It("should return tensor parallelism value if exists", func() {
			profile := &appsv1alpha1.NIMProfile{
				Name:   "test-profile",
				Config: map[string]string{"tp": "4"},
			}

			tensorParallelism, err := utils.GetTensorParallelismByProfileTags(profile.Config)
			Expect(err).To(BeNil())
			Expect(tensorParallelism).To(Equal("4"))
		})

		It("should return empty string if tensor parallelism does not exist", func() {
			profile := &appsv1alpha1.NIMProfile{
				Name:   "test-profile",
				Config: map[string]string{},
			}
			tensorParallelism, err := utils.GetTensorParallelismByProfileTags(profile.Config)
			Expect(err).To(BeNil())
			Expect(tensorParallelism).To(BeEmpty())
		})
	})

	Describe("addGPUResources", func() {
		It("should not provide GPU resources if user has already provided them", func() {
			profile := &appsv1alpha1.NIMProfile{
				Name:   "test-profile",
				Config: map[string]string{"tp": "4"},
			}

			// Initialize InferenceService params with user-provided GPU resources
			nimService.Spec.Resources = &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("8"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("8"),
				},
			}

			resources, err := reconciler.addGPUResources(context.TODO(), nimService, profile)
			Expect(err).ToNot(HaveOccurred())
			Expect(resources).ToNot(BeNil())

			// Ensure the user-provided GPU resources (8) are retained and not overridden
			Expect(resources.Requests).To(HaveKeyWithValue(corev1.ResourceName("nvidia.com/gpu"), resource.MustParse("8")))
			Expect(resources.Limits).To(HaveKeyWithValue(corev1.ResourceName("nvidia.com/gpu"), resource.MustParse("8")))
		})

		It("should assign GPU resources when tensor parallelism is provided", func() {
			profile := &appsv1alpha1.NIMProfile{
				Name:   "test-profile",
				Config: map[string]string{"tp": "4"},
			}

			resources, err := reconciler.addGPUResources(context.TODO(), nimService, profile)
			Expect(err).ToNot(HaveOccurred())
			Expect(resources).ToNot(BeNil())

			Expect(resources.Requests).To(HaveKeyWithValue(corev1.ResourceName("nvidia.com/gpu"), resource.MustParse("4")))
			Expect(resources.Limits).To(HaveKeyWithValue(corev1.ResourceName("nvidia.com/gpu"), resource.MustParse("4")))
		})

		It("should respect non GPU resources after adding GPU resources", func() {
			profile := &appsv1alpha1.NIMProfile{
				Name:   "test-profile",
				Config: map[string]string{},
			}

			resources, err := reconciler.addGPUResources(context.TODO(), nimService, profile)
			Expect(err).ToNot(HaveOccurred())
			Expect(resources).ToNot(BeNil())

			Expect(resources.Requests).To(HaveKeyWithValue(corev1.ResourceCPU, resource.MustParse("250m")))
			Expect(resources.Requests).To(HaveKeyWithValue(corev1.ResourceMemory, resource.MustParse("64Mi")))
			Expect(resources.Limits).To(HaveKeyWithValue(corev1.ResourceCPU, resource.MustParse("500m")))
			Expect(resources.Limits).To(HaveKeyWithValue(corev1.ResourceMemory, resource.MustParse("128Mi")))
		})

		It("should assign 1 GPU resource if tensor parallelism is not provided", func() {
			profile := &appsv1alpha1.NIMProfile{
				Name:   "test-profile",
				Config: map[string]string{},
			}

			resources, err := reconciler.addGPUResources(context.TODO(), nimService, profile)
			Expect(err).ToNot(HaveOccurred())
			Expect(resources).ToNot(BeNil())

			Expect(resources.Requests).To(HaveKeyWithValue(corev1.ResourceName("nvidia.com/gpu"), resource.MustParse("1")))
			Expect(resources.Limits).To(HaveKeyWithValue(corev1.ResourceName("nvidia.com/gpu"), resource.MustParse("1")))
		})

		It("should assign GPU resource equal to multiNode.GPUSPerPod in multi-node deployment", func() {
			nimService.Spec.MultiNode = &appsv1alpha1.NimServiceMultiNodeConfig{
				Parallelism: &appsv1alpha1.ParallelismSpec{Tensor: ptr.To(uint32(2))},
			}
			profile := &appsv1alpha1.NIMProfile{
				Name:   "test-profile",
				Config: map[string]string{"tp": "4"},
			}

			resources, err := reconciler.addGPUResources(context.TODO(), nimService, profile)
			Expect(err).ToNot(HaveOccurred())
			Expect(resources).ToNot(BeNil())

			Expect(resources.Requests).To(HaveKeyWithValue(corev1.ResourceName("nvidia.com/gpu"), resource.MustParse("2")))
			Expect(resources.Limits).To(HaveKeyWithValue(corev1.ResourceName("nvidia.com/gpu"), resource.MustParse("2")))
		})

		It("should return an error if tensor parallelism cannot be parsed", func() {
			profile := &appsv1alpha1.NIMProfile{
				Name:   "test-profile",
				Config: map[string]string{"tp": "invalid"},
			}

			_, err := reconciler.addGPUResources(context.TODO(), nimService, profile)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Reconcile NIMService with Proxy Setting", func() {
		BeforeEach(func() {
			nimService.Spec.Proxy = &appsv1alpha1.ProxySpec{
				HttpProxy:     "http://proxy:1000",
				HttpsProxy:    "https://proxy:1000",
				NoProxy:       "http://no-proxy",
				CertConfigMap: "custom-ca-configmap",
			}

		})

		It("should create InferenceService with appropriate parameters", func() {
			namespacedName := types.NamespacedName{Name: nimService.Name, Namespace: nimService.Namespace}
			err := client.Create(context.TODO(), nimService)
			Expect(err).NotTo(HaveOccurred())

			result, err := reconciler.reconcileNIMService(context.TODO(), nimService)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// InferenceService should be created
			isvc := &kservev1beta1.InferenceService{}
			err = client.Get(context.TODO(), namespacedName, isvc)
			Expect(err).NotTo(HaveOccurred())
			Expect(isvc.Name).To(Equal(nimService.GetName()))
			Expect(isvc.Namespace).To(Equal(nimService.GetNamespace()))
			Expect(isvc.Annotations["annotation-key"]).To(Equal("annotation-value"))
			Expect(isvc.Spec.Predictor.Containers[0].Name).To(Equal(nimService.GetContainerName()))
			Expect(isvc.Spec.Predictor.Containers[0].Image).To(Equal(nimService.GetImage()))
			Expect(isvc.Spec.Predictor.Containers[0].ReadinessProbe).To(Equal(nimService.Spec.ReadinessProbe.Probe))
			Expect(isvc.Spec.Predictor.Containers[0].LivenessProbe).To(Equal(nimService.Spec.LivenessProbe.Probe))
			Expect(isvc.Spec.Predictor.Containers[0].StartupProbe).To(Equal(nimService.Spec.StartupProbe.Probe))
			Expect(*isvc.Spec.Predictor.RuntimeClassName).To(Equal(nimService.Spec.RuntimeClassName))

			// Verify CertConfig volume and mounts
			Expect(isvc.Spec.Predictor.Volumes).To(ContainElement(
				corev1.Volume{
					Name: "custom-ca",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "custom-ca-configmap",
							},
						},
					},
				},
			))

			Expect(isvc.Spec.Predictor.Volumes).To(ContainElement(
				corev1.Volume{
					Name: "ca-cert-volume",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			))

			Expect(isvc.Spec.Predictor.Containers[0].VolumeMounts).To(ContainElement(
				corev1.VolumeMount{
					Name:      "ca-cert-volume",
					MountPath: "/etc/ssl",
				},
			))

			Expect(isvc.Spec.Predictor.InitContainers[0].VolumeMounts).To(ContainElement(
				corev1.VolumeMount{
					Name:      "ca-cert-volume",
					MountPath: "/ca-certs",
				},
			))

			Expect(isvc.Spec.Predictor.InitContainers[0].VolumeMounts).To(ContainElement(
				corev1.VolumeMount{
					Name:      "custom-ca",
					MountPath: "/custom",
				},
			))
			Expect(isvc.Spec.Predictor.InitContainers[0].Command).To(ContainElements(k8sutil.GetUpdateCaCertInitContainerCommand()))
			Expect(isvc.Spec.Predictor.InitContainers[0].SecurityContext).To(Equal(k8sutil.GetUpdateCaCertInitContainerSecurityContext()))

			// Expected environment variables
			expectedEnvs := map[string]string{
				"HTTP_PROXY":             "http://proxy:1000",
				"HTTPS_PROXY":            "https://proxy:1000",
				"NO_PROXY":               "http://no-proxy",
				"http_proxy":             "http://proxy:1000",
				"https_proxy":            "https://proxy:1000",
				"no_proxy":               "http://no-proxy",
				"NIM_SDK_USE_NATIVE_TLS": "1",
			}

			// Verify each custom environment variable
			for key, value := range expectedEnvs {
				var found bool
				for _, envVar := range isvc.Spec.Predictor.Containers[0].Env {
					if envVar.Name == key && envVar.Value == value {
						found = true
						break
					}
				}
				Expect(found).To(BeTrue(), "Expected environment variable %s=%s not found", key, value)
			}
		})

		Context("Hugging Face model handling", func() {
			It("should replace NGC_API_KEY with HF_TOKEN when NIMCache is a Hugging Face model", func() {
				// Create a Hugging Face NIMCache
				hfNimCache := &appsv1alpha1.NIMCache{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-hf-nimcache",
						Namespace: "default",
					},
					Spec: appsv1alpha1.NIMCacheSpec{
						Source: appsv1alpha1.NIMSource{
							HF: &appsv1alpha1.HuggingFaceHubSource{
								Endpoint:  "https://huggingface.co",
								Namespace: "meta-llama",
								DSHFCommonFields: appsv1alpha1.DSHFCommonFields{
									ModelName:   ptr.To("meta-llama/Llama-2-7b-chat-hf"),
									AuthSecret:  "hf-secret",
									ModelPuller: "nvcr.io/nvidia/hf-model-puller:latest",
									PullSecret:  "hf-secret",
								},
							},
						},
						Storage: appsv1alpha1.NIMCacheStorage{
							PVC: appsv1alpha1.PersistentVolumeClaim{
								Create:       ptr.To[bool](true),
								StorageClass: "standard",
								Size:         "1Gi",
							},
						},
					},
					Status: appsv1alpha1.NIMCacheStatus{
						State: appsv1alpha1.NimCacheStatusReady,
						PVC:   "test-hf-nimcache-pvc",
						Profiles: []appsv1alpha1.NIMProfile{{
							Name:   "test-profile",
							Config: map[string]string{"tp": "2"}},
						},
					},
				}
				Expect(client.Create(context.TODO(), hfNimCache)).To(Succeed())

				// Create PVC for the HF NIMCache
				hfPVC := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-hf-nimcache-pvc",
						Namespace: "default",
					},
				}
				Expect(client.Create(context.TODO(), hfPVC)).To(Succeed())

				// Create a NIMService that uses the HF NIMCache
				testNimService := &appsv1alpha1.NIMService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-hf-nimservice",
						Namespace: "default",
					},
					Spec: appsv1alpha1.NIMServiceSpec{
						Labels:      map[string]string{"app": "test-hf-app"},
						Annotations: map[string]string{"annotation-key": "annotation-value"},
						Image: appsv1alpha1.Image{
							Repository:  "nvcr.io/nvidia/nim-llm",
							Tag:         "v0.1.0",
							PullPolicy:  "IfNotPresent",
							PullSecrets: []string{"hf-secret"},
						},
						AuthSecret: "hf-secret",
						Storage: appsv1alpha1.NIMServiceStorage{
							NIMCache: appsv1alpha1.NIMCacheVolSpec{
								Name: "test-hf-nimcache",
							},
						},
						Env: []corev1.EnvVar{
							{
								Name:  "custom-env",
								Value: "custom-value",
							},
						},
						Resources: &corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("250m"),
								corev1.ResourceMemory: resource.MustParse("64Mi"),
							},
						},
						Expose: appsv1alpha1.Expose{
							Service: appsv1alpha1.Service{Type: corev1.ServiceTypeClusterIP, Port: ptr.To[int32](8000)},
						},
					},
				}

				namespacedName := types.NamespacedName{Name: testNimService.Name, Namespace: testNimService.Namespace}
				Expect(client.Create(context.TODO(), testNimService)).To(Succeed())

				result, err := reconciler.reconcileNIMService(context.TODO(), testNimService)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// InferenceService should be created
				isvc := &kservev1beta1.InferenceService{}
				err = client.Get(context.TODO(), namespacedName, isvc)
				Expect(err).NotTo(HaveOccurred())
				Expect(isvc.Name).To(Equal(testNimService.GetName()))
				Expect(isvc.Namespace).To(Equal(testNimService.GetNamespace()))

				// Verify environment variables
				container := isvc.Spec.Predictor.Containers[0]

				// NGC_API_KEY should NOT be present
				var ngcKeyPresent bool
				for _, env := range container.Env {
					if env.Name == appsv1alpha1.NGCAPIKey {
						ngcKeyPresent = true
						break
					}
				}
				Expect(ngcKeyPresent).To(BeFalse(), "NGC_API_KEY should not be present for HuggingFace models")

				// HF_TOKEN should be present with correct secret reference
				var hfTokenEnv *corev1.EnvVar
				for _, env := range container.Env {
					if env.Name == appsv1alpha1.HFToken {
						hfTokenEnv = &env
						break
					}
				}
				Expect(hfTokenEnv).NotTo(BeNil(), "HF_TOKEN environment variable should be present")
				Expect(hfTokenEnv.ValueFrom).NotTo(BeNil())
				Expect(hfTokenEnv.ValueFrom.SecretKeyRef).NotTo(BeNil())
				Expect(hfTokenEnv.ValueFrom.SecretKeyRef.Name).To(Equal("hf-secret"))
				Expect(hfTokenEnv.ValueFrom.SecretKeyRef.Key).To(Equal(appsv1alpha1.HFToken))

				// Verify that custom environment variables are still present
				var customEnv *corev1.EnvVar
				for _, env := range container.Env {
					if env.Name == "custom-env" {
						customEnv = &env
						break
					}
				}
				Expect(customEnv).NotTo(BeNil(), "Custom environment variables should still be present")
				Expect(customEnv.Value).To(Equal("custom-value"))
			})

			It("should replace NGC_API_KEY with HF_TOKEN when NIMCache is a DataStore source", func() {
				// Create a DataStore NIMCache
				dsNimCache := &appsv1alpha1.NIMCache{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ds-nimcache-kserve",
						Namespace: "default",
					},
					Spec: appsv1alpha1.NIMCacheSpec{
						Source: appsv1alpha1.NIMSource{
							DataStore: &appsv1alpha1.NemoDataStoreSource{
								Endpoint:  "https://datastore.nvidia.com/v1/hf/",
								Namespace: "default",
								DSHFCommonFields: appsv1alpha1.DSHFCommonFields{
									ModelName:   ptr.To("meta-llama/Llama-2-7b-chat-hf"),
									AuthSecret:  "hf-secret",
									ModelPuller: "nvcr.io/nvidia/hf-model-puller:latest",
									PullSecret:  "hf-secret",
								},
							},
						},
						Storage: appsv1alpha1.NIMCacheStorage{
							PVC: appsv1alpha1.PersistentVolumeClaim{
								Create:       ptr.To[bool](true),
								StorageClass: "standard",
								Size:         "1Gi",
							},
						},
					},
					Status: appsv1alpha1.NIMCacheStatus{
						State: appsv1alpha1.NimCacheStatusReady,
						PVC:   "test-ds-nimcache-kserve-pvc",
						Profiles: []appsv1alpha1.NIMProfile{{
							Name:   "test-profile",
							Config: map[string]string{"tp": "2"}},
						},
					},
				}
				Expect(client.Create(context.TODO(), dsNimCache)).To(Succeed())

				// Create PVC for the DataStore NIMCache
				dsPVC := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ds-nimcache-kserve-pvc",
						Namespace: "default",
					},
				}
				Expect(client.Create(context.TODO(), dsPVC)).To(Succeed())

				// Create a NIMService that uses the DataStore NIMCache
				testNimService := &appsv1alpha1.NIMService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ds-nimservice-kserve",
						Namespace: "default",
					},
					Spec: appsv1alpha1.NIMServiceSpec{
						Labels:      map[string]string{"app": "test-ds-app"},
						Annotations: map[string]string{"annotation-key": "annotation-value"},
						Image: appsv1alpha1.Image{
							Repository:  "nvcr.io/nvidia/nim-llm",
							Tag:         "v0.1.0",
							PullPolicy:  "IfNotPresent",
							PullSecrets: []string{"hf-secret"},
						},
						AuthSecret: "hf-secret",
						Storage: appsv1alpha1.NIMServiceStorage{
							NIMCache: appsv1alpha1.NIMCacheVolSpec{
								Name: "test-ds-nimcache-kserve",
							},
						},
						Env: []corev1.EnvVar{
							{
								Name:  "custom-env",
								Value: "custom-value",
							},
						},
						Resources: &corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("250m"),
								corev1.ResourceMemory: resource.MustParse("64Mi"),
							},
						},
						Expose: appsv1alpha1.Expose{
							Service: appsv1alpha1.Service{Type: corev1.ServiceTypeClusterIP, Port: ptr.To[int32](8000)},
						},
					},
				}

				namespacedName := types.NamespacedName{Name: testNimService.Name, Namespace: testNimService.Namespace}
				Expect(client.Create(context.TODO(), testNimService)).To(Succeed())

				result, err := reconciler.reconcileNIMService(context.TODO(), testNimService)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// InferenceService should be created
				isvc := &kservev1beta1.InferenceService{}
				err = client.Get(context.TODO(), namespacedName, isvc)
				Expect(err).NotTo(HaveOccurred())
				Expect(isvc.Name).To(Equal(testNimService.GetName()))
				Expect(isvc.Namespace).To(Equal(testNimService.GetNamespace()))

				// Verify environment variables
				container := isvc.Spec.Predictor.Containers[0]

				// NGC_API_KEY should NOT be present
				var ngcKeyPresent bool
				for _, env := range container.Env {
					if env.Name == appsv1alpha1.NGCAPIKey {
						ngcKeyPresent = true
						break
					}
				}
				Expect(ngcKeyPresent).To(BeFalse(), "NGC_API_KEY should not be present for DataStore models")

				// HF_TOKEN should be present with correct secret reference
				var hfTokenEnv *corev1.EnvVar
				for _, env := range container.Env {
					if env.Name == appsv1alpha1.HFToken {
						hfTokenEnv = &env
						break
					}
				}
				Expect(hfTokenEnv).NotTo(BeNil(), "HF_TOKEN environment variable should be present")
				Expect(hfTokenEnv.ValueFrom).NotTo(BeNil())
				Expect(hfTokenEnv.ValueFrom.SecretKeyRef).NotTo(BeNil())
				Expect(hfTokenEnv.ValueFrom.SecretKeyRef.Name).To(Equal("hf-secret"))
				Expect(hfTokenEnv.ValueFrom.SecretKeyRef.Key).To(Equal(appsv1alpha1.HFToken))

				// Verify that custom environment variables are still present
				var customEnv *corev1.EnvVar
				for _, env := range container.Env {
					if env.Name == "custom-env" {
						customEnv = &env
						break
					}
				}
				Expect(customEnv).NotTo(BeNil(), "Custom environment variables should still be present")
				Expect(customEnv.Value).To(Equal("custom-value"))
			})

			It("should replace NGC_API_KEY with HF_TOKEN when NIMService has HF model name", func() {
				// Create a regular NGC NIMCache (not HF)
				regularNimCache := &appsv1alpha1.NIMCache{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-regular-nimcache-kserve",
						Namespace: "default",
					},
					Spec: appsv1alpha1.NIMCacheSpec{
						Source: appsv1alpha1.NIMSource{
							NGC: &appsv1alpha1.NGCSource{
								ModelPuller: "test-container",
								PullSecret:  "my-secret",
							},
						},
						Storage: appsv1alpha1.NIMCacheStorage{
							PVC: appsv1alpha1.PersistentVolumeClaim{
								Create:       ptr.To[bool](true),
								StorageClass: "standard",
								Size:         "1Gi",
							},
						},
					},
					Status: appsv1alpha1.NIMCacheStatus{
						State: appsv1alpha1.NimCacheStatusReady,
						PVC:   "test-regular-nimcache-kserve-pvc",
						Profiles: []appsv1alpha1.NIMProfile{{
							Name:   "test-profile",
							Config: map[string]string{"tp": "2"}},
						},
					},
				}
				Expect(client.Create(context.TODO(), regularNimCache)).To(Succeed())

				// Create PVC for the NIMCache
				regularPVC := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-regular-nimcache-kserve-pvc",
						Namespace: "default",
					},
				}
				Expect(client.Create(context.TODO(), regularPVC)).To(Succeed())

				// Create a NIMService with hf:// model name
				testNimService := &appsv1alpha1.NIMService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-hf-model-nimservice-kserve",
						Namespace: "default",
					},
					Spec: appsv1alpha1.NIMServiceSpec{
						Labels:      map[string]string{"app": "test-hf-model-app"},
						Annotations: map[string]string{"annotation-key": "annotation-value"},
						Image: appsv1alpha1.Image{
							Repository:  "nvcr.io/nvidia/nim-llm",
							Tag:         "v0.1.0",
							PullPolicy:  "IfNotPresent",
							PullSecrets: []string{"hf-secret"},
						},
						AuthSecret: "hf-secret",
						Storage: appsv1alpha1.NIMServiceStorage{
							NIMCache: appsv1alpha1.NIMCacheVolSpec{
								Name: "test-regular-nimcache-kserve",
							},
						},
						Env: []corev1.EnvVar{
							{
								Name:  "NIM_MODEL_NAME",
								Value: "hf://meta-llama/Llama-2-7b-chat-hf",
							},
							{
								Name:  "custom-env",
								Value: "custom-value",
							},
						},
						Resources: &corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("250m"),
								corev1.ResourceMemory: resource.MustParse("64Mi"),
							},
						},
						Expose: appsv1alpha1.Expose{
							Service: appsv1alpha1.Service{Type: corev1.ServiceTypeClusterIP, Port: ptr.To[int32](8000)},
						},
					},
				}

				namespacedName := types.NamespacedName{Name: testNimService.Name, Namespace: testNimService.Namespace}
				Expect(client.Create(context.TODO(), testNimService)).To(Succeed())

				result, err := reconciler.reconcileNIMService(context.TODO(), testNimService)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// InferenceService should be created
				isvc := &kservev1beta1.InferenceService{}
				err = client.Get(context.TODO(), namespacedName, isvc)
				Expect(err).NotTo(HaveOccurred())
				Expect(isvc.Name).To(Equal(testNimService.GetName()))
				Expect(isvc.Namespace).To(Equal(testNimService.GetNamespace()))

				// Verify environment variables
				container := isvc.Spec.Predictor.Containers[0]

				// NGC_API_KEY should NOT be present
				var ngcKeyPresent bool
				for _, env := range container.Env {
					if env.Name == appsv1alpha1.NGCAPIKey {
						ngcKeyPresent = true
						break
					}
				}
				Expect(ngcKeyPresent).To(BeFalse(), "NGC_API_KEY should not be present for HuggingFace models")

				// HF_TOKEN should be present with correct secret reference
				var hfTokenEnv *corev1.EnvVar
				for _, env := range container.Env {
					if env.Name == appsv1alpha1.HFToken {
						hfTokenEnv = &env
						break
					}
				}
				Expect(hfTokenEnv).NotTo(BeNil(), "HF_TOKEN environment variable should be present")
				Expect(hfTokenEnv.ValueFrom).NotTo(BeNil())
				Expect(hfTokenEnv.ValueFrom.SecretKeyRef).NotTo(BeNil())
				Expect(hfTokenEnv.ValueFrom.SecretKeyRef.Name).To(Equal("hf-secret"))
				Expect(hfTokenEnv.ValueFrom.SecretKeyRef.Key).To(Equal(appsv1alpha1.HFToken))

				// Verify that NIM_MODEL_NAME is still present
				var modelNameEnv *corev1.EnvVar
				for _, env := range container.Env {
					if env.Name == "NIM_MODEL_NAME" {
						modelNameEnv = &env
						break
					}
				}
				Expect(modelNameEnv).NotTo(BeNil(), "NIM_MODEL_NAME should still be present")
				Expect(modelNameEnv.Value).To(Equal("hf://meta-llama/Llama-2-7b-chat-hf"))
			})

			It("should keep NGC_API_KEY when neither NIMCache nor NIMService is a Hugging Face model", func() {
				// Create a regular NGC NIMCache
				normalNimCache := &appsv1alpha1.NIMCache{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-normal-nimcache-kserve",
						Namespace: "default",
					},
					Spec: appsv1alpha1.NIMCacheSpec{
						Source: appsv1alpha1.NIMSource{
							NGC: &appsv1alpha1.NGCSource{
								ModelPuller: "test-container",
								PullSecret:  "my-secret",
							},
						},
						Storage: appsv1alpha1.NIMCacheStorage{
							PVC: appsv1alpha1.PersistentVolumeClaim{
								Create:       ptr.To[bool](true),
								StorageClass: "standard",
								Size:         "1Gi",
							},
						},
					},
					Status: appsv1alpha1.NIMCacheStatus{
						State: appsv1alpha1.NimCacheStatusReady,
						PVC:   "test-normal-nimcache-kserve-pvc",
						Profiles: []appsv1alpha1.NIMProfile{{
							Name:   "test-profile",
							Config: map[string]string{"tp": "2"}},
						},
					},
				}
				Expect(client.Create(context.TODO(), normalNimCache)).To(Succeed())

				// Create PVC for the NIMCache
				normalPVC := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-normal-nimcache-kserve-pvc",
						Namespace: "default",
					},
				}
				Expect(client.Create(context.TODO(), normalPVC)).To(Succeed())

				// Create a regular NIMService (no HF model)
				testNimService := &appsv1alpha1.NIMService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-normal-nimservice-kserve",
						Namespace: "default",
					},
					Spec: appsv1alpha1.NIMServiceSpec{
						Labels:      map[string]string{"app": "test-normal-app"},
						Annotations: map[string]string{"annotation-key": "annotation-value"},
						Image: appsv1alpha1.Image{
							Repository:  "nvcr.io/nvidia/nim-llm",
							Tag:         "v0.1.0",
							PullPolicy:  "IfNotPresent",
							PullSecrets: []string{"ngc-secret"},
						},
						AuthSecret: "ngc-secret",
						Storage: appsv1alpha1.NIMServiceStorage{
							NIMCache: appsv1alpha1.NIMCacheVolSpec{
								Name: "test-normal-nimcache-kserve",
							},
						},
						Env: []corev1.EnvVar{
							{
								Name:  "custom-env",
								Value: "custom-value",
							},
						},
						Resources: &corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("250m"),
								corev1.ResourceMemory: resource.MustParse("64Mi"),
							},
						},
						Expose: appsv1alpha1.Expose{
							Service: appsv1alpha1.Service{Type: corev1.ServiceTypeClusterIP, Port: ptr.To[int32](8000)},
						},
					},
				}

				namespacedName := types.NamespacedName{Name: testNimService.Name, Namespace: testNimService.Namespace}
				Expect(client.Create(context.TODO(), testNimService)).To(Succeed())

				result, err := reconciler.reconcileNIMService(context.TODO(), testNimService)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// InferenceService should be created
				isvc := &kservev1beta1.InferenceService{}
				err = client.Get(context.TODO(), namespacedName, isvc)
				Expect(err).NotTo(HaveOccurred())
				Expect(isvc.Name).To(Equal(testNimService.GetName()))
				Expect(isvc.Namespace).To(Equal(testNimService.GetNamespace()))

				// Verify environment variables
				container := isvc.Spec.Predictor.Containers[0]

				// NGC_API_KEY should be present
				var ngcKeyEnv *corev1.EnvVar
				for _, env := range container.Env {
					if env.Name == appsv1alpha1.NGCAPIKey {
						ngcKeyEnv = &env
						break
					}
				}
				Expect(ngcKeyEnv).NotTo(BeNil(), "NGC_API_KEY should be present for non-HF models")
				Expect(ngcKeyEnv.ValueFrom).NotTo(BeNil())
				Expect(ngcKeyEnv.ValueFrom.SecretKeyRef).NotTo(BeNil())
				Expect(ngcKeyEnv.ValueFrom.SecretKeyRef.Name).To(Equal("ngc-secret"))
				Expect(ngcKeyEnv.ValueFrom.SecretKeyRef.Key).To(Equal(appsv1alpha1.NGCAPIKey))

				// HF_TOKEN should NOT be present
				var hfTokenPresent bool
				for _, env := range container.Env {
					if env.Name == appsv1alpha1.HFToken {
						hfTokenPresent = true
						break
					}
				}
				Expect(hfTokenPresent).To(BeFalse(), "HF_TOKEN should not be present for non-HF models")
			})
		})
	})

	Describe("InitContainers and SidecarContainers rendering for InferenceService", func() {
		BeforeEach(func() {
			// Create InferenceService ConfigMap if it doesn't exist
			isvcConfig := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kserveconstants.InferenceServiceConfigMapName,
					Namespace: "default",
				},
				Data: map[string]string{
					"deploy": `{"defaultDeploymentMode": "Serverless"}`,
				},
			}
			err := client.Create(context.TODO(), isvcConfig)
			if err != nil && !errors.IsAlreadyExists(err) {
				Expect(err).To(Succeed())
			}
		})

		It("should render initContainers and sidecarContainers in InferenceService", func() {
			testNimService := &appsv1alpha1.NIMService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc-containers",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMServiceSpec{
					Image: appsv1alpha1.Image{
						Repository: "nvcr.io/nvidia/nim",
						Tag:        "1.0.0",
						PullPolicy: "IfNotPresent",
					},
					AuthSecret:        "ngc-secret",
					InferencePlatform: appsv1alpha1.PlatformTypeKServe,
					Storage: appsv1alpha1.NIMServiceStorage{
						EmptyDir: &appsv1alpha1.EmptyDirSpec{},
					},
					InitContainers: []*appsv1alpha1.NIMContainerSpec{
						{
							Name: "isvc-init-setup",
							Image: appsv1alpha1.Image{
								Repository: "busybox",
								Tag:        "1.35",
								PullPolicy: "Always",
							},
							Command: []string{"sh", "-c"},
							Args:    []string{"echo 'Initializing InferenceService...'"},
							Env: []corev1.EnvVar{
								{
									Name:  "ISVC_INIT_ENV",
									Value: "isvc-init-value",
								},
							},
							WorkingDir: "/workspace",
						},
						{
							Name: "isvc-model-loader",
							Image: appsv1alpha1.Image{
								Repository: "alpine",
								Tag:        "3.18",
							},
							Command: []string{"wget"},
							Args:    []string{"-O", "/models/config.json", "https://example.com/config.json"},
							Env: []corev1.EnvVar{
								{
									Name:  "MODEL_CONFIG_URL",
									Value: "https://example.com/config.json",
								},
							},
						},
					},
					SidecarContainers: []*appsv1alpha1.NIMContainerSpec{
						{
							Name: "isvc-logging-sidecar",
							Image: appsv1alpha1.Image{
								Repository: "fluent/fluent-bit",
								Tag:        "2.0",
								PullPolicy: "IfNotPresent",
							},
							Command: []string{"/fluent-bit/bin/fluent-bit"},
							Args:    []string{"-c", "/fluent-bit/etc/fluent-bit.conf"},
							Env: []corev1.EnvVar{
								{
									Name:  "FLUENT_ISVC_ENV",
									Value: "production",
								},
							},
						},
						{
							Name: "isvc-metrics-agent",
							Image: appsv1alpha1.Image{
								Repository: "prom/pushgateway",
								Tag:        "v1.5.0",
							},
							Env: []corev1.EnvVar{
								{
									Name:  "PUSH_GATEWAY_PORT",
									Value: "9091",
								},
							},
						},
					},
					Expose: appsv1alpha1.Expose{
						Service: appsv1alpha1.Service{
							Type: corev1.ServiceTypeClusterIP,
							Port: ptr.To[int32](8000),
						},
					},
				},
			}

			namespacedName := types.NamespacedName{Name: testNimService.Name, Namespace: testNimService.Namespace}
			Expect(client.Create(context.TODO(), testNimService)).To(Succeed())

			result, err := reconciler.reconcileNIMService(context.TODO(), testNimService)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Get the rendered InferenceService
			isvc := &kservev1beta1.InferenceService{}
			err = client.Get(context.TODO(), namespacedName, isvc)
			Expect(err).NotTo(HaveOccurred())

			// Verify initContainers are rendered correctly
			Expect(isvc.Spec.Predictor.InitContainers).To(HaveLen(2))

			initContainer1 := isvc.Spec.Predictor.InitContainers[0]
			Expect(initContainer1.Name).To(Equal("isvc-init-setup"))
			Expect(initContainer1.Image).To(Equal("busybox:1.35"))
			Expect(initContainer1.ImagePullPolicy).To(Equal(corev1.PullAlways))
			Expect(initContainer1.Command).To(Equal([]string{"sh", "-c"}))
			Expect(initContainer1.Args).To(Equal([]string{"echo 'Initializing InferenceService...'"}))
			Expect(initContainer1.WorkingDir).To(Equal("/workspace"))

			// Verify environment variables are merged
			var foundISVCInitEnv, foundGlobalEnv bool
			for _, env := range initContainer1.Env {
				if env.Name == "ISVC_INIT_ENV" && env.Value == "isvc-init-value" {
					foundISVCInitEnv = true
				}
				// Global NIM env vars should be present
				if env.Name == "NIM_CACHE_PATH" {
					foundGlobalEnv = true
				}
			}
			Expect(foundISVCInitEnv).To(BeTrue(), "Init-specific env var should be present")
			Expect(foundGlobalEnv).To(BeTrue(), "Global NIM env vars should be merged")

			initContainer2 := isvc.Spec.Predictor.InitContainers[1]
			Expect(initContainer2.Name).To(Equal("isvc-model-loader"))
			Expect(initContainer2.Image).To(Equal("alpine:3.18"))
			Expect(initContainer2.Command).To(Equal([]string{"wget"}))
			Expect(initContainer2.Args).To(Equal([]string{"-O", "/models/config.json", "https://example.com/config.json"}))

			var foundModelConfigURL bool
			for _, env := range initContainer2.Env {
				if env.Name == "MODEL_CONFIG_URL" {
					foundModelConfigURL = true
				}
			}
			Expect(foundModelConfigURL).To(BeTrue())

			// Verify sidecarContainers are rendered correctly
			// Main container + 2 sidecars = 3 total
			Expect(isvc.Spec.Predictor.Containers).To(HaveLen(3))

			// Find the sidecar containers
			var loggingSidecar, metricsAgent *corev1.Container
			for i := range isvc.Spec.Predictor.Containers {
				c := &isvc.Spec.Predictor.Containers[i]
				if c.Name == "isvc-logging-sidecar" {
					loggingSidecar = c
				} else if c.Name == "isvc-metrics-agent" {
					metricsAgent = c
				}
			}

			Expect(loggingSidecar).NotTo(BeNil(), "logging-sidecar should be present in InferenceService")
			Expect(loggingSidecar.Image).To(Equal("fluent/fluent-bit:2.0"))
			Expect(loggingSidecar.ImagePullPolicy).To(Equal(corev1.PullIfNotPresent))
			Expect(loggingSidecar.Command).To(Equal([]string{"/fluent-bit/bin/fluent-bit"}))
			Expect(loggingSidecar.Args).To(Equal([]string{"-c", "/fluent-bit/etc/fluent-bit.conf"}))

			var foundFluentEnv bool
			for _, env := range loggingSidecar.Env {
				if env.Name == "FLUENT_ISVC_ENV" && env.Value == "production" {
					foundFluentEnv = true
				}
			}
			Expect(foundFluentEnv).To(BeTrue(), "Sidecar-specific env var should be present")

			Expect(metricsAgent).NotTo(BeNil(), "metrics-agent should be present in InferenceService")
			Expect(metricsAgent.Image).To(Equal("prom/pushgateway:v1.5.0"))

			var foundPushGatewayPort bool
			for _, env := range metricsAgent.Env {
				if env.Name == "PUSH_GATEWAY_PORT" && env.Value == "9091" {
					foundPushGatewayPort = true
				}
			}
			Expect(foundPushGatewayPort).To(BeTrue(), "Metrics agent env var should be present")
		})

		It("should handle empty initContainers and sidecarContainers in InferenceService", func() {
			testNimService := &appsv1alpha1.NIMService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc-no-containers",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMServiceSpec{
					Image: appsv1alpha1.Image{
						Repository: "nvcr.io/nvidia/nim",
						Tag:        "1.0.0",
					},
					AuthSecret:        "ngc-secret",
					InferencePlatform: appsv1alpha1.PlatformTypeKServe,
					Storage: appsv1alpha1.NIMServiceStorage{
						EmptyDir: &appsv1alpha1.EmptyDirSpec{},
					},
					Expose: appsv1alpha1.Expose{
						Service: appsv1alpha1.Service{
							Type: corev1.ServiceTypeClusterIP,
							Port: ptr.To[int32](8000),
						},
					},
				},
			}

			namespacedName := types.NamespacedName{Name: testNimService.Name, Namespace: testNimService.Namespace}
			Expect(client.Create(context.TODO(), testNimService)).To(Succeed())

			result, err := reconciler.reconcileNIMService(context.TODO(), testNimService)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			isvc := &kservev1beta1.InferenceService{}
			err = client.Get(context.TODO(), namespacedName, isvc)
			Expect(err).NotTo(HaveOccurred())

			// Should only have system-generated init containers (if any)
			// No user-defined initContainers
			for _, ic := range isvc.Spec.Predictor.InitContainers {
				// All init containers should be system-generated
				Expect(ic.Name).NotTo(ContainSubstring("isvc-init-"))
			}

			// Should only have 1 container (the main NIM container)
			Expect(isvc.Spec.Predictor.Containers).To(HaveLen(1))
			Expect(isvc.Spec.Predictor.Containers[0].Name).To(Equal(testNimService.GetContainerName()))
		})

		It("should use custom pull policy for initContainers and sidecarContainers in InferenceService", func() {
			testNimService := &appsv1alpha1.NIMService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-isvc-pullpolicy",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMServiceSpec{
					Image: appsv1alpha1.Image{
						Repository: "nvcr.io/nvidia/nim",
						Tag:        "1.0.0",
						PullPolicy: "IfNotPresent",
					},
					AuthSecret:        "ngc-secret",
					InferencePlatform: appsv1alpha1.PlatformTypeKServe,
					Storage: appsv1alpha1.NIMServiceStorage{
						EmptyDir: &appsv1alpha1.EmptyDirSpec{},
					},
					InitContainers: []*appsv1alpha1.NIMContainerSpec{
						{
							Name: "isvc-init-always",
							Image: appsv1alpha1.Image{
								Repository: "busybox",
								Tag:        "latest",
								PullPolicy: "Always",
							},
						},
						{
							Name: "isvc-init-default",
							Image: appsv1alpha1.Image{
								Repository: "alpine",
								Tag:        "latest",
								// No PullPolicy specified, should inherit from parent
							},
						},
					},
					SidecarContainers: []*appsv1alpha1.NIMContainerSpec{
						{
							Name: "isvc-sidecar-never",
							Image: appsv1alpha1.Image{
								Repository: "nginx",
								Tag:        "latest",
								PullPolicy: "Never",
							},
						},
					},
					Expose: appsv1alpha1.Expose{
						Service: appsv1alpha1.Service{
							Type: corev1.ServiceTypeClusterIP,
							Port: ptr.To[int32](8000),
						},
					},
				},
			}

			namespacedName := types.NamespacedName{Name: testNimService.Name, Namespace: testNimService.Namespace}
			Expect(client.Create(context.TODO(), testNimService)).To(Succeed())

			result, err := reconciler.reconcileNIMService(context.TODO(), testNimService)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			isvc := &kservev1beta1.InferenceService{}
			err = client.Get(context.TODO(), namespacedName, isvc)
			Expect(err).NotTo(HaveOccurred())

			// Verify initContainer pull policies
			Expect(isvc.Spec.Predictor.InitContainers).To(HaveLen(2))
			Expect(isvc.Spec.Predictor.InitContainers[0].ImagePullPolicy).To(Equal(corev1.PullAlways))
			Expect(isvc.Spec.Predictor.InitContainers[1].ImagePullPolicy).To(Equal(corev1.PullIfNotPresent))

			// Verify sidecar pull policy
			var sidecarFound bool
			for i := range isvc.Spec.Predictor.Containers {
				if isvc.Spec.Predictor.Containers[i].Name == "isvc-sidecar-never" {
					Expect(isvc.Spec.Predictor.Containers[i].ImagePullPolicy).To(Equal(corev1.PullNever))
					sidecarFound = true
					break
				}
			}
			Expect(sidecarFound).To(BeTrue(), "Sidecar should be present in InferenceService")
		})
	})
})
