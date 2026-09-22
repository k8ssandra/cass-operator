// Copyright DataStax, Inc.
// Please see the included license file for details.

package bring_up_graph

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

var (
	testName  = "Bring Up Graph"
	namespace = "test-bring-up-graph"
	dcName    = "dc1"
	dcYaml    = "../testdata/graph-dc.yaml"
	ns        = ginkgo_util.NewWrapper(testName, namespace)
)

func TestLifecycle(t *testing.T) {
	ginkgo_util.RunTestLifecycle(t, testName, ns)
}

var _ = Describe(testName, func() {
	Context("when in a new cluster", func() {
		Specify("the operator bring up a graph node", func() {
			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step := "creating a datacenter resource with graph"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k := kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dcName)

			// graph takes a LONG time to come up in my environment
			ns.WaitForDatacenterOperatorProgress(dcName, "Ready", 300)
		})
	})
})
