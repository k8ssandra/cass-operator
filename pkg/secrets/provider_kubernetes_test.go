package secrets

import (
	"context"
	"fmt"
	"testing"

	"github.com/k8ssandra/cass-operator/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ SecretProvider = &KubernetesProvider{}

func TestStoreAndReadSecret(t *testing.T) {
	require := require.New(t)
	mockClient := &mocks.Client{}

	var storedSecret *corev1.Secret
	var errToRespond error

	k := NewKubernetesProvider(mockClient)

	mockClient.On("Get",
		mock.MatchedBy(
			func(ctx context.Context) bool {
				return ctx != nil
			}),
		mock.MatchedBy(
			func(key client.ObjectKey) bool {
				return key != client.ObjectKey{}
			}),
		mock.MatchedBy(
			func(obj runtime.Object) bool {
				return obj != nil
			})).
		Return(nil).
		Run(func(args mock.Arguments) {
			arg := args.Get(2).(*corev1.Secret)
			arg.StringData = storedSecret.StringData
			// return &corev1.Endpoints{}
		}).Times(2)

	mockClient.On("Create",
		mock.MatchedBy(
			func(ctx context.Context) bool {
				return ctx != nil
			}),
		mock.MatchedBy(
			func(s *corev1.Secret) bool {
				return true
			})).
		Return(nil).
		Run(func(args mock.Arguments) {
			arg := args.Get(1).(*corev1.Secret)
			storedSecret = arg
		})

	mockClient.On("Create",
		mock.MatchedBy(
			func(ctx context.Context) bool {
				return ctx != nil
			}),
		mock.MatchedBy(
			func(s *corev1.Secret) bool {
				fmt.Printf("We're here?\n")
				if storedSecret != nil {
					errToRespond = errors.NewAlreadyExists(schema.ParseGroupResource("Secret"), storedSecret.Name)
					return false
				}
				return true
			})).
		Return(errToRespond)

	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ns",
			Namespace: "testnamespace",
		},
		StringData: map[string]string{
			"a": "b",
		},
	}

	err := k.StoreOrUpdateSecret(context.TODO(), s)
	require.NoError(err)

	s2, err := k.RetrieveSecret(context.TODO(), types.NamespacedName{Name: s.Name, Namespace: s.Namespace})
	require.NoError(err)

	require.Equal(s.StringData, s2.StringData)

	s2.StringData["c"] = "e"

	err = k.StoreOrUpdateSecret(context.TODO(), s2)
	require.NoError(err)

	s3, err := k.RetrieveSecret(context.TODO(), types.NamespacedName{Name: s.Name, Namespace: s.Namespace})
	require.NoError(err)

	require.Equal("e", s3.StringData["c"])
}
