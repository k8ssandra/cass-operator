// Copyright DataStax, Inc.
// Please see the included license file for details.

// Package secretcache caches the contents of Secrets referenced by the operator.
package secretcache

import (
	"context"

	"github.com/k8ssandra/cass-operator/pkg/dynamicwatch"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Read tries the normal client's filtered cache, then reads the API on NotFound.
// Callers opt in explicitly; ordinary Secret reads are not intercepted.
// The live object is returned while the informer catches up with the label patch.
func Read(ctx context.Context, c client.Client, apiReader client.Reader, key client.ObjectKey, secret *corev1.Secret) error {
	if err := c.Get(ctx, key, secret); !apierrors.IsNotFound(err) {
		return err
	}

	if err := apiReader.Get(ctx, key, secret); err != nil {
		return err
	}

	if metav1.HasLabel(secret.ObjectMeta, dynamicwatch.WatchedLabel) {
		return nil
	}

	patch := client.MergeFrom(secret.DeepCopy())
	metav1.SetMetaDataLabel(&secret.ObjectMeta, dynamicwatch.WatchedLabel, "true")
	return c.Patch(ctx, secret, patch)
}
