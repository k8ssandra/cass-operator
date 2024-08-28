// Copyright DataStax, Inc.
// Please see the included license file for details.

package httphelper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"

	cassdcapi "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type NodeMgmtClient struct {
	Client   HttpClient
	Log      logr.Logger
	Protocol string
}

type nodeMgmtRequest struct {
	endpoint string
	host     string
	port     int
	method   string
	timeout  time.Duration
	body     []byte
}

func buildEndpoint(path string, queryParams ...string) string {
	params := url.Values{}
	for i := 0; i < len(queryParams)-1; i = i + 2 {
		params[queryParams[i]] = []string{queryParams[i+1]}
	}

	url := &url.URL{
		Path:     path,
		RawQuery: params.Encode(),
	}
	return url.String()
}

type EndpointState struct {
	Datacenter             string `json:"DC,omitempty"`
	Rack                   string `json:"RACK,omitempty"`
	ReleaseVersion         string `json:"RELEASE_VERSION,omitempty"`
	SchemaVersion          string `json:"SCHEMA,omitempty"`
	SSTableVersion         string `json:"SSTABLE_VERSIONS,omitempty"`
	HostID                 string `json:"HOST_ID,omitempty"`
	IsAlive                string `json:"IS_ALIVE,omitempty"`
	EndpointIP             string `json:"ENDPOINT_IP,omitempty"`
	NativeAddressAndPort   string `json:"NATIVE_ADDRESS_AND_PORT,omitempty"`
	NativeTransportAddress string `json:"NATIVE_TRANSPORT_ADDRESS,omitempty"`
	RpcAddress             string `json:"RPC_ADDRESS,omitempty"`
	Status                 string `json:"STATUS,omitempty"`
	StatusWithPort         string `json:"STATUS_WITH_PORT,omitempty"`
	Load                   string `json:"LOAD,omitempty"`
}

type EndpointStateStatus string

const (
	StatusNormal  EndpointStateStatus = "NORMAL"
	StatusLeaving EndpointStateStatus = "LEAVING"
	StatusLeft    EndpointStateStatus = "LEFT"
	StatusMoving  EndpointStateStatus = "MOVING"
	StatusRemoved EndpointStateStatus = "removed"
)

func (e *EndpointState) HasStatus(status EndpointStateStatus) bool {
	// Verify if this is either in Status or StatusWithPort
	return strings.HasPrefix(e.Status, string(status)) || strings.HasPrefix(e.StatusWithPort, string(status))
}

func (x *EndpointState) GetRpcAddress() string {
	if x.NativeAddressAndPort != "" {
		// Cassandra 4.0+
		ip, _, err := net.SplitHostPort(x.NativeAddressAndPort)
		if err != nil {
			return ""
		}
		return ip
	} else if x.NativeTransportAddress != "" {
		// DSE 6.8+
		return x.NativeTransportAddress
	} else {
		// Cassandra 3.11
		return x.RpcAddress
	}
}

type CassMetadataEndpoints struct {
	Entity []EndpointState `json:"entity"`
}

type NoPodIPError error

func newNoPodIPError(pod *corev1.Pod) NoPodIPError {
	return fmt.Errorf("pod %s has no IP", pod.Name)
}

type JobDetails struct {
	Id         string `json:"id"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	SubmitTime string `json:"submit_time,omitempty"`
	EndTime    string `json:"end_time,omitempty"`
	Error      string `json:"error,omitempty"`
}

type Feature string

type FeatureSet struct {
	CassandraVersion string
	Features         map[string]struct{}
}

// Features are generational (mgmt-api version bound) and can include multiple endpoints under a single Feature name.
const (
	// AsyncSSTableTasks includes "cleanup" and "decommission"
	AsyncSSTableTasks       Feature = "async_sstable_tasks"
	AsyncUpgradeSSTableTask Feature = "async_upgrade_sstable_task"
	AsyncCompactionTask     Feature = "async_compaction_task"
	AsyncScrubTask          Feature = "async_scrub_task"
	FullQuerySupport        Feature = "full_query_logging"
	Rebuild                 Feature = "rebuild"
	Move                    Feature = "async_move_task"
	AsyncGarbageCollect     Feature = "async_gc_task"
	AsyncFlush              Feature = "async_flush_task"
	ReloadInodeTruststore   Feature = "reload_internode_truststore"
)

func (f *FeatureSet) UnmarshalJSON(b []byte) error {
	var input map[string]interface{}
	if err := json.Unmarshal(b, &input); err != nil {
		return err
	}

	f.CassandraVersion = input["cassandra_version"].(string)
	var empty struct{}
	f.Features = make(map[string]struct{})
	if fList, ok := input["features"].([]interface{}); ok {
		for _, feature := range fList {
			f.Features[feature.(string)] = empty
		}
	}
	return nil
}

// Supports returns true if the target pod's management-api supports certain feature
func (f *FeatureSet) Supports(feature Feature) bool {
	_, found := f.Features[string(feature)]
	return found
}

func NewMgmtClient(ctx context.Context, client client.Client, dc *cassdcapi.CassandraDatacenter) (NodeMgmtClient, error) {
	logger := log.FromContext(ctx)

	httpClient, err := BuildManagementApiHttpClient(dc, client, ctx)
	if err != nil {
		logger.Error(err, "error in BuildManagementApiHttpClient")
		return NodeMgmtClient{}, err
	}

	protocol, err := GetManagementApiProtocol(dc)
	if err != nil {
		logger.Error(err, "error in GetManagementApiProtocol")
		return NodeMgmtClient{}, err
	}

	return NodeMgmtClient{
		Client:   httpClient,
		Log:      logger,
		Protocol: protocol,
	}, nil
}

func BuildPodHostFromPod(pod *corev1.Pod) (string, int, error) {
	// This function previously returned the dns hostname which includes the StatefulSet's headless service,
	// which is the datacenter service. There are times though that we want to make a mgmt api call to the pod
	// before the dns hostnames are available. It is therefore more reliable to simply use the PodIP.

	if len(pod.Status.PodIP) == 0 {
		return "", 0, newNoPodIPError(pod)
	}

	mgmtApiPort := 8080

	// Check for port override
	for _, container := range pod.Spec.Containers {
		if container.Name == "cassandra" {
			for _, port := range container.Ports {
				if port.Name == "mgmt-api-http" {
					mgmtApiPort = int(port.ContainerPort)
				}
			}
		}
	}

	return pod.Status.PodIP, mgmtApiPort, nil
}

func GetPodHost(podName, clusterName, dcName, namespace string) string {
	nodeServicePattern := "%s.%s-%s-service.%s"

	return fmt.Sprintf(nodeServicePattern, podName, clusterName, dcName, namespace)
}

func parseMetadataEndpointsResponseBody(body []byte) (*CassMetadataEndpoints, error) {
	endpoints := &CassMetadataEndpoints{}
	if err := json.Unmarshal(body, &endpoints); err != nil {
		return nil, err
	}
	return endpoints, nil
}

func (client *NodeMgmtClient) CallMetadataEndpointsEndpoint(pod *corev1.Pod) (CassMetadataEndpoints, error) {
	client.Log.Info("requesting Cassandra metadata endpoints from Node Management API", "pod", pod.Name)

	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return CassMetadataEndpoints{}, err
	}

	request := nodeMgmtRequest{
		endpoint: "/api/v0/metadata/endpoints",
		host:     podHost,
		port:     podPort,
		method:   http.MethodGet,
		timeout:  60 * time.Second,
	}

	bytes, err := callNodeMgmtEndpoint(client, request, "")
	if err != nil {
		return CassMetadataEndpoints{}, err

	}

	endpoints, err := parseMetadataEndpointsResponseBody(bytes)
	if err != nil {
		return CassMetadataEndpoints{}, err
	} else {
		return *endpoints, nil
	}
}

// CallSchemaVersionsEndpoint returns a map of all schema versions. Map keys are the schema
// UUIDs and the values are lists of nodes' IPs that are at that version. The JSON response
// looks like this:
//
//	{"2207c2a9-f598-3971-986b-2926e09e239d": ["10.244.1.4", "10.244.2.3, 10.244.3.3"]}
//
// A map length of 1 indicates schema agreement.
func (client *NodeMgmtClient) CallSchemaVersionsEndpoint(pod *corev1.Pod) (map[string][]string, error) {
	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return nil, err
	}

	request := nodeMgmtRequest{
		endpoint: "/api/v1/ops/node/schema/versions",
		host:     podHost,
		port:     podPort,
		method:   http.MethodGet,
		timeout:  60 * time.Second,
	}

	bytes, err := callNodeMgmtEndpoint(client, request, "")
	if err != nil {
		return nil, err
	}

	result := make(map[string][]string)
	if err = json.Unmarshal(bytes, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// Create a new superuser with the given username and password
func (client *NodeMgmtClient) CallCreateRoleEndpoint(pod *corev1.Pod, username string, password string, superuser bool) error {
	client.Log.Info(
		"calling Management API create role - POST /api/v0/ops/auth/role",
		"pod", pod.Name,
	)

	postData := url.Values{}
	postData.Set("username", username)
	postData.Set("password", password)
	postData.Set("can_login", "true")
	postData.Set("is_superuser", strconv.FormatBool(superuser))

	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return err
	}

	request := nodeMgmtRequest{
		endpoint: fmt.Sprintf("/api/v0/ops/auth/role?%s", postData.Encode()),
		host:     podHost,
		port:     podPort,
		method:   http.MethodPost,
		timeout:  60 * time.Second,
	}
	if _, err = callNodeMgmtEndpoint(client, request, ""); err != nil {
		// The error could include a password, strip it
		strippedErrMsg := strings.ReplaceAll(err.Error(), password, "******")
		return errors.New(strippedErrMsg)
	}
	return nil
}

func (client *NodeMgmtClient) CallProbeClusterEndpoint(pod *corev1.Pod, consistencyLevel string, rfPerDc int) error {
	client.Log.Info(
		"calling Management API cluster health - GET /api/v0/probes/cluster",
		"pod", pod.Name,
	)

	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return err
	}

	request := nodeMgmtRequest{
		endpoint: fmt.Sprintf("/api/v0/probes/cluster?consistency_level=%s&rf_per_dc=%d", consistencyLevel, rfPerDc),
		host:     podHost,
		port:     podPort,
		method:   http.MethodGet,
		timeout:  60 * time.Second,
	}

	_, err = callNodeMgmtEndpoint(client, request, "")
	return err
}

func (client *NodeMgmtClient) CallDrainEndpoint(pod *corev1.Pod) error {
	client.Log.Info(
		"calling Management API drain node - POST /api/v0/ops/node/drain",
		"pod", pod.Name,
	)

	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return err
	}

	request := nodeMgmtRequest{
		endpoint: "/api/v0/ops/node/drain",
		host:     podHost,
		port:     podPort,
		method:   http.MethodPost,
		timeout:  time.Minute * 2,
	}

	_, err = callNodeMgmtEndpoint(client, request, "")
	return err
}

// CallKeyspaceCleanupEndpoint is deprecated. Use it only when accessing old management-api versions. Otherwise, use CallKeyspaceCleanup
func (client *NodeMgmtClient) CallKeyspaceCleanupEndpoint(pod *corev1.Pod, jobs int, keyspaceName string, tables []string) error {
	client.Log.Info(
		"calling Management API keyspace cleanup - POST /api/v0/ops/keyspace/cleanup",
		"pod", pod.Name,
	)

	req, err := createKeySpaceRequest(pod, jobs, keyspaceName, tables, "/api/v0/ops/keyspace/cleanup")
	if err != nil {
		return err
	}

	_, err = callNodeMgmtEndpoint(client, *req, "application/json")
	return err
}

func createKeySpaceRequest(pod *corev1.Pod, jobs int, keyspaceName string, tables []string, endpoint string) (*nodeMgmtRequest, error) {
	postData := make(map[string]interface{})
	if jobs > -1 {
		postData["jobs"] = strconv.Itoa(jobs)
	}

	if keyspaceName != "" {
		postData["keyspace_name"] = keyspaceName
	}

	if len(tables) > 0 {
		postData["tables"] = tables
	}

	body, err := json.Marshal(postData)
	if err != nil {
		return nil, err
	}

	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return nil, err
	}

	request := &nodeMgmtRequest{
		endpoint: endpoint,
		host:     podHost,
		port:     podPort,
		method:   http.MethodPost,
		body:     body,
	}

	return request, nil
}

// CallKeyspaceCleanup returns the job id of the cleanup job
func (client *NodeMgmtClient) CallKeyspaceCleanup(pod *corev1.Pod, jobs int, keyspaceName string, tables []string) (string, error) {
	client.Log.Info(
		"calling Management API keyspace cleanup - POST /api/v1/ops/keyspace/cleanup",
		"pod", pod.Name,
	)

	req, err := createKeySpaceRequest(pod, jobs, keyspaceName, tables, "/api/v1/ops/keyspace/cleanup")
	if err != nil {
		return "", err
	}

	req.timeout = 20 * time.Second

	jobId, err := callNodeMgmtEndpoint(client, *req, "application/json")
	if err != nil {
		return "", err
	}

	return string(jobId), nil
}

// CallDatacenterRebuild returns the job id of the rebuild job.
func (client *NodeMgmtClient) CallDatacenterRebuild(pod *corev1.Pod, sourceDatacenter string) (string, error) {
	client.Log.Info(
		"calling Management API keyspace rebuild - POST /api/v1/ops/node/rebuild",
		"pod", pod.Name,
	)

	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return "", err
	}

	queryUrl := fmt.Sprintf("/api/v1/ops/node/rebuild?src_dc=%s", sourceDatacenter)

	req := nodeMgmtRequest{
		endpoint: queryUrl,
		host:     podHost,
		port:     podPort,
		method:   http.MethodPost,
		timeout:  60 * time.Second,
	}
	jobId, err := callNodeMgmtEndpoint(client, req, "application/json")
	if err != nil {
		return "", err
	}

	return string(jobId), nil
}

// CallUpgradeSSTables calls the v1 version of upgradeSSTables, returning the jobId
func (client *NodeMgmtClient) CallUpgradeSSTables(pod *corev1.Pod, jobs int, keyspaceName string, tables []string) (string, error) {
	client.Log.Info(
		"calling Management API keyspace sstable upgrade - POST /api/v1/ops/tables/sstables/upgrade",
		"pod", pod.Name,
	)

	req, err := createKeySpaceRequest(pod, jobs, keyspaceName, tables, "/api/v1/ops/tables/sstables/upgrade")
	if err != nil {
		return "", err
	}

	req.timeout = 20 * time.Second

	jobId, err := callNodeMgmtEndpoint(client, *req, "application/json")
	if err != nil {
		return "", err
	}

	return string(jobId), nil
}

// CallUpgradeSSTablesEndpoint calls the v0 version of upgradeSSTables endpoint (blocking operation)
func (client *NodeMgmtClient) CallUpgradeSSTablesEndpoint(pod *corev1.Pod, jobs int, keyspaceName string, tables []string) error {
	client.Log.Info(
		"calling Management API keyspace sstable upgrade - POST /api/v0/ops/tables/sstables/upgrade",
		"pod", pod.Name,
	)

	req, err := createKeySpaceRequest(pod, jobs, keyspaceName, tables, "/api/v0/ops/tables/sstables/upgrade")
	if err != nil {
		return err
	}

	req.timeout = 20 * time.Second

	_, err = callNodeMgmtEndpoint(client, *req, "application/json")
	return err
}

type CompactRequest struct {
	SplitOutput      bool     `json:"split_output,omitempty"`
	UserDefined      bool     `json:"user_defined,omitempty"`
	StartToken       string   `json:"start_token,omitempty"`
	EndToken         string   `json:"end_token,omitempty"`
	KeyspaceName     string   `json:"keyspace_name"`
	UserDefinedFiles []string `json:"user_defined_files,omitempty"`
	Tables           []string `json:"tables"`
}

func createCompactRequest(pod *corev1.Pod, compactRequest *CompactRequest, endpoint string) (*nodeMgmtRequest, error) {
	body, err := json.Marshal(compactRequest)
	if err != nil {
		return nil, err
	}

	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return nil, err
	}

	request := &nodeMgmtRequest{
		endpoint: endpoint,
		host:     podHost,
		port:     podPort,
		method:   http.MethodPost,
		body:     body,
	}

	return request, nil
}

// CallCompaction calls the v1 version of compaction to force a (major) compaction on one or more tables or user-defined compaction on given SSTables, returning the jobId
func (client *NodeMgmtClient) CallCompaction(pod *corev1.Pod, compactRequest *CompactRequest) (string, error) {
	client.Log.Info(
		"calling Management API keyspace force compaction - POST /api/v1/ops/tables/compact",
		"pod", pod.Name,
	)

	req, err := createCompactRequest(pod, compactRequest, "/api/v1/ops/tables/compact")
	if err != nil {
		return "", err
	}
	req.timeout = 20 * time.Second

	jobId, err := callNodeMgmtEndpoint(client, *req, "application/json")
	if err != nil {
		return "", err
	}

	return string(jobId), nil
}

// CallCompactionEndpoint calls the blocking version (v0) of compactionto force a (major) compaction on one or more tables or user-defined compaction on given SSTables
func (client *NodeMgmtClient) CallCompactionEndpoint(pod *corev1.Pod, compactRequest *CompactRequest) error {
	client.Log.Info(
		"calling Management API keyspace force compaction - POST /api/v0/ops/tables/compact",
		"pod", pod.Name,
	)

	req, err := createCompactRequest(pod, compactRequest, "/api/v0/ops/tables/compact")
	if err != nil {
		return err
	}
	req.timeout = 60 * time.Second

	_, err = callNodeMgmtEndpoint(client, *req, "application/json")
	if err != nil {
		return err
	}

	return nil
}

// CallTSReloadEndpoint calls the async version of TSReload
func (client *NodeMgmtClient) CallinodeTsReloadEndpoint(pod *corev1.Pod) error {
	client.Log.Info(
		"calling Management API TS REload endpoint - POST /api/v0/node/encryption/internode/truststore/reload",
		"pod", pod.Name,
	)
	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return err
	}

	request := nodeMgmtRequest{
		endpoint: "/api/v0/ops/node/encryption/internode/truststore/reload",
		host:     podHost,
		port:     podPort,
		method:   http.MethodPost,
		timeout:  60 * time.Second,
	}

	_, err = callNodeMgmtEndpoint(client, request, "application/json")
	if err != nil {
		return err
	}

	return nil
}

type ScrubRequest struct {
	DisableSnapshot       bool     `json:"disable_snapshot"`
	SkipCorrupted         bool     `json:"skip_corrupted"`
	CheckData             bool     `json:"check_data"`
	ReinsertOverflowedTTL bool     `json:"reinsert_overflowed_ttl"`
	Jobs                  int      `json:"jobs"`
	KeyspaceName          string   `json:"keyspace_name"`
	Tables                []string `json:"tables"`
}

func createScrubRequest(pod *corev1.Pod, scrubRequest *ScrubRequest, endpoint string) (*nodeMgmtRequest, error) {
	body, err := json.Marshal(scrubRequest)
	if err != nil {
		return nil, err
	}

	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return nil, err
	}

	request := &nodeMgmtRequest{
		endpoint: endpoint,
		host:     podHost,
		port:     podPort,
		method:   http.MethodPost,
		body:     body,
	}

	return request, nil
}

// CallScrub calls the v1 version of scrub, rebuilding sstables for one or more tables, returning the jobId.
func (client *NodeMgmtClient) CallScrub(pod *corev1.Pod, scrubRequest *ScrubRequest) (string, error) {
	client.Log.Info(
		"calling Management API scrub - POST /api/v1/ops/tables/scrub",
		"pod", pod.Name,
	)
	req, err := createScrubRequest(pod, scrubRequest, "/api/v1/ops/tables/scrub")
	if err != nil {
		return "", err
	}
	req.timeout = 20 * time.Second

	jobId, err := callNodeMgmtEndpoint(client, *req, "application/json")
	if err != nil {
		return "", err
	}

	return string(jobId), nil
}

// CallScrubEndpoint calls the blocking version of scrub (v0 version) rebuilding sstables for one or more tables
func (client *NodeMgmtClient) CallScrubEndpoint(pod *corev1.Pod, scrubRequest *ScrubRequest) error {
	client.Log.Info(
		"calling Management API scrub - POST /api/v0/ops/tables/scrub",
		"pod", pod.Name,
	)

	req, err := createScrubRequest(pod, scrubRequest, "/api/v0/ops/tables/scrub")
	if err != nil {
		return err
	}
	req.timeout = 60 * time.Second

	_, err = callNodeMgmtEndpoint(client, *req, "application/json")
	if err != nil {
		return err
	}

	return nil
}

// CreateKeyspace calls management API to create a new Keyspace.
func (client *NodeMgmtClient) CreateKeyspace(pod *corev1.Pod, keyspaceName string, replicationSettings []map[string]string) error {
	return client.modifyKeyspace("create", pod, keyspaceName, replicationSettings)
}

// AlterKeyspace modifies the keyspace by calling management API
func (client *NodeMgmtClient) AlterKeyspace(pod *corev1.Pod, keyspaceName string, replicationSettings []map[string]string) error {
	return client.modifyKeyspace("alter", pod, keyspaceName, replicationSettings)
}

func (client *NodeMgmtClient) modifyKeyspace(endpoint string, pod *corev1.Pod, keyspaceName string, replicationSettings []map[string]string) error {
	postData := make(map[string]interface{})

	if keyspaceName == "" || replicationSettings == nil {
		return fmt.Errorf("keyspacename and replication settings are required")
	}

	postData["keyspace_name"] = keyspaceName
	postData["replication_settings"] = replicationSettings

	body, err := json.Marshal(postData)
	if err != nil {
		return err
	}

	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return err
	}

	request := nodeMgmtRequest{
		endpoint: fmt.Sprintf("/api/v0/ops/keyspace/%s", endpoint),
		host:     podHost,
		port:     podPort,
		method:   http.MethodPost,
		timeout:  time.Second * 20,
		body:     body,
	}

	_, err = callNodeMgmtEndpoint(client, request, "application/json")
	return err
}

func parseListKeyspacesEndpointsResponseBody(body []byte) ([]string, error) {
	var keyspaces []string
	if err := json.Unmarshal(body, &keyspaces); err != nil {
		return nil, err
	}
	return keyspaces, nil
}

// GetKeyspace calls the management API to check if a specific keyspace exists
func (client *NodeMgmtClient) GetKeyspace(pod *corev1.Pod, keyspaceName string) ([]string, error) {
	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return nil, err
	}
	endpoint := "/api/v0/ops/keyspace"
	if keyspaceName != "" {
		endpoint += fmt.Sprintf("?keyspaceName=%s", keyspaceName)
	}
	request := nodeMgmtRequest{
		endpoint: endpoint,
		host:     podHost,
		port:     podPort,
		method:   http.MethodGet,
		timeout:  time.Second * 20,
	}

	body, err := callNodeMgmtEndpoint(client, request, "application/json")
	if err != nil {
		return nil, err
	}

	keyspaces, err := parseListKeyspacesEndpointsResponseBody(body)
	return keyspaces, err
}

// ListKeyspaces calls the management API to list existing keyspaces
func (client *NodeMgmtClient) ListKeyspaces(pod *corev1.Pod) ([]string, error) {
	// Calling GetKeyspace with an empty keyspace name lists all keyspaces
	return client.GetKeyspace(pod, "")
}

// GetKeyspaceReplication calls the management API to retrieve the replication settings of the
// given keyspace.
func (client *NodeMgmtClient) GetKeyspaceReplication(pod *corev1.Pod, keyspaceName string) (map[string]string, error) {
	if keyspaceName == "" {
		return nil, fmt.Errorf("keyspace name cannot be empty")
	}
	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return nil, err
	}
	endpoint := "/api/v0/ops/keyspace/replication?keyspaceName=" + keyspaceName
	request := nodeMgmtRequest{
		endpoint: endpoint,
		host:     podHost,
		port:     podPort,
		method:   http.MethodGet,
		timeout:  time.Second * 20,
	}
	body, err := callNodeMgmtEndpoint(client, request, "application/json")
	if err != nil {
		return nil, err
	}
	var replication map[string]string
	if err := json.Unmarshal(body, &replication); err != nil {
		return nil, err
	}
	return replication, nil
}

// ListTables calls the management API and returns the table names in the given keyspace
func (client *NodeMgmtClient) ListTables(pod *corev1.Pod, keyspaceName string) ([]string, error) {
	if keyspaceName == "" {
		return nil, fmt.Errorf("keyspace name cannot be empty")
	}
	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return nil, err
	}
	endpoint := "/api/v0/ops/tables?keyspaceName=" + keyspaceName
	request := nodeMgmtRequest{
		endpoint: endpoint,
		host:     podHost,
		port:     podPort,
		method:   http.MethodGet,
		timeout:  time.Second * 20,
	}
	body, err := callNodeMgmtEndpoint(client, request, "application/json")
	if err != nil {
		return nil, err
	}
	var tables []string
	if err := json.Unmarshal(body, &tables); err != nil {
		return nil, err
	}
	return tables, nil
}

type TableDefinition struct {
	KeyspaceName string                 `json:"keyspace_name"`
	TableName    string                 `json:"table_name"`
	Columns      []*ColumnDefinition    `json:"columns"`
	Options      map[string]interface{} `json:"options,omitempty"`
}

func NewTableDefinition(keyspaceName string, tableName string, columns ...*ColumnDefinition) *TableDefinition {
	return &TableDefinition{
		KeyspaceName: keyspaceName,
		TableName:    tableName,
		Columns:      columns,
	}
}

type ColumnKind string

const (
	ColumnKindPartitionKey     ColumnKind = "PARTITION_KEY"
	ColumnKindClusteringColumn ColumnKind = "CLUSTERING_COLUMN"
	ColumnKindRegular          ColumnKind = "REGULAR"
	ColumnKindStatic           ColumnKind = "STATIC"
)

type ClusteringOrder string

const (
	ClusteringOrderAsc  ClusteringOrder = "ASC"
	ClusteringOrderDesc ClusteringOrder = "DESC"
)

type ColumnDefinition struct {
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Kind     ColumnKind      `json:"kind"`
	Position int             `json:"position"`
	Order    ClusteringOrder `json:"order,omitempty"`
}

func NewPartitionKeyColumn(name string, dataType string, position int) *ColumnDefinition {
	return &ColumnDefinition{
		Name:     name,
		Type:     dataType,
		Kind:     ColumnKindPartitionKey,
		Position: position,
	}
}

func NewClusteringColumn(name string, dataType string, position int, order ClusteringOrder) *ColumnDefinition {
	return &ColumnDefinition{
		Name:     name,
		Type:     dataType,
		Kind:     ColumnKindClusteringColumn,
		Position: position,
		Order:    order,
	}
}

func NewRegularColumn(name string, dataType string) *ColumnDefinition {
	return &ColumnDefinition{
		Name: name,
		Type: dataType,
		Kind: ColumnKindRegular,
	}
}

func NewStaticColumn(name string, dataType string) *ColumnDefinition {
	return &ColumnDefinition{
		Name: name,
		Type: dataType,
		Kind: ColumnKindStatic,
	}
}

// CreateTable calls the management API to create a new table.
func (client *NodeMgmtClient) CreateTable(pod *corev1.Pod, table *TableDefinition) error {
	if table == nil {
		return fmt.Errorf("table definition cannot be nil")
	} else if table.KeyspaceName == "" {
		return fmt.Errorf("keyspace name cannot be empty")
	} else if table.TableName == "" {
		return fmt.Errorf("table name cannot be empty")
	} else if len(table.Columns) == 0 {
		return fmt.Errorf("columns cannot be empty")
	}
	// The rest will be validated server-side
	body, err := json.Marshal(table)
	if err != nil {
		return err
	}
	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return err
	}
	endpoint := "/api/v0/ops/tables/create"
	request := nodeMgmtRequest{
		endpoint: endpoint,
		host:     podHost,
		port:     podPort,
		method:   http.MethodPost,
		timeout:  time.Second * 40,
		body:     body,
	}
	_, err = callNodeMgmtEndpoint(client, request, "application/json")
	return err
}

func (client *NodeMgmtClient) CallLifecycleStartEndpointWithReplaceIp(pod *corev1.Pod, replaceIp string) error {
	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return err
	}

	client.Log.Info(
		"calling Management API start node - POST /api/v0/lifecycle/start",
		"pod", pod.Name,
		"podIP", podHost,
		"replaceIP", replaceIp,
	)

	endpoint := "/api/v0/lifecycle/start"

	if replaceIp != "" {
		endpoint = buildEndpoint(endpoint, "replace_ip", replaceIp)
	}

	request := nodeMgmtRequest{
		endpoint: endpoint,
		host:     podHost,
		port:     podPort,
		method:   http.MethodPost,
		timeout:  10 * time.Minute,
	}

	_, err = callNodeMgmtEndpoint(client, request, "")
	return err
}

func (client *NodeMgmtClient) CallLifecycleStartEndpoint(pod *corev1.Pod) error {
	return client.CallLifecycleStartEndpointWithReplaceIp(pod, "")
}

func (client *NodeMgmtClient) CallReloadSeedsEndpoint(pod *corev1.Pod) error {
	client.Log.Info(
		"calling Management API reload seeds - POST /api/v0/ops/seeds/reload",
		"pod", pod.Name,
	)

	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return err
	}

	request := nodeMgmtRequest{
		endpoint: "/api/v0/ops/seeds/reload",
		host:     podHost,
		port:     podPort,
		method:   http.MethodPost,
		timeout:  60 * time.Second,
	}

	_, err = callNodeMgmtEndpoint(client, request, "")
	return err
}

// CallDecommissionNodeEndpoint is for the old /api/v0 decommission. Use the CallDecommissionNode for async behavior if available
func (client *NodeMgmtClient) CallDecommissionNodeEndpoint(pod *corev1.Pod) error {
	client.Log.Info(
		"calling Management API decommission node - POST /api/v0/ops/node/decommission",
		"pod", pod.Name,
	)

	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return err
	}

	request := nodeMgmtRequest{
		endpoint: "/api/v0/ops/node/decommission?force=true",
		host:     podHost,
		port:     podPort,
		method:   http.MethodPost,
		timeout:  60 * time.Second,
	}

	_, err = callNodeMgmtEndpoint(client, request, "")
	return err
}

// CallDecommissionNode returns the job id of the decommission job.
func (client *NodeMgmtClient) CallDecommissionNode(pod *corev1.Pod, force bool) (string, error) {
	client.Log.Info(
		"calling Management API keyspace rebuild - POST /api/v1/ops/node/decommission",
		"pod", pod.Name,
	)

	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return "", err
	}

	queryUrl := fmt.Sprintf("/api/v1/ops/node/decommission?force=%t", force)

	req := nodeMgmtRequest{
		endpoint: queryUrl,
		host:     podHost,
		port:     podPort,
		method:   http.MethodPost,
		timeout:  60 * time.Second,
	}
	jobId, err := callNodeMgmtEndpoint(client, req, "application/json")
	if err != nil {
		return "", err
	}

	return string(jobId), nil
}

// FeatureSet returns supported features on the target pod. If the target pod is too old, empty
// FeatureSet is returned. One can check the supported feature by using FeatureSet.Supports(feature) and that
// will work regardless if this endpoint returns a result or 404 (other errors are passed as error)
func (client *NodeMgmtClient) FeatureSet(pod *corev1.Pod) (*FeatureSet, error) {
	client.Log.Info(
		"calling Management API features - GET /api/v0/metadata/versions/features",
		"pod", pod.Name,
	)

	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return nil, err
	}

	request := nodeMgmtRequest{
		endpoint: "/api/v0/metadata/versions/features",
		host:     podHost,
		port:     podPort,
		method:   http.MethodGet,
	}

	data, err := callNodeMgmtEndpoint(client, request, "")
	if err != nil {
		if re, ok := err.(*RequestError); ok {
			// There's no supported new features on this endpoint
			if re.NotFound() {
				return &FeatureSet{}, nil
			}
		}
		return nil, err
	}

	features := &FeatureSet{}
	if err := json.Unmarshal(data, &features); err != nil {
		return nil, err
	}

	return features, nil
}

func (client *NodeMgmtClient) JobDetails(pod *corev1.Pod, jobId string) (*JobDetails, error) {
	client.Log.Info(
		"calling Management API jobDetails - GET /api/v0/ops/executor/job",
		"pod", pod.Name,
		"jobId", jobId,
	)

	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return nil, err
	}

	request := nodeMgmtRequest{
		endpoint: fmt.Sprintf("/api/v0/ops/executor/job?job_id=%s", jobId),
		host:     podHost,
		port:     podPort,
		method:   http.MethodGet,
	}

	job := &JobDetails{}
	data, err := callNodeMgmtEndpoint(client, request, "")
	if err != nil {
		if re, ok := err.(*RequestError); ok && re.NotFound() {
			// Job was not found, the request did succeed
			return job, nil
		}
		client.Log.Error(err, "failed to fetch job details from management-api")
		return nil, err
	}

	if err := json.Unmarshal(data, job); err != nil {
		return nil, err
	}

	return job, nil
}

// CallMove invokes the node move endpoint and returns the job id of the move job.
func (client *NodeMgmtClient) CallMove(pod *corev1.Pod, newToken string) (string, error) {
	client.Log.Info(
		"calling Management API node move - POST /api/v0/ops/node/move",
		"pod", pod.Name,
		"newToken", newToken,
	)

	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return "", err
	}

	queryUrl := fmt.Sprintf("/api/v0/ops/node/move?newToken=%s", newToken)

	req := nodeMgmtRequest{
		endpoint: queryUrl,
		host:     podHost,
		port:     podPort,
		method:   http.MethodPost,
		timeout:  60 * time.Second,
	}
	jobId, err := callNodeMgmtEndpoint(client, req, "")
	if err != nil {
		return "", err
	}

	return string(jobId), nil
}

func callNodeMgmtEndpoint(client *NodeMgmtClient, request nodeMgmtRequest, contentType string) ([]byte, error) {
	client.Log.Info("client::callNodeMgmtEndpoint")

	port := 8080
	if request.port > 0 {
		port = request.port
	}

	url := fmt.Sprintf("%s://%s:%d%s", client.Protocol, request.host, port, request.endpoint)

	var reqBody io.Reader
	if len(request.body) > 0 {
		reqBody = bytes.NewBuffer(request.body)
	}

	req, err := http.NewRequest(request.method, url, reqBody)
	if err != nil {
		return nil, err
	}
	req.Close = true

	if request.timeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), request.timeout)
		defer cancel()
		req = req.WithContext(ctx)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	res, err := client.Client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		err := res.Body.Close()
		if err != nil {
			client.Log.Error(err, "unable to close response body")
		}
	}()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		client.Log.Error(err, "Unable to read response from Node Management Endpoint")
		return nil, err
	}

	goodStatus := res.StatusCode >= 200 && res.StatusCode < 300
	if !goodStatus {
		reqErr := &RequestError{
			StatusCode: res.StatusCode,
			Err:        fmt.Errorf("incorrect status code of %d when calling endpoint", res.StatusCode),
		}
		if res.StatusCode != http.StatusNotFound {
			client.Log.Error(reqErr, "incorrect status code when calling Node Management Endpoint",
				"statusCode", res.StatusCode,
				"pod", request.host)
		}

		return nil, reqErr
	}

	return body, nil
}

type RequestError struct {
	StatusCode int
	Err        error
}

func (r *RequestError) Error() string {
	return r.Err.Error()
}

func (r *RequestError) NotFound() bool {
	return r.StatusCode == http.StatusNotFound
}

func (client *NodeMgmtClient) CallIsFullQueryLogEnabledEndpoint(pod *corev1.Pod) (bool, error) {
	client.Log.Info(
		"calling Management API full query logging - GET /api/v0/ops/node/fullquerylogging",
		"pod", pod.Name,
	)

	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return false, err
	}
	request := nodeMgmtRequest{
		endpoint: "/api/v0/ops/node/fullquerylogging",
		host:     podHost,
		port:     podPort,
		method:   http.MethodGet,
		timeout:  time.Minute * 2,
	}
	apiResponse, err := callNodeMgmtEndpoint(client, request, "")
	if err != nil {
		client.Log.Error(err, "failed to call endpoint /api/v0/ops/node/fullquerylogging")
		return false, err
	}
	var parsedResponse map[string]interface{}
	err = json.Unmarshal([]byte(apiResponse), &parsedResponse)
	if err != nil {
		client.Log.Error(err, "failed to unmarshall JSON response from /api/v0/ops/node/fullquerylogging", "response", string(apiResponse))
		return false, err
	}
	fqlIsEnabled, ok := parsedResponse["entity"]
	if !ok {
		err := errors.New("failed to retrieve Entity key from /api/v0/ops/node/fullquerylogging")
		return false, err
	}
	fqlIsEnabledBool, err := strconv.ParseBool(fmt.Sprintf("%v", fqlIsEnabled))
	if err != nil {
		err := errors.New("failed to cast response from /api/v0/ops/node/fullquerylogging to bool")
		return false, err
	}
	return fqlIsEnabledBool, err
}

func (client *NodeMgmtClient) CallSetFullQueryLog(pod *corev1.Pod, enableFullQueryLogging bool) error {

	client.Log.Info("client::callIsFullQueryLogEnabledEndpoint")
	podHost, podPort, err := BuildPodHostFromPod(pod)
	if err != nil {
		return err
	}
	request := nodeMgmtRequest{
		endpoint: "/api/v0/ops/node/fullquerylogging?enabled=" + strconv.FormatBool(enableFullQueryLogging),
		host:     podHost,
		port:     podPort,
		method:   http.MethodPost,
		timeout:  time.Minute * 2,
	}
	client.Log.Info(
		fmt.Sprintf("calling Management API set full query logging - POST %s", request.endpoint),
		"pod", pod.Name,
	)

	_, err = callNodeMgmtEndpoint(client, request, "")

	return err
}

func (client *NodeMgmtClient) CallFlush(pod *corev1.Pod, keyspaceName string, tables []string) (string, error) {
	client.Log.Info(
		"calling Management API keyspace flush - POST /api/v1/ops/tables/flush",
		"pod", pod.Name,
	)

	req, err := createKeySpaceRequest(pod, -1, keyspaceName, tables, "/api/v1/ops/tables/flush")
	if err != nil {
		return "", err
	}

	req.timeout = 20 * time.Second

	jobId, err := callNodeMgmtEndpoint(client, *req, "application/json")
	return string(jobId), err
}

func (client *NodeMgmtClient) CallFlushEndpoint(pod *corev1.Pod, keyspaceName string, tables []string) error {
	client.Log.Info(
		"calling Management API keyspace flush - POST /api/v0/ops/tables/flush",
		"pod", pod.Name,
	)

	req, err := createKeySpaceRequest(pod, -1, keyspaceName, tables, "/api/v0/ops/tables/flush")
	if err != nil {
		return err
	}

	req.timeout = 60 * time.Second

	_, err = callNodeMgmtEndpoint(client, *req, "application/json")
	return err
}

func (client *NodeMgmtClient) CallGarbageCollect(pod *corev1.Pod, keyspaceName string, tables []string) (string, error) {
	client.Log.Info(
		"calling Management API keyspace flush - POST /api/v1/ops/tables/garbagecollect",
		"pod", pod.Name,
	)

	req, err := createKeySpaceRequest(pod, -1, keyspaceName, tables, "/api/v1/ops/tables/garbagecollect")
	if err != nil {
		return "", err
	}

	req.timeout = 20 * time.Second

	jobId, err := callNodeMgmtEndpoint(client, *req, "application/json")
	return string(jobId), err
}

func (client *NodeMgmtClient) CallGarbageCollectEndpoint(pod *corev1.Pod, keyspaceName string, tables []string) error {
	client.Log.Info(
		"calling Management API keyspace flush - POST /api/v0/ops/tables/garbagecollect",
		"pod", pod.Name,
	)

	req, err := createKeySpaceRequest(pod, -1, keyspaceName, tables, "/api/v0/ops/tables/garbagecollect")
	if err != nil {
		return err
	}

	req.timeout = 60 * time.Second

	_, err = callNodeMgmtEndpoint(client, *req, "application/json")
	return err
}
