// podlabeler — a Kubernetes controller that labels pods based on their
// container image, driven by PodLabelerPolicy custom resources.
//
// THIS IS THE ACT I BASELINE — the broken version of the controller.
// See controller/reconciler.go for BUG 1 (lost update) and BUG 2 (stale
// cache read). BUG 3 (no leader election) lives in this file.
package main

import (
	"flag"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	labelingv1alpha1 "github.com/knabben/istio-poc/api/v1alpha1"
	"github.com/knabben/istio-poc/controller"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(labelingv1alpha1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
}

func main() {
	var (
		metricsAddr string
		probeAddr   string
	)
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "metrics server bind address")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "health probe bind address")
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,

		// BUG 3: leader election disabled.
		// In a real production deployment this controller will be scaled
		// to multiple replicas for HA. Without leader election, every
		// replica reconciles every event independently. Two replicas
		// writing to the same pod produces:
		//   - Lost updates (BUG 1 amplified)
		//   - Lease holder oscillation (visible via the Lease object)
		//   - Istio config push storm (every label flip is a mesh event)
		//
		// FIX (Act II):
		//   LeaderElection:                true,
		//   LeaderElectionID:              "podlabeler.knabben.dev",
		//   LeaderElectionReleaseOnCancel: true,
		LeaderElection: false,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controller.PodReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info(fmt.Sprintf("starting podlabeler (leader election=%v)", false))
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
