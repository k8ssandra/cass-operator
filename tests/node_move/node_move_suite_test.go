// Copyright DataStax, Inc.
// Please see the included license file for details.

package node_move

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
	testName  = "Node Move"
	namespace = "test-node-move"
	dcName    = "dc1"
	dcYaml    = "../testdata/single-token-three-rack-three-node-dc.yaml"
	taskYaml  = "../testdata/tasks/move_node_task.yaml"
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
		Specify("operator is installed and cluster is created", func() {
			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step := "creating a datacenter resource with 3 racks/3 nodes"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k := kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dcName)
		})
		Specify("the operator can move a cassandra node to a new token", func() {

			// Create CassandraTask that should move tokens
			step := "creating a cassandra task to move a node"
			k := kubectl.ApplyFiles(taskYaml)
			ns.ExecAndLog(step, k)

			// Wait for the task to be completed
			ns.WaitForCompleteTask("move-node")

			// Check that the nodes have been moved
			user, pw := ns.RetrieveSuperuserCreds("cluster1")

			k1 := kubectl.ExecOnPod(
				"cluster1-dc1-r1-sts-0", "--", "cqlsh",
				"--user", user,
				"--password", pw,
				"-e", "SELECT tokens FROM system.local;").
				WithFlag("container", "cassandra")
			k2 := kubectl.ExecOnPod(
				"cluster1-dc1-r2-sts-0", "--", "cqlsh",
				"--user", user,
				"--password", pw,
				"-e", "SELECT tokens FROM system.local;").
				WithFlag("container", "cassandra")
			k3 := kubectl.ExecOnPod(
				"cluster1-dc1-r3-sts-0", "--", "cqlsh",
				"--user", user,
				"--password", pw,
				"-e", "SELECT tokens FROM system.local;").
				WithFlag("container", "cassandra")

			ns.WaitForOutputContainsAndLog("check token on pod 1", k1, "-9223372036854775708", 30)
			ns.WaitForOutputContainsAndLog("check token on pod 2", k2, "-3074457345618258603", 30)
			ns.WaitForOutputContainsAndLog("check token on pod 3", k3, "3074457345618258602", 30)
		})
	})
})
