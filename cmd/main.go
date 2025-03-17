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
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"strings"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	"go.uber.org/zap/zapcore"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	configv1beta1 "github.com/k8ssandra/cass-operator/apis/config/v1beta1"
	controlv1alpha1 "github.com/k8ssandra/cass-operator/apis/control/v1alpha1"
	scheduledtaskv1alpha1 "github.com/k8ssandra/cass-operator/apis/scheduledtask.k8ssandra.io/v1alpha1"
	controllers "github.com/k8ssandra/cass-operator/internal/controllers/cassandra"
	controlcontrollers "github.com/k8ssandra/cass-operator/internal/controllers/control"
	scheduledtaskcontrollers "github.com/k8ssandra/cass-operator/internal/controllers/scheduledtask"
	apiwebhook "github.com/k8ssandra/cass-operator/internal/webhooks/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/pkg/images"
	"github.com/k8ssandra/cass-operator/pkg/utils"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(api.AddToScheme(scheme))
	utilruntime.Must(configv1beta1.AddToScheme(scheme))
	utilruntime.Must(controlv1alpha1.AddToScheme(scheme))
	utilruntime.Must(scheduledtaskv1alpha1.AddToScheme(scheme))
}

func main() {

	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var configFile string
	var tlsOpts []func(*tls.Config)
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.StringVar(&configFile, "config", "",
		"The cass-operator will load its configuration from this file. "+
			"Omit this flag to use the default configuration values. ")

	opts := zap.Options{
		Development: true,
		TimeEncoder: zapcore.ISO8601TimeEncoder,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	ns, err := utils.GetWatchNamespace()
	if err != nil {
		setupLog.Error(err, "unable to get WatchNamespace, "+
			"the manager will watch and manage resources in all namespaces")
	}

	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	operConfig := &configv1beta1.OperatorConfig{}
	options := ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "b569adb7.cassandra.datastax.com",
	}
	if configFile != "" {
		var err error
		operConfig, err = readOperConfig(configFile)
		if err != nil {
			setupLog.Error(err, "unable to load the config file")
			os.Exit(1)
		}
	}

	if operConfig.ImageConfigFile == "" {
		operConfig.ImageConfigFile = "/configs/image_config.yaml"
	}

	err = images.ParseImageConfig(operConfig.ImageConfigFile)
	if err != nil {
		setupLog.Error(err, "unable to load the image config file")
		os.Exit(1)
	}

	options.Cache = cache.Options{
		DefaultNamespaces: map[string]cache.Config{},
	}

	// Add support for MultiNamespace set in WATCH_NAMESPACE (e.g ns1,ns2)
	if strings.Contains(ns, ",") {
		setupLog.Info("manager set up with multiple namespaces", "namespaces", ns)
		// configure cluster-scoped with MultiNamespacedCacheBuilder
		namespaces := strings.Split(ns, ",")
		for _, namespace := range namespaces {
			options.Cache.DefaultNamespaces[namespace] = cache.Config{}
		}
	} else if ns != "" {
		setupLog.Info("watch namespace configured", "namespace", ns)
		options.Cache.DefaultNamespaces[ns] = cache.Config{}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()

	if err = (&controllers.CassandraDatacenterReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("CassandraDatacenter"),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("cass-operator"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CassandraDatacenter")
		os.Exit(1)
	}

	if !operConfig.DisableWebhooks {
		if err = apiwebhook.SetupCassandraDatacenterWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "CassandraDatacenter")
			os.Exit(1)
		}
	}
	if err = (&controlcontrollers.CassandraTaskReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CassandraTask")
		os.Exit(1)
	}

	if err := mgr.GetCache().IndexField(ctx, &corev1.Pod{}, "spec.volumes.persistentVolumeClaim.claimName", func(obj client.Object) []string {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return nil
		}

		var pvcNames []string
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil {
				pvcNames = append(pvcNames, volume.PersistentVolumeClaim.ClaimName)
			}
		}
		return pvcNames
	}); err != nil {
		setupLog.Error(err, "unable to set up field indexer")
		os.Exit(1)
	}

	if err = (&scheduledtaskcontrollers.ScheduledTaskReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("ScheduledTask"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ScheduledTask")
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

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func readOperConfig(configFile string) (*configv1beta1.OperatorConfig, error) {
	operConfig := &configv1beta1.OperatorConfig{}
	_, err := os.Stat(configFile)
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}

	codecs := serializer.NewCodecFactory(scheme)
	if err := runtime.DecodeInto(codecs.UniversalDecoder(), content, operConfig); err != nil {
		return nil, fmt.Errorf("could not decode file into runtime.Object: %v", err)
	}

	return operConfig, nil
}
