package httphelper

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
)

var featuresReply = `{
	"cassandra_version": "4.0.1",
	"features": [
		"async_sstable_tasks",
		"rebuild"
	]
	}`

var jobDetailsCompleted = `{ "id": "%s", "status": "COMPLETED", "type": "Cleanup" }`

func CreateJobDetailsFakeServer() (*httptest.Server, error) {
	// TODO Modify cass-operator to allow different ports
	// The client in cass-operator has hardcoded port of 8080, so we need to run our mgtt-api listener in that port
	mgttapiListener, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		return nil, err
	}

	managementMockServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
		} else {
			w.WriteHeader(200)
		}
		if r.Method == http.MethodGet && r.RequestURI == "/api/v0/metadata/versions/features" {
			w.Write([]byte(featuresReply))
		} else if r.Method == http.MethodGet && r.URL.Path == "/api/v0/ops/executor/job" {
			jobId := query.Get("job_id")
			w.Write([]byte(fmt.Sprintf(jobDetailsCompleted, jobId)))
		}
	}))
	managementMockServer.Listener.Close()
	managementMockServer.Listener = mgttapiListener

	return managementMockServer, nil
}
