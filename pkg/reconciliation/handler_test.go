// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	record "k8s.io/client-go/tools/events"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/pkg/dynamicwatch"
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

	fakeClient := fake.NewClientBuilder().WithScheme(setupScheme()).WithStatusSubresource(rc.Datacenter, service).WithRuntimeObjects(trackObjects...).Build()
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

	getErr := fmt.Errorf("")
	rc.Client = fake.NewClientBuilder().
		WithScheme(setupScheme()).
		WithStatusSubresource(rc.Datacenter).
		WithRuntimeObjects(rc.Datacenter).
		WithIndex(&corev1.Pod{}, podPVCClaimNameField, podPVCClaimNames).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*corev1.Service); ok {
					return getErr
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).
		Build()

	_, err := rc.CalculateReconciliationActions()
	assert.Errorf(t, err, "Should have returned an error while calculating reconciliation actions")
}

func TestCalculateReconciliationActions_FailedUpdate(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	updateErr := fmt.Errorf("failed to update CassandraDatacenter with removed finalizers")
	rc.Client = fake.NewClientBuilder().
		WithScheme(setupScheme()).
		WithStatusSubresource(rc.Datacenter).
		WithRuntimeObjects(rc.Datacenter).
		WithIndex(&corev1.Pod{}, podPVCClaimNameField, podPVCClaimNames).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*api.CassandraDatacenter); ok {
					return updateErr
				}
				return c.Update(ctx, obj, opts...)
			},
		}).
		Build()

	_, err := rc.CalculateReconciliationActions()
	assert.Errorf(t, err, "Should have returned an error while calculating reconciliation actions")
}

func emptySecretWatcher(rc *ReconciliationContext) {
	rc.SecretWatches = dynamicwatch.NewDynamicSecretWatches(rc.Client)
}

func pvcProto(rc *ReconciliationContext) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "server-data",
			Namespace: rc.Datacenter.Namespace,
			Labels:    rc.Datacenter.GetDatacenterLabels(),
		},
	}
}

// TestProcessDeletion_FailedDelete fails one step of the deletion process and should not cause
// the removal of the finalizer, instead controller-runtime will return an error and retry later
func TestProcessDeletion_FailedDelete(t *testing.T) {
	assert := assert.New(t)
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	rc.Datacenter.Spec.Size = 0
	rc.Datacenter.SetFinalizers([]string{api.Finalizer})
	now := metav1.Now()
	rc.Datacenter.SetDeletionTimestamp(&now)
	sts, err := newStatefulSetForCassandraDatacenter(nil, "default", rc.Datacenter, 0, imageRegistry)
	assert.NoError(err)
	pvc := pvcProto(rc)
	deleteErr := fmt.Errorf("failed to delete pvc")
	rc.Client = fake.NewClientBuilder().
		WithScheme(setupScheme()).
		WithStatusSubresource(rc.Datacenter).
		WithRuntimeObjects(rc.Datacenter, sts, pvc).
		WithIndex(&corev1.Pod{}, podPVCClaimNameField, podPVCClaimNames).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				if _, ok := obj.(*corev1.PersistentVolumeClaim); ok {
					return deleteErr
				}
				return c.Delete(ctx, obj, opts...)
			},
		}).
		Build()
	emptySecretWatcher(rc)

	result, err := rc.CalculateReconciliationActions()
	assert.Errorf(err, "Should have returned an error while calculating reconciliation actions")
	assert.Equal(reconcile.Result{}, result, "Should not requeue request as error does cause requeue")

	storedDC := &api.CassandraDatacenter{}
	getErr := rc.Client.Get(rc.Ctx, client.ObjectKeyFromObject(rc.Datacenter), storedDC)
	assert.NoError(getErr)
	assert.True(controllerutil.ContainsFinalizer(storedDC, api.Finalizer))
}

// TestProcessDeletion verifies the correct amount of calls to k8sClient on the deletion process
func TestProcessDeletion(t *testing.T) {
	assert := assert.New(t)
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	rc.Datacenter.SetFinalizers([]string{api.Finalizer})
	now := metav1.Now()
	rc.Datacenter.SetDeletionTimestamp(&now)
	sts, err := newStatefulSetForCassandraDatacenter(nil, "default", rc.Datacenter, 0, imageRegistry)
	assert.NoError(err)
	pvc := pvcProto(rc)
	rc.Client = fake.NewClientBuilder().
		WithScheme(setupScheme()).
		WithStatusSubresource(rc.Datacenter).
		WithRuntimeObjects(rc.Datacenter, sts, pvc).
		WithIndex(&corev1.Pod{}, podPVCClaimNameField, podPVCClaimNames).
		Build()
	emptySecretWatcher(rc)

	result, err := rc.CalculateReconciliationActions()
	assert.NoError(err)
	assert.Equal(reconcile.Result{}, result, "Should not requeue request")

	storedDC := &api.CassandraDatacenter{}
	getErr := rc.Client.Get(rc.Ctx, client.ObjectKeyFromObject(rc.Datacenter), storedDC)
	assert.True(apierrors.IsNotFound(getErr))
}

func TestProcessDeletionWithMissingSecrets(t *testing.T) {
	tests := []struct {
		name                 string
		deleting             bool
		decommissionOnDelete bool
		scalingDown          bool
		manualTLS            bool
		wantErr              bool
	}{
		{name: "active datacenter with missing user secret", wantErr: true},
		{name: "active datacenter with missing TLS secrets", manualTLS: true, wantErr: true},
		{name: "deletion with missing user secret", deleting: true},
		{name: "deletion with missing TLS secrets", deleting: true, manualTLS: true},
		{name: "decommission on delete with missing user secret", deleting: true, decommissionOnDelete: true, wantErr: true},
		{name: "decommission on delete with missing TLS secrets", deleting: true, decommissionOnDelete: true, manualTLS: true, wantErr: true},
		{name: "deletion while scaling down with missing TLS secrets", deleting: true, scalingDown: true, manualTLS: true, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require := require.New(t)
			mockRc, _, cleanupMockScr := setupTest()
			defer cleanupMockScr()

			dc := mockRc.Datacenter
			dc.SetFinalizers([]string{api.Finalizer})
			dc.Spec.Users = []api.CassandraUser{{SecretName: "missing-user"}}
			if test.manualTLS {
				dc.Spec.ManagementApiAuth.Manual = &api.ManagementApiAuthManualConfig{
					ClientSecretName: "missing-client", ServerSecretName: "missing-server",
				}
			}
			if test.deleting {
				now := metav1.Now()
				dc.SetDeletionTimestamp(&now)
			}
			if test.decommissionOnDelete {
				metav1.SetMetaDataAnnotation(&dc.ObjectMeta, api.DecommissionOnDeleteAnnotation, "true")
			}
			if test.scalingDown {
				dc.SetCondition(*api.NewDatacenterCondition(api.DatacenterScalingDown, corev1.ConditionTrue))
			}
			sts, err := newStatefulSetForCassandraDatacenter(nil, "default", dc, 0, imageRegistry)
			require.NoError(err)
			fakeClient := fake.NewClientBuilder().
				WithScheme(setupScheme()).
				WithStatusSubresource(dc).
				WithRuntimeObjects(dc, sts).
				WithIndex(&corev1.Pod{}, podPVCClaimNameField, podPVCClaimNames).
				Build()

			rc, err := CreateReconciliationContext(mockRc.Ctx, mockRc.Request, fakeClient, fakeClient, mockRc.Scheme,
				record.NewFakeRecorder(100), dynamicwatch.NewDynamicSecretWatches(fakeClient), mockRc.ImageRegistry, false)
			if err == nil {
				err = rc.IsValid(rc.Datacenter)
			}
			if test.wantErr {
				require.Error(err)
				return
			}
			require.NoError(err)

			_, err = rc.CalculateReconciliationActions()
			require.NoError(err)
			getErr := fakeClient.Get(rc.Ctx, client.ObjectKeyFromObject(dc), &api.CassandraDatacenter{})
			require.True(apierrors.IsNotFound(getErr), "datacenter should be deleted, got %v", getErr)
		})
	}
}

// TestProcessDeletion_NoFinalizer verifies that the removal of finalizer means cass-operator will do nothing
// on the deletion process.
func TestProcessDeletion_NoFinalizer(t *testing.T) {
	assert := assert.New(t)
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	now := metav1.Now()
	rc.Datacenter.SetDeletionTimestamp(&now)

	result, err := rc.CalculateReconciliationActions()
	assert.NoError(err)
	assert.Equal(reconcile.Result{}, result, "Should not requeue request")
}

func TestAddFinalizer(t *testing.T) {
	assert := assert.New(t)
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	err := rc.addFinalizer()
	assert.NoError(err)
	assert.True(controllerutil.ContainsFinalizer(rc.Datacenter, api.Finalizer))

	// This should not add the finalizer again
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

	err := rc.Client.Create(rc.Ctx, &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dc1",
			Namespace: rc.Datacenter.Namespace,
		},
		Spec: api.CassandraDatacenterSpec{
			ClusterName:    "cluster1",
			DatacenterName: "CassandraDatacenter_example",
		},
		Status: api.CassandraDatacenterStatus{
			DatacenterName: new("CassandraDatacenter_example"),
		},
	})
	assert.NoError(err)

	errs := rc.validateDatacenterNameConflicts()
	assert.NotEmpty(errs, "validateDatacenterNameConflicts should return an error as the datacenter name is already in use")
}

func TestChangeDcNameFailure1(t *testing.T) {
	assert := assert.New(t)
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	rc.Datacenter.Status = api.CassandraDatacenterStatus{
		DatacenterName: new("test"),
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
		DatacenterName: new(""),
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
		DatacenterName: new("test"),
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
