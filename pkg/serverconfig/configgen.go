// Copyright DataStax, Inc.
// Please see the included license file for details.

package serverconfig

import (
	"encoding/json"
	"strings"

	"github.com/Jeffail/gabs/v2"
	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/pkg/errors"
)

// This needs to be outside of the apis package or else code-gen fails
type NodeConfig map[string]interface{}

// GetModelValues will gather the cluster model values for cluster and datacenter
func GetModelValues(
	seeds []string,
	clusterName string,
	dcName string,
	graphEnabled int,
	solrEnabled int,
	sparkEnabled int,
	nativePort int,
	nativeSSLPort int,
	internodePort int,
	internodeSSLPort int,
) NodeConfig {
	seedsString := strings.Join(seeds, ",")

	// Note: the operator does not currently support graph, solr, and spark
	modelValues := NodeConfig{
		"cluster-info": NodeConfig{
			"name":  clusterName,
			"seeds": seedsString,
		},
		"datacenter-info": NodeConfig{
			"name":          dcName,
			"graph-enabled": graphEnabled,
			"solr-enabled":  solrEnabled,
			"spark-enabled": sparkEnabled,
		},
		"cassandra-yaml": NodeConfig{},
	}

	if nativeSSLPort != 0 {
		modelValues["cassandra-yaml"].(NodeConfig)["native_transport_port_ssl"] = nativeSSLPort
	} else if nativePort != 0 {
		modelValues["cassandra-yaml"].(NodeConfig)["native_transport_port"] = nativePort
	}

	if internodeSSLPort != 0 {
		modelValues["cassandra-yaml"].(NodeConfig)["ssl_storage_port"] = internodeSSLPort
	} else if internodePort != 0 {
		modelValues["cassandra-yaml"].(NodeConfig)["storage_port"] = internodePort
	}

	return modelValues
}

// GetConfigAsJSON gets a JSON-encoded string suitable for passing to configBuilder
func GetConfigAsJSON(dc *api.CassandraDatacenter, config []byte) (string, error) {
	if config == nil {
		config = dc.Spec.Config
	}

	// We use the cluster seed-service name here for the seed list as it will
	// resolve to the seed nodes. This obviates the need to update the
	// cassandra.yaml whenever the seed nodes change.
	seeds := []string{dc.GetSeedServiceName(), dc.GetAdditionalSeedsServiceName()}

	graphEnabled := 0
	solrEnabled := 0
	sparkEnabled := 0

	if dc.Spec.ServerType == "dse" && dc.Spec.DseWorkloads != nil {
		if dc.Spec.DseWorkloads.AnalyticsEnabled {
			sparkEnabled = 1
		}
		if dc.Spec.DseWorkloads.GraphEnabled {
			graphEnabled = 1
		}
		if dc.Spec.DseWorkloads.SearchEnabled {
			solrEnabled = 1
		}
	}

	native := 0
	nativeSSL := 0
	internode := 0
	internodeSSL := 0
	if dc.IsNodePortEnabled() {
		native = dc.Spec.Networking.NodePort.Native
		nativeSSL = dc.Spec.Networking.NodePort.NativeSSL
		internode = dc.Spec.Networking.NodePort.Internode
		internodeSSL = dc.Spec.Networking.NodePort.InternodeSSL
	}

	modelValues := GetModelValues(
		seeds,
		dc.Spec.ClusterName,
		dc.DatacenterName(),
		graphEnabled,
		solrEnabled,
		sparkEnabled,
		native,
		nativeSSL,
		internode,
		internodeSSL)

	var modelBytes []byte

	modelBytes, err := json.Marshal(modelValues)
	if err != nil {
		return "", err
	}

	// Combine the model values with the user-specified values
	modelParsed, err := gabs.ParseJSON(modelBytes)
	if err != nil {
		return "", errors.Wrap(err, "Model information for CassandraDatacenter resource was not properly configured")
	}

	if config != nil {
		configParsed, err := gabs.ParseJSON(config)
		if err != nil {
			return "", errors.Wrap(err, "Error parsing Spec.Config for CassandraDatacenter resource")
		}

		if err := modelParsed.Merge(configParsed); err != nil {
			return "", errors.Wrap(err, "Error merging Spec.Config for CassandraDatacenter resource")
		}
	}

	return modelParsed.String(), nil
}
