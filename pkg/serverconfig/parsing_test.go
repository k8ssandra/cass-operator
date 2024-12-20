package serverconfig

import (
	"encoding/json"
	"testing"

	"github.com/Jeffail/gabs/v2"
	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			Config: json.RawMessage(internodeEnabledAll),
		},
	}

	assert.True(t, LegacyInternodeEnabled(dc))
}

func TestLegacyInternodeDisabled(t *testing.T) {
	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			Config: json.RawMessage(internodeSomethingElse),
		},
	}

	assert.False(t, LegacyInternodeEnabled(dc))
}

func TestDatacenterNoOverrideConfig(t *testing.T) {
	assert := assert.New(t)
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName: "cluster1",
		},
	}

	config, err := GetConfigAsJSON(dc, dc.Spec.Config)
	assert.NoError(err)

	container, err := gabs.ParseJSON([]byte(config))
	assert.NoError(err)

	dataCenterInfo := container.ChildrenMap()["datacenter-info"]
	assert.NotEmpty(dataCenterInfo)
	assert.Equal(dc.Name, dataCenterInfo.ChildrenMap()["name"].Data().(string))
	assert.Equal(dc.DatacenterName(), dc.Name)
}

func TestDatacenterOverrideInConfig(t *testing.T) {
	assert := assert.New(t)
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName:    "cluster1",
			DatacenterName: "Home_Dc",
		},
	}

	config, err := GetConfigAsJSON(dc, dc.Spec.Config)
	assert.NoError(err)

	container, err := gabs.ParseJSON([]byte(config))
	assert.NoError(err)

	dataCenterInfo := container.ChildrenMap()["datacenter-info"]
	assert.NotEmpty(dataCenterInfo)
	assert.Equal(dc.Spec.DatacenterName, dataCenterInfo.ChildrenMap()["name"].Data().(string))
	assert.Equal(dc.DatacenterName(), dc.Spec.DatacenterName)
}
