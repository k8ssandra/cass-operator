// Copyright DataStax, Inc.
// Please see the included license file for details.

package test_bad_config_and_fix

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

var (
	testName   = "test rolling out a bad config and fixing it"
	namespace  = "test-bad-config-and-fix"
	dcName     = "dc1"
	dcYaml     = "../testdata/oss-three-rack-three-node-dc.yaml"
	dcResource = fmt.Sprintf("CassandraDatacenter/%s", dcName)
	dcLabel    = fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)
	ns         = ginkgo_util.NewWrapper(testName, namespace)
)

func TestLifecycle(t *testing.T) {
	ginkgo_util.RunTestLifecycle(t, testName, ns)
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

			ns.WaitForDatacenterReady(dcName)
			ns.AssertDatacenterInvariants(dcName)
			ns.WaitForDatacenterCondition(dcName, "Ready", string(corev1.ConditionTrue))
			ns.WaitForDatacenterCondition(dcName, "Initialized", string(corev1.ConditionTrue))

			before := ns.GetDatacenterState(dcName)
			// The racks that are not being forced must stay available
			sampler := ns.SampleAvailability(dcName)

			step = "apply a bad image"
			json := "{\"spec\": {\"serverImage\": \"datastax/cassandra-v314159\"}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 30)

			time.Sleep(time.Minute * 6)
			ns.WaitForDatacenterReadyPodCount(dcName, 2)

			step = "apply a good image"
			json = "{\"spec\": {\"serverImage\": \"\"}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			step = "set the forceUpgradeRacks config"
			json = "{\"spec\": {\"forceUpgradeRacks\": [\"r1\"]}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dcName)
			ns.AssertDatacenterInvariants(dcName)
			sampler.Stop()
			ns.ExpectSameHostIDs(dcName, before)

			// A configuration Cassandra refuses to start with: the pod runs, but the start fails
			badConfigStart := time.Now()
			sampler = ns.SampleAvailability(dcName)
			step = "apply a bad Cassandra configuration"
			json = `{"spec": {"config": {"cassandra-yaml": {"concurrent_reads": 1}}}}`
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 60)

			step = "checking that the failed start is recorded in status.failedStarts"
			ns.Log(step)
			Eventually(func() []string {
				return ns.GetDatacenter(dcName).Status.FailedStarts
			}).WithTimeout(15*time.Minute).WithPolling(10*time.Second).ShouldNot(BeEmpty(), "no pod in status.failedStarts")
			ns.WaitForWarningEvent(dcName, badConfigStart, "StartingCassandra", "Failed to start pod", 2*time.Minute)

			step = "remove the bad Cassandra configuration and force the update of r1"
			json = `{"spec": {"config": {"cassandra-yaml": {"concurrent_reads": null}}, "forceUpgradeRacks": ["r1"]}}`
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)
			// The Warning events of the failed start were checked above
			ns.ResetEventCheckpoint(dcName)

			ns.WaitForDatacenterReady(dcName)
			ns.AssertDatacenterInvariants(dcName)
			sampler.Stop()
			ns.ExpectSameHostIDs(dcName, before)

			step = "checking that statefulsets have the right owner reference"
			json = "jsonpath={.metadata.ownerReferences[0].name}"
			k = kubectl.Get("sts/cluster1-dc1-r1-sts").
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "dc1", 30)

			step = "deleting the dc"
			k = kubectl.DeleteFromFiles(testFile)
			ns.ExecAndLog(step, k)
			ns.AssertDatacenterDeleted(dcName)

			step = "checking that the dc no longer exists"
			json = "jsonpath={.items}"
			k = kubectl.Get("CassandraDatacenter").
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 60)

			step = "checking that the statefulsets no longer exists"
			json = "jsonpath={.items}"
			k = kubectl.Get("StatefulSet").
				WithLabel(dcLabel).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 60)
		})
	})
})
