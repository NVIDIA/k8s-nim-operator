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

package standalone

import (
	"context"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
	"github.com/NVIDIA/k8s-nim-operator/internal/render"
	"github.com/NVIDIA/k8s-nim-operator/internal/shared"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ManifestsDir is the directory to render k8s resource manifests
	ManifestsDir = "/manifests"
)

// Standalone implements the Platform interface for standalone deployment
type Standalone struct{}

// Define reconcilers for Standalone mode

// NIMCacheReconciler represents the NIMCache reconciler instance for standalone mode
type NIMCacheReconciler struct {
	client.Client
	scheme   *runtime.Scheme
	log      logr.Logger
	updater  conditions.Updater
	recorder record.EventRecorder
}

// NIMServiceReconciler represents the NIMService reconciler instance for standalone mode
type NIMServiceReconciler struct {
	client.Client
	scheme           *runtime.Scheme
	log              logr.Logger
	updater          conditions.Updater
	renderer         render.Renderer
	recorder         record.EventRecorder
	orchestratorType k8sutil.OrchestratorType
}

// NewNIMCacheReconciler returns NIMCacheReconciler for standalone mode
func NewNIMCacheReconciler(r shared.Reconciler) *NIMCacheReconciler {
	return &NIMCacheReconciler{
		Client:   r.GetClient(),
		scheme:   r.GetScheme(),
		log:      r.GetLogger(),
		updater:  r.GetUpdater(),
		recorder: r.GetEventRecorder(),
	}
}

// NewNIMServiceReconciler returns NIMServiceReconciler for standalone mode
func NewNIMServiceReconciler(r shared.Reconciler) *NIMServiceReconciler {
	orchestratorType, _ := r.GetOrchestratorType()

	return &NIMServiceReconciler{
		Client:           r.GetClient(),
		scheme:           r.GetScheme(),
		log:              r.GetLogger(),
		updater:          r.GetUpdater(),
		recorder:         r.GetEventRecorder(),
		orchestratorType: orchestratorType,
	}
}

// Delete handles cleanup of resources created for standlone Standalone caching
func (s *Standalone) Delete(ctx context.Context, r shared.Reconciler, resource client.Object) error {
	logger := r.GetLogger()

	if nimService, ok := resource.(*appsv1alpha1.NIMService); ok {
		reconciler := NewNIMServiceReconciler(r)
		err := reconciler.cleanupNIMService(ctx, nimService)
		if err != nil {
			logger.Error(err, "failed to cleanup resources", "name", nimService.Name)
			return err
		}
		return nil
	}
	return errors.NewBadRequest("invalid resource type")
}

// Sync handles reconciliation for standalone Standalone caching
func (s *Standalone) Sync(ctx context.Context, r shared.Reconciler, resource client.Object) (ctrl.Result, error) {
	logger := r.GetLogger()

	if nimService, ok := resource.(*appsv1alpha1.NIMService); ok {
		reconciler := NewNIMServiceReconciler(r)
		reconciler.renderer = render.NewRenderer(ManifestsDir)
		logger.Info("Reconciling NIMService instance", "nimservice", nimService.GetName())
		result, err := reconciler.reconcileNIMService(ctx, nimService)
		if err != nil {
			r.GetEventRecorder().Eventf(nimService, corev1.EventTypeWarning, "ReconcileFailed",
				"NIMService %s failed, msg: %s", nimService.Name, err.Error())
			errConditionUpdate := reconciler.updater.SetConditionsFailed(ctx, nimService, conditions.Failed, err.Error())
			if errConditionUpdate != nil {
				logger.Error(err, "Unable to update status")
				return result, errConditionUpdate
			}
		}
		return result, err
	}
	return ctrl.Result{}, errors.NewBadRequest("invalid resource type")
}
