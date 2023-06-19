// Copyright DataStax, Inc.
// Please see the included license file for details.

package config_change

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
	testName    = "Config change rollout"
	namespace   = "test-config-change-rollout"
	dcName      = "dc2"
	clusterName = "cluster2"
	dcYaml      = "../testdata/default-single-rack-single-node-dc.yaml"
	dcResource  = fmt.Sprintf("CassandraDatacenter/%s", dcName)
	dcLabel     = fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)
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
		Specify("the operator can scale up a datacenter", func() {
			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step := "creating a datacenter resource with 1 racks/1 node"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k := kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dcName)

			step = "scale up to 3 nodes"
			json := `{"spec": {"size": 3}}`
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 60)
			ns.WaitForDatacenterReady(dcName)

			step = "change the config"
			json = "{\"spec\": {\"config\": {\"cassandra-yaml\": {\"roles_validity_in_ms\": 256000, \"materialized_views_enabled\": \"true\"}}}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 60)
			ns.WaitForDatacenterCondition(dcName, "Updating", string(corev1.ConditionTrue))
			ns.WaitForDatacenterOperatorProgress(dcName, "Ready", 1800)
			ns.WaitForDatacenterCondition(dcName, "Updating", string(corev1.ConditionFalse))

			step = "checking that the init container got the updated config roles_validity_in_ms=256000"
			json = "jsonpath={.spec.initContainers[0].env[7].value}"
			k = kubectl.Get(fmt.Sprintf("pod/%s-%s-r1-sts-0", clusterName, dcName)).
				FormatOutput(json)
			ns.WaitForOutputContainsAndLog(step, k, "\"roles_validity_in_ms\":256000", 30)

			step = "checking that statefulsets have the right owner reference"
			json = "jsonpath={.metadata.ownerReferences[0].name}"
			k = kubectl.Get(fmt.Sprintf("sts/%s-%s-r1-sts", clusterName, dcName)).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, dcName, 30)

			step = "deleting the dc"
			k = kubectl.DeleteFromFiles(testFile)
			ns.ExecAndLog(step, k)

			step = "checking that the cassdc no longer exists"
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
