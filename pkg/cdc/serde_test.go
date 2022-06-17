package cdc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

var serdeTestConfig = `
{
	"cassandra-env-sh":{
		"initial_heap_size": "800M",
		"additional-jvm-opts":[
			"-Ddse.system_distributed_replication_dc_names=dc1"
		]
	},
	"cassandra-yaml": {
	  "authenticator": "PasswordAuthenticator"
	},
	"unknownfields": {
		"unknown1": true
	}
}
`

// Tests that Unmarhsalling works as expected.
func Test_UnmarshallConfig(t *testing.T) {
	c := configData{}
	err := json.Unmarshal([]byte(serdeTestConfig), &c)
	assert.NoError(t, err, err)
	assert.NotNil(t, c.CassandraYaml)
	assert.Contains(t, c.CassandraYaml, "authenticator")
	assert.NotNil(t, c.CassEnvSh)
	assert.NotNil(t, c.CassEnvSh.AddtnlJVMOptions)
	assert.Contains(t, *c.CassEnvSh.AddtnlJVMOptions, "-Ddse.system_distributed_replication_dc_names=dc1")
	assert.Contains(t, c.UnknownFields["unknownfields"], "unknown1")
}

// Tests that Marshalling works as expected.
func Test_MarshallConfig(t *testing.T) {
	c := configData{
		CassEnvSh: &cassEnvSh{
			AddtnlJVMOptions: &[]string{"-Ddse.system_distributed_replication_dc_names=dc1"},
			UnknownFields: map[string]interface{}{
				"initial_heap_size": "800M",
			},
		},
		CassandraYaml: map[string]interface{}{
			"authenticator": "PasswordAuthenticator",
		},
		UnknownFields: map[string]interface{}{
			"unknownfields": map[string]interface{}{
				"unknown1": true,
			},
		},
	}
	jsonConfig, err := json.Marshal(&c)
	assert.NoError(t, err, err)
	// Have to marshall everything back to a map[string]interface{} to avoid issues with line breaks etc.
	actual := make(map[string]interface{})
	err = json.Unmarshal(jsonConfig, &actual)
	assert.NoError(t, err, err)

	expected := make(map[string]interface{})
	err = json.Unmarshal([]byte(serdeTestConfig), &expected)
	assert.NoError(t, err, err)
	assert.Equal(t, expected, actual)
}
