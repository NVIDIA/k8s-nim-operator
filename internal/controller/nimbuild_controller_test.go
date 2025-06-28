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
	"fmt"

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
			fmt.Println("HEHRHEHEHupdatedNIMBuild", updatedNIMBuild.Status.Conditions)
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
})
