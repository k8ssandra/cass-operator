package images

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	configv1beta2 "github.com/k8ssandra/cass-operator/apis/config/v1beta2"
)

func newTestImageRegistryV2(content []byte) (ImageRegistry, error) {
	return NewImageRegistryV2(content)
}

func TestDefaultRegistryOverrideV2(t *testing.T) {
	assert := assert.New(t)
	yamlConfig := `
apiVersion: config.k8ssandra.io/v1beta2
kind: ImageConfig
defaults:
  registry: localhost:5000
images:
  config-builder:
    name: config-builder-temp
    tag: latest
types:
  cassandra:
    repository: k8ssandra
    name: cass-management-api
`
	registry, err := newTestImageRegistryV2([]byte(yamlConfig))
	require.NoError(t, err)

	image := registry.GetConfigBuilderImage()
	assert.Equal("localhost:5000/config-builder-temp:latest", image)

	image, err = registry.GetCassandraImage("cassandra", "4.0.6")
	assert.NoError(err)
	assert.Equal("localhost:5000/k8ssandra/cass-management-api:4.0.6", image)
}

func TestDefaultImageConfigParsingV2(t *testing.T) {
	assert := require.New(t)
	imageConfigFile := filepath.Join("..", "..", "config", "manager", "image_config.yaml")
	content, err := os.ReadFile(imageConfigFile)
	assert.NoError(err)

	registry, err := NewImageRegistryV2(content)
	assert.NoError(err, "imageConfig parsing should succeed")

	// Verify some default values are set
	imageConfig := &registry.(*imageRegistryV1Beta2).imageConfig
	assert.NotNil(imageConfig)
	assert.NotNil(imageConfig.Images)
	assert.Contains(registry.GetSystemLoggerImage(), "k8ssandra/system-logger:")
	assert.Contains(registry.GetConfigBuilderImage(), "datastax/cass-config-builder:")
	assert.Contains(registry.GetClientImage(), "k8ssandra/k8ssandra-client:")

	assert.Equal("ghcr.io", imageConfig.Types["cassandra"].Registry)
	assert.Equal("k8ssandra", imageConfig.Types["cassandra"].Repository)
	assert.Equal("datastax", imageConfig.Types["dse"].Repository)

	path, err := registry.GetCassandraImage("dse", "6.8.47")
	assert.NoError(err)
	assert.Equal("datastax/dse-server:6.8.47-ubi8", path)

	path, err = registry.GetCassandraImage("hcd", "1.0.0")
	assert.NoError(err)
	assert.Equal("datastax/hcd:1.0.0-ubi", path)

	path, err = registry.GetCassandraImage("cassandra", "4.1.4")
	assert.NoError(err)
	assert.Equal("ghcr.io/k8ssandra/cass-management-api:4.1.4-ubi", path)
}

func TestImageConfigParsingV2(t *testing.T) {
	assert := require.New(t)
	imageConfigFile := filepath.Join("..", "..", "tests", "testdata", "image_config_parsing_v2.yaml")
	content, err := os.ReadFile(imageConfigFile)
	assert.NoError(err)

	registry, err := NewImageRegistryV2(content)
	assert.NoError(err, "imageConfig parsing should succeed")

	// Verify some default values are set
	imageConfig := &registry.(*imageRegistryV1Beta2).imageConfig
	assert.NotNil(imageConfig)
	assert.NotNil(imageConfig.Images)
	assert.True(registry.GetSystemLoggerImage() == "localhost:5000/k8ssandra/system-logger:latest" || registry.GetSystemLoggerImage() == "localhost:5000/k8ssandra/system-logger:v0.2.1")
	assert.Contains(registry.GetConfigBuilderImage(), "datastax/cass-config-builder:")
	assert.Contains(registry.GetClientImage(), "k8ssandra/k8ssandra-client:")

	assert.Equal("cr.k8ssandra.io", imageConfig.Types["cassandra"].Registry)
	assert.Equal("k8ssandra", imageConfig.Types["cassandra"].Repository)
	assert.Equal("cr.dtsx.io", imageConfig.Types["dse"].Registry)
	assert.Equal("datastax", imageConfig.Types["dse"].Repository)

	assert.Equal("localhost:5000", imageConfig.Defaults.Registry)
	assert.Equal(corev1.PullAlways, imageConfig.Defaults.PullPolicy)
	assert.Equal("my-secret-pull-registry", imageConfig.Defaults.PullSecrets[0])

	path, err := registry.GetCassandraImage("dse", "6.8.43")
	assert.NoError(err)
	assert.Equal("localhost:5000/cr.dtsx.io/datastax/dse-server:6.8.43-ubi8", path)

	path, err = registry.GetCassandraImage("dse", "6.8.999")
	assert.NoError(err)
	assert.Equal("localhost:5000/datastax/dse-server-prototype:latest", path)

	path, err = registry.GetCassandraImage("cassandra", "4.0.0")
	assert.NoError(err)
	assert.Equal("localhost:5000/k8ssandra/cassandra-ubi:latest", path)
}

func TestExtendedImageConfigParsingV2(t *testing.T) {
	assert := require.New(t)
	imageConfigFile := filepath.Join("..", "..", "tests", "testdata", "image_config_parsing_more_options_v2.yaml")
	content, err := os.ReadFile(imageConfigFile)
	assert.NoError(err)

	registry, err := NewImageRegistryV2(content)
	assert.NoError(err, "imageConfig parsing should succeed")

	// Verify some default values are set
	assert.NotNil(&registry.(*imageRegistryV1Beta2).imageConfig)
	assert.NotNil(&registry.(*imageRegistryV1Beta2).imageConfig.Images)
	assert.NotNil(&registry.(*imageRegistryV1Beta2).imageConfig.Types)

	medusaImage := registry.GetImage("medusa")
	assert.Equal("localhost:5005/enterprise/medusa:latest", medusaImage)
	reaperImage := registry.GetImage("reaper")
	assert.Equal("localhost:5000/enterprise/reaper:latest", reaperImage)

	assert.Equal(corev1.PullAlways, registry.GetImagePullPolicy(configv1beta2.SystemLoggerImageComponent))
	assert.Equal(corev1.PullIfNotPresent, registry.GetImagePullPolicy("cassandra"))
}

func TestDefaultRepositoriesV2(t *testing.T) {
	assert := assert.New(t)
	yamlConfig := `
apiVersion: config.k8ssandra.io/v1beta2
kind: ImageConfig
types:
  cassandra:
    repository: k8ssandra
    name: cass-management-api
  dse:
    repository: datastax
    name: dse-mgmtapi-6_8
`
	registry, err := newTestImageRegistryV2([]byte(yamlConfig))
	require.NoError(t, err)

	path, err := registry.GetCassandraImage("cassandra", "4.0.1")
	assert.NoError(err)
	assert.Equal("k8ssandra/cass-management-api:4.0.1", path)

	path, err = registry.GetCassandraImage("dse", "6.8.17")
	assert.NoError(err)
	assert.Equal("datastax/dse-mgmtapi-6_8:6.8.17", path)
}

func TestPullPolicyOverrideV2(t *testing.T) {
	assert := require.New(t)
	imageConfigFile := filepath.Join("..", "..", "tests", "testdata", "image_config_parsing_v2.yaml")
	content, err := os.ReadFile(imageConfigFile)
	assert.NoError(err)

	registry, err := NewImageRegistryV2(content)
	assert.NoError(err, "imageConfig parsing should succeed")

	secrets := registry.GetImagePullSecrets()
	assert.Equal(1, len(secrets))
	assert.Equal("my-secret-pull-registry", secrets[0])
}

func TestRepositoryAndNamespaceOverrideV2(t *testing.T) {
	assert := assert.New(t)
	yamlConfig := `
apiVersion: config.k8ssandra.io/v1beta2
kind: ImageConfig
types:
  dse:
    repository: datastax
    name: dse-mgmtapi-6_8
`
	registry, err := newTestImageRegistryV2([]byte(yamlConfig))
	require.NoError(t, err)

	path, err := registry.GetCassandraImage("dse", "6.8.44")
	assert.NoError(err)
	assert.Equal("datastax/dse-mgmtapi-6_8:6.8.44", path)

	// With registry
	yamlConfig = `
apiVersion: config.k8ssandra.io/v1beta2
kind: ImageConfig
defaults:
  registry: ghcr.io
types:
  dse:
    repository: datastax
    name: dse-mgmtapi-6_8
`
	registry, err = newTestImageRegistryV2([]byte(yamlConfig))
	require.NoError(t, err)
	path, err = registry.GetCassandraImage("dse", "6.8.44")
	assert.NoError(err)
	assert.Equal("ghcr.io/datastax/dse-mgmtapi-6_8:6.8.44", path)

	// with namespace override
	yamlConfig = `
apiVersion: config.k8ssandra.io/v1beta2
kind: ImageConfig
defaults:
  registry: ghcr.io
  namespace: enterprise
types:
  dse:
    repository: datastax
    name: dse-mgmtapi-6_8
`
	registry, err = newTestImageRegistryV2([]byte(yamlConfig))
	require.NoError(t, err)
	path, err = registry.GetCassandraImage("dse", "6.8.44")
	assert.NoError(err)
	assert.Equal("ghcr.io/enterprise/dse-mgmtapi-6_8:6.8.44", path)

	// without registry, with namespace
	yamlConfig = `
apiVersion: config.k8ssandra.io/v1beta2
kind: ImageConfig
defaults:
  namespace: enterprise
types:
  dse:
    repository: datastax
    name: dse-mgmtapi-6_8
`
	registry, err = newTestImageRegistryV2([]byte(yamlConfig))
	require.NoError(t, err)
	path, err = registry.GetCassandraImage("dse", "6.8.44")
	assert.NoError(err)
	assert.Equal("enterprise/dse-mgmtapi-6_8:6.8.44", path)

	// with full repo path in type
	yamlConfig = `
apiVersion: config.k8ssandra.io/v1beta2
kind: ImageConfig
types:
  dse:
    repository: cr.dtsx.io/datastax
    name: dse-mgmtapi-6_8
`
	registry, err = newTestImageRegistryV2([]byte(yamlConfig))
	require.NoError(t, err)
	path, err = registry.GetCassandraImage("dse", "6.8.44")
	assert.NoError(err)
	assert.Equal("cr.dtsx.io/datastax/dse-mgmtapi-6_8:6.8.44", path)

	// with full repo path and namespace override
	yamlConfig = `
apiVersion: config.k8ssandra.io/v1beta2
kind: ImageConfig
defaults:
  namespace: internal
types:
  dse:
    repository: cr.dtsx.io/datastax
    name: dse-mgmtapi-6_8
`
	registry, err = newTestImageRegistryV2([]byte(yamlConfig))
	require.NoError(t, err)
	path, err = registry.GetCassandraImage("dse", "6.8.44")
	assert.NoError(err)
	assert.Equal("cr.dtsx.io/internal/dse-mgmtapi-6_8:6.8.44", path)

	// with full repo path and empty namespace override
	yamlConfig = `
apiVersion: config.k8ssandra.io/v1beta2
kind: ImageConfig
defaults:
  namespace: ""
types:
  dse:
    repository: cr.dtsx.io/datastax
    name: dse-mgmtapi-6_8
`
	registry, err = newTestImageRegistryV2([]byte(yamlConfig))
	require.NoError(t, err)
	path, err = registry.GetCassandraImage("dse", "6.8.44")
	assert.NoError(err)
	assert.Equal("cr.dtsx.io/dse-mgmtapi-6_8:6.8.44", path)
}

func TestImageConfigByteParsingV2(t *testing.T) {
	require := require.New(t)
	yamlConfig := `
apiVersion: config.k8ssandra.io/v1beta2
kind: ImageConfig
defaults:
  registry: localhost:5000
images:
  system-logger:
    name: k8ssandra/system-logger
    tag: "next"
  config-builder:
    name: k8ssandra/config-builder
    tag: "next"
types:
  cassandra:
    repository: k8ssandra
    name: management-api
    suffix: "-next"
`
	registry, err := newTestImageRegistryV2([]byte(yamlConfig))
	require.NoError(err)

	imageConfig := &registry.(*imageRegistryV1Beta2).imageConfig

	// Some sanity checks
	require.Equal("localhost:5000", imageConfig.Defaults.Registry)
	require.Equal("k8ssandra/system-logger:next", registry.GetSystemLoggerImage())
	require.Equal("k8ssandra/config-builder:next", registry.GetConfigBuilderImage())
	cassandraImage, err := registry.GetCassandraImage("cassandra", "4.0.0")
	require.NoError(err)
	require.Equal("localhost:5000/k8ssandra/management-api:4.0.0-next", cassandraImage)
}
