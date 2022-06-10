// Copyright DataStax, Inc.
// Please see the included license file for details.

package oplabels

import (
	"fmt"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
)

const (
	ManagedByLabel             = "app.kubernetes.io/managed-by"
	ManagedByLabelValue        = "cass-operator"
	ManagedByLabelDefunctValue = "cassandra-operator"
	NameLabel                  = "app.kubernetes.io/name"
	NameLabelValue             = "cassandra"
	InstanceLabel              = "app.kubernetes.io/instance"
	VersionLabel               = "app.kubernetes.io/version"
	CreatedByLabel             = "app.kubernetes.io/created-by"
)

func AddOperatorLabels(m map[string]string, dc *api.CassandraDatacenter) {
	m[ManagedByLabel] = ManagedByLabelValue
	m[NameLabel] = NameLabelValue
	m[VersionLabel] = dc.Spec.ServerVersion

	instanceName := fmt.Sprintf("cassandra-%s", dc.Spec.ClusterName)
	m[InstanceLabel] = instanceName

	if len(dc.Spec.AdditionalLabels) != 0 {
		for key, value := range dc.Spec.AdditionalLabels {
			m[key] = value
		}
	}
}

func AddDefunctManagedByLabel(m map[string]string) {
	m[ManagedByLabel] = ManagedByLabelDefunctValue
}

func HasManagedByCassandraOperatorLabel(m map[string]string) bool {
	v, ok := m[ManagedByLabel]
	return ok && v == ManagedByLabelValue
}
