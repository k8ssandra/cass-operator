package secrets

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type KubernetesProvider struct {
	client.Client
}

// SecretProvider interfaces

func (k *KubernetesProvider) RetrieveSecret(name types.NamespacedName) (*corev1.Secret, error) {
	return nil, nil
}

func (k *KubernetesProvider) StoreOrUpdateSecret(secret *corev1.Secret) error {

	return nil
}
