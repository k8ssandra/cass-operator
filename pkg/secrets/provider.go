package secrets

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
)

type SecretProvider interface {
	RetrieveSecret(context.Context, types.NamespacedName) (*corev1.Secret, error)
	StoreOrUpdateSecret(context.Context, *corev1.Secret) error
}

// Common methods that could be used by multiple implementations

func decodeSecret(data []byte) (*corev1.Secret, error) {
	decoder := serializer.NewCodecFactory(scheme.Scheme).UniversalDecoder()
	object := &corev1.Secret{}

	err := runtime.DecodeInto(decoder, data, object)
	if err != nil {
		return nil, err
	}
	return object, nil
}

func encodeSecret(secret *corev1.Secret) ([]byte, error) {
	encoder := serializer.NewCodecFactory(scheme.Scheme).LegacyCodec(corev1.SchemeGroupVersion)
	b, err := runtime.Encode(encoder, secret)
	if err != nil {
		return nil, err
	}
	return b, nil

}
