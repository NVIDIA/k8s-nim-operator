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
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
	nimparserv1 "github.com/NVIDIA/k8s-nim-operator/internal/nimparser/v1"
)

var _ = Describe("NIMCache Controller", func() {
	var (
		cli        client.Client
		reconciler *NIMCacheReconciler
		scheme     *runtime.Scheme
	)
	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(appsv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(batchv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(rbacv1.AddToScheme(scheme)).To(Succeed())

		cli = fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&appsv1alpha1.NIMCache{}).
			WithStatusSubresource(&batchv1.Job{}).
			WithStatusSubresource(&corev1.ConfigMap{}).
			Build()
		reconciler = &NIMCacheReconciler{
			Client:   cli,
			scheme:   scheme,
			recorder: record.NewFakeRecorder(1000),
		}

		nimCache := &appsv1alpha1.NIMCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-nimcache",
				Namespace: "default",
			},
			Spec: appsv1alpha1.NIMCacheSpec{
				Source: appsv1alpha1.NIMSource{NGC: &appsv1alpha1.NGCSource{ModelPuller: "nvcr.io/nim:test", PullSecret: "my-secret"}},
			},
		}

		// Create a model manifest configmap, as we cannot run a sample NIM container to extract for tests
		filePath := filepath.Join("testdata", "manifest_trtllm.yaml")
		nimparser := nimparserv1.NIMParser{}
		manifestData, err := nimparser.ParseModelManifest(filePath)
		Expect(err).NotTo(HaveOccurred())

		err = reconciler.createManifestConfigMap(context.TODO(), nimCache, &manifestData)
		Expect(err).NotTo(HaveOccurred())

		// Verify that the ConfigMap was created
		createdConfigMap := &corev1.ConfigMap{}
		err = cli.Get(context.TODO(), types.NamespacedName{Name: getManifestConfigName(nimCache), Namespace: "default"}, createdConfigMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(createdConfigMap.Data).To(HaveKey("model_manifest.yaml"))
	})

	AfterEach(func() {
		// Clean up the NIMCache instance
		nimCache := &appsv1alpha1.NIMCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-nimcache",
				Namespace: "default",
			},
		}
		_ = cli.Delete(context.TODO(), nimCache)
	})

	Context("When creating a NIMCache", func() {
		It("should create a Job and PVC", func() {
			ctx := context.TODO()
			runtimeClassName := "test-class"
			NIMCache := &appsv1alpha1.NIMCache{
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
							Create:           ptr.To[bool](true),
							StorageClass:     "standard",
							Size:             "1Gi",
							VolumeAccessMode: corev1.ReadWriteOnce,
							Annotations: map[string]string{
								"my-custom-annotation": "import-volume",
							},
						},
					},
					RuntimeClassName: runtimeClassName,
				},
				Status: appsv1alpha1.NIMCacheStatus{
					State: appsv1alpha1.NimCacheStatusNotReady,
				},
			}
			Expect(cli.Create(ctx, NIMCache)).To(Succeed())

			// Reconcile the resource
			_, err := reconciler.reconcileNIMCache(ctx, NIMCache)
			Expect(err).ToNot(HaveOccurred())

			// Check if the Job was created
			// Wait for reconciliation to complete with a timeout
			job := &batchv1.Job{}
			Eventually(func() error {
				jobName := types.NamespacedName{Name: "test-nimcache-job", Namespace: "default"}
				return cli.Get(ctx, jobName, job)
			}, time.Second*10).Should(Succeed())
			Expect(job.Spec.Template.Spec.Containers[0].Image).To(Equal("test-container"))
			Expect(job.Spec.Template.Spec.RuntimeClassName).To(Equal(&runtimeClassName))
			Expect(job.Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"feature.node.kubernetes.io/pci-10de.present": "true"}))

			// Check if the PVC was created and has the correct annotations
			pvc := &corev1.PersistentVolumeClaim{}
			pvcName := types.NamespacedName{Name: "test-nimcache-pvc", Namespace: "default"}

			// Wait for reconciliation to complete and PVC to be created
			Eventually(func() error {
				return cli.Get(ctx, pvcName, pvc)
			}, time.Second*10).Should(Succeed())

			// Check for specific annotation
			Expect(pvc.Annotations).To(HaveKeyWithValue("my-custom-annotation", "import-volume"))
		})

		It("should return an error if the PVC size is not specified", func() {
			ctx := context.TODO()
			NIMCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimcache",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source:  appsv1alpha1.NIMSource{NGC: &appsv1alpha1.NGCSource{ModelPuller: "test-container", PullSecret: "my-secret"}},
					Storage: appsv1alpha1.NIMCacheStorage{PVC: appsv1alpha1.PersistentVolumeClaim{Create: ptr.To[bool](true), StorageClass: "standard"}},
				},
				Status: appsv1alpha1.NIMCacheStatus{
					State: appsv1alpha1.NimCacheStatusNotReady,
				},
			}
			Expect(cli.Create(ctx, NIMCache)).To(Succeed())

			// Reconcile the resource
			_, err := reconciler.reconcileNIMCache(ctx, NIMCache)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("PVC size must be specified"))
		})

		It("should update the PVC size and annotations when NIMCache is updated", func() {
			ctx := context.TODO()
			nimcacheName := types.NamespacedName{Name: "test-nimcache", Namespace: "default"}

			initialNIMCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nimcacheName.Name,
					Namespace: nimcacheName.Namespace,
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
							Create:           ptr.To[bool](true),
							StorageClass:     "standard",
							Size:             "1Gi",
							VolumeAccessMode: corev1.ReadWriteOnce,
							Annotations: map[string]string{
								"initial-annotation": "true",
							},
						},
					},
					RuntimeClassName: "test-class",
				},
				Status: appsv1alpha1.NIMCacheStatus{
					State: appsv1alpha1.NimCacheStatusNotReady,
				},
			}
			Expect(cli.Create(ctx, initialNIMCache)).To(Succeed())

			// Initial reconcile to create PVC
			_, err := reconciler.reconcileNIMCache(ctx, initialNIMCache)
			Expect(err).ToNot(HaveOccurred())

			pvc := &corev1.PersistentVolumeClaim{}
			Eventually(func() error {
				return cli.Get(ctx, types.NamespacedName{Name: "test-nimcache-pvc", Namespace: "default"}, pvc)
			}, time.Second*10).Should(Succeed())
			Expect(pvc.Spec.Resources.Requests[corev1.ResourceStorage]).To(Equal(resource.MustParse("1Gi")))
			Expect(pvc.Annotations).To(HaveKeyWithValue("initial-annotation", "true"))

			// Update NIMCache with new size and annotations
			updated := &appsv1alpha1.NIMCache{}
			Expect(cli.Get(ctx, nimcacheName, updated)).To(Succeed())
			updated.Spec.Storage.PVC.Size = "2Gi"
			updated.Spec.Storage.PVC.Annotations = map[string]string{
				"updated-annotation": "yes",
			}
			Expect(cli.Update(ctx, updated)).To(Succeed())

			// Reconcile again
			_, err = reconciler.reconcileNIMCache(ctx, updated)
			Expect(err).ToNot(HaveOccurred())

			// Verify PVC reflects the updated size and annotations
			Eventually(func() corev1.ResourceList {
				_ = cli.Get(ctx, types.NamespacedName{Name: "test-nimcache-pvc", Namespace: "default"}, pvc)
				return pvc.Spec.Resources.Requests
			}, time.Second*10).Should(HaveKeyWithValue(corev1.ResourceStorage, resource.MustParse("2Gi")))

			Expect(pvc.Annotations).To(HaveKeyWithValue("updated-annotation", "yes"))
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
					Storage: appsv1alpha1.NIMCacheStorage{PVC: appsv1alpha1.PersistentVolumeClaim{Create: ptr.To[bool](true), StorageClass: "standard", Size: "1Gi", VolumeAccessMode: corev1.ReadWriteOnce}},
				},
				Status: appsv1alpha1.NIMCacheStatus{
					State: "Initializing",
				},
			}
			Expect(cli.Create(ctx, NIMCache)).To(Succeed())

			// Reconcile the resource
			_, err := reconciler.reconcileNIMCache(ctx, NIMCache)
			Expect(err).ToNot(HaveOccurred())

			// Check if the Job was created
			// Wait for reconciliation to complete with a timeout
			Eventually(func() error {
				job := &batchv1.Job{}
				jobName := types.NamespacedName{Name: "test-nimcache-job", Namespace: "default"}
				return cli.Get(ctx, jobName, job)
			}, time.Second*10).Should(Succeed())

			// Set the Job as completed
			job := &batchv1.Job{}
			jobName := types.NamespacedName{Name: "test-nimcache-job", Namespace: "default"}
			Expect(cli.Get(ctx, jobName, job)).To(Succeed())
			job.Status.Succeeded = 1
			Expect(cli.Status().Update(ctx, job)).To(Succeed())

			// Reconcile the resource again
			_, err = reconciler.reconcileNIMCache(ctx, NIMCache)
			Expect(err).ToNot(HaveOccurred())

			// Check if the NIMCache status was updated
			Expect(cli.Get(ctx, types.NamespacedName{Name: "test-nimcache", Namespace: "default"}, NIMCache)).To(Succeed())
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
					Storage: appsv1alpha1.NIMCacheStorage{PVC: appsv1alpha1.PersistentVolumeClaim{Create: ptr.To[bool](true), StorageClass: "standard", Size: "1Gi", VolumeAccessMode: corev1.ReadWriteOnce}},
				},
				Status: appsv1alpha1.NIMCacheStatus{
					State: "Initializing",
				},
			}
			Expect(cli.Create(ctx, NIMCache)).To(Succeed())

			// Reconcile the resource
			_, err := reconciler.reconcileNIMCache(ctx, NIMCache)
			Expect(err).ToNot(HaveOccurred())

			// Check if the Job was created
			// Wait for reconciliation to complete with a timeout
			Eventually(func() error {
				job := &batchv1.Job{}
				jobName := types.NamespacedName{Name: "test-nimcache-job", Namespace: "default"}
				return cli.Get(ctx, jobName, job)
			}, time.Second*10).Should(Succeed())

			// Check if the PVC was created
			// Wait for reconciliation to complete with a timeout
			Eventually(func() error {
				pvc := &corev1.PersistentVolumeClaim{}
				pvcName := types.NamespacedName{Name: "test-nimcache-pvc", Namespace: "default"}
				return cli.Get(ctx, pvcName, pvc)
			}, time.Second*10).Should(Succeed())

			// Delete the NIMCache instance
			Expect(cli.Delete(ctx, NIMCache)).To(Succeed())

			// Reconcile the resource again
			err = reconciler.cleanupNIMCache(ctx, NIMCache)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("when creating a NIMCache resource", func() {
		It("should create a Role with SCC rules", func() {
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

			err := reconciler.reconcileRole(ctx, nimCache)
			Expect(err).NotTo(HaveOccurred())

			// Check if the Role was created
			role := &rbacv1.Role{}
			roleName := types.NamespacedName{Name: NIMCacheRole, Namespace: "default"}

			err = cli.Get(ctx, roleName, role)
			Expect(err).NotTo(HaveOccurred())
			Expect(role.Rules).To(HaveLen(1))

			// Check the Role has the expected SCC rules
			expectedRule := rbacv1.PolicyRule{
				APIGroups:     []string{"security.openshift.io"},
				Resources:     []string{"securitycontextconstraints"},
				ResourceNames: []string{"nonroot"},
				Verbs:         []string{"use"},
			}
			Expect(role.Rules[0]).To(Equal(expectedRule))
		})

		It("should create a RoleBinding for the Role", func() {
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

			err := reconciler.reconcileRole(ctx, nimCache)
			Expect(err).NotTo(HaveOccurred())

			err = reconciler.reconcileRoleBinding(ctx, nimCache)
			Expect(err).NotTo(HaveOccurred())

			// Check if the RoleBinding was created
			rb := &rbacv1.RoleBinding{}
			rbName := types.NamespacedName{Name: NIMCacheRoleBinding, Namespace: "default"}
			err = cli.Get(ctx, rbName, rb)
			Expect(err).NotTo(HaveOccurred())

			// Check that the RoleBinding is bound to the correct Role
			Expect(rb.RoleRef.Name).To(Equal(NIMCacheRole))
			Expect(rb.Subjects).To(HaveLen(1))
			Expect(rb.Subjects[0].Kind).To(Equal("ServiceAccount"))
			Expect(rb.Subjects[0].Name).To(Equal(NIMCacheServiceAccount))
			Expect(rb.Subjects[0].Namespace).To(Equal(nimCache.GetNamespace()))
		})

		It("should construct a pod with right specifications", func() {
			nimCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimcache",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source: appsv1alpha1.NIMSource{NGC: &appsv1alpha1.NGCSource{ModelPuller: "nvcr.io/nim:test", PullSecret: "my-secret"}},
					Resources: appsv1alpha1.Resources{
						CPU:    resource.MustParse("250m"),
						Memory: resource.MustParse("256Mi"),
					},
				},
			}

			pod := constructPodSpec(nimCache, k8sutil.K8s)
			Expect(pod.Name).To(Equal(getPodName(nimCache)))
			Expect(pod.Spec.Containers[0].Image).To(Equal("nvcr.io/nim:test"))
			Expect(pod.Spec.ImagePullSecrets[0].Name).To(Equal("my-secret"))
			Expect(*pod.Spec.SecurityContext.RunAsUser).To(Equal(int64(1000)))
			Expect(*pod.Spec.SecurityContext.FSGroup).To(Equal(int64(2000)))
			Expect(*pod.Spec.SecurityContext.RunAsNonRoot).To(Equal(true))
			Expect(pod.Spec.NodeSelector["feature.node.kubernetes.io/pci-10de.present"]).To(Equal("true"))
			// Add resource checks
			resources := pod.Spec.Containers[0].Resources
			Expect(resources.Requests.Cpu().String()).To(Equal("250m"))
			Expect(resources.Requests.Memory().String()).To(Equal("256Mi"))
			Expect(resources.Limits.Cpu().String()).To(Equal("250m"))
			Expect(resources.Limits.Memory().String()).To(Equal("256Mi"))
		})

		It("should construct a pod with runtime class and node selector", func() {
			runtimeClassName := "test-class"
			nimCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimcache",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source:           appsv1alpha1.NIMSource{NGC: &appsv1alpha1.NGCSource{ModelPuller: "nvcr.io/nim:test", PullSecret: "my-secret"}},
					RuntimeClassName: runtimeClassName,
					NodeSelector: map[string]string{
						"test-label": "true",
					},
				},
			}
			pod := constructPodSpec(nimCache, k8sutil.K8s)
			Expect(pod.Name).To(Equal(getPodName(nimCache)))
			Expect(pod.Spec.Containers[0].Image).To(Equal("nvcr.io/nim:test"))
			Expect(pod.Spec.ImagePullSecrets[0].Name).To(Equal("my-secret"))
			Expect(*pod.Spec.SecurityContext.RunAsUser).To(Equal(int64(1000)))
			Expect(*pod.Spec.SecurityContext.FSGroup).To(Equal(int64(2000)))
			Expect(*pod.Spec.SecurityContext.RunAsNonRoot).To(Equal(true))
			Expect(pod.Spec.NodeSelector["test-label"]).To(Equal("true"))
			Expect(pod.Spec.RuntimeClassName).To(Equal(&runtimeClassName))
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

			pod := constructPodSpec(nimCache, k8sutil.K8s)

			err := cli.Create(context.TODO(), pod)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() error {
				pod := &corev1.Pod{}
				podName := types.NamespacedName{Name: getPodName(nimCache), Namespace: "default"}
				return cli.Get(ctx, podName, pod)
			}, time.Second*10).Should(Succeed())
		})

		It("should construct a job with a specific profile", func() {
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
					Source: appsv1alpha1.NIMSource{NGC: &appsv1alpha1.NGCSource{ModelPuller: "nvcr.io/nim:test", PullSecret: "my-secret"}},
				},
			}

			job, err := reconciler.constructJob(context.TODO(), nimCache, k8sutil.K8s)
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
			Expect(job.Spec.Template.Spec.Volumes[0].VolumeSource.PersistentVolumeClaim.ClaimName).To(Equal(nimCache.GetPVCName()))
		})

		It("should construct a job with multiple profiles", func() {
			profiles := []string{"36fc1fa4fc35c1d54da115a39323080b08d7937dceb8ba47be44f4da0ec720ff", "04fdb4d11f01be10c31b00e7c0540e2835e89a0079b483ad2dd3c25c8cc12345"}
			profilesJSON, err := json.Marshal(profiles)
			Expect(err).ToNot(HaveOccurred())

			nimCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-nimcache",
					Namespace:   "default",
					Annotations: map[string]string{SelectedNIMProfilesAnnotationKey: string(profilesJSON)},
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source: appsv1alpha1.NIMSource{NGC: &appsv1alpha1.NGCSource{ModelPuller: "nvcr.io/nim:test", PullSecret: "my-secret"}},
				},
			}

			job, err := reconciler.constructJob(context.TODO(), nimCache, k8sutil.K8s)
			Expect(err).ToNot(HaveOccurred())

			Expect(job.Name).To(Equal(getJobName(nimCache)))
			Expect(job.Spec.Template.Spec.Containers[0].Image).To(Equal("nvcr.io/nim:test"))
			Expect(job.Spec.Template.Spec.ImagePullSecrets[0].Name).To(Equal("my-secret"))
			Expect(job.Spec.Template.Spec.Containers[0].Command).To(ContainElements("download-to-cache"))
			Expect(job.Spec.Template.Spec.Containers[0].Args).To(ContainElements("--profiles", "36fc1fa4fc35c1d54da115a39323080b08d7937dceb8ba47be44f4da0ec720ff", "04fdb4d11f01be10c31b00e7c0540e2835e89a0079b483ad2dd3c25c8cc12345"))
			Expect(*job.Spec.Template.Spec.SecurityContext.RunAsUser).To(Equal(int64(1000)))
			Expect(*job.Spec.Template.Spec.SecurityContext.FSGroup).To(Equal(int64(2000)))
			Expect(*job.Spec.Template.Spec.SecurityContext.RunAsNonRoot).To(Equal(true))
			Expect(job.Spec.Template.Spec.Volumes[0].Name).To(Equal("nim-cache-volume"))
			Expect(job.Spec.Template.Spec.Volumes[0].VolumeSource.PersistentVolumeClaim.ClaimName).To(Equal(nimCache.GetPVCName()))
		})

		It("should construct a job set to download all profiles", func() {
			profiles := []string{AllProfiles}
			nimCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimcache",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source: appsv1alpha1.NIMSource{NGC: &appsv1alpha1.NGCSource{ModelPuller: "nvcr.io/nim:test", PullSecret: "my-secret", Model: appsv1alpha1.ModelSpec{Profiles: profiles}}},
				},
			}

			job, err := reconciler.constructJob(context.TODO(), nimCache, k8sutil.K8s)
			Expect(err).ToNot(HaveOccurred())

			Expect(job.Name).To(Equal(getJobName(nimCache)))
			Expect(job.Spec.Template.Spec.Containers[0].Image).To(Equal("nvcr.io/nim:test"))
			Expect(job.Spec.Template.Spec.ImagePullSecrets[0].Name).To(Equal("my-secret"))
			Expect(job.Spec.Template.Spec.Containers[0].Command).To(ContainElements("download-to-cache"))
			Expect(job.Spec.Template.Spec.Containers[0].Args).To(ContainElements("--all"))
		})

		It("should create a job with the correct specifications", func() {
			profiles := []string{"36fc1fa4fc35c1d54da115a39323080b08d7937dceb8ba47be44f4da0ec720ff"}
			profilesJSON, err := json.Marshal(profiles)
			Expect(err).ToNot(HaveOccurred())
			ctx := context.TODO()
			nimCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-nimcache",
					Namespace:   "default",
					Annotations: map[string]string{SelectedNIMProfilesAnnotationKey: string(profilesJSON)},
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source: appsv1alpha1.NIMSource{NGC: &appsv1alpha1.NGCSource{ModelPuller: "nvcr.io/nim:test", PullSecret: "my-secret", Model: appsv1alpha1.ModelSpec{GPUs: []appsv1alpha1.GPUSpec{{IDs: []string{"26b5"}}}}}},
					Env: []corev1.EnvVar{
						{
							Name:  "NGC_HOME",
							Value: "/opt/ngc/hub",
						},
						{
							Name:  "HTTPS_PROXY",
							Value: "https://my-custom-proxy-server:port",
						},
					},
				},
			}

			job, err := reconciler.constructJob(context.TODO(), nimCache, k8sutil.K8s)
			Expect(err).ToNot(HaveOccurred())

			err = cli.Create(context.TODO(), job)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() error {
				job := &batchv1.Job{}
				jobName := types.NamespacedName{Name: getJobName(nimCache), Namespace: "default"}
				return cli.Get(ctx, jobName, job)
			}, time.Second*10).Should(Succeed())

			// Expected environment variables
			expectedEnvs := map[string]string{
				"NGC_HOME":    "/opt/ngc/hub",
				"HTTPS_PROXY": "https://my-custom-proxy-server:port",
			}

			// Verify each custom environment variable
			for key, value := range expectedEnvs {
				var found bool
				for _, envVar := range job.Spec.Template.Spec.Containers[0].Env {
					if envVar.Name == key && envVar.Value == value {
						found = true
						break
					}
				}
				Expect(found).To(BeTrue(), "Expected environment variable %s=%s not found", key, value)
			}
		})

		It("should create a job with the right custom CA certificate volumes and Init Container", func() {
			ctx := context.TODO()
			profiles := []string{AllProfiles}
			nimCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimcache",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source: appsv1alpha1.NIMSource{NGC: &appsv1alpha1.NGCSource{ModelPuller: "nvcr.io/nim:test", PullSecret: "my-secret", Model: appsv1alpha1.ModelSpec{Profiles: profiles}}},
					Proxy: &appsv1alpha1.ProxySpec{
						HttpProxy:     "http://proxy:1000",
						HttpsProxy:    "https://proxy:1000",
						NoProxy:       "http://no-proxy",
						CertConfigMap: "custom-ca-configmap",
					},
				},
			}

			// Create a sample ConfigMap with certificate files
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "custom-ca-configmap",
					Namespace: "default",
				},
				Data: map[string]string{
					"custom-ca-cert.pem": "fake-cert-data",
					"another-cert.pem":   "fake-cert-data-2",
				},
			}

			err := reconciler.Create(context.TODO(), configMap)
			Expect(err).ToNot(HaveOccurred())

			job, err := reconciler.constructJob(context.TODO(), nimCache, k8sutil.K8s)
			Expect(err).ToNot(HaveOccurred())

			err = cli.Create(context.TODO(), job)
			Expect(err).ToNot(HaveOccurred())

			job = &batchv1.Job{}
			jobName := types.NamespacedName{Name: getJobName(nimCache), Namespace: "default"}
			err = cli.Get(ctx, jobName, job)
			Expect(err).ToNot(HaveOccurred())

			// Verify CertConfig volume and mounts
			Expect(job.Spec.Template.Spec.Volumes).To(ContainElement(
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

			Expect(job.Spec.Template.Spec.Volumes).To(ContainElement(
				corev1.Volume{
					Name: "ca-cert-volume",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			))

			Expect(job.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(
				corev1.VolumeMount{
					Name:      "ca-cert-volume",
					MountPath: "/etc/ssl",
				},
			))

			Expect(job.Spec.Template.Spec.InitContainers[0].VolumeMounts).To(ContainElement(
				corev1.VolumeMount{
					Name:      "ca-cert-volume",
					MountPath: "/ca-certs",
				},
			))

			Expect(job.Spec.Template.Spec.InitContainers[0].VolumeMounts).To(ContainElement(
				corev1.VolumeMount{
					Name:      "custom-ca",
					MountPath: "/custom",
				},
			))
			Expect(job.Spec.Template.Spec.InitContainers[0].Command).To(ContainElements(k8sutil.GetUpdateCaCertInitContainerCommand()))
			Expect(job.Spec.Template.Spec.InitContainers[0].SecurityContext).To(Equal(k8sutil.GetUpdateCaCertInitContainerSecurityContext()))

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
				for _, envVar := range job.Spec.Template.Spec.Containers[0].Env {
					if envVar.Name == key && envVar.Value == value {
						found = true
						break
					}
				}
				Expect(found).To(BeTrue(), "Expected environment variable %s=%s not found", key, value)
			}

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

			nimparser := nimparserv1.NIMParser{}
			manifestData, err := nimparser.ParseModelManifest(filePath)
			Expect(err).NotTo(HaveOccurred())

			err = reconciler.createManifestConfigMap(ctx, nimCache, &manifestData)
			Expect(err).NotTo(HaveOccurred())

			// Verify that the ConfigMap was created
			createdConfigMap := &corev1.ConfigMap{}
			err = cli.Get(ctx, types.NamespacedName{Name: getManifestConfigName(nimCache), Namespace: "default"}, createdConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(createdConfigMap.Data).To(HaveKey("model_manifest.yaml"))

			// Verify the content of model_manifest.yaml
			extractedManifest, err := reconciler.extractNIMManifest(ctx, createdConfigMap.Name, createdConfigMap.Namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(extractedManifest).NotTo(BeNil())
			nimManifest, ok := extractedManifest.(nimparserv1.NIMManifest)
			Expect(ok).To(BeTrue())
			profile, exists := (nimManifest)["03fdb4d11f01be10c31b00e7c0540e2835e89a0079b483ad2dd3c25c8cc29b61"]
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

	Context("when error reconciling NIMCache resource", func() {
		BeforeEach(func() {
			scheme = runtime.NewScheme()
			Expect(appsv1alpha1.AddToScheme(scheme)).To(Succeed())
			Expect(appsv1alpha1.AddToScheme(scheme)).To(Succeed())
			Expect(batchv1.AddToScheme(scheme)).To(Succeed())
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			Expect(rbacv1.AddToScheme(scheme)).To(Succeed())

			cli = fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&appsv1alpha1.NIMCache{}).
				WithStatusSubresource(&appsv1alpha1.NIMCache{}).
				WithStatusSubresource(&batchv1.Job{}).
				WithStatusSubresource(&corev1.ConfigMap{}).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if key.Name == NIMCacheServiceAccount {
							return errors.NewBadRequest("error getting SA for NIMCache")
						}
						return client.Get(ctx, key, obj, opts...)
					},
				}).
				Build()

			reconciler = &NIMCacheReconciler{
				Client:   cli,
				scheme:   scheme,
				recorder: record.NewFakeRecorder(1000),
			}

		})

		It("should update the status on NIMCache object", func() {
			ctx := context.TODO()
			NIMCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimcache",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source:  appsv1alpha1.NIMSource{NGC: &appsv1alpha1.NGCSource{ModelPuller: "test-container", PullSecret: "my-secret"}},
					Storage: appsv1alpha1.NIMCacheStorage{PVC: appsv1alpha1.PersistentVolumeClaim{Create: ptr.To[bool](true), StorageClass: "standard", Size: "1Gi"}},
				},
			}
			Expect(cli.Create(ctx, NIMCache)).To(Succeed())
			Expect(cli.Get(ctx, types.NamespacedName{Name: "test-nimcache", Namespace: "default"}, NIMCache)).To(Succeed())

			// Reconcile the resource
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-nimcache", Namespace: "default"}})
			Expect(err).To(HaveOccurred())

			Expect(cli.Get(ctx, types.NamespacedName{Name: "test-nimcache", Namespace: "default"}, NIMCache)).To(Succeed())
			Expect(NIMCache.Status.State).To(Equal(appsv1alpha1.NimCacheStatusNotReady))
			Expect(NIMCache.Status.Conditions[0].Type).To(Equal(appsv1alpha1.NimCacheConditionReconcileFailed))
			Expect(NIMCache.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(NIMCache.Status.Conditions[0].Message).To(Equal("error getting SA for NIMCache"))

		})

	})
})
