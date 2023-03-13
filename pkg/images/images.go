// Copyright DataStax, Inc.
// Please see the included license file for details.

package images

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	configv1beta1 "github.com/k8ssandra/cass-operator/apis/config/v1beta1"
	"github.com/pkg/errors"
)

var (
	imageConfig configv1beta1.ImageConfig
	scheme      = runtime.NewScheme()
)

const (
	ValidDseVersionRegexp      = "(6\\.[89]\\.\\d+)"
	ValidHcdVersionRegexp      = "(1\\.\\d+\\.\\d+)"
	ValidOssVersionRegexp      = "(3\\.11\\.\\d+)|(4\\.\\d+\\.\\d+)|(5\\.\\d+\\.\\d+)"
	DefaultCassandraRepository = "k8ssandra/cass-management-api"
	DefaultDSERepository       = "datastax/dse-mgmtapi-6_8"
	DefaultHCDRepository       = "datastax/hcd"
)

func init() {
	utilruntime.Must(configv1beta1.AddToScheme(scheme))
}

func ParseImageConfig(imageConfigFile string) error {
	content, err := os.ReadFile(imageConfigFile)
	if err != nil {
		return fmt.Errorf("could not read file at %s", imageConfigFile)
	}

	if _, err = LoadImageConfig(content); err != nil {
		err = errors.Wrapf(err, "unable to load imageConfig from %s", imageConfigFile)
		return err
	}

	return nil
}

func LoadImageConfig(content []byte) (*configv1beta1.ImageConfig, error) {
	codecs := serializer.NewCodecFactory(scheme)

	parsedImageConfig := &configv1beta1.ImageConfig{}
	if err := runtime.DecodeInto(codecs.UniversalDecoder(), content, parsedImageConfig); err != nil {
		return nil, fmt.Errorf("could not decode file into runtime.Object: %v", err)
	}

	imageConfig = *parsedImageConfig

	return parsedImageConfig, nil
}

func IsDseVersionSupported(version string) bool {
	validVersions := regexp.MustCompile(ValidDseVersionRegexp)
	return validVersions.MatchString(version)
}

func IsOssVersionSupported(version string) bool {
	validVersions := regexp.MustCompile(ValidOssVersionRegexp)
	return validVersions.MatchString(version)
}

func IsHCDVersionSupported(version string) bool {
	validVersions := regexp.MustCompile(ValidHcdVersionRegexp)
	return validVersions.MatchString(version)
}

func stripRegistry(image string) string {
	comps := strings.Split(image, "/")

	if len(comps) > 1 && (strings.Contains(comps[0], ".") || strings.Contains(comps[0], ":")) {
		return strings.Join(comps[1:], "/")
	} else {
		return image
	}
}

func applyDefaultRegistryOverride(customRegistry, image string) string {
	customRegistry = strings.TrimSuffix(customRegistry, "/")

	if customRegistry == "" {
		return image
	} else {
		imageNoRegistry := stripRegistry(image)
		return fmt.Sprintf("%s/%s", customRegistry, imageNoRegistry)
	}
}

func getRegistryOverride(imageType string) string {
	customRegistry := ""
	defaults := GetImageConfig().DefaultImages
	if defaults != nil {
		if component, found := defaults.ImageComponents[imageType]; found {
			customRegistry = component.ImageRegistry
		}
	}

	defaultRegistry := GetImageConfig().ImageRegistry

	if customRegistry != "" {
		return customRegistry
	}

	return defaultRegistry
}

func applyRegistry(imageType, image string) string {
	registry := getRegistryOverride(imageType)

	return applyDefaultRegistryOverride(registry, image)
}

func GetImageConfig() *configv1beta1.ImageConfig {
	// For now, this is static configuration (updated only on start of the pod), even if the actual ConfigMap underneath is updated.
	return &imageConfig
}

func getCassandraContainerImageOverride(serverType, version string) (bool, string) {
	// If there's a mapped volume with overrides in the ConfigMap, use these. Otherwise only calculate
	images := GetImageConfig().Images
	if images != nil {
		if serverType == "dse" {
			if value, found := images.DSEVersions[version]; found {
				return true, value
			}
		}
		if serverType == "cassandra" {
			if value, found := images.CassandraVersions[version]; found {
				return true, value
			}
		}
	}
	return false, ""
}

func getImageComponents(serverType string) (string, string) {
	var defaultPrefix string

	if serverType == "dse" {
		defaultPrefix = DefaultDSERepository
	}
	if serverType == "cassandra" {
		defaultPrefix = DefaultCassandraRepository
	}

	defaults := GetImageConfig().DefaultImages
	if defaults != nil {
		var component configv1beta1.ImageComponent
		if serverType == "dse" {
			component = defaults.ImageComponents[configv1beta1.DSEImageComponent]
		}
		if serverType == "cassandra" {
			component = defaults.ImageComponents[configv1beta1.CassandraImageComponent]
		}
		if serverType == "hcd" {
			component = defaults.ImageComponents[configv1beta1.HCDImageComponent]
		}

		if component.Repository != "" {
			return component.Repository, component.Suffix
		}
	}

	return defaultPrefix, ""
}

func GetCassandraImage(serverType, version string) (string, error) {
	if found, image := getCassandraContainerImageOverride(serverType, version); found {
		return applyRegistry(serverType, image), nil
	}

	switch serverType {
	case "dse":
		if !IsDseVersionSupported(version) {
			return "", fmt.Errorf("server 'dse' and version '%s' do not work together", version)
		}
	case "cassandra":
		if !IsOssVersionSupported(version) {
			return "", fmt.Errorf("server 'cassandra' and version '%s' do not work together", version)
		}
	case "hcd":
		if !IsHCDVersionSupported(version) {
			return "", fmt.Errorf("server 'hcd' and version '%s' do not work together", version)
		}
	default:
		return "", fmt.Errorf("server type '%s' is not supported", serverType)
	}

	prefix, suffix := getImageComponents(serverType)

	return applyRegistry(serverType, fmt.Sprintf("%s:%s%s", prefix, version, suffix)), nil
}

func GetConfiguredImage(imageType, image string) string {
	return applyRegistry(imageType, image)
}

func GetImage(imageType string) string {
	return applyRegistry(imageType, GetImageConfig().Images.Others[imageType])
}

func GetImagePullPolicy(imageType string) corev1.PullPolicy {
	var customPolicy corev1.PullPolicy
	defaults := GetImageConfig().DefaultImages
	if defaults != nil {
		if component, found := defaults.ImageComponents[imageType]; found {
			customPolicy = component.ImagePullPolicy
		}
	}

	defaultOverridePolicy := GetImageConfig().ImagePullPolicy

	if customPolicy != "" {
		return customPolicy
	} else if defaultOverridePolicy != "" {
		return defaultOverridePolicy
	}

	return ""
}

func GetConfigBuilderImage() string {
	return applyRegistry(configv1beta1.ConfigBuilderImageComponent, GetImageConfig().Images.ConfigBuilder)
}

func GetClientImage() string {
	return applyRegistry(configv1beta1.ClientImageComponent, GetImageConfig().Images.Client)
}

func GetSystemLoggerImage() string {
	return applyRegistry(configv1beta1.SystemLoggerImageComponent, GetImageConfig().Images.SystemLogger)
}

func AddDefaultRegistryImagePullSecrets(podSpec *corev1.PodSpec, imageTypes ...string) {
	secretNames := make([]string, 0)
	secretName := GetImageConfig().ImagePullSecret.Name
	if secretName != "" {
		secretNames = append(secretNames, secretName)
	}

	imageTypesToAdd := make(map[string]bool, len(imageTypes))
	if len(imageTypes) < 1 {
		if GetImageConfig().DefaultImages != nil {
			for name := range GetImageConfig().DefaultImages.ImageComponents {
				imageTypesToAdd[name] = true
			}
		}
	} else {
		for _, image := range imageTypes {
			imageTypesToAdd[image] = true
		}
	}

	if GetImageConfig().DefaultImages != nil {
		for name, component := range GetImageConfig().DefaultImages.ImageComponents {
			if _, found := imageTypesToAdd[name]; found {
				if component.ImagePullSecret.Name != "" {
					secretNames = append(secretNames, component.ImagePullSecret.Name)
				}
			}
		}
	}

	for _, s := range secretNames {
		podSpec.ImagePullSecrets = append(
			podSpec.ImagePullSecrets,
			corev1.LocalObjectReference{Name: s})
	}
}
