// Copyright DataStax, Inc.
// Please see the included license file for details.

package images

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

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

func TestImageConfigParsing(t *testing.T) {
	assert := assert.New(t)
	imageConfigFile := filepath.Join("..", "..", "config", "manager", "image_config.yaml")
	err := ParseImageConfig(imageConfigFile)
	assert.NoError(err, "imageConfig parsing should succeed")

	// Verify some default values are set
	assert.NotNil(GetImageConfig())
	assert.NotNil(GetImageConfig().Images)
	assert.True(strings.HasPrefix(GetImageConfig().Images.SystemLogger, "k8ssandra/system-logger:"))
	assert.True(strings.HasPrefix(GetImageConfig().Images.ConfigBuilder, "datastax/cass-config-builder:"))
}
