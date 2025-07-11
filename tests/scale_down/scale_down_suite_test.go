// Copyright DataStax, Inc.
// Please see the included license file for details.

package scale_down

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
	testName   = "Scale down datacenter"
	namespace  = "test-scale-down"
	dcName     = "dc1"
	dcYaml     = "../testdata/default-three-rack-four-node-dc.yaml"
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
		Specify("a datacenter can be scaled down", func() {
			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step := "creating a datacenter resource with 3 racks/4 nodes"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k := kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dcName)

			step = "scale down to 3 nodes"
			json := "{\"spec\": {\"size\": 3}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterCondition(dcName, "ScalingDown", string(corev1.ConditionTrue))

			podWithDecommissionedNode := "cluster1-dc1-r1-sts-1"
			podPvcName := "server-data-cluster2-dc1-r1-sts-1"

			step = "check node status set to decommissioning"
			json = "jsonpath={.items[*].metadata.name}"
			k = kubectl.Get("pod").
				WithLabel("cassandra.datastax.com/node-state=Decommissioning").
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, podWithDecommissionedNode, 90)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 30)
			ns.WaitForDatacenterCondition(dcName, "ScalingDown", string(corev1.ConditionFalse))
			ns.WaitForDatacenterOperatorProgress(dcName, "Ready", 360)

			step = "check that the decomm'd pod got terminated"
			json = "jsonpath={.items}"
			k = kubectl.Get("pod").
				WithFlag("field-selector", fmt.Sprintf("metadata.name=%s", podWithDecommissionedNode)).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 30)

			step = "check that the decomm'd pod's PVCs got terminated"
			json = "jsonpath={.items}"
			k = kubectl.Get("pvc").
				WithFlag("field-selector", fmt.Sprintf("metadata.name=%s", podPvcName)).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 30)

			step = "ensure that the node status got deleted for decommissioned pod"
			json = "jsonpath={.status.nodeStatuses['cluster1-dc1-r1-sts-1']}"
			k = kubectl.Get("CassandraDatacenter", dcName).FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "", 30)

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
