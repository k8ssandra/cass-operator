// Copyright DataStax, Inc.
// Please see the included license file for details.

package httphelper

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
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
	require.NoError(api.AddToScheme(scheme.Scheme))
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
	ctx := t.Context()

	httpClient, err := BuildManagementApiHttpClient(ctx, client, dc, nil)
	require.NoError(err)

	tlsConfig := httpClient.(*http.Client).Transport.(*http.Transport).TLSClientConfig
	require.NotNil(tlsConfig)
}

// Test mTLS probe conversion logic
func TestConfigureManagementApiAuth_HTTPGetProbe_Converted(t *testing.T) {
	cassContainer := &corev1.Container{
		Name: "cassandra",
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port: intstr.FromInt(8080),
					Path: "/custom/liveness",
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       20,
			TimeoutSeconds:      15,
			FailureThreshold:    5,
			SuccessThreshold:    1,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port: intstr.FromInt(8080),
					Path: "/custom/readiness",
				},
			},
			InitialDelaySeconds: 25,
			PeriodSeconds:       15,
			TimeoutSeconds:      10,
			FailureThreshold:    4,
			SuccessThreshold:    1,
		},
	}

	pod := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{*cassContainer},
		},
	}

	provider := &ManualManagementApiSecurityProvider{
		Config: &api.ManagementApiAuthManualConfig{
			ClientSecretName: "test-secret",
			ServerSecretName: "test-server-secret",
		},
	}
	err := provider.AddServerSecurity(pod)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Get the updated container
	cassContainer = &pod.Spec.Containers[0]

	// Verify liveness probe was converted to Exec
	if cassContainer.LivenessProbe.Exec == nil {
		t.Error("Expected liveness probe to have Exec handler")
	}
	if cassContainer.LivenessProbe.HTTPGet != nil {
		t.Error("Expected liveness probe HTTPGet to be nil after conversion")
	}
	if cassContainer.LivenessProbe.TCPSocket != nil {
		t.Error("Expected liveness probe TCPSocket to be nil")
	}
	if cassContainer.LivenessProbe.GRPC != nil {
		t.Error("Expected liveness probe GRPC to be nil")
	}

	// Verify timing settings were preserved
	if cassContainer.LivenessProbe.InitialDelaySeconds != 30 {
		t.Errorf("Expected InitialDelaySeconds=30, got %d", cassContainer.LivenessProbe.InitialDelaySeconds)
	}
	if cassContainer.LivenessProbe.PeriodSeconds != 20 {
		t.Errorf("Expected PeriodSeconds=20, got %d", cassContainer.LivenessProbe.PeriodSeconds)
	}
	if cassContainer.LivenessProbe.FailureThreshold != 5 {
		t.Errorf("Expected FailureThreshold=5, got %d", cassContainer.LivenessProbe.FailureThreshold)
	}

	// Verify readiness probe was converted to Exec
	if cassContainer.ReadinessProbe.Exec == nil {
		t.Error("Expected readiness probe to have Exec handler")
	}
	if cassContainer.ReadinessProbe.HTTPGet != nil {
		t.Error("Expected readiness probe HTTPGet to be nil after conversion")
	}

	// Verify timing settings were preserved
	if cassContainer.ReadinessProbe.InitialDelaySeconds != 25 {
		t.Errorf("Expected InitialDelaySeconds=25, got %d", cassContainer.ReadinessProbe.InitialDelaySeconds)
	}
}

func TestConfigureManagementApiAuth_TCPSocketProbe_Preserved(t *testing.T) {
	cassContainer := &corev1.Container{
		Name: "cassandra",
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(9042),
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       20,
			TimeoutSeconds:      15,
		},
	}

	pod := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{*cassContainer},
		},
	}

	provider := &ManualManagementApiSecurityProvider{
		Config: &api.ManagementApiAuthManualConfig{
			ClientSecretName: "test-secret",
			ServerSecretName: "test-server-secret",
		},
	}
	err := provider.AddServerSecurity(pod)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Get the updated container
	cassContainer = &pod.Spec.Containers[0]

	// Verify TCPSocket probe was NOT converted
	if cassContainer.LivenessProbe.TCPSocket == nil {
		t.Error("Expected liveness probe to still have TCPSocket handler")
	}
	if cassContainer.LivenessProbe.Exec != nil {
		t.Error("Expected liveness probe Exec to be nil (not converted)")
	}
	if cassContainer.LivenessProbe.HTTPGet != nil {
		t.Error("Expected liveness probe HTTPGet to be nil")
	}
	if cassContainer.LivenessProbe.GRPC != nil {
		t.Error("Expected liveness probe GRPC to be nil")
	}

	// Verify timing settings were preserved
	if cassContainer.LivenessProbe.InitialDelaySeconds != 30 {
		t.Errorf("Expected InitialDelaySeconds=30, got %d", cassContainer.LivenessProbe.InitialDelaySeconds)
	}
	if cassContainer.LivenessProbe.PeriodSeconds != 20 {
		t.Errorf("Expected PeriodSeconds=20, got %d", cassContainer.LivenessProbe.PeriodSeconds)
	}
}

func TestConfigureManagementApiAuth_GRPCProbe_Preserved(t *testing.T) {
	cassContainer := &corev1.Container{
		Name: "cassandra",
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				GRPC: &corev1.GRPCAction{
					Port: 8080,
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       20,
			TimeoutSeconds:      15,
		},
	}

	pod := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{*cassContainer},
		},
	}

	provider := &ManualManagementApiSecurityProvider{
		Config: &api.ManagementApiAuthManualConfig{
			ClientSecretName: "test-secret",
			ServerSecretName: "test-server-secret",
		},
	}
	err := provider.AddServerSecurity(pod)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Get the updated container
	cassContainer = &pod.Spec.Containers[0]

	// Verify GRPC probe was NOT converted
	if cassContainer.LivenessProbe.GRPC == nil {
		t.Error("Expected liveness probe to still have GRPC handler")
	}
	if cassContainer.LivenessProbe.Exec != nil {
		t.Error("Expected liveness probe Exec to be nil (not converted)")
	}
	if cassContainer.LivenessProbe.HTTPGet != nil {
		t.Error("Expected liveness probe HTTPGet to be nil")
	}
	if cassContainer.LivenessProbe.TCPSocket != nil {
		t.Error("Expected liveness probe TCPSocket to be nil")
	}

	// Verify timing settings were preserved
	if cassContainer.LivenessProbe.InitialDelaySeconds != 30 {
		t.Errorf("Expected InitialDelaySeconds=30, got %d", cassContainer.LivenessProbe.InitialDelaySeconds)
	}
}

func TestConfigureManagementApiAuth_ExecProbe_Preserved(t *testing.T) {
	customCommand := []string{"custom", "health", "check"}
	cassContainer := &corev1.Container{
		Name: "cassandra",
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: customCommand,
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       20,
			TimeoutSeconds:      15,
		},
	}

	pod := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{*cassContainer},
		},
	}

	provider := &ManualManagementApiSecurityProvider{
		Config: &api.ManagementApiAuthManualConfig{
			ClientSecretName: "test-secret",
			ServerSecretName: "test-server-secret",
		},
	}
	err := provider.AddServerSecurity(pod)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Get the updated container
	cassContainer = &pod.Spec.Containers[0]

	// Verify Exec probe was NOT overwritten
	if cassContainer.LivenessProbe.Exec == nil {
		t.Error("Expected liveness probe to still have Exec handler")
	}
	// Verify it's still the custom command, not the operator's command
	if len(cassContainer.LivenessProbe.Exec.Command) != len(customCommand) {
		t.Errorf("Expected custom command to be preserved, got %v", cassContainer.LivenessProbe.Exec.Command)
	}
	for i, cmd := range customCommand {
		if cassContainer.LivenessProbe.Exec.Command[i] != cmd {
			t.Errorf("Expected command[%d]=%s, got %s", i, cmd, cassContainer.LivenessProbe.Exec.Command[i])
		}
	}

	if cassContainer.LivenessProbe.HTTPGet != nil {
		t.Error("Expected liveness probe HTTPGet to be nil")
	}
	if cassContainer.LivenessProbe.TCPSocket != nil {
		t.Error("Expected liveness probe TCPSocket to be nil")
	}
	if cassContainer.LivenessProbe.GRPC != nil {
		t.Error("Expected liveness probe GRPC to be nil")
	}
}

func TestConfigureManagementApiAuth_NilProbe_NotCreated(t *testing.T) {
	cassContainer := &corev1.Container{
		Name:           "cassandra",
		LivenessProbe:  nil,
		ReadinessProbe: nil,
	}

	pod := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{*cassContainer},
		},
	}

	provider := &ManualManagementApiSecurityProvider{
		Config: &api.ManagementApiAuthManualConfig{
			ClientSecretName: "test-secret",
			ServerSecretName: "test-server-secret",
		},
	}
	err := provider.AddServerSecurity(pod)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Get the updated container
	cassContainer = &pod.Spec.Containers[0]

	// Verify probes were NOT created
	if cassContainer.LivenessProbe != nil {
		t.Error("Expected liveness probe to remain nil")
	}
	if cassContainer.ReadinessProbe != nil {
		t.Error("Expected readiness probe to remain nil")
	}
}

func TestConfigureManagementApiAuth_HTTPGetProbe_CustomEndpointExtracted(t *testing.T) {
	customPath := "/my/custom/health"
	cassContainer := &corev1.Container{
		Name: "cassandra",
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port: intstr.FromInt(8080),
					Path: customPath,
				},
			},
			TimeoutSeconds: 5,
		},
	}

	pod := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{*cassContainer},
		},
	}

	provider := &ManualManagementApiSecurityProvider{
		Config: &api.ManagementApiAuthManualConfig{
			ClientSecretName: "test-secret",
			ServerSecretName: "test-server-secret",
		},
	}
	err := provider.AddServerSecurity(pod)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Get the updated container
	cassContainer = &pod.Spec.Containers[0]

	// Verify the custom path was extracted and used in the Exec command
	if cassContainer.LivenessProbe.Exec == nil {
		t.Fatal("Expected liveness probe to have Exec handler")
	}

	// Check if custom path is in the curl command
	foundPath := false
	expectedURL := fmt.Sprintf("https://localhost:8080%s", customPath)
	for _, arg := range cassContainer.LivenessProbe.Exec.Command {
		if arg == expectedURL {
			foundPath = true
			break
		}
	}
	if !foundPath {
		t.Errorf("Expected URL %s to be in Exec command, got %v", expectedURL, cassContainer.LivenessProbe.Exec.Command)
	}
}

func TestConfigureManagementApiAuth_HTTPGetProbe_DefaultEndpointUsed(t *testing.T) {
	cassContainer := &corev1.Container{
		Name: "cassandra",
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port: intstr.FromInt(8080),
					Path: "", // Empty path
				},
			},
			TimeoutSeconds: 5,
		},
	}

	pod := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{*cassContainer},
		},
	}

	provider := &ManualManagementApiSecurityProvider{
		Config: &api.ManagementApiAuthManualConfig{
			ClientSecretName: "test-secret",
			ServerSecretName: "test-server-secret",
		},
	}
	err := provider.AddServerSecurity(pod)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Get the updated container
	cassContainer = &pod.Spec.Containers[0]

	// Verify the default liveness endpoint was used
	if cassContainer.LivenessProbe.Exec == nil {
		t.Fatal("Expected liveness probe to have Exec handler")
	}

	// Check if default liveness endpoint is in the curl command
	foundPath := false
	expectedURL := fmt.Sprintf("https://localhost:8080%s", LivenessEndpoint)
	for _, arg := range cassContainer.LivenessProbe.Exec.Command {
		if arg == expectedURL {
			foundPath = true
			break
		}
	}
	if !foundPath {
		t.Errorf("Expected URL %s to be in Exec command, got %v", expectedURL, cassContainer.LivenessProbe.Exec.Command)
	}
}
