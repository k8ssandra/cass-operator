// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/k8ssandra/cass-operator/pkg/oplabels"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
)

func TestCassandraDatacenter_buildLabelSelectorForSeedService(t *testing.T) {
	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName: "bob",
		},
	}
	want := map[string]string{
		api.ClusterLabel:  "bob",
		api.SeedNodeLabel: "true",
	}

	got := buildLabelSelectorForSeedService(dc)

	if !reflect.DeepEqual(want, got) {
		t.Errorf("buildLabelSelectorForSeedService = %v, want %v", got, want)
	}
}

func TestCassandraDatacenter_allPodsServiceLabels(t *testing.T) {
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "bob",
			ServerVersion: "4.0.1",
		},
	}
	wantLabels := map[string]string{
		oplabels.ManagedByLabel: oplabels.ManagedByLabelValue,
		oplabels.InstanceLabel:  fmt.Sprintf("%s-%s", oplabels.NameLabelValue, dc.Spec.ClusterName),
		oplabels.NameLabel:      oplabels.NameLabelValue,
		oplabels.VersionLabel:   "4.0.1",
		api.ClusterLabel:        "bob",
		api.DatacenterLabel:     "dc1",
		api.PromMetricsLabel:    "true",
	}

	service := newAllPodsServiceForCassandraDatacenter(dc)

	gotLabels := service.ObjectMeta.Labels
	if !reflect.DeepEqual(wantLabels, gotLabels) {
		t.Errorf("allPodsService labels = %v, want %v", gotLabels, wantLabels)
	}
}

func TestServiceNameGeneration(t *testing.T) {
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName: "NotCool_Bob",
		},
	}

	service := newSeedServiceForCassandraDatacenter(dc)
	assert.Equal(t, "notcool-bob-seed-service", service.Name)
}

func TestAddingAdditionalLabels(t *testing.T)  {
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName: "piclem",
			ServerVersion: "4.0.1",
			AdditionalLabels: map[string]string{
				"Add": "label",
			},
		},
	}

	expected := map[string]string{
		oplabels.ManagedByLabel: oplabels.ManagedByLabelValue,
		oplabels.InstanceLabel:  fmt.Sprintf("%s-%s", oplabels.NameLabelValue, dc.Spec.ClusterName),
		oplabels.NameLabel:      oplabels.NameLabelValue,
		oplabels.VersionLabel:   "4.0.1",
		api.DatacenterLabel:     "dc1",
		api.ClusterLabel:        "piclem",
		"Add": "label",
	}


	service := newServiceForCassandraDatacenter(dc)

	if !reflect.DeepEqual(expected, service.Labels) {
		t.Errorf("service labels = %v, want %v", service.Labels, expected)
	}


}
