package reconciliation

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/k8ssandra/cass-operator/pkg/httphelper"
	corev1 "k8s.io/api/core/v1"
)

func TestMapContains(t *testing.T) {
	labels := make(map[string]string)
	labels["key1"] = "val1"
	labels["key2"] = "val2"
	labels["key3"] = "val3"

	selectors := make(map[string]string)
	selectors["key1"] = "val1"
	selectors["key3"] = "val3"

	isMatch := mapContains(labels, selectors)

	if !isMatch {
		t.Fatalf("mapContains should have found match.")
	}
}

func TestMapContainsDoesntContain(t *testing.T) {
	labels := make(map[string]string)
	labels["key1"] = "val1"
	labels["key2"] = "val2"
	labels["key3"] = "val3"

	selectors := make(map[string]string)
	selectors["key1"] = "val4"
	selectors["key3"] = "val3"

	isMatch := mapContains(labels, selectors)

	if isMatch {
		t.Fatalf("mapContains should not have found match.")
	}
}

func TestPodPrtsFromPodList(t *testing.T) {
	pod1 := corev1.Pod{}
	pod1.Name = "pod1"

	pod2 := corev1.Pod{}
	pod2.Name = "pod2"

	pod3 := corev1.Pod{}
	pod3.Name = "pod3"
	podList := corev1.PodList{
		Items: []corev1.Pod{pod1, pod2, pod3},
	}

	prts := PodPtrsFromPodList(&podList)

	expectedNames := []string{"pod1", "pod2", "pod3"}
	var actualNames []string
	for _, p := range prts {
		actualNames = append(actualNames, p.Name)
	}
	assert.ElementsMatch(t, expectedNames, actualNames)
}

func TestFindHostIdFromEndpointsData(t *testing.T) {
	// Create test data based on the provided example
	testCases := []struct {
		name           string
		endpointsData  []httphelper.EndpointState
		expectedHostId string
	}{
		{
			name: "Local node is present and NORMAL",
			endpointsData: []httphelper.EndpointState{
				{
					HostID:  "f6878295-4e81-4a42-a5e2-98710987543b",
					IsLocal: "true",
					Status:  "NORMAL,4241053645453754050",
				},
				{
					HostID:  "0e620ae9-1ab3-406e-a3cb-d1d25972c90c",
					IsLocal: "false",
					Status:  "NORMAL,-3369421087898920741",
				},
				{
					HostID:  "eaf1d782-2fe3-4cae-b648-8235635fd8d2",
					IsLocal: "false",
					Status:  "NORMAL,-6989932144271474238",
				},
			},
			expectedHostId: "f6878295-4e81-4a42-a5e2-98710987543b",
		},
		{
			name: "No local node present",
			endpointsData: []httphelper.EndpointState{
				{
					HostID:  "0e620ae9-1ab3-406e-a3cb-d1d25972c90c",
					IsLocal: "false",
					Status:  "NORMAL,-3369421087898920741",
				},
				{
					HostID:  "eaf1d782-2fe3-4cae-b648-8235635fd8d2",
					IsLocal: "false",
					Status:  "NORMAL,-6989932144271474238",
				},
			},
			expectedHostId: "",
		},
		{
			name: "Local node present but not NORMAL",
			endpointsData: []httphelper.EndpointState{
				{
					HostID:  "f6878295-4e81-4a42-a5e2-98710987543b",
					IsLocal: "true",
					Status:  "JOINING,4241053645453754050",
				},
				{
					HostID:  "0e620ae9-1ab3-406e-a3cb-d1d25972c90c",
					IsLocal: "false",
					Status:  "NORMAL,-3369421087898920741",
				},
			},
			expectedHostId: "",
		},
		{
			name:           "Empty endpoints data",
			endpointsData:  []httphelper.EndpointState{},
			expectedHostId: "",
		},
		{
			name: "Multiple local nodes (abnormal scenario)",
			endpointsData: []httphelper.EndpointState{
				{
					HostID:  "f6878295-4e81-4a42-a5e2-98710987543b",
					IsLocal: "true",
					Status:  "NORMAL,4241053645453754050",
				},
				{
					HostID:  "0e620ae9-1ab3-406e-a3cb-d1d25972c90c",
					IsLocal: "true",
					Status:  "NORMAL,-3369421087898920741",
				},
			},
			expectedHostId: "f6878295-4e81-4a42-a5e2-98710987543b", // Should return the first one found
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hostId := findHostIdFromEndpointsData(tc.endpointsData)
			assert.Equal(t, tc.expectedHostId, hostId, "Expected host ID doesn't match")
		})
	}
}

// Test for the findHostIdForIpFromEndpointsData function based on the same example data
func TestFindHostIdForIpFromEndpointsDataLegacy(t *testing.T) {
	// Create test data with real IP addresses from the example
	endpointsData := []httphelper.EndpointState{
		{
			HostID:     "f6878295-4e81-4a42-a5e2-98710987543b",
			IsLocal:    "true",
			Status:     "NORMAL,4241053645453754050",
			RpcAddress: "10.116.3.176",
		},
		{
			HostID:     "0e620ae9-1ab3-406e-a3cb-d1d25972c90c",
			IsLocal:    "false",
			Status:     "NORMAL,-3369421087898920741",
			RpcAddress: "10.116.1.226",
		},
		{
			HostID:     "eaf1d782-2fe3-4cae-b648-8235635fd8d2",
			IsLocal:    "false",
			Status:     "JOINING,-6989932144271474238", // Changed to JOINING to test non-NORMAL status
			RpcAddress: "10.116.7.155",
		},
	}

	testCases := []struct {
		name           string
		ipToSearch     string
		expectedReady  bool
		expectedHostId string
	}{
		{
			name:           "Find NORMAL node by IP",
			ipToSearch:     "10.116.3.176",
			expectedReady:  true,
			expectedHostId: "f6878295-4e81-4a42-a5e2-98710987543b",
		},
		{
			name:           "Find non-NORMAL node by IP",
			ipToSearch:     "10.116.7.155",
			expectedReady:  false,
			expectedHostId: "eaf1d782-2fe3-4cae-b648-8235635fd8d2",
		},
		{
			name:           "IP not found",
			ipToSearch:     "10.116.9.999",
			expectedReady:  false,
			expectedHostId: "",
		},
		{
			name:           "IPv6 address (not in sample data)",
			ipToSearch:     "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
			expectedReady:  false,
			expectedHostId: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ready, hostId := findHostIdForIpFromEndpointsData(endpointsData, tc.ipToSearch)
			assert.Equal(t, tc.expectedReady, ready, "Expected ready status doesn't match")
			assert.Equal(t, tc.expectedHostId, hostId, "Expected host ID doesn't match")
		})
	}
}
