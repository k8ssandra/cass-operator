// Copyright DataStax, Inc.
// Please see the included license file for details.

package scheduled_task

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
	testName              = "ScheduledTask"
	namespace             = "test-scheduled-task"
	dcName                = "dc2"
	dcYaml                = "../testdata/default-single-rack-2-node-dc.yaml"
	scheduledTaskYaml     = "../testdata/tasks/scheduled_cleanup_task.yaml"
	scheduledTaskName     = "scheduled-cleanup-task"
	scheduledTaskResource = fmt.Sprintf("ScheduledTask/%s", scheduledTaskName)
	dcLabel               = fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)
	ns                    = ginkgo_util.NewWrapper(testName, namespace)
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
		Specify("the operator can schedule and execute CassandraTasks", func() {
			var step string
			var json string
			var k kubectl.KCmd

			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			step = "waiting for operator pod to be created"
			json = "jsonpath={.items[*].metadata.name}"
			k = kubectl.Get("pods").
				WithLabel("name=cass-operator").
				FormatOutput(json)
			ns.WaitForOutputContainsAndLog(step, k, "cass-operator", 300)

			ns.WaitForOperatorReady()

			step = "creating a datacenter resource with 2 nodes"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k = kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dcName)

			step = "creating a ScheduledTask for cleanup"
			k = kubectl.ApplyFiles(scheduledTaskYaml)
			ns.ExecAndLog(step, k)

			ns.WaitForCompletedCassandraTasks(dcName, "cleanup", 1)

			step = "verify CassandraTask references correct datacenter"
			json = "jsonpath={.items[0].spec.datacenter.name}"
			k = kubectl.Get("CassandraTask").
				WithLabel(dcLabel).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, dcName, 60)

			step = "verify LastExecution is updated after task completion"
			json = "jsonpath={.status.lastExecution}"
			k = kubectl.Get(scheduledTaskResource).FormatOutput(json)
			executionTime := ns.OutputAndLog(step, k)
			Expect(len(executionTime)).To(BeNumerically(">", 1))

			ns.WaitForCompletedCassandraTasks(dcName, "cleanup", 2)

			// Get all tasks and check that only one should be running at a time
			json = "jsonpath={.items[?(@.status.completionTime == \"\")].metadata.name}"
			k = kubectl.Get("CassandraTask").
				WithLabel(dcLabel).
				FormatOutput(json)

			Consistently(func() int {
				output, err := ns.Output(k)
				if err != nil {
					return 1000
				}
				if output == "" {
					return 0
				}
				runningTasks := strings.Fields(output)
				return len(runningTasks)
			}, time.Second*30, time.Second*5).Should(BeNumerically("<=", 1))

			// step = "verify ScheduledTask owner reference is set to CassandraDatacenter"
			json = "jsonpath={.metadata.ownerReferences[0].name}"
			k = kubectl.Get(scheduledTaskResource).FormatOutput(json)
			Expect(ns.WaitForOutput(k, dcName, 60)).ToNot(HaveOccurred())

			// step = "verify ScheduledTask owner reference kind is CassandraDatacenter"
			json = "jsonpath={.metadata.ownerReferences[0].kind}"
			k = kubectl.Get(scheduledTaskResource).FormatOutput(json)
			Expect(ns.WaitForOutput(k, "CassandraDatacenter", 60)).ToNot(HaveOccurred())

			step = "deleting the ScheduledTask"
			k = kubectl.DeleteFromFiles(scheduledTaskYaml)
			ns.ExecAndLog(step, k)

			step = "deleting the datacenter"
			k = kubectl.DeleteFromFiles(testFile)
			ns.ExecAndLog(step, k)

			step = "checking that the datacenter no longer exists"
			json = "jsonpath={.items}"
			k = kubectl.Get("CassandraDatacenter").
				WithLabel(dcLabel).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)
		})

	})
})
