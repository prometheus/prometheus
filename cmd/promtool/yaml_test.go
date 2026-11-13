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

package main

import (
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestLoadYAMLUnitTestFile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		input       string
		want        unitTestFile
		errContains string
	}{
		{
			name: "empty input",
		},
		{
			name:  "comment-only input",
			input: "# No tests.\n",
		},
		{
			name:  "null input",
			input: "null\n",
		},
		{
			name:  "null fields",
			input: "evaluation_interval: null\nfuzzy_compare: null\ntests: null\n",
		},
		{
			name: "durations labels and timestamp callbacks",
			input: `evaluation_interval: 2m
tests:
- name: example
  interval: 30s
  external_labels: {job: node, switch: on}
  start_timestamp: "2024-01-01T00:00:00Z"
`,
			want: unitTestFile{
				EvaluationInterval: model.Duration(2 * time.Minute),
				Tests: []testGroup{{
					TestGroupName:  "example",
					Interval:       model.Duration(30 * time.Second),
					ExternalLabels: labels.FromStrings("job", "node", "switch", "on"),
					StartTimestamp: testStartTimestamp{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
				}},
			},
		},
		{
			name:  "numeric timestamp callback",
			input: "tests:\n- start_timestamp: 1609459200\n",
			want: unitTestFile{Tests: []testGroup{{
				StartTimestamp: testStartTimestamp{Time: time.Unix(1609459200, 0).UTC()},
			}}},
		},
		{
			name:        "unknown field",
			input:       "unknown: value\n",
			errContains: "field unknown not found",
		},
		{
			name:        "unknown nested field",
			input:       "tests:\n- unknown: value\n",
			errContains: "field unknown not found",
		},
		{
			name:        "duplicate field",
			input:       "evaluation_interval: 1m\nevaluation_interval: 2m\n",
			errContains: "evaluation_interval",
		},
		{
			name:        "duplicate nested field",
			input:       "tests:\n- name: first\n  name: second\n",
			errContains: "name",
		},
		{
			name:  "first document only",
			input: "evaluation_interval: 2m\n---\nunknown: ignored\n",
			want:  unitTestFile{EvaluationInterval: model.Duration(2 * time.Minute)},
		},
		{
			name:  "malformed trailing document ignored",
			input: "evaluation_interval: 2m\n---\n[unterminated\n",
			want:  unitTestFile{EvaluationInterval: model.Duration(2 * time.Minute)},
		},
		{
			name: "aliases and merge keys",
			input: `tests:
- &first {name: first, interval: 30s}
- {<<: *first, name: second}
`,
			want: unitTestFile{Tests: []testGroup{
				{TestGroupName: "first", Interval: model.Duration(30 * time.Second)},
				{TestGroupName: "second", Interval: model.Duration(30 * time.Second)},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var got unitTestFile
			err := loadYAML([]byte(test.input), &got)
			if test.errContains != "" {
				require.ErrorContains(t, err, test.errContains)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}
