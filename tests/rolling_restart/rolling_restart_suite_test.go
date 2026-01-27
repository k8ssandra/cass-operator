// Copyright DataStax, Inc.
// Please see the included license file for details.

package rolling_restart

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

var (
	testName     = "Rolling Restart"
	namespace    = "test-rolling-restart"
	dcName       = "dc2"
	dcYaml       = "../testdata/default-single-rack-2-node-dc.yaml"
	taskYaml     = "../testdata/tasks/rolling_restart.yaml"
	fastTaskYaml = "../testdata/tasks/rolling_restart_fast.yaml"
	dcResource   = fmt.Sprintf("CassandraDatacenter/%s", dcName)
	ns           = ginkgo_util.NewWrapper(testName, namespace)
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

			step := "creating a datacenter resource with 1 rack/2 node"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k := kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dcName)
		})
		Specify("the operator can perform a rolling restart with rollingRestartRequested spec change", func() {
			step := "trigger restart"
			json := `{"spec": {"rollingRestartRequested": true}}`
			k := kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			// Ensure we actually set the condition
			ns.WaitForDatacenterCondition(dcName, "RollingRestart", string(corev1.ConditionTrue))

			// Ensure we actually unset the condition
			ns.WaitForDatacenterCondition(dcName, "RollingRestart", string(corev1.ConditionFalse))

			// Once the RollingRestart condition becomes true, all the pods in the cluster
			// _should_ be ready
			step = "get ready pods"
			json = "jsonpath={.items[*].status.containerStatuses[0].ready}"
			k = kubectl.Get("pods").
				WithLabel(fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)).
				WithFlag("field-selector", "status.phase=Running").
				FormatOutput(json)

			Expect(ns.OutputAndLog(step, k)).To(Equal("true true"))

			ns.WaitForDatacenterReady(dcName)
		})
		Specify("cassandratask can be used to do rolling restart of the cluster", func() {
			step := "creating a cassandra task to do rolling restart"
			k := kubectl.ApplyFiles(taskYaml)
			ns.ExecAndLog(step, k)

			// Wait for the task to be completed
			ns.WaitForCompleteTask("rolling-restart")

			// Verify each pod does have the annotation..
			json := `jsonpath={.items[0].metadata.annotations.control\.k8ssandra\.io/restartedAt}`
			k = kubectl.Get("pods").
				WithLabel(fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)).
				WithFlag("field-selector", "status.phase=Running").
				FormatOutput(json)
			ns.WaitForOutputPatternAndLog(step, k, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`, 360)
		})
		Specify("cassandratask can be used with fast path", func() {
			// Scale the datacenter to 3 racks and 6 nodes
			step := "scaling datacenter to 3 racks"
			json := `{"spec":{"size":6,"racks":[{"name":"r1"},{"name":"r2"},{"name":"r3"}]}}`
			ns.ExecAndLog(step, kubectl.PatchMerge(dcResource, json))

			ns.WaitForDatacenterReady(dcName)

			step = "creating a cassandra task to do rolling restart (fast)"
			ns.ExecAndLog(step, kubectl.ApplyFiles(fastTaskYaml))

			// Wait for the task to be completed
			ns.WaitForCompleteTask("rolling-restart-fast")
			ns.WaitForDatacenterReady(dcName) // Should be instant check

			step = "verify all pods have restartedAt annotation"
			ns.Log(step)
			podNames := ns.GetDatacenterReadyPodNames(dcName)
			Expect(podNames).To(HaveLen(6), "Expected 6 ready pods")

			for _, podName := range podNames {
				json = `jsonpath={.metadata.annotations.control\.k8ssandra\.io/restartedAt}`
				k := kubectl.Get("pod", podName).FormatOutput(json)
				val, err := ns.Output(k)
				Expect(err).ToNot(HaveOccurred(), "Should be able to get restartedAt annotation for pod %s", podName)
				Expect(val).To(MatchRegexp(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`),
					fmt.Sprintf("Pod %s should have valid restartedAt annotation", podName))
			}

			step = "verify pods were restarted in correct order (rack by rack)"
			ns.Log(step)

			// Group pods by rack and collect their start times for comparison
			rackStartTimes := make(map[string][]time.Time)
			for _, podName := range podNames {
				json = `jsonpath={.metadata.labels.cassandra\.datastax\.com/rack}`
				k := kubectl.Get("pod", podName).FormatOutput(json)
				rackName, err := ns.Output(k)
				Expect(err).ToNot(HaveOccurred(), "Should be able to get rack label for pod %s", podName)
				Expect(rackName).ToNot(BeEmpty(), "Pod %s should have rack label", podName)

				// Parse the timestamp from pod's start time status
				json = `jsonpath={.status.startTime}`
				k = kubectl.Get("pod", podName).FormatOutput(json)
				startTimeStr, err := ns.Output(k)
				Expect(err).ToNot(HaveOccurred(), "Should be able to get startTime for pod %s", podName)
				Expect(startTimeStr).To(MatchRegexp(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`),
					fmt.Sprintf("Pod %s should have valid startTime", podName))
				startTime, err := time.Parse(time.RFC3339, startTimeStr)
				Expect(err).ToNot(HaveOccurred(), "Should be able to parse startTime timestamp for pod %s", podName)

				rackStartTimes[rackName] = append(rackStartTimes[rackName], startTime)
			}

			// Verify we have 3 racks with 2 pods each
			Expect(rackStartTimes).To(HaveLen(3), "Expected 3 racks")
			Expect(rackStartTimes["r1"]).To(HaveLen(2), "Expected 2 pods in rack r1")
			Expect(rackStartTimes["r2"]).To(HaveLen(2), "Expected 2 pods in rack r2")
			Expect(rackStartTimes["r3"]).To(HaveLen(2), "Expected 2 pods in rack r3")

			// Calculate the maximum start time for each rack (when the last pod in the rack started)
			getMaxTime := func(times []time.Time) time.Time {
				maxTime := times[0]
				for _, t := range times {
					if t.After(maxTime) {
						maxTime = t
					}
				}
				return maxTime
			}

			// Calculate the minimum start time for each rack (when the first pod in the rack started)
			getMinTime := func(times []time.Time) time.Time {
				minTime := times[0]
				for _, t := range times {
					if t.Before(minTime) {
						minTime = t
					}
				}
				return minTime
			}

			maxR1Time := getMaxTime(rackStartTimes["r1"])
			maxR2Time := getMaxTime(rackStartTimes["r2"])

			minR2Time := getMinTime(rackStartTimes["r2"])
			minR3Time := getMinTime(rackStartTimes["r3"])

			// Verify rack restart order: all pods in r1 should start before any pod in r2,
			// and all pods in r2 should start before any pod in r3
			Expect(maxR1Time.Before(minR2Time) || maxR1Time.Equal(minR2Time)).To(BeTrue(),
				fmt.Sprintf("Rack r1 should complete restart (max: %v) before rack r2 starts (min: %v)",
					maxR1Time, minR2Time))
			Expect(maxR2Time.Before(minR3Time) || maxR2Time.Equal(minR3Time)).To(BeTrue(),
				fmt.Sprintf("Rack r2 should complete restart (max: %v) before rack r3 starts (min: %v)",
					maxR2Time, minR3Time))

			ns.Log(fmt.Sprintf("Verified restart order - r1 max: %v, r2 min: %v, r2 max: %v, r3 min: %v",
				maxR1Time, minR2Time, maxR2Time, minR3Time))
		})
	})
})
