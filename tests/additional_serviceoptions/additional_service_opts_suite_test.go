// Copyright DataStax, Inc.
// Please see the included license file for details.

package additional_serviceoptions

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

var (
	testName  = "Additional service options"
	namespace = "test-additional-service-options"
	dcName    = "dc2"
	dcYaml    = "../testdata/additional-service-annotations-and-labels.yaml"
	ns        = ginkgo_util.NewWrapper(testName, namespace)
)

func TestLifecycle(t *testing.T) {
	ginkgo_util.RunTestLifecycle(t, testName, ns)
}

var _ = Describe(testName, func() {
	Context("when in a new cluster", func() {
		Specify("the operator can create a datacenter where services have additional properties", func() {
			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step := "creating a datacenter resource with 1 rack/1 node"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k := kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReadyWithTimeouts(dcName, 600, 120)
			ns.AssertDatacenterInvariants(dcName)

			step = "verify service has additional label and annotation"
			k = kubectl.Get("svc").WithLabel("test=additional-serviceoptions").FormatOutput(`jsonpath={.items[0].metadata.annotations.external-dns\.alpha\.kubernetes\.io/hostname}`)
			ns.WaitForOutputAndLog(step, k, "localhost", 300)

			step = "deleting the dc"
			k = kubectl.DeleteFromFiles(testFile)
			ns.ExecAndLog(step, k)
			ns.AssertDatacenterDeleted(dcName)
		})
	})
})
