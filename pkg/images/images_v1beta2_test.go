package images

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	api "github.com/k8ssandra/cass-operator/apis/config/v1beta2"
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
	imageConfigFile := filepath.Join("..", "..", "tests", "testdata", "image_config_parsing_v2.yaml")
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
	assert.Equal("cr.dtsx.io/datastax/dse-mgmtapi-6_8:6.8.47-ubi", path)

	path, err = registry.GetCassandraImage("hcd", "1.0.0")
	assert.NoError(err)
	assert.Equal("docker.io/datastax/hcd:v1.0.0-ubi", path) // This is to test the Prefix pattern also

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
	assert.Equal("ghcr.io/k8ssandra/system-logger:latest", registry.GetSystemLoggerImage())
	assert.Contains(registry.GetConfigBuilderImage(), "datastax/cass-config-builder:")
	assert.Contains(registry.GetClientImage(), "k8ssandra/k8ssandra-client:")

	assert.Equal("ghcr.io", imageConfig.Types["cassandra"].Registry)
	assert.Equal("k8ssandra", imageConfig.Types["cassandra"].Repository)
	assert.Equal("cr.dtsx.io", imageConfig.Types["dse"].Registry)
	assert.Equal("datastax", imageConfig.Types["dse"].Repository)

	assert.Equal("docker.io", *imageConfig.Defaults.Registry)
	assert.Equal(corev1.PullAlways, imageConfig.Defaults.PullPolicy)
	assert.Equal("my-secret-pull-registry", imageConfig.Defaults.PullSecrets[0])

	path, err := registry.GetCassandraImage("dse", "6.8.43")
	assert.NoError(err)
	assert.Equal("cr.dtsx.io/datastax/dse-mgmtapi-6_8:6.8.43-ubi", path)
}

func TestExtendedImageConfigParsingV2(t *testing.T) {
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
	assert.NotNil(imageConfig.Types)
	imageConfig.Overrides = &api.ImagePolicy{
		Repository: ptr.To("enterprise"),
	}
	imageConfig.Defaults.Registry = ptr.To("localhost:5005")

	medusaImage := registry.GetImage("medusa")
	assert.Equal("localhost:5005/enterprise/cassandra-medusa:latest", medusaImage)
	reaperImage := registry.GetImage("reaper")
	assert.Equal("localhost:5005/enterprise/cassandra-reaper:latest", reaperImage)

	assert.Equal(corev1.PullAlways, registry.GetImagePullPolicy(api.SystemLoggerImageComponent))
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

	yamlConfig = `
apiVersion: config.k8ssandra.io/v1beta2
kind: ImageConfig
defaults:
  registry: ghcr.io
overrides:
  repository: enterprise
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

	yamlConfig = `
apiVersion: config.k8ssandra.io/v1beta2
kind: ImageConfig
defaults:
  repository: enterprise
types:
  dse:
    repository: datastax
    name: dse-mgmtapi-6_8
`
	registry, err = newTestImageRegistryV2([]byte(yamlConfig))
	require.NoError(t, err)
	path, err = registry.GetCassandraImage("dse", "6.8.44")
	assert.NoError(err)
	assert.Equal("datastax/dse-mgmtapi-6_8:6.8.44", path)

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

	yamlConfig = `
apiVersion: config.k8ssandra.io/v1beta2
kind: ImageConfig
overrides:
  repository: internal
types:
  dse:
    registry: cr.dtsx.io
    repository: datastax
    name: dse-mgmtapi-6_8
`
	registry, err = newTestImageRegistryV2([]byte(yamlConfig))
	require.NoError(t, err)
	path, err = registry.GetCassandraImage("dse", "6.8.44")
	assert.NoError(err)
	assert.Equal("cr.dtsx.io/internal/dse-mgmtapi-6_8:6.8.44", path)

	yamlConfig = `
apiVersion: config.k8ssandra.io/v1beta2
kind: ImageConfig
overrides:
  repository: ""
types:
  dse:
    registry: cr.dtsx.io
    repository: "datastax"
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
    repository: k8ssandra
    name: system-logger
    tag: "next"
  config-builder:
    repository: k8ssandra
    name: config-builder
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
	require.Equal("localhost:5000", *imageConfig.Defaults.Registry)
	require.Equal("localhost:5000/k8ssandra/system-logger:next", registry.GetSystemLoggerImage())
	require.Equal("localhost:5000/k8ssandra/config-builder:next", registry.GetConfigBuilderImage())
	cassandraImage, err := registry.GetCassandraImage("cassandra", "4.0.0")
	require.NoError(err)
	require.Equal("localhost:5000/k8ssandra/management-api:4.0.0-next", cassandraImage)
}

func TestNewImageRegistryFromClient_UsesConfigMapV2(t *testing.T) {
	require := require.New(t)

	// Load v2 ImageConfig content from testdata
	imageConfigFile := filepath.Join("..", "..", "tests", "testdata", "image_config_parsing_v2.yaml")
	content, err := os.ReadFile(imageConfigFile)
	require.NoError(err)

	// Build a fake client with a labeled ConfigMap containing the v2 config
	scheme := runtime.NewScheme()
	require.NoError(corev1.AddToScheme(scheme))

	cm := &corev1.ConfigMap{}
	cm.Name = "k8ssandra-image-config"
	cm.Namespace = "default"
	cm.Labels = map[string]string{"k8ssandra.io/config": "image"}
	cm.Data = map[string]string{"image_config.yaml": string(content)}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	// The function should return a v2 registry parsed from the ConfigMap
	ctx := context.Background()
	reg, err := NewImageRegistryFromClient(ctx, c)
	require.NoError(err)

	// Validate a few fields parsed from the v2 config
	require.Equal("ghcr.io/k8ssandra/system-logger:latest", reg.GetSystemLoggerImage())
}
