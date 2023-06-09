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
	"k8s.io/utils/pointer"

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
	// k8sMockClientCreate(mockClient, nil)

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
		DatacenterName: pointer.String("test"),
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
		DatacenterName: pointer.String(""),
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
		DatacenterName: pointer.String("test"),
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

// func TestReconcile(t *testing.T) {
// 	t.Skip()

// 	// Set up verbose logging
// 	logger := zap.New()
// 	logf.SetLogger(logger)

// 	var (
// 		name            = "cluster-example-cluster.dc-example-datacenter"
// 		namespace       = "default"
// 		size      int32 = 2
// 	)
// 	storageSize := resource.MustParse("1Gi")
// 	storageName := "server-data"
// 	storageConfig := api.StorageConfig{
// 		CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
// 			StorageClassName: &storageName,
// 			AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
// 			Resources: corev1.ResourceRequirements{
// 				Requests: map[corev1.ResourceName]resource.Quantity{"storage": storageSize},
// 			},
// 		},
// 	}

// 	// Instance a CassandraDatacenter
// 	dc := &api.CassandraDatacenter{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      name,
// 			Namespace: namespace,
// 		},
// 		Spec: api.CassandraDatacenterSpec{
// 			ManagementApiAuth: api.ManagementApiAuthConfig{
// 				Insecure: &api.ManagementApiAuthInsecureConfig{},
// 			},
// 			Size:          size,
// 			ServerVersion: "6.8.0",
// 			StorageConfig: storageConfig,
// 			ClusterName:   "cluster-example",
// 		},
// 	}

// 	// Objects to keep track of
// 	trackObjects := []runtime.Object{
// 		dc,
// 	}

// 	s := scheme.Scheme
// 	s.AddKnownTypes(api.GroupVersion, dc)

// 	fakeClient := fake.NewFakeClient(trackObjects...)

// 	r := &ReconcileCassandraDatacenter{
// 		client:   fakeClient,
// 		scheme:   s,
// 		recorder: record.NewFakeRecorder(100),
// 	}

// 	request := reconcile.Request{
// 		NamespacedName: types.NamespacedName{
// 			Name:      name,
// 			Namespace: namespace,
// 		},
// 	}

// 	result, err := r.Reconcile(request)
// 	if err != nil {
// 		t.Fatalf("Reconciliation Failure: (%v)", err)
// 	}

// 	if result != (reconcile.Result{Requeue: true}) {
// 		t.Error("Reconcile did not return a correct result.")
// 	}
// }

// func TestReconcile_NotFound(t *testing.T) {
// 	t.Skip()

// 	// Set up verbose logging
// 	logger := zap.New()
// 	logf.SetLogger(logger)

// 	var (
// 		name            = "datacenter-example"
// 		namespace       = "default"
// 		size      int32 = 2
// 	)

// 	storageSize := resource.MustParse("1Gi")
// 	storageName := "server-data"
// 	storageConfig := api.StorageConfig{
// 		CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
// 			StorageClassName: &storageName,
// 			AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
// 			Resources: corev1.ResourceRequirements{
// 				Requests: map[corev1.ResourceName]resource.Quantity{"storage": storageSize},
// 			},
// 		},
// 	}

// 	// Instance a CassandraDatacenter
// 	dc := &api.CassandraDatacenter{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      name,
// 			Namespace: namespace,
// 		},
// 		Spec: api.CassandraDatacenterSpec{
// 			ManagementApiAuth: api.ManagementApiAuthConfig{
// 				Insecure: &api.ManagementApiAuthInsecureConfig{},
// 			},
// 			Size:          size,
// 			StorageConfig: storageConfig,
// 		},
// 	}

// 	// Objects to keep track of
// 	trackObjects := []runtime.Object{}

// 	s := scheme.Scheme
// 	s.AddKnownTypes(api.GroupVersion, dc)

// 	fakeClient := fake.NewFakeClient(trackObjects...)

// 	r := &ReconcileCassandraDatacenter{
// 		client: fakeClient,
// 		scheme: s,
// 	}

// 	request := reconcile.Request{
// 		NamespacedName: types.NamespacedName{
// 			Name:      name,
// 			Namespace: namespace,
// 		},
// 	}

// 	result, err := r.Reconcile(request)
// 	if err != nil {
// 		t.Fatalf("Reconciliation Failure: (%v)", err)
// 	}

// 	expected := reconcile.Result{}
// 	if result != expected {
// 		t.Error("expected to get a zero-value reconcile.Result")
// 	}
// }

// func TestReconcile_Error(t *testing.T) {
// 	// Set up verbose logging
// 	logger := zap.New()
// 	logf.SetLogger(logger)

// 	var (
// 		name            = "datacenter-example"
// 		namespace       = "default"
// 		size      int32 = 2
// 	)

// 	storageSize := resource.MustParse("1Gi")
// 	storageName := "server-data"
// 	storageConfig := api.StorageConfig{
// 		CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
// 			StorageClassName: &storageName,
// 			AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
// 			Resources: corev1.ResourceRequirements{
// 				Requests: map[corev1.ResourceName]resource.Quantity{"storage": storageSize},
// 			},
// 		},
// 	}

// 	// Instance a CassandraDatacenter
// 	dc := &api.CassandraDatacenter{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      name,
// 			Namespace: namespace,
// 		},
// 		Spec: api.CassandraDatacenterSpec{
// 			ManagementApiAuth: api.ManagementApiAuthConfig{
// 				Insecure: &api.ManagementApiAuthInsecureConfig{},
// 			},
// 			Size:          size,
// 			StorageConfig: storageConfig,
// 		},
// 	}

// 	// Objects to keep track of

// 	s := scheme.Scheme
// 	s.AddKnownTypes(api.GroupVersion, dc)

// 	mockClient := &mocks.Client{}
// 	k8sMockClientGet(mockClient, fmt.Errorf(""))

// 	r := &ReconcileCassandraDatacenter{
// 		client: mockClient,
// 		scheme: s,
// 	}

// 	request := reconcile.Request{
// 		NamespacedName: types.NamespacedName{
// 			Name:      name,
// 			Namespace: namespace,
// 		},
// 	}

// 	_, err := r.Reconcile(request)
// 	if err == nil {
// 		t.Fatalf("Reconciliation should have failed")
// 	}
// }

// func TestReconcile_CassandraDatacenterToBeDeleted(t *testing.T) {
// 	t.Skip()
// 	// Set up verbose logging
// 	logger := zap.New()
// 	logf.SetLogger(logger)

// 	var (
// 		name            = "datacenter-example"
// 		namespace       = "default"
// 		size      int32 = 2
// 	)

// 	storageSize := resource.MustParse("1Gi")
// 	storageName := "server-data"
// 	storageConfig := api.StorageConfig{
// 		CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
// 			StorageClassName: &storageName,
// 			AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
// 			Resources: corev1.ResourceRequirements{
// 				Requests: map[corev1.ResourceName]resource.Quantity{"storage": storageSize},
// 			},
// 		},
// 	}

// 	// Instance a CassandraDatacenter
// 	now := metav1.Now()
// 	dc := &api.CassandraDatacenter{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:              name,
// 			Namespace:         namespace,
// 			DeletionTimestamp: &now,
// 			Finalizers:        nil,
// 		},
// 		Spec: api.CassandraDatacenterSpec{
// 			ManagementApiAuth: api.ManagementApiAuthConfig{
// 				Insecure: &api.ManagementApiAuthInsecureConfig{},
// 			},
// 			Size:          size,
// 			ServerVersion: "6.8.0",
// 			StorageConfig: storageConfig,
// 		},
// 	}

// 	// Objects to keep track of
// 	trackObjects := []runtime.Object{
// 		dc,
// 	}

// 	s := scheme.Scheme
// 	s.AddKnownTypes(api.GroupVersion, dc)

// 	fakeClient := fake.NewFakeClient(trackObjects...)

// 	r := &controllers.CassandraDatacenterReconciler{
// 		Client: fakeClient,
// 		Scheme: s,
// 	}

// 	request := reconcile.Request{
// 		NamespacedName: types.NamespacedName{
// 			Name:      name,
// 			Namespace: namespace,
// 		},
// 	}

// 	result, err := r.Reconcile(context.TODO(), request)
// 	if err != nil {
// 		t.Fatalf("Reconciliation Failure: (%v)", err)
// 	}

// 	if result != (reconcile.Result{}) {
// 		t.Error("Reconcile did not return an empty result.")
// 	}
// }
