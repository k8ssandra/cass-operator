// Copyright DataStax, Inc.
// Please see the included license file for details.

package images

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	configv1beta1 "github.com/k8ssandra/cass-operator/apis/config/v1beta1"
)

func TestDefaultRegistryOverride(t *testing.T) {
	assert := assert.New(t)
	imageConfig = configv1beta1.ImageConfig{}
	imageConfig.ImageRegistry = "localhost:5000"
	imageConfig.Images = &configv1beta1.Images{}
	imageConfig.DefaultImages = &configv1beta1.DefaultImages{}
	imageConfig.Images.ConfigBuilder = "k8ssandra/config-builder-temp:latest"

	image := GetConfigBuilderImage()
	assert.True(strings.HasPrefix(image, "localhost:5000"))

	image, err := GetCassandraImage("cassandra", "4.0.6")
	assert.NoError(err)
	assert.Equal("localhost:5000/k8ssandra/cass-management-api:4.0.6", image)
}

func TestCassandraOverride(t *testing.T) {
	assert := assert.New(t)

	customImageName := "my-custom-image:4.0.0"

	imageConfig = configv1beta1.ImageConfig{}
	imageConfig.Images = &configv1beta1.Images{}
	imageConfig.DefaultImages = &configv1beta1.DefaultImages{}

	cassImage, err := GetCassandraImage("cassandra", "4.0.0")
	assert.NoError(err, "getting Cassandra image should succeed")
	assert.Equal("k8ssandra/cass-management-api:4.0.0", cassImage)

	imageConfig.Images.CassandraVersions = map[string]string{
		"4.0.0": customImageName,
	}

	cassImage, err = GetCassandraImage("cassandra", "4.0.0")
	assert.NoError(err, "getting Cassandra image with override should succeed")
	assert.Equal(customImageName, cassImage)

	imageConfig.ImageRegistry = "ghcr.io"
	cassImage, err = GetCassandraImage("cassandra", "4.0.0")
	assert.NoError(err, "getting Cassandra image with overrides should succeed")
	assert.Equal(fmt.Sprintf("ghcr.io/%s", customImageName), cassImage)

	customImageNamespace := "modified"
	imageConfig.Images.CassandraVersions = map[string]string{
		"4.0.0": fmt.Sprintf("us-docker.pkg.dev/%s/cass-management-api:4.0.0", customImageNamespace),
	}
	imageConfig.Images.DSEVersions = map[string]string{
		"6.8.0": fmt.Sprintf("us-docker.pkg.dev/%s/dse-mgmtapi-6_8:6.8.0", customImageNamespace),
	}
	imageConfig.Images.HCDVersions = map[string]string{
		"1.0.0": fmt.Sprintf("us-docker.pkg.dev/%s/hcd:1.0.0", customImageNamespace),
	}

	cassImage, err = GetCassandraImage("cassandra", "4.0.0")
	assert.NoError(err, "getting Cassandra image with overrides should succeed")
	assert.Equal("ghcr.io/modified/cass-management-api:4.0.0", cassImage)

	cassImage, err = GetCassandraImage("dse", "6.8.0")
	assert.NoError(err, "getting Cassandra image with overrides should succeed")
	assert.Equal("ghcr.io/modified/dse-mgmtapi-6_8:6.8.0", cassImage)

	cassImage, err = GetCassandraImage("hcd", "1.0.0")
	assert.NoError(err, "getting Cassandra image with overrides should succeed")
	assert.Equal("ghcr.io/modified/hcd:1.0.0", cassImage)
}

func TestDefaultImageConfigParsing(t *testing.T) {
	assert := require.New(t)
	imageConfigFile := filepath.Join("..", "..", "config", "manager", "image_config.yaml")
	err := ParseImageConfig(imageConfigFile)
	assert.NoError(err, "imageConfig parsing should succeed")

	// Verify some default values are set
	assert.NotNil(GetImageConfig())
	assert.NotNil(GetImageConfig().Images)
	assert.True(strings.Contains(GetImageConfig().Images.SystemLogger, "k8ssandra/system-logger:"))
	assert.True(strings.Contains(GetImageConfig().Images.ConfigBuilder, "datastax/cass-config-builder:"))
	assert.True(strings.Contains(GetImageConfig().Images.Client, "k8ssandra/k8ssandra-client:"))

	assert.Equal("ghcr.io/k8ssandra/cass-management-api", GetImageConfig().DefaultImages.ImageComponents[configv1beta1.CassandraImageComponent].Repository)
	assert.Equal("datastax/dse-mgmtapi-6_8", GetImageConfig().DefaultImages.ImageComponents[configv1beta1.DSEImageComponent].Repository)

	path, err := GetCassandraImage("dse", "6.8.47")
	assert.NoError(err)
	assert.Equal("datastax/dse-mgmtapi-6_8:6.8.47-ubi8", path)

	path, err = GetCassandraImage("hcd", "1.0.0")
	assert.NoError(err)
	assert.Equal("datastax/hcd:1.0.0-ubi", path)

	path, err = GetCassandraImage("cassandra", "4.1.4")
	assert.NoError(err)
	assert.Equal("ghcr.io/k8ssandra/cass-management-api:4.1.4-ubi", path)
}

func TestImageConfigParsing(t *testing.T) {
	assert := require.New(t)
	imageConfigFile := filepath.Join("..", "..", "tests", "testdata", "image_config_parsing.yaml")
	err := ParseImageConfig(imageConfigFile)
	assert.NoError(err, "imageConfig parsing should succeed")

	// Verify some default values are set
	assert.NotNil(GetImageConfig())
	assert.NotNil(GetImageConfig().Images)
	assert.True(strings.HasPrefix(GetImageConfig().Images.SystemLogger, "k8ssandra/system-logger:"))
	assert.True(strings.HasPrefix(GetImageConfig().Images.ConfigBuilder, "datastax/cass-config-builder:"))
	assert.True(strings.Contains(GetImageConfig().Images.Client, "k8ssandra/k8ssandra-client:"))

	assert.Equal("cr.k8ssandra.io/k8ssandra/cass-management-api", GetImageConfig().DefaultImages.ImageComponents[configv1beta1.CassandraImageComponent].Repository)
	assert.Equal("cr.dtsx.io/datastax/dse-mgmtapi-6_8", GetImageConfig().DefaultImages.ImageComponents[configv1beta1.DSEImageComponent].Repository)

	assert.Equal("localhost:5000", GetImageConfig().ImageRegistry)
	assert.Equal(corev1.PullAlways, GetImageConfig().ImagePullPolicy)
	assert.Equal("my-secret-pull-registry", GetImageConfig().ImagePullSecret.Name)

	path, err := GetCassandraImage("dse", "6.8.43")
	assert.NoError(err)
	assert.Equal("localhost:5000/datastax/dse-mgmtapi-6_8:6.8.43-ubi8", path)

	path, err = GetCassandraImage("dse", "6.8.999")
	assert.NoError(err)
	assert.Equal("localhost:5000/datastax/dse-server-prototype:latest", path)

	path, err = GetCassandraImage("cassandra", "4.0.0")
	assert.NoError(err)
	assert.Equal("localhost:5000/k8ssandra/cassandra-ubi:latest", path)
}

func TestExtendedImageConfigParsing(t *testing.T) {
	assert := require.New(t)
	imageConfigFile := filepath.Join("..", "..", "tests", "testdata", "image_config_parsing_more_options.yaml")
	err := ParseImageConfig(imageConfigFile)
	assert.NoError(err, "imageConfig parsing should succeed")

	// Verify some default values are set
	assert.NotNil(GetImageConfig())
	assert.NotNil(GetImageConfig().Images)
	assert.NotNil(GetImageConfig().DefaultImages)

	medusaImage := GetImage("medusa")
	assert.Equal("localhost:5005/enterprise/medusa:latest", medusaImage)
	reaperImage := GetImage("reaper")
	assert.Equal("localhost:5000/enterprise/reaper:latest", reaperImage)

	assert.Equal(corev1.PullAlways, GetImagePullPolicy(configv1beta1.SystemLoggerImageComponent))
	assert.Equal(corev1.PullIfNotPresent, GetImagePullPolicy(configv1beta1.CassandraImageComponent))
}

func TestDefaultRepositories(t *testing.T) {
	assert := assert.New(t)
	imageConfig = configv1beta1.ImageConfig{}
	imageConfig.Images = &configv1beta1.Images{}
	imageConfig.DefaultImages = &configv1beta1.DefaultImages{}

	path, err := GetCassandraImage("cassandra", "4.0.1")
	assert.NoError(err)
	assert.Equal("k8ssandra/cass-management-api:4.0.1", path)

	path, err = GetCassandraImage("dse", "6.8.17")
	assert.NoError(err)
	assert.Equal("datastax/dse-mgmtapi-6_8:6.8.17", path)
}

func TestOssValidVersions(t *testing.T) {
	assert := assert.New(t)
	assert.True(IsOssVersionSupported("4.0.0"))
	assert.True(IsOssVersionSupported("4.1.0"))
	assert.False(IsOssVersionSupported("4.0"))
	assert.False(IsOssVersionSupported("4.1"))
	assert.False(IsOssVersionSupported("6.8.0"))
}

func TestPullPolicyOverride(t *testing.T) {
	assert := require.New(t)
	imageConfigFile := filepath.Join("..", "..", "tests", "testdata", "image_config_parsing.yaml")
	err := ParseImageConfig(imageConfigFile)
	assert.NoError(err, "imageConfig parsing should succeed")

	podSpec := &corev1.PodSpec{}
	AddDefaultRegistryImagePullSecrets(podSpec)
	assert.Equal(1, len(podSpec.ImagePullSecrets))
	assert.Equal("my-secret-pull-registry", podSpec.ImagePullSecrets[0].Name)
}

func TestRepositoryAndNamespaceOverride(t *testing.T) {
	assert := assert.New(t)
	imageConfig = configv1beta1.ImageConfig{}
	imageConfig.Images = &configv1beta1.Images{}
	imageConfig.DefaultImages = &configv1beta1.DefaultImages{}

	path, err := GetCassandraImage("dse", "6.8.44")
	assert.NoError(err)
	assert.Equal("datastax/dse-mgmtapi-6_8:6.8.44", path)

	imageConfig.ImageRegistry = "ghcr.io"
	path, err = GetCassandraImage("dse", "6.8.44")
	assert.NoError(err)
	assert.Equal("ghcr.io/datastax/dse-mgmtapi-6_8:6.8.44", path)

	imageConfig.ImageNamespace = ptr.To[string]("enterprise")
	path, err = GetCassandraImage("dse", "6.8.44")
	assert.NoError(err)
	assert.Equal("ghcr.io/enterprise/dse-mgmtapi-6_8:6.8.44", path)

	imageConfig = configv1beta1.ImageConfig{}
	imageConfig.Images = &configv1beta1.Images{}
	imageConfig.DefaultImages = &configv1beta1.DefaultImages{}
	imageConfig.ImageNamespace = ptr.To[string]("enterprise")
	path, err = GetCassandraImage("dse", "6.8.44")
	assert.NoError(err)
	assert.Equal("enterprise/dse-mgmtapi-6_8:6.8.44", path)

	imageConfig = configv1beta1.ImageConfig{}
	imageConfig.Images = &configv1beta1.Images{}
	imageConfig.DefaultImages = &configv1beta1.DefaultImages{
		ImageComponents: map[string]configv1beta1.ImageComponent{
			configv1beta1.DSEImageComponent: {
				Repository: "cr.dtsx.io/datastax/dse-mgmtapi-6_8",
			},
		},
	}
	path, err = GetCassandraImage("dse", "6.8.44")
	assert.NoError(err)
	assert.Equal("cr.dtsx.io/datastax/dse-mgmtapi-6_8:6.8.44", path)

	imageConfig.ImageNamespace = ptr.To[string]("internal")
	path, err = GetCassandraImage("dse", "6.8.44")
	assert.NoError(err)
	assert.Equal("cr.dtsx.io/internal/dse-mgmtapi-6_8:6.8.44", path)

	imageConfig.ImageNamespace = ptr.To[string]("")
	path, err = GetCassandraImage("dse", "6.8.44")
	assert.NoError(err)
	assert.Equal("cr.dtsx.io/dse-mgmtapi-6_8:6.8.44", path)
}

func TestImageConfigByteParsing(t *testing.T) {
	require := require.New(t)
	imageConfig := configv1beta1.ImageConfig{
		Images: &configv1beta1.Images{
			SystemLogger:  "k8ssandra/system-logger:next",
			ConfigBuilder: "k8ssandra/config-builder:next",
		},
		DefaultImages: &configv1beta1.DefaultImages{
			ImageComponents: configv1beta1.ImageComponents{
				configv1beta1.CassandraImageComponent: configv1beta1.ImageComponent{
					Repository: "k8ssandra/management-api:next",
				},
			},
		},
		ImagePolicy: configv1beta1.ImagePolicy{
			ImageRegistry: "localhost:5000",
		},
	}

	b, err := json.Marshal(imageConfig)
	require.NoError(err)
	require.True(len(b) > 1)

	parsedImageConfig, err := LoadImageConfig(b)
	require.NoError(err)

	// Some sanity checks
	require.Equal("localhost:5000", parsedImageConfig.ImageRegistry)
	require.Equal(imageConfig.Images.SystemLogger, parsedImageConfig.Images.SystemLogger)
	require.Equal(imageConfig.Images.ConfigBuilder, parsedImageConfig.Images.ConfigBuilder)
	require.Equal(imageConfig.DefaultImages.ImageComponents[configv1beta1.CassandraImageComponent].Repository, parsedImageConfig.DefaultImages.ImageComponents[configv1beta1.CassandraImageComponent].Repository)
	require.Equal(imageConfig.ImageRegistry, parsedImageConfig.ImageRegistry)

	// And now check that images.GetImageConfig() works also..
	require.True(reflect.DeepEqual(parsedImageConfig, GetImageConfig()))
}
