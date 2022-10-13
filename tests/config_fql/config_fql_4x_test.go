// Copyright DataStax, Inc.
// Please see the included license file for details.

package config_fql_4x

import (
	"fmt"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

var (
	testName  = "FQL config is applied on Cassandra 4x"
	namespace = "fql-config"
	dcName    = "dc1"
	dcYaml    = "../testdata/config_fql_4x_test.yaml"
	dcLabel   = fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)
	ns        = ginkgo_util.NewWrapper(testName, namespace)
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
	Context("when in a new cluster", func() {
		Specify("the operator can create a 4x datacenter where FQL is enabled", func() {
			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step := "creating a datacenter resource with 1 rack/1 node"
			k := kubectl.ApplyFiles(dcYaml)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReadyWithTimeouts(dcName, 1200, 1200)
			lsCmd := kubectl.KCmd{
				Command: "exec",
				Args:    []string{"cluster1-dc1-default-sts-0", "--", "ls", "/var/log/cassandra/fql"},
				Flags:   map[string]string{"container": "cassandra"},
			}
			Eventually(func() bool {
				res, _, err := ns.ExecVCapture(lsCmd)
				if err != nil {
					fmt.Println(err)
					Fail("error running ls on Cassandra container")
				}
				return (strings.Contains(res, ".cq4") && strings.Contains(res, "metadata.cq4t"))
			}, time.Second*60).Should(BeTrue())

			step = "deleting the dc"
			k = kubectl.DeleteFromFiles(dcYaml)
			ns.ExecAndLog(step, k)

			step = "checking that the dc no longer exists"
			json := "jsonpath={.items}"
			k = kubectl.Get("CassandraDatacenter").
				WithLabel(dcLabel).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)
		})
	})
})
