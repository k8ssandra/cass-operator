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
)

// The datacenters are in different namespaces, so the operator does not see the pods of dc1 when reconciling dc2. This is
// the same situation as running each datacenter in its own Kubernetes cluster: once only the last dc2 node remains and it
// is decommissioning, there is no other pod to ask for the peers' metadata.
var (
	testName     = "Delete DC and verify it is correctly decommissioned in multi-dc cluster"
	opNamespace  = "cass-operator"
	dc1Namespace = "test-decommission-dc1"
	dc2Namespace = "test-decommission-dc2"
	dc1Name      = "dc1"
	dc2Name      = "dc2"
	dc1Yaml      = "../testdata/default-two-rack-two-node-dc.yaml"
	dc2Yaml      = "../testdata/default-two-rack-two-node-dc2.yaml"
	dc1Label     = fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dc1Name)
	dc2Label     = fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dc2Name)
	seedLabel    = "cassandra.datastax.com/seed-node=true"
	taskYaml     = "../testdata/tasks/rebuild_task.yaml"
	ns           = ginkgo_util.NewWrapper(testName, opNamespace)
	ns1          = ginkgo_util.NewWrapper(testName, dc1Namespace)
	ns2          = ginkgo_util.NewWrapper(testName, dc2Namespace)
)

func TestLifecycle(t *testing.T) {
	ginkgo_util.RunTestLifecycleWithCleanup(t, testName, ns, func() error {
		return kustomize.UndeployDir(opNamespace, "decommission_dc")
	}, dc1Namespace, dc2Namespace)
}

func execStatus(nodeName string) string {
	k := kubectl.ExecOnPod(nodeName, "-c", "cassandra", "--", "nodetool", "status")
	output := ns1.OutputPanic(k)
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
			By("creating a namespace for the cass-operator")
			err := kubectl.CreateNamespace(opNamespace).ExecV()
			Expect(err).ToNot(HaveOccurred())

			By("deploy cluster-wide cass-operator with kustomize")
			err = kustomize.DeployDir(opNamespace, "decommission_dc")
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			By("creating a namespace for each dc")
			Expect(kubectl.CreateNamespace(dc1Namespace).ExecV()).To(Succeed())
			Expect(kubectl.CreateNamespace(dc2Namespace).ExecV()).To(Succeed())

			step := "creating the first datacenter resource with 1 rack/1 node"
			testFile1, err := ginkgo_util.CreateTestFile(dc1Yaml)
			Expect(err).ToNot(HaveOccurred())

			k := kubectl.ApplyFiles(testFile1)
			ns1.ExecAndLog(step, k)

			ns1.WaitForDatacenterReady(dc1Name)

			By("get seed node IP address")
			json := "jsonpath={.items[0].status.podIP}"
			k = kubectl.Get("pods").
				WithLabel(seedLabel).
				FormatOutput(json)

			podIP, _, err := ns1.ExecVCapture(k)
			Expect(err).ToNot(HaveOccurred())

			step = "creating the second datacenter resource with 1 rack/1 node"
			testFile2, err := ginkgo_util.CreateTestFile(dc2Yaml)
			Expect(err).ToNot(HaveOccurred())

			k = kubectl.ApplyFiles(testFile2)
			ns2.ExecAndLog(step, k)

			// Add annotation to indicate we don't want the superuser created in dc2
			step = "annotate dc2 to prevent user creation"
			k = kubectl.Annotate("cassdc", dc2Name, "cassandra.datastax.com/skip-user-creation", "true")
			ns2.ExecAndLog(step, k)

			dcResource := fmt.Sprintf("CassandraDatacenter/%s", dc2Name)
			step = "add seed node IP as additional seed for the new datacenter"
			json = fmt.Sprintf(`
			{
				"spec": {
					"additionalSeeds": ["%s"]
				}
			}`, podIP)

			k = kubectl.PatchMerge(dcResource, json)
			ns2.ExecAndLog(step, k)

			ns2.WaitForDatacenterReady(dc2Name)

			// We need to verify that reconciliation has stopped for both dcs
			ns1.ExpectDoneReconciling(dc1Name)
			ns2.ExpectDoneReconciling(dc2Name)

			// Create a CassandraTask to rebuild dc2 from dc1
			// Create CassandraTask that should replace a node
			step = "creating a cassandra rebuild dc2"
			k = kubectl.ApplyFiles(taskYaml)
			ns2.ExecAndLog(step, k)

			// Wait for the task to be completed
			ns2.WaitForCompleteTask("rebuild-dc")

			podNames := ns1.GetDatacenterReadyPodNames(dc1Name)
			Expect(podNames).To(HaveLen(2))
			dcs := findDatacenters(podNames[0])

			Expect(dcs).To(HaveLen(2), fmt.Sprintf("Expected to find 2 datacenters in nodetool status but found %v", dcs))

			step = "annotate dc2 to do decommission on delete"
			k = kubectl.Annotate("cassdc", dc2Name, "cassandra.datastax.com/decommission-on-delete", "true")
			ns2.ExecAndLog(step, k)

			// Time to remove the dc2 and verify it has been correctly cleaned up
			step = "deleting the dc2"
			k = kubectl.DeleteFromFiles(testFile2)
			ns2.ExecAndLog(step, k)

			step = "checking that the dc2 no longer exists"
			json = "jsonpath={.items}"
			k = kubectl.Get("CassandraDatacenter").
				WithLabel(dc2Label).
				FormatOutput(json)
			ns2.WaitForOutputAndLog(step, k, "[]", 600)

			Eventually(func() bool {
				// Verify that the nodetool status has been updated
				podNames = ns1.GetDatacenterReadyPodNames(dc1Name)
				if len(podNames) != 2 {
					return false
				}
				dcs = findDatacenters(podNames[0])
				return len(dcs) == 1
			}, "15s", "100ms").Should(BeTrue(), func() string {
				return fmt.Sprintf("Expected to find 1 datacenter in nodetool status but found %v", dcs)
			})

			// Delete the remaining DC and expect it to finish correctly (it should not be decommissioned - that will hang the process and fail)
			step = "deleting the dc1"
			k = kubectl.DeleteFromFiles(testFile1)
			ns1.ExecAndLog(step, k)

			step = "checking that the dc1 no longer exists"
			json = "jsonpath={.items}"
			k = kubectl.Get("CassandraDatacenter").
				WithLabel(dc1Label).
				FormatOutput(json)
			ns1.WaitForOutputAndLog(step, k, "[]", 300)

			step = "checking that no dc stateful sets remain"
			json = "jsonpath={.items}"
			k = kubectl.Get("statefulsets").
				FormatOutput(json)
			ns1.WaitForOutputAndLog(step, k, "[]", 300)
			ns2.WaitForOutputAndLog(step, k, "[]", 300)
		})
	})
})
