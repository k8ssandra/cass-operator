// Copyright DataStax, Inc.
// Please see the included license file for details.

package external_secrets

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
	shutil "github.com/k8ssandra/cass-operator/tests/util/sh"
)

var (
	testName  = "External secrets test with Vault"
	namespace = "test-external-secrets"
	dcName    = "dc2"
	dcYaml    = "../testdata/default-single-rack-single-node-dc-vault.yaml"
	ns        = ginkgo_util.NewWrapper(testName, namespace)
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
	Context("when in a new cluster with Vault installed", func() {
		Specify("Vault is installed with cass-operator", func() {

			By("creating a namespace for the cass-operator")
			err := kubectl.CreateNamespace(namespace).ExecV()
			Expect(err).ToNot(HaveOccurred())

			By("deploy cass-operator with kustomize")
			err = kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())
			ns.WaitForOperatorReady()

			By("deploying Vault")
			err = shutil.RunV("helm", "repo", "add", "hashicorp", "https://helm.releases.hashicorp.com")
			Expect(err).ShouldNot(HaveOccurred())

			err = shutil.RunV("helm", "repo", "update")
			Expect(err).ShouldNot(HaveOccurred())

			err = shutil.RunV("helm", "install", "-n", namespace, "vault", "hashicorp/vault", "--set", "server.dev.enabled=true")
			Expect(err).ShouldNot(HaveOccurred())

			By("Waiting for all components to be ready")
			readyGetter := kubectl.Get("pods").
				WithFlag("selector", "component=server").
				FormatOutput("jsonpath={.items[0].status.conditions[?(@.type=='Ready')].status}")
			err = kubectl.WaitForOutputContains(readyGetter, "True", 1800)
			Expect(err).ShouldNot(HaveOccurred())

			podName := "vault-0"

			// Write settings to the Vault pod
			k := kubectl.ExecOnPod(podName, "--", "sh", "-c", "vault secrets enable -path=internal kv-v2")
			err = ns.ExecV(k)
			Expect(err).ShouldNot(HaveOccurred())

			username := "superuser"
			password := "superpassword"

			k = kubectl.ExecOnPod(podName, "--", "sh", "-c", fmt.Sprintf("vault kv put internal/database/config username=%s password=%s", username, password))
			err = ns.ExecV(k)
			Expect(err).ShouldNot(HaveOccurred())

			k = kubectl.ExecOnPod(podName, "--", "sh", "-c", "vault auth enable kubernetes")
			err = ns.ExecV(k)
			Expect(err).ShouldNot(HaveOccurred())

			k = kubectl.ExecOnPod(podName, "--", "sh", "-c", `vault write auth/kubernetes/config kubernetes_host="https://$KUBERNETES_PORT_443_TCP_ADDR:443"`)
			err = ns.ExecV(k)
			Expect(err).ShouldNot(HaveOccurred())

			k = kubectl.ExecOnPod(podName, "--", "sh", "-c", `vault policy write internal-app - <<EOF
			path "internal/data/database/config" {
			  capabilities = ["read"]
			}`)
			err = ns.ExecV(k)
			Expect(err).ShouldNot(HaveOccurred())

			// Create SA
			// k = kubectl.KCmd{Command: "create", Args: []string{"sa", "internal-app"}}
			// err = ns.ExecV(k)
			// Expect(err).ShouldNot(HaveOccurred())

			writeCommand := fmt.Sprintf("vault write auth/kubernetes/role/internal-app bound_service_account_names=cass-operator-controller-manager bound_service_account_namespaces=%s policies=internal-app ttl=24h", namespace)

			k = kubectl.ExecOnPod(podName, "--", "sh", "-c", writeCommand)
			err = ns.ExecV(k)
			Expect(err).ShouldNot(HaveOccurred())

			step := "creating a DC"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k = kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReadyWithTimeouts(dcName, 1200, 1200)

			podNames := ns.GetDatacenterPodNames(dcName)

			fmt.Printf("Waiting now..\n")
			time.Sleep(1200 * time.Second)

			// Verify the set passwords are actually working
			step = "check superuser credentials work"
			k = kubectl.ExecOnPod(
				podNames[0], "--", "cqlsh",
				"--user", username,
				"--password", password,
				"-e", "select * from system_schema.keyspaces;").
				WithFlag("container", "cassandra")
			ns.ExecAndLog(step, k)

			step = "check that bad credentials don't work"
			k = kubectl.ExecOnPod(
				podNames[0], "--", "cqlsh",
				"--user", username,
				"--password", "notthepassword",
				"-e", "select * from system_schema.keyspaces;").
				WithFlag("container", "cassandra")
			By(step)
			err = ns.ExecV(k)
			Expect(err).To(HaveOccurred())
		})
	})
})
