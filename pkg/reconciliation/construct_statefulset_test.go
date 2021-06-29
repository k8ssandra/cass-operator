// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"fmt"
	"reflect"
	"testing"

	api "github.com/k8ssandra/cass-operator/api/v1beta1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_newStatefulSetForCassandraDatacenter(t *testing.T) {
	type args struct {
		rackName     string
		dc           *api.CassandraDatacenter
		replicaCount int
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "test nodeSelector",
			args: args{
				rackName:     "r1",
				replicaCount: 1,
				dc: &api.CassandraDatacenter{
					Spec: api.CassandraDatacenterSpec{
						ClusterName:  "c1",
						NodeSelector: map[string]string{"dedicated": "cassandra"},
						StorageConfig: api.StorageConfig{
							CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{},
						},
						ServerType:    "cassandra",
						ServerVersion: "3.11.7",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Log(tt.name)
		got, err := newStatefulSetForCassandraDatacenter(nil, tt.args.rackName, tt.args.dc, tt.args.replicaCount, false)
		assert.NoError(t, err, "newStatefulSetForCassandraDatacenter should not have errored")
		assert.NotNil(t, got, "newStatefulSetForCassandraDatacenter should not have returned a nil statefulset")
		assert.Equal(t, map[string]string{"dedicated": "cassandra"}, got.Spec.Template.Spec.NodeSelector)
	}
}

func Test_newStatefulSetForCassandraDatacenter_rackNodeAffinitylabels(t *testing.T) {
	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName:        "bob",
			ServerType:         "cassandra",
			ServerVersion:      "3.11.7",
			PodTemplateSpec:    &corev1.PodTemplateSpec{},
			NodeAffinityLabels: map[string]string{"dclabel1": "dcvalue1", "dclabel2": "dcvalue2"},
			Racks: []api.Rack{
				{
					Name:               "rack1",
					Zone:               "z1",
					NodeAffinityLabels: map[string]string{"r1label1": "r1value1", "r1label2": "r1value2"},
				},
			},
		},
	}
	var nodeAffinityLabels map[string]string
	var nodeAffinityLabelsConfigurationError error

	nodeAffinityLabels, nodeAffinityLabelsConfigurationError = rackNodeAffinitylabels(dc, "rack1")

	assert.NoError(t, nodeAffinityLabelsConfigurationError,
		"should not have gotten error when getting NodeAffinitylabels of rack rack1")

	expected := map[string]string{
		"dclabel1": "dcvalue1",
		"dclabel2": "dcvalue2",
		"r1label1": "r1value1",
		"r1label2": "r1value2",
		zoneLabel:  "z1",
	}

	assert.Equal(t, expected, nodeAffinityLabels)
}

func Test_newStatefulSetForCassandraDatacenterWithAdditionalVolumes(t *testing.T) {
	type args struct {
		rackName     string
		dc           *api.CassandraDatacenter
		replicaCount int
	}

	customCassandraDataStorageClass := "data"
	customCassandraServerLogsStorageClass := "logs"
	customCassandraCommitLogsStorageClass := "commitlogs"
	tests := []struct {
		name string
		args args
	}{
		{
			name: "test nodeSelector",
			args: args{
				rackName:     "r1",
				replicaCount: 1,
				dc: &api.CassandraDatacenter{
					Spec: api.CassandraDatacenterSpec{
						ClusterName: "c1",
						PodTemplateSpec: &corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								InitContainers: []corev1.Container{
									{
										Name:  "initContainer1",
										Image: "initImage1",
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      "server-logs",
												MountPath: "/var/log/cassandra",
											},
										},
									},
								},
							},
						},
						StorageConfig: api.StorageConfig{
							CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
								StorageClassName: &customCassandraDataStorageClass,
							},
							AdditionalVolumes: api.AdditionalVolumesSlice{
								api.AdditionalVolumes{
									MountPath: "/var/log/cassandra",
									Name:      "server-logs",
									PVCSpec: corev1.PersistentVolumeClaimSpec{
										StorageClassName: &customCassandraServerLogsStorageClass,
									},
								},
								api.AdditionalVolumes{
									MountPath: "/var/lib/cassandra/commitlog",
									Name:      "cassandra-commitlogs",
									PVCSpec: corev1.PersistentVolumeClaimSpec{
										StorageClassName: &customCassandraCommitLogsStorageClass,
									},
								},
							},
						},
						ServerType:    "cassandra",
						ServerVersion: "3.11.7",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Log(tt.name)
		got, err := newStatefulSetForCassandraDatacenter(nil, tt.args.rackName, tt.args.dc, tt.args.replicaCount, false)
		assert.NoError(t, err, "newStatefulSetForCassandraDatacenter should not have errored")
		assert.NotNil(t, got, "newStatefulSetForCassandraDatacenter should not have returned a nil statefulset")

		assert.Equal(t, 3, len(got.Spec.VolumeClaimTemplates))
		assert.Equal(t, "server-data", got.Spec.VolumeClaimTemplates[0].Name)
		assert.Equal(t, customCassandraDataStorageClass, *got.Spec.VolumeClaimTemplates[0].Spec.StorageClassName)
		assert.Equal(t, "server-logs", got.Spec.VolumeClaimTemplates[1].Name)
		assert.Equal(t, customCassandraServerLogsStorageClass, *got.Spec.VolumeClaimTemplates[1].Spec.StorageClassName)
		assert.Equal(t, "cassandra-commitlogs", got.Spec.VolumeClaimTemplates[2].Name)
		assert.Equal(t, customCassandraCommitLogsStorageClass, *got.Spec.VolumeClaimTemplates[2].Spec.StorageClassName)

		assert.Equal(t, 2, len(got.Spec.Template.Spec.Volumes))
		assert.Equal(t, "server-config", got.Spec.Template.Spec.Volumes[0].Name)
		assert.Equal(t, "encryption-cred-storage", got.Spec.Template.Spec.Volumes[1].Name)

		assert.Equal(t, 2, len(got.Spec.Template.Spec.Containers))

		assert.Equal(t, 5, len(got.Spec.Template.Spec.Containers[0].VolumeMounts))
		assert.Equal(t, "server-logs", got.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name)
		assert.Equal(t, "cassandra-commitlogs", got.Spec.Template.Spec.Containers[0].VolumeMounts[1].Name)
		assert.Equal(t, "server-data", got.Spec.Template.Spec.Containers[0].VolumeMounts[2].Name)
		assert.Equal(t, "encryption-cred-storage", got.Spec.Template.Spec.Containers[0].VolumeMounts[3].Name)
		assert.Equal(t, "server-config", got.Spec.Template.Spec.Containers[0].VolumeMounts[4].Name)

		assert.Equal(t, 2, len(got.Spec.Template.Spec.Containers[1].VolumeMounts))
		assert.Equal(t, 2, len(got.Spec.Template.Spec.InitContainers))

		assert.Equal(t, "initContainer1", got.Spec.Template.Spec.InitContainers[0].Name)
		assert.Equal(t, "initImage1", got.Spec.Template.Spec.InitContainers[0].Image)
		assert.Equal(t, 1, len(got.Spec.Template.Spec.InitContainers[0].VolumeMounts))
		assert.Equal(t, "server-logs", got.Spec.Template.Spec.InitContainers[0].VolumeMounts[0].Name)
		assert.Equal(t, "/var/log/cassandra", got.Spec.Template.Spec.InitContainers[0].VolumeMounts[0].MountPath)

		assert.Equal(t, "server-config-init", got.Spec.Template.Spec.InitContainers[1].Name)
		assert.Equal(t, "datastax/cass-config-builder:1.0.4", got.Spec.Template.Spec.InitContainers[1].Image)
		assert.Equal(t, 1, len(got.Spec.Template.Spec.InitContainers[1].VolumeMounts))
		assert.Equal(t, "server-config", got.Spec.Template.Spec.InitContainers[1].VolumeMounts[0].Name)
		assert.Equal(t, "/config", got.Spec.Template.Spec.InitContainers[1].VolumeMounts[0].MountPath)
	}
}

func Test_newStatefulSetForCassandraPodSecurityContext(t *testing.T) {
	clusterName := "test"
	rack := "rack1"
	replicas := 1
	storageClass := "standard"
	storageConfig := api.StorageConfig{
		CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storageClass,
		},
	}

	defaultSecurityContext := &corev1.PodSecurityContext{
		RunAsUser:  int64Ptr(999),
		RunAsGroup: int64Ptr(999),
		FSGroup:    int64Ptr(999),
	}

	tests := []struct {
		name     string
		dc       *api.CassandraDatacenter
		expected *corev1.PodSecurityContext
	}{
		{
			name: "run cassandra as non-root user",
			dc: &api.CassandraDatacenter{
				Spec: api.CassandraDatacenterSpec{
					ClusterName:                clusterName,
					ServerType:                 "cassandra",
					ServerVersion:              "3.11.10",
					DockerImageRunsAsCassandra: boolPtr(true),
					StorageConfig:              storageConfig,
				},
			},
			expected: defaultSecurityContext,
		},
		{
			name: "run cassandra as root user",
			dc: &api.CassandraDatacenter{
				Spec: api.CassandraDatacenterSpec{
					ClusterName:                clusterName,
					ServerType:                 "cassandra",
					ServerVersion:              "3.11.7",
					DockerImageRunsAsCassandra: boolPtr(false),
					StorageConfig:              storageConfig,
				},
			},
			expected: nil,
		},
		{
			// Note that DSE only supports running as non-root
			name: "run dse as non-root user",
			dc: &api.CassandraDatacenter{
				Spec: api.CassandraDatacenterSpec{
					ClusterName:   clusterName,
					ServerType:    "dse",
					ServerVersion: "6.8.7",
					StorageConfig: storageConfig,
				},
			},
			expected: defaultSecurityContext,
		},
		{
			name: "run cassandra with pod security context override",
			dc: &api.CassandraDatacenter{
				Spec: api.CassandraDatacenterSpec{
					ClusterName:   clusterName,
					ServerType:    "cassandra",
					ServerVersion: "3.11.10",
					StorageConfig: storageConfig,
					PodTemplateSpec: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:  int64Ptr(12345),
								RunAsGroup: int64Ptr(54321),
								FSGroup:    int64Ptr(11111),
							},
						},
					},
				},
			},
			expected: &corev1.PodSecurityContext{
				RunAsUser:  int64Ptr(12345),
				RunAsGroup: int64Ptr(54321),
				FSGroup:    int64Ptr(11111),
			},
		},
		{
			name: "run dse with pod security context override",
			dc: &api.CassandraDatacenter{
				Spec: api.CassandraDatacenterSpec{
					ClusterName:   clusterName,
					ServerType:    "dse",
					ServerVersion: "6.8.7",
					StorageConfig: storageConfig,
					PodTemplateSpec: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:  int64Ptr(12345),
								RunAsGroup: int64Ptr(54321),
								FSGroup:    int64Ptr(11111),
							},
						},
					},
				},
			},
			expected: &corev1.PodSecurityContext{
				RunAsUser:  int64Ptr(12345),
				RunAsGroup: int64Ptr(54321),
				FSGroup:    int64Ptr(11111),
			},
		},
		{
			name: "run cassandra with empty pod security context override",
			dc: &api.CassandraDatacenter{
				Spec: api.CassandraDatacenterSpec{
					ClusterName:   clusterName,
					ServerType:    "cassandra",
					ServerVersion: "3.11.10",
					StorageConfig: storageConfig,
					PodTemplateSpec: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							SecurityContext: &corev1.PodSecurityContext{},
						},
					},
				},
			},
			expected: &corev1.PodSecurityContext{},
		},
		{
			name: "run dse with empty pod security context override",
			dc: &api.CassandraDatacenter{
				Spec: api.CassandraDatacenterSpec{
					ClusterName:   clusterName,
					ServerType:    "dse",
					ServerVersion: "6.8.7",
					StorageConfig: storageConfig,
					PodTemplateSpec: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							SecurityContext: &corev1.PodSecurityContext{},
						},
					},
				},
			},
			expected: &corev1.PodSecurityContext{},
		},
	}
	for _, tt := range tests {
		t.Log(tt.name)
		statefulSet, err := newStatefulSetForCassandraDatacenter(nil, rack, tt.dc, replicas, false)
		assert.NoError(t, err, fmt.Sprintf("%s: failed to create new statefulset", tt.name))
		assert.NotNil(t, statefulSet, fmt.Sprintf("%s: statefulset is nil", tt.name))

		actual := statefulSet.Spec.Template.Spec.SecurityContext
		if tt.expected == nil {
			assert.Nil(t, actual, fmt.Sprintf("%s: expected pod security context to be nil", tt.name))
		} else {
			assert.NotNil(t, actual, fmt.Sprintf("%s: pod security context is nil", tt.name))
			assert.True(t, reflect.DeepEqual(tt.expected, actual),
				fmt.Sprintf("%s: pod security context does not match expected value:\n expected: %+v\n actual: %+v", tt.name, tt.expected, actual))
		}
	}
}

func int64Ptr(n int64) *int64 {
	return &n
}

func boolPtr(b bool) *bool {
	return &b
}
