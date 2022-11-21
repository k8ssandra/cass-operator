// Copyright DataStax, Inc.
// Please see the included license file for details.

package additional_seeds

import (
	"encoding/json"
	"fmt"
	"regexp"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

var (
	testName                       = "Seed Selection"
	namespace                      = "test-additional-seeds"
	dcName                         = "dc1"
	dcYaml                         = "../testdata/additional-seeds-two-rack-four-node-dc.yaml"
	dcResource                     = fmt.Sprintf("CassandraDatacenter/%s", dcName)
	dcLabel                        = fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)
	additionalSeedServiceResource  = "services/cluster1-dc1-additional-seed-service"
	additionalSeedEndpointResource = "endpoints/cluster1-dc1-additional-seed-service"
	ns                             = ginkgo_util.NewWrapper(testName, namespace)
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

type Node struct {
	Name    string
	Rack    string
	Ready   bool
	Seed    bool
	Started bool
	IP      string
	Ordinal int
}

type DatacenterInfo struct {
	Size      int
	RackNames []string
	Nodes     []Node
}

func retrieveNodes() []Node {
	k := kubectl.Get("pods").
		WithLabel(dcLabel).
		FormatOutput("json")
	output := ns.OutputPanic(k)
	data := corev1.PodList{}
	err := json.Unmarshal([]byte(output), &data)
	Expect(err).ToNot(HaveOccurred())
	result := []Node{}
	for idx := range data.Items {
		pod := &data.Items[idx]
		node := Node{}
		node.Name = pod.Name
		node.IP = pod.Status.PodIP
		node.Rack = pod.Labels["cassandra.datastax.com/rack"]
		isSeed, hasSeedLabel := pod.Labels["cassandra.datastax.com/seed-node"]
		node.Seed = hasSeedLabel && isSeed == "true"
		isStarted, hasStartedLabel := pod.Labels["cassandra.datastax.com/node-state"]
		node.Started = hasStartedLabel && isStarted == "Started"
		for _, condition := range pod.Status.Conditions {
			if condition.Type == "Ready" {
				node.Ready = condition.Status == "True"
			}
		}
		result = append(result, node)
	}
	return result
}

func retrieveDatacenterInfo() DatacenterInfo {
	k := kubectl.Get(dcResource).
		FormatOutput("json")
	output := ns.OutputPanic(k)
	data := map[string]interface{}{}
	err := json.Unmarshal([]byte(output), &data)
	Expect(err).ToNot(HaveOccurred())

	spec := data["spec"].(map[string]interface{})
	rackNames := []string{}
	for _, rackData := range spec["racks"].([]interface{}) {
		name := rackData.(map[string]interface{})["name"]
		if name != nil {
			rackNames = append(rackNames, name.(string))
		}
	}

	dc := DatacenterInfo{
		Size:      int(spec["size"].(float64)),
		Nodes:     retrieveNodes(),
		RackNames: rackNames,
	}

	return dc
}

func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func checkThereAreAtLeastThreeSeedsPerDc(info DatacenterInfo) {
	seedCount := 0

	for _, node := range info.Nodes {
		if node.Seed {
			seedCount += 1
		}
	}

	expectedSeedCount := MinInt(info.Size, 3)
	Expect(seedCount >= expectedSeedCount).To(BeTrue(),
		"Expected there to be at least %d seed nodes, but only found %d.",
		expectedSeedCount, seedCount)
}

func checkThereIsAtLeastOneSeedNodePerRack(info DatacenterInfo) {
	rackToFoundSeed := map[string]bool{}
	for _, node := range info.Nodes {
		if node.Seed {
			rackToFoundSeed[node.Rack] = true
		}
	}

	for _, rackName := range info.RackNames {
		value, ok := rackToFoundSeed[rackName]
		Expect(ok && value).To(BeTrue(), "Expected rack %s to have a seed node, but none found.", rackName)
	}
}

func checkDesignatedSeedNodesAreStartedAndReady(info DatacenterInfo) {
	for _, node := range info.Nodes {
		if node.Seed {
			Expect(node.Started).To(BeTrue(), "Expected %s to be labeled as started but was not.", node.Name)
			Expect(node.Ready).To(BeTrue(), "Expected %s to be ready but was not.", node.Name)
		}
	}
}

func checkSeedConstraints() {
	info := retrieveDatacenterInfo()
	// There should be 3 seed nodes for every datacenter
	checkThereAreAtLeastThreeSeedsPerDc(info)

	// There should be 2 seed nodes per rack
	// this is because of the additional seeds
	checkThereIsAtLeastOneSeedNodePerRack(info)

	// Seed nodes should not be down
	checkDesignatedSeedNodesAreStartedAndReady(info)

	// Ensure seed lists actually align
	//
	// NOTE: The following check does not presently work due to
	// the lag time between when we update a seed label and when
	// that change is reflected in DNS. Since we reload seed lists
	// right after upating the label, some cassandra nodes will
	// likely end up with slight out-of-date seed lists. KO-375
	//
	// checkCassandraSeedListsAlignWithSeedLabels(info)
}

func getAdditionalSeedEndpointResourceAddresses() ([]interface{}, error) {
	// Should be addresses and then go through them in the later check
	jsonpath := "jsonpath={.subsets[0].addresses}"
	k := kubectl.Get(additionalSeedEndpointResource).FormatOutput(jsonpath)
	output, err := ns.Output(k)
	if err != nil {
		return nil, err
	}
	ips := []interface{}{}
	err = json.Unmarshal([]byte(output), &ips)
	return ips, err
}

func getAdditionalSeedServiceData() (map[string]interface{}, error) {
	// Check the service
	k := kubectl.Get(additionalSeedServiceResource).FormatOutput("json")
	output := ns.OutputPanic(k)
	data := map[string]interface{}{}
	err := json.Unmarshal([]byte(output), &data)
	return data, err
}

func checkAdditionalSeedService() {
	// Check the service
	data, err := getAdditionalSeedServiceData()
	Expect(err).ToNot(HaveOccurred())

	spec := data["spec"].(map[string]interface{})
	actualType := spec["type"].(string)
	Expect(actualType).To(Equal("ClusterIP"), "Expected additional seed service type %s to be ClusterIP", actualType)

	// Check the endpoints
	ipList, err := getAdditionalSeedEndpointResourceAddresses()
	Expect(err).ToNot(HaveOccurred())
	firstIP := ipList[0].(map[string]interface{})
	Expect(firstIP["ip"]).To(Equal("192.168.1.1"), "Expected additional seed endpoints IP %s to be 192.168.1.1", firstIP)

	secondIP := ipList[1].(map[string]interface{})
	actualIp := secondIP["ip"].(string)
	match, _ := regexp.MatchString("^[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}$", actualIp)
	Expect(match).To(BeTrue())
}

var _ = Describe(testName, func() {
	Context("when in a new cluster", func() {
		Specify("the operator can properly create an additional seed service", func() {
			var step string
			var k kubectl.KCmd

			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step = "creating a datacenter resource with 2 racks/4 nodes"
			testFile, err := ginkgo_util.CreateTestFile(dcYaml)
			Expect(err).ToNot(HaveOccurred())

			k = kubectl.ApplyFiles(testFile)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dcName)

			checkSeedConstraints()

			// We should have all the services deployed, even if they're empty
			_, err = getAdditionalSeedServiceData()
			Expect(err).ToNot(HaveOccurred())

			// kubectl get endpoints kubernetes -o jsonpath={.metadata.resourceVersion}
			// beforeModification := getResourceVersionOfEndpoints()

			step = "add additionalSeeds"
			json := `
			{
				"spec": {
					"additionalSeeds": ["192.168.1.1", "www.datastax.com"]
				}
			}`
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			Eventually(func() error {
				_, err := getAdditionalSeedEndpointResourceAddresses()
				return err
			}, "15s", "100ms").Should(Succeed())

			checkSeedConstraints()

			checkAdditionalSeedService()

			// TODO provision a DC in another namespace and connect it to the first one
			// with additional seeds, and test that's working
		})
	})
})
