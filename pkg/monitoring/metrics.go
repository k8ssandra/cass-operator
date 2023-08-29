package monitoring

import (
	"strings"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	corev1 "k8s.io/api/core/v1"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

type PodStatus string

const (
	PodStatusInitializing    PodStatus = "Initializing"
	PodStatusReady           PodStatus = "Ready"
	PodStatusPending         PodStatus = "Pending"
	PodStatusError           PodStatus = "Error"
	PodStatusDecommissioning PodStatus = "Decommissioning"
)

func getPodStatus(pod *corev1.Pod) PodStatus {
	status := PodStatusReady

	if pod.Labels[api.CassNodeState] == "Decommissioning" {
		return PodStatusDecommissioning
	}

	switch pod.Status.Phase {
	case corev1.PodPending:
		return PodStatusPending
	case corev1.PodFailed:
		return PodStatusError
	case corev1.PodRunning:
	default:
	}

	allContainersReady := true

	for _, s := range pod.Status.ContainerStatuses {
		if !s.Ready {
			allContainersReady = false
			break
		}
	}

	if !allContainersReady {
		return PodStatusInitializing
	}

	return status
}

var (
	PodStatusVec *prometheus.GaugeVec
)

func init() {
	podVec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "cass_operator",
		Subsystem: "datacenter_pods",
		Name:      "status",
		Help:      "Cassandra pod statuses",
	}, []string{"namespace", "cluster", "datacenter", "rack", "pod", "status"})

	metrics.Registry.MustRegister(podVec)
	PodStatusVec = podVec
}

func UpdatePodStatusMetric(pod *corev1.Pod) {
	currentState := getPodStatus(pod)

	// Delete all status metrics for this pod
	RemovePodStatusMetric(pod)

	// Add just the current one
	PodStatusVec.WithLabelValues(pod.Namespace, pod.Labels[api.ClusterLabel], pod.Labels[api.DatacenterLabel], pod.Labels[api.RackLabel], pod.Name, strings.ToLower(string(currentState))).Set(1)
}

func RemovePodStatusMetric(pod *corev1.Pod) {
	PodStatusVec.DeletePartialMatch(prometheus.Labels{"namespace": pod.Namespace, "cluster": pod.Labels[api.ClusterLabel], "datacenter": pod.Labels[api.DatacenterLabel], "rack": pod.Labels[api.RackLabel], "pod": pod.Name})
}

func RemoveDatacenterPods(namespace, cluster, datacenter string) {
	PodStatusVec.DeletePartialMatch(prometheus.Labels{"namespace": namespace, "cluster": cluster, "datacenter": datacenter})
}
