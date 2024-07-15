package pvc_expansion

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

var (
	testName   = "PVC Expansion"
	namespace  = "pvc-expansion"
	dcName     = "dc1"
	dcYaml     = "../testdata/default-single-rack-single-node-dc-lvm.yaml"
	podName    = "cluster1-dc1-r1-sts-0"
	dcResource = fmt.Sprintf("CassandraDatacenter/%s", dcName)
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

var _ = Describe(testName, func() {
	Context("when in a new cluster", func() {
		Specify("operator is installed and cluster is created", func() {
			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step := "creating a datacenter resource with 1 rack/1 node"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k := kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dcName)
		})
		Specify("user is able to expand the existing server-data", func() {
			step := "retrieve the persistent volume claim"
			json := "jsonpath={.spec.volumes[?(.name=='server-data')].persistentVolumeClaim.claimName}"
			k := kubectl.Get("pod", podName).FormatOutput(json)
			pvcName := ns.OutputAndLog(step, k)

			step = "find PVC volume"
			json = "jsonpath={.spec.volumeName}"
			k = kubectl.Get("pvc", pvcName).FormatOutput(json)
			pvName := ns.OutputAndLog(step, k)

			step = "find the PV volume size"
			json = "jsonpath={.spec.capacity.storage}"
			k = kubectl.Get("pv", pvName).FormatOutput(json)
			existingPvSize := ns.OutputAndLog(step, k)
			Expect(existingPvSize).To(Equal("1Gi"), "Expected PV size to be 1Gi but got %s", existingPvSize)

			step = "patch CassandraDatacenter to increase the StorageConfig size"
			patch := fmt.Sprintf(`{"spec":{"storageConfig":{"cassandraDataVolumeClaimSpec":{"resources":{"requests":{"storage":"%s"}}}}}}`, "2Gi")
			k = kubectl.PatchMerge(dcResource, patch)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterCondition(dcName, "ResizingVolumes", string(corev1.ConditionTrue))
			ns.WaitForDatacenterReady(dcName)
			ns.WaitForDatacenterCondition(dcName, "ResizingVolumes", string(corev1.ConditionFalse))

			step = "find the PV volume size"
			json = "jsonpath={.spec.capacity.storage}"
			k = kubectl.Get("pv", pvName).FormatOutput(json)
			pvSize := ns.OutputAndLog(step, k)

			Expect(pvSize).To(Equal("2Gi"), "Expected PV size to be 2Gi but got %s", pvSize)
		})
	})
})
