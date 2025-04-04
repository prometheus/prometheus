// Copyright 2025 The Prometheus Authors
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

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtraTags(t *testing.T) {
	testCases := map[string]struct {
		version          string
		existingVersions []string
		expected         []string
	}{
		"latest in previous major": {
			version:          "v2.53.4",
			existingVersions: []string{"v3.2.1", "v3.0.0", "v2.0.0", "v2.53.3"},
			expected:         []string{"v2", "v2.53"},
		},
		"latest in previous minor": {
			version:          "v2.0.1",
			existingVersions: []string{"v3.2.1", "v3.0.0", "v2.0.0", "v2.53.3"},
			expected:         []string{"v2.0"},
		},
		"latest in minor": {
			version:          "v3.0.1",
			existingVersions: []string{"v3.2.1", "v3.0.0", "v2.0.0", "v2.53.3"},
			expected:         []string{"v3.0"},
		},
		"latest patch": {
			version:          "v3.2.2",
			existingVersions: []string{"v3.2.1", "v3.0.0", "v2.0.0", "v2.53.3"},
			expected:         []string{"v3", "v3.2", "latest"},
		},
		"latest minor": {
			version:          "v3.3.0",
			existingVersions: []string{"v3.2.1", "v3.0.0", "v2.0.0", "v2.53.3"},
			expected:         []string{"v3", "v3.3", "latest"},
		},
		"weird version": {
			version:          "v3.2.0",
			existingVersions: []string{"v3.2.1", "v3.0.0", "v2.0.0", "v2.53.3"},
			expected:         []string{},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			actual := getExtraTags(tc.version, tc.existingVersions)
			require.Equal(t, tc.expected, actual)
		})
		t.Run(name+" idempotent", func(t *testing.T) {
			// Idempotency test.
			existingVersions := make([]string, len(tc.existingVersions))
			copy(existingVersions, tc.existingVersions)
			existingVersions = append(existingVersions, tc.version)
			actual := getExtraTags(tc.version, existingVersions)
			require.Equal(t, tc.expected, actual)
		})
	}
}
