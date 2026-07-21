/*
Copyright 2026.

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

package imageprotocol

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

type sequenceResolver struct {
	protocols []Protocol
	index     int
}

func (r *sequenceResolver) Resolve(context.Context, string, string, []string) (Protocol, error) {
	protocol := r.protocols[r.index]
	if r.index < len(r.protocols)-1 {
		r.index++
	}
	return protocol, nil
}

func TestResolveModelLayout(t *testing.T) {
	service := &appsv1alpha1.NIMService{
		ObjectMeta: metav1.ObjectMeta{Name: "retriever", Namespace: "models"},
		Spec: appsv1alpha1.NIMServiceSpec{
			Image: appsv1alpha1.Image{Repository: "nvcr.io/nim/retriever", Tag: "2.0"},
		},
	}
	cache := &appsv1alpha1.NIMCache{
		ObjectMeta: metav1.ObjectMeta{Name: "nemotron-page-elements-v3", Namespace: "models"},
		Spec: appsv1alpha1.NIMCacheSpec{
			Source: appsv1alpha1.NIMSource{NGC: &appsv1alpha1.NGCSource{ModelPuller: service.GetImage()}},
			Storage: appsv1alpha1.NIMCacheStorage{
				PVC: appsv1alpha1.PersistentVolumeClaim{Create: ptr.To(false), Name: "shared-models"},
			},
		},
	}

	t.Run("native shared cache uses a cache-specific directory", func(t *testing.T) {
		layout, err := ResolveModelLayout(context.Background(), &sequenceResolver{protocols: []Protocol{NativeV1, NativeV1}}, service, cache)
		if err != nil {
			t.Fatal(err)
		}
		if layout.MountPath != NativeModelMountPath || layout.ModelPath != "/model/nemotron-page-elements-v3" {
			t.Fatalf("layout = %#v, want native shared-cache layout", layout)
		}
	})

	t.Run("native operator-created cache uses the model root", func(t *testing.T) {
		dedicated := cache.DeepCopy()
		dedicated.Spec.Storage.PVC.Create = ptr.To(true)
		layout, err := ResolveModelLayout(context.Background(), &sequenceResolver{protocols: []Protocol{NativeV1, NativeV1}}, service, dedicated)
		if err != nil {
			t.Fatal(err)
		}
		if layout.ModelPath != NativeModelMountPath {
			t.Fatalf("ModelPath = %q, want %q", layout.ModelPath, NativeModelMountPath)
		}
	})

	t.Run("service model path overrides cache default", func(t *testing.T) {
		overridden := service.DeepCopy()
		overridden.Spec.Env = []corev1.EnvVar{{Name: ModelPathEnv, Value: "/model/custom"}}
		layout, err := ResolveModelLayout(context.Background(), &sequenceResolver{protocols: []Protocol{NativeV1, NativeV1}}, overridden, cache)
		if err != nil {
			t.Fatal(err)
		}
		if layout.ModelPath != "/model/custom" {
			t.Fatalf("ModelPath = %q, want service override", layout.ModelPath)
		}
	})

	t.Run("cache and serving protocol mismatch fails", func(t *testing.T) {
		_, err := ResolveModelLayout(context.Background(), &sequenceResolver{protocols: []Protocol{NativeV1, Legacy}}, service, cache)
		if err == nil || !strings.Contains(err.Error(), "model download protocol") || !strings.Contains(err.Error(), "does not match") {
			t.Fatalf("ResolveModelLayout() error = %v, want clear mismatch", err)
		}
	})

	t.Run("native serving image rejects a legacy Hugging Face cache", func(t *testing.T) {
		hfCache := cache.DeepCopy()
		hfCache.Spec.Source = appsv1alpha1.NIMSource{HF: &appsv1alpha1.HuggingFaceHubSource{}}
		_, err := ResolveModelLayout(context.Background(), &sequenceResolver{protocols: []Protocol{NativeV1}}, service, hfCache)
		if err == nil || !strings.Contains(err.Error(), "does not match") {
			t.Fatalf("ResolveModelLayout() error = %v, want legacy cache mismatch", err)
		}
	})

	t.Run("native serving image rejects a legacy model endpoint cache", func(t *testing.T) {
		endpointCache := cache.DeepCopy()
		endpoint := "https://models.example.com/v1"
		endpointCache.Spec.Source.NGC.ModelEndpoint = &endpoint
		_, err := ResolveModelLayout(context.Background(), &sequenceResolver{protocols: []Protocol{NativeV1}}, service, endpointCache)
		if err == nil || !strings.Contains(err.Error(), "does not match") {
			t.Fatalf("ResolveModelLayout() error = %v, want model-endpoint cache mismatch", err)
		}
	})
}
