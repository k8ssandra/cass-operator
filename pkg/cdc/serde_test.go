package cdc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

var serdeTestConfig = `
{
	"jvm-options":{
		"initial_heap_size": "800M",
		"additional-jvm-opts":[
			"-Ddse.system_distributed_replication_dc_names=dc1"
		]
	},
	"cassandra-yaml": {
	  "authenticator": "PasswordAuthenticator"
	}
}
`

// Tests that Unmarhsalling works as expected.
func Test_UnmarshallConfig(t *testing.T) {
	c := configData{}
	err := json.Unmarshal([]byte(serdeTestConfig), &c)
	assert.NoError(t, err, err)
	assert.Contains(t, c.UnknownFields, "cassandra-yaml")
	assert.NotNil(t, c.JvmOptions)
	assert.NotNil(t, c.JvmOptions.AddtnlJVMOptions)
	assert.Contains(t, *c.JvmOptions.AddtnlJVMOptions, "-Ddse.system_distributed_replication_dc_names=dc1")
}

// Tests that Marshalling works as expected.
func Test_MarshallConfig(t *testing.T) {
	c := configData{
		JvmOptions: &jvmOptions{
			AddtnlJVMOptions: &[]string{"-Ddse.system_distributed_replication_dc_names=dc1"},
			UnknownFields: map[string]interface{}{
				"initial_heap_size": "800M",
			},
		},
		UnknownFields: map[string]interface{}{
			"cassandra-yaml": map[string]interface{}{
				"authenticator": "PasswordAuthenticator",
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
