// Copyright 2019 Tobias Guggenmos
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

package langserver

import (
	"context"

	"github.com/pkg/errors"

	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/lsp/protocol"
)

func (s *server) GetDiagnostics(uri protocol.DocumentURI) (*protocol.PublishDiagnosticsParams, error) {
	d, err := s.cache.GetDocument(uri)
	if err != nil {
		return nil, errors.Wrapf(err, "document not found in cache")
	}

	version, expired := d.GetVersion()
	if expired != nil {
		return nil, err
	}

	reply := &protocol.PublishDiagnosticsParams{
		URI:     uri,
		Version: version,
	}

	diagnostics, err := d.GetDiagnostics()
	if err != nil {
		return nil, err
	}

	reply.Diagnostics = diagnostics

	return reply, nil
}

func (s *server) diagnostics(uri protocol.DocumentURI) {
	reply, err := s.GetDiagnostics(uri)
	if err != nil {
		// nolint: errcheck
		s.client.LogMessage(s.lifetime, &protocol.LogMessageParams{
			Type:    protocol.Error,
			Message: err.Error(),
		})
	}

	if err = s.client.PublishDiagnostics(s.lifetime, reply); err != nil {
		// nolint: errcheck
		s.client.LogMessage(s.lifetime, &protocol.LogMessageParams{
			Type:    protocol.Error,
			Message: errors.Wrapf(err, "failed to publish diagnostics").Error(),
		})
	}
}

func (s *server) clearDiagnostics(ctx context.Context, uri protocol.DocumentURI, version float64) {
	diagnostics := &protocol.PublishDiagnosticsParams{
		URI:         uri,
		Version:     version,
		Diagnostics: []protocol.Diagnostic{},
	}

	if err := s.client.PublishDiagnostics(ctx, diagnostics); err != nil {
		// nolint: errcheck
		s.client.LogMessage(s.lifetime, &protocol.LogMessageParams{
			Type:    protocol.Error,
			Message: errors.Wrapf(err, "failed to publish diagnostics").Error(),
		})
	}
}
