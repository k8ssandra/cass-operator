package images

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/k8ssandra/cass-operator/apis/config/v1beta2"
)

var _ ImageRegistry = &imageRegistryV1Beta2{}

type imageRegistryV1Beta2 struct {
	imageConfig api.ImageConfig
	scheme      *runtime.Scheme
}

func NewImageRegistryFromConfigMap(ctx context.Context, c client.Client) (ImageRegistry, error) {
	cmList := &corev1.ConfigMapList{}
	sel := labels.SelectorFromSet(labels.Set{"k8ssandra.io/config": "image"})
	if err := c.List(ctx, cmList, &client.ListOptions{LabelSelector: sel}); err != nil {
		return nil, fmt.Errorf("listing image config configmaps: %w", err)
	}

	if len(cmList.Items) == 0 {
		// Not found, we don't want an error here, this is perfectly acceptable situation. User might be using the v1beta1 configuration
		return nil, nil
	}

	for _, cm := range cmList.Items {
		if data, ok := cm.Data["image_config.yaml"]; ok && data != "" {
			reg, err := NewImageRegistryV2([]byte(data))
			if err != nil {
				return nil, fmt.Errorf("parsing v1beta2 image config: %w", err)
			}
			return reg, nil
		}
	}

	// This is different, we found the ConfigMap but it was incorrectly formatted. Now we do want an error.
	return nil, fmt.Errorf("no ConfigMap with label k8ssandra.io/config=image and key image_config.yaml found")
}

func (r *imageRegistryV1Beta2) loadImageConfig(content []byte) (*api.ImageConfig, error) {
	codecs := serializer.NewCodecFactory(r.scheme)
	cfg := &api.ImageConfig{}
	if err := runtime.DecodeInto(codecs.UniversalDecoder(), content, cfg); err != nil {
		return nil, fmt.Errorf("could not decode file into runtime.Object: %v", err)
	}
	r.imageConfig = *cfg
	return cfg, nil
}

func NewImageRegistryV2(content []byte) (ImageRegistry, error) {
	scheme := runtime.NewScheme()
	utilruntime.Must(api.AddToScheme(scheme))
	r := &imageRegistryV1Beta2{scheme: scheme}
	imageConfig, err := r.loadImageConfig(content)
	if err != nil {
		return nil, fmt.Errorf("could not load image config: %w", err)
	}
	r.imageConfig = *imageConfig
	return r, nil
}

func (r *imageRegistryV1Beta2) mergeGlobalValues(img api.Image) *api.Image {
	defaults := r.imageConfig.Defaults
	overrides := r.imageConfig.Overrides
	if img.Registry == "" && defaults.Registry != nil {
		img.Registry = *defaults.Registry
	}
	if img.Repository == "" && defaults.Repository != nil {
		img.Repository = *defaults.Repository
		// Replace repository namespace according to defaults
		if *defaults.Repository == "" {
			img.Repository = ""
		} else {
			img.Repository = *defaults.Repository
		}
	}
	if img.PullPolicy == "" && defaults.PullPolicy != "" {
		img.PullPolicy = defaults.PullPolicy
	}
	if img.PullSecret == "" && len(defaults.PullSecrets) > 0 {
		img.PullSecret = defaults.PullSecrets[0]
	}

	if overrides != nil {
		if overrides.Repository != nil {
			// Replace repository namespace according to defaults
			img.Repository = *overrides.Repository
		}
		if overrides.Registry != nil {
			img.Registry = *overrides.Registry
		}
		if overrides.PullPolicy != "" {
			img.PullPolicy = overrides.PullPolicy
		}
	}
	return &img
}

func (r *imageRegistryV1Beta2) buildServerImage(serverType, version string) (*api.Image, error) {
	types := r.imageConfig.Types
	var imageComponent *api.ImageComponent
	if types != nil {
		imageComponent = types[serverType]
		if imageComponent == nil {
			return nil, fmt.Errorf("unknown server type: %s", serverType)
		}
	}

	img := api.Image{
		Registry:   imageComponent.Registry,
		Repository: imageComponent.Repository,
		Name:       imageComponent.Name,
		Tag:        imageComponent.Prefix + version + imageComponent.Suffix,
	}

	return r.mergeGlobalValues(img), nil
}

// Typed accessors
func (r *imageRegistryV1Beta2) Image(imageType string) *api.Image {
	base := r.imageConfig.Images[imageType]
	return r.mergeGlobalValues(base)
}

func (r *imageRegistryV1Beta2) ConfigBuilderImage() *api.Image {
	return r.Image(api.ConfigBuilderImageComponent)
}

func (r *imageRegistryV1Beta2) ClientImage() *api.Image {
	return r.Image(api.ClientImageComponent)
}

func (r *imageRegistryV1Beta2) SystemLoggerImage() *api.Image {
	return r.Image(api.SystemLoggerImageComponent)
}

func (r *imageRegistryV1Beta2) GetImagePullPolicy(imageType string) corev1.PullPolicy {
	if image, found := r.imageConfig.Types[imageType]; found {
		if image.PullPolicy != "" {
			return image.PullPolicy
		}
	}
	img := r.Image(imageType)
	return img.PullPolicy
}

func (r *imageRegistryV1Beta2) ImagePullSecrets(imageType string) []string {
	secretNames := make([]string, 0)

	if r.imageConfig.Overrides != nil && r.imageConfig.Overrides.PullSecrets != nil {
		secretNames = append(secretNames, r.imageConfig.Overrides.PullSecrets...)
		return secretNames
	}

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
	if len(r.imageConfig.Defaults.PullSecrets) > 0 {
		secretNames = append(secretNames, r.imageConfig.Defaults.PullSecrets...)
	}
	return secretNames
}

// Interface methods..

func (r *imageRegistryV1Beta2) GetImage(imageType string) string {
	return r.Image(imageType).String()
}

func (r *imageRegistryV1Beta2) GetConfigBuilderImage() string {
	return r.ConfigBuilderImage().String()
}

func (r *imageRegistryV1Beta2) GetClientImage() string {
	return r.ClientImage().String()
}

func (r *imageRegistryV1Beta2) GetSystemLoggerImage() string {
	return r.SystemLoggerImage().String()
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
