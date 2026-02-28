// Copyright DataStax, Inc.
// Please see the included license file for details.

package node_replace

import (
	"fmt"
	"strings"
	"sync"
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
	testName         = "Node Replace"
	namespace        = "test-node-replace"
	dcName           = "dc1"
	podNames         = []string{"cluster1-dc1-r1-sts-0", "cluster1-dc1-r2-sts-0", "cluster1-dc1-r3-sts-0"}
	podNameToReplace = podNames[2]
	dcYaml           = "../testdata/default-three-rack-three-node-dc-4x.yaml"
	taskYaml         = "../testdata/tasks/replace_node_task.yaml"
	taskYamlRack     = "../testdata/tasks/replace_node_task_rack.yaml"
	dcResource       = fmt.Sprintf("CassandraDatacenter/%s", dcName)
	ns               = ginkgo_util.NewWrapper(testName, namespace)
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

func quotedList(stringArray []string) string {
	result := []string{}
	for _, s := range stringArray {
		result = append(result, fmt.Sprintf("'%s'", s))
	}

	return strings.Join(result, ",")
}

func duplicate(value string, count int) string {
	result := []string{}
	for i := 0; i < count; i++ {
		result = append(result, value)
	}

	return strings.Join(result, " ")
}

func DeleteIgnoreFinalizersAndLog(description string, resourceName string) {
	var wg sync.WaitGroup

	wg.Add(1)

	// Delete might hang due to a finalizer such as kubernetes.io/pvc-protection
	// so we run it asynchronously and then remove any finalizers to unblock it.
	go func() {
		defer wg.Done()
		k := kubectl.Delete(resourceName)
		ns.ExecAndLog(description, k)
	}()

	// Give the resource a second to get to a terminating state. Note that this
	// may not be reflected in the resource's status... hence the sleep here as
	// opposed to checking the status.
	time.Sleep(5 * time.Second)

	// In the case of PVCs at least, finalizers removed before deletion can be
	// automatically added back. Consequently, we delete the resource first,
	// then remove any finalizers while it is terminating.
	k := kubectl.PatchMerge(resourceName, `{"metadata":{"finalizers": null}}`)

	// Ignore errors as this may fail due to the resource already having been
	// deleted (which is what we want).
	_ = ns.ExecV(k)

	// Wait for the delete to finish, which should have been unblocked by
	// removing the finalizers.
	wg.Wait()
}

func verifyAllPodsAreCorrect() {
	step := "verify in nodetool that we still have the right number of cassandra nodes and correct HostID is reflected in CRD"
	By(step)
	for _, podName := range podNames {
		nodeInfos := ns.RetrieveStatusFromNodetool(podName)
		Expect(nodeInfos).To(HaveLen(len(podNames)), "Expect nodetool to return info on exactly %d nodes", len(podNames))

		for _, nodeInfo := range nodeInfos {
			Expect(nodeInfo.Status).To(Equal("up"), "Expected all nodes to be up, but node %s was down", nodeInfo.HostId)
			Expect(nodeInfo.State).To(Equal("normal"), "Expected all nodes to have a state of normal, but node %s was %s", nodeInfo.HostId, nodeInfo.State)

			// Make sure that NodeStatus reflects the HostID for the replacement pod. Otherwise subsequent replaces will fail as the CassandraDatacenter has stale information
			k := kubectl.Get("pod", podNameToReplace).FormatOutput("jsonpath={.status.podIP}")
			// step = "get podIP"
			podIP, err := ns.Output(k)
			Expect(err).ToNot(HaveOccurred(), "Failed to get pod IP for pod %s: %v", podNameToReplace, err)
			if podName == podNameToReplace && podIP == nodeInfo.Address {
				step = "verify nodeStatus HostID is up to date"
				json := fmt.Sprintf("jsonpath={.status.nodeStatuses['%s'].hostID}", podNameToReplace)
				hostIdInCassandraDatacenter := ns.OutputAndLog(step, kubectl.Get("cassandradatacenter", dcName).FormatOutput(json))
				Expect(nodeInfo.HostId).To(Equal(hostIdInCassandraDatacenter), "Expected HostId to be %s but got %s in CassandraDatacenter CRD", nodeInfo.HostId, hostIdInCassandraDatacenter)
			}
		}
	}
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
		Specify("the operator can replace a defunct cassandra node on pod start", func() {
			step := "ensure we actually recorded the host IDs for our cassandra nodes"
			json := fmt.Sprintf("jsonpath={.status.nodeStatuses[%s].hostID}", quotedList(podNames))
			k := kubectl.Get("cassandradatacenter", dcName).FormatOutput(json)
			ns.WaitForOutputPatternAndLog(step, k, duplicate(`[a-zA-Z0-9-]{36}`, len(podNames)), 60)

			step = "retrieve the persistent volume claim"
			json = "jsonpath={.spec.volumes[?(.name=='server-data')].persistentVolumeClaim.claimName}"
			k = kubectl.Get("pod", podNameToReplace).FormatOutput(json)
			pvcName := ns.OutputAndLog(step, k)

			step = "find PVC volume"
			json = "jsonpath={.spec.volumeName}"
			k = kubectl.Get("pvc", pvcName).FormatOutput(json)
			pvName, err := ns.Output(k)
			Expect(err).ToNot(HaveOccurred(), "Failed to get PV name")

			ns.DisableGossipWaitNotReady(podNameToReplace)
			ns.WaitForPodNotStarted(podNameToReplace)

			time.Sleep(1 * time.Minute)

			step = "patch CassandraDatacenter with appropriate replaceNodes setting"
			patch := fmt.Sprintf(`{"spec":{"replaceNodes":["%s"]}}`, podNameToReplace)
			k = kubectl.PatchMerge(dcResource, patch)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterCondition(dcName, "ReplacingNodes", string(corev1.ConditionTrue))

			step = "wait for the status to indicate we are replacing pods"
			json = "jsonpath={.status.nodeReplacements[0]}"
			k = kubectl.Get("cassandradatacenter", dcName).FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, podNameToReplace, 10)

			step = "kill the pod and its little persistent volume claim too"

			// We need to remove the PVC first as the statefulset controller will
			// recreate the pod as soon as it is deleted and we don't want the
			// resurrected pod to use the PVC we are taking to the gallows.
			DeleteIgnoreFinalizersAndLog(step, "pvc/"+pvcName)

			// Sanity check that the persistent volume got jettisoned with the
			// persistent volume claim.
			k = kubectl.Get("pv").WithFlag("field-selector", "metadata.name="+pvName)
			ns.WaitForOutputPanic(k, "", 30)

			// Now we can delete the pod. The statefulset controller _should_
			// create both a new pod and a new PVC for us.
			k = kubectl.Delete("pod", podNameToReplace)
			ns.ExecVPanic(k)

			// Ensure that all pods up and running when ReplacingNodes gets unset
			ns.WaitForDatacenterCondition(dcName, "ReplacingNodes", string(corev1.ConditionFalse))
			Expect(ns.GetDatacenterReadyPodNames(dcName)).To(HaveLen(3))

			step = "wait for the pod to return to life"
			json = "jsonpath={.status.containerStatuses[?(.name=='cassandra')].ready}"
			k = kubectl.Get("pod", podNameToReplace).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "true", 1200)

			// If we do this wrong and start the node we replaced normally (instead of setting the replace
			// flag), we will end up with an additional node in our cluster. This issue should be caught by
			// checking nodetool.
			verifyAllPodsAreCorrect()
		})
		Specify("cassandratask can be used to replace a node", func() {
			// Get PVC id
			step := "retrieve the persistent volume claim"
			json := "jsonpath={.spec.volumes[?(.name=='server-data')].persistentVolumeClaim.claimName}"
			k := kubectl.Get("pod", podNameToReplace).FormatOutput(json)
			pvcName, err := ns.Output(k)
			Expect(err).ToNot(HaveOccurred(), "Failed to get PVC name for pod %s: %v", podNameToReplace, err)

			step = "find PVC volume"
			json = "jsonpath={.spec.volumeName}"
			k = kubectl.Get("pvc", pvcName).FormatOutput(json)
			pvName, err := ns.Output(k)
			Expect(err).ToNot(HaveOccurred(), "Failed to get PV name for PVC %s: %v", pvcName, err)

			// Kill the Cassandra instance (emulate fsync failure or similar)
			ns.KillCassandra(podNameToReplace)

			// Create CassandraTask that should replace a node
			step = "creating a cassandra task to replace a node"
			k = kubectl.ApplyFiles(taskYaml)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterCondition(dcName, "ReplacingNodes", string(corev1.ConditionTrue))

			// Wait for the task to be completed
			ns.WaitForCompleteTask("replace-node")
			ns.WaitForDatacenterCondition(dcName, "ReplacingNodes", string(corev1.ConditionFalse))
			Expect(ns.GetDatacenterReadyPodNames(dcName)).To(HaveLen(3))

			step = "wait for the pod to return to life"
			json = "jsonpath={.status.containerStatuses[?(.name=='cassandra')].ready}"
			k = kubectl.Get("pod", podNameToReplace).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "true", 1200)

			verifyAllPodsAreCorrect()

			// Verify the PV id is different
			step = "retrieve the persistent volume claim after pod replace"
			json = "jsonpath={.spec.volumes[?(.name=='server-data')].persistentVolumeClaim.claimName}"
			k = kubectl.Get("pod", podNameToReplace).FormatOutput(json)
			pvcName, err = ns.Output(k)
			Expect(err).ToNot(HaveOccurred(), "Failed to get PVC name for pod %s: %v", podNameToReplace, err)

			step = "find PVC volume"
			json = "jsonpath={.spec.volumeName}"
			k = kubectl.Get("pvc", pvcName).FormatOutput(json)
			newPvName, err := ns.Output(k)
			Expect(err).ToNot(HaveOccurred(), "Failed to get PV name for PVC %s: %v", pvcName, err)
			Expect(pvName).ToNot(Equal(newPvName), "Expected PV volume to be different after node replace")
		})
		Specify("cassandratask can be used to replace a rack", func() {
			rackPodNames := []string{"cluster1-dc1-r1-sts-0", "cluster1-dc1-r1-sts-1"}
			oldPvByPod := make(map[string]string, len(rackPodNames))
			newPvByPod := make(map[string]string, len(rackPodNames))

			// This is to ensure we have enough nodes to see multiple replaces instead of just filtering change
			step := "scale up to 6 nodes"
			patch := `{"spec":{"size":6}}`
			k := kubectl.PatchMerge(dcResource, patch)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterCondition(dcName, "ScaleUp", string(corev1.ConditionFalse))
			Expect(ns.GetDatacenterReadyPodNames(dcName)).To(HaveLen(6))

			for _, podName := range rackPodNames {
				step = fmt.Sprintf("retrieve the persistent volume claim for %s", podName)
				json := "jsonpath={.spec.volumes[?(.name=='server-data')].persistentVolumeClaim.claimName}"
				k = kubectl.Get("pod", podName).FormatOutput(json)
				pvcName, err := ns.Output(k)
				Expect(err).ToNot(HaveOccurred(), "Failed to get PVC name for pod %s: %v", podName, err)

				step = fmt.Sprintf("find PVC volume for %s", podName)
				json = "jsonpath={.spec.volumeName}"
				k = kubectl.Get("pvc", pvcName).FormatOutput(json)
				oldPvByPod[podName], err = ns.Output(k)
				Expect(err).ToNot(HaveOccurred(), "Failed to get PV name for PVC %s: %v", pvcName, err)
			}

			step = "creating a cassandra task to replace nodes in rack r1"
			k = kubectl.ApplyFiles(taskYamlRack)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterCondition(dcName, "ReplacingNodes", string(corev1.ConditionTrue))

			ns.WaitForCompleteTaskTimeout("replace-node-rack", 720)
			ns.WaitForDatacenterCondition(dcName, "ReplacingNodes", string(corev1.ConditionFalse))
			Expect(ns.GetDatacenterReadyPodNames(dcName)).To(HaveLen(6))

			for _, podName := range rackPodNames {
				step = fmt.Sprintf("retrieve the persistent volume claim after replace for %s", podName)
				json := "jsonpath={.spec.volumes[?(.name=='server-data')].persistentVolumeClaim.claimName}"
				k = kubectl.Get("pod", podName).FormatOutput(json)
				pvcName, err := ns.Output(k)
				Expect(err).ToNot(HaveOccurred(), "Failed to get PVC name for pod %s: %v", podName, err)

				step = fmt.Sprintf("find PVC volume after replace for %s", podName)
				json = "jsonpath={.spec.volumeName}"
				k = kubectl.Get("pvc", pvcName).FormatOutput(json)
				newPvByPod[podName], err = ns.Output(k)
				Expect(err).ToNot(HaveOccurred(), "Failed to get PV name for PVC %s: %v", pvcName, err)
			}

			for _, podName := range rackPodNames {
				Expect(oldPvByPod[podName]).ToNot(Equal(newPvByPod[podName]),
					"Expected PV volume to be different after rack replace for pod %s", podName)
			}
		})
	})
})
