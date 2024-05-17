// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

// This file defines constructors for k8s statefulset-related objects

import (
	"fmt"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/pkg/httphelper"
	"github.com/k8ssandra/cass-operator/pkg/oplabels"
	"github.com/k8ssandra/cass-operator/pkg/utils"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const zoneLabel = "failure-domain.beta.kubernetes.io/zone"

func newNamespacedNameForStatefulSet(
	dc *api.CassandraDatacenter,
	rackName string) types.NamespacedName {

	name := api.CleanupForKubernetes(dc.Spec.ClusterName) + "-" + dc.SanitizedName() + "-" + api.CleanupSubdomain(rackName) + "-sts"
	ns := dc.Namespace

	return types.NamespacedName{
		Name:      name,
		Namespace: ns,
	}
}

func rackNodeAffinitylabels(dc *api.CassandraDatacenter, rackName string) (map[string]string, error) {
	var nodeAffinityLabels map[string]string
	var log = logf.Log.WithName("construct_statefulset")
	racks := dc.GetRacks()
	for _, rack := range racks {
		if rack.Name == rackName {
			nodeAffinityLabels = utils.MergeMap(emptyMapIfNil(dc.Spec.NodeAffinityLabels),
				emptyMapIfNil(rack.NodeAffinityLabels))
			if rack.DeprecatedZone != "" {
				if _, found := nodeAffinityLabels[zoneLabel]; found {
					log.Error(nil,
						"Deprecated parameter Zone is used and also defined in NodeAffinityLabels. "+
							"You should only define it in NodeAffinityLabels")
				}
				nodeAffinityLabels = utils.MergeMap(
					emptyMapIfNil(nodeAffinityLabels), map[string]string{zoneLabel: rack.DeprecatedZone},
				)
			}
			break
		}
	}
	return nodeAffinityLabels, nil
}

// Create a statefulset object for the Datacenter.
func newStatefulSetForCassandraDatacenter(
	sts *appsv1.StatefulSet,
	rackName string,
	dc *api.CassandraDatacenter,
	replicaCount int) (*appsv1.StatefulSet, error) {

	replicaCountInt32 := int32(replicaCount)

	// see https://github.com/kubernetes/kubernetes/pull/74941
	// pvc labels are ignored before k8s 1.15.0
	pvcLabels := dc.GetRackLabels(rackName)
	oplabels.AddOperatorLabels(pvcLabels, dc)

	statefulSetLabels := dc.GetRackLabels(rackName)
	oplabels.AddOperatorLabels(statefulSetLabels, dc)

	statefulSetSelectorLabels := dc.GetRackLabels(rackName)

	anns := make(map[string]string)
	oplabels.AddOperatorAnnotations(anns, dc)

	var volumeClaimTemplates []corev1.PersistentVolumeClaim

	rack := dc.GetRack(rackName)

	// Add storage
	if dc.Spec.StorageConfig.CassandraDataVolumeClaimSpec == nil {
		err := fmt.Errorf("StorageConfig.cassandraDataVolumeClaimSpec is required")
		return nil, err
	}

	volumeClaimTemplates = []corev1.PersistentVolumeClaim{{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      pvcLabels,
			Name:        PvcName,
			Annotations: anns,
		},
		Spec: *dc.Spec.StorageConfig.CassandraDataVolumeClaimSpec,
	}}

	for _, storage := range dc.Spec.StorageConfig.AdditionalVolumes {
		if storage.PVCSpec != nil {
			pvc := corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:        storage.Name,
					Labels:      pvcLabels,
					Annotations: anns,
				},
				Spec: *storage.PVCSpec,
			}

			volumeClaimTemplates = append(volumeClaimTemplates, pvc)
		}
	}

	nsName := newNamespacedNameForStatefulSet(dc, rackName)

	template, err := buildPodTemplateSpec(dc, rack, legacyInternodeMount(dc, sts))
	if err != nil {
		return nil, err
	}

	// if the dc.Spec has a nodeSelector map, copy it into each sts pod template
	if len(dc.Spec.NodeSelector) > 0 {
		template.Spec.NodeSelector = utils.MergeMap(map[string]string{}, dc.Spec.NodeSelector)
	}

	_ = httphelper.AddManagementApiServerSecurity(dc, template)

	result := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        nsName.Name,
			Namespace:   nsName.Namespace,
			Labels:      statefulSetLabels,
			Annotations: anns,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: statefulSetSelectorLabels,
			},
			Replicas:             &replicaCountInt32,
			ServiceName:          dc.GetAllPodsServiceName(),
			PodManagementPolicy:  appsv1.ParallelPodManagement,
			Template:             *template,
			VolumeClaimTemplates: volumeClaimTemplates,
		},
	}

	if sts != nil && sts.Spec.ServiceName != "" && sts.Spec.ServiceName != result.Spec.ServiceName {
		result.Spec.ServiceName = sts.Spec.ServiceName
	}

	if dc.Spec.CanaryUpgrade {
		var partition int32
		if dc.Spec.CanaryUpgradeCount == 0 || dc.Spec.CanaryUpgradeCount > replicaCountInt32 {
			partition = replicaCountInt32
		} else {
			partition = replicaCountInt32 - dc.Spec.CanaryUpgradeCount
		}

		strategy := appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.RollingUpdateStatefulSetStrategyType,
			RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
				Partition: &partition,
			},
		}
		result.Spec.UpdateStrategy = strategy
	}

	// add a hash here to facilitate checking if updates are needed
	utils.AddHashAnnotation(result)

	return result, nil
}

func legacyInternodeMount(dc *api.CassandraDatacenter, sts *appsv1.StatefulSet) bool {
	if dc.LegacyInternodeEnabled() {
		return true
	}

	if sts != nil {
		for _, vol := range sts.Spec.Template.Spec.Volumes {
			if vol.Name == "encryption-cred-storage" {
				return true
			}
		}
	}

	return false
}
