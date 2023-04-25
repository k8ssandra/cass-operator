package secrets

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type KubernetesProvider struct {
	client.Client
}

func NewKubernetesProvider(cli client.Client) *KubernetesProvider {
	return &KubernetesProvider{
		Client: cli,
	}
}

// SecretProvider interfaces

func (k *KubernetesProvider) RetrieveSecret(ctx context.Context, name types.NamespacedName) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	if err := k.Client.Get(ctx, name, secret); err != nil {
		return nil, err
	}
	return secret, nil
}

func (k *KubernetesProvider) StoreOrUpdateSecret(ctx context.Context, secret *corev1.Secret) error {
	if err := k.Client.Create(ctx, secret); err != nil {
		if errors.IsAlreadyExists(err) {
			return k.Client.Update(ctx, secret)
		}
	}
	return nil
}
