// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"fmt"
	"net"
	"reflect"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	taskapi "github.com/k8ssandra/cass-operator/apis/control/v1alpha1"
	"github.com/k8ssandra/cass-operator/internal/result"
	"github.com/k8ssandra/cass-operator/pkg/events"
	"github.com/k8ssandra/cass-operator/pkg/httphelper"
	"github.com/k8ssandra/cass-operator/pkg/monitoring"
	"github.com/k8ssandra/cass-operator/pkg/oplabels"
	"github.com/k8ssandra/cass-operator/pkg/utils"
	pkgerrors "github.com/pkg/errors"
)

var (
	ResultShouldNotRequeue  reconcile.Result = reconcile.Result{Requeue: false}
	ResultShouldRequeueNow  reconcile.Result = reconcile.Result{Requeue: true}
	ResultShouldRequeueSoon reconcile.Result = reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}

	QuietDurationFunc func(int) time.Duration = func(secs int) time.Duration { return time.Duration(secs) * time.Second }
)

const (
	stateReadyToStart    = "Ready-to-Start"
	stateStartedNotReady = "Started-not-Ready"
	stateStarted         = "Started"
	stateStarting        = "Starting"
	stateDecommissioning = "Decommissioning"
)

// CalculateRackInformation determine how many nodes per rack are needed
func (rc *ReconciliationContext) CalculateRackInformation() error {
	rc.ReqLogger.Info("reconcile_racks::calculateRackInformation")

	// Create RackInformation

	nodeCount := int(rc.Datacenter.Spec.Size)
	racks := rc.Datacenter.GetRacks()
	rackCount := len(racks)
	if nodeCount < rackCount && rc.Datacenter.GetDeletionTimestamp() == nil {
		return fmt.Errorf("the number of nodes cannot be smaller than the number of racks")
	}

	if rc.Datacenter.Spec.Stopped {
		nodeCount = 0
	}

	// 3 seeds per datacenter (this could be two, but we would like three seeds per cluster
	// and it's not easy for us to know if we're in a multi DC cluster in this part of the code)
	// OR all of the nodes, if there's less than 3
	// OR one per rack if there are four or more racks
	seedCount := 3
	if nodeCount < 3 {
		seedCount = nodeCount
	} else if rackCount > 3 {
		seedCount = rackCount
	}

	var desiredRackInformation []*RackInformation

	if rackCount < 1 {
		return fmt.Errorf("assertion failed! rackCount should not possibly be zero here")
	}

	rackSeedCounts := api.SplitRacks(seedCount, rackCount)
	rackNodeCounts := api.SplitRacks(nodeCount, rackCount)

	for rackIndex, currentRack := range racks {
		nextRack := &RackInformation{}
		nextRack.RackName = currentRack.Name
		nextRack.NodeCount = rackNodeCounts[rackIndex]
		nextRack.SeedCount = rackSeedCounts[rackIndex]

		desiredRackInformation = append(desiredRackInformation, nextRack)
	}

	statefulSets := make([]*appsv1.StatefulSet, len(desiredRackInformation))

	rc.desiredRackInformation = desiredRackInformation
	rc.statefulSets = statefulSets

	return nil
}

func (rc *ReconciliationContext) CheckSuperuserSecretCreation() result.ReconcileResult {
	rc.ReqLogger.Info("reconcile_racks::CheckSuperuserSecretCreation")

	_, err := rc.retrieveSuperuserSecretOrCreateDefault()
	if err != nil {
		rc.ReqLogger.Error(err, "error retrieving SuperuserSecret for CassandraDatacenter.")
		return result.Error(err)
	}

	return result.Continue()
}

func (rc *ReconciliationContext) CheckInternodeCredentialCreation() result.ReconcileResult {
	rc.ReqLogger.Info("reconcile_racks::CheckInternodeCredentialCreation")

	if !rc.Datacenter.LegacyInternodeEnabled() {
		return result.Continue()
	}

	_, err := rc.retrieveInternodeCredentialSecretOrCreateDefault()
	if err != nil {
		rc.ReqLogger.Error(err, "error retrieving InternodeCredential for CassandraDatacenter.")
		return result.Error(err)
	}

	return result.Continue()
}

func (rc *ReconciliationContext) CheckRackCreation() result.ReconcileResult {
	rc.ReqLogger.Info("reconcile_racks::CheckRackCreation")
	for idx := range rc.desiredRackInformation {
		rackInfo := rc.desiredRackInformation[idx]

		// Does this rack have a statefulset?

		statefulSet, statefulSetFound, err := rc.GetStatefulSetForRack(rackInfo)
		if err != nil {
			rc.ReqLogger.Error(
				err,
				"Could not locate statefulSet for",
				"Rack", rackInfo.RackName)
			return result.Error(err)
		}

		if !statefulSetFound {
			rc.ReqLogger.Info(
				"Need to create new StatefulSet for",
				"Rack", rackInfo.RackName)
			err := rc.ReconcileNextRack(statefulSet)
			if err != nil {
				rc.ReqLogger.Error(
					err,
					"error creating new StatefulSet",
					"Rack", rackInfo.RackName)
				return result.Error(err)
			}
		}
		rc.statefulSets[idx] = statefulSet
	}

	return result.Continue()
}

func (rc *ReconciliationContext) failureModeDetection() bool {
	for _, pod := range rc.dcPods {
		if pod == nil {
			continue
		}
		if pod.Status.Phase == corev1.PodPending {
			if pod.Status.StartTime == nil || hasBeenXMinutes(5, pod.Status.StartTime.Time) {
				// Pod has been over 5 minutes in Pending state. This can be normal, but lets see
				// if we have some detected failures events like FailedScheduling
				events := &corev1.EventList{}
				if err := rc.Client.List(rc.Ctx, events, &client.ListOptions{Namespace: pod.Namespace, FieldSelector: fields.SelectorFromSet(fields.Set{"involvedObject.name": pod.Name})}); err != nil {
					rc.ReqLogger.Error(err, "error getting events for pod", "pod", pod.Name)
					return false
				}

				for _, event := range events.Items {
					if event.Reason == "FailedScheduling" {
						rc.ReqLogger.Info("Found FailedScheduling event for pod", "pod", pod.Name)
						// We have a failed scheduling event
						return true
					}
				}
			}
		}

		// Pod could also be running / terminated, we need to find if any container is in crashing state
		// Sadly, this state is ephemeral, so it can change between reconciliations
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.State.Waiting != nil {
				waitingReason := containerStatus.State.Waiting.Reason
				if waitingReason == "CrashLoopBackOff" ||
					waitingReason == "ImagePullBackOff" ||
					waitingReason == "ErrImagePull" {
					rc.ReqLogger.Info("Failing container state for pod", "pod", pod.Name, "reason", waitingReason)
					// We have a container in a failing state
					return true
				}
			}
			if containerStatus.RestartCount > 2 {
				if containerStatus.State.Terminated != nil {
					if containerStatus.State.Terminated.ExitCode != 0 {
						rc.ReqLogger.Info("Failing container state for pod", "pod", pod.Name, "exitCode", containerStatus.State.Terminated.ExitCode)
						return true
					}
				}
			}
		}
		// Check the same for initContainers
		for _, containerStatus := range pod.Status.InitContainerStatuses {
			if containerStatus.State.Waiting != nil {
				waitingReason := containerStatus.State.Waiting.Reason
				if waitingReason == "CrashLoopBackOff" ||
					waitingReason == "ImagePullBackOff" ||
					waitingReason == "ErrImagePull" {
					// We have a container in a failing state
					rc.ReqLogger.Info("Failing initcontainer state for pod", "pod", pod.Name, "reason", waitingReason)
					return true
				}
			}
			if containerStatus.RestartCount > 2 {
				if containerStatus.State.Terminated != nil {
					if containerStatus.State.Terminated.ExitCode != 0 {
						rc.ReqLogger.Info("Failing initcontainer state for pod", "pod", pod.Name, "exitCode", containerStatus.State.Terminated.ExitCode)
						return true
					}
				}
			}
		}
	}

	return false
}

func (rc *ReconciliationContext) UpdateAllowed() bool {
	// HasAnnotation might require also checking if it's "once / always".. or then we need to validate those allowed values in the webhook
	return rc.Datacenter.GenerationChanged() || metav1.HasAnnotation(rc.Datacenter.ObjectMeta, api.UpdateAllowedAnnotation)
}

func (rc *ReconciliationContext) CheckPVCResizing() result.ReconcileResult {
	rc.ReqLogger.Info("reconcile_racks::CheckPVCResizing")
	pvcList, err := rc.listPVCs(rc.Datacenter.GetDatacenterLabels())
	if err != nil {
		return result.Error(err)
	}

	for _, pvc := range pvcList {
		if isPVCResizing(&pvc) {
			rc.ReqLogger.Info("Waiting for PVC resize to complete",
				"pvc", pvc.Name)
			return result.RequeueSoon(10)
		}
	}

	if err := rc.setConditionStatus(api.DatacenterResizingVolumes, corev1.ConditionFalse); err != nil {
		return result.Error(err)
	}

	return result.Continue()
}

func isPVCResizing(pvc *corev1.PersistentVolumeClaim) bool {
	return isPVCStatusConditionTrue(pvc, corev1.PersistentVolumeClaimResizing) ||
		isPVCStatusConditionTrue(pvc, corev1.PersistentVolumeClaimFileSystemResizePending)
}

func isPVCStatusConditionTrue(pvc *corev1.PersistentVolumeClaim, conditionType corev1.PersistentVolumeClaimConditionType) bool {
	for _, condition := range pvc.Status.Conditions {
		if condition.Type == conditionType && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func (rc *ReconciliationContext) CheckVolumeClaimSizes(statefulSet, desiredSts *appsv1.StatefulSet) result.ReconcileResult {
	rc.ReqLogger.Info("reconcile_racks::CheckVolumeClaims")

	for i, claim := range statefulSet.Spec.VolumeClaimTemplates {
		// Find the desired one
		desiredClaim := desiredSts.Spec.VolumeClaimTemplates[i]
		if claim.Name != desiredClaim.Name {
			return result.Error(fmt.Errorf("statefulSet and desiredSts have different volume claim templates"))
		}

		currentSize := claim.Spec.Resources.Requests[corev1.ResourceStorage]
		createdSize := desiredClaim.Spec.Resources.Requests[corev1.ResourceStorage]

		if currentSize.Cmp(createdSize) > 0 {
			msg := fmt.Sprintf("shrinking PVC %s is not supported", claim.Name)
			if err := rc.setCondition(
				api.NewDatacenterConditionWithReason(api.DatacenterValid,
					corev1.ConditionFalse, "shrinkingDataVolumeNotSupported", msg,
				)); err != nil {
				return result.Error(err)
			}

			rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeWarning, events.InvalidDatacenterSpec, "Shrinking CassandraDatacenter PVCs is not supported")
			return result.Error(pkgerrors.New(msg))
		}

		if currentSize.Cmp(createdSize) < 0 {
			rc.ReqLogger.Info("PVC resize request detected", "pvc", claim.Name, "currentSize", currentSize.String(), "createdSize", createdSize.String())
			if !metav1.HasAnnotation(rc.Datacenter.ObjectMeta, api.AllowStorageChangesAnnotation) || rc.Datacenter.Annotations[api.AllowStorageChangesAnnotation] != "true" {
				msg := fmt.Sprintf("PVC resize requested, but %s annotation is not set to 'true'", api.AllowStorageChangesAnnotation)
				rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeWarning, events.InvalidDatacenterSpec, msg)
				return result.Error(pkgerrors.New(msg))
			}

			supportsExpansion, err := rc.storageExpansion()
			if err != nil {
				return result.Error(err)
			}

			if !supportsExpansion {
				msg := fmt.Sprintf("PVC resize requested, but StorageClass %s does not support expansion", *claim.Spec.StorageClassName)
				rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeWarning, events.InvalidDatacenterSpec, msg)
				if err := rc.setCondition(
					api.NewDatacenterConditionWithReason(api.DatacenterValid,
						corev1.ConditionFalse, "storageClassDoesNotSupportExpansion", msg,
					)); err != nil {
					return result.Error(err)
				}
				return result.Error(pkgerrors.New(msg))
			}

			if err := rc.setConditionStatus(api.DatacenterResizingVolumes, corev1.ConditionTrue); err != nil {
				return result.Error(err)
			}

			rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.ResizingPVC, "Resizing PVCs for %s", statefulSet.Name)

			claims, err := rc.listPVCs(claim.Labels)
			if err != nil {
				return result.Error(err)
			}

			pvcNamePrefix := fmt.Sprintf("%s-%s-", claim.Name, statefulSet.Name)
			targetPVCs := make([]*corev1.PersistentVolumeClaim, 0)
			for _, pvc := range claims {
				if strings.HasPrefix(pvc.Name, pvcNamePrefix) {
					targetPVCs = append(targetPVCs, &pvc)
				}
			}

			for _, pvc := range targetPVCs {
				if isPVCResizing(pvc) {
					return result.RequeueSoon(10)
				}

				patch := client.MergeFrom(pvc.DeepCopy())
				pvc.Spec.Resources.Requests[corev1.ResourceStorage] = createdSize
				if err := rc.Client.Patch(rc.Ctx, pvc, patch); err != nil {
					return result.Error(err)
				}
			}

			// Update the StatefulSet to reflect the new PVC size
			claim.Spec.Resources.Requests[corev1.ResourceStorage] = createdSize
			statefulSet.Spec.VolumeClaimTemplates[i] = claim

			return result.Continue()
		}
	}

	return result.Continue()
}

func (rc *ReconciliationContext) CheckRackPodTemplate(force bool) result.ReconcileResult {
	logger := rc.ReqLogger
	dc := rc.Datacenter
	logger.Info("reconcile_racks::CheckRackPodTemplate")

	for idx := range rc.desiredRackInformation {
		rackName := rc.desiredRackInformation[idx].RackName
		if force {
			forceRacks := dc.Spec.ForceUpgradeRacks
			if len(forceRacks) > 0 {
				if utils.IndexOfString(forceRacks, rackName) <= 0 {
					continue
				}
			}
		}

		if dc.Spec.CanaryUpgrade && idx > 0 {
			logger.
				WithValues("rackName", rackName).
				Info("Skipping rack because CanaryUpgrade is turned on")
			return result.Continue()
		}
		statefulSet := rc.statefulSets[idx]

		status := statefulSet.Status

		updatedReplicas := status.UpdatedReplicas
		if status.CurrentRevision != status.UpdateRevision {
			// Previous update was a canary upgrade, so we have pods in different versions
			updatedReplicas = status.CurrentReplicas + status.UpdatedReplicas
		}

		if !force && (statefulSet.Generation != status.ObservedGeneration ||
			status.Replicas != status.ReadyReplicas ||
			status.Replicas != updatedReplicas) {

			logger.Info(
				"waiting for upgrade to finish on statefulset",
				"statefulset", statefulSet.Name,
				"replicas", status.Replicas,
				"readyReplicas", status.ReadyReplicas,
				"currentReplicas", status.CurrentReplicas,
				"updatedReplicas", status.UpdatedReplicas,
			)

			return result.RequeueSoon(10)
		}

		desiredSts, err := newStatefulSetForCassandraDatacenter(statefulSet, rackName, dc, int(*statefulSet.Spec.Replicas))

		if err != nil {
			logger.Error(err, "error calling newStatefulSetForCassandraDatacenter")
			return result.Error(err)
		}

		// Set the CassandraDatacenter as the owner and controller
		err = setControllerReference(
			rc.Datacenter,
			desiredSts,
			rc.Scheme)
		if err != nil {
			logger.Error(err, "error calling setControllerReference for statefulset", "desiredSts.Namespace",
				desiredSts.Namespace, "desireSts.Name", desiredSts.Name)
			return result.Error(err)
		}

		if !force && !utils.ResourcesHaveSameHash(statefulSet, desiredSts) && !rc.UpdateAllowed() {
			logger.
				WithValues("rackName", rackName).
				Info("update is blocked, but statefulset needs an update. Marking datacenter as requiring update.")

			if err := rc.setConditionStatus(api.DatacenterRequiresUpdate, corev1.ConditionTrue); err != nil {
				return result.Error(err)
			}

			return result.Continue()
		}

		if !utils.ResourcesHaveSameHash(statefulSet, desiredSts) && (force || rc.UpdateAllowed()) {
			logger.
				WithValues("rackName", rackName).
				Info("statefulset needs an update")

			// "fix" the replica count, and maintain labels and annotations the k8s admin may have set
			desiredSts.Spec.Replicas = statefulSet.Spec.Replicas
			desiredSts.Labels = utils.MergeMap(map[string]string{}, statefulSet.Labels, desiredSts.Labels)
			desiredSts.Annotations = utils.MergeMap(map[string]string{}, statefulSet.Annotations, desiredSts.Annotations)

			// copy the stuff that can't be updated
			if res := rc.CheckVolumeClaimSizes(statefulSet, desiredSts); res.Completed() {
				return res
			}
			desiredSts.Spec.VolumeClaimTemplates = statefulSet.Spec.VolumeClaimTemplates

			// selector must match podTemplate.Labels, those can't be updated either
			desiredSts.Spec.Selector = statefulSet.Spec.Selector

			stateMeta, err := meta.Accessor(statefulSet)
			resVersion := stateMeta.GetResourceVersion()
			if err != nil {
				return result.Error(err)
			}

			desiredSts.DeepCopyInto(statefulSet)

			rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.UpdatingRack,
				"Updating rack %s", rackName, "force", force)

			if err := rc.setConditionStatus(api.DatacenterUpdating, corev1.ConditionTrue); err != nil {
				return result.Error(err)
			}

			if err := setOperatorProgressStatus(rc, api.ProgressUpdating); err != nil {
				return result.Error(err)
			}

			logger.Info("Updating statefulset pod specs",
				"statefulSet", statefulSet,
			)

			statefulSet.SetResourceVersion(resVersion)
			if err := rc.Client.Update(rc.Ctx, statefulSet); err != nil {
				if errors.IsInvalid(err) {
					rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.RecreatingStatefulSet, "Recreating statefulset %s", statefulSet.Name)
					if err = rc.deleteStatefulSet(statefulSet); err != nil {
						return result.Error(err)
					}
				} else {
					return result.Error(err)
				}
			}

			if !force {
				if err := rc.enableQuietPeriod(20); err != nil {
					logger.Error(
						err,
						"Error when enabling quiet period")
					return result.Error(err)
				}
			}

			// TODO Do we really want to modify spec here?

			// we just updated k8s and pods will be knocked out of ready state, so let k8s
			// call us back when these changes are done and the new pods are back to ready
			return result.Done()
		}
	}

	logger.Info("done CheckRackPodTemplate()")
	return result.Continue()
}

func (rc *ReconciliationContext) CheckRackForceUpgrade() result.ReconcileResult {
	dc := rc.Datacenter
	logger := rc.ReqLogger
	logger.Info("reconcile_racks::CheckRackForceUpgrade")

	// Datacenter configuration isn't healthy, we allow upgrades here before pods start
	if rc.failureModeDetection() {
		logger.Info("Failure detected, forcing CheckRackPodTemplate()")
		return rc.CheckRackPodTemplate(true)
	}

	forceRacks := dc.Spec.ForceUpgradeRacks
	if len(forceRacks) == 0 {
		return result.Continue()
	}

	return rc.CheckRackPodTemplate(true)
}

func (rc *ReconciliationContext) deleteStatefulSet(statefulSet *appsv1.StatefulSet) error {
	policy := metav1.DeletePropagationOrphan
	cascadePolicy := client.DeleteOptions{
		PropagationPolicy: &policy,
	}

	if err := rc.Client.Delete(rc.Ctx, statefulSet, &cascadePolicy); err != nil {
		return err
	}

	return nil
}

func (rc *ReconciliationContext) CheckRackLabels() result.ReconcileResult {
	rc.ReqLogger.Info("reconcile_racks::CheckRackLabels")

	for idx := range rc.desiredRackInformation {
		rackInfo := rc.desiredRackInformation[idx]
		statefulSet := rc.statefulSets[idx]
		patch := client.MergeFrom(statefulSet.DeepCopy())

		// Has this statefulset been reconciled?

		stsLabels := statefulSet.GetLabels()
		shouldUpdateLabels, updatedLabels := shouldUpdateLabelsForRackResource(stsLabels, rc.Datacenter, rackInfo.RackName)

		if shouldUpdateLabels {
			rc.ReqLogger.Info("Updating labels",
				"statefulSet", statefulSet,
				"current", stsLabels,
				"desired", updatedLabels)
			statefulSet.SetLabels(updatedLabels)

			if err := rc.Client.Patch(rc.Ctx, statefulSet, patch); err != nil {
				return result.Error(err)
			}

			rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.LabeledRackResource,
				"Update rack labels for StatefulSet %s", statefulSet.Name)
		}

		stsAnns := statefulSet.GetAnnotations()
		oplabels.AddOperatorAnnotations(stsAnns, rc.Datacenter)
		if !reflect.DeepEqual(stsAnns, statefulSet.GetAnnotations()) {
			rc.ReqLogger.Info("Updating annotations",
				"statefulSet", statefulSet,
				"current", stsAnns,
				"desired", updatedLabels)
			statefulSet.SetAnnotations(stsAnns)

			if err := rc.Client.Patch(rc.Ctx, statefulSet, patch); err != nil {
				return result.Error(err)
			}

			rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.LabeledRackResource,
				"Update rack annotations for StatefulSet %s", statefulSet.Name)
		}
	}

	return result.Continue()
}

func (rc *ReconciliationContext) CheckRackStoppedState() result.ReconcileResult {
	logger := rc.ReqLogger

	emittedStoppingEvent := false
	racksUpdated := false
	for idx := range rc.desiredRackInformation {
		rackInfo := rc.desiredRackInformation[idx]
		statefulSet := rc.statefulSets[idx]

		stopped := rc.Datacenter.Spec.Stopped
		currentPodCount := *statefulSet.Spec.Replicas

		if stopped && currentPodCount > 0 {
			logger.Info(
				"CassandraDatacenter is stopped, setting rack to zero replicas",
				"rack", rackInfo.RackName,
				"currentSize", currentPodCount,
			)

			if !emittedStoppingEvent {
				if err := rc.setConditionStatus(api.DatacenterStopped, corev1.ConditionTrue); err != nil {
					return result.Error(err)
				}

				if err := rc.setConditionStatus(api.DatacenterReady, corev1.ConditionFalse); err != nil {
					return result.Error(err)
				}

				rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.StoppingDatacenter,
					"Stopping datacenter")
				emittedStoppingEvent = true
			}

			rackPods := rc.rackPods(rackInfo.RackName)

			nodesDrained := 0
			nodeDrainErrors := 0

			for _, pod := range rackPods {
				if isMgmtApiRunning(pod) {
					nodesDrained++
					err := rc.NodeMgmtClient.CallDrainEndpoint(pod)
					// if we got an error during drain, just log it and count it
					// and then keep going, because we don't want to try restarting
					// the server just to bring it down
					if err != nil {
						logger.Error(err, "error during node drain",
							"pod", pod.Name)
						nodeDrainErrors++
					}
				}
			}

			logger.Info("rack drains done",
				"rack", rackInfo.RackName,
				"nodesDrained", nodesDrained,
				"nodeDrainErrors", nodeDrainErrors,
			)

			err := rc.UpdateRackNodeCount(statefulSet, 0)
			if err != nil {
				return result.Error(err)
			}
			racksUpdated = true
		}
	}

	if racksUpdated {
		return result.Done()
	}
	return result.Continue()
}

// checkSeedLabels loops over all racks and makes sure that the proper pods are labelled as seeds.
func (rc *ReconciliationContext) checkSeedLabels() (int, error) {
	rc.ReqLogger.Info("reconcile_racks::CheckSeedLabels")
	seedCount := 0
	for idx := range rc.desiredRackInformation {
		rackInfo := rc.desiredRackInformation[idx]
		n, err := rc.labelSeedPods(rackInfo)
		seedCount += n
		if err != nil {
			return 0, err
		}
	}
	return seedCount, nil
}

func shouldUseFastPath(dc *api.CassandraDatacenter, seedCount int) bool {
	return seedCount > 0 && !(metav1.HasAnnotation(dc.ObjectMeta, api.AllowParallelStartsAnnotations) && dc.Annotations[api.AllowParallelStartsAnnotations] == "false")
}

// CheckPodsReady loops over all the server pods and starts them
func (rc *ReconciliationContext) CheckPodsReady(endpointData httphelper.CassMetadataEndpoints) result.ReconcileResult {
	rc.ReqLogger.Info("reconcile_racks::CheckPodsReady")

	if rc.Datacenter.Spec.Stopped {
		return result.Continue()
	}

	// all errors in this function we're going to treat as likely ephemeral problems that would resolve
	// so we use ResultShouldRequeueSoon to check again soon

	// successes where we want to end this reconcile loop, we generally also want to wait a bit
	// because stuff is happening concurrently in k8s (getting pods from pending to running)
	// or Cassandra (getting a node bootstrapped and ready), so we use ResultShouldRequeueSoon to try again soon

	// step 0 - see if any nodes lost their readiness
	// or gained it back
	nodeStartedNotReady, err := rc.findStartedNotReadyNodes()
	if err != nil {
		return result.Error(err)
	}
	if nodeStartedNotReady {
		return result.RequeueSoon(2)
	}

	// delete stuck nodes

	deletedNode, err := rc.deleteStuckNodes()
	if err != nil {
		return result.Error(err)
	}
	if deletedNode {
		return result.Done()
	}

	// get the nodes labelled as seeds before we start any nodes

	seedCount, err := rc.checkSeedLabels()
	if err != nil {
		return result.Error(err)
	}
	err = rc.refreshSeeds()
	if err != nil {
		return result.Error(err)
	}

	// step 0 - fastpath
	if shouldUseFastPath(rc.Datacenter, seedCount) {
		notReadyPods, err := rc.startBootstrappedNodes(endpointData)
		if err != nil {
			return result.Error(err)
		}

		// Technically this is checked in the next part, but there could be cache issues
		if notReadyPods {
			return result.RequeueSoon(2)
		}
	}

	// step 1 - see if any nodes are already coming up

	nodeIsStarting, _, err := rc.findStartingNodes()

	if err != nil {
		return result.Error(err)
	}
	if nodeIsStarting {
		return result.RequeueSoon(2)
	}

	// step 2 - get one node up per rack

	atLeastOneFirstNodeNotReady, err := rc.startOneNodePerRack(endpointData, seedCount)

	if err != nil {
		return result.Error(err)
	}
	if atLeastOneFirstNodeNotReady {
		return result.RequeueSoon(10)
	}

	// step 3 - if the cluster isn't healthy, that's ok, but go back to step 1

	clusterHealthy := rc.isClusterHealthy()
	if err := rc.updateHealth(clusterHealthy); err != nil {
		return result.Error(err)
	}

	if !clusterHealthy && !(rc.Datacenter.Status.GetConditionStatus(api.DatacenterDecommission) == corev1.ConditionTrue) {
		rc.ReqLogger.Info(
			"cluster isn't healthy",
		)
		return result.RequeueSoon(5)
	}

	// step 4 - make sure all nodes are ready

	atLeastOneNodeNotReady, err := rc.startAllNodes(endpointData)
	if err != nil {
		return result.Error(err)
	}
	if atLeastOneNodeNotReady {
		return result.RequeueSoon(2)
	}

	// Wait on any nodes that are still being replaced
	if len(rc.Datacenter.Status.NodeReplacements) > 0 {
		return result.RequeueSoon(2)
	}

	// step 5 sanity check that all pods are labelled as started and are ready

	readyPodCount, startedLabelCount := rc.countReadyAndStarted()
	desiredSize := int(rc.Datacenter.Spec.Size)

	if desiredSize <= readyPodCount && desiredSize <= startedLabelCount {
		return result.Continue()
	} else {
		err := fmt.Errorf("checks failed desired:%d, ready:%d, started:%d", desiredSize, readyPodCount, startedLabelCount)
		return result.Error(err)
	}
}

func getStatefulSetPodNameForIdx(sts *appsv1.StatefulSet, idx int32) string {
	return fmt.Sprintf("%s-%v", sts.Name, idx)
}

func getStatefulSetPodNames(sts *appsv1.StatefulSet) utils.StringSet {
	status := sts.Status
	podNames := utils.StringSet{}
	for i := int32(0); i < status.Replicas; i++ {
		podNames[getStatefulSetPodNameForIdx(sts, i)] = true
	}
	return podNames
}

func getPodNamesFromPods(pods []*corev1.Pod) utils.StringSet {
	podNames := utils.StringSet{}
	for _, pod := range pods {
		podNames[pod.Name] = true
	}
	return podNames
}

// From check_nodes.go

func (rc *ReconciliationContext) GetPodPVC(podNamespace string, podName string) (*corev1.PersistentVolumeClaim, error) {
	pvcFullName := fmt.Sprintf("%s-%s", PvcName, podName)

	pvc := &corev1.PersistentVolumeClaim{}
	err := rc.Client.Get(rc.Ctx, types.NamespacedName{Namespace: podNamespace, Name: pvcFullName}, pvc)
	if err != nil {
		rc.ReqLogger.Error(err, "error retrieving PersistentVolumeClaim")
		return nil, err
	}

	return pvc, nil
}

func (rc *ReconciliationContext) getDCPodByName(podName string) *corev1.Pod {
	for _, pod := range rc.dcPods {
		if pod.Name == podName {
			return pod
		}
	}
	return nil
}

func hasStatefulSetControllerCaughtUp(statefulSets []*appsv1.StatefulSet, dcPods []*corev1.Pod) bool {
	for _, statefulSet := range statefulSets {
		if statefulSet == nil {
			continue
		}

		// Has the statefulset controller seen the latest spec
		if statefulSet.Generation != statefulSet.Status.ObservedGeneration {
			return false
		}

		// Has the statefulset controller gotten around to creating the pods
		podsThatShouldExist := getStatefulSetPodNames(statefulSet)
		dcPodNames := getPodNamesFromPods(dcPods)
		delta := utils.SubtractStringSet(podsThatShouldExist, dcPodNames)
		if len(delta) > 0 {
			return false
		}
	}

	return true
}

// CheckRackScale loops over each statefulset and makes sure that it has the right
// amount of desired replicas. At this time we can only increase the amount of replicas.
func (rc *ReconciliationContext) CheckRackScale() result.ReconcileResult {
	logger := rc.ReqLogger
	logger.Info("reconcile_racks::CheckRackScale")
	dc := rc.Datacenter

	for idx := range rc.desiredRackInformation {
		rackInfo := rc.desiredRackInformation[idx]
		statefulSet := rc.statefulSets[idx]

		// By the time we get here we know all the racks are ready for that particular size

		desiredNodeCount := int32(rackInfo.NodeCount)
		maxReplicas := *statefulSet.Spec.Replicas

		if maxReplicas < desiredNodeCount {
			// Check to see if we are resuming from stopped and update conditions appropriately
			if dc.GetConditionStatus(api.DatacenterStopped) == corev1.ConditionTrue {
				if err := rc.setConditionStatus(api.DatacenterStopped, corev1.ConditionFalse); err != nil {
					return result.Error(err)
				}

				if err := rc.setConditionStatus(api.DatacenterResuming, corev1.ConditionTrue); err != nil {
					return result.Error(err)
				}
			} else if dc.GetConditionStatus(api.DatacenterReady) == corev1.ConditionTrue {
				// We weren't resuming from a stopped state, so we must be growing the
				// size of the rack and this isn't the initialization stage
				if err := rc.setConditionStatus(api.DatacenterScalingUp, corev1.ConditionTrue); err != nil {
					return result.Error(err)
				}
			}

			// update it
			rc.ReqLogger.Info(
				"Need to update the rack's node count",
				"Rack", rackInfo.RackName,
				"maxReplicas", maxReplicas,
				"desiredSize", desiredNodeCount,
			)

			rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.ScalingUpRack,
				"Scaling up rack %s", rackInfo.RackName)

			err := rc.UpdateRackNodeCount(statefulSet, desiredNodeCount)
			if err != nil {
				return result.Error(err)
			}
		}
	}

	return result.Continue()
}

// CheckRackPodLabels checks each pod and its volume(s) and makes sure they have the
// proper labels
func (rc *ReconciliationContext) CheckRackPodLabels() result.ReconcileResult {
	rc.ReqLogger.Info("reconcile_racks::CheckRackPodLabels")

	for idx := range rc.desiredRackInformation {
		statefulSet := rc.statefulSets[idx]

		if err := rc.ReconcilePods(statefulSet); err != nil {
			return result.Error(err)
		}
	}

	return result.Continue()
}

func (rc *ReconciliationContext) upsertUser(user api.CassandraUser) error {
	dc := rc.Datacenter
	namespace := dc.ObjectMeta.Namespace

	namespacedName := types.NamespacedName{
		Name:      user.SecretName,
		Namespace: namespace,
	}

	secret, err := rc.retrieveSecret(namespacedName)
	if err != nil {
		return err
	}

	// We will call mgmt API on the first pod
	pod := rc.dcPods[0]

	err = rc.NodeMgmtClient.CallCreateRoleEndpoint(
		pod,
		string(secret.Data["username"]),
		string(secret.Data["password"]),
		user.Superuser)

	return err
}

func (rc *ReconciliationContext) GetUsers() []api.CassandraUser {
	dc := rc.Datacenter
	// add the standard superuser to our list of users
	users := dc.Spec.Users
	users = append(users, api.CassandraUser{
		Superuser:  true,
		SecretName: dc.GetSuperuserSecretNamespacedName().Name,
	})

	return users
}

func (rc *ReconciliationContext) UpdateSecretWatches() error {
	dc := rc.Datacenter
	users := rc.GetUsers()
	names := []types.NamespacedName{}
	for _, user := range users {
		name := types.NamespacedName{Name: user.SecretName, Namespace: dc.Namespace}
		names = append(names, name)
	}
	dcNamespacedName := types.NamespacedName{Name: dc.Name, Namespace: dc.Namespace}
	err := rc.SecretWatches.UpdateWatch(dcNamespacedName, names)

	return err
}

func (rc *ReconciliationContext) CreateUsers() result.ReconcileResult {
	dc := rc.Datacenter

	if val, found := dc.Annotations[api.SkipUserCreationAnnotation]; found && val == "true" {
		rc.ReqLogger.Info(api.SkipUserCreationAnnotation + " is set, skipping CreateUser")
		return result.Continue()
	}

	if dc.Spec.Stopped || rc.Datacenter.GetDeletionTimestamp() != nil {
		rc.ReqLogger.Info("cluster is stopped, skipping CreateUser")
		return result.Continue()
	}

	rc.ReqLogger.Info("reconcile_racks::CreateUsers")

	err := rc.UpdateSecretWatches()
	if err != nil {
		rc.ReqLogger.Error(err, "Failed to update dynamic watches on secrets")
	}

	// make sure the default superuser secret exists
	_, err = rc.retrieveSuperuserSecretOrCreateDefault()
	if err != nil {
		rc.ReqLogger.Error(err, "Failed to verify superuser secret status")
	}

	users := rc.GetUsers()

	for _, user := range users {
		err := rc.upsertUser(user)
		if err != nil {
			rc.ReqLogger.Error(err, "error updating user", "secretName", user.SecretName)
			return result.Error(err)
		}
	}

	rc.Recorder.Eventf(dc, corev1.EventTypeNormal, events.CreatedUsers,
		"Created users")

	// For backwards compatibility
	rc.Recorder.Eventf(dc, corev1.EventTypeNormal, events.CreatedSuperuser,
		"Created superuser")

	patch := client.MergeFrom(rc.Datacenter.DeepCopy())
	rc.Datacenter.Status.UsersUpserted = metav1.Now()

	// For backwards compatibility
	rc.Datacenter.Status.SuperUserUpserted = metav1.Now()

	if err = rc.Client.Status().Patch(rc.Ctx, rc.Datacenter, patch); err != nil {
		rc.ReqLogger.Error(err, "error updating the users upsert timestamp")
		return result.Error(err)
	}

	return result.Continue()
}

func findHostIdForIpFromEndpointsData(endpointsData []httphelper.EndpointState, ip string) (bool, string) {
	for _, data := range endpointsData {
		if net.ParseIP(data.GetRpcAddress()).Equal(net.ParseIP(ip)) {
			if data.HasStatus(httphelper.StatusNormal) {
				return true, data.HostID
			}
			return false, data.HostID
		}
	}
	return false, ""
}

func getRpcAddress(dc *api.CassandraDatacenter, pod *corev1.Pod) string {
	nc := dc.Spec.Networking
	if nc != nil {
		if nc.HostNetwork {
			return pod.Status.HostIP
		}
		if nc.NodePort != nil {
			if nc.NodePort.Internode > 0 ||
				nc.NodePort.InternodeSSL > 0 {
				return pod.Status.HostIP
			}
		}
	}
	return pod.Status.PodIP
}

func (rc *ReconciliationContext) UpdateCassandraNodeStatus(force bool) error {
	logger := rc.ReqLogger
	dc := rc.Datacenter

	if dc.Status.NodeStatuses == nil {
		dc.Status.NodeStatuses = map[string]api.CassandraNodeStatus{}
	}

	for _, pod := range ListAllStartedPods(rc.dcPods) {
		nodeStatus, ok := dc.Status.NodeStatuses[pod.Name]
		if !ok {
			nodeStatus = api.CassandraNodeStatus{}
		}

		if metav1.HasLabel(pod.ObjectMeta, api.CassNodeState) {
			if pod.Labels[api.CassNodeState] == stateDecommissioning {
				continue
			}
		}

		logger.Info("Setting state", "Running", isMgmtApiRunning(pod), "pod", pod.Name)
		if pod.Status.PodIP != "" && isMgmtApiRunning(pod) {
			ip := getRpcAddress(dc, pod)
			nodeStatus.IP = ip
			nodeStatus.Rack = pod.Labels[api.RackLabel]

			// Getting the HostID requires a call to the node management API which is
			// moderately expensive, so if we already have a HostID, don't bother. This
			// would only change if something has gone horribly horribly wrong.
			if force || nodeStatus.HostID == "" {
				endpointsResponse, err := rc.NodeMgmtClient.CallMetadataEndpointsEndpoint(pod)
				if err == nil {
					ready, hostId := findHostIdForIpFromEndpointsData(
						endpointsResponse.Entity, ip)
					if nodeStatus.HostID == "" {
						logger.Info("Failed to find host ID", "pod", pod.Name)
					}
					if ready {
						nodeStatus.HostID = hostId
					}
				}
			}
		}

		dc.Status.NodeStatuses[pod.Name] = nodeStatus
	}

	return nil
}

func getTimePodCreated(pod *corev1.Pod) metav1.Time {
	return pod.ObjectMeta.CreationTimestamp
}

func getTimeStartedReplacingNodes(dc *api.CassandraDatacenter) (metav1.Time, bool) {
	replaceCondition, hasReplaceCondition := dc.GetCondition(api.DatacenterReplacingNodes)
	if hasReplaceCondition && replaceCondition.Status == corev1.ConditionTrue {
		return replaceCondition.LastTransitionTime, true
	} else {
		return metav1.Time{}, false
	}
}

func (rc *ReconciliationContext) updateCurrentReplacePodsProgress() error {
	dc := rc.Datacenter
	logger := rc.ReqLogger
	startedPods := ListAllStartedPods(rc.dcPods)

	// Update current progress of replacing pods
	if len(dc.Status.NodeReplacements) > 0 {
		for _, pod := range startedPods {
			// Since pod is labeled as started, it should be done being replaced
			if utils.IndexOfString(dc.Status.NodeReplacements, pod.Name) > -1 {

				// Ensure the pod is not only started but created _after_ we
				// started replacing nodes. This is because the Pod may have
				// been ready, marked for replacement, and then deleted, so we
				// have to make sure this is the incarnation of the Pod from
				// after the pod was deleted to be replaced.
				timeStartedReplacing, isReplacing := getTimeStartedReplacingNodes(dc)
				if isReplacing {
					timeCreated := getTimePodCreated(pod)

					// There isn't a good way to tell the operator to abort
					// replacing a node, so if we've been replacing for over
					// 30 minutes, and the pod is started, we'll go ahead and
					// clear it.
					replacingForOver30min := hasBeenXMinutes(30, timeStartedReplacing.Time)

					if replacingForOver30min || timeStartedReplacing.Before(&timeCreated) || timeStartedReplacing.Equal(&timeCreated) {
						logger.Info("Finished replacing pod", "pod", pod.Name)

						rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.FinishedReplaceNode,
							"Finished replacing pod %s", pod.Name)

						dc.Status.NodeReplacements = utils.RemoveValueFromStringArray(dc.Status.NodeReplacements, pod.Name)
						if err := rc.UpdateCassandraNodeStatus(true); err != nil {
							return err
						}
					}
				}
			}
		}
	}

	return nil
}

func (rc *ReconciliationContext) startReplacePodsIfReplacePodsSpecified() error {
	dc := rc.Datacenter

	if len(dc.Spec.DeprecatedReplaceNodes) > 0 {
		rc.ReqLogger.Info("Requested replacing pods", "pods", dc.Spec.DeprecatedReplaceNodes)

		for _, podName := range dc.Spec.DeprecatedReplaceNodes {
			// Each podName has to be in the dcPods
			for _, dcPod := range rc.dcPods {
				if podName == dcPod.Name {
					dc.Status.NodeReplacements = utils.AppendValuesToStringArrayIfNotPresent(
						dc.Status.NodeReplacements, podName)
					break
				}
			}
			rc.ReqLogger.Error(fmt.Errorf("invalid pod name in ReplaceNodes"), "Rejected ReplaceNode entry, pod does not exist in the Datacenter", "PodName", podName)
		}

		if len(dc.Status.NodeReplacements) > 0 {
			podNamesString := strings.Join(dc.Status.NodeReplacements, ", ")

			if err := rc.setConditionStatus(api.DatacenterReplacingNodes, corev1.ConditionTrue); err != nil {
				return err
			}

			rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.ReplacingNode,
				"Replacing Cassandra nodes for pods %s", podNamesString)
		}

		// Now that we've recorded these nodes in the status, we can blank
		// out this field on the spec
		dc.Spec.DeprecatedReplaceNodes = []string{}
	}

	return nil
}

func (rc *ReconciliationContext) UpdateStatusForUserActions() error {
	var err error

	err = rc.updateCurrentReplacePodsProgress()
	if err != nil {
		return err
	}

	err = rc.startReplacePodsIfReplacePodsSpecified()
	if err != nil {
		return err
	}

	return nil
}

func (rc *ReconciliationContext) UpdateStatus() result.ReconcileResult {
	dc := rc.Datacenter
	oldDc := rc.Datacenter.DeepCopy()

	rc.ReqLogger.Info("Setting pod statuses")
	err := rc.UpdateCassandraNodeStatus(false)
	if err != nil {
		return result.Error(err)
	}

	err = rc.UpdateStatusForUserActions()
	if err != nil {
		return result.Error(err)
	}

	status := &api.CassandraDatacenterStatus{}
	dc.Status.DeepCopyInto(status)
	oldDc.Status.DeepCopyInto(&dc.Status)

	if !reflect.DeepEqual(dc, oldDc) {
		patch := client.MergeFrom(oldDc)
		if err := rc.Client.Patch(rc.Ctx, dc, patch); err != nil {
			return result.Error(err)
		}
	}

	if !reflect.DeepEqual(status, &oldDc.Status) {
		// Make a new patch for just the status. We want to use our potentially
		// updated DC as the base. Keep in mind the patch we did above may or
		// may not have stomped on our status changes.
		oldDcForStatus := dc.DeepCopy()
		patch := client.MergeFrom(oldDcForStatus)

		// Update the DC with our status
		status.DeepCopyInto(&dc.Status)

		if err := rc.Client.Status().Patch(rc.Ctx, dc, patch); err != nil {
			return result.Error(err)
		}
	}

	// Update current state of the pods
	monitoring.RemoveDatacenterPods(dc.Namespace, dc.Spec.ClusterName, dc.DatacenterName())
	for _, pod := range rc.dcPods {
		monitoring.UpdatePodStatusMetric(pod)
	}

	return result.Continue()
}

func (rc *ReconciliationContext) updateHealth(healthy bool) error {
	if !healthy {
		return rc.setConditionStatus(api.DatacenterHealthy, corev1.ConditionFalse)
	}
	return rc.setConditionStatus(api.DatacenterHealthy, corev1.ConditionTrue)
}

func hasBeenXMinutes(x int, sinceTime time.Time) bool {
	xMinutesAgo := time.Now().Add(time.Minute * time.Duration(-x))
	return sinceTime.Before(xMinutesAgo)
}

func hasBeenXMinutesSinceReady(x int, pod *corev1.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == "Ready" && c.Status == "False" {
			return hasBeenXMinutes(x, c.LastTransitionTime.Time)
		}
	}
	return false
}

func hasBeenXMinutesSinceTerminated(x int, pod *corev1.Pod) bool {
	if status := getCassContainerStatus(pod); status != nil {
		lastState := status.LastTerminationState
		if lastState.Terminated != nil {
			return hasBeenXMinutes(x, lastState.Terminated.FinishedAt.Time)
		}
	}
	return false
}

func getCassContainerStatus(pod *corev1.Pod) *corev1.ContainerStatus {
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name != "cassandra" {
			continue
		}
		return &status
	}
	return nil
}

func isNodeStuckAfterTerminating(pod *corev1.Pod) bool {
	if isServerReady(pod) || isServerReadyToStart(pod) {
		return false
	}

	return hasBeenXMinutesSinceTerminated(10, pod)
}

func isNodeStuckAfterLosingReadiness(pod *corev1.Pod) bool {
	if !isServerStartedNotReady(pod) || isServerReadyToStart(pod) {
		return false
	}
	return hasBeenXMinutesSinceReady(10, pod)
}

func (rc *ReconciliationContext) getCassMetadataEndpoints() httphelper.CassMetadataEndpoints {
	var metadata httphelper.CassMetadataEndpoints
	for _, pod := range rc.clusterPods {
		// Try to query the first ready pod we find.
		// We won't get any endpoints back if no pods are ready yet.
		if !isServerReady(pod) {
			continue
		}

		metadata, _ = rc.NodeMgmtClient.CallMetadataEndpointsEndpoint(pod)

		if len(metadata.Entity) == 0 {
			continue
		}
		break
	}

	return metadata
}

// When a vmware taint occurs and we delete the pvc, pv, and
// pod, then a new pod is created to replace the deleted pod.
// However, there is a kubernetes bug that prevents a new pvc
// from being created, and the new pod gets stuck in Pending.
// see: https://github.com/kubernetes/kubernetes/issues/89910
//
// If we then delete this new pod, then the stateful will
// properly recreate a pvc, pv, and pod.
func (rc *ReconciliationContext) isNodeStuckWithoutPVC(pod *corev1.Pod) bool {
	if pod.Status.Phase == corev1.PodPending {
		_, err := rc.GetPodPVC(pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
		if err != nil {
			if errors.IsNotFound(err) {
				return true
			} else {
				rc.ReqLogger.Error(err,
					"Unable to get PersistentVolumeClaim")
			}
		}
	}

	return false
}

func (rc *ReconciliationContext) deleteStuckNodes() (bool, error) {
	rc.ReqLogger.Info("reconcile_racks::deleteStuckNodes")
	for _, pod := range rc.dcPods {
		shouldDelete := false
		reason := ""
		if isNodeStuckAfterTerminating(pod) {
			reason = "Pod got stuck after Cassandra container terminated"
			shouldDelete = true
		} else if isNodeStuckAfterLosingReadiness(pod) {
			reason = "Pod got stuck after losing readiness"
			shouldDelete = true
		}

		if shouldDelete {
			rc.ReqLogger.Info(fmt.Sprintf("Deleting stuck pod: %s. Reason: %s", pod.Name, reason))
			rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeWarning, events.DeletingStuckPod,
				reason)
			return true, rc.Client.Delete(rc.Ctx, pod)
		}
	}

	return false, nil
}

// isClusterHealthy does a LOCAL_QUORUM query to the Cassandra pods and returns true if all the pods were able to
// respond without error.
func (rc *ReconciliationContext) isClusterHealthy() bool {
	pods := FilterPodListByCassNodeState(rc.clusterPods, stateStarted)

	numRacks := len(rc.Datacenter.GetRacks())
	for _, pod := range pods {
		err := rc.NodeMgmtClient.CallProbeClusterEndpoint(pod, "LOCAL_QUORUM", numRacks)
		if err != nil {
			reason := fmt.Sprintf("Pod %s failed the LOCAL_QUORUM check", pod.Name)
			rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeWarning, events.UnhealthyDatacenter,
				reason)
			return false
		}
	}

	return true
}

// labelSeedPods iterates over all pods for a statefulset and makes sure the right number of
// ready pods are labelled as seeds, so that they are picked up by the headless seed service
// Returns the number of ready seeds.
func (rc *ReconciliationContext) labelSeedPods(rackInfo *RackInformation) (int, error) {
	logger := rc.ReqLogger.WithName("labelSeedPods")

	rackPods := rc.rackPods(rackInfo.RackName)

	sort.SliceStable(rackPods, func(i, j int) bool {
		return rackPods[i].Name < rackPods[j].Name
	})
	count := 0
	for _, pod := range rackPods {
		patch := client.MergeFrom(pod.DeepCopy())

		newLabels := make(map[string]string)
		utils.MergeMap(newLabels, pod.GetLabels())

		ready := isServerReady(pod)
		starting := isServerStarting(pod)

		isSeed := ready && count < rackInfo.SeedCount
		currentVal := pod.GetLabels()[api.SeedNodeLabel]
		if isSeed {
			count++
		}

		// this is the main place we label pods as seeds / not-seeds
		// the one exception to this is the very first node we bring up
		// in an empty cluster, and we set that node as a seed
		// in startOneNodePerRack()

		shouldUpdate := false
		if isSeed && currentVal != "true" {
			rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.LabeledPodAsSeed,
				"Labeled as seed node pod %s", pod.Name)

			newLabels[api.SeedNodeLabel] = "true"
			shouldUpdate = true
		}
		// if this pod is starting, we should leave the seed label alone
		if !isSeed && currentVal == "true" && !starting {
			rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.UnlabeledPodAsSeed,
				"Unlabled as seed node pod %s", pod.Name)

			delete(newLabels, api.SeedNodeLabel)
			shouldUpdate = true
		}

		if shouldUpdate {
			pod.SetLabels(newLabels)
			if err := rc.Client.Patch(rc.Ctx, pod, patch); err != nil {
				logger.Error(
					err, "Unable to update pod with seed label",
					"pod", pod.Name)
				return 0, err
			}
		}
	}
	return count, nil
}

// GetStatefulSetForRack returns the statefulset for the rack
// and whether it currently exists and whether an error occurred
func (rc *ReconciliationContext) GetStatefulSetForRack(
	nextRack *RackInformation) (*appsv1.StatefulSet, bool, error) {

	rc.ReqLogger.Info("reconcile_racks::getStatefulSetForRack")

	// Check if the desiredStatefulSet already exists
	currentStatefulSet := &appsv1.StatefulSet{}
	err := rc.Client.Get(
		rc.Ctx,
		NewNamespacedNameForStatefulSet(rc.Datacenter, nextRack.RackName),
		currentStatefulSet)

	if err == nil {
		return currentStatefulSet, true, nil
	}

	if !errors.IsNotFound(err) {
		return nil, false, err
	}

	desiredStatefulSet, err := newStatefulSetForCassandraDatacenter(
		currentStatefulSet,
		nextRack.RackName,
		rc.Datacenter,
		nextRack.NodeCount)
	if err != nil {
		return nil, false, err
	}

	// Set the CassandraDatacenter as the owner and controller
	err = setControllerReference(
		rc.Datacenter,
		desiredStatefulSet,
		rc.Scheme)
	if err != nil {
		return nil, false, err
	}

	return desiredStatefulSet, false, nil
}

// ReconcileNextRack ensures that the resources for a rack have been properly created
func (rc *ReconciliationContext) ReconcileNextRack(statefulSet *appsv1.StatefulSet) error {

	rc.ReqLogger.Info("reconcile_racks::reconcileNextRack")

	if err := setOperatorProgressStatus(rc, api.ProgressUpdating); err != nil {
		return err
	}

	// Create the StatefulSet

	rc.ReqLogger.Info(
		"Creating a new StatefulSet.",
		"statefulSetNamespace", statefulSet.Namespace,
		"statefulSetName", statefulSet.Name)
	if err := rc.Client.Create(rc.Ctx, statefulSet); err != nil {
		return err
	}
	rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.CreatedResource,
		"Created statefulset %s", statefulSet.Name)

	return nil
}

func (rc *ReconciliationContext) CheckDcPodDisruptionBudget() result.ReconcileResult {
	// Create a PodDisruptionBudget for the CassandraDatacenter
	dc := rc.Datacenter
	ctx := rc.Ctx
	desiredBudget := newPodDisruptionBudgetForDatacenter(dc)

	// Set CassandraDatacenter as the owner and controller
	if err := setControllerReference(dc, desiredBudget, rc.Scheme); err != nil {
		return result.Error(err)
	}

	// Check if the budget already exists
	currentBudget := &policyv1.PodDisruptionBudget{}
	err := rc.Client.Get(
		ctx,
		types.NamespacedName{
			Name:      desiredBudget.Name,
			Namespace: desiredBudget.Namespace},
		currentBudget)

	if err != nil && !errors.IsNotFound(err) {
		return result.Error(err)
	}

	found := err == nil

	if found && utils.ResourcesHaveSameHash(currentBudget, desiredBudget) {
		return result.Continue()
	}

	// it's not possible to update a PodDisruptionBudget, so we need to delete this one and remake it
	if found {
		rc.ReqLogger.Info(
			"Deleting and re-creating a PodDisruptionBudget",
			"pdbNamespace", desiredBudget.Namespace,
			"pdbName", desiredBudget.Name,
			"oldMinAvailable", currentBudget.Spec.MinAvailable,
			"desiredMinAvailable", desiredBudget.Spec.MinAvailable,
		)
		err = rc.Client.Delete(ctx, currentBudget)
		if err != nil {
			return result.Error(err)
		}
	}

	// Create the Budget
	rc.ReqLogger.Info(
		"Creating a new PodDisruptionBudget.",
		"pdbNamespace", desiredBudget.Namespace,
		"pdbName", desiredBudget.Name)

	err = rc.Client.Create(ctx, desiredBudget)
	if err != nil {
		return result.Error(err)
	}

	rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.CreatedResource,
		"Created PodDisruptionBudget %s", desiredBudget.Name)

	return result.Continue()
}

// Updates the node count on a rack (statefulset)
func (rc *ReconciliationContext) UpdateRackNodeCount(statefulSet *appsv1.StatefulSet, newNodeCount int32) error {
	rc.ReqLogger.Info("reconcile_racks::updateRack")

	rc.ReqLogger.Info(
		"updating StatefulSet node count",
		"statefulSetNamespace", statefulSet.Namespace,
		"statefulSetName", statefulSet.Name,
		"newNodeCount", newNodeCount,
	)

	if err := setOperatorProgressStatus(rc, api.ProgressUpdating); err != nil {
		return err
	}

	patch := client.MergeFrom(statefulSet.DeepCopy())
	statefulSet.Spec.Replicas = &newNodeCount

	err := rc.Client.Patch(rc.Ctx, statefulSet, patch)

	return err
}

// ReconcilePods ...
func (rc *ReconciliationContext) ReconcilePods(statefulSet *appsv1.StatefulSet) error {
	rc.ReqLogger.Info("reconcile_racks::ReconcilePods")

	for i := int32(0); i < statefulSet.Status.Replicas; i++ {
		podName := getStatefulSetPodNameForIdx(statefulSet, i)

		pod := &corev1.Pod{}
		err := rc.Client.Get(
			rc.Ctx,
			types.NamespacedName{
				Name:      podName,
				Namespace: statefulSet.Namespace},
			pod)
		if err != nil {
			rc.ReqLogger.Error(
				err,
				"Unable to get pod",
				"Pod", podName,
			)
			return err
		}

		podPatch := client.MergeFrom(pod.DeepCopy())

		podLabels := pod.GetLabels()
		shouldUpdateLabels, updatedLabels := shouldUpdateLabelsForRackResource(podLabels,
			rc.Datacenter, statefulSet.GetLabels()[api.RackLabel])
		if shouldUpdateLabels {
			rc.ReqLogger.Info(
				"Updating labels",
				"Pod", podName,
				"current", podLabels,
				"desired", updatedLabels)

			pod.SetLabels(updatedLabels)

			if err := rc.Client.Patch(rc.Ctx, pod, podPatch); err != nil {
				rc.ReqLogger.Error(
					err,
					"Unable to update pod with label",
					"Pod", podName,
				)
			}

			rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.LabeledRackResource,
				"Update rack labels for Pod %s", podName)
		}

		if len(pod.Spec.Volumes) == 0 || pod.Spec.Volumes[0].PersistentVolumeClaim == nil {
			continue
		}

		pvcName := pod.Spec.Volumes[0].PersistentVolumeClaim.ClaimName
		pvc := &corev1.PersistentVolumeClaim{
			TypeMeta: metav1.TypeMeta{
				Kind:       "PersistentVolumeClaim",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: statefulSet.Namespace,
			},
		}
		err = rc.Client.Get(
			rc.Ctx,
			types.NamespacedName{
				Name:      pvcName,
				Namespace: statefulSet.Namespace},
			pvc)
		if err != nil {
			rc.ReqLogger.Error(
				err,
				"Unable to get pvc",
				"PVC", pvcName,
			)
			return err
		}

		pvcPatch := client.MergeFrom(pvc.DeepCopy())

		pvcLabels := pvc.GetLabels()
		shouldUpdateLabels, updatedLabels = shouldUpdateLabelsForRackResource(pvcLabels,
			rc.Datacenter, statefulSet.GetLabels()[api.RackLabel])
		if shouldUpdateLabels {
			rc.ReqLogger.Info("Updating labels",
				"PVC", pvc,
				"current", pvcLabels,
				"desired", updatedLabels)

			pvc.SetLabels(updatedLabels)

			if err := rc.Client.Patch(rc.Ctx, pvc, pvcPatch); err != nil {
				rc.ReqLogger.Error(
					err,
					"Unable to update pvc with labels",
					"PVC", pvc,
				)
			}

			rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.LabeledRackResource,
				"Update rack labels for PersistentVolumeClaim %s", pvc.Name)
		}
		pvcAnns := pvc.GetAnnotations()
		oplabels.AddOperatorAnnotations(pvcAnns, rc.Datacenter)
		if !reflect.DeepEqual(pvcAnns, pvc.GetAnnotations()) {
			rc.ReqLogger.Info("Updating annotations",
				"PVC", pvc,
				"current", pvc.GetAnnotations(),
				"desired", pvcAnns)
			pvc.SetAnnotations(pvcAnns)
			pvcPatch := client.MergeFrom(pvc.DeepCopy())
			if err := rc.Client.Patch(rc.Ctx, pvc, pvcPatch); err != nil {
				rc.ReqLogger.Error(
					err,
					"Unable to update pvc with annotation",
					"PVC", pvc,
				)
			}

			rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.LabeledRackResource,
				"Update rack annotations for pvc %s", pvc.Name)
		}
	}

	return nil
}

func mergeInLabelsIfDifferent(existingLabels, newLabels map[string]string) (bool, map[string]string) {
	updatedLabels := utils.MergeMap(map[string]string{}, existingLabels, newLabels)
	if reflect.DeepEqual(existingLabels, updatedLabels) {
		return false, existingLabels
	} else {
		return true, updatedLabels
	}
}

// shouldUpdateLabelsForClusterResource will compare the labels passed in with what the labels should be for a cluster level
// resource. It will return the updated map and a boolean denoting whether the resource needs to be updated with the new labels.
func shouldUpdateLabelsForClusterResource(resourceLabels map[string]string, dc *api.CassandraDatacenter) (bool, map[string]string) {
	desired := dc.GetClusterLabels()
	oplabels.AddOperatorLabels(desired, dc)
	return mergeInLabelsIfDifferent(resourceLabels, desired)
}

// shouldUpdateLabelsForRackResource will compare the labels passed in with what the labels should be for a rack level
// resource. It will return the updated map and a boolean denoting whether the resource needs to be updated with the new labels.
func shouldUpdateLabelsForRackResource(resourceLabels map[string]string, dc *api.CassandraDatacenter, rackName string) (bool, map[string]string) {
	desired := dc.GetRackLabels(rackName)
	oplabels.AddOperatorLabels(desired, dc)
	return mergeInLabelsIfDifferent(resourceLabels, desired)
}

func (rc *ReconciliationContext) labelServerPodStarting(pod *corev1.Pod) error {
	ctx := rc.Ctx
	dc := rc.Datacenter
	podPatch := client.MergeFrom(pod.DeepCopy())
	pod.Labels[api.CassNodeState] = stateStarting
	err := rc.Client.Patch(ctx, pod, podPatch)
	if err != nil {
		return err
	}

	statusPatch := client.MergeFrom(dc.DeepCopy())
	dc.Status.LastServerNodeStarted = metav1.Now()
	err = rc.Client.Status().Patch(rc.Ctx, dc, statusPatch)
	return err
}

func (rc *ReconciliationContext) enableQuietPeriod(seconds int) error {
	dc := rc.Datacenter

	dur := QuietDurationFunc(seconds)
	statusPatch := client.MergeFrom(dc.DeepCopy())
	dc.Status.QuietPeriod = metav1.NewTime(time.Now().Add(dur))
	err := rc.Client.Status().Patch(rc.Ctx, dc, statusPatch)
	return err
}

func (rc *ReconciliationContext) labelServerPodStarted(pod *corev1.Pod) error {
	patch := client.MergeFrom(pod.DeepCopy())
	pod.Labels[api.CassNodeState] = stateStarted
	err := rc.Client.Patch(rc.Ctx, pod, patch)
	return err
}

func (rc *ReconciliationContext) labelServerPodStartedNotReady(pod *corev1.Pod) error {
	patch := client.MergeFrom(pod.DeepCopy())
	pod.Labels[api.CassNodeState] = stateStartedNotReady
	err := rc.Client.Patch(rc.Ctx, pod, patch)
	return err
}

// Checks to see if any node is starting. This is done by checking to see if the cassandra.datastax.com/node-state label
// has a value of Starting. If it does then check to see if the C* node is ready. If the node is ready, the pod's
// cassandra.datastax.com/node-state label is set to a value of Started. This function returns two bools and an error.
// The first bool is true if there is a C* node that is Starting. The second bool is set to true if a C* node has just
// transitioned to the Started state by having its cassandra.datastax.com/node-state label set to Started. The error is
// non-nil if updating the pod's labels fails.
func (rc *ReconciliationContext) findStartingNodes() (bool, bool, error) {
	rc.ReqLogger.Info("reconcile_racks::findStartingNodes")

	for _, pod := range rc.clusterPods {
		if pod.Labels[api.CassNodeState] == stateStarting {
			if isServerReady(pod) {
				rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.StartedCassandra,
					"Started Cassandra for pod %s", pod.Name)
				if err := rc.labelServerPodStarted(pod); err != nil {
					return false, false, err
				} else {
					return false, true, nil
				}
			} else {
				return true, false, nil
			}
		}
	}
	return false, false, nil
}

func (rc *ReconciliationContext) startBootstrappedNodes(endpointData httphelper.CassMetadataEndpoints) (bool, error) {
	rc.ReqLogger.Info("reconcile_racks::startBootstrappedNodes")

	startingNodes := false

	for _, pod := range rc.dcPods {
		if _, ok := rc.Datacenter.Status.NodeStatuses[pod.Name]; ok {
			// Verify pod is not going to be replaced
			if utils.IndexOfString(rc.Datacenter.Status.NodeReplacements, pod.Name) > -1 {
				continue
			}
			notReady, err := rc.startNode(pod, false, endpointData)
			if err != nil {
				return startingNodes, err
			}

			startingNodes = startingNodes || notReady
		}
	}

	return startingNodes, nil
}

func (rc *ReconciliationContext) findStartedNotReadyNodes() (bool, error) {
	rc.ReqLogger.Info("reconcile_racks::findStartedNotReadyNodes")

	for _, pod := range rc.dcPods {
		if didServerLoseReadiness(pod) {
			if err := rc.labelServerPodStartedNotReady(pod); err != nil {
				return false, err
			}
			return true, nil
		}

		if isServerStartedNotReady(pod) {
			if isServerReady(pod) {
				if err := rc.labelServerPodStarted(pod); err != nil {
					return false, err
				}
				return false, nil
			}
		}
	}
	return false, nil
}

func (rc *ReconciliationContext) startCassandra(endpointData httphelper.CassMetadataEndpoints, pod *corev1.Pod) error {
	dc := rc.Datacenter

	// Are we replacing this node?
	shouldReplacePod := utils.IndexOfString(dc.Status.NodeReplacements, pod.Name) > -1

	replaceAddress := ""

	if shouldReplacePod {
		// Get the HostID for pod if it has one
		nodeStatus, ok := dc.Status.NodeStatuses[pod.Name]
		hostId := ""
		if ok {
			hostId = nodeStatus.HostID
		}

		// Get the replace address
		var err error
		if hostId != "" {
			replaceAddress, err = FindIpForHostId(endpointData, hostId)
			if err != nil {
				return fmt.Errorf("failed to start replace of cassandra node %s for pod %s due to error: %w", hostId, pod.Name, err)
			}
		}
	}

	go func(pod *corev1.Pod) {
		var err error
		if shouldReplacePod && replaceAddress != "" {
			// If we have a replace address that means the cassandra node did
			// join the ring previously and is marked for replacement, so we
			// start it accordingly
			rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.StartingCassandraAndReplacingNode,
				"Starting Cassandra for pod %s to replace Cassandra node with address %s", pod.Name, replaceAddress)
			err = rc.NodeMgmtClient.CallLifecycleStartEndpointWithReplaceIp(pod, replaceAddress)
		} else {
			// Either we are not replacing this pod or the relevant cassandra node
			// never joined the ring in the first place and can be started normally
			rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.StartingCassandra,
				"Starting Cassandra for pod %s", pod.Name)
			err = rc.NodeMgmtClient.CallLifecycleStartEndpoint(pod)
		}

		if err != nil {
			// Pod was unable to start. Most likely this is not a recoverable error, so lets kill the pod and
			// try again.
			if deleteErr := rc.Client.Delete(rc.Ctx, pod); deleteErr != nil {
				rc.ReqLogger.Error(err, "Unable to delete the pod, pod has failed to start", "Pod", pod.Name)
			}
			rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeWarning, events.StartingCassandra,
				"Failed to start pod %s, deleting it", pod.Name)
		}
	}(pod)

	return rc.labelServerPodStarting(pod)
}

// startOneNodePerRack starts the first pod in each rack in a deterministic order, dictated by the
// order in which the racks were declared in the datacenter spec. It returns true if a Cassandra
// node is not ready yet, and an error if that node also failed to start.
func (rc *ReconciliationContext) startOneNodePerRack(endpointData httphelper.CassMetadataEndpoints, readySeeds int) (bool, error) {
	rc.ReqLogger.Info("reconcile_racks::startOneNodePerRack")

	labelSeedBeforeStart := readySeeds == 0 && !rc.hasAdditionalSeeds()

	for _, statefulSet := range rc.statefulSets {
		if *statefulSet.Spec.Replicas < 1 {
			continue
		}
		podName := getStatefulSetPodNameForIdx(statefulSet, 0)
		pod := rc.getDCPodByName(podName)
		notReady, err := rc.startNode(pod, labelSeedBeforeStart, endpointData)
		if notReady || err != nil {
			return notReady, err
		}
	}
	return false, nil
}

// startAllNodes starts all nodes in the datacenter in a deterministic order, dictated by the order
// in which the racks were declared in the datacenter spec, and in such a way that at any given
// time, the number of started nodes in a rack cannot be greater or lesser than the number of
// started nodes in all other racks by more than 1. It returns true if a Cassandra node is not ready
// yet, and an error if that node also failed to start.
func (rc *ReconciliationContext) startAllNodes(endpointData httphelper.CassMetadataEndpoints) (bool, error) {
	rc.ReqLogger.Info("reconcile_racks::startAllNodes")

	for podRankWithinRack := int32(0); ; podRankWithinRack++ {

		done := true
		for _, statefulSet := range rc.statefulSets {

			maxPodRankInThisRack := *statefulSet.Spec.Replicas - 1
			if podRankWithinRack <= maxPodRankInThisRack {

				podName := getStatefulSetPodNameForIdx(statefulSet, podRankWithinRack)
				pod := rc.getDCPodByName(podName)
				notReady, err := rc.startNode(pod, false, endpointData)
				if notReady || err != nil {
					return notReady, err
				}

				done = done && podRankWithinRack == maxPodRankInThisRack
			}
		}
		if done {
			return false, nil
		}
	}
}

// hasAdditionalSeeds returns true if the datacenter has at least one additional seed.
func (rc *ReconciliationContext) hasAdditionalSeeds() bool {
	if len(rc.Datacenter.Spec.AdditionalSeeds) > 0 {
		return true
	}
	additionalSeedEndpoints := 0
	if additionalSeedEndpoint, err := rc.GetAdditionalSeedEndpoint(); err == nil {
		if len(additionalSeedEndpoint.Subsets) > 0 {
			additionalSeedEndpoints = len(additionalSeedEndpoint.Subsets[0].Addresses)
		}
	}
	return additionalSeedEndpoints > 0
}

// startNode starts the Cassandra node in the given pod, if it wasn't started yet. It returns true
// if the node isn't ready yet, and a non-nil error if the node that isn't ready yet also failed to
// start.
func (rc *ReconciliationContext) startNode(pod *corev1.Pod, labelSeedBeforeStart bool, endpointData httphelper.CassMetadataEndpoints) (bool, error) {
	if pod == nil {
		return true, nil
	}

	if !isServerReady(pod) {
		if isServerReadyToStart(pod) && isMgmtApiRunning(pod) {

			// this is the one exception to all seed labelling happening in labelSeedPods()
			if labelSeedBeforeStart {
				patch := client.MergeFrom(pod.DeepCopy())
				pod.Labels[api.SeedNodeLabel] = "true"
				if err := rc.Client.Patch(rc.Ctx, pod, patch); err != nil {
					return true, err
				}

				rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.LabeledPodAsSeed,
					"Labeled pod a seed node %s", pod.Name)
			}

			if err := rc.startCassandra(endpointData, pod); err != nil {
				return true, err
			}
		}
		return true, nil
	}
	return false, nil
}

func (rc *ReconciliationContext) countReadyAndStarted() (int, int) {
	ready := 0
	started := 0
	for _, pod := range rc.dcPods {
		if isServerReady(pod) {
			ready++
		}

		if isServerStarted(pod) {
			started++
		}
	}
	return ready, started
}

func isMgmtApiRunning(pod *corev1.Pod) bool {
	podStatus := pod.Status
	statuses := podStatus.ContainerStatuses
	for _, status := range statuses {
		if status.Name != "cassandra" {
			continue
		}
		state := status.State
		runInfo := state.Running
		if runInfo != nil {
			// give management API ten seconds to come up
			tenSecondsAgo := time.Now().Add(time.Second * -10)
			return runInfo.StartedAt.Time.Before(tenSecondsAgo)
		}
	}
	return false
}

func isServerStarting(pod *corev1.Pod) bool {
	return pod.Labels[api.CassNodeState] == stateStarting
}

func isServerStarted(pod *corev1.Pod) bool {
	return pod.Labels[api.CassNodeState] == stateStarted ||
		pod.Labels[api.CassNodeState] == stateStartedNotReady
}

func isServerStartedNotReady(pod *corev1.Pod) bool {
	return pod.Labels[api.CassNodeState] == stateStartedNotReady
}

func isServerReadyToStart(pod *corev1.Pod) bool {
	return pod.Labels[api.CassNodeState] == stateReadyToStart
}

func didServerLoseReadiness(pod *corev1.Pod) bool {
	if pod.Labels[api.CassNodeState] == stateStarted {
		return !isServerReady(pod)
	}
	return false
}

func isServerReady(pod *corev1.Pod) bool {
	status := pod.Status
	statuses := status.ContainerStatuses
	for _, status := range statuses {
		if status.Name != "cassandra" {
			continue
		}
		return status.Ready
	}
	return false
}

func (rc *ReconciliationContext) refreshSeeds() error {
	rc.ReqLogger.Info("reconcile_racks::refreshSeeds")
	if rc.Datacenter.Spec.Stopped || rc.Datacenter.GetDeletionTimestamp() != nil {
		rc.ReqLogger.Info("cluster is stopped/deleted, skipping refreshSeeds")
		return nil
	}

	startedPods := FilterPodListByCassNodeState(rc.clusterPods, stateStarted)

	for _, pod := range startedPods {
		if err := rc.NodeMgmtClient.CallReloadSeedsEndpoint(pod); err != nil {
			return err
		}
	}

	return nil
}

func (rc *ReconciliationContext) listPods(selector map[string]string) ([]*corev1.Pod, error) {
	rc.ReqLogger.Info("reconcile_racks::listPods")

	listOptions := &client.ListOptions{
		Namespace:     rc.Datacenter.Namespace,
		LabelSelector: labels.SelectorFromSet(selector),
	}

	podList := &corev1.PodList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
	}

	if err := rc.Client.List(rc.Ctx, podList, listOptions); err != nil {
		return nil, err
	}

	return PodPtrsFromPodList(podList), nil
}

func (rc *ReconciliationContext) CheckRollingRestart() result.ReconcileResult {
	dc := rc.Datacenter
	logger := rc.ReqLogger

	if dc.Spec.DeprecatedRollingRestartRequested {
		dc.Status.LastRollingRestart = metav1.Now()
		if err := rc.setConditionStatus(api.DatacenterRollingRestart, corev1.ConditionTrue); err != nil {
			return result.Error(err)
		}

		dcPatch := client.MergeFromWithOptions(dc.DeepCopy(), client.MergeFromWithOptimisticLock{})
		dc.Spec.DeprecatedRollingRestartRequested = false
		if err := rc.Client.Patch(rc.Ctx, dc, dcPatch); err != nil {
			logger.Error(err, "error patching datacenter for rolling restart")
			return result.Error(err)
		}
	}

	cutoff := &dc.Status.LastRollingRestart
	for _, pod := range rc.dcPods {
		podStartTime := pod.GetCreationTimestamp()
		if podStartTime.Before(cutoff) {
			rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.RestartingCassandra,
				"Restarting Cassandra for pod %s", pod.Name)

			// drain the node
			err := rc.NodeMgmtClient.CallDrainEndpoint(pod)
			if err != nil {
				logger.Error(err, "error during drain during rolling restart",
					"pod", pod.Name)
			}
			// get a fresh pod
			err = rc.Client.Delete(rc.Ctx, pod)
			if err != nil {
				return result.Error(err)
			}
			return result.Done()
		}
	}

	return result.Continue()
}

func (rc *ReconciliationContext) setConditionStatus(conditionType api.DatacenterConditionType, status corev1.ConditionStatus) error {
	return rc.setCondition(api.NewDatacenterCondition(conditionType, status))
}

func (rc *ReconciliationContext) setCondition(condition *api.DatacenterCondition) error {
	dc := rc.Datacenter
	updated := false
	if dc.GetConditionStatus(condition.Type) != condition.Status {
		// We are changing the status, so record the transition time
		condition.LastTransitionTime = metav1.Now()
		dc.SetCondition(*condition)
		updated = true
	}

	if updated {
		// Modify the metric also
		monitoring.SetDatacenterConditionMetric(dc, condition.Type, condition.Status)
		// We use Update here to avoid removing some other changes to the Status that might have happened,
		// as well as updating them at the same time
		return rc.Client.Status().Update(rc.Ctx, dc)
	}

	return nil
}

func (rc *ReconciliationContext) CheckConditionInitializedAndReady() result.ReconcileResult {
	rc.ReqLogger.Info("reconcile_racks::CheckConditionInitializedAndReady")
	dc := rc.Datacenter

	if err := rc.setConditionStatus(api.DatacenterInitialized, corev1.ConditionTrue); err != nil {
		return result.Error(err)
	}

	if dc.GetConditionStatus(api.DatacenterStopped) == corev1.ConditionFalse {
		if err := rc.setConditionStatus(api.DatacenterReady, corev1.ConditionTrue); err != nil {
			return result.Error(err)
		}
	}

	return result.Continue()
}

func (rc *ReconciliationContext) CheckCassandraNodeStatuses() result.ReconcileResult {
	dc := rc.Datacenter
	logger := rc.ReqLogger

	// Check that we have a HostID for every pod in the datacenter
	for _, pod := range rc.dcPods {
		nodeStatus, ok := dc.Status.NodeStatuses[pod.Name]
		if !ok || nodeStatus.HostID == "" {
			logger.Info("Missing host id", "pod", pod.Name)
			return result.RequeueSoon(2)
		}
	}

	return result.Continue()
}

func (rc *ReconciliationContext) cleanupAfterScaling() result.ReconcileResult {
	if !metav1.HasAnnotation(rc.Datacenter.ObjectMeta, api.NoAutomatedCleanupAnnotation) {

		if metav1.HasAnnotation(rc.Datacenter.ObjectMeta, api.TrackCleanupTasksAnnotation) {
			// Verify if the cleanup task has completed before moving on the with ScalingUp finished
			task, err := rc.findActiveTask(taskapi.CommandCleanup)
			if err != nil {
				return result.Error(err)
			}

			if task != nil {
				return rc.activeTaskCompleted(task)
			}
		}

		// Create the cleanup task
		if err := rc.createTask(taskapi.CommandCleanup); err != nil {
			return result.Error(err)
		}

		if metav1.HasAnnotation(rc.Datacenter.ObjectMeta, api.TrackCleanupTasksAnnotation) {
			return result.RequeueSoon(10)
		}
	}

	return result.Continue()
}

func (rc *ReconciliationContext) createTask(command taskapi.CassandraCommand) error {
	generatedName := fmt.Sprintf("%s-%d", command, time.Now().Unix())
	dc := rc.Datacenter
	anns := make(map[string]string)

	oplabels.AddOperatorAnnotations(anns, dc)
	task := &taskapi.CassandraTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:        generatedName,
			Namespace:   rc.Datacenter.Namespace,
			Labels:      dc.GetDatacenterLabels(),
			Annotations: anns,
		},
		Spec: taskapi.CassandraTaskSpec{
			Datacenter: corev1.ObjectReference{
				Name:      rc.Datacenter.Name,
				Namespace: rc.Datacenter.Namespace,
			},
			CassandraTaskTemplate: taskapi.CassandraTaskTemplate{
				Jobs: []taskapi.CassandraJob{
					{
						Name:    fmt.Sprintf("%s-%s", command, rc.Datacenter.Name),
						Command: command,
					},
				},
			},
		},
		Status: taskapi.CassandraTaskStatus{},
	}

	oplabels.AddOperatorLabels(task.GetLabels(), dc)

	if err := rc.Client.Create(rc.Ctx, task); err != nil {
		return err
	}

	if !metav1.HasAnnotation(rc.Datacenter.ObjectMeta, api.TrackCleanupTasksAnnotation) {
		return nil
	}

	dcPatch := client.MergeFrom(dc.DeepCopy())

	rc.Datacenter.Status.AddTaskToTrack(task.ObjectMeta)

	return rc.Client.Status().Patch(rc.Ctx, dc, dcPatch)
}

func (rc *ReconciliationContext) activeTaskCompleted(task *taskapi.CassandraTask) result.ReconcileResult {
	if task.Status.CompletionTime != nil {
		// Job was completed, remove it from followed task
		dc := rc.Datacenter
		dcPatch := client.MergeFrom(dc.DeepCopy())
		rc.Datacenter.Status.RemoveTrackedTask(task.ObjectMeta)
		if err := rc.Client.Status().Patch(rc.Ctx, dc, dcPatch); err != nil {
			return result.Error(err)
		}
		return result.Continue()
	}
	return result.RequeueSoon(10)
}

func (rc *ReconciliationContext) findActiveTask(command taskapi.CassandraCommand) (*taskapi.CassandraTask, error) {
	if len(rc.Datacenter.Status.TrackedTasks) > 0 {
		for _, taskMeta := range rc.Datacenter.Status.TrackedTasks {
			taskKey := types.NamespacedName{Name: taskMeta.Name, Namespace: taskMeta.Namespace}
			task := &taskapi.CassandraTask{}
			if err := rc.Client.Get(rc.Ctx, taskKey, task); err != nil {
				return nil, err
			}

			for _, job := range task.Spec.Jobs {
				if job.Command == command {
					return task, nil
				}
			}
		}
	}
	return nil, nil
}

func (rc *ReconciliationContext) CheckClearActionConditions() result.ReconcileResult {
	rc.ReqLogger.Info("reconcile_racks::CheckClearActionConditions")
	dc := rc.Datacenter

	// If we are here, any action that was in progress should now be completed, so start
	// clearing conditions
	conditionsThatShouldBeFalse := []api.DatacenterConditionType{
		api.DatacenterReplacingNodes,
		api.DatacenterUpdating,
		api.DatacenterRollingRestart,
		api.DatacenterResuming,
		api.DatacenterScalingDown,
	}
	conditionsThatShouldBeTrue := []api.DatacenterConditionType{
		api.DatacenterValid,
	}

	// Explicitly handle scaling up here because we want to run a cleanup afterwards
	if dc.GetConditionStatus(api.DatacenterScalingUp) == corev1.ConditionTrue {
		// Spawn a cleanup task before continuing
		if res := rc.cleanupAfterScaling(); res.Completed() {
			return res
		}

		if err := rc.setConditionStatus(api.DatacenterScalingUp, corev1.ConditionFalse); err != nil {
			return result.Error(err)
		}
	}

	// Make sure that the stopped condition matches the spec, because logically
	// we can make it through a reconcile loop while the dc is in a stopped state
	// and we don't want to reset the stopped condition prematurely
	if dc.Spec.Stopped {
		if err := rc.setConditionStatus(api.DatacenterStopped, corev1.ConditionTrue); err != nil {
			return result.Error(err)
		}
	} else {
		if err := rc.setConditionStatus(api.DatacenterStopped, corev1.ConditionFalse); err != nil {
			return result.Error(err)
		}
	}

	for _, conditionType := range conditionsThatShouldBeFalse {
		if err := rc.setConditionStatus(conditionType, corev1.ConditionFalse); err != nil {
			return result.Error(err)
		}
	}

	for _, conditionType := range conditionsThatShouldBeTrue {
		if err := rc.setConditionStatus(conditionType, corev1.ConditionTrue); err != nil {
			return result.Error(err)
		}
	}

	// Nothing has changed, carry on
	return result.Continue()
}

func (rc *ReconciliationContext) CheckStatefulSetControllerCaughtUp() result.ReconcileResult {
	if hasStatefulSetControllerCaughtUp(rc.statefulSets, rc.dcPods) {
		// We do this here instead of in CheckPodsReady where we fix stuck pods
		// normally because if we were to do it there, every check we do before
		// CheckPodsReady would have to be cognizant of this problem and not fail
		// for ResourceNotFound errors when retrieving a PVC.
		fixedAny, err := rc.fixMissingPVC()
		if err != nil {
			return result.Error(err)
		}
		if fixedAny {
			return result.RequeueSoon(2)
		}
		return result.Continue()
	} else {
		// Reconcile will be called again once the observed generation of the
		// statefulset catches up with the actual generation
		return result.Done()
	}
}

func (rc *ReconciliationContext) fixMissingPVC() (bool, error) {
	// There is a bug where the statefulset does not correctly handle pod and
	// and PVC deletion. If a PVC is deleted, deletion blocks because it is in
	// use by the pod. When the pod is deleted, the statefulset recreates the
	// pod. The pod is then stuck in pending because its PVC is terminating.
	// Once the PVC finishes terminating, the pod remains in pending because
	// the PVC doesn't exist. The statefulset controller doesn't recreate the
	// PVC because the pod already exists.
	//
	// The fix is if we see a pod stuck in pending with no PVC, we just delete
	// the pod so that the statefulset controller will do the right thing and
	// fix it.
	//
	// https://github.com/kubernetes/kubernetes/issues/74374

	for _, pod := range rc.dcPods {
		if rc.isNodeStuckWithoutPVC(pod) {
			reason := "Pod got stuck waiting for PersistentValueClaim"
			rc.ReqLogger.Info(fmt.Sprintf("Deleting stuck pod: %s. Reason: %s", pod.Name, reason))
			rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeWarning, events.DeletingStuckPod,
				reason)
			return true, rc.Client.Delete(rc.Ctx, pod)
		}
	}
	return false, nil
}

func (rc *ReconciliationContext) datacenterPods() []*corev1.Pod {
	if rc.dcPods != nil {
		return rc.dcPods
	}

	dcSelector := rc.Datacenter.GetDatacenterLabels()
	dcPods := FilterPodListByLabels(rc.clusterPods, dcSelector)

	if rc.Datacenter.Status.MetadataVersion < 1 && rc.Datacenter.Status.DatacenterName != nil && *rc.Datacenter.Status.DatacenterName == rc.Datacenter.Spec.DatacenterName {
		rc.ReqLogger.Info("Fetching datacenter pods with the old metadata version labels")
		dcSelector[api.DatacenterLabel] = api.CleanLabelValue(rc.Datacenter.Spec.DatacenterName)
		dcPods = append(dcPods, FilterPodListByLabels(rc.clusterPods, dcSelector)...)
	}

	return dcPods
}

func (rc *ReconciliationContext) rackPods(rackName string) []*corev1.Pod {
	return FilterPodListByLabels(rc.datacenterPods(), map[string]string{api.RackLabel: rackName})
}

// ReconcileAllRacks determines if a rack needs to be reconciled.
func (rc *ReconciliationContext) ReconcileAllRacks() (reconcile.Result, error) {
	rc.ReqLogger.Info("reconciliationContext::reconcileAllRacks")

	logger := rc.ReqLogger

	pods, err := rc.listPods(rc.Datacenter.GetClusterLabels())
	if err != nil {
		logger.Error(err, "error listing all pods in the cluster")
	}

	rc.clusterPods = pods
	rc.dcPods = rc.datacenterPods()

	endpointData := rc.getCassMetadataEndpoints()

	if recResult := rc.CheckStatefulSetControllerCaughtUp(); recResult.Completed() {
		return recResult.Output()
	}

	if recResult := rc.UpdateStatus(); recResult.Completed() {
		return recResult.Output()
	}

	if recResult := rc.CheckConfigSecret(); recResult.Completed() {
		return recResult.Output()
	}

	if recResult := rc.CheckRackCreation(); recResult.Completed() {
		return recResult.Output()
	}

	if recResult := rc.CheckRackLabels(); recResult.Completed() {
		return recResult.Output()
	}

	if recResult := rc.CheckDecommissioningNodes(endpointData); recResult.Completed() {
		return recResult.Output()
	}

	if recResult := rc.CheckSuperuserSecretCreation(); recResult.Completed() {
		return recResult.Output()
	}

	if recResult := rc.CheckInternodeCredentialCreation(); recResult.Completed() {
		return recResult.Output()
	}

	if recResult := rc.CheckRackStoppedState(); recResult.Completed() {
		return recResult.Output()
	}

	if recResult := rc.CheckRackForceUpgrade(); recResult.Completed() {
		return recResult.Output()
	}

	if recResult := rc.CheckRackScale(); recResult.Completed() {
		return recResult.Output()
	}

	if recResult := rc.CheckPodsReady(endpointData); recResult.Completed() {
		return recResult.Output()
	}

	if recResult := rc.CheckCassandraNodeStatuses(); recResult.Completed() {
		return recResult.Output()
	}

	if recResult := rc.DecommissionNodes(endpointData); recResult.Completed() {
		return recResult.Output()
	}

	if recResult := rc.CheckRollingRestart(); recResult.Completed() {
		return recResult.Output()
	}

	if recResult := rc.CheckDcPodDisruptionBudget(); recResult.Completed() {
		return recResult.Output()
	}

	if recResult := rc.CheckPVCResizing(); recResult.Completed() {
		return recResult.Output()
	}

	if recResult := rc.CheckRackPodTemplate(false); recResult.Completed() {
		return recResult.Output()
	}

	if recResult := rc.CheckRackPodLabels(); recResult.Completed() {
		return recResult.Output()
	}

	if recResult := rc.CreateUsers(); recResult.Completed() {
		return recResult.Output()
	}

	if recResult := rc.CheckClearActionConditions(); recResult.Completed() {
		return recResult.Output()
	}

	if recResult := rc.CheckConditionInitializedAndReady(); recResult.Completed() {
		return recResult.Output()
	}

	if recResult := rc.CheckFullQueryLogging(); recResult.Completed() {
		return recResult.Output()
	}

	if err := setOperatorProgressStatus(rc, api.ProgressReady); err != nil {
		return result.Error(err).Output()
	}

	if err := setDatacenterStatus(rc); err != nil {
		return result.Error(err).Output()
	}

	if err := rc.enableQuietPeriod(5); err != nil {
		logger.Error(
			err,
			"Error when enabling quiet period")
		return result.Error(err).Output()
	}

	rc.ReqLogger.Info("All StatefulSets should now be reconciled.")

	return result.Done().Output()
}
