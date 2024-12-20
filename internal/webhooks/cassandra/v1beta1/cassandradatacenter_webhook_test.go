// Copyright DataStax, Inc.
// Please see the included license file for details.

package v1beta1

import (
	"encoding/json"
	"strings"
	"testing"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_ValidateSingleDatacenter(t *testing.T) {
	tests := []struct {
		name      string
		dc        *api.CassandraDatacenter
		errString string
	}{
		{
			name: "DSE Valid",
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					ServerType:    "dse",
					ServerVersion: "6.8.0",
				},
			},
			errString: "",
		},
		{
			name: "DSE 6.8.4 Valid",
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					ServerType:    "dse",
					ServerVersion: "6.8.4",
				},
			},
			errString: "",
		},
		{
			name: "DSE 6.9.1 Valid",
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					ServerType:    "dse",
					ServerVersion: "6.9.1",
				},
			},
			errString: "",
		},
		{
			name: "HCD 1.0.0 Valid",
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					ServerType:    "hcd",
					ServerVersion: "1.0.0",
				},
			},
			errString: "",
		},
		{
			name: "DSE 7.0.0 invalid",
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					ServerType:    "dse",
					ServerVersion: "7.0.0",
				},
			},
			errString: "use unsupported DSE version '7.0.0'",
		},
		{
			name: "DSE Invalid",
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					ServerType:    "dse",
					ServerVersion: "4.8.0",
				},
			},
			errString: "use unsupported DSE version '4.8.0'",
		},
		{
			name: "DSE 5 Invalid",
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					ServerType:    "dse",
					ServerVersion: "5.0.0",
				},
			},
			errString: "use unsupported DSE version '5.0.0'",
		},
		{
			name: "Cassandra valid",
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					ServerType:    "cassandra",
					ServerVersion: "3.11.7",
				},
			},
			errString: "",
		},
		{
			name: "Cassandra 4.0.x must be valid",
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					ServerType:    "cassandra",
					ServerVersion: "4.0.3",
				},
			},
			errString: "",
		},
		{
			name: "Cassandra 4.1 must be valid",
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					ServerType:    "cassandra",
					ServerVersion: "4.1.0",
				},
			},
			errString: "",
		},
		{
			name: "Cassandra 5.0.0 must be valid",
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					ServerType:    "cassandra",
					ServerVersion: "5.0.0",
				},
			},
			errString: "",
		},
		{
			name: "Cassandra Invalid",
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					ServerType:    "cassandra",
					ServerVersion: "6.8.0",
				},
			},
			errString: "use unsupported Cassandra version '6.8.0'",
		},
		{
			name: "Cassandra Invalid too",
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					ServerType:    "cassandra",
					ServerVersion: "7.0.0",
				},
			},
			errString: "use unsupported Cassandra version '7.0.0'",
		},
		{
			name: "Dse Workloads in Cassandra Invalid",
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					ServerType:    "cassandra",
					ServerVersion: "6.8.0",
					DseWorkloads: &api.DseWorkloads{
						AnalyticsEnabled: true,
					},
				},
			},
			errString: "CassandraDatacenter write rejected, attempted to enable DSE workloads if server type is Cassandra",
		},
		{
			name: "Dse Workloads in Dse valid",
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					ServerType:    "dse",
					ServerVersion: "6.8.4",
					DseWorkloads: &api.DseWorkloads{
						AnalyticsEnabled: true,
					},
				},
			},
			errString: "",
		},
		{
			name: "Cassandra 3.11 invalid config file dse-yaml",
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
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
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
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
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
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
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
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
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					ServerType:                  "dse",
					ServerVersion:               "6.8.4",
					Config:                      json.RawMessage(`{}`),
					AllowMultipleNodesPerWorker: true,
				},
			},
			errString: "use multiple nodes per worker without cpu and memory requests and limits",
		},
		{
			name: "Prevent user specified cassandra.datastax.com Service labels and annotations",
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					ServerType:    "cassandra",
					ServerVersion: "4.0.4",
					AdditionalServiceConfig: api.ServiceConfig{
						DatacenterService: api.ServiceConfigAdditions{
							Labels:      map[string]string{"cassandra.datastax.com/key1": "val1"},
							Annotations: map[string]string{"cassandra.datastax.com/key2": "val2"},
						},
					},
				},
			},
			errString: "configure DatacenterService with reserved annotations and/or labels (prefix cassandra.datastax.com)",
		},
		{
			name: "Allow user specified k8ssandra.io Service labels and annotations",
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					ServerType:    "cassandra",
					ServerVersion: "4.0.4",
					AdditionalServiceConfig: api.ServiceConfig{
						DatacenterService: api.ServiceConfigAdditions{
							Labels:      map[string]string{"k8ssandra.io/key1": "val1"},
							Annotations: map[string]string{"k8ssandra.io/key2": "val2"},
						},
					},
				},
			},
			errString: "",
		},
		{
			name: "Allow upgrade should not accept invalid values",
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
					Annotations: map[string]string{
						"cassandra.datastax.com/autoupdate-spec": "invalid",
					},
				},
				Spec: api.CassandraDatacenterSpec{
					ServerType:    "dse",
					ServerVersion: "6.8.42",
				},
			},
			errString: "use cassandra.datastax.com/autoupdate-spec annotation with value other than 'once' or 'always'",
		},
		{
			name: "Allow upgrade should accept once value",
			dc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
					Annotations: map[string]string{
						"cassandra.datastax.com/autoupdate-spec": "once",
					},
				},
				Spec: api.CassandraDatacenterSpec{
					ServerType:    "dse",
					ServerVersion: "6.8.42",
				},
			},
			errString: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSingleDatacenter(tt.dc)
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
	storageName := ptr.To[string]("server-data")

	tests := []struct {
		name      string
		oldDc     *api.CassandraDatacenter
		newDc     *api.CassandraDatacenter
		errString string
	}{
		{
			name: "No significant changes",
			oldDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					ClusterName:                 "oldname",
					AllowMultipleNodesPerWorker: false,
					SuperuserSecretName:         "hush",
					DeprecatedServiceAccount:    "admin",
					StorageConfig: api.StorageConfig{
						CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: storageName,
							AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
							Resources: corev1.VolumeResourceRequirements{
								Requests: map[corev1.ResourceName]resource.Quantity{"storage": storageSize},
							},
						},
					},
					Racks: []api.Rack{{
						Name: "rack0",
					}, {
						Name: "rack1",
					}, {
						Name: "rack2",
					}},
				},
			},
			newDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					ClusterName:                 "oldname",
					AllowMultipleNodesPerWorker: false,
					SuperuserSecretName:         "hush",
					DeprecatedServiceAccount:    "admin",
					StorageConfig: api.StorageConfig{
						CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: storageName,
							AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
							Resources: corev1.VolumeResourceRequirements{
								Requests: map[corev1.ResourceName]resource.Quantity{"storage": storageSize},
							},
						},
					},
					Racks: []api.Rack{{
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
			oldDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					ClusterName: "oldname",
				},
			},
			newDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					ClusterName: "newname",
				},
			},
			errString: "change clusterName",
		},
		{
			name: "DatacenterName changed",
			oldDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					DatacenterName: "oldname",
				},
			},
			newDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					DatacenterName: "newname",
				},
			},
			errString: "change datacenterName",
		},
		{
			name: "AllowMultipleNodesPerWorker changed",
			oldDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					AllowMultipleNodesPerWorker: false,
				},
			},
			newDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					AllowMultipleNodesPerWorker: true,
				},
			},
			errString: "change allowMultipleNodesPerWorker",
		},
		{
			name: "SuperuserSecretName changed",
			oldDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					SuperuserSecretName: "hush",
				},
			},
			newDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					SuperuserSecretName: "newsecret",
				},
			},
			errString: "change superuserSecretName",
		},
		{
			name: "ServiceAccount changed",
			oldDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					DeprecatedServiceAccount: "admin",
				},
			},
			newDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					DeprecatedServiceAccount: "newadmin",
				},
			},
			errString: "change serviceAccount",
		},
		{
			name: "StorageConfig changes",
			oldDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					StorageConfig: api.StorageConfig{
						CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: storageName,
							AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
							Resources: corev1.VolumeResourceRequirements{
								Requests: map[corev1.ResourceName]resource.Quantity{"storage": storageSize},
							},
						},
					},
				},
			},
			newDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					StorageConfig: api.StorageConfig{
						CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: storageName,
							AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteMany"},
							Resources: corev1.VolumeResourceRequirements{
								Requests: map[corev1.ResourceName]resource.Quantity{"storage": storageSize},
							},
						},
					},
				},
			},
			errString: "change storageConfig.CassandraDataVolumeClaimSpec",
		},
		{
			name: "StorageClassName changes with storageConfig changes allowed",
			oldDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					StorageConfig: api.StorageConfig{
						CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: storageName,
							AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
							Resources: corev1.VolumeResourceRequirements{
								Requests: map[corev1.ResourceName]resource.Quantity{"storage": storageSize},
							},
						},
					},
				},
			},
			newDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
					Annotations: map[string]string{
						api.AllowStorageChangesAnnotation: "true",
					},
				},
				Spec: api.CassandraDatacenterSpec{
					StorageConfig: api.StorageConfig{
						CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: ptr.To[string]("new-server-data"),
							AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
							Resources: corev1.VolumeResourceRequirements{
								Requests: map[corev1.ResourceName]resource.Quantity{"storage": storageSize},
							},
						},
					},
				},
			},
			errString: "change storageConfig.CassandraDataVolumeClaimSpec",
		},
		{
			name: "storage requests size changes with storageConfig changes allowed",
			oldDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					StorageConfig: api.StorageConfig{
						CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: storageName,
							AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
							Resources: corev1.VolumeResourceRequirements{
								Requests: map[corev1.ResourceName]resource.Quantity{"storage": storageSize},
							},
						},
					},
				},
			},
			newDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
					Annotations: map[string]string{
						api.AllowStorageChangesAnnotation: "true",
					},
				},
				Spec: api.CassandraDatacenterSpec{
					StorageConfig: api.StorageConfig{
						CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: storageName,
							AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
							Resources: corev1.VolumeResourceRequirements{
								Requests: map[corev1.ResourceName]resource.Quantity{"storage": resource.MustParse("2Gi")},
							},
						},
					},
				},
			},
			errString: "",
		},
		{
			name: "Removing a rack",
			oldDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					Racks: []api.Rack{{
						Name: "rack0",
					}, {
						Name: "rack1",
					}, {
						Name: "rack2",
					}},
				},
			},
			newDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					Racks: []api.Rack{{
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
			oldDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					Racks: []api.Rack{{
						Name: "rack0",
					}},
					Size: 6,
				},
			},
			newDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					Racks: []api.Rack{{
						Name: "rack0",
					}},
					Size: 3,
				},
			},
			errString: "",
		},
		{
			name: "Changed a rack name",
			oldDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					Racks: []api.Rack{{
						Name: "rack0",
					}, {
						Name: "rack1",
					}, {
						Name: "rack2",
					}},
				},
			},
			newDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					Racks: []api.Rack{{
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
			oldDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					Size: 3,
					Racks: []api.Rack{{
						Name: "rack0",
					}, {
						Name: "rack1",
					}, {
						Name: "rack2",
					}},
				},
			},
			newDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					Size: 4,
					Racks: []api.Rack{{
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
			oldDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					Racks: []api.Rack{{
						Name: "rack0",
					}, {
						Name: "rack1",
					}, {
						Name: "rack2",
					}},
				},
			},
			newDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					Racks: []api.Rack{{
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
			oldDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					Size: 9,
					Racks: []api.Rack{{
						Name: "rack0",
					}, {
						Name: "rack1",
					}},
				},
			},
			newDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					Size: 11,
					Racks: []api.Rack{{
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
			oldDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					Size: 9,
					Racks: []api.Rack{{
						Name: "rack0",
					}, {
						Name: "rack1",
					}},
				},
			},
			newDc: &api.CassandraDatacenter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "exampleDC",
				},
				Spec: api.CassandraDatacenterSpec{
					Size: 16,
					Racks: []api.Rack{{
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
			err := ValidateDatacenterFieldChanges(tt.oldDc, tt.newDc)
			if err == nil {
				if tt.errString != "" {
					t.Errorf("ValidateDatacenterFieldChanges() err = %v, want %v", err, tt.errString)
				}
			} else {
				if tt.errString == "" {
					t.Errorf("ValidateDatacenterFieldChanges() err = %v, should be valid", err)
				} else if !strings.Contains(err.Error(), tt.errString) {
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

func CreateCassDc(serverType string) *api.CassandraDatacenter {
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "exampleDC",
		},
		Spec: api.CassandraDatacenterSpec{
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
