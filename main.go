/*
Copyright 2021.

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
	"strconv"
	"strings"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	api "github.com/k8ssandra/cass-operator/api/v1beta1"
	"github.com/k8ssandra/cass-operator/controllers"
	"github.com/k8ssandra/cass-operator/pkg/utils"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(api.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	options := ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "b569adb7.cassandra.datastax.com",
	}

	ns, err := utils.GetWatchNamespace()
	if err != nil {
		setupLog.Error(err, "unable to get WatchNamespace, "+
			"the manager will watch and manage resources in all namespaces")
	}

	// Add support for MultiNamespace set in WATCH_NAMESPACE (e.g ns1,ns2)
	if strings.Contains(ns, ",") {
		setupLog.Info("manager set up with multiple namespaces", "namespaces", ns)
		// configure cluster-scoped with MultiNamespacedCacheBuilder
		options.Namespace = ""
		options.NewCache = cache.MultiNamespacedCacheBuilder(strings.Split(ns, ","))
	} else {
		setupLog.Info("watch namespace configured", "namespace", ns)
		options.Namespace = ns
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.CassandraDatacenterReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("CassandraDatacenter"),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("cass-operator"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CassandraDatacenter")
		os.Exit(1)
	}

	skipWebhookEnvVal := os.Getenv("SKIP_VALIDATING_WEBHOOK")
	if skipWebhookEnvVal == "" {
		skipWebhookEnvVal = "FALSE"
	}
	skipWebhook, err := strconv.ParseBool(skipWebhookEnvVal)
	if err != nil {
		setupLog.Error(err, "bad value for SKIP_VALIDATING_WEBHOOK env")
		os.Exit(1)
	}

	if !skipWebhook {
		if err = (&api.CassandraDatacenter{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "CassandraDatacenter")
			os.Exit(1)
		}
	}
	//+kubebuilder:scaffold:builder

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
