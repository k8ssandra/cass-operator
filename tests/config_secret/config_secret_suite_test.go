package config_secret

import (
	"fmt"
	"testing"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/tests/kustomize"
	corev1 "k8s.io/api/core/v1"

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
			ns.WaitForDatacenterReadyWithTimeouts(dcName, dcName, 420, 30)

			step = "update config secret"
			testUpdatedSecretFile, err := ginkgo_util.CreateTestSecretsConfig(updatedSecretYaml)
			Expect(err).ToNot(HaveOccurred())
			k = kubectl.ApplyFiles(testUpdatedSecretFile)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 60)
			ns.WaitForDatacenterConditionWithTimeout(dcName, string(api.DatacenterReady), string(corev1.ConditionTrue), 1200)

			step = "checking cassandra.yaml"
			k = kubectl.ExecOnPod("cluster1-dc1-r1-sts-0", "-c", "cassandra", "--", "cat", ginkgo_util.GetCassandraConfigYamlLocation())
			ns.WaitForOutputContainsAndLog(step, k, "read_request_timeout_in_ms: 10000", 60)

			step = "stop using config secret"
			json := ginkgo_util.CreateTestJson(`[{"op": "remove", "path": "/spec/configSecret"}, {"op": "add", "path": "/spec/config", "value": {"cassandra-yaml": {"read_request_timeout_in_ms": 25000}, "jvm-options": {"initial_heap_size": "512M", "max_heap_size": "512M"}}}]`)
			k = kubectl.PatchJson(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 120)
			ns.WaitForDatacenterConditionWithTimeout(dcName, string(api.DatacenterReady), string(corev1.ConditionTrue), 1200)

			step = "checking cassandra.yaml"
			k = kubectl.ExecOnPod("cluster1-dc1-r1-sts-0", "-c", "cassandra", "--", "cat", ginkgo_util.GetCassandraConfigYamlLocation())
			ns.WaitForOutputContainsAndLog(step, k, "read_request_timeout_in_ms: 25000", 120)

			step = "use config secret again"
			json = `{"spec": {"config": null, "configSecret": "test-config"}}`
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 120)
			ns.WaitForDatacenterConditionWithTimeout(dcName, string(api.DatacenterReady), string(corev1.ConditionTrue), 1200)

			step = "checking cassandra.yaml"
			k = kubectl.ExecOnPod("cluster1-dc1-r1-sts-0", "-c", "cassandra", "--", "cat", ginkgo_util.GetCassandraConfigYamlLocation())
			ns.WaitForOutputContainsAndLog(step, k, "read_request_timeout_in_ms: 10000", 120)
		})
	})
})
