// Copyright DataStax, Inc.
// Please see the included license file for details.

package upgrade_operator

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

var (
	testName   = "Upgrade Operator"
	namespace  = "test-upgrade-operator"
	dcName     = "dc1"
	dcYaml     = "../testdata/operator-1.7.1-oss-dc.yaml"
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

// InstallOldOperator installs the oldest supported upgrade path (for Kubernetes 1.25)
func InstallOldOperator() {
	step := "install cass-operator 1.12.0"
	By(step)

	err := kustomize.DeployDir(namespace, "upgrade_operator")
	Expect(err).ToNot(HaveOccurred())
}

func UpgradeOperator() {
	step := "upgrade Cass Operator"
	By(step)

	// Install as usual.
	err := kustomize.Deploy(namespace)
	Expect(err).ToNot(HaveOccurred())
}

var _ = Describe(testName, func() {
	Context("when upgrading the Cass Operator", func() {
		Specify("the upgrade process from earlier operator-sdk should work", func() {
			By("creating a namespace")
			err := kubectl.CreateNamespace(namespace).ExecV()
			Expect(err).ToNot(HaveOccurred())

			InstallOldOperator()

			ns.WaitForOperatorReady()

			step := "creating a datacenter resource with 3 racks/3 node"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k := kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dcName)

			// Get UID of the cluster pod
			// step = "get Cassandra pods UID"
			// k = kubectl.Get("pod/cluster1-dc1-r1-sts-0").FormatOutput("jsonpath={.metadata.uid}")
			// createdPodUID := ns.OutputAndLog(step, k)

			step = "get name of 1.8.0 operator pod"
			json := "jsonpath={.items[].metadata.name}"
			k = kubectl.Get("pods").WithFlag("selector", "name=cass-operator").FormatOutput(json)
			oldOperatorName := ns.OutputAndLog(step, k)

			UpgradeOperator()

			step = "wait for 1.8.0 operator pod to be removed"
			k = kubectl.Get("pods").WithFlag("field-selector", fmt.Sprintf("metadata.name=%s", oldOperatorName))
			ns.WaitForOutputAndLog(step, k, "", 60)

			ns.WaitForOperatorReady()

			// give the operator a minute to reconcile and update the datacenter
			time.Sleep(1 * time.Minute)

			ns.WaitForDatacenterReadyWithTimeouts(dcName, 1200, 1200)

			ns.ExpectDoneReconciling(dcName)

			// Verify Pod hasn't restarted
			// step = "get Cassandra pods UID"
			// k = kubectl.Get("pod/cluster1-dc1-r1-sts-0").FormatOutput("jsonpath={.metadata.uid}")
			// postUpgradeCassPodUID := ns.OutputAndLog(step, k)

			// Expect(createdPodUID).To(Equal(postUpgradeCassPodUID))

			// Verify PodDisruptionBudget is available (1.11 updates from v1beta1 -> v1)
			json = "jsonpath={.items[].metadata.name}"
			k = kubectl.Get("poddisruptionbudgets").WithLabel("cassandra.datastax.com/datacenter").FormatOutput(json)
			err = ns.WaitForOutputContains(k, "dc1-pdb", 20)
			Expect(err).ToNot(HaveOccurred())

			// Update Cassandra version to ensure we can still do changes
			step = "perform cassandra upgrade"
			json = "{\"spec\": {\"serverVersion\": \"3.11.14\"}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 60)
			ns.WaitForDatacenterReadyWithTimeouts(dcName, 1200, 1200)
			ns.WaitForDatacenterReadyPodCount(dcName, 3)

			ns.ExpectDoneReconciling(dcName)

			// Verify delete still works correctly and that we won't leave any resources behind
			step = "deleting the dc"
			k = kubectl.DeleteFromFiles(testFile)
			ns.ExecAndLog(step, k)

			step = "checking that the dc no longer exists"
			json = "jsonpath={.items}"
			k = kubectl.Get("CassandraDatacenter").
				WithLabel(dcLabel).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)

			step = "checking that the pvcs no longer exists"
			json = "jsonpath={.items}"
			k = kubectl.Get("pvc").
				WithLabel(dcLabel).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)

			step = "checking that nothing was left behind"
			json = "jsonpath={.items}"
			k = kubectl.Get("all").
				WithLabel(dcLabel).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)

		})
	})
})
