// Copyright DataStax, Inc.
// Please see the included license file for details.

package images

import (
	api "github.com/k8ssandra/cass-operator/apis/config/v1beta2"
	corev1 "k8s.io/api/core/v1"
)

type ImageRegistry interface {
	GetImage(imageType string) string
	GetImagePullPolicy(imageType string) corev1.PullPolicy
	GetImagePullSecrets(imageTypes ...string) []string

	// Shortcuts for common images used by cass-operator internally. TODO: Remove "Get" prefix for consistency with golang idioms
	GetConfigBuilderImage() string
	GetClientImage() string
	GetSystemLoggerImage() string
	GetCassandraImage(serverType, version string) (string, error)

	Image(imageType string) *api.Image
}
