package decommission_dc

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
)

var (
	testName        = "Delete DC and verify it is correctly decommissioned in multi-dc cluster"
	namespace       = "test-decommission-dc"
	dc1Name         = "dc1"
	dc1OverrideName = "My_Super_Dc"
	dc2Name         = "dc2"
	dc1Yaml         = "../testdata/default-two-rack-two-node-dc.yaml"
	dc2Yaml         = "../testdata/default-two-rack-two-node-dc2.yaml"
	dc1Label        = fmt.Sprintf("cassandra.datastax.com/datacenter=%s", api.CleanupForKubernetes(dc1OverrideName))
	dc2Label        = fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dc2Name)
	seedLabel       = "cassandra.datastax.com/seed-node=true"
	taskYaml        = "../testdata/tasks/rebuild_task.yaml"
	// dcLabel   = fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)
	ns = ginkgo_util.NewWrapper(testName, namespace)
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

func execStatus(nodeName string) string {
	k := kubectl.ExecOnPod(nodeName, "-c", "cassandra", "--", "nodetool", "status")
	output := ns.OutputPanic(k)
	return output
}

func findDatacenters(nodeName string) []string {
	re := regexp.MustCompile(`Datacenter:.*`)
	output := execStatus(nodeName)
	dcLines := re.FindAllString(output, -1)
	dcs := make([]string, 0, len(dcLines))
	for _, dcLine := range dcLines {
		dcParts := strings.Split(dcLine, ": ")
		dcs = append(dcs, strings.TrimSpace(dcParts[1]))
	}

	return dcs
}

var _ = Describe(testName, func() {
	Context("when in a new cluster", func() {
		Specify("the operator runs decommission correctly when dc is deleted in multi-dc env", func() {
			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step := "creating the first datacenter resource with 1 rack/1 node"
			testFile1, err := ginkgo_util.CreateTestFile(dc1Yaml)
			Expect(err).ToNot(HaveOccurred())

			k := kubectl.ApplyFiles(testFile1)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dc1Name)

			By("get seed node IP address")
			json := "jsonpath={.items[0].status.podIP}"
			k = kubectl.Get("pods").
				WithLabel(seedLabel).
				FormatOutput(json)

			podIP, _, err := ns.ExecVCapture(k)
			Expect(err).ToNot(HaveOccurred())

			step = "creating the second datacenter resource with 1 rack/1 node"
			testFile2, err := ginkgo_util.CreateTestFile(dc2Yaml)
			Expect(err).ToNot(HaveOccurred())

			k = kubectl.ApplyFiles(testFile2)
			ns.ExecAndLog(step, k)

			// Add annotation to indicate we don't want the superuser created in dc2
			step = "annotate dc2 to prevent user creation"
			k = kubectl.Annotate("cassdc", dc2Name, "cassandra.datastax.com/skip-user-creation", "true")
			ns.ExecAndLog(step, k)

			dcResource := fmt.Sprintf("CassandraDatacenter/%s", dc2Name)
			step = "add seed node IP as additional seed for the new datacenter"
			json = fmt.Sprintf(`
			{
				"spec": {
					"additionalSeeds": ["%s"]
				}
			}`, podIP)

			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dc2Name)

			// We need to verify that reconciliation has stopped for both dcs
			ns.ExpectDoneReconciling(dc1Name)
			ns.ExpectDoneReconciling(dc2Name)

			// Create a CassandraTask to rebuild dc2 from dc1
			// Create CassandraTask that should replace a node
			step = "creating a cassandra rebuild dc2"
			k = kubectl.ApplyFiles(taskYaml)
			ns.ExecAndLog(step, k)

			// Wait for the task to be completed
			ns.WaitForCompleteTask("rebuild-dc")

			podNames := ns.GetDatacenterReadyPodNames(dc1OverrideName)
			Expect(len(podNames)).To(Equal(2))
			dcs := findDatacenters(podNames[0])

			Expect(len(dcs)).To(Equal(2), fmt.Sprintf("Expected to find 2 datacenters in nodetool status but found %v", dcs))

			step = "annotate dc2 to do decommission on delete"
			k = kubectl.Annotate("cassdc", dc2Name, "cassandra.datastax.com/decommission-on-delete", "true")
			ns.ExecAndLog(step, k)

			// Time to remove the dc2 and verify it has been correctly cleaned up
			step = "deleting the dc2"
			k = kubectl.DeleteFromFiles(testFile2)
			ns.ExecAndLog(step, k)

			step = "checking that the dc2 no longer exists"
			json = "jsonpath={.items}"
			k = kubectl.Get("CassandraDatacenter").
				WithLabel(dc2Label).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)

			// Verify nodetool status has only a single Datacenter
			podNames = ns.GetDatacenterReadyPodNames(dc1OverrideName)

			if len(podNames) != 2 {
				// This is to catch why the test sometimes fails on the check (string parsing? or real issue?)
				fmt.Println(execStatus(podNames[0]))
			}

			Expect(len(podNames)).To(Equal(2))
			dcs = findDatacenters(podNames[0])
			Expect(len(dcs)).To(Equal(1), fmt.Sprintf("Expected to find 1 datacenter in nodetool status but found %v", dcs))

			// Delete the remaining DC and expect it to finish correctly (it should not be decommissioned - that will hang the process and fail)
			step = "deleting the dc1"
			k = kubectl.DeleteFromFiles(testFile1)
			ns.ExecAndLog(step, k)

			step = "checking that the dc1 no longer exists"
			json = "jsonpath={.items}"
			k = kubectl.Get("CassandraDatacenter").
				WithLabel(dc1Label).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)

			step = "checking that no dc stateful sets remain"
			json = "jsonpath={.items}"
			k = kubectl.Get("statefulsets").
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)
		})
	})
})
