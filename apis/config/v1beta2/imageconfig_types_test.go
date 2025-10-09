package v1beta2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestImage_ApplyOverrides(t *testing.T) {
	tests := []struct {
		name      string
		original  *Image
		overrides *Image
		expected  *Image
	}{
		{
			name: "nil overrides",
			original: &Image{
				Registry:   "docker.io",
				Repository: "library",
				Name:       "cassandra",
				Tag:        "4.0.0",
				PullPolicy: corev1.PullIfNotPresent,
				PullSecret: "my-secret",
			},
			overrides: nil,
			expected: &Image{
				Registry:   "docker.io",
				Repository: "library",
				Name:       "cassandra",
				Tag:        "4.0.0",
				PullPolicy: corev1.PullIfNotPresent,
				PullSecret: "my-secret",
			},
		},
		{
			name: "full overrides",
			original: &Image{
				Registry:   "docker.io",
				Repository: "library",
				Name:       "cassandra",
				Tag:        "4.0.0",
				PullPolicy: corev1.PullIfNotPresent,
				PullSecret: "my-secret",
			},
			overrides: &Image{
				Registry:   "gcr.io",
				Repository: "k8ssandra",
				Name:       "cass-operator",
				Tag:        "1.10.0",
				PullPolicy: corev1.PullAlways,
				PullSecret: "another-secret",
			},
			expected: &Image{
				Registry:   "gcr.io",
				Repository: "k8ssandra",
				Name:       "cass-operator",
				Tag:        "1.10.0",
				PullPolicy: corev1.PullAlways,
				PullSecret: "another-secret",
			},
		},
		{
			name: "partial overrides",
			original: &Image{
				Registry:   "docker.io",
				Repository: "library",
				Name:       "cassandra",
				Tag:        "4.0.0",
				PullPolicy: corev1.PullIfNotPresent,
				PullSecret: "my-secret",
			},
			overrides: &Image{
				Registry: "quay.io",
				Tag:      "latest",
			},
			expected: &Image{
				Registry:   "quay.io",
				Repository: "library",
				Name:       "cassandra",
				Tag:        "latest",
				PullPolicy: corev1.PullIfNotPresent,
				PullSecret: "my-secret",
			},
		},
		{
			name: "empty overrides",
			original: &Image{
				Registry:   "docker.io",
				Repository: "library",
				Name:       "cassandra",
				Tag:        "4.0.0",
				PullPolicy: corev1.PullIfNotPresent,
				PullSecret: "my-secret",
			},
			overrides: &Image{},
			expected: &Image{
				Registry:   "docker.io",
				Repository: "library",
				Name:       "cassandra",
				Tag:        "4.0.0",
				PullPolicy: corev1.PullIfNotPresent,
				PullSecret: "my-secret",
			},
		},
		{
			name: "digest override",
			original: &Image{
				Registry:   "docker.io",
				Repository: "library",
				Name:       "cassandra",
				Tag:        "4.0.0",
			},
			overrides: &Image{
				Digest: "sha256:a94a8fe5ccb19ba61c4c0873d391e987982fbbd3f6ce540cc88fb1cf6fbb4e26",
			},
			expected: &Image{
				Registry:   "docker.io",
				Repository: "library",
				Name:       "cassandra",
				Tag:        "4.0.0",
				Digest:     "sha256:a94a8fe5ccb19ba61c4c0873d391e987982fbbd3f6ce540cc88fb1cf6fbb4e26",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.original.ApplyOverrides(tt.overrides)
			assert.Equal(t, tt.expected, tt.original)
		})
	}
}

func TestImage_String(t *testing.T) {
	tests := []struct {
		name     string
		image    Image
		expected string
	}{
		{
			name: "full image",
			image: Image{
				Registry:   "docker.io",
				Repository: "library",
				Name:       "cassandra",
				Tag:        "4.0.0",
			},
			expected: "docker.io/library/cassandra:4.0.0",
		},
		{
			name: "no registry",
			image: Image{
				Repository: "library",
				Name:       "cassandra",
				Tag:        "4.0.0",
			},
			expected: "library/cassandra:4.0.0",
		},
		{
			name: "no repository",
			image: Image{
				Registry: "docker.io",
				Name:     "cassandra",
				Tag:      "4.0.0",
			},
			expected: "docker.io/cassandra:4.0.0",
		},
		{
			name: "no registry and no repository",
			image: Image{
				Name: "cassandra",
				Tag:  "4.0.0",
			},
			expected: "cassandra:4.0.0",
		},
		{
			name: "no tag",
			image: Image{
				Registry:   "docker.io",
				Repository: "library",
				Name:       "cassandra",
			},
			expected: "docker.io/library/cassandra:",
		},
		{
			name: "tag is digest",
			image: Image{
				Registry:   "docker.io",
				Repository: "library",
				Name:       "cassandra",
				Tag:        "sha256:a94a8fe5ccb19ba61c4c0873d391e987982fbbd3f6ce540cc88fb1cf6fbb4e26",
			},
			expected: "docker.io/library/cassandra@sha256:a94a8fe5ccb19ba61c4c0873d391e987982fbbd3f6ce540cc88fb1cf6fbb4e26",
		},
		{
			name: "digest field takes precedence",
			image: Image{
				Name:   "cassandra",
				Tag:    "4.0.0",
				Digest: "sha256:2d711642b726b04401627ca9fbac32f5da7d61d74a0f5120e7131095d5f14f3f",
			},
			expected: "cassandra@sha256:2d711642b726b04401627ca9fbac32f5da7d61d74a0f5120e7131095d5f14f3f",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.image.String())
		})
	}
}
