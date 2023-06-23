// Copyright DataStax, Inc.
// Please see the included license file for details.

package v1beta1

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_ValidateSingleDatacenter(t *testing.T) {
	tests := []struct {
		name      string
		dc        *CassandraDatacenter
		errString string
	}{
		{
			name: "Dse Valid",
			dc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ServerType:    "dse",
					ServerVersion: "6.8.0",
				},
			},
			errString: "",
		},
		{
			name: "Dse 6.8.4 Valid",
			dc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ServerType:    "dse",
					ServerVersion: "6.8.4",
				},
			},
			errString: "",
		},
		{
			name: "Dse 7.0.0 Valid",
			dc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ServerType:    "dse",
					ServerVersion: "7.0.0",
				},
			},
			errString: "",
		},
		{
			name: "Dse Invalid",
			dc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ServerType:    "dse",
					ServerVersion: "4.8.0",
				},
			},
			errString: "use unsupported DSE version '4.8.0'",
		},
		{
			name: "Dse 5 Invalid",
			dc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ServerType:    "dse",
					ServerVersion: "5.0.0",
				},
			},
			errString: "use unsupported DSE version '5.0.0'",
		},
		{
			name: "Cassandra valid",
			dc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ServerType:    "cassandra",
					ServerVersion: "3.11.7",
				},
			},
			errString: "",
		},
		{
			name: "Cassandra 4.0.x must be valid",
			dc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ServerType:    "cassandra",
					ServerVersion: "4.0.3",
				},
			},
			errString: "",
		},
		{
			name: "Cassandra 4.1 must be valid",
			dc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ServerType:    "cassandra",
					ServerVersion: "4.1.0",
				},
			},
			errString: "",
		},
		{
			name: "Cassandra 5.0.0 must be valid",
			dc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ServerType:    "cassandra",
					ServerVersion: "5.0.0",
				},
			},
			errString: "",
		},
		{
			name: "Cassandra Invalid",
			dc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ServerType:    "cassandra",
					ServerVersion: "6.8.0",
				},
			},
			errString: "use unsupported Cassandra version '6.8.0'",
		},
		{
			name: "Cassandra Invalid too",
			dc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ServerType:    "cassandra",
					ServerVersion: "7.0.0",
				},
			},
			errString: "use unsupported Cassandra version '7.0.0'",
		},
		{
			name: "Dse Workloads in Cassandra Invalid",
			dc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ServerType:    "cassandra",
					ServerVersion: "6.8.0",
					DseWorkloads: &DseWorkloads{
						AnalyticsEnabled: true,
					},
				},
			},
			errString: "CassandraDatacenter write rejected, attempted to enable DSE workloads if server type is Cassandra",
		},
		{
			name: "Dse Workloads in Dse valid",
			dc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ServerType:    "dse",
					ServerVersion: "6.8.4",
					DseWorkloads: &DseWorkloads{
						AnalyticsEnabled: true,
					},
				},
			},
			errString: "",
		},
		{
			name: "Cassandra 3.11 invalid config file dse-yaml",
			dc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ServerType:    "cassandra",
					ServerVersion: "3.11.7",
					Config: json.RawMessage(`
					{
						"cassandra-yaml": {},
						"dse-yaml": {
							"key1": "value1"
						}
					}
					`),
				},
			},
			errString: "attempted to define config dse-yaml with cassandra-3.11.7",
		},
		{
			name: "Cassandra 3.11 invalid config file jvm-server-options",
			dc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ServerType:    "cassandra",
					ServerVersion: "3.11.7",
					Config: json.RawMessage(`
					{
						"cassandra-yaml": {},
						"jvm-server-options": {
							"key1": "value1"
						}
					}
					`),
				},
			},
			errString: "attempted to define config jvm-server-options with cassandra-3.11.7",
		},
		{
			name: "DSE 6.8 invalid config file jvm-options",
			dc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ServerType:    "dse",
					ServerVersion: "6.8.4",
					Config: json.RawMessage(`
					{
						"cassandra-yaml": {},
						"jvm-options": {
							"key1": "value1"
						}
					}
					`),
				},
			},
			errString: "attempted to define config jvm-options with dse-6.8.4",
		},
		{
			name: "Allow multiple nodes per worker requires resource requests",
			dc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ServerType:                  "dse",
					ServerVersion:               "6.8.4",
					Config:                      json.RawMessage(`{}`),
					AllowMultipleNodesPerWorker: true,
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1000m"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1000m"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			},
			errString: "",
		},
		{
			name: "Allow multiple nodes per worker requires resource requests",
			dc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ServerType:                  "dse",
					ServerVersion:               "6.8.4",
					Config:                      json.RawMessage(`{}`),
					AllowMultipleNodesPerWorker: true,
				},
			},
			errString: "use multiple nodes per worker without cpu and memory requests and limits",
		},
		{
			name: "Prevent user specified reserved Service labels and annotations",
			dc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ServerType:    "cassandra",
					ServerVersion: "4.0.4",
					AdditionalServiceConfig: ServiceConfig{
						DatacenterService: ServiceConfigAdditions{
							Labels:      map[string]string{"k8ssandra.io/key1": "val1", "cassandra.datastax.com/key2": "val2"},
							Annotations: map[string]string{"k8ssandra.io/key3": "val3", "cassandra.datastax.com/key4": "val4"},
						},
					},
				},
			},
			errString: "configure DatacenterService with reserved annotations and/or labels (prefixes cassandra.datastax.com and/or k8ssandra.io)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSingleDatacenter(*tt.dc)
			if err == nil {
				if tt.errString != "" {
					t.Errorf("ValidateSingleDatacenter() err = %v, want %v", err, tt.errString)
				}
			} else {
				if tt.errString == "" {
					t.Errorf("ValidateSingleDatacenter() err = %v, should be valid", err)
				} else if !strings.HasSuffix(err.Error(), tt.errString) {
					t.Errorf("ValidateSingleDatacenter() err = %v, want suffix %v", err, tt.errString)
				}
			}
		})
	}
}

func Test_ValidateDatacenterFieldChanges(t *testing.T) {
	storageSize := resource.MustParse("1Gi")
	storageName := "server-data"

	tests := []struct {
		name      string
		oldDc     *CassandraDatacenter
		newDc     *CassandraDatacenter
		errString string
	}{
		{
			name: "No significant changes",
			oldDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ClusterName:                 "oldname",
					AllowMultipleNodesPerWorker: false,
					SuperuserSecretName:         "hush",
					DeprecatedServiceAccount:    "admin",
					StorageConfig: StorageConfig{
						CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: &storageName,
							AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
							Resources: corev1.ResourceRequirements{
								Requests: map[corev1.ResourceName]resource.Quantity{"storage": storageSize},
							},
						},
					},
					Racks: []Rack{{
						Name: "rack0",
					}, {
						Name: "rack1",
					}, {
						Name: "rack2",
					}},
				},
			},
			newDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ClusterName:                 "oldname",
					AllowMultipleNodesPerWorker: false,
					SuperuserSecretName:         "hush",
					DeprecatedServiceAccount:    "admin",
					StorageConfig: StorageConfig{
						CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: &storageName,
							AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
							Resources: corev1.ResourceRequirements{
								Requests: map[corev1.ResourceName]resource.Quantity{"storage": storageSize},
							},
						},
					},
					Racks: []Rack{{
						Name: "rack0",
					}, {
						Name: "rack1",
					}, {
						Name: "rack2",
					}},
				},
			},
			errString: "",
		},
		{
			name: "Clustername changed",
			oldDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ClusterName: "oldname",
				},
			},
			newDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					ClusterName: "newname",
				},
			},
			errString: "change clusterName",
		},
		{
			name: "DatacenterName changed",
			oldDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					DatacenterName: "oldname",
				},
			},
			newDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					DatacenterName: "newname",
				},
			},
			errString: "change datacenterName",
		},
		{
			name: "AllowMultipleNodesPerWorker changed",
			oldDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					AllowMultipleNodesPerWorker: false,
				},
			},
			newDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					AllowMultipleNodesPerWorker: true,
				},
			},
			errString: "change allowMultipleNodesPerWorker",
		},
		{
			name: "SuperuserSecretName changed",
			oldDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					SuperuserSecretName: "hush",
				},
			},
			newDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					SuperuserSecretName: "newsecret",
				},
			},
			errString: "change superuserSecretName",
		},
		{
			name: "ServiceAccount changed",
			oldDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					DeprecatedServiceAccount: "admin",
				},
			},
			newDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					DeprecatedServiceAccount: "newadmin",
				},
			},
			errString: "change serviceAccount",
		},
		{
			name: "StorageConfig changes",
			oldDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					StorageConfig: StorageConfig{
						CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: &storageName,
							AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
							Resources: corev1.ResourceRequirements{
								Requests: map[corev1.ResourceName]resource.Quantity{"storage": storageSize},
							},
						},
					},
				},
			},
			newDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					StorageConfig: StorageConfig{
						CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: &storageName,
							AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteMany"},
							Resources: corev1.ResourceRequirements{
								Requests: map[corev1.ResourceName]resource.Quantity{"storage": storageSize},
							},
						},
					},
				},
			},
			errString: "change storageConfig.CassandraDataVolumeClaimSpec",
		},
		{
			name: "Removing a rack",
			oldDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					Racks: []Rack{{
						Name: "rack0",
					}, {
						Name: "rack1",
					}, {
						Name: "rack2",
					}},
				},
			},
			newDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					Racks: []Rack{{
						Name: "rack0",
					}, {
						Name: "rack2",
					}},
				},
			},
			errString: "remove rack",
		},
		{
			name: "Scaling down",
			oldDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					Racks: []Rack{{
						Name: "rack0",
					}},
					Size: 6,
				},
			},
			newDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					Racks: []Rack{{
						Name: "rack0",
					}},
					Size: 3,
				},
			},
			errString: "",
		},
		{
			name: "Changed a rack name",
			oldDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					Racks: []Rack{{
						Name: "rack0",
					}, {
						Name: "rack1",
					}, {
						Name: "rack2",
					}},
				},
			},
			newDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					Racks: []Rack{{
						Name: "rack0-changed",
					}, {
						Name: "rack1",
					}, {
						Name: "rack2",
					}},
				},
			},
			errString: "change rack name from 'rack0' to 'rack0-changed'",
		},
		{
			name: "Adding a rack is allowed if size increases",
			oldDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					Size: 3,
					Racks: []Rack{{
						Name: "rack0",
					}, {
						Name: "rack1",
					}, {
						Name: "rack2",
					}},
				},
			},
			newDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					Size: 4,
					Racks: []Rack{{
						Name: "rack0",
					}, {
						Name: "rack1",
					}, {
						Name: "rack2",
					}, {
						Name: "rack3",
					}},
				},
			},
			errString: "",
		},
		{
			name: "Adding a rack is not allowed if size doesn't increase",
			oldDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					Racks: []Rack{{
						Name: "rack0",
					}, {
						Name: "rack1",
					}, {
						Name: "rack2",
					}},
				},
			},
			newDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					Racks: []Rack{{
						Name: "rack0",
					}, {
						Name: "rack1",
					}, {
						Name: "rack2",
					}, {
						Name: "rack3",
					}},
				},
			},
			errString: "add rack without increasing size",
		},
		{
			name: "Adding a rack is not allowed if size doesn't increase enough to prevent moving nodes from existing racks",
			oldDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					Size: 9,
					Racks: []Rack{{
						Name: "rack0",
					}, {
						Name: "rack1",
					}},
				},
			},
			newDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					Size: 11,
					Racks: []Rack{{
						Name: "rack0",
					}, {
						Name: "rack1",
					}, {
						Name: "rack2",
					}},
				},
			},
			errString: "add racks without increasing size enough to prevent existing nodes from moving to new racks to maintain balance.\nNew racks added: 1, size increased by: 2. Expected size increase to be at least 4",
		},
		{
			name: "Adding multiple racks is not allowed if size doesn't increase enough to prevent moving nodes from existing racks",
			oldDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					Size: 9,
					Racks: []Rack{{
						Name: "rack0",
					}, {
						Name: "rack1",
					}},
				},
			},
			newDc: &CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: CassandraDatacenterSpec{
					Size: 16,
					Racks: []Rack{{
						Name: "rack0",
					}, {
						Name: "rack1",
					}, {
						Name: "rack2",
					}, {
						Name: "rack3",
					}},
				},
			},
			errString: "add racks without increasing size enough to prevent existing nodes from moving to new racks to maintain balance.\nNew racks added: 2, size increased by: 7. Expected size increase to be at least 8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDatacenterFieldChanges(*tt.oldDc, *tt.newDc)
			if err == nil {
				if tt.errString != "" {
					t.Errorf("ValidateDatacenterFieldChanges() err = %v, want %v", err, tt.errString)
				}
			} else {
				if tt.errString == "" {
					t.Errorf("ValidateDatacenterFieldChanges() err = %v, should be valid", err)
				} else if !strings.HasSuffix(err.Error(), tt.errString) {
					t.Errorf("ValidateDatacenterFieldChanges() err = %v, want suffix %v", err, tt.errString)
				}
			}
		})
	}
}

var fqlEnabledConfig string = `{"cassandra-yaml": { 
	"full_query_logging_options": {
		"log_dir": "/var/log/cassandra/fql" 
		}
	}
}
`

func CreateCassDc(serverType string) CassandraDatacenter {
	dc := CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "exampleDC",
		},
		Spec: CassandraDatacenterSpec{
			ServerType: serverType,
		},
	}

	if serverType == "dse" {
		dc.Spec.ServerVersion = "6.8.13"
	} else {
		dc.Spec.ServerVersion = "4.0.1"
	}

	return dc
}

func Test_parseFQLFromConfig_fqlEnabled(t *testing.T) {
	// Test parsing when fql is set, should return (true, continue).
	dc := CreateCassDc("cassandra")
	dc.Spec.Config = json.RawMessage(fqlEnabledConfig)
	assert.NoError(t, ValidateFQLConfig(dc))
	parsedFQLisEnabled, err := dc.FullQueryEnabled()
	assert.True(t, parsedFQLisEnabled)
	assert.NoError(t, err)
}

var fqlDisabledConfig string = `{"cassandra-yaml": {
"key_cache_size_in_mb": 256
}
}
`

func Test_parseFQLFromConfig_fqlDisabled(t *testing.T) {
	// Test parsing when config exists + fql not set, should return (false, continue()).
	dc := CreateCassDc("cassandra")
	dc.Spec.Config = json.RawMessage(fqlDisabledConfig)
	assert.NoError(t, ValidateFQLConfig(dc))
	parsedFQLisEnabled, err := dc.FullQueryEnabled()
	assert.False(t, parsedFQLisEnabled)
	assert.NoError(t, err)
}

func Test_parseFQLFromConfig_noConfig(t *testing.T) {
	// Test parsing when DC config key does not exist at all, should return (false, continue()).
	dc := CreateCassDc("cassandra")
	dc.Spec.Config = json.RawMessage("{}")
	assert.NoError(t, ValidateFQLConfig(dc))
	parsedFQLisEnabled, err := dc.FullQueryEnabled()
	assert.False(t, parsedFQLisEnabled)
	assert.NoError(t, err)
}

func Test_parseFQLFromConfig_malformedConfig(t *testing.T) {
	// Test parsing when dcConfig is malformed, should return (false, error).
	dc := CreateCassDc("cassandra")
	var corruptedCfg []byte
	for _, b := range json.RawMessage(fqlEnabledConfig) {
		corruptedCfg = append(corruptedCfg, b<<3) // corrupt the byte array.
	}
	dc.Spec.Config = corruptedCfg

	assert.Error(t, ValidateFQLConfig(dc))
	parsedFQLisEnabled, err := dc.FullQueryEnabled()
	assert.False(t, parsedFQLisEnabled)
	assert.Error(t, err)
}

func Test_parseFQLFromConfig_3xFQLEnabled(t *testing.T) {
	// Test parsing when dcConfig asks for FQL on a non-4x server, should return (false, error).
	dc := CreateCassDc("cassandra")
	dc.Spec.Config = json.RawMessage(fqlEnabledConfig)
	dc.Spec.ServerVersion = "3.11.10"
	assert.Error(t, ValidateFQLConfig(dc))

	parsedFQLisEnabled, err := dc.FullQueryEnabled()
	assert.True(t, parsedFQLisEnabled)
	assert.NoError(t, err)
}

func Test_parseFQLFromConfig_DSEFQLEnabled(t *testing.T) {
	// Test parsing when dcConfig asks for FQL on a non-4x server, should return (false, error).
	dc := CreateCassDc("dse")
	dc.Spec.Config = json.RawMessage(fqlEnabledConfig)
	assert.Error(t, ValidateFQLConfig(dc))

	dc.Spec.ServerVersion = "4.0.0"
	assert.Error(t, ValidateFQLConfig(dc))
	parsedFQLisEnabled, err := dc.FullQueryEnabled()
	assert.True(t, parsedFQLisEnabled)
	assert.NoError(t, err)
}

func Test_parseFQLConfigIsNotSet(t *testing.T) {
	dc := CreateCassDc("cassandra")
	assert.NoError(t, ValidateFQLConfig(dc))

	dc.Spec.ServerVersion = "4.0.0"
	assert.NoError(t, ValidateFQLConfig(dc))
	parsedFQLisEnabled, err := dc.FullQueryEnabled()
	assert.False(t, parsedFQLisEnabled)
	assert.NoError(t, err)
}
