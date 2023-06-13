package reconciliation

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/pkg/httphelper"
	"github.com/k8ssandra/cass-operator/pkg/internal/result"
	"github.com/k8ssandra/cass-operator/pkg/mocks"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

var httpResponseFullQueryEnabled string = `{"entity": true}`
var httpResponseFullQueryDisabled string = `{"entity": false}`

func setupPodList(t *testing.T, rc *ReconciliationContext) {
	podIP := "192.168.101.11"

	mockClient := mocks.NewClient(t)

	pods := []corev1.Pod{{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sts-0",
		},
		Status: corev1.PodStatus{
			PodIP: podIP,
		},
	}}

	rc.dcPods = []*corev1.Pod{
		&pods[0],
	}

	k8sMockClientList(mockClient, nil).
		Run(func(args mock.Arguments) {
			arg := args.Get(1).(*corev1.PodList)
			arg.Items = pods
		})

	rc.Client = mockClient
}

func mockFeaturesEnabled(mockHttpClient *mocks.HttpClient) {
	resFeatureSet := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(fullQueryIsSupported)),
	}

	mockHttpClient.On("Do",
		mock.MatchedBy(
			func(req *http.Request) bool {
				return req.URL.Path == "/api/v0/metadata/versions/features"
			})).
		Return(resFeatureSet, nil).
		Once()
}

func mockFeaturesNotAvailable(mockHttpClient *mocks.HttpClient) {
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
}

func mockFullQueryLoggingRequestToTrue(mockHttpClient *mocks.HttpClient) {
	resFullQueryStatus := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(httpResponseFullQueryEnabled)),
	}
	mockHttpClient.On("Do",
		mock.MatchedBy(
			func(req *http.Request) bool {
				return req.URL.Path == "/api/v0/ops/node/fullquerylogging"
			})).
		Return(resFullQueryStatus, nil).
		Once()
}

func mockFullQueryLoggingRequestToFalse(mockHttpClient *mocks.HttpClient) {
	resFullQueryStatus := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(httpResponseFullQueryDisabled)),
	}
	mockHttpClient.On("Do",
		mock.MatchedBy(
			func(req *http.Request) bool {
				return req.URL.Path == "/api/v0/ops/node/fullquerylogging"
			})).
		Return(resFullQueryStatus, nil).
		Once()
}

func setupTestEnv(t *testing.T) (*ReconciliationContext, func()) {
	rc, _, cleanupMockScr := setupTest()
	setupPodList(t, rc)
	rc.Datacenter.Spec.ServerType = "cassandra"
	rc.Datacenter.Spec.ServerVersion = "4.0.1"
	return rc, cleanupMockScr
}

func TestCheckFullQueryLoggingNoChangeEnabled(t *testing.T) {
	rc, cleanupMockScr := setupTestEnv(t)
	defer cleanupMockScr()

	mockHttpClient := mocks.NewHttpClient(t)

	// Mock features request to support FQL
	mockFeaturesEnabled(mockHttpClient)

	// Mock fullQueryLogging to return true
	mockFullQueryLoggingRequestToTrue(mockHttpClient)

	// Enable FQL config in the Datacenter
	rc.Datacenter.Spec.Config = json.RawMessage(fqlEnabledConfig)

	rc.NodeMgmtClient = httphelper.NodeMgmtClient{
		Client:   mockHttpClient,
		Log:      rc.ReqLogger,
		Protocol: "http",
	}

	r := rc.CheckFullQueryLogging()
	if r != result.Continue() {
		t.Fatalf("expected result of result.Continue() but got %s", r)
	}
}

func TestCheckFullQueryLoggingNoChangeDisabled(t *testing.T) {
	rc, cleanupMockScr := setupTestEnv(t)
	defer cleanupMockScr()

	mockHttpClient := mocks.NewHttpClient(t)

	// Mock features request to support FQL
	mockFeaturesEnabled(mockHttpClient)

	// Mock fullQueryLogging to return true
	mockFullQueryLoggingRequestToFalse(mockHttpClient)

	// Don't enable FQL config in the Datacenter
	// rc.Datacenter.Spec.Config = json.RawMessage(fqlDisabledConfig)

	rc.NodeMgmtClient = httphelper.NodeMgmtClient{
		Client:   mockHttpClient,
		Log:      rc.ReqLogger,
		Protocol: "http",
	}

	r := rc.CheckFullQueryLogging()
	if r != result.Continue() {
		t.Fatalf("expected result of result.Continue() but got %s", r)
	}
}

func TestCheckFullQueryNotSupported(t *testing.T) {
	rc, cleanupMockScr := setupTestEnv(t)
	defer cleanupMockScr()

	mockHttpClient := mocks.NewHttpClient(t)

	// Mock features request to not support FQL
	mockFeaturesNotAvailable(mockHttpClient)

	rc.NodeMgmtClient = httphelper.NodeMgmtClient{
		Client:   mockHttpClient,
		Log:      rc.ReqLogger,
		Protocol: "http",
	}

	r := rc.CheckFullQueryLogging()
	if r != result.Continue() {
		t.Fatalf("expected result of result.Continue() but got %s", r)
	}
}

func TestCheckFullQueryLoggingChangeToEnabled(t *testing.T) {
	rc, cleanupMockScr := setupTestEnv(t)
	defer cleanupMockScr()

	mockHttpClient := mocks.NewHttpClient(t)

	// Mock features request to support FQL
	mockFeaturesEnabled(mockHttpClient)

	// Mock fullQueryLogging to return false
	mockFullQueryLoggingRequestToFalse(mockHttpClient)

	// Mock fullQueryLogging change request
	mockFullQueryLoggingRequestToTrue(mockHttpClient)

	// Enable FQL config in the Datacenter
	rc.Datacenter.Spec.Config = json.RawMessage(fqlEnabledConfig)

	rc.NodeMgmtClient = httphelper.NodeMgmtClient{
		Client:   mockHttpClient,
		Log:      rc.ReqLogger,
		Protocol: "http",
	}

	r := rc.CheckFullQueryLogging()
	if r != result.Continue() {
		t.Fatalf("expected result of result.Continue() but got %s", r)
	}
}

func TestCheckFullQueryLoggingChangeToDisabled(t *testing.T) {
	rc, cleanupMockScr := setupTestEnv(t)
	defer cleanupMockScr()

	mockHttpClient := mocks.NewHttpClient(t)

	// Mock features request to support FQL
	mockFeaturesEnabled(mockHttpClient)

	// Mock fullQueryLogging to return true
	mockFullQueryLoggingRequestToTrue(mockHttpClient)

	// Mock fullQueryLogging change request to false
	mockFullQueryLoggingRequestToFalse(mockHttpClient)

	// Keep FQL config disabled in the Datacenter

	rc.NodeMgmtClient = httphelper.NodeMgmtClient{
		Client:   mockHttpClient,
		Log:      rc.ReqLogger,
		Protocol: "http",
	}

	r := rc.CheckFullQueryLogging()
	if r != result.Continue() {
		t.Fatalf("expected result of result.Continue() but got %s", r)
	}
}

func TestCheckFullQueryNotSupportedTriedToUse(t *testing.T) {
	rc, cleanupMockScr := setupTestEnv(t)
	defer cleanupMockScr()

	mockHttpClient := mocks.NewHttpClient(t)

	// Mock features request to not support FQL
	mockFeaturesNotAvailable(mockHttpClient)

	// Enable FQL config in the Datacenter
	rc.Datacenter.Spec.Config = json.RawMessage(fqlEnabledConfig)

	rc.NodeMgmtClient = httphelper.NodeMgmtClient{
		Client:   mockHttpClient,
		Log:      rc.ReqLogger,
		Protocol: "http",
	}

	// The error is thrown in handler, but this test bypasses the validation - that's why we take Continue
	// as correct result.
	r := rc.CheckFullQueryLogging()
	_, err := r.Output()
	if err == nil {
		t.Fatalf("expected result of result.Error() but got %s", r)
	}
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
