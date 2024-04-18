// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/pkg/dynamicwatch"
	"github.com/k8ssandra/cass-operator/pkg/mocks"
)

func TestCalculateReconciliationActions(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	result, err := rc.CalculateReconciliationActions()
	assert.NoErrorf(t, err, "Should not have returned an error while calculating reconciliation actions")
	assert.NotNil(t, result, "Result should not be nil")

	// Add a service and check the logic

	fakeClient, _ := fakeClientWithService(rc.Datacenter)
	rc.Client = *fakeClient

	result, err = rc.CalculateReconciliationActions()
	assert.NoErrorf(t, err, "Should not have returned an error while calculating reconciliation actions")
	assert.NotNil(t, result, "Result should not be nil")
}

func TestCalculateReconciliationActions_GetServiceError(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	mockClient := mocks.NewClient(t)
	rc.Client = mockClient

	k8sMockClientGet(mockClient, fmt.Errorf(""))
	k8sMockClientUpdate(mockClient, nil).Times(1)

	_, err := rc.CalculateReconciliationActions()
	assert.Errorf(t, err, "Should have returned an error while calculating reconciliation actions")

	mockClient.AssertExpectations(t)
}

func TestCalculateReconciliationActions_FailedUpdate(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	mockClient := mocks.NewClient(t)
	rc.Client = mockClient

	k8sMockClientUpdate(mockClient, fmt.Errorf("failed to update CassandraDatacenter with removed finalizers"))

	_, err := rc.CalculateReconciliationActions()
	assert.Errorf(t, err, "Should have returned an error while calculating reconciliation actions")

	mockClient.AssertExpectations(t)
}

func emptySecretWatcher(t *testing.T, rc *ReconciliationContext) {
	mockClient := mocks.NewClient(t)
	rc.SecretWatches = dynamicwatch.NewDynamicSecretWatches(mockClient)
	k8sMockClientList(mockClient, nil).
		Run(func(args mock.Arguments) {
			arg := args.Get(1).(*unstructured.UnstructuredList)
			arg.Items = []unstructured.Unstructured{}
		})
}

// TestProcessDeletion_FailedDelete fails one step of the deletion process and should not cause
// the removal of the finalizer, instead controller-runtime will return an error and retry later
func TestProcessDeletion_FailedDelete(t *testing.T) {
	assert := assert.New(t)
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	mockClient := mocks.NewClient(t)
	rc.Client = mockClient

	k8sMockClientList(mockClient, nil).
		Run(func(args mock.Arguments) {
			arg := args.Get(1).(*v1.PersistentVolumeClaimList)
			arg.Items = []v1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pvc-1",
				},
			}}
		})

	k8sMockClientDelete(mockClient, fmt.Errorf(""))

	emptySecretWatcher(t, rc)

	k8sMockClientStatusPatch(mockClient.Status().(*mocks.SubResourceClient), nil) // Update dc status

	rc.Datacenter.SetFinalizers([]string{"finalizer.cassandra.datastax.com"})
	now := metav1.Now()
	rc.Datacenter.SetDeletionTimestamp(&now)

	result, err := rc.CalculateReconciliationActions()
	assert.Errorf(err, "Should have returned an error while calculating reconciliation actions")
	assert.Equal(reconcile.Result{}, result, "Should not requeue request as error does cause requeue")
	assert.True(len(rc.Datacenter.GetFinalizers()) > 0)

	mockClient.AssertExpectations(t)
}

// TestProcessDeletion verifies the correct amount of calls to k8sClient on the deletion process
func TestProcessDeletion(t *testing.T) {
	assert := assert.New(t)
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	mockClient := mocks.NewClient(t)
	rc.Client = mockClient

	k8sMockClientList(mockClient, nil).
		Run(func(args mock.Arguments) {
			arg := args.Get(1).(*v1.PersistentVolumeClaimList)
			arg.Items = []v1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pvc-1",
				},
			}}
		}) // ListPods

	k8sMockClientDelete(mockClient, nil) // Delete PVC
	k8sMockClientUpdate(mockClient, nil) // Remove dc finalizer

	emptySecretWatcher(t, rc)

	k8sMockClientStatusPatch(mockClient.Status().(*mocks.SubResourceClient), nil) // Update dc status

	rc.Datacenter.SetFinalizers([]string{"finalizer.cassandra.datastax.com"})
	now := metav1.Now()
	rc.Datacenter.SetDeletionTimestamp(&now)

	result, err := rc.CalculateReconciliationActions()
	assert.NoError(err)
	assert.Equal(reconcile.Result{}, result, "Should not requeue request")

	mockClient.AssertExpectations(t)
}

// TestProcessDeletion_NoFinalizer verifies that the removal of finalizer means cass-operator will do nothing
// on the deletion process.
func TestProcessDeletion_NoFinalizer(t *testing.T) {
	assert := assert.New(t)
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	mockClient := mocks.NewClient(t)
	rc.Client = mockClient

	now := metav1.Now()
	rc.Datacenter.SetDeletionTimestamp(&now)

	result, err := rc.CalculateReconciliationActions()
	assert.NoError(err)
	assert.Equal(reconcile.Result{Requeue: false}, result, "Should not requeue request")

	mockClient.AssertExpectations(t)
}

func TestAddFinalizer(t *testing.T) {
	assert := assert.New(t)
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	mockClient := mocks.NewClient(t)
	rc.Client = mockClient
	k8sMockClientUpdate(mockClient, nil).Times(1) // Add finalizer

	err := rc.addFinalizer()
	assert.NoError(err)
	assert.True(controllerutil.ContainsFinalizer(rc.Datacenter, api.Finalizer))
	mockClient.AssertExpectations(t)

	// This should not add the finalizer again
	mockClient = mocks.NewClient(t)
	rc.Client = mockClient
	rc.Datacenter.Annotations = make(map[string]string)
	rc.Datacenter.Annotations[api.NoFinalizerAnnotation] = "true"
	controllerutil.RemoveFinalizer(rc.Datacenter, api.Finalizer)
	err = rc.addFinalizer()
	assert.NoError(err)
	assert.False(controllerutil.ContainsFinalizer(rc.Datacenter, api.Finalizer))
}

func TestConflictingDcNameOverride(t *testing.T) {
	assert := assert.New(t)
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	mockClient := mocks.NewClient(t)
	rc.Client = mockClient

	k8sMockClientList(mockClient, nil).
		Run(func(args mock.Arguments) {
			arg := args.Get(1).(*api.CassandraDatacenterList)
			arg.Items = []api.CassandraDatacenter{{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dc1",
				},
				Spec: api.CassandraDatacenterSpec{
					ClusterName:    "cluster1",
					DatacenterName: "CassandraDatacenter_example",
				}}}
		})

	errs := rc.validateDatacenterNameConflicts()
	assert.NotEmpty(errs, "validateDatacenterNameConflicts should return an error as the datacenter name is already in use")
}

func TestChangeDcNameFailure1(t *testing.T) {
	assert := assert.New(t)
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	rc.Datacenter.Status = api.CassandraDatacenterStatus{
		DatacenterName: ptr.To("test"),
	}

	errs := rc.validateDatacenterNameOverride()
	assert.NotEmpty(errs, "validateDatacenterNameOverride should return an error as the datacenter name is being modified")
}

func TestChangeDcNameFailure2(t *testing.T) {
	assert := assert.New(t)
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	rc.Datacenter.Spec.DatacenterName = "test"
	rc.Datacenter.Status = api.CassandraDatacenterStatus{
		DatacenterName: ptr.To(""),
	}

	errs := rc.validateDatacenterNameOverride()
	assert.NotEmpty(errs, "validateDatacenterNameOverride should return an error as the datacenter name is being modified")
}

func TestChangeDcNameNotModified1(t *testing.T) {
	assert := assert.New(t)
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()
	rc.Datacenter.Spec.DatacenterName = "test"
	rc.Datacenter.Status = api.CassandraDatacenterStatus{
		DatacenterName: ptr.To("test"),
	}

	errs := rc.validateDatacenterNameOverride()
	assert.Empty(errs, "validateDatacenterNameOverride should return an error as the datacenter name is being modified")
}

func TestChangeDcNameNotModified2(t *testing.T) {
	assert := assert.New(t)
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()
	rc.Datacenter.Spec.DatacenterName = "test"
	rc.Datacenter.Status = api.CassandraDatacenterStatus{
		DatacenterName: nil,
	}

	errs := rc.validateDatacenterNameOverride()
	assert.Empty(errs, "validateDatacenterNameOverride should return an error as the datacenter name is being modified")
}
