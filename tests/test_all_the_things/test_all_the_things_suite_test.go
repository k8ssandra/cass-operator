// Copyright DataStax, Inc.
// Please see the included license file for details.

package oss_test_all_the_things

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

var (
	testName   = "Test all the things"
	namespace  = "test-test-all-the-things"
	dcName     = "dc1"
	dcYaml     = "../testdata/oss-three-rack-three-node-dc.yaml"
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
		Specify("the operator can scale up, stop, resume, and terminate a datacenter", func() {
			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step := "creating a datacenter resource with 3 racks/3 nodes"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k := kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			ns.WaitForSuperUserUpserted(dcName, 600)

			step = "check recorded host IDs"
			ns.Log(step)
			nodeStatusesHostIds := ns.GetNodeStatusesHostIds(dcName)
			Expect(len(nodeStatusesHostIds), 3)

			ns.WaitForDatacenterReady(dcName)
			ns.WaitForDatacenterCondition(dcName, "Ready", string(corev1.ConditionTrue))
			ns.WaitForDatacenterCondition(dcName, "Initialized", string(corev1.ConditionTrue))

			step = "scale up to 4 nodes"
			json := "{\"spec\": {\"size\": 4}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 60)
			ns.WaitForDatacenterReady(dcName)

			step = "scale up to 5 nodes"
			json = "{\"spec\": {\"size\": 5}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 60)
			ns.WaitForDatacenterReady(dcName)

			step = "stopping the dc"
			json = "{\"spec\": {\"stopped\": true}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			step = "checking the spec size hasn't changed"
			json = "jsonpath={.spec.size}"
			k = kubectl.Get(dcResource).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "5", 20)

			ns.WaitForDatacenterToHaveNoPods(dcName)

			step = "resume the dc"
			json = "{\"spec\": {\"stopped\": false}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dcName)

			ns.ExpectDoneReconciling(dcName)

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
