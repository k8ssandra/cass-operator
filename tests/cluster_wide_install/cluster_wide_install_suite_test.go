// Copyright DataStax, Inc.
// Please see the included license file for details.

package cluster_wide_install

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

// Note: the cass-operator itself will be installed in the test-cluster-wide-install namespace
// The two test dcs will be setup in test-cluster-wide-install-ns1 and test-cluster-wide-install-ns2

var (
	testName     = "Cluster-wide install"
	opNamespace  = "cass-operator"
	dcNamespace1 = "test-cluster-wide-install-ns1"
	dcNamespace2 = "test-cluster-wide-install-ns2"
	dc1Name      = "dc1"
	dc1Yaml      = "../testdata/cluster-wide-install-dc1.yaml"
	dc2Name      = "dc2"
	dc2Yaml      = "../testdata/cluster-wide-install-dc2.yaml"
	dc1Resource  = fmt.Sprintf("CassandraDatacenter/%s", dc1Name)
	dc2Resource  = fmt.Sprintf("CassandraDatacenter/%s", dc2Name)
	ns           = ginkgo_util.NewWrapper(testName, opNamespace)
	ns1          = ginkgo_util.NewWrapper(testName, dcNamespace1)
	ns2          = ginkgo_util.NewWrapper(testName, dcNamespace2)
)

func TestLifecycle(t *testing.T) {
	AfterSuite(func() {
		logPath := fmt.Sprintf("%s/aftersuite", ns.LogDir)
		err := kubectl.DumpAllLogs(logPath).ExecV()
		if err != nil {
			fmt.Printf("\n\tError during dumping logs: %s\n\n", err.Error())
		}
		fmt.Printf("\n\tPost-run logs dumped at: %s\n\n", logPath)
		ns.Terminate()
		err = kustomize.UndeployDir(opNamespace, "cluster_wide_install")
		if err != nil {
			t.Logf("Failed to undeploy cass-operator: %v", err)
		}
	})
	RegisterFailHandler(Fail)
	RunSpecs(t, testName)
}

var _ = Describe(testName, func() {
	Context("when in a new cluster", func() {
		Specify("the operator can monitor multiple namespaces", func() {
			// Workaround for kustomize reorder bug
			By("creating a namespace for the cass-operator")
			err := kubectl.CreateNamespace(opNamespace).ExecV()
			Expect(err).ToNot(HaveOccurred())

			By("deploy cass-operator with kustomize")
			err = kustomize.DeployDir(opNamespace, "cluster_wide_install")
			Expect(err).ToNot(HaveOccurred())

			// var overrides = map[string]string{
			// 	"clusterWideInstall": "true",
			// }
			// chartPath := "../../charts/cass-operator-chart"
			// ginkgo_util.HelmInstallWithOverrides(chartPath, ns.Namespace, overrides)

			ns.WaitForOperatorReady()

			By("creating a namespace for the first dc")
			err = kubectl.CreateNamespace(dcNamespace1).ExecV()
			Expect(err).ToNot(HaveOccurred())

			By("creating a namespace for the second dc")
			err = kubectl.CreateNamespace(dcNamespace2).ExecV()
			Expect(err).ToNot(HaveOccurred())

			step := "creating first datacenter resource"
			testFile1, err := ginkgo_util.CreateTestFile(dc1Yaml)
			Expect(err).ToNot(HaveOccurred())

			k := kubectl.ApplyFiles(testFile1)
			ns1.ExecAndLog(step, k)

			step = "creating second datacenter resource"
			testFile2, err := ginkgo_util.CreateTestFile(dc2Yaml)
			Expect(err).ToNot(HaveOccurred())

			k = kubectl.ApplyFiles(testFile2)
			ns2.ExecAndLog(step, k)

			ns1.WaitForDatacenterReady(dc1Name)
			ns2.WaitForDatacenterReady(dc2Name)

			step = "scale first dc up to 2 nodes"
			json := `{"spec": {"size": 2}}`
			k = kubectl.PatchMerge(dc1Resource, json)
			ns1.ExecAndLog(step, k)

			ns1.WaitForDatacenterOperatorProgress(dc1Name, "Updating", 30)
			ns1.WaitForDatacenterReady(dc1Name)

			step = "scale second dc up to 2 nodes"
			json = `{"spec": {"size": 2}}`
			k = kubectl.PatchMerge(dc2Resource, json)
			ns2.ExecAndLog(step, k)

			ns2.WaitForDatacenterOperatorProgress(dc2Name, "Updating", 30)
			ns2.WaitForDatacenterReady(dc2Name)

			By("deleting a namespace for the first dc")
			err = kubectl.DeleteNamespace(dcNamespace1).ExecV()
			Expect(err).ToNot(HaveOccurred())

			By("deleting a namespace for the second dc")
			err = kubectl.DeleteNamespace(dcNamespace2).ExecV()
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
