package control

import (
	"context"
	"fmt"

	"github.com/k8ssandra/cass-operator/pkg/httphelper"
	"github.com/k8ssandra/cass-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	cassapi "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
)

// Cleanup functionality

func callCleanup(nodeMgmtClient httphelper.NodeMgmtClient, pod *corev1.Pod, taskConfig *TaskConfiguration) (string, error) {
	// TODO Add more arguments configurations
	keyspaceName := ""
	if keyspace, found := taskConfig.Arguments["keyspace_name"]; found {
		keyspaceName = keyspace
	}
	return nodeMgmtClient.CallKeyspaceCleanup(pod, -1, keyspaceName, nil)
}

func callCleanupSync(nodeMgmtClient httphelper.NodeMgmtClient, pod *corev1.Pod, taskConfig *TaskConfiguration) error {
	// TODO Add more arguments configurations
	keyspaceName := ""
	if keyspace, found := taskConfig.Arguments["keyspace_name"]; found {
		keyspaceName = keyspace
	}
	return nodeMgmtClient.CallKeyspaceCleanupEndpoint(pod, -1, keyspaceName, nil)
}

func cleanup(taskConfig *TaskConfiguration) {
	taskConfig.AsyncFeature = httphelper.AsyncSSTableTasks
	taskConfig.AsyncFunc = callCleanup
	taskConfig.SyncFunc = callCleanupSync
}

// Rebuild functionality

func callRebuild(nodeMgmtClient httphelper.NodeMgmtClient, pod *corev1.Pod, taskConfig *TaskConfiguration) (string, error) {
	return nodeMgmtClient.CallDatacenterRebuild(pod, taskConfig.Arguments["source_datacenter"])
}

func rebuild(taskConfig *TaskConfiguration) {
	taskConfig.AsyncFeature = httphelper.Rebuild
	taskConfig.AsyncFunc = callRebuild
}

// Rolling restart functionality

func (r *CassandraTaskReconciler) callRestartSync(nodeMgmtClient httphelper.NodeMgmtClient, pod *corev1.Pod, taskConfig *TaskConfiguration) error {
	podStartTime := pod.GetCreationTimestamp()
	// TODO How do we verify the previous pod has actually restarted before we move on to restart the next one?
	//		cass-operator used to take care of this.
	if podStartTime.Before(taskConfig.TaskStartTime) {
		// TODO Our taskController needs events
		// rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.RestartingCassandra,
		// 	"Restarting Cassandra for pod %s", pod.Name)

		// Drain the node
		err := nodeMgmtClient.CallDrainEndpoint(pod)
		if err != nil {
			return err
		}

		// Delete the pod
		err = r.Client.Delete(context.Background(), pod)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *CassandraTaskReconciler) restart(taskConfig *TaskConfiguration) {
	taskConfig.SyncFunc = r.callRestartSync
}

// UpgradeSSTables functionality

func callUpgradeSSTables(nodeMgmtClient httphelper.NodeMgmtClient, pod *corev1.Pod, taskConfig *TaskConfiguration) (string, error) {
	// TODO Add more arguments configurations
	keyspaceName := ""
	if keyspace, found := taskConfig.Arguments["keyspace_name"]; found {
		keyspaceName = keyspace
	}
	return nodeMgmtClient.CallUpgradeSSTables(pod, -1, keyspaceName, nil)
}

func callUpgradeSSTablesSync(nodeMgmtClient httphelper.NodeMgmtClient, pod *corev1.Pod, taskConfig *TaskConfiguration) error {
	// TODO Add more arguments configurations
	keyspaceName := ""
	if keyspace, found := taskConfig.Arguments["keyspace_name"]; found {
		keyspaceName = keyspace
	}
	return nodeMgmtClient.CallUpgradeSSTablesEndpoint(pod, -1, keyspaceName, nil)
}

func upgradesstables(taskConfig *TaskConfiguration) {
	taskConfig.AsyncFeature = httphelper.AsyncUpgradeSSTableTask
	taskConfig.AsyncFunc = callUpgradeSSTables
	taskConfig.SyncFunc = callUpgradeSSTablesSync
}

// Replace nodes functionality

const (
	replacePodNameArgument = "pod_name"
)

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
	if podName, found := taskConfig.Arguments[replacePodNameArgument]; found {
		pods, err := r.getDatacenterPods(taskConfig.Context, taskConfig.Datacenter)
		if err != nil {
			return err
		}
		for _, pod := range pods {
			if pod.Name == podName {
				return nil
			}
		}
	}

	return fmt.Errorf("valid pod_name to replace is required")
}

func replaceFilter(pod *corev1.Pod, taskConfig *TaskConfiguration) bool {
	// If pod isn't in the to be replaced pods, return false
	podName := taskConfig.Arguments[replacePodNameArgument]
	return pod.Name == podName
}

// replacePreProcess adds enough information to CassandraDatacenter to ensure cass-operator knows this pod is being replaced
func (r *CassandraTaskReconciler) replacePreProcess(taskConfig *TaskConfiguration) error {
	dc := taskConfig.Datacenter
	podName := taskConfig.Arguments[replacePodNameArgument]
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
