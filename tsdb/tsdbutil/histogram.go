// Copyright 2023 The Prometheus Authors
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
	"math/rand"

	"github.com/prometheus/prometheus/model/histogram"
)

func GenerateTestHistograms(n int) (r []*histogram.Histogram) {
	for i := 0; i < n; i++ {
		h := GenerateTestHistogram(i)
		if i > 0 {
			h.CounterResetHint = histogram.NotCounterReset
		}
		r = append(r, h)
	}
	return r
}

// GenerateTestHistogram but it is up to the user to set any known counter reset hint.
func GenerateTestHistogram(i int) *histogram.Histogram {
	return &histogram.Histogram{
		Count:         10 + uint64(i*8),
		ZeroCount:     2 + uint64(i),
		ZeroThreshold: 0.001,
		Sum:           18.4 * float64(i+1),
		Schema:        1,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{int64(i + 1), 1, -1, 0},
		NegativeSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 1, Length: 2},
		},
		NegativeBuckets: []int64{int64(i + 1), 1, -1, 0},
	}
}

func GenerateTestGaugeHistograms(n int) (r []*histogram.Histogram) {
	for x := 0; x < n; x++ {
		r = append(r, GenerateTestGaugeHistogram(rand.Intn(n)))
	}
	return r
}

func GenerateTestGaugeHistogram(i int) *histogram.Histogram {
	h := GenerateTestHistogram(i)
	h.CounterResetHint = histogram.GaugeType
	return h
}

func GenerateTestFloatHistograms(n int) (r []*histogram.FloatHistogram) {
	for i := 0; i < n; i++ {
		h := GenerateTestFloatHistogram(i)
		if i > 0 {
			h.CounterResetHint = histogram.NotCounterReset
		}
		r = append(r, h)
	}
	return r
}

// GenerateTestFloatHistogram but it is up to the user to set any known counter reset hint.
func GenerateTestFloatHistogram(i int) *histogram.FloatHistogram {
	return &histogram.FloatHistogram{
		Count:         10 + float64(i*8),
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

func GenerateTestGaugeFloatHistograms(n int) (r []*histogram.FloatHistogram) {
	for x := 0; x < n; x++ {
		r = append(r, GenerateTestGaugeFloatHistogram(rand.Intn(n)))
	}
	return r
}

func GenerateTestGaugeFloatHistogram(i int) *histogram.FloatHistogram {
	h := GenerateTestFloatHistogram(i)
	h.CounterResetHint = histogram.GaugeType
	return h
}
