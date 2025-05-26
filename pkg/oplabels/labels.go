// Copyright DataStax, Inc.
// Please see the included license file for details.

package oplabels

import (
	"fmt"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ManagedByLabel      = "app.kubernetes.io/managed-by"
	ManagedByLabelValue = "cass-operator"
	NameLabel           = "app.kubernetes.io/name"
	NameLabelValue      = "cassandra"
	InstanceLabel       = "app.kubernetes.io/instance"
	VersionLabel        = "app.kubernetes.io/version"
	CreatedByLabel      = "app.kubernetes.io/created-by"
	CreatedByLabelValue = ManagedByLabelValue
)

func AddOperatorMetadata(obj *metav1.ObjectMeta, dc *api.CassandraDatacenter) {
	if obj.Labels == nil {
		obj.Labels = make(map[string]string)
	}
	if obj.Annotations == nil {
		obj.Annotations = make(map[string]string)
	}

	AddOperatorLabels(obj.Labels, dc)
	AddOperatorAnnotations(obj.Annotations, dc)
}

func AddOperatorLabels(m map[string]string, dc *api.CassandraDatacenter) {
	if m == nil {
		m = make(map[string]string)
	}
	m[ManagedByLabel] = ManagedByLabelValue
	m[NameLabel] = NameLabelValue
	m[VersionLabel] = dc.Spec.ServerVersion
	m[InstanceLabel] = fmt.Sprintf("cassandra-%s", api.CleanLabelValue(dc.Spec.ClusterName))
	m[CreatedByLabel] = CreatedByLabelValue

	if len(dc.Spec.AdditionalLabels) != 0 {
		for key, value := range dc.Spec.AdditionalLabels {
			m[key] = api.CleanLabelValue(value)
		}
	}

}

func AddOperatorAnnotations(m map[string]string, dc *api.CassandraDatacenter) {
	if m == nil {
		m = make(map[string]string)
	}
	if len(dc.Spec.AdditionalAnnotations) != 0 {
		for key, value := range dc.Spec.AdditionalAnnotations {
			m[key] = value
		}
	}
}

func HasManagedByCassandraOperatorLabel(m map[string]string) bool {
	v, ok := m[ManagedByLabel]
	return ok && v == ManagedByLabelValue
}
