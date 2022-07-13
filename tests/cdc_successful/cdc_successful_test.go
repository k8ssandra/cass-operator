// Copyright DataStax, Inc.
// Please see the included license file for details.

package cdc_successful

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
	shutil "github.com/k8ssandra/cass-operator/tests/util/sh"
)

var (
	testName            = "OSS CDC flows work"
	namespace           = "test-cdc"
	dcName              = "dc1"
	dcYaml              = "../testdata/test-cdc/cassandra-datacenter.yaml"
	pulsarValues        = "../testdata/test-cdc/dev-values.yaml"
	testUtilsDeployment = "../testdata/test-cdc/testutils-deployment.yaml"
	ns                  = ginkgo_util.NewWrapper(testName, namespace)
)

func TestLifecycle(t *testing.T) {
	AfterSuite(func() {
		logPath := fmt.Sprintf("%s/aftersuite", ns.LogDir)
		err := kubectl.DumpAllLogs(logPath).ExecV()
		if err != nil {
			t.Logf("Failed to dump all the logs: %v", err)
		}

		fmt.Printf("\n\tPost-run logs dumped at: %s\n\n", logPath)
		ns.Terminate()
		err = kustomize.Undeploy(namespace)
		if err != nil {
			t.Logf("Failed to undeploy cass-operator: %v", err)
		}
		kubectl.Delete("ns", "pulsar").OutputPanic()
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, testName)
}

var _ = Describe(testName, func() {
	Context("when in a new cluster with CDC enabled", func() {
		Specify("CDC feeds will become available for read", func() {

			By("creating a namespace for the cass-operator")
			err := kubectl.CreateNamespace(namespace).ExecV()
			Expect(err).ToNot(HaveOccurred())

			By("deploy cass-operator with kustomize")
			err = kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())
			ns.WaitForOperatorReady()

			step := "creating a DC"
			k := kubectl.ApplyFiles(dcYaml)
			ns.ExecAndLog(step, k)

			step = "Deploying Pulsar"
			if err := shutil.RunV("../../bin/helm", "repo", "add", "datastax-pulsar", "https://datastax.github.io/pulsar-helm-chart"); err != nil {
				Fail(err.Error())
			}
			ns.ExecAndLog(step, k)
			if err := shutil.RunV("../../bin/helm", "repo", "update"); err != nil {
				Fail(err.Error())
			}
			ns.ExecAndLog(step, k)
			if err := shutil.RunV("../../bin/helm", "install", "--create-namespace", "-n", "pulsar", "-f", pulsarValues, "pulsar", "datastax-pulsar/pulsar"); err != nil {
				Fail(err.Error())
			}
			By("Waiting for all components ready")
			readyGetter := kubectl.Get("pods").
				WithFlag("selector", "app=cdc-testutil").
				WithFlag("selector", "component=proxy").
				WithFlag("namespace", "pulsar").
				FormatOutput("jsonpath={.items[0].status.conditions[?(@.type=='Ready')].status}")
			err = kubectl.WaitForOutputContains(readyGetter, "True", 1800)
			Expect(err).To(Not(HaveOccurred()))

			ns.WaitForDatacenterReadyWithTimeouts(dcName, 1200, 1200)

			By("Creating a testutils deployment")
			step = "Deploying TestUtils"
			k = kubectl.ApplyFiles(testUtilsDeployment)
			ns.ExecAndLog(step, k)

			By("Confirming testutils ready")
			readyGetter = kubectl.Get("pods").
				WithFlag("selector", "app=cdc-testutil").
				FormatOutput("jsonpath={.items[0].status.conditions[?(@.type=='Ready')].status}")
			err = ns.WaitForOutputContains(readyGetter, "True", 1800)
			Expect(err).To(Not(HaveOccurred()))

			By("Running testutils applications")
			podGetter := kubectl.Get("pods").
				WithFlag("selector", "app=cdc-testutil").
				WithFlag("namespace", namespace).
				FormatOutput("jsonpath='{.items[0].metadata.name}'")
			testUtilsPod := podGetter.OutputPanic()
			testCommand := kubectl.
				ExecOnPod(
					strings.ReplaceAll(testUtilsPod, "'", ""),
					"--",
					"bash", "-c",
					"/opt/bin/pulsar-cdc-testutil --cass-contact-points test-cluster-dc1-all-pods-service.test-cdc.svc.cluster.local:9042 --pulsar-url pulsar://pulsar-proxy.pulsar.svc.cluster.local:6650 --pulsar-admin-url http://pulsar-proxy.pulsar.svc.cluster.local:8080 --pulsar-cass-contact-point test-cluster-dc1-all-pods-service.test-cdc.svc.cluster.local").
				InNamespace(namespace)
			if err := kubectl.WaitForOutputContains(testCommand, "SUCCESS", 1200); err != nil {
				Fail(err.Error())
			}
		})
	})
})
