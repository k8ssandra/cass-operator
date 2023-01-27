// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"fmt"
	"github.com/k8ssandra/cass-operator/pkg/oplabels"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
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
		oplabels.CreatedByLabel: oplabels.CreatedByLabelValue,
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

func TestLabelsWithNewSeedServiceForCassandraDatacenter(t *testing.T) {
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "piclem",
			ServerVersion: "4.0.1",
			AdditionalServiceConfig: api.ServiceConfig{
				DatacenterService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"DatacenterService": "add",
					},
				},
				SeedService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"SeedService": "add",
					},
				},
				AllPodsService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"AllPodsService": "add",
					},
				},
				AdditionalSeedService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"AdditionalSeedService": "add",
					},
				},
				NodePortService: api.ServiceConfigAdditions{
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
		oplabels.CreatedByLabel: oplabels.CreatedByLabelValue,
		oplabels.VersionLabel:   "4.0.1",
		api.ClusterLabel:        "piclem",
		"Add":                   "label",
		"SeedService":           "add",
	}

	service := newSeedServiceForCassandraDatacenter(dc)

	if !reflect.DeepEqual(expected, service.Labels) {
		t.Errorf("service labels = \n %v \n, want \n %v", service.Labels, expected)
	}
}

func TestLabelsWithNewNodePortServiceForCassandraDatacenter(t *testing.T) {
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "piclem",
			ServerVersion: "4.0.1",
			AdditionalServiceConfig: api.ServiceConfig{
				DatacenterService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"DatacenterService": "add",
					},
				},
				SeedService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"SeedService": "add",
					},
				},
				AllPodsService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"AllPodsService": "add",
					},
				},
				AdditionalSeedService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"AdditionalSeedService": "add",
					},
				},
				NodePortService: api.ServiceConfigAdditions{
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
		oplabels.CreatedByLabel: oplabels.CreatedByLabelValue,
		oplabels.VersionLabel:   "4.0.1",
		api.DatacenterLabel:     "dc1",
		api.ClusterLabel:        "piclem",
		"Add":                   "label",
		"NodePortService":       "add",
	}

	service := newNodePortServiceForCassandraDatacenter(dc)

	if !reflect.DeepEqual(expected, service.Labels) {
		t.Errorf("service labels = \n %v \n, want \n %v", service.Labels, expected)
	}
}

func TestLabelsWithNewAllPodsServiceForCassandraDatacenter(t *testing.T) {
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "piclem",
			ServerVersion: "4.0.1",
			AdditionalServiceConfig: api.ServiceConfig{
				DatacenterService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"DatacenterService": "add",
					},
				},
				SeedService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"SeedService": "add",
					},
				},
				AllPodsService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"AllPodsService": "add",
					},
				},
				AdditionalSeedService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"AdditionalSeedService": "add",
					},
				},
				NodePortService: api.ServiceConfigAdditions{
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
		oplabels.CreatedByLabel: oplabels.CreatedByLabelValue,
		oplabels.VersionLabel:   "4.0.1",
		api.DatacenterLabel:     "dc1",
		api.ClusterLabel:        "piclem",
		api.PromMetricsLabel:    "true",
		"Add":                   "label",
		"AllPodsService":        "add",
	}

	service := newAllPodsServiceForCassandraDatacenter(dc)

	if !reflect.DeepEqual(expected, service.Labels) {
		t.Errorf("service labels = \n %v \n, want \n %v", service.Labels, expected)
	}
}

func TestLabelsWithNewServiceForCassandraDatacenter(t *testing.T) {
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "piclem",
			ServerVersion: "4.0.1",
			AdditionalServiceConfig: api.ServiceConfig{
				DatacenterService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"DatacenterService": "add",
					},
				},
				SeedService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"SeedService": "add",
					},
				},
				AllPodsService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"AllPodsService": "add",
					},
				},
				AdditionalSeedService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"AdditionalSeedService": "add",
					},
				},
				NodePortService: api.ServiceConfigAdditions{
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
		oplabels.CreatedByLabel: oplabels.CreatedByLabelValue,
		oplabels.VersionLabel:   "4.0.1",
		api.DatacenterLabel:     "dc1",
		api.ClusterLabel:        "piclem",
		"Add":                   "label",
		"DatacenterService":     "add",
	}

	service := newServiceForCassandraDatacenter(dc)

	if !reflect.DeepEqual(expected, service.Labels) {
		t.Errorf("service labels = \n %v \n, want \n %v", service.Labels, expected)
	}
}

func TestLabelsWithNewAdditionalSeedServiceForCassandraDatacenter(t *testing.T) {
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "piclem",
			ServerVersion: "4.0.1",
			AdditionalServiceConfig: api.ServiceConfig{
				DatacenterService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"DatacenterService": "add",
					},
				},
				SeedService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"SeedService": "add",
					},
				},
				AllPodsService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"AllPodsService": "add",
					},
				},
				AdditionalSeedService: api.ServiceConfigAdditions{
					Labels: map[string]string{
						"AdditionalSeedService": "add",
					},
				},
				NodePortService: api.ServiceConfigAdditions{
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
		oplabels.CreatedByLabel: oplabels.CreatedByLabelValue,
		oplabels.VersionLabel:   "4.0.1",
		api.DatacenterLabel:     "dc1",
		api.ClusterLabel:        "piclem",
		"Add":                   "label",
		"AdditionalSeedService": "add",
	}

	service := newAdditionalSeedServiceForCassandraDatacenter(dc)

	if !reflect.DeepEqual(expected, service.Labels) {
		t.Errorf("service labels = \n %v \n, want \n %v", service.Labels, expected)
	}
}

func TestAddingAdditionalLabels(t *testing.T) {
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "piclem",
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
		oplabels.CreatedByLabel: oplabels.CreatedByLabelValue,
		oplabels.VersionLabel:   "4.0.1",
		api.DatacenterLabel:     "dc1",
		api.ClusterLabel:        "piclem",
		"Add":                   "label",
	}

	service := newServiceForCassandraDatacenter(dc)

	if !reflect.DeepEqual(expected, service.Labels) {
		t.Errorf("service labels = %v, want %v", service.Labels, expected)
	}
}

func TestServicePorts(t *testing.T) {
	tests := []struct {
		name                string
		dc                  *api.CassandraDatacenter
		dcServicePorts      []int32
		allPodsServicePorts []int32
	}{
		{
			name: "Cassandra 3.11.14",
			dc: &api.CassandraDatacenter{
				Spec: api.CassandraDatacenterSpec{
					ClusterName:   "bob",
					ServerType:    "cassandra",
					ServerVersion: "3.11.14",
				},
			},
			dcServicePorts:      []int32{8080, 9000, 9042, 9103, 9142, 9160},
			allPodsServicePorts: []int32{8080, 9000, 9042, 9103},
		},
		{
			name: "Cassandra 4.0.7",
			dc: &api.CassandraDatacenter{
				Spec: api.CassandraDatacenterSpec{
					ClusterName:   "bob",
					ServerType:    "cassandra",
					ServerVersion: "4.0.7",
				},
			},
			dcServicePorts:      []int32{8080, 9000, 9042, 9103, 9142},
			allPodsServicePorts: []int32{8080, 9000, 9042, 9103},
		},
		{
			name: "DSE 6.8.31",
			dc: &api.CassandraDatacenter{
				Spec: api.CassandraDatacenterSpec{
					ClusterName:   "bob",
					ServerType:    "dse",
					ServerVersion: "6.8.31",
				},
			},
			dcServicePorts:      []int32{8080, 9000, 9042, 9103, 9142, 9160},
			allPodsServicePorts: []int32{8080, 9000, 9042, 9103},
		},
		{
			name: "Cassandra 4.0.7 with custom ports",
			dc: &api.CassandraDatacenter{
				Spec: api.CassandraDatacenterSpec{
					ClusterName:   "bob",
					ServerType:    "cassandra",
					ServerVersion: "4.0.7",
					PodTemplateSpec: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "cassandra",
									Ports: []corev1.ContainerPort{
										{
											Name:          "metrics",
											ContainerPort: 9004,
										},
									},
								},
							},
						},
					},
				},
			},
			// FIXME: 9004 should be in the list of open ports
			dcServicePorts:      []int32{8080, 9000, 9042, 9103, 9142},
			allPodsServicePorts: []int32{8080, 9000, 9042, 9103},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var getServicePorts = func(svc *corev1.Service) []int32 {
				servicePorts := make([]int32, len(svc.Spec.Ports))
				for i := 0; i < len(svc.Spec.Ports); i++ {
					servicePorts[i] = svc.Spec.Ports[i].Port
				}
				return servicePorts
			}
			t.Run("dc service", func(t *testing.T) {
				svc := newServiceForCassandraDatacenter(test.dc)
				servicePorts := getServicePorts(svc)
				assert.ElementsMatch(t, servicePorts, test.dcServicePorts)
			})
			t.Run("all pods service", func(t *testing.T) {
				svc := newAllPodsServiceForCassandraDatacenter(test.dc)
				servicePorts := getServicePorts(svc)
				assert.ElementsMatch(t, servicePorts, test.allPodsServicePorts)
			})
		})
	}
}
