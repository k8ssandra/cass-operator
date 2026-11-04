// Copyright DataStax, Inc.
// Please see the included license file for details.
package reconciliation

import (
	"fmt"
	"net"
	"slices"
	"strings"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/pkg/httphelper"
	corev1 "k8s.io/api/core/v1"
)

func mapContains(base map[string]string, submap map[string]string) bool {
	for k, v := range submap {
		if val, ok := base[k]; !ok || val != v {
			return false
		}
	}
	return true
}

// Takes a list of *Pod and filters down to only the pods that
// match every label/val in the provided label map.
func FilterPodListByLabels(pods []*corev1.Pod, labelMap map[string]string) []*corev1.Pod {
	filtered := []*corev1.Pod{}
	for _, p := range pods {
		if mapContains(p.Labels, labelMap) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func FilterPodListByLabel(pods []*corev1.Pod, labelName string, labelVal string) []*corev1.Pod {
	labels := map[string]string{
		labelName: labelVal,
	}
	return FilterPodListByLabels(pods, labels)
}

func FilterPodListByCassNodeState(pods []*corev1.Pod, state string) []*corev1.Pod {
	filtered := []*corev1.Pod{}
	for _, p := range pods {
		if val := p.Labels[api.CassNodeState]; val == state {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func ListAllStartedPods(pods []*corev1.Pod) []*corev1.Pod {
	return FilterPodListByCassNodeState(pods, stateStarted)
}

func FindIpForHostId(endpointData httphelper.CassMetadataEndpoints, hostId string) (string, error) {
	// If there are no nodes to ask, then of course we will not find an IP. We
	// treat this as an error since we have not way to determine the mapping.
	if len(endpointData.Entity) < 1 {
		return "", fmt.Errorf("no pods available to ask for the IP address of %s", hostId)
	}

	// Search for a cassandra node that knows about the given hostId
	for _, ep := range endpointData.Entity {
		if ep.HostID == hostId && len(ep.EndpointAddress()) > 0 {
			return ep.EndpointAddress(), nil
		}
	}

	// This indicates the cassandra node with the given hostId never
	// actually joined the ring
	return "", nil
}

// findStaleReplaceJoiners returns the addresses of dead nodes that were replacing replaceAddress.
// These are earlier incarnations of the pod about to start: when a replacing pod is killed, its
// endpoint stays in gossip as a down node with status BOOT_REPLACE. Cassandra then refuses to
// start the next replace attempt ("Other bootstrapping/leaving/moving nodes detected") and the
// dead pending replica makes QUORUM writes fail. Alive endpoints, replaceAddress itself and the
// excluded addresses (held by Cassandra pods that exist now) are never returned.
func findStaleReplaceJoiners(endpointData httphelper.CassMetadataEndpoints, replaceAddress string, excluded []string) []string {
	replaceIP := net.ParseIP(replaceAddress)
	if replaceIP == nil {
		return nil
	}

	var stale []string
	for _, ep := range endpointData.Entity {
		if ep.IsAlive != "false" {
			continue
		}
		address := ep.EndpointAddress()
		ip := net.ParseIP(address)
		if ip == nil || ip.Equal(replaceIP) || slices.ContainsFunc(excluded, func(e string) bool { return ip.Equal(net.ParseIP(e)) }) {
			continue
		}
		if target := bootReplaceTarget(ep); target != nil && target.Equal(replaceIP) {
			stale = append(stale, address)
		}
	}
	return stale
}

// bootReplaceTarget returns the address an endpoint is replacing, or nil if its status is not
// BOOT_REPLACE. STATUS_WITH_PORT ("BOOT_REPLACE,10.0.0.1:7000") is used when present, STATUS
// ("BOOT_REPLACE,10.0.0.1") otherwise.
func bootReplaceTarget(ep httphelper.EndpointState) net.IP {
	status := ep.StatusWithPort
	if status == "" {
		status = ep.Status
	}
	target, found := strings.CutPrefix(status, string(httphelper.StatusBootReplace)+",")
	if !found {
		return nil
	}
	if host, _, err := net.SplitHostPort(target); err == nil {
		target = host
	}
	return net.ParseIP(target)
}

func findHostIdForIpFromEndpointsData(endpointsData []httphelper.EndpointState, ip string) (bool, string) {
	for _, data := range endpointsData {
		if net.ParseIP(data.GetRpcAddress()).Equal(net.ParseIP(ip)) {
			if data.HasStatus(httphelper.StatusNormal) {
				return true, data.HostID
			}
			return false, data.HostID
		}
	}
	return false, ""
}

func findHostIdFromEndpointsData(endpointsData []httphelper.EndpointState) string {
	for _, data := range endpointsData {
		if data.IsLocal == "true" && data.HasStatus(httphelper.StatusNormal) {
			return data.HostID
		}
	}

	return ""
}

func PodPtrsFromPodList(podList *corev1.PodList) []*corev1.Pod {
	var pods []*corev1.Pod
	for idx := range podList.Items {
		pod := &podList.Items[idx]
		pods = append(pods, pod)
	}
	return pods
}

// MapPodsToEndpointDataByName matches pods to their endpoint state by host id,
// which is stable across restarts, instead of by pod IP, which is not. A pod
// that has no host id recorded has not finished bootstrapping and gets no
// entry.
func MapPodsToEndpointDataByName(pods []*corev1.Pod, epData httphelper.CassMetadataEndpoints, nodeStatuses api.CassandraStatusMap) map[string]httphelper.EndpointState {
	result := make(map[string]httphelper.EndpointState)
	for idx := range pods {
		pod := pods[idx]
		hostID := nodeStatuses[pod.Name].HostID
		if hostID == "" {
			continue
		}
		for _, data := range epData.Entity {
			if data.HostID == hostID {
				result[pod.Name] = data
				break
			}
		}
	}

	return result
}
