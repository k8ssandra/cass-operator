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
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
)

func TestDeletePVCs(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	pvc := pvcProto(rc)
	rc.Client = fake.NewClientBuilder().
		WithScheme(setupScheme()).
		WithStatusSubresource(rc.Datacenter).
		WithRuntimeObjects(rc.Datacenter, pvc).
		WithIndex(&corev1.Pod{}, podPVCClaimNameField, podPVCClaimNames).
		Build()

	err := rc.deletePVCs()
	if err != nil {
		t.Fatalf("deletePVCs should not have failed")
	}
}

func TestDeletePVCs_FailedToList(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	listErr := fmt.Errorf("failed to list PVCs for CassandraDatacenter")
	rc.Client = fake.NewClientBuilder().
		WithScheme(setupScheme()).
		WithStatusSubresource(rc.Datacenter).
		WithRuntimeObjects(rc.Datacenter).
		WithIndex(&corev1.Pod{}, podPVCClaimNameField, podPVCClaimNames).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*corev1.PersistentVolumeClaimList); ok {
					return listErr
				}
				return c.List(ctx, list, opts...)
			},
		}).
		Build()

	err := rc.deletePVCs()
	if err == nil {
		t.Fatalf("deletePVCs should have failed")
	}
}

func TestDeletePVCs_PVCsNotFound(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()
	assert := assert.New(t)

	notFoundErr := errors.NewNotFound(schema.GroupResource{}, "name")
	rc.Client = fake.NewClientBuilder().
		WithScheme(setupScheme()).
		WithStatusSubresource(rc.Datacenter).
		WithRuntimeObjects(rc.Datacenter).
		WithIndex(&corev1.Pod{}, podPVCClaimNameField, podPVCClaimNames).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*corev1.PersistentVolumeClaimList); ok {
					return notFoundErr
				}
				return c.List(ctx, list, opts...)
			},
		}).
		Build()

	assert.NoError(rc.deletePVCs())
}

func TestDeletePVCs_FailedToDelete(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	pvc := pvcProto(rc)
	deleteErr := fmt.Errorf("failed to delete")
	rc.Client = fake.NewClientBuilder().
		WithScheme(setupScheme()).
		WithStatusSubresource(rc.Datacenter).
		WithRuntimeObjects(rc.Datacenter, pvc).
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

	err := rc.deletePVCs()
	if err == nil {
		t.Fatalf("deletePVCs should have failed")
	}

	assert.EqualError(t, err, "failed to delete")
}

func TestDeletePVCs_FailedToDeleteBeingUsed(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()
	assert := assert.New(t)

	pvc := pvcProto(rc)
	pod := podWithPVC(rc, "pod-1", pvc.Name)
	rc.Client = fake.NewClientBuilder().
		WithScheme(setupScheme()).
		WithStatusSubresource(rc.Datacenter).
		WithRuntimeObjects(rc.Datacenter, pvc, pod).
		WithIndex(&corev1.Pod{}, podPVCClaimNameField, podPVCClaimNames).
		Build()

	err := rc.deletePVCs()
	assert.Error(err)
	assert.EqualError(err, "PersistentVolumeClaim server-data is still being used by a pod")
}

func TestDeletePVCsSkip(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	rc.Datacenter.Annotations = map[string]string{
		api.DeletePVCAnnotation: "false",
	}

	if err := rc.deletePVCs(); err != nil {
		t.Fatalf("deletePVCs should not have failed")
	}
}

func TestStorageExpansionNils(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()
	require := require.New(t)

	rc.Datacenter.Spec.StorageConfig.CassandraDataVolumeClaimSpec.StorageClassName = nil
	supports, err := rc.storageExpansion()
	require.NoError(err)
	require.False(supports)

	storageClass := &storagev1.StorageClass{}
	require.NoError(rc.Client.Get(rc.Ctx, types.NamespacedName{Name: "standard"}, storageClass))
	storageClass.AllowVolumeExpansion = new(true)
	require.NoError(rc.Client.Update(rc.Ctx, storageClass))

	supports, err = rc.storageExpansion()
	require.NoError(err)
	require.True(supports)
}

func podWithPVC(rc *ReconciliationContext, podName string, pvcName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: rc.Datacenter.Namespace,
		},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{{
				Name: "server-data",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvcName,
					},
				},
			}},
		},
	}
}
