package decommission_dc

/*
	Create DC1, wait for it

	Create DC2 that starts with additional seeds set to the dc1's seed-service

	Verify DC1<->DC2 see each other

	Delete DC2, wait for it to complete

	Check that DC1 has no longer any DC2 linkage (the DC was correctly decommissioned)

	TODO Should we add scale up?
*/
import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

var (
	testName  = "Delete DC and verify it is correctly decommissioned in multi-dc cluster"
	namespace = "test-decommission-dc"
	dc1Name   = "dc1"
	dc2Name   = "dc2"
	dc1Yaml   = "../testdata/default-single-rack-2-node-dc1.yaml"
	dc2Yaml   = "../testdata/default-single-rack-2-node-dc.yaml"
	dc1Label  = fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dc1Name)
	dc2Label  = fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dc2Name)
	seedLabel = "cassandra.datastax.com/seed-node=true"
	// dcLabel   = fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)
	ns = ginkgo_util.NewWrapper(testName, namespace)
)

func TestLifecycle(t *testing.T) {
	AfterSuite(func() {
		logPath := fmt.Sprintf("%s/aftersuite", ns.LogDir)
		kubectl.DumpAllLogs(logPath).ExecV()
		fmt.Printf("\n\tPost-run logs dumped at: %s\n\n", logPath)
		ns.Terminate()
		kustomize.Undeploy(namespace)
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, testName)
}

func findDatacenters(nodeName string) []string {
	re := regexp.MustCompile(`Datacenter:.*`)
	k := kubectl.ExecOnPod(nodeName, "-c", "cassandra", "--", "nodetool", "status")
	output := ns.OutputPanic(k)
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
			k := kubectl.ApplyFiles(dc1Yaml)
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
			k = kubectl.ApplyFiles(dc2Yaml)
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

			podNames := ns.GetDatacenterReadyPodNames(dc1Name)
			Expect(len(podNames)).To(Equal(2))
			dcs := findDatacenters(podNames[0])
			Expect(len(dcs)).To(Equal(2))

			// Time to remove the dc2 and verify it has been correctly cleaned up
			step = "deleting the dc2"
			k = kubectl.DeleteFromFiles(dc2Yaml)
			ns.ExecAndLog(step, k)

			step = "checking that the dc2 no longer exists"
			json = "jsonpath={.items}"
			k = kubectl.Get("CassandraDatacenter").
				WithLabel(dc2Label).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)

			// Verify nodetool status has only a single Datacenter
			podNames = ns.GetDatacenterReadyPodNames(dc1Name)
			Expect(len(podNames)).To(Equal(2))
			dcs = findDatacenters(podNames[0])
			Expect(len(dcs)).To(Equal(1))

			// Delete the remaining DC and expect it to finish correctly (it should not be decommissioned - that will hang the process and fail)
			step = "deleting the dc1"
			k = kubectl.DeleteFromFiles(dc1Yaml)
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

			// Get seed-node Pod: "cassandra.datastax.com/seed-node"
			// kubectl get pods -l cassandra.datastax.com/seed-node=true --output=jsonpath='{.items[*].status.podIP}'

			/*
				Parse the datacenter lines and verify there's two

				➜  cass-operator git:(decommission_dc) ✗ kubectl exec -i -t -c cassandra cluster2-dc1-r1-sts-0 -- nodetool status
				Datacenter: dc1
				===============
				Status=Up/Down
				|/ State=Normal/Leaving/Joining/Moving
				--  Address     Load       Owns (effective)  Host ID                               Token                 Rack
				UN  10.244.2.3  97.09 KiB  100.0%            11e24d4a-a85f-4429-b701-3865c80bf887  3537893417023478360   r1

				Datacenter: dc2
				===============
				Status=Up/Down
				|/ State=Normal/Leaving/Joining/Moving
				--  Address     Load       Owns (effective)  Host ID                               Token                 Rack
				UN  10.244.6.3  68.72 KiB  100.0%            008e6938-3934-4ac2-9d03-2cd070f1cf1f  -8131327229031358013  r1

				➜  cass-operator git:(decommission_dc) ✗

				➜  cass-operator git:(decommission_dc) ✗ kubectl exec -i -t -c cassandra cluster2-dc2-r1-sts-0 -- nodetool decommission
				nodetool: Unsupported operation: Not enough live nodes to maintain replication factor in keyspace system_distributed (RF = 3, N = 2). Perform a forceful decommission to ignore.
			*/
			/*
							After decommission:
				➜  cass-operator git:(decommission_dc) ✗ kubectl exec -i -t -c cassandra cluster2-dc1-r1-sts-0 -- nodetool status
				Datacenter: dc1
				===============
				Status=Up/Down
				|/ State=Normal/Leaving/Joining/Moving
				--  Address     Load        Owns (effective)  Host ID                               Token                Rack
				UN  10.244.2.3  107.68 KiB  100.0%            11e24d4a-a85f-4429-b701-3865c80bf887  3537893417023478360  r1

				➜  cass-operator git:(decommission_dc) ✗
			*/
			/*
				Incorrect deletion:

				➜  cass-operator git:(decommission_dc) ✗ kubectl exec -i -t -c cassandra cluster2-dc1-r1-sts-0 -- nodetool status
				Datacenter: dc1
				===============
				Status=Up/Down
				|/ State=Normal/Leaving/Joining/Moving
				--  Address     Load        Owns (effective)  Host ID                               Token                Rack
				UN  10.244.2.3  112.79 KiB  100.0%            11e24d4a-a85f-4429-b701-3865c80bf887  3537893417023478360  r1

				Datacenter: dc2
				===============
				Status=Up/Down
				|/ State=Normal/Leaving/Joining/Moving
				--  Address     Load        Owns (effective)  Host ID                               Token                Rack
				DN  10.244.6.6  78.45 KiB   100.0%            fcdf813e-5861-4363-86f0-12e894b6e957  5934102370350368596  r1

				➜  cass-operator git:(decommission_dc) ✗
			*/
		})
	})
})
