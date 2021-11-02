/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/k8ssandra/cass-operator/pkg/images"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	datastaxPrefix  string = "cassandra.datastax.com"
	k8ssandraPrefix string = "k8ssandra.io"
)

var log = logf.Log.WithName("api")

func (dc *CassandraDatacenter) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(dc).
		Complete()
}

// kubebuilder:webhook:path=/mutate-cassandra-datastax-com-v1beta1-cassandradatacenter,mutating=true,failurePolicy=fail,sideEffects=None,groups=cassandra.datastax.com,resources=cassandradatacenters,verbs=create;update,versions=v1beta1,name=mcassandradatacenter.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:webhook:path=/validate-cassandra-datastax-com-v1beta1-cassandradatacenter,mutating=false,failurePolicy=fail,sideEffects=None,groups=cassandra.datastax.com,resources=cassandradatacenters,verbs=create;update,versions=v1beta1,name=vcassandradatacenter.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Defaulter = &CassandraDatacenter{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (dc *CassandraDatacenter) Default() {
	// No mutations at this point
}

func attemptedTo(action string, actionStrArgs ...interface{}) error {
	var msg string
	if actionStrArgs != nil {
		msg = fmt.Sprintf(action, actionStrArgs...)
	} else {
		msg = action
	}
	return fmt.Errorf("CassandraDatacenter write rejected, attempted to %s", msg)
}

// ValidateSingleDatacenter checks that no values are improperly set on a CassandraDatacenter
func ValidateSingleDatacenter(dc CassandraDatacenter) error {
	// Ensure serverVersion and serverType are compatible

	if dc.Spec.ServerType == "dse" {
		if !images.IsDseVersionSupported(dc.Spec.ServerVersion) {
			return attemptedTo("use unsupported DSE version '%s'", dc.Spec.ServerVersion)
		}
	}

	if dc.Spec.ServerType == "cassandra" && dc.Spec.DseWorkloads != nil {
		if dc.Spec.DseWorkloads.AnalyticsEnabled || dc.Spec.DseWorkloads.GraphEnabled || dc.Spec.DseWorkloads.SearchEnabled {
			return attemptedTo("enable DSE workloads if server type is Cassandra")
		}
	}

	if dc.Spec.ServerType == "cassandra" {
		if !images.IsOssVersionSupported(dc.Spec.ServerVersion) {
			return attemptedTo("use unsupported Cassandra version '%s'", dc.Spec.ServerVersion)
		}
	}

	isDse := dc.Spec.ServerType == "dse"
	isCassandra3 := dc.Spec.ServerType == "cassandra" && strings.HasPrefix(dc.Spec.ServerVersion, "3.")
	isCassandra4 := dc.Spec.ServerType == "cassandra" && strings.HasPrefix(dc.Spec.ServerVersion, "4.")

	var c map[string]interface{}
	_ = json.Unmarshal(dc.Spec.Config, &c)

	_, hasJvmOptions := c["jvm-options"]
	_, hasJvmServerOptions := c["jvm-server-options"]
	_, hasDseYaml := c["dse-yaml"]

	serverStr := fmt.Sprintf("%s-%s", dc.Spec.ServerType, dc.Spec.ServerVersion)
	if hasJvmOptions && (isDse || isCassandra4) {
		return attemptedTo("define config jvm-options with %s", serverStr)
	}
	if hasJvmServerOptions && isCassandra3 {
		return attemptedTo("define config jvm-server-options with %s", serverStr)
	}
	if hasDseYaml && (isCassandra3 || isCassandra4) {
		return attemptedTo("define config dse-yaml with %s", serverStr)
	}

	// if using multiple nodes per worker, requests and limits should be set for both cpu and memory
	if dc.Spec.AllowMultipleNodesPerWorker {
		if dc.Spec.Resources.Requests.Cpu().IsZero() ||
			dc.Spec.Resources.Limits.Cpu().IsZero() ||
			dc.Spec.Resources.Requests.Memory().IsZero() ||
			dc.Spec.Resources.Limits.Memory().IsZero() {

			return attemptedTo("use multiple nodes per worker without cpu and memory requests and limits")
		}
	}

	if err := ValidateServiceLabelsAndAnnotations(dc); err != nil {
		return err
	}

	if err := ValidateFQLConfig(dc); err != nil {
		return err
	}
	if !isDse {
		err := ValidateConfig(&dc)
		if err != nil {
			return err
		}
	}

	return nil
}

// ValidateDatacenterFieldChanges checks that no values are improperly changing while updating
// a CassandraDatacenter
func ValidateDatacenterFieldChanges(oldDc CassandraDatacenter, newDc CassandraDatacenter) error {

	if oldDc.Spec.ClusterName != newDc.Spec.ClusterName {
		return attemptedTo("change clusterName")
	}

	if oldDc.Spec.AllowMultipleNodesPerWorker != newDc.Spec.AllowMultipleNodesPerWorker {
		return attemptedTo("change allowMultipleNodesPerWorker")
	}

	if oldDc.Spec.SuperuserSecretName != newDc.Spec.SuperuserSecretName {
		return attemptedTo("change superuserSecretName")
	}

	if oldDc.Spec.ServiceAccount != newDc.Spec.ServiceAccount {
		return attemptedTo("change serviceAccount")
	}

	// StorageConfig changes are disallowed
	if !reflect.DeepEqual(oldDc.Spec.StorageConfig, newDc.Spec.StorageConfig) {
		return attemptedTo("change storageConfig")
	}

	// Topology changes - Racks
	// - Rack Name and Zone changes are disallowed.
	// - Removing racks is not supported.
	// - Reordering the rack list is not supported.
	// - Any new racks must be added to the end of the current rack list.

	oldRacks := oldDc.GetRacks()
	newRacks := newDc.GetRacks()

	if len(oldRacks) > len(newRacks) {
		return attemptedTo("remove rack")
	}

	newRackCount := len(newRacks) - len(oldRacks)
	if newRackCount > 0 {
		newSizeDifference := newDc.Spec.Size - oldDc.Spec.Size
		oldRackNodeSplit := SplitRacks(int(oldDc.Spec.Size), len(oldRacks))
		minNodesFromOldRacks := oldRackNodeSplit[len(oldRackNodeSplit)-1]
		minSizeAdjustment := minNodesFromOldRacks * newRackCount

		if newSizeDifference <= 0 {
			return attemptedTo("add rack without increasing size")
		}

		if int(newSizeDifference) < minSizeAdjustment {
			return attemptedTo(
				fmt.Sprintf("add racks without increasing size enough to prevent existing"+
					" nodes from moving to new racks to maintain balance.\n"+
					"New racks added: %d, size increased by: %d. Expected size increase to be at least %d",
					newRackCount, newSizeDifference, minSizeAdjustment))
		}
	}

	for index, oldRack := range oldRacks {
		newRack := newRacks[index]
		if oldRack.Name != newRack.Name {
			return attemptedTo("change rack name from '%s' to '%s'",
				oldRack.Name,
				newRack.Name)
		}
		if oldRack.Zone != newRack.Zone {
			return attemptedTo("change rack zone from '%s' to '%s'",
				oldRack.Zone,
				newRack.Zone)
		}
	}

	return nil
}

// +kubebuilder:webhook:path=/validate-cassandra-datastax-com-v1beta1-cassandradatacenter,mutating=false,failurePolicy=ignore,sideEffects=None,groups=cassandra.datastax.com,resources=cassandradatacenters,verbs=create;update,versions=v1beta1,name=vcassandradatacenter.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:webhook:path=/validate-cassandradatacenter,mutating=false,failurePolicy=ignore,groups=cassandra.datastax.com,resources=cassandradatacenters,verbs=create;update,versions=v1beta1,name=validate-cassandradatacenter-webhook
var _ webhook.Validator = &CassandraDatacenter{}

func (dc *CassandraDatacenter) ValidateCreate() error {
	log.Info("Validating webhook called for create")
	err := ValidateSingleDatacenter(*dc)
	if err != nil {
		return err
	}

	return nil
}

func (dc *CassandraDatacenter) ValidateUpdate(old runtime.Object) error {
	log.Info("Validating webhook called for update")
	oldDc, ok := old.(*CassandraDatacenter)
	if !ok {
		return errors.New("old object in ValidateUpdate cannot be cast to CassandraDatacenter")
	}

	err := ValidateSingleDatacenter(*dc)
	if err != nil {
		return err
	}

	return ValidateDatacenterFieldChanges(*oldDc, *dc)
}

func (dc *CassandraDatacenter) ValidateDelete() error {
	return nil
}

var (
	ErrFQLNotSupported = fmt.Errorf("full query logging is only supported on OSS Cassandra 4.0+")
)

func ValidateConfig(dc *CassandraDatacenter) error {
	// TODO Cleanup to more common processing after ModelValues is moved to apis
	if dc.Spec.Config != nil {
		var dcConfig map[string]interface{}
		if err := json.Unmarshal(dc.Spec.Config, &dcConfig); err != nil {
			return err
		}
		casYaml, found := dcConfig["cassandra-yaml"]
		if !found {
			return nil
		}

		casYamlMap, ok := casYaml.(map[string]interface{})
		if !ok {
			err := fmt.Errorf("failed to parse cassandra-yaml")
			return err
		}

		configValues, err := GetCassandraConfigValues(dc.Spec.ServerVersion)
		if err != nil {
			return err
		}
		for k := range casYamlMap {
			if !configValues.HasProperty(k) {
				// We should probably add an event to tell the user that they're using old values
				return fmt.Errorf("property %s is not valid for serverVersion %s", k, dc.Spec.ServerVersion)
			}
		}
	}
	return nil
}

func ValidateFQLConfig(dc CassandraDatacenter) error {
	if dc.Spec.Config != nil {
		enabled, err := dc.FullQueryEnabled()
		if err != nil {
			return err
		}

		if enabled && !dc.DeploymentSupportsFQL() {
			return ErrFQLNotSupported
		}
	}

	return nil
}

func ValidateServiceLabelsAndAnnotations(dc CassandraDatacenter) error {
	// check each service
	addSeedSvc := dc.Spec.AdditionalServiceConfig.AdditionalSeedService
	allPodsSvc := dc.Spec.AdditionalServiceConfig.AllPodsService
	dcSvc := dc.Spec.AdditionalServiceConfig.DatacenterService
	nodePortSvc := dc.Spec.AdditionalServiceConfig.NodePortService
	seedSvc := dc.Spec.AdditionalServiceConfig.SeedService

	services := map[string]ServiceConfigAdditions{
		"AdditionalSeedService": addSeedSvc,
		"AllPOdsService":        allPodsSvc,
		"DatacenterService":     dcSvc,
		"NodePOrtService":       nodePortSvc,
		"SeedService":           seedSvc,
	}

	for svcName, config := range services {
		if containsReservedAnnotations(config) || containsReservedLabels(config) {
			return attemptedTo(fmt.Sprintf("configure %s with reserved annotations and/or labels (prefixes %s and/or %s)", svcName, datastaxPrefix, k8ssandraPrefix))
		}
	}

	return nil
}

func containsReservedAnnotations(config ServiceConfigAdditions) bool {
	return containsReservedPrefixes(config.Annotations)
}

func containsReservedLabels(config ServiceConfigAdditions) bool {
	return containsReservedPrefixes(config.Labels)
}

func containsReservedPrefixes(config map[string]string) bool {
	for k := range config {
		if strings.HasPrefix(k, datastaxPrefix) || strings.HasPrefix(k, k8ssandraPrefix) {
			// reserved prefix found
			return true
		}
	}
	return false
}
