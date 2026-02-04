// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package web

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// generateTestCert generates a self-signed certificate and key for testing.
func generateTestCert(t *testing.T, certPath, keyPath string) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	require.NoError(t, err)

	certFile, err := os.Create(certPath)
	require.NoError(t, err)
	defer certFile.Close()
	err = pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	require.NoError(t, err)

	keyFile, err := os.Create(keyPath)
	require.NoError(t, err)
	defer keyFile.Close()
	keyDER, err := x509.MarshalECPrivateKey(priv)
	require.NoError(t, err)
	err = pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	require.NoError(t, err)
}

func TestNewHTTP3ServerRequiresTLS(t *testing.T) {
	logger := slog.Default()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	// Should fail without TLS config.
	_, err := newHTTP3Server(":9090", handler, nil, logger)
	require.Error(t, err)
	require.Contains(t, err.Error(), "TLS configuration is required")

	// Should succeed with TLS config.
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{},
	}
	server, err := newHTTP3Server(":9090", handler, tlsConfig, logger)
	require.NoError(t, err)
	require.NotNil(t, server)
}

func TestWrapHandlerWithAltSvc(t *testing.T) {
	logger := slog.Default()
	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{},
	}
	h3Server, err := newHTTP3Server(":9090", handler, tlsConfig, logger)
	require.NoError(t, err)

	wrapped := wrapHandlerWithAltSvc(handler, h3Server)

	// Simulate HTTP/1.1 request.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.ProtoMajor = 1
	req.ProtoMinor = 1

	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	// The handler should always be called, even if SetQUICHeaders fails
	// (which it will if the server isn't listening).
	require.True(t, handlerCalled, "Handler should be called")
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestGetTLSConfigFromWebConfig(t *testing.T) {
	t.Run("empty path returns nil", func(t *testing.T) {
		cfg, err := getTLSConfigFromWebConfig("")
		require.NoError(t, err)
		require.Nil(t, cfg)
	})

	t.Run("non-existent file returns nil", func(t *testing.T) {
		cfg, err := getTLSConfigFromWebConfig("/nonexistent/path/config.yml")
		require.NoError(t, err)
		require.Nil(t, cfg)
	})

	t.Run("empty file returns nil", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "empty.yml")
		err := os.WriteFile(configPath, []byte{}, 0o644)
		require.NoError(t, err)

		cfg, err := getTLSConfigFromWebConfig(configPath)
		require.NoError(t, err)
		require.Nil(t, cfg)
	})

	t.Run("config without TLS returns nil", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "no_tls.yml")
		content := `
http_server_config:
  http2: true
`
		err := os.WriteFile(configPath, []byte(content), 0o644)
		require.NoError(t, err)

		cfg, err := getTLSConfigFromWebConfig(configPath)
		require.NoError(t, err)
		require.Nil(t, cfg)
	})

	t.Run("config with TLS returns config", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create test certificate and key files.
		certPath := filepath.Join(tmpDir, "cert.pem")
		keyPath := filepath.Join(tmpDir, "key.pem")
		generateTestCert(t, certPath, keyPath)

		configPath := filepath.Join(tmpDir, "with_tls.yml")
		content := `
tls_server_config:
  cert_file: cert.pem
  key_file: key.pem
`
		err := os.WriteFile(configPath, []byte(content), 0o644)
		require.NoError(t, err)

		cfg, err := getTLSConfigFromWebConfig(configPath)
		require.NoError(t, err)
		require.NotNil(t, cfg)
		// The toolkit uses GetCertificate callback, not Certificates slice directly.
		// Verify we can get a certificate through the callback.
		require.NotNil(t, cfg.GetCertificate, "GetCertificate should be set")
		cert, err := cfg.GetCertificate(&tls.ClientHelloInfo{})
		require.NoError(t, err)
		require.NotNil(t, cert)
	})
}
