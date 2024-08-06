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

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/shared"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// KServe implements the Platform interface for KServe
type KServe struct{}

// Define reconcilers for KServe platform

// NIMCacheReconciler represents the NIMCache reconciler instance for KServe platform
type NIMCacheReconciler struct {
	shared.Reconciler
	client.Client
	scheme *runtime.Scheme
	log    logr.Logger
}

// NIMServiceReconciler represents the NIMService reconciler instance for KServe platform
type NIMServiceReconciler struct {
	client.Client
	scheme *runtime.Scheme
	log    logr.Logger
}

// NewNIMCacheReconciler returns NIMCacheReconciler for KServe platform
func NewNIMCacheReconciler(r shared.Reconciler) *NIMCacheReconciler {
	return &NIMCacheReconciler{
		Client: r.GetClient(),
		scheme: r.GetScheme(),
		log:    r.GetLogger(),
	}
}

// NewNIMServiceReconciler returns NIMServiceReconciler for KServe platform
func NewNIMServiceReconciler(r shared.Reconciler) *NIMServiceReconciler {
	return &NIMServiceReconciler{
		Client: r.GetClient(),
		scheme: r.GetScheme(),
		log:    r.GetLogger(),
	}
}

// Delete handles cleanup of resources created for NIM caching
func (k *KServe) Delete(ctx context.Context, r shared.Reconciler, resource client.Object) error {
	logger := r.GetLogger()

	if nimService, ok := resource.(*appsv1alpha1.NIMService); ok {
		reconciler := NewNIMServiceReconciler(r)
		err := reconciler.cleanupNIMService(ctx, nimService)
		if err != nil {
			logger.Error(err, "failed to cleanup nimservice resources", "name", nimService.Name)
			return err
		}
		return nil
	}
	return errors.NewBadRequest("invalid resource type")

}

// Sync handles reconciliation of Kserve resources
func (k *KServe) Sync(ctx context.Context, r shared.Reconciler, resource client.Object) (ctrl.Result, error) {
	// TODO: add reconciliation logic specific to Kserve modelcache
	return ctrl.Result{}, nil
}
