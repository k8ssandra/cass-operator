// Copyright DataStax, Inc.
// Please see the included license file for details.

package smoke_test_oss

import (
	"fmt"
	"testing"

	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/rand"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

var (
	testName = "Smoke test of basic functionality for one-node OSS Cassandra cluster."
	// namespace = "test-smoke-test-oss"
	dcName  = "dc2"
	dcYaml  = "../testdata/smoke-test-oss.yaml"
	dcLabel = fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)
)

func TestLifecycle(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, testName)
}

var _ = Describe(testName, func() {
	var ns ginkgo_util.NsWrapper
	var namespace string
	var inputFilepath string

	singleClusterVerify := func() {
		Specify("the operator can stand up a one node cluster", func() {
			step := "waiting for the operator to become ready"
			json := "jsonpath={.items[0].status.containerStatuses[0].ready}"
			k := kubectl.Get("pods").
				WithLabel("name=cass-operator").
				WithFlag("field-selector", "status.phase=Running").
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "true", 360)

			step = "creating a datacenter resource with 1 rack/1 node"
			k = kubectl.ApplyFiles(inputFilepath)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dcName)
			ns.ExpectDoneReconciling(dcName)

			step = "deleting the dc"
			k = kubectl.DeleteFromFiles(inputFilepath)
			ns.ExecAndLog(step, k)

			step = "checking that the dc no longer exists"
			json = "jsonpath={.items}"
			k = kubectl.Get("CassandraDatacenter").
				WithLabel(dcLabel).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 600)

			step = "checking that no dc pods remain"
			json = "jsonpath={.items}"
			k = kubectl.Get("pods").
				WithLabel(dcLabel).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 600)

			step = "checking that no dc services remain"
			json = "jsonpath={.items}"
			k = kubectl.Get("services").
				WithLabel(dcLabel).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 600)

			step = "checking that no dc stateful sets remain"
			json = "jsonpath={.items}"
			k = kubectl.Get("statefulsets").
				WithLabel(dcLabel).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 600)
		})
	}

	AfterEach(func() {
		if GinkgoT().Failed() {
			logPath := fmt.Sprintf("%s/aftersuite", ns.LogDir)
			fmt.Printf("\n\tPost-run logs dumped at: %s\n\n", logPath)
			err := kubectl.DumpAllLogs(logPath).ExecV()
			if err != nil {
				GinkgoT().Logf("Failed to dump all the logs: %v", err)
			}
		}
		ns.Terminate()
		err := kustomize.Undeploy(namespace)
		if err != nil {
			GinkgoT().Logf("Failed to undeploy cass-operator: %v", err)
		}
	})

	Context("the operator can stand up a one node cluster", func() {
		namespace = fmt.Sprintf("test-smoke-test-oss-%s", rand.String(6))

		err := kustomize.Deploy(namespace)
		Expect(err).ToNot(HaveOccurred())
		ns = ginkgo_util.NewWrapper(testName, namespace)

		inputFilepath, err = ginkgo_util.CreateTestFile(dcYaml)
		Expect(err).ToNot(HaveOccurred())

		singleClusterVerify()
	})
})
