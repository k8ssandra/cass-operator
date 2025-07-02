// Copyright DataStax, Inc.
// Please see the included license file for details.

package namespace_deletion

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

var (
	testName  = "Namespace deletion with CassandraDatacenter"
	namespace = "test-namespace-deletion"
	dcName    = "dc1"
	dcYaml    = "../testdata/default-three-rack-three-node-dc.yaml"
	dcLabel   = fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)
	ns        = ginkgo_util.NewWrapper(testName, namespace)
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
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, testName)
}

var _ = Describe(testName, func() {
	Context("when in a new cluster", func() {
		Specify("the namespace can be deleted successfully when it contains a CassandraDatacenter", func() {
			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step := "creating a datacenter resource with 3 racks/3 nodes"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k := kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dcName)

			step = "verifying that the datacenter is ready"
			json := "jsonpath={.status.cassandraOperatorProgress}"
			k = kubectl.Get("CassandraDatacenter", dcName).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "Ready", 60)

			step = "verifying that all pods are ready"
			json = "jsonpath={.items[*].status.containerStatuses[0].ready}"
			k = kubectl.Get("pods").
				WithLabel(dcLabel).
				WithFlag("field-selector", "status.phase=Running").
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "true true true", 120)

			step = "deleting the namespace containing the datacenter"
			k = kubectl.DeleteNamespace(namespace)
			ns.ExecAndLog(step, k)

			step = "verifying that the namespace deletion eventually succeeds"
			json = "jsonpath={.items[*].metadata.name}"
			k = kubectl.Get("namespaces").
				WithFlag("field-selector", fmt.Sprintf("metadata.name=%s", namespace)).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "", 600)
		})
	})
})
