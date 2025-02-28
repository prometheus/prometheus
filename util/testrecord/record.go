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
	Realistic1000Samples       RefSamplesCase = "real1000"
	Realistic1000WithCTSamples RefSamplesCase = "real1000-ct"
	WorstCase1000Samples       RefSamplesCase = "worst1000"
)

func GenTestRefSamplesCase(t testing.TB, c RefSamplesCase) []record.RefSample {
	t.Helper()

	ret := make([]record.RefSample, 1e3)
	switch c {
	case Realistic1000Samples:
		for i := range ret {
			ret[i].Ref = chunks.HeadSeriesRef(i)
			ret[i].T = 12423423
			ret[i].V = highVarianceFloat(i)
		}
	case Realistic1000WithCTSamples:
		for i := range ret {
			ret[i].Ref = chunks.HeadSeriesRef(i)
			// For cumulative or gauges, typically in one record from
			// scrape we would have exactly same CT and T values.
			ret[i].CT = 11234567
			ret[i].T = 12423423
			ret[i].V = highVarianceFloat(i)
		}
	case WorstCase1000Samples:
		for i := range ret {
			ret[i].Ref = chunks.HeadSeriesRef(i)

			// Worst case is when the values are significantly different
			// to each other which breaks delta encoding.
			ret[i].CT = highVarianceInt(i)
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
