// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/k8ssandra/cass-operator/pkg/oplabels"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
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

	gotLabels := service.Labels
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
			AdditionalAnnotations: map[string]string{
				"Add": "annotation",
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
	if !reflect.DeepEqual(expected, service.Labels) {
		t.Errorf("service labels = \n %v \n, want \n %v", service.Annotations, map[string]string{
			"Add": "annotation",
		})
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
			AdditionalAnnotations: map[string]string{
				"Add": "annotation",
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
	if !reflect.DeepEqual(expected, service.Labels) {
		t.Errorf("service labels = \n %v \n, want \n %v", service.Annotations, map[string]string{
			"Add": "annotation",
		})
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
			AdditionalAnnotations: map[string]string{
				"Add": "annotation",
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
	if !reflect.DeepEqual(expected, service.Labels) {
		t.Errorf("service labels = \n %v \n, want \n %v", service.Annotations, map[string]string{
			"Add": "annotation",
		})
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
			AdditionalAnnotations: map[string]string{
				"Add": "annotation",
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
	if !reflect.DeepEqual(expected, service.Labels) {
		t.Errorf("service labels = \n %v \n, want \n %v", service.Annotations, map[string]string{
			"Add": "annotation",
		})
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
			AdditionalAnnotations: map[string]string{
				"Add": "annotation",
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
	if !reflect.DeepEqual(expected, service.Labels) {
		t.Errorf("service labels = \n %v \n, want \n %v", service.Annotations, map[string]string{
			"Add": "annotation",
		})
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

func TestAddingAdditionalAnnotations(t *testing.T) {
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "piclem",
			ServerVersion: "4.0.1",
			AdditionalAnnotations: map[string]string{
				"Add": "annotation",
			},
		},
	}

	service := newServiceForCassandraDatacenter(dc)

	assert.Contains(t, service.Annotations, "Add")
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
			getServicePorts := func(svc *corev1.Service) []int32 {
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

func TestNewEndpointSlicesForAdditionalSeeds(t *testing.T) {
	expectedTypes := []discoveryv1.AddressType{discoveryv1.AddressTypeIPv4, discoveryv1.AddressTypeIPv6, discoveryv1.AddressTypeFQDN}

	testCases := []struct {
		name            string
		additionalSeeds []string
		expectedCounts  map[discoveryv1.AddressType]int
	}{
		{
			name:            "IPv4 addresses only",
			additionalSeeds: []string{"192.168.1.1", "10.0.0.1", "172.16.0.1"},
			expectedCounts: map[discoveryv1.AddressType]int{
				discoveryv1.AddressTypeIPv4: 3,
			},
		},
		{
			name:            "IPv6 addresses only",
			additionalSeeds: []string{"2001:db8::1", "2001:db8:1::1"},
			expectedCounts: map[discoveryv1.AddressType]int{
				discoveryv1.AddressTypeIPv6: 2,
			},
		},
		{
			name:            "FQDN addresses only",
			additionalSeeds: []string{"seed1.example.com", "seed2.example.com"},
			expectedCounts: map[discoveryv1.AddressType]int{
				discoveryv1.AddressTypeFQDN: 2,
			},
		},
		{
			name:            "Mixed address types",
			additionalSeeds: []string{"192.168.1.1", "2001:db8::1", "seed1.example.com"},
			expectedCounts: map[discoveryv1.AddressType]int{
				discoveryv1.AddressTypeIPv4: 1,
				discoveryv1.AddressTypeIPv6: 1,
				discoveryv1.AddressTypeFQDN: 1,
			},
		},
		{
			name:            "Empty additional seeds",
			additionalSeeds: []string{},
			expectedCounts:  map[discoveryv1.AddressType]int{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dc := &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dc1",
					Namespace: "test",
				},
				Spec: api.CassandraDatacenterSpec{
					ClusterName:     "test-cluster",
					AdditionalSeeds: tc.additionalSeeds,
				},
			}

			slices := newEndpointSlicesForAdditionalSeeds(dc)
			assert.Equal(t, len(expectedTypes), len(slices), "Number of slices should match expected types")

			// Check that we have the expected types of slices
			foundTypes := make([]discoveryv1.AddressType, 0, len(slices))
			for _, slice := range slices {
				foundTypes = append(foundTypes, slice.AddressType)

				// Check the count of addresses for each type
				if count, exists := tc.expectedCounts[slice.AddressType]; exists {
					assert.Equal(t, 1, len(slice.Endpoints), "Should have one endpoint")
					if len(slice.Endpoints) > 0 {
						assert.Equal(t, count, len(slice.Endpoints[0].Addresses),
							"Address count mismatch for type %s", slice.AddressType)
					}
				}

				// Check service name label
				assert.Equal(t, dc.GetAdditionalSeedsServiceName(),
					slice.Labels[discoveryv1.LabelServiceName],
					"Service name label should match")

				// Check the name follows the pattern
				expectedName := fmt.Sprintf("%s-%s",
					dc.GetAdditionalSeedsServiceName(),
					strings.ToLower(string(slice.AddressType)))
				assert.Equal(t, expectedName, slice.Name, "Slice name should follow the pattern")

				// Check namespace
				assert.Equal(t, dc.Namespace, slice.Namespace, "Namespace should match")
			}

			// Check that we have all expected types (order-independent)
			assert.ElementsMatch(t, expectedTypes, foundTypes, "Address types should match expected")
		})
	}
}

func TestCreateEndpointSlice(t *testing.T) {
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dc1",
			Namespace: "test",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName: "test-cluster",
		},
	}

	addresses := []string{"192.168.1.1", "192.168.1.2"}
	slice := CreateEndpointSlice(dc, dc.GetAdditionalSeedsServiceName(), discoveryv1.AddressTypeIPv4, addresses)

	assert.Equal(t, discoveryv1.AddressTypeIPv4, slice.AddressType)
	assert.Equal(t, "test", slice.Namespace)
	assert.Equal(t, fmt.Sprintf("%s-ipv4", dc.GetAdditionalSeedsServiceName()), slice.Name)
	assert.Equal(t, 1, len(slice.Endpoints))
	assert.Equal(t, addresses, slice.Endpoints[0].Addresses)
	assert.Equal(t, dc.GetAdditionalSeedsServiceName(), slice.Labels[discoveryv1.LabelServiceName])

	for k, v := range dc.GetDatacenterLabels() {
		assert.Equal(t, v, slice.Labels[k])
	}
}

func TestEndpointSlicesCorrectAddressSlice(t *testing.T) {
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dc1",
			Namespace: "test",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName: "test-cluster",
			AdditionalSeeds: []string{
				"192.168.1.1",       // IPv4
				"2001:db8::1",       // IPv6
				"seed1.example.com", // FQDN
			},
		},
	}

	assert := assert.New(t)
	endpointSlices := newEndpointSlicesForAdditionalSeeds(dc)
	assert.Equal(3, len(endpointSlices), "Should create 3 EndpointSlices for 3 different address types")

	addressTypeCounts := map[discoveryv1.AddressType]int{
		discoveryv1.AddressTypeIPv4: 0,
		discoveryv1.AddressTypeIPv6: 0,
		discoveryv1.AddressTypeFQDN: 0,
	}

	for _, slice := range endpointSlices {
		switch slice.AddressType {
		case discoveryv1.AddressTypeIPv4:
			assert.Equal(1, len(slice.Endpoints[0].Addresses))
			assert.Equal("192.168.1.1", slice.Endpoints[0].Addresses[0])
			addressTypeCounts[discoveryv1.AddressTypeIPv4]++
		case discoveryv1.AddressTypeIPv6:
			assert.Equal(1, len(slice.Endpoints[0].Addresses))
			assert.Equal("2001:db8::1", slice.Endpoints[0].Addresses[0])
			addressTypeCounts[discoveryv1.AddressTypeIPv6]++
		case discoveryv1.AddressTypeFQDN:
			assert.Equal(1, len(slice.Endpoints[0].Addresses))
			assert.Equal("seed1.example.com", slice.Endpoints[0].Addresses[0])
			addressTypeCounts[discoveryv1.AddressTypeFQDN]++
		}
	}

	assert.Equal(1, addressTypeCounts[discoveryv1.AddressTypeIPv4])
	assert.Equal(1, addressTypeCounts[discoveryv1.AddressTypeIPv6])
	assert.Equal(1, addressTypeCounts[discoveryv1.AddressTypeFQDN])
}
