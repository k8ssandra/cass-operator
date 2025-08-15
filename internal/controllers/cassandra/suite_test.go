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

package controllers

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/k8ssandra/cass-operator/internal/controllers/control"
	internal "github.com/k8ssandra/cass-operator/internal/envtest"
	"github.com/k8ssandra/cass-operator/internal/result"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	cassandradatastaxcomv1beta1 "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	controlapi "github.com/k8ssandra/cass-operator/apis/control/v1alpha1"
	"github.com/k8ssandra/cass-operator/pkg/images"
	"github.com/k8ssandra/cass-operator/pkg/reconciliation"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	opts := zap.Options{
		Development: true,
		TimeEncoder: zapcore.ISO8601TimeEncoder,
		DestWriter:  GinkgoWriter,
	}

	logf.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	_, err := images.NewImageRegistry(filepath.Join("..", "..", "..", "tests", "testdata", "image_config_parsing.yaml"))
	Expect(err).To(Succeed())

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = cassandradatastaxcomv1beta1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = controlapi.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:  scheme.Scheme,
		Metrics: metricsserver.Options{BindAddress: "0"},
	})
	Expect(err).ToNot(HaveOccurred())

	err = (&CassandraDatacenterReconciler{
		Client:   k8sManager.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("CassandraDatacenter"),
		Scheme:   k8sManager.GetScheme(),
		Recorder: k8sManager.GetEventRecorderFor("cass-operator"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&internal.StatefulSetReconciler{
		Client: k8sClient,
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&control.CassandraTaskReconciler{
		Client: k8sClient,
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	Expect(k8sManager.GetCache().IndexField(ctx, &corev1.Pod{}, "spec.volumes.persistentVolumeClaim.claimName", func(obj client.Object) []string {
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
	})).ToNot(HaveOccurred())

	// Reduce the polling times and sleeps to speed up the tests
	cooldownPeriod = 1 * time.Millisecond
	minimumRequeueTime = 10 * time.Millisecond
	result.DurationFunc = func(msec int) time.Duration {
		return time.Duration(msec * int(time.Millisecond))
	}

	reconciliation.QuietDurationFunc = func(msec int) time.Duration {
		return time.Duration(msec * int(time.Millisecond))
	}

	control.JobRunningRequeue = 1 * time.Millisecond
	control.TaskRunningRequeue = 1 * time.Millisecond

	createStorageClass(ctx, "default")

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
