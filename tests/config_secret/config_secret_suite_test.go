package config_secret

import (
	"fmt"
	"testing"

	"github.com/k8ssandra/cass-operator/tests/kustomize"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

var (
	testName          = "Config change rollout with config secret"
	namespace         = "test-config-change-rollout-secret"
	dcName            = "dc1"
	dcYaml            = "../testdata/cluster-with-config-secret.yaml"
	dcResource        = fmt.Sprintf("CassandraDatacenter/%s", dcName)
	secretYaml        = "../testdata/test-config-secret.yaml"
	updatedSecretYaml = "../testdata/updated-test-config-secret.yaml"
	ns                = ginkgo_util.NewWrapper(testName, namespace)
)

func TestLifecycle(t *testing.T) {
	ginkgo_util.RunTestLifecycle(t, testName, ns)
}

var _ = Describe(testName, func() {
	Context("when in a new cluster", func() {
		Specify("cassandra configuration can be applied with a secret", func() {
			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step := "creating config secret"
			testSecretFile, err := ginkgo_util.CreateTestSecretsConfig(secretYaml)
			Expect(err).ToNot(HaveOccurred())
			k := kubectl.ApplyFiles(testSecretFile)
			ns.ExecAndLog(step, k)

			step = "creating datacenter"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k = kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)
			ns.WaitForDatacenterReadyWithTimeouts(dcName, 420, 30)
			ns.AssertDatacenterInvariants(dcName)

			before := ns.GetDatacenterState(dcName)
			sampler := ns.SampleAvailability(dcName)

			step = "update config secret"
			testUpdatedSecretFile, err := ginkgo_util.CreateTestSecretsConfig(updatedSecretYaml)
			Expect(err).ToNot(HaveOccurred())
			k = kubectl.ApplyFiles(testUpdatedSecretFile)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 60)
			ns.WaitForDatacenterOperatorProgress(dcName, "Ready", 1200)

			ns.AssertDatacenterInvariants(dcName)
			sampler.Stop()
			ns.ExpectSameHostIDs(dcName, before)
			for _, pod := range ns.GetDatacenterPodNames(dcName) {
				ns.ExpectCassandraSetting(pod, "cluster1", "read_request_timeout", `(?m)^\s*10000ms\s*$`)
			}

			before = ns.GetDatacenterState(dcName)
			sampler = ns.SampleAvailability(dcName)

			step = "stop using config secret"
			json := ginkgo_util.CreateTestJson(`[{"op": "remove", "path": "/spec/configSecret"}, {"op": "add", "path": "/spec/config", "value": {"cassandra-yaml": {"read_request_timeout": "25000ms"}, "jvm-server-options": {"initial_heap_size": "512M", "max_heap_size": "512M"}}}]`)
			k = kubectl.PatchJson(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 120)
			ns.WaitForDatacenterOperatorProgress(dcName, "Ready", 1200)

			ns.AssertDatacenterInvariants(dcName)
			sampler.Stop()
			ns.ExpectSameHostIDs(dcName, before)
			for _, pod := range ns.GetDatacenterPodNames(dcName) {
				ns.ExpectCassandraSetting(pod, "cluster1", "read_request_timeout", `(?m)^\s*25000ms\s*$`)
			}

			before = ns.GetDatacenterState(dcName)
			sampler = ns.SampleAvailability(dcName)

			step = "use config secret again"
			json = `{"spec": {"config": null, "configSecret": "test-config"}}`
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 120)
			ns.WaitForDatacenterOperatorProgress(dcName, "Ready", 1200)

			ns.AssertDatacenterInvariants(dcName)
			sampler.Stop()
			ns.ExpectSameHostIDs(dcName, before)
			for _, pod := range ns.GetDatacenterPodNames(dcName) {
				ns.ExpectCassandraSetting(pod, "cluster1", "read_request_timeout", `(?m)^\s*10000ms\s*$`)
			}
		})
	})
})
