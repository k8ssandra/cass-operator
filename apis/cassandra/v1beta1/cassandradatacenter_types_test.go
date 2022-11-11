package v1beta1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

/*
   cassandra-yaml:
     server_encryption_options:
       internode_encryption: all
       keystore: /etc/encryption/node-keystore.jks
       keystore_password: dc2
       truststore: /etc/encryption/node-keystore.jks
       truststore_password: dc2
*/

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
