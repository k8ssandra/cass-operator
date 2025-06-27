// Copyright DataStax, Inc.
// Please see the included license file for details.

package reconciliation

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/pkg/mocks"
)

func TestDeletePVCs(t *testing.T) {
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
		}).Twice()

	k8sMockClientDelete(mockClient, nil)

	err := rc.deletePVCs()
	if err != nil {
		t.Fatalf("deletePVCs should not have failed")
	}
}

func TestDeletePVCs_FailedToList(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	mockClient := mocks.NewClient(t)
	rc.Client = mockClient

	k8sMockClientList(mockClient, fmt.Errorf("failed to list PVCs for CassandraDatacenter")).
		Run(func(args mock.Arguments) {
			arg := args.Get(1).(*corev1.PersistentVolumeClaimList)
			arg.Items = []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pvc-1",
				},
			}}
		})

	err := rc.deletePVCs()
	if err == nil {
		t.Fatalf("deletePVCs should have failed")
	}
}

func TestDeletePVCs_PVCsNotFound(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()
	assert := assert.New(t)

	mockClient := mocks.NewClient(t)
	rc.Client = mockClient

	k8sMockClientList(mockClient, errors.NewNotFound(schema.GroupResource{}, "name")).
		Run(func(args mock.Arguments) {
			arg := args.Get(1).(*corev1.PersistentVolumeClaimList)
			arg.Items = []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pvc-1",
				},
			}}
		})

	assert.NoError(rc.deletePVCs())
}

func TestDeletePVCs_FailedToDelete(t *testing.T) {
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
		}).Twice()

	k8sMockClientDelete(mockClient, fmt.Errorf("failed to delete"))

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

	mockClient := mocks.NewClient(t)
	rc.Client = mockClient

	k8sMockClientList(mockClient, nil).
		Run(func(args mock.Arguments) {
			_, ok := args.Get(1).(*corev1.PodList)
			if ok {
				if strings.HasPrefix(args.Get(2).(*client.ListOptions).FieldSelector.String(), "spec.volumes.persistentVolumeClaim.claimName") {
					arg := args.Get(1).(*corev1.PodList)
					arg.Items = []corev1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod-1",
							},
						},
					}
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

	err := rc.deletePVCs()
	assert.Error(err)
	assert.EqualError(err, "PersistentVolumeClaim pvc-1 is still being used by a pod")
}

func TestDeletePVCsSkip(t *testing.T) {
	rc, _, cleanupMockScr := setupTest()
	defer cleanupMockScr()

	mockClient := mocks.NewClient(t)
	rc.Client = mockClient

	rc.Datacenter.Annotations = map[string]string{
		api.DeletePVCAnnotation: "false",
	}

	if err := rc.deletePVCs(); err != nil {
		t.Fatalf("deletePVCs should not have failed")
	}

	mockClient.AssertNotCalled(t, "List", mock.Anything, mock.Anything, mock.Anything)
	mockClient.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
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
	storageClass.AllowVolumeExpansion = ptr.To[bool](true)
	require.NoError(rc.Client.Update(rc.Ctx, storageClass))

	supports, err = rc.storageExpansion()
	require.NoError(err)
	require.True(supports)
}
