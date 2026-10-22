package reconciliation

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/internal/result"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var fqlEnabledConfig string = `{"cassandra-yaml": { 
	"full_query_logging_options": {
		"log_dir": "/var/log/cassandra/fql" 
		}
	}
}
`

var fullQueryIsSupported string = `{"cassandra_version": "4.0.1",
	"features": [
		"async_sstable_tasks",
		"full_query_logging"
	]
}
`

var (
	httpResponseFullQueryEnabled  string = `{"entity": true}`
	httpResponseFullQueryDisabled string = `{"entity": false}`
)

func fqlTestPod(name, namespace string, labels map[string]string) *corev1.Pod {
	podLabels := map[string]string{}
	for k, v := range labels {
		podLabels[k] = v
	}
	podLabels[api.CassNodeState] = stateStarted

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    podLabels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "cassandra",
			}},
		},
		Status: corev1.PodStatus{
			PodIP: "192.168.101.11",
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "cassandra",
				Ready: true,
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{
						StartedAt: metav1.NewTime(time.Now().Add(-time.Minute)),
					},
				},
			}},
		},
	}
}

func setupPodList(rc *ReconciliationContext) {
	pod := fqlTestPod("test-sts-0", rc.Datacenter.Namespace, rc.Datacenter.GetDatacenterLabels())
	rc.clusterPods = []*corev1.Pod{pod}
	rc.dcPods = []*corev1.Pod{pod}
}

func fqlFakeMgmtServer(t *testing.T, supportFQL bool, currentEnabled bool) *fakeMgmtApiServer {
	return newFakeMgmtApiServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.RequestURI() {
		case "/api/v0/metadata/versions/features":
			if !supportFQL {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fullQueryIsSupported))
		case "/api/v0/ops/node/fullquerylogging":
			w.Header().Set("Content-Type", "application/json")
			if currentEnabled {
				_, _ = w.Write([]byte(httpResponseFullQueryEnabled))
			} else {
				_, _ = w.Write([]byte(httpResponseFullQueryDisabled))
			}
		case "/api/v0/ops/node/fullquerylogging?enabled=true", "/api/v0/ops/node/fullquerylogging?enabled=false":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
}

func setupTestEnv() (*ReconciliationContext, func()) {
	rc, _, cleanupMockScr := setupTest()
	setupPodList(rc)
	rc.Datacenter.Spec.ServerType = "cassandra"
	rc.Datacenter.Spec.ServerVersion = "4.0.1"
	return rc, cleanupMockScr
}

func attachPods(t *testing.T, rc *ReconciliationContext, server *fakeMgmtApiServer) {
	objs := []runtime.Object{rc.Datacenter}
	for _, pod := range rc.clusterPods {
		server.attachToPod(t, pod)
		objs = append(objs, pod)
	}
	rc.Client = fake.NewClientBuilder().
		WithScheme(setupScheme()).
		WithStatusSubresource(rc.Datacenter).
		WithRuntimeObjects(objs...).
		WithIndex(&corev1.Pod{}, podPVCClaimNameField, podPVCClaimNames).
		Build()
	rc.NodeMgmtClient = server.client(rc.ReqLogger)
}

func setFQLEnabledAnnotation(pod *corev1.Pod) {
	metav1.SetMetaDataAnnotation(&pod.ObjectMeta, api.FullQueryLoggingEnabledAnnotation, "true")
}

func assertFQLEnabledAnnotation(t *testing.T, rc *ReconciliationContext, pod *corev1.Pod, expected bool) {
	t.Helper()
	stored := &corev1.Pod{}
	require.NoError(t, rc.Client.Get(rc.Ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, stored))
	require.Equal(t, expected, metav1.HasAnnotation(stored.ObjectMeta, api.FullQueryLoggingEnabledAnnotation))
}

func TestCheckFullQueryLoggingNoChangeEnabled(t *testing.T) {
	rc, cleanupMockScr := setupTestEnv()
	defer cleanupMockScr()
	server := fqlFakeMgmtServer(t, true, true)

	rc.Datacenter.Spec.Config = json.RawMessage(fqlEnabledConfig)
	attachPods(t, rc, server)

	r := rc.CheckFullQueryLogging()
	require.Equal(t, result.Continue(), r)
	server.assertCallCount(t, "/api/v0/metadata/versions/features", 1)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging", 1)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging?enabled=true", 0)
	assertFQLEnabledAnnotation(t, rc, rc.dcPods[0], true)
}

func TestCheckFullQueryLoggingDisabledWithoutAnnotation(t *testing.T) {
	rc, cleanupMockScr := setupTestEnv()
	defer cleanupMockScr()
	server := fqlFakeMgmtServer(t, true, false)
	attachPods(t, rc, server)

	r := rc.CheckFullQueryLogging()
	require.Equal(t, result.Continue(), r)
	server.assertCallCount(t, "/api/v0/metadata/versions/features", 0)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging", 0)
	assertFQLEnabledAnnotation(t, rc, rc.dcPods[0], false)
}

func TestCheckFullQueryLoggingEnabledWithAnnotation(t *testing.T) {
	rc, cleanupMockScr := setupTestEnv()
	defer cleanupMockScr()
	server := fqlFakeMgmtServer(t, true, true)

	rc.Datacenter.Spec.Config = json.RawMessage(fqlEnabledConfig)
	setFQLEnabledAnnotation(rc.dcPods[0])
	attachPods(t, rc, server)

	r := rc.CheckFullQueryLogging()
	require.Equal(t, result.Continue(), r)
	server.assertCallCount(t, "/api/v0/metadata/versions/features", 0)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging", 0)
}

func TestCheckFullQueryNotSupported(t *testing.T) {
	rc, cleanupMockScr := setupTestEnv()
	defer cleanupMockScr()
	server := fqlFakeMgmtServer(t, false, false)

	setFQLEnabledAnnotation(rc.dcPods[0])
	attachPods(t, rc, server)

	r := rc.CheckFullQueryLogging()
	require.Equal(t, result.Continue(), r)
	server.assertCallCount(t, "/api/v0/metadata/versions/features", 1)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging", 0)
	assertFQLEnabledAnnotation(t, rc, rc.dcPods[0], false)
}

func TestCheckFullQueryLoggingChangeToEnabled(t *testing.T) {
	rc, cleanupMockScr := setupTestEnv()
	defer cleanupMockScr()
	server := fqlFakeMgmtServer(t, true, false)

	rc.Datacenter.Spec.Config = json.RawMessage(fqlEnabledConfig)
	attachPods(t, rc, server)

	r := rc.CheckFullQueryLogging()
	require.Equal(t, result.Continue(), r)
	server.assertCallCount(t, "/api/v0/metadata/versions/features", 1)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging", 2)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging?enabled=true", 1)
	assertFQLEnabledAnnotation(t, rc, rc.dcPods[0], true)

	r = rc.CheckFullQueryLogging()
	require.Equal(t, result.Continue(), r)
	server.assertCallCount(t, "/api/v0/metadata/versions/features", 1)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging", 2)
}

func TestCheckFullQueryLoggingChangeToDisabled(t *testing.T) {
	rc, cleanupMockScr := setupTestEnv()
	defer cleanupMockScr()
	server := fqlFakeMgmtServer(t, true, true)

	setFQLEnabledAnnotation(rc.dcPods[0])
	attachPods(t, rc, server)

	r := rc.CheckFullQueryLogging()
	require.Equal(t, result.Continue(), r)
	server.assertCallCount(t, "/api/v0/metadata/versions/features", 1)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging", 2)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging?enabled=false", 1)
	assertFQLEnabledAnnotation(t, rc, rc.dcPods[0], false)

	r = rc.CheckFullQueryLogging()
	require.Equal(t, result.Continue(), r)
	server.assertCallCount(t, "/api/v0/metadata/versions/features", 1)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging", 2)
}

func TestCheckFullQueryNotSupportedTriedToUse(t *testing.T) {
	rc, cleanupMockScr := setupTestEnv()
	defer cleanupMockScr()
	server := fqlFakeMgmtServer(t, false, false)

	rc.Datacenter.Spec.Config = json.RawMessage(fqlEnabledConfig)
	attachPods(t, rc, server)

	// The webhook rejects this configuration, but this test bypasses the validation
	r := rc.CheckFullQueryLogging()
	_, err := r.Output()
	require.Error(t, err)
	server.assertCallCount(t, "/api/v0/metadata/versions/features", 1)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging", 0)
	assertFQLEnabledAnnotation(t, rc, rc.dcPods[0], false)
}

func TestCheckFullQueryLoggingOnlyDatacenterPods(t *testing.T) {
	rc, cleanupMockScr := setupTestEnv()
	defer cleanupMockScr()
	server := fqlFakeMgmtServer(t, true, false)

	otherLabels := rc.Datacenter.GetClusterLabels()
	otherLabels[api.DatacenterLabel] = "other-dc"
	otherPod := fqlTestPod("other-dc-sts-0", rc.Datacenter.Namespace, otherLabels)
	rc.clusterPods = append(rc.clusterPods, otherPod)

	notStarted := fqlTestPod("test-sts-1", rc.Datacenter.Namespace, rc.Datacenter.GetDatacenterLabels())
	notStarted.Labels[api.CassNodeState] = stateReadyToStart
	rc.clusterPods = append(rc.clusterPods, notStarted)
	rc.dcPods = append(rc.dcPods, notStarted)

	notReady := fqlTestPod("test-sts-2", rc.Datacenter.Namespace, rc.Datacenter.GetDatacenterLabels())
	notReady.Status.ContainerStatuses[0].Ready = false
	rc.clusterPods = append(rc.clusterPods, notReady)
	rc.dcPods = append(rc.dcPods, notReady)

	rc.Datacenter.Spec.Config = json.RawMessage(fqlEnabledConfig)
	attachPods(t, rc, server)

	r := rc.CheckFullQueryLogging()
	require.Equal(t, result.Continue(), r)
	server.assertCallCount(t, "/api/v0/metadata/versions/features", 1)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging?enabled=true", 1)
	assertFQLEnabledAnnotation(t, rc, rc.dcPods[0], true)
	assertFQLEnabledAnnotation(t, rc, otherPod, false)
	assertFQLEnabledAnnotation(t, rc, notStarted, false)
	assertFQLEnabledAnnotation(t, rc, notReady, false)
}

func TestNotSupportedVersion(t *testing.T) {
	rc := ReconciliationContext{
		Datacenter: &api.CassandraDatacenter{
			Spec: api.CassandraDatacenterSpec{},
		},
	}

	rc.Datacenter.Spec.ServerVersion = "3.11.11"

	// Don't mock any calls, if they're called (and they shouldn't be), then this
	// test will fail
	r := rc.CheckFullQueryLogging()
	if r != result.Continue() {
		t.Fatalf("expected result of result.Continue() but got %s", r)
	}
}
