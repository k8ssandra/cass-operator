// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"context"
	"fmt"
	"maps"
	"testing"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/k8ssandra/cass-operator/pkg/oplabels"
	"github.com/k8ssandra/cass-operator/pkg/utils"
)

func TestReconcileHeadlessService(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	recResult := rc.CheckHeadlessServices()
	assert.False(t, recResult.Completed(), "Reconcile loop should not be completed")
}

func TestReconcileHeadlessService_UpdateLabelsAndAnnotations(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	recResult := rc.CheckHeadlessServices()
	assert.False(t, recResult.Completed(), "Reconcile loop should not be completed")

	dcSvcName := rc.Datacenter.GetDatacenterServiceName()
	dcSvc := &corev1.Service{}
	err := rc.Client.Get(rc.Ctx, types.NamespacedName{Name: dcSvcName, Namespace: rc.Datacenter.Namespace}, dcSvc)
	assert.NoError(t, err)

	dcSvcLabels := dcSvc.GetLabels()
	dcSvcLabels["AddKey1"] = "Value1"
	dcSvcLabels["AddKey2"] = "Value2"
	dcSvc.SetLabels(dcSvcLabels)

	dcSvcAnnotations := dcSvc.GetAnnotations()
	dcSvcAnnotations["AddAnnotation1"] = "AddValue1"
	dcSvcAnnotations["AddAnnotation2"] = "AddValue2"
	dcSvc.SetAnnotations(dcSvcAnnotations)
	assert.NoError(t, rc.Client.Update(rc.Ctx, dcSvc))

	rc.Datacenter.Spec.AdditionalServiceConfig.DatacenterService.Labels = map[string]string{"AddKey1": "ChangeValue1", "AddKey3": "Value3"}
	updatedDcSvcLabels := maps.Clone(dcSvcLabels)
	delete(updatedDcSvcLabels, "AddKey2")
	updatedDcSvcLabels["AddKey1"] = "ChangeValue1"
	updatedDcSvcLabels["AddKey3"] = "Value3"

	rc.Datacenter.Spec.AdditionalServiceConfig.DatacenterService.Annotations = map[string]string{"AddAnnotation1": "ChangeAnnotation1", "AddAnnotation3": "AddValue3"}
	updatedDcSvcAnnotations := maps.Clone(dcSvcAnnotations)
	delete(updatedDcSvcAnnotations, "AddAnnotation2")
	updatedDcSvcAnnotations["AddAnnotation1"] = "ChangeAnnotation1"
	updatedDcSvcAnnotations["AddAnnotation3"] = "AddValue3"
	delete(updatedDcSvcAnnotations, utils.ResourceHashAnnotationKey)

	recResult = rc.CheckHeadlessServices()
	assert.False(t, recResult.Completed(), "Reconcile loop should not be completed")

	updatedSvc := &corev1.Service{}
	err = rc.Client.Get(rc.Ctx, types.NamespacedName{Name: dcSvcName, Namespace: rc.Datacenter.Namespace}, updatedSvc)
	assert.NoError(t, err)
	assert.Equal(t, updatedDcSvcLabels, updatedSvc.GetLabels())

	observedAnnotations := updatedSvc.GetAnnotations()
	delete(observedAnnotations, utils.ResourceHashAnnotationKey)
	assert.Equal(t, updatedDcSvcAnnotations, observedAnnotations)
}

func TestCreateHeadlessService(t *testing.T) {
	rc, svc, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	rc.Services = []*corev1.Service{svc}

	recResult := rc.CreateHeadlessServices()

	// kind of weird to check this path we don't want in a test, but
	// it's useful to see what the error is
	if recResult.Completed() {
		_, err := recResult.Output()
		assert.NoErrorf(t, err, "Should not have returned an error")
	}

	assert.False(t, recResult.Completed(), "Reconcile loop should not be completed")
}

func TestCreateHeadlessService_ClientReturnsError(t *testing.T) {
	rc, svc, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	rc.Client = fake.NewClientBuilder().
		WithScheme(setupScheme()).
		WithStatusSubresource(rc.Datacenter).
		WithRuntimeObjects(rc.Datacenter).
		WithIndex(&corev1.Pod{}, podPVCClaimNameField, podPVCClaimNames).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*corev1.Service); ok {
					return fmt.Errorf("")
				}
				return c.Create(ctx, obj, opts...)
			},
		}).
		Build()

	rc.Services = []*corev1.Service{svc}

	recResult := rc.CreateHeadlessServices()

	assert.True(t, recResult.Completed(), "Reconcile loop should be completed")
	_, err := recResult.Output()
	assert.Error(t, err, "Should have returned the service creation error")
}

func TestEndpointSliceControllerIntegration(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	fakeClient := fake.NewClientBuilder().
		WithScheme(setupScheme()).
		WithStatusSubresource(rc.Datacenter).
		WithRuntimeObjects(rc.Datacenter).
		Build()

	rc.Client = fakeClient
	rc.Datacenter.Spec.AdditionalSeeds = []string{
		"192.168.1.1",      // IPv4
		"2001:db8::1",      // IPv6
		"seed.example.com", // FQDN
	}

	dc := rc.Datacenter

	// err := fakeClient.Create(context.Background(), dc)
	// assert.NoError(t, err)

	additionalSvc := newAdditionalSeedServiceForCassandraDatacenter(dc)
	err := fakeClient.Create(context.Background(), additionalSvc)
	assert.NoError(t, err)

	endpointSlices := newEndpointSlicesForAdditionalSeeds(dc)
	assert.Equal(t, 3, len(endpointSlices))

	for _, slice := range endpointSlices {
		err = fakeClient.Create(context.Background(), slice)
		assert.NoError(t, err)
	}

	res := rc.CheckAdditionalSeedEndpointSlices()
	assert.False(t, res.Completed())

	sliceList := &discoveryv1.EndpointSliceList{}
	err = fakeClient.List(context.Background(), sliceList,
		client.InNamespace("default"),
		client.MatchingLabels{discoveryv1.LabelServiceName: dc.GetAdditionalSeedsServiceName()})
	assert.NoError(t, err)
	assert.Equal(t, 3, len(sliceList.Items))

	addressTypeCounts := map[discoveryv1.AddressType]int{
		discoveryv1.AddressTypeIPv4: 0,
		discoveryv1.AddressTypeIPv6: 0,
		discoveryv1.AddressTypeFQDN: 0,
	}

	for _, slice := range sliceList.Items {
		assert.Equal(t, dc.GetAdditionalSeedsServiceName(),
			slice.Labels[discoveryv1.LabelServiceName])

		addressTypeCounts[slice.AddressType]++

		switch slice.AddressType {
		case discoveryv1.AddressTypeIPv4:
			assert.Equal(t, "192.168.1.1", slice.Endpoints[0].Addresses[0])
		case discoveryv1.AddressTypeIPv6:
			assert.Equal(t, "2001:db8::1", slice.Endpoints[0].Addresses[0])
		case discoveryv1.AddressTypeFQDN:
			assert.Equal(t, "seed.example.com", slice.Endpoints[0].Addresses[0])
		}
	}

	assert.Equal(t, 1, addressTypeCounts[discoveryv1.AddressTypeIPv4])
	assert.Equal(t, 1, addressTypeCounts[discoveryv1.AddressTypeIPv6])
	assert.Equal(t, 1, addressTypeCounts[discoveryv1.AddressTypeFQDN])
}

func TestCheckAdditionalSeedEndpointSlicesLegacyEndpointCleanup(t *testing.T) {
	makeLegacyEndpoint := func(dc *api.CassandraDatacenter, name string) *corev1.Endpoints {
		labels := dc.GetDatacenterLabels()
		labels[oplabels.ManagedByLabel] = oplabels.ManagedByLabelValue
		return &corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: dc.Namespace,
				Labels:    labels,
			},
		}
	}

	tests := []struct {
		name                   string
		metadataVersion        int64
		legacyEndpointNames    []string
		wantEndpointListCalled bool
		wantRemainingEndpoints int
		wantMetadataVersion    int64
	}{
		{
			name:                   "legacy endpoints are deleted and metadataVersion set to 2 when metadataVersion is 0",
			metadataVersion:        0,
			legacyEndpointNames:    []string{"legacy-ep-1", "legacy-ep-2"},
			wantEndpointListCalled: true,
			wantRemainingEndpoints: 0,
			wantMetadataVersion:    2,
		},
		{
			name:                   "metadataVersion 1 advances even when no legacy endpoints remain",
			metadataVersion:        1,
			wantEndpointListCalled: true,
			wantRemainingEndpoints: 0,
			wantMetadataVersion:    2,
		},
		{
			name:                   "legacy endpoints are not touched when metadataVersion is already 2",
			metadataVersion:        2,
			legacyEndpointNames:    []string{"legacy-ep-1"},
			wantEndpointListCalled: false,
			wantRemainingEndpoints: 1,
			wantMetadataVersion:    2,
		},
		{
			name:                   "a future metadataVersion is preserved",
			metadataVersion:        3,
			legacyEndpointNames:    []string{"legacy-ep-1"},
			wantEndpointListCalled: false,
			wantRemainingEndpoints: 1,
			wantMetadataVersion:    3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc, _, cleanupMockScr := setupTest()
			defer cleanupMockScr()

			rc.Datacenter.Spec.AdditionalSeeds = []string{"192.168.1.1"}
			rc.Datacenter.Status.MetadataVersion = tt.metadataVersion
			dc := rc.Datacenter
			dc.Status.ObservedGeneration = dc.Generation
			if tt.metadataVersion == 0 {
				dc.Spec.DatacenterName = "old-dc-name"
				dc.Status.DatacenterName = new("old-dc-name")
				rc.clusterPods = []*corev1.Pod{{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
						api.ClusterLabel:    api.CleanLabelValue(dc.Spec.ClusterName),
						api.DatacenterLabel: api.CleanLabelValue(dc.Spec.DatacenterName),
					}},
				}}
			}

			runtimeObjs := []runtime.Object{dc}
			for _, epName := range tt.legacyEndpointNames {
				runtimeObjs = append(runtimeObjs, makeLegacyEndpoint(dc, epName))
			}

			endpointListCalled := false
			fakeClient := fake.NewClientBuilder().
				WithScheme(setupScheme()).
				WithStatusSubresource(dc).
				WithRuntimeObjects(runtimeObjs...).
				WithInterceptorFuncs(interceptor.Funcs{
					List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
						if _, ok := list.(*corev1.EndpointsList); ok {
							endpointListCalled = true
						}
						return c.List(ctx, list, opts...)
					},
				}).
				Build()
			rc.Client = fakeClient

			res := rc.CheckAdditionalSeedEndpointSlices()

			require.False(t, res.Completed())
			assert.Equal(t, tt.wantEndpointListCalled, endpointListCalled)
			epList := &corev1.EndpointsList{}
			require.NoError(t, fakeClient.List(rc.Ctx, epList, client.InNamespace(dc.Namespace)))
			assert.Len(t, epList.Items, tt.wantRemainingEndpoints)
			assert.EqualValues(t, tt.metadataVersion, rc.Datacenter.Status.MetadataVersion)
			wasCleanupExpected := tt.metadataVersion < 2
			assert.Equal(t, wasCleanupExpected, rc.legacyEndpointsCleanupCompleted)
			if tt.metadataVersion == 0 {
				assert.Len(t, rc.datacenterPods(), 1, "old-labeled pods must remain visible before status is advanced")
			}

			storedDC := &api.CassandraDatacenter{}
			dcKey := types.NamespacedName{Name: dc.Name, Namespace: dc.Namespace}
			require.NoError(t, fakeClient.Get(rc.Ctx, dcKey, storedDC))
			assert.EqualValues(t, tt.metadataVersion, storedDC.Status.MetadataVersion)

			require.NoError(t, setDatacenterStatus(rc))
			assert.EqualValues(t, tt.wantMetadataVersion, rc.Datacenter.Status.MetadataVersion)
			require.NoError(t, fakeClient.Get(rc.Ctx, dcKey, storedDC))
			assert.EqualValues(t, tt.wantMetadataVersion, storedDC.Status.MetadataVersion)
		})
	}
}

func TestAdditionalSeedEndpointSliceCleanupFailureDoesNotAdvanceMetadataVersion(t *testing.T) {
	for _, failure := range []string{"list", "delete"} {
		t.Run(failure, func(t *testing.T) {
			rc, _, cleanupMockScr := setupTest()
			defer cleanupMockScr()

			rc.Datacenter.Spec.AdditionalSeeds = []string{"192.168.1.1"}
			rc.Datacenter.Status.MetadataVersion = 1
			dc := rc.Datacenter
			labels := dc.GetDatacenterLabels()
			labels[oplabels.ManagedByLabel] = oplabels.ManagedByLabelValue
			legacyEndpoint := &corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{
				Name: "legacy-ep", Namespace: dc.Namespace, Labels: labels,
			}}

			fakeClient := fake.NewClientBuilder().
				WithScheme(setupScheme()).
				WithStatusSubresource(dc).
				WithRuntimeObjects(dc, legacyEndpoint).
				WithInterceptorFuncs(interceptor.Funcs{
					List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
						if _, ok := list.(*corev1.EndpointsList); ok && failure == "list" {
							return fmt.Errorf("legacy endpoints list failed")
						}
						return c.List(ctx, list, opts...)
					},
					Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						if _, ok := obj.(*corev1.Endpoints); ok && failure == "delete" {
							return fmt.Errorf("legacy endpoint deletion failed")
						}
						return c.Delete(ctx, obj, opts...)
					},
				}).
				Build()
			rc.Client = fakeClient

			res := rc.CheckAdditionalSeedEndpointSlices()
			require.True(t, res.Completed())
			_, err := res.Output()
			require.Error(t, err)
			assert.False(t, rc.legacyEndpointsCleanupCompleted)
			assert.EqualValues(t, 1, rc.Datacenter.Status.MetadataVersion)

			storedDC := &api.CassandraDatacenter{}
			dcKey := types.NamespacedName{Name: dc.Name, Namespace: dc.Namespace}
			require.NoError(t, fakeClient.Get(rc.Ctx, dcKey, storedDC))
			assert.EqualValues(t, 1, storedDC.Status.MetadataVersion)
		})
	}
}
