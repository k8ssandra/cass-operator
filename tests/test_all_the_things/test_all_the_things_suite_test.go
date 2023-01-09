// Copyright DataStax, Inc.
// Please see the included license file for details.

package oss_test_all_the_things

import (
	"fmt"
	"regexp"
	"sync"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
)

var (
	testName       = "Test all the things"
	namespace      = "test-test-all-the-things"
	dcName         = "dc1"
	dcNameOverride = "my_super_dc"
	dcYaml         = "../testdata/default-two-rack-two-node-dc.yaml"
	dcResource     = fmt.Sprintf("CassandraDatacenter/%s", dcName)
	dcLabel        = fmt.Sprintf("cassandra.datastax.com/datacenter=%s", api.CleanupForKubernetes(dcNameOverride))
	ns             = ginkgo_util.NewWrapper(testName, namespace)
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
		Specify("the operator can scale up, stop, resume, and terminate a datacenter", func() {
			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step := "creating a datacenter resource with 2 racks/2 nodes"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k := kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			ns.WaitForSuperUserUpserted(dcName, 600)

			step = "check recorded host IDs"
			ns.Log(step)
			nodeStatusesHostIds := ns.GetNodeStatusesHostIds(dcName)
			Expect(len(nodeStatusesHostIds), 2)

			ns.WaitForDatacenterReadyWithOverride(dcName, dcNameOverride)
			ns.WaitForDatacenterCondition(dcName, "Ready", string(corev1.ConditionTrue))
			ns.WaitForDatacenterCondition(dcName, "Initialized", string(corev1.ConditionTrue))
			ns.ExpectDoneReconciling(dcName)

			step = "scale up to 4 nodes"
			json := "{\"spec\": {\"size\": 4}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterCondition(dcName, "ScalingUp", string(corev1.ConditionTrue))
			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 60)
			ns.WaitForDatacenterConditionWithTimeout(dcName, "ScalingUp", string(corev1.ConditionFalse), 1200)
			// Ensure that when 'ScaleUp' becomes 'false' that our pods are in fact up and running
			Expect(len(ns.GetDatacenterReadyPodNames(api.CleanupForKubernetes(dcNameOverride)))).To(Equal(4))

			ns.ExpectDoneReconciling(dcName)
			ns.WaitForDatacenterReadyWithOverride(dcName, dcNameOverride)

			step = "stopping the dc"
			json = "{\"spec\": {\"stopped\": true}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			// Ensure conditions set correctly for stopped
			ns.WaitForDatacenterCondition(dcName, "Stopped", string(corev1.ConditionTrue))
			ns.WaitForDatacenterCondition(dcName, "Ready", string(corev1.ConditionFalse))

			step = "checking the spec size hasn't changed"
			json = "jsonpath={.spec.size}"
			k = kubectl.Get(dcResource).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "4", 20)

			ns.WaitForDatacenterToHaveNoPods(api.CleanupForKubernetes(dcNameOverride))

			step = "resume the dc"
			json = "{\"spec\": {\"stopped\": false}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			// Ensure conditions set correctly for resuming
			ns.WaitForDatacenterCondition(dcName, "Stopped", string(corev1.ConditionFalse))
			ns.WaitForDatacenterCondition(dcName, "Resuming", string(corev1.ConditionTrue))

			ns.WaitForDatacenterReadyWithOverride(dcName, dcNameOverride)
			ns.ExpectDoneReconciling(dcName)

			logOutput := ""
			wg := &sync.WaitGroup{}
			wg.Add(1)
			go func() {
				k = kubectl.Logs("-f").
					WithLabel("statefulset.kubernetes.io/pod-name=cluster1-mysuperdc-r1-sts-0").
					WithFlag("container", "cassandra")
				output, err := ns.Output(k)
				Expect(err).ToNot(HaveOccurred())
				logOutput = output
				defer wg.Done()
			}()

			step = "deleting the dc"
			k = kubectl.DeleteFromFiles(testFile)
			ns.ExecAndLog(step, k)
			wg.Wait()

			// Check the log contains node/drain..
			Expect(regexp.MatchString("node/drain status=200 OK", logOutput)).To(BeTrue())

			step = "checking that the dc no longer exists"
			json = "jsonpath={.items}"
			k = kubectl.Get("CassandraDatacenter").
				WithLabel(dcLabel).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)

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
	})
})
