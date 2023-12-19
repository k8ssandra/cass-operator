//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by controller-gen. DO NOT EDIT.

package v1beta1

import (
	"encoding/json"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AdditionalVolumes) DeepCopyInto(out *AdditionalVolumes) {
	*out = *in
	if in.PVCSpec != nil {
		in, out := &in.PVCSpec, &out.PVCSpec
		*out = new(v1.PersistentVolumeClaimSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.VolumeSource != nil {
		in, out := &in.VolumeSource, &out.VolumeSource
		*out = new(v1.VolumeSource)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AdditionalVolumes.
func (in *AdditionalVolumes) DeepCopy() *AdditionalVolumes {
	if in == nil {
		return nil
	}
	out := new(AdditionalVolumes)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in AdditionalVolumesSlice) DeepCopyInto(out *AdditionalVolumesSlice) {
	{
		in := &in
		*out = make(AdditionalVolumesSlice, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AdditionalVolumesSlice.
func (in AdditionalVolumesSlice) DeepCopy() AdditionalVolumesSlice {
	if in == nil {
		return nil
	}
	out := new(AdditionalVolumesSlice)
	in.DeepCopyInto(out)
	return *out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CDCConfiguration) DeepCopyInto(out *CDCConfiguration) {
	*out = *in
	if in.PulsarServiceUrl != nil {
		in, out := &in.PulsarServiceUrl, &out.PulsarServiceUrl
		*out = new(string)
		**out = **in
	}
	if in.TopicPrefix != nil {
		in, out := &in.TopicPrefix, &out.TopicPrefix
		*out = new(string)
		**out = **in
	}
	if in.CDCWorkingDir != nil {
		in, out := &in.CDCWorkingDir, &out.CDCWorkingDir
		*out = new(string)
		**out = **in
	}
	if in.CDCPollIntervalMs != nil {
		in, out := &in.CDCPollIntervalMs, &out.CDCPollIntervalMs
		*out = new(int)
		**out = **in
	}
	if in.ErrorCommitLogReprocessEnabled != nil {
		in, out := &in.ErrorCommitLogReprocessEnabled, &out.ErrorCommitLogReprocessEnabled
		*out = new(bool)
		**out = **in
	}
	if in.CDCConcurrentProcessors != nil {
		in, out := &in.CDCConcurrentProcessors, &out.CDCConcurrentProcessors
		*out = new(int)
		**out = **in
	}
	if in.PulsarBatchDelayInMs != nil {
		in, out := &in.PulsarBatchDelayInMs, &out.PulsarBatchDelayInMs
		*out = new(int)
		**out = **in
	}
	if in.PulsarKeyBasedBatcher != nil {
		in, out := &in.PulsarKeyBasedBatcher, &out.PulsarKeyBasedBatcher
		*out = new(bool)
		**out = **in
	}
	if in.PulsarMaxPendingMessages != nil {
		in, out := &in.PulsarMaxPendingMessages, &out.PulsarMaxPendingMessages
		*out = new(int)
		**out = **in
	}
	if in.PulsarMaxPendingMessagesAcrossPartitions != nil {
		in, out := &in.PulsarMaxPendingMessagesAcrossPartitions, &out.PulsarMaxPendingMessagesAcrossPartitions
		*out = new(int)
		**out = **in
	}
	if in.PulsarAuthPluginClassName != nil {
		in, out := &in.PulsarAuthPluginClassName, &out.PulsarAuthPluginClassName
		*out = new(string)
		**out = **in
	}
	if in.PulsarAuthParams != nil {
		in, out := &in.PulsarAuthParams, &out.PulsarAuthParams
		*out = new(string)
		**out = **in
	}
	if in.SSLProvider != nil {
		in, out := &in.SSLProvider, &out.SSLProvider
		*out = new(string)
		**out = **in
	}
	if in.SSLTruststorePath != nil {
		in, out := &in.SSLTruststorePath, &out.SSLTruststorePath
		*out = new(string)
		**out = **in
	}
	if in.SSLTruststorePassword != nil {
		in, out := &in.SSLTruststorePassword, &out.SSLTruststorePassword
		*out = new(string)
		**out = **in
	}
	if in.SSLTruststoreType != nil {
		in, out := &in.SSLTruststoreType, &out.SSLTruststoreType
		*out = new(string)
		**out = **in
	}
	if in.SSLKeystorePath != nil {
		in, out := &in.SSLKeystorePath, &out.SSLKeystorePath
		*out = new(string)
		**out = **in
	}
	if in.SSLKeystorePassword != nil {
		in, out := &in.SSLKeystorePassword, &out.SSLKeystorePassword
		*out = new(string)
		**out = **in
	}
	if in.SSLCipherSuites != nil {
		in, out := &in.SSLCipherSuites, &out.SSLCipherSuites
		*out = new(string)
		**out = **in
	}
	if in.SSLEnabledProtocols != nil {
		in, out := &in.SSLEnabledProtocols, &out.SSLEnabledProtocols
		*out = new(string)
		**out = **in
	}
	if in.SSLAllowInsecureConnection != nil {
		in, out := &in.SSLAllowInsecureConnection, &out.SSLAllowInsecureConnection
		*out = new(string)
		**out = **in
	}
	if in.SSLHostnameVerificationEnable != nil {
		in, out := &in.SSLHostnameVerificationEnable, &out.SSLHostnameVerificationEnable
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CDCConfiguration.
func (in *CDCConfiguration) DeepCopy() *CDCConfiguration {
	if in == nil {
		return nil
	}
	out := new(CDCConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CassandraDatacenter) DeepCopyInto(out *CassandraDatacenter) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CassandraDatacenter.
func (in *CassandraDatacenter) DeepCopy() *CassandraDatacenter {
	if in == nil {
		return nil
	}
	out := new(CassandraDatacenter)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CassandraDatacenter) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CassandraDatacenterList) DeepCopyInto(out *CassandraDatacenterList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]CassandraDatacenter, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CassandraDatacenterList.
func (in *CassandraDatacenterList) DeepCopy() *CassandraDatacenterList {
	if in == nil {
		return nil
	}
	out := new(CassandraDatacenterList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CassandraDatacenterList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CassandraDatacenterSpec) DeepCopyInto(out *CassandraDatacenterSpec) {
	*out = *in
	if in.DeprecatedDockerImageRunsAsCassandra != nil {
		in, out := &in.DeprecatedDockerImageRunsAsCassandra, &out.DeprecatedDockerImageRunsAsCassandra
		*out = new(bool)
		**out = **in
	}
	if in.Config != nil {
		in, out := &in.Config, &out.Config
		*out = make(json.RawMessage, len(*in))
		copy(*out, *in)
	}
	in.ManagementApiAuth.DeepCopyInto(&out.ManagementApiAuth)
	if in.NodeAffinityLabels != nil {
		in, out := &in.NodeAffinityLabels, &out.NodeAffinityLabels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	in.Resources.DeepCopyInto(&out.Resources)
	in.SystemLoggerResources.DeepCopyInto(&out.SystemLoggerResources)
	in.ConfigBuilderResources.DeepCopyInto(&out.ConfigBuilderResources)
	if in.Racks != nil {
		in, out := &in.Racks, &out.Racks
		*out = make([]Rack, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.StorageConfig.DeepCopyInto(&out.StorageConfig)
	if in.DeprecatedReplaceNodes != nil {
		in, out := &in.DeprecatedReplaceNodes, &out.DeprecatedReplaceNodes
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.NodeSelector != nil {
		in, out := &in.NodeSelector, &out.NodeSelector
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.ForceUpgradeRacks != nil {
		in, out := &in.ForceUpgradeRacks, &out.ForceUpgradeRacks
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.DseWorkloads != nil {
		in, out := &in.DseWorkloads, &out.DseWorkloads
		*out = new(DseWorkloads)
		**out = **in
	}
	if in.PodTemplateSpec != nil {
		in, out := &in.PodTemplateSpec, &out.PodTemplateSpec
		*out = new(v1.PodTemplateSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.Users != nil {
		in, out := &in.Users, &out.Users
		*out = make([]CassandraUser, len(*in))
		copy(*out, *in)
	}
	if in.Networking != nil {
		in, out := &in.Networking, &out.Networking
		*out = new(NetworkingConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.AdditionalSeeds != nil {
		in, out := &in.AdditionalSeeds, &out.AdditionalSeeds
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	in.AdditionalServiceConfig.DeepCopyInto(&out.AdditionalServiceConfig)
	if in.Tolerations != nil {
		in, out := &in.Tolerations, &out.Tolerations
		*out = make([]v1.Toleration, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.AdditionalLabels != nil {
		in, out := &in.AdditionalLabels, &out.AdditionalLabels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.AdditionalAnnotations != nil {
		in, out := &in.AdditionalAnnotations, &out.AdditionalAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.CDC != nil {
		in, out := &in.CDC, &out.CDC
		*out = new(CDCConfiguration)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CassandraDatacenterSpec.
func (in *CassandraDatacenterSpec) DeepCopy() *CassandraDatacenterSpec {
	if in == nil {
		return nil
	}
	out := new(CassandraDatacenterSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CassandraDatacenterStatus) DeepCopyInto(out *CassandraDatacenterStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]DatacenterCondition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.SuperUserUpserted.DeepCopyInto(&out.SuperUserUpserted)
	in.UsersUpserted.DeepCopyInto(&out.UsersUpserted)
	in.LastServerNodeStarted.DeepCopyInto(&out.LastServerNodeStarted)
	in.LastRollingRestart.DeepCopyInto(&out.LastRollingRestart)
	if in.NodeStatuses != nil {
		in, out := &in.NodeStatuses, &out.NodeStatuses
		*out = make(CassandraStatusMap, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.NodeReplacements != nil {
		in, out := &in.NodeReplacements, &out.NodeReplacements
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	in.QuietPeriod.DeepCopyInto(&out.QuietPeriod)
	if in.TrackedTasks != nil {
		in, out := &in.TrackedTasks, &out.TrackedTasks
		*out = make([]v1.ObjectReference, len(*in))
		copy(*out, *in)
	}
	if in.DatacenterName != nil {
		in, out := &in.DatacenterName, &out.DatacenterName
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CassandraDatacenterStatus.
func (in *CassandraDatacenterStatus) DeepCopy() *CassandraDatacenterStatus {
	if in == nil {
		return nil
	}
	out := new(CassandraDatacenterStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CassandraNodeStatus) DeepCopyInto(out *CassandraNodeStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CassandraNodeStatus.
func (in *CassandraNodeStatus) DeepCopy() *CassandraNodeStatus {
	if in == nil {
		return nil
	}
	out := new(CassandraNodeStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in CassandraStatusMap) DeepCopyInto(out *CassandraStatusMap) {
	{
		in := &in
		*out = make(CassandraStatusMap, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CassandraStatusMap.
func (in CassandraStatusMap) DeepCopy() CassandraStatusMap {
	if in == nil {
		return nil
	}
	out := new(CassandraStatusMap)
	in.DeepCopyInto(out)
	return *out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CassandraUser) DeepCopyInto(out *CassandraUser) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CassandraUser.
func (in *CassandraUser) DeepCopy() *CassandraUser {
	if in == nil {
		return nil
	}
	out := new(CassandraUser)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatacenterCondition) DeepCopyInto(out *DatacenterCondition) {
	*out = *in
	in.LastTransitionTime.DeepCopyInto(&out.LastTransitionTime)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatacenterCondition.
func (in *DatacenterCondition) DeepCopy() *DatacenterCondition {
	if in == nil {
		return nil
	}
	out := new(DatacenterCondition)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DseWorkloads) DeepCopyInto(out *DseWorkloads) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DseWorkloads.
func (in *DseWorkloads) DeepCopy() *DseWorkloads {
	if in == nil {
		return nil
	}
	out := new(DseWorkloads)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ManagementApiAuthConfig) DeepCopyInto(out *ManagementApiAuthConfig) {
	*out = *in
	if in.Insecure != nil {
		in, out := &in.Insecure, &out.Insecure
		*out = new(ManagementApiAuthInsecureConfig)
		**out = **in
	}
	if in.Manual != nil {
		in, out := &in.Manual, &out.Manual
		*out = new(ManagementApiAuthManualConfig)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ManagementApiAuthConfig.
func (in *ManagementApiAuthConfig) DeepCopy() *ManagementApiAuthConfig {
	if in == nil {
		return nil
	}
	out := new(ManagementApiAuthConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ManagementApiAuthInsecureConfig) DeepCopyInto(out *ManagementApiAuthInsecureConfig) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ManagementApiAuthInsecureConfig.
func (in *ManagementApiAuthInsecureConfig) DeepCopy() *ManagementApiAuthInsecureConfig {
	if in == nil {
		return nil
	}
	out := new(ManagementApiAuthInsecureConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ManagementApiAuthManualConfig) DeepCopyInto(out *ManagementApiAuthManualConfig) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ManagementApiAuthManualConfig.
func (in *ManagementApiAuthManualConfig) DeepCopy() *ManagementApiAuthManualConfig {
	if in == nil {
		return nil
	}
	out := new(ManagementApiAuthManualConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NetworkingConfig) DeepCopyInto(out *NetworkingConfig) {
	*out = *in
	if in.NodePort != nil {
		in, out := &in.NodePort, &out.NodePort
		*out = new(NodePortConfig)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NetworkingConfig.
func (in *NetworkingConfig) DeepCopy() *NetworkingConfig {
	if in == nil {
		return nil
	}
	out := new(NetworkingConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NodePortConfig) DeepCopyInto(out *NodePortConfig) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NodePortConfig.
func (in *NodePortConfig) DeepCopy() *NodePortConfig {
	if in == nil {
		return nil
	}
	out := new(NodePortConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Rack) DeepCopyInto(out *Rack) {
	*out = *in
	if in.NodeAffinityLabels != nil {
		in, out := &in.NodeAffinityLabels, &out.NodeAffinityLabels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Affinity != nil {
		in, out := &in.Affinity, &out.Affinity
		*out = new(v1.Affinity)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Rack.
func (in *Rack) DeepCopy() *Rack {
	if in == nil {
		return nil
	}
	out := new(Rack)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ServiceConfig) DeepCopyInto(out *ServiceConfig) {
	*out = *in
	in.DatacenterService.DeepCopyInto(&out.DatacenterService)
	in.SeedService.DeepCopyInto(&out.SeedService)
	in.AllPodsService.DeepCopyInto(&out.AllPodsService)
	in.AdditionalSeedService.DeepCopyInto(&out.AdditionalSeedService)
	in.NodePortService.DeepCopyInto(&out.NodePortService)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ServiceConfig.
func (in *ServiceConfig) DeepCopy() *ServiceConfig {
	if in == nil {
		return nil
	}
	out := new(ServiceConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ServiceConfigAdditions) DeepCopyInto(out *ServiceConfigAdditions) {
	*out = *in
	if in.Labels != nil {
		in, out := &in.Labels, &out.Labels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Annotations != nil {
		in, out := &in.Annotations, &out.Annotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ServiceConfigAdditions.
func (in *ServiceConfigAdditions) DeepCopy() *ServiceConfigAdditions {
	if in == nil {
		return nil
	}
	out := new(ServiceConfigAdditions)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StorageConfig) DeepCopyInto(out *StorageConfig) {
	*out = *in
	if in.CassandraDataVolumeClaimSpec != nil {
		in, out := &in.CassandraDataVolumeClaimSpec, &out.CassandraDataVolumeClaimSpec
		*out = new(v1.PersistentVolumeClaimSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.AdditionalVolumes != nil {
		in, out := &in.AdditionalVolumes, &out.AdditionalVolumes
		*out = make(AdditionalVolumesSlice, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StorageConfig.
func (in *StorageConfig) DeepCopy() *StorageConfig {
	if in == nil {
		return nil
	}
	out := new(StorageConfig)
	in.DeepCopyInto(out)
	return out
}
