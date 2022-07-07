package v1beta1

import (
	"encoding/json"
	"github.com/Jeffail/gabs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestGetConfigAsJSON(t *testing.T) {
	type test struct {
		name string
		dc   *CassandraDatacenter
		want string
	}

	tests := []test{
		{
			name: "custom DC name",
			dc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "dc1",
				},
				Spec: CassandraDatacenterSpec{
					Size:          1,
					ServerType:    "cassandra",
					ServerVersion: "4.0.4",
					ClusterName:   "test",
					Config:        json.RawMessage(`{"datacenter-info": {"name": "dev1"}}`),
				},
			},
			want: `{
              "cassandra-yaml": {},
              "cluster-info": {
                "name": "test",
                "seeds": "test-seed-service,test-dc1-additional-seed-service"
              },
              "datacenter-info": {
                "name": "dev1",
                "graph-enabled": 0,
                "solr-enabled": 0,
                "spark-enabled": 0
              }
            }`,
		},
		{
			name: "default DC name",
			dc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "dc1",
				},
				Spec: CassandraDatacenterSpec{
					Size:          1,
					ServerType:    "cassandra",
					ServerVersion: "4.0.4",
					ClusterName:   "test",
				},
			},
			want: `{
              "cassandra-yaml": {},
              "cluster-info": {
                "name": "test",
                "seeds": "test-seed-service,test-dc1-additional-seed-service"
              },
              "datacenter-info": {
                "name": "dc1",
                "graph-enabled": 0,
                "solr-enabled": 0,
                "spark-enabled": 0
              }
            }`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.dc.GetConfigAsJSON(tc.dc.Spec.Config)
			require.NoError(t, err, "failed to parse CassandraDatacenter config into JSON")
			expected, err := gabs.ParseJSON([]byte(tc.want))
			require.NoError(t, err, "failed to parse tc.want into gabs.Container")
			actual, err := gabs.ParseJSON([]byte(got))
			require.NoError(t, err, "failed to parse got into gabs.Container")
			assert.Equal(t, expected, actual)
		})
	}
}
