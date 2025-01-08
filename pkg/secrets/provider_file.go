package secrets

import (
	"context"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type FileProvider struct {
	path              string
	namespacePrefixed bool
}

func NewFileProvider(directoryPath string, namespacePrefix bool) *FileProvider {
	return &FileProvider{
		path:              directoryPath,
		namespacePrefixed: namespacePrefix,
	}
}

// SecretProvider interface

func (f *FileProvider) RetrieveSecret(ctx context.Context, name types.NamespacedName) (*corev1.Secret, error) {
	filename := f.secretPath(name)
	return readSecret(filename)
}

func (f *FileProvider) StoreOrUpdateSecret(ctx context.Context, secret *corev1.Secret) error {
	// This provider does not store updates to the disk. This is intended for use-cases where the secrets are externally
	// applied to operator's volume
	return nil
}

// Rest of the implementation

func (f *FileProvider) secretPath(name types.NamespacedName) string {
	if f.namespacePrefixed {
		return filepath.Join(f.path, name.Namespace, name.Name)
	}
	return filepath.Join(f.path, name.Name)
}

func readSecret(path string) (*corev1.Secret, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return decodeSecret(b)
}
