// Copyright DataStax, Inc.
// Please see the included license file for details.

package upgrade_operator

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

var (
	testName        = "Upgrade Operator"
	namespace       = "test-upgrade-operator"
	oldOperatorYaml = "../testdata/cass-operator-1.7.1-manifests.yaml"
	dcName          = "dc1"
	dcYaml          = "../testdata/operator-1.7.1-oss-dc.yaml"
	dcResource      = fmt.Sprintf("CassandraDatacenter/%s", dcName)
	dcLabel         = fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)
	ns              = ginkgo_util.NewWrapper(testName, namespace)
)

func TestLifecycle(t *testing.T) {
	AfterSuite(func() {
		logPath := fmt.Sprintf("%s/aftersuite", ns.LogDir)
		kubectl.DumpAllLogs(logPath).ExecV()
		fmt.Printf("\n\tPost-run logs dumped at: %s\n\n", logPath)
		ns.Terminate()
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, testName)
}

// InstallOldOperator installs the oldest supported upgrade path (this is the first k8ssandra/cass-operator release)
func InstallOldOperator() {
	step := "install cass-operator 1.7.1"
	By(step)

	// kubectl apply -f https://raw.githubusercontent.com/k8ssandra/cass-operator/v1.7.1/docs/user/cass-operator-manifests.yaml
	k := kubectl.ApplyFiles(oldOperatorYaml)
	ns.ExecAndLog(step, k)
}

func UpgradeOperator() {
	step := "upgrade Cass Operator"
	By(step)

	// Update steps needed for 1.7.1
	// kubectl -n cass-operator delete deployment.apps/cass-operator
	// kubectl -n cass-operator delete service/cassandradatacenter-webhook-service
	// kubectl patch crd/cassandradatacenters.cassandra.datastax.com -p '{"spec":{"preserveUnknownFields":false}}'
	k := kubectl.Delete("deployment.apps/cass-operator")
	ns.ExecAndLog(step, k)

	k = kubectl.Delete("service/cassandradatacenter-webhook-service")
	ns.ExecAndLog(step, k)

	k = kubectl.Patch("crd/cassandradatacenters.cassandra.datastax.com", `{"spec":{"preserveUnknownFields":false}}`)
	ns.ExecAndLog(step, k)

	// Then install as usual.
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

			step := "creating a datacenter resource with 1 racks/1 node"
			k := kubectl.ApplyFiles(dcYaml)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dcName)

			step = "get name of 1.7.1 operator pod"
			json := "jsonpath={.items[].metadata.name}"
			k = kubectl.Get("pods").WithFlag("selector", "name=cass-operator").FormatOutput(json)
			oldOperatorName := ns.OutputAndLog(step, k)

			UpgradeOperator()

			step = "wait for 1.7.1 operator pod to be removed"
			k = kubectl.Get("pods").WithFlag("field-selector", fmt.Sprintf("metadata.name=%s", oldOperatorName))
			ns.WaitForOutputAndLog(step, k, "", 60)

			ns.WaitForOperatorReady()

			// give the operator a minute to reconcile and update the datacenter
			time.Sleep(1 * time.Minute)

			ns.WaitForDatacenterReadyWithTimeouts(dcName, 800, 60)

			ns.ExpectDoneReconciling(dcName)

			// Update Cassandra version to ensure we can still do changes
			step = "perform cassandra upgrade"
			json = "{\"spec\": {\"serverVersion\": \"3.11.11\"}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 30)
			ns.WaitForDatacenterReadyPodCount(dcName, 1)
			ns.WaitForDatacenterReadyWithTimeouts(dcName, 800, 60)

			ns.ExpectDoneReconciling(dcName)

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
