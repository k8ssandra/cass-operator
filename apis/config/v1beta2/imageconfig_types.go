package v1beta2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:images

// Image uniquely describes a container image and also specifies how to pull it from its remote repository.
// More info: https://kubernetes.io/docs/concepts/containers/images.
// +kubebuilder:object:generate=true
type Image struct {
	metav1.TypeMeta `json:",inline"`

	// The Docker registry to use. Defaults to "docker.io", the official Docker Hub.
	// +optional
	Registry string `json:"registry,omitempty"`

	// The Docker repository to use.
	// +optional
	Repository string `json:"repository,omitempty"`

	// The image name to use.
	// +optional
	Name string `json:"name,omitempty"`

	// The image tag to use. Defaults to "latest".
	// +kubebuilder:default="latest"
	// +optional
	Tag string `json:"tag,omitempty"`

	// The image pull policy to use. Defaults to "Always" if the tag is "latest", otherwise to "IfNotPresent".
	// +optional
	// +kubebuilder:validation:Enum:=Always;IfNotPresent;Never
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`

	// The secret to use when pulling the image from private repositories. If specified, this secret will be passed to
	// individual puller implementations for them to use. For example, in the case of Docker, only DockerConfig type
	// secrets are honored. More info:
	// https://kubernetes.io/docs/concepts/containers/images#specifying-imagepullsecrets-on-a-pod
	// +optional
	PullSecret string `json:"pullSecret,omitempty"`
}

func (i Image) String() string {
	// These values are initialized by the ImageRegistry so the Image type always has the correct overridden values

	registry := i.Registry
	if i.Registry != "" {
		registry = i.Registry + "/"
	}

	repository := i.Repository
	if i.Repository != "" {
		repository = i.Repository + "/"
	}

	return registry + repository + i.Name + ":" + i.Tag
}

// ImageConfig is the Schema for the imageconfigs API
// +kubebuilder:object:root=true
type ImageConfig struct {
	metav1.TypeMeta `json:",inline"`

	// Images defines the images to be used for the various components.
	// The key is the component name, and the value is the image definition.
	// +optional
	Images Images `json:"images,omitempty"`

	// Defaults contains default values for images.
	Defaults ImagePolicy `json:"defaults"`

	// Types defines image types that can be referenced. These types are not matched to a tag
	// +optional
	Types ImageTypes `json:"types,omitempty"`

	// Overrides contains values that override any setting images might otherwise have
	// +optional
	Overrides *ImagePolicy `json:"overrides,omitempty"`
}

type ImagePolicy struct {
	Registry *string `json:"registry,omitempty"`

	PullSecrets []string `json:"pullSecrets,omitempty"`

	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`

	Repository *string `json:"repository,omitempty"`
}

// Images defines a map of images.
type Images map[string]Image

const (
	CassandraImageComponent     string = "cassandra"
	DSEImageComponent           string = "dse"
	HCDImageComponent           string = "hcd"
	SystemLoggerImageComponent  string = "system-logger"
	ConfigBuilderImageComponent string = "config-builder"
	ClientImageComponent        string = "k8ssandra-client"
)

type ImageTypes map[string]*ImageComponent

type ImageComponent struct {
	Suffix string `json:"suffix,omitempty"`
	Prefix string `json:"prefix,omitempty"`
	// We ignore Tag from the Image, but other fields are preserved.
	Image
}

func init() {
	SchemeBuilder.Register(&ImageConfig{})
}
