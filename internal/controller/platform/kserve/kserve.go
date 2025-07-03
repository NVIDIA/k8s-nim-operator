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
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	"github.com/NVIDIA/k8s-nim-operator/internal/shared"
)

// KServe implements the Platform interface for KServe.
type KServe struct{}

// Delete handles cleanup of resources created for NIM caching.
func (k *KServe) Delete(ctx context.Context, r shared.Reconciler, resource client.Object) error {
	logger := r.GetLogger()

	nimService, ok := resource.(*appsv1alpha1.NIMService)
	if ok {
		reconciler := NewNIMServiceReconciler(ctx, r)
		err := reconciler.cleanupNIMService(ctx, nimService)
		if err != nil {
			logger.Error(err, "failed to cleanup nimservice resources", "name", nimService.Name)
			return err
		}
		return nil
	}
	return errors.NewBadRequest("invalid resource type")
}

// Sync handles reconciliation of Kserve resources.
func (s *KServe) Sync(ctx context.Context, r shared.Reconciler, resource client.Object) (ctrl.Result, error) {
	logger := r.GetLogger()

	nimService, ok := resource.(*appsv1alpha1.NIMService)
	if ok {
		reconciler := NewNIMServiceReconciler(ctx, r)

		logger.Info("Reconciling NIMService instance", "nimservice", nimService.GetName())
		result, err := reconciler.reconcileNIMService(ctx, nimService)

		if err != nil {
			if errors.IsConflict(err) {
				// Ignore conflict errors and retry.
				return ctrl.Result{Requeue: true}, nil
			}

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
