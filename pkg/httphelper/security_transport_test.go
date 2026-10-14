// Copyright DataStax, Inc.
// Please see the included license file for details.

package httphelper

import (
	"net/http"
	"testing"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestInsecureBuildHttpClientDoesNotMutateDefaultClient(t *testing.T) {
	original := http.DefaultClient.Transport
	custom := &http.Transport{}

	provider := &InsecureManagementApiSecurityProvider{}
	withTransport, err := provider.BuildHttpClient(t.Context(), nil, nil, custom)
	require.NoError(t, err)
	withoutTransport, err := provider.BuildHttpClient(t.Context(), nil, nil, nil)
	require.NoError(t, err)

	require.NotSame(t, http.DefaultClient, withTransport)
	require.NotSame(t, http.DefaultClient, withoutTransport)
	require.Same(t, custom, withTransport.(*http.Client).Transport)
	require.Nil(t, withoutTransport.(*http.Client).Transport)
	require.Equal(t, original, http.DefaultClient.Transport)
}

func TestManualBuildHttpClientDoesNotMutateSuppliedTransport(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "client", Namespace: "test"},
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"ca.crt":  helperLoadBytes(t, "ca.crt"),
			"tls.crt": helperLoadBytes(t, "server.crt"),
			"tls.key": helperLoadBytes(t, "server.key"),
		},
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	supplied := &http.Transport{}

	provider := &ManualManagementApiSecurityProvider{
		Namespace: "test",
		Config:    &api.ManagementApiAuthManualConfig{ClientSecretName: "client"},
	}
	built, err := provider.BuildHttpClient(t.Context(), kube, kube, supplied)
	require.NoError(t, err)

	// http.Transport.Clone may populate HTTP/2 defaults on the source, but the
	// datacenter's client certificate and CA must never be written into it.
	if supplied.TLSClientConfig != nil {
		require.Empty(t, supplied.TLSClientConfig.Certificates)
		require.Nil(t, supplied.TLSClientConfig.RootCAs)
	}
	transport := built.(*http.Client).Transport.(*http.Transport)
	require.NotSame(t, supplied, transport)
	require.NotNil(t, transport.TLSClientConfig)
}
