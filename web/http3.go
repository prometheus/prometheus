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
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	toolkit_web "github.com/prometheus/exporter-toolkit/web"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"go.yaml.in/yaml/v2"
)

// http3Server wraps an HTTP/3 server.
type http3Server struct {
	server *http3.Server
	logger *slog.Logger
}

// newHTTP3Server creates a new HTTP/3 server.
func newHTTP3Server(addr string, handler http.Handler, tlsConfig *tls.Config, logger *slog.Logger) (*http3Server, error) {
	if tlsConfig == nil {
		return nil, fmt.Errorf("TLS configuration is required for HTTP/3")
	}

	server := &http3.Server{
		Addr:      addr,
		Handler:   handler,
		TLSConfig: http3.ConfigureTLSConfig(tlsConfig.Clone()),
		QUICConfig: &quic.Config{
			Allow0RTT: true,
		},
	}

	return &http3Server{server: server, logger: logger}, nil
}

// ListenAndServe starts the HTTP/3 server.
func (s *http3Server) ListenAndServe() error {
	s.logger.Info("Starting HTTP/3 server", "addr", s.server.Addr)
	return s.server.ListenAndServeTLS("", "")
}

// Close shuts down the HTTP/3 server.
func (s *http3Server) Close() error {
	s.logger.Info("Shutting down HTTP/3 server")
	return s.server.Close()
}

// SetQUICHeaders adds Alt-Svc header to advertise HTTP/3 availability.
func (s *http3Server) SetQUICHeaders(h http.Header) error {
	return s.server.SetQUICHeaders(h)
}

// wrapHandlerWithAltSvc wraps a handler to add Alt-Svc headers for HTTP/1.1 and HTTP/2 responses.
func wrapHandlerWithAltSvc(handler http.Handler, h3 *http3Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor < 3 {
			_ = h3.SetQUICHeaders(w.Header())
		}
		handler.ServeHTTP(w, r)
	})
}

// webConfigFile is a minimal struct to parse the web config file for TLS settings.
type webConfigFile struct {
	TLSConfig toolkit_web.TLSConfig `yaml:"tls_server_config"`
}

// getTLSConfigFromWebConfig reads the web config file and returns the TLS configuration.
// Returns nil if the file is empty or doesn't exist, or if TLS is not configured.
func getTLSConfigFromWebConfig(configPath string) (*tls.Config, error) {
	if configPath == "" {
		return nil, nil
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read web config file: %w", err)
	}

	if len(content) == 0 {
		return nil, nil
	}

	var cfg webConfigFile
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse web config file: %w", err)
	}

	// Check if TLS is configured (either cert file or inline cert must be present).
	if cfg.TLSConfig.TLSCertPath == "" && cfg.TLSConfig.TLSCert == "" {
		return nil, nil
	}

	// Set directory for relative paths.
	cfg.TLSConfig.SetDirectory(filepath.Dir(configPath))

	return toolkit_web.ConfigToTLSConfig(&cfg.TLSConfig)
}
