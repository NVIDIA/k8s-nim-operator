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

package controller

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/nimparser"
)

var _ = Describe("NIMCache Controller", func() {
	var (
		client     client.Client
		reconciler *NIMCacheReconciler
		scheme     *runtime.Scheme
	)
	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(appsv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(batchv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		client = fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&appsv1alpha1.NIMCache{}).
			WithStatusSubresource(&batchv1.Job{}).
			WithStatusSubresource(&corev1.ConfigMap{}).
			Build()
		reconciler = &NIMCacheReconciler{
			Client: client,
			scheme: scheme,
		}
	})

	AfterEach(func() {
		// Clean up the NIMCache instance
		nimCache := &appsv1alpha1.NIMCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-nimcache",
				Namespace: "default",
			},
		}
		_ = client.Delete(context.TODO(), nimCache)
	})

	Context("When creating a NIMCache", func() {
		It("should create a Job and PVC", func() {
			ctx := context.TODO()
			NIMCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimcache",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source:  appsv1alpha1.NIMSource{NGC: &appsv1alpha1.NGCSource{ModelPuller: "test-container", PullSecret: "my-secret"}},
					Storage: appsv1alpha1.Storage{PVC: appsv1alpha1.PersistentVolumeClaim{Create: ptr.To[bool](true), StorageClass: "standard", Size: "1Gi"}},
				},
				Status: appsv1alpha1.NIMCacheStatus{
					State: appsv1alpha1.NimCacheStatusNotReady,
				},
			}
			Expect(client.Create(ctx, NIMCache)).To(Succeed())

			// Reconcile the resource
			_, err := reconciler.reconcileNIMCache(ctx, NIMCache)
			Expect(err).ToNot(HaveOccurred())

			// Check if the Job was created
			// Wait for reconciliation to complete with a timeout
			Eventually(func() error {
				job := &batchv1.Job{}
				jobName := types.NamespacedName{Name: "test-nimcache-job", Namespace: "default"}
				return client.Get(ctx, jobName, job)
			}, time.Second*10).Should(Succeed())

			// Check if the PVC was created
			// Wait for reconciliation to complete with a timeout
			Eventually(func() error {
				pvc := &corev1.PersistentVolumeClaim{}
				pvcName := types.NamespacedName{Name: "test-nimcache-pvc", Namespace: "default"}
				return client.Get(ctx, pvcName, pvc)
			}, time.Second*10).Should(Succeed())
		})
	})

	Context("When the Job completes", func() {
		It("should update the NIMCache status", func() {
			ctx := context.TODO()
			NIMCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimcache",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source:  appsv1alpha1.NIMSource{NGC: &appsv1alpha1.NGCSource{ModelPuller: "test-container", PullSecret: "my-secret"}},
					Storage: appsv1alpha1.Storage{PVC: appsv1alpha1.PersistentVolumeClaim{Create: ptr.To[bool](true), StorageClass: "standard", Size: "1Gi"}},
				},
				Status: appsv1alpha1.NIMCacheStatus{
					State: "Initializing",
				},
			}
			Expect(client.Create(ctx, NIMCache)).To(Succeed())

			// Reconcile the resource
			_, err := reconciler.reconcileNIMCache(ctx, NIMCache)
			Expect(err).ToNot(HaveOccurred())

			// Check if the Job was created
			// Wait for reconciliation to complete with a timeout
			Eventually(func() error {
				job := &batchv1.Job{}
				jobName := types.NamespacedName{Name: "test-nimcache-job", Namespace: "default"}
				return client.Get(ctx, jobName, job)
			}, time.Second*10).Should(Succeed())

			// Set the Job as completed
			job := &batchv1.Job{}
			jobName := types.NamespacedName{Name: "test-nimcache-job", Namespace: "default"}
			Expect(client.Get(ctx, jobName, job)).To(Succeed())
			job.Status.Succeeded = 1
			Expect(client.Status().Update(ctx, job)).To(Succeed())

			// Reconcile the resource again
			_, err = reconciler.reconcileNIMCache(ctx, NIMCache)
			Expect(err).ToNot(HaveOccurred())

			// Check if the NIMCache status was updated
			Expect(client.Get(ctx, types.NamespacedName{Name: "test-nimcache", Namespace: "default"}, NIMCache)).To(Succeed())
			Expect(NIMCache.Status.State).To(Equal(appsv1alpha1.NimCacheStatusReady))
		})
	})

	Context("When deleting a NIMCache", func() {
		It("should clean up resources", func() {
			ctx := context.TODO()
			NIMCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-nimcache",
					Namespace:         "default",
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
					Finalizers:        []string{NIMCacheFinalizer},
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source:  appsv1alpha1.NIMSource{NGC: &appsv1alpha1.NGCSource{ModelPuller: "test-container", PullSecret: "my-secret"}},
					Storage: appsv1alpha1.Storage{PVC: appsv1alpha1.PersistentVolumeClaim{Create: ptr.To[bool](true), StorageClass: "standard", Size: "1Gi"}},
				},
				Status: appsv1alpha1.NIMCacheStatus{
					State: "Initializing",
				},
			}
			Expect(client.Create(ctx, NIMCache)).To(Succeed())

			// Reconcile the resource
			_, err := reconciler.reconcileNIMCache(ctx, NIMCache)
			Expect(err).ToNot(HaveOccurred())

			// Check if the Job was created
			// Wait for reconciliation to complete with a timeout
			Eventually(func() error {
				job := &batchv1.Job{}
				jobName := types.NamespacedName{Name: "test-nimcache-job", Namespace: "default"}
				return client.Get(ctx, jobName, job)
			}, time.Second*10).Should(Succeed())

			// Check if the PVC was created
			// Wait for reconciliation to complete with a timeout
			Eventually(func() error {
				pvc := &corev1.PersistentVolumeClaim{}
				pvcName := types.NamespacedName{Name: "test-nimcache-pvc", Namespace: "default"}
				return client.Get(ctx, pvcName, pvc)
			}, time.Second*10).Should(Succeed())

			// Delete the NIMCache instance
			Expect(client.Delete(ctx, NIMCache)).To(Succeed())

			// Reconcile the resource again
			err = reconciler.cleanupNIMCache(ctx, NIMCache)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("when creating a NIMCache resource", func() {
		It("should construct a pod with right specifications", func() {
			nimCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimcache",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source: appsv1alpha1.NIMSource{NGC: &appsv1alpha1.NGCSource{ModelPuller: "nvcr.io/nim:test", PullSecret: "my-secret"}},
				},
			}
			pod := constructPodSpec(nimCache)
			Expect(pod.Name).To(Equal(getPodName(nimCache)))
			Expect(pod.Spec.Containers[0].Image).To(Equal("nvcr.io/nim:test"))
			Expect(pod.Spec.ImagePullSecrets[0].Name).To(Equal("my-secret"))
			Expect(*pod.Spec.SecurityContext.RunAsUser).To(Equal(int64(1000)))
			Expect(*pod.Spec.SecurityContext.FSGroup).To(Equal(int64(2000)))
			Expect(*pod.Spec.SecurityContext.RunAsNonRoot).To(Equal(true))
		})

		It("should create a pod with the correct specifications", func() {
			ctx := context.TODO()
			nimCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimcache",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source: appsv1alpha1.NIMSource{NGC: &appsv1alpha1.NGCSource{ModelPuller: "nvcr.io/nim:test", PullSecret: "my-secret"}},
				},
			}

			pod := constructPodSpec(nimCache)

			err := client.Create(context.TODO(), pod)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() error {
				pod := &corev1.Pod{}
				podName := types.NamespacedName{Name: getPodName(nimCache), Namespace: "default"}
				return client.Get(ctx, podName, pod)
			}, time.Second*10).Should(Succeed())
		})

		It("should construct a job with right specifications", func() {
			profiles := []string{"36fc1fa4fc35c1d54da115a39323080b08d7937dceb8ba47be44f4da0ec720ff"}
			profilesJSON, err := json.Marshal(profiles)
			Expect(err).ToNot(HaveOccurred())

			nimCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-nimcache",
					Namespace:   "default",
					Annotations: map[string]string{SelectedNIMProfilesAnnotationKey: string(profilesJSON)},
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source: appsv1alpha1.NIMSource{NGC: &appsv1alpha1.NGCSource{ModelPuller: "nvcr.io/nim:test", PullSecret: "my-secret", Model: appsv1alpha1.ModelSpec{AutoDetect: ptr.To[bool](true)}}},
				},
			}

			job, err := constructJob(nimCache)
			Expect(err).ToNot(HaveOccurred())

			Expect(job.Name).To(Equal(getJobName(nimCache)))
			Expect(job.Spec.Template.Spec.Containers[0].Image).To(Equal("nvcr.io/nim:test"))
			Expect(job.Spec.Template.Spec.ImagePullSecrets[0].Name).To(Equal("my-secret"))
			Expect(job.Spec.Template.Spec.Containers[0].Command).To(ContainElements("download-to-cache"))
			Expect(job.Spec.Template.Spec.Containers[0].Args).To(ContainElements("--profiles", "36fc1fa4fc35c1d54da115a39323080b08d7937dceb8ba47be44f4da0ec720ff"))
			Expect(*job.Spec.Template.Spec.SecurityContext.RunAsUser).To(Equal(int64(1000)))
			Expect(*job.Spec.Template.Spec.SecurityContext.FSGroup).To(Equal(int64(2000)))
			Expect(*job.Spec.Template.Spec.SecurityContext.RunAsNonRoot).To(Equal(true))
			Expect(job.Spec.Template.Spec.Volumes[0].Name).To(Equal("nim-cache-volume"))
			Expect(job.Spec.Template.Spec.Volumes[0].VolumeSource.PersistentVolumeClaim.ClaimName).To(Equal(getPvcName(nimCache, nimCache.Spec.Storage.PVC)))
		})

		It("should create a job with the correct specifications", func() {
			ctx := context.TODO()
			nimCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimcache",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source: appsv1alpha1.NIMSource{NGC: &appsv1alpha1.NGCSource{ModelPuller: "nvcr.io/nim:test", PullSecret: "my-secret"}},
				},
			}

			job, err := constructJob(nimCache)
			Expect(err).ToNot(HaveOccurred())

			err = client.Create(context.TODO(), job)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() error {
				job := &batchv1.Job{}
				jobName := types.NamespacedName{Name: getJobName(nimCache), Namespace: "default"}
				return client.Get(ctx, jobName, job)
			}, time.Second*10).Should(Succeed())
		})

		It("should create a ConfigMap with the given model manifest data", func() {
			ctx := context.TODO()
			nimCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimcache",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source: appsv1alpha1.NIMSource{NGC: &appsv1alpha1.NGCSource{ModelPuller: "nvcr.io/nim:test", PullSecret: "my-secret"}},
				},
			}

			filePath := filepath.Join("testdata", "manifest_trtllm.yaml")
			manifestData, err := nimparser.ParseModelManifest(filePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(*manifestData).To(HaveLen(1))

			err = reconciler.createManifestConfigMap(ctx, nimCache, manifestData)
			Expect(err).NotTo(HaveOccurred())

			// Verify that the ConfigMap was created
			createdConfigMap := &corev1.ConfigMap{}
			err = client.Get(ctx, types.NamespacedName{Name: getManifestConfigName(nimCache), Namespace: "default"}, createdConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(createdConfigMap.Data).To(HaveKey("model_manifest.yaml"))

			// Verify the content of model_manifest.yaml
			extractedManifest, err := reconciler.extractNIMManifest(ctx, createdConfigMap.Name, createdConfigMap.Namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(extractedManifest).NotTo(BeNil())
			Expect(*extractedManifest).To(HaveLen(1))
			profile, exists := (*extractedManifest)["03fdb4d11f01be10c31b00e7c0540e2835e89a0079b483ad2dd3c25c8cc29b61"]
			Expect(exists).To(BeTrue())
			Expect(profile.Model).To(Equal("meta/llama3-70b-instruct"))
			Expect(profile.Tags["llm_engine"]).To(Equal("tensorrt_llm"))
			Expect(profile.Tags["precision"]).To(Equal("fp16"))
			Expect(profile.ContainerURL).To(Equal("nvcr.io/nim/meta/llama3-70b-instruct:1.0.0"))
		})

		It("should return an error if model_manifest.yaml is not found in ConfigMap", func() {
			ctx := context.TODO()

			// Create a ConfigMap without model_manifest.yaml
			emptyConfig := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "default",
				},
				Data: map[string]string{},
			}
			Expect(reconciler.Create(ctx, emptyConfig)).To(Succeed())

			_, err := reconciler.extractNIMManifest(ctx, emptyConfig.Name, emptyConfig.Namespace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("model_manifest.yaml not found in ConfigMap"))
		})
	})
})
