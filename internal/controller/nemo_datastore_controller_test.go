package controller

import (
	"context"
	"path/filepath"
	"time"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	"github.com/NVIDIA/k8s-nim-operator/internal/render"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("NemoDatastore Controller", func() {
	const resourceName = "test-resource"

	var (
		dsRec  *NemoDatastoreReconciler
		scheme *runtime.Scheme
		client client.Client

		ctx = context.Background()

		typeNamespacedName = types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}

		t        = true
		resource = &appsv1alpha1.NemoDatastore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: "default",
			},
			Spec: appsv1alpha1.NemoDatastoreSpec{
				Image: appsv1alpha1.Image{
					Repository: "test-repo",
					Tag:        "test-tag",
				},
				Replicas: 1,
				DatabaseConfig: appsv1alpha1.DatabaseConfig{
					Host:         "test-pg-host",
					DatabaseName: "test-pg-database",
					Credentials: appsv1alpha1.DatabaseCredentials{
						User:       "test-pg-user",
						SecretName: "test-pg-secret",
					},
				},
				PVC: &appsv1alpha1.PersistentVolumeClaim{
					Name:             "test-resource-pvc",
					Create:           &t,
					StorageClass:     "local-path",
					VolumeAccessMode: corev1.ReadWriteOnce,
					Size:             "1Gi",
				},
				ObjectStoreConfig: appsv1alpha1.ObjectStoreConfig{
					Endpoint:   "test-mino-host",
					BucketName: "test-bucket",
					Region:     "test-region",
					SSL:        false,
				},
			},
		}
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(appsv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(batchv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(autoscalingv2.AddToScheme(scheme)).To(Succeed())
		Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
		Expect(networkingv1.AddToScheme(scheme)).To(Succeed())
		Expect(monitoringv1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())

		manifestsDir, err := filepath.Abs("../../manifests")
		Expect(err).ToNot(HaveOccurred())

		client = fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&appsv1alpha1.NemoDatastore{}).
			WithStatusSubresource(&appsv1.Deployment{}).
			Build()

		dsRec = &NemoDatastoreReconciler{
			Client:   client,
			scheme:   scheme,
			updater:  conditions.NewUpdater(client),
			recorder: record.NewFakeRecorder(1000),
			renderer: render.NewRenderer(manifestsDir),
		}
	})

	Context("DataStore CR validation", func() {
		var (
			invalidDS *appsv1alpha1.NemoDatastore
		)

		BeforeEach(func() {
			invalidDS = &appsv1alpha1.NemoDatastore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: appsv1alpha1.NemoDatastoreSpec{},
			}
		})

		It("should reject DataStore with empty spec", func() {
			err := k8sClient.Create(ctx, invalidDS)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject DataStore with invalid database config", func() {
			invalidDS.Spec.DatabaseConfig = appsv1alpha1.DatabaseConfig{}
			err := k8sClient.Create(ctx, invalidDS)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject DataStore with invalid object store config", func() {
			invalidDS.Spec.ObjectStoreConfig = appsv1alpha1.ObjectStoreConfig{}
			err := k8sClient.Create(ctx, invalidDS)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject DataStore with invalid pre-requisite secrets", func() {
			invalidDS.Spec.Secrets = appsv1alpha1.Secrets{}
			err := k8sClient.Create(ctx, invalidDS)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsInvalid(err)).To(BeTrue())
		})
	})

	Context("DataStore CR reconciliation", func() {
		It("should successfully reconcile the DataStore object", func() {
			By("creating the custom resource for the kind NemoDatastore")
			Expect(client.Create(ctx, resource)).To(Succeed())

			_, err := dsRec.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).ToNot(HaveOccurred())

			ds := &appsv1alpha1.NemoDatastore{}
			Expect(client.Get(ctx, typeNamespacedName, ds)).To(Succeed())
			Expect(ds.Finalizers).To(ContainElement(NemoDatastoreFinalizer))
			Expect(ds.Status.State).To(Equal(appsv1alpha1.NemoDatastoreStatusNotReady))

			By("Ensuring PVC is created")
			pvc := &corev1.PersistentVolumeClaim{}
			Expect(client.Get(ctx, types.NamespacedName{
				Name:      resource.Spec.PVC.Name,
				Namespace: resource.Namespace,
			}, pvc))

			By("Ensuring Deployment is created")
			dsDep := appsv1.Deployment{}
			Expect(client.Get(ctx, typeNamespacedName, &dsDep)).To(Succeed())

			By("Ensuring resources are deleted along with datastore CR")
			Expect(client.Delete(ctx, resource)).To(Succeed())
			_, err = dsRec.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				ds = &appsv1alpha1.NemoDatastore{}
				err := client.Get(ctx, typeNamespacedName, ds)
				return err != nil && errors.IsNotFound(err)
			}, time.Second*5, time.Millisecond*500).Should(BeTrue())
		})
	})
})
