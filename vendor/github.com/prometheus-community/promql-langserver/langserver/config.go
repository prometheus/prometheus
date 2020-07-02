// Copyright 2019 The Prometheus Authors
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
	"fmt"
	"net/url"
	"time"

	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/lsp/protocol"
	"github.com/prometheus/common/model"
)

// DidChangeConfiguration is required by the protocol.Server interface.
func (s *server) DidChangeConfiguration(ctx context.Context, params *protocol.DidChangeConfigurationParams) error {
	if params == nil {
		return nil
	}
	// nolint: errcheck
	s.client.LogMessage(
		s.lifetime,
		&protocol.LogMessageParams{
			Type:    protocol.Info,
			Message: fmt.Sprintf("Received notification change: %v\n", params),
		})

	setting := params.Settings

	// the struct expected is the following
	// promql:
	//   url: http://
	//   interval: 3w
	m, ok := setting.(map[string]map[string]string)
	if !ok {
		// nolint: errcheck
		s.client.LogMessage(ctx, &protocol.LogMessageParams{
			Type:    protocol.Error,
			Message: fmt.Sprint("unexpected format of the configuration"),
		})
		return nil
	}
	config, ok := m["promql"]
	if !ok {
		// nolint: errcheck
		s.client.LogMessage(ctx, &protocol.LogMessageParams{
			Type:    protocol.Error,
			Message: fmt.Sprint("promQL key not found"),
		})
		return nil
	}

	if err := s.setURLFromChangeConfiguration(config); err != nil {
		// nolint: errcheck
		s.client.LogMessage(ctx, &protocol.LogMessageParams{
			Type:    protocol.Info,
			Message: err.Error(),
		})
	}

	if err := s.setMetadataLookbackInterval(config); err != nil {
		// nolint: errcheck
		s.client.LogMessage(ctx, &protocol.LogMessageParams{
			Type:    protocol.Info,
			Message: err.Error(),
		})
	}
	return nil
}

func (s *server) setURLFromChangeConfiguration(settings map[string]string) error {
	if promURL, ok := settings["url"]; ok {
		if _, err := url.Parse(promURL); err != nil {
			return err
		}
		if err := s.connectPrometheus(promURL); err != nil {
			return err
		}
	}
	return nil
}

func (s *server) connectPrometheus(url string) error {
	if err := s.metadataService.ChangeDataSource(url); err != nil {
		// nolint: errcheck
		s.client.ShowMessage(s.lifetime, &protocol.ShowMessageParams{
			Type:    protocol.Error,
			Message: fmt.Sprintf("Failed to connect to Prometheus at %s:\n\n%s ", url, err.Error()),
		})
		return err
	}
	return nil
}

func (s *server) setMetadataLookbackInterval(settings map[string]string) error {
	if interval, ok := settings["metadataLookbackInterval"]; ok {
		duration, err := model.ParseDuration(interval)
		if err != nil {
			return err
		}
		s.metadataService.SetLookbackInterval(time.Duration(duration))
	}
	return nil
}
