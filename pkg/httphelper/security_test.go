// Copyright DataStax, Inc.
// Please see the included license file for details.

package httphelper

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/scheme"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func helperLoadBytes(t *testing.T, name string) []byte {
	path := filepath.Join("testdata", name)
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return bytes
}

func Test_buildVerifyPeerCertificateNoHostCheck_AcceptsGoodCert(t *testing.T) {
	goodCaPem := helperLoadBytes(t, "ca.crt")
	certPem := helperLoadBytes(t, "server.crt")

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(goodCaPem)

	verifyPeerCertificate := buildVerifyPeerCertificateNoHostCheck(caCertPool)

	block, _ := pem.Decode(certPem)
	err := verifyPeerCertificate([][]byte{block.Bytes}, nil)

	// We should not get an error because certPem is signed by good CA
	assert.NoError(t, err)
}

func Test_buildVerifyPeerCertificateNoHostCheck_RejectsBadCert(t *testing.T) {
	badCaPem := helperLoadBytes(t, "evil_ca.crt")
	certPem := helperLoadBytes(t, "server.crt")

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(badCaPem)

	verifyPeerCertificate := buildVerifyPeerCertificateNoHostCheck(caCertPool)

	block, _ := pem.Decode(certPem)
	err := verifyPeerCertificate([][]byte{block.Bytes}, nil)

	// We should get an error becase certPem is not signed by bad CA
	assert.Error(t, err)
}

func Test_validatePrivateKey(t *testing.T) {
	var errs []error
	certPem := helperLoadBytes(t, "server.crt")
	privateKey := helperLoadBytes(t, "server.key")
	privateRsaKey := helperLoadBytes(t, "server.rsa.key")
	privateEncryptedKey := helperLoadBytes(t, "server.encrypted.key")

	// use actual private key
	errs = validatePrivateKey(privateKey)
	assert.Equal(
		t, 0, len(errs),
		"Should have no errors for valid private key")

	// use cert instead of private key
	errs = validatePrivateKey(certPem)

	assert.Equal(
		t, 1, len(errs),
		"Should have error about type being a certificate when private key expected")

	// use PKCS#1 key
	errs = validatePrivateKey(privateRsaKey)
	assert.Equal(
		t, 1, len(errs),
		"Should have error about using PKCS#1 when PKCS#8 expected")

	// use encrypted key
	errs = validatePrivateKey(privateEncryptedKey)
	assert.Equal(
		t, 1, len(errs),
		"Should have error about using an encrypted key")

	// use jibberish
	errs = validatePrivateKey([]byte("some non-key"))
	assert.Equal(
		t, 1, len(errs),
		"Should have an error about not being properly PEM encoded")

	// TODO: Is the empty PEM file valid? Assuming not for now
	errs = validatePrivateKey([]byte(""))
	assert.Equal(
		t, 1, len(errs),
		"Should consider an empty key as an invalid key")
}

// Create Datacenter with managementAuth set to manual and TLS enabled, test that the client is created with the correct TLS configuration using
// BuildManagementApiHttpClient method
func TestBuildMTLSClient(t *testing.T) {
	require := require.New(t)
	api.AddToScheme(scheme.Scheme)
	decode := serializer.NewCodecFactory(scheme.Scheme).UniversalDeserializer().Decode

	loadYaml := func(path string) (runtime.Object, error) {
		bytes, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		obj, _, err := decode(bytes, nil, nil)
		return obj, err
	}

	clientSecret, err := loadYaml(filepath.Join("..", "..", "tests", "testdata", "mtls-certs-client.yaml"))
	require.NoError(err)

	serverSecret, err := loadYaml(filepath.Join("..", "..", "tests", "testdata", "mtls-certs-server.yaml"))
	require.NoError(err)

	dc := &api.CassandraDatacenter{
		Spec: api.CassandraDatacenterSpec{
			ClusterName: "test-cluster",
			ManagementApiAuth: api.ManagementApiAuthConfig{
				Manual: &api.ManagementApiAuthManualConfig{
					ClientSecretName: "mgmt-api-client-credentials",
					ServerSecretName: "mgmt-api-server-credentials",
				},
			},
		},
	}

	trackObjects := []runtime.Object{
		clientSecret,
		serverSecret,
		dc,
	}

	client := fake.NewClientBuilder().WithRuntimeObjects(trackObjects...).Build()
	ctx := context.TODO()

	httpClient, err := BuildManagementApiHttpClient(ctx, client, dc, nil)
	require.NoError(err)

	tlsConfig := httpClient.(*http.Client).Transport.(*http.Transport).TLSClientConfig
	require.NotNil(tlsConfig)
}
