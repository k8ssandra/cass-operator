// Copyright DataStax, Inc.
// Please see the included license file for details.

package internode_secret_generated

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

var (
	testName          = "Internode Secret Generated"
	namespace         = "test-internode-secret-generated"
	defaultSecretName = "dc2-keystore"
	secretResource    = fmt.Sprintf("secret/%s", defaultSecretName)
	dcName            = "dc2"
	dcYaml            = "../testdata/encrypted-single-rack-2-node-dc.yaml"
	ns                = ginkgo_util.NewWrapper(testName, namespace)
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
	Context("when in a new cluster where internodeSecretName is unspecified", func() {
		Specify("the operator generates an appropriate internode secret", func() {
			var step string
			var k kubectl.KCmd

			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step = "creating a datacenter resource with 1 racks/2 nodes"
			k = kubectl.ApplyFiles(dcYaml)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dcName)

			// verify the secret was created
			step = "check that the internode secret was created"
			k = kubectl.Get(secretResource)
			ns.ExecAndLog(step, k)
		})
	})
})
