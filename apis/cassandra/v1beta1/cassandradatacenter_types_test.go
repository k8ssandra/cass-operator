package v1beta1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

var internodeEnabledAll = `
{
	"cassandra-yaml": {
	  "server_encryption_options": {
		"internode_encryption": "all",
        "keystore": "/etc/encryption/node-keystore.jks",
        "keystore_password": "dc2",
        "truststore": "/etc/encryption/node-keystore.jks",
        "truststore_password": "dc2"
	  }
	}
}
`

var internodeSomethingElse = `
{
	"cassandra-yaml": {
	  "server_encryption_options": {
		"internode_encryption": "all",
        "keystore": "/etc/encryption/cert-manager.jks",
        "keystore_password": "aaaaaa",
        "truststore": "/etc/encryption/cert-manager.jks",
        "truststore_password": "bbbbb"
	  }
	}
}
`

func TestLegacyInternodeEnabled(t *testing.T) {
	dc := CassandraDatacenter{
		Spec: CassandraDatacenterSpec{
			Config: json.RawMessage(internodeEnabledAll),
		},
	}

	assert.True(t, dc.LegacyInternodeEnabled())
}

func TestLegacyInternodeDisabled(t *testing.T) {
	dc := CassandraDatacenter{
		Spec: CassandraDatacenterSpec{
			Config: json.RawMessage(internodeSomethingElse),
		},
	}

	assert.False(t, dc.LegacyInternodeEnabled())
}

func TestUseClientImage(t *testing.T) {
	assert := assert.New(t)

	tests := []struct {
		serverType string
		version    string
		should     bool
	}{
		{
			serverType: "cassandra",
			version:    "4.1.0",
			should:     true,
		},
		{
			serverType: "cassandra",
			version:    "4.1.2",
			should:     true,
		},
		{
			serverType: "cassandra",
			version:    "5.0.0",
			should:     true,
		},
		{
			serverType: "cassandra",
			version:    "3.11.17",
			should:     false,
		},
		{
			serverType: "cassandra",
			version:    "4.0.8",
			should:     false,
		},
		{
			serverType: "dse",
			version:    "6.8.39",
			should:     false,
		},
		{
			serverType: "dse",
			version:    "4.1.2",
			should:     false,
		},
	}

	for _, tt := range tests {
		dc := CassandraDatacenter{
			Spec: CassandraDatacenterSpec{
				ServerVersion: tt.version,
				ServerType:    tt.serverType,
			},
		}

		if tt.should {
			assert.True(dc.UseClientImage())
		} else {
			assert.False(dc.UseClientImage())
		}
	}
}
