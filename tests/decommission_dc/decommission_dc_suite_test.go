package decommission_dc

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/k8ssandra/cass-operator/pkg/events"
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
			ns1.AssertDatacenterInvariants(dc1Name)

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
			ns2.AssertDatacenterInvariants(dc2Name)

			// dc1 must also have stopped reconciling after dc2 joined the cluster
			ns1.ExpectDoneReconciling(dc1Name)

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

			dc1Before := ns1.GetDatacenterState(dc1Name)
			dc1Sampler := ns1.SampleAvailability(dc1Name)

			// The Decommission condition must be visible on dc2 while it is being deleted
			decommissionSeen := make(chan bool, 1)
			stopWatching := make(chan struct{})
			go func() {
				defer GinkgoRecover()
				seen := false
				defer func() { decommissionSeen <- seen }()
				json := "jsonpath={.status.conditions[?(.type=='Decommission')].status}"
				for {
					if out, err := ns2.Output(kubectl.Get("CassandraDatacenter", dc2Name).FormatOutput(json)); err == nil && out == "True" {
						seen = true
						return
					}
					select {
					case <-stopWatching:
						return
					case <-time.After(time.Second):
					}
				}
			}()

			// Time to remove the dc2 and verify it has been correctly cleaned up
			step = "deleting the dc2"
			k = kubectl.DeleteFromFiles(testFile2)
			ns2.ExecAndLog(step, k)
			ns2.AssertDatacenterDeleted(dc2Name)
			close(stopWatching)
			Expect(<-decommissionSeen).To(BeTrue(), "the Decommission condition of dc2 was never True during its deletion")

			By("checking that a DecommissionDatacenter event was emitted for dc2")
			actions := []string{}
			for _, event := range ns2.EventsSince(dc2Name, corev1.EventTypeNormal, time.Time{}) {
				actions = append(actions, ginkgo_util.EventAction(&event))
			}
			Expect(actions).To(ContainElement(events.DecommissionDatacenter))

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

			// dc1 must not have been restarted by the decommission of dc2
			dc1Sampler.Stop()
			ns1.AssertDatacenterInvariants(dc1Name)
			Expect(ns1.GetDatacenterState(dc1Name).PodUIDs).To(Equal(dc1Before.PodUIDs), "dc1 pods were restarted by the deletion of dc2")

			// Delete the remaining DC and expect it to finish correctly (it should not be decommissioned - that will hang the process and fail)
			step = "deleting the dc1"
			k = kubectl.DeleteFromFiles(testFile1)
			ns1.ExecAndLog(step, k)
			ns1.AssertDatacenterDeleted(dc1Name)
		})
	})
})
