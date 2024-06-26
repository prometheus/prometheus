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

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
)

func testIntHistogram() histogram.Histogram {
	return histogram.Histogram{
		CounterResetHint: histogram.GaugeType,
		Schema:           0,
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
	}
}

func testFloatHistogram() histogram.FloatHistogram {
	return histogram.FloatHistogram{
		CounterResetHint: histogram.GaugeType,
		Schema:           0,
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
	}
}

func TestHistogram[T HistogramConvertable](t *testing.T, converter T, fromInt FromIntHistogramFunc[T], fromFloat FromFloatHistogramFunc[T]) {
	t.Helper()

	t.Run("ToHistogram_Empty", func(t *testing.T) {
		require.NotNilf(t, converter.ToIntHistogram(), "")
		require.NotNilf(t, converter.ToFloatHistogram(), "")
	})
	t.Run("FromIntToFloatOrIntHistogram", func(t *testing.T) {
		h := testIntHistogram()
		fh := testFloatHistogram()

		got := fromInt(123, &h)
		require.False(t, got.IsFloatHistogram())
		require.Equal(t, int64(123), got.T())
		require.Equal(t, &h, got.ToIntHistogram())
		require.Equal(t, &fh, got.ToFloatHistogram())
	})
	t.Run("FromFloatToFloatHistogram(", func(t *testing.T) {
		fh := testFloatHistogram()

		got := fromFloat(123, &fh)
		require.True(t, got.IsFloatHistogram())
		require.Equal(t, int64(123), got.T())
		require.Equal(t, &fh, got.ToFloatHistogram())
	})
}
