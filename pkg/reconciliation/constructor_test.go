package reconciliation

import (
	"testing"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	assert.Equal(pdb.Spec.MinAvailable.IntVal, dc.Spec.Size-1)
}

func TestPodDisruptionBudgetIntMaxUnavailable(t *testing.T) {
	assert := assert.New(t)

	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dc1",
			Namespace: "test",
		},
		Spec: api.CassandraDatacenterSpec{
			Size:           6,
			MaxUnavailable: new(intstr.FromInt(2)),
		},
	}

	pdb := newPodDisruptionBudgetForDatacenter(dc)
	assert.Equal(int32(4), pdb.Spec.MinAvailable.IntVal)
}

func TestPodDisruptionBudgetPercentageMaxUnavailable(t *testing.T) {
	assert := assert.New(t)

	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dc1",
			Namespace: "test",
		},
		Spec: api.CassandraDatacenterSpec{
			Size: 6,
			Racks: []api.Rack{
				{Name: "rack1"},
				{Name: "rack2"},
			},
			MaxUnavailable: new(intstr.Parse("50%")),
		},
	}

	pdb := newPodDisruptionBudgetForDatacenter(dc)
	assert.Equal(int32(4), pdb.Spec.MinAvailable.IntVal) // This was roundup

	dc.Spec.MaxUnavailable = new(intstr.Parse("100%"))
	pdb = newPodDisruptionBudgetForDatacenter(dc)
	assert.Equal(int32(3), pdb.Spec.MinAvailable.IntVal)
}
