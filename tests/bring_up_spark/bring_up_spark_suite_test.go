// Copyright DataStax, Inc.
// Please see the included license file for details.

package bring_up_spark

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

var (
	testName  = "Bring Up Spark"
	namespace = "test-bring-up-spark"
	dcName    = "dc1"
	dcYaml    = "../testdata/spark-dc.yaml"
	ns        = ginkgo_util.NewWrapper(testName, namespace)
)

func TestLifecycle(t *testing.T) {
	ginkgo_util.RunTestLifecycle(t, testName, ns)
}

var _ = Describe(testName, func() {
	Context("when in a new cluster", func() {
		Specify("the operator bring up a spark node", func() {
			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step := "creating a datacenter resource with spark"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k := kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dcName)

			// spark takes a LONG time to come up in my environment
			ns.WaitForDatacenterOperatorProgress(dcName, "Ready", 2000)
		})
	})
})
