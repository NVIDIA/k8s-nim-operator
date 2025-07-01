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
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	kservev1beta1 "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	resourcev1beta2 "k8s.io/api/resource/v1beta2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	lwsv1 "sigs.k8s.io/lws/api/leaderworkerset/v1"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	platform "github.com/NVIDIA/k8s-nim-operator/internal/controller/platform"
	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
	"github.com/NVIDIA/k8s-nim-operator/internal/render"
	"github.com/NVIDIA/k8s-nim-operator/internal/shared"
	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

// NIMServiceFinalizer is the finalizer annotation.
const NIMServiceFinalizer = "finalizer.nimservice.apps.nvidia.com"

// NIMServiceReconciler reconciles a NIMService object.
type NIMServiceReconciler struct {
	client.Client
	scheme           *runtime.Scheme
	log              logr.Logger
	updater          conditions.Updater
	discoveryClient  discovery.DiscoveryInterface
	renderer         render.Renderer
	Config           *rest.Config
	Platform         platform.Platform
	orchestratorType k8sutil.OrchestratorType
	recorder         record.EventRecorder
}

// Ensure NIMServiceReconciler implements the Reconciler interface.
var _ shared.Reconciler = &NIMServiceReconciler{}

// NewNIMServiceReconciler creates a new reconciler for NIMService with the given platform.
func NewNIMServiceReconciler(client client.Client, scheme *runtime.Scheme, updater conditions.Updater, discoveryClient discovery.DiscoveryInterface, renderer render.Renderer, log logr.Logger, platform platform.Platform) *NIMServiceReconciler {
	return &NIMServiceReconciler{
		Client:          client,
		scheme:          scheme,
		updater:         updater,
		discoveryClient: discoveryClient,
		renderer:        renderer,
		log:             log,
		Platform:        platform,
	}
}

// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimservices/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nimcaches,verbs=get;list;watch;
// +kubebuilder:rbac:groups=config.openshift.io,resources=clusterversions;proxies,verbs=get;list;watch
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=use,resourceNames=nonroot
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=resource.k8s.io,resources=resourceclaims;resourceclaimtemplates,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=serviceaccounts;pods;pods/eviction;services;services/finalizers;endpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims;configmaps;secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors;prometheusrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scheduling.k8s.io,resources=priorityclasses,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;update;patch
// +kubebuilder:rbac:groups=leaderworkerset.x-k8s.io,resources=leaderworkersets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list
// +kubebuilder:rbac:groups=serving.kserve.io,resources=inferenceservices,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NIMService object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *NIMServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	defer r.refreshMetrics(ctx)

	// Fetch the NIMService instance
	nimService := &appsv1alpha1.NIMService{}
	if err := r.Get(ctx, req.NamespacedName, nimService); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "unable to fetch NIMService", "name", req.NamespacedName)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling", "NIMService", nimService.Name)

	// Check if the instance is marked for deletion
	if nimService.DeletionTimestamp.IsZero() {
		// Add finalizer if not present
		if !controllerutil.ContainsFinalizer(nimService, NIMServiceFinalizer) {
			controllerutil.AddFinalizer(nimService, NIMServiceFinalizer)
			if err := r.Update(ctx, nimService); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The instance is being deleted
		if controllerutil.ContainsFinalizer(nimService, NIMServiceFinalizer) {
			// Perform platform specific cleanup of resources
			if err := r.Platform.Delete(ctx, r, nimService); err != nil {
				r.GetEventRecorder().Eventf(nimService, corev1.EventTypeNormal, "Delete",
					"NIMService %s in deleted", nimService.Name)
				return ctrl.Result{}, err
			}

			// Remove finalizer to allow for deletion
			controllerutil.RemoveFinalizer(nimService, NIMServiceFinalizer)
			if err := r.Update(ctx, nimService); err != nil {
				r.GetEventRecorder().Eventf(nimService, corev1.EventTypeNormal, "Delete",
					"NIMService %s finalizer removed", nimService.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	// Fetch container orchestrator type
	_, err := r.GetOrchestratorType(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to get container orchestrator type, %v", err)
	}

	// Handle platform-specific reconciliation
	result, err := r.Platform.Sync(ctx, r, nimService)
	if err != nil {
		logger.Error(err, "error reconciling NIMService", "name", nimService.Name)
		return result, err
	}

	return result, nil
}

// GetScheme returns the scheme of the reconciler.
func (r *NIMServiceReconciler) GetScheme() *runtime.Scheme {
	return r.scheme
}

// GetLogger returns the logger of the reconciler.
func (r *NIMServiceReconciler) GetLogger() logr.Logger {
	return r.log
}

// GetClient returns the client instance.
func (r *NIMServiceReconciler) GetClient() client.Client {
	return r.Client
}

// GetConfig returns the rest config.
func (r *NIMServiceReconciler) GetConfig() *rest.Config {
	return r.Config
}

// GetUpdater returns the conditions updater instance.
func (r *NIMServiceReconciler) GetUpdater() conditions.Updater {
	return r.updater
}

// GetDiscoveryClient returns the discovery client instance.
func (r *NIMServiceReconciler) GetDiscoveryClient() discovery.DiscoveryInterface {
	return r.discoveryClient
}

// GetRenderer returns the renderer instance.
func (r *NIMServiceReconciler) GetRenderer() render.Renderer {
	return r.renderer
}

// GetEventRecorder returns the event recorder.
func (r *NIMServiceReconciler) GetEventRecorder() record.EventRecorder {
	return r.recorder
}

// GetOrchestratorType returns the container platform type.
func (r *NIMServiceReconciler) GetOrchestratorType(ctx context.Context) (k8sutil.OrchestratorType, error) {
	if r.orchestratorType == "" {
		orchestratorType, err := k8sutil.GetOrchestratorType(ctx, r.GetClient())
		if err != nil {
			return k8sutil.Unknown, fmt.Errorf("unable to get container orchestrator type, %v", err)
		}
		r.orchestratorType = orchestratorType
		r.GetLogger().Info("Container orchestrator is successfully set", "type", orchestratorType)
	}
	return r.orchestratorType, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NIMServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor("nimservice-controller")
	err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&appsv1alpha1.NIMService{},
		"spec.storage.nimCache.name",
		func(rawObj client.Object) []string {
			nimService, ok := rawObj.(*appsv1alpha1.NIMService)
			if !ok {
				return []string{}
			}
			return []string{nimService.Spec.Storage.NIMCache.Name}
		},
	)
	if err != nil {
		return err
	}

	nimServiceBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.NIMService{}).
		Owns(&appsv1.Deployment{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&networkingv1.Ingress{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Type assert to NIMService
				if oldNIMService, ok := e.ObjectOld.(*appsv1alpha1.NIMService); ok {
					newNIMService, ok := e.ObjectNew.(*appsv1alpha1.NIMService)
					if ok {
						// Handle case where object is marked for deletion
						if !newNIMService.ObjectMeta.DeletionTimestamp.IsZero() {
							return true
						}

						// Handle only spec updates
						return !reflect.DeepEqual(oldNIMService.Spec, newNIMService.Spec)
					}
				}
				// For other types we watch, reconcile them
				return true
			},
		}).
		Watches(
			&appsv1alpha1.NIMCache{},
			handler.EnqueueRequestsFromMapFunc(r.mapNIMCacheToNIMService),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		)

	resourceClaimCRDExists, err := k8sutil.CRDExists(r.discoveryClient, resourcev1beta2.SchemeGroupVersion.WithResource("resourceclaims"))
	if err != nil {
		return err
	}
	if resourceClaimCRDExists {
		nimServiceBuilder = nimServiceBuilder.Watches(
			&resourcev1beta2.ResourceClaim{},
			handler.EnqueueRequestsFromMapFunc(r.mapResourceClaimToNIMService),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		)
	}

	lwsCRDExists, err := k8sutil.CRDExists(r.discoveryClient, lwsv1.SchemeGroupVersion.WithResource("leaderworkersets"))
	if err != nil {
		return err
	}
	if lwsCRDExists {
		nimServiceBuilder = nimServiceBuilder.Owns(&lwsv1.LeaderWorkerSet{})
	}

	isvcCRDExists, err := k8sutil.CRDExists(r.discoveryClient, kservev1beta1.SchemeGroupVersion.WithResource("inferenceservices"))
	if err != nil {
		return err
	}
	if isvcCRDExists {
		nimServiceBuilder = nimServiceBuilder.Owns(&kservev1beta1.InferenceService{})
	}

	return nimServiceBuilder.Complete(r)
}

func (r *NIMServiceReconciler) mapNIMCacheToNIMService(ctx context.Context, obj client.Object) []ctrl.Request {
	nimCache, ok := obj.(*appsv1alpha1.NIMCache)
	if !ok {
		return []ctrl.Request{}
	}

	// Get all NIMServices that reference this NIMCache
	var nimServices appsv1alpha1.NIMServiceList
	if err := r.List(ctx, &nimServices, client.MatchingFields{"spec.storage.nimCache.name": nimCache.GetName()}, client.InNamespace(nimCache.GetNamespace())); err != nil {
		return []ctrl.Request{}
	}

	// Enqueue reconciliation for each matching NIMService
	requests := make([]ctrl.Request, len(nimServices.Items))
	for i, item := range nimServices.Items {
		requests[i] = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      item.Name,
				Namespace: item.Namespace,
			},
		}
	}
	return requests
}

func (r *NIMServiceReconciler) mapResourceClaimToNIMService(ctx context.Context, obj client.Object) []ctrl.Request {
	resourceClaim, ok := obj.(*resourcev1beta2.ResourceClaim)
	if !ok {
		return []ctrl.Request{}
	}

	// Get all NIMServices directly requesting resourceClaims.
	var nimServices appsv1alpha1.NIMServiceList
	if err := r.List(ctx, &nimServices, client.InNamespace(resourceClaim.GetNamespace())); err != nil {
		return []ctrl.Request{}
	}

	// Enqueue reconciliation for each matching NIMService
	requests := make([]ctrl.Request, 0)
	for _, item := range nimServices.Items {
		nameCache := make(map[string]int)
		for _, draResource := range item.Spec.DRAResources {
			if draResource.ResourceClaimName != nil && *draResource.ResourceClaimName == resourceClaim.GetName() {
				requests = append(requests, ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      item.Name,
						Namespace: item.Namespace,
					},
				})
				break
			}
			if draResource.ResourceClaimTemplateName != nil {
				draResourceName := shared.GenerateUniquePodClaimName(nameCache, item.Name, &draResource)
				podClaimName, ok := resourceClaim.GetAnnotations()[utils.DRAPodClaimNameAnnotationKey]
				if ok && podClaimName == draResourceName {
					requests = append(requests, ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      item.Name,
							Namespace: item.Namespace,
						},
					})
					break
				}
			}
		}
	}
	return requests
}

func (r *NIMServiceReconciler) refreshMetrics(ctx context.Context) {
	logger := log.FromContext(ctx)
	// List all nodes
	nimServiceList := &appsv1alpha1.NIMServiceList{}
	err := r.List(ctx, nimServiceList, &client.ListOptions{})
	if err != nil {
		logger.Error(err, "unable to list nimServices in the cluster")
		return
	}
	refreshNIMServiceMetrics(nimServiceList)
}
