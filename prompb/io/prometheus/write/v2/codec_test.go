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

package writev2

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestToLabels(t *testing.T) {
	// symbols[0] is always empty string per spec
	symbols := []string{"", "__name__", "test_metric", "foo", "bar", "__type__", "counter", "__unit__", "bytes"}

	for _, tc := range []struct {
		name           string
		ts             TimeSeries
		addTypeAndUnit bool
		expectedLabels labels.Labels
		expectError    bool
	}{
		{
			name: "basic labels without type and unit",
			ts: TimeSeries{
				LabelsRefs: []uint32{1, 2, 3, 4}, // __name__=test_metric, foo=bar
			},
			addTypeAndUnit: false,
			expectedLabels: labels.FromStrings("__name__", "test_metric", "foo", "bar"),
		},
		{
			name: "labels with type and unit from metadata",
			ts: TimeSeries{
				LabelsRefs: []uint32{1, 2, 3, 4}, // __name__=test_metric, foo=bar
				Metadata: Metadata{
					Type:    Metadata_METRIC_TYPE_COUNTER,
					UnitRef: 8, // "bytes"
				},
			},
			addTypeAndUnit: true,
			expectedLabels: labels.FromStrings("__name__", "test_metric", "__type__", "counter", "__unit__", "bytes", "foo", "bar"),
		},
		{
			name: "labels with type only (no unit)",
			ts: TimeSeries{
				LabelsRefs: []uint32{1, 2, 3, 4}, // __name__=test_metric, foo=bar
				Metadata: Metadata{
					Type:    Metadata_METRIC_TYPE_GAUGE,
					UnitRef: 0, // empty string
				},
			},
			addTypeAndUnit: true,
			expectedLabels: labels.FromStrings("__name__", "test_metric", "__type__", "gauge", "foo", "bar"),
		},
		{
			name: "labels with existing __type__ should not duplicate",
			ts: TimeSeries{
				LabelsRefs: []uint32{1, 2, 5, 6, 3, 4}, // __name__=test_metric, __type__=counter, foo=bar
				Metadata: Metadata{
					Type:    Metadata_METRIC_TYPE_COUNTER,
					UnitRef: 0,
				},
			},
			addTypeAndUnit: true,
			expectedLabels: labels.FromStrings("__name__", "test_metric", "__type__", "counter", "foo", "bar"),
		},
		{
			name: "labels with existing __unit__ should not duplicate",
			ts: TimeSeries{
				LabelsRefs: []uint32{1, 2, 7, 8, 3, 4}, // __name__=test_metric, __unit__=bytes, foo=bar
				Metadata: Metadata{
					Type:    Metadata_METRIC_TYPE_GAUGE,
					UnitRef: 8, // "bytes"
				},
			},
			addTypeAndUnit: true,
			expectedLabels: labels.FromStrings("__name__", "test_metric", "__type__", "gauge", "__unit__", "bytes", "foo", "bar"),
		},
		{
			name: "addTypeAndUnit false ignores metadata even if present",
			ts: TimeSeries{
				LabelsRefs: []uint32{1, 2, 3, 4}, // __name__=test_metric, foo=bar
				Metadata: Metadata{
					Type:    Metadata_METRIC_TYPE_COUNTER,
					UnitRef: 8,
				},
			},
			addTypeAndUnit: false,
			expectedLabels: labels.FromStrings("__name__", "test_metric", "foo", "bar"),
		},
		{
			name: "unknown metric type does not add __type__",
			ts: TimeSeries{
				LabelsRefs: []uint32{1, 2, 3, 4},
				Metadata: Metadata{
					Type:    Metadata_METRIC_TYPE_UNSPECIFIED,
					UnitRef: 8, // "bytes"
				},
			},
			addTypeAndUnit: true,
			expectedLabels: labels.FromStrings("__name__", "test_metric", "__unit__", "bytes", "foo", "bar"),
		},
		{
			name: "invalid UnitRef returns error",
			ts: TimeSeries{
				LabelsRefs: []uint32{1, 2, 3, 4},
				Metadata: Metadata{
					Type:    Metadata_METRIC_TYPE_COUNTER,
					UnitRef: 999, // out of bounds
				},
			},
			addTypeAndUnit: true,
			expectError:    true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := labels.NewScratchBuilder(0)
			lbls, err := tc.ts.ToLabels(&b, symbols, tc.addTypeAndUnit)

			if tc.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expectedLabels, lbls)
		})
	}
}

func TestToExemplar(t *testing.T) {
	symbols := []string{"", "trace_id", "abc123", "span_id", "def456"}

	ex := Exemplar{
		LabelsRefs: []uint32{1, 2, 3, 4}, // trace_id=abc123, span_id=def456
		Value:      1.5,
		Timestamp:  1234567890,
	}

	b := labels.NewScratchBuilder(0)
	result, err := ex.ToExemplar(&b, symbols)

	require.NoError(t, err)
	require.Equal(t, labels.FromStrings("span_id", "def456", "trace_id", "abc123"), result.Labels)
	require.Equal(t, 1.5, result.Value)
	require.Equal(t, int64(1234567890), result.Ts)
	require.True(t, result.HasTs)
}
