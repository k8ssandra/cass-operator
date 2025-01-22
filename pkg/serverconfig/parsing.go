package serverconfig

import (
	"strings"

	"github.com/Jeffail/gabs/v2"
	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
)

func LegacyInternodeEnabled(dc *api.CassandraDatacenter) bool {
	config, err := gabs.ParseJSON(dc.Spec.Config)
	if err != nil {
		return false
	}

	hasOldKeyStore := func(gobContainer map[string]*gabs.Container) bool {
		if gobContainer == nil {
			return false
		}

		if keystorePath, found := gobContainer["keystore"]; found {
			if strings.TrimSpace(keystorePath.Data().(string)) == "/etc/encryption/node-keystore.jks" {
				return true
			}
		}
		return false
	}

	if config.Exists("cassandra-yaml", "client_encryption_options") || config.Exists("cassandra-yaml", "server_encryption_options") {
		serverContainer := config.Path("cassandra-yaml.server_encryption_options").ChildrenMap()
		clientContainer := config.Path("cassandra-yaml.client_encryption_options").ChildrenMap()

		if hasOldKeyStore(clientContainer) || hasOldKeyStore(serverContainer) {
			return true
		}
	}

	return false
}
