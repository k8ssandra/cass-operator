// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/internal/result"
	"github.com/k8ssandra/cass-operator/pkg/httphelper"
	"github.com/k8ssandra/cass-operator/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRetryDecommissionNode(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()
	state := "UP"
	podIP := "192.168.101.11"

	mockClient := mocks.NewClient(t)
	rc.Client = mockClient

	rc.Datacenter.SetCondition(api.DatacenterCondition{
		Status: corev1.ConditionTrue,
		Type:   api.DatacenterScalingDown,
	})
	res := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(strings.NewReader("OK")),
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)
	mockHttpClient := mocks.NewHttpClient(t)
	mockHttpClient.On("Do",
		mock.MatchedBy(
			func(req *http.Request) bool {
				return req.URL.Path == "/api/v0/metadata/endpoints"
			})).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"NORMAL"}]}`)),
		}, nil).
		Once()

	mockHttpClient.On("Do",
		mock.MatchedBy(
			func(req *http.Request) bool {
				return req.URL.Path == "/api/v0/ops/node/decommission"
			})).
		Return(res, nil).
		Once().
		Run(func(args mock.Arguments) { wg.Done() })

	resFeatureSet := &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader("")),
	}

	mockHttpClient.On("Do",
		mock.MatchedBy(
			func(req *http.Request) bool {
				return req.URL.Path == "/api/v0/metadata/versions/features"
			})).
		Return(resFeatureSet, nil).
		Once()

	rc.NodeMgmtClient = httphelper.NodeMgmtClient{
		Client:   mockHttpClient,
		Log:      rc.ReqLogger,
		Protocol: "http",
	}

	labels := make(map[string]string)
	labels[api.CassNodeState] = stateDecommissioning

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "pod-1",
			Labels: labels,
		},
		Status: corev1.PodStatus{
			PodIP: podIP,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "cassandra",
					Ready: true,
				},
			},
		},
	}
	rc.Datacenter.Status.NodeStatuses = api.CassandraStatusMap{pod.Name: {HostID: "target-host"}}
	rc.dcPods = []*corev1.Pod{pod}

	epData := httphelper.CassMetadataEndpoints{
		Entity: []httphelper.EndpointState{
			{
				RpcAddress: podIP,
				Status:     state,
			},
		},
	}
	r := rc.CheckDecommissioningNodes(epData)
	if r != result.RequeueSoon(5) {
		t.Fatalf("expected result of result.RequeueSoon(5) but got %s", r)
	}
	wg.Wait()
}

func TestRemoveResourcesWhenDone(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()
	podIP := "192.168.101.11"
	state := "LEFT"

	mockClient := mocks.NewClient(t)
	rc.Client = mockClient
	rc.Datacenter.SetCondition(api.DatacenterCondition{
		Status: corev1.ConditionTrue,
		Type:   api.DatacenterScalingDown,
	})

	k8sMockClientStatusPatch(mockClient.Status().(*mocks.SubResourceClient), nil)

	mockHttpClient := mocks.NewHttpClient(t)
	mockHttpClient.On("Do",
		mock.MatchedBy(
			func(req *http.Request) bool {
				return req.URL.Path == "/api/v0/metadata/endpoints"
			})).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"LEFT"}]}`)),
		}, nil).
		Once()

	rc.NodeMgmtClient = httphelper.NodeMgmtClient{
		Client:   mockHttpClient,
		Log:      rc.ReqLogger,
		Protocol: "http",
	}

	labels := make(map[string]string)
	labels[api.CassNodeState] = stateDecommissioning

	rc.dcPods = []*corev1.Pod{{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "pod-1",
			Labels: labels,
		},
		Status: corev1.PodStatus{
			PodIP: podIP,
		},
	}}
	rc.Datacenter.Status.NodeStatuses = api.CassandraStatusMap{"pod-1": {HostID: "target-host"}}

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
				RpcAddress: podIP,
				HostID:     "target-host",
				Status:     state,
			},
		},
	}

	r := rc.CheckDecommissioningNodes(epData)
	if r != result.RequeueSoon(5) {
		t.Fatalf("expected result of blah but got %s", r)
	}
}

func TestCheckDecommissioningNodesRequiresLocalLeft(t *testing.T) {
	tests := []struct {
		name          string
		peerStatus    string
		localResponse string
		localCode     int
		annotation    string
		wantDone      bool
		wantError     bool
		unknownHostID bool
		noNodeStatus  bool
	}{
		{name: "peer left and local left", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"LEFT"}]}`, wantDone: true},
		{name: "peer left but local normal", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"NORMAL"}]}`, wantDone: false},
		{name: "peer left but local still leaving", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"LEAVING"}]}`, wantDone: false},
		{name: "peer left but only remote left", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"false","HOST_ID":"target-host","STATUS":"LEFT"}]}`, wantDone: false},
		{name: "gone from peer and local left", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS_WITH_PORT":"LEFT"}]}`, wantDone: false},
		{name: "peer left and local left with port", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS_WITH_PORT":"LEFT"}]}`, wantDone: true},
		{name: "gone from peer but local normal", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"NORMAL"}]}`, wantDone: false},
		{name: "local request fails", peerStatus: "LEFT", localCode: http.StatusInternalServerError, wantError: true},
		{name: "override bypasses unavailable pod", peerStatus: "LEFT", localCode: http.StatusInternalServerError, annotation: "true", wantDone: true},
		{name: "override cannot bypass missing peer entry", localCode: http.StatusInternalServerError, annotation: "true", wantDone: false},
		{name: "false annotation does not bypass", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"NORMAL"}]}`, annotation: "false", wantDone: false},
		{name: "fallback still requires peer completion", peerStatus: "NORMAL", localCode: http.StatusInternalServerError, annotation: "true", wantDone: false},
		{name: "local left but peer normal", peerStatus: "NORMAL", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"LEFT"}]}`, wantDone: false},
		{name: "local left but peer still leaving", peerStatus: "LEAVING", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"LEFT"}]}`, wantDone: false},
		{name: "override does not bypass local normal", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"target-host","STATUS":"NORMAL"}]}`, annotation: "true", wantDone: false},
		{name: "local left belongs to a different host", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","HOST_ID":"other-host","STATUS":"LEFT"}]}`, wantDone: false},
		{name: "local left has no host ID", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","STATUS":"LEFT"}]}`, wantDone: false},
		{name: "empty host IDs cannot prove identity", peerStatus: "LEFT", localResponse: `{"entity":[{"IS_LOCAL":"true","STATUS":"LEFT"}]}`, unknownHostID: true, wantDone: false},
		{name: "never bootstrapped pod needs no local metadata", noNodeStatus: true, wantDone: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc, _, cleanupMockScr := setupTest()
			defer cleanupMockScr()

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-1",
					Namespace: rc.Datacenter.Namespace,
					Labels:    map[string]string{api.CassNodeState: stateDecommissioning},
				},
				Status: corev1.PodStatus{
					PodIP: "192.168.101.11",
				},
			}

			if tt.annotation != "" {
				rc.Datacenter.Annotations = map[string]string{api.SkipLocalDecommissionCheckAnnotation: tt.annotation}
			}

			rc.Datacenter.Status.NodeStatuses = api.CassandraStatusMap{
				pod.Name: {HostID: "target-host"},
			}
			if tt.unknownHostID {
				rc.Datacenter.Status.NodeStatuses[pod.Name] = api.CassandraNodeStatus{}
			}
			if tt.noNodeStatus {
				delete(rc.Datacenter.Status.NodeStatuses, pod.Name)
			}

			mockHttpClient := mocks.NewHttpClient(t)
			if !tt.noNodeStatus {
				if tt.localCode != 0 {
					mockHttpClient.On("Do", mock.Anything).Return(&http.Response{
						StatusCode: tt.localCode,
						Body:       io.NopCloser(strings.NewReader("")),
					}, nil).Once()
				} else if tt.localResponse != "" {
					mockHttpClient.On("Do", mock.Anything).Return(&http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(tt.localResponse)),
					}, nil).Once()
				}
			}

			rc.NodeMgmtClient = httphelper.NodeMgmtClient{
				Client:   mockHttpClient,
				Log:      rc.ReqLogger,
				Protocol: "http",
			}

			peer := httphelper.EndpointState{RpcAddress: "other-pod-ip", HostID: "other-host", Status: "NORMAL"}
			if tt.peerStatus != "" {
				peer.RpcAddress = pod.Status.PodIP
				peer.HostID = "target-host"
				peer.Status = tt.peerStatus
			}
			epData := httphelper.CassMetadataEndpoints{Entity: []httphelper.EndpointState{peer}}

			done, err := rc.IsDoneDecommissioning(pod, epData, rc.Datacenter.Status.NodeStatuses)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantDone, done)
			}
		})
	}
}

func TestCheckDecommissioningNodesRequiresMetadata(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "pod-1",
			Labels: map[string]string{api.CassNodeState: stateDecommissioning},
		},
		Status: corev1.PodStatus{
			PodIP: "192.168.101.11",
		},
	}
	rc.dcPods = []*corev1.Pod{pod}
	rc.Datacenter.SetCondition(api.DatacenterCondition{
		Status: corev1.ConditionTrue,
		Type:   api.DatacenterScalingDown,
	})

	epData := httphelper.CassMetadataEndpoints{Entity: nil}
	res := rc.CheckDecommissioningNodes(epData)
	_, err := res.Output()
	require.EqualError(t, err, fmt.Sprintf("cannot check decommissioning node %s without Cassandra metadata", pod.Name))
}
