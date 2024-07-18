package monitoring

import (
	"fmt"
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
	PodStatusVec                *prometheus.GaugeVec
	DatacenterStatusVec         *prometheus.GaugeVec
	DatacenterOperatorStatusVec *prometheus.GaugeVec
)

func init() {
	podVec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "cass_operator",
		Subsystem: "datacenter_pods",
		Name:      "status",
		Help:      "Cassandra pod statuses",
	}, []string{"namespace", "cluster", "datacenter", "rack", "pod", "status"})

	datacenterConditionVec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "cass_operator",
		Subsystem: "datacenter",
		Name:      "status",
		Help:      "CassandraDatacenter conditions",
	}, []string{"namespace", "cluster", "datacenter", "condition"})

	datacenterOperatorStatusVec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "cass_operator",
		Subsystem: "datacenter",
		Name:      "progress",
		Help:      "CassandraDatacenter progress state",
	}, []string{"namespace", "cluster", "datacenter", "progress"})

	metrics.Registry.MustRegister(podVec)
	metrics.Registry.MustRegister(datacenterConditionVec)
	metrics.Registry.MustRegister(datacenterOperatorStatusVec)
	PodStatusVec = podVec
	DatacenterStatusVec = datacenterConditionVec
	DatacenterOperatorStatusVec = datacenterOperatorStatusVec
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

func SetDatacenterConditionMetric(dc *api.CassandraDatacenter, conditionType api.DatacenterConditionType, status corev1.ConditionStatus) {
	cond := float64(0)
	if status == corev1.ConditionTrue {
		cond = 1
	}

	DatacenterStatusVec.WithLabelValues(dc.Namespace, dc.Spec.ClusterName, dc.DatacenterName(), string(conditionType)).Set(cond)
}

func UpdateOperatorDatacenterProgressStatusMetric(dc *api.CassandraDatacenter, state api.ProgressState) {
	// Delete other statuses
	DatacenterOperatorStatusVec.DeletePartialMatch(prometheus.Labels{"namespace": dc.Namespace, "cluster": dc.Spec.ClusterName, "datacenter": dc.DatacenterName()})

	// Set this one only
	DatacenterOperatorStatusVec.WithLabelValues(dc.Namespace, dc.Spec.ClusterName, dc.DatacenterName(), string(state)).Set(1)
}

// Add CassandraTask status also (how many pods done etc) per task
// Add podnames to the CassandraTask status that are done? Or waiting?

func GetMetricValue(name string, labels map[string]string) (float64, error) {
	families, err := metrics.Registry.Gather()
	if err != nil {
		return 0, err
	}

	for _, fam := range families {
		if *fam.Name == name {
		Metric:
			for _, m := range fam.Metric {
				for _, label := range m.Label {
					if val, ok := labels[*label.Name]; ok {
						if val != *label.Value {
							continue Metric
						}
					}
				}
				return *m.Gauge.Value, nil
			}
		}
	}
	return 0, fmt.Errorf("no metric found")
}
