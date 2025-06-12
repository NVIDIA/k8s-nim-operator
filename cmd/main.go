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

package main

import (
	"crypto/tls"
	"flag"
	"os"

	monitoring "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	discovery "k8s.io/client-go/discovery"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/NVIDIA/k8s-nim-operator/internal/conditions"
	"github.com/NVIDIA/k8s-nim-operator/internal/controller"
	"github.com/NVIDIA/k8s-nim-operator/internal/controller/platform"
	"github.com/NVIDIA/k8s-nim-operator/internal/controller/platform/kserve"
	"github.com/NVIDIA/k8s-nim-operator/internal/controller/platform/standalone"
	"github.com/NVIDIA/k8s-nim-operator/internal/render"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(appsv1alpha1.AddToScheme(scheme))
	utilruntime.Must(monitoring.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var platformType string

	flag.StringVar(&platformType, "platform", "standalone", "The model-serving inference platform to use."+
		"E.g., 'standalone (default)', 'kserve'.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metric endpoint binds to. "+
		"Use the port :8080. If not set, it will be 0 in order to disable the metrics server")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", false,
		"If set the metrics endpoint is served securely")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	var platformImpl platform.Platform
	switch platformType {
	case "standalone":
		platformImpl = &standalone.Standalone{}
	case "kserve":
		platformImpl = &kserve.KServe{}
	default:
		setupLog.Error(nil, "unsupported model-serving platform type", "platformType", platformType)
		os.Exit(1)
	}

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress:    metricsAddr,
			SecureServing:  secureMetrics,
			TLSOpts:        tlsOpts,
			FilterProvider: filters.WithAuthenticationAndAuthorization,
		},
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "a0715c6e.nvidia.com",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	updater := conditions.NewUpdater(mgr.GetClient())

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		setupLog.Error(err, "unable to create discovery client")
		os.Exit(1)
	}

	if err = controller.NewNIMCacheReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		ctrl.Log.WithName("controllers").WithName("NIMCache"),
		platformImpl,
	).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NIMCache")
		os.Exit(1)
	}

	if err = controller.NewNIMServiceReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		updater,
		discoveryClient,
		render.NewRenderer("/manifests"),
		ctrl.Log.WithName("controllers").WithName("NIMService"),
		platformImpl,
	).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NIMService")
		os.Exit(1)
	}

	if err = (&controller.NIMPipelineReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NIMPipeline")
		os.Exit(1)
	}

	if err = controller.NewNemoGuardrailReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		updater,
		render.NewRenderer("/manifests"),
		ctrl.Log.WithName("controllers").WithName("NemoGuardrail"),
	).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NemoGuardrail")
		os.Exit(1)
	}

	if err = controller.NewNemoEvaluatorReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		updater,
		render.NewRenderer("/manifests"),
		ctrl.Log.WithName("controllers").WithName("NemoEvaluator"),
	).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NemoEvaluator")
		os.Exit(1)
	}

	if err = controller.NewNemoEntitystoreReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		updater,
		render.NewRenderer("/manifests"),
		ctrl.Log.WithName("controllers").WithName("NemoEntitystore"),
	).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NemoEntitystore")
		os.Exit(1)
	}

	if err = controller.NewNemoDatastoreReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		updater,
		render.NewRenderer("/manifests"),
		ctrl.Log.WithName("controllers").WithName("NemoDatastore"),
	).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NemoDatastore")
		os.Exit(1)
	}

	if err = controller.NewNemoCustomizerReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		updater,
		render.NewRenderer("/manifests"),
		ctrl.Log.WithName("controllers").WithName("NemoCustomizer"),
	).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NemoCustomizer")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
