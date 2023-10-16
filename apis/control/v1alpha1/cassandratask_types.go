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

const (
	RestartedAtAnnotation = "control.k8ssandra.io/restartedAt"
)

// CassandraTaskSpec defines the desired state of CassandraTask
type CassandraTaskSpec struct {

	// Which datacenter this task is targetting. Note, this must be a datacenter which the current cass-operator
	// can access
	Datacenter corev1.ObjectReference `json:"datacenter,omitempty"`

	CassandraTaskTemplate `json:",inline"`
}

// CassandraTaskTemplate defines the execution attributes of a CassandraTaskSpec.
type CassandraTaskTemplate struct {

	// ScheduledTime indicates the earliest possible time this task is executed. This does not necessarily
	// equal to the time it is actually executed (if other tasks are blocking for example). If not set,
	// the task will be executed immediately.
	// +optional
	ScheduledTime *metav1.Time `json:"scheduledTime,omitempty"`

	// TODO BackOffLimit for RetryPolicy and maxMissedSeconds deadline for ScheduledTime?

	// Jobs defines the jobs this task will execute (and their order)
	Jobs []CassandraJob `json:"jobs,omitempty"`

	// RestartPolicy indicates the behavior n case of failure. Default is Never.
	// +optional
	RestartPolicy corev1.RestartPolicy `json:"restartPolicy,omitempty"`

	// TTLSecondsAfterFinished defines how long the completed job will kept before being cleaned up. If set to 0
	// the task will not be cleaned up by the cass-operator. If unset, the default time (86400s) is used.
	// +optional
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`

	// Specifics if this task can be run concurrently with other active tasks. Valid values are:
	// - "Allow": allows multiple Tasks to run concurrently on Cassandra cluster
	// - "Forbid" (default): only a single task is executed at once
	// The "Allow" property is only valid if all the other active Tasks have "Allow" as well.
	// +optional
	ConcurrencyPolicy batchv1.ConcurrencyPolicy `json:"concurrencyPolicy,omitempty"`
}

type CassandraCommand string

const (
	CommandCleanup         CassandraCommand = "cleanup"
	CommandRebuild         CassandraCommand = "rebuild"
	CommandRestart         CassandraCommand = "restart"
	CommandUpgradeSSTables CassandraCommand = "upgradesstables"
	CommandReplaceNode     CassandraCommand = "replacenode"
	CommandCompaction      CassandraCommand = "compact"
	CommandScrub           CassandraCommand = "scrub"
	CommandMove            CassandraCommand = "move"
	CommandGarbageCollect  CassandraCommand = "garbagecollect"
	CommandFlush           CassandraCommand = "flush"
)

type CassandraJob struct {
	Name string `json:"name"`

	// Command defines what is run against Cassandra pods
	Command CassandraCommand `json:"command"`

	// Arguments are additional parameters for the command
	Arguments JobArguments `json:"args,omitempty"`
}

type JobArguments struct {
	KeyspaceName     string   `json:"keyspace_name,omitempty"`
	SourceDatacenter string   `json:"source_datacenter,omitempty"`
	PodName          string   `json:"pod_name,omitempty"`
	RackName         string   `json:"rack,omitempty"`
	Tables           []string `json:"tables,omitempty"`
	JobsCount        *int     `json:"jobs,omitempty"`

	// Scrub arguments
	NoValidate    bool `json:"no_validate,omitempty"`
	NoSnapshot    bool `json:"no_snapshot,omitempty"`
	SkipCorrupted bool `json:"skip_corrupted,omitempty"`

	// Compaction arguments
	SplitOutput bool   `json:"split_output,omitempty"`
	StartToken  string `json:"start_token,omitempty"`
	EndToken    string `json:"end_token,omitempty"`

	// NewTokens is a map of pod names to their newly-assigned tokens. Required for the move
	// command, ignored otherwise. Pods referenced in this map must exist; any existing pod not
	// referenced in this map will not be moved.
	NewTokens map[string]string `json:"new_tokens,omitempty"`
}

// CassandraTaskStatus defines the observed state of CassandraJob
type CassandraTaskStatus struct {
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
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

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
	Active int `json:"active,omitempty"`

	// The number of pods which reached phase Succeeded.
	// +optional
	Succeeded int `json:"succeeded,omitempty"`

	// The number of pods which reached phase Failed.
	// +optional
	Failed int `json:"failed,omitempty"`
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

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// +kubebuilder:printcolumn:name="Datacenter",type=string,JSONPath=".spec.datacenter.name",description="Datacenter which the task targets"
// +kubebuilder:printcolumn:name="Job",type=string,JSONPath=".spec.jobs[0].command",description="The job that is executed"
// +kubebuilder:printcolumn:name="Scheduled",type="date",JSONPath=".spec.scheduledTime",description="When the execution of the task is allowed at earliest"
// +kubebuilder:printcolumn:name="Started",type="date",JSONPath=".status.startTime",description="When the execution of the task started"
// +kubebuilder:printcolumn:name="Completed",type="date",JSONPath=".status.completionTime",description="When the execution of the task finished"
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
