// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"github.com/k8ssandra/cass-operator/internal/result"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/k8ssandra/cass-operator/pkg/utils"
)

func (rc *ReconciliationContext) CheckAdditionalSeedEndpointSlices() result.ReconcileResult {
	logger := rc.ReqLogger
	client := rc.Client

	logger.Info("reconcile_services::checkAdditionalSeedEndpointSlices")

	slices := newEndpointSlicesForAdditionalSeeds(rc.Datacenter)

	for _, slice := range slices {
		hasAddresses := len(slice.Endpoints) > 0 && len(slice.Endpoints[0].Addresses) > 0

		nsName := types.NamespacedName{Name: slice.Name, Namespace: slice.Namespace}
		currentSlice := &discoveryv1.EndpointSlice{}

		err := client.Get(rc.Ctx, nsName, currentSlice)
		if err != nil && errors.IsNotFound(err) {
			if hasAddresses {
				logger.Info("Additional seed endpoint slice not found, creating it", "slice", nsName)
				if err := client.Create(rc.Ctx, slice); err != nil {
					logger.Error(err, "Could not create additional seed endpoint slice",
						"slice", nsName,
					)
					return result.Error(err)
				}
			}
		} else if err != nil {
			logger.Error(err, "Could not get additional seed endpoint slice",
				"slice", nsName,
			)
			return result.Error(err)
		} else {
			if !hasAddresses {
				logger.Info("Deleting endpoint slice as it should now be empty", "slice", nsName)
				if err := client.Delete(rc.Ctx, currentSlice); err != nil {
					logger.Error(err, "Could not delete additional seed endpoint slice",
						"slice", nsName,
					)
					return result.Error(err)
				}
			} else if !utils.ResourcesHaveSameHash(currentSlice, slice) {
				resourceVersion := currentSlice.GetResourceVersion()
				slice.DeepCopyInto(currentSlice)
				currentSlice.SetResourceVersion(resourceVersion)

				if err := client.Update(rc.Ctx, currentSlice); err != nil {
					logger.Error(err, "Unable to update additional seed endpoint slice",
						"slice", currentSlice)
					return result.Error(err)
				}
			}
		}
	}

	return result.Continue()
}

// GetAdditionalSeedAddressCount fetches all EndpointSlices for the additional seeds service
// and returns the total count of addresses across all slices
func (rc *ReconciliationContext) GetAdditionalSeedAddressCount() (int, error) {
	logger := rc.ReqLogger
	kubeClient := rc.Client
	dc := rc.Datacenter
	logger.Info("reconcile_services::getAdditionalSeedAddressCount")

	slices := newEndpointSlicesForAdditionalSeeds(dc)
	totalAddresses := 0

	for _, slice := range slices {
		nsName := types.NamespacedName{Name: slice.Name, Namespace: slice.Namespace}
		currentSlice := &discoveryv1.EndpointSlice{}

		if err := kubeClient.Get(rc.Ctx, nsName, currentSlice); err != nil {
			if errors.IsNotFound(err) {
				logger.V(1).Info("EndpointSlice not found", "name", nsName.Name)
				continue
			}
			logger.Error(err, "Failed to get endpoint slice", "name", nsName.Name)
			return 0, err
		}

		// Count addresses in this slice
		sliceAddresses := 0
		for _, endpoint := range currentSlice.Endpoints {
			sliceAddresses += len(endpoint.Addresses)
		}

		totalAddresses += sliceAddresses

		logger.V(1).Info("Found endpoint slice with addresses",
			"name", currentSlice.Name,
			"addressType", currentSlice.AddressType,
			"addresses", sliceAddresses,
			"totalAddresses", totalAddresses)
	}

	return totalAddresses, nil
}
