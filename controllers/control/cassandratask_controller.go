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
	"encoding/json"
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
	"github.com/k8ssandra/cass-operator/pkg/utils"
)

const (
	taskStatusLabel         = "control.k8ssandra.io/status"
	activeTaskLabelValue    = "active"
	completedTaskLabelValue = "completed"
	defaultTTL              = time.Duration(86400) * time.Second
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
	Id            string
	AsyncFunc     AsyncTaskExecutorFunc
	SyncFunc      SyncTaskExecutorFunc
	AsyncFeature  httphelper.Feature
	RestartPolicy corev1.RestartPolicy
	Arguments     map[string]string
}

//+kubebuilder:rbac:groups=control.k8ssandra.io,namespace=cass-operator,resources=cassandratasks,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=control.k8ssandra.io,namespace=cass-operator,resources=cassandratasks/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=control.k8ssandra.io,namespace=cass-operator,resources=cassandratasks/finalizers,verbs=update

//+kubebuilder:rbac:groups=core,namespace=cass-operator,resources=pods;events,verbs=get;list;watch;create;update;patch;delete

func (r *CassandraTaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cassTask api.CassandraTask
	if err := r.Get(ctx, req.NamespacedName, &cassTask); err != nil {
		logger.Error(err, "unable to fetch CassandraTask", "Request", req.NamespacedName)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if cassTask.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	setDefaults(&cassTask)

	timeNow := metav1.Now()

	if cassTask.Spec.ScheduledTime != nil && timeNow.Before(cassTask.Spec.ScheduledTime) {
		logger.V(1).Info("this job isn't scheduled to be run yet", "Request", req.NamespacedName)
		nextRunTime := cassTask.Spec.ScheduledTime.Sub(timeNow.Time)
		return ctrl.Result{RequeueAfter: nextRunTime}, nil
	}

	calculateDeletionTime := func() time.Time {
		var deletionTime time.Time
		if cassTask.Spec.TTLSecondsAfterFinished != nil {
			deletionTime = cassTask.Status.CompletionTime.Add(time.Duration(*cassTask.Spec.TTLSecondsAfterFinished) * time.Second)
		} else {
			deletionTime = cassTask.Status.CompletionTime.Add(defaultTTL)
		}

		return deletionTime
	}

	// If we're active, we can proceed - otherwise verify if we're allowed to run
	status, found := cassTask.GetLabels()[taskStatusLabel]

	if cassTask.Status.CompletionTime == nil && status == completedTaskLabelValue {
		// This is out of sync
		return ctrl.Result{Requeue: true}, nil
	}

	// Check if job is finished, and if and only if, check the TTL from last finished time.
	if cassTask.Status.CompletionTime != nil {
		deletionTime := calculateDeletionTime()

		// Nothing more to do here, other than delete it..
		if deletionTime.Before(timeNow.Time) {
			logger.V(1).Info("this task is scheduled to be deleted now", "Request", req.NamespacedName, "deletionTime", deletionTime.String())
			err := r.Delete(ctx, &cassTask)
			// Do not requeue anymore, this task has been completed
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		// Reschedule for later deletion
		logger.V(1).Info("this task is scheduled to be deleted later", "Request", req.NamespacedName)
		nextRunTime := deletionTime.Sub(timeNow.Time)
		return ctrl.Result{RequeueAfter: nextRunTime}, nil
	}

	// Get the CassandraDatacenter
	dc := cassapi.CassandraDatacenter{}
	dcNamespacedName := types.NamespacedName{
		Namespace: cassTask.Spec.Datacenter.Namespace,
		Name:      cassTask.Spec.Datacenter.Name,
	}
	if err := r.Get(ctx, dcNamespacedName, &dc); err != nil {
		logger.Error(err, "unable to fetch CassandraDatacenter", "Datacenter", cassTask.Spec.Datacenter)
		// This is unrecoverable error at this point, do not requeue
		return ctrl.Result{}, err
	}

	logger = log.FromContext(ctx, "datacenterName", dc.Name, "clusterName", dc.Spec.ClusterName)
	log.IntoContext(ctx, logger)

	// If we're active, we can proceed - otherwise verify if we're allowed to run
	if !found {
		// Does our concurrencypolicy allow the task to run? Are there any other active ones?
		activeTasks, err := r.activeTasks(ctx, &dc)
		if err != nil {
			logger.Error(err, "unable to fetch active CassandraTasks", "Request", req.NamespacedName)
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		// Remove current job from the slice
		for index, task := range activeTasks {
			if task.Name == req.Name && task.Namespace == req.Namespace {
				activeTasks = append(activeTasks[:index], activeTasks[index+1:]...)
			}
		}

		if len(activeTasks) > 0 {
			if cassTask.Spec.ConcurrencyPolicy == batchv1.ForbidConcurrent {
				// TODO Or should we push an event?
				logger.V(1).Info("this job isn't allowed to run due to ConcurrencyPolicy restrictions", "activeTasks", len(activeTasks))
				return ctrl.Result{Requeue: true}, nil // TODO Add some sane time here
			}
			for _, task := range activeTasks {
				if task.Spec.ConcurrencyPolicy == batchv1.ForbidConcurrent {
					logger.V(1).Info("this job isn't allowed to run due to ConcurrencyPolicy restrictions", "activeTasks", len(activeTasks))
					return ctrl.Result{Requeue: true}, nil // TODO Add some sane time here
				}
			}
		}

		// Link this resource to the Datacenter and copy labels from it
		if err := controllerutil.SetOwnerReference(&dc, &cassTask, r.Scheme); err != nil {
			logger.Error(err, "unable to set ownerReference to the task", "Datacenter", cassTask.Spec.Datacenter, "CassandraTask", req.NamespacedName)
			return ctrl.Result{}, err
		}

		utils.MergeMap(cassTask.Labels, dc.GetDatacenterLabels())
		oplabels.AddOperatorLabels(cassTask.GetLabels(), &dc)

		// Starting the run, set the Active label so we can quickly fetch the active ones
		cassTask.GetLabels()[taskStatusLabel] = activeTaskLabelValue

		if err := r.Client.Update(ctx, &cassTask); err != nil {
			return ctrl.Result{}, err
		}

		cassTask.Status.StartTime = &timeNow
		cassTask.Status.Active = 1 // We don't have concurrency inside a task at the moment
	}

	var res ctrl.Result
	// completedCount := int32(0)

	// TODO We only support a single job at this stage (helps dev work)
	if len(cassTask.Spec.Jobs) > 1 {
		return ctrl.Result{}, fmt.Errorf("only a single job can be defined in this version of cass-operator")
	}

	// TODO We need to verify that the cluster is actually healthy before we run these..
	taskId := string(cassTask.UID)

	var err error
	var failed, completed int
	for _, job := range cassTask.Spec.Jobs {
		taskConfigProto := &TaskConfiguration{
			RestartPolicy: cassTask.Spec.RestartPolicy,
			Id:            taskId,
		}
		switch job.Command {
		case "rebuild":
			res, failed, completed, err = r.reconcileEveryPodTask(ctx, &dc, rebuild(taskConfigProto, job.Arguments))
		case "cleanup":
			res, failed, completed, err = r.reconcileEveryPodTask(ctx, &dc, cleanup(taskConfigProto))
		case "decommission":
			// res, err = r.reconcileEveryPodTask(ctx, taskId, &dc, decommission())
		default:
			err = fmt.Errorf("unknown job command: %s", job.Command)
			return ctrl.Result{}, err
		}

		// TODO How would we detect what sort of error we've had here? For the correct reconcile action
		if err != nil {
			return ctrl.Result{}, err
		}

		if res.RequeueAfter > 0 {
			// This job isn't complete yet or there's an error, do not continue
			break
		}
		// completedCount++
	}

	if res.RequeueAfter == 0 && !res.Requeue {
		// Job has been completed
		cassTask.GetLabels()[taskStatusLabel] = completedTaskLabelValue
		if err = r.Client.Update(ctx, &cassTask); err != nil {
			return res, err
		}

		r.cleanupJobAnnotations(ctx, &dc, taskId)

		cassTask.Status.Active = 0
		cassTask.Status.CompletionTime = &timeNow

		// Requeue for deletion later
		deletionTime := calculateDeletionTime()
		nextRunTime := deletionTime.Sub(timeNow.Time)
		res.RequeueAfter = nextRunTime
		logger.V(1).Info("This task has completed, scheduling for deletion in " + nextRunTime.String())
	}

	cassTask.Status.Succeeded = completed
	cassTask.Status.Failed = failed

	// TODO Add conditions also
	if err = r.Client.Status().Update(ctx, &cassTask); err != nil {
		return ctrl.Result{}, err
	}

	// log.V(1).Info for DEBUG logging
	return res, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *CassandraTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.CassandraTask{}).
		Complete(r)
}

func setDefaults(cassTask *api.CassandraTask) {
	if cassTask.GetLabels() == nil {
		cassTask.Labels = make(map[string]string)
	}

	if cassTask.Spec.RestartPolicy == "" {
		cassTask.Spec.RestartPolicy = corev1.RestartPolicyNever
	}

	if cassTask.Spec.ConcurrencyPolicy == "" {
		cassTask.Spec.ConcurrencyPolicy = batchv1.ForbidConcurrent
	}
}

func (r *CassandraTaskReconciler) activeTasks(ctx context.Context, dc *cassapi.CassandraDatacenter) ([]api.CassandraTask, error) {
	var taskList api.CassandraTaskList
	matcher := client.MatchingLabels(utils.MergeMap(dc.GetDatacenterLabels(), map[string]string{taskStatusLabel: activeTaskLabelValue}))
	if err := r.Client.List(ctx, &taskList, client.InNamespace(dc.Namespace), matcher); err != nil {
		return nil, err
	}

	return taskList.Items, nil
}

/**
	Jobs executing something on every pod in the datacenter
**/
const (
	// PodJobAnnotationPrefix defines the prefix key for a job data (json serialized) in the annotations of the pod
	PodJobAnnotationPrefix = "control.k8ssandra.io/job"
	jobHandlerMgmtApi      = "management-api"

	podJobCompleted = "COMPLETED"
	podJobError     = "ERROR"
	podJobWaiting   = "WAITING"
)

type JobStatus struct {
	Id      string `json:"id,omitempty"`
	Status  string `json:"status,omitempty"`
	Handler string `json:"handler,omitempty"`
	Retries int    `json:"retries,omitempty"`
}

func getJobAnnotationKey(taskId string) string {
	return fmt.Sprintf("%s-%s", PodJobAnnotationPrefix, taskId)
}

// GetJobStatusFromPodAnnotations gets the json serialized pod job statusfrom Pod.Annotations
// and converts it to the Affinity type in api.
func GetJobStatusFromPodAnnotations(taskId string, annotations map[string]string) (JobStatus, error) {
	annotationKey := getJobAnnotationKey(taskId)
	var jobStatus JobStatus
	if jobData, found := annotations[annotationKey]; found {
		err := json.Unmarshal([]byte(jobData), &jobStatus)
		if err != nil {
			return jobStatus, err
		}
	}
	return jobStatus, nil
}

func JobStatusToPodAnnotations(taskId string, annotations map[string]string, jobStatus JobStatus) error {
	outputVal, err := json.Marshal(jobStatus)
	if err != nil {
		return err
	}
	annotationKey := getJobAnnotationKey(taskId)
	annotations[annotationKey] = string(outputVal)
	return nil
}

var (
	// TODO This should be per Datacenter for sync tasks also
	jobRunner         chan int = make(chan int, 1)
	jobRunningRequeue          = time.Duration(10 * time.Second)
)

func callCleanup(nodeMgmtClient httphelper.NodeMgmtClient, pod *corev1.Pod, args map[string]string) (string, error) {
	return nodeMgmtClient.CallKeyspaceCleanup(pod, -1, "", nil)
}

func callCleanupSync(nodeMgmtClient httphelper.NodeMgmtClient, pod *corev1.Pod, args map[string]string) error {
	return nodeMgmtClient.CallKeyspaceCleanupEndpoint(pod, -1, "", nil)
}

func cleanup(taskConfig *TaskConfiguration) *TaskConfiguration {
	taskConfig.AsyncFeature = httphelper.AsyncSSTableTasks
	taskConfig.AsyncFunc = callCleanup
	taskConfig.SyncFunc = callCleanupSync
	return taskConfig
}

func callRebuild(nodeMgmtClient httphelper.NodeMgmtClient, pod *corev1.Pod, args map[string]string) (string, error) {
	return nodeMgmtClient.CallDatacenterRebuild(pod, args["source_datacenter"])
}

func rebuild(taskConfig *TaskConfiguration, args map[string]string) *TaskConfiguration {
	taskConfig.AsyncFeature = httphelper.Rebuild
	taskConfig.AsyncFunc = callRebuild
	taskConfig.Arguments = args
	return taskConfig
}

func (r *CassandraTaskReconciler) getDatacenterPods(ctx context.Context, dc *cassapi.CassandraDatacenter) ([]corev1.Pod, error) {
	var pods corev1.PodList

	if err := r.Client.List(ctx, &pods, client.InNamespace(dc.Namespace), client.MatchingLabels(dc.GetDatacenterLabels())); err != nil {
		return nil, err
	}

	return pods.Items, nil
}

// cleanupJobAnnotations removes the job annotations from the pod once it has finished
func (r *CassandraTaskReconciler) cleanupJobAnnotations(ctx context.Context, dc *cassapi.CassandraDatacenter, taskId string) error {
	logger := log.FromContext(ctx)

	// We sort to ensure we process the dcPods in the same order
	dcPods, err := r.getDatacenterPods(ctx, dc)
	if err != nil {
		return err
	}
	for _, pod := range dcPods {
		podPatch := client.MergeFrom(pod.DeepCopy())
		annotationKey := getJobAnnotationKey(taskId)
		delete(pod.GetAnnotations(), annotationKey)
		err = r.Client.Patch(ctx, &pod, podPatch)
		if err != nil {
			logger.Error(err, "Failed to patch pod's status to include jobId", "Pod", pod)
			return err
		}
	}
	return nil
}

func (r *CassandraTaskReconciler) reconcileEveryPodTask(ctx context.Context, dc *cassapi.CassandraDatacenter, taskConfig *TaskConfiguration) (ctrl.Result, int, int, error) {
	logger := log.FromContext(ctx)

	// We sort to ensure we process the dcPods in the same order
	dcPods, err := r.getDatacenterPods(ctx, dc)
	if err != nil {
		return ctrl.Result{}, 0, 0, err
	}

	sort.Slice(dcPods, func(i, j int) bool {
		rackI := dcPods[i].Labels[cassapi.RackLabel]
		rackJ := dcPods[j].Labels[cassapi.RackLabel]

		if rackI != rackJ {
			return rackI < rackJ
		}

		return dcPods[i].Name < dcPods[j].Name
	})

	nodeMgmtClient, err := httphelper.NewMgmtClient(ctx, r.Client, dc)
	if err != nil {
		return ctrl.Result{}, 0, 0, err
	}

	failed, completed := 0, 0
	for idx, pod := range dcPods {
		features, err := nodeMgmtClient.FeatureSet(&pod)
		if err != nil {
			return ctrl.Result{}, failed, completed, err
		}

		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}

		jobStatus, err := GetJobStatusFromPodAnnotations(taskConfig.Id, pod.Annotations)
		if err != nil {
			return ctrl.Result{}, failed, completed, err
		}

		// podPatch := client.MergeFromWithOptions(pod.DeepCopy(), client.MergeFromWithOptimisticLock{})
		// podPatch := client.MergeFrom(pod.DeepCopy())
		if jobStatus.Id != "" {
			// Check the completed statuses
			if jobStatus.Status == podJobCompleted {
				completed++
				continue
			} else if jobStatus.Status == podJobError {
				failed++
				continue
			}

			// Only if we have "Running" status should we check the jobDetails
			if features.Supports(taskConfig.AsyncFeature) {
				// Pod is currently processing something, or has finished processing.. update the status
				details, err := nodeMgmtClient.JobDetails(&pod, jobStatus.Id)
				if err != nil {
					logger.Error(err, "Could not get JobDetails for pod", "Pod", pod)
					return ctrl.Result{}, failed, completed, err
				}

				if details.Id == "" {
					// This job was not found, pod most likely restarted. Let's retry..
					delete(pod.Annotations, getJobAnnotationKey(taskConfig.Id))
					err = r.Client.Update(ctx, &pod)
					if err != nil {
						return ctrl.Result{}, failed, completed, err
					}
					return ctrl.Result{RequeueAfter: 1 * time.Second}, failed, completed, nil
				} else if details.Status == podJobError {
					// Log the error, move on
					logger.Error(fmt.Errorf("task failed: %s", details.Error), "Job failed to successfully complete the task", "Pod", pod)
					if taskConfig.RestartPolicy != corev1.RestartPolicyOnFailure || jobStatus.Retries >= 1 {
						jobStatus.Status = podJobError
					} else {
						logger.V(1).Info("Restarting the pod due to the RestartPolicy", "Pod", pod)
						jobStatus.Retries++
						jobStatus.Status = podJobWaiting
						jobStatus.Id = "" // Remove id, so next requeue restarts it
					}

					if err = JobStatusToPodAnnotations(taskConfig.Id, pod.Annotations, jobStatus); err != nil {
						return ctrl.Result{}, failed, completed, err
					}

					if err = r.Client.Update(ctx, &pod); err != nil {
						return ctrl.Result{}, failed, completed, err
					}

					if jobStatus.Status == podJobError {
						failed++
						continue
					}
				} else if details.Status == podJobCompleted {
					// Pod has finished, remove the job_id and let us move to the next pod
					jobStatus.Status = podJobCompleted
					// TODO Repeating code - refactor
					if err = JobStatusToPodAnnotations(taskConfig.Id, pod.Annotations, jobStatus); err != nil {
						return ctrl.Result{}, failed, completed, err
					}

					if err = r.Client.Update(ctx, &pod); err != nil {
						return ctrl.Result{}, failed, completed, err
					}
					completed++
					continue
				} else if details.Status == podJobWaiting {
					// Job is still running or waiting
					return ctrl.Result{RequeueAfter: jobRunningRequeue}, failed, completed, nil
				}
			} else {
				if len(jobRunner) > 0 {
					// Something is still holding the worker
					return ctrl.Result{RequeueAfter: jobRunningRequeue}, failed, completed, nil
				}

				// Nothing is holding the job, this pod has finished
				jobStatus.Status = podJobCompleted
				if err = JobStatusToPodAnnotations(taskConfig.Id, pod.Annotations, jobStatus); err != nil {
					return ctrl.Result{}, failed, completed, err
				}

				if err = r.Client.Update(ctx, &pod); err != nil {
					return ctrl.Result{}, failed, completed, err
				}
				completed++
				continue
			}
		}

		if features.Supports(taskConfig.AsyncFeature) {
			// Pod isn't running anything at the moment, this pod should run next
			jobId, err := taskConfig.AsyncFunc(nodeMgmtClient, &pod, taskConfig.Arguments)
			if err != nil {
				// We can retry this later, it will only restart the cleanup but won't otherwise hurt
				return ctrl.Result{}, failed, completed, err
			}
			// podPatch := client.MergeFromWithOptions(pod.DeepCopy(), client.MergeFromWithOptimisticLock{})
			jobStatus.Handler = jobHandlerMgmtApi
			jobStatus.Id = jobId

			if err = JobStatusToPodAnnotations(taskConfig.Id, pod.Annotations, jobStatus); err != nil {
				return ctrl.Result{}, failed, completed, err
			}

			err = r.Client.Update(ctx, &pod)
			if err != nil {
				logger.Error(err, "Failed to patch pod's status to include jobId", "Pod", pod)
				return ctrl.Result{}, failed, completed, err
			}
		} else {
			if taskConfig.SyncFunc == nil {
				// This feature is not supported in sync mode, mark everything as done
				err := fmt.Errorf("this job isn't supported by the target pod")
				logger.Error(err, "unable to execute requested job against pod", "Pod", pod)
				return ctrl.Result{}, failed, completed, err
			}

			jobId := strconv.Itoa(idx)

			// This pod should run next, mark it
			// podPatch := client.MergeFromWithOptions(pod.DeepCopy(), client.MergeFromWithOptimisticLock{})
			jobStatus.Handler = oplabels.ManagedByLabelValue
			jobStatus.Id = jobId

			if err = JobStatusToPodAnnotations(taskConfig.Id, pod.Annotations, jobStatus); err != nil {
				return ctrl.Result{}, failed, completed, err
			}

			err = r.Client.Update(ctx, &pod)
			if err != nil {
				logger.Error(err, "Failed to patch pod's status to indicate its running a local job", "Pod", pod)
				return ctrl.Result{}, failed, completed, err
			}

			pod := pod

			go func(targetPod *corev1.Pod) {
				// Write value to the jobRunner to indicate we're running
				logger.V(1).Info("starting execution of sync blocking job", "Pod", targetPod)
				jobRunner <- idx
				defer func() {
					// Read the value from the jobRunner
					<-jobRunner
				}()

				// podPatch := client.MergeFromWithOptions(targetPod.DeepCopy(), client.MergeFromWithOptimisticLock{})
				// podPatch := client.MergeFrom(targetPod.DeepCopy())
				if err = taskConfig.SyncFunc(nodeMgmtClient, targetPod, taskConfig.Arguments); err != nil {
					// We only log, nothing else to do - we won't even retry this pod
					logger.Error(err, "executing the sync task failed", "Pod", targetPod)
					jobStatus.Status = podJobError
				} else {
					jobStatus.Status = podJobCompleted
				}

				if err = JobStatusToPodAnnotations(taskConfig.Id, pod.Annotations, jobStatus); err != nil {
					logger.Error(err, "Failed to update local job's status", "Pod", targetPod)
				}

				err = r.Client.Update(ctx, &pod)
				if err != nil {
					logger.Error(err, "Failed to update local job's status", "Pod", targetPod)
				}
			}(&pod)
		}

		// We have a job going on, return back later to check the status
		return ctrl.Result{RequeueAfter: jobRunningRequeue}, failed, completed, nil
	}
	return ctrl.Result{}, failed, completed, nil
}
