// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

// This file defines constructors for k8s objects

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/pkg/errors"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/pkg/cdc"
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
	ServerConfigContainerName            = "server-config-init"
	CassandraContainerName               = "cassandra"
	PvcName                              = "server-data"
	SystemLoggerContainerName            = "server-system-logger"
)

// calculateNodeAffinity provides a way to decide where to schedule pods within a statefulset based on labels
func calculateNodeAffinity(labels map[string]string, existingNodeAffinity *corev1.NodeAffinity, rackNodeAffinity *corev1.NodeAffinity) *corev1.NodeAffinity {
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

	newNodeAffinity := &corev1.NodeAffinity{}

	if len(nodeSelectors) > 0 {
		newNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: nodeSelectors,
				},
			},
		}
	}

	// Merge nodeSelectorTerms
	if existingNodeAffinity != nil {
		if newNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
			newNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = existingNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
		} else if existingNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil && len(existingNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms) > 0 {
			newNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = append(newNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms, existingNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms...)
		}

		if existingNodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution != nil {
			newNodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution = existingNodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution
		}
	}

	if rackNodeAffinity != nil {
		if newNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
			newNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = rackNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
		} else if rackNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil && len(rackNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms) > 0 {
			newNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = append(newNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms, rackNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms...)
		}

		if rackNodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution != nil {
			if newNodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution != nil {
				newNodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution = append(newNodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution, rackNodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution...)
			} else {
				newNodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution = rackNodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution
			}
		}
	}

	// This is to preserve old behavior
	if len(newNodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution) < 1 && newNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return nil
	}

	return newNodeAffinity
}

// calculatePodAntiAffinity provides a way to keep the db pods of a statefulset away from other db pods
func calculatePodAntiAffinity(allowMultipleNodesPerWorker bool, existingAntiAffinity *corev1.PodAntiAffinity, rackAntiAffinity *corev1.PodAntiAffinity) *corev1.PodAntiAffinity {
	antiAffinity := &corev1.PodAntiAffinity{}
	if !allowMultipleNodesPerWorker {
		antiAffinity = &corev1.PodAntiAffinity{
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

	// Preserve old behavior
	if allowMultipleNodesPerWorker && (existingAntiAffinity == nil && rackAntiAffinity == nil) {
		return nil
	}

	// Add PodTemplateSpec rules first
	if existingAntiAffinity != nil {
		if antiAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
			antiAffinity.RequiredDuringSchedulingIgnoredDuringExecution = existingAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution
		} else {
			if len(existingAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution) > 0 {
				antiAffinity.RequiredDuringSchedulingIgnoredDuringExecution = append(antiAffinity.RequiredDuringSchedulingIgnoredDuringExecution, existingAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution...)
			}
		}
		antiAffinity.PreferredDuringSchedulingIgnoredDuringExecution = existingAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution
	}

	// Then rack.Affinity
	if rackAntiAffinity != nil {
		if antiAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
			antiAffinity.RequiredDuringSchedulingIgnoredDuringExecution = rackAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution
		} else {
			if len(rackAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution) > 0 {
				antiAffinity.RequiredDuringSchedulingIgnoredDuringExecution = append(antiAffinity.RequiredDuringSchedulingIgnoredDuringExecution, rackAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution...)
			}
		}

		if antiAffinity.PreferredDuringSchedulingIgnoredDuringExecution == nil {
			antiAffinity.PreferredDuringSchedulingIgnoredDuringExecution = rackAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution
		} else {
			if len(rackAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution) > 0 {
				antiAffinity.PreferredDuringSchedulingIgnoredDuringExecution = append(antiAffinity.PreferredDuringSchedulingIgnoredDuringExecution, rackAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution...)
			}
		}
	}

	return antiAffinity
}

func selectorFromFieldPath(fieldPath string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		FieldRef: &corev1.ObjectFieldSelector{
			FieldPath: fieldPath,
		},
	}
}

func httpGetAction(port int, path string) *corev1.HTTPGetAction {
	return &corev1.HTTPGetAction{
		Port: intstr.FromInt(port),
		Path: path,
	}
}

func probe(port int, path string, initDelay int, period int, timeout int) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: httpGetAction(port, path),
		},
		InitialDelaySeconds: int32(initDelay),
		PeriodSeconds:       int32(period),
		TimeoutSeconds:      int32(timeout),
	}
}

func getJvmExtraOpts(dc *api.CassandraDatacenter) string {
	flags := ""

	if dc.Spec.DseWorkloads.AnalyticsEnabled {
		flags += "-Dspark-trackers=true "
	}
	if dc.Spec.DseWorkloads.GraphEnabled {
		flags += "-Dgraph-enabled=true "
	}
	if dc.Spec.DseWorkloads.SearchEnabled {
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

func addVolumes(dc *api.CassandraDatacenter, baseTemplate *corev1.PodTemplateSpec, addLegacyInternodeMount bool) {
	vServerConfig := corev1.Volume{
		Name: "server-config",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}

	vServerLogs := corev1.Volume{
		Name: "server-logs",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}

	volumeDefaults := []corev1.Volume{vServerConfig, vServerLogs}

	if addLegacyInternodeMount {
		vServerEncryption := corev1.Volume{
			Name: "encryption-cred-storage",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: fmt.Sprintf("%s-keystore", dc.SanitizedName()),
				},
			},
		}

		volumeDefaults = append(volumeDefaults, vServerEncryption)
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
		Name:      "server-config",
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

	envDefaults = append(envDefaults, configEnvVar...)

	serverCfg.Env = combineEnvSlices(envDefaults, serverCfg.Env)

	if !foundOverrides {
		// Note that append makes a copy, so we must do this after
		// serverCfg has been properly set up.
		baseTemplate.Spec.InitContainers = append(baseTemplate.Spec.InitContainers, *serverCfg)
	}

	return nil
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

		return nil, fmt.Errorf("datacenter %s is missing %s annotation", dc.SanitizedName(), api.ConfigHashAnnotation)
	}

	configData, err := dc.GetConfigAsJSON(dc.Spec.Config)
	if err != nil {
		return envVars, err
	}
	cdcAdded, err := cdc.UpdateConfig(json.RawMessage(configData), *dc)
	if err != nil {
		return envVars, err
	}
	envVars = append(envVars, corev1.EnvVar{Name: "CONFIG_FILE_DATA", Value: string(cdcAdded)})

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
		cassContainer.LivenessProbe = probe(8080, httphelper.LivenessEndpoint, 15, 15, 10)
	}

	if cassContainer.ReadinessProbe == nil {
		cassContainer.ReadinessProbe = probe(8080, httphelper.ReadinessEndpoint, 20, 10, 10)
	}

	if cassContainer.Lifecycle == nil {
		cassContainer.Lifecycle = &corev1.Lifecycle{}
	}

	if cassContainer.Lifecycle.PreStop == nil {
		action, err := httphelper.GetMgmtApiWgetPostAction(dc, httphelper.NodeDrainEndpoint, "", 0)
		if err != nil {
			return err
		}
		cassContainer.Lifecycle.PreStop = &corev1.LifecycleHandler{
			Exec: action,
		}
	}

	// Combine env vars

	envDefaults := []corev1.EnvVar{
		{Name: "POD_NAME", ValueFrom: selectorFromFieldPath("metadata.name")},
		{Name: "NODE_NAME", ValueFrom: selectorFromFieldPath("spec.nodeName")},
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
		Name:      "server-config",
		MountPath: "/config",
	}
	volumeDefaults = append(volumeDefaults, serverCfgMount)

	cassServerLogsMount := corev1.VolumeMount{
		Name:      "server-logs",
		MountPath: "/var/log/cassandra",
	}

	volumeMounts := combineVolumeMountSlices(volumeDefaults,
		[]corev1.VolumeMount{
			cassServerLogsMount,
			{
				Name:      PvcName,
				MountPath: "/var/lib/cassandra",
			},
		})

	for _, vol := range baseTemplate.Spec.Volumes {
		if vol.Name == "encryption-cred-storage" {
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      "encryption-cred-storage",
				MountPath: "/etc/encryption/",
			})
			break
		}
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

func buildPodTemplateSpec(dc *api.CassandraDatacenter, rack api.Rack, addLegacyInternodeMount bool) (*corev1.PodTemplateSpec, error) {

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
		// workaround for https://cloud.google.com/kubernetes-engine/docs/security-bulletins#may-31-2019
		if shouldDefineSecurityContext(dc) {
			var userID int64 = 999
			baseTemplate.Spec.SecurityContext = &corev1.PodSecurityContext{
				RunAsUser:  &userID,
				RunAsGroup: &userID,
				FSGroup:    &userID,
			}
		}
	}

	// Adds custom registry pull secret if needed

	_ = images.AddDefaultRegistryImagePullSecrets(&baseTemplate.Spec)

	// Labels

	podLabels := dc.GetRackLabels(rack.Name)
	oplabels.AddOperatorLabels(podLabels, dc)
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
	nodeAffinityLabels, nodeAffinityLabelsConfigurationError := rackNodeAffinitylabels(dc, rack.Name)
	if nodeAffinityLabelsConfigurationError != nil {
		return nil, nodeAffinityLabelsConfigurationError
	}

	rackAffinities := rack.Affinity

	baseTemplate.Spec.Affinity = buildAffinity(baseTemplate.Spec.Affinity, rackAffinities, nodeAffinityLabels, dc.Spec.AllowMultipleNodesPerWorker)

	// Tolerations
	baseTemplate.Spec.Tolerations = dc.Spec.Tolerations

	// Volumes

	addVolumes(dc, baseTemplate, addLegacyInternodeMount)

	// Init Containers

	err := buildInitContainers(dc, rack.Name, baseTemplate)
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

func buildAffinity(affinity *corev1.Affinity, rackAffinity *corev1.Affinity, nodeAffinityLabels map[string]string, allowMultipleWorkers bool) *corev1.Affinity {
	if affinity == nil {
		affinity = &corev1.Affinity{}
	}
	var rackPodAntiAffinity *corev1.PodAntiAffinity
	var rackNodeAffinity *corev1.NodeAffinity
	if rackAffinity != nil {
		rackPodAntiAffinity = rackAffinity.PodAntiAffinity
		rackNodeAffinity = rackAffinity.NodeAffinity
	}
	affinity.NodeAffinity = calculateNodeAffinity(nodeAffinityLabels, affinity.NodeAffinity, rackNodeAffinity)
	affinity.PodAntiAffinity = calculatePodAntiAffinity(allowMultipleWorkers, affinity.PodAntiAffinity, rackPodAntiAffinity)

	return affinity
}
