// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/internal/result"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/k8ssandra/cass-operator/pkg/utils"
)

func (rc *ReconciliationContext) CreateHeadlessServices() result.ReconcileResult {
	// unpacking
	logger := rc.ReqLogger
	client := rc.Client

	for idx := range rc.Services {
		service := rc.Services[idx]

		logger.Info(
			"Creating a new headless service",
			"serviceNamespace", service.Namespace,
			"serviceName", service.Name)

		if err := setOperatorProgressStatus(rc, api.ProgressUpdating); err != nil {
			return result.Error(err)
		}

		if err := client.Create(rc.Ctx, service); err != nil {
			logger.Error(err, "Could not create headless service")

			return result.Error(err)
		}

		rc.Recorder.Eventf(rc.Datacenter, "Normal", "CreatedResource", "Created service %s", service.Name)
	}

	// at this point we had previously been saying this reconcile call was over, we're done
	// but that seems wrong, we should just continue on to the next resources
	return result.Continue()
}

// ReconcileHeadlessService ...
func (rc *ReconciliationContext) CheckHeadlessServices() result.ReconcileResult {
	// unpacking
	logger := rc.ReqLogger
	dc := rc.Datacenter
	client := rc.Client

	logger.Info("reconcile_services::ReconcileHeadlessServices")

	// Check if there is a headless service for the cluster

	cqlService := newServiceForCassandraDatacenter(dc)
	seedService := newSeedServiceForCassandraDatacenter(dc)
	allPodsService := newAllPodsServiceForCassandraDatacenter(dc)
	additionalSeedService := newAdditionalSeedServiceForCassandraDatacenter(dc)

	services := []*corev1.Service{cqlService, seedService, allPodsService, additionalSeedService}

	if dc.IsNodePortEnabled() {
		nodePortService := newNodePortServiceForCassandraDatacenter(dc)
		services = append(services, nodePortService)
	}

	createNeeded := []*corev1.Service{}

	for idx := range services {
		desiredSvc := services[idx]

		if err := setControllerReference(dc, desiredSvc, rc.Scheme); err != nil {
			logger.Error(err, "Could not set controller reference for headless service")
			return result.Error(err)
		}

		// See if the service already exists
		nsName := types.NamespacedName{Name: desiredSvc.Name, Namespace: desiredSvc.Namespace}
		currentService := &corev1.Service{}
		if err := client.Get(rc.Ctx, nsName, currentService); err != nil && errors.IsNotFound(err) {
			// if it's not found, put the service in the slice to be created when Apply is called
			createNeeded = append(createNeeded, desiredSvc)
		} else if err != nil {
			// if we hit a k8s error, log it and error out
			logger.Error(err, "Could not get headless seed service",
				"name", nsName,
			)
			return result.Error(err)
		} else {
			if controllerutil.HasControllerReference(currentService) && currentService.GetOwnerReferences()[0].Kind == "CassandraDatacenter" && currentService.GetOwnerReferences()[0].Name != dc.GetName() {
				// Some other CassandraDatacenter owns this service, so we should ignore it
				logger.Info("Service is owned by another CassandraDatacenter, not updating it as part of this reconcile", "service", currentService, "datacenter", currentService.GetOwnerReferences()[0].Name)
				continue
			}

			// if we found the service already, check if they need updating
			if !utils.ResourcesHaveSameHash(currentService, desiredSvc) {
				resourceVersion := currentService.GetResourceVersion()

				// ClusterIP may have been updated for the NodePort service
				// so we need to preserve it.  Copying should not break any of
				// the other services either.
				desiredSvc.Spec.ClusterIP = currentService.Spec.ClusterIP

				logger.Info("Updating service",
					"service", currentService,
					"desired", desiredSvc)

				desiredSvc.DeepCopyInto(currentService)

				currentService.SetResourceVersion(resourceVersion)

				if err := client.Update(rc.Ctx, currentService); err != nil {
					logger.Error(err, "Unable to update service",
						"service", currentService)
					return result.Error(err)
				}
			}
		}
	}

	if len(createNeeded) > 0 {
		rc.Services = createNeeded
		return rc.CreateHeadlessServices()
	}

	if len(dc.Spec.AdditionalSeeds) > 0 {
		return rc.CheckAdditionalSeedEndpointSlices()
	}

	return result.Continue()
}
