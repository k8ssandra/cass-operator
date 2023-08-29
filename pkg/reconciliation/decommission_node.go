package reconciliation

import (
	"fmt"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/pkg/events"
	"github.com/k8ssandra/cass-operator/pkg/httphelper"
	"github.com/k8ssandra/cass-operator/pkg/internal/result"
	"github.com/k8ssandra/cass-operator/pkg/monitoring"
	"k8s.io/apimachinery/pkg/types"
)

func (rc *ReconciliationContext) CalculateRackInfoForDecomm(currentSize int) ([]*RackInformation, error) {
	racks := rc.Datacenter.GetRacks()
	rackCount := len(racks)

	// only worry about scaling 1 node at a time
	desiredSize := currentSize - 1

	if desiredSize < rackCount && rc.Datacenter.Status.GetConditionStatus(api.DatacenterDecommission) != corev1.ConditionTrue {
		return nil, fmt.Errorf("the number of nodes cannot be smaller than the number of racks")
	}

	var decommRackInfo []*RackInformation
	rackNodeCounts := api.SplitRacks(desiredSize, rackCount)

	for rackIndex, currentRack := range racks {
		nextRack := &RackInformation{}
		nextRack.RackName = currentRack.Name
		nextRack.NodeCount = rackNodeCounts[rackIndex]

		decommRackInfo = append(decommRackInfo, nextRack)
	}

	return decommRackInfo, nil
}

func (rc *ReconciliationContext) DecommissionNodes(epData httphelper.CassMetadataEndpoints) result.ReconcileResult {
	logger := rc.ReqLogger
	logger.Info("reconcile_racks::DecommissionNodes")
	dc := rc.Datacenter

	var currentSize int32
	for _, sts := range rc.statefulSets {
		if sts != nil {
			currentSize += *sts.Spec.Replicas
		}
	}

	targetSize := dc.Spec.Size

	if rc.Datacenter.Status.GetConditionStatus(api.DatacenterDecommission) == corev1.ConditionTrue {
		targetSize = 0
	}

	if currentSize <= targetSize {
		return result.Continue()
	}

	decommRackInfo, err := rc.CalculateRackInfoForDecomm(int(currentSize))
	if err != nil {
		logger.Error(err, "error calculating rack info for decommissioning nodes")
		return result.Error(err)
	}

	for idx := range decommRackInfo {
		rackInfo := decommRackInfo[idx]
		statefulSet := rc.statefulSets[idx]
		desiredNodeCount := int32(rackInfo.NodeCount)
		maxReplicas := *statefulSet.Spec.Replicas
		lastPodSuffix := stsLastPodSuffix(maxReplicas)

		if maxReplicas > desiredNodeCount {
			logger.V(1).Info("reconcile_racks::DecommissionNodes::scaleDownRack", "Rack", rackInfo.RackName, "maxReplicas", maxReplicas, "desiredNodeCount", desiredNodeCount)

			dcPatch := client.MergeFrom(dc.DeepCopy())
			updated := false

			updated = rc.setCondition(
				api.NewDatacenterCondition(
					api.DatacenterScalingDown, corev1.ConditionTrue)) || updated

			if updated {
				err := rc.Client.Status().Patch(rc.Ctx, dc, dcPatch)
				if err != nil {
					logger.Error(err, "error patching datacenter status for scaling down rack started")
					return result.Error(err)
				}
			}

			rc.ReqLogger.Info(
				"Need to update the rack's node count",
				"Rack", rackInfo.RackName,
				"maxReplicas", maxReplicas,
				"desiredSize", desiredNodeCount,
			)

			rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.ScalingDownRack,
				"Scaling down rack %s", rackInfo.RackName)

			if err := setOperatorProgressStatus(rc, api.ProgressUpdating); err != nil {
				return result.Error(err)
			}

			err := rc.DecommissionNodeOnRack(rackInfo.RackName, epData, lastPodSuffix)
			if err != nil {
				return result.Error(err)
			}

			return result.RequeueSoon(10)
		}
	}

	return result.Continue()
}

func (rc *ReconciliationContext) DecommissionNodeOnRack(rackName string, epData httphelper.CassMetadataEndpoints, lastPodSuffix string) error {
	for _, pod := range rc.dcPods {
		podRack := pod.Labels[api.RackLabel]
		if podRack == rackName && strings.HasSuffix(pod.Name, lastPodSuffix) {
			mgmtApiUp := isMgmtApiRunning(pod)
			if !mgmtApiUp {
				return fmt.Errorf("management API is not up on node that we are trying to decommission")
			}

			if err := rc.EnsurePodsCanAbsorbDecommData(pod, epData); err != nil {
				return err
			}

			if err := rc.callDecommission(pod); err != nil {
				return err
			}

			rc.ReqLogger.V(1).Info("Marking node as decommissioning")
			patch := client.MergeFrom(pod.DeepCopy())
			metav1.SetMetaDataLabel(&pod.ObjectMeta, api.CassNodeState, stateDecommissioning)
			if err := rc.Client.Patch(rc.Ctx, pod, patch); err != nil {
				return err
			}

			monitoring.UpdatePodStatusMetric(pod)

			rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.LabeledPodAsDecommissioning,
				"Labeled node as decommissioning %s", pod.Name)

			return nil
		}
	}

	// this shouldn't happen
	return fmt.Errorf("could not find pod to decommission on rack %s", rackName)
}

func (rc *ReconciliationContext) callDecommission(pod *corev1.Pod) error {
	if !isPodUp(pod) {
		// The pod must be started before it can be decommissioned
		rc.ReqLogger.V(1).Info("Error while trying to decommission, pod isn't running.", "Pod", pod)
		return nil
	}

	features, err := rc.NodeMgmtClient.FeatureSet(pod)
	if err != nil {
		return err
	} else if features.Supports(httphelper.AsyncSSTableTasks) {
		if jobId, err := rc.NodeMgmtClient.CallDecommissionNode(pod, true); err != nil {
			return err
		} else {
			rc.ReqLogger.V(1).Info(fmt.Sprintf("Decommission requested, returned jobId: %s", jobId))
		}
	} else {
		// Fallback to older code
		pod := pod
		go func(pod *corev1.Pod) {
			if err := rc.NodeMgmtClient.CallDecommissionNodeEndpoint(pod); err != nil {
				rc.ReqLogger.V(1).Info(fmt.Sprintf("Error from decommission attempt. This is only an attempt and can fail. Error: %v", err))
			}
		}(pod)
	}

	return nil
}

// Wait for decommissioning nodes to finish before continuing to reconcile
func (rc *ReconciliationContext) CheckDecommissioningNodes(epData httphelper.CassMetadataEndpoints) result.ReconcileResult {
	rc.ReqLogger.Info("reconcile_racks::CheckDecommissioningNodes")
	if rc.Datacenter.GetConditionStatus(api.DatacenterScalingDown) != corev1.ConditionTrue {
		return result.Continue()
	}

	nodeStatuses := rc.Datacenter.Status.NodeStatuses

	for _, pod := range rc.dcPods {
		if pod.Labels[api.CassNodeState] == stateDecommissioning {
			if !IsDoneDecommissioning(pod, epData, nodeStatuses, rc.ReqLogger) {
				if !HasStartedDecommissioning(pod, epData, nodeStatuses) {
					rc.ReqLogger.V(1).Info("Decommission has not started trying again", "Pod", pod.Name)
					err := rc.callDecommission(pod)
					if err != nil {
						return result.Error(err)
					}
				}
			} else {
				rc.ReqLogger.V(1).Info("Node finished decommissioning", "Pod", pod.Name)
				if res := rc.cleanUpAfterDecommissionedPod(pod); res != nil {
					return res
				}
			}
			rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.DecommissioningNode, fmt.Sprintf("Decommissioning node %s", pod.Name))
			return result.RequeueSoon(5)
		}
	}

	dcPatch := client.MergeFrom(rc.Datacenter.DeepCopy())
	updated := false

	updated = rc.setCondition(
		api.NewDatacenterCondition(
			api.DatacenterScalingDown, corev1.ConditionFalse)) || updated

	if updated {
		err := rc.Client.Status().Patch(rc.Ctx, rc.Datacenter, dcPatch)
		if err != nil {
			rc.ReqLogger.Error(err, "error patching datacenter status for scaling down finished")
			return result.Error(err)
		}
		// Requeue after updating to ensure we verify previous steps with the new size
		return result.RequeueSoon(0)
	}

	return result.Continue()
}

func (rc *ReconciliationContext) cleanUpAfterDecommissionedPod(pod *corev1.Pod) result.ReconcileResult {
	rc.ReqLogger.Info("Scaling down statefulset")
	err := rc.RemoveDecommissionedPodFromSts(pod)
	if err != nil {
		return result.Error(err)
	}
	rc.ReqLogger.Info("Deleting pod PVCs")
	err = rc.DeletePodPvcs(pod)
	if err != nil {
		return result.Error(err)
	}

	dcPatch := client.MergeFrom(rc.Datacenter.DeepCopy())
	delete(rc.Datacenter.Status.NodeStatuses, pod.Name)

	err = rc.Client.Status().Patch(rc.Ctx, rc.Datacenter, dcPatch)
	if err != nil {
		rc.ReqLogger.Error(err, "error patching datacenter status to remove decommissioned pod from node status")
		return result.Error(err)
	}

	return nil
}

func HasStartedDecommissioning(pod *corev1.Pod, epData httphelper.CassMetadataEndpoints, nodeStatuses api.CassandraStatusMap) bool {
	for idx := range epData.Entity {
		ep := &epData.Entity[idx]
		if ep.GetRpcAddress() == pod.Status.PodIP || ep.EndpointIP == pod.Status.PodIP {
			return ep.HasStatus(httphelper.StatusLeaving)
		}
	}

	// Lets try with HostID next
	if nodeStatus, found := nodeStatuses[pod.Name]; found {
		for idx := range epData.Entity {
			ep := &epData.Entity[idx]
			if ep.HostID == nodeStatus.HostID {
				return ep.HasStatus(httphelper.StatusLeaving)
			}
		}
	}

	return false
}

func IsDoneDecommissioning(pod *corev1.Pod, epData httphelper.CassMetadataEndpoints, nodeStatuses api.CassandraStatusMap, logger logr.Logger) bool {
	for idx := range epData.Entity {
		ep := &epData.Entity[idx]
		if ep.GetRpcAddress() == pod.Status.PodIP || ep.EndpointIP == pod.Status.PodIP {
			return ep.HasStatus(httphelper.StatusLeft)
		}
	}

	// Lets try with HostID next
	if nodeStatus, found := nodeStatuses[pod.Name]; found {
		for idx := range epData.Entity {
			ep := &epData.Entity[idx]
			if ep.HostID == nodeStatus.HostID {
				return ep.HasStatus(httphelper.StatusLeft)
			}
		}
	}

	// Gone from the ring completely?
	return true
}

func isPodUp(pod *corev1.Pod) bool {
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

func (rc *ReconciliationContext) DeletePodPvcs(pod *corev1.Pod) error {
	for _, v := range pod.Spec.Volumes {
		if v.PersistentVolumeClaim == nil {
			continue
		}

		pvcName := v.PersistentVolumeClaim.ClaimName
		name := types.NamespacedName{
			Name:      v.PersistentVolumeClaim.ClaimName,
			Namespace: rc.Datacenter.Namespace,
		}

		podPvc := &corev1.PersistentVolumeClaim{}
		err := rc.Client.Get(rc.Ctx, name, podPvc)
		if err != nil {
			rc.ReqLogger.Error(err, "Failed to get pod PVC", "Claim Name", pvcName)
			return err
		}

		err = rc.Client.Delete(rc.Ctx, podPvc)
		if err != nil {
			rc.ReqLogger.Error(err, "Failed to delete pod PVC", "Claim Name", pvcName)
			return err
		}

		rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.DeletedPvc,
			"Claim Name: %s", pvcName)

	}

	return nil
}

func (rc *ReconciliationContext) RemoveDecommissionedPodFromSts(pod *corev1.Pod) error {
	podRack := pod.Labels[api.RackLabel]
	var sts *appsv1.StatefulSet
	for _, s := range rc.statefulSets {
		if s.Labels[api.RackLabel] == podRack {
			sts = s
			break
		}
	}

	if sts == nil {
		// Failed to find the statefulset for this pod
		return fmt.Errorf("failed to find matching statefulSet for pod rack: %s", podRack)
	}

	maxReplicas := *sts.Spec.Replicas
	lastPodSuffix := stsLastPodSuffix(maxReplicas)
	if strings.HasSuffix(pod.Name, lastPodSuffix) {
		monitoring.RemovePodStatusMetric(pod)
		rc.ReqLogger.Info(fmt.Sprintf("UpdateRackNodeCount in STS %s to %d", sts.Name, *sts.Spec.Replicas-1))
		return rc.UpdateRackNodeCount(sts, *sts.Spec.Replicas-1)
	} else {
		rc.ReqLogger.Error(fmt.Errorf("pod does not match the last pod in the STS"), "Could not find last matching pod", "PodName", pod.Name, "lastPodSuffix", lastPodSuffix)
		// Pod does not match the last pod in statefulSet
		// This scenario should only happen if the pod
		// has already been terminated
		return nil
	}
}

func stsLastPodSuffix(maxReplicas int32) string {
	return fmt.Sprintf("sts-%v", maxReplicas-1)
}

func (rc *ReconciliationContext) EnsurePodsCanAbsorbDecommData(decommPod *corev1.Pod, epData httphelper.CassMetadataEndpoints) error {
	podsUsedStorage, err := rc.GetUsedStorageForPods(epData)
	if err != nil {
		return err
	}

	spaceUsedByDecommPod := podsUsedStorage[decommPod.Name]
	for _, pod := range rc.dcPods {
		if pod.Name == decommPod.Name {
			continue
		}

		serverDataPv, err := rc.getServerDataPv(pod)
		if err != nil {
			return err
		}

		pvCapacity := serverDataPv.Spec.Capacity
		if pvCapacity == nil {
			return fmt.Errorf("could not determine storage capacity when checking if scale-down attempt is valid")
		}

		storage, ok := pvCapacity["storage"]
		if !ok {
			return fmt.Errorf("could not determine storage capacity when checking if scale-down attempt is valid")
		}

		total := storage.AsDec().UnscaledBig().Int64()
		used := podsUsedStorage[pod.Name]
		free := total - int64(used)

		if free < int64(spaceUsedByDecommPod) {
			msg := fmt.Sprintf("Not enough free space available to decommission. %s has %d free space, but %d is needed.",
				pod.Name, free, int64(spaceUsedByDecommPod),
			)
			rc.ReqLogger.Error(fmt.Errorf(msg), msg)

			dcPatch := client.MergeFrom(rc.Datacenter.DeepCopy())
			updated := rc.setCondition(
				api.NewDatacenterConditionWithReason(api.DatacenterValid,
					corev1.ConditionFalse, "notEnoughSpaceToScaleDown", msg,
				),
			)

			if updated {
				patchErr := rc.Client.Status().Patch(rc.Ctx, rc.Datacenter, dcPatch)
				if patchErr != nil {
					msg := "error patching condition Valid for failed scale down."
					rc.ReqLogger.Error(patchErr, msg)
					return patchErr
				}
			}

			return fmt.Errorf(msg)
		}
	}

	return nil
}

func (rc *ReconciliationContext) GetUsedStorageForPods(epData httphelper.CassMetadataEndpoints) (map[string]float64, error) {
	podStorageMap := make(map[string]float64)
	mappedData := MapPodsToEndpointDataByName(rc.dcPods, epData)
	for podName, data := range mappedData {
		load, err := strconv.ParseFloat(data.Load, 64)
		if err != nil {
			rc.ReqLogger.Error(
				fmt.Errorf("failed to parse pod load reported from mgmt api"),
				"pod", podName,
				"Bytes reported by mgmt api", data.Load)
			return nil, err

		}
		podStorageMap[podName] = load
	}

	return podStorageMap, nil
}

func (rc *ReconciliationContext) getServerDataPv(pod *corev1.Pod) (*corev1.PersistentVolume, error) {
	pvcName := types.NamespacedName{
		Name:      fmt.Sprintf("server-data-%s", pod.Name),
		Namespace: rc.Datacenter.Namespace,
	}
	pvc := &corev1.PersistentVolumeClaim{}
	if err := rc.Client.Get(rc.Ctx, pvcName, pvc); err != nil {
		rc.ReqLogger.Error(err, "Failed to get server-data pvc", "pod", pod.Name)
		return nil, err
	}

	pvName := types.NamespacedName{
		Name:      pvc.Spec.VolumeName,
		Namespace: "",
	}
	pv := &corev1.PersistentVolume{}
	if err := rc.Client.Get(rc.Ctx, pvName, pv); err != nil {
		rc.ReqLogger.Error(err, "Failed to get server-data pv", "pod", pod.Name)
		return nil, err
	}

	return pv, nil
}

func (rc *ReconciliationContext) getClusterDatacenters(pods []*corev1.Pod) ([]string, error) {
	// We need the result from at least one pod
	clusterDcs := make(map[string]bool)
	for _, pod := range pods {
		if val, found := pod.GetAnnotations()[api.CassNodeState]; found && val == stateDecommissioning {
			// Do not poll a node that is decommissioning
			continue
		}
		if !isPodUp(pod) {
			// cassandra container has shutdown for this pod
			continue
		}

		metadata, err := rc.NodeMgmtClient.CallMetadataEndpointsEndpoint(pod)
		if err != nil {
			return nil, err
		}

		for _, ep := range metadata.Entity {
			if ep.IsAlive == "true" && ep.HasStatus(httphelper.StatusNormal) {
				clusterDcs[ep.Datacenter] = true
			}
		}
		break
	}

	clusterDcList := make([]string, 0, len(clusterDcs))
	for k := range clusterDcs {
		clusterDcList = append(clusterDcList, k)
	}

	return clusterDcList, nil
}
