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

package v1alpha1

import (
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/api/core/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CassandraJob) DeepCopyInto(out *CassandraJob) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CassandraJob.
func (in *CassandraJob) DeepCopy() *CassandraJob {
	if in == nil {
		return nil
	}
	out := new(CassandraJob)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CassandraTask) DeepCopyInto(out *CassandraTask) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CassandraTask.
func (in *CassandraTask) DeepCopy() *CassandraTask {
	if in == nil {
		return nil
	}
	out := new(CassandraTask)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CassandraTask) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CassandraTaskList) DeepCopyInto(out *CassandraTaskList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]CassandraTask, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CassandraTaskList.
func (in *CassandraTaskList) DeepCopy() *CassandraTaskList {
	if in == nil {
		return nil
	}
	out := new(CassandraTaskList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CassandraTaskList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CassandraTaskSpec) DeepCopyInto(out *CassandraTaskSpec) {
	*out = *in
	if in.ScheduledTime != nil {
		in, out := &in.ScheduledTime, &out.ScheduledTime
		*out = (*in).DeepCopy()
	}
	out.Datacenter = in.Datacenter
	if in.Jobs != nil {
		in, out := &in.Jobs, &out.Jobs
		*out = make([]CassandraJob, len(*in))
		copy(*out, *in)
	}
	if in.RestartPolicy != nil {
		in, out := &in.RestartPolicy, &out.RestartPolicy
		*out = new(v1.RestartPolicy)
		**out = **in
	}
	if in.TTLSecondsAfterFinished != nil {
		in, out := &in.TTLSecondsAfterFinished, &out.TTLSecondsAfterFinished
		*out = new(int32)
		**out = **in
	}
	if in.ConcurrencyPolicy != nil {
		in, out := &in.ConcurrencyPolicy, &out.ConcurrencyPolicy
		*out = new(batchv1.ConcurrencyPolicy)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CassandraTaskSpec.
func (in *CassandraTaskSpec) DeepCopy() *CassandraTaskSpec {
	if in == nil {
		return nil
	}
	out := new(CassandraTaskSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CassandraTaskStatus) DeepCopyInto(out *CassandraTaskStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]JobCondition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.StartTime != nil {
		in, out := &in.StartTime, &out.StartTime
		*out = (*in).DeepCopy()
	}
	if in.CompletionTime != nil {
		in, out := &in.CompletionTime, &out.CompletionTime
		*out = (*in).DeepCopy()
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CassandraTaskStatus.
func (in *CassandraTaskStatus) DeepCopy() *CassandraTaskStatus {
	if in == nil {
		return nil
	}
	out := new(CassandraTaskStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *JobCondition) DeepCopyInto(out *JobCondition) {
	*out = *in
	in.LastProbeTime.DeepCopyInto(&out.LastProbeTime)
	in.LastTransitionTime.DeepCopyInto(&out.LastTransitionTime)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new JobCondition.
func (in *JobCondition) DeepCopy() *JobCondition {
	if in == nil {
		return nil
	}
	out := new(JobCondition)
	in.DeepCopyInto(out)
	return out
}
