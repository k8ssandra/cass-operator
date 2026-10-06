// Copyright DataStax, Inc.
// Please see the included license file for details.

package superuser_secret_provided

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

var (
	testName            = "Superuser Secret Provided"
	namespace           = "test-superuser-secret-provided"
	superuserSecretName = "my-superuser-secret"
	defaultSecretName   = "cluster2-superuser"
	superuserName       = "bob"
	superuserPass       = "bobber"
	superuserNewPass    = "somefancypantsnewpassword"
	secretChangedYaml   = "../testdata/bob-secret-changed.yaml"
	secretYaml          = "../testdata/bob-secret.yaml"
	bobbyuserName       = "bobby"
	bobbyuserPass       = "littlebobbydroptables"
	dcName              = "dc2"
	dcYaml              = "../testdata/default-single-rack-2-node-dc-with-superuser-secret.yaml"
	ns                  = ginkgo_util.NewWrapper(testName, namespace)
)

func TestLifecycle(t *testing.T) {
	ginkgo_util.RunTestLifecycle(t, testName, ns)
}

var _ = Describe(testName, func() {
	Context("when in a new cluster where superuserSecretName is specified", func() {
		Specify("the operator uses the provided superuser secret and deletes the datacenter when the secret is removed", func() {
			var step string
			var k kubectl.KCmd

			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step = "create superuser secret"
			k = kubectl.ApplyFiles(secretYaml)
			ns.ExecAndLog(step, k)

			step = "create user secret"
			k = kubectl.CreateSecretLiteral("bobby-secret", bobbyuserName, bobbyuserPass)
			ns.ExecAndLog(step, k)

			step = "creating a datacenter resource with 1 racks/2 nodes"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k = kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dcName)

			podNames := ns.GetDatacenterPodNames(dcName)

			step = "check Bobby's credentials work"
			k = kubectl.ExecOnPod(
				podNames[0], "--", "cqlsh",
				"--user", bobbyuserName,
				"--password", bobbyuserPass,
				"-e", "select * from system_schema.keyspaces;").
				WithFlag("container", "cassandra")
			ns.ExecAndLog(step, k)

			step = "check superuser credentials work"
			k = kubectl.ExecOnPod(
				podNames[0], "--", "cqlsh",
				"--user", superuserName,
				"--password", superuserPass,
				"-e", "select * from system_schema.keyspaces;").
				WithFlag("container", "cassandra")
			ns.ExecAndLog(step, k)

			step = "check that bad credentials don't work"
			k = kubectl.ExecOnPod(
				podNames[0], "--", "cqlsh",
				"--user", superuserName,
				"--password", "notthepassword",
				"-e", "select * from system_schema.keyspaces;").
				WithFlag("container", "cassandra")
			By(step)
			err = ns.ExecV(k)
			Expect(err).To(HaveOccurred())

			// It wouldn't be the end of the world if the default secret were
			// unnecessarily created (so long as it isn't used), but it would
			// be confusing.
			step = "check that the default superuser secret was not generated"
			k = kubectl.Get("secret", defaultSecretName)
			By(step)
			err = ns.ExecV(k)
			Expect(err).To(HaveOccurred())

			step = "check change superuser secret updates user"
			k = kubectl.ApplyFiles(secretChangedYaml)
			ns.ExecAndLog(step, k)

			// Give the operator a few seconds to respond to the change
			time.Sleep(30 * time.Second)

			step = "verify new credentials work"
			k = kubectl.ExecOnPod(
				podNames[0], "--", "cqlsh",
				"--user", superuserName,
				"--password", superuserNewPass,
				"-e", "select * from system_schema.keyspaces;").
				WithFlag("container", "cassandra")
			ns.ExecAndLog(step, k)

			step = "delete the provided superuser secret"
			k = kubectl.Delete("secret", superuserSecretName)
			ns.ExecAndLog(step, k)

			step = "delete datacenter with its superuser secret missing"
			k = kubectl.Delete("CassandraDatacenter", dcName).WithFlag("wait", "false")
			ns.ExecAndLog(step, k)

			step = "checking that the dc2 no longer exists"
			json := "jsonpath={.items}"
			k = kubectl.Get("CassandraDatacenter").
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)

			step = "checking that no dc stateful sets remain"
			k = kubectl.Get("statefulsets").
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)
		})
	})
})
