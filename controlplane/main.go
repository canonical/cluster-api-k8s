/*


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
	"flag"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/utils/ptr"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1beta1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	bootstrapv1 "github.com/canonical/cluster-api-k8s/bootstrap/api/v1beta2"
	controlplanev1 "github.com/canonical/cluster-api-k8s/controlplane/api/v1beta2"
	"github.com/canonical/cluster-api-k8s/controlplane/controllers"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = clusterv1beta1.AddToScheme(scheme)
	_ = expv1beta1.AddToScheme(scheme)
	_ = bootstrapv1.AddToScheme(scheme)

	_ = controlplanev1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var syncPeriod time.Duration
	var k8sdDialTimeout time.Duration

	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	flag.DurationVar(&syncPeriod, "sync-period", 10*time.Minute,
		"The minimum interval at which watched resources are reconciled (e.g. 15m)")

	flag.DurationVar(&k8sdDialTimeout, "k8sd-dial-timeout-duration", 60*time.Second,
		"Duration that the proxy client waits at most to establish a connection with k8sd")

	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	ctx := ctrl.SetupSignalHandler()

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port: 9443,
		}),
		LeaderElection:   enableLeaderElection,
		LeaderElectionID: "148fa072.controlplane.cluster.x-k8s.io",
		Cache: cache.Options{
			SyncPeriod: &syncPeriod,
		},
		Controller: config.Controller{
			// TODO: avoid duplicate controller names.
			SkipNameValidation: ptr.To(true),
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	ctrPlaneLogger := ctrl.Log.WithName("controllers").WithName("CK8sControlPlane")
	if err = (&controllers.CK8sControlPlaneReconciler{
		Client:          mgr.GetClient(),
		Log:             ctrPlaneLogger,
		Scheme:          mgr.GetScheme(),
		K8sdDialTimeout: k8sdDialTimeout,
	}).SetupWithManager(ctx, mgr, &ctrPlaneLogger); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CK8sControlPlane")
		os.Exit(1)
	}

	ctrMachineLogger := ctrl.Log.WithName("controllers").WithName("Machine")
	if err = (&controllers.MachineReconciler{
		Client:          mgr.GetClient(),
		Log:             ctrMachineLogger,
		Scheme:          mgr.GetScheme(),
		K8sdDialTimeout: k8sdDialTimeout,
	}).SetupWithManager(ctx, mgr, &ctrMachineLogger); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Machine")
		os.Exit(1)
	}

	inplaceUpgradeLogger := ctrl.Log.WithName("controllers").WithName("OrchestratedInPlaceUpgrade")
	if err = (&controllers.OrchestratedInPlaceUpgradeController{
		Client: mgr.GetClient(),
		Log:    inplaceUpgradeLogger,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "failed to create controller", "controller", "OrchestratedInPlaceUpgrade")
	}

	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = (&controlplanev1.CK8sControlPlane{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "CK8sControlPlane")
			os.Exit(1)
		}
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
