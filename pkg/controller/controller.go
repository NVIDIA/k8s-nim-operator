package controller

import (
	ctrl "sigs.k8s.io/controller-runtime"

	internalController "github.com/NVIDIA/k8s-nim-operator/internal/controller"
)

type Controller interface {
	SetupWithManager(mgr ctrl.Manager) error
}

func NewNimCacheController(mgr ctrl.Manager) Controller {
	return internalController.NewNIMCacheReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		ctrl.Log.WithName("controllers").WithName("NIMCache"),
		nil,
	)
}
