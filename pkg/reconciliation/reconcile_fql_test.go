package reconciliation

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/k8ssandra/cass-operator/pkg/httphelper"
	"github.com/k8ssandra/cass-operator/pkg/internal/result"
	"github.com/k8ssandra/cass-operator/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

////////////////////////////////////
//////Test SetFullQueryLogging//////
////////////////////////////////////
func Test_SetFullQueryLogging_EnableSucceed(t *testing.T) {
	mockRC, _, cleanupMockScr := setupTest()
	res := &http.Response{
		StatusCode: http.StatusOK,
		Body:       ioutil.NopCloser(strings.NewReader("OK")),
	}
	mockHttpClient := &mocks.HttpClient{}
	mockHttpClient.On("Do",
		mock.MatchedBy(
			func(req *http.Request) bool {
				return req != nil
			})).
		Return(res, nil).
		Once()

	client := httphelper.NodeMgmtClient{
		Client:   mockHttpClient,
		Log:      mockRC.ReqLogger,
		Protocol: "http",
	}
	mockRC.NodeMgmtClient = client
	recResult := SetFullQueryLogging(mockRC, true)
	assert.Equal(t, result.Continue(), recResult)
	cleanupMockScr()
}

func Test_SetFullQueryLogging_EnableFail(t *testing.T) {
	mockRC, _, cleanupMockScr := setupTest()
	res := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       ioutil.NopCloser(strings.NewReader("Testing failure scenario.")),
	}
	mockHttpClient := &mocks.HttpClient{}
	mockHttpClient.On("Do",
		mock.MatchedBy(
			func(req *http.Request) bool {
				return req != nil
			})).
		Return(res, nil).
		Once()

	client := httphelper.NodeMgmtClient{
		Client:   mockHttpClient,
		Log:      mockRC.ReqLogger,
		Protocol: "http",
	}
	mockRC.NodeMgmtClient = client
	recResult := SetFullQueryLogging(mockRC, true)
	assert.IsType(t, result.RequeueSoon(2), recResult)
	cleanupMockScr()
}

// TDOO: we cannot currently write a func Test_SetFullQueryLogging_ListFail(t *testing.T) {...} test because we cannot inject a mock listPods method into the rc to test this code path.

// TDOO: we cannot currently write a func Test_SetFullQueryLogging_StatusFail(t *testing.T) {...} test because the mocks do not include a mock sts controller to instantiate the pods.
// While an e2e test could theoretically be written, it would be a significant investment. An ability to override the listPods method with a mock would probably allow us to inject a fake
// list of pods here too.

////////////////////////////////////
//////Test parseFQLFromConfig///////
////////////////////////////////////
var fqlEnabledConfig string = `{"cassandra-yaml": { 
		"full_query_logging_options": {
			"log_dir": "/var/log/cassandra/fql" 
			}
		}
	}
`

func Test_parseFQLFromConfig_fqlEnabled(t *testing.T) {
	// Test parsing when fql is set, should return (true, continue).
	mockRC, _, cleanupMockScr := setupTest()
	mockRC.Datacenter.Spec.Config = json.RawMessage(fqlEnabledConfig)
	parsedFQLisEnabled, recResult := parseFQLFromConfig(mockRC)
	assert.True(t, parsedFQLisEnabled)
	assert.Equal(t, result.Continue(), recResult)
	cleanupMockScr()
}

var fqlDisabledConfig string = `{"cassandra-yaml": {
	"key_cache_size_in_mb": 256
	}
}
`

func Test_parseFQLFromConfig_fqlDisabled(t *testing.T) {
	// Test parsing when config exists + fql not set, should return (false, continue()).
	mockRC, _, cleanupMockScr := setupTest()
	mockRC.Datacenter.Spec.Config = json.RawMessage(fqlDisabledConfig)
	parsedFQLisEnabled, recResult := parseFQLFromConfig(mockRC)
	assert.False(t, parsedFQLisEnabled)
	assert.Equal(t, result.Continue(), recResult)
	cleanupMockScr()
}
func Test_parseFQLFromConfig_noConfig(t *testing.T) {
	// Test parsing when DC config key does not exist at all, should return (false, continue()).
	mockRC, _, cleanupMockScr := setupTest()
	mockRC.Datacenter.Spec.Config = json.RawMessage("{}")
	parsedFQLisEnabled, recResult := parseFQLFromConfig(mockRC)
	assert.False(t, parsedFQLisEnabled)
	assert.Equal(t, result.Continue(), recResult)
	cleanupMockScr()
}

func Test_parseFQLFromConfig_malformedConfig(t *testing.T) {
	// Test parsing when dcConfig is malformed, should return (false, error).
	mockRC, _, cleanupMockScr := setupTest()
	var corruptedCfg []byte
	for _, b := range json.RawMessage(fqlEnabledConfig) {
		corruptedCfg = append(corruptedCfg, b<<3) // corrupt the byte array.
	}
	mockRC.Datacenter.Spec.Config = corruptedCfg
	parsedFQLisEnabled, recResult := parseFQLFromConfig(mockRC)
	assert.False(t, parsedFQLisEnabled)
	assert.IsType(t, result.Error(errors.New("")), recResult)
	cleanupMockScr()
}

func Test_parseFQLFromConfig_3xFQLEnabled(t *testing.T) {
	// Test parsing when dcConfig asks for FQL on a non-4x server, should return (false, error).
	mockRC, _, cleanupMockScr := setupTest()
	mockRC.Datacenter.Spec.Config = json.RawMessage(fqlEnabledConfig)
	mockRC.Datacenter.Spec.ServerVersion = "3.11.10"
	parsedFQLisEnabled, recResult := parseFQLFromConfig(mockRC)
	assert.False(t, parsedFQLisEnabled)
	assert.IsType(t, result.Error(errors.New("")), recResult)
	cleanupMockScr()
}
