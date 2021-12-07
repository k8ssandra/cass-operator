package httphelper

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
)

var featuresReply = `{
	"cassandra_version": "4.0.1",
	"features": [
		"async_sstable_tasks",
		"rebuild"
	]
	}`

var jobDetailsCompleted = `{"submit_time":"1638545895255","end_time":"1638545895255","id":"%s","type":"Cleanup","status":"COMPLETED"}`

var noJobDetails = `{}`

func mgmtApiListener() (net.Listener, error) {
	mgmtApiListener, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		return nil, err
	}

	return mgmtApiListener, nil
}

type CallDetails struct {
	URLCounts map[string]int
}

func NewCallDetails() *CallDetails {
	return &CallDetails{
		URLCounts: make(map[string]int),
	}
}

func (c *CallDetails) incr(url string) {
	if currentCount, found := c.URLCounts[url]; !found {
		c.URLCounts[url] = 1
	} else {
		c.URLCounts[url] = currentCount + 1
	}
}

func FakeExecutorServerWithDetails(callDetails *CallDetails) (*httptest.Server, error) {
	// TODO Modify cass-operator to allow different ports
	// The client in cass-operator has hardcoded port of 8080, so we need to run our mgtt-api listener in that port
	mgmtApiListener, err := mgmtApiListener()
	if err != nil {
		return nil, err
	}

	jobId := 0

	managementMockServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
		}

		if callDetails != nil {
			callDetails.incr(r.URL.Path)
		}

		if r.Method == http.MethodGet && r.RequestURI == "/api/v0/metadata/versions/features" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(featuresReply))
		} else if r.Method == http.MethodGet && r.URL.Path == "/api/v0/ops/executor/job" {
			w.WriteHeader(http.StatusOK)
			jobId := query.Get("job_id")
			w.Write([]byte(fmt.Sprintf(jobDetailsCompleted, jobId)))
		} else if r.Method == http.MethodPost && (r.URL.Path == "/api/v1/ops/keyspace/cleanup" || r.URL.Path == "/api/v1/ops/node/rebuild") {
			w.WriteHeader(http.StatusOK)
			// Write jobId
			jobId++
			w.Write([]byte(strconv.Itoa(jobId)))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}

	}))
	managementMockServer.Listener.Close()
	managementMockServer.Listener = mgmtApiListener

	return managementMockServer, nil
}

func FakeServerWithoutFeaturesEndpoint(callDetails *CallDetails) (*httptest.Server, error) {
	mgmtApiListener, err := mgmtApiListener()
	if err != nil {
		return nil, err
	}

	managementMockServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if callDetails != nil {
			callDetails.incr(r.URL.Path)
		}

		if r.Method == http.MethodPost && r.URL.Path == "/api/v0/ops/keyspace/cleanup" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	managementMockServer.Listener.Close()
	managementMockServer.Listener = mgmtApiListener

	return managementMockServer, nil
}
