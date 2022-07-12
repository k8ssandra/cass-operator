// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"github.com/k8ssandra/cass-operator/pkg/events"
	"github.com/k8ssandra/cass-operator/pkg/internal/result"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/pkg/utils"
)

// ProcessDeletion ...
func (rc *ReconciliationContext) ProcessDeletion() result.ReconcileResult {
	if rc.Datacenter.GetDeletionTimestamp() == nil {
		return result.Continue()
	}

	// If finalizer was removed, we will not do our finalizer processes
	if !controllerutil.ContainsFinalizer(rc.Datacenter, api.Finalizer) {
		return result.Done()
	}

	// set the label here but no need to remove since we're deleting the CassandraDatacenter
	if err := setOperatorProgressStatus(rc, api.ProgressUpdating); err != nil {
		return result.Error(err)
	}

	origSize := rc.Datacenter.Spec.Size
	if rc.Datacenter.Status.GetConditionStatus(api.DatacenterDecommission) == corev1.ConditionTrue {
		rc.Datacenter.Spec.Size = 0
	}

	if rc.Datacenter.Status.GetConditionStatus(api.DatacenterScalingDown) == corev1.ConditionTrue {
		// ScalingDown is still happening
		rc.Recorder.Eventf(rc.Datacenter, corev1.EventTypeNormal, events.DecommissionDatacenter, "Datacenter is decommissioning")
		rc.ReqLogger.V(1).Info("Waiting for the decommission to complete first, before deleting")
		return result.Continue()
	}

	if _, found := rc.Datacenter.Annotations[api.DecommissionOnDeleteAnnotation]; found {
		podList, err := rc.listPods(rc.Datacenter.GetDatacenterLabels())
		if err != nil {
			rc.ReqLogger.Error(err, "Failed to list pods, unable to proceed with deletion")
			return result.Error(err)
		}
		dcPods := PodPtrsFromPodList(podList)
		if len(podList.Items) > 0 {
			rc.ReqLogger.V(1).Info("Deletion is being processed by the decommission check")
			dcs, err := rc.getClusterDatacenters(dcPods)
			if err != nil {
				rc.ReqLogger.Error(err, "Unable to verify if Cassandra Cluster has multiple datacenters")
				// We can't continue, we risk corrupting the Datacenter
				return result.Error(err)
			}

			if len(dcs) > 1 {
				dcPatch := client.MergeFrom(rc.Datacenter.DeepCopy())
				if updated := rc.setCondition(api.NewDatacenterCondition(api.DatacenterDecommission, corev1.ConditionTrue)); updated {
					err := rc.Client.Status().Patch(rc.Ctx, rc.Datacenter, dcPatch)
					if err != nil {
						rc.ReqLogger.Error(err, "error patching datacenter status for decommissiong started")
						return result.Error(err)
					}
				}

				rc.ReqLogger.V(1).Info("Decommissioning the datacenter to 0 nodes first before deletion")
				// Exiting to let other parts of the process take care of the decommission
				return result.Continue()
			}
			// How could we have pods if we've decommissioned everything?
			return result.RequeueSoon(5)
		}
	}

	// Clean up annotation litter on the user Secrets
	err := rc.SecretWatches.RemoveWatcher(types.NamespacedName{
		Name: rc.Datacenter.GetName(), Namespace: rc.Datacenter.GetNamespace()})

	if err != nil {
		rc.ReqLogger.Error(err, "Failed to remove dynamic secret watches for CassandraDatacenter")
	}

	if err := rc.deletePVCs(); err != nil {
		rc.ReqLogger.Error(err, "Failed to delete PVCs for CassandraDatacenter")
		return result.Error(err)
	}

	if utils.IsPSPEnabled() {
		rc.RemoveDcFromNodeToDcMap(types.NamespacedName{
			Name:      rc.Datacenter.GetName(),
			Namespace: rc.Datacenter.GetNamespace()})
	}

	// Update finalizer to allow delete of CassandraDatacenter
	rc.Datacenter.SetFinalizers(nil)
	rc.Datacenter.Spec.Size = origSize // Has to be set to original size, since 0 isn't allowed for the Update to succeed

	// Update CassandraDatacenter
	if err := rc.Client.Update(rc.Ctx, rc.Datacenter); err != nil {
		return result.Error(err)
	}

	return result.Done()
}

func (rc *ReconciliationContext) deletePVCs() error {
	rc.ReqLogger.Info("reconciler::deletePVCs")
	logger := rc.ReqLogger.WithValues(
		"cassandraDatacenterNamespace", rc.Datacenter.Namespace,
		"cassandraDatacenterName", rc.Datacenter.Name,
	)

	persistentVolumeClaimList, err := rc.listPVCs()
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("No PVCs found for CassandraDatacenter")
			return nil
		}
		logger.Error(err, "Failed to list PVCs for cassandraDatacenter")
		return err
	}

	logger.Info(
		"Found PVCs for cassandraDatacenter",
		"numPVCs", len(persistentVolumeClaimList.Items))

	for _, pvc := range persistentVolumeClaimList.Items {
		if err := rc.Client.Delete(rc.Ctx, &pvc); err != nil {
			logger.Error(err, "Failed to delete PVCs for cassandraDatacenter")
			return err
		}
		logger.Info(
			"Deleted PVC",
			"pvcNamespace", pvc.Namespace,
			"pvcName", pvc.Name)
	}

	return nil
}

func (rc *ReconciliationContext) listPVCs() (*corev1.PersistentVolumeClaimList, error) {
	rc.ReqLogger.Info("reconciler::listPVCs")

	selector := map[string]string{
		api.DatacenterLabel: rc.Datacenter.Name,
	}

	listOptions := &client.ListOptions{
		Namespace:     rc.Datacenter.Namespace,
		LabelSelector: labels.SelectorFromSet(selector),
	}

	persistentVolumeClaimList := &corev1.PersistentVolumeClaimList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolumeClaim",
			APIVersion: "v1",
		},
	}

	return persistentVolumeClaimList, rc.Client.List(rc.Ctx, persistentVolumeClaimList, listOptions)
}
