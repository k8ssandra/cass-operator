package reconciliation

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/internal/result"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func setupPodList(rc *ReconciliationContext) {
	podIP := "192.168.101.11"

	pods := []corev1.Pod{{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sts-0",
			Namespace: rc.Datacenter.Namespace,
			Labels:    rc.Datacenter.GetClusterLabels(),
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "cassandra",
			}},
		},
		Status: corev1.PodStatus{
			PodIP: podIP,
		},
	}}

	rc.clusterPods = []*corev1.Pod{
		&pods[0],
	}
	rc.dcPods = []*corev1.Pod{
		&pods[0],
	}

	rc.Client = fake.NewClientBuilder().
		WithScheme(setupScheme()).
		WithStatusSubresource(rc.Datacenter).
		WithRuntimeObjects(rc.Datacenter, &pods[0]).
		WithIndex(&corev1.Pod{}, podPVCClaimNameField, podPVCClaimNames).
		Build()
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
	server.attachToPod(t, rc.dcPods[0])
	server.attachToPod(t, rc.clusterPods[0])
	rc.Client = fake.NewClientBuilder().
		WithScheme(setupScheme()).
		WithStatusSubresource(rc.Datacenter, rc.dcPods[0]).
		WithRuntimeObjects(rc.Datacenter, rc.dcPods[0]).
		WithIndex(&corev1.Pod{}, podPVCClaimNameField, podPVCClaimNames).
		Build()
}

func TestCheckFullQueryLoggingNoChangeEnabled(t *testing.T) {
	rc, cleanupMockScr := setupTestEnv()
	defer cleanupMockScr()
	server := fqlFakeMgmtServer(t, true, true)
	for _, pod := range rc.dcPods {
		server.attachToPod(t, pod)
	}
	rc.NodeMgmtClient = server.client(rc.ReqLogger)

	// Enable FQL config in the Datacenter
	rc.Datacenter.Spec.Config = json.RawMessage(fqlEnabledConfig)
	attachPods(t, rc, server)
	rc.NodeMgmtClient = server.client(rc.ReqLogger)

	r := rc.CheckFullQueryLogging()
	if r != result.Continue() {
		t.Fatalf("expected result of result.Continue() but got %s", r)
	}
	server.assertCallCount(t, "/api/v0/metadata/versions/features", 1)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging", 1)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging?enabled=true", 0)
}

func TestCheckFullQueryLoggingNoChangeDisabled(t *testing.T) {
	rc, cleanupMockScr := setupTestEnv()
	defer cleanupMockScr()
	server := fqlFakeMgmtServer(t, true, false)

	// Don't enable FQL config in the Datacenter
	// rc.Datacenter.Spec.Config = json.RawMessage(fqlDisabledConfig)
	attachPods(t, rc, server)
	rc.NodeMgmtClient = server.client(rc.ReqLogger)

	r := rc.CheckFullQueryLogging()
	if r != result.Continue() {
		t.Fatalf("expected result of result.Continue() but got %s", r)
	}
	server.assertCallCount(t, "/api/v0/metadata/versions/features", 1)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging", 1)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging?enabled=false", 0)
}

func TestCheckFullQueryNotSupported(t *testing.T) {
	rc, cleanupMockScr := setupTestEnv()
	defer cleanupMockScr()
	server := fqlFakeMgmtServer(t, false, false)
	attachPods(t, rc, server)
	rc.NodeMgmtClient = server.client(rc.ReqLogger)

	r := rc.CheckFullQueryLogging()
	if r != result.Continue() {
		t.Fatalf("expected result of result.Continue() but got %s", r)
	}
	server.assertCallCount(t, "/api/v0/metadata/versions/features", 1)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging", 0)
}

func TestCheckFullQueryLoggingChangeToEnabled(t *testing.T) {
	rc, cleanupMockScr := setupTestEnv()
	defer cleanupMockScr()
	server := fqlFakeMgmtServer(t, true, false)

	// Enable FQL config in the Datacenter
	rc.Datacenter.Spec.Config = json.RawMessage(fqlEnabledConfig)
	attachPods(t, rc, server)
	rc.NodeMgmtClient = server.client(rc.ReqLogger)

	r := rc.CheckFullQueryLogging()
	if r != result.Continue() {
		t.Fatalf("expected result of result.Continue() but got %s", r)
	}
	server.assertCallCount(t, "/api/v0/metadata/versions/features", 1)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging", 2)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging?enabled=true", 1)
}

func TestCheckFullQueryLoggingChangeToDisabled(t *testing.T) {
	rc, cleanupMockScr := setupTestEnv()
	defer cleanupMockScr()
	server := fqlFakeMgmtServer(t, true, true)

	// Keep FQL config disabled in the Datacenter
	attachPods(t, rc, server)
	rc.NodeMgmtClient = server.client(rc.ReqLogger)

	r := rc.CheckFullQueryLogging()
	if r != result.Continue() {
		t.Fatalf("expected result of result.Continue() but got %s", r)
	}
	server.assertCallCount(t, "/api/v0/metadata/versions/features", 1)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging", 2)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging?enabled=false", 1)
}

func TestCheckFullQueryNotSupportedTriedToUse(t *testing.T) {
	rc, cleanupMockScr := setupTestEnv()
	defer cleanupMockScr()
	server := fqlFakeMgmtServer(t, false, false)

	// Enable FQL config in the Datacenter
	rc.Datacenter.Spec.Config = json.RawMessage(fqlEnabledConfig)
	attachPods(t, rc, server)
	rc.NodeMgmtClient = server.client(rc.ReqLogger)

	// The error is thrown in handler, but this test bypasses the validation - that's why we take Continue
	// as correct result.
	r := rc.CheckFullQueryLogging()
	_, err := r.Output()
	require.Error(t, err)
	server.assertCallCount(t, "/api/v0/metadata/versions/features", 1)
	server.assertCallCount(t, "/api/v0/ops/node/fullquerylogging", 0)
}

func TestNotSupportedVersion(t *testing.T) {
	rc := ReconciliationContext{
		Datacenter: &v1beta1.CassandraDatacenter{
			Spec: v1beta1.CassandraDatacenterSpec{},
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
