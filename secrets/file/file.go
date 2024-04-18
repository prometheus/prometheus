// Copyright 2024 The Prometheus Authors
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

package file

import (
	"context"
	"fmt"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v3"

	"github.com/prometheus/prometheus/secrets"
	"github.com/prometheus/prometheus/secrets/provider"
)

func init() {
	secrets.RegisterProviderExact("file", func(_ context.Context, _ secrets.ProviderOptions, _ prometheus.Registerer) secrets.Provider[*yaml.Node] {
		return provider.MapNode(provider.SecretDataProvider(func(_ context.Context, config string) (string, error) {
			data, err := os.ReadFile(config)
			if err != nil {
				return "", fmt.Errorf("unable to read file %q: %w", config, err)
			}
			return string(data), nil
		}))
	})
}
