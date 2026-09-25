// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/go-logr/logr/funcr"
	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/internal/result"
	"github.com/k8ssandra/cass-operator/pkg/httphelper"
	"github.com/k8ssandra/cass-operator/pkg/monitoring"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRemoveDecommissionedPodFromZeroReplicaSts(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()
	require := require.New(t)

	var logs []string
	rc.ReqLogger = funcr.NewJSON(func(log string) {
		logs = append(logs, log)
	}, funcr.Options{})

	replicas := int32(0)
	rc.statefulSets = []*appsv1.StatefulSet{{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-dc2-default-sts",
			Labels: map[string]string{
				api.RackLabel: "default",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
		},
	}}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dc2-default-sts-0",
			Namespace: "remove-dc",
			Labels: map[string]string{
				api.ClusterLabel:    "test",
				api.DatacenterLabel: "dc2",
				api.RackLabel:       "default",
			},
		},
	}
	defer monitoring.RemovePodStatusMetric(pod)

	statuses := []monitoring.PodStatus{
		monitoring.PodStatusInitializing,
		monitoring.PodStatusReady,
		monitoring.PodStatusPending,
		monitoring.PodStatusError,
		monitoring.PodStatusDecommissioning,
		monitoring.PodStatusTerminating,
	}
	for _, status := range statuses {
		status := strings.ToLower(string(status))
		monitoring.PodStatusVec.WithLabelValues(
			pod.Namespace,
			pod.Labels[api.ClusterLabel],
			pod.Labels[api.DatacenterLabel],
			pod.Labels[api.RackLabel],
			pod.Name,
			status,
		).Set(1)
		_, err := monitoring.GetMetricValue("cass_operator_datacenter_pods_status", map[string]string{
			"namespace":  pod.Namespace,
			"cluster":    pod.Labels[api.ClusterLabel],
			"datacenter": pod.Labels[api.DatacenterLabel],
			"rack":       pod.Labels[api.RackLabel],
			"pod":        pod.Name,
			"status":     status,
		})
		require.NoError(err, "expected %s pod status metric to be registered", status)
	}

	require.NoError(rc.RemoveDecommissionedPodFromSts(pod), "expected an already scaled-down StatefulSet to be a no-op")
	require.NotContains(strings.Join(logs, "\n"), "sts--1", "expected cleanup not to look for a negative pod ordinal")
	require.Equal(int32(0), *rc.statefulSets[0].Spec.Replicas, "expected replicas to remain at zero")
	for _, status := range statuses {
		status := strings.ToLower(string(status))
		_, err := monitoring.GetMetricValue("cass_operator_datacenter_pods_status", map[string]string{
			"namespace":  pod.Namespace,
			"cluster":    pod.Labels[api.ClusterLabel],
			"datacenter": pod.Labels[api.DatacenterLabel],
			"rack":       pod.Labels[api.RackLabel],
			"pod":        pod.Name,
			"status":     status,
		})
		require.Error(err, "expected %s pod status metric to be removed", status)
	}
}

func TestRetryDecommissionNode(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()
	state := "UP"

	rc.Datacenter.SetCondition(api.DatacenterCondition{
		Status: corev1.ConditionTrue,
		Type:   api.DatacenterScalingDown,
	})

	wg := &sync.WaitGroup{}
	wg.Add(1)
	server := newFakeMgmtApiServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.RequestURI() {
		case "/api/v0/metadata/endpoints":
			_, _ = w.Write([]byte(`{"entity":[{"IS_LOCAL":"true","STATUS":"NORMAL"}]}`))
		case "/api/v0/metadata/versions/features":
			http.NotFound(w, r)
		case "/api/v0/ops/node/decommission?force=true":
			w.WriteHeader(http.StatusBadRequest)
			wg.Done()
		default:
			http.NotFound(w, r)
		}
	}))
	rc.NodeMgmtClient = server.client(rc.ReqLogger)

	labels := make(map[string]string)
	labels[api.CassNodeState] = stateDecommissioning

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "pod-1",
			Labels: labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "cassandra",
				},
			},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "cassandra",
					Ready: true,
				},
			},
		},
	}
	server.attachToPod(t, pod)
	rc.dcPods = []*corev1.Pod{pod}

	epData := httphelper.CassMetadataEndpoints{
		Entity: []httphelper.EndpointState{
			{
				RpcAddress: pod.Status.PodIP,
				Status:     state,
			},
		},
	}
	r := rc.CheckDecommissioningNodes(epData)
	if r != result.RequeueSoon(5) {
		t.Fatalf("expected result of result.RequeueSoon(5) but got %s", r)
	}
	wg.Wait()
	server.assertCallCount(t, "/api/v0/metadata/endpoints", 1)
	server.assertCallCount(t, "/api/v0/metadata/versions/features", 1)
	server.assertCallCount(t, "/api/v0/ops/node/decommission", 1)
}

func TestRemoveResourcesWhenDone(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()
	state := "LEFT"

	rc.Datacenter.SetCondition(api.DatacenterCondition{
		Status: corev1.ConditionTrue,
		Type:   api.DatacenterScalingDown,
	})
	server := newFakeMgmtApiServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v0/metadata/endpoints" {
			_, _ = w.Write([]byte(`{"entity":[{"IS_LOCAL":"true","STATUS":"LEFT"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	rc.NodeMgmtClient = server.client(rc.ReqLogger)

	labels := make(map[string]string)
	labels[api.CassNodeState] = stateDecommissioning

	rc.dcPods = []*corev1.Pod{{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "pod-1",
			Labels: labels,
		},
		Spec:   corev1.PodSpec{Containers: []corev1.Container{{Name: "cassandra"}}},
		Status: corev1.PodStatus{},
	}}
	server.attachToPod(t, rc.dcPods[0])

	makeInt := func(i int32) *int32 {
		return &i
	}
	ssLabels := make(map[string]string)
	rc.statefulSets = []*appsv1.StatefulSet{{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "ss-1",
			Labels: ssLabels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: makeInt(1),
		},
	}}

	epData := httphelper.CassMetadataEndpoints{
		Entity: []httphelper.EndpointState{
			{
				RpcAddress: rc.dcPods[0].Status.PodIP,
				Status:     state,
			},
		},
	}

	r := rc.CheckDecommissioningNodes(epData)
	if r != result.RequeueSoon(5) {
		t.Fatalf("expected result of blah but got %s", r)
	}
	server.assertCallCount(t, "/api/v0/metadata/endpoints", 1)
}

func TestCheckDecommissioningNodesRequiresLocalLeft(t *testing.T) {
	tests := []struct {
		name          string
		peerStatus    string
		localResponse string
		localCode     int
		annotation    string
		podReady      bool
		wantCleaned   bool
		wantCalls     int
		wantRetries   int
		wantError     bool
	}{
		{name: "peer left and local left", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","STATUS":"LEFT"}]}`, wantCleaned: true, wantCalls: 1},
		{name: "peer left but local normal", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","STATUS":"NORMAL"}]}`, wantCalls: 1},
		{name: "peer left but local still leaving", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","STATUS":"LEAVING"}]}`, podReady: true, wantCalls: 1, wantRetries: 1},
		{name: "peer left but only remote left", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"false","STATUS":"LEFT"}]}`, wantCalls: 1},
		{name: "gone from peer and local left", localResponse: `{"entity":[{"IS_LOCAL":"true","STATUS_WITH_PORT":"LEFT"}]}`, wantCleaned: true, wantCalls: 1},
		{name: "gone from peer but local normal", localResponse: `{"entity":[{"IS_LOCAL":"true","STATUS":"NORMAL"}]}`, wantCalls: 1},
		{name: "local request fails", peerStatus: "LEFT", localCode: http.StatusInternalServerError, wantCalls: 1, wantError: true},
		{name: "override bypasses unavailable pod", peerStatus: "LEFT", localCode: http.StatusInternalServerError, annotation: "true", wantCleaned: true, wantCalls: 1},
		{name: "override permits missing peer entry and unavailable pod", localCode: http.StatusInternalServerError, annotation: "true", wantCleaned: true, wantCalls: 1},
		{name: "override does not bypass local normal", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","STATUS":"NORMAL"}]}`, annotation: "true", wantCalls: 1},
		{name: "false annotation does not bypass", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","STATUS":"NORMAL"}]}`, annotation: "false", wantCalls: 1},
		{name: "local left is authoritative even when peer reports normal", peerStatus: "NORMAL", localResponse: `{"entity":[{"IS_LOCAL":"true","STATUS":"LEFT"}]}`, wantCleaned: true, wantCalls: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc, _, cleanupMockScr := setupTest()
			defer cleanupMockScr()
			rc.Datacenter.SetCondition(api.DatacenterCondition{Status: corev1.ConditionTrue, Type: api.DatacenterScalingDown})
			if tt.annotation != "" {
				rc.Datacenter.Annotations = map[string]string{api.SkipLocalDecommissionCheckAnnotation: tt.annotation}
			}

			server := newFakeMgmtApiServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v0/metadata/endpoints" {
					if tt.localCode != 0 {
						w.WriteHeader(tt.localCode)
						return
					}
					_, _ = w.Write([]byte(tt.localResponse))
					return
				}
				if r.URL.Path == "/api/v0/metadata/versions/features" {
					_, _ = w.Write([]byte(`{"cassandra_version":"4.0","features":["async_sstable_tasks"]}`))
					return
				}
				if r.URL.Path == "/api/v1/ops/node/decommission" {
					_, _ = w.Write([]byte(`"job-1"`))
					return
				}
				http.NotFound(w, r)
			}))
			rc.NodeMgmtClient = server.client(rc.ReqLogger)
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Labels: map[string]string{api.CassNodeState: stateDecommissioning, api.RackLabel: "default"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "cassandra"}}},
				Status:     corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "cassandra", Ready: tt.podReady}}},
			}
			server.attachToPod(t, pod)
			rc.dcPods = []*corev1.Pod{pod}
			rc.Datacenter.Status.NodeStatuses = api.CassandraStatusMap{pod.Name: {HostID: "target-host"}}
			zero := int32(0)
			rc.statefulSets = []*appsv1.StatefulSet{{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{api.RackLabel: "default"}}, Spec: appsv1.StatefulSetSpec{Replicas: &zero}}}

			peer := httphelper.EndpointState{RpcAddress: "other-pod-ip", HostID: "other-host", Status: "NORMAL"}
			if tt.peerStatus != "" {
				peer.RpcAddress = pod.Status.PodIP
				peer.HostID = "target-host"
				peer.Status = tt.peerStatus
			}
			res := rc.CheckDecommissioningNodes(httphelper.CassMetadataEndpoints{Entity: []httphelper.EndpointState{peer}})
			_, err := res.Output()
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			_, remains := rc.Datacenter.Status.NodeStatuses[pod.Name]
			require.Equal(t, tt.wantCleaned, !remains)
			server.assertCallCount(t, "/api/v0/metadata/endpoints", tt.wantCalls)
			server.assertCallCount(t, "/api/v0/metadata/versions/features", tt.wantRetries)
			server.assertCallCount(t, "/api/v1/ops/node/decommission", tt.wantRetries)
		})
	}
}

func TestReconcileAllRacksDoesNotCleanUpDecommissioningNodeWhenMetadataRequestFails(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	rc.Datacenter.Spec.Size = 1
	rc.Datacenter.SetCondition(api.DatacenterCondition{
		Status: corev1.ConditionTrue,
		Type:   api.DatacenterScalingDown,
	})

	server := newFakeMgmtApiServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v0/metadata/endpoints" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		http.NotFound(w, r)
	}))
	rc.NodeMgmtClient = server.client(rc.ReqLogger)

	statefulSet, err := newStatefulSetForCassandraDatacenter(
		nil,
		"default",
		rc.Datacenter,
		2,
		imageRegistry,
	)
	require.NoError(t, err)
	statefulSet.Status.ObservedGeneration = statefulSet.Generation

	podName := getStatefulSetPodNameForIdx(statefulSet, 1)
	pvcName := fmt.Sprintf("%s-%s", PvcName, podName)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: rc.Datacenter.Namespace,
			Labels: map[string]string{
				api.ClusterLabel:    rc.Datacenter.Spec.ClusterName,
				api.DatacenterLabel: rc.Datacenter.Name,
				api.CassNodeState:   stateDecommissioning,
				api.RackLabel:       "default",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "cassandra"}},
			Volumes: []corev1.Volume{{
				Name: "server-data",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName},
				},
			}},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{Name: "cassandra", Ready: true}},
		},
	}
	server.attachToPod(t, pod)

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: pvcName, Namespace: rc.Datacenter.Namespace},
	}
	remainingPod := pod.DeepCopy()
	remainingPod.Name = getStatefulSetPodNameForIdx(statefulSet, 0)
	remainingPod.Labels[api.CassNodeState] = stateStarted
	remainingPVC := pvc.DeepCopy()
	remainingPVC.Name = fmt.Sprintf("%s-%s", PvcName, remainingPod.Name)
	remainingPod.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = remainingPVC.Name
	rc.Client = fake.NewClientBuilder().
		WithScheme(setupScheme()).
		WithStatusSubresource(rc.Datacenter, statefulSet).
		WithRuntimeObjects(rc.Datacenter, statefulSet, pod, pvc, remainingPod, remainingPVC).
		WithIndex(&corev1.Pod{}, podPVCClaimNameField, podPVCClaimNames).
		Build()
	rc.desiredRackInformation = []*RackInformation{{RackName: "default", NodeCount: 1, SeedCount: 1}}
	rc.statefulSets = []*appsv1.StatefulSet{statefulSet}

	_, err = rc.ReconcileAllRacks()
	require.Error(t, err)
	server.assertCallCount(t, "/api/v0/metadata/endpoints", 2)
	server.assertCallCount(t, "/api/v0/ops/node/decommission", 0)

	storedPVC := &corev1.PersistentVolumeClaim{}
	require.NoError(t, rc.Client.Get(rc.Ctx, types.NamespacedName{
		Name: pvcName, Namespace: rc.Datacenter.Namespace,
	}, storedPVC))

	storedStatefulSet := &appsv1.StatefulSet{}
	require.NoError(t, rc.Client.Get(rc.Ctx, types.NamespacedName{
		Name: statefulSet.Name, Namespace: statefulSet.Namespace,
	}, storedStatefulSet))
	require.Equal(t, int32(2), *storedStatefulSet.Spec.Replicas)
}

func TestDecommissionNodesRequiresMetadata(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	rc.Datacenter.Spec.Size = 1
	two := int32(2)
	rc.statefulSets = []*appsv1.StatefulSet{{
		Spec: appsv1.StatefulSetSpec{Replicas: &two},
	}}
	reconcileResult := rc.DecommissionNodes(httphelper.CassMetadataEndpoints{})
	require.True(t, reconcileResult.Completed())
	_, err := reconcileResult.Output()
	require.Error(t, err)
	require.NotEqual(t, corev1.ConditionTrue, rc.Datacenter.GetConditionStatus(api.DatacenterScalingDown))
	require.Equal(t, int32(2), *rc.statefulSets[0].Spec.Replicas)
}
