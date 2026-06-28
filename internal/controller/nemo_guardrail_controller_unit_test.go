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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

func TestReconcileNemoGuardrailRejectsInvalidConfigStore(t *testing.T) {
	tests := []struct {
		name          string
		configStore   appsv1alpha1.GuardrailConfig
		expectedError string
	}{
		{
			name:          "should reject config store if config sources are omitted",
			configStore:   appsv1alpha1.GuardrailConfig{},
			expectedError: "exactly one of spec.configStore.configMap or spec.configStore.pvc must be set",
		},
		{
			name: "should reject config store if both config sources are set",
			configStore: appsv1alpha1.GuardrailConfig{
				ConfigMap: &appsv1alpha1.ConfigMapRef{Name: "guardrail-config"},
				PVC:       &appsv1alpha1.PersistentVolumeClaim{Name: "guardrail-pvc"},
			},
			expectedError: "exactly one of spec.configStore.configMap or spec.configStore.pvc must be set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := appsv1alpha1.AddToScheme(scheme); err != nil {
				t.Fatalf("AddToScheme() error = %v", err)
			}

			reconciler := &NemoGuardrailReconciler{
				Client:   fake.NewClientBuilder().WithScheme(scheme).Build(),
				scheme:   scheme,
				recorder: record.NewFakeRecorder(10),
			}

			nemoGuardrail := &appsv1alpha1.NemoGuardrail{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nemoguardrail",
					Namespace: "default",
				},
				Spec: appsv1alpha1.NemoGuardrailSpec{
					ConfigStore: tt.configStore,
				},
			}

			_, err := reconciler.reconcileNemoGuardrail(context.Background(), nemoGuardrail)
			if err == nil {
				t.Fatal("reconcileNemoGuardrail() error = nil, want non-nil")
			}
			if err.Error() != tt.expectedError {
				t.Fatalf("reconcileNemoGuardrail() error = %q, want %q", err.Error(), tt.expectedError)
			}
		})
	}
}
