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

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/api/meta"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
)

var _ = Describe("NIMBuild Controller", func() {
	var (
		cli        client.Client
		reconciler *NIMBuildReconciler
		scheme     *runtime.Scheme
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.TODO()
		scheme = runtime.NewScheme()
		Expect(appsv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		cli = fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&appsv1alpha1.NIMBuild{}).
			WithStatusSubresource(&appsv1alpha1.NIMCache{}).
			WithStatusSubresource(&corev1.Pod{}).
			WithStatusSubresource(&corev1.ConfigMap{}).
			Build()

		reconciler = &NIMBuildReconciler{
			Client:   cli,
			scheme:   scheme,
			recorder: record.NewFakeRecorder(1000),
		}
	})

	Context("When creating a NIMBuild", func() {
		It("should add finalizer when not present", func() {
			nimBuild := &appsv1alpha1.NIMBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimbuild",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMBuildSpec{
					NIMCache: appsv1alpha1.NIMCacheReference{
						Name: "test-nimcache",
					},
					Image: appsv1alpha1.Image{
						Repository: "nvcr.io/nim/test",
						Tag:        "latest",
					},
				},
			}

			Expect(cli.Create(ctx, nimBuild)).To(Succeed())

			// Reconcile
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      nimBuild.Name,
					Namespace: nimBuild.Namespace,
				},
			})
			Expect(err).ToNot(HaveOccurred())

			// Check if finalizer was added
			updatedNIMBuild := &appsv1alpha1.NIMBuild{}
			Expect(cli.Get(ctx, types.NamespacedName{Name: nimBuild.Name, Namespace: nimBuild.Namespace}, updatedNIMBuild)).To(Succeed())
			Expect(updatedNIMBuild.Finalizers).To(ContainElement(NIMBuildFinalizer))
		})

		It("should fail when NIMCache is not found", func() {
			nimBuild := &appsv1alpha1.NIMBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimbuild",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMBuildSpec{
					NIMCache: appsv1alpha1.NIMCacheReference{
						Name: "non-existent-nimcache",
					},
					Image: appsv1alpha1.Image{
						Repository: "nvcr.io/nim/test",
						Tag:        "latest",
					},
				},
			}

			Expect(cli.Create(ctx, nimBuild)).To(Succeed())

			// Reconcile
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      nimBuild.Name,
					Namespace: nimBuild.Namespace,
				},
			})
			Expect(err).ToNot(HaveOccurred())

			// Check status
			updatedNIMBuild := &appsv1alpha1.NIMBuild{}
			Expect(cli.Get(ctx, types.NamespacedName{Name: nimBuild.Name, Namespace: nimBuild.Namespace}, updatedNIMBuild)).To(Succeed())
			Expect(updatedNIMBuild.Status.State).To(Equal(appsv1alpha1.NimBuildStatusFailed))
			Expect(meta.IsStatusConditionTrue(updatedNIMBuild.Status.Conditions, appsv1alpha1.NimBuildConditionNIMCacheNotFound)).To(BeTrue())
		})

		It("should fail when NIMCache is in failed state", func() {
			nimCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimcache",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source: appsv1alpha1.NIMSource{
						NGC: &appsv1alpha1.NGCSource{
							ModelPuller: "nvcr.io/nim/test",
							AuthSecret:  "my-secret",
						},
					},
				},
				Status: appsv1alpha1.NIMCacheStatus{
					State: appsv1alpha1.NimCacheStatusFailed,
				},
			}
			Expect(cli.Create(ctx, nimCache)).To(Succeed())

			nimBuild := &appsv1alpha1.NIMBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimbuild",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMBuildSpec{
					NIMCache: appsv1alpha1.NIMCacheReference{
						Name: nimCache.Name,
					},
					Image: appsv1alpha1.Image{
						Repository: "nvcr.io/nim/test",
						Tag:        "latest",
					},
				},
			}

			Expect(cli.Create(ctx, nimBuild)).To(Succeed())

			// Reconcile
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      nimBuild.Name,
					Namespace: nimBuild.Namespace,
				},
			})
			Expect(err).ToNot(HaveOccurred())

			// Check status
			updatedNIMBuild := &appsv1alpha1.NIMBuild{}
			Expect(cli.Get(ctx, types.NamespacedName{Name: nimBuild.Name, Namespace: nimBuild.Namespace}, updatedNIMBuild)).To(Succeed())
			Expect(updatedNIMBuild.Status.State).To(Equal(appsv1alpha1.NimBuildStatusFailed))
			Expect(meta.IsStatusConditionTrue(updatedNIMBuild.Status.Conditions, appsv1alpha1.NimBuildConditionNimCacheFailed)).To(BeTrue())
		})
	})

	Context("When NIMCache is ready", func() {
		var nimCache *appsv1alpha1.NIMCache

		BeforeEach(func() {
			nimCache = &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimcache",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source: appsv1alpha1.NIMSource{
						NGC: &appsv1alpha1.NGCSource{
							ModelPuller: "nvcr.io/nim/test",
							AuthSecret:  "my-secret",
						},
					},
					Storage: appsv1alpha1.NIMCacheStorage{
						PVC: appsv1alpha1.PersistentVolumeClaim{
							Create:       ptr.To[bool](true),
							StorageClass: "standard",
							Size:         "1Gi",
							SubPath:      "test-subpath",
						},
					},
				},
				Status: appsv1alpha1.NIMCacheStatus{
					State: appsv1alpha1.NimCacheStatusReady,
					Profiles: []appsv1alpha1.NIMProfile{
						{
							Name:   "buildable-profile",
							Model:  "test-model",
							Config: map[string]string{"trtllm_buildable": "true", "tp": "8"},
						},
					},
				},
			}
			Expect(cli.Create(ctx, nimCache)).To(Succeed())
		})

		It("should create engine build pod when no profile specified", func() {
			nimBuild := &appsv1alpha1.NIMBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimbuild",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMBuildSpec{
					NIMCache: appsv1alpha1.NIMCacheReference{
						Name: nimCache.Name,
					},
					Image: appsv1alpha1.Image{
						Repository: "nvcr.io/nim/test",
						Tag:        "latest",
					},
				},
			}

			Expect(cli.Create(ctx, nimBuild)).To(Succeed())

			// Reconcile
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      nimBuild.Name,
					Namespace: nimBuild.Namespace,
				},
			})
			Expect(err).ToNot(HaveOccurred())

			// Check if pod was created
			pod := &corev1.Pod{}
			podName := types.NamespacedName{
				Name:      nimBuild.GetEngineBuildPodName(),
				Namespace: nimBuild.Namespace,
			}
			Expect(cli.Get(ctx, podName, pod)).To(Succeed())
			Expect(pod.Spec.Containers).To(HaveLen(1))
			Expect(pod.Spec.Containers[0].Name).To(Equal(NIMBuildContainerName))
			Expect(pod.Spec.Containers[0].Image).To(Equal(nimBuild.GetImage()))

			// Check status
			updatedNIMBuild := &appsv1alpha1.NIMBuild{}
			Expect(cli.Get(ctx, types.NamespacedName{Name: nimBuild.Name, Namespace: nimBuild.Namespace}, updatedNIMBuild)).To(Succeed())
			Expect(meta.IsStatusConditionTrue(updatedNIMBuild.Status.Conditions, appsv1alpha1.NimBuildConditionEngineBuildPodCreated)).To(BeTrue())
		})

		It("should fail when multiple buildable profiles found", func() {
			// Update NIMCache to have multiple buildable profiles
			nimCache.Status.Profiles = append(nimCache.Status.Profiles, appsv1alpha1.NIMProfile{
				Name:   "buildable-profile-2",
				Model:  "test-model-2",
				Config: map[string]string{"trtllm_buildable": "true", "tp": "4"},
			})
			Expect(cli.Status().Update(ctx, nimCache)).To(Succeed())

			nimBuild := &appsv1alpha1.NIMBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimbuild",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMBuildSpec{
					NIMCache: appsv1alpha1.NIMCacheReference{
						Name: nimCache.Name,
					},
					Image: appsv1alpha1.Image{
						Repository: "nvcr.io/nim/test",
						Tag:        "latest",
					},
				},
			}

			Expect(cli.Create(ctx, nimBuild)).To(Succeed())

			// Reconcile
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      nimBuild.Name,
					Namespace: nimBuild.Namespace,
				},
			})
			Expect(err).ToNot(HaveOccurred())

			// Check status
			updatedNIMBuild := &appsv1alpha1.NIMBuild{}
			Expect(cli.Get(ctx, types.NamespacedName{Name: nimBuild.Name, Namespace: nimBuild.Namespace}, updatedNIMBuild)).To(Succeed())
			Expect(updatedNIMBuild.Status.State).To(Equal(appsv1alpha1.NimBuildStatusFailed))
			Expect(meta.IsStatusConditionTrue(updatedNIMBuild.Status.Conditions, appsv1alpha1.NimBuildConditionMultipleBuildableProfilesFound)).To(BeTrue())
		})

		It("should fail when no buildable profiles found", func() {
			// Update NIMCache to have no buildable profiles
			nimCache.Status.Profiles = []appsv1alpha1.NIMProfile{
				{
					Name:   "non-buildable-profile",
					Model:  "test-model",
					Config: map[string]string{"trtllm_buildable": "false"},
				},
			}
			Expect(cli.Status().Update(ctx, nimCache)).To(Succeed())

			nimBuild := &appsv1alpha1.NIMBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimbuild",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMBuildSpec{
					NIMCache: appsv1alpha1.NIMCacheReference{
						Name: nimCache.Name,
					},
					Image: appsv1alpha1.Image{
						Repository: "nvcr.io/nim/test",
						Tag:        "latest",
					},
				},
			}

			Expect(cli.Create(ctx, nimBuild)).To(Succeed())

			// Reconcile
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      nimBuild.Name,
					Namespace: nimBuild.Namespace,
				},
			})
			Expect(err).ToNot(HaveOccurred())

			// Check status
			updatedNIMBuild := &appsv1alpha1.NIMBuild{}
			Expect(cli.Get(ctx, types.NamespacedName{Name: nimBuild.Name, Namespace: nimBuild.Namespace}, updatedNIMBuild)).To(Succeed())
			Expect(updatedNIMBuild.Status.State).To(Equal(appsv1alpha1.NimBuildStatusFailed))
			Expect(meta.IsStatusConditionTrue(updatedNIMBuild.Status.Conditions, appsv1alpha1.NimBuildConditionNoBuildableProfilesFound)).To(BeTrue())
		})
	})

	Context("When engine build pod is running", func() {
		var nimBuild *appsv1alpha1.NIMBuild
		var nimCache *appsv1alpha1.NIMCache
		var pod *corev1.Pod

		BeforeEach(func() {
			nimCache = &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimcache",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source: appsv1alpha1.NIMSource{
						NGC: &appsv1alpha1.NGCSource{
							ModelPuller: "nvcr.io/nim/test",
							AuthSecret:  "my-secret",
						},
					},
					Storage: appsv1alpha1.NIMCacheStorage{
						PVC: appsv1alpha1.PersistentVolumeClaim{
							Create:       ptr.To[bool](true),
							StorageClass: "standard",
							Size:         "1Gi",
							SubPath:      "test-subpath",
						},
					},
				},
				Status: appsv1alpha1.NIMCacheStatus{
					State: appsv1alpha1.NimCacheStatusReady,
					Profiles: []appsv1alpha1.NIMProfile{
						{
							Name:   "buildable-profile",
							Model:  "test-model",
							Config: map[string]string{"trtllm_buildable": "true", "tp": "8"},
						},
					},
				},
			}
			Expect(cli.Create(ctx, nimCache)).To(Succeed())

			nimBuild = &appsv1alpha1.NIMBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimbuild",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMBuildSpec{
					NIMCache: appsv1alpha1.NIMCacheReference{
						Name: nimCache.Name,
					},
					Image: appsv1alpha1.Image{
						Repository: "nvcr.io/nim/test",
						Tag:        "latest",
					},
					Resources: &appsv1alpha1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("8"),
						},
						Limits: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("8"),
						},
					},
				},
			}
			Expect(cli.Create(ctx, nimBuild)).To(Succeed())

			pod = &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nimBuild.GetEngineBuildPodName(),
					Namespace: nimBuild.Namespace,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  NIMBuildContainerName,
							Image: nimBuild.GetImage(),
						},
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			}
			Expect(cli.Create(ctx, pod)).To(Succeed())

			// Set condition that pod was created
			conditions.UpdateCondition(&nimBuild.Status.Conditions, appsv1alpha1.NimBuildConditionEngineBuildPodCreated, metav1.ConditionTrue, "PodCreated", "Pod created")
			Expect(cli.Status().Update(ctx, nimBuild)).To(Succeed())
		})

		It("should update status to in progress when pod is running", func() {
			// Reconcile
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      nimBuild.Name,
					Namespace: nimBuild.Namespace,
				},
			})
			Expect(err).ToNot(HaveOccurred())

			// Check status
			updatedNIMBuild := &appsv1alpha1.NIMBuild{}
			Expect(cli.Get(ctx, types.NamespacedName{Name: nimBuild.Name, Namespace: nimBuild.Namespace}, updatedNIMBuild)).To(Succeed())
			Expect(updatedNIMBuild.Status.State).To(Equal(appsv1alpha1.NimBuildStatusInProgress))
			Expect(meta.IsStatusConditionTrue(updatedNIMBuild.Status.Conditions, appsv1alpha1.NimBuildConditionEngineBuildPodPending)).To(BeTrue())
		})

		It("should update status to failed when pod fails", func() {
			// Update pod to failed state
			pod.Status.Phase = corev1.PodFailed
			Expect(cli.Status().Update(ctx, pod)).To(Succeed())

			// Reconcile
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      nimBuild.Name,
					Namespace: nimBuild.Namespace,
				},
			})
			Expect(err).ToNot(HaveOccurred())

			// Check status
			updatedNIMBuild := &appsv1alpha1.NIMBuild{}
			Expect(cli.Get(ctx, types.NamespacedName{Name: nimBuild.Name, Namespace: nimBuild.Namespace}, updatedNIMBuild)).To(Succeed())
			Expect(updatedNIMBuild.Status.State).To(Equal(appsv1alpha1.NimBuildStatusFailed))
		})

		It("should complete when pod is ready", func() {
			// Update pod to ready state
			pod.Status.Conditions = []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			}
			Expect(cli.Status().Update(ctx, pod)).To(Succeed())

			// Reconcile
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      nimBuild.Name,
					Namespace: nimBuild.Namespace,
				},
			})
			Expect(err).ToNot(HaveOccurred())

			// Check status
			updatedNIMBuild := &appsv1alpha1.NIMBuild{}
			Expect(cli.Get(ctx, types.NamespacedName{Name: nimBuild.Name, Namespace: nimBuild.Namespace}, updatedNIMBuild)).To(Succeed())
			Expect(meta.IsStatusConditionTrue(updatedNIMBuild.Status.Conditions, appsv1alpha1.NimBuildConditionEngineBuildPodCompleted)).To(BeTrue())

			// Check if pod was deleted
			deletedPod := &corev1.Pod{}
			err = cli.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, deletedPod)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})
	})

	Context("Helper functions", func() {
		It("should correctly identify buildable profiles", func() {
			nimCache := &appsv1alpha1.NIMCache{
				Status: appsv1alpha1.NIMCacheStatus{
					Profiles: []appsv1alpha1.NIMProfile{
						{
							Name:   "buildable-1",
							Config: map[string]string{"trtllm_buildable": "true"},
						},
						{
							Name:   "non-buildable-1",
							Config: map[string]string{"trtllm_buildable": "false"},
						},
						{
							Name:   "buildable-2",
							Config: map[string]string{"trtllm_buildable": "true"},
						},
					},
				},
			}

			buildableProfiles := getBuildableProfiles(nimCache)
			Expect(buildableProfiles).To(HaveLen(2))
			Expect(buildableProfiles[0].Name).To(Equal("buildable-1"))
			Expect(buildableProfiles[1].Name).To(Equal("buildable-2"))
		})

		It("should find buildable profile by name", func() {
			nimCache := &appsv1alpha1.NIMCache{
				Status: appsv1alpha1.NIMCacheStatus{
					Profiles: []appsv1alpha1.NIMProfile{
						{
							Name:   "buildable-1",
							Config: map[string]string{"trtllm_buildable": "true"},
						},
						{
							Name:   "non-buildable-1",
							Config: map[string]string{"trtllm_buildable": "false"},
						},
					},
				},
			}

			profile := getBuildableProfileByName(nimCache, "buildable-1")
			Expect(profile).NotTo(BeNil())
			Expect(profile.Name).To(Equal("buildable-1"))

			profile = getBuildableProfileByName(nimCache, "non-buildable-1")
			Expect(profile).To(BeNil())
		})

		It("should correctly check if pod is ready", func() {
			pod := &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}
			Expect(isPodReady(pod)).To(BeTrue())

			pod.Status.Conditions[0].Status = corev1.ConditionFalse
			Expect(isPodReady(pod)).To(BeFalse())
		})
	})

	Context("Pod construction", func() {
		It("should construct engine build pod correctly", func() {
			nimCache := &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimcache",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source: appsv1alpha1.NIMSource{
						NGC: &appsv1alpha1.NGCSource{
							ModelPuller: "nvcr.io/nim/test",
							AuthSecret:  "my-secret",
						},
					},
					Storage: appsv1alpha1.NIMCacheStorage{
						PVC: appsv1alpha1.PersistentVolumeClaim{
							Create:       ptr.To[bool](true),
							StorageClass: "standard",
							Size:         "1Gi",
							SubPath:      "test-subpath",
						},
					},
				},
			}

			nimBuild := &appsv1alpha1.NIMBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimbuild",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMBuildSpec{
					NIMCache: appsv1alpha1.NIMCacheReference{
						Name: nimCache.Name,
					},
					Image: appsv1alpha1.Image{
						Repository: "nvcr.io/nim/test",
						Tag:        "latest",
					},
				},
			}

			nimProfile := appsv1alpha1.NIMProfile{
				Name:   "test-profile",
				Config: map[string]string{"tp": "8"},
			}

			pod, err := reconciler.constructEngineBuildPod(nimBuild, nimCache, k8sutil.K8s, nimProfile)
			Expect(err).ToNot(HaveOccurred())
			Expect(pod.Name).To(Equal(nimBuild.GetEngineBuildPodName()))
			Expect(pod.Namespace).To(Equal(nimBuild.Namespace))
			Expect(pod.Spec.Containers).To(HaveLen(1))
			Expect(pod.Spec.Containers[0].Name).To(Equal(NIMBuildContainerName))
			Expect(pod.Spec.Containers[0].Image).To(Equal(nimBuild.GetImage()))
			Expect(pod.Spec.Volumes).To(HaveLen(1))
			Expect(pod.Spec.Volumes[0].Name).To(Equal("nim-cache-volume"))
		})
	})

	Context("Environment variable handling", func() {
		var nimCache *appsv1alpha1.NIMCache

		BeforeEach(func() {
			nimCache = &appsv1alpha1.NIMCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimcache",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMCacheSpec{
					Source: appsv1alpha1.NIMSource{
						NGC: &appsv1alpha1.NGCSource{
							ModelPuller: "nvcr.io/nim/test",
							AuthSecret:  "my-secret",
						},
					},
					Storage: appsv1alpha1.NIMCacheStorage{
						PVC: appsv1alpha1.PersistentVolumeClaim{
							Create:       ptr.To[bool](true),
							StorageClass: "standard",
							Size:         "1Gi",
							SubPath:      "test-subpath",
						},
					},
				},
				Status: appsv1alpha1.NIMCacheStatus{
					State: appsv1alpha1.NimCacheStatusReady,
					Profiles: []appsv1alpha1.NIMProfile{
						{
							Name:   "test-profile",
							Model:  "test-model",
							Config: map[string]string{"trtllm_buildable": "true"},
						},
					},
				},
			}
		})

		It("should set default environment variables correctly", func() {
			nimBuild := &appsv1alpha1.NIMBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimbuild",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMBuildSpec{
					NIMCache: appsv1alpha1.NIMCacheReference{
						Name: nimCache.Name,
					},
					Image: appsv1alpha1.Image{
						Repository: "nvcr.io/nim/test",
						Tag:        "latest",
					},
					Resources: &appsv1alpha1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("8"),
						},
						Limits: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("8"),
						},
					},
				},
			}

			// Test pod construction with default environment variables
			pod, err := reconciler.constructEngineBuildPod(nimBuild, nimCache, k8sutil.K8s, nimCache.Status.Profiles[0])
			Expect(err).ToNot(HaveOccurred())

			// Check that default environment variables are set
			envVars := pod.Spec.Containers[0].Env
			Expect(envVars).NotTo(BeEmpty())

			// Check for required environment variables
			var nimCachePathFound, nimServerPortFound, nimHttpApiPortFound, nimCustomModelNameFound, nimModelProfileFound, ngcApiKeyFound bool
			for _, env := range envVars {
				switch env.Name {
				case "NIM_CACHE_PATH":
					Expect(env.Value).To(Equal("/model-store"))
					nimCachePathFound = true
				case "NIM_SERVER_PORT":
					Expect(env.Value).To(Equal("8000"))
					nimServerPortFound = true
				case "NIM_HTTP_API_PORT":
					Expect(env.Value).To(Equal("8000"))
					nimHttpApiPortFound = true
				case "NIM_CUSTOM_MODEL_NAME":
					Expect(env.Value).To(Equal(nimBuild.GetModelName()))
					nimCustomModelNameFound = true
				case "NIM_MODEL_PROFILE":
					Expect(env.Value).To(Equal(nimCache.Status.Profiles[0].Name))
					nimModelProfileFound = true
				case "NGC_API_KEY":
					Expect(env.ValueFrom).NotTo(BeNil())
					Expect(env.ValueFrom.SecretKeyRef).NotTo(BeNil())
					Expect(env.ValueFrom.SecretKeyRef.Name).To(Equal(nimCache.Spec.Source.NGC.AuthSecret))
					Expect(env.ValueFrom.SecretKeyRef.Key).To(Equal("NGC_API_KEY"))
					ngcApiKeyFound = true
				}
			}
			Expect(nimCachePathFound).To(BeTrue(), "NIM_CACHE_PATH should be set")
			Expect(nimServerPortFound).To(BeTrue(), "NIM_SERVER_PORT should be set")
			Expect(nimHttpApiPortFound).To(BeTrue(), "NIM_HTTP_API_PORT should be set")
			Expect(nimCustomModelNameFound).To(BeTrue(), "NIM_CUSTOM_MODEL_NAME should be set")
			Expect(nimModelProfileFound).To(BeTrue(), "NIM_MODEL_PROFILE should be set")
			Expect(ngcApiKeyFound).To(BeTrue(), "NGC_API_KEY should be set")
		})

		It("should merge user-provided environment variables", func() {
			nimBuild := &appsv1alpha1.NIMBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimbuild",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMBuildSpec{
					NIMCache: appsv1alpha1.NIMCacheReference{
						Name: nimCache.Name,
					},
					Image: appsv1alpha1.Image{
						Repository: "nvcr.io/nim/test",
						Tag:        "latest",
					},
					Resources: &appsv1alpha1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("8"),
						},
						Limits: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("8"),
						},
					},
					Env: []corev1.EnvVar{
						{
							Name:  "CUSTOM_VAR",
							Value: "custom-value",
						},
						{
							Name:  "NIM_CACHE_PATH",
							Value: "overridden-path",
						},
						{
							Name:  "ANOTHER_VAR",
							Value: "another-value",
						},
					},
				},
			}

			// Test pod construction with user-provided environment variables
			pod, err := reconciler.constructEngineBuildPod(nimBuild, nimCache, k8sutil.K8s, nimCache.Status.Profiles[0])
			Expect(err).ToNot(HaveOccurred())

			// Check that all environment variables are present
			envVars := pod.Spec.Containers[0].Env
			Expect(envVars).NotTo(BeEmpty())

			// Check for user-provided environment variables
			var customVarFound, overriddenVarFound, anotherVarFound bool
			for _, env := range envVars {
				switch env.Name {
				case "CUSTOM_VAR":
					Expect(env.Value).To(Equal("custom-value"))
					customVarFound = true
				case "NIM_CACHE_PATH":
					Expect(env.Value).To(Equal("overridden-path"))
					overriddenVarFound = true
				case "ANOTHER_VAR":
					Expect(env.Value).To(Equal("another-value"))
					anotherVarFound = true
				}
			}
			Expect(customVarFound).To(BeTrue(), "CUSTOM_VAR should be set")
			Expect(overriddenVarFound).To(BeTrue(), "NIM_CACHE_PATH should be overridden")
			Expect(anotherVarFound).To(BeTrue(), "ANOTHER_VAR should be set")
		})

		It("should handle environment variables with valueFrom", func() {
			nimBuild := &appsv1alpha1.NIMBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimbuild",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMBuildSpec{
					NIMCache: appsv1alpha1.NIMCacheReference{
						Name: nimCache.Name,
					},
					Image: appsv1alpha1.Image{
						Repository: "nvcr.io/nim/test",
						Tag:        "latest",
					},
					Resources: &appsv1alpha1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("8"),
						},
						Limits: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("8"),
						},
					},
					Env: []corev1.EnvVar{
						{
							Name: "SECRET_VAR",
							ValueFrom: &corev1.EnvVarSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "my-secret",
									},
									Key: "secret-key",
								},
							},
						},
						{
							Name: "CONFIG_VAR",
							ValueFrom: &corev1.EnvVarSource{
								ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "my-config",
									},
									Key: "config-key",
								},
							},
						},
					},
				},
			}

			// Test pod construction with environment variables using valueFrom
			pod, err := reconciler.constructEngineBuildPod(nimBuild, nimCache, k8sutil.K8s, nimCache.Status.Profiles[0])
			Expect(err).ToNot(HaveOccurred())

			// Check that environment variables with valueFrom are preserved
			envVars := pod.Spec.Containers[0].Env
			var secretVarFound, configVarFound bool
			for _, env := range envVars {
				switch env.Name {
				case "SECRET_VAR":
					Expect(env.ValueFrom).NotTo(BeNil())
					Expect(env.ValueFrom.SecretKeyRef).NotTo(BeNil())
					Expect(env.ValueFrom.SecretKeyRef.Name).To(Equal("my-secret"))
					Expect(env.ValueFrom.SecretKeyRef.Key).To(Equal("secret-key"))
					secretVarFound = true
				case "CONFIG_VAR":
					Expect(env.ValueFrom).NotTo(BeNil())
					Expect(env.ValueFrom.ConfigMapKeyRef).NotTo(BeNil())
					Expect(env.ValueFrom.ConfigMapKeyRef.Name).To(Equal("my-config"))
					Expect(env.ValueFrom.ConfigMapKeyRef.Key).To(Equal("config-key"))
					configVarFound = true
				}
			}
			Expect(secretVarFound).To(BeTrue(), "SECRET_VAR should be set with valueFrom")
			Expect(configVarFound).To(BeTrue(), "CONFIG_VAR should be set with valueFrom")
		})

		It("should preserve default environment variables when user provides additional ones", func() {
			nimBuild := &appsv1alpha1.NIMBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimbuild",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMBuildSpec{
					NIMCache: appsv1alpha1.NIMCacheReference{
						Name: nimCache.Name,
					},
					Image: appsv1alpha1.Image{
						Repository: "nvcr.io/nim/test",
						Tag:        "latest",
					},
					Resources: &appsv1alpha1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("8"),
						},
						Limits: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("8"),
						},
					},
					Env: []corev1.EnvVar{
						{
							Name:  "USER_VAR",
							Value: "user-value",
						},
					},
				},
			}

			// Test pod construction
			pod, err := reconciler.constructEngineBuildPod(nimBuild, nimCache, k8sutil.K8s, nimCache.Status.Profiles[0])
			Expect(err).ToNot(HaveOccurred())

			// Check that both default and user environment variables are present
			envVars := pod.Spec.Containers[0].Env
			var nimCachePathFound, userVarFound bool
			for _, env := range envVars {
				switch env.Name {
				case "NIM_CACHE_PATH":
					Expect(env.Value).To(Equal("/model-store"))
					nimCachePathFound = true
				case "USER_VAR":
					Expect(env.Value).To(Equal("user-value"))
					userVarFound = true
				}
			}
			Expect(nimCachePathFound).To(BeTrue(), "Default NIM_CACHE_PATH should be preserved")
			Expect(userVarFound).To(BeTrue(), "User-provided USER_VAR should be set")
		})

		It("should handle empty environment variables list", func() {
			nimBuild := &appsv1alpha1.NIMBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimbuild",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMBuildSpec{
					NIMCache: appsv1alpha1.NIMCacheReference{
						Name: nimCache.Name,
					},
					Image: appsv1alpha1.Image{
						Repository: "nvcr.io/nim/test",
						Tag:        "latest",
					},
					Resources: &appsv1alpha1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("8"),
						},
						Limits: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("8"),
						},
					},
					Env: []corev1.EnvVar{}, // Empty environment variables
				},
			}

			// Test pod construction with empty environment variables
			pod, err := reconciler.constructEngineBuildPod(nimBuild, nimCache, k8sutil.K8s, nimCache.Status.Profiles[0])
			Expect(err).ToNot(HaveOccurred())

			// Check that default environment variables are still set
			envVars := pod.Spec.Containers[0].Env
			Expect(envVars).NotTo(BeEmpty())

			var nimCachePathFound bool
			for _, env := range envVars {
				if env.Name == "NIM_CACHE_PATH" {
					Expect(env.Value).To(Equal("/model-store"))
					nimCachePathFound = true
					break
				}
			}
			Expect(nimCachePathFound).To(BeTrue(), "Default NIM_CACHE_PATH should be set even with empty user env vars")
		})

		It("should handle environment variables in end-to-end reconciliation", func() {
			Expect(cli.Create(ctx, nimCache)).To(Succeed())

			nimBuild := &appsv1alpha1.NIMBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimbuild",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMBuildSpec{
					NIMCache: appsv1alpha1.NIMCacheReference{
						Name: nimCache.Name,
					},
					Image: appsv1alpha1.Image{
						Repository: "nvcr.io/nim/test",
						Tag:        "latest",
					},
					Resources: &appsv1alpha1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("8"),
						},
						Limits: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("8"),
						},
					},
					Env: []corev1.EnvVar{
						{
							Name:  "BUILD_VAR",
							Value: "build-value",
						},
						{
							Name:  "NIM_MODEL_PATH",
							Value: "custom-model-path",
						},
					},
				},
			}

			Expect(cli.Create(ctx, nimBuild)).To(Succeed())

			// Reconcile
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      nimBuild.Name,
					Namespace: nimBuild.Namespace,
				},
			})
			Expect(err).ToNot(HaveOccurred())

			// Check if pod was created with correct environment variables
			pod := &corev1.Pod{}
			podName := types.NamespacedName{
				Name:      nimBuild.GetEngineBuildPodName(),
				Namespace: nimBuild.Namespace,
			}
			Expect(cli.Get(ctx, podName, pod)).To(Succeed())

			// Verify environment variables in the created pod
			envVars := pod.Spec.Containers[0].Env
			var buildVarFound, customModelPathFound, defaultCachePathFound bool
			for _, env := range envVars {
				switch env.Name {
				case "BUILD_VAR":
					Expect(env.Value).To(Equal("build-value"))
					buildVarFound = true
				case "NIM_MODEL_PATH":
					Expect(env.Value).To(Equal("custom-model-path"))
					customModelPathFound = true
				case "NIM_CACHE_PATH":
					Expect(env.Value).To(Equal("/model-store"))
					defaultCachePathFound = true
				}
			}
			Expect(buildVarFound).To(BeTrue(), "BUILD_VAR should be set in created pod")
			Expect(customModelPathFound).To(BeTrue(), "NIM_MODEL_PATH should be overridden in created pod")
			Expect(defaultCachePathFound).To(BeTrue(), "Default NIM_CACHE_PATH should be preserved in created pod")
		})

		It("should set NIM_PEFT_SOURCE when LORA is enabled in profile", func() {
			// Create a profile with LORA enabled
			loraProfile := appsv1alpha1.NIMProfile{
				Name:   "lora-profile",
				Model:  "test-model",
				Config: map[string]string{"feat_lora": "true", "trtllm_buildable": "true"},
			}

			nimBuild := &appsv1alpha1.NIMBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimbuild",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMBuildSpec{
					NIMCache: appsv1alpha1.NIMCacheReference{
						Name: nimCache.Name,
					},
					Image: appsv1alpha1.Image{
						Repository: "nvcr.io/nim/test",
						Tag:        "latest",
					},
					Resources: &appsv1alpha1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("8"),
						},
						Limits: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("8"),
						},
					},
				},
			}

			// Test pod construction with LORA-enabled profile
			pod, err := reconciler.constructEngineBuildPod(nimBuild, nimCache, k8sutil.K8s, loraProfile)
			Expect(err).ToNot(HaveOccurred())

			// Check that NIM_PEFT_SOURCE environment variable is set
			envVars := pod.Spec.Containers[0].Env
			var nimPeftSourceFound bool
			for _, env := range envVars {
				if env.Name == "NIM_PEFT_SOURCE" {
					Expect(env.Value).To(Equal("/tmp"))
					nimPeftSourceFound = true
					break
				}
			}
			Expect(nimPeftSourceFound).To(BeTrue(), "NIM_PEFT_SOURCE should be set when LORA is enabled")
		})

		It("should not set NIM_PEFT_SOURCE when LORA is disabled in profile", func() {
			// Create a profile with LORA disabled
			noLoraProfile := appsv1alpha1.NIMProfile{
				Name:   "no-lora-profile",
				Model:  "test-model",
				Config: map[string]string{"feat_lora": "false", "trtllm_buildable": "true"},
			}

			nimBuild := &appsv1alpha1.NIMBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimbuild",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMBuildSpec{
					NIMCache: appsv1alpha1.NIMCacheReference{
						Name: nimCache.Name,
					},
					Image: appsv1alpha1.Image{
						Repository: "nvcr.io/nim/test",
						Tag:        "latest",
					},
					Resources: &appsv1alpha1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("8"),
						},
						Limits: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("8"),
						},
					},
				},
			}

			// Test pod construction with LORA-disabled profile
			pod, err := reconciler.constructEngineBuildPod(nimBuild, nimCache, k8sutil.K8s, noLoraProfile)
			Expect(err).ToNot(HaveOccurred())

			// Check that NIM_PEFT_SOURCE environment variable is NOT set
			envVars := pod.Spec.Containers[0].Env
			for _, env := range envVars {
				Expect(env.Name).NotTo(Equal("NIM_PEFT_SOURCE"), "NIM_PEFT_SOURCE should not be set when LORA is disabled")
			}
		})

		It("should not set NIM_PEFT_SOURCE when LORA config is missing from profile", func() {
			// Create a profile without LORA config
			noLoraConfigProfile := appsv1alpha1.NIMProfile{
				Name:   "no-lora-config-profile",
				Model:  "test-model",
				Config: map[string]string{"trtllm_buildable": "true"},
			}

			nimBuild := &appsv1alpha1.NIMBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nimbuild",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NIMBuildSpec{
					NIMCache: appsv1alpha1.NIMCacheReference{
						Name: nimCache.Name,
					},
					Image: appsv1alpha1.Image{
						Repository: "nvcr.io/nim/test",
						Tag:        "latest",
					},
					Resources: &appsv1alpha1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("8"),
						},
						Limits: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("8"),
						},
					},
				},
			}

			// Test pod construction with profile missing LORA config
			pod, err := reconciler.constructEngineBuildPod(nimBuild, nimCache, k8sutil.K8s, noLoraConfigProfile)
			Expect(err).ToNot(HaveOccurred())

			// Check that NIM_PEFT_SOURCE environment variable is NOT set
			envVars := pod.Spec.Containers[0].Env
			for _, env := range envVars {
				Expect(env.Name).NotTo(Equal("NIM_PEFT_SOURCE"), "NIM_PEFT_SOURCE should not be set when LORA config is missing")
			}
		})
	})
})
