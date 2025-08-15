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

type imageRegistry struct {
	imageConfig configv1beta1.ImageConfig
	scheme      *runtime.Scheme
}

const (
	ValidDseVersionRegexp      = "(6\\.[89]\\.\\d+)"
	ValidHcdVersionRegexp      = "(1\\.\\d+\\.\\d+)"
	ValidOssVersionRegexp      = "(3\\.11\\.\\d+)|(4\\.\\d+\\.\\d+)|(5\\.\\d+\\.\\d+)"
	DefaultCassandraRepository = "k8ssandra/cass-management-api"
	DefaultDSERepository       = "datastax/dse-mgmtapi-6_8"
	DefaultHCDRepository       = "datastax/hcd"
)

// NewImageRegistry creates a new ImageRegistry.
func NewImageRegistry(imageConfigFile string) (ImageRegistry, error) {
	scheme := runtime.NewScheme()
	utilruntime.Must(configv1beta1.AddToScheme(scheme))
	i := &imageRegistry{
		scheme: scheme,
	}
	err := i.parseImageConfig(imageConfigFile)
	return i, err
}

func (i *imageRegistry) parseImageConfig(imageConfigFile string) error {
	content, err := os.ReadFile(imageConfigFile)
	if err != nil {
		return fmt.Errorf("could not read file at %s", imageConfigFile)
	}

	if _, err = i.loadImageConfig(content); err != nil {
		err = errors.Wrapf(err, "unable to load imageConfig from %s", imageConfigFile)
		return err
	}

	return nil
}

func (i *imageRegistry) loadImageConfig(content []byte) (*configv1beta1.ImageConfig, error) {
	codecs := serializer.NewCodecFactory(i.scheme)

	parsedImageConfig := &configv1beta1.ImageConfig{}
	if err := runtime.DecodeInto(codecs.UniversalDecoder(), content, parsedImageConfig); err != nil {
		return nil, fmt.Errorf("could not decode file into runtime.Object: %v", err)
	}

	i.imageConfig = *parsedImageConfig

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

func (i *imageRegistry) splitRegistry(image string) (registry string, imageNoRegistry string) {
	comps := strings.Split(image, "/")

	if len(comps) > 1 && (strings.Contains(comps[0], ".") || strings.Contains(comps[0], ":")) {
		return comps[0], strings.Join(comps[1:], "/")
	} else {
		return "", image
	}
}

// applyNamespaceOverride takes only input without registry
func (i *imageRegistry) applyNamespaceOverride(imageNoRegistry string) string {
	if i.GetImageConfig().ImageNamespace == nil {
		return imageNoRegistry
	}

	namespace := *i.GetImageConfig().ImageNamespace

	comps := strings.Split(imageNoRegistry, "/")
	if len(comps) > 1 {
		noNamespace := strings.Join(comps[1:], "/")
		if namespace == "" {
			return noNamespace
		}
		return fmt.Sprintf("%s/%s", namespace, noNamespace)
	} else {
		// We can't process this correctly, we only have 1 component. We do not support a case where the original image has no registry and no namespace.
		return imageNoRegistry
	}
}

func (i *imageRegistry) applyDefaultRegistryOverride(customRegistry, imageNoRegistry string) string {
	if customRegistry == "" {
		return imageNoRegistry
	}
	return fmt.Sprintf("%s/%s", customRegistry, imageNoRegistry)
}

func (i *imageRegistry) getRegistryOverride(imageType string) string {
	customRegistry := ""
	defaults := i.GetImageConfig().DefaultImages
	if defaults != nil {
		if component, found := defaults.ImageComponents[imageType]; found {
			customRegistry = component.ImageRegistry
		}
	}

	defaultRegistry := i.GetImageConfig().ImageRegistry

	if customRegistry != "" {
		return customRegistry
	}

	return defaultRegistry
}

func (i *imageRegistry) applyOverrides(imageType, image string) string {
	registryOverride := i.getRegistryOverride(imageType)
	registryOverride = strings.TrimSuffix(registryOverride, "/")
	registry, imageNoRegistry := i.splitRegistry(image)

	if registryOverride == "" && i.GetImageConfig().ImageNamespace == nil {
		return image
	}

	if i.GetImageConfig().ImageNamespace != nil {
		imageNoRegistry = i.applyNamespaceOverride(imageNoRegistry)
	}

	if registryOverride != "" {
		return i.applyDefaultRegistryOverride(registryOverride, imageNoRegistry)
	}

	return i.applyDefaultRegistryOverride(registry, imageNoRegistry)
}

func (i *imageRegistry) GetImageConfig() *configv1beta1.ImageConfig {
	// For now, this is static configuration (updated only on start of the pod), even if the actual ConfigMap underneath is updated.
	return &i.imageConfig
}

func (i *imageRegistry) getCassandraContainerImageOverride(serverType, version string) (bool, string) {
	// If there's a mapped volume with overrides in the ConfigMap, use these. Otherwise only calculate
	images := i.GetImageConfig().Images
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
		if serverType == "hcd" {
			if value, found := images.HCDVersions[version]; found {
				return true, value
			}
		}
	}
	return false, ""
}

func (i *imageRegistry) getImageComponents(serverType string) (string, string) {
	var defaultPrefix string

	if serverType == "dse" {
		defaultPrefix = DefaultDSERepository
	}
	if serverType == "cassandra" {
		defaultPrefix = DefaultCassandraRepository
	}

	defaults := i.GetImageConfig().DefaultImages
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

func (i *imageRegistry) GetCassandraImage(serverType, version string) (string, error) {
	if found, image := i.getCassandraContainerImageOverride(serverType, version); found {
		return i.applyOverrides(serverType, image), nil
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

	prefix, suffix := i.getImageComponents(serverType)

	return i.applyOverrides(serverType, fmt.Sprintf("%s:%s%s", prefix, version, suffix)), nil
}

func (i *imageRegistry) GetConfiguredImage(imageType, image string) string {
	return i.applyOverrides(imageType, image)
}

func (i *imageRegistry) GetImage(imageType string) string {
	return i.applyOverrides(imageType, i.GetImageConfig().Images.Others[imageType])
}

func (i *imageRegistry) GetImagePullPolicy(imageType string) corev1.PullPolicy {
	var customPolicy corev1.PullPolicy
	defaults := i.GetImageConfig().DefaultImages
	if defaults != nil {
		if component, found := defaults.ImageComponents[imageType]; found {
			customPolicy = component.ImagePullPolicy
		}
	}

	defaultOverridePolicy := i.GetImageConfig().ImagePullPolicy

	if customPolicy != "" {
		return customPolicy
	} else if defaultOverridePolicy != "" {
		return defaultOverridePolicy
	}

	return ""
}

func (i *imageRegistry) GetConfigBuilderImage() string {
	return i.applyOverrides(configv1beta1.ConfigBuilderImageComponent, i.GetImageConfig().Images.ConfigBuilder)
}

func (i *imageRegistry) GetClientImage() string {
	return i.applyOverrides(configv1beta1.ClientImageComponent, i.GetImageConfig().Images.Client)
}

func (i *imageRegistry) GetSystemLoggerImage() string {
	return i.applyOverrides(configv1beta1.SystemLoggerImageComponent, i.GetImageConfig().Images.SystemLogger)
}

func (i *imageRegistry) GetImagePullSecrets(imageTypes ...string) []string {
	secretNames := make([]string, 0)
	secretName := i.GetImageConfig().ImagePullSecret.Name
	if secretName != "" {
		secretNames = append(secretNames, secretName)
	}

	imageTypesToAdd := make(map[string]bool, len(imageTypes))
	if len(imageTypes) < 1 {
		if i.GetImageConfig().DefaultImages != nil {
			for name := range i.GetImageConfig().DefaultImages.ImageComponents {
				imageTypesToAdd[name] = true
			}
		}
	} else {
		for _, image := range imageTypes {
			imageTypesToAdd[image] = true
		}
	}

	if i.GetImageConfig().DefaultImages != nil {
		for name, component := range i.GetImageConfig().DefaultImages.ImageComponents {
			if _, found := imageTypesToAdd[name]; found {
				if component.ImagePullSecret.Name != "" {
					secretNames = append(secretNames, component.ImagePullSecret.Name)
				}
			}
		}
	}

	return secretNames
}
