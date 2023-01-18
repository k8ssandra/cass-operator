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

	configv1beta1 "github.com/k8ssandra/cass-operator/apis/config/v1beta1"
)

func TestDefaultRegistryOverride(t *testing.T) {
	assert := assert.New(t)
	imageConfig = &configv1beta1.ImageConfig{}
	imageConfig.ImageRegistry = "localhost:5000"
	imageConfig.Images = &configv1beta1.Images{}
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

	imageConfig = &configv1beta1.ImageConfig{}
	imageConfig.Images = &configv1beta1.Images{}

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

	customImageWithOrg := "k8ssandra/cass-management-api:4.0.0"
	imageConfig.Images.CassandraVersions = map[string]string{
		"4.0.0": fmt.Sprintf("us-docker.pkg.dev/%s", customImageWithOrg),
	}

	cassImage, err = GetCassandraImage("cassandra", "4.0.0")
	assert.NoError(err, "getting Cassandra image with overrides should succeed")
	assert.Equal(fmt.Sprintf("ghcr.io/%s", customImageWithOrg), cassImage)
}

func TestDefaultImageConfigParsing(t *testing.T) {
	assert := require.New(t)
	imageConfigFile := filepath.Join("..", "..", "config", "manager", "image_config.yaml")
	err := ParseImageConfig(imageConfigFile)
	assert.NoError(err, "imageConfig parsing should succeed")

	// Verify some default values are set
	assert.NotNil(GetImageConfig())
	assert.NotNil(GetImageConfig().Images)
	assert.True(strings.HasPrefix(GetImageConfig().Images.SystemLogger, "k8ssandra/system-logger:"))
	assert.True(strings.HasPrefix(GetImageConfig().Images.ConfigBuilder, "datastax/cass-config-builder:"))

	assert.Equal("k8ssandra/cass-management-api", GetImageConfig().DefaultImages.CassandraImageComponent.Repository)
	assert.Equal("datastax/dse-mgmtapi-6_8", GetImageConfig().DefaultImages.DSEImageComponent.Repository)

	path, err := GetCassandraImage("dse", "6.8.17")
	assert.NoError(err)
	assert.Equal("datastax/dse-server:6.8.17-ubi7", path)
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

	assert.Equal("k8ssandra/cass-management-api", GetImageConfig().DefaultImages.CassandraImageComponent.Repository)
	assert.Equal("datastax/dse-server", GetImageConfig().DefaultImages.DSEImageComponent.Repository)

	assert.Equal("localhost:5000", GetImageConfig().ImageRegistry)
	assert.Equal(corev1.PullAlways, GetImageConfig().ImagePullPolicy)
	assert.Equal("my-secret-pull-registry", GetImageConfig().ImagePullSecret.Name)

	path, err := GetCassandraImage("dse", "6.8.17")
	assert.NoError(err)
	assert.Equal("localhost:5000/datastax/dse-server:6.8.17-ubi7", path)

	path, err = GetCassandraImage("dse", "6.8.999")
	assert.NoError(err)
	assert.Equal("localhost:5000/datastax/dse-server-prototype:latest", path)

	path, err = GetCassandraImage("cassandra", "4.0.0")
	assert.NoError(err)
	assert.Equal("localhost:5000/k8ssandra/cassandra-ubi:latest", path)
}

func TestDefaultRepositories(t *testing.T) {
	assert := assert.New(t)
	imageConfig = &configv1beta1.ImageConfig{}
	imageConfig.Images = &configv1beta1.Images{}

	path, err := GetCassandraImage("cassandra", "4.0.1")
	assert.NoError(err)
	assert.Equal("k8ssandra/cass-management-api:4.0.1", path)

	path, err = GetCassandraImage("dse", "6.8.17")
	assert.NoError(err)
	assert.Equal("datastax/dse-server:6.8.17", path)
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
	added := AddDefaultRegistryImagePullSecrets(podSpec)
	assert.True(added)
	assert.Equal(1, len(podSpec.ImagePullSecrets))
	assert.Equal("my-secret-pull-registry", podSpec.ImagePullSecrets[0].Name)
}

func TestImageConfigByteParsing(t *testing.T) {
	require := require.New(t)
	imageConfig := configv1beta1.ImageConfig{
		Images: &configv1beta1.Images{
			SystemLogger:  "k8ssandra/system-logger:next",
			ConfigBuilder: "k8ssandra/config-builder:next",
		},
		DefaultImages: &configv1beta1.DefaultImages{
			CassandraImageComponent: configv1beta1.ImageComponent{
				Repository: "k8ssandra/management-api:next",
			},
		},
		ImageRegistry: "localhost:5000",
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
	require.Equal(imageConfig.DefaultImages.CassandraImageComponent.Repository, parsedImageConfig.DefaultImages.CassandraImageComponent.Repository)
	require.Equal(imageConfig.ImageRegistry, parsedImageConfig.ImageRegistry)

	// And now check that images.GetImageConfig() works also..
	require.True(reflect.DeepEqual(parsedImageConfig, GetImageConfig()))
}
