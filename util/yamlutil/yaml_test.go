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

package yamlutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type testDocument struct {
	Items []map[string]string `yaml:"items"`
}

func TestUnmarshalStrict(t *testing.T) {
	t.Run("merge key override", func(t *testing.T) {
		input := []byte(`items:
- &defaults
  severity: warning
  team: operations
- <<: *defaults
  severity: critical
`)
		var got testDocument
		require.NoError(t, UnmarshalStrict(input, &got))
		require.Equal(t, []map[string]string{
			{"severity": "warning", "team": "operations"},
			{"severity": "critical", "team": "operations"},
		}, got.Items)
	})

	t.Run("unknown field", func(t *testing.T) {
		var got testDocument
		err := UnmarshalStrict([]byte("unknown: value\n"), &got)
		require.ErrorContains(t, err, "field unknown not found")
	})

	t.Run("merge sequence precedence", func(t *testing.T) {
		input := []byte(`items:
- &first
  severity: critical
  team: operations
- &second
  severity: warning
  region: us-east
- <<: [*first, *second]
`)
		var got testDocument
		require.NoError(t, UnmarshalStrict(input, &got))
		require.Equal(t, map[string]string{
			"severity": "critical",
			"team":     "operations",
			"region":   "us-east",
		}, got.Items[2])
	})

	t.Run("duplicate key", func(t *testing.T) {
		var got testDocument
		err := UnmarshalStrict([]byte("items: []\nitems: []\n"), &got)
		require.ErrorContains(t, err, "field items already set")
	})

	t.Run("empty document", func(t *testing.T) {
		var got testDocument
		require.NoError(t, UnmarshalStrict(nil, &got))
	})
}
