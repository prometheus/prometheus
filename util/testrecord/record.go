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

package testrecord

import (
	"math"
	"testing"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
)

type RefSamplesCase string

const (
	Realistic1000Samples               RefSamplesCase = "real1000"
	Realistic1000WithVariableSTSamples RefSamplesCase = "real1000-vst"
	Realistic1000WithConstSTSamples    RefSamplesCase = "real1000-cst"
	WorstCase1000                      RefSamplesCase = "worst1000"
	WorstCase1000WithSTSamples         RefSamplesCase = "worst1000-st"
)

func GenTestRefSamplesCase(t testing.TB, c RefSamplesCase) []record.RefSample {
	t.Helper()

	ret := make([]record.RefSample, 1e3)
	switch c {
	// Samples are across series, so likely all have the same timestamp.
	case Realistic1000Samples:
		for i := range ret {
			ret[i].Ref = chunks.HeadSeriesRef(i)
			ret[i].T = int64(12423423)
			ret[i].V = highVarianceFloat(i)
		}
	// Likely the start times will all be the same with deltas.
	case Realistic1000WithConstSTSamples:
		for i := range ret {
			ret[i].Ref = chunks.HeadSeriesRef(i)
			ret[i].ST = int64(12423423)
			ret[i].T = int64(12423423 + 15)
			ret[i].V = highVarianceFloat(i)
		}
	// Maybe series have different start times though
	case Realistic1000WithVariableSTSamples:
		for i := range ret {
			ret[i].Ref = chunks.HeadSeriesRef(i)
			ret[i].ST = int64((12423423 / 9) * (i % 10))
			ret[i].T = int64(12423423)
			ret[i].V = highVarianceFloat(i)
		}
	case WorstCase1000:
		for i := range ret {
			ret[i].Ref = chunks.HeadSeriesRef(i)
			ret[i].T = highVarianceInt(i)
			ret[i].V = highVarianceFloat(i)
		}
	case WorstCase1000WithSTSamples:
		for i := range ret {
			ret[i].Ref = chunks.HeadSeriesRef(i)

			// Worst case is when the values are significantly different
			// to each other which breaks delta encoding.
			ret[i].ST = highVarianceInt(i+1) / 1024 // Make sure ST is not comparable to T
			ret[i].T = highVarianceInt(i)
			ret[i].V = highVarianceFloat(i)
		}
	default:
		t.Fatal("unknown case", c)
	}
	return ret
}

// HistSTCase selects the start-time pattern for histogram test data generators.
type HistSTCase string

const (
	HistNoST       HistSTCase = "no-st"
	HistConstST    HistSTCase = "const-st"
	HistPrevTST    HistSTCase = "prevt-st"
	HistVariableST HistSTCase = "var-st"
)

// HistSTCases is the standard set of histogram ST cases for benchmarks.
var HistSTCases = []HistSTCase{HistNoST, HistConstST, HistPrevTST, HistVariableST}

// applyHistST sets the ST field on histogram samples according to the given case.
func applyHistST(out []record.RefHistogramSample, stCase HistSTCase) {
	switch stCase {
	case HistNoST:
		// Nothing to do.
	case HistConstST:
		for i := range out {
			out[i].ST = 1709000000
		}
	case HistPrevTST:
		for i := range out {
			if i == 0 {
				continue
			}
			out[i].ST = out[i-1].T
		}
	case HistVariableST:
		for i := range out {
			out[i].ST = highVarianceInt(i+1) / 1024
		}
	}
}

// GenExpHistograms generates n standard exponential histograms (schema=1)
// with incrementing refs, same timestamp, and realistic bucket distributions.
// The stCase parameter controls how the ST field is populated.
func GenExpHistograms(n int, stCase HistSTCase) []record.RefHistogramSample {
	out := make([]record.RefHistogramSample, n)
	for i := range out {
		out[i] = record.RefHistogramSample{
			Ref: chunks.HeadSeriesRef(i),
			T:   1709000000 + int64(i)*15,
			H: &histogram.Histogram{
				Count:         uint64(10 + i%100),
				ZeroCount:     uint64(1 + i%5),
				ZeroThreshold: 0.001,
				Sum:           float64(100+i) * 1.5,
				Schema:        1,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 4},
					{Offset: 2, Length: 3},
				},
				PositiveBuckets: []int64{1, 2, -1, 0, 3, -2, 1},
				NegativeSpans: []histogram.Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				NegativeBuckets: []int64{1, 1, -1, 0},
			},
		}
	}
	applyHistST(out, stCase)
	return out
}

// GenCustomBucketHistograms generates n custom-bucket (NHCB) histograms (schema=-53)
// with incrementing refs. The stCase parameter controls how the ST field is populated.
func GenCustomBucketHistograms(n int, stCase HistSTCase) []record.RefHistogramSample {
	out := make([]record.RefHistogramSample, n)
	for i := range out {
		out[i] = record.RefHistogramSample{
			Ref: chunks.HeadSeriesRef(i),
			T:   1709000000 + int64(i)*15,
			H: &histogram.Histogram{
				Count:  uint64(10 + i%100),
				Sum:    float64(100+i) * 1.5,
				Schema: histogram.CustomBucketsSchema,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 8},
				},
				PositiveBuckets: []int64{5, -2, 3, -1, 4, 0, -3, 2},
				CustomValues:    []float64{0.001, 0.01, 0.1, 1, 10, 100, 1000},
			},
		}
	}
	applyHistST(out, stCase)
	return out
}

// GenFloatHistograms converts int histograms to float histograms, preserving ST.
func GenFloatHistograms(src []record.RefHistogramSample) []record.RefFloatHistogramSample {
	out := make([]record.RefFloatHistogramSample, len(src))
	for i, h := range src {
		out[i] = record.RefFloatHistogramSample{
			Ref: h.Ref,
			ST:  h.ST,
			T:   h.T,
			FH:  h.H.ToFloat(nil),
		}
	}
	return out
}

// HistDataCase pairs a name with a histogram generator for benchmark tables.
type HistDataCase struct {
	Name string
	Gen  func(n int, stCase HistSTCase) []record.RefHistogramSample
}

// HistDataCases is the standard set of histogram data cases for benchmarks.
var HistDataCases = []HistDataCase{
	{"exp", GenExpHistograms},
	{"nhcb", GenCustomBucketHistograms},
}

// HistCounts is the standard set of histogram counts for benchmarks.
var HistCounts = []int{10, 100, 1000}

func highVarianceInt(i int) int64 {
	if i%2 == 0 {
		return math.MinInt32
	}
	return math.MaxInt32
}

func highVarianceFloat(i int) float64 {
	if i%2 == 0 {
		return math.SmallestNonzeroFloat32
	}
	return math.MaxFloat32
}
