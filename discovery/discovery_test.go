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

package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"
)

func TestConfigsCustomUnMarshalMarshal(t *testing.T) {
	input := `static_configs:
- targets:
  - foo:1234
  - bar:4321
`
	cfg := &Configs{}
	err := yaml.UnmarshalStrict([]byte(input), cfg)
	require.NoError(t, err)

	output, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	require.Equal(t, input, string(output))
}

func TestStaticDiscovererDoesNotCloseUpdates(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	up := make(chan []*targetgroup.Group, 1)
	done := make(chan struct{})
	go func() {
		staticDiscoverer{{Source: "0"}}.Run(ctx, up)
		close(done)
	}()

	require.Equal(t, []*targetgroup.Group{{Source: "0"}}, <-up)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("static discoverer did not return after sending targets")
	}

	select {
	case _, ok := <-up:
		require.True(t, ok, "static discoverer closed the update channel")
	default:
	}
}
