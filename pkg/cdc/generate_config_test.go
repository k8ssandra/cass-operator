package cdc

import (
	"encoding/json"
	"testing"

	cassdcapi "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/stretchr/testify/assert"
)

var existingConfig = `
{
	"cassandra-env-sh": {
	  "additional-jvm-opts": [
		"-Dcassandra.system_distributed_replication=test-dc:1",
		"-Dcom.sun.management.jmxremote.authenticate=true"
	  ]
	},
	"cassandra-yaml": {
	  "authenticator": "PasswordAuthenticator",
	  "authorizer": "CassandraAuthorizer",
	  "num_tokens": 256,
	  "role_manager": "CassandraRoleManager",
	  "start_rpc": false
	},
	"cluster-info": {
	  "name": "test",
	  "seeds": "test-seed-service,test-dc-additional-seed-service"
	},
	"datacenter-info": {
	  "graph-enabled": 0,
	  "name": "dc1",
	  "solr-enabled": 0,
	  "spark-enabled": 0
	}
}
`

type testCase struct {
	Description    string
	InitialConfig  string
	DC             cassdcapi.CassandraDatacenter
	Expected       string
	ParsedExpected map[string]interface{}
	Actual         map[string]interface{}
}

// run runs the testCase and populates the actual and ParsedExpected maps.
func (c *testCase) run(t *testing.T) {
	newConfig, err := UpdateConfig(json.RawMessage(c.InitialConfig), c.DC)
	assert.NoError(t, err, err)
	c.Actual = make(map[string]interface{})
	err = json.Unmarshal(newConfig, &c.Actual)
	assert.NoError(t, err, err)
	if c.Expected != "" {
		c.ParsedExpected = make(map[string]interface{})
		err = json.Unmarshal([]byte(c.Expected), &c.ParsedExpected)
		assert.NoError(t, err, err)
	}
}

// TestUpdateConfig_ExistingConfig_NoCDC tests for when there are already existing entries in the config field.
// The main purpose here is to ensure that when marshalling and unmarshalling from the structs, we aren't losing fields.
func TestUpdateConfig_ExistingConfig_NoCDC(t *testing.T) {
	dc := GetCassandraDatacenter("test-dc", "test-ns")
	dc.Spec.CDC = (*cassdcapi.CDCConfiguration)(nil)
	test := testCase{
		Description:   "When CDC not requested and a config json exists, UpdateConfig() does nothing.",
		InitialConfig: existingConfig,
		DC:            dc,
		Expected: `{
			"cassandra-env-sh": {
			  "additional-jvm-opts": [
				"-Dcassandra.system_distributed_replication=test-dc:1",
				"-Dcom.sun.management.jmxremote.authenticate=true"
			  ]
			},
			"cassandra-yaml": {
			  "authenticator": "PasswordAuthenticator",
			  "authorizer": "CassandraAuthorizer",
			  "num_tokens": 256,
			  "role_manager": "CassandraRoleManager",
			  "start_rpc": false
			},
			"cluster-info": {
			  "name": "test",
			  "seeds": "test-seed-service,test-dc-additional-seed-service"
			},
			"datacenter-info": {
			  "graph-enabled": 0,
			  "name": "dc1",
			  "solr-enabled": 0,
			  "spark-enabled": 0
			}
		}
		`,
	}
	test.run(t)
	assert.Equal(t, test.ParsedExpected, test.Actual, "modified config was not what we expected")

	// Make sure that this also works on cassandra-env-sh.

	test = testCase{
		Description: "When CDC not requested and a config json with cassandra-env-sh exists, UpdateConfig() preserves all fields in cassandra-env-sh.",
		InitialConfig: `
		{
			"cassandra-env-sh": {
				"additional-jvm-opts": [
					"jvmopt"
				],
				"unknownfield": true
			}
		}
		`,
		DC: dc,
		Expected: `
		{
			"cassandra-env-sh": {
				"additional-jvm-opts": [
					"jvmopt"
				],
				"unknownfield": true
			}
		}
		`,
	}
	test.run(t)
	assert.Equal(t, test.ParsedExpected, test.Actual, "modified config and initial config did not match, we expected them to")
}

// TestUpdateConfig_ExistingConfig_WithCDC tests for when there is an existing config, and we are adding the CDC parms. The main purpose
// of this test is to ensure that the relevant jvm additional opts are added to the existing config.
func TestUpdateConfig_ExistingConfig_WithCDC(t *testing.T) {
	dc := GetCassandraDatacenter("test-dc", "test-ns")
	pulsarServiceUrl := "pulsar://pulsar:6650"
	topicPrefix := "test-prefix-"
	dc.Spec.CDC = &cassdcapi.CDCConfiguration{
		PulsarServiceUrl: &pulsarServiceUrl,
		TopicPrefix:      &topicPrefix,
	}
	test := testCase{
		Description:   "When CDC IS requested and a config json exists, UpdateConfig() adds the expected CDC related additional-jvm-opts.",
		InitialConfig: existingConfig,
		DC:            dc,
	}
	test.run(t)
	assert.Contains(t,
		test.Actual["cassandra-env-sh"].(map[string]interface{})["additional-jvm-opts"],
		"-javaagent:/opt/cdc_agent/cdc-agent.jar=pulsarServiceUrl=pulsar://pulsar:6650,topicPrefix=test-prefix-",
	)
}

// TestUpdateConfig_ExistingConfig_WithoutCDC tests that CDC is removed from additional-jvm-opts when it is present but CDC should be disabled.
func TestUpdateConfig_ExistingConfig_WithoutCDC(t *testing.T) {
	// Test case when the DC has CDC explicitly marked false.
	dc := GetCassandraDatacenter("test-dc", "test-ns")
	dc.Spec.CDC = (*cassdcapi.CDCConfiguration)(nil)
	jvmAddtnlOptionsJson := `
	{
		"cassandra-env-sh": {
			"test-option1": "100M",
			"additional-jvm-opts": [
				"additional-option2"
			]
		}
	}`
	test := testCase{
		Description:   "When CDC is to be disabled UpdateConfig() removes all CDC related parts of additional-jvm-opts while retaining all others.",
		InitialConfig: jvmAddtnlOptionsJson,
		DC:            dc,
		Expected:      jvmAddtnlOptionsJson,
	}
	test.run(t)
	assert.NotContains(t, test.Actual["cassandra-env-sh"].(map[string]interface{})["additional-jvm-opts"], "-javaagent:/opt/cdc_agent/cdc-agent.jar=pulsarServiceUrl=pulsar://pulsar:6650,topicPrefix=test-prefix-")
	assert.Contains(t, test.Actual["cassandra-env-sh"].(map[string]interface{})["additional-jvm-opts"], "additional-option2")
}
