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

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
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
		result, err := ts.ToLabels(&b, v2Symbols)
		require.NoError(t, err)
		require.Equal(t, expected, result)
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

func TestToMetadata(t *testing.T) {
	sym := writev2.NewSymbolTable()

	for _, tc := range []struct {
		input    writev2.Metadata
		expected metadata.Metadata
	}{
		{
			input: writev2.Metadata{},
			expected: metadata.Metadata{
				Type: model.MetricTypeUnknown,
			},
		},
		{
			input: writev2.Metadata{
				Type: 12414, // Unknown.
			},
			expected: metadata.Metadata{
				Type: model.MetricTypeUnknown,
			},
		},
		{
			input: writev2.Metadata{
				Type:    writev2.Metadata_METRIC_TYPE_COUNTER,
				HelpRef: sym.Symbolize("help1"),
				UnitRef: sym.Symbolize("unit1"),
			},
			expected: metadata.Metadata{
				Type: model.MetricTypeCounter,
				Help: "help1",
				Unit: "unit1",
			},
		},
		{
			input: writev2.Metadata{
				Type:    writev2.Metadata_METRIC_TYPE_STATESET,
				HelpRef: sym.Symbolize("help2"),
			},
			expected: metadata.Metadata{
				Type: model.MetricTypeStateset,
				Help: "help2",
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			ts := writev2.TimeSeries{Metadata: tc.input}
			require.Equal(t, tc.expected, ts.ToMetadata(sym.Symbols()))
		})
	}
}

func TestToHistogram_Empty(t *testing.T) {
	t.Run("v1", func(t *testing.T) {
		require.NotNil(t, prompb.Histogram{}.ToIntHistogram())
		require.NotNil(t, prompb.Histogram{}.ToFloatHistogram())
	})
	t.Run("v2", func(t *testing.T) {
		require.NotNil(t, writev2.Histogram{}.ToIntHistogram())
		require.NotNil(t, writev2.Histogram{}.ToFloatHistogram())
	})
}

// NOTE(bwplotka): This is technically not a valid histogram, but it represents
// important cases to test when copying or converting to/from int/float histograms.
func testIntHistogram() histogram.Histogram {
	return histogram.Histogram{
		CounterResetHint: histogram.GaugeType,
		Schema:           1,
		Count:            19,
		Sum:              2.7,
		ZeroThreshold:    1e-128,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 4},
			{Offset: 0, Length: 0},
			{Offset: 0, Length: 3},
		},
		PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 0},
		NegativeSpans: []histogram.Span{
			{Offset: 0, Length: 5},
			{Offset: 1, Length: 0},
			{Offset: 0, Length: 1},
		},
		NegativeBuckets: []int64{1, 2, -2, 1, -1, 0},
		CustomValues:    []float64{21421, 523},
	}
}

// NOTE(bwplotka): This is technically not a valid histogram, but it represents
// important cases to test when copying or converting to/from int/float histograms.
func testFloatHistogram() histogram.FloatHistogram {
	return histogram.FloatHistogram{
		CounterResetHint: histogram.GaugeType,
		Schema:           1,
		Count:            19,
		Sum:              2.7,
		ZeroThreshold:    1e-128,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 4},
			{Offset: 0, Length: 0},
			{Offset: 0, Length: 3},
		},
		PositiveBuckets: []float64{1, 3, 1, 2, 1, 1, 1},
		NegativeSpans: []histogram.Span{
			{Offset: 0, Length: 5},
			{Offset: 1, Length: 0},
			{Offset: 0, Length: 1},
		},
		NegativeBuckets: []float64{1, 3, 1, 2, 1, 1},
		CustomValues:    []float64{21421, 523},
	}
}

func TestFromIntToFloatOrIntHistogram(t *testing.T) {
	t.Run("v1", func(t *testing.T) {
		// v1 does not support nhcb.
		testIntHistWithoutNHCB := testIntHistogram()
		testIntHistWithoutNHCB.CustomValues = nil
		testFloatHistWithoutNHCB := testFloatHistogram()
		testFloatHistWithoutNHCB.CustomValues = nil

		h := prompb.FromIntHistogram(123, &testIntHistWithoutNHCB)
		require.False(t, h.IsFloatHistogram())
		require.Equal(t, int64(123), h.Timestamp)
		require.Equal(t, testIntHistWithoutNHCB, *h.ToIntHistogram())
		require.Equal(t, testFloatHistWithoutNHCB, *h.ToFloatHistogram())
	})
	t.Run("v2", func(t *testing.T) {
		testIntHist := testIntHistogram()
		testFloatHist := testFloatHistogram()

		h := writev2.FromIntHistogram(123, &testIntHist)
		require.False(t, h.IsFloatHistogram())
		require.Equal(t, int64(123), h.Timestamp)
		require.Equal(t, testIntHist, *h.ToIntHistogram())
		require.Equal(t, testFloatHist, *h.ToFloatHistogram())
	})
}

func TestFromFloatToFloatHistogram(t *testing.T) {
	t.Run("v1", func(t *testing.T) {
		// v1 does not support nhcb.
		testFloatHistWithoutNHCB := testFloatHistogram()
		testFloatHistWithoutNHCB.CustomValues = nil

		h := prompb.FromFloatHistogram(123, &testFloatHistWithoutNHCB)
		require.True(t, h.IsFloatHistogram())
		require.Equal(t, int64(123), h.Timestamp)
		require.Nil(t, h.ToIntHistogram())
		require.Equal(t, testFloatHistWithoutNHCB, *h.ToFloatHistogram())
	})
	t.Run("v2", func(t *testing.T) {
		testFloatHist := testFloatHistogram()

		h := writev2.FromFloatHistogram(123, &testFloatHist)
		require.True(t, h.IsFloatHistogram())
		require.Equal(t, int64(123), h.Timestamp)
		require.Nil(t, h.ToIntHistogram())
		require.Equal(t, testFloatHist, *h.ToFloatHistogram())
	})
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
