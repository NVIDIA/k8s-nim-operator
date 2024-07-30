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

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/v1alpha1"
)

func (r *NIMCacheReconciler) cleanupNIMCache(ctx context.Context, nimCache *appsv1alpha1.NIMCache) error {
	// TODO: add cleanup logic specific to Kserve modelcache
	return nil
}
