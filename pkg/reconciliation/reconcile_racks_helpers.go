// Copyright DataStax, Inc.
// Please see the included license file for details.
package reconciliation

import (
	"fmt"
	"net"

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

func MapPodsToEndpointDataByName(pods []*corev1.Pod, epData httphelper.CassMetadataEndpoints) map[string]httphelper.EndpointState {
	result := make(map[string]httphelper.EndpointState)
	for idx := range pods {
		pod := pods[idx]
		for _, data := range epData.Entity {
			if data.GetRpcAddress() == pod.Status.PodIP {
				result[pod.Name] = data
				continue
			}
		}
	}

	return result
}
