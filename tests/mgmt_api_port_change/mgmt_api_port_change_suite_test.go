// Copyright DataStax, Inc.
// Please see the included license file for details.

package mgmt_api_port_change

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

const managementApiPort = 8087

var (
	testName   = "Management API port change"
	namespace  = "test-mgmt-api-port-change"
	dcName     = "dc2"
	dcYaml     = "../testdata/default-single-rack-single-node-dc.yaml"
	dcResource = fmt.Sprintf("CassandraDatacenter/%s", dcName)
	podName    = "cluster2-dc2-r1-sts-0"
	ns         = ginkgo_util.NewWrapper(testName, namespace)
)

func TestLifecycle(t *testing.T) {
	ginkgo_util.RunTestLifecycle(t, testName, ns)
}

var _ = Describe(testName, func() {
	Context("when a datacenter is running", func() {
		Specify("the operator changes the management API port", func() {
			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step := "creating a datacenter resource with 1 rack/1 node"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k := kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dcName)

			step = fmt.Sprintf("changing the management API port to %d", managementApiPort)
			patch := fmt.Sprintf(`{"spec":{"podTemplateSpec":{"spec":{"containers":[{"name":"cassandra","ports":[{"name":"mgmt-api-http","containerPort":%d}]}]}}}}`, managementApiPort)
			k = kubectl.PatchMerge(dcResource, patch)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 60)
			ns.WaitForDatacenterReady(dcName)

			step = "checking the management API container port on the running pod"
			json := `jsonpath={.spec.containers[?(@.name=='cassandra')].ports[?(@.name=='mgmt-api-http')].containerPort}`
			k = kubectl.Get(fmt.Sprintf("pod/%s", podName)).FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, fmt.Sprintf("%d", managementApiPort), 30)

			step = "checking the management API listen port environment variable"
			json = `jsonpath={.spec.containers[?(@.name=='cassandra')].env[?(@.name=='MGMT_API_LISTEN_TCP_PORT')].value}`
			k = kubectl.Get(fmt.Sprintf("pod/%s", podName)).FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, fmt.Sprintf("%d", managementApiPort), 30)

			step = fmt.Sprintf("calling the management API on port %d", managementApiPort)
			k = kubectl.ExecOnPod(
				podName,
				"-c", "cassandra", "--",
				"curl", "-s", "-o", "/dev/null", "--show-error", "--fail",
				fmt.Sprintf("http://localhost:%d/api/v0/probes/liveness", managementApiPort),
			)
			ns.ExecAndLog(step, k)
		})
	})
})
