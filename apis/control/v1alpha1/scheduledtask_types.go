/*
Copyright 2024.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScheduledTaskSpec defines the desired state of ScheduledTask
type ScheduledTaskSpec struct {
	Schedule    string      `json:"schedule,omitempty"`
	TaskDetails TaskDetails `json:"taskDetails,omitempty"`
}

type TaskDetails struct {
	// Name of the task. Always populated.
	Name string `json:"name,omitempty"`
	// +inline
	CassandraTaskSpec `json:"omitempty"`
}

// MedusaTaskStatus defines the observed state of MedusaTask
type ScheduledTaskStatus struct {
	// NextSchedule indicates when the next backup is going to be done
	NextSchedule metav1.Time `json:"nextSchedule,omitempty"`

	// LastExecution tells when the backup was last time taken. If empty, the backup has never been taken
	LastExecution metav1.Time `json:"lastExecution,omitempty"`
}

type TaskResult struct {
	// Name of the pod that ran the task. Always populated.
	PodName string `json:"podName,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ScheduledTask is the Schema for the scheduledtasks API
type ScheduledTask struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScheduledTaskSpec   `json:"spec,omitempty"`
	Status ScheduledTaskStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ScheduledTaskList contains a list of ScheduledTask
type ScheduledTaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScheduledTask `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ScheduledTask{}, &ScheduledTaskList{})
}
