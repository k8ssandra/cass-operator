// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"context"
	"testing"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/pkg/oplabels"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestCheckAdditionalSeedEndpointSlicesRemoveLegacyEndpoints(t *testing.T) {
	tt := []struct {
		name                             string
		legacyEndpointNames              []string
		metadataVersion                  int64
		isLegacyEndpointsRemovalExpected bool
	}{
		{
			name:                             "metadata version is below 2",
			isLegacyEndpointsRemovalExpected: true,
			metadataVersion:                  1,
		},
		{
			name:                             "metadata version is equal or greater than 2",
			isLegacyEndpointsRemovalExpected: false,
			metadataVersion:                  2,
		},
	}

	//nolint:staticcheck // Intentionally test migration from the deprecated API.
	makeLegacyEndpoint := func(dc *api.CassandraDatacenter, name string) *corev1.Endpoints {
		labels := dc.GetDatacenterLabels()
		labels[oplabels.ManagedByLabel] = oplabels.ManagedByLabelValue
		//nolint:staticcheck // Intentionally test migration from the deprecated API.
		return &corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: dc.Namespace,
				Labels:    labels,
			},
		}
	}
	for _, tc := range tt {
		legacyEndpointNames := []string{"endpoint1", "endpoint2"}
		t.Run(tc.name, func(t *testing.T) {
			rc, _, cleanupMockScr := setupTest()
			defer cleanupMockScr()
			rc.Datacenter.Status.MetadataVersion = tc.metadataVersion
			dc := rc.Datacenter
			runtimeObjs := []runtime.Object{dc}
			for _, epName := range legacyEndpointNames {
				runtimeObjs = append(runtimeObjs, makeLegacyEndpoint(dc, epName))
			}
			var endpointListCalled bool
			var endpointDeleteCalled bool
			var deletedLegacyEndpointNames []string
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
					Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						endpointDeleteCalled = true
						deletedLegacyEndpointNames = append(deletedLegacyEndpointNames, obj.GetName())
						return c.Delete(ctx, obj, opts...)
					},
				}).
				Build()
			rc.Client = fakeClient

			res := rc.CheckAdditionalSeedEndpointSlices()

			assert.False(t, res.Completed())
			if tc.isLegacyEndpointsRemovalExpected {
				assert.True(t, endpointListCalled)
				assert.True(t, endpointDeleteCalled)
				assert.Equal(t, deletedLegacyEndpointNames, legacyEndpointNames)
			} else {
				assert.False(t, endpointListCalled)
				assert.False(t, endpointDeleteCalled)
			}
		})
	}
}
