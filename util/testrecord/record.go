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
