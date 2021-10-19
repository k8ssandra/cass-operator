package reconciliation

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/k8ssandra/cass-operator/pkg/internal/result"
	"github.com/stretchr/testify/assert"
)

// TDOO: we cannot currently write a func Test_SetFullQueryLogging_ListFail(t *testing.T) {...} test because we cannot inject a mock listPods method into the rc to test this code path.

// TDOO: we cannot currently write a func Test_SetFullQueryLogging_StatusFail(t *testing.T) {...} test because the mocks do not include a mock sts controller to instantiate the pods.
// An ability to override the listPods method with a mock would probably allow us to inject a fake list of pods here too.

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
	mockRC.Datacenter.Spec.ServerType = "cassandra"
	parsedFQLisEnabled, _, recResult := parseFQLFromConfig(mockRC)
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
	mockRC.Datacenter.Spec.ServerType = "cassandra"
	parsedFQLisEnabled, _, recResult := parseFQLFromConfig(mockRC)
	assert.False(t, parsedFQLisEnabled)
	assert.Equal(t, result.Continue(), recResult)
	cleanupMockScr()
}
func Test_parseFQLFromConfig_noConfig(t *testing.T) {
	// Test parsing when DC config key does not exist at all, should return (false, continue()).
	mockRC, _, cleanupMockScr := setupTest()
	mockRC.Datacenter.Spec.Config = json.RawMessage("{}")
	mockRC.Datacenter.Spec.ServerType = "cassandra"
	parsedFQLisEnabled, _, recResult := parseFQLFromConfig(mockRC)
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
	mockRC.Datacenter.Spec.ServerType = "cassandra"
	parsedFQLisEnabled, _, recResult := parseFQLFromConfig(mockRC)
	assert.False(t, parsedFQLisEnabled)
	assert.IsType(t, result.Error(errors.New("")), recResult)
	cleanupMockScr()
}

func Test_parseFQLFromConfig_3xFQLEnabled(t *testing.T) {
	// Test parsing when dcConfig asks for FQL on a non-4x server, should return (false, error).
	mockRC, _, cleanupMockScr := setupTest()
	mockRC.Datacenter.Spec.Config = json.RawMessage(fqlEnabledConfig)
	mockRC.Datacenter.Spec.ServerVersion = "3.11.10"
	mockRC.Datacenter.Spec.ServerType = "cassandra"
	parsedFQLisEnabled, _, recResult := parseFQLFromConfig(mockRC)
	assert.False(t, parsedFQLisEnabled)
	assert.IsType(t, result.Error(errors.New("")), recResult)
	cleanupMockScr()
}

func Test_parseFQLFromConfig_DSEFQLEnabled(t *testing.T) {
	// Test parsing when dcConfig asks for FQL on a non-4x server, should return (false, error).
	mockRC, _, cleanupMockScr := setupTest()
	mockRC.Datacenter.Spec.Config = json.RawMessage(fqlEnabledConfig)
	mockRC.Datacenter.Spec.ServerVersion = "4.0.0"
	mockRC.Datacenter.Spec.ServerType = "dse"
	parsedFQLisEnabled, _, recResult := parseFQLFromConfig(mockRC)
	assert.False(t, parsedFQLisEnabled)
	assert.IsType(t, result.Error(errors.New("")), recResult)
	cleanupMockScr()
}
