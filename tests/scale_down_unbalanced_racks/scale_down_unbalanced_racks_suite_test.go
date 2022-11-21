// Copyright DataStax, Inc.
// Please see the included license file for details.

package scale_down_unbalanced_racks

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

var (
	testName   = "Scale down datacenter with unbalanced racks"
	namespace  = "test-scale-down-unbalanced-racks"
	dcName     = "dc1"
	dcYaml     = "../testdata/default-two-rack-four-node-dc.yaml"
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
		Specify("a datacenter can be scaled down with unbalanced racks", func() {
			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step := "creating a datacenter resource with 2 racks/4 nodes"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k := kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dcName)

			step = "scale up second rack to 3 nodes"
			json := "{\"spec\": {\"replicas\": 3}}"
			k = kubectl.PatchMerge("sts/cluster1-dc1-r2-sts", json)
			ns.ExecAndLog(step, k)

			extraPod := "cluster1-dc1-r2-sts-2"

			step = "check that the extra pod is ready"
			json = "jsonpath={.items[*].status.containerStatuses[0].ready}"
			k = kubectl.Get("pod").
				WithFlag("field-selector", fmt.Sprintf("metadata.name=%s", extraPod)).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "true", 600)

			// Kill all the pods in rack1

			step = "kill pod cluster1-dc1-r1-sts-0"
			k = kubectl.Delete("pod", "cluster1-dc1-r1-sts-0")
			ns.ExecAndLog(step, k)

			step = "kill pod cluster1-dc1-r1-sts-1"
			k = kubectl.Delete("pod", "cluster1-dc1-r1-sts-1")
			ns.ExecAndLog(step, k)

			// Scale down while we have pods down - expect this setup to recover

			step = "scale down dc to 2 nodes"
			json = "{\"spec\": {\"size\": 2}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			step = "wait for cluster1-dc1-r1-sts-0 to be ready again"
			json = "jsonpath={.items[*].status.containerStatuses[0].ready}"
			k = kubectl.Get("pod").
				WithFlag("field-selector", fmt.Sprintf("metadata.name=%s", "cluster1-dc1-r1-sts-0")).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "true", 600)

			expectedRemainingPods := []string{
				"cluster1-dc1-r1-sts-0", "cluster1-dc1-r1-sts-1",
				"cluster1-dc1-r2-sts-0",
			}
			ensurePodGetsDecommissionedNext("cluster1-dc1-r2-sts-1", expectedRemainingPods)

			expectedRemainingPods = []string{
				"cluster1-dc1-r1-sts-0",
				"cluster1-dc1-r2-sts-0",
			}
			ensurePodGetsDecommissionedNext("cluster1-dc1-r1-sts-1", expectedRemainingPods)

			ns.WaitForDatacenterCondition(dcName, "ScalingDown", string(corev1.ConditionFalse))
			ns.WaitForDatacenterOperatorProgress(dcName, "Ready", 180)

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

func ensurePodGetsDecommissionedNext(podName string, expectedRemainingPods []string) {
	step := fmt.Sprintf("check that pod %s status set to decommissioning", podName)
	json := "jsonpath={.items[*].metadata.name}"
	k := kubectl.Get("pod").
		WithLabel("cassandra.datastax.com/node-state=Decommissioning").
		FormatOutput(json)
	ns.WaitForOutputAndLog(step, k, podName, 120)

	step = "check the remaining pods haven't been decommissioned yet"
	json = "jsonpath={.items[*].metadata.name}"
	k = kubectl.Get("pod").
		WithLabel("cassandra.datastax.com/node-state=Started").
		FormatOutput(json)
	ns.WaitForOutputAndLog(step, k, strings.Join(expectedRemainingPods, " "), 10)

	step = fmt.Sprintf("check that pod %s got terminated", podName)
	json = "jsonpath={.items}"
	k = kubectl.Get("pod").
		WithFlag("field-selector", fmt.Sprintf("metadata.name=%s", podName)).
		FormatOutput(json)
	ns.WaitForOutputAndLog(step, k, "[]", 360)
}
