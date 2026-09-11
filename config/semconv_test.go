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

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"
)

func TestSemconvConfigUnmarshal(t *testing.T) {
	t.Run("accepts a configured registry", func(t *testing.T) {
		var c SemconvConfig
		require.NoError(t, yaml.UnmarshalStrict([]byte("registry:\n  files:\n    - reg/*\n"), &c))
		require.True(t, c.Registry.Configured())
	})

	t.Run("rejects an empty block", func(t *testing.T) {
		var c SemconvConfig
		require.Error(t, yaml.UnmarshalStrict([]byte("{}\n"), &c))
	})
}

func TestSemconvRegistryConfigUnmarshal(t *testing.T) {
	t.Run("accepts a files source", func(t *testing.T) {
		var c SemconvRegistryConfig
		require.NoError(t, yaml.UnmarshalStrict([]byte("files:\n  - reg/*\n"), &c))
		require.True(t, c.Configured())
		require.Equal(t, []string{"reg/*"}, c.Files)
	})

	t.Run("accepts a url source with an http client config", func(t *testing.T) {
		var c SemconvRegistryConfig
		y := "url: https://example.com/registry.tar.gz\nbasic_auth:\n  username: u\n  password: p\n"
		require.NoError(t, yaml.UnmarshalStrict([]byte(y), &c))
		require.True(t, c.Configured())
		require.Equal(t, "https://example.com/registry.tar.gz", c.URL)
		require.NotNil(t, c.HTTPClientConfig.BasicAuth)
		require.Equal(t, "u", c.HTTPClientConfig.BasicAuth.Username)
	})

	t.Run("rejects neither files nor url", func(t *testing.T) {
		var c SemconvRegistryConfig
		require.Error(t, yaml.UnmarshalStrict([]byte("{}\n"), &c))
	})

	t.Run("rejects both files and url", func(t *testing.T) {
		var c SemconvRegistryConfig
		require.Error(t, yaml.UnmarshalStrict([]byte("files: [reg/*]\nurl: https://example.com/x.tar.gz\n"), &c))
	})

	t.Run("rejects a multi-level glob", func(t *testing.T) {
		var c SemconvRegistryConfig
		require.Error(t, yaml.UnmarshalStrict([]byte("files:\n  - a/*/b/*\n"), &c))
	})

	t.Run("rejects a non-http url scheme", func(t *testing.T) {
		var c SemconvRegistryConfig
		require.Error(t, yaml.UnmarshalStrict([]byte("url: file:///etc/passwd\n"), &c))
	})

	t.Run("SetDirectory joins relative file paths", func(t *testing.T) {
		c := SemconvRegistryConfig{Files: []string{"reg/registry.yaml"}}
		c.SetDirectory("/etc/prometheus")
		require.Equal(t, []string{"/etc/prometheus/reg/registry.yaml"}, c.Files)
	})
}
