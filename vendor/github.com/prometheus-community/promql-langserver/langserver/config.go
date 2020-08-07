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
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/lsp/protocol"
	"github.com/prometheus/common/model"
)

// localLSPConfiguration is the configuration that should be used by the client such as VSCode.
// This struct shouldn't be used outside of the method DidChangeConfiguration.
type localLSPConfiguration struct {
	PromQL struct {
		URL                      string `json:"url"`
		MetadataLookbackInterval string `json:"metadataLookbackInterval"`
	} `json:"promql"`
}

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

	config := localLSPConfiguration{}

	// Go doesn't provide an easy way to cast nested maps into structs.
	//As a workaround, the configuration is first converted back to the JSON that was provided by the client and then marshalled back into the expected structure of the settings.
	//If you are reading this code and are aware of a better solution to do this, feel free to submit a PR.
	rawData, marshallError := json.Marshal(params.Settings)
	if marshallError != nil {
		// nolint: errcheck
		s.client.LogMessage(ctx, &protocol.LogMessageParams{
			Type:    protocol.Error,
			Message: fmt.Sprint("unable to serialize the configuration"),
		})
		return nil
	}
	if err := json.Unmarshal(rawData, &config); err != nil {
		// nolint: errcheck
		s.client.LogMessage(ctx, &protocol.LogMessageParams{
			Type:    protocol.Error,
			Message: errors.Wrap(err, "unexpected configuration format").Error(),
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

func (s *server) setURLFromChangeConfiguration(settings localLSPConfiguration) error {
	if _, err := url.Parse(settings.PromQL.URL); err != nil {
		return err
	}
	if err := s.connectPrometheus(settings.PromQL.URL); err != nil {
		return err
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

func (s *server) setMetadataLookbackInterval(settings localLSPConfiguration) error {
	duration, err := model.ParseDuration(settings.PromQL.MetadataLookbackInterval)
	if err != nil {
		return err
	}
	s.metadataService.SetLookbackInterval(time.Duration(duration))
	return nil
}
