// Copyright DataStax, Inc.
// Please see the included license file for details.

package upgrade_operator

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
	corev1 "k8s.io/api/core/v1"
)

var (
	testName   = "Upgrade Operator"
	namespace  = "test-upgrade-operator"
	dcName     = "dc1"
	podName    = "cluster1-my-super-dc-r1-sts-0"
	dcYaml     = "../testdata/default-three-rack-three-node-dc-4x.yaml"
	dcResource = fmt.Sprintf("CassandraDatacenter/%s", dcName)
	dcLabel    = fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)
	ns         = ginkgo_util.NewWrapper(testName, namespace)
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

// InstallOldOperator installs the oldest supported upgrade path (for Kubernetes 1.25)
func InstallOldOperator() {
	step := "install cass-operator 1.19.1"
	By(step)

	err := kustomize.DeployDir(namespace, "upgrade_operator")
	Expect(err).ToNot(HaveOccurred())
}

func UpgradeOperator() {
	step := "upgrade Cass Operator"
	By(step)

	// Install as usual.
	err := kustomize.Deploy(namespace)
	Expect(err).ToNot(HaveOccurred())
}

var _ = Describe(testName, func() {
	Context("when upgrading the Cass Operator", func() {
		Specify("the upgrade process from earlier operator-sdk should work", func() {
			By("creating a namespace")
			err := kubectl.CreateNamespace(namespace).ExecV()
			Expect(err).ToNot(HaveOccurred())

			InstallOldOperator()

			ns.WaitForOperatorReady()

			step := "creating a datacenter resource with 3 racks/3 node"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k := kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Ready", 1800)

			// Get UID of the cluster pod
			step = "get Cassandra pods UID"
			json := "jsonpath={.metadata.uid}"
			k = kubectl.GetByTypeAndName("pod", podName).
				FormatOutput(json)
			createdPodUID := ns.OutputAndLog(step, k)

			step = "get the previous Datacenter label value"
			json = `jsonpath={.metadata.labels.cassandra\.datastax\.com/datacenter}`
			k = kubectl.GetByTypeAndName("pod", podName).
				FormatOutput(json)
			createdPodDatacenterLabel := ns.OutputAndLog(step, k)

			Expect(createdPodDatacenterLabel).To(Equal("My_Super_Dc"))

			step = "get name of 1.19.1 operator pod"
			json = "jsonpath={.items[].metadata.name}"
			k = kubectl.Get("pods").WithFlag("selector", "name=cass-operator").FormatOutput(json)
			oldOperatorName := ns.OutputAndLog(step, k)

			UpgradeOperator()

			step = "wait for 1.19.1 operator pod to be removed"
			k = kubectl.Get("pods").WithFlag("field-selector", fmt.Sprintf("metadata.name=%s", oldOperatorName))
			ns.WaitForOutputAndLog(step, k, "", 60)

			ns.WaitForOperatorReady()

			// Let the new operator get the lock and start reconciling if it's going to
			time.Sleep(60 * time.Second)
			ns.WaitForDatacenterCondition(dcName, "Ready", string(corev1.ConditionTrue))
			ns.ExpectDoneReconciling(dcName)

			// Verify Pod hasn't restarted
			step = "get Cassandra pods UID"
			k = kubectl.GetByTypeAndName("pod", podName).FormatOutput("jsonpath={.metadata.uid}")
			postUpgradeCassPodUID := ns.OutputAndLog(step, k)

			Expect(createdPodUID).To(Equal(postUpgradeCassPodUID))

			// Verify PodDisruptionBudget is available (1.11 updates from v1beta1 -> v1)
			// json = "jsonpath={.items[].metadata.name}"
			// k = kubectl.Get("poddisruptionbudgets").WithLabel("cassandra.datastax.com/datacenter").FormatOutput(json)
			// err = ns.WaitForOutputContains(k, "dc1-pdb", 20)
			// Expect(err).ToNot(HaveOccurred())

			// Get current system-logger image
			// Verify the Pod now has updated system-logger container image
			step = "get Cassandra pod system-logger"
			k = kubectl.GetByTypeAndName("pod", podName).FormatOutput("jsonpath={.spec.containers[?(@.name == 'server-system-logger')].image}")
			loggerImage := ns.OutputAndLog(step, k)
			Expect(loggerImage).To(Equal("cr.k8ssandra.io/k8ssandra/system-logger:v1.19.1"))

			// Add annotation to allow upgrade to update the StatefulSets
			step = "add annotation to allow upgrade"
			json = "{\"metadata\": {\"annotations\": {\"cassandra.datastax.com/autoupdate-spec\": \"once\"}}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			// Wait for the operator to reconcile the datacenter
			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 60)
			ns.WaitForDatacenterReady(dcName)
			ns.ExpectDoneReconciling(dcName)

			// Verify pod has been restarted
			step = "get Cassandra pods UID"
			k = kubectl.GetByTypeAndName("pod", podName).FormatOutput("jsonpath={.metadata.uid}")
			postAllowUpgradeUID := ns.OutputAndLog(step, k)

			Expect(postUpgradeCassPodUID).ToNot(Equal(postAllowUpgradeUID))

			// Verify the Pod now has updated system-logger container image
			step = "get Cassandra pod system-logger"
			k = kubectl.GetByTypeAndName("pod", podName).FormatOutput("jsonpath={.spec.containers[?(@.name == 'server-system-logger')].image}")
			loggerImageNew := ns.OutputAndLog(step, k)
			Expect(loggerImage).To(Not(Equal(loggerImageNew)))

			// Verify the allow-upgrade=once annotation was removed from CassandraDatacenter
			step = "get CassandraDatacenter allow-upgrade annotation"
			k = kubectl.Get("CassandraDatacenter", dcName).FormatOutput("jsonpath={.metadata.annotations}")
			annotations := ns.OutputAndLog(step, k)
			Expect(annotations).To(Not(ContainSubstring("cassandra.datastax.com/autoupdate-spec")))

			// Verify the label is now updated
			step = "get the updated Datacenter label value"
			json = `jsonpath={.metadata.labels.cassandra\.datastax\.com/datacenter}`
			k = kubectl.GetByTypeAndName("pod", podName).
				FormatOutput(json)
			updatedPodDatacenterLabel := ns.OutputAndLog(step, k)

			Expect(updatedPodDatacenterLabel).To(Equal("dc1"))

			// Verify delete still works correctly and that we won't leave any resources behind
			step = "deleting the dc"
			k = kubectl.DeleteFromFiles(dcYaml)
			ns.ExecAndLog(step, k)

			step = "checking that the dc no longer exists"
			json = "jsonpath={.items}"
			k = kubectl.Get("CassandraDatacenter").
				WithLabel(dcLabel).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)

			step = "checking that the pvcs no longer exists"
			json = "jsonpath={.items}"
			k = kubectl.Get("pvc").
				WithLabel(dcLabel).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)

			step = "checking that nothing was left behind"
			json = "jsonpath={.items}"
			k = kubectl.Get("all").
				WithLabel(dcLabel).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)
		})
	})
})
