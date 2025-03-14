// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/k8ssandra/cass-operator/pkg/oplabels"
	"github.com/k8ssandra/cass-operator/pkg/utils"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
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
		got, err := newStatefulSetForCassandraDatacenter(nil, tt.args.rackName, tt.args.dc, tt.args.replicaCount)
		assert.NoError(t, err, "newStatefulSetForCassandraDatacenter should not have errored")
		assert.NotNil(t, got, "newStatefulSetForCassandraDatacenter should not have returned a nil statefulset")
		assert.Equal(t, map[string]string{"dedicated": "cassandra"}, got.Spec.Template.Spec.NodeSelector)
	}
}

func Test_newStatefulSetForCassandraDatacenter_additionalLabels(t *testing.T) {
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName:        "piclem",
			ServerType:         "cassandra",
			ServerVersion:      "4.0.1",
			PodTemplateSpec:    &corev1.PodTemplateSpec{},
			NodeAffinityLabels: map[string]string{"label1": "dc", "label2": "dc"},
			StorageConfig: api.StorageConfig{
				CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{},
			},
			Racks: []api.Rack{
				{
					Name:               "rack1",
					NodeAffinityLabels: map[string]string{"label2": "rack1", "label3": "rack1"},
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

	expectedStatefulsetLabels := map[string]string{
		oplabels.ManagedByLabel: oplabels.ManagedByLabelValue,
		oplabels.InstanceLabel:  fmt.Sprintf("%s-%s", oplabels.NameLabelValue, dc.Spec.ClusterName),
		oplabels.NameLabel:      oplabels.NameLabelValue,
		oplabels.CreatedByLabel: oplabels.CreatedByLabelValue,
		oplabels.VersionLabel:   "4.0.1",
		api.DatacenterLabel:     "dc1",
		api.ClusterLabel:        "piclem",
		api.RackLabel:           dc.Spec.Racks[0].Name,
		"Add":                   "label",
	}

	expectedPodTemplateLabels := map[string]string{
		oplabels.ManagedByLabel: oplabels.ManagedByLabelValue,
		oplabels.InstanceLabel:  fmt.Sprintf("%s-%s", oplabels.NameLabelValue, dc.Spec.ClusterName),
		oplabels.NameLabel:      oplabels.NameLabelValue,
		oplabels.CreatedByLabel: oplabels.CreatedByLabelValue,
		oplabels.VersionLabel:   "4.0.1",
		api.DatacenterLabel:     "dc1",
		api.ClusterLabel:        "piclem",
		api.RackLabel:           dc.Spec.Racks[0].Name,
		api.CassNodeState:       stateReadyToStart,
		"Add":                   "label",
	}

	statefulset, newStatefulSetForCassandraDatacenterError := newStatefulSetForCassandraDatacenter(nil,
		"rack1", dc, 1)

	assert.NoError(t, newStatefulSetForCassandraDatacenterError,
		"should not have gotten error when creating the new statefulset")

	assert.Equal(t, expectedStatefulsetLabels, statefulset.Labels)
	assert.Equal(t, expectedPodTemplateLabels, statefulset.Spec.Template.Labels)

	for _, volumeClaim := range statefulset.Spec.VolumeClaimTemplates {
		assert.Equal(t, expectedStatefulsetLabels, volumeClaim.Labels)
	}

	assert.Contains(t,
		statefulset.Annotations, "Add")

	for _, volumeClaim := range statefulset.Spec.VolumeClaimTemplates {
		assert.Contains(t,
			volumeClaim.Annotations,
			"Add")
	}

}

func Test_newStatefulSetForCassandraDatacenter_rackNodeAffinitylabels(t *testing.T) {
	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName:        "bob",
			ServerType:         "cassandra",
			ServerVersion:      "3.11.7",
			PodTemplateSpec:    &corev1.PodTemplateSpec{},
			NodeAffinityLabels: map[string]string{"label1": "dc", "label2": "dc"},
			Racks: []api.Rack{
				{
					Name:               "rack1",
					NodeAffinityLabels: map[string]string{"label2": "rack1", "label3": "rack1"},
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
		"label1": "dc",
		"label2": "rack1",
		"label3": "rack1",
	}

	assert.Equal(t, expected, nodeAffinityLabels)
}

func Test_newStatefulSetForCassandraDatacenter_ServiceName(t *testing.T) {
	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "test",
			ServerType:    "cassandra",
			ServerVersion: "4.0.3",
			Size:          1,
			StorageConfig: api.StorageConfig{
				CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{},
			},
		},
	}

	sts, err := newStatefulSetForCassandraDatacenter(&appsv1.StatefulSet{}, "default", dc, 1)

	require.NoError(t, err)
	assert.Equal(t, dc.GetAllPodsServiceName(), sts.Spec.ServiceName)
}

func TestStatefulSetWithAdditionalVolumesFromSource(t *testing.T) {
	assert := assert.New(t)

	storageClassName := "default"

	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ServerType:    "cassandra",
			ServerVersion: "4.1.0",
			ClusterName:   "cluster1",
			StorageConfig: api.StorageConfig{
				CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
					StorageClassName: &storageClassName,
				},
				AdditionalVolumes: api.AdditionalVolumesSlice{
					api.AdditionalVolumes{
						MountPath: "/configs/metrics",
						Name:      "metrics-config",
						VolumeSource: &corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "metrics-config-map",
								},
							},
						},
					},
				},
			},
		},
	}

	sts, err := newStatefulSetForCassandraDatacenter(nil, "r1", dc, 3)
	assert.NoError(err)

	assert.Equal(5, len(sts.Spec.Template.Spec.Volumes))
	assert.Equal("server-config", sts.Spec.Template.Spec.Volumes[0].Name)
	assert.Equal("server-logs", sts.Spec.Template.Spec.Volumes[1].Name)
	assert.Equal("server-config-base", sts.Spec.Template.Spec.Volumes[2].Name)
	assert.Equal("vector-lib", sts.Spec.Template.Spec.Volumes[3].Name)
	assert.Equal("metrics-config", sts.Spec.Template.Spec.Volumes[4].Name)
	assert.NotNil(sts.Spec.Template.Spec.Volumes[4].ConfigMap)
	assert.Equal("metrics-config-map", sts.Spec.Template.Spec.Volumes[4].ConfigMap.Name)

	cassandraContainer := findContainer(sts.Spec.Template.Spec.Containers, CassandraContainerName)
	assert.NotNil(cassandraContainer)

	cassandraVolumeMounts := cassandraContainer.VolumeMounts

	assert.Equal(4, len(cassandraVolumeMounts))
	assert.True(volumeMountsContains(cassandraVolumeMounts, volumeMountNameMatcher("server-config")))
	assert.True(volumeMountsContains(cassandraVolumeMounts, volumeMountNameMatcher("server-logs")))
	assert.True(volumeMountsContains(cassandraVolumeMounts, volumeMountNameMatcher("server-data")))
	assert.True(volumeMountsContains(cassandraVolumeMounts, volumeMountNameMatcher("metrics-config")))

	// Test that both still work together, one additional PVC and one that overrides server-logs EmptyVolumeSource

	dc = &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ServerType:    "cassandra",
			ServerVersion: "4.0.8",
			ClusterName:   "cluster1",
			StorageConfig: api.StorageConfig{
				CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
					StorageClassName: &storageClassName,
				},
				AdditionalVolumes: api.AdditionalVolumesSlice{
					api.AdditionalVolumes{
						MountPath: "/var/log/cassandra",
						Name:      "server-logs",
						PVCSpec: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: &storageClassName,
						},
					},
					api.AdditionalVolumes{
						MountPath: "/var/lib/cassandra/commitlog",
						Name:      "cassandra-commitlogs",
						PVCSpec: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: &storageClassName,
						},
					},
					api.AdditionalVolumes{
						MountPath: "/configs/metrics",
						Name:      "metrics-config",
						VolumeSource: &corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "metrics-config",
								},
							},
						},
					},
				},
			},
		},
	}

	sts, err = newStatefulSetForCassandraDatacenter(nil, "r1", dc, 3)
	assert.NoError(err)

	assert.Equal(3, len(sts.Spec.VolumeClaimTemplates))
	assert.Equal("server-data", sts.Spec.VolumeClaimTemplates[0].Name)
	assert.Equal(storageClassName, *sts.Spec.VolumeClaimTemplates[0].Spec.StorageClassName)
	assert.Equal("server-logs", sts.Spec.VolumeClaimTemplates[1].Name)
	assert.Equal(storageClassName, *sts.Spec.VolumeClaimTemplates[1].Spec.StorageClassName)
	assert.Equal("cassandra-commitlogs", sts.Spec.VolumeClaimTemplates[2].Name)
	assert.Equal(storageClassName, *sts.Spec.VolumeClaimTemplates[2].Spec.StorageClassName)

	assert.Equal(3, len(sts.Spec.Template.Spec.Volumes))
	assert.Equal("server-config", sts.Spec.Template.Spec.Volumes[0].Name)
	assert.Equal("metrics-config", sts.Spec.Template.Spec.Volumes[2].Name)
	assert.NotNil(sts.Spec.Template.Spec.Volumes[2].ConfigMap)
	assert.Equal("metrics-config", sts.Spec.Template.Spec.Volumes[2].ConfigMap.Name)

	cassandraContainer = findContainer(sts.Spec.Template.Spec.Containers, CassandraContainerName)
	assert.NotNil(cassandraContainer)

	cassandraVolumeMounts = cassandraContainer.VolumeMounts
	assert.Equal(5, len(cassandraVolumeMounts))
	assert.Equal("server-logs", cassandraVolumeMounts[0].Name)
	assert.Equal("cassandra-commitlogs", cassandraVolumeMounts[1].Name)
	assert.Equal("/var/lib/cassandra/commitlog", cassandraVolumeMounts[1].MountPath)
	assert.Equal("metrics-config", cassandraVolumeMounts[2].Name)
	assert.Equal("/configs/metrics", cassandraVolumeMounts[2].MountPath)
	assert.Equal("server-data", cassandraVolumeMounts[3].Name)
	assert.Equal("server-config", cassandraVolumeMounts[4].Name)
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
									PVCSpec: &corev1.PersistentVolumeClaimSpec{
										StorageClassName: &customCassandraServerLogsStorageClass,
									},
								},
								api.AdditionalVolumes{
									MountPath: "/var/lib/cassandra/commitlog",
									Name:      "cassandra-commitlogs",
									PVCSpec: &corev1.PersistentVolumeClaimSpec{
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
		got, err := newStatefulSetForCassandraDatacenter(nil, tt.args.rackName, tt.args.dc, tt.args.replicaCount)
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

		assert.Equal(t, 2, len(got.Spec.Template.Spec.Containers))

		assert.Equal(t, 4, len(got.Spec.Template.Spec.Containers[0].VolumeMounts))
		assert.Equal(t, "server-logs", got.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name)
		assert.Equal(t, "cassandra-commitlogs", got.Spec.Template.Spec.Containers[0].VolumeMounts[1].Name)
		assert.Equal(t, "server-data", got.Spec.Template.Spec.Containers[0].VolumeMounts[2].Name)
		assert.Equal(t, "server-config", got.Spec.Template.Spec.Containers[0].VolumeMounts[3].Name)

		assert.Equal(t, 3, len(got.Spec.Template.Spec.Containers[1].VolumeMounts))
		assert.Equal(t, 2, len(got.Spec.Template.Spec.InitContainers))

		assert.Equal(t, "initContainer1", got.Spec.Template.Spec.InitContainers[0].Name)
		assert.Equal(t, "initImage1", got.Spec.Template.Spec.InitContainers[0].Image)
		assert.Equal(t, 1, len(got.Spec.Template.Spec.InitContainers[0].VolumeMounts))
		assert.Equal(t, "server-logs", got.Spec.Template.Spec.InitContainers[0].VolumeMounts[0].Name)
		assert.Equal(t, "/var/log/cassandra", got.Spec.Template.Spec.InitContainers[0].VolumeMounts[0].MountPath)

		assert.Equal(t, "server-config-init", got.Spec.Template.Spec.InitContainers[1].Name)
		assert.Equal(t, "localhost:5000/datastax/cass-config-builder:1.0-ubi8", got.Spec.Template.Spec.InitContainers[1].Image)
		assert.Equal(t, 1, len(got.Spec.Template.Spec.InitContainers[1].VolumeMounts))
		assert.Equal(t, "server-config", got.Spec.Template.Spec.InitContainers[1].VolumeMounts[0].Name)
		assert.Equal(t, "/config", got.Spec.Template.Spec.InitContainers[1].VolumeMounts[0].MountPath)
		assert.Equal(t, int32(5), got.Spec.MinReadySeconds)
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
		RunAsUser:    ptr.To(int64(999)),
		RunAsGroup:   ptr.To(int64(999)),
		FSGroup:      ptr.To(int64(999)),
		RunAsNonRoot: ptr.To[bool](true),
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
					ClusterName:   clusterName,
					ServerType:    "cassandra",
					ServerVersion: "3.11.10",
					StorageConfig: storageConfig,
				},
			},
			expected: defaultSecurityContext,
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
								RunAsUser:  ptr.To(int64(12345)),
								RunAsGroup: ptr.To(int64(54321)),
								FSGroup:    ptr.To(int64(11111)),
							},
						},
					},
				},
			},
			expected: &corev1.PodSecurityContext{
				RunAsUser:  ptr.To(int64(12345)),
				RunAsGroup: ptr.To(int64(54321)),
				FSGroup:    ptr.To(int64(11111)),
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
								RunAsUser:  ptr.To(int64(12345)),
								RunAsGroup: ptr.To(int64(54321)),
								FSGroup:    ptr.To(int64(11111)),
							},
						},
					},
				},
			},
			expected: &corev1.PodSecurityContext{
				RunAsUser:  ptr.To(int64(12345)),
				RunAsGroup: ptr.To(int64(54321)),
				FSGroup:    ptr.To(int64(11111)),
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
		statefulSet, err := newStatefulSetForCassandraDatacenter(nil, rack, tt.dc, replicas)
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

func TestValidSubdomainNames(t *testing.T) {
	assert := assert.New(t)
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName: "cluster1",
		},
	}

	tests := []struct {
		name     string
		expected string
	}{
		{
			name:     "r1",
			expected: "cluster1-dc1-r1-sts",
		},
		{
			name:     "r1.r2",
			expected: "cluster1-dc1-r1.r2-sts",
		},
		{
			name:     "Rack1",
			expected: "cluster1-dc1-rack1-sts",
		},
		{
			name:     "invalid_pod_name*",
			expected: "cluster1-dc1-invalid-pod-name-sts",
		},
	}
	for _, tt := range tests {
		typedName := NewNamespacedNameForStatefulSet(dc, tt.name)
		assert.Equal(tt.expected, typedName.Name)
	}
}

func TestEmptyDatacenterStatusName(t *testing.T) {
	assert := assert.New(t)
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName: "cluster1",
		},
		Status: api.CassandraDatacenterStatus{
			DatacenterName: ptr.To[string](""),
		},
	}

	typedName := NewNamespacedNameForStatefulSet(dc, "r1")
	assert.Equal("cluster1-dc1-r1-sts", typedName.Name)
}

func Test_newStatefulSetForCassandraDatacenter_dcNameOverride(t *testing.T) {
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dc1",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName:     "piclem",
			DatacenterName:  "My Super DC",
			ServerType:      "cassandra",
			ServerVersion:   "4.0.1",
			PodTemplateSpec: &corev1.PodTemplateSpec{},
			StorageConfig: api.StorageConfig{
				CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{},
			},
			Racks: []api.Rack{
				{
					Name:           "rack1",
					DeprecatedZone: "z1",
				},
			},
		},
	}

	expectedStatefulsetLabels := map[string]string{
		oplabels.ManagedByLabel: oplabels.ManagedByLabelValue,
		oplabels.InstanceLabel:  fmt.Sprintf("%s-%s", oplabels.NameLabelValue, dc.Spec.ClusterName),
		oplabels.NameLabel:      oplabels.NameLabelValue,
		oplabels.CreatedByLabel: oplabels.CreatedByLabelValue,
		oplabels.VersionLabel:   "4.0.1",
		api.DatacenterLabel:     "dc1",
		api.ClusterLabel:        "piclem",
		api.RackLabel:           dc.Spec.Racks[0].Name,
	}

	expectedPodTemplateLabels := map[string]string{
		oplabels.ManagedByLabel: oplabels.ManagedByLabelValue,
		oplabels.InstanceLabel:  fmt.Sprintf("%s-%s", oplabels.NameLabelValue, dc.Spec.ClusterName),
		oplabels.NameLabel:      oplabels.NameLabelValue,
		oplabels.CreatedByLabel: oplabels.CreatedByLabelValue,
		oplabels.VersionLabel:   "4.0.1",
		api.DatacenterLabel:     "dc1",
		api.ClusterLabel:        "piclem",
		api.RackLabel:           dc.Spec.Racks[0].Name,
		api.CassNodeState:       stateReadyToStart,
	}

	statefulset, newStatefulSetForCassandraDatacenterError := newStatefulSetForCassandraDatacenter(nil,
		"rack1", dc, 1)

	assert.NoError(t, newStatefulSetForCassandraDatacenterError,
		"should not have gotten error when creating the new statefulset")

	assert.Equal(t, expectedStatefulsetLabels, statefulset.Labels)
	assert.Equal(t, expectedPodTemplateLabels, statefulset.Spec.Template.Labels)

	for _, volumeClaim := range statefulset.Spec.VolumeClaimTemplates {
		assert.Equal(t, expectedStatefulsetLabels, volumeClaim.Labels)
	}
}

func TestPodTemplateSpecHashAnnotationChanges(t *testing.T) {
	assert := assert.New(t)
	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "test",
			ServerType:    "cassandra",
			ServerVersion: "4.0.7",
			StorageConfig: api.StorageConfig{
				CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{},
			},
			Racks: []api.Rack{
				{
					Name: "testrack",
				},
			},
			PodTemplateSpec: &corev1.PodTemplateSpec{},
		},
	}

	sts, err := newStatefulSetForCassandraDatacenter(nil, dc.Spec.Racks[0].Name, dc, 3)
	assert.NoError(err, "failed to build statefulset")

	// Test that the hash annotation is set
	assert.Contains(sts.Annotations, utils.ResourceHashAnnotationKey)
	currentHash := sts.Annotations[utils.ResourceHashAnnotationKey]

	// Add PodTemplateSpec labels
	dc.Spec.PodTemplateSpec.Labels = map[string]string{"abc": "123"}
	sts, err = newStatefulSetForCassandraDatacenter(nil, dc.Spec.Racks[0].Name, dc, 3)
	assert.NoError(err)
	updatedHash := sts.Annotations[utils.ResourceHashAnnotationKey]
	assert.NotEqual(currentHash, updatedHash, "expected hash to change when PodTemplateSpec labels change")

	// Add more labels
	dc.Spec.PodTemplateSpec.Labels["more"] = "labels"
	sts, err = newStatefulSetForCassandraDatacenter(nil, dc.Spec.Racks[0].Name, dc, 3)
	// spec, err := buildPodTemplateSpec(dc, dc.Spec.Racks[0], false)
	assert.NoError(err)
	updatedHash = sts.Annotations[utils.ResourceHashAnnotationKey]
	assert.NotEqual(currentHash, updatedHash, "expected hash to change when PodTemplateSpec labels change")
}

func TestMinReadySecondsChange(t *testing.T) {
	assert := assert.New(t)
	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "test",
			ServerType:    "cassandra",
			ServerVersion: "4.0.7",
			StorageConfig: api.StorageConfig{
				CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{},
			},
			Racks: []api.Rack{
				{
					Name: "testrack",
				},
			},
			PodTemplateSpec: &corev1.PodTemplateSpec{},
		},
	}

	sts, err := newStatefulSetForCassandraDatacenter(nil, dc.Spec.Racks[0].Name, dc, 3)
	assert.NoError(err, "failed to build statefulset")

	assert.Equal(int32(5), sts.Spec.MinReadySeconds)

	dc.Spec.MinReadySeconds = ptr.To[int32](10)

	sts, err = newStatefulSetForCassandraDatacenter(nil, dc.Spec.Racks[0].Name, dc, 3)
	assert.NoError(err, "failed to build statefulset")

	assert.Equal(int32(10), sts.Spec.MinReadySeconds)
}
