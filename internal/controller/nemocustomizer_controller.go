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
	"encoding/base64"
	"fmt"
	"reflect"

	goerrors "errors"

	"github.com/go-logr/logr"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/yaml"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
	"github.com/NVIDIA/k8s-nim-operator/internal/render"
	"github.com/NVIDIA/k8s-nim-operator/internal/shared"
	"github.com/NVIDIA/k8s-nim-operator/internal/utils"
)

const (
	// NemoCustomizerFinalizer is the finalizer annotation.
	NemoCustomizerFinalizer = "finalizer.nemocustomizer.apps.nvidia.com"
	// ConfigHashAnnotationKey is the annotation key for storing hash of the training configuration.
	ConfigHashAnnotationKey = "apps.nvidia.com/training-config-hash"
)

// NemoCustomizerReconciler reconciles a NemoCustomizer object.
type NemoCustomizerReconciler struct {
	client.Client
	scheme           *runtime.Scheme
	log              logr.Logger
	updater          conditions.Updater
	renderer         render.Renderer
	Config           *rest.Config
	recorder         record.EventRecorder
	orchestratorType k8sutil.OrchestratorType
}

// Ensure NemoCustomizerReconciler implements the Reconciler interface.
var _ shared.Reconciler = &NemoCustomizerReconciler{}

// NewNemoCustomizerReconciler creates a new reconciler for NemoCustomizer with the given platform.
func NewNemoCustomizerReconciler(client client.Client, scheme *runtime.Scheme, updater conditions.Updater, renderer render.Renderer, log logr.Logger) *NemoCustomizerReconciler {
	return &NemoCustomizerReconciler{
		Client:   client,
		scheme:   scheme,
		updater:  updater,
		renderer: renderer,
		log:      log,
	}
}

// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemocustomizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemocustomizers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.nvidia.com,resources=nemocustomizers/finalizers,verbs=update
// +kubebuilder:rbac:groups=run.ai,resources=trainingworkloads;runaijobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=nvidia.com,resources=nemotrainingjobs;nemotrainingjobs/status;nemoentityhandlers,verbs=create;get;list;watch;update;delete;patch
// +kubebuilder:rbac:groups=batch.volcano.sh,resources=jobs;jobs/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=nodeinfo.volcano.sh,resources=numatopologies,verbs=get;list;watch
// +kubebuilder:rbac:groups=scheduling.incubator.k8s.io;scheduling.volcano.sh,resources=queues;queues/status;podgroups,verbs=get;list;watch
// +kubebuilder:rbac:groups=config.openshift.io,resources=clusterversions;proxies,verbs=get;list;watch
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=use,resourceNames=nonroot
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts;pods;pods/eviction;services;services/finalizers;endpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims;configmaps;secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors;prometheusrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scheduling.k8s.io,resources=priorityclasses,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalars,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;update;patch
// +kubebuilder:rbac:groups="nodeinfo.volcano.sh",resources=numatopologies,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NemoCustomizer object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *NemoCustomizerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	defer r.refreshMetrics(ctx)

	// Fetch the NemoCustomizer instance
	NemoCustomizer := &appsv1alpha1.NemoCustomizer{}
	if err := r.Get(ctx, req.NamespacedName, NemoCustomizer); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "unable to fetch NemoCustomizer", "name", req.NamespacedName)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling", "NemoCustomizer", NemoCustomizer.Name)

	// Check if the instance is marked for deletion
	if NemoCustomizer.DeletionTimestamp.IsZero() {
		// Add finalizer if not present
		if !controllerutil.ContainsFinalizer(NemoCustomizer, NemoCustomizerFinalizer) {
			controllerutil.AddFinalizer(NemoCustomizer, NemoCustomizerFinalizer)
			if err := r.Update(ctx, NemoCustomizer); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The instance is being deleted
		if controllerutil.ContainsFinalizer(NemoCustomizer, NemoCustomizerFinalizer) {
			// Perform platform specific cleanup of resources
			if err := r.cleanupNemoCustomizer(ctx, NemoCustomizer); err != nil {
				r.GetEventRecorder().Eventf(NemoCustomizer, corev1.EventTypeNormal, "Delete",
					"NemoCustomizer %s in deleted", NemoCustomizer.Name)
				return ctrl.Result{}, err
			}

			// Remove finalizer to allow for deletion
			controllerutil.RemoveFinalizer(NemoCustomizer, NemoCustomizerFinalizer)
			if err := r.Update(ctx, NemoCustomizer); err != nil {
				r.GetEventRecorder().Eventf(NemoCustomizer, corev1.EventTypeNormal, "Delete",
					"NemoCustomizer %s finalizer removed", NemoCustomizer.Name)
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
	if result, err := r.reconcileNemoCustomizer(ctx, NemoCustomizer); err != nil {
		logger.Error(err, "error reconciling NemoCustomizer", "name", NemoCustomizer.Name)
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *NemoCustomizerReconciler) cleanupNemoCustomizer(ctx context.Context, nemoCustomizer *appsv1alpha1.NemoCustomizer) error {
	// All dependent (owned) objects will be automatically garbage collected.
	// TODO: Handle any custom cleanup logic for the NIM microservice
	return nil
}

// GetScheme returns the scheme of the reconciler.
func (r *NemoCustomizerReconciler) GetScheme() *runtime.Scheme {
	return r.scheme
}

// GetLogger returns the logger of the reconciler.
func (r *NemoCustomizerReconciler) GetLogger() logr.Logger {
	return r.log
}

// GetClient returns the client instance.
func (r *NemoCustomizerReconciler) GetClient() client.Client {
	return r.Client
}

// GetConfig returns the rest config.
func (r *NemoCustomizerReconciler) GetConfig() *rest.Config {
	return r.Config
}

// GetUpdater returns the conditions updater instance.
func (r *NemoCustomizerReconciler) GetUpdater() conditions.Updater {
	return r.updater
}

// GetDiscoveryClient returns the discovery client instance.
func (r *NemoCustomizerReconciler) GetDiscoveryClient() discovery.DiscoveryInterface {
	return nil
}

// GetRenderer returns the renderer instance.
func (r *NemoCustomizerReconciler) GetRenderer() render.Renderer {
	return r.renderer
}

// GetEventRecorder returns the event recorder.
func (r *NemoCustomizerReconciler) GetEventRecorder() record.EventRecorder {
	return r.recorder
}

// GetOrchestratorType returns the container platform type.
func (r *NemoCustomizerReconciler) GetOrchestratorType(ctx context.Context) (k8sutil.OrchestratorType, error) {
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
func (r *NemoCustomizerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor("nemo-customizer-service-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.NemoCustomizer{}).
		Owns(&appsv1.Deployment{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&networkingv1.Ingress{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		Owns(&corev1.ConfigMap{}).
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Type assert to NemoCustomizer
				if oldNemoCustomizer, ok := e.ObjectOld.(*appsv1alpha1.NemoCustomizer); ok {
					newNemoCustomizer, ok := e.ObjectNew.(*appsv1alpha1.NemoCustomizer)
					if ok {
						// Handle case where object is marked for deletion
						if !newNemoCustomizer.ObjectMeta.DeletionTimestamp.IsZero() {
							return true
						}

						// Handle only spec updates
						return !reflect.DeepEqual(oldNemoCustomizer.Spec, newNemoCustomizer.Spec)
					}
				}
				// For other types we watch, reconcile them
				return true
			},
		}).
		Complete(r)
}

func (r *NemoCustomizerReconciler) refreshMetrics(ctx context.Context) {
	logger := log.FromContext(ctx)
	// List all customizer instances
	NemoCustomizerList := &appsv1alpha1.NemoCustomizerList{}
	err := r.List(ctx, NemoCustomizerList, &client.ListOptions{})
	if err != nil {
		logger.Error(err, "unable to list NemoCustomizers in the cluster")
		return
	}
	refreshNemoCustomizerMetrics(NemoCustomizerList)
}

func (r *NemoCustomizerReconciler) reconcileNemoCustomizer(ctx context.Context, nemoCustomizer *appsv1alpha1.NemoCustomizer) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var err error
	defer func() {
		if err != nil {
			r.GetEventRecorder().Eventf(nemoCustomizer, corev1.EventTypeWarning, conditions.Failed,
				"NemoCustomizer %s failed, msg: %s", nemoCustomizer.Name, err.Error())
		}
	}()
	// Generate annotation for the current operator-version and apply to all resources
	// Get generic name for all resources
	namespacedName := types.NamespacedName{Name: nemoCustomizer.GetName(), Namespace: nemoCustomizer.GetNamespace()}
	renderer := r.GetRenderer()

	// Sync serviceaccount
	err = r.renderAndSyncResource(ctx, nemoCustomizer, &renderer, &corev1.ServiceAccount{}, func() (client.Object, error) {
		return renderer.ServiceAccount(nemoCustomizer.GetServiceAccountParams())
	}, "serviceaccount", conditions.ReasonServiceAccountFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync role
	err = r.renderAndSyncResource(ctx, nemoCustomizer, &renderer, &rbacv1.Role{}, func() (client.Object, error) {
		return renderer.Role(nemoCustomizer.GetRoleParams())
	}, "role", conditions.ReasonRoleFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync rolebinding
	err = r.renderAndSyncResource(ctx, nemoCustomizer, &renderer, &rbacv1.RoleBinding{}, func() (client.Object, error) {
		return renderer.RoleBinding(nemoCustomizer.GetRoleBindingParams())
	}, "rolebinding", conditions.ReasonRoleBindingFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync service
	err = r.renderAndSyncResource(ctx, nemoCustomizer, &renderer, &corev1.Service{}, func() (client.Object, error) {
		return renderer.Service(nemoCustomizer.GetServiceParams())
	}, "service", conditions.ReasonServiceFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync ingress
	if nemoCustomizer.IsIngressEnabled() {
		err = r.renderAndSyncResource(ctx, nemoCustomizer, &renderer, &networkingv1.Ingress{}, func() (client.Object, error) {
			return renderer.Ingress(nemoCustomizer.GetIngressParams())
		}, "ingress", conditions.ReasonIngressFailed)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		err = k8sutil.CleanupResource(ctx, r.GetClient(), &networkingv1.Ingress{}, namespacedName)
		if err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	// Sync HPA
	if nemoCustomizer.IsAutoScalingEnabled() {
		err = r.renderAndSyncResource(ctx, nemoCustomizer, &renderer, &autoscalingv2.HorizontalPodAutoscaler{}, func() (client.Object, error) {
			return renderer.HPA(nemoCustomizer.GetHPAParams())
		}, "hpa", conditions.ReasonHPAFailed)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		// If autoscaling is disabled, ensure the HPA is deleted
		err = k8sutil.CleanupResource(ctx, r.GetClient(), &autoscalingv2.HorizontalPodAutoscaler{}, namespacedName)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Sync Service Monitor
	if nemoCustomizer.IsServiceMonitorEnabled() {
		err = r.renderAndSyncResource(ctx, nemoCustomizer, &renderer, &monitoringv1.ServiceMonitor{}, func() (client.Object, error) {
			return renderer.ServiceMonitor(nemoCustomizer.GetServiceMonitorParams())
		}, "servicemonitor", conditions.ReasonServiceMonitorFailed)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Sync Customizer ConfigMap
	customizerConfigYAML, err := r.renderCustomizerConfig(ctx, nemoCustomizer)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("rendering customizer config: %w", err)
	}

	err = r.renderAndSyncResource(ctx, nemoCustomizer, &renderer, &corev1.ConfigMap{}, func() (client.Object, error) {
		return renderer.ConfigMap(nemoCustomizer.GetConfigMapParams(customizerConfigYAML))
	}, "configmap", conditions.ReasonConfigMapFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	secretValue, err := r.getValueFromSecret(ctx, nemoCustomizer.GetNamespace(), nemoCustomizer.Spec.DatabaseConfig.Credentials.SecretName, nemoCustomizer.Spec.DatabaseConfig.Credentials.PasswordKey)
	if err != nil {
		return ctrl.Result{}, err
	}
	data := nemoCustomizer.GeneratePostgresConnString(secretValue)
	// Encode to base64
	encoded := base64.StdEncoding.EncodeToString([]byte(data))

	secretMapData := map[string]string{
		"dsn": encoded,
	}
	// Sync Customizer Secret
	err = r.renderAndSyncResource(ctx, nemoCustomizer, &renderer, &corev1.Secret{}, func() (client.Object, error) {
		return renderer.Secret(nemoCustomizer.GetSecretParams(secretMapData))
	}, "secret", conditions.ReasonSecretFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sync Customizer PVC for model storage
	if err = r.reconcilePVC(ctx, nemoCustomizer); err != nil {
		return ctrl.Result{}, err
	}

	// Get params to render Deployment resource
	deploymentParams := nemoCustomizer.GetDeploymentParams()

	// Calculate the hash of the config data
	configHash := utils.CalculateSHA256(string(customizerConfigYAML))
	annotations := deploymentParams.PodAnnotations
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// Check if the hash has changed
	if annotations[ConfigHashAnnotationKey] != configHash {
		annotations[ConfigHashAnnotationKey] = configHash
		deploymentParams.PodAnnotations = annotations
	}

	// Setup volume mounts with customizer config
	deploymentParams.Volumes = nemoCustomizer.GetVolumes()
	deploymentParams.VolumeMounts = nemoCustomizer.GetVolumeMounts()

	// Sync deployment
	err = r.renderAndSyncResource(ctx, nemoCustomizer, &renderer, &appsv1.Deployment{}, func() (client.Object, error) {
		return renderer.Deployment(deploymentParams)
	}, "deployment", conditions.ReasonDeploymentFailed)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Wait for deployment
	msg, ready, err := k8sutil.IsDeploymentReady(ctx, r.GetClient(), &namespacedName)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !ready {
		// Update status as NotReady
		err = r.updater.SetConditionsNotReady(ctx, nemoCustomizer, conditions.NotReady, msg)
		r.GetEventRecorder().Eventf(nemoCustomizer, corev1.EventTypeNormal, conditions.NotReady,
			"NemoCustomizer %s not ready yet, msg: %s", nemoCustomizer.Name, msg)
	} else {
		// Update status as ready
		err = r.updater.SetConditionsReady(ctx, nemoCustomizer, conditions.Ready, msg)
		r.GetEventRecorder().Eventf(nemoCustomizer, corev1.EventTypeNormal, conditions.Ready,
			"NemoCustomizer %s ready, msg: %s", nemoCustomizer.Name, msg)
	}

	if err != nil {
		logger.Error(err, "Unable to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NemoCustomizerReconciler) reconcilePVC(ctx context.Context, nemoCustomizer *appsv1alpha1.NemoCustomizer) error {
	logger := r.GetLogger()
	pvcName := shared.GetPVCName(nemoCustomizer, nemoCustomizer.Spec.Training.ModelPVC)
	pvcNamespacedName := types.NamespacedName{Name: pvcName, Namespace: nemoCustomizer.GetNamespace()}
	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, pvcNamespacedName, pvc)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return err
	}

	// If PVC does not exist, create a new one if creation flag is enabled
	if err != nil {
		if nemoCustomizer.Spec.Training.ModelPVC.Create != nil && *nemoCustomizer.Spec.Training.ModelPVC.Create {
			pvc, err = shared.ConstructPVC(nemoCustomizer.Spec.Training.ModelPVC, metav1.ObjectMeta{Name: pvcName, Namespace: nemoCustomizer.GetNamespace()})
			if err != nil {
				logger.Error(err, "Failed to construct pvc", "name", pvcName)
				return err
			}
			if err := controllerutil.SetControllerReference(nemoCustomizer, pvc, r.GetScheme()); err != nil {
				return err
			}
			err = r.Create(ctx, pvc)
			if err != nil {
				logger.Error(err, "Failed to create pvc", "name", pvcName)
				return err
			}
			logger.Info("Created PVC for NeMo Customizer model storage", "pvc", pvc.Name)
		} else {
			logger.Error(err, "PVC doesn't exist and auto-creation is not enabled", "name", pvcNamespacedName)
			return err
		}
	}
	return nil
}

func (r *NemoCustomizerReconciler) renderCustomizerConfig(ctx context.Context, n *appsv1alpha1.NemoCustomizer) ([]byte, error) {
	cfg := make(map[string]interface{})

	r.addBaseConfig(ctx, cfg, n)
	r.addWandBConfig(ctx, cfg, n)

	if err := r.addModelDownloadJobsConfig(ctx, cfg, n); err != nil {
		return nil, err
	}

	if err := r.addDatastoreToolsConfig(ctx, cfg, n); err != nil {
		return nil, err
	}

	if err := r.addTrainingConfig(ctx, cfg, n); err != nil {
		return nil, err
	}

	if err := r.addModelConfig(ctx, n, cfg); err != nil {
		return nil, err
	}

	if err := r.addCustomizationTargetConfig(ctx, n, cfg); err != nil {
		return nil, err
	}

	if err := r.addCustomizationConfigTemplates(ctx, n, cfg); err != nil {
		return nil, err
	}

	return yaml.Marshal(cfg)
}

func (r *NemoCustomizerReconciler) addBaseConfig(ctx context.Context, cfg map[string]interface{}, n *appsv1alpha1.NemoCustomizer) {
	cfg["namespace"] = n.GetNamespace()
	cfg["entity_store_url"] = n.Spec.Entitystore.Endpoint
	cfg["nemo_data_store_url"] = n.Spec.Datastore.Endpoint
	cfg["mlflow_tracking_url"] = n.Spec.MLFlow.Endpoint
}

func (r *NemoCustomizerReconciler) addDatastoreToolsConfig(ctx context.Context, cfg map[string]interface{}, n *appsv1alpha1.NemoCustomizer) error {
	if n.Spec.NemoDatastoreTools == nil {
		return fmt.Errorf("NeMo datastore tools image configuration is not set")
	}

	cfg["nemo_data_store_tools"] = map[string]interface{}{
		"image": n.Spec.NemoDatastoreTools.Image,
	}

	if len(n.Spec.Image.PullSecrets) > 0 {
		if tools, ok := cfg["nemo_data_store_tools"].(map[string]interface{}); ok {
			tools["imagePullSecret"] = n.Spec.Image.PullSecrets[0]
		}
	}
	return nil
}

func (r *NemoCustomizerReconciler) addModelDownloadJobsConfig(ctx context.Context, cfg map[string]interface{}, n *appsv1alpha1.NemoCustomizer) error {
	if n.Spec.ModelDownloadJobs == nil {
		return fmt.Errorf("NeMo configuration for model download jobs is not set")
	}

	// use same pull secrets as the main customizer api image
	pullSecrets := []map[string]string{}
	for _, secret := range n.Spec.Image.PullSecrets {
		pullSecrets = append(pullSecrets, map[string]string{"name": secret})
	}

	modelDownloadJobsCfg := map[string]interface{}{
		"image":                   n.Spec.ModelDownloadJobs.Image,
		"imagePullPolicy":         n.Spec.Image.PullPolicy,
		"imagePullSecrets":        pullSecrets,
		"securityContext":         n.Spec.ModelDownloadJobs.SecurityContext,
		"ttlSecondsAfterFinished": n.Spec.ModelDownloadJobs.TTLSecondsAfterFinished,
		"pollIntervalSeconds":     n.Spec.ModelDownloadJobs.PollIntervalSeconds,
	}

	// add NGC secret if present
	if n.Spec.ModelDownloadJobs.NGCSecret != nil {
		modelDownloadJobsCfg["ngcAPISecret"] = n.Spec.ModelDownloadJobs.NGCSecret.Name
		modelDownloadJobsCfg["ngcAPISecretKey"] = n.Spec.ModelDownloadJobs.NGCSecret.Key
		modelDownloadJobsCfg["ngcSecretName"] = n.Spec.ModelDownloadJobs.NGCSecret.Name
		modelDownloadJobsCfg["ngcSecretKey"] = n.Spec.ModelDownloadJobs.NGCSecret.Key
	}

	// add HF secret if present
	if n.Spec.ModelDownloadJobs.HFSecret != nil {
		modelDownloadJobsCfg["hfSecretName"] = n.Spec.ModelDownloadJobs.HFSecret.Name
		modelDownloadJobsCfg["hfSecretKey"] = n.Spec.ModelDownloadJobs.HFSecret.Key
	}

	cfg["model_download_jobs"] = modelDownloadJobsCfg

	return nil
}

func (r *NemoCustomizerReconciler) addWorkspacePVCConfig(ctx context.Context, cfg map[string]interface{}, n *appsv1alpha1.NemoCustomizer) {
	cfg["pvc"] = map[string]string{
		"storageClass":     n.Spec.Training.WorkspacePVC.StorageClass,
		"volumeAccessMode": string(n.Spec.Training.WorkspacePVC.VolumeAccessMode),
		"size":             n.Spec.Training.WorkspacePVC.Size,
	}
}

func (r *NemoCustomizerReconciler) addWandBConfig(ctx context.Context, cfg map[string]interface{}, n *appsv1alpha1.NemoCustomizer) {
	cfg["wandb"] = map[string]string{
		"project": n.Spec.WandBConfig.Project,
		"entity":  "null",
	}

	if n.Spec.WandBConfig.Entity != nil {
		if wandb, ok := cfg["wandb"].(map[string]interface{}); ok {
			wandb["entity"] = *n.Spec.WandBConfig.Entity
		}
	}
}

func (r *NemoCustomizerReconciler) addTrainingConfig(ctx context.Context, cfg map[string]interface{}, n *appsv1alpha1.NemoCustomizer) error {
	if n.Spec.Training == nil {
		return fmt.Errorf("NeMo training configuration is not set")
	}

	trainingCfg := make(map[string]interface{})
	if cmName := n.Spec.Training.ConfigMap.Name; cmName != "" {
		trainingRaw, err := k8sutil.GetRawYAMLFromConfigMap(ctx, r.GetClient(), n.GetNamespace(), cmName, "training")
		if err != nil {
			return fmt.Errorf("loading training config: %w", err)
		}
		if err := yaml.Unmarshal([]byte(trainingRaw), &trainingCfg); err != nil {
			return fmt.Errorf("parsing training config: %w", err)
		}
	}

	// use same pull secrets as the main customizer api image
	pullSecrets := []map[string]string{}
	for _, secret := range n.Spec.Image.PullSecrets {
		pullSecrets = append(pullSecrets, map[string]string{"name": secret})
	}

	trainingCfg["image"] = fmt.Sprintf("%s:%s", n.Spec.Training.Image.Repository, n.Spec.Training.Image.Tag)
	trainingCfg["imagePullPolicy"] = n.Spec.Image.PullPolicy
	trainingCfg["imagePullSecrets"] = pullSecrets
	trainingCfg["env"] = n.Spec.Training.Env
	trainingCfg["training_networking"] = n.Spec.Training.NetworkConfig
	trainingCfg["workspace_dir"] = n.Spec.Training.WorkspacePVC.MountPath
	trainingCfg["use_run_ai_executor"] = false

	if n.Spec.Scheduler.Type == appsv1alpha1.SchedulerTypeRunAI {
		trainingCfg["use_run_ai_executor"] = true
	}
	if n.Spec.Training.TTLSecondsAfterFinished != nil {
		trainingCfg["ttl_seconds_after_finished"] = *n.Spec.Training.TTLSecondsAfterFinished
	}
	if n.Spec.Training.Timeout != nil {
		trainingCfg["training_timeout"] = n.Spec.Training.Timeout
	}

	// Add PVC configuration
	r.addWorkspacePVCConfig(ctx, trainingCfg, n)

	emptyDir := map[string]interface{}{
		"medium": "Memory",
	}
	if n.Spec.Training.SharedMemorySizeLimit != nil {
		emptyDir["sizeLimit"] = n.Spec.Training.SharedMemorySizeLimit.String()
	}
	trainingCfg["volumes"] = []map[string]interface{}{
		{
			"name": "models",
			"persistentVolumeClaim": map[string]interface{}{
				"claimName": shared.GetPVCName(n, n.Spec.Training.ModelPVC),
				"readOnly":  true,
			},
		},
		{
			"name":     "dshm",
			"emptyDir": emptyDir,
		},
	}

	trainingCfg["volumeMounts"] = []map[string]interface{}{
		{
			"name":      "models",
			"mountPath": "/mount/models",
			"readOnly":  true,
		},
		{
			"name":      "dshm",
			"mountPath": "/dev/shm",
		},
	}

	cfg["training"] = trainingCfg

	// Add additional training pod spec fields
	if len(n.Spec.Training.Tolerations) > 0 {
		trainingCfg["tolerations"] = n.Spec.Training.Tolerations
	}
	if len(n.Spec.Training.NodeSelector) > 0 {
		trainingCfg["nodeSelector"] = n.Spec.Training.NodeSelector
	}
	if n.Spec.Training.PodAffinity != nil {
		trainingCfg["affinity"] = n.Spec.Training.PodAffinity
	}
	if n.Spec.Training.Resources != nil {
		trainingCfg["resources"] = n.Spec.Training.Resources
	}

	return nil
}

func (r *NemoCustomizerReconciler) addModelConfig(ctx context.Context, n *appsv1alpha1.NemoCustomizer, cfg map[string]interface{}) error {
	modelsRaw, err := k8sutil.GetRawYAMLFromConfigMap(ctx, r.GetClient(), n.GetNamespace(), n.Spec.Models.Name, "models")
	if err != nil {
		if goerrors.Is(err, k8sutil.ErrConfigMapKeyNotFound) {
			// Ignore missing models key
			return nil
		}
		return fmt.Errorf("loading models config: %w", err)
	}

	var models map[string]interface{}
	if err := yaml.Unmarshal([]byte(modelsRaw), &models); err != nil {
		return fmt.Errorf("parsing models config: %w", err)
	}

	cfg["models"] = models
	return nil
}

func (r *NemoCustomizerReconciler) addCustomizationTargetConfig(ctx context.Context, n *appsv1alpha1.NemoCustomizer, cfg map[string]interface{}) error {
	customizationTargetsRaw, err := k8sutil.GetRawYAMLFromConfigMap(ctx, r.GetClient(), n.GetNamespace(), n.Spec.Models.Name, "customizationTargets")
	if err != nil {
		if goerrors.Is(err, k8sutil.ErrConfigMapKeyNotFound) {
			// Ignore missing customizationTargetConfig key
			return nil
		}
		return fmt.Errorf("loading models config: %w", err)
	}

	var customizationTargets map[string]interface{}
	if err := yaml.Unmarshal([]byte(customizationTargetsRaw), &customizationTargets); err != nil {
		return fmt.Errorf("parsing customization targets config: %w", err)
	}

	cfg["customizationTargets"] = customizationTargets
	return nil
}

func (r *NemoCustomizerReconciler) addCustomizationConfigTemplates(ctx context.Context, n *appsv1alpha1.NemoCustomizer, cfg map[string]interface{}) error {
	customizationConfigTemplatesRaw, err := k8sutil.GetRawYAMLFromConfigMap(ctx, r.GetClient(), n.GetNamespace(), n.Spec.Models.Name, "customizationConfigTemplates")
	if err != nil {
		if goerrors.Is(err, k8sutil.ErrConfigMapKeyNotFound) {
			// Ignore missing customizationConfigTemplates key
			return nil
		}
		return fmt.Errorf("loading models config: %w", err)
	}

	var customizationConfigTemplates map[string]interface{}
	if err := yaml.Unmarshal([]byte(customizationConfigTemplatesRaw), &customizationConfigTemplates); err != nil {
		return fmt.Errorf("parsing customization config templates config: %w", err)
	}

	cfg["customizationConfigTemplates"] = customizationConfigTemplates
	return nil
}

func (r *NemoCustomizerReconciler) renderAndSyncResource(ctx context.Context, nemoCustomizer *appsv1alpha1.NemoCustomizer, renderer *render.Renderer, obj client.Object, renderFunc func() (client.Object, error), conditionType string, reason string) error {
	logger := log.FromContext(ctx)

	namespacedName := types.NamespacedName{Name: nemoCustomizer.GetName(), Namespace: nemoCustomizer.GetNamespace()}
	err := r.Get(ctx, namespacedName, obj)
	if err != nil && !errors.IsNotFound(err) {
		logger.Error(err, fmt.Sprintf("Error is not NotFound for %s: %v", obj.GetObjectKind(), err))
		return err
	}
	// Don't do anything if CR is unchanged.
	if err == nil && !utils.IsParentSpecChanged(obj, utils.DeepHashObject(nemoCustomizer.Spec)) {
		return nil
	}

	resource, err := renderFunc()
	if err != nil {
		logger.Error(err, "failed to render", conditionType, nemoCustomizer.GetName(), nemoCustomizer.GetNamespace())
		statusError := r.updater.SetConditionsFailed(ctx, nemoCustomizer, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NemoCustomizer", nemoCustomizer.GetName())
		}
		return err
	}

	// Check if the resource is nil
	if resource == nil {
		logger.V(2).Info("rendered nil resource")
		return nil
	}

	metaAccessor, ok := resource.(metav1.Object)
	if !ok {
		logger.V(2).Info("rendered un-initialized resource")
		return nil
	}

	if metaAccessor == nil || metaAccessor.GetName() == "" || metaAccessor.GetNamespace() == "" {
		logger.V(2).Info("rendered un-initialized resource")
		return nil
	}

	if err = controllerutil.SetControllerReference(nemoCustomizer, resource, r.GetScheme()); err != nil {
		logger.Error(err, "failed to set owner", conditionType, namespacedName)
		statusError := r.updater.SetConditionsFailed(ctx, nemoCustomizer, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NemoCustomizer", nemoCustomizer.GetName())
		}
		return err
	}

	err = k8sutil.SyncResource(ctx, r.GetClient(), obj, resource)
	if err != nil {
		logger.Error(err, "failed to sync", conditionType, namespacedName)
		statusError := r.updater.SetConditionsFailed(ctx, nemoCustomizer, reason, err.Error())
		if statusError != nil {
			logger.Error(statusError, "failed to update status", "NemoCustomizer", nemoCustomizer.GetName())
		}
		return err
	}
	return nil
}

func (r *NemoCustomizerReconciler) getValueFromSecret(ctx context.Context, namespace, secretName, key string) (string, error) {

	// Get the secret
	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: secretName, Namespace: namespace}, secret)
	if err != nil {
		return "", fmt.Errorf("failed to get secret: %w", err)
	}

	// Extract the secret value
	value, exists := secret.Data[key]

	if !exists {
		return "", fmt.Errorf("key '%s' not found in secret '%s'", key, secretName)
	}
	return string(value), nil
}
