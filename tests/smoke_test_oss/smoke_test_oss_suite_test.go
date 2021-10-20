// Copyright DataStax, Inc.
// Please see the included license file for details.

package smoke_test_oss

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
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

func createTestFile(cassandraVersion, serverImage string) (string, error) {
	var data map[interface{}]interface{}

	d, err := ioutil.ReadFile(dcYaml)
	if err != nil {
		return "", err
	}

	if err = yaml.Unmarshal(d, &data); err != nil {
		return "", err
	}

	// jvm-options <-> jvm-server-options

	spec := data["spec"].(map[interface{}]interface{})
	if serverImage != "" {
		spec["serverImage"] = serverImage
	}

	spec["serverVersion"] = cassandraVersion

	if strings.HasPrefix(cassandraVersion, "3.") {
		config := spec["config"].(map[interface{}]interface{})
		config["jvm-options"] = config["jvm-server-options"]
		delete(config, "jvm-server-options")
	}

	// Marshal back to temp file and return it
	testFilename := filepath.Join(os.TempDir(), "smoke-test-oss.yaml")
	os.Remove(testFilename) // Ignore the error

	updated, err := yaml.Marshal(data)
	if err != nil {
		return "", err
	}

	if err = ioutil.WriteFile(testFilename, updated, os.ModePerm); err != nil {
		return "", err
	}

	return testFilename, nil
}

var _ = Describe(testName, func() {
	var ns ginkgo_util.NsWrapper
	var namespace string
	var inputFilepath string

	singleClusterVerify := func() {
		Context("the operator can stand up a one node cluster", func() {
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
			kubectl.DumpAllLogs(logPath).ExecV()
			fmt.Printf("\n\tPost-run logs dumped at: %s\n\n", logPath)
		}
		ns.Terminate()
		kustomize.Undeploy(namespace)
	})

	type ServerDetails struct {
		ServerVersion string
		ServerImage   string
	}

	DescribeTable("the operator can stand up a one node cluster", func(s ServerDetails) {
		namespace = fmt.Sprintf("test-smoke-test-oss-%s", rand.String(6))

		err := kustomize.Deploy(namespace)
		Expect(err).ToNot(HaveOccurred())
		ns = ginkgo_util.NewWrapper(testName, namespace)

		inputFilepath, err = createTestFile(s.ServerVersion, s.ServerImage)
		Expect(err).ToNot(HaveOccurred())

		singleClusterVerify()
	},
		Entry("3.11.11", ServerDetails{
			ServerVersion: "3.11.11",
		}),

		Entry("4.0.0", ServerDetails{
			ServerVersion: "4.0.0",
		}),

		Entry("4.0.1", ServerDetails{
			ServerVersion: "4.0.1",
		}),

		Entry("3.11.7 k8ssandra 1.1", ServerDetails{
			ServerVersion: "3.11.7",
			ServerImage:   "k8ssandra/cass-management-api:3.11.7-v0.1.24",
		}),

		Entry("4.0.0 k8ssandra 1.3", ServerDetails{
			ServerVersion: "4.0.0",
			ServerImage:   "k8ssandra/cass-management-api:4.0.0-v0.1.28",
		}),
	)

})
