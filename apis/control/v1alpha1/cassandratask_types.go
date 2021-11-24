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

package v1alpha1

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TODO Labels to add that indicate the status of the task? So they can be searched? Like jobs have.

// CassandraTaskSpec defines the desired state of CassandraTask
type CassandraTaskSpec struct {

	// ScheduledTime indicates the earliest possible time this task is executed. This does not necessarily
	// equal to the time it is actually executed (if other tasks are blocking for example)
	// +optional
	ScheduledTime *metav1.Time `json:"scheduledTime,omitempty"`

	// TODO BackOffLimit for RetryPolicy and maxMissedSeconds deadline for ScheduledTime?

	// Which datacenter this task is targetting. Note, this must be a datacenter which the current cass-operator
	// can access
	Datacenter corev1.ObjectReference `json:"datacenter,omitempty"`

	// Jobs defines the jobs this task will execute (and their order)
	Jobs []CassandraJob `json:"jobs,omitempty"`

	// RestartPolicy indicates the behavior n case of failure. Default is Never.
	// +optional
	RestartPolicy *corev1.RestartPolicy `json:"restartPolicy,omitempty"`

	// TTLSecondsAfterFinished defines how long the completed job will kept before being cleaned up. If not set,
	// the task will not be cleaned up by the cass-operator.
	// +optional
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`

	// Specifics if this task can be run concurrently with other active tasks. Valid values are:
	// - "Allow": allows multiple Tasks to run concurrently on Cassandra cluster
	// - "Forbid" (default): only a single task is executed at once
	// The "Allow" property is only valid if all the other active Tasks have "Allow" as well.
	// +optional
	ConcurrencyPolicy *batchv1.ConcurrencyPolicy `json:"concurrencyPolicy,omitempty"`

	// TODO Add webhook?

	/*
		spec:
			datacenter:
				name: dc1
				namespace: cass-operator
			scheduledTime: "nnnnnn"
			jobs:
				- name: rebuild-dc1
				  command: rebuild
			restartPolicy: Never
			concurrencyPolicy: Forbid
			ttlSecondsAfterFinished: 86400
	*/
}

type CassandraJob struct {
	// name, command.. anything else?
}

// CassandraTaskStatus defines the observed state of CassandraJob
type CassandraTaskStatus struct {

	// TODO Status and Conditions is almost 1:1 to Kubernetes Job's definitions.

	// The latest available observations of an object's current state. When a Job
	// fails, one of the conditions will have type "Failed" and status true. When
	// a Job is suspended, one of the conditions will have type "Suspended" and
	// status true; when the Job is resumed, the status of this condition will
	// become false. When a Job is completed, one of the conditions will have
	// type "Complete" and status true.
	// More info: https://kubernetes.io/docs/concepts/workloads/controllers/jobs-run-to-completion/
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=atomic
	Conditions []JobCondition `json:"conditions,omitempty"`

	// Represents time when the job controller started processing a job. When a
	// Job is created in the suspended state, this field is not set until the
	// first time it is resumed. This field is reset every time a Job is resumed
	// from suspension. It is represented in RFC3339 form and is in UTC.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// Represents time when the job was completed. It is not guaranteed to
	// be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	// The completion time is only set when the job finishes successfully.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// The number of actively running pods.
	// +optional
	Active int32 `json:"active,omitempty"`

	// The number of pods which reached phase Succeeded.
	// +optional
	Succeeded int32 `json:"succeeded,omitempty"`

	// The number of pods which reached phase Failed.
	// +optional
	Failed int32 `json:"failed,omitempty"`
}

type JobConditionType string

const (
	// JobComplete means the job has completed its execution.
	JobComplete JobConditionType = "Complete"
	// JobFailed means the job has failed its execution.
	JobFailed JobConditionType = "Failed"
	// JobRunning means the job is currently executing
	JobRunning JobConditionType = "Running"
)

type JobCondition struct {
	// Type of job condition, Complete or Failed.
	Type JobConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// Last time the condition was checked.
	// +optional
	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty"`
	// Last time the condition transit from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// (brief) reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Human readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// CassandraTask is the Schema for the cassandrajobs API
type CassandraTask struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CassandraTaskSpec   `json:"spec,omitempty"`
	Status CassandraTaskStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CassandraTaskList contains a list of CassandraJob
type CassandraTaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CassandraTask `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CassandraTask{}, &CassandraTaskList{})
}
