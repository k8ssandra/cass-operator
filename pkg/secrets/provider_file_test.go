package secrets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ SecretProvider = &FileProvider{}

func TestSecretPathsNoPrefix(t *testing.T) {
	assert := assert.New(t)
	f := &FileProvider{
		path:              "/etc/secrets",
		namespacePrefixed: false,
	}

	sName := types.NamespacedName{Name: "cert-client", Namespace: "cluster1"}
	path := f.secretPath(sName)
	assert.Equal("/etc/secrets/cert-client", path)
}

func TestSecretPathsWithPrefix(t *testing.T) {
	assert := assert.New(t)
	f := &FileProvider{
		path:              "/etc/secrets",
		namespacePrefixed: true,
	}

	sName := types.NamespacedName{Name: "cert-client", Namespace: "cluster1"}
	path := f.secretPath(sName)
	assert.Equal("/etc/secrets/cluster1/cert-client", path)
}

func TestSecretReading(t *testing.T) {
	require := require.New(t)
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-ns",
		},
		StringData: map[string]string{
			"a": "b",
			"c": "d",
		},
	}

	data, err := encodeSecret(s)
	require.NoError(err)

	tempDir, err := os.MkdirTemp("", "")
	require.NoError(err)
	defer os.RemoveAll(tempDir)

	err = os.WriteFile(filepath.Join(tempDir, "test-secret"), data, 0755)
	require.NoError(err)

	f := NewFileProvider(tempDir, false)

	s2, err := f.RetrieveSecret(types.NamespacedName{Name: s.Name, Namespace: s.Namespace})
	require.NoError(err)

	require.Equal(s.Name, s2.Name)
	require.Equal(s.Namespace, s2.Namespace)
	require.Equal(s.StringData, s2.StringData)
}
