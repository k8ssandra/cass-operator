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

func TestLabelsWithNewSeedServiceForCassandraDatacenter(t *testing.T)  {
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName: "piclem",
			ServerVersion: "4.0.1",
			AdditionalServiceConfig: api.ServiceConfig{
				DatacenterService:     api.ServiceConfigAdditions{
					Labels: map[string]string{
						"DatacenterService": "add",
					},
				},
				SeedService:           api.ServiceConfigAdditions{
					Labels: map[string]string{
						"SeedService": "add",
					},
				},
				AllPodsService:        api.ServiceConfigAdditions{
					Labels: map[string]string{
						"AllPodsService": "add",
					},
				},
				AdditionalSeedService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"AdditionalSeedService": "add",
					},
				},
				NodePortService:       api.ServiceConfigAdditions{
					Labels: map[string]string{
						"NodePortService": "add",
					},
				},
			},
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
		api.ClusterLabel:        "piclem",
		"Add": "label",
		"SeedService": "add",
	}


	service := newSeedServiceForCassandraDatacenter(dc)

	if !reflect.DeepEqual(expected, service.Labels) {
		t.Errorf("service labels = \n %v \n, want \n %v", service.Labels, expected)
	}
}

func TestLabelsWithNewNodePortServiceForCassandraDatacenter(t *testing.T)  {
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName: "piclem",
			ServerVersion: "4.0.1",
			AdditionalServiceConfig: api.ServiceConfig{
				DatacenterService:     api.ServiceConfigAdditions{
					Labels: map[string]string{
						"DatacenterService": "add",
					},
				},
				SeedService:           api.ServiceConfigAdditions{
					Labels: map[string]string{
						"SeedService": "add",
					},
				},
				AllPodsService:        api.ServiceConfigAdditions{
					Labels: map[string]string{
						"AllPodsService": "add",
					},
				},
				AdditionalSeedService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"AdditionalSeedService": "add",
					},
				},
				NodePortService:       api.ServiceConfigAdditions{
					Labels: map[string]string{
						"NodePortService": "add",
					},
				},
			},
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
		"NodePortService": "add",
	}


	service := newNodePortServiceForCassandraDatacenter(dc)

	if !reflect.DeepEqual(expected, service.Labels) {
		t.Errorf("service labels = \n %v \n, want \n %v", service.Labels, expected)
	}
}

func TestLabelsWithNewAllPodsServiceForCassandraDatacenter(t *testing.T)  {
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName: "piclem",
			ServerVersion: "4.0.1",
			AdditionalServiceConfig: api.ServiceConfig{
				DatacenterService:     api.ServiceConfigAdditions{
					Labels: map[string]string{
						"DatacenterService": "add",
					},
				},
				SeedService:           api.ServiceConfigAdditions{
					Labels: map[string]string{
						"SeedService": "add",
					},
				},
				AllPodsService:        api.ServiceConfigAdditions{
					Labels: map[string]string{
						"AllPodsService": "add",
					},
				},
				AdditionalSeedService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"AdditionalSeedService": "add",
					},
				},
				NodePortService:       api.ServiceConfigAdditions{
					Labels: map[string]string{
						"NodePortService": "add",
					},
				},
			},
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
		api.PromMetricsLabel: "true",
		"Add": "label",
		"AllPodsService": "add",
	}


	service := newAllPodsServiceForCassandraDatacenter(dc)

	if !reflect.DeepEqual(expected, service.Labels) {
		t.Errorf("service labels = \n %v \n, want \n %v", service.Labels, expected)
	}
}

func TestLabelsWithNewServiceForCassandraDatacenter(t *testing.T)  {
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName: "piclem",
			ServerVersion: "4.0.1",
			AdditionalServiceConfig: api.ServiceConfig{
				DatacenterService:     api.ServiceConfigAdditions{
					Labels: map[string]string{
						"DatacenterService": "add",
					},
				},
				SeedService:           api.ServiceConfigAdditions{
					Labels: map[string]string{
						"SeedService": "add",
					},
				},
				AllPodsService:        api.ServiceConfigAdditions{
					Labels: map[string]string{
						"AllPodsService": "add",
					},
				},
				AdditionalSeedService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"AdditionalSeedService": "add",
					},
				},
				NodePortService:       api.ServiceConfigAdditions{
					Labels: map[string]string{
						"NodePortService": "add",
					},
				},
			},
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
		"DatacenterService": "add",
	}


	service := newServiceForCassandraDatacenter(dc)

	if !reflect.DeepEqual(expected, service.Labels) {
		t.Errorf("service labels = \n %v \n, want \n %v", service.Labels, expected)
	}
}

func TestLabelsWithNewAdditionalSeedServiceForCassandraDatacenter(t *testing.T)  {
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName: "piclem",
			ServerVersion: "4.0.1",
			AdditionalServiceConfig: api.ServiceConfig{
				DatacenterService:     api.ServiceConfigAdditions{
					Labels: map[string]string{
						"DatacenterService": "add",
					},
				},
				SeedService:           api.ServiceConfigAdditions{
					Labels: map[string]string{
						"SeedService": "add",
					},
				},
				AllPodsService:        api.ServiceConfigAdditions{
					Labels: map[string]string{
						"AllPodsService": "add",
					},
				},
				AdditionalSeedService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"AdditionalSeedService": "add",
					},
				},
				NodePortService:       api.ServiceConfigAdditions{
					Labels: map[string]string{
						"NodePortService": "add",
					},
				},
			},
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
		"AdditionalSeedService": "add",
	}


	service := newAdditionalSeedServiceForCassandraDatacenter(dc)

	if !reflect.DeepEqual(expected, service.Labels) {
		t.Errorf("service labels = \n %v \n, want \n %v", service.Labels, expected)
	}
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
