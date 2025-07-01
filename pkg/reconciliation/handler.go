// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"fmt"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/types"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/internal/result"
	apiwebhook "github.com/k8ssandra/cass-operator/internal/webhooks/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/pkg/httphelper"
)

// Use a var so we can mock this function
var setControllerReference = controllerutil.SetControllerReference

// key: Node.Name, value: CassandraDatacenter.Name
var (
	nodeToDc     = make(map[string][]types.NamespacedName)
	nodeToDcLock = sync.RWMutex{}
)

// Get the dcNames and dcNamespaces for a node
func DatacentersForNode(nodeName string) []types.NamespacedName {
	nodeToDcLock.RLock()
	defer nodeToDcLock.RUnlock()

	dcs, ok := nodeToDc[nodeName]
	if ok {
		return dcs
	}
	return []types.NamespacedName{}
}

// calculateReconciliationActions will iterate over an ordered list of reconcilers which will determine if any action needs to
// be taken on the CassandraDatacenter. If a change is needed then the apply function will be called on that reconciler and the
// request will be requeued for the next reconciler to handle in the subsequent reconcile loop, otherwise the next reconciler
// will be called.
func (rc *ReconciliationContext) CalculateReconciliationActions() (reconcile.Result, error) {
	rc.ReqLogger.Info("handler::calculateReconciliationActions")

	// Check if the CassandraDatacenter was marked to be deleted
	if result := rc.ProcessDeletion(); result.Completed() {
		return result.Output()
	}

	if err := rc.addFinalizer(); err != nil {
		return result.Error(err).Output()
	}

	if result := rc.CheckHeadlessServices(); result.Completed() {
		return result.Output()
	}

	// if result := rc.CheckAdditionalSeedEndpoints(); result.Completed() {
	// 	return result.Output()
	// }

	if err := rc.CalculateRackInformation(); err != nil {
		return result.Error(err).Output()
	}

	return rc.ReconcileAllRacks()
}

// This file contains various definitions and plumbing setup used for reconciliation.

// For information on log usage, see:
// https://godoc.org/github.com/go-logr/logr

func (rc *ReconciliationContext) addFinalizer() error {
	if _, found := rc.Datacenter.Annotations[api.NoFinalizerAnnotation]; found {
		return nil
	}

	if !controllerutil.ContainsFinalizer(rc.Datacenter, api.Finalizer) && rc.Datacenter.GetDeletionTimestamp() == nil {
		rc.ReqLogger.Info("Adding Finalizer for the CassandraDatacenter")
		controllerutil.AddFinalizer(rc.Datacenter, api.Finalizer)

		// Update CR
		err := rc.Client.Update(rc.Ctx, rc.Datacenter)
		if err != nil {
			return err
		}
	}
	return nil
}

func (rc *ReconciliationContext) IsValid(dc *api.CassandraDatacenter) error {
	errs := []error{}

	// Basic validation up here

	// validate the required superuser
	errs = append(errs, rc.validateSuperuserSecret()...)

	// validate any other defined users
	errs = append(errs, rc.validateCassandraUserSecrets()...)

	// Validate FQL config
	errs = append(errs, apiwebhook.ValidateFQLConfig(dc))

	// Validate Service labels and annotations
	errs = append(errs, apiwebhook.ValidateServiceLabelsAndAnnotations(dc))

	// Validate Management API config
	errs = append(errs, httphelper.ValidateManagementApiConfig(dc, rc.Client, rc.Ctx)...)

	// Check that the datacenter name override hasn't been changed
	errs = append(errs, rc.validateDatacenterNameOverride()...)

	// Validate that the DC sanitized name doesn't conflict with another DC in the same namespace
	errs = append(errs, rc.validateDatacenterNameConflicts()...)

	if len(errs) > 0 {
		return errs[0]
	}

	claim := dc.Spec.StorageConfig.CassandraDataVolumeClaimSpec
	if claim == nil {
		err := fmt.Errorf("storageConfig.cassandraDataVolumeClaimSpec is required")
		return err
	}

	if claim.StorageClassName == nil || *claim.StorageClassName == "" {
		err := fmt.Errorf("storageConfig.cassandraDataVolumeClaimSpec.storageClassName is required")
		return err
	}

	if len(claim.AccessModes) == 0 {
		err := fmt.Errorf("storageConfig.cassandraDataVolumeClaimSpec.accessModes is required")
		return err
	}

	return nil
}
