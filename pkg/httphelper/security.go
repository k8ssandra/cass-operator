// Copyright DataStax, Inc.
// Please see the included license file for details.

package httphelper

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	NodeDrainEndpoint        = "/api/v0/ops/node/drain"
	MgmtApiTargetHostAndPort = "localhost:8080"
	LivenessEndpoint         = "/api/v0/probes/liveness"
	ReadinessEndpoint        = "/api/v0/probes/readiness"
	DefaultTimeout           = 10

	caCertPath = "/management-api-certs/ca.crt"
	tlsCrt     = "/management-api-certs/tls.crt"
	tlsKey     = "/management-api-certs/tls.key"
)

// API for Node Management mAuth Config
func GetManagementApiProtocol(dc *api.CassandraDatacenter) (string, error) {
	provider, err := BuildManagementApiSecurityProvider(dc)
	if err != nil {
		return "", err
	}
	return provider.GetProtocol(), nil
}

func BuildManagementApiHttpClient(ctx context.Context, client client.Client, dc *api.CassandraDatacenter, customTransport *http.Transport) (HttpClient, error) {
	provider, err := BuildManagementApiSecurityProvider(dc)
	if err != nil {
		return nil, err
	}
	return provider.BuildHttpClient(ctx, client, customTransport)
}

func AddManagementApiServerSecurity(dc *api.CassandraDatacenter, pod *corev1.PodTemplateSpec) error {
	provider, err := BuildManagementApiSecurityProvider(dc)
	if err != nil {
		return err
	}
	return provider.AddServerSecurity(pod)
}

func BuildManagementApiSecurityProvider(dc *api.CassandraDatacenter) (ManagementApiSecurityProvider, error) {
	options := []func(*api.CassandraDatacenter) (ManagementApiSecurityProvider, error){
		buildManualApiSecurityProvider,
		buildInsecureManagementApiSecurityProvider,
	}

	var selectedProvider ManagementApiSecurityProvider = nil

	for _, builder := range options {
		provider, err := builder(dc)
		if err != nil {
			return nil, err
		}
		if provider != nil && selectedProvider != nil {
			return nil, fmt.Errorf("multiple options specified for 'managementApiAuth', but expected exactly one")
		}
		if provider != nil {
			selectedProvider = provider
		}
	}

	if selectedProvider == nil {
		return nil, fmt.Errorf("no security strategy specified for 'managementApiAuth'")
	}

	return selectedProvider, nil
}

func ValidateManagementApiConfig(dc *api.CassandraDatacenter, client client.Client, ctx context.Context) []error {
	provider, err := BuildManagementApiSecurityProvider(dc)
	if err != nil {
		return []error{err}
	}

	return provider.ValidateConfig(ctx, client)
}

// SPI for adding new mechanisms for securing the management API
type ManagementApiSecurityProvider interface {
	BuildHttpClient(ctx context.Context, client client.Client, transport *http.Transport) (HttpClient, error)
	BuildMgmtApiGetAction(endpoint string, timeout int) *corev1.ExecAction
	BuildMgmtApiPostAction(endpoint string, timeout int) *corev1.ExecAction
	AddServerSecurity(pod *corev1.PodTemplateSpec) error
	GetProtocol() string
	ValidateConfig(ctx context.Context, client client.Client) []error
}

type InsecureManagementApiSecurityProvider struct{}

func buildInsecureManagementApiSecurityProvider(dc *api.CassandraDatacenter) (ManagementApiSecurityProvider, error) {
	// If both are nil, then default to insecure
	if dc.Spec.ManagementApiAuth.Insecure != nil || (dc.Spec.ManagementApiAuth.Manual == nil && dc.Spec.ManagementApiAuth.Insecure == nil) {
		return &InsecureManagementApiSecurityProvider{}, nil
	}
	return nil, nil
}

func (provider *InsecureManagementApiSecurityProvider) GetProtocol() string {
	return "http"
}

func (provider *InsecureManagementApiSecurityProvider) BuildHttpClient(ctx context.Context, client client.Client, transport *http.Transport) (HttpClient, error) {
	c := http.DefaultClient

	if transport != nil {
		c.Transport = transport
	}

	return c, nil
}

func (provider *InsecureManagementApiSecurityProvider) AddServerSecurity(pod *corev1.PodTemplateSpec) error {
	return nil
}

func (provider *InsecureManagementApiSecurityProvider) ValidateConfig(ctx context.Context, client client.Client) []error {
	return []error{}
}

type ManualManagementApiSecurityProvider struct {
	Namespace string
	Config    *api.ManagementApiAuthManualConfig
}

func buildManualApiSecurityProvider(dc *api.CassandraDatacenter) (ManagementApiSecurityProvider, error) {
	if dc.Spec.ManagementApiAuth.Manual != nil {
		provider := &ManualManagementApiSecurityProvider{}
		provider.Config = dc.Spec.ManagementApiAuth.Manual
		provider.Namespace = dc.Namespace
		return provider, nil
	}
	return nil, nil
}

func (provider *ManualManagementApiSecurityProvider) GetProtocol() string {
	return "https"
}

func GetMgmtApiPostAction(dc *api.CassandraDatacenter, endpoint string, timeout int) (*corev1.ExecAction, error) {
	provider, err := BuildManagementApiSecurityProvider(dc)
	if err != nil {
		return nil, err
	}
	return provider.BuildMgmtApiPostAction(endpoint, timeout), nil
}

func (provider *InsecureManagementApiSecurityProvider) BuildMgmtApiGetAction(endpoint string, timeout int) *corev1.ExecAction {
	return &corev1.ExecAction{
		Command: []string{
			"curl",
			"-X",
			"GET",
			"-s",
			"-m", strconv.Itoa(timeout),
			"-o",
			"/dev/null",
			"--show-error",
			"--fail",
			fmt.Sprintf("http://%s%s", MgmtApiTargetHostAndPort, endpoint),
		},
	}
}

func (provider *ManualManagementApiSecurityProvider) BuildMgmtApiGetAction(endpoint string, timeout int) *corev1.ExecAction {
	return &corev1.ExecAction{
		Command: []string{
			"curl",
			"-X",
			"GET",
			"-s",
			"-k",
			"--cert", tlsCrt,
			"--key", tlsKey,
			"--cacert", caCertPath,
			"-m", strconv.Itoa(timeout),
			"-o",
			"/dev/null",
			"--show-error",
			"--fail",
			fmt.Sprintf("https://%s%s", MgmtApiTargetHostAndPort, endpoint),
		},
	}
}

func (provider *InsecureManagementApiSecurityProvider) BuildMgmtApiPostAction(endpoint string, timeout int) *corev1.ExecAction {
	return &corev1.ExecAction{
		Command: []string{
			"curl",
			"-X",
			"POST",
			"-s",
			"-m", strconv.Itoa(timeout),
			"-o",
			"/dev/null",
			"--show-error",
			"--fail",
			fmt.Sprintf("http://%s%s", MgmtApiTargetHostAndPort, endpoint),
		},
	}
}

func (provider *ManualManagementApiSecurityProvider) BuildMgmtApiPostAction(endpoint string, timeout int) *corev1.ExecAction {
	return &corev1.ExecAction{
		Command: []string{
			"curl",
			"-X",
			"POST",
			"-s",
			"-k",
			"--cert", tlsCrt,
			"--key", tlsKey,
			"--cacert", caCertPath,
			"-m", strconv.Itoa(timeout),
			"-o",
			"/dev/null",
			"--show-error",
			"--fail",
			fmt.Sprintf("https://%s%s", MgmtApiTargetHostAndPort, endpoint),
		},
	}
}

func (provider *ManualManagementApiSecurityProvider) AddServerSecurity(pod *corev1.PodTemplateSpec) error {
	// find the container
	var container *corev1.Container = nil
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == "cassandra" {
			container = &pod.Spec.Containers[i]
		}
	}

	if container == nil {
		return fmt.Errorf("could not find cassandra container")
	}

	// Add volume containing certificates
	secretVolumeName := "management-api-server-certs-volume"
	secretVolume := corev1.Volume{
		Name: secretVolumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: provider.Config.ServerSecretName,
			},
		},
	}

	if pod.Spec.Volumes == nil {
		pod.Spec.Volumes = []corev1.Volume{}
	}

	pod.Spec.Volumes = append(pod.Spec.Volumes, secretVolume)

	// Mount certificates volume in container
	secretVolumeMount := corev1.VolumeMount{
		Name:      secretVolumeName,
		ReadOnly:  true,
		MountPath: "/management-api-certs",
	}

	if container.VolumeMounts == nil {
		container.VolumeMounts = []corev1.VolumeMount{}
	}

	container.VolumeMounts = append(container.VolumeMounts, secretVolumeMount)

	// Configure Management API to use certificates
	envVars := []corev1.EnvVar{
		{
			Name:  "MGMT_API_TLS_CA_CERT_FILE",
			Value: caCertPath,
		},
		{
			Name:  "MGMT_API_TLS_CERT_FILE",
			Value: tlsCrt,
		},
		{
			Name:  "MGMT_API_TLS_KEY_FILE",
			Value: tlsKey,
		},
		// TODO remove the below stuff post 1.0
		{
			Name:  "DSE_MGMT_TLS_CA_CERT_FILE",
			Value: caCertPath,
		},
		{
			Name:  "DSE_MGMT_TLS_CERT_FILE",
			Value: tlsCrt,
		},
		{
			Name:  "DSE_MGMT_TLS_KEY_FILE",
			Value: tlsKey,
		},
	}

	if container.Env == nil {
		container.Env = []corev1.EnvVar{}
	}

	container.Env = append(envVars, container.Env...)

	// Update Liveness probe to account for mutual auth (can't just use HTTP probe now)
	if container.LivenessProbe == nil {
		container.LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{},
		}
	}

	livenessTimeout := int(container.LivenessProbe.TimeoutSeconds)
	if livenessTimeout < 1 {
		livenessTimeout = DefaultTimeout
	}

	container.LivenessProbe.HTTPGet = nil
	container.LivenessProbe.TCPSocket = nil
	container.LivenessProbe.Exec = provider.BuildMgmtApiGetAction(LivenessEndpoint, livenessTimeout)

	// Update Readiness probe to account for mutual auth (can't just use HTTP probe now)
	// TODO: Get endpoint from configured HTTPGet probe
	if container.ReadinessProbe == nil {
		container.ReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{},
		}
	}

	readinessTimeout := int(container.ReadinessProbe.TimeoutSeconds)
	if readinessTimeout < 1 {
		readinessTimeout = DefaultTimeout
	}

	container.ReadinessProbe.HTTPGet = nil
	container.ReadinessProbe.TCPSocket = nil
	container.ReadinessProbe.Exec = provider.BuildMgmtApiGetAction(ReadinessEndpoint, readinessTimeout)

	return nil
}

func validatePrivateKey(data []byte) []error {
	const privateKeyExpect = "Private key should be unencrypted PKCS#8 format using PEM encoding with preamble 'PRIVATE KEY'"
	var validationErrors []error
	var block *pem.Block
	rest := data
	foundBlocks := false

	for {
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}

		foundBlocks = true

		if block.Type != "PRIVATE KEY" {
			if block.Type == "RSA PRIVATE KEY" {
				validationErrors = append(
					validationErrors,
					fmt.Errorf("%s, but found PKCS#1 format using preamble '%s'", privateKeyExpect, block.Type))
			} else if block.Type == "CERTIFICATE" {
				validationErrors = append(
					validationErrors,
					fmt.Errorf("%s, but found certificate using preamble '%s'", privateKeyExpect, block.Type))
			} else if strings.Contains(block.Type, "ENCRYPTED") {
				validationErrors = append(
					validationErrors,
					fmt.Errorf("%s, but found certificate using preamble '%s'", privateKeyExpect, block.Type))
			} else {
				validationErrors = append(
					validationErrors,
					fmt.Errorf("%s, but found preamble '%s'", privateKeyExpect, block.Type))
			}
		} else { // block.Type == "PRIVATE_KEY"
			// but is it _really_ a PKCS#8 key?
			_, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				validationErrors = append(
					validationErrors,
					// TODO: Switch %v to %w when golang version updated
					fmt.Errorf("%s, correct preamble was found but does not appear to be in PKCS#8 format. %w", privateKeyExpect, err))
			}
		}
	}

	if !foundBlocks {
		validationErrors = append(
			validationErrors,
			fmt.Errorf("%s, but provided key does not appear to be PEM encoded", privateKeyExpect))
	}

	return validationErrors
}

func validateCertificate(data []byte) []error {
	var validationErrors []error
	foundBlocks := false

	for rest := data; ; {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}

		foundBlocks = true

		if block.Type != "CERTIFICATE" {
			validationErrors = append(
				validationErrors,
				fmt.Errorf("certificate should be PEM encoded with preamble 'CERTIFICATE', but found preamble '%s'", block.Type))
		} else {
			_, err := x509.ParseCertificates(block.Bytes)
			if err != nil {
				validationErrors = append(
					validationErrors,
					fmt.Errorf("found PEM block with correct preamble of 'CERTIFICATE', but content does not appear to be a valid certificate. %w", err))
			}
		}
	}

	if !foundBlocks {
		validationErrors = append(
			validationErrors,
			fmt.Errorf("did not find any certificates"))
	}

	return validationErrors
}

func validateKeyAndCertificate(certificate, privateKey, caCertificate []byte) []error {
	var validationErrors []error

	privateKeyValidationErrors := validatePrivateKey(privateKey)
	certificateValidationErrors := validateCertificate(certificate)
	caValidationErrors := validateCertificate(caCertificate)

	validationErrors = append(
		validationErrors,
		privateKeyValidationErrors...)

	validationErrors = append(
		validationErrors,
		certificateValidationErrors...)

	validationErrors = append(
		validationErrors,
		caValidationErrors...)

	// This will catch errors with the certificate and check whether it matches
	// the private key.
	_, err := tls.X509KeyPair(
		certificate,
		privateKey)
	if err != nil {
		validationErrors = append(
			validationErrors,
			fmt.Errorf("could not load x509 key pair. %w", err))
	}

	return validationErrors
}

func pemToCertificateChain(certificate []byte) ([]*x509.Certificate, error) {
	certs := []*x509.Certificate{}
	rest := certificate
	var block *pem.Block

	for {
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}

		if block.Type == "CERTIFICATE" {
			parsedCerts, err := x509.ParseCertificates(block.Bytes)
			if err != nil {
				return nil, err
			}
			certs = append(certs, parsedCerts...)
		}
	}
	return certs, nil
}

func validateCertificateChain(chain []*x509.Certificate) error {
	for i := 0; i < len(chain)-1; i = i + 1 {
		certificateA := chain[i]
		certificateB := chain[i+1]
		err := certificateA.CheckSignatureFrom(certificateB)
		if err != nil {
			return fmt.Errorf(
				"failed to validate chain, certificate %s not signed by certificate %s. %w",
				certificateA.Subject.CommonName, certificateB.Subject.CommonName, err)
		}
	}
	return nil
}

func validatePeerACertificateSignedByPeerBCa(peerACertificate, peerACa, peerBCa []byte) error {
	// In order for the certificate of peer A (`peerACertificate`) to be
	// properly signed, it must be possible to construct a chain of trust from
	// peer A's certificate and peer A's CA (`peerACA`) to peer B's CA
	// (`peerBCa`).

	// Load the certificate chain for peerA
	peerACertificateChain, err := pemToCertificateChain(peerACertificate)
	if err != nil {
		return err
	}

	// Make sure the certificate chain is valid (i.e. that it is a sequence of
	// certificates for which each certificate in chain has signed the one
	// preceding it)
	err = validateCertificateChain(peerACertificateChain)
	if err != nil {
		return err
	}

	// Now we need to construct candidate chains to test against peer B's CA
	// pool
	candidateChains := [][]*x509.Certificate{}

	// One such chain is peer A's certificate chain as it may have all that is
	// needed for to tie it to one of peer B's CAs
	candidateChains = append(candidateChains, peerACertificateChain)

	// It might be the case that there are some intermediate CAs in peer A's CA
	// pool, so we find all such chains.
	peerACaCertPool := x509.NewCertPool()
	peerACaCertPool.AppendCertsFromPEM(peerACa)
	chainsUsingPeerACAs, err := verifyPeerCertificateNoHostCheck(peerACertificateChain, peerACaCertPool)
	if err == nil {
		// we found some chains, add them to our candidates
		candidateChains = append(candidateChains, chainsUsingPeerACAs...)
	}

	// Now we see if any of our candidate chains will work with peer B's CA
	// pool.
	peerBCaCertPool := x509.NewCertPool()
	peerBCaCertPool.AppendCertsFromPEM(peerBCa)
	var lastVerifyCertificateError error = nil
	for _, candidateChain := range candidateChains {
		_, lastVerifyCertificateError = verifyPeerCertificateNoHostCheck(candidateChain, peerBCaCertPool)
		if lastVerifyCertificateError == nil {
			// We found a valid chain, success!
			return nil
		}
	}

	if lastVerifyCertificateError == nil {
		// This should not ever happen because we will always have at least one
		// chain to test which means we should either return above or have an
		// error here. But it would cause an insidious bug if the logic above
		// was broken and we didn't do this check.
		lastVerifyCertificateError = errors.New("no candidate chains found to check")
	}

	return lastVerifyCertificateError
}

func validateSecretStructure(secret *corev1.Secret) error {
	secretNamespacedName := types.NamespacedName{
		Name:      secret.Name,
		Namespace: secret.Namespace,
	}

	// Check secret type
	if secret.Type != "kubernetes.io/tls" {
		// Not the right type
		err := fmt.Errorf("expected Secret %s to have type 'kubernetes.io/tls' but was '%s'",
			secretNamespacedName.String(),
			secret.Type)
		return err
	}

	// Ensure all keys are present
	for _, key := range []string{"ca.crt", "tls.crt", "tls.key"} {
		if _, ok := secret.Data[key]; !ok {
			err := fmt.Errorf("expected Secret %s to have data key '%s' but was not found",
				secretNamespacedName.String(),
				key)
			return err
		}
	}

	return nil
}

func loadSecret(client client.Client, ctx context.Context, namespace, name string) (*corev1.Secret, error) {
	secretNamespacedName := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}

	secret := &corev1.Secret{}
	err := client.Get(
		ctx,
		secretNamespacedName,
		secret)
	if err != nil {
		// Couldn't get the secret
		return nil, err
	}

	return secret, nil
}

func validateSecret(secret *corev1.Secret) []error {
	var validationErrors []error

	err := validateSecretStructure(secret)
	if err != nil {
		validationErrors = append(
			validationErrors,
			err)
		return validationErrors
	}

	keyAndCertificateErrors := validateKeyAndCertificate(secret.Data["tls.crt"], secret.Data["tls.key"], secret.Data["ca.crt"])
	validationErrors = append(validationErrors, keyAndCertificateErrors...)

	return validationErrors
}

func (provider *ManualManagementApiSecurityProvider) ValidateConfig(ctx context.Context, client client.Client) []error {
	var validationErrors []error

	if provider.Config.SkipSecretValidation {
		return validationErrors
	}

	var clientSecret *corev1.Secret
	var serverSecret *corev1.Secret

	clientSecretName := provider.Config.ClientSecretName
	serverSecretName := provider.Config.ServerSecretName

	secretChecks := []struct {
		secretName   string
		secretPtrPtr **corev1.Secret // everyone likes a pointer to a pointer
		configKey    string
	}{
		{
			secretName:   clientSecretName,
			secretPtrPtr: &clientSecret,
			configKey:    ".managementApiAuth.manual.clientSecretName",
		},
		{
			secretName:   serverSecretName,
			secretPtrPtr: &serverSecret,
			configKey:    ".managementApiAuth.manual.serverSecretName",
		},
	}

	for _, check := range secretChecks {
		var err error
		*check.secretPtrPtr, err = loadSecret(client, ctx, provider.Namespace, check.secretName)
		if err != nil {
			validationErrors = append(
				validationErrors,
				fmt.Errorf("failed to load Management API secret specified at %s with value '%s'. %w",
					check.configKey, check.secretName, err))
			return validationErrors
		}

		errs := validateSecret(*check.secretPtrPtr)
		for _, err := range errs {
			validationErrors = append(
				validationErrors,
				fmt.Errorf("loaded Management API secret specified at %s with value '%s' is not valid. %w",
					check.configKey, check.secretName, err))
		}
	}

	certificateSigningChecks := []struct {
		peerAsecret *corev1.Secret
		peerBsecret *corev1.Secret
		configKey   string
	}{
		{
			peerAsecret: clientSecret,
			peerBsecret: serverSecret,
			configKey:   ".managementApiAuth.manual.clientSecretName",
		},
		{
			peerAsecret: serverSecret,
			peerBsecret: clientSecret,
			configKey:   ".managementApiAuth.manual.serverSecretName",
		},
	}

	for _, check := range certificateSigningChecks {
		var err error
		secretName := check.peerAsecret.Name
		err = validatePeerACertificateSignedByPeerBCa(check.peerAsecret.Data["tls.crt"], check.peerAsecret.Data["ca.crt"], check.peerBsecret.Data["ca.crt"])
		if err != nil {
			validationErrors = append(
				validationErrors,
				fmt.Errorf("loaded Management API client secret specified at %s with value '%s' is not properly signed. %w", check.configKey, secretName, err))
		}
	}

	return validationErrors
}

func (provider *ManualManagementApiSecurityProvider) BuildHttpClient(ctx context.Context, client client.Client, transport *http.Transport) (HttpClient, error) {
	httpClient := &http.Client{Transport: transport}
	if transport != nil && transport.TLSClientConfig != nil {
		return httpClient, nil
	}

	// Get the client Secret
	secretNamespacedName := types.NamespacedName{
		Name:      provider.Config.ClientSecretName,
		Namespace: provider.Namespace,
	}

	secret := &corev1.Secret{}
	err := client.Get(
		ctx,
		secretNamespacedName,
		secret)
	if err != nil {
		// Couldn't get the secret
		return nil, err
	}

	err = validateSecretStructure(secret)
	if err != nil {
		// Secret didn't look the way we expect
		return nil, err
	}

	// Create the CA certificate pool
	caCertPool := x509.NewCertPool()
	ok := caCertPool.AppendCertsFromPEM(secret.Data["ca.crt"])
	if !ok {
		err = fmt.Errorf("no certificates found in %s when parsing 'ca.crt' value: %v",
			secretNamespacedName.String(),
			secret.Data["ca.crt"])
		return nil, err
	}

	// Load client key pair
	cert, err := tls.X509KeyPair(secret.Data["tls.crt"], secret.Data["tls.key"])
	if err != nil {
		return nil, err
	}

	// Build the client
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		// TODO: ...we should probably verify something here...
		InsecureSkipVerify:    true,
		VerifyPeerCertificate: buildVerifyPeerCertificateNoHostCheck(caCertPool),
	}

	if transport != nil && transport.TLSClientConfig == nil {
		transport.TLSClientConfig = tlsConfig
	} else if transport == nil {
		transport = &http.Transport{TLSClientConfig: tlsConfig}
	}

	httpClient.Transport = transport

	return httpClient, nil
}

// Below implementation modified from:
//
// https://go-review.googlesource.com/c/go/+/193620/5/src/crypto/tls/example_test.go#210
func buildVerifyPeerCertificateNoHostCheck(rootCAs *x509.CertPool) func([][]byte, [][]*x509.Certificate) error {
	f := func(certificates [][]byte, _ [][]*x509.Certificate) error {
		certs := make([]*x509.Certificate, len(certificates))
		for i, asn1Data := range certificates {
			cert, err := x509.ParseCertificate(asn1Data)
			if err != nil {
				return err
			}
			certs[i] = cert
		}

		_, err := verifyPeerCertificateNoHostCheck(certs, rootCAs)
		return err
	}
	return f
}

func verifyPeerCertificateNoHostCheck(certificates []*x509.Certificate, rootCAs *x509.CertPool) ([][]*x509.Certificate, error) {
	opts := x509.VerifyOptions{
		Roots: rootCAs,
		// Setting the DNSName to the empty string will cause
		// Certificate.Verify() to skip hostname checking
		DNSName:       "",
		Intermediates: x509.NewCertPool(),
	}
	for _, cert := range certificates[1:] {
		opts.Intermediates.AddCert(cert)
	}
	chains, err := certificates[0].Verify(opts)
	return chains, err
}
