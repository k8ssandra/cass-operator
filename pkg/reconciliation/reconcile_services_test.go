// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"context"
	"fmt"
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/k8ssandra/cass-operator/pkg/utils"
	discoveryv1 "k8s.io/api/discovery/v1"
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
		WithScheme(setupScheme(nil)).
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

	fakeClient := fake.NewClientBuilder().WithScheme(setupScheme(nil)).WithRuntimeObjects(rc.Datacenter).Build()

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
