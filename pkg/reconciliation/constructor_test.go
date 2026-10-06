package reconciliation

import (
	"context"
	"testing"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestPodDisruptionBudget(t *testing.T) {
	assert := assert.New(t)

	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dc1",
			Namespace: "test",
		},
		Spec: api.CassandraDatacenterSpec{
			DatacenterName: "dc1-override",
			Size:           3,
		},
	}

	// create a PodDisruptionBudget object
	pdb := newPodDisruptionBudgetForDatacenter(dc)
	assert.Equal("dc1-pdb", pdb.Name)
	assert.Equal("test", pdb.Namespace)
	assert.Equal("dc1", pdb.Spec.Selector.MatchLabels["cassandra.datastax.com/datacenter"])
	assert.Equal(pdb.Spec.MinAvailable.IntVal, dc.Spec.Size-1)
}

func TestPodDisruptionBudgetIntMaxUnavailable(t *testing.T) {
	assert := assert.New(t)

	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dc1",
			Namespace: "test",
		},
		Spec: api.CassandraDatacenterSpec{
			Size:           6,
			MaxUnavailable: new(intstr.FromInt(2)),
		},
	}

	pdb := newPodDisruptionBudgetForDatacenter(dc)
	assert.Equal(int32(4), pdb.Spec.MinAvailable.IntVal)
}

func TestPodDisruptionBudgetPercentageMaxUnavailable(t *testing.T) {
	assert := assert.New(t)

	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dc1",
			Namespace: "test",
		},
		Spec: api.CassandraDatacenterSpec{
			Size: 6,
			Racks: []api.Rack{
				{Name: "rack1"},
				{Name: "rack2"},
			},
			MaxUnavailable: new(intstr.Parse("50%")),
		},
	}

	pdb := newPodDisruptionBudgetForDatacenter(dc)
	assert.Equal(int32(4), pdb.Spec.MinAvailable.IntVal) // This was roundup

	dc.Spec.MaxUnavailable = new(intstr.Parse("100%"))
	pdb = newPodDisruptionBudgetForDatacenter(dc)
	assert.Equal(int32(3), pdb.Spec.MinAvailable.IntVal)
}

func TestSetDatacenterStatusUpdateProgressState(t *testing.T) {
	tt := []struct {
		name                 string
		inputMetadataVersion int64
		inputObservedGen     int64
		inputGeneration      int64

		expectedStatusPatch     bool
		expectedMetadataVersion int64
		expectedObservedGen     int64
	}{
		{
			name:                 "migration pending, generation current: bumps MetadataVersion only",
			inputMetadataVersion: 1,
			inputObservedGen:     3,
			inputGeneration:      3,

			expectedStatusPatch:     true,
			expectedMetadataVersion: 2,
			expectedObservedGen:     3,
		},
		{
			name:                 "migration pending, spec changed: bumps MetadataVersion and ObservedGeneration",
			inputMetadataVersion: 1,
			inputObservedGen:     0,
			inputGeneration:      1,

			expectedStatusPatch:     true,
			expectedMetadataVersion: 2,
			expectedObservedGen:     1,
		},
		{
			name:                 "migration done, spec changed: bumps ObservedGeneration only",
			inputMetadataVersion: 2,
			inputObservedGen:     1,
			inputGeneration:      2,

			expectedStatusPatch:     true,
			expectedMetadataVersion: 2,
			expectedObservedGen:     2,
		},
		{
			name:                 "migration done, generation current: skips status patch",
			inputMetadataVersion: 2,
			inputObservedGen:     3,
			inputGeneration:      3,

			expectedStatusPatch:     false,
			expectedMetadataVersion: 2,
			expectedObservedGen:     3,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			rc, _, cleanupMockScr := setupTest()
			defer cleanupMockScr()

			rc.Datacenter.Status.MetadataVersion = tc.inputMetadataVersion
			rc.Datacenter.Status.ObservedGeneration = tc.inputObservedGen
			rc.Datacenter.Generation = tc.inputGeneration

			dc := rc.Datacenter
			runtimeObjs := []runtime.Object{dc}

			var statusPatchCalled bool
			fakeClient := fake.NewClientBuilder().
				WithScheme(setupScheme()).
				WithStatusSubresource(dc).
				WithRuntimeObjects(runtimeObjs...).
				WithInterceptorFuncs(interceptor.Funcs{
					SubResourcePatch: func(
						ctx context.Context,
						c client.Client,
						subResourceName string,
						obj client.Object,
						patch client.Patch,
						opts ...client.SubResourcePatchOption,
					) error {
						if subResourceName == "status" {
							statusPatchCalled = true
						}
						return c.SubResource(subResourceName).Patch(ctx, obj, patch, opts...)
					},
				}).
				Build()
			rc.Client = fakeClient

			err := setDatacenterStatus(rc)

			assert.NoError(t, err)
			assert.Equal(t, tc.expectedStatusPatch, statusPatchCalled)
			assert.Equal(t, tc.expectedMetadataVersion, rc.Datacenter.Status.MetadataVersion)
			assert.Equal(t, tc.expectedObservedGen, rc.Datacenter.Status.ObservedGeneration)
		})
	}
}
