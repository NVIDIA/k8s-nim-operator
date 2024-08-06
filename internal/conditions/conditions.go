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

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Ready indicates that the service is ready
	Ready = "Ready"
	// NotReady indicates that the service is not yet ready
	NotReady = "NotReady"
	// Failed indicates that the service has failed
	Failed = "Failed"
	// ReasonServiceAccountFailed indicates that the creation of serviceaccount has failed
	ReasonServiceAccountFailed = "ServiceAccountFailed"
	// ReasonRoleFailed indicates that the creation of serviceaccount has failed
	ReasonRoleFailed = "RoleFailed"
	// ReasonRoleBindingFailed indicates that the creation of rolebinding has failed
	ReasonRoleBindingFailed = "RoleBindingFailed"
	// ReasonServiceFailed indicates that the creation of service has failed
	ReasonServiceFailed = "ServiceFailed"
	// ReasonIngressFailed indicates that the creation of ingress has failed
	ReasonIngressFailed = "IngressFailed"
	// ReasonHPAFailed indicates that the creation of hpa has failed
	ReasonHPAFailed = "HPAFailed"
	// ReasonSCCFailed indicates that the creation of scc has failed
	ReasonSCCFailed = "SCCFailed"
	// ReasonDeploymentFailed indicates that the creation of deployment has failed
	ReasonDeploymentFailed = "DeploymentFailed"
	// ReasonStatefulSetFailed indicates that the creation of statefulset has failed
	ReasonStatefulSetFailed = "StatefulsetFailed"
)

// Updater is the condition updater
type Updater interface {
	SetConditionsReady(ctx context.Context, cr *appsv1alpha1.NIMService, reason, message string) error
	SetConditionsNotReady(ctx context.Context, cr *appsv1alpha1.NIMService, reason, message string) error
	SetConditionsFailed(ctx context.Context, cr *appsv1alpha1.NIMService, reason, message string) error
}

type updater struct {
	client client.Client
}

// NewUpdater returns an instance of updater
func NewUpdater(c client.Client) Updater {
	return &updater{client: c}
}

func (u *updater) SetConditionsReady(ctx context.Context, cr *appsv1alpha1.NIMService, reason, message string) error {
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

	cr.Status.State = Ready
	return u.client.Status().Update(ctx, cr)
}

func (u *updater) SetConditionsNotReady(ctx context.Context, cr *appsv1alpha1.NIMService, reason, message string) error {
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

	cr.Status.State = NotReady
	return u.client.Status().Update(ctx, cr)
}

func (u *updater) SetConditionsFailed(ctx context.Context, cr *appsv1alpha1.NIMService, reason, message string) error {
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
	cr.Status.State = NotReady
	return u.client.Status().Update(ctx, cr)
}

// UpdateCondition updates the given condition into the conditions list
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
