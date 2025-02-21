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

package scheduledtask

import (
	"context"
	"fmt"
	"time"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	controlapi "github.com/k8ssandra/cass-operator/apis/control/v1alpha1"
	scheduledtaskv1alpha1 "github.com/k8ssandra/cass-operator/apis/scheduledtask.k8ssandra.io/v1alpha1"
	"github.com/robfig/cron/v3"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ScheduledTaskReconciler reconciles a ScheduledTask object
type ScheduledTaskReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Clock  Clock
}

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (r *RealClock) Now() time.Time {
	return time.Now()
}

// +kubebuilder:rbac:groups=scheduledtask.k8ssandra.io.cassandra.datastax.com,resources=scheduledtasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scheduledtask.k8ssandra.io.cassandra.datastax.com,resources=scheduledtasks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=scheduledtask.k8ssandra.io.cassandra.datastax.com,resources=scheduledtasks/finalizers,verbs=update
func (r *ScheduledTaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	logger := log.FromContext(ctx).WithValues("medusabackupscheduler", req.NamespacedName)

	scheduledtask := &scheduledtaskv1alpha1.ScheduledTask{}
	err := r.Get(ctx, req.NamespacedName, scheduledtask)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	sched, err := cron.ParseStandard(scheduledtask.Spec.Schedule)
	if err != nil {
		// The schedule is in incorrect format
		return ctrl.Result{}, err
	}

	dcKey := types.NamespacedName{Namespace: scheduledtask.Namespace, Name: scheduledtask.Spec.TaskDetails.Datacenter.Name}
	dc := &api.CassandraDatacenter{}
	if err := r.Get(ctx, dcKey, dc); err != nil {
		logger.Error(err, "failed to get cassandradatacenter", "CassandraDatacenter", dcKey)
		return ctrl.Result{}, err
	}

	// Set an owner reference on the task so that it can be cleaned up when the cassandra datacenter is deleted
	if scheduledtask.OwnerReferences == nil {
		if err = controllerutil.SetControllerReference(dc, scheduledtask, r.Scheme); err != nil {
			logger.Error(err, "failed to set controller reference", "CassandraDatacenter", dcKey)
			return ctrl.Result{}, err
		}
		if err = r.Update(ctx, scheduledtask); err != nil {
			logger.Error(err, "failed to update task with owner reference", "CassandraDatacenter", dcKey)
			return ctrl.Result{}, err
		} else {
			logger.Info("updated task with owner reference", "CassandraDatacenter", dcKey)
		}
	}

	defaults(scheduledtask)

	previousExecution, err := getPreviousExecutionTime(ctx, scheduledtask)
	if err != nil {
		return ctrl.Result{}, err
	}

	now := r.Clock.Now().UTC()

	// Calculate the next execution time
	nextExecution := sched.Next(previousExecution).UTC()

	if nextExecution.Before(now) {
		if scheduledtask.Spec.TaskDetails.ConcurrencyPolicy == batchv1.ForbidConcurrent {
			for _, job := range scheduledtask.Spec.TaskDetails.Jobs {
				if activeTasks, err := r.activeTasks(scheduledtask, dc, job.Command); err != nil {
					logger.V(1).Info("failed to get activeTasks", "error", err)
					return ctrl.Result{}, err
				} else {
					if activeTasks > 0 {
						logger.V(1).Info("Postponing backup schedule due to an unfinished existing job", "MedusaBackupSchedule", req.NamespacedName)
						return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
					}
				}
			}
		}
		nextExecution = sched.Next(now)
		previousExecution = now
	}

	// Update the status if there are modifications
	if scheduledtask.Status.LastExecution.Time.Before(previousExecution) ||
		scheduledtask.Status.NextSchedule.Time.Before(nextExecution) {
		scheduledtask.Status.NextSchedule = metav1.NewTime(nextExecution)
		scheduledtask.Status.LastExecution = metav1.NewTime(previousExecution)

		if err := r.Client.Status().Update(ctx, scheduledtask); err != nil {
			return ctrl.Result{}, err
		}
	}

	generatedName := fmt.Sprintf("%s-%d", scheduledtask.Name, now.Unix())
	logger.V(1).Info("Scheduled time has been reached, creating a cassandraTask", "CassandraTask name", generatedName)
	cassandraTask := &controlapi.CassandraTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generatedName,
			Namespace: scheduledtask.Namespace,
			Labels:    dc.GetDatacenterLabels(),
		},
		Spec: scheduledtask.Spec.TaskDetails.CassandraTaskSpec,
	}

	if err := r.Client.Create(ctx, cassandraTask); err != nil {
		// We've already updated the Status times.. we'll miss this job now?
		return ctrl.Result{}, err
	}

	nextRunTime := nextExecution.Sub(now)
	logger.V(1).Info("Requeing for next scheduled event", "nextRuntime", nextRunTime.String())
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ScheduledTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&scheduledtaskv1alpha1.ScheduledTask{}).
		Complete(r)
}

func defaults(scheduledtask *scheduledtaskv1alpha1.ScheduledTask) {
	if scheduledtask.Spec.TaskDetails.ConcurrencyPolicy == "" {
		scheduledtask.Spec.TaskDetails.ConcurrencyPolicy = batchv1.ForbidConcurrent
	}
}

func getPreviousExecutionTime(ctx context.Context, scheduledtask *scheduledtaskv1alpha1.ScheduledTask) (time.Time, error) {
	previousExecution := scheduledtask.Status.LastExecution

	if previousExecution.IsZero() {
		// This job has never been executed, we use creationTimestamp
		previousExecution = scheduledtask.CreationTimestamp
	}

	return previousExecution.Time.UTC(), nil
}

func (r *ScheduledTaskReconciler) activeTasks(scheduledtask *scheduledtaskv1alpha1.ScheduledTask, dc *api.CassandraDatacenter, command controlapi.CassandraCommand) (int, error) {
	tasks := &scheduledtaskv1alpha1.ScheduledTaskList{}
	if err := r.Client.List(context.Background(), tasks, client.InNamespace(scheduledtask.Namespace), client.MatchingLabels(dc.GetDatacenterLabels())); err != nil {
		return 0, err
	}
	activeJobs := make([]scheduledtaskv1alpha1.ScheduledTask, 0)
	for _, task := range tasks.Items {
		cassTask := controlapi.CassandraTask{}
		if err := r.Client.Get(context.Background(), types.NamespacedName{Namespace: task.Namespace, Name: task.Spec.TaskDetails.Name}, &cassTask); err != nil {
			return 0, err
		}
		for _, job := range cassTask.Spec.Jobs {
			if job.Command != command {
				continue
			} else {
				activeJobs = append(activeJobs, task)
			}
		}
	}
	return len(activeJobs), nil
}
