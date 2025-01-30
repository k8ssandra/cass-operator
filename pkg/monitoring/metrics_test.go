package monitoring

import (
	"fmt"
	"testing"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

func TestMetricAdder(t *testing.T) {
	pods := make([]*corev1.Pod, 6)
	for i := 0; i < len(pods); i++ {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pod%d", i),
				Namespace: "ns",
				Labels: map[string]string{
					api.ClusterLabel:    "cluster1",
					api.DatacenterLabel: "datacenter1",
					api.RackLabel:       "rack1",
					api.CassNodeState:   "Started",
				},
			},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Ready: true,
					},
				},
			},
		}
		pods[i] = pod
		UpdatePodStatusMetric(pod)
	}

	require := require.New(t)

	status, err := getCurrentPodStatus("pod0")
	require.NoError(err)
	require.Equal("ready", status)

	pods[2].Status.ContainerStatuses[0].Ready = false
	UpdatePodStatusMetric(pods[2])

	status, err = getCurrentPodStatus("pod2")
	require.NoError(err)
	require.Equal("initializing", status)

	pods[3].Status.Phase = corev1.PodFailed
	UpdatePodStatusMetric(pods[3])

	status, err = getCurrentPodStatus("pod3")
	require.NoError(err)
	require.Equal("error", status)

	RemovePodStatusMetric(pods[4])
	_, err = getCurrentPodStatus("pod4")
	require.Error(err)

	metav1.SetMetaDataLabel(&pods[5].ObjectMeta, api.CassNodeState, "Decommissioning")
	UpdatePodStatusMetric(pods[5])
	status, err = getCurrentPodStatus("pod5")
	require.NoError(err)
	require.Equal("decommissioning", status)

	// Verify pod4 can be added back
	UpdatePodStatusMetric(pods[4])
	status, err = getCurrentPodStatus("pod4")
	require.NoError(err)
	require.Equal("ready", status)

	now := metav1.Now()
	pods[1].SetDeletionTimestamp(&now)
	UpdatePodStatusMetric(pods[1])
	status, err = getCurrentPodStatus("pod1")
	require.NoError(err)
	require.Equal("terminating", status)

	// Decommissioning should be prefered to Terminating if we are decommissioning the pod
	pods[5].SetDeletionTimestamp(&now)
	UpdatePodStatusMetric(pods[1])
	status, err = getCurrentPodStatus("pod5")
	require.NoError(err)
	require.Equal("decommissioning", status)

	RemoveDatacenterPods("ns", "cluster1", "datacenter1")
	_, err = getCurrentPodStatus("pod4")
	require.Error(err)
}

func TestNamespaceSeparation(t *testing.T) {
	require := require.New(t)
	pods := make([]*corev1.Pod, 2)
	for i := 0; i < len(pods); i++ {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pod%d", i),
				Namespace: fmt.Sprintf("ns%d", i),
				Labels: map[string]string{
					api.ClusterLabel:    "cluster1",
					api.DatacenterLabel: "datacenter1",
					api.RackLabel:       "rack1",
					api.CassNodeState:   "Started",
				},
			},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Ready: true,
					},
				},
			},
		}
		pods[i] = pod
		UpdatePodStatusMetric(pod)
	}
	status, err := getCurrentPodStatus("pod0")
	require.NoError(err)
	require.Equal("ready", status)

	status, err = getCurrentPodStatus("pod1")
	require.NoError(err)
	require.Equal("ready", status)

	RemoveDatacenterPods("ns0", "cluster1", "datacenter1")
	_, err = getCurrentPodStatus("pod0")
	require.Error(err)

	status, err = getCurrentPodStatus("pod1")
	require.NoError(err)
	require.Equal("ready", status)
}

func getCurrentPodStatus(podName string) (string, error) {
	families, err := metrics.Registry.Gather()
	if err != nil {
		return "", err
	}

	for _, fam := range families {
		if *fam.Name == "cass_operator_datacenter_pods_status" {
		Metric:
			for _, m := range fam.Metric {
				status := ""
				for _, label := range m.Label {
					if *label.Name == "pod" {
						if *label.Value != podName {
							continue Metric
						}
					}
					if *label.Name == "status" {
						status = *label.Value
					}
				}
				if *m.Gauge.Value > 0 {
					return status, nil
				}
			}
		}
	}
	return "", fmt.Errorf("No pod status found")
}

func TestOperatorStateMetrics(t *testing.T) {
	require := require.New(t)

	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dc1",
			Namespace: "ns",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName: "cluster1",
		},
		Status: api.CassandraDatacenterStatus{},
	}

	UpdateOperatorDatacenterProgressStatusMetric(dc, api.ProgressUpdating)

	status, err := getCurrentDatacenterStatus("dc1")
	require.NoError(err)
	require.Equal("Updating", status)

	UpdateOperatorDatacenterProgressStatusMetric(dc, api.ProgressReady)

	status, err = getCurrentDatacenterStatus("dc1")
	require.NoError(err)
	require.Equal("Ready", status)
}

func getCurrentDatacenterStatus(dcName string) (string, error) {
	families, err := metrics.Registry.Gather()
	if err != nil {
		return "", err
	}

	for _, fam := range families {
		if *fam.Name == "cass_operator_datacenter_progress" {
		Metric:
			for _, m := range fam.Metric {
				status := ""
				for _, label := range m.Label {
					if *label.Name == "datacenter" {
						if *label.Value != dcName {
							continue Metric
						}
					}
					if *label.Name == "progress" {
						status = *label.Value
					}
				}
				if *m.Gauge.Value > 0 {
					return status, nil
				}
			}
		}
	}
	return "", fmt.Errorf("No datacenter status found")
}

func TestDatacenterConditionMetrics(t *testing.T) {
	require := require.New(t)

	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dc1",
			Namespace: "ns",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName: "cluster1",
		},
		Status: api.CassandraDatacenterStatus{
			Conditions: []api.DatacenterCondition{
				{
					Type:   api.DatacenterReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	SetDatacenterConditionMetric(dc, api.DatacenterReady, corev1.ConditionTrue)

	status, err := getCurrentDatacenterCondition("dc1", api.DatacenterReady)
	require.NoError(err)
	require.Equal(float64(1), status)

	SetDatacenterConditionMetric(dc, api.DatacenterInitialized, corev1.ConditionTrue)
	SetDatacenterConditionMetric(dc, api.DatacenterReady, corev1.ConditionFalse)

	status, err = getCurrentDatacenterCondition("dc1", api.DatacenterReady)
	require.NoError(err)
	require.Equal(float64(0), status)

	status, err = getCurrentDatacenterCondition("dc1", api.DatacenterInitialized)
	require.NoError(err)
	require.Equal(float64(1), status)
}

func getCurrentDatacenterCondition(dcName string, conditionType api.DatacenterConditionType) (float64, error) {
	return GetMetricValue("cass_operator_datacenter_status", map[string]string{"datacenter": dcName, "condition": string(conditionType)})
}
