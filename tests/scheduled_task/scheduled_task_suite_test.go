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
			ns.Log(step)

			k = kubectl.ApplyFiles(scheduledTaskYaml)
			stdout, stderr, err := ns.ExecVCapture(k)
			Expect(err).ToNot(HaveOccurred())
			fmt.Println(stdout)
			fmt.Println(stderr)

			step = "verify ScheduledTask is created"
			json = "jsonpath={.metadata.name}"
			k = kubectl.Get(scheduledTaskResource).FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, scheduledTaskName, 60)

			step = "verify ScheduledTask status fields are populated"
			ns.Log(step)
			json = "jsonpath={.status.nextSchedule}"
			k = kubectl.Get(scheduledTaskResource).FormatOutput(json)
			// Wait for nextSchedule to be set (non-empty)
			Eventually(func() string {
				output, err := ns.Output(k)
				if err != nil {
					return ""
				}
				return output
			}, time.Minute*2, time.Second*5).ShouldNot(BeEmpty())

			step = "verify ScheduledTask creates CassandraTasks"
			ns.Log(step)
			json = "jsonpath={.items[*].metadata.name}"
			k = kubectl.Get("CassandraTask").
				WithLabel(dcLabel).
				FormatOutput(json)

			// Wait for at least one CassandraTask to be created
			Eventually(func() string {
				output, err := ns.Output(k)
				if err != nil {
					return ""
				}
				return output
			}, time.Minute*5, time.Second*10).ShouldNot(BeEmpty())

			step = "verify CassandraTask has correct command"
			json = "jsonpath={.items[0].spec.jobs[0].command}"
			k = kubectl.Get("CassandraTask").
				WithLabel(dcLabel).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "cleanup", 60)

			step = "verify CassandraTask references correct datacenter"
			json = "jsonpath={.items[0].spec.datacenter.name}"
			k = kubectl.Get("CassandraTask").
				WithLabel(dcLabel).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, dcName, 60)

			step = "wait for first CassandraTask to complete"
			ns.Log(step)
			json = "jsonpath={.items[0].status.completionTime}"
			k = kubectl.Get("CassandraTask").
				WithLabel(dcLabel).
				FormatOutput(json)

			Eventually(func() string {
				output, err := ns.Output(k)
				if err != nil {
					return ""
				}
				return output
			}, time.Minute*5, time.Second*10).ShouldNot(BeEmpty())

			step = "verify LastExecution is updated after task completion"
			ns.Log(step)
			json = "jsonpath={.status.lastExecution}"
			k = kubectl.Get(scheduledTaskResource).FormatOutput(json)

			Eventually(func() string {
				output, err := ns.Output(k)
				if err != nil {
					return ""
				}
				return output
			}, time.Minute*2, time.Second*5).ShouldNot(BeEmpty())

			step = "verify multiple CassandraTasks are created over time with ForbidConcurrent policy"
			ns.Log(step)
			// Wait a bit more for the next scheduled execution

			// Should have at least 2 tasks created by now
			Eventually(func() int {
				json = "jsonpath={.items[*].metadata.name}"
				k = kubectl.Get("CassandraTask").
					WithLabel(dcLabel).
					FormatOutput(json)
				output, err := ns.Output(k)
				if err != nil {
					return 0
				}
				if output == "" {
					return 0
				}
				tasks := strings.Fields(output)
				return len(tasks)
			}, time.Minute*3, time.Second*10).Should(BeNumerically(">=", 2))

			step = "verify concurrency policy prevents overlapping tasks"
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

			step = "verify ScheduledTask owner reference is set to CassandraDatacenter"
			json = "jsonpath={.metadata.ownerReferences[0].name}"
			k = kubectl.Get(scheduledTaskResource).FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, dcName, 60)

			step = "verify ScheduledTask owner reference kind is CassandraDatacenter"
			json = "jsonpath={.metadata.ownerReferences[0].kind}"
			k = kubectl.Get(scheduledTaskResource).FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "CassandraDatacenter", 60)

			step = "deleting the ScheduledTask"
			k = kubectl.DeleteFromFiles(scheduledTaskYaml)
			ns.ExecAndLog(step, k)

			step = "verify ScheduledTask is deleted"
			json = "jsonpath={.items}"
			k = kubectl.Get("ScheduledTask").
				WithFlag("field-selector", fmt.Sprintf("metadata.name=%s", scheduledTaskName)).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)

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
