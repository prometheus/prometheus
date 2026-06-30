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
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"
)

func TestResolveAddressesConfigUnmarshal(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		expected ResolveAddressesConfig
		err      string
	}{
		{
			name:  "defaults applied",
			input: "enabled: true",
			expected: ResolveAddressesConfig{
				Enabled:              true,
				RefreshInterval:      model.Duration(30 * time.Second),
				Type:                 resolveTypeAuto,
				MaxResolvedAddresses: defaultMaxResolvedAddresses,
			},
		},
		{
			name:  "explicit values",
			input: "enabled: true\nrefresh_interval: 5s\ntype: A",
			expected: ResolveAddressesConfig{
				Enabled:              true,
				RefreshInterval:      model.Duration(5 * time.Second),
				Type:                 "A",
				MaxResolvedAddresses: defaultMaxResolvedAddresses,
			},
		},
		{
			name:  "lowercase type canonicalised",
			input: "enabled: true\ntype: a",
			expected: ResolveAddressesConfig{
				Enabled:              true,
				RefreshInterval:      model.Duration(30 * time.Second),
				Type:                 resolveTypeA,
				MaxResolvedAddresses: defaultMaxResolvedAddresses,
			},
		},
		{
			name:  "mixed-case auto canonicalised",
			input: "enabled: true\ntype: AuTo",
			expected: ResolveAddressesConfig{
				Enabled:              true,
				RefreshInterval:      model.Duration(30 * time.Second),
				Type:                 resolveTypeAuto,
				MaxResolvedAddresses: defaultMaxResolvedAddresses,
			},
		},
		{
			name:  "lowercase aaaa canonicalised",
			input: "enabled: true\ntype: aaaa",
			expected: ResolveAddressesConfig{
				Enabled:              true,
				RefreshInterval:      model.Duration(30 * time.Second),
				Type:                 resolveTypeAAAA,
				MaxResolvedAddresses: defaultMaxResolvedAddresses,
			},
		},
		{
			name:  "explicit max_resolved_addresses",
			input: "enabled: true\nmax_resolved_addresses: 8",
			expected: ResolveAddressesConfig{
				Enabled:              true,
				RefreshInterval:      model.Duration(30 * time.Second),
				Type:                 resolveTypeAuto,
				MaxResolvedAddresses: 8,
			},
		},
		{
			name:  "zero max_resolved_addresses uses default",
			input: "enabled: true\nmax_resolved_addresses: 0",
			expected: ResolveAddressesConfig{
				Enabled:              true,
				RefreshInterval:      model.Duration(30 * time.Second),
				Type:                 resolveTypeAuto,
				MaxResolvedAddresses: defaultMaxResolvedAddresses,
			},
		},
		{
			name:  "negative max_resolved_addresses rejected",
			input: "enabled: true\nmax_resolved_addresses: -1",
			err:   "max_resolved_addresses must not be negative",
		},
		{
			name:  "invalid type",
			input: "enabled: true\ntype: CNAME",
			err:   "invalid resolve_addresses type",
		},
		{
			name:  "zero refresh interval when enabled",
			input: "enabled: true\nrefresh_interval: 0s",
			err:   "refresh_interval must be greater than 0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got ResolveAddressesConfig
			err := yaml.Unmarshal([]byte(tc.input), &got)
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, got)
		})
	}
}
