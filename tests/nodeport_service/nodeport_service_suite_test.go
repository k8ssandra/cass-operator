// Copyright DataStax, Inc.
// Please see the included license file for details.

package nodeport_service

import (
	"encoding/json"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	// corev1 "k8s.io/api/core/v1"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

var (
	testName                = "NodePort Service"
	namespace               = "test-node-port-service"
	dcName                  = "dc1"
	dcYaml                  = "../testdata/nodeport-service-dc.yaml"
	nodePortServiceResource = "services/cluster1-dc1-node-port-service"
	ns                      = ginkgo_util.NewWrapper(testName, namespace)
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

func checkNodePortService() {
	// Check the service

	k := kubectl.Get(nodePortServiceResource).FormatOutput("json")
	output := ns.OutputPanic(k)
	data := map[string]interface{}{}
	err := json.Unmarshal([]byte(output), &data)
	Expect(err).ToNot(HaveOccurred())

	err = json.Unmarshal([]byte(output), &data)
	Expect(err).ToNot(HaveOccurred())

	spec := data["spec"].(map[string]interface{})
	policy := spec["externalTrafficPolicy"].(string)
	Expect(policy).To(Equal("Local"), "Expected externalTrafficPolicy %s to be Local", policy)

	portData := spec["ports"].([]interface{})
	port0 := portData[0].(map[string]interface{})
	port1 := portData[1].(map[string]interface{})

	// for some reason, k8s is giving the port numbers back as floats
	ns.ExpectKeyValues(port0, map[string]string{
		"name":       "internode",
		"nodePort":   "30002.000000",
		"port":       "30002.000000",
		"targetPort": "30002.000000",
	})

	ns.ExpectKeyValues(port1, map[string]string{
		"name":       "native",
		"nodePort":   "30001.000000",
		"port":       "30001.000000",
		"targetPort": "30001.000000",
	})
}

var _ = Describe(testName, func() {
	Context("when in a new cluster", func() {
		Specify("the operator can properly create a nodeport service", func() {
			var step string
			var k kubectl.KCmd

			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step = "creating a datacenter resource with a nodeport service"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k = kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dcName)

			checkNodePortService()
		})
	})
})
