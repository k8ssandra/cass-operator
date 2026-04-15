// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"net/http"
	"sync"
	"testing"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/internal/result"
	"github.com/k8ssandra/cass-operator/pkg/httphelper"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

	labels := make(map[string]string)
	labels[api.CassNodeState] = stateDecommissioning

	rc.dcPods = []*corev1.Pod{{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "pod-1",
			Labels: labels,
		},
		Status: corev1.PodStatus{},
	}}

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
}
