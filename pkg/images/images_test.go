// Copyright DataStax, Inc.
// Please see the included license file for details.

package images

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"

	configv1beta1 "github.com/k8ssandra/cass-operator/apis/config/v1beta1"
)

func TestDefaultRegistryOverride(t *testing.T) {
	imageConfig = &configv1beta1.ImageConfig{}
	imageConfig.ImageRegistry = "localhost:5000"
	imageConfig.Images = &configv1beta1.Images{}
	imageConfig.Images.ConfigBuilder = "k8ssandra/config-builder-temp:latest"

	image := GetConfigBuilderImage()
	assert.True(t, strings.HasPrefix(image, "localhost:5000/"))
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
	assert.Equal("datastax/dse-server", GetImageConfig().DefaultImages.DSEImageComponent.Repository)

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
	assert.Equal(v1.PullAlways, GetImageConfig().ImagePullPolicy)
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
