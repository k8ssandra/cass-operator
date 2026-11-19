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
	ns          = ginkgo_util.NewWrapper(testName, namespace)
)

func TestLifecycle(t *testing.T) {
	ginkgo_util.RunTestLifecycle(t, testName, ns)
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
			ns.AssertDatacenterInvariants(dcName)

			step = "scale up to 3 nodes"
			json := `{"spec": {"size": 3}}`
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 60)
			ns.WaitForDatacenterReady(dcName)
			ns.AssertDatacenterInvariants(dcName)

			before := ns.GetDatacenterState(dcName)
			sampler := ns.SampleAvailability(dcName)

			step = "change the config"
			json = "{\"spec\": {\"config\": {\"cassandra-yaml\": {\"roles_validity\": \"256000ms\", \"materialized_views_enabled\": \"true\"}}}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 60)
			ns.WaitForDatacenterCondition(dcName, "Updating", string(corev1.ConditionTrue))
			ns.WaitForDatacenterOperatorProgress(dcName, "Ready", 1800)
			ns.AssertDatacenterInvariants(dcName)
			sampler.Stop()
			ns.ExpectSameHostIDs(dcName, before)
			for _, pod := range ns.GetDatacenterPodNames(dcName) {
				ns.ExpectCassandraSetting(pod, clusterName, "materialized_views_enabled", `(?m)^\s*true\s*$`)
				ns.ExpectCassandraSetting(pod, clusterName, "roles_validity", `(?m)^\s*(256000ms|256s)\s*$`)
			}

			step = "checking that statefulsets have the right owner reference"
			json = "jsonpath={.metadata.ownerReferences[0].name}"
			k = kubectl.Get(fmt.Sprintf("sts/%s-%s-r1-sts", clusterName, dcName)).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, dcName, 30)

			step = "deleting the dc"
			k = kubectl.DeleteFromFiles(testFile)
			ns.ExecAndLog(step, k)
			ns.AssertDatacenterDeleted(dcName)
		})
	})
})
