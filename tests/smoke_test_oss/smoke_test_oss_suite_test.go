// Copyright DataStax, Inc.
// Please see the included license file for details.

package smoke_test_oss

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	. "github.com/onsi/ginkgo/v2"
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

func createTestFile(cassandraVersion, serverImage, serverType string) (string, error) {
	var data map[interface{}]interface{}

	d, err := os.ReadFile(dcYaml)
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

	if serverType != "" {
		spec["serverType"] = serverType
	}

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

	if err = os.WriteFile(testFilename, updated, os.ModePerm); err != nil {
		return "", err
	}

	return testFilename, nil
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

	type ServerDetails struct {
		ServerVersion string
		ServerImage   string
		ServerType    string
	}

	Context("the operator can stand up a one node cluster", func() {
		serverVersion := os.Getenv("M_SERVER_VERSION")
		serverImage := os.Getenv("M_SERVER_IMAGE")
		serverType := os.Getenv("M_SERVER_TYPE")
		s := ServerDetails{
			ServerVersion: serverVersion,
			ServerImage:   serverImage,
			ServerType:    serverType,
		}
		namespace = fmt.Sprintf("test-smoke-test-oss-%s", rand.String(6))

		err := kustomize.Deploy(namespace)
		Expect(err).ToNot(HaveOccurred())
		ns = ginkgo_util.NewWrapper(testName, namespace)

		inputFilepath, err = createTestFile(s.ServerVersion, s.ServerImage, s.ServerType)
		Expect(err).ToNot(HaveOccurred())

		singleClusterVerify()
	})
})
