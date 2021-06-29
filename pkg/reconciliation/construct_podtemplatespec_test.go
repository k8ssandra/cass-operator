// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/resource"

	api "github.com/k8ssandra/cass-operator/api/v1beta1"
	"github.com/k8ssandra/cass-operator/pkg/images"
	"github.com/k8ssandra/cass-operator/pkg/oplabels"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_calculatePodAntiAffinity(t *testing.T) {
	t.Run("check when we allow more than one server pod per node", func(t *testing.T) {
		paa := calculatePodAntiAffinity(true)
		if paa != nil {
			t.Errorf("calculatePodAntiAffinity() = %v, and we want nil", paa)
		}
	})

	t.Run("check when we do not allow more than one server pod per node", func(t *testing.T) {
		paa := calculatePodAntiAffinity(false)
		if paa == nil ||
			len(paa.RequiredDuringSchedulingIgnoredDuringExecution) != 1 {
			t.Errorf("calculatePodAntiAffinity() = %v, and we want one element in RequiredDuringSchedulingIgnoredDuringExecution", paa)
		}
	})
}

func Test_calculateNodeAffinity(t *testing.T) {
	t.Run("check when we dont have a zone we want to use", func(t *testing.T) {
		na := calculateNodeAffinity(map[string]string{})
		if na != nil {
			t.Errorf("calculateNodeAffinity() = %v, and we want nil", na)
		}
	})

	t.Run("check when we do not allow more than one dse pod per node", func(t *testing.T) {
		na := calculateNodeAffinity(map[string]string{zoneLabel: "thezone"})
		if na == nil ||
			na.RequiredDuringSchedulingIgnoredDuringExecution == nil {
			t.Errorf("calculateNodeAffinity() = %v, and we want a non-nil RequiredDuringSchedulingIgnoredDuringExecution", na)
		}
	})
}

func TestCassandraDatacenter_buildInitContainer_resources_set(t *testing.T) {
	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "bob",
			ServerType:    "cassandra",
			ServerVersion: "3.11.7",
			ConfigBuilderResources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					"cpu":    *resource.NewMilliQuantity(1, resource.DecimalSI),
					"memory": *resource.NewScaledQuantity(1, resource.Giga),
				},
				Requests: corev1.ResourceList{
					"cpu":    *resource.NewMilliQuantity(1, resource.DecimalSI),
					"memory": *resource.NewScaledQuantity(1, resource.Giga),
				},
			},
		},
	}

	podTemplateSpec := corev1.PodTemplateSpec{}
	err := buildInitContainers(dc, "testRack", &podTemplateSpec)
	initContainers := podTemplateSpec.Spec.InitContainers
	assert.NotNil(t, initContainers, "Unexpected init containers received")
	assert.Nil(t, err, "Unexpected error encountered")

	assert.Len(t, initContainers, 1, "Unexpected number of init containers returned")
	if !reflect.DeepEqual(dc.Spec.ConfigBuilderResources, initContainers[0].Resources) {
		t.Errorf("system-config-init container resources not correctly set")
		t.Errorf("got: %v", initContainers[0])
	}
}

func TestCassandraDatacenter_buildInitContainer_resources_set_when_not_specified(t *testing.T) {
	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "bob",
			ServerType:    "cassandra",
			ServerVersion: "3.11.7",
		},
	}

	podTemplateSpec := corev1.PodTemplateSpec{}
	err := buildInitContainers(dc, "testRack", &podTemplateSpec)
	initContainers := podTemplateSpec.Spec.InitContainers
	assert.NotNil(t, initContainers, "Unexpected init containers received")
	assert.Nil(t, err, "Unexpected error encountered")

	assert.Len(t, initContainers, 1, "Unexpected number of init containers returned")
	if !reflect.DeepEqual(initContainers[0].Resources, DefaultsConfigInitContainer) {
		t.Error("Unexpected default resources allocated for the init container.")
	}
}

func TestCassandraDatacenter_buildInitContainer_with_overrides(t *testing.T) {
	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "bob",
			ServerType:    "cassandra",
			ServerVersion: "3.11.7",
		},
	}

	podTemplateSpec := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name: ServerConfigContainerName,
					Env: []corev1.EnvVar{
						{
							Name:  "k1",
							Value: "v1",
						},
					},
				},
			},
		},
	}

	err := buildInitContainers(dc, "testRack", podTemplateSpec)
	initContainers := podTemplateSpec.Spec.InitContainers
	assert.NotNil(t, initContainers, "Unexpected init containers received")
	assert.Nil(t, err, "Unexpected error encountered")

	assert.Len(t, initContainers, 1, "Unexpected number of init containers returned")
	if !reflect.DeepEqual(initContainers[0].Resources, DefaultsConfigInitContainer) {
		t.Error("Unexpected default resources allocated for the init container.")
	}

	assert.Contains(t, initContainers[0].Env, corev1.EnvVar{Name: "k1", Value: "v1"},
		fmt.Sprintf("Unexpected env vars allocated for the init container: %v", initContainers[0].Env))

	assert.Contains(t, initContainers[0].Env, corev1.EnvVar{Name: "USE_HOST_IP_FOR_BROADCAST", Value: "false"},
		fmt.Sprintf("Unexpected env vars allocated for the init container: %v", initContainers[0].Env))
}

func TestCassandraDatacenter_buildContainers_systemlogger_resources_set(t *testing.T) {
	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "bob",
			ServerType:    "cassandra",
			ServerVersion: "3.11.7",
			SystemLoggerResources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					"cpu":    *resource.NewMilliQuantity(1, resource.DecimalSI),
					"memory": *resource.NewScaledQuantity(1, resource.Giga),
				},
				Requests: corev1.ResourceList{
					"cpu":    *resource.NewMilliQuantity(1, resource.DecimalSI),
					"memory": *resource.NewScaledQuantity(1, resource.Giga),
				},
			},
		},
	}

	podTemplateSpec := &corev1.PodTemplateSpec{}
	err := buildContainers(dc, podTemplateSpec)
	containers := podTemplateSpec.Spec.Containers
	assert.NotNil(t, containers, "Unexpected containers containers received")
	assert.Nil(t, err, "Unexpected error encountered")

	assert.Len(t, containers, 2, "Unexpected number of containers containers returned")
	assert.Equal(t, containers[1].Resources, dc.Spec.SystemLoggerResources,
		"server-system-logger container resources are unexpected")
}

func TestCassandraDatacenter_buildContainers_systemlogger_resources_set_when_not_specified(t *testing.T) {
	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "bob",
			ServerType:    "cassandra",
			ServerVersion: "3.11.7",
		},
	}

	podTemplateSpec := &corev1.PodTemplateSpec{}
	err := buildContainers(dc, podTemplateSpec)
	containers := podTemplateSpec.Spec.Containers
	assert.NotNil(t, containers, "Unexpected containers containers received")
	assert.Nil(t, err, "Unexpected error encountered")

	assert.Len(t, containers, 2, "Unexpected number of containers containers returned")
	if !reflect.DeepEqual(containers[1].Resources, DefaultsLoggerContainer) {
		t.Error("server-system-logger container resources are not set to default values.")
	}
}

func TestCassandraDatacenter_buildContainers_use_cassandra_settings(t *testing.T) {
	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "bob",
			ServerType:    "cassandra",
			ServerVersion: "3.11.7",
		},
	}

	cassContainer := corev1.Container{
		Name: "cassandra",
		Env: []corev1.EnvVar{
			corev1.EnvVar{
				Name:  "k1",
				Value: "v1",
			},
		},
	}

	podTemplateSpec := &corev1.PodTemplateSpec{}
	podTemplateSpec.Spec.Containers = append(podTemplateSpec.Spec.Containers, cassContainer)

	err := buildContainers(dc, podTemplateSpec)
	containers := podTemplateSpec.Spec.Containers
	assert.NotNil(t, containers, "Unexpected containers containers received")
	assert.Nil(t, err, "Unexpected error encountered")

	assert.Len(t, containers, 2, "Unexpected number of containers containers returned")

	if !reflect.DeepEqual(containers[0].Env[0].Name, "k1") {
		t.Errorf("Unexpected env vars allocated for the cassandra container: %v", containers[0].Env)
	}
}

func TestServerConfigInitContainerEnvVars(t *testing.T) {
	rack := "rack1"
	podIPEnvVar := corev1.EnvVar{Name: "POD_IP", ValueFrom: selectorFromFieldPath("status.podIP")}
	hostIPEnvVar := corev1.EnvVar{Name: "HOST_IP", ValueFrom: selectorFromFieldPath("status.hostIP")}

	tests := []struct {
		name         string
		annotations  map[string]string
		config       []byte
		configSecret string
		want         []corev1.EnvVar
	}{
		{
			name:   "use config",
			config: []byte(`{"cassandra-yaml":{"read_request_timeout_in_ms":10000}}`),
			want: []corev1.EnvVar{
				podIPEnvVar,
				hostIPEnvVar,
				{
					Name:  "USE_HOST_IP_FOR_BROADCAST",
					Value: "false",
				},
				{
					Name:  "RACK_NAME",
					Value: rack,
				},
				{
					Name:  "PRODUCT_VERSION",
					Value: "3.11.10",
				},
				{
					Name:  "PRODUCT_NAME",
					Value: "cassandra",
				},
				{
					Name:  "DSE_VERSION",
					Value: "3.11.10",
				},
			},
		},
		{
			name: "use config secret",
			annotations: map[string]string{
				api.ConfigHashAnnotation: "123456789",
			},
			configSecret: "secret-config",
			want: []corev1.EnvVar{
				podIPEnvVar,
				hostIPEnvVar,
				{
					Name:  "USE_HOST_IP_FOR_BROADCAST",
					Value: "false",
				},
				{
					Name:  "RACK_NAME",
					Value: rack,
				},
				{
					Name:  "PRODUCT_VERSION",
					Value: "3.11.10",
				},
				{
					Name:  "PRODUCT_NAME",
					Value: "cassandra",
				},
				{
					Name:  "DSE_VERSION",
					Value: "3.11.10",
				},
			},
		},
	}
	for _, tt := range tests {
		templateSpec := &corev1.PodTemplateSpec{}
		dc := &api.CassandraDatacenter{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   "test",
				Name:        "test",
				Annotations: tt.annotations,
			},
			Spec: api.CassandraDatacenterSpec{
				ClusterName:   "test",
				ServerType:    "cassandra",
				ServerVersion: "3.11.10",
				Config:        tt.config,
				ConfigSecret:  tt.configSecret,
			},
		}

		configEnVars, err := getConfigDataEnVars(dc)
		assert.NoError(t, err, "failed to get config env vars")

		for _, v := range configEnVars {
			tt.want = append(tt.want, v)
		}

		if err := buildInitContainers(dc, rack, templateSpec); err == nil {
			assert.Equal(t, 1, len(templateSpec.Spec.InitContainers), fmt.Sprintf("%s: expected to find 1 init container", tt.name))

			initContainer := templateSpec.Spec.InitContainers[0]
			assert.Equal(t, ServerConfigContainerName, initContainer.Name, fmt.Sprintf("%s: expected to find %s init container", tt.name, ServerConfigContainerName))

			assert.True(t, envVarsMatch(tt.want, initContainer.Env), fmt.Sprintf("%s: wanted %+v, got %+v", tt.name, tt.want, initContainer.Env))
		} else {
			t.Errorf("%s: failed to build init containers: %s", tt.name, err)
		}
	}
}

func TestCassandraDatacenter_buildContainers_override_other_containers(t *testing.T) {
	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "bob",
			ServerType:    "cassandra",
			ServerVersion: "3.11.7",
		},
	}

	podTemplateSpec := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				corev1.Container{
					Name: SystemLoggerContainerName,
					VolumeMounts: []corev1.VolumeMount{
						corev1.VolumeMount{
							Name:      "extra",
							MountPath: "/extra",
						},
					},
				},
			},
		},
	}

	err := buildContainers(dc, podTemplateSpec)
	containers := podTemplateSpec.Spec.Containers
	assert.NotNil(t, containers, "Unexpected containers containers received")
	assert.Nil(t, err, "Unexpected error encountered")

	assert.Len(t, containers, 2, "Unexpected number of containers containers returned")

	if !reflect.DeepEqual(containers[0].VolumeMounts,
		[]corev1.VolumeMount{
			corev1.VolumeMount{
				Name:      "extra",
				MountPath: "/extra",
			},
			corev1.VolumeMount{
				Name:      "server-logs",
				MountPath: "/var/log/cassandra",
			},
		}) {
		t.Errorf("Unexpected volume mounts for the logger container: %v", containers[0].VolumeMounts)
	}
}

func TestCassandraDatacenter_buildPodTemplateSpec_containers_merge(t *testing.T) {
	testContainer := corev1.Container{}
	testContainer.Name = "test-container"
	testContainer.Image = "test-image"
	testContainer.Env = []corev1.EnvVar{
		{Name: "TEST_VAL", Value: "TEST"},
	}

	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "bob",
			ServerType:    "cassandra",
			ServerVersion: "3.11.7",
			PodTemplateSpec: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{testContainer}},
			},
		},
	}
	got, err := buildPodTemplateSpec(dc, map[string]string{zoneLabel: "testzone"}, "testrack")

	assert.NoError(t, err, "should not have gotten error when building podTemplateSpec")
	assert.Equal(t, 3, len(got.Spec.Containers))
	if !reflect.DeepEqual(testContainer, got.Spec.Containers[0]) {
		t.Errorf("third container = %v, want %v", got, testContainer)
	}
}

func TestCassandraDatacenter_buildPodTemplateSpec_initcontainers_merge(t *testing.T) {
	testContainer := corev1.Container{}
	testContainer.Name = "test-container-init"
	testContainer.Image = "test-image-init"
	testContainer.Env = []corev1.EnvVar{
		{Name: "TEST_VAL", Value: "TEST"},
	}

	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "bob",
			ServerType:    "cassandra",
			ServerVersion: "3.11.7",
			PodTemplateSpec: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{testContainer}},
			},
			ConfigBuilderResources: testContainer.Resources,
		},
	}
	got, err := buildPodTemplateSpec(dc, map[string]string{zoneLabel: "testzone"}, "testrack")

	assert.NoError(t, err, "should not have gotten error when building podTemplateSpec")
	assert.Equal(t, 2, len(got.Spec.InitContainers))
	if !reflect.DeepEqual(testContainer, got.Spec.InitContainers[0]) {
		t.Errorf("second init container = %v, want %v", got, testContainer)
	}
}

func TestCassandraDatacenter_buildPodTemplateSpec_add_initContainer_after_config_initContainer(t *testing.T) {
	// When adding an initContainer with podTemplate spec it will run before
	// the server config initContainer by default. This test demonstrates and
	// verifies how to specify the initContainer to run after the server config
	// initContainer.

	initContainer := corev1.Container{
		Name:  "test-container",
		Image: "test-image",
	}

	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "bob",
			ServerType:    "cassandra",
			ServerVersion: "3.11.7",
			PodTemplateSpec: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name: ServerConfigContainerName,
						},
						initContainer,
					},
				},
			},
		},
	}

	podTemplateSpec, err := buildPodTemplateSpec(dc, map[string]string{zoneLabel: "testzone"}, "testrack")

	assert.NoError(t, err, "should not have gotten error when building podTemplateSpec")

	initContainers := podTemplateSpec.Spec.InitContainers
	assert.Equal(t, 2, len(initContainers))
	assert.Equal(t, initContainers[0].Name, ServerConfigContainerName)
	assert.Equal(t, initContainers[1].Name, initContainer.Name)
}

func TestCassandraDatacenter_buildPodTemplateSpec_add_initContainer_with_volumes(t *testing.T) {
	// This test adds an initContainer, a new volume, a volume mount for the
	// new volume, and mounts for existing volumes. Not only does the test
	// verify that the initContainer has the correct volumes, but it also
	// verifies that the "built-in" containers have the correct mounts.

	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "bob",
			ServerType:    "cassandra",
			ServerVersion: "3.11.7",
			PodTemplateSpec: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:  "test",
							Image: "test",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "server-data",
									MountPath: "/var/lib/cassandra",
								},
								{
									Name:      "server-config",
									MountPath: "/config",
								},
								{
									Name:      "test-data",
									MountPath: "/test",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "test-data",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	podTemplateSpec, err := buildPodTemplateSpec(dc, map[string]string{zoneLabel: "testzone"}, "testrack")

	assert.NoError(t, err, "should not have gotten error when building podTemplateSpec")

	initContainers := podTemplateSpec.Spec.InitContainers

	assert.Equal(t, 2, len(initContainers))
	assert.Equal(t, "test", initContainers[0].Name)
	assert.Equal(t, 3, len(initContainers[0].VolumeMounts))
	// We use a contains check here because the ordering is not important
	assert.True(t, volumeMountsContains(initContainers[0].VolumeMounts, volumeMountNameMatcher(PvcName)))
	assert.True(t, volumeMountsContains(initContainers[0].VolumeMounts, volumeMountNameMatcher("test-data")))
	assert.True(t, volumeMountsContains(initContainers[0].VolumeMounts, volumeMountNameMatcher("server-config")))

	assert.Equal(t, ServerConfigContainerName, initContainers[1].Name)
	assert.Equal(t, 1, len(initContainers[1].VolumeMounts))
	// We use a contains check here because the ordering is not important
	assert.True(t, volumeMountsContains(initContainers[1].VolumeMounts, volumeMountNameMatcher("server-config")))

	volumes := podTemplateSpec.Spec.Volumes
	assert.Equal(t, 4, len(volumes))
	// We use a contains check here because the ordering is not important
	assert.True(t, volumesContains(volumes, volumeNameMatcher("server-config")))
	assert.True(t, volumesContains(volumes, volumeNameMatcher("test-data")))
	assert.True(t, volumesContains(volumes, volumeNameMatcher("server-logs")))
	assert.True(t, volumesContains(volumes, volumeNameMatcher("encryption-cred-storage")))

	containers := podTemplateSpec.Spec.Containers
	assert.Equal(t, 2, len(containers))

	cassandraContainer := findContainer(containers, CassandraContainerName)
	assert.NotNil(t, cassandraContainer)

	cassandraVolumeMounts := cassandraContainer.VolumeMounts
	assert.Equal(t, 4, len(cassandraVolumeMounts))
	assert.True(t, volumeMountsContains(cassandraVolumeMounts, volumeMountNameMatcher("server-config")))
	assert.True(t, volumeMountsContains(cassandraVolumeMounts, volumeMountNameMatcher("server-logs")))
	assert.True(t, volumeMountsContains(cassandraVolumeMounts, volumeMountNameMatcher("encryption-cred-storage")))
	assert.True(t, volumeMountsContains(cassandraVolumeMounts, volumeMountNameMatcher("server-data")))

	loggerContainer := findContainer(containers, SystemLoggerContainerName)
	assert.NotNil(t, loggerContainer)

	loggerVolumeMounts := loggerContainer.VolumeMounts
	assert.Equal(t, 1, len(loggerVolumeMounts))
	assert.True(t, volumeMountsContains(loggerVolumeMounts, volumeMountNameMatcher("server-logs")))
}

func TestCassandraDatacenter_buildPodTemplateSpec_add_container_with_volumes(t *testing.T) {
	// This test adds a container, a new volume, a volume mount for the
	// new volume, and mounts for existing volumes. Not only does the test
	// verify that the container has the correct volumes, but it also verifies
	// that the "built-in" containers have the correct mounts. Note that a
	// volume mount for the new volume is also added to the cassandra container.

	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "bob",
			ServerType:    "cassandra",
			ServerVersion: "3.11.7",
			PodTemplateSpec: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "test",
							Image: "test",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "server-data",
									MountPath: "/var/lib/cassandra",
								},
								{
									Name:      "server-config",
									MountPath: "/config",
								},
								{
									Name:      "test-data",
									MountPath: "/test",
								},
							},
						},
						{
							Name: "cassandra",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "test-data",
									MountPath: "/test",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "test-data",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	podTemplateSpec, err := buildPodTemplateSpec(dc, map[string]string{zoneLabel: "testzone"}, "testrack")

	assert.NoError(t, err, "should not have gotten error when building podTemplateSpec")

	initContainers := podTemplateSpec.Spec.InitContainers

	assert.Equal(t, 1, len(initContainers))
	assert.Equal(t, ServerConfigContainerName, initContainers[0].Name)

	serverConfigInitContainer := initContainers[0]
	assert.Equal(t, 1, len(serverConfigInitContainer.VolumeMounts))
	// We use a contains check here because the ordering is not important
	assert.True(t, volumeMountsContains(serverConfigInitContainer.VolumeMounts, volumeMountNameMatcher("server-config")))

	volumes := podTemplateSpec.Spec.Volumes
	assert.Equal(t, 4, len(volumes))
	// We use a contains check here because the ordering is not important
	assert.True(t, volumesContains(volumes, volumeNameMatcher("server-config")))
	assert.True(t, volumesContains(volumes, volumeNameMatcher("test-data")))
	assert.True(t, volumesContains(volumes, volumeNameMatcher("server-logs")))
	assert.True(t, volumesContains(volumes, volumeNameMatcher("encryption-cred-storage")))

	containers := podTemplateSpec.Spec.Containers
	assert.Equal(t, 3, len(containers))

	testContainer := findContainer(containers, "test")
	assert.NotNil(t, testContainer)

	assert.Equal(t, 3, len(testContainer.VolumeMounts))
	// We use a contains check here because the ordering is not important
	assert.True(t, volumeMountsContains(testContainer.VolumeMounts, volumeMountNameMatcher(PvcName)))
	assert.True(t, volumeMountsContains(testContainer.VolumeMounts, volumeMountNameMatcher("test-data")))
	assert.True(t, volumeMountsContains(testContainer.VolumeMounts, volumeMountNameMatcher("server-config")))

	cassandraContainer := findContainer(containers, CassandraContainerName)
	assert.NotNil(t, cassandraContainer)

	cassandraVolumeMounts := cassandraContainer.VolumeMounts
	assert.Equal(t, 5, len(cassandraVolumeMounts))
	assert.True(t, volumeMountsContains(cassandraVolumeMounts, volumeMountNameMatcher("server-config")))
	assert.True(t, volumeMountsContains(cassandraVolumeMounts, volumeMountNameMatcher("server-logs")))
	assert.True(t, volumeMountsContains(cassandraVolumeMounts, volumeMountNameMatcher("encryption-cred-storage")))
	assert.True(t, volumeMountsContains(cassandraVolumeMounts, volumeMountNameMatcher("server-data")))
	assert.True(t, volumeMountsContains(cassandraVolumeMounts, volumeMountNameMatcher("test-data")))

	loggerContainer := findContainer(containers, SystemLoggerContainerName)
	assert.NotNil(t, loggerContainer)

	loggerVolumeMounts := loggerContainer.VolumeMounts
	assert.Equal(t, 1, len(loggerVolumeMounts))
	assert.True(t, volumeMountsContains(loggerVolumeMounts, volumeMountNameMatcher("server-logs")))
}

type VolumeMountMatcher func(volumeMount corev1.VolumeMount) bool

type VolumeMatcher func(volume corev1.Volume) bool

func volumeNameMatcher(name string) VolumeMatcher {
	return func(volume corev1.Volume) bool {
		return volume.Name == name
	}
}

func volumeMountNameMatcher(name string) VolumeMountMatcher {
	return func(volumeMount corev1.VolumeMount) bool {
		return volumeMount.Name == name
	}
}

func volumeMountsContains(volumeMounts []corev1.VolumeMount, matcher VolumeMountMatcher) bool {
	for _, mount := range volumeMounts {
		if matcher(mount) {
			return true
		}
	}
	return false
}

func volumesContains(volumes []corev1.Volume, matcher VolumeMatcher) bool {
	for _, volume := range volumes {
		if matcher(volume) {
			return true
		}
	}
	return false
}

func envVarsMatch(expected, actual []corev1.EnvVar) bool {
	if len(expected) != len(actual) {
		return false
	}

	for _, v := range expected {
		if !envVarsContains(actual, v) {
			return false
		}
	}

	return true
}

func envVarsContains(envVars []corev1.EnvVar, envVar corev1.EnvVar) bool {
	for _, v := range envVars {
		if v.Name == envVar.Name {
			return reflect.DeepEqual(envVar, v)
		}
	}
	return false
}

func findContainer(containers []corev1.Container, name string) *corev1.Container {
	for _, container := range containers {
		if container.Name == name {
			return &container
		}
	}
	return nil
}

func TestCassandraDatacenter_buildPodTemplateSpec_labels_merge(t *testing.T) {
	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName:     "bob",
			ServerType:      "cassandra",
			ServerVersion:   "3.11.7",
			PodTemplateSpec: &corev1.PodTemplateSpec{},
		},
	}
	dc.Spec.PodTemplateSpec.Labels = map[string]string{"abc": "123"}

	spec, err := buildPodTemplateSpec(dc, map[string]string{zoneLabel: "testzone"}, "testrack")
	got := spec.Labels

	expected := dc.GetRackLabels("testrack")
	expected[api.CassNodeState] = stateReadyToStart
	expected["app.kubernetes.io/managed-by"] = oplabels.ManagedByLabelValue
	expected["abc"] = "123"

	assert.NoError(t, err, "should not have gotten error when building podTemplateSpec")
	if !reflect.DeepEqual(expected, got) {
		t.Errorf("labels = %v, want %v", got, expected)
	}
}

func TestCassandraDatacenter_buildPodTemplateSpec_overrideSecurityContext(t *testing.T) {
	uid := int64(1111)
	gid := int64(2222)

	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "test",
			ServerType:    "cassandra",
			ServerVersion: "3.11.7",
			PodTemplateSpec: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser:  &uid,
						RunAsGroup: &gid,
					},
				},
			},
		},
	}

	spec, err := buildPodTemplateSpec(dc, map[string]string{zoneLabel: "testzone"}, "rack1")

	assert.NoError(t, err, "should not have gotten an error when building podTemplateSpec")
	assert.NotNil(t, spec)

	expected := &corev1.PodSecurityContext{
		RunAsUser:  &uid,
		RunAsGroup: &gid,
	}

	actual := spec.Spec.SecurityContext

	assert.True(t, reflect.DeepEqual(expected, actual), "SecurityContext does not match expected value")
}

func TestCassandraDatacenter_buildPodTemplateSpec_do_not_propagate_volumes(t *testing.T) {
	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "bob",
			ServerType:    "cassandra",
			ServerVersion: "3.11.7",
			PodTemplateSpec: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name: ServerConfigContainerName,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "extra",
									MountPath: "/extra",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "extra",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	spec, err := buildPodTemplateSpec(dc, map[string]string{zoneLabel: "testzone"}, "testrack")
	assert.NoError(t, err, "should not have gotten error when building podTemplateSpec")

	initContainers := spec.Spec.InitContainers

	assert.Equal(t, 1, len(initContainers))
	assert.Equal(t, ServerConfigContainerName, initContainers[0].Name)

	serverConfigInitContainer := initContainers[0]
	assert.Equal(t, 2, len(serverConfigInitContainer.VolumeMounts))
	// We use a contains check here because the ordering is not important
	assert.True(t, volumeMountsContains(serverConfigInitContainer.VolumeMounts, volumeMountNameMatcher("server-config")))
	assert.True(t, volumeMountsContains(serverConfigInitContainer.VolumeMounts, volumeMountNameMatcher("extra")))

	containers := spec.Spec.Containers
	cassandraContainer := findContainer(containers, CassandraContainerName)
	assert.NotNil(t, cassandraContainer)

	cassandraVolumeMounts := cassandraContainer.VolumeMounts
	assert.Equal(t, 4, len(cassandraVolumeMounts))
	assert.True(t, volumeMountsContains(cassandraVolumeMounts, volumeMountNameMatcher("server-config")))
	assert.True(t, volumeMountsContains(cassandraVolumeMounts, volumeMountNameMatcher("server-logs")))
	assert.True(t, volumeMountsContains(cassandraVolumeMounts, volumeMountNameMatcher("encryption-cred-storage")))
	assert.True(t, volumeMountsContains(cassandraVolumeMounts, volumeMountNameMatcher("server-data")))

	systemLoggerContainer := findContainer(containers, SystemLoggerContainerName)
	assert.NotNil(t, systemLoggerContainer)

	systemLoggerVolumeMounts := systemLoggerContainer.VolumeMounts
	assert.Equal(t, 1, len(systemLoggerVolumeMounts))
	assert.True(t, volumeMountsContains(systemLoggerVolumeMounts, volumeMountNameMatcher("server-logs")))
}

func TestCassandraDatacenter_buildContainers_DisableSystemLoggerSidecar(t *testing.T) {
	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName:                "bob",
			ServerType:                 "cassandra",
			ServerVersion:              "3.11.7",
			PodTemplateSpec:            nil,
			DisableSystemLoggerSidecar: true,
			SystemLoggerImage:          "alpine",
		},
	}

	podTemplateSpec := &corev1.PodTemplateSpec{}

	err := buildContainers(dc, podTemplateSpec)

	assert.NoError(t, err, "should not have gotten error from calling buildContainers()")

	assert.Len(t, podTemplateSpec.Spec.Containers, 1, "should have one container in the podTemplateSpec")
	assert.Equal(t, "cassandra", podTemplateSpec.Spec.Containers[0].Name)
}

func TestCassandraDatacenter_buildContainers_EnableSystemLoggerSidecar_CustomImage(t *testing.T) {
	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName:                "bob",
			ServerType:                 "cassandra",
			ServerVersion:              "3.11.7",
			PodTemplateSpec:            nil,
			DisableSystemLoggerSidecar: false,
			SystemLoggerImage:          "alpine",
		},
	}

	podTemplateSpec := &corev1.PodTemplateSpec{}

	err := buildContainers(dc, podTemplateSpec)

	assert.NoError(t, err, "should not have gotten error from calling buildContainers()")

	assert.Len(t, podTemplateSpec.Spec.Containers, 2, "should have two containers in the podTemplateSpec")
	assert.Equal(t, "cassandra", podTemplateSpec.Spec.Containers[0].Name)
	assert.Equal(t, "server-system-logger", podTemplateSpec.Spec.Containers[1].Name)

	assert.Equal(t, "alpine", podTemplateSpec.Spec.Containers[1].Image)
}

func Test_makeImage(t *testing.T) {
	type args struct {
		serverType    string
		serverImage   string
		serverVersion string
	}
	tests := []struct {
		name      string
		args      args
		want      string
		errString string
	}{
		{
			name: "test empty image",
			args: args{
				serverImage:   "",
				serverType:    "dse",
				serverVersion: "6.8.0",
			},
			want:      "datastax/dse-server:6.8.0",
			errString: "",
		},
		{
			name: "test empty image cassandra",
			args: args{
				serverImage:   "",
				serverType:    "cassandra",
				serverVersion: "3.11.7",
			},
			want:      "k8ssandra/cass-management-api:3.11.7-v0.1.25",
			errString: "",
		},
		{
			name: "test private repo server",
			args: args{
				serverImage:   "datastax.jfrog.io/secret-debug-image/dse-server:6.8.0-test123",
				serverType:    "dse",
				serverVersion: "6.8.0",
			},
			want:      "datastax.jfrog.io/secret-debug-image/dse-server:6.8.0-test123",
			errString: "",
		},
		{
			name: "test unknown dse version",
			args: args{
				serverImage:   "",
				serverType:    "dse",
				serverVersion: "6.7.0",
			},
			want:      "",
			errString: "server 'dse' and version '6.7.0' do not work together",
		},
		{
			name: "test unknown cassandra version",
			args: args{
				serverImage:   "",
				serverType:    "cassandra",
				serverVersion: "3.10.0",
			},
			want:      "",
			errString: "server 'cassandra' and version '3.10.0' do not work together",
		},
		{
			name: "test fallback",
			args: args{
				serverImage:   "",
				serverType:    "dse",
				serverVersion: "6.8.1234",
			},
			want:      "datastax/dse-server:6.8.1234",
			errString: "",
		},
		{
			name: "test cassandra fallback",
			args: args{
				serverImage:   "",
				serverType:    "cassandra",
				serverVersion: "3.11.1234",
			},
			want:      "datastax/cassandra-mgmtapi:3.11.1234",
			errString: "",
		},
		{
			name: "test 6.8.4",
			args: args{
				serverImage:   "",
				serverType:    "dse",
				serverVersion: "6.8.4",
			},
			want:      "datastax/dse-server:6.8.4",
			errString: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc := &api.CassandraDatacenter{
				Spec: api.CassandraDatacenterSpec{
					ServerType:    tt.args.serverType,
					ServerVersion: tt.args.serverVersion,
					ServerImage:   tt.args.serverImage,
				},
			}
			got, err := makeImage(dc)
			if got != tt.want {
				t.Errorf("makeImage() = %v, want %v", got, tt.want)
			}
			if err == nil {
				if tt.errString != "" {
					t.Errorf("makeImage() err = %v, want %v", err, tt.errString)
				}
			} else {
				if err.Error() != tt.errString {
					t.Errorf("makeImage() err = %v, want %v", err, tt.errString)
				}
			}
		})
	}
}

func Test_makeUbiImage(t *testing.T) {
	type args struct {
		serverType    string
		serverImage   string
		serverVersion string
	}
	tests := []struct {
		name      string
		args      args
		want      string
		errString string
	}{
		{
			name: "test fallback",
			args: args{
				serverImage:   "",
				serverType:    "dse",
				serverVersion: "6.8.1234",
			},
			want:      "datastax/dse-server:6.8.1234-ubi7",
			errString: "",
		},
		{
			name: "test cassandra fallback",
			args: args{
				serverImage:   "",
				serverType:    "cassandra",
				serverVersion: "4.0.1234",
			},
			want:      "datastax/cassandra-mgmtapi:4.0.1234-ubi7",
			errString: "",
		},
		{
			name: "test unknown dse version",
			args: args{
				serverImage:   "",
				serverType:    "dse",
				serverVersion: "6.7.0",
			},
			want:      "",
			errString: "server 'dse' and version '6.7.0' do not work together",
		},
		{
			name: "test unknown cassandra version",
			args: args{
				serverImage:   "",
				serverType:    "cassandra",
				serverVersion: "3.10.0",
			},
			want:      "",
			errString: "server 'cassandra' and version '3.10.0' do not work together",
		},
	}
	for _, tt := range tests {
		os.Setenv(images.EnvBaseImageOS, "example")

		t.Run(tt.name, func(t *testing.T) {
			dc := &api.CassandraDatacenter{
				Spec: api.CassandraDatacenterSpec{
					ServerType:    tt.args.serverType,
					ServerVersion: tt.args.serverVersion,
					ServerImage:   tt.args.serverImage,
				},
			}
			got, err := makeImage(dc)
			if got != tt.want {
				t.Errorf("makeImage() = %v, want %v", got, tt.want)
			}
			if err == nil {
				if tt.errString != "" {
					t.Errorf("makeImage() err = %v, want %v", err, tt.errString)
				}
			} else {
				if err.Error() != tt.errString {
					t.Errorf("makeImage() err = %v, want %v", err, tt.errString)
				}
			}
		})
		os.Unsetenv(images.EnvBaseImageOS)
	}
}

func TestTolerations(t *testing.T) {
	tolerations := []corev1.Toleration{
		{
			Key:      "cassandra-node",
			Operator: corev1.TolerationOpExists,
			Value:    "true",
			Effect:   corev1.TaintEffectNoExecute,
		},
		{
			Key:      "search-node",
			Operator: corev1.TolerationOpExists,
			Value:    "true",
			Effect:   corev1.TaintEffectNoSchedule,
		},
	}

	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Name:      "test",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "test",
			ServerType:    "cassandra",
			ServerVersion: "3.11.10",
			Tolerations:   tolerations,
		},
	}

	spec, err := buildPodTemplateSpec(dc, nil, "rack1")

	assert.NoError(t, err, "failed to build PodTemplateSpec")
	// using ElementsMatch instead of Equal because we do not really care about ordering.
	assert.ElementsMatch(t, tolerations, spec.Spec.Tolerations, "tolerations do not match")

	// Now verify that we cannot override the tolerations with the PodTemplateSpec property
	dc = &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Name:      "test",
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName:   "test",
			ServerType:    "cassandra",
			ServerVersion: "3.11.10",
			Tolerations:   tolerations,
			PodTemplateSpec: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Tolerations: []corev1.Toleration{
						{
							Key:      "cassandra-node",
							Operator: corev1.TolerationOpExists,
							Value:    "false",
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
		},
	}

	spec, err = buildPodTemplateSpec(dc, nil, "rack1")

	assert.NoError(t, err, "failed to build PodTemplateSpec")
	// using ElementsMatch instead of Equal because we do not really care about ordering.
	assert.ElementsMatch(t, tolerations, spec.Spec.Tolerations, "tolerations do not match")
}
