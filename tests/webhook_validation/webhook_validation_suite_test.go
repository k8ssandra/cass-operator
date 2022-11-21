// Copyright DataStax, Inc.
// Please see the included license file for details.

package webhook_validation

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
	testName   = "Cluster webhook validation test"
	namespace  = "test-webhook-validation"
	dcName     = "dc2"
	dcYaml     = "../testdata/default-single-rack-single-node-dc.yaml"
	dcResource = fmt.Sprintf("CassandraDatacenter/%s", dcName)
	dcLabel    = fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)
	ns         = ginkgo_util.NewWrapper(testName, namespace)
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
		Specify("the webhook disallows invalid updates", func() {
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

			step = "creating a datacenter resource with 1 racks/1 nodes"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k = kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			step = "waiting for the node to become ready"
			json = "jsonpath={.items[*].status.containerStatuses[0].ready}"
			k = kubectl.Get("pods").
				WithLabel(dcLabel).
				WithFlag("field-selector", "status.phase=Running").
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "true", 1200)

			step = "checking the cassandra operator progress status is set to Ready"
			json = "jsonpath={.status.cassandraOperatorProgress}"
			k = kubectl.Get(dcResource).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "Ready", 30)

			step = "attempt to use invalid dse version"
			json = "{\"spec\": {\"serverType\": \"dse\", \"serverVersion\": \"6.7.0\"}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLogAndExpectErrorString(step, k,
				`spec.serverVersion: Invalid value: "6.7.0": spec.serverVersion in body should match '(6\.8\.\d+)|(3\.11\.\d+)|(4\.\d+\.\d+)'`)
			step = "attempt to change the dc name"
			json = "{\"spec\": {\"clusterName\": \"NewName\"}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLogAndExpectErrorString(step, k,
				"change clusterName")

			step = "attempt to add rack without increasing size"
			json = `{"spec": {"racks": [{"name": "r1"}, {"name": "r2"}]}}`
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLogAndExpectErrorString(step, k,
				"add rack without increasing size")

			step = "attempt to add racks without increasing size enough to not reduce nodes on existing racks"
			json = `{"spec": {"size": 2,"racks": [{"name": "r1"}, {"name": "r2"}, {"name": "r3"}]}}`
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLogAndExpectErrorString(step, k,
				"add racks without increasing size enough"+
					" to prevent existing nodes from moving to new racks to maintain balance."+
					"\nNew racks added: 2, size increased by: 1. Expected size increase to be at least 2")

			step = "deleting the dc"
			k = kubectl.DeleteFromFiles(testFile)
			ns.ExecAndLog(step, k)

			step = "checking that the dc no longer exists"
			json = "jsonpath={.items}"
			k = kubectl.Get("CassandraDatacenter").
				WithLabel(dcLabel).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)
		})
	})
})
