// Copyright DataStax, Inc.
// Please see the included license file for details.

package httphelper

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/k8ssandra/cass-operator/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"io"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
)

func Test_BuildPodHostFromPod(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-foo",
			Namespace: "somenamespace",
			Labels: map[string]string{
				api.DatacenterLabel: "dc-bar",
				api.ClusterLabel:    "the-foobar-cluster",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "1.2.3.4",
		},
	}

	result, err := BuildPodHostFromPod(pod)
	assert.NoError(t, err)

	expected := "1.2.3.4"

	assert.Equal(t, expected, result)
}

func Test_parseMetadataEndpointsResponseBody(t *testing.T) {
	endpoints, err := parseMetadataEndpointsResponseBody([]byte(`{
		"entity": [
		  {
			"DC": "dtcntr",
			"ENDPOINT_IP": "10.233.90.45",
			"HOST_ID": "95c157dc-2811-446a-a541-9faaab2e6930",
			"INTERNAL_IP": "10.233.90.45",
			"IS_ALIVE": "true",
			"LOAD": "72008.0",
			"NET_VERSION": "11",
			"RACK": "r0",
			"RELEASE_VERSION": "3.11.6",
			"RPC_ADDRESS": "10.233.90.45",
			"RPC_READY": "true",
			"SCHEMA": "e84b6a60-24cf-30ca-9b58-452d92911703",
			"STATUS": "NORMAL,2756844028858338669",
			"TOKENS": "\u0000\u0000\u0000\b&BG\t±B\rm\u0000\u0000\u0000\u0000"
		  },
		  {
			"DC": "dtcntr",
			"ENDPOINT_IP": "10.233.92.102",
			"HOST_ID": "828e6980-9cac-48f2-a2c9-0650edc4d114",
			"INTERNAL_IP": "10.233.92.102",
			"IS_ALIVE": "true",
			"LOAD": "71880.0",
			"NET_VERSION": "11",
			"RACK": "r0",
			"RELEASE_VERSION": "3.11.6",
			"RPC_ADDRESS": "10.233.92.102",
			"RPC_READY": "true",
			"SCHEMA": "e84b6a60-24cf-30ca-9b58-452d92911703",
			"STATUS": "NORMAL,-1589726493696519215",
			"TOKENS": "\u0000\u0000\u0000\béð(-=1\u0013Ñ\u0000\u0000\u0000\u0000"
		  }
		],
		"variant": {
		  "language": null,
		  "mediaType": {
			"type": "application",
			"subtype": "json",
			"parameters": {},
			"wildcardType": false,
			"wildcardSubtype": false
		  },
		  "encoding": null,
		  "languageString": null
		},
		"annotations": [],
		"mediaType": {
		  "type": "application",
		  "subtype": "json",
		  "parameters": {},
		  "wildcardType": false,
		  "wildcardSubtype": false
		},
		"language": null,
		"encoding": null
	  }`))

	assert.Nil(t, err)
	assert.Equal(t, 2, len(endpoints.Entity))
	assert.Equal(t, "10.233.90.45", endpoints.Entity[0].RpcAddress)
	assert.Equal(t, "95c157dc-2811-446a-a541-9faaab2e6930", endpoints.Entity[0].HostID)
}

func Test_parseListKeyspacesEndpointsResponseBody(t *testing.T) {
	keyspaces, err := parseListKeyspacesEndpointsResponseBody([]byte(`["keyspace1", "keyspace2"]`))

	assert.Nil(t, err)
	assert.Equal(t, 2, len(keyspaces))
	assert.Equal(t, "keyspace1", keyspaces[0])
	assert.Equal(t, "keyspace2", keyspaces[1])
}

func Test_featureSet(t *testing.T) {
	assert := assert.New(t)

	exampleData := `{
		"cassandra_version": "4.0.0",
		"features": [
			"async_sstable_tasks",
			"this_feature_is_not_real"
		]
		}`

	featureSet := &FeatureSet{}
	if err := json.Unmarshal([]byte(exampleData), featureSet); err != nil {
		assert.FailNow("failed to unmarshal featureSet")
	}

	assert.True(featureSet.Supports(AsyncSSTableTasks))

	exampleDataEmpty := `{
		"cassandra_version": "3.11.11",
		"features": [
		]
		}`

	featureSet = &FeatureSet{}
	if err := json.Unmarshal([]byte(exampleDataEmpty), featureSet); err != nil {
		assert.FailNow("failed to unmarshal featureSet")
	}

	assert.False(featureSet.Supports(AsyncSSTableTasks))
}

func TestNodeMgmtClient_GetKeyspaceReplication(t *testing.T) {
	successBody := map[string]string{"class": "org.apache.cassandra.locator.NetworkTopologyStrategy", "dc1": "3", "dc2": "1"}
	tests := []struct {
		name         string
		pod          *corev1.Pod
		keyspaceName string
		httpClient   *mocks.HttpClient
		expected     map[string]string
		err          error
	}{
		{
			"success",
			goodPod,
			"ks1",
			newMockHttpClient(newHttpResponse(successBody, http.StatusOK), nil),
			successBody,
			nil,
		},
		{
			"keyspace name empty",
			goodPod,
			"",
			nil,
			nil,
			errors.New("keyspace name cannot be empty"),
		},
		{
			"pod has no IP",
			badPod,
			"ks1",
			nil,
			nil,
			errors.New("pod pod1 has no IP"),
		},
		{
			"request failure",
			goodPod,
			"ks1",
			newMockHttpClient(nil, errors.New("connection reset by peer")),
			nil,
			errors.New("connection reset by peer"),
		},
		{
			"keyspace not found",
			goodPod,
			"ks1",
			newMockHttpClient(newHttpResponse("Keyspace 'ks1' does not exist", http.StatusNotFound), nil),
			nil,
			&RequestError{
				StatusCode: http.StatusNotFound,
				Err:        errors.New("incorrect status code of 404 when calling endpoint"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgmtClient := newMockMgmtClient(tt.httpClient)
			actual, err := mgmtClient.GetKeyspaceReplication(tt.pod, tt.keyspaceName)
			assert.Equal(t, tt.expected, actual)
			assert.Equal(t, tt.err, err)
		})
	}
}

func TestNodeMgmtClient_ListTables(t *testing.T) {
	tests := []struct {
		name         string
		pod          *corev1.Pod
		keyspaceName string
		httpClient   *mocks.HttpClient
		expected     []string
		err          error
	}{
		{
			"success",
			goodPod,
			"ks1",
			newMockHttpClient(newHttpResponse([]string{"table1", "table2"}, http.StatusOK), nil),
			[]string{"table1", "table2"},
			nil,
		},
		{
			"keyspace name empty",
			goodPod,
			"",
			nil,
			nil,
			errors.New("keyspace name cannot be empty"),
		},
		{
			"pod has no IP",
			badPod,
			"ks1",
			nil,
			nil,
			errors.New("pod pod1 has no IP"),
		},
		{
			"request failure",
			goodPod,
			"ks1",
			newMockHttpClient(nil, errors.New("connection reset by peer")),
			nil,
			errors.New("connection reset by peer"),
		},
		{
			"keyspace not found",
			goodPod,
			"ks1",
			newMockHttpClient(newHttpResponse([]string{}, http.StatusOK), nil),
			[]string{},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgmtClient := newMockMgmtClient(tt.httpClient)
			actual, err := mgmtClient.ListTables(tt.pod, tt.keyspaceName)
			assert.Equal(t, tt.expected, actual)
			assert.Equal(t, tt.err, err)
		})
	}
}

func TestNodeMgmtClient_CreateTable(t *testing.T) {
	goodTable := NewTableDefinition(
		"ks1",
		"table1",
		NewPartitionKeyColumn("pk1", "int", 0),
		NewPartitionKeyColumn("pk2", "int", 1),
		NewClusteringColumn("cc1", "int", 0, ClusteringOrderAsc),
		NewClusteringColumn("cc2", "int", 1, ClusteringOrderDesc),
		NewRegularColumn("c", "list<text>"),
		NewStaticColumn("s", "tuple<int,boolean,inet>"),
	)
	tests := []struct {
		name       string
		pod        *corev1.Pod
		table      *TableDefinition
		httpClient *mocks.HttpClient
		err        error
	}{
		{
			"success",
			goodPod,
			goodTable,
			newMockHttpClient(newHttpResponse("OK", http.StatusOK), nil),
			nil,
		},
		{
			"nil table definition",
			goodPod,
			nil,
			nil,
			errors.New("table definition cannot be nil"),
		},
		{
			"keyspace name empty",
			goodPod,
			NewTableDefinition(
				"",
				"table1",
			),
			nil,
			errors.New("keyspace name cannot be empty"),
		},
		{
			"table name empty",
			goodPod,
			NewTableDefinition(
				"ks1",
				"",
			),
			nil,
			errors.New("table name cannot be empty"),
		},
		{
			"columns empty",
			goodPod,
			NewTableDefinition(
				"ks1",
				"table1",
			),
			nil,
			errors.New("columns cannot be empty"),
		},
		{
			"pod has no IP",
			badPod,
			goodTable,
			nil,
			errors.New("pod pod1 has no IP"),
		},
		{
			"request failure",
			goodPod,
			goodTable,
			newMockHttpClient(nil, errors.New("connection reset by peer")),
			errors.New("connection reset by peer"),
		},
		{
			"invalid column", // validated server-side
			goodPod,
			NewTableDefinition(
				"ks1",
				"table1",
				NewPartitionKeyColumn("", "int", 0),
			),
			newMockHttpClient(newHttpResponse("Table creation failed: 'columns[0].name' must not be empty", http.StatusBadRequest), nil),
			&RequestError{
				StatusCode: http.StatusBadRequest,
				Err:        errors.New("incorrect status code of 400 when calling endpoint"),
			},
		},
		{
			"keyspace not found",
			goodPod,
			goodTable,
			newMockHttpClient(newHttpResponse("keyspace does not exist", http.StatusInternalServerError), nil),
			&RequestError{
				StatusCode: http.StatusInternalServerError,
				Err:        errors.New("incorrect status code of 500 when calling endpoint"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgmtClient := newMockMgmtClient(tt.httpClient)
			err := mgmtClient.CreateTable(tt.pod, tt.table)
			assert.Equal(t, tt.err, err)
		})
	}
}

func newMockMgmtClient(httpClient *mocks.HttpClient) *NodeMgmtClient {
	return &NodeMgmtClient{
		Client:   httpClient,
		Log:      log.NullLogger{},
		Protocol: "http",
	}
}

func newMockHttpClient(response *http.Response, err error) *mocks.HttpClient {
	httpClient := new(mocks.HttpClient)
	httpClient.On("Do", mock.Anything).Return(response, err)
	return httpClient
}

func newHttpResponse(responseBody interface{}, status int) *http.Response {
	marshalled, _ := json.Marshal(responseBody)
	body := io.NopCloser(bytes.NewReader(marshalled))
	bodyLength := int64(len(marshalled))
	return &http.Response{
		StatusCode:    status,
		Body:          body,
		ContentLength: bodyLength,
	}
}

var goodPod = &corev1.Pod{
	ObjectMeta: metav1.ObjectMeta{
		Name: "pod1",
	},
	Status: corev1.PodStatus{
		PodIP: "1.2.3.4",
	},
}

var badPod = &corev1.Pod{
	ObjectMeta: metav1.ObjectMeta{
		Name: "pod1",
	},
}
