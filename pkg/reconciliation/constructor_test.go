package reconciliation

import (
	"testing"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodDisruptionBudget(t *testing.T) {
	assert := assert.New(t)

	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dc1",
			Namespace: "test",
		},
		Spec: api.CassandraDatacenterSpec{
			DatacenterName: "dc1-override",
			Size:           3,
		},
	}

	// create a PodDisruptionBudget object
	pdb := newPodDisruptionBudgetForDatacenter(dc)
	assert.Equal("dc1-pdb", pdb.Name)
	assert.Equal("test", pdb.Namespace)
	assert.Equal("dc1", pdb.Spec.Selector.MatchLabels["cassandra.datastax.com/datacenter"])
	assert.Equal(pdb.Spec.MinAvailable.IntVal, int32(dc.Spec.Size-1))
}
