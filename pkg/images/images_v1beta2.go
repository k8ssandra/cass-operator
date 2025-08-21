package images

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	configv1beta2 "github.com/k8ssandra/cass-operator/apis/config/v1beta2"
)

type imageRegistryV1Beta2 struct {
	imageConfig configv1beta2.ImageConfig
	scheme      *runtime.Scheme
}

func (r *imageRegistryV1Beta2) loadImageConfig(content []byte) (*configv1beta2.ImageConfig, error) {
	codecs := serializer.NewCodecFactory(r.scheme)
	cfg := &configv1beta2.ImageConfig{}
	if err := runtime.DecodeInto(codecs.UniversalDecoder(), content, cfg); err != nil {
		return nil, fmt.Errorf("could not decode file into runtime.Object: %v", err)
	}
	r.imageConfig = *cfg
	return cfg, nil
}

func NewImageRegistryV2(content []byte) (ImageRegistry, error) {
	scheme := runtime.NewScheme()
	utilruntime.Must(configv1beta2.AddToScheme(scheme))
	r := &imageRegistryV1Beta2{scheme: scheme}
	imageConfig, err := r.loadImageConfig(content)
	if err != nil {
		return nil, fmt.Errorf("could not load image config: %w", err)
	}
	r.imageConfig = *imageConfig
	return r, nil
}

// mergeDefaults applies Defaults and optional Namespace override.
func (r *imageRegistryV1Beta2) mergeDefaults(img configv1beta2.Image) configv1beta2.Image {
	out := img
	if r.imageConfig.Defaults != nil {
		d := r.imageConfig.Defaults
		if out.Registry == "" && d.Registry != "" {
			out.Registry = d.Registry
		}
		if d.Namespace != nil {
			// Replace repository namespace according to defaults
			if *d.Namespace == "" {
				out.Repository = ""
			} else {
				out.Repository = *d.Namespace
			}
		}
		if out.PullPolicy == "" && d.PullPolicy != "" {
			out.PullPolicy = d.PullPolicy
		}
		if out.PullSecret == "" && len(d.PullSecrets) > 0 {
			out.PullSecret = d.PullSecrets[0]
		}
	}
	return out
}

func (r *imageRegistryV1Beta2) buildServerImage(serverType, version string) (configv1beta2.Image, error) {
	types := r.imageConfig.Types
	var imageComponent *configv1beta2.ImageComponent
	if types != nil {
		imageComponent = types[serverType]
		if imageComponent == nil {
			return configv1beta2.Image{}, fmt.Errorf("unknown server type: %s", serverType)
		}
	}

	img := configv1beta2.Image{
		Registry:   imageComponent.Registry,
		Repository: imageComponent.Repository,
		Name:       imageComponent.Name,
		Tag:        version + imageComponent.Suffix,
	}

	return r.mergeDefaults(img), nil
}

// Typed accessors
func (r *imageRegistryV1Beta2) imageGetImage(imageType string) configv1beta2.Image {
	base := r.imageConfig.Images[imageType]
	return r.mergeDefaults(base)
}

func (r *imageRegistryV1Beta2) ImageGetConfigBuilderImage() configv1beta2.Image {
	return r.imageGetImage(configv1beta2.ConfigBuilderImageComponent)
}

func (r *imageRegistryV1Beta2) ImageGetClientImage() configv1beta2.Image {
	return r.imageGetImage(configv1beta2.ClientImageComponent)
}

func (r *imageRegistryV1Beta2) ImageGetSystemLoggerImage() configv1beta2.Image {
	return r.imageGetImage(configv1beta2.SystemLoggerImageComponent)
}

func (r *imageRegistryV1Beta2) GetImagePullPolicy(imageType string) corev1.PullPolicy {
	img := r.imageGetImage(imageType)
	return img.PullPolicy
}

func (r *imageRegistryV1Beta2) ImagePullSecrets(imageType string) []string {
	secretNames := make([]string, 0)
	if imageType != "" {
		if image, found := r.imageConfig.Images[imageType]; found {
			if image.PullSecret != "" {
				secretNames = append(secretNames, image.PullSecret)
				// If there was a specified pull secret, we only return that one without the defaults
				return secretNames
			}
		}
		if image, found := r.imageConfig.Types[imageType]; found {
			if image.PullSecret != "" {
				secretNames = append(secretNames, image.PullSecret)
				// If there was a specified pull secret, we only return that one without the defaults
				return secretNames
			}
		}
	}

	// We only add defaults if the specific type didn't have a pullSecret defined
	if r.imageConfig.Defaults != nil {
		secretNames = append(secretNames, r.imageConfig.Defaults.PullSecrets...)
	}
	return secretNames
}

// Interface methods..

func (r *imageRegistryV1Beta2) GetImage(imageType string) string {
	return r.imageGetImage(imageType).String()
}

func (r *imageRegistryV1Beta2) GetConfigBuilderImage() string {
	return r.ImageGetConfigBuilderImage().String()
}

func (r *imageRegistryV1Beta2) GetClientImage() string {
	return r.ImageGetClientImage().String()
}

func (r *imageRegistryV1Beta2) GetSystemLoggerImage() string {
	return r.ImageGetSystemLoggerImage().String()
}

func (r *imageRegistryV1Beta2) GetCassandraImage(serverType, version string) (string, error) {
	img, err := r.buildServerImage(serverType, version)
	return img.String(), err
}

func (r *imageRegistryV1Beta2) GetImagePullSecrets(imageType ...string) []string {
	if len(imageType) > 0 {
		return r.ImagePullSecrets(imageType[0])
	}

	return r.ImagePullSecrets("")
}
