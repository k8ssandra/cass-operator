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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cassapi "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	api "github.com/k8ssandra/cass-operator/apis/control/v1alpha1"
	"github.com/k8ssandra/cass-operator/pkg/httphelper"
	"github.com/k8ssandra/cass-operator/pkg/oplabels"
	"github.com/k8ssandra/cass-operator/pkg/utils"
	"github.com/pkg/errors"
)

const (
	taskStatusLabel         = "control.k8ssandra.io/status"
	activeTaskLabelValue    = "active"
	completedTaskLabelValue = "completed"
	defaultTTL              = time.Duration(86400) * time.Second
)

// These are vars to allow modifications for testing
var (
	JobRunningRequeue  = 10 * time.Second
	TaskRunningRequeue = 5 * time.Second
)

// CassandraTaskReconciler reconciles a CassandraJob object
type CassandraTaskReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// AsyncTaskExecutorFunc is called for all methods that support async processing
type AsyncTaskExecutorFunc func(httphelper.NodeMgmtClient, *corev1.Pod, *TaskConfiguration) (string, error)

// SyncTaskExecutorFunc is called as a backup if async one isn't supported
type SyncTaskExecutorFunc func(httphelper.NodeMgmtClient, *corev1.Pod, *TaskConfiguration) error

// ValidatorFunc validates that necessary parameters are set for the task. If false is returned, the task
// has failed the validation and the error has the details. If true is returned, the error is transient and
// should be retried.
type ValidatorFunc func(*TaskConfiguration) (bool, error)

// ProcessFunc is a function that's run before the pods are being processed individually, or after
// the pods have been processed.
type ProcessFunc func(*TaskConfiguration) error

// PodFilterFunc approves or rejects the target pod for processing purposes.
type PodFilterFunc func(*corev1.Pod, *TaskConfiguration) bool

// TaskConfiguration sets the command's functions to execute
type TaskConfiguration struct {
	// Meta / status
	Id            string
	TaskStartTime *metav1.Time
	Datacenter    *cassapi.CassandraDatacenter
	Context       context.Context
	Job           api.CassandraCommand

	// Input parameters
	RestartPolicy     corev1.RestartPolicy
	Arguments         api.JobArguments
	MaxConcurrentPods *int
	Retries           *int

	// Execution functionality per pod
	AsyncFeature httphelper.Feature
	AsyncFunc    AsyncTaskExecutorFunc
	SyncFeature  httphelper.Feature
	SyncFunc     SyncTaskExecutorFunc
	PodFilter    PodFilterFunc

	// Functions not targeting the pod
	ValidateFunc   ValidatorFunc
	PreProcessFunc ProcessFunc

	// Status tracking
	Completed int
}

func (t *TaskConfiguration) Validate() (bool, error) {
	if t.ValidateFunc != nil {
		return t.ValidateFunc(t)
	}
	return true, nil
}

func (t *TaskConfiguration) Filter(pod *corev1.Pod) bool {
	if t.PodFilter != nil {
		return t.PodFilter(pod, t)
	}

	return true
}

func (t *TaskConfiguration) PreProcess() error {
	if t.PreProcessFunc != nil {
		return t.PreProcessFunc(t)
	}
	return nil
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

	// If we're active, we can proceed - otherwise verify if we're allowed to run
	status, found := cassTask.GetLabels()[taskStatusLabel]

	if cassTask.Status.CompletionTime == nil && status == completedTaskLabelValue {
		// This is out of sync
		return ctrl.Result{RequeueAfter: 200 * time.Millisecond}, nil
	}

	// Check if job is finished, and if and only if, check the TTL from last finished time.
	if cassTask.Status.CompletionTime != nil {
		deletionTime := calculateDeletionTime(&cassTask)

		if deletionTime.IsZero() {
			// This task does not have a TTL, we end the processing here
			return ctrl.Result{}, nil
		}

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
	dc := &cassapi.CassandraDatacenter{}
	dcNamespacedName := types.NamespacedName{
		Namespace: cassTask.Spec.Datacenter.Namespace,
		Name:      cassTask.Spec.Datacenter.Name,
	}
	if err := r.Get(ctx, dcNamespacedName, dc); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "unable to fetch target CassandraDatacenter: %s", cassTask.Spec.Datacenter.Name)
	}

	logger = log.FromContext(ctx, "datacenterName", dc.LabelResourceName(), "clusterName", dc.Spec.ClusterName)
	log.IntoContext(ctx, logger)

	// If we're active, we can proceed - otherwise verify if we're allowed to run
	if !found {
		// Does our concurrencypolicy allow the task to run? Are there any other active ones?
		activeTasks, err := r.activeTasks(ctx, dc)
		if err != nil {
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
				logger.V(1).Info("this job isn't allowed to run due to ConcurrencyPolicy restrictions", "activeTasks", len(activeTasks))
				return ctrl.Result{RequeueAfter: TaskRunningRequeue}, nil
			}
			for _, task := range activeTasks {
				if task.Spec.ConcurrencyPolicy == batchv1.ForbidConcurrent {
					logger.V(1).Info("this job isn't allowed to run due to ConcurrencyPolicy restrictions", "activeTasks", len(activeTasks))
					return ctrl.Result{RequeueAfter: TaskRunningRequeue}, nil
				}
			}
		}

		// Link this resource to the Datacenter and copy labels from it
		if err := controllerutil.SetControllerReference(dc, &cassTask, r.Scheme); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "unable to set ownerReference to the task %v", req.NamespacedName)
		}

		utils.MergeMap(cassTask.Labels, dc.GetDatacenterLabels())
		oplabels.AddOperatorMetadata(&cassTask.ObjectMeta, dc)

		// Starting the run, set the Active label so we can quickly fetch the active ones
		cassTask.GetLabels()[taskStatusLabel] = activeTaskLabelValue

		if err := r.Update(ctx, &cassTask); err != nil {
			return ctrl.Result{}, err
		}

		cassTask.Status.StartTime = &timeNow
		cassTask.Status.Active = 1 // We don't have concurrency inside a task at the moment

		if err := r.Status().Update(ctx, &cassTask); err != nil {
			return ctrl.Result{}, err
		}
	}

	var res ctrl.Result

	// We only support a single job at this stage
	if len(cassTask.Spec.Jobs) > 1 {
		return ctrl.Result{}, fmt.Errorf("only a single job can be defined in this version of cass-operator")
	}

	taskName := cassTask.Name

	res, failed, completed, err := r.processJobs(ctx, dc, &cassTask, taskName)
	if err != nil && !errors.Is(err, reconcile.TerminalError(nil)) {
		return ctrl.Result{}, err
	}

	// Since we do patch/update calls on the processJobs, re-fetch the cassTask
	if err := r.Get(ctx, req.NamespacedName, &cassTask); err != nil {
		return res, err
	}

	if res.RequeueAfter == 0 {
		// Job has been completed
		cassTask.GetLabels()[taskStatusLabel] = completedTaskLabelValue
		if errUpdate := r.Update(ctx, &cassTask); errUpdate != nil {
			return res, errUpdate
		}

		cassTask.Status.Active = 0
		cassTask.Status.CompletionTime = &timeNow
		SetCondition(&cassTask, api.JobComplete, metav1.ConditionTrue, "")
		SetCondition(&cassTask, api.JobRunning, metav1.ConditionFalse, "")

		if failed > 0 {
			errMsg := fmt.Sprintf("%d pods failed during processing", failed)
			if err != nil && errors.Is(err, reconcile.TerminalError(nil)) {
				errMsg = err.Error()
			}
			SetCondition(&cassTask, api.JobFailed, metav1.ConditionTrue, errMsg)
		}

		// Requeue for deletion later
		deletionTime := calculateDeletionTime(&cassTask)
		nextRunTime := deletionTime.Sub(timeNow.Time)
		res.RequeueAfter = nextRunTime
		logger.V(1).Info("This task has completed, scheduling for deletion in " + nextRunTime.String())
	}

	podSucceeded := 0
	for _, podStatus := range cassTask.Status.PodStatuses {
		if podStatus.Status == api.PodCompleted {
			podSucceeded++
		}
	}

	if cassTask.Spec.Jobs[0].Command == api.CommandReplaceNode && podSucceeded > completed {
		// This is to offset one-off error from cache timings with replace node's Pod deletion
		completed = podSucceeded
	}

	cassTask.Status.Succeeded = completed
	cassTask.Status.Failed = failed

	if err = r.Status().Update(ctx, &cassTask); err != nil {
		return ctrl.Result{}, err
	}

	return res, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *CassandraTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.CassandraTask{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}

func (r *CassandraTaskReconciler) HasCondition(task *api.CassandraTask, condition api.JobConditionType, status metav1.ConditionStatus) bool {
	for _, cond := range task.Status.Conditions {
		if cond.Type == string(condition) {
			return cond.Status == status
		}
	}
	return false
}

func SetCondition(task *api.CassandraTask, condition api.JobConditionType, status metav1.ConditionStatus, message string) bool {
	existing := false
	for i := 0; i < len(task.Status.Conditions); i++ {
		cond := task.Status.Conditions[i]
		if cond.Type == string(condition) {
			if cond.Status == status {
				// Already correct status
				return false
			}
			cond.Status = status
			cond.LastTransitionTime = metav1.Now()
			if message != "" {
				cond.Message = message
			}
			existing = true
			task.Status.Conditions[i] = cond
			break
		}
	}

	if !existing {
		cond := metav1.Condition{
			Type:               string(condition),
			Reason:             string(condition),
			Status:             status,
			LastTransitionTime: metav1.Now(),
		}
		if message != "" {
			cond.Message = message
		}
		task.Status.Conditions = append(task.Status.Conditions, cond)
	}

	return true
}

func setDefaults(cassTask *api.CassandraTask) {
	if cassTask.GetLabels() == nil {
		cassTask.Labels = make(map[string]string)
	}

	if cassTask.Spec.RestartPolicy == "" {
		cassTask.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
	}

	if cassTask.Spec.ConcurrencyPolicy == "" {
		cassTask.Spec.ConcurrencyPolicy = batchv1.ForbidConcurrent
	}
}

func calculateDeletionTime(cassTask *api.CassandraTask) time.Time {
	if cassTask.Spec.TTLSecondsAfterFinished != nil {
		if *cassTask.Spec.TTLSecondsAfterFinished == 0 {
			return time.Time{}
		}
		return cassTask.Status.CompletionTime.Add(time.Duration(*cassTask.Spec.TTLSecondsAfterFinished) * time.Second)
	}
	return cassTask.Status.CompletionTime.Add(defaultTTL)
}

func (r *CassandraTaskReconciler) activeTasks(ctx context.Context, dc *cassapi.CassandraDatacenter) ([]api.CassandraTask, error) {
	var taskList api.CassandraTaskList
	matcher := client.MatchingLabels(utils.MergeMap(dc.GetDatacenterLabels(), map[string]string{taskStatusLabel: activeTaskLabelValue}))
	if err := r.List(ctx, &taskList, client.InNamespace(dc.Namespace), matcher); err != nil {
		return nil, err
	}

	return taskList.Items, nil
}

/*
*

	Jobs executing something on every pod in the datacenter

*
*/
var jobRunner chan int = make(chan int, 1) // Sync tasks are still limited to no concurrency

type JobStatus struct {
	Id      string `json:"id,omitempty"`
	Status  string `json:"status,omitempty"`
	Handler string `json:"handler,omitempty"`
	Retries int    `json:"retries,omitempty"`
}

func (r *CassandraTaskReconciler) getDatacenterPods(ctx context.Context, dc *cassapi.CassandraDatacenter) ([]corev1.Pod, error) {
	var pods corev1.PodList

	if err := r.List(ctx, &pods, client.InNamespace(dc.Namespace), client.MatchingLabels(dc.GetDatacenterLabels())); err != nil {
		return nil, err
	}

	return pods.Items, nil
}

func (r *CassandraTaskReconciler) getDatacenterStatefulSets(ctx context.Context, dc *cassapi.CassandraDatacenter) ([]appsv1.StatefulSet, error) {
	var sts appsv1.StatefulSetList

	if err := r.List(ctx, &sts, client.InNamespace(dc.Namespace), client.MatchingLabels(dc.GetDatacenterLabels())); err != nil {
		return nil, err
	}

	return sts.Items, nil
}

func (r *CassandraTaskReconciler) getStatefulSetPods(ctx context.Context, dc *cassapi.CassandraDatacenter, st *appsv1.StatefulSet) ([]corev1.Pod, error) {
	var pods corev1.PodList
	if err := r.List(ctx, &pods, client.InNamespace(dc.Namespace), client.MatchingLabels(st.Spec.Selector.MatchLabels)); err != nil {
		return nil, err
	}
	return pods.Items, nil
}

// reconcileEveryPodTask executes the given task against all the Datacenter pods
func (r *CassandraTaskReconciler) reconcileEveryPodTask(ctx context.Context, cassTask *api.CassandraTask, dc *cassapi.CassandraDatacenter, taskConfig *TaskConfiguration) (ctrl.Result, int, int, error) {
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

	podsByRack := make(map[string][]corev1.Pod)
	for _, pod := range dcPods {
		rack := pod.Labels[cassapi.RackLabel]
		podsByRack[rack] = append(podsByRack[rack], pod)
	}

	// Get concurrency limit (default is 1)
	maxConcurrent := 1
	if taskConfig.MaxConcurrentPods != nil {
		maxConcurrent = *taskConfig.MaxConcurrentPods
	}

	nodeMgmtClient, err := httphelper.NewMgmtClient(ctx, r.Client, dc, nil)
	if err != nil {
		return ctrl.Result{}, 0, 0, err
	}

	failed, completed := 0, 0

	// Process each rack sequentially, but pods within rack in parallel
	for _, rack := range getSortedRacks(podsByRack) {
		rackPods := make([]corev1.Pod, 0, len(podsByRack[rack]))

		for _, pod := range podsByRack[rack] {
			if taskConfig.Filter(&pod) {
				rackPods = append(rackPods, pod)
			}
		}

		rackResult, rackFailed, rackCompleted, rackRunning, err := r.processRack(ctx, cassTask, rackPods, taskConfig, nodeMgmtClient, maxConcurrent)
		if err != nil {
			logger.Error(err, "Error processing rack", "rack", rack)
			return ctrl.Result{}, failed, completed, err
		}

		failed += rackFailed
		completed += rackCompleted

		if rackRunning > 0 || rackResult.RequeueAfter > 0 {
			// Some pods in this rack are still processing
			logger.V(1).Info("Rack still processing, requeueing", "rack", rack, "requeueAfter", rackResult.RequeueAfter)
			return rackResult, failed, completed, nil
		}

		logger.V(1).Info("Rack completed", "rack", rack, "failed", rackFailed, "completed", rackCompleted)
	}

	return ctrl.Result{}, failed, completed, nil
}

func (r *CassandraTaskReconciler) processJobs(ctx context.Context, dc *cassapi.CassandraDatacenter, cassTask *api.CassandraTask, taskName string) (ctrl.Result, int, int, error) {
	var res ctrl.Result
	var err error
	var failed, completed int
	logger := log.FromContext(ctx)

JobDefinition:
	for _, job := range cassTask.Spec.Jobs {
		taskConfig := &TaskConfiguration{
			RestartPolicy:     cassTask.Spec.RestartPolicy,
			Id:                taskName,
			Datacenter:        dc,
			TaskStartTime:     cassTask.Status.StartTime,
			Context:           ctx,
			Arguments:         job.Arguments,
			MaxConcurrentPods: cassTask.Spec.MaxConcurrentPods,
			Retries:           cassTask.Spec.Retries,
			PodFilter:         genericPodFilter,
			Job:               job.Command,
		}

		// Process all the reconcileEveryPodTasks
		switch job.Command {
		case api.CommandRebuild:
			rebuild(taskConfig)
		case api.CommandCleanup:
			cleanup(taskConfig)
		case api.CommandRestart:
			// This job is targeting StatefulSets and not Pods
			sts, err := r.getDatacenterStatefulSets(ctx, dc)
			if err != nil {
				return ctrl.Result{}, failed, completed, err
			}

			var restartRes ctrl.Result
			restartRes, err = r.restartSts(taskConfig, sts)
			if err != nil {
				return ctrl.Result{}, failed, completed, err
			}
			res = restartRes
			completed = taskConfig.Completed
			break JobDefinition
		case api.CommandReplaceNode:
			r.replace(taskConfig)
		case api.CommandUpgradeSSTables:
			upgradesstables(taskConfig)
		case api.CommandScrub:
			scrub(taskConfig)
		case api.CommandCompaction:
			compact(taskConfig)
		case api.CommandMove:
			r.move(taskConfig)
		case api.CommandFlush:
			flush(taskConfig)
		case api.CommandGarbageCollect:
			gc(taskConfig)
		case api.CommandRefresh:
			// This targets the Datacenter only
			var refreshRes ctrl.Result
			refreshRes, err = r.refreshDatacenter(ctx, dc, cassTask)
			if err != nil {
				return ctrl.Result{}, failed, completed, err
			}
			res = refreshRes
			completed = taskConfig.Completed
			break JobDefinition
		case api.CommandTSReload:
			inodeTsReload(taskConfig)
		default:
			err = fmt.Errorf("unknown job command: %s", job.Command)
			return ctrl.Result{}, failed, completed, err
		}

		if !r.HasCondition(cassTask, api.JobRunning, metav1.ConditionTrue) {
			valid, errValidate := taskConfig.Validate()
			if errValidate != nil && valid {
				// Retry, this is a transient error
				return ctrl.Result{}, failed, completed, errValidate
			}

			if !valid {
				failed++
				err = reconcile.TerminalError(errValidate)
				res = ctrl.Result{}
				break
			}

			if err := taskConfig.PreProcess(); err != nil {
				return ctrl.Result{}, failed, completed, err
			}
		}

		if modified := SetCondition(cassTask, api.JobRunning, metav1.ConditionTrue, ""); modified {
			if err = r.Status().Update(ctx, cassTask); err != nil {
				return ctrl.Result{}, failed, completed, err
			}
		}

		var podTaskRes ctrl.Result
		podTaskRes, failed, completed, err = r.reconcileEveryPodTask(ctx, cassTask, dc, taskConfig)
		if err != nil {
			return podTaskRes, failed, completed, err
		}
		res = podTaskRes

		if res.RequeueAfter > 0 {
			// This job isn't complete yet or there's an error, do not continue
			logger.V(1).Info("This job isn't complete yet or there's an error, requeueing", "requeueAfter", res.RequeueAfter)
			break
		}
	}

	return res, failed, completed, err
}

func getSortedRacks(podsByRack map[string][]corev1.Pod) []string {
	racks := make([]string, 0, len(podsByRack))
	for rack := range podsByRack {
		racks = append(racks, rack)
	}
	sort.Strings(racks)
	return racks
}

func (r *CassandraTaskReconciler) processRack(
	ctx context.Context,
	cassTask *api.CassandraTask,
	pods []corev1.Pod,
	taskConfig *TaskConfiguration,
	nodeMgmtClient httphelper.NodeMgmtClient,
	maxConcurrent int,
) (ctrl.Result, int, int, int, error) {
	logger := log.FromContext(ctx)

	cassTaskPatch := client.MergeFrom(cassTask.DeepCopy())

	if cassTask.Status.PodStatuses == nil {
		cassTask.Status.PodStatuses = make(map[string]api.PodProcessingStatus)
	}

	rackRemaining, rackFailed, rackCompleted, runningCount, err := r.checkRackCompletion(ctx, cassTask, pods, taskConfig, nodeMgmtClient)
	if err != nil {
		return ctrl.Result{}, 0, 0, 0, err
	}

	if runningCount >= maxConcurrent {
		return ctrl.Result{RequeueAfter: JobRunningRequeue}, 0, 0, 0, nil
	}

	if rackRemaining > 0 && rackRemaining == runningCount {
		return ctrl.Result{RequeueAfter: JobRunningRequeue}, 0, 0, 0, nil
	}

	// Start new pods (up to available slots)
	availableSlots := maxConcurrent - runningCount
	started := 0

	for _, pod := range pods {
		if started >= availableSlots {
			break
		}

		status, exists := cassTask.Status.PodStatuses[pod.Name]

		if exists && (status.Status == api.PodCompleted || status.Status == api.PodError) {
			continue
		}

		if exists && status.Status == api.PodRunning {
			continue
		}

		logger.V(1).Info("Starting pod task", "pod", pod.Name)
		if err := r.startPodTask(ctx, cassTask, &pod, taskConfig, nodeMgmtClient); err != nil {
			return ctrl.Result{}, 0, 0, 0, err
		}
		started++
	}

	// Update task status to persist all changes
	// TODO Move this to the caller?
	if err := r.Status().Patch(ctx, cassTask, cassTaskPatch); err != nil {
		return ctrl.Result{}, 0, 0, 0, err
	}

	if rackRemaining > 0 {
		return ctrl.Result{RequeueAfter: JobRunningRequeue}, rackFailed, rackCompleted, runningCount + started, nil
	}

	return ctrl.Result{}, rackFailed, rackCompleted, runningCount + started, nil
}

func (r *CassandraTaskReconciler) startPodTask(
	ctx context.Context,
	cassTask *api.CassandraTask,
	pod *corev1.Pod,
	taskConfig *TaskConfiguration,
	nodeMgmtClient httphelper.NodeMgmtClient,
) error {
	logger := log.FromContext(ctx)
	features, err := nodeMgmtClient.FeatureSet(pod)
	if err != nil {
		return err
	}

	status, found := cassTask.Status.PodStatuses[pod.Name]
	if !found {
		status = api.PodProcessingStatus{
			Status:  api.PodRunning,
			Retries: 0,
		}
	}

	if status.Status == api.PodWaiting {
		status.Status = api.PodRunning
	}

	if status.StartTime == nil {
		status.StartTime = new(metav1.Now())
	}

	if taskConfig.Job != api.CommandReplaceNode && features.Supports(taskConfig.AsyncFeature) {
		jobId, err := taskConfig.AsyncFunc(nodeMgmtClient, pod, taskConfig)
		if err != nil {
			return err
		}

		status.JobID = jobId
		logger.V(1).Info("Started async task for pod", "pod", pod.Name, "jobId", jobId)
	} else {
		if taskConfig.SyncFunc == nil {
			// This feature is not supported in sync mode, mark everything as done
			err := fmt.Errorf("this job isn't supported by the target pod")
			logger.Error(err, "unable to execute requested job against pod", "Pod", pod)
			status.Error = err.Error()
			status.Status = api.PodError
		}

		if taskConfig.Job != api.CommandReplaceNode && taskConfig.SyncFeature != "" {
			if !features.Supports(taskConfig.SyncFeature) {
				err := fmt.Errorf("this job isn't supported by the target pod")
				logger.Error(err, "Pod doesn't support this feature", "Pod", pod, "Feature", taskConfig.SyncFeature)
				status.Error = err.Error()
				status.Status = api.PodError
			}
		}

		if len(jobRunner) >= cap(jobRunner) {
			status.Status = api.PodWaiting
			cassTask.Status.PodStatuses[pod.Name] = status
			return nil
		}

		if status.Status == api.PodRunning {
			go func() {
				// Write value to the jobRunner to indicate we're running
				taskKey := types.NamespacedName{Name: cassTask.Name, Namespace: cassTask.Namespace}
				logger.V(1).Info("starting execution of sync blocking job", "Pod", pod)
				jobRunner <- 1
				defer func() {
					// Remove the value from the jobRunner
					<-jobRunner
				}()

				if taskConfig.PreProcessFunc != nil {
					if err := taskConfig.PreProcessFunc(taskConfig); err != nil {
						logger.Error(err, "executing preprocessing functionality failed", "Pod", pod)
						status.Error = err.Error()
						status.Status = api.PodError
					}
				}

				if status.Error == "" {
					if err = taskConfig.SyncFunc(nodeMgmtClient, pod, taskConfig); err != nil {
						// We only log, nothing else to do - we won't even retry this pod.
						logger.Error(err, "executing the sync task failed", "Pod", pod)
						status.Error = err.Error()
						status.Status = api.PodError
					} else {
						status.Status = api.PodCompleted
						status.CompletionTime = new(metav1.Now())
					}
				}

				cassTask := &api.CassandraTask{}
				if err := r.Get(context.Background(), taskKey, cassTask); err != nil {
					logger.Error(err, "Failed to get task for status update", "CassandraTask", cassTask)
				}

				taskPatch := client.MergeFrom(cassTask.DeepCopy())
				if cassTask.Status.PodStatuses == nil {
					cassTask.Status.PodStatuses = make(map[string]api.PodProcessingStatus)
				}
				cassTask.Status.PodStatuses[pod.Name] = status

				if err = r.Status().Patch(ctx, cassTask, taskPatch); err != nil {
					logger.Error(err, "Failed to update cassandraTask's status", "CassandraTask", cassTask)
				}
			}()
		}
	}

	cassTask.Status.PodStatuses[pod.Name] = status
	return nil
}

func (r *CassandraTaskReconciler) checkRackCompletion(
	ctx context.Context,
	cassTask *api.CassandraTask,
	pods []corev1.Pod,
	taskConfig *TaskConfiguration,
	nodeMgmtClient httphelper.NodeMgmtClient,
) (int, int, int, int, error) {
	logger := log.FromContext(ctx)

	failed := 0
	completed := 0
	running := 0
	remaining := len(pods)
	for _, pod := range pods {
		status, exists := cassTask.Status.PodStatuses[pod.Name]
		if !exists {
			continue
		}

		switch status.Status {
		case api.PodWaiting:
			continue
		case api.PodCompleted:
			completed++
		case api.PodError:
			failed++
		case api.PodRunning:
			if status.JobID != "" {
				details, err := nodeMgmtClient.JobDetails(&pod, status.JobID)
				if err != nil {
					logger.Error(err, "Could not get JobDetails", "pod", pod.Name)
					return remaining, failed, completed, running, err
				}

				if details.Id == "" {
					// This should get requeued and restarted
					delete(cassTask.Status.PodStatuses, pod.Name)
				}

				switch details.Status {
				case "COMPLETED":
					status.Status = api.PodCompleted
					status.CompletionTime = new(metav1.Now())
					cassTask.Status.PodStatuses[pod.Name] = status
					completed++
				case "ERROR":
					failed = failed + podFailedHandling(taskConfig, &status, details.Error)
					cassTask.Status.PodStatuses[pod.Name] = status
				default:
					running++
				}
			} else {
				if len(jobRunner) > 0 {
					running++
				} else {
					logger.V(1).Info("Pod or controller has crashed during sync task, need to retry or fail", "Pod", pod.Name)
					failed = failed + podFailedHandling(taskConfig, &status, "Pod or controller crashed during sync task")
					cassTask.Status.PodStatuses[pod.Name] = status
				}
			}
		default:
			panic(fmt.Sprintf("This shouldn't happen, current status is %s", status.Status))
		}
	}

	remaining = remaining - (completed + failed)

	if remaining == 0 && running != 0 {
		err := fmt.Errorf("inconsistent state: more pod states than pods, running=%d, completed=%d, failed=%d, totalPods=%d", running, completed, failed, len(pods))
		logger.Error(err, "Pods marked running but all completed/failed")
		return remaining, failed, completed, running, err
	}

	return remaining, failed, completed, running, nil
}

func podFailedHandling(taskConfig *TaskConfiguration, status *api.PodProcessingStatus, errMsg string) int {
	maxRetries := 0

	if taskConfig.RestartPolicy == corev1.RestartPolicyOnFailure {
		maxRetries = 1
		if taskConfig.Retries != nil {
			maxRetries = *taskConfig.Retries
		}
	}

	if status.Retries < maxRetries {
		status.Retries++
		status.Status = api.PodWaiting
		status.JobID = ""
		status.Error = ""
	} else {
		status.Status = api.PodError
		status.Error = errMsg
		status.CompletionTime = new(metav1.Now())
		return 1
	}

	return 0
}
