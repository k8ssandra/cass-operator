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

package control

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	cassdcapi "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	taskapi "github.com/k8ssandra/cass-operator/apis/control/v1alpha1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type FakeClock struct {
	currentTime time.Time
}

func (f *FakeClock) Now() time.Time {
	return f.currentTime
}

var _ Clock = &FakeClock{}

func TestScheduler(t *testing.T) {
	require := require.New(t)
	require.NoError(cassdcapi.AddToScheme(scheme.Scheme))
	require.NoError(taskapi.AddToScheme(scheme.Scheme))

	fClock := &FakeClock{}

	dc := cassdcapi.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dc1",
			Namespace: "test-ns",
		},
		Spec: cassdcapi.CassandraDatacenterSpec{},
	}

	// To manipulate time and requeue, we use fakeclient here instead of envtest
	scheduledTask := &taskapi.ScheduledTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scheduled-task",
			Namespace: "test-ns",
		},
		Spec: taskapi.ScheduledTaskSpec{
			Schedule: "* * * * *",
			TaskDetails: taskapi.TaskDetails{
				Name: "the-operation",
				CassandraTaskSpec: taskapi.CassandraTaskSpec{
					Datacenter: corev1.ObjectReference{
						Name: "dc1",
					},
					CassandraTaskTemplate: taskapi.CassandraTaskTemplate{
						Jobs: []taskapi.CassandraJob{
							{
								Command: taskapi.CommandCleanup,
							},
						},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithObjects(scheduledTask, &dc).
		WithStatusSubresource(scheduledTask).
		WithScheme(scheme.Scheme).
		Build()

	nsName := types.NamespacedName{
		Name:      scheduledTask.Name,
		Namespace: scheduledTask.Namespace,
	}

	r := &ScheduledTaskReconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
		Clock:  fClock,
		Log:    ctrl.Log.WithName("controllers").WithName("ScheduledTask"),
	}

	res, err := r.Reconcile(context.TODO(), reconcile.Request{NamespacedName: nsName})
	require.NoError(err)
	require.True(res.RequeueAfter > 0)

	fClock.currentTime = fClock.currentTime.Add(1 * time.Minute).Add(1 * time.Second)

	res, err = r.Reconcile(context.TODO(), reconcile.Request{NamespacedName: nsName})
	require.NoError(err)
	require.True(res.RequeueAfter > 0)

	// We should have a task now..
	tasks := taskapi.CassandraTaskList{}
	err = fakeClient.List(context.TODO(), &tasks)
	require.NoError(err)
	require.Equal(1, len(tasks.Items))

	// Ensure the task object is created correctly
	task := tasks.Items[0]
	require.Equal(scheduledTask.Spec.TaskDetails.Datacenter, task.Spec.Datacenter)
	require.Equal(scheduledTask.Spec.TaskDetails.Jobs[0].Command, task.Spec.Jobs[0].Command)

	// Verify the Status of the scheduledTask is modified and the object is requeued
	scheduledTaskLive := &taskapi.ScheduledTask{}
	err = fakeClient.Get(context.TODO(), nsName, scheduledTaskLive)
	require.NoError(err)

	require.Equal(fClock.currentTime, scheduledTaskLive.Status.LastExecution.Time.UTC())
	require.Equal(time.Time{}.Add(2*time.Minute), scheduledTaskLive.Status.NextSchedule.Time.UTC())

	// Test that next invocation also works
	fClock.currentTime = fClock.currentTime.Add(1 * time.Minute)

	res, err = r.Reconcile(context.TODO(), reconcile.Request{NamespacedName: nsName})
	require.NoError(err)
	require.True(res.RequeueAfter > 0)

	// We should not have more than 1, since we never set the previous one as finished
	taskList := taskapi.CassandraTaskList{}
	err = fakeClient.List(context.TODO(), &taskList)
	require.NoError(err)
	require.Equal(1, len(taskList.Items))

	// Mark the first one as finished and try again
	currentTime := metav1.NewTime(fClock.currentTime)
	task.Status.CompletionTime = &currentTime
	require.NoError(fakeClient.Update(context.TODO(), &task))

	scheduledTasksLive := &taskapi.ScheduledTaskList{}
	err = fakeClient.List(context.TODO(), scheduledTasksLive)
	require.NoError(err)
	require.Equal(1, len(taskList.Items))

	res, err = r.Reconcile(context.TODO(), reconcile.Request{NamespacedName: nsName})
	require.NoError(err)
	require.True(res.RequeueAfter > 0)

	taskList = taskapi.CassandraTaskList{}
	err = fakeClient.List(context.TODO(), &taskList)
	require.NoError(err)
	require.Equal(2, len(taskList.Items))

	for _, backup := range taskList.Items {
		currentTime := metav1.NewTime(fClock.currentTime)
		backup.Status.CompletionTime = &currentTime
		require.NoError(fakeClient.Update(context.TODO(), &backup))
	}

	// Verify that invoking again without reaching the next time does not generate another backup
	// or modify the Status
	scheduledTaskLive = &taskapi.ScheduledTask{}
	err = fakeClient.Get(context.TODO(), nsName, scheduledTaskLive)
	require.NoError(err)

	previousExecutionTime := scheduledTaskLive.Status.LastExecution
	fClock.currentTime = fClock.currentTime.Add(30 * time.Second)
	res, err = r.Reconcile(context.TODO(), reconcile.Request{NamespacedName: nsName})
	require.NoError(err)
	require.True(res.RequeueAfter > 0)

	cassandraTasks := taskapi.CassandraTaskList{}
	err = fakeClient.List(context.TODO(), &cassandraTasks)
	require.NoError(err)
	require.Equal(2, len(cassandraTasks.Items))

	scheduledTaskLive = &taskapi.ScheduledTask{}
	err = fakeClient.Get(context.TODO(), nsName, scheduledTaskLive)
	require.NoError(err)
	require.Equal(previousExecutionTime, scheduledTaskLive.Status.LastExecution)
}

func TestSchedulerParseError(t *testing.T) {
	require := require.New(t)

	// To manipulate time and requeue, we use fakeclient here instead of envtest
	scheduledTask := &taskapi.ScheduledTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-schedule",
			Namespace: "test-ns",
		},
		Spec: taskapi.ScheduledTaskSpec{
			Schedule: "***",
			TaskDetails: taskapi.TaskDetails{
				Name: "the-operation",
				CassandraTaskSpec: taskapi.CassandraTaskSpec{
					Datacenter: corev1.ObjectReference{
						Name: "dc1",
					},
					CassandraTaskTemplate: taskapi.CassandraTaskTemplate{
						Jobs: []taskapi.CassandraJob{
							{
								Command: taskapi.CommandCleanup,
							},
						},
					},
				},
			},
		},
	}
	require.NoError(taskapi.AddToScheme(scheme.Scheme))
	require.NoError(cassdcapi.AddToScheme(scheme.Scheme))
	require.NoError(taskapi.AddToScheme(scheme.Scheme))

	fakeClient := fake.NewClientBuilder().
		WithRuntimeObjects(scheduledTask).
		WithScheme(scheme.Scheme).
		Build()

	fClock := &FakeClock{}

	r := &ScheduledTaskReconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
		Clock:  fClock,
		Log:    ctrl.Log.WithName("controllers").WithName("ScheduledTask"),
	}

	nsName := types.NamespacedName{
		Name:      scheduledTask.Name,
		Namespace: scheduledTask.Namespace,
	}

	_, err := r.Reconcile(context.TODO(), reconcile.Request{NamespacedName: nsName})
	require.Error(err)
}

func TestSchedulerLabels(t *testing.T) {
	require := require.New(t)
	require.NoError(cassdcapi.AddToScheme(scheme.Scheme))
	require.NoError(taskapi.AddToScheme(scheme.Scheme))

	fClock := &FakeClock{}

	dc := cassdcapi.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dc1",
			Namespace: "test-ns",
		},
		Spec: cassdcapi.CassandraDatacenterSpec{
			AdditionalLabels: map[string]string{
				"test-label": "test-value",
			},
			AdditionalAnnotations: map[string]string{
				"test-annotation": "test-value",
			},
		},
	}

	// To manipulate time and requeue, we use fakeclient here instead of envtest
	scheduledTask := &taskapi.ScheduledTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scheduled-task",
			Namespace: "test-ns",
		},
		Spec: taskapi.ScheduledTaskSpec{
			Schedule: "* * * * *",
			TaskDetails: taskapi.TaskDetails{
				Name: "the-operation",
				CassandraTaskSpec: taskapi.CassandraTaskSpec{
					Datacenter: corev1.ObjectReference{
						Name: "dc1",
					},
					CassandraTaskTemplate: taskapi.CassandraTaskTemplate{
						Jobs: []taskapi.CassandraJob{
							{
								Command: taskapi.CommandCleanup,
							},
						},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithObjects(scheduledTask, &dc).
		WithStatusSubresource(scheduledTask).
		WithScheme(scheme.Scheme).
		Build()

	nsName := types.NamespacedName{
		Name:      scheduledTask.Name,
		Namespace: scheduledTask.Namespace,
	}

	r := &ScheduledTaskReconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
		Clock:  fClock,
		Log:    ctrl.Log.WithName("controllers").WithName("ScheduledTask"),
	}

	fClock.currentTime = fClock.currentTime.Add(1 * time.Minute).Add(1 * time.Second)

	res, err := r.Reconcile(context.TODO(), reconcile.Request{NamespacedName: nsName})
	require.NoError(err)
	require.True(res.RequeueAfter > 0)

	// We should have a task now..
	tasks := taskapi.CassandraTaskList{}
	err = fakeClient.List(context.TODO(), &tasks)
	require.NoError(err)
	require.Equal(1, len(tasks.Items))

	// Ensure the task object is created correctly
	task := tasks.Items[0]
	require.Equal(scheduledTask.Spec.TaskDetails.Datacenter, task.Spec.Datacenter)
	require.Equal(scheduledTask.Spec.TaskDetails.Jobs[0].Command, task.Spec.Jobs[0].Command)
	require.Equal("test-value", task.Labels["test-label"])
	require.Equal("test-value", task.Annotations["test-annotation"])
}

func TestSchedulerWithJsonForm(t *testing.T) {
	require := require.New(t)
	require.NoError(cassdcapi.AddToScheme(scheme.Scheme))
	require.NoError(taskapi.AddToScheme(scheme.Scheme))

	// Use JSON form to define the ScheduledTask
	scheduledTaskJson := `{
        "apiVersion": "control.k8ssandra.io/v1alpha1",
        "kind": "ScheduledTask",
        "metadata": {
            "name": "json-scheduled-task",
            "namespace": "test-ns"
        },
        "spec": {
            "schedule": "*/5 * * * *",
            "taskDetails": {
                "name": "json-operation",
                "datacenter": {
                    "name": "dc1"
                },
                "jobs": [
                    {
                        "command": "cleanup"
                    }
                ]
            }
        }
    }`

	// Create a datacenter for the task to reference
	dc := cassdcapi.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dc1",
			Namespace: "test-ns",
		},
		Spec: cassdcapi.CassandraDatacenterSpec{},
	}

	// Parse JSON into an unstructured object
	scheduledTaskObj := &unstructured.Unstructured{}
	err := json.Unmarshal([]byte(scheduledTaskJson), scheduledTaskObj)
	require.NoError(err)

	// Create fake clock for testing
	fClock := &FakeClock{}

	// Setup fake client with the JSON-based ScheduledTask
	fakeClient := fake.NewClientBuilder().
		WithObjects(&dc).
		WithRuntimeObjects(scheduledTaskObj).
		WithStatusSubresource(&taskapi.ScheduledTask{}).
		WithScheme(scheme.Scheme).
		Build()

	r := &ScheduledTaskReconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
		Clock:  fClock,
		Log:    ctrl.Log.WithName("controllers").WithName("ScheduledTask"),
	}

	nsName := types.NamespacedName{
		Name:      "json-scheduled-task",
		Namespace: "test-ns",
	}

	// Initial reconcile
	res, err := r.Reconcile(context.TODO(), reconcile.Request{NamespacedName: nsName})
	require.NoError(err)
	require.True(res.RequeueAfter > 0)

	// Advance time to trigger task creation
	fClock.currentTime = fClock.currentTime.Add(6 * time.Minute)

	// Reconcile again to create the task
	res, err = r.Reconcile(context.TODO(), reconcile.Request{NamespacedName: nsName})
	require.NoError(err)
	require.True(res.RequeueAfter > 0)

	// Verify task was created
	tasks := taskapi.CassandraTaskList{}
	err = fakeClient.List(context.TODO(), &tasks)
	require.NoError(err)
	require.Equal(1, len(tasks.Items))

	// Verify task details
	task := tasks.Items[0]
	require.Equal("dc1", task.Spec.Datacenter.Name)
	require.Equal(taskapi.CommandCleanup, task.Spec.Jobs[0].Command)
	require.True(strings.HasPrefix(task.Name, "json-scheduled-task-"))
}
