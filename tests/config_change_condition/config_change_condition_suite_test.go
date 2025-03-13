// Copyright DataStax, Inc.
// Please see the included license file for details.

package config_change_condition

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
	testName    = "Config change condition with failure"
	namespace   = "test-config-change-condition"
	dcName      = "dc1"
	clusterName = "cluster1"
	dcYaml      = "../testdata/default-three-rack-three-node-dc-zones.yaml"
	dcResource  = fmt.Sprintf("CassandraDatacenter/%s", dcName)
	ns          = ginkgo_util.NewWrapper(testName, namespace)
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
		Specify("the Updating condition is set for config updates", func() {
			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step := "creating a datacenter resource with 3 racks/3 nodes using unavailable zones"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k := kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			// Wait for status to be Unschedulable
			step = "waiting the nodes to be unschedulable"
			json := `jsonpath={.status.conditions[?(@.type=="PodScheduled")].status}`
			k = kubectl.Get(fmt.Sprintf("pod/%s-%s-r1-sts-0", clusterName, dcName)).
				FormatOutput(json)
			Expect(ns.WaitForOutputContains(k, "False", 30)).ToNot(HaveOccurred())

			json = `jsonpath={.status.conditions[?(@.type=="PodScheduled")].reason}`
			k = kubectl.Get(fmt.Sprintf("pod/%s-%s-r1-sts-0", clusterName, dcName)).
				FormatOutput(json)
			ns.WaitForOutputContainsAndLog(step, k, "Unschedulable", 30)

			step = "change the config by removing zones"
			json = `{"spec": { "racks": [{"name": "r1"}, {"name": "r2"}, {"name": "r3"}]}}`
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterCondition(dcName, "Updating", string(corev1.ConditionTrue))
			ns.WaitForDatacenterCondition(dcName, "Updating", string(corev1.ConditionFalse))
			ns.WaitForDatacenterReady(dcName)
			ns.WaitForDatacenterOperatorProgress(dcName, "Ready", 1800)
		})
	})
})
