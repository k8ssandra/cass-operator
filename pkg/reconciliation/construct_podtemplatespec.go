// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

// This file defines constructors for k8s objects

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/pkg/errors"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/pkg/httphelper"
	"github.com/k8ssandra/cass-operator/pkg/images"
	"github.com/k8ssandra/cass-operator/pkg/oplabels"
	"github.com/k8ssandra/cass-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	DefaultTerminationGracePeriodSeconds = 120
	baseConfigInitContainerName          = "base-config-init"
	ServerConfigContainerName            = "server-config-init"
	CassandraContainerName               = "cassandra"
	PvcName                              = "server-data"
	SystemLoggerContainerName            = "server-system-logger"
	cassandraUid                         = int64(999)
	cassandraGid                         = int64(999)
	serverConfigInitVol                  = "server-config"
	serverConfigVol                      = "config"
	serverLogsVol                        = "server-logs"
	encyptionCredsVol                    = "encryption-cred-storage"
	tmpVol                               = "tmp"
	dseBinVol                            = "dse-bin"
	mcacConfigVol                        = "mcac-config"
	cassandraHomeVol                     = "cassandra-home"
)

// calculateNodeAffinity provides a way to decide where to schedule pods within a statefulset based on labels
func calculateNodeAffinity(labels map[string]string) *corev1.NodeAffinity {
	if len(labels) == 0 {
		return nil
	}

	var nodeSelectors []corev1.NodeSelectorRequirement

	//we make a new map in order to sort because a map is random by design
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys) // Keep labels in the same order across statefulsets
	for _, key := range keys {
		selector := corev1.NodeSelectorRequirement{
			Key:      key,
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{labels[key]},
		}
		nodeSelectors = append(nodeSelectors, selector)
	}

	return &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: nodeSelectors,
				},
			},
		},
	}
}

// calculatePodAntiAffinity provides a way to keep the db pods of a statefulset away from other db pods
func calculatePodAntiAffinity(allowMultipleNodesPerWorker bool) *corev1.PodAntiAffinity {
	if allowMultipleNodesPerWorker {
		return nil
	}
	return &corev1.PodAntiAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
			{
				LabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      api.ClusterLabel,
							Operator: metav1.LabelSelectorOpExists,
						},
						{
							Key:      api.DatacenterLabel,
							Operator: metav1.LabelSelectorOpExists,
						},
						{
							Key:      api.RackLabel,
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				},
				TopologyKey: "kubernetes.io/hostname",
			},
		},
	}
}

func selectorFromFieldPath(fieldPath string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		FieldRef: &corev1.ObjectFieldSelector{
			FieldPath: fieldPath,
		},
	}
}

func probe(port int, path string, initDelay int, period int) *corev1.Probe {
	return &corev1.Probe{
		Handler: corev1.Handler{
			HTTPGet: &corev1.HTTPGetAction{
				Port: intstr.FromInt(port),
				Path: path,
			},
		},
		InitialDelaySeconds: int32(initDelay),
		PeriodSeconds:       int32(period),
	}
}

func getJvmExtraOpts(dc *api.CassandraDatacenter) string {
	flags := ""

	if dc.Spec.DseWorkloads.AnalyticsEnabled == true {
		flags += "-Dspark-trackers=true "
	}
	if dc.Spec.DseWorkloads.GraphEnabled == true {
		flags += "-Dgraph-enabled=true "
	}
	if dc.Spec.DseWorkloads.SearchEnabled == true {
		flags += "-Dsearch-service=true"
	}
	return flags
}

func combineVolumeMountSlices(defaults []corev1.VolumeMount, overrides []corev1.VolumeMount) []corev1.VolumeMount {
	out := append([]corev1.VolumeMount{}, overrides...)
outerLoop:
	// Only add the defaults that don't have an override
	for _, volumeDefault := range defaults {
		for _, volumeOverride := range overrides {
			if volumeDefault.Name == volumeOverride.Name {
				continue outerLoop
			}
		}
		out = append(out, volumeDefault)
	}
	return out
}

func combineVolumeSlices(defaults []corev1.Volume, overrides []corev1.Volume) []corev1.Volume {
	out := append([]corev1.Volume{}, overrides...)
outerLoop:
	// Only add the defaults that don't have an override
	for _, volumeDefault := range defaults {
		for _, volumeOverride := range overrides {
			if volumeDefault.Name == volumeOverride.Name {
				continue outerLoop
			}
		}
		out = append(out, volumeDefault)
	}
	return out
}

func combinePortSlices(defaults []corev1.ContainerPort, overrides []corev1.ContainerPort) []corev1.ContainerPort {
	out := append([]corev1.ContainerPort{}, overrides...)
outerLoop:
	// Only add the defaults that don't have an override
	for _, portDefault := range defaults {
		for _, portOverride := range overrides {
			if portDefault.Name == portOverride.Name {
				continue outerLoop
			}
		}
		out = append(out, portDefault)
	}
	return out
}

func combineEnvSlices(defaults []corev1.EnvVar, overrides []corev1.EnvVar) []corev1.EnvVar {
	out := append([]corev1.EnvVar{}, overrides...)
outerLoop:
	// Only add the defaults that don't have an override
	for _, envDefault := range defaults {
		for _, envOverride := range overrides {
			if envDefault.Name == envOverride.Name {
				continue outerLoop
			}
		}
		out = append(out, envDefault)
	}
	return out
}

func generateStorageConfigVolumesMount(cc *api.CassandraDatacenter) []corev1.VolumeMount {
	var vms []corev1.VolumeMount
	for _, storage := range cc.Spec.StorageConfig.AdditionalVolumes {
		vms = append(vms, corev1.VolumeMount{Name: storage.Name, MountPath: storage.MountPath})
	}
	return vms
}

func generateStorageConfigEmptyVolumes(cc *api.CassandraDatacenter) []corev1.Volume {
	var volumes []corev1.Volume
	for _, storage := range cc.Spec.StorageConfig.AdditionalVolumes {
		volumes = append(volumes, corev1.Volume{Name: storage.Name})
	}
	return volumes
}

func addVolumes(dc *api.CassandraDatacenter, baseTemplate *corev1.PodTemplateSpec) {
	// The Cassandra or DSE base configuration files are copied onto this volume from an
	// init container. The cassandra container will mount this volume at /etc/cassandra
	// for Cassandra and at /opt/dse/resources for DSE.
	//baseConfig := corev1.Volume{
	//	Name: "base-config",
	//	VolumeSource: corev1.VolumeSource{
	//		EmptyDir: &corev1.EmptyDirVolumeSource{},
	//	},
	//}

	// The server-config-init init container writes config files here. The entry point
	// script for Cassandra images copies the files from this volume into /etc/cassandra.
	// It doesn't look like the DSE entry point script performs a similar copy operation.
	vServerConfig := corev1.Volume{
		Name: serverConfigInitVol,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}

	// This is mounted at /opt/dse/resources for DSE and /etc/cassandra for Cassandra. It
	// contains the base configs along with any custom configs generated by the
	// server-config-init init container.
	config := corev1.Volume{
		Name: serverConfigVol,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}

	// The management-api stores socket files in /tmp so we need a volume for it.
	tmp := corev1.Volume{
		Name: tmpVol,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}

	vServerLogs := corev1.Volume{
		Name: serverLogsVol,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}

	vServerEncryption := corev1.Volume{
		Name: encyptionCredsVol,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: fmt.Sprintf("%s-keystore", dc.Name),
			},
		},
	}

	volumeDefaults := []corev1.Volume{vServerConfig, config, tmp, vServerLogs, vServerEncryption}

	if dc.Spec.ServerType == "dse" {
		// The entry point script for DSE creates a sym link at /opt/dse/bin/dse-env. We
		// therefore need a separate volume for the bin directory.
		volumeDefaults = append(volumeDefaults, corev1.Volume{
			Name: dseBinVol,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	} else {
		// management-api entry point script updates the MCAC config which is located at
		// /opt/metrics-collector.
		volumeDefaults = append(volumeDefaults, corev1.Volume{
			Name: mcacConfigVol,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})

		// management-api requires Cassandra home dir to be writeable.
		volumeDefaults = append(volumeDefaults, corev1.Volume{
			Name: cassandraHomeVol,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	volumeDefaults = combineVolumeSlices(
		volumeDefaults, baseTemplate.Spec.Volumes)

	baseTemplate.Spec.Volumes = symmetricDifference(volumeDefaults, generateStorageConfigEmptyVolumes(dc))
}

func symmetricDifference(list1 []corev1.Volume, list2 []corev1.Volume) []corev1.Volume {
	out := []corev1.Volume{}
	for _, volume := range list1 {
		found := false
		for _, storage := range list2 {
			if storage.Name == volume.Name {
				found = true
				break
			}
		}
		if !found {
			out = append(out, volume)
		}
	}
	return out
}

// This ensure that the server-config-builder init container is properly configured.
func buildInitContainers(dc *api.CassandraDatacenter, rackName string, baseTemplate *corev1.PodTemplateSpec) error {
	baseConfig, err := createBaseConfigInitContainer(dc)
	if err != nil {
		return err
	}

	// We want to always include the base config init container since it provides the
	// base or default configuration.
	baseTemplate.Spec.InitContainers = insertContainer(baseTemplate.Spec.InitContainers, *baseConfig, 0)

	serverCfg := &corev1.Container{}
	foundOverrides := false

	for i, c := range baseTemplate.Spec.InitContainers {
		if c.Name == ServerConfigContainerName {
			// Modify the existing container
			foundOverrides = true
			serverCfg = &baseTemplate.Spec.InitContainers[i]
			break
		}
	}

	serverCfg.Name = ServerConfigContainerName

	if serverCfg.SecurityContext == nil {
		serverCfg.SecurityContext = newSecurityContext()
	}

	if serverCfg.Image == "" {
		if dc.GetConfigBuilderImage() != "" {
			serverCfg.Image = dc.GetConfigBuilderImage()
		} else {
			serverCfg.Image = images.GetConfigBuilderImage()
		}
		if images.GetImageConfig() != nil && images.GetImageConfig().ImagePullPolicy != "" {
			serverCfg.ImagePullPolicy = images.GetImageConfig().ImagePullPolicy
		}
	}

	serverCfgMount := corev1.VolumeMount{
		Name:      serverConfigInitVol,
		MountPath: "/config",
	}

	serverCfg.VolumeMounts = combineVolumeMountSlices([]corev1.VolumeMount{serverCfgMount}, serverCfg.VolumeMounts)

	serverCfg.Resources = *getResourcesOrDefault(&dc.Spec.ConfigBuilderResources, &DefaultsConfigInitContainer)

	// Convert the bool to a string for the env var setting
	useHostIpForBroadcast := "false"
	if dc.IsNodePortEnabled() {
		useHostIpForBroadcast = "true"
	}

	configEnvVar, err := getConfigDataEnVars(dc)
	if err != nil {
		return errors.Wrap(err, "failed to get config env vars")
	}

	serverVersion := dc.Spec.ServerVersion

	envDefaults := []corev1.EnvVar{
		{Name: "POD_IP", ValueFrom: selectorFromFieldPath("status.podIP")},
		{Name: "HOST_IP", ValueFrom: selectorFromFieldPath("status.hostIP")},
		{Name: "USE_HOST_IP_FOR_BROADCAST", Value: useHostIpForBroadcast},
		{Name: "RACK_NAME", Value: rackName},
		{Name: "PRODUCT_VERSION", Value: serverVersion},
		{Name: "PRODUCT_NAME", Value: dc.Spec.ServerType},
		// TODO remove this post 1.0
		{Name: "DSE_VERSION", Value: serverVersion},
	}

	for _, envVar := range configEnvVar {
		envDefaults = append(envDefaults, envVar)
	}

	serverCfg.Env = combineEnvSlices(envDefaults, serverCfg.Env)

	if !foundOverrides {
		// Note that append makes a copy, so we must do this after
		// serverCfg has been properly set up.
		baseTemplate.Spec.InitContainers = append(baseTemplate.Spec.InitContainers, *serverCfg)
	}

	return nil
}

func createBaseConfigInitContainer(dc *api.CassandraDatacenter) (*corev1.Container, error) {
	baseConfig := corev1.Container{}
	baseConfig.Name = baseConfigInitContainerName

	image, err := images.GetCassandraImage(dc.Spec.ServerType, dc.Spec.ServerVersion)
	if err != nil {
		return nil, err
	}
	baseConfig.Image = image

	baseConfig.SecurityContext = newSecurityContext()
	baseConfig.ImagePullPolicy = images.GetImageConfig().ImagePullPolicy
	baseConfig.Command = []string{"/bin/sh"}

	if dc.Spec.ServerType == "dse" {
		baseConfig.Args = []string{"-c", "cp -r /opt/dse/resources/* /base-config && cp -r /opt/dse/bin/* /base-bin"}
		baseConfig.VolumeMounts = []corev1.VolumeMount{
			{
				Name:      serverConfigVol,
				MountPath: "/base-config",
			},
			{
				// Needed because the symlink /opt/dse/bin/dse-env.sh is created to point to
				// /config/dse-env.sh
				Name:      dseBinVol,
				MountPath: "/base-bin",
			},
		}
	} else {
		baseConfig.Args = []string{"-c", "cp -r /opt/cassandra/* /cassandra && cp -R /opt/cassandra/conf/* /cassandra-base-config && cp -r /opt/metrics-collector/config/* /mcac-config"}
		baseConfig.VolumeMounts = []corev1.VolumeMount{
			{
				Name:      cassandraHomeVol,
				MountPath: "/cassandra",
			},
			{
				Name:      serverConfigVol,
				MountPath: "/cassandra-base-config",
			},
			{
				Name:      mcacConfigVol,
				MountPath: "/mcac-config",
			},
		}
	}

	return &baseConfig, nil
}

func getConfigDataEnVars(dc *api.CassandraDatacenter) ([]corev1.EnvVar, error) {
	envVars := make([]corev1.EnvVar, 0)

	if len(dc.Spec.ConfigSecret) > 0 {
		envVars = append(envVars, corev1.EnvVar{
			Name: "CONFIG_FILE_DATA",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: getDatacenterConfigSecretName(dc),
					},
					Key: "config",
				},
			},
		})

		if configHash, ok := dc.Annotations[api.ConfigHashAnnotation]; ok {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "CONFIG_HASH",
				Value: configHash,
			})
			return envVars, nil
		}

		return nil, fmt.Errorf("datacenter %s is missing %s annotation", dc.Name, api.ConfigHashAnnotation)
	}

	configData, err := dc.GetConfigAsJSON(dc.Spec.Config)

	if err != nil {
		return envVars, err
	}
	envVars = append(envVars, corev1.EnvVar{Name: "CONFIG_FILE_DATA", Value: configData})

	return envVars, nil
}

// makeImage takes the server type/version and image from the spec,
// and returns a docker pullable server container image
// serverVersion should be a semver-like string
// serverImage should be an empty string, or [hostname[:port]/][path/with/repo]:[Server container img tag]
// If serverImage is empty, we attempt to find an appropriate container image based on the serverVersion
// In the event that no image is found, an error is returned
func makeImage(dc *api.CassandraDatacenter) (string, error) {
	if dc.GetServerImage() == "" {
		return images.GetCassandraImage(dc.Spec.ServerType, dc.Spec.ServerVersion)
	}
	return dc.GetServerImage(), nil
}

// If values are provided in the matching containers in the
// PodTemplateSpec field of the dc, they will override defaults.
func buildContainers(dc *api.CassandraDatacenter, baseTemplate *corev1.PodTemplateSpec) error {

	// Create new Container structs or get references to existing ones

	cassContainer := &corev1.Container{}
	loggerContainer := &corev1.Container{}

	foundCass := false
	foundLogger := false
	for i, c := range baseTemplate.Spec.Containers {
		if c.Name == CassandraContainerName {
			foundCass = true
			cassContainer = &baseTemplate.Spec.Containers[i]
		} else if c.Name == SystemLoggerContainerName {
			foundLogger = true
			loggerContainer = &baseTemplate.Spec.Containers[i]
		}
	}

	// Cassandra container

	cassContainer.Name = CassandraContainerName
	if cassContainer.Image == "" {
		serverImage, err := makeImage(dc)
		if err != nil {
			return err
		}

		cassContainer.Image = serverImage
		if images.GetImageConfig() != nil && images.GetImageConfig().ImagePullPolicy != "" {
			cassContainer.ImagePullPolicy = images.GetImageConfig().ImagePullPolicy
		}
	}

	if reflect.DeepEqual(cassContainer.Resources, corev1.ResourceRequirements{}) {
		cassContainer.Resources = dc.Spec.Resources
	}

	if cassContainer.LivenessProbe == nil {
		cassContainer.LivenessProbe = probe(8080, "/api/v0/probes/liveness", 15, 15)
	}

	if cassContainer.ReadinessProbe == nil {
		cassContainer.ReadinessProbe = probe(8080, "/api/v0/probes/readiness", 20, 10)
	}

	if cassContainer.Lifecycle == nil {
		cassContainer.Lifecycle = &corev1.Lifecycle{}
	}

	if cassContainer.Lifecycle.PreStop == nil {
		action, err := httphelper.GetMgmtApiWgetPostAction(dc, httphelper.WgetNodeDrainEndpoint, "")
		if err != nil {
			return err
		}
		cassContainer.Lifecycle.PreStop = &corev1.Handler{
			Exec: action,
		}
	}

	if cassContainer.SecurityContext == nil {
		cassContainer.SecurityContext = newSecurityContext()
	}

	// Combine env vars

	envDefaults := []corev1.EnvVar{
		{Name: "DS_LICENSE", Value: "accept"},
		{Name: "DSE_AUTO_CONF_OFF", Value: "all"},
		{Name: "USE_MGMT_API", Value: "true"},
		{Name: "MGMT_API_EXPLICIT_START", Value: "true"},
		// TODO remove this post 1.0
		{Name: "DSE_MGMT_EXPLICIT_START", Value: "true"},
	}

	if dc.Spec.ServerType == "dse" && dc.Spec.DseWorkloads != nil {
		envDefaults = append(
			envDefaults,
			corev1.EnvVar{Name: "JVM_EXTRA_OPTS", Value: getJvmExtraOpts(dc)})
	}

	cassContainer.Env = combineEnvSlices(envDefaults, cassContainer.Env)

	// Combine ports

	portDefaults, err := dc.GetContainerPorts()
	if err != nil {
		return err
	}

	cassContainer.Ports = combinePortSlices(portDefaults, cassContainer.Ports)

	// Combine volumeMounts

	var volumeDefaults []corev1.VolumeMount
	serverCfgMount := corev1.VolumeMount{
		Name:      serverConfigInitVol,
		MountPath: "/config",
	}
	volumeDefaults = append(volumeDefaults, serverCfgMount)

	cassServerLogsMount := corev1.VolumeMount{
		Name:      serverLogsVol,
		MountPath: "/var/log/cassandra",
	}

	volumeMounts := combineVolumeMountSlices(volumeDefaults,
		[]corev1.VolumeMount{
			cassServerLogsMount,
			{
				Name:      PvcName,
				MountPath: "/var/lib/cassandra",
			},
			{
				// The keystore and truststore files for internode encryption are stored
				// here.
				Name:      encyptionCredsVol,
				MountPath: "/etc/encryption/",
			},
			{
				// The management-api stores socket files in /tmp.
				Name:      tmpVol,
				MountPath: "/tmp",
			},
		})

	if dc.Spec.ServerType == "dse" {
		confiigVol := corev1.VolumeMount{
			Name:      serverConfigVol,
			MountPath: "/opt/dse/resources",
		}
		binVol := corev1.VolumeMount{
			Name:      dseBinVol,
			MountPath: "/opt/dse/bin",
		}
		cassContainer.VolumeMounts = append(cassContainer.VolumeMounts, confiigVol, binVol)
	} else {
		configVol := corev1.VolumeMount{
			Name:      serverConfigVol,
			MountPath: "/opt/cassandra/conf",
		}

		mcacVol := corev1.VolumeMount{
			Name:      mcacConfigVol,
			MountPath: "/opt/metrics-collector/config",
		}

		cassandraHomeVol := corev1.VolumeMount{
			Name:      "cassandra-home",
			MountPath: "/opt/cassandra",
		}

		cassContainer.VolumeMounts = append(cassContainer.VolumeMounts, configVol, mcacVol, cassandraHomeVol)
	}

	volumeMounts = combineVolumeMountSlices(volumeMounts, cassContainer.VolumeMounts)
	cassContainer.VolumeMounts = combineVolumeMountSlices(volumeMounts, generateStorageConfigVolumesMount(dc))

	// Server Logger Container

	loggerContainer.Name = SystemLoggerContainerName

	if loggerContainer.Image == "" {
		specImage := dc.Spec.SystemLoggerImage
		if specImage != "" {
			loggerContainer.Image = specImage
		} else {
			loggerContainer.Image = images.GetSystemLoggerImage()
		}
		if images.GetImageConfig() != nil && images.GetImageConfig().ImagePullPolicy != "" {
			loggerContainer.ImagePullPolicy = images.GetImageConfig().ImagePullPolicy
		}
	}

	if loggerContainer.SecurityContext == nil {
		loggerContainer.SecurityContext = newSecurityContext()
	}

	volumeMounts = combineVolumeMountSlices([]corev1.VolumeMount{cassServerLogsMount}, loggerContainer.VolumeMounts)

	loggerContainer.VolumeMounts = combineVolumeMountSlices(volumeMounts, generateStorageConfigVolumesMount(dc))

	loggerContainer.Resources = *getResourcesOrDefault(&dc.Spec.SystemLoggerResources, &DefaultsLoggerContainer)

	// Note that append() can make copies of each element,
	// so we call it after modifying any existing elements.

	if !foundCass {
		baseTemplate.Spec.Containers = append(baseTemplate.Spec.Containers, *cassContainer)
	}

	if !dc.Spec.DisableSystemLoggerSidecar {
		if !foundLogger {
			baseTemplate.Spec.Containers = append(baseTemplate.Spec.Containers, *loggerContainer)
		}
	}

	return nil
}

func buildPodTemplateSpec(dc *api.CassandraDatacenter, nodeAffinityLabels map[string]string,
	rackName string) (*corev1.PodTemplateSpec, error) {

	baseTemplate := dc.Spec.PodTemplateSpec.DeepCopy()

	if baseTemplate == nil {
		baseTemplate = &corev1.PodTemplateSpec{}
	}

	// Service Account

	serviceAccount := "default"
	if dc.Spec.ServiceAccount != "" {
		serviceAccount = dc.Spec.ServiceAccount
	}
	baseTemplate.Spec.ServiceAccountName = serviceAccount

	// Host networking

	if dc.IsHostNetworkEnabled() {
		baseTemplate.Spec.HostNetwork = true
		baseTemplate.Spec.DNSPolicy = corev1.DNSClusterFirstWithHostNet
	}

	if baseTemplate.Spec.TerminationGracePeriodSeconds == nil {
		// Note: we cannot take the address of a constant
		gracePeriodSeconds := int64(DefaultTerminationGracePeriodSeconds)
		baseTemplate.Spec.TerminationGracePeriodSeconds = &gracePeriodSeconds
	}

	if baseTemplate.Spec.SecurityContext == nil {
		baseTemplate.Spec.SecurityContext = defaultPodSecurityContext()
	}

	// Adds custom registry pull secret if needed

	_ = images.AddDefaultRegistryImagePullSecrets(&baseTemplate.Spec)

	// Labels

	podLabels := dc.GetRackLabels(rackName)
	oplabels.AddManagedByLabel(podLabels)
	podLabels[api.CassNodeState] = stateReadyToStart

	if baseTemplate.Labels == nil {
		baseTemplate.Labels = make(map[string]string)
	}
	baseTemplate.Labels = utils.MergeMap(baseTemplate.Labels, podLabels)

	// Annotations

	podAnnotations := map[string]string{}

	if baseTemplate.Annotations == nil {
		baseTemplate.Annotations = make(map[string]string)
	}
	baseTemplate.Annotations = utils.MergeMap(baseTemplate.Annotations, podAnnotations)

	// Affinity

	affinity := &corev1.Affinity{}
	affinity.NodeAffinity = calculateNodeAffinity(nodeAffinityLabels)
	affinity.PodAntiAffinity = calculatePodAntiAffinity(dc.Spec.AllowMultipleNodesPerWorker)
	baseTemplate.Spec.Affinity = affinity

	// Tolerations
	baseTemplate.Spec.Tolerations = dc.Spec.Tolerations

	// Volumes

	addVolumes(dc, baseTemplate)

	// Init Containers

	err := buildInitContainers(dc, rackName, baseTemplate)
	if err != nil {
		return nil, err
	}

	// Containers

	err = buildContainers(dc, baseTemplate)
	if err != nil {
		return nil, err
	}

	return baseTemplate, nil
}

func newSecurityContext() *corev1.SecurityContext {
	readOnlyRootFs := true
	privileged := false
	allowPrivilegedEscalation := false

	return &corev1.SecurityContext{
		ReadOnlyRootFilesystem:   &readOnlyRootFs,
		Privileged:               &privileged,
		AllowPrivilegeEscalation: &allowPrivilegedEscalation,
	}
}

func defaultPodSecurityContext() *corev1.PodSecurityContext {
	uid := cassandraUid
	gid := cassandraGid
	fsGroup := cassandraUid
	runAsNonRoot := true

	return &corev1.PodSecurityContext{
		RunAsUser:    &uid,
		RunAsGroup:   &gid,
		FSGroup:      &fsGroup,
		RunAsNonRoot: &runAsNonRoot,
	}
}

func insertContainer(containers []corev1.Container, container corev1.Container, position int) []corev1.Container {
	newContainers := make([]corev1.Container, len(containers)+1)
	newContainers[0] = container
	for i, c := range containers {
		newContainers[i+1] = c
	}
	return newContainers
}
