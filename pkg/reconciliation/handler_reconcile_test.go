package reconciliation_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	controllers "github.com/k8ssandra/cass-operator/internal/controllers/cassandra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcile(t *testing.T) {
	var (
		name            = "cluster-example-cluster"
		namespace       = "default"
		size      int32 = 2
	)
	storageSize := resource.MustParse("1Gi")
	storageName := "server-data"
	storageConfig := api.StorageConfig{
		CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storageName,
			AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
			Resources: corev1.VolumeResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{"storage": storageSize},
			},
		},
	}

	// Instance a CassandraDatacenter
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: api.CassandraDatacenterSpec{
			ManagementApiAuth: api.ManagementApiAuthConfig{
				Insecure: &api.ManagementApiAuthInsecureConfig{},
			},
			Size:          size,
			ServerType:    "dse",
			ServerVersion: "6.8.42",
			StorageConfig: storageConfig,
			ClusterName:   "cluster-example",
		},
	}

	// Objects to keep track of
	trackObjects := []runtime.Object{
		dc,
	}

	s := scheme.Scheme
	s.AddKnownTypes(api.GroupVersion, dc)

	fakeClient := fake.NewClientBuilder().WithStatusSubresource(dc).WithRuntimeObjects(trackObjects...).Build()

	r := &controllers.CassandraDatacenterReconciler{
		Client:   fakeClient,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(100),
	}

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	result, err := r.Reconcile(context.TODO(), request)
	if err != nil {
		t.Fatalf("Reconciliation Failure: (%v)", err)
	}

	if result != (reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}) {
		t.Error("Reconcile did not return a correct result.")
	}
}

func TestReconcile_NotFound(t *testing.T) {
	var (
		name            = "datacenter-example"
		namespace       = "default"
		size      int32 = 2
	)

	storageSize := resource.MustParse("1Gi")
	storageName := "server-data"
	storageConfig := api.StorageConfig{
		CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storageName,
			AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
			Resources: corev1.VolumeResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{"storage": storageSize},
			},
		},
	}

	// Instance a CassandraDatacenter
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: api.CassandraDatacenterSpec{
			ManagementApiAuth: api.ManagementApiAuthConfig{
				Insecure: &api.ManagementApiAuthInsecureConfig{},
			},
			Size:          size,
			StorageConfig: storageConfig,
		},
	}

	// Objects to keep track of
	trackObjects := []runtime.Object{}

	s := scheme.Scheme
	s.AddKnownTypes(api.GroupVersion, dc)

	fakeClient := fake.NewClientBuilder().WithStatusSubresource(dc).WithRuntimeObjects(trackObjects...).Build()

	r := &controllers.CassandraDatacenterReconciler{
		Client: fakeClient,
		Scheme: s,
	}

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	result, err := r.Reconcile(context.TODO(), request)
	require.Error(t, err)
	require.Equal(t, reconcile.Result{}, result)
}

func TestReconcile_Error(t *testing.T) {
	var (
		name            = "datacenter-example"
		namespace       = "default"
		size      int32 = 2
	)

	storageSize := resource.MustParse("1Gi")
	storageName := "server-data"
	storageConfig := api.StorageConfig{
		CassandraDataVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storageName,
			AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
			Resources: corev1.VolumeResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{"storage": storageSize},
			},
		},
	}

	// Instance a CassandraDatacenter
	dc := &api.CassandraDatacenter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: api.CassandraDatacenterSpec{
			ManagementApiAuth: api.ManagementApiAuthConfig{
				Insecure: &api.ManagementApiAuthInsecureConfig{},
			},
			Size:          size,
			StorageConfig: storageConfig,
		},
	}

	// Objects to keep track of

	s := scheme.Scheme
	s.AddKnownTypes(api.GroupVersion, dc)

	mockClient := &mocks.Client{}
	mockClient.On("Get",
		mock.MatchedBy(
			func(ctx context.Context) bool {
				return ctx != nil
			}),
		mock.MatchedBy(
			func(key client.ObjectKey) bool {
				return key != client.ObjectKey{}
			}),
		mock.MatchedBy(
			func(obj runtime.Object) bool {
				return obj != nil
			})).
		Return(fmt.Errorf("some cryptic error")).
		Once()

	r := &controllers.CassandraDatacenterReconciler{
		Client: mockClient,
		Scheme: s,
	}

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	_, err := r.Reconcile(context.TODO(), request)
	require.Error(t, err, "Reconciliation should have failed")
}
