package test

import (
	cassdcapi "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetCassandraDatacenter gets a minimal CassandraDatacenter object.
func GetCassandraDatacenter(name string, namespace string) cassdcapi.CassandraDatacenter {
	return cassdcapi.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cassdcapi.CassandraDatacenterSpec{
			Size:          1,
			ServerVersion: "4.0.1",
			ServerType:    "cassandra",
		},
	}
}
