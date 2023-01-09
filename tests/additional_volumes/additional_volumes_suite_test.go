// Copyright DataStax, Inc.
// Please see the included license file for details.

package additional_volumes

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

var (
	testName  = "Additional volumes"
	namespace = "test-additional-volumes"
	dcName    = "dc2"
	dcYaml    = "../testdata/default-single-rack-single-node-addtional-volumes-dc.yaml"
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
		Specify("the operator can create a datacenter where nodes use additional volumes", func() {
			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step := "creating a datacenter resource with 1 rack/1 node"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k := kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReadyWithTimeouts(dcName, dcName, 600, 120)

			// Verify the volumes are actually there
			step = "retrieve the persistent volume claim"
			json := "jsonpath={.spec.volumes[?(.name=='cassandra-commitlogs')].persistentVolumeClaim.claimName}"
			k = kubectl.Get("pod", "cluster2-dc2-r1-sts-0").FormatOutput(json)
			pvcName := ns.OutputAndLog(step, k)
			Expect(strings.Contains(pvcName, "cassandra-commitlogs")).To(BeTrue())

			step = "find PVC volume"
			json = "jsonpath={.spec.volumeName}"
			k = kubectl.Get("pvc", pvcName).FormatOutput(json)
			pvName := ns.OutputAndLog(step, k)
			Expect(len(pvName) > 1).To(BeTrue())

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
