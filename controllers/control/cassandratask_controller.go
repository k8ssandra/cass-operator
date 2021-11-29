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
	corev1 "k8s.io/api/core/v1"
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

const (
	taskLabel               = "control.k8ssandra.io/status"
	activeTaskLabelValue    = "active"
	completedTaskLabelValue = "completed"
)

// CassandraTaskReconciler reconciles a CassandraJob object
type CassandraTaskReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// AsyncTaskExecutorFunc is called for all methods that support async processing
type AsyncTaskExecutorFunc func(httphelper.NodeMgmtClient, *corev1.Pod, map[string]string) (string, error)

// SyncTaskExecutorFunc is called as a backup if async one isn't supported
type SyncTaskExecutorFunc func(httphelper.NodeMgmtClient, *corev1.Pod, map[string]string) error

// TaskConfiguration sets the command's functions to execute
type TaskConfiguration struct {
	AsyncFunc    AsyncTaskExecutorFunc
	SyncFunc     SyncTaskExecutorFunc
	AsyncFeature httphelper.Feature
	Arguments    map[string]string
}

//+kubebuilder:rbac:groups=control.k8ssandra.io,namespace=cass-operator,resources=cassandrajobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=control.k8ssandra.io,namespace=cass-operator,resources=cassandrajobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=control.k8ssandra.io,namespace=cass-operator,resources=cassandrajobs/finalizers,verbs=update

// Do we need to repeat this? It's already on the cassandradatacenter_controller.go
//+kubebuilder:rbac:groups=core,namespace=cass-operator,resources=pods;events,verbs=get;list;watch;create;update;patch;delete

func (r *CassandraTaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cassJob api.CassandraTask
	if err := r.Get(ctx, req.NamespacedName, &cassJob); err != nil {
		logger.Error(err, "unable to fetch CassandraTask", "Request", req.NamespacedName)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	timeNow := metav1.Now()

	if cassJob.Spec.ScheduledTime != nil && timeNow.Before(cassJob.Spec.ScheduledTime) {
		// TODO ScheduledTime is before current time, requeue for later processing
		logger.V(1).Info("this job isn't scheduled to be run yet", "Request", req.NamespacedName)
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
				logger.V(1).Info("this task is scheduled to be deleted now", "Request", req.NamespacedName)
			} else {
				// Reschedule for later deletion
				logger.V(1).Info("this task is scheduled to be deleted later", "Request", req.NamespacedName)
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
		logger.Error(err, "unable to fetch CassandraDatacenter", "Datacenter", cassJob.Spec.Datacenter)
		// This is unrecoverable error at this point, do not requeue
		return ctrl.Result{}, err
	}

	logger = log.FromContext(ctx, "datacenterName", dc.Name, "clusterName", dc.Spec.ClusterName)
	log.IntoContext(ctx, logger)

	if err := controllerutil.SetOwnerReference(&dc.ObjectMeta, &cassJob.ObjectMeta, r.Scheme); err != nil {
		logger.Error(err, "unable to set ownerReference to the task", "Datacenter", cassJob.Spec.Datacenter, "CassandraTask", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// TODO Does our concurrencypolicy allow the task to run? Are there any other active ones?
	activeTasks, err := r.activeTasks(ctx, &dc)
	if err != nil {
		logger.Error(err, "unable to fetch active CassandraTasks", "Request", req.NamespacedName)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if len(activeTasks) > 0 {
		// TODO Exclude this current job
		if *cassJob.Spec.ConcurrencyPolicy == batchv1.ForbidConcurrent {
			// TODO Can't run right now, requeue
			// TODO Or should we push an event?
			logger.V(1).Info("this job isn't allowed to run due to ConcurrencyPolicy restrictions", "activeTasks", len(activeTasks))
			return ctrl.Result{Requeue: true}, nil // TODO Add some sane time here
		}
		// TODO There are other tasks running, are they allowing or forbiding the concurrent work?
		for _, task := range activeTasks {
			if *task.Spec.ConcurrencyPolicy == batchv1.ForbidConcurrent {
				logger.V(1).Info("this job isn't allowed to run due to ConcurrencyPolicy restrictions", "activeTasks", len(activeTasks))
				return ctrl.Result{Requeue: true}, nil // TODO Add some sane time here
			}
		}
	}

	var res ctrl.Result
	completedCount := int32(0)

	// TODO We only support a single job at this stage (helps dev work)
	if len(cassJob.Spec.Jobs) > 1 {
		return ctrl.Result{}, fmt.Errorf("only a single job can be defined in this version of cass-operator")
	}

	for _, job := range cassJob.Spec.Jobs {
		// TODO This should check the status of the job from Status, not reconciling it (we overwrite the jobId in those cases)
		switch job.Command {
		case "rebuild":
			res, err = r.reconcileEveryPodTask(ctx, &dc, rebuild(job.Arguments))
		case "cleanup":
			res, err = r.reconcileEveryPodTask(ctx, &dc, cleanup())
		case "decommission":
			// res, err = r.reconcileEveryPodTask(ctx, &dc, decommission())
		default:
			err = fmt.Errorf("unknown job command: %s", job.Command)
			return ctrl.Result{}, err
		}

		if err != nil {
			return ctrl.Result{}, err
		}

		if res.RequeueAfter > 0 {
			// This job isn't complete yet or there's an error, do not continue
			break
		}
		completedCount++
	}

	// TODO Starting the run, set the Active label so we can quickly fetch the active ones
	if taskLabel, found := cassJob.GetLabels()[taskLabel]; found {
		if res.RequeueAfter == 0 {
			// Job has been completed
			cassJob.GetLabels()[taskLabel] = completedTaskLabelValue
			cassJob.Status.Active = 0
			cassJob.Status.CompletionTime = &timeNow
			if err = r.Client.Update(ctx, &cassJob); err != nil {
				return res, err
			}
		}
	} else {
		cassJob.GetLabels()[taskLabel] = activeTaskLabelValue
		cassJob.Status.StartTime = &timeNow
		cassJob.Status.Active = 1 // We don't have concurrency inside a task at the moment
		if err = r.Client.Update(ctx, &cassJob); err != nil {
			return res, err
		}
	}

	if cassJob.Status.Succeeded != completedCount {
		cassJob.Status.Succeeded = completedCount

		if err = r.Client.Status().Update(ctx, &cassJob); err != nil {
			return res, err
		}
	}

	// TODO Add conditions also

	// log.V(1).Info for DEBUG logging
	return res, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *CassandraTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.CassandraTask{}).
		Complete(r)
}

func (r *CassandraTaskReconciler) activeTasks(ctx context.Context, dc *cassapi.CassandraDatacenter) ([]api.CassandraTask, error) {
	var taskList api.CassandraTaskList
	// TODO This should add per-datacenter active (we can execute concurrently in different datacenters)
	if err := r.Client.List(ctx, &taskList, client.InNamespace(dc.Namespace), client.MatchingLabels{taskLabel: activeTaskLabelValue}); err != nil {
		return nil, err
	}

	return taskList.Items, nil
}

/**
	Jobs executing something on every pod in the datacenter
**/
const (
	podJobIdAnnotation     = "control.k8ssandra.io/job-id"
	podJobStatusAnnotation = "control.k8ssandra.io/job-status"
	podJobHandler          = "control.k8ssandra.io/job-runner"
	jobHandlerMgmtApi      = "management-api"

	podJobCompleted = "COMPLETED"
	podJobError     = "ERROR"
	podJobWaiting   = "WAITING"

	jobRunningRequeue = time.Duration(10 * time.Second)
)

var (
	// TODO This should be per Datacenter
	jobRunner chan int = make(chan int, 1)
)

func callCleanup(nodeMgmtClient httphelper.NodeMgmtClient, pod *corev1.Pod, args map[string]string) (string, error) {
	return nodeMgmtClient.CallKeyspaceCleanup(pod, -1, "", nil)
}

func callCleanupSync(nodeMgmtClient httphelper.NodeMgmtClient, pod *corev1.Pod, args map[string]string) error {
	return nodeMgmtClient.CallKeyspaceCleanupEndpoint(pod, -1, "", nil)
}

func cleanup() TaskConfiguration {
	return TaskConfiguration{
		AsyncFeature: httphelper.AsyncSSTableTasks,
		AsyncFunc:    callCleanup,
		SyncFunc:     callCleanupSync,
	}
}

func callRebuild(nodeMgmtClient httphelper.NodeMgmtClient, pod *corev1.Pod, args map[string]string) (string, error) {
	return nodeMgmtClient.CallDatacenterRebuild(pod, args["source_datacenter"])
}

func rebuild(args map[string]string) TaskConfiguration {
	return TaskConfiguration{
		AsyncFeature: httphelper.Rebuild,
		AsyncFunc:    callRebuild,
		Arguments:    args,
	}
}

func (r *CassandraTaskReconciler) getDatacenterPods(ctx context.Context, dc *cassapi.CassandraDatacenter) ([]corev1.Pod, error) {
	var pods corev1.PodList

	if err := r.Client.List(ctx, &pods, client.InNamespace(dc.Namespace), client.MatchingLabels(dc.GetDatacenterLabels())); err != nil {
		return nil, err
	}

	return pods.Items, nil
}

// TODO reconcile_racks has similar function, so refactor them to same place
func (r *CassandraTaskReconciler) initializeMgmtClient(ctx context.Context, dc *cassapi.CassandraDatacenter) (httphelper.NodeMgmtClient, error) {
	logger := log.FromContext(ctx)

	httpClient, err := httphelper.BuildManagementApiHttpClient(dc, r.Client, ctx)
	if err != nil {
		logger.Error(err, "error in BuildManagementApiHttpClient")
		return httphelper.NodeMgmtClient{}, err
	}

	protocol, err := httphelper.GetManagementApiProtocol(dc)
	if err != nil {
		logger.Error(err, "error in GetManagementApiProtocol")
		return httphelper.NodeMgmtClient{}, err
	}

	return httphelper.NodeMgmtClient{
		Client:   httpClient,
		Log:      logger,
		Protocol: protocol,
	}, nil
}

func (r *CassandraTaskReconciler) reconcileEveryPodTask(ctx context.Context, dc *cassapi.CassandraDatacenter, taskConfig TaskConfiguration) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// We sort to ensure we process the dcPods in the same order
	dcPods, err := r.getDatacenterPods(ctx, dc)
	if err != nil {
		return ctrl.Result{}, err
	}

	sort.Slice(dcPods, func(i, j int) bool {
		rackI := dcPods[i].Labels[cassapi.RackLabel]
		rackJ := dcPods[j].Labels[cassapi.RackLabel]

		if rackI != rackJ {
			return rackI < rackJ
		}

		return dcPods[i].Name < dcPods[j].Name
	})

	nodeMgmtClient, err := r.initializeMgmtClient(ctx, dc)
	if err != nil {
		return ctrl.Result{}, err
	}

	for idx, pod := range dcPods {
		features, err := nodeMgmtClient.FeatureSet(&pod)
		if err != nil {
			return ctrl.Result{}, err
		}

		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}

		if podJobId, found := pod.Annotations[podJobIdAnnotation]; found {
			if features.Supports(taskConfig.AsyncFeature) {
				_, found := pod.Annotations[podJobStatusAnnotation]
				if !found {
					podPatch := client.MergeFrom(pod.DeepCopy())
					// Pod is currently processing something, or has finished processing.. update the status
					details, err := nodeMgmtClient.JobDetails(&pod, podJobId)
					if err != nil {
						logger.Error(err, "Could not get JobDetails for pod", "Pod", pod)
						return ctrl.Result{}, err
					}

					if details.Id == "" {
						// This job was not found, pod most likely restarted. Let's retry..
						delete(pod.Annotations, podJobIdAnnotation)
						err = r.Client.Patch(ctx, &pod, podPatch)
						if err != nil {
							return ctrl.Result{}, err
						}
						return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
					} else if details.Status == podJobError {
						// Log the error, move on
						logger.Error(fmt.Errorf("task failed: %s", details.Error), "Job failed to successfully complete the task", "Pod", pod)
						pod.Annotations[podJobStatusAnnotation] = podJobError
						err = r.Client.Patch(ctx, &pod, podPatch)
						if err != nil {
							return ctrl.Result{}, err
						}
						continue
					} else if details.Status == podJobCompleted {
						// Pod has finished, remove the job_id and let us move to the next pod
						pod.Annotations[podJobStatusAnnotation] = podJobCompleted
						err = r.Client.Patch(ctx, &pod, podPatch)
						if err != nil {
							return ctrl.Result{}, err
						}
						continue
					} else if details.Status == podJobWaiting {
						// Job is still running or waiting
						return ctrl.Result{RequeueAfter: jobRunningRequeue}, nil
					}
				} else {
					// This pod has finished since it has a status
					continue
				}
			} else {
				if len(jobRunner) > 0 {
					// Something is still holding the worker
					return ctrl.Result{RequeueAfter: jobRunningRequeue}, nil
				}

				// Nothing is holding the job, has this pod finished or hasn't it ran anything?
				if _, found := pod.Annotations[podJobStatusAnnotation]; found {
					// Pod has finished, check next one
					continue
				}
			}
		}

		if features.Supports(taskConfig.AsyncFeature) {
			// Pod isn't running anything at the moment, this pod should run next
			jobId, err := taskConfig.AsyncFunc(nodeMgmtClient, &pod, taskConfig.Arguments)
			// jobId, err := rc.NodeMgmtClient.CallKeyspaceCleanup(pod, -1, "", nil)
			if err != nil {
				// We can retry this later, it will only restart the cleanup but won't otherwise hurt
				return ctrl.Result{}, err
			}
			podPatch := client.MergeFrom(pod.DeepCopy())
			pod.Annotations[podJobHandler] = jobHandlerMgmtApi
			pod.Annotations[podJobIdAnnotation] = jobId

			err = r.Client.Patch(ctx, &pod, podPatch)
			if err != nil {
				logger.Error(err, "Failed to patch pod's status to include jobId", "Pod", pod)
				return ctrl.Result{}, err
			}
		} else {
			if taskConfig.SyncFunc == nil {
				// This feature is not supported in sync mode, mark everything as done
				return ctrl.Result{}, nil
			}

			jobId := strconv.Itoa(idx)

			// This pod should run next, mark it
			podPatch := client.MergeFrom(pod.DeepCopy())
			pod.Annotations[podJobHandler] = oplabels.ManagedByLabelValue
			pod.Annotations[podJobIdAnnotation] = jobId

			err = r.Client.Patch(ctx, &pod, podPatch)
			if err != nil {
				logger.Error(err, "Failed to patch pod's status to indicate its running a local job", "Pod", pod)
				return ctrl.Result{}, err
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
				if err = taskConfig.SyncFunc(nodeMgmtClient, targetPod, taskConfig.Arguments); err != nil {
					// if err = rc.NodeMgmtClient.CallKeyspaceCleanupEndpoint(targetPod, -1, "", nil); err != nil {
					// We only log, nothing else to do - we won't even retry this pod
					logger.Error(err, "Pod", targetPod)
					pod.Annotations[podJobStatusAnnotation] = podJobError
				} else {
					pod.Annotations[podJobStatusAnnotation] = podJobCompleted
				}

				err = r.Client.Patch(ctx, &pod, podPatch)
				if err != nil {
					logger.Error(err, "Failed to update local job's status", "Pod", targetPod)
				}
			}(&pod)
		}

		// We have a job going on, return back later to check the status
		return ctrl.Result{RequeueAfter: jobRunningRequeue}, nil
	}
	return ctrl.Result{}, nil
}
