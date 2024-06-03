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

package secrets

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRegistry(t *testing.T) {
	newProviderFunc := func(ctx context.Context, opts ProviderOptions, reg prometheus.Registerer) Provider[*yaml.Node] {
		return nil
	}

	registry := NewRegistry()
	err := registry.Register("a", nil)
	require.ErrorContains(t, err, "secrets provider must not be nil")
	require.Empty(t, registry.Get("a"))

	err = registry.Register("a", newProviderFunc)
	require.NoError(t, err)
	require.NotEmpty(t, registry.Get("a_sp_config"))

	err = registry.RegisterExact("b", newProviderFunc)
	require.NoError(t, err)
	require.NotEmpty(t, registry.Get("b"))

	err = registry.RegisterExact("a_sp_config", newProviderFunc)
	require.ErrorContains(t, err, "secrets provider named \"a_sp_config\" is already registered")
	require.NotEmpty(t, registry.Get("a_sp_config"))
}
