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

package tsdbutil

import (
	"math"

	"github.com/prometheus/prometheus/model/histogram"
)

func GenerateTestHistograms(n int) (r []*histogram.Histogram) {
	for i := range n {
		h := GenerateTestHistogram(int64(i))
		if i > 0 {
			h.CounterResetHint = histogram.NotCounterReset
		}
		r = append(r, h)
	}
	return r
}

func GenerateTestHistogramWithHint(n int, hint histogram.CounterResetHint) *histogram.Histogram {
	h := GenerateTestHistogram(int64(n))
	h.CounterResetHint = hint
	return h
}

// GenerateTestHistogram but it is up to the user to set any known counter reset hint.
func GenerateTestHistogram(i int64) *histogram.Histogram {
	return &histogram.Histogram{
		Count:         12 + uint64(i*9),
		ZeroCount:     2 + uint64(i),
		ZeroThreshold: 0.001,
		Sum:           18.4 * float64(i+1),
		Schema:        1,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{i + 1, 1, -1, 0},
		NegativeSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		NegativeBuckets: []int64{i + 1, 1, -1, 0},
	}
}

func GenerateTestCustomBucketsHistograms(n int) (r []*histogram.Histogram) {
	for i := range n {
		h := GenerateTestCustomBucketsHistogram(int64(i))
		if i > 0 {
			h.CounterResetHint = histogram.NotCounterReset
		}
		r = append(r, h)
	}
	return r
}

func GenerateTestCustomBucketsHistogram(i int64) *histogram.Histogram {
	return &histogram.Histogram{
		Count:  5 + uint64(i*4),
		Sum:    18.4 * float64(i+1),
		Schema: histogram.CustomBucketsSchema,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{i + 1, 1, -1, 0},
		CustomValues:    []float64{0, 1, 2, 3, 4},
	}
}

func GenerateTestGaugeHistograms(n int) (r []*histogram.Histogram) {
	for x := range n {
		i := int64(math.Sin(float64(x))*100) + 100
		r = append(r, GenerateTestGaugeHistogram(i))
	}
	return r
}

func GenerateTestGaugeHistogram(i int64) *histogram.Histogram {
	h := GenerateTestHistogram(i)
	h.CounterResetHint = histogram.GaugeType
	return h
}

func GenerateTestFloatHistograms(n int) (r []*histogram.FloatHistogram) {
	for i := range n {
		h := GenerateTestFloatHistogram(int64(i))
		if i > 0 {
			h.CounterResetHint = histogram.NotCounterReset
		}
		r = append(r, h)
	}
	return r
}

// GenerateTestFloatHistogram but it is up to the user to set any known counter reset hint.
func GenerateTestFloatHistogram(i int64) *histogram.FloatHistogram {
	return &histogram.FloatHistogram{
		Count:         12 + float64(i*9),
		ZeroCount:     2 + float64(i),
		ZeroThreshold: 0.001,
		Sum:           18.4 * float64(i+1),
		Schema:        1,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []float64{float64(i + 1), float64(i + 2), float64(i + 1), float64(i + 1)},
		NegativeSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		NegativeBuckets: []float64{float64(i + 1), float64(i + 2), float64(i + 1), float64(i + 1)},
	}
}

func GenerateTestCustomBucketsFloatHistograms(n int) (r []*histogram.FloatHistogram) {
	for i := range n {
		h := GenerateTestCustomBucketsFloatHistogram(int64(i))
		if i > 0 {
			h.CounterResetHint = histogram.NotCounterReset
		}
		r = append(r, h)
	}
	return r
}

func GenerateTestCustomBucketsFloatHistogram(i int64) *histogram.FloatHistogram {
	return &histogram.FloatHistogram{
		Count:  5 + float64(i*4),
		Sum:    18.4 * float64(i+1),
		Schema: histogram.CustomBucketsSchema,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []float64{float64(i + 1), float64(i + 2), float64(i + 1), float64(i + 1)},
		CustomValues:    []float64{0, 1, 2, 3, 4},
	}
}

func GenerateTestGaugeFloatHistograms(n int) (r []*histogram.FloatHistogram) {
	for x := range n {
		i := int64(math.Sin(float64(x))*100) + 100
		r = append(r, GenerateTestGaugeFloatHistogram(i))
	}
	return r
}

func GenerateTestGaugeFloatHistogram(i int64) *histogram.FloatHistogram {
	h := GenerateTestFloatHistogram(i)
	h.CounterResetHint = histogram.GaugeType
	return h
}

func SetHistogramNotCounterReset(h *histogram.Histogram) *histogram.Histogram {
	h.CounterResetHint = histogram.NotCounterReset
	return h
}

func SetHistogramCounterReset(h *histogram.Histogram) *histogram.Histogram {
	h.CounterResetHint = histogram.CounterReset
	return h
}

func SetFloatHistogramNotCounterReset(h *histogram.FloatHistogram) *histogram.FloatHistogram {
	h.CounterResetHint = histogram.NotCounterReset
	return h
}

func SetFloatHistogramCounterReset(h *histogram.FloatHistogram) *histogram.FloatHistogram {
	h.CounterResetHint = histogram.CounterReset
	return h
}
