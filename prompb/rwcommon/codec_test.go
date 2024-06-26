// Copyright 2024 Prometheus Team
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

package rwcommon

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"

	"github.com/prometheus/prometheus/model/histogram"

	"github.com/prometheus/prometheus/prompb"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
)

func TestToLabels(t *testing.T) {
	expected := labels.FromStrings("__name__", "metric1", "foo", "bar")

	t.Run("v1", func(t *testing.T) {
		ts := prompb.TimeSeries{Labels: []prompb.Label{{Name: "__name__", Value: "metric1"}, {Name: "foo", Value: "bar"}}}
		b := labels.NewScratchBuilder(2)
		require.Equal(t, expected, ts.ToLabels(&b, nil))
		require.Equal(t, ts.Labels, prompb.FromLabels(expected, nil))
		require.Equal(t, ts.Labels, prompb.FromLabels(expected, ts.Labels))
	})
	t.Run("v2", func(t *testing.T) {
		v2Symbols := []string{"", "__name__", "metric1", "foo", "bar"}
		ts := writev2.TimeSeries{LabelsRefs: []uint32{1, 2, 3, 4}}
		b := labels.NewScratchBuilder(2)
		require.Equal(t, expected, ts.ToLabels(&b, v2Symbols))
		// No need for FromLabels in our prod code as we use symbol table to do so.
	})
}

func TestFromMetadataType(t *testing.T) {
	for _, tc := range []struct {
		desc       string
		input      model.MetricType
		expectedV1 prompb.MetricMetadata_MetricType
		expectedV2 writev2.Metadata_MetricType
	}{
		{
			desc:       "with a single-word metric",
			input:      model.MetricTypeCounter,
			expectedV1: prompb.MetricMetadata_COUNTER,
			expectedV2: writev2.Metadata_METRIC_TYPE_COUNTER,
		},
		{
			desc:       "with a two-word metric",
			input:      model.MetricTypeStateset,
			expectedV1: prompb.MetricMetadata_STATESET,
			expectedV2: writev2.Metadata_METRIC_TYPE_STATESET,
		},
		{
			desc:       "with an unknown metric",
			input:      "not-known",
			expectedV1: prompb.MetricMetadata_UNKNOWN,
			expectedV2: writev2.Metadata_METRIC_TYPE_UNSPECIFIED,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			t.Run("v1", func(t *testing.T) {
				require.Equal(t, tc.expectedV1, prompb.FromMetadataType(tc.input))
			})
			t.Run("v2", func(t *testing.T) {
				require.Equal(t, tc.expectedV2, writev2.FromMetadataType(tc.input))
			})
		})
	}
}

func TestFromIntOrFloatHistogram_ResetHint(t *testing.T) {
	for _, tc := range []struct {
		input      histogram.CounterResetHint
		expectedV1 prompb.Histogram_ResetHint
		expectedV2 writev2.Histogram_ResetHint
	}{
		{
			input:      histogram.UnknownCounterReset,
			expectedV1: prompb.Histogram_UNKNOWN,
			expectedV2: writev2.Histogram_RESET_HINT_UNSPECIFIED,
		},
		{
			input:      histogram.CounterReset,
			expectedV1: prompb.Histogram_YES,
			expectedV2: writev2.Histogram_RESET_HINT_YES,
		},
		{
			input:      histogram.NotCounterReset,
			expectedV1: prompb.Histogram_NO,
			expectedV2: writev2.Histogram_RESET_HINT_NO,
		},
		{
			input:      histogram.GaugeType,
			expectedV1: prompb.Histogram_GAUGE,
			expectedV2: writev2.Histogram_RESET_HINT_GAUGE,
		},
	} {
		t.Run("", func(t *testing.T) {
			t.Run("v1", func(t *testing.T) {
				h := testIntHistogram()
				h.CounterResetHint = tc.input
				got := prompb.FromIntHistogram(1337, &h)
				require.Equal(t, tc.expectedV1, got.GetResetHint())

				fh := testFloatHistogram()
				fh.CounterResetHint = tc.input
				got2 := prompb.FromFloatHistogram(1337, &fh)
				require.Equal(t, tc.expectedV1, got2.GetResetHint())
			})
			t.Run("v2", func(t *testing.T) {
				h := testIntHistogram()
				h.CounterResetHint = tc.input
				got := writev2.FromIntHistogram(1337, &h)
				require.Equal(t, tc.expectedV2, got.GetResetHint())

				fh := testFloatHistogram()
				fh.CounterResetHint = tc.input
				got2 := writev2.FromFloatHistogram(1337, &fh)
				require.Equal(t, tc.expectedV2, got2.GetResetHint())
			})
		})
	}
}
