package secretcache

import (
	"context"
	"errors"
	"testing"

	"github.com/k8ssandra/cass-operator/pkg/dynamicwatch"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestSecretReadBootstrap(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: "test", Labels: map[string]string{"external": "owner"}}, Data: map[string][]byte{"password": []byte("original")}}
	liveReads, patches := 0, 0
	live := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			liveReads++
			return c.Get(ctx, key, obj, opts...)
		},
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			patches++
			return c.Patch(ctx, obj, patch, opts...)
		},
	}).Build()
	cached := fake.NewClientBuilder().WithScheme(scheme).Build()
	c := interceptor.NewClient(live, interceptor.Funcs{
		Get: func(ctx context.Context, _ client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			return cached.Get(ctx, key, obj, opts...)
		},
	})
	key := client.ObjectKeyFromObject(secret)
	for range 2 { // The informer has not caught up after the label patch yet.
		got := &corev1.Secret{}
		require.NoError(t, Read(ctx, c, live, key, got))
		require.Equal(t, secret.Data, got.Data)
		require.Equal(t, "owner", got.Labels["external"])
		require.Equal(t, "true", got.Labels[dynamicwatch.WatchedLabel])
		require.Empty(t, got.OwnerReferences)
	}
	require.Equal(t, 2, liveReads)
	require.Equal(t, 1, patches)
	stored := &corev1.Secret{}
	require.NoError(t, live.Get(ctx, key, stored))
	stored.ResourceVersion = ""
	require.NoError(t, cached.Create(ctx, stored))
	liveReads = 0
	require.NoError(t, Read(ctx, c, live, key, &corev1.Secret{}))
	require.Zero(t, liveReads)
	require.Equal(t, 1, patches)
	require.True(t, apierrors.IsNotFound(Read(ctx, c, live, client.ObjectKey{Namespace: "test", Name: "missing"}, &corev1.Secret{})))
}

func TestSecretReadErrors(t *testing.T) {
	for _, readErr := range []error{&cache.ErrCacheNotStarted{}, &cache.ErrResourceNotCached{}, errors.New("sync failed")} {
		t.Run(readErr.Error(), func(t *testing.T) {
			live := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
				Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
					t.Fatal("must not hide cache failures with a live read")
					return nil
				},
			}).Build()
			cached := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
				Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
					return readErr
				},
			}).Build()
			require.ErrorIs(t, Read(context.Background(), cached, live, client.ObjectKey{}, &corev1.Secret{}), readErr)
		})
	}
}

func TestSecretLabelPatchFailure(t *testing.T) {
	patchErr := errors.New("patch denied")
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "config", Namespace: "test"}}
	live := fake.NewClientBuilder().WithObjects(secret).WithInterceptorFuncs(interceptor.Funcs{
		Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
			return patchErr
		},
	}).Build()
	cached := fake.NewClientBuilder().Build()
	c := interceptor.NewClient(live, interceptor.Funcs{
		Get: func(ctx context.Context, _ client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			return cached.Get(ctx, key, obj, opts...)
		},
	})
	err := Read(context.Background(), c, live, client.ObjectKeyFromObject(secret), &corev1.Secret{})
	require.ErrorIs(t, err, patchErr)
}
