package control

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/k8ssandra/cass-operator/pkg/httphelper"
	"github.com/k8ssandra/cass-operator/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	cassapi "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	api "github.com/k8ssandra/cass-operator/apis/control/v1alpha1"
)

// Cleanup functionality

func callCleanup(nodeMgmtClient httphelper.NodeMgmtClient, pod *corev1.Pod, taskConfig *TaskConfiguration) (string, error) {
	// TODO Add more arguments configurations
	keyspaceName := taskConfig.Arguments.KeyspaceName
	return nodeMgmtClient.CallKeyspaceCleanup(pod, -1, keyspaceName, nil)
}

func callCleanupSync(nodeMgmtClient httphelper.NodeMgmtClient, pod *corev1.Pod, taskConfig *TaskConfiguration) error {
	// TODO Add more arguments configurations
	keyspaceName := taskConfig.Arguments.KeyspaceName
	return nodeMgmtClient.CallKeyspaceCleanupEndpoint(pod, -1, keyspaceName, nil)
}

func cleanup(taskConfig *TaskConfiguration) {
	taskConfig.AsyncFeature = httphelper.AsyncSSTableTasks
	taskConfig.AsyncFunc = callCleanup
	taskConfig.SyncFunc = callCleanupSync
}

// Rebuild functionality

func callRebuild(nodeMgmtClient httphelper.NodeMgmtClient, pod *corev1.Pod, taskConfig *TaskConfiguration) (string, error) {
	return nodeMgmtClient.CallDatacenterRebuild(pod, taskConfig.Arguments.SourceDatacenter)
}

func rebuild(taskConfig *TaskConfiguration) {
	taskConfig.AsyncFeature = httphelper.Rebuild
	taskConfig.AsyncFunc = callRebuild
}

// Rolling restart functionality

func (r *CassandraTaskReconciler) restartSts(ctx context.Context, sts []appsv1.StatefulSet, taskConfig *TaskConfiguration) (ctrl.Result, error) {
	// Sort to ensure we don't process StatefulSets in wrong order and restart multiple racks at the same time
	sort.Slice(sts, func(i, j int) bool {
		return sts[i].Name < sts[j].Name
	})

	restartTime := taskConfig.TaskStartTime.Format(time.RFC3339)

	if taskConfig.Arguments.RackName != "" {
		singleSts := make([]appsv1.StatefulSet, 1)
		for _, st := range sts {
			if st.ObjectMeta.Labels[cassapi.RackLabel] == taskConfig.Arguments.RackName {
				singleSts[0] = st
				sts = singleSts
				break
			}
		}
	}
	restartedPods := make(map[string]int)
	for _, st := range sts {
		if st.Spec.Template.ObjectMeta.Annotations == nil {
			st.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
		}
		if st.Spec.Template.ObjectMeta.Annotations[api.RestartedAtAnnotation] == restartTime {
			// This one has been called to restart already - is it ready?

			status := st.Status
			if status.CurrentRevision == status.UpdateRevision &&
				status.UpdatedReplicas == status.Replicas &&
				status.CurrentReplicas == status.Replicas &&
				status.ReadyReplicas == status.Replicas &&
				status.ObservedGeneration == st.GetObjectMeta().GetGeneration() {
				// This one has been updated, move on to the next one

				restartedPods[st.Name] = int(status.UpdatedReplicas)
				continue
			}
			restartedPods[st.Name] = int(status.UpdatedReplicas)
			totalRestarted := 0
			for _, v := range restartedPods {
				totalRestarted += v
			}
			taskConfig.Completed = totalRestarted
			// This is still restarting
			return ctrl.Result{RequeueAfter: jobRunningRequeue}, nil
		}
		st.Spec.Template.ObjectMeta.Annotations[api.RestartedAtAnnotation] = restartTime
		if err := r.Client.Update(ctx, &st); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: jobRunningRequeue}, nil
	}

	// We're done
	return ctrl.Result{}, nil
}

// UpgradeSSTables functionality

func callUpgradeSSTables(nodeMgmtClient httphelper.NodeMgmtClient, pod *corev1.Pod, taskConfig *TaskConfiguration) (string, error) {
	// TODO Add more arguments configurations
	keyspaceName := taskConfig.Arguments.KeyspaceName
	return nodeMgmtClient.CallUpgradeSSTables(pod, -1, keyspaceName, nil)
}

func callUpgradeSSTablesSync(nodeMgmtClient httphelper.NodeMgmtClient, pod *corev1.Pod, taskConfig *TaskConfiguration) error {
	// TODO Add more arguments configurations
	keyspaceName := taskConfig.Arguments.KeyspaceName
	return nodeMgmtClient.CallUpgradeSSTablesEndpoint(pod, -1, keyspaceName, nil)
}

func upgradesstables(taskConfig *TaskConfiguration) {
	taskConfig.AsyncFeature = httphelper.AsyncUpgradeSSTableTask
	taskConfig.AsyncFunc = callUpgradeSSTables
	taskConfig.SyncFunc = callUpgradeSSTablesSync
}

// Replace nodes functionality

// replacePod will drain the node, remove the PVCs and delete the pod. cass-operator will then call replace-node process when it starts Cassandra
func (r *CassandraTaskReconciler) replacePod(nodeMgmtClient httphelper.NodeMgmtClient, pod *corev1.Pod, taskConfig *TaskConfiguration) error {
	// We check the podStartTime to prevent replacing the pod multiple times since annotations are removed when we delete the pod
	podStartTime := pod.GetCreationTimestamp()
	if podStartTime.Before(taskConfig.TaskStartTime) {
		if isCassandraUp(pod) {
			// Verify the cassandra pod is healthy before trying the drain
			if err := nodeMgmtClient.CallDrainEndpoint(pod); err != nil {
				return err
			}
		}

		// Get all the PVCs that the pod is using?
		pvcs, err := r.getPodPVCs(taskConfig.Context, taskConfig.Datacenter.Namespace, pod)
		if err != nil {
			return err
		}

		// Delete the PVCs .. without waiting (set status to terminating - finalizer will block)
		for _, pvc := range pvcs {
			if err := r.Client.Delete(taskConfig.Context, pvc); err != nil {
				return err
			}
		}

		// Finally, delete the pod
		if err := r.Client.Delete(taskConfig.Context, pod); err != nil {
			return err
		}
	}
	return nil
}

func (r *CassandraTaskReconciler) replaceValidator(taskConfig *TaskConfiguration) error {
	// Check that arguments has replaceable pods and that those pods are actually existing pods
	if taskConfig.Arguments.PodName != "" {
		pods, err := r.getDatacenterPods(taskConfig.Context, taskConfig.Datacenter)
		if err != nil {
			return err
		}
		for _, pod := range pods {
			if pod.Name == taskConfig.Arguments.PodName {
				return nil
			}
		}
	}

	return fmt.Errorf("valid pod_name to replace is required")
}

func replaceFilter(pod *corev1.Pod, taskConfig *TaskConfiguration) bool {
	// If pod isn't in the to be replaced pods, return false
	podName := taskConfig.Arguments.PodName
	return pod.Name == podName
}

// replacePreProcess adds enough information to CassandraDatacenter to ensure cass-operator knows this pod is being replaced
func (r *CassandraTaskReconciler) replacePreProcess(taskConfig *TaskConfiguration) error {
	dc := taskConfig.Datacenter
	podName := taskConfig.Arguments.PodName
	dc.Status.NodeReplacements = utils.AppendValuesToStringArrayIfNotPresent(
		dc.Status.NodeReplacements, podName)

	r.setDatacenterCondition(dc, cassapi.NewDatacenterCondition(cassapi.DatacenterReplacingNodes, corev1.ConditionTrue))

	return r.Client.Status().Update(taskConfig.Context, dc)
}

func (r *CassandraTaskReconciler) setDatacenterCondition(dc *cassapi.CassandraDatacenter, condition *cassapi.DatacenterCondition) {
	if dc.GetConditionStatus(condition.Type) != condition.Status {
		// We are changing the status, so record the transition time
		condition.LastTransitionTime = metav1.Now()
		dc.SetCondition(*condition)
	}
}

func (r *CassandraTaskReconciler) replace(taskConfig *TaskConfiguration) {
	taskConfig.SyncFunc = r.replacePod
	taskConfig.ValidateFunc = r.replaceValidator
	taskConfig.PodFilter = replaceFilter
	taskConfig.PreProcessFunc = r.replacePreProcess
}

// Common functions

func isCassandraUp(pod *corev1.Pod) bool {
	status := pod.Status
	statuses := status.ContainerStatuses
	ready := false
	for _, status := range statuses {
		if status.Name != "cassandra" {
			continue
		}
		ready = status.Ready
	}
	return ready
}

func (r *CassandraTaskReconciler) getPodPVCs(ctx context.Context, namespace string, pod *corev1.Pod) ([]*corev1.PersistentVolumeClaim, error) {
	pvcs := make([]*corev1.PersistentVolumeClaim, 0, len(pod.Spec.Volumes))
	for _, v := range pod.Spec.Volumes {
		if v.PersistentVolumeClaim == nil {
			continue
		}

		name := types.NamespacedName{
			Name:      v.PersistentVolumeClaim.ClaimName,
			Namespace: namespace,
		}

		podPvc := &corev1.PersistentVolumeClaim{}
		err := r.Client.Get(ctx, name, podPvc)
		if err != nil {
			return nil, err
		}

		pvcs = append(pvcs, podPvc)
	}
	return pvcs, nil
}
