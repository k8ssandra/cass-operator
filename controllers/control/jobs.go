package control

import (
	"context"

	"github.com/k8ssandra/cass-operator/pkg/httphelper"
	corev1 "k8s.io/api/core/v1"
)

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

func callRebuild(nodeMgmtClient httphelper.NodeMgmtClient, pod *corev1.Pod, taskConfig *TaskConfiguration) (string, error) {
	return nodeMgmtClient.CallDatacenterRebuild(pod, taskConfig.Arguments["source_datacenter"])
}

func rebuild(taskConfig *TaskConfiguration) {
	taskConfig.AsyncFeature = httphelper.Rebuild
	taskConfig.AsyncFunc = callRebuild
}

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
			// TODO And a logger..
			// logger.Error(err, "error during drain during rolling restart",
			// 	"pod", pod.Name)
		}

		// Delete the pod
		// TODO And a Kubernetes client it seems..
		err = r.Client.Delete(context.Background(), pod)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *CassandraTaskReconciler) restart(taskConfig *TaskConfiguration) {
	// As a short version, what if we just upgraded the Datacenter.Status fields and let dc reconciler take care of the function still?
	// We only need to get the dc reconciler to refresh itself when we do that (by default it ignores status updates)
	taskConfig.AsyncFeature = httphelper.Feature("") // This should never match
	taskConfig.SyncFunc = r.callRestartSync
}

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
