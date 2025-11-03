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

package conditions

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
)

const (
	// Ready indicates that the service is ready.
	Ready = "Ready"
	// NotReady indicates that the service is not yet ready.
	NotReady = "NotReady"
	// Failed indicates that the service has failed.
	Failed = "Failed"
	// ValidationFailed indicates that the service CR has failed validations.
	ValidationFailed = "ValidationFailed"
	// ReasonServiceAccountFailed indicates that the creation of serviceaccount has failed.
	ReasonServiceAccountFailed = "ServiceAccountFailed"
	// ReasonRoleFailed indicates that the creation of serviceaccount has failed.
	ReasonRoleFailed = "RoleFailed"
	// ReasonRoleBindingFailed indicates that the creation of rolebinding has failed.
	ReasonRoleBindingFailed = "RoleBindingFailed"
	// ReasonServiceFailed indicates that the creation of service has failed.
	ReasonServiceFailed = "ServiceFailed"
	// ReasonIngressFailed indicates that the creation of ingress has failed.
	ReasonIngressFailed = "IngressFailed"
	// ReasonHTTPRouteFailed indicates that the creation of httproute has failed.
	ReasonHTTPRouteFailed = "HTTPRouteFailed"
	// ReasonGRPCRouteFailed indicates that the creation of grpcroute has failed.
	ReasonGRPCRouteFailed = "GRPCRouteFailed"
	// ReasonHPAFailed indicates that the creation of hpa has failed.
	ReasonHPAFailed = "HPAFailed"
	// ReasonSCCFailed indicates that the creation of scc has failed.
	ReasonSCCFailed = "SCCFailed"
	// ReasonServiceMonitorFailed indicates that the creation of Service Monitor has failed.
	ReasonServiceMonitorFailed = "ServiceMonitorFailed"
	// ReasonConfigMapFailed indicates that the creation of configmap has failed.
	ReasonConfigMapFailed = "ConfigMapFailed"
	// ReasonDeploymentFailed indicates that the creation of deployment has failed.
	ReasonDeploymentFailed = "DeploymentFailed"
	// ReasonStatefulSetFailed indicates that the creation of statefulset has failed.
	ReasonStatefulSetFailed = "StatefulsetFailed"
	// ReasonLeaderWorkerSetFailed indicates that the creation of leader-worker set has failed.
	ReasonLeaderWorkerSetFailed = "LeaderWorkerSetFailed"
	// ReasonSecretFailed indicates that the creation of secret has failed.
	ReasonSecretFailed = "SecretFailed"
	// ReasonNIMCacheFailed indicates that the NIMCache is in failed state.
	ReasonNIMCacheFailed = "NIMCacheFailed"
	// ReasonNIMCacheNotFound indicates that the NIMCache is not found.
	ReasonNIMCacheNotFound = "NIMCacheNotFound"
	// ReasonNIMCacheNotReady indicates that the NIMCache is not ready.
	ReasonNIMCacheNotReady = "NIMCacheNotReady"
	// ReasonDRAResourcesUnsupported indicates that the DRA resources are not supported on this cluster version.
	ReasonDRAResourcesUnsupported = "DRAResourcesUnsupported"
	// ReasonInferenceServiceFailed indicates that the creation of inferenceservice has failed.
	ReasonInferenceServiceFailed = "InferenceServiceFailed"
	// ReasonResourceClaimFailed indicates that the creation of resourceclaim has failed.
	ReasonResourceClaimFailed = "ResourceClaimFailed"
	// ReasonResourceClaimTemplateFailed indicates that the creation of resourceclaimtemplate has failed.
	ReasonResourceClaimTemplateFailed = "ResourceClaimTemplateFailed"
	// ReasonComputeDomainFailed indicates that the creation of computedomain has failed.
	ReasonComputeDomainFailed = "ComputeDomainFailed"
)

// Updater is the condition updater.
type Updater interface {
	SetConditionsReady(ctx context.Context, cr client.Object, reason, message string) error
	SetConditionsNotReady(ctx context.Context, cr client.Object, reason, message string) error
	SetConditionsFailed(ctx context.Context, cr client.Object, reason, message string) error
}

type updater struct {
	client client.Client
}

// NewUpdater returns an instance of updater.
func NewUpdater(c client.Client) Updater {
	return &updater{client: c}
}

func (u *updater) SetConditionsReady(ctx context.Context, obj client.Object, reason, message string) error {
	switch cr := obj.(type) {
	case *appsv1alpha1.NIMService:
		return u.SetConditionsReadyNIMService(ctx, cr, reason, message)
	case *appsv1alpha1.NemoGuardrail:
		return u.SetConditionsReadyNemoGuardrail(ctx, cr, reason, message)
	case *appsv1alpha1.NemoEntitystore:
		return u.SetConditionsReadyNemoEntitystore(ctx, cr, reason, message)
	case *appsv1alpha1.NemoCustomizer:
		return u.SetConditionsReadyNemoCustomizer(ctx, cr, reason, message)
	case *appsv1alpha1.NemoDatastore:
		return u.SetConditionsReadyNemoDatastore(ctx, cr, reason, message)
	case *appsv1alpha1.NemoEvaluator:
		return u.SetConditionsReadyNemoEvaluator(ctx, cr, reason, message)
	default:
		return fmt.Errorf("unknown CRD type for %v", obj)
	}
}

func (u *updater) SetConditionsReadyNIMService(ctx context.Context, cr *appsv1alpha1.NIMService, reason, message string) error {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Ready,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:   Failed,
		Status: metav1.ConditionFalse,
		Reason: Ready,
	})
	cr.Status.State = appsv1alpha1.NIMServiceStatusReady
	return u.updateNIMServiceStatus(ctx, cr)
}

func (u *updater) SetConditionsReadyNemoGuardrail(ctx context.Context, cr *appsv1alpha1.NemoGuardrail, reason, message string) error {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Ready,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:   Failed,
		Status: metav1.ConditionFalse,
		Reason: Ready,
	})
	cr.Status.State = appsv1alpha1.NemoGuardrailStatusReady
	return u.updateNemoGuardrailStatus(ctx, cr)
}

func (u *updater) SetConditionsReadyNemoEntitystore(ctx context.Context, cr *appsv1alpha1.NemoEntitystore, reason, message string) error {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Ready,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:   Failed,
		Status: metav1.ConditionFalse,
		Reason: Ready,
	})
	cr.Status.State = appsv1alpha1.NemoEntitystoreStatusReady
	return u.updateNemoEntitystoreStatus(ctx, cr)
}

func (u *updater) SetConditionsReadyNemoDatastore(ctx context.Context, cr *appsv1alpha1.NemoDatastore, reason, message string) error {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Ready,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:   Failed,
		Status: metav1.ConditionFalse,
		Reason: Ready,
	})
	cr.Status.State = appsv1alpha1.NemoDatastoreStatusReady
	return u.updateNemoDatastoreStatus(ctx, cr)
}

func (u *updater) SetConditionsReadyNemoCustomizer(ctx context.Context, cr *appsv1alpha1.NemoCustomizer, reason, message string) error {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Ready,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:   Failed,
		Status: metav1.ConditionFalse,
		Reason: Ready,
	})
	cr.Status.State = appsv1alpha1.NemoCustomizerStatusReady
	return u.updateNemoCustomizerStatus(ctx, cr)
}

func (u *updater) SetConditionsReadyNemoEvaluator(ctx context.Context, cr *appsv1alpha1.NemoEvaluator, reason, message string) error {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Ready,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:   Failed,
		Status: metav1.ConditionFalse,
		Reason: Ready,
	})
	cr.Status.State = appsv1alpha1.NemoEvaluatorStatusReady
	return u.updateNemoEvaluatorStatus(ctx, cr)
}

func (u *updater) SetConditionsNotReady(ctx context.Context, obj client.Object, reason, message string) error {
	switch cr := obj.(type) {
	case *appsv1alpha1.NIMService:
		return u.SetConditionsNotReadyNIMService(ctx, cr, reason, message)
	case *appsv1alpha1.NemoGuardrail:
		return u.SetConditionsNotReadyNemoGuardrail(ctx, cr, reason, message)
	case *appsv1alpha1.NemoEntitystore:
		return u.SetConditionsNotReadyNemoEntitystore(ctx, cr, reason, message)
	case *appsv1alpha1.NemoDatastore:
		return u.SetConditionsNotReadyNemoDatastore(ctx, cr, reason, message)
	case *appsv1alpha1.NemoCustomizer:
		return u.SetConditionsNotReadyNemoCustomizer(ctx, cr, reason, message)
	case *appsv1alpha1.NemoEvaluator:
		return u.SetConditionsNotReadyNemoEvaluator(ctx, cr, reason, message)
	default:
		return fmt.Errorf("unknown CRD type for %v", obj)
	}
}

func (u *updater) SetConditionsNotReadyNIMService(ctx context.Context, cr *appsv1alpha1.NIMService, reason, message string) error {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Ready,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Failed,
		Status:  metav1.ConditionFalse,
		Reason:  Ready,
		Message: message,
	})
	cr.Status.State = appsv1alpha1.NIMServiceStatusNotReady
	return u.updateNIMServiceStatus(ctx, cr)
}

func (u *updater) SetConditionsNotReadyNemoGuardrail(ctx context.Context, cr *appsv1alpha1.NemoGuardrail, reason, message string) error {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Ready,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Failed,
		Status:  metav1.ConditionFalse,
		Reason:  Ready,
		Message: message,
	})
	cr.Status.State = appsv1alpha1.NemoGuardrailStatusNotReady
	return u.updateNemoGuardrailStatus(ctx, cr)
}

func (u *updater) SetConditionsNotReadyNemoEntitystore(ctx context.Context, cr *appsv1alpha1.NemoEntitystore, reason, message string) error {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Ready,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Failed,
		Status:  metav1.ConditionFalse,
		Reason:  Ready,
		Message: message,
	})
	cr.Status.State = appsv1alpha1.NemoEntitystoreStatusNotReady
	return u.updateNemoEntitystoreStatus(ctx, cr)
}

func (u *updater) SetConditionsNotReadyNemoDatastore(ctx context.Context, cr *appsv1alpha1.NemoDatastore, reason, message string) error {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Ready,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Failed,
		Status:  metav1.ConditionFalse,
		Reason:  Ready,
		Message: message,
	})
	cr.Status.State = appsv1alpha1.NemoDatastoreStatusNotReady
	return u.updateNemoDatastoreStatus(ctx, cr)
}

func (u *updater) SetConditionsNotReadyNemoCustomizer(ctx context.Context, cr *appsv1alpha1.NemoCustomizer, reason, message string) error {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Ready,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Failed,
		Status:  metav1.ConditionFalse,
		Reason:  Ready,
		Message: message,
	})
	cr.Status.State = appsv1alpha1.NemoCustomizerStatusNotReady
	return u.updateNemoCustomizerStatus(ctx, cr)
}

func (u *updater) SetConditionsNotReadyNemoEvaluator(ctx context.Context, cr *appsv1alpha1.NemoEvaluator, reason, message string) error {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Ready,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Failed,
		Status:  metav1.ConditionFalse,
		Reason:  Ready,
		Message: message,
	})
	cr.Status.State = appsv1alpha1.NemoEvaluatorStatusNotReady
	return u.updateNemoEvaluatorStatus(ctx, cr)
}

func (u *updater) SetConditionsFailed(ctx context.Context, obj client.Object, reason, message string) error {
	switch cr := obj.(type) {
	case *appsv1alpha1.NIMService:
		return u.SetConditionsFailedNIMService(ctx, cr, reason, message)
	case *appsv1alpha1.NemoGuardrail:
		return u.SetConditionsFailedNemoGuardrail(ctx, cr, reason, message)
	case *appsv1alpha1.NemoEntitystore:
		return u.SetConditionsFailedNemoEntitystore(ctx, cr, reason, message)
	case *appsv1alpha1.NemoDatastore:
		return u.SetConditionsFailedNemoDatastore(ctx, cr, reason, message)
	case *appsv1alpha1.NemoCustomizer:
		return u.SetConditionsFailedNemoCustomizer(ctx, cr, reason, message)
	case *appsv1alpha1.NemoEvaluator:
		return u.SetConditionsFailedNemoEvaluator(ctx, cr, reason, message)
	default:
		return fmt.Errorf("unknown CRD type for %v", obj)
	}
}

func (u *updater) SetConditionsFailedNIMService(ctx context.Context, cr *appsv1alpha1.NIMService, reason, message string) error {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:   Ready,
		Status: metav1.ConditionFalse,
		Reason: Failed,
	})

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Failed,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
	cr.Status.State = appsv1alpha1.NIMServiceStatusFailed
	return u.updateNIMServiceStatus(ctx, cr)
}

func (u *updater) SetConditionsFailedNemoGuardrail(ctx context.Context, cr *appsv1alpha1.NemoGuardrail, reason, message string) error {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:   Ready,
		Status: metav1.ConditionFalse,
		Reason: Failed,
	})

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Failed,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
	cr.Status.State = appsv1alpha1.NemoGuardrailStatusFailed
	return u.updateNemoGuardrailStatus(ctx, cr)
}

func (u *updater) SetConditionsFailedNemoEntitystore(ctx context.Context, cr *appsv1alpha1.NemoEntitystore, reason, message string) error {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:   Ready,
		Status: metav1.ConditionFalse,
		Reason: Failed,
	})

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Failed,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
	cr.Status.State = appsv1alpha1.NemoEntitystoreStatusFailed
	return u.updateNemoEntitystoreStatus(ctx, cr)
}

func (u *updater) SetConditionsFailedNemoDatastore(ctx context.Context, cr *appsv1alpha1.NemoDatastore, reason, message string) error {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:   Ready,
		Status: metav1.ConditionFalse,
		Reason: Failed,
	})

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Failed,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
	cr.Status.State = appsv1alpha1.NemoDatastoreStatusFailed
	return u.updateNemoDatastoreStatus(ctx, cr)
}

func (u *updater) SetConditionsFailedNemoCustomizer(ctx context.Context, cr *appsv1alpha1.NemoCustomizer, reason, message string) error {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:   Ready,
		Status: metav1.ConditionFalse,
		Reason: Failed,
	})

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Failed,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
	cr.Status.State = appsv1alpha1.NemoCustomizerStatusFailed
	return u.updateNemoCustomizerStatus(ctx, cr)
}

func (u *updater) SetConditionsFailedNemoEvaluator(ctx context.Context, cr *appsv1alpha1.NemoEvaluator, reason, message string) error {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:   Ready,
		Status: metav1.ConditionFalse,
		Reason: Failed,
	})

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    Failed,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
	cr.Status.State = appsv1alpha1.NemoEvaluatorStatusFailed
	return u.updateNemoEvaluatorStatus(ctx, cr)
}

func (u *updater) updateNIMServiceStatus(ctx context.Context, cr *appsv1alpha1.NIMService) error {

	obj := &appsv1alpha1.NIMService{}
	errGet := u.client.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.GetNamespace()}, obj)
	if errGet != nil {
		return errGet
	}
	obj.Status = cr.Status
	if err := u.client.Status().Update(ctx, obj); err != nil {
		return err
	}
	return nil
}

func (u *updater) updateNemoGuardrailStatus(ctx context.Context, cr *appsv1alpha1.NemoGuardrail) error {
	obj := &appsv1alpha1.NemoGuardrail{}
	errGet := u.client.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.GetNamespace()}, obj)
	if errGet != nil {
		return errGet
	}
	obj.Status = cr.Status
	if err := u.client.Status().Update(ctx, obj); err != nil {
		return err
	}
	return nil
}

func (u *updater) updateNemoEntitystoreStatus(ctx context.Context, cr *appsv1alpha1.NemoEntitystore) error {
	obj := &appsv1alpha1.NemoEntitystore{}
	errGet := u.client.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.GetNamespace()}, obj)
	if errGet != nil {
		return errGet
	}
	obj.Status = cr.Status
	if err := u.client.Status().Update(ctx, obj); err != nil {
		return err
	}
	return nil
}

func (u *updater) updateNemoDatastoreStatus(ctx context.Context, cr *appsv1alpha1.NemoDatastore) error {
	obj := &appsv1alpha1.NemoDatastore{}
	errGet := u.client.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.GetNamespace()}, obj)
	if errGet != nil {
		return errGet
	}
	obj.Status = cr.Status
	if err := u.client.Status().Update(ctx, obj); err != nil {
		return err
	}
	return nil
}

func (u *updater) updateNemoCustomizerStatus(ctx context.Context, cr *appsv1alpha1.NemoCustomizer) error {
	obj := &appsv1alpha1.NemoCustomizer{}
	errGet := u.client.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.GetNamespace()}, obj)
	if errGet != nil {
		return errGet
	}
	obj.Status = cr.Status
	if err := u.client.Status().Update(ctx, obj); err != nil {
		return err
	}
	return nil
}

func (u *updater) updateNemoEvaluatorStatus(ctx context.Context, cr *appsv1alpha1.NemoEvaluator) error {
	obj := &appsv1alpha1.NemoEvaluator{}
	errGet := u.client.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.GetNamespace()}, obj)
	if errGet != nil {
		return errGet
	}
	obj.Status = cr.Status
	if err := u.client.Status().Update(ctx, obj); err != nil {
		return err
	}
	return nil
}

// UpdateCondition updates the given condition into the conditions list.
func UpdateCondition(conditions *[]metav1.Condition, conditionType string, status metav1.ConditionStatus, reason, message string) {
	for i := range *conditions {
		if (*conditions)[i].Type == conditionType {
			// existing condition
			(*conditions)[i].Status = status
			(*conditions)[i].LastTransitionTime = metav1.Now()
			(*conditions)[i].Reason = reason
			(*conditions)[i].Message = message
			// condition updated
			return
		}
	}
	// new condition
	*conditions = append(*conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
	// condition updated
}

// IfPresentUpdateCondition updates an already existing condition.
func IfPresentUpdateCondition(conditions *[]metav1.Condition, conditionType string, status metav1.ConditionStatus, reason, message string) {
	for i := range *conditions {
		if (*conditions)[i].Type == conditionType {
			// existing condition
			(*conditions)[i].Status = status
			(*conditions)[i].LastTransitionTime = metav1.Now()
			(*conditions)[i].Reason = reason
			(*conditions)[i].Message = message
			// condition updated
			return
		}
	}
}
