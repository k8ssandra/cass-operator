// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"context"
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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
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
	require.NoError(rc.Client.Create(rc.Ctx, rc.statefulSets[0]))
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
			_, _ = w.Write([]byte(`{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"NORMAL"}]}`))
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
	rc.Datacenter.Status.NodeStatuses = api.CassandraStatusMap{pod.Name: {HostID: "target-host"}}
	require.NoError(t, rc.Client.Create(rc.Ctx, pod))
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

func setupDecommissioningPodTest(t *testing.T, handler http.HandlerFunc) (*ReconciliationContext, *fakeMgmtApiServer, *corev1.Pod, *appsv1.StatefulSet) {
	t.Helper()
	rc, _, cleanup := setupTest()
	t.Cleanup(cleanup)
	rc.Datacenter.SetCondition(api.DatacenterCondition{Status: corev1.ConditionTrue, Type: api.DatacenterScalingDown})

	server := newFakeMgmtApiServer(t, handler)
	rc.NodeMgmtClient = server.client(rc.ReqLogger)
	sts, err := newStatefulSetForCassandraDatacenter(nil, "default", rc.Datacenter, 2, imageRegistry)
	require.NoError(t, err)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: getStatefulSetPodNameForIdx(sts, 1), Namespace: rc.Datacenter.Namespace, UID: "target-pod",
			Labels: map[string]string{api.CassNodeState: stateDecommissioning, api.RackLabel: "default"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "cassandra"}},
			Volumes: []corev1.Volume{{Name: "server-data", VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "target-pvc"},
			}}},
		},
	}
	server.attachToPod(t, pod)
	remainingPod := pod.DeepCopy()
	remainingPod.Name = getStatefulSetPodNameForIdx(sts, 0)
	remainingPod.UID = "remaining-pod"
	remainingPod.Labels[api.CassNodeState] = stateStarted
	remainingPod.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = "remaining-pvc"
	rc.Datacenter.Status.NodeStatuses = api.CassandraStatusMap{
		pod.Name: {HostID: "target-host"}, remainingPod.Name: {HostID: "remaining-host"},
	}
	rc.Client = fake.NewClientBuilder().WithScheme(setupScheme()).
		WithStatusSubresource(rc.Datacenter, sts).
		WithRuntimeObjects(rc.Datacenter, sts, pod, remainingPod,
			&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "target-pvc", Namespace: pod.Namespace, UID: "target-pvc"}},
			&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "remaining-pvc", Namespace: pod.Namespace, UID: "remaining-pvc"}}).
		Build()
	rc.APIReader = rc.Client
	require.NoError(t, rc.Client.Get(rc.Ctx, client.ObjectKeyFromObject(pod), pod))
	require.NoError(t, rc.Client.Get(rc.Ctx, client.ObjectKeyFromObject(sts), sts))
	require.NoError(t, rc.Client.Get(rc.Ctx, client.ObjectKeyFromObject(rc.Datacenter), rc.Datacenter))
	rc.dcPods = []*corev1.Pod{pod, remainingPod}
	rc.statefulSets = []*appsv1.StatefulSet{sts}
	return rc, server, pod, sts
}

func TestRemoveResourcesWhenDone(t *testing.T) {
	for _, failFirstDelete := range []bool{false, true} {
		t.Run(fmt.Sprintf("retry_after_delete_failure_%t", failFirstDelete), func(t *testing.T) {
			firstRequest := true
			rc, server, pod, sts := setupDecommissioningPodTest(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v0/metadata/endpoints" || !firstRequest {
					http.Error(w, "pod is no longer reachable", http.StatusServiceUnavailable)
					return
				}
				firstRequest = false
				_, _ = w.Write([]byte(`{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"LEFT"}]}`))
			})
			var logs []string
			rc.ReqLogger = funcr.NewJSON(func(log string) { logs = append(logs, log) }, funcr.Options{Verbosity: 1})
			if failFirstDelete {
				fail := true
				rc.Client = interceptor.NewClient(rc.Client.(client.WithWatch), interceptor.Funcs{
					Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						if obj.GetName() == "target-pvc" && fail {
							fail = false
							return fmt.Errorf("temporary PVC deletion failure")
						}
						return c.Delete(ctx, obj, opts...)
					},
				})
			}
			epData := httphelper.CassMetadataEndpoints{Entity: []httphelper.EndpointState{{HostID: "target-host", Status: "LEFT"}}}
			cachedSts := sts.DeepCopy()
			_, err := rc.CheckDecommissioningNodes(epData).Output()
			if failFirstDelete {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, rc.Client.Get(rc.Ctx, client.ObjectKeyFromObject(sts), sts))
			if failFirstDelete {
				require.Equal(t, int32(2), *sts.Spec.Replicas, "cleanup failure must leave the completed pod available for retries")
			} else {
				require.Equal(t, int32(1), *sts.Spec.Replicas)
			}
			require.NoError(t, rc.Client.Get(rc.Ctx, client.ObjectKeyFromObject(pod), pod))
			require.Equal(t, stateDecommissioned, pod.Labels[api.CassNodeState])

			// Observe the persisted completion label through the normal pod cache.
			rc.dcPods[0] = pod
			rc.statefulSets[0] = cachedSts
			require.NoError(t, rc.Client.Get(rc.Ctx, client.ObjectKeyFromObject(rc.Datacenter), rc.Datacenter))
			require.Equal(t, result.RequeueSoon(5), rc.CheckDecommissioningNodes(httphelper.CassMetadataEndpoints{}))
			// The completed pod can remain visible while the StatefulSet controller
			// processes the replica reduction. Further reconciles only wait for it.
			require.Equal(t, result.RequeueSoon(5), rc.CheckDecommissioningNodes(httphelper.CassMetadataEndpoints{}))
			server.assertCallCount(t, "/api/v0/metadata/endpoints", 1)
			require.NoError(t, rc.Client.Get(rc.Ctx, client.ObjectKeyFromObject(sts), sts))
			require.Equal(t, int32(1), *sts.Spec.Replicas)
			require.NotContains(t, rc.Datacenter.Status.NodeStatuses, pod.Name)
			require.Contains(t, rc.Datacenter.Status.NodeStatuses, getStatefulSetPodNameForIdx(sts, 0))
			require.True(t, apierrors.IsNotFound(rc.Client.Get(rc.Ctx, types.NamespacedName{Name: "target-pvc", Namespace: pod.Namespace}, &corev1.PersistentVolumeClaim{})))
			require.NoError(t, rc.Client.Get(rc.Ctx, types.NamespacedName{Name: "remaining-pvc", Namespace: pod.Namespace}, &corev1.PersistentVolumeClaim{}))
			require.Equal(t, 1, strings.Count(strings.Join(logs, "\n"), "Node finished decommissioning"))
			require.NotContains(t, strings.Join(logs, "\n"), "Could not find last matching pod")
			wantCleanupAttempts := 1
			if failFirstDelete {
				wantCleanupAttempts = 2
			}
			require.Equal(t, wantCleanupAttempts, strings.Count(strings.Join(logs, "\n"), "Deleting pod PVCs"))
			require.Equal(t, 1, strings.Count(strings.Join(logs, "\n"), "UpdateRackNodeCount in STS"))

			require.NoError(t, rc.Client.Delete(rc.Ctx, pod))
			rc.dcPods = rc.dcPods[1:]
			require.False(t, rc.CheckDecommissioningNodes(epData).Completed())
			require.Equal(t, corev1.ConditionFalse, rc.Datacenter.GetConditionStatus(api.DatacenterScalingDown))
		})
	}
}

func TestCheckDecommissioningNodesWaitsForCachedCompletion(t *testing.T) {
	rc, server, pod, sts := setupDecommissioningPodTest(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"LEFT"}]}`))
	})
	var logs []string
	rc.ReqLogger = funcr.NewJSON(func(log string) { logs = append(logs, log) }, funcr.Options{Verbosity: 1})
	cachedPod := pod.DeepCopy()
	cachedDC := rc.Datacenter.DeepCopy()
	epData := httphelper.CassMetadataEndpoints{Entity: []httphelper.EndpointState{{HostID: "target-host", Status: "LEFT"}}}
	require.Equal(t, result.RequeueSoon(5), rc.CheckDecommissioningNodes(epData))

	// Until the cache catches up, verification can repeat, but its stale Pod
	// resource version must prevent it from repeating the completion and cleanup.
	rc.dcPods[0] = cachedPod
	rc.Datacenter = cachedDC
	require.Equal(t, result.RequeueSoon(5), rc.CheckDecommissioningNodes(epData))
	require.Equal(t, stateDecommissioning, cachedPod.Labels[api.CassNodeState])
	server.assertCallCount(t, "/api/v0/metadata/endpoints", 2)

	require.NoError(t, rc.Client.Get(rc.Ctx, client.ObjectKeyFromObject(pod), pod))
	require.Equal(t, stateDecommissioned, pod.Labels[api.CassNodeState])
	rc.dcPods[0] = pod
	require.NoError(t, rc.Client.Get(rc.Ctx, client.ObjectKeyFromObject(rc.Datacenter), rc.Datacenter))
	require.Equal(t, result.RequeueSoon(5), rc.CheckDecommissioningNodes(httphelper.CassMetadataEndpoints{}))
	server.assertCallCount(t, "/api/v0/metadata/endpoints", 2)
	require.NoError(t, rc.Client.Get(rc.Ctx, client.ObjectKeyFromObject(sts), sts))
	require.Equal(t, int32(1), *sts.Spec.Replicas)
	require.NotContains(t, rc.Datacenter.Status.NodeStatuses, pod.Name)
	require.Contains(t, rc.Datacenter.Status.NodeStatuses, getStatefulSetPodNameForIdx(sts, 0))
	require.NoError(t, rc.Client.Get(rc.Ctx, types.NamespacedName{Name: "remaining-pvc", Namespace: pod.Namespace}, &corev1.PersistentVolumeClaim{}))
	require.Equal(t, 1, strings.Count(strings.Join(logs, "\n"), "Node finished decommissioning"))
	require.Equal(t, 1, strings.Count(strings.Join(logs, "\n"), "Deleting pod PVCs"))
	require.Equal(t, 1, strings.Count(strings.Join(logs, "\n"), "UpdateRackNodeCount in STS"))
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
		unknownHostID bool
		noNodeStatus  bool
		noPeerData    bool
	}{
		{name: "peer left and local left", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"LEFT"}]}`, wantCleaned: true, wantCalls: 1},
		{name: "peer left but local normal", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"NORMAL"}]}`, wantCalls: 1},
		{name: "peer left but local still leaving", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"LEAVING"}]}`, podReady: true, wantCalls: 1, wantRetries: 1},
		{name: "peer left but only remote left", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"false","HOST_ID":"target-host","STATUS":"LEFT"}]}`, wantCalls: 1},
		{name: "gone from peer and local left", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS_WITH_PORT":"LEFT"}]}`, wantCalls: 1},
		{name: "peer left and local left with port", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS_WITH_PORT":"LEFT"}]}`, wantCleaned: true, wantCalls: 1},
		{name: "gone from peer but local normal", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"NORMAL"}]}`, wantCalls: 1},
		{name: "local request fails", peerStatus: "LEFT", localCode: http.StatusInternalServerError, wantCalls: 1, wantError: true},
		{name: "override bypasses unavailable pod", peerStatus: "LEFT", localCode: http.StatusInternalServerError, annotation: "true", wantCleaned: true, wantCalls: 1},
		{name: "override cannot bypass missing peer entry", localCode: http.StatusInternalServerError, annotation: "true", wantCalls: 1},
		{name: "false annotation does not bypass", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"NORMAL"}]}`, annotation: "false", wantCalls: 1},
		{name: "fallback still requires peer completion", peerStatus: "NORMAL", localCode: http.StatusInternalServerError, annotation: "true", wantCalls: 1},
		{name: "local left but peer normal", peerStatus: "NORMAL", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"LEFT"}]}`, wantCalls: 1},
		{name: "local left but peer still leaving", peerStatus: "LEAVING", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"LEFT"}]}`, podReady: true, wantCalls: 1},
		{name: "override does not bypass local normal", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"NORMAL"}]}`, annotation: "true", wantCalls: 1},
		{name: "local left belongs to a different host", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"other-host","STATUS":"LEFT"}]}`, wantCalls: 1},
		{name: "local left has no host ID", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","STATUS":"LEFT"}]}`, wantCalls: 1},
		{name: "empty host IDs cannot prove identity", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","STATUS":"LEFT"}]}`, unknownHostID: true, wantCalls: 1},
		{name: "local left cannot use empty peer metadata", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"LEFT"}]}`, noPeerData: true, podReady: true, wantError: true},
		{name: "empty peer metadata must not retry decommission", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"NORMAL"}]}`, noPeerData: true, podReady: true, wantError: true},
		{name: "fallback cannot use empty peer metadata", localCode: http.StatusInternalServerError, annotation: "true", noPeerData: true, podReady: true, wantError: true},
		{name: "never bootstrapped pod needs no local metadata", noNodeStatus: true, localCode: http.StatusInternalServerError, wantCleaned: true},
		{name: "never bootstrapped pod still requires fetched metadata", noNodeStatus: true, localCode: http.StatusInternalServerError, noPeerData: true, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc, server, pod, sts := setupDecommissioningPodTest(t, func(w http.ResponseWriter, r *http.Request) {
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
			})
			if tt.annotation != "" {
				rc.Datacenter.Annotations = map[string]string{api.SkipLocalDecommissionCheckAnnotation: tt.annotation}
			}
			if tt.unknownHostID {
				rc.Datacenter.Status.NodeStatuses[pod.Name] = api.CassandraNodeStatus{}
			}
			if tt.noNodeStatus {
				delete(rc.Datacenter.Status.NodeStatuses, pod.Name)
				require.NoError(t, rc.Client.Status().Update(rc.Ctx, rc.Datacenter))
			}
			pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "cassandra", Ready: tt.podReady}}
			require.NoError(t, rc.Client.Status().Update(rc.Ctx, pod))

			peer := httphelper.EndpointState{RpcAddress: "other-pod-ip", HostID: "other-host", Status: "NORMAL"}
			if tt.peerStatus != "" {
				peer.RpcAddress = pod.Status.PodIP
				peer.HostID = "target-host"
				peer.Status = tt.peerStatus
			}
			epData := httphelper.CassMetadataEndpoints{Entity: []httphelper.EndpointState{peer}}
			if tt.noPeerData {
				epData.Entity = nil
			}
			res := rc.CheckDecommissioningNodes(epData)
			_, err := res.Output()
			if tt.wantError {
				require.Error(t, err)
				if tt.noPeerData {
					require.EqualError(t, err, fmt.Sprintf("cannot check decommissioning node %s without Cassandra metadata", pod.Name))
				}
			} else {
				require.NoError(t, err)
			}
			_, remains := rc.Datacenter.Status.NodeStatuses[pod.Name]
			require.Equal(t, tt.wantCleaned || tt.noNodeStatus, !remains)
			require.Contains(t, rc.Datacenter.Status.NodeStatuses, getStatefulSetPodNameForIdx(sts, 0))
			require.NoError(t, rc.Client.Get(rc.Ctx, client.ObjectKeyFromObject(sts), sts))
			wantReplicas := int32(2)
			if tt.wantCleaned {
				wantReplicas = 1
			}
			require.Equal(t, wantReplicas, *sts.Spec.Replicas)
			pvcErr := rc.Client.Get(rc.Ctx, types.NamespacedName{Name: "target-pvc", Namespace: pod.Namespace}, &corev1.PersistentVolumeClaim{})
			if tt.wantCleaned {
				require.True(t, apierrors.IsNotFound(pvcErr))
			} else {
				require.NoError(t, pvcErr)
			}
			require.NoError(t, rc.Client.Get(rc.Ctx, types.NamespacedName{Name: "remaining-pvc", Namespace: pod.Namespace}, &corev1.PersistentVolumeClaim{}))
			server.assertCallCount(t, "/api/v0/metadata/endpoints", tt.wantCalls)
			server.assertCallCount(t, "/api/v0/metadata/versions/features", tt.wantRetries)
			server.assertCallCount(t, "/api/v1/ops/node/decommission", tt.wantRetries)
		})
	}
}

func TestDecommissionCleanupSkipsRetainedPod(t *testing.T) {
	rc, _, _, sts := setupDecommissioningPodTest(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	pod := rc.dcPods[1]
	res := rc.cleanUpAfterDecommissionedPod(pod)
	require.Nil(t, res)
	require.NoError(t, rc.Client.Get(rc.Ctx, client.ObjectKeyFromObject(sts), sts))
	require.Equal(t, int32(2), *sts.Spec.Replicas)
	require.NoError(t, rc.Client.Get(rc.Ctx, types.NamespacedName{Name: "remaining-pvc", Namespace: pod.Namespace}, &corev1.PersistentVolumeClaim{}))
	require.Contains(t, rc.Datacenter.Status.NodeStatuses, pod.Name)
}

func TestCheckDecommissioningNodesRejectsPodChangedDuringVerification(t *testing.T) {
	var changePod func() error
	rc, _, pod, sts := setupDecommissioningPodTest(t, func(w http.ResponseWriter, r *http.Request) {
		if err := changePod(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"LEFT"}]}`))
	})
	changePod = func() error {
		livePod := &corev1.Pod{}
		if err := rc.Client.Get(rc.Ctx, client.ObjectKeyFromObject(pod), livePod); err != nil {
			return err
		}
		livePod.Labels[api.CassNodeState] = stateStarted
		return rc.Client.Update(rc.Ctx, livePod)
	}
	epData := httphelper.CassMetadataEndpoints{Entity: []httphelper.EndpointState{{HostID: "target-host", Status: "LEFT"}}}
	require.Equal(t, result.RequeueSoon(5), rc.CheckDecommissioningNodes(epData))
	require.Equal(t, stateDecommissioning, pod.Labels[api.CassNodeState])
	require.NoError(t, rc.Client.Get(rc.Ctx, client.ObjectKeyFromObject(sts), sts))
	require.Equal(t, int32(2), *sts.Spec.Replicas)
	require.NoError(t, rc.Client.Get(rc.Ctx, types.NamespacedName{Name: "target-pvc", Namespace: pod.Namespace}, &corev1.PersistentVolumeClaim{}))
	require.Contains(t, rc.Datacenter.Status.NodeStatuses, pod.Name)
}

func TestGetCassMetadataEndpointsSkipsDecommissioningPods(t *testing.T) {
	for _, state := range []string{stateDecommissioning, stateDecommissioned} {
		t.Run(state, func(t *testing.T) {
			rc, targetServer, pod, _ := setupDecommissioningPodTest(t, func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "decommissioning pod", http.StatusServiceUnavailable)
			})
			healthyServer := newFakeMgmtApiServer(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"entity":[{"HOST_ID":"remaining-host","STATUS":"NORMAL"}]}`))
			})
			pod.Labels[api.CassNodeState] = state
			pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "cassandra", Ready: true}}
			rc.dcPods[1].Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "cassandra", Ready: true}}
			healthyServer.attachToPod(t, rc.dcPods[1])
			rc.clusterPods = rc.dcPods
			metadata := rc.getCassMetadataEndpoints()
			require.Len(t, metadata.Entity, 1)
			require.Equal(t, "remaining-host", metadata.Entity[0].HostID)
			targetServer.assertCallCount(t, "/api/v0/metadata/endpoints", 0)
			healthyServer.assertCallCount(t, "/api/v0/metadata/endpoints", 1)
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
	rc.APIReader = rc.Client
	rc.desiredRackInformation = []*RackInformation{{RackName: "default", NodeCount: 1, SeedCount: 1}}
	rc.statefulSets = []*appsv1.StatefulSet{statefulSet}

	_, err = rc.ReconcileAllRacks()
	require.EqualError(t, err, fmt.Sprintf("cannot check decommissioning node %s without Cassandra metadata", pod.Name))
	server.assertCallCount(t, "/api/v0/metadata/endpoints", 1)
	server.assertCallCount(t, "/api/v0/metadata/versions/features", 0)
	server.assertCallCount(t, "/api/v1/ops/node/decommission", 0)
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
