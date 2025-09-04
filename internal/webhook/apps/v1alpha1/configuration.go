package v1alpha1

import (
	"context"
	"encoding/json"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/NVIDIA/k8s-nim-operator/internal/config"
	"github.com/NVIDIA/k8s-nim-operator/internal/k8sutil"
)

// EnsureValidatingWebhook creates or updates the ValidatingWebhookConfiguration
// that used to be templated by Helm.  It is a best-effort reconciliation and
// returns an error only when we cannot make the desired state match the spec.
func EnsureValidatingWebhook(
	ctx context.Context,
	apiReader client.Reader,
	writer client.Client,
	namespace string,
	fullNamePrefix string,
) error {
	// Desired validatingwebhookconfiguration spec.
	desired := buildConfigurationSpec(namespace, fullNamePrefix)

	// Check if there is already a spec.
	existing := &admissionv1.ValidatingWebhookConfiguration{}
	err := apiReader.Get(ctx, types.NamespacedName{Name: desired.Name}, existing)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if errors.IsNotFound(err) {
		return writer.Create(ctx, desired)
	}

	// Deep-compare; update only if something differs.
	cur, _ := json.Marshal(existing.Webhooks)
	want, _ := json.Marshal(desired.Webhooks)

	if string(cur) == string(want) {
		return nil
	}

	existing.Webhooks = desired.Webhooks
	existing.Annotations = desired.Annotations
	return writer.Update(ctx, existing)
}

// buildDesired reproduces the spec that used to be in Helm.
func buildConfigurationSpec(namespace, namePrefix string) *admissionv1.ValidatingWebhookConfiguration {
	pathCache := "/validate-apps-nvidia-com-v1alpha1-nimcache"
	pathService := "/validate-apps-nvidia-com-v1alpha1-nimservice"

	// Use appropriate annotations/labels as per deployment mode.
	var annotations map[string]string
	var labels map[string]string
	var clientconfignimcache admissionv1.WebhookClientConfig
	var clientconfignimservice admissionv1.WebhookClientConfig

	clientconfignimcache = admissionv1.WebhookClientConfig{
		Service: &admissionv1.ServiceReference{
			Namespace: namespace,
			Name:      namePrefix + "-webhook-service",
			Path:      &pathCache,
		},
	}
	clientconfignimservice = admissionv1.WebhookClientConfig{
		Service: &admissionv1.ServiceReference{
			Namespace: namespace,
			Name:      namePrefix + "-webhook-service",
			Path:      &pathService,
		},
	}

	// Deployment specific values.
	if config.OrchestratorType == k8sutil.K8s {
		if config.TLSMode == "cert-manager" {
			annotations = map[string]string{"cert-manager.io/inject-ca-from": namespace + "/" + namePrefix + "-serving-cert"}
		} else {
			annotations = map[string]string{}
			clientconfignimcache = admissionv1.WebhookClientConfig{
				Service: &admissionv1.ServiceReference{
					Namespace: namespace,
					Name:      namePrefix + "-webhook-service",
					Path:      &pathCache,
				},
				CABundle: config.TLSCA,
			}
			clientconfignimservice = admissionv1.WebhookClientConfig{
				Service: &admissionv1.ServiceReference{
					Namespace: namespace,
					Name:      namePrefix + "-webhook-service",
					Path:      &pathService,
				},
				CABundle: config.TLSCA,
			}
		}
		labels = map[string]string{
			"app.kubernetes.io/name":       "k8s-nim-operator",
			"app.kubernetes.io/managed-by": "helm",
		}
	} else {
		annotations = map[string]string{"service.beta.openshift.io/inject-cabundle": "true"}
		labels = map[string]string{
			"app.kubernetes.io/name":       "k8s-nim-operator",
			"app.kubernetes.io/managed-by": "openshift",
		}
	}

	return &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:        namePrefix + "-validating-webhook-configuration",
			Annotations: annotations,
			Labels:      labels,
		},
		Webhooks: []admissionv1.ValidatingWebhook{
			{
				Name:                    "vnimcache-v1alpha1.kb.io",
				AdmissionReviewVersions: []string{"v1"},
				ClientConfig:            clientconfignimcache,
				FailurePolicy: func() *admissionv1.FailurePolicyType {
					fp := admissionv1.Fail
					return &fp
				}(),
				SideEffects: func() *admissionv1.SideEffectClass {
					s := admissionv1.SideEffectClassNone
					return &s
				}(),
				Rules: []admissionv1.RuleWithOperations{{
					Operations: []admissionv1.OperationType{
						admissionv1.Create, admissionv1.Update,
					},
					Rule: admissionv1.Rule{
						APIGroups:   []string{"apps.nvidia.com"},
						APIVersions: []string{"v1alpha1"},
						Resources:   []string{"nimcaches"},
					},
				}},
			},
			{
				Name:                    "vnimservice-v1alpha1.kb.io",
				AdmissionReviewVersions: []string{"v1"},
				ClientConfig:            clientconfignimservice,
				FailurePolicy: func() *admissionv1.FailurePolicyType {
					fp := admissionv1.Fail
					return &fp
				}(),
				SideEffects: func() *admissionv1.SideEffectClass {
					s := admissionv1.SideEffectClassNone
					return &s
				}(),
				Rules: []admissionv1.RuleWithOperations{{
					Operations: []admissionv1.OperationType{
						admissionv1.Create, admissionv1.Update,
					},
					Rule: admissionv1.Rule{
						APIGroups:   []string{"apps.nvidia.com"},
						APIVersions: []string{"v1alpha1"},
						Resources:   []string{"nimservices"},
					},
				}},
			},
		},
	}
}
