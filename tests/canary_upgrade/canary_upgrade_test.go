// Copyright DataStax, Inc.
// Please see the included license file for details.

package canary_upgrade

import (
	"fmt"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	"github.com/k8ssandra/cass-operator/pkg/images"
	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

var (
	testName   = "OSS test canary upgrade"
	namespace  = "test-canary-upgrade"
	dcName     = "dc1"
	dcYaml     = "../testdata/oss-upgrade-dc.yaml"
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

var _ = Describe(testName, func() {
	Context("when in a new cluster", func() {
		Specify("the operator can perform a canary upgrade", func() {
			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step := "creating a datacenter"
			k := kubectl.ApplyFiles(dcYaml)
			ns.ExecAndLog(step, k)

			ns.WaitForSuperUserUpserted(dcName, 600)
			ns.WaitForDatacenterReady(dcName)

			step = "check recorded host IDs"
			ns.Log(step)
			nodeStatusesHostIds := ns.GetNodeStatusesHostIds(dcName)
			Expect(nodeStatusesHostIds).To(HaveLen(3))

			ns.WaitForDatacenterCondition(dcName, "Initialized", string(corev1.ConditionTrue))

			step = "prepare for canary upgrade"
			json := "{\"spec\": {\"canaryUpgrade\": true, \"canaryUpgradeCount\": 1}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			step = "perform canary upgrade"
			json = "{\"spec\": {\"serverVersion\": \"4.0.7\"}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 30)
			ns.WaitForDatacenterReadyPodCount(dcName, 3)

			imageConfigFile := filepath.Join("..", "..", "config", "manager", "image_config.yaml")
			err = images.ParseImageConfig(imageConfigFile)
			Expect(err).ToNot(HaveOccurred())

			old, _ := images.GetCassandraImage("cassandra", "4.0.1")
			updated, _ := images.GetCassandraImage("cassandra", "4.0.7")

			images := []string{
				old,
				old,
				updated,
			}
			ns.WaitForCassandraImages(dcName, images, 300)
			ns.WaitForDatacenterReadyPodCount(dcName, 3)

			// TODO Verify that after the canary upgrade we can issue upgrades to the rest of the nodes
			step = "remove canary upgrade"
			json = "{\"spec\": {\"canaryUpgrade\": false}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 30)
			ns.WaitForDatacenterReadyPodCount(dcName, 3)

			images = []string{
				updated,
				updated,
				updated,
			}
			ns.WaitForCassandraImages(dcName, images, 300)

			step = "deleting the dc"
			k = kubectl.DeleteFromFiles(dcYaml)
			ns.ExecAndLog(step, k)

			step = "checking that the dc no longer exists"
			json = "jsonpath={.items}"
			k = kubectl.Get("CassandraDatacenter").
				WithLabel(dcLabel).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)
		})
	})
})
