// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/internal/result"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/k8ssandra/cass-operator/pkg/utils"
)

func (rc *ReconciliationContext) CreateEndpointsForAdditionalSeedService() result.ReconcileResult {
	// unpacking
	logger := rc.ReqLogger
	client := rc.Client
	endpoints := rc.Endpoints

	logger.Info(
		"Creating endpoints for additional seed service",
		"endpointsNamespace", endpoints.Namespace,
		"endpointsName", endpoints.Name)

	if err := setOperatorProgressStatus(rc, api.ProgressUpdating); err != nil {
		return result.Error(err)
	}

	if err := client.Create(rc.Ctx, endpoints); err != nil {
		logger.Error(err, "Could not create endpoints for additional seed service")

		return result.Error(err)
	}

	rc.Recorder.Eventf(rc.Datacenter, "Normal", "CreatedResource", "Created endpoints %s", endpoints.Name)

	return result.Continue()
}

func (rc *ReconciliationContext) CheckAdditionalSeedEndpoints() result.ReconcileResult {
	// unpacking
	logger := rc.ReqLogger
	dc := rc.Datacenter
	client := rc.Client

	logger.Info("reconcile_endpoints::CheckAdditionalSeedEndpoints")

	if len(dc.Spec.AdditionalSeeds) == 0 {
		return result.Continue()
	}

	desiredEndpoints, err := newEndpointsForAdditionalSeeds(dc)
	if err != nil {
		logger.Error(err, "Could not set additional seeds for endpoints for additional seed service")
		return result.Error(err)
	}

	createNeeded := false

	// Set CassandraDatacenter dc as the owner and controller
	err = setControllerReference(dc, desiredEndpoints, rc.Scheme)
	if err != nil {
		logger.Error(err, "Could not set controller reference for endpoints for additional seed service")
		return result.Error(err)
	}

	// See if the Endpoints already exists
	currentEndpoints, err := rc.GetAdditionalSeedEndpoint()

	if err != nil && errors.IsNotFound(err) {
		// if it's not found, we need to create it
		createNeeded = true
	} else if err != nil {
		// if we hit a k8s error, log it and error out
		nsName := types.NamespacedName{Name: desiredEndpoints.Name, Namespace: desiredEndpoints.Namespace}
		logger.Error(err, "Could not get endpoints for additional seed service",
			"name", nsName,
		)
		return result.Error(err)
	} else {
		// desiredEndpoints always has just a single Subset at most - we can apply safely there all the addresses we still want to keep
		for _, endpoints := range currentEndpoints.Endpoints {
			if endpoints.TargetRef != nil {
				desiredEndpoints.Endpoints[0].Addresses = append(desiredEndpoints.Endpoints[0].Addresses, endpoints.Addresses...)
			}
		}

		// if we found the endpoints already, check if it needs updating
		if !utils.ResourcesHaveSameHash(currentEndpoints, desiredEndpoints) {
			resourceVersion := currentEndpoints.GetResourceVersion()
			// preserve any labels and annotations that were added to the Endpoints post-creation
			desiredEndpoints.Labels = utils.MergeMap(map[string]string{}, currentEndpoints.Labels, desiredEndpoints.Labels)
			desiredEndpoints.Annotations = utils.MergeMap(map[string]string{}, currentEndpoints.Annotations, desiredEndpoints.Annotations)

			logger.Info("Updating endpoints for additional seed service",
				"endpoints", currentEndpoints,
				"desired", desiredEndpoints)

			desiredEndpoints.DeepCopyInto(currentEndpoints)

			currentEndpoints.SetResourceVersion(resourceVersion)

			if err := client.Update(rc.Ctx, currentEndpoints); err != nil {
				logger.Error(err, "Unable to update endpoints for additional seed service",
					"endpoints", currentEndpoints)
				return result.Error(err)
			}
		}
	}

	if createNeeded {
		rc.Endpoints = desiredEndpoints
		return rc.CreateEndpointsForAdditionalSeedService()
	}

	return result.Continue()
}

func (rc *ReconciliationContext) GetAdditionalSeedEndpoint() (*discoveryv1.EndpointSlice, error) {
	dc := rc.Datacenter
	nsName := types.NamespacedName{Name: dc.GetAdditionalSeedsServiceName(), Namespace: dc.Namespace}
	currentEndpoints := &discoveryv1.EndpointSlice{}
	err := rc.Client.Get(rc.Ctx, nsName, currentEndpoints)
	return currentEndpoints, err
}
