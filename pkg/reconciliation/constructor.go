// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

// This file defines constructors for k8s objects

import (
	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/pkg/oplabels"
	"github.com/k8ssandra/cass-operator/pkg/utils"

	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Create a PodDisruptionBudget object for the Datacenter
func newPodDisruptionBudgetForDatacenter(dc *api.CassandraDatacenter) *policyv1.PodDisruptionBudget {
	minAvailable := intstr.FromInt(int(dc.Spec.Size - 1))
	labels := dc.GetDatacenterLabels()
	oplabels.AddOperatorLabels(labels, dc)
	selectorLabels := dc.GetDatacenterLabels()
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:        dc.Name + "-pdb",
			Namespace:   dc.Namespace,
			Labels:      labels,
			Annotations: map[string]string{},
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
	currentState := rc.Datacenter.Status.CassandraOperatorProgress
	if currentState == newState {
		// early return, no need to ping k8s
		return nil
	}

	patch := client.MergeFrom(rc.Datacenter.DeepCopy())
	rc.Datacenter.Status.CassandraOperatorProgress = newState
	// TODO there may be a better place to push status.observedGeneration in the reconcile loop
	if newState == api.ProgressReady {
		rc.Datacenter.Status.ObservedGeneration = rc.Datacenter.Generation
	}
	if err := rc.Client.Status().Patch(rc.Ctx, rc.Datacenter, patch); err != nil {
		rc.ReqLogger.Error(err, "error updating the Cassandra Operator Progress state")
		return err
	}

	return nil
}
