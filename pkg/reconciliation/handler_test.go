// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/pkg/dynamicwatch"
	"github.com/k8ssandra/cass-operator/pkg/mocks"
)

func TestCalculateReconciliationActions(t *testing.T) {
	t.Skip("This does not test current functionality and is covered by envtests")
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	service := newServiceForCassandraDatacenter(rc.Datacenter)

	// Objects to keep track of

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cassandradatacenter-example-cluster-cassandradatacenter-example-default-sts-1",
			Namespace: rc.Datacenter.Namespace,
			Labels:    rc.Datacenter.GetRackLabels("default"),
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "cassandra",
				},
			},
		},
	}

	trackObjects := []runtime.Object{
		rc.Datacenter,
		service,
		pod,
	}

	fakeClient := fake.NewClientBuilder().WithStatusSubresource(rc.Datacenter, service).WithRuntimeObjects(trackObjects...).Build()
	rc.Client = fakeClient

	result, err := rc.CalculateReconciliationActions()
	assert.NoErrorf(t, err, "Should not have returned an error while calculating reconciliation actions")
	assert.NotNil(t, result, "Result should not be nil")

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
	rc.Datacenter.Spec.Size = 0

	k8sMockClientList(mockClient, nil).
		Run(func(args mock.Arguments) {
			_, ok := args.Get(1).(*corev1.PodList)
			if ok {
				if strings.HasPrefix(args.Get(2).(*client.ListOptions).FieldSelector.String(), "spec.volumes.persistentVolumeClaim.claimName") {
					arg := args.Get(1).(*corev1.PodList)
					arg.Items = []corev1.Pod{}
				} else {
					t.Fail()
				}
				return
			}
			arg := args.Get(1).(*corev1.PersistentVolumeClaimList)
			arg.Items = []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pvc-1",
				},
			}}
		}).Twice()

	k8sMockClientGet(mockClient, nil).
		Run(func(args mock.Arguments) {
			arg := args.Get(2).(*appsv1.StatefulSet)
			arg.Spec.Replicas = ptr.To[int32](0)
		}).Once()

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
			_, ok := args.Get(1).(*corev1.PodList)
			if ok {
				if strings.HasPrefix(args.Get(2).(*client.ListOptions).FieldSelector.String(), "spec.volumes.persistentVolumeClaim.claimName") {
					arg := args.Get(1).(*corev1.PodList)
					arg.Items = []corev1.Pod{}
				} else {
					t.Fail()
				}
				return
			}
			arg := args.Get(1).(*corev1.PersistentVolumeClaimList)
			arg.Items = []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pvc-1",
				},
			}}
		}).Twice() // ListPods

	k8sMockClientDelete(mockClient, nil) // Delete PVC
	k8sMockClientUpdate(mockClient, nil) // Remove dc finalizer
	k8sMockClientGet(mockClient, nil).
		Run(func(args mock.Arguments) {
			arg := args.Get(2).(*appsv1.StatefulSet)
			arg.Spec.Replicas = ptr.To[int32](0)
		}).Once()

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
	assert.Equal(reconcile.Result{}, result, "Should not requeue request")

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
				},
				Status: api.CassandraDatacenterStatus{
					DatacenterName: ptr.To[string]("CassandraDatacenter_example"),
				},
			}}
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
