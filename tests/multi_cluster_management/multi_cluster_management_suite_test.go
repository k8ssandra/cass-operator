// Copyright DataStax, Inc.
// Please see the included license file for details.

package multi_cluster_management

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
	testName  = "Multi-cluster Management"
	namespace = "test-multi-cluster-management"
	dcNames   = [2]string{"dc1", "dc2"}
	dcYamls   = [2]string{"../testdata/default-three-rack-three-node-dc.yaml", "../testdata/default-single-rack-single-node-dc.yaml"}
	ns        = ginkgo_util.NewWrapper(testName, namespace)
)

func dcResourceForName(dcName string) string {
	return fmt.Sprintf("CassandraDatacenter/%s", dcName)
}

func dcLabelForName(dcName string) string {
	return fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)
}

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
		Specify("the operator manages multiple clusters in the same namespace", func() {
			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			step := "waiting for the operator to become ready"
			json := "jsonpath={.items[0].status.containerStatuses[0].ready}"
			k := kubectl.Get("pods").
				WithLabel("name=cass-operator").
				WithFlag("field-selector", "status.phase=Running").
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "true", 120)

			step = "creating a datacenter resource with 3 racks/3 nodes"
			testFile1, err := ginkgo_util.CreateTestFile(dcYamls[0])
			Expect(err).ToNot(HaveOccurred())

			k = kubectl.ApplyFiles(testFile1)
			ns.ExecAndLog(step, k)

			step = "creating another datacenter resource with 1 rack/1 node"
			testFile2, err := ginkgo_util.CreateTestFile(dcYamls[1])
			Expect(err).ToNot(HaveOccurred())

			k = kubectl.ApplyFiles(testFile2)
			ns.ExecAndLog(step, k)

			step = "waiting for the node to become ready"
			json = "jsonpath={.items[*].status.containerStatuses[0].ready}"
			k = kubectl.Get("pods").
				WithLabel(dcLabelForName(dcNames[0])).
				WithFlag("field-selector", "status.phase=Running").
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "true true true", 1200)

			step = "checking the cassandra operator progress status is set to Ready for first dc"
			json = "jsonpath={.status.cassandraOperatorProgress}"
			k = kubectl.Get(dcResourceForName(dcNames[0])).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "Ready", 30)

			step = "waiting for the nodes to become ready"
			json = "jsonpath={.items[*].status.containerStatuses[0].ready}"
			k = kubectl.Get("Pods").
				WithLabel(dcLabelForName(dcNames[1])).
				WithFlag("field-selector", "status.phase=Running").
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "true", 1200)

			step = "checking the cassandra operator progress status is set to Ready for second dc"
			json = "jsonpath={.status.cassandraOperatorProgress}"
			k = kubectl.Get(dcResourceForName(dcNames[1])).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "Ready", 30)

			step = "deleting the first dc"
			k = kubectl.DeleteFromFiles(testFile1)
			ns.ExecAndLog(step, k)

			step = "checking that the dc no longer exists"
			json = "jsonpath={.items}"
			k = kubectl.Get("CassandraDatacenter").
				WithLabel(dcLabelForName(dcNames[0])).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)

			step = "checking that no dc pods remain"
			json = "jsonpath={.items}"
			k = kubectl.Get("pods").
				WithLabel(dcLabelForName(dcNames[0])).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)

			step = "deleting the second dc"
			k = kubectl.DeleteFromFiles(testFile2)
			ns.ExecAndLog(step, k)

			step = "checking that the dc no longer exists"
			json = "jsonpath={.items}"
			k = kubectl.Get("CassandraDatacenter").
				WithLabel(dcLabelForName(dcNames[1])).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)

			step = "checking that no dc pods remain"
			json = "jsonpath={.items}"
			k = kubectl.Get("pods").
				WithLabel(dcLabelForName(dcNames[1])).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)
		})
	})
})
