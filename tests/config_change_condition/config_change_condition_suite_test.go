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
	testName   = "Config change condition"
	namespace  = "test-config-change-condition"
	dcName     = "dc2"
	dcYaml     = "../testdata/default-single-rack-2-node-dc.yaml"
	dcResource = fmt.Sprintf("CassandraDatacenter/%s", dcName)
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
		Specify("the Updating condition is set for config updates", func() {
			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step := "creating a datacenter resource with 1 racks/2 nodes"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k := kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dcName)

			step = "change the config"
			json := ginkgo_util.CreateTestJson("{\"spec\": {\"config\": {\"cassandra-yaml\": {\"roles_validity\": \"256000ms\"}, \"jvm-server-options\": {\"garbage_collector\": \"CMS\"}}}}")
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterCondition(dcName, "Updating", string(corev1.ConditionTrue))
			ns.WaitForDatacenterCondition(dcName, "Updating", string(corev1.ConditionFalse))
			ns.WaitForDatacenterOperatorProgress(dcName, "Ready", 1800)
		})
	})
})
