package main

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/k8ssandra/cass-operator/pkg/utils"
	"github.com/stretchr/testify/require"
)

func TestSetupWebhookServerWithoutCertPath(t *testing.T) {
	require := require.New(t)

	webhookServer, webhookCertWatcher, webhookEnabled, err := setupWebhookServer("", "tls.crt", "tls.key", nil)
	require.NoError(err)
	require.False(webhookEnabled)
	require.Nil(webhookServer)
	require.Nil(webhookCertWatcher)
}

func TestSetupWebhookServerWithCertPath(t *testing.T) {
	require := require.New(t)

	tmpDir := t.TempDir()
	keyPEM, certPEM, err := utils.GetNewCAandKey("cassandradatacenter-webhook-service", "test")
	require.NoError(err)

	require.NoError(os.WriteFile(filepath.Join(tmpDir, "tls.crt"), []byte(certPEM), 0o600))
	require.NoError(os.WriteFile(filepath.Join(tmpDir, "tls.key"), []byte(keyPEM), 0o600))

	webhookServer, webhookCertWatcher, webhookEnabled, err := setupWebhookServer(tmpDir, "tls.crt", "tls.key", []func(*tls.Config){})
	require.NoError(err)
	require.True(webhookEnabled)
	require.NotNil(webhookCertWatcher)

	defaultServer, ok := webhookServer.(*webhook.DefaultServer)
	require.True(ok, "expected default webhook server implementation, got %T", webhookServer)
	require.Len(defaultServer.Options.TLSOpts, 1)
}
