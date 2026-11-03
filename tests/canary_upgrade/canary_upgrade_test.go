// Copyright DataStax, Inc.
// Please see the included license file for details.

package canary_upgrade

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	ginkgo_util.RunTestLifecycle(t, testName, ns)
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
			ns.AssertDatacenterInvariants(dcName)

			step = "check recorded host IDs"
			ns.Log(step)
			nodeStatusesHostIds := ns.GetNodeStatusesHostIds(dcName)
			Expect(nodeStatusesHostIds).To(HaveLen(3))

			ns.WaitForDatacenterCondition(dcName, "Initialized", string(corev1.ConditionTrue))

			step = "prepare for canary upgrade"
			json := "{\"spec\": {\"canaryUpgrade\": true, \"canaryUpgradeCount\": 1}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			before := ns.GetDatacenterState(dcName)
			sampler := ns.SampleAvailability(dcName)

			step = "perform canary upgrade"
			json = "{\"spec\": {\"serverVersion\": \"4.0.7\"}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 30)
			ns.WaitForDatacenterReadyPodCount(dcName, 3)

			imageConfigFile := filepath.Join("..", "..", "config", "imageconfig", "image_config.yaml")
			b, err := os.ReadFile(imageConfigFile)
			Expect(err).ToNot(HaveOccurred())
			registry, err := images.NewImageRegistryV2(b)
			Expect(err).ToNot(HaveOccurred())

			old, _ := registry.GetCassandraImage("cassandra", "4.0.1")
			updated, _ := registry.GetCassandraImage("cassandra", "4.0.7")

			images := []string{
				strings.Replace(old, "docker.io/", "ghcr.io/", 1),
				strings.Replace(old, "docker.io/", "ghcr.io/", 1),
				strings.Replace(updated, "docker.io/", "ghcr.io/", 1),
			}
			ns.WaitForCassandraImages(dcName, images, 300)
			ns.WaitForDatacenterReadyPodCount(dcName, 3)
			ns.WaitForDatacenterOperatorProgress(dcName, "Ready", 600)
			// Only the canary pod is on the update revision
			ns.AssertDatacenterInvariants(dcName, ginkgo_util.SkipStatefulSetRevisionCheck())
			sampler.Stop()

			step = "checking that only the canary pod was restarted"
			ns.Log(step)
			afterCanary := ns.GetDatacenterState(dcName)
			restarted := []string{}
			for pod, uid := range before.PodUIDs {
				if afterCanary.PodUIDs[pod] != uid {
					restarted = append(restarted, pod)
				}
			}
			Expect(restarted).To(HaveLen(1), "pods restarted by the canary upgrade: %v", restarted)
			ns.ExpectSameHostIDs(dcName, before)

			sampler = ns.SampleAvailability(dcName)

			// TODO Verify that after the canary upgrade we can issue upgrades to the rest of the nodes
			step = "remove canary upgrade"
			json = "{\"spec\": {\"canaryUpgrade\": false}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 30)
			ns.WaitForDatacenterReadyPodCount(dcName, 3)

			images = []string{
				strings.Replace(updated, "docker.io/", "ghcr.io/", 1),
				strings.Replace(updated, "docker.io/", "ghcr.io/", 1),
				strings.Replace(updated, "docker.io/", "ghcr.io/", 1),
			}
			ns.WaitForCassandraImages(dcName, images, 300)
			ns.WaitForDatacenterReady(dcName)
			ns.AssertDatacenterInvariants(dcName)
			sampler.Stop()
			ns.ExpectSameHostIDs(dcName, before)

			step = "deleting the dc"
			k = kubectl.DeleteFromFiles(dcYaml)
			ns.ExecAndLog(step, k)
			ns.AssertDatacenterDeleted(dcName)

			step = "checking that the dc no longer exists"
			json = "jsonpath={.items}"
			k = kubectl.Get("CassandraDatacenter").
				WithLabel(dcLabel).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)
		})
	})
})
