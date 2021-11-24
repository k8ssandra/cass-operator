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

package control

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cassapi "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	api "github.com/k8ssandra/cass-operator/apis/control/v1alpha1"
	"github.com/k8ssandra/cass-operator/pkg/httphelper"
	"github.com/k8ssandra/cass-operator/pkg/oplabels"
)

// CassandraTaskReconciler reconciles a CassandraJob object
type CassandraTaskReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=control.k8ssandra.io,resources=cassandrajobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=control.k8ssandra.io,resources=cassandrajobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=control.k8ssandra.io,resources=cassandrajobs/finalizers,verbs=update

func (r *CassandraTaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var cassJob api.CassandraTask
	if err := r.Get(ctx, req.NamespacedName, &cassJob); err != nil {
		log.Error(err, "unable to fetch CassandraTask", "Request", req.NamespacedName)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	timeNow := metav1.Now()

	if cassJob.Spec.ScheduledTime != nil && timeNow.Before(cassJob.Spec.ScheduledTime) {
		// TODO ScheduledTime is before current time, requeue for later processing
		log.V(1).Info("this job isn't scheduled to be run yet", "Request", req.NamespacedName)
		nextRunTime := cassJob.Spec.ScheduledTime.Sub(timeNow.Time)
		return ctrl.Result{RequeueAfter: nextRunTime}, nil
	}

	// Check if job is finished, and if and only if, check the TTL from last finished time.
	if cassJob.Status.CompletionTime != nil {
		// Nothing more to do here, other than delete it..
		if cassJob.Spec.TTLSecondsAfterFinished != nil {
			deletionTime := cassJob.Status.CompletionTime.Add(time.Duration(*cassJob.Spec.TTLSecondsAfterFinished) * time.Second)
			if deletionTime.After(timeNow.Time) {
				// TODO Delete this object
				log.V(1).Info("this task is scheduled to be deleted now", "Request", req.NamespacedName)
			} else {
				// Reschedule for later deletion
				log.V(1).Info("this task is scheduled to be deleted later", "Request", req.NamespacedName)
				nextRunTime := deletionTime.Sub(timeNow.Time)
				return ctrl.Result{RequeueAfter: nextRunTime}, nil
			}
		}
		// Do not requeue anymore, this task has been completed
		return ctrl.Result{}, nil
	}

	// Get the CassandraDatacenter
	dc := cassapi.CassandraDatacenter{}
	dcNamespacedName := types.NamespacedName{
		Namespace: cassJob.Spec.Datacenter.Namespace,
		Name:      cassJob.Spec.Datacenter.Name,
	}
	if err := r.Get(ctx, dcNamespacedName, &dc); err != nil {
		log.Error(err, "unable to fetch CassandraDatacenter", "Datacenter", cassJob.Spec.Datacenter)
		// This is unrecoverable error at this point, do not requeue
		return ctrl.Result{}, err
	}

	if err := controllerutil.SetOwnerReference(&dc.ObjectMeta, &cassJob.ObjectMeta, r.Scheme); err != nil {
		log.Error(err, "unable to set ownerReference to the task", "Datacenter", cassJob.Spec.Datacenter, "CassandraTask", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// TODO Does our concurrencypolicy allow the task to run? Are there any other active ones?
	activeTasks, err := r.activeTasks(ctx)
	if err != nil {
		log.Error(err, "unable to fetch active CassandraTasks", "Request", req.NamespacedName)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if len(activeTasks) > 0 {
		if *cassJob.Spec.ConcurrencyPolicy == batchv1.ForbidConcurrent {
			// TODO Can't run right now, requeue
			// TODO Or should we push an event?
			log.V(1).Info("this job isn't allowed to run due to ConcurrencyPolicy restrictions", "activeTasks", len(activeTasks))
			return ctrl.Result{Requeue: true}, nil // TODO Add some sane time here
		}
		// TODO There are other tasks running, are they allowing or forbiding the concurrent work?
		for _, task := range activeTasks {
			if *task.Spec.ConcurrencyPolicy == batchv1.ForbidConcurrent {
				log.V(1).Info("this job isn't allowed to run due to ConcurrencyPolicy restrictions", "activeTasks", len(activeTasks))
				return ctrl.Result{Requeue: true}, nil // TODO Add some sane time here
			}
		}
	}

	for _ = range cassJob.Spec.Jobs {
		// TODO Check if this part of the job has finished (from Status?)
		// Job undefined at this point, define later

	}

	// TODO Implement "cleanup" first

	// TODO Starting the run, set the Active label so we can quickly fetch the active ones

	// TODO Update status before any exit

	// log.V(1).Info for DEBUG logging

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CassandraTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.CassandraTask{}).
		Complete(r)
}

func (r *CassandraTaskReconciler) activeTasks(ctx context.Context) ([]api.CassandraTask, error) {
	var taskList api.CassandraTaskList
	if err := r.Client.List(ctx, &taskList); err != nil {
		return nil, err
	}

	return taskList.Items, nil
}

/**
	Implementation part
**/
const (
	podJobIdAnnotation     = "control.k8ssandra.io/job-id"
	podJobStatusAnnotation = "control.k8ssandra.io/job-status"
	podJobHandler          = "control.k8ssandra.io/job-runner"
	jobHandlerMgmtApi      = "management-api"

	podJobCompleted = "COMPLETED"
	podJobError     = "ERROR"
	podJobWaiting   = "WAITING"
)

var (
	jobRunner chan int = make(chan int, 1)
)

/*
	We need to replicate following behavior:
		* GetDcPods
		* Initialize NodeMgmtClient
*/

func (r *CassandraTaskReconciler) cleanupAfterScaling(dc *cassapi.CassandraDatacenter) error {
	// We sort to ensure we process the pods in the same order
	sort.Slice(rc.dcPods, func(i, j int) bool {
		rackI := rc.dcPods[i].Labels[api.RackLabel]
		rackJ := rc.dcPods[j].Labels[api.RackLabel]

		if rackI != rackJ {
			return rackI < rackJ
		}

		return rc.dcPods[i].Name < rc.dcPods[j].Name
	})

	for idx, pod := range rc.dcPods {
		features, err := rc.NodeMgmtClient.FeatureSet(pod)
		if err != nil {
			return result.Error(err)
		}

		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}

		if podJobId, found := pod.Annotations[podJobIdAnnotation]; found {
			if features.Supports(httphelper.AsyncSSTableTasks) {
				_, found := pod.Annotations[podJobStatusAnnotation]
				if !found {
					podPatch := client.MergeFrom(pod.DeepCopy())
					// Pod is currently processing something, or has finished processing.. update the status
					details, err := rc.NodeMgmtClient.JobDetails(pod, podJobId)
					if err != nil {
						rc.ReqLogger.Error(err, "Could not get JobDetails for pod", "Pod", pod)
						return result.Error(err)
					}

					if details.Id == "" {
						// This job was not found, pod most likely restarted. Let's retry..
						delete(pod.Annotations, podJobIdAnnotation)
						err = rc.Client.Patch(rc.GetContext(), pod, podPatch)
						if err != nil {
							return result.Error(err)
						}
						return result.RequeueSoon(1)
					} else if details.Status == podJobError {
						// Log the error, move on
						rc.ReqLogger.Error(fmt.Errorf("cleanup failed: %s", details.Error), "Job failed to successfully complete the cleanup", "Pod", pod)
						pod.Annotations[podJobStatusAnnotation] = podJobError
						err = rc.Client.Patch(rc.GetContext(), pod, podPatch)
						if err != nil {
							return result.Error(err)
						}
						continue
					} else if details.Status == podJobCompleted {
						// Pod has finished, remove the job_id and let us move to the next pod
						pod.Annotations[podJobStatusAnnotation] = podJobCompleted
						err = rc.Client.Patch(rc.GetContext(), pod, podPatch)
						if err != nil {
							return result.Error(err)
						}
						continue
					} else if details.Status == podJobWaiting {
						// Job is still running or waiting
						return result.RequeueSoon(10)
					}
				} else {
					// This pod has finished since it has a status
					continue
				}
			} else {
				if len(jobRunner) > 0 {
					// Something is still holding the worker
					return result.RequeueSoon(10)
				}

				// Nothing is holding the job, has this pod finished or hasn't it ran anything?
				if _, found := pod.Annotations[podJobStatusAnnotation]; found {
					// Pod has finished, check next one
					continue
				}
			}
		}

		if features.Supports(httphelper.AsyncSSTableTasks) {
			// Pod isn't running anything at the moment, this pod should run next
			jobId, err := rc.NodeMgmtClient.CallKeyspaceCleanup(pod, -1, "", nil)
			if err != nil {
				// We can retry this later, it will only restart the cleanup but won't otherwise hurt
				return result.Error(err)
			}
			podPatch := client.MergeFrom(pod.DeepCopy())
			pod.Annotations[podJobHandler] = jobHandlerMgmtApi
			pod.Annotations[podJobIdAnnotation] = jobId

			err = rc.Client.Patch(rc.GetContext(), pod, podPatch)
			if err != nil {
				rc.ReqLogger.Error(err, "Failed to patch pod's status to include jobId", "Pod", pod)
				return result.Error(err)
			}
		} else {
			jobId := strconv.Itoa(idx)

			// This pod should run next, mark it
			podPatch := client.MergeFrom(pod.DeepCopy())
			pod.Annotations[podJobHandler] = oplabels.ManagedByLabelValue
			pod.Annotations[podJobIdAnnotation] = jobId

			err = rc.Client.Patch(rc.GetContext(), pod, podPatch)
			if err != nil {
				rc.ReqLogger.Error(err, "Failed to patch pod's status to indicate its running local job", "Pod", pod)
				return result.Error(err)
			}

			pod := pod

			go func(targetPod *corev1.Pod) {
				// Write value to the jobRunner to indicate we're running
				jobRunner <- idx
				defer func() {
					// Read the value from the jobRunner
					<-jobRunner
				}()

				podPatch := client.MergeFrom(targetPod.DeepCopy())
				if err = rc.NodeMgmtClient.CallKeyspaceCleanupEndpoint(targetPod, -1, "", nil); err != nil {
					// We only log, nothing else to do - we won't even retry this pod
					rc.ReqLogger.Error(err, "Pod", targetPod)
					pod.Annotations[podJobStatusAnnotation] = podJobError
				} else {
					pod.Annotations[podJobStatusAnnotation] = podJobCompleted
				}

				err = rc.Client.Patch(rc.GetContext(), pod, podPatch)
				if err != nil {
					rc.ReqLogger.Error(err, "Failed to update local job's status", "Pod", targetPod)
				}
			}(pod)
		}

		// We have a job going on, return back later to check the status
		return result.RequeueSoon(10)
	}
	return result.Continue()
}
