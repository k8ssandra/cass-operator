// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

// This file defines constructors for k8s objects

import (
	"fmt"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/pkg/monitoring"
	"github.com/k8ssandra/cass-operator/pkg/oplabels"
	"github.com/k8ssandra/cass-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// newPodDisruptionBudgetForDatacenter creates a PodDisruptionBudget object for the Datacenter
func newPodDisruptionBudgetForDatacenter(dc *api.CassandraDatacenter) *policyv1.PodDisruptionBudget {
	minAvailable := intstr.FromInt(int(dc.Spec.Size - 1))
	labels := dc.GetDatacenterLabels()
	oplabels.AddOperatorLabels(labels, dc)
	selectorLabels := dc.GetDatacenterLabels()
	anns := map[string]string{}
	oplabels.AddOperatorAnnotations(anns, dc)

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:        dc.Name + "-pdb",
			Namespace:   dc.Namespace,
			Labels:      labels,
			Annotations: anns,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			MinAvailable: &minAvailable,
		},
	}

	// add a hash here to facilitate checking if updates are needed
	utils.AddHashAnnotation(pdb)

	return pdb
}

func setOperatorProgressStatus(rc *ReconciliationContext, newState api.ProgressState) error {
	rc.ReqLogger.Info(fmt.Sprintf("reconcile_racks::setOperatorProgressStatus::%v", newState))
	currentState := rc.Datacenter.Status.CassandraOperatorProgress
	if currentState == newState {
		// early return, no need to ping k8s
		return nil
	}

	patch := client.MergeFrom(rc.Datacenter.DeepCopy())
	rc.Datacenter.Status.CassandraOperatorProgress = newState

	if newState == api.ProgressReady {
		if rc.Datacenter.Status.DatacenterName == nil {
			rc.Datacenter.Status.DatacenterName = &rc.Datacenter.Name
		}
	}
	if err := rc.Client.Status().Patch(rc.Ctx, rc.Datacenter, patch); err != nil {
		rc.ReqLogger.Error(err, "error updating the Cassandra Operator Progress state")
		return err
	}

	monitoring.UpdateOperatorDatacenterProgressStatusMetric(rc.Datacenter, newState)

	return nil
}

func setDatacenterStatus(rc *ReconciliationContext) error {
	if rc.Datacenter.Status.ObservedGeneration != rc.Datacenter.Generation {
		patch := client.MergeFrom(rc.Datacenter.DeepCopy())
		if rc.Datacenter.Status.MetadataVersion < 1 {
			rc.Datacenter.Status.MetadataVersion = 1
		}
		rc.Datacenter.Status.ObservedGeneration = rc.Datacenter.Generation
		if err := rc.Client.Status().Patch(rc.Ctx, rc.Datacenter, patch); err != nil {
			rc.ReqLogger.Error(err, "error updating the Cassandra Operator Progress state")
			return err
		}
	}

	if err := rc.setConditionStatus(api.DatacenterRequiresUpdate, corev1.ConditionFalse); err != nil {
		return err
	}

	// The allow-upgrade=once annotation is temporary and should be removed after first successful reconcile
	if metav1.HasAnnotation(rc.Datacenter.ObjectMeta, api.UpdateAllowedAnnotation) && rc.Datacenter.Annotations[api.UpdateAllowedAnnotation] == string(api.AllowUpdateOnce) {
		// remove the annotation
		patch := client.MergeFrom(rc.Datacenter.DeepCopy())
		delete(rc.Datacenter.Annotations, api.UpdateAllowedAnnotation)
		if err := rc.Client.Patch(rc.Ctx, rc.Datacenter, patch); err != nil {
			rc.ReqLogger.Error(err, "error removing the allow-upgrade=once annotation")
			return err
		}
	}

	return nil
}
