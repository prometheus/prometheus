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

package histogramconv

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// buildBenchNHCBSeries mirrors the prom-bench synthetic workload: numSeries
// histograms, each with numBuckets custom buckets, each carrying numSamples
// raw points (a 5m range at a 1m scrape interval yields ~6 raw samples).
func buildBenchNHCBSeries(numSeries, numBuckets, numSamples int) []storage.Series {
	customValues := make([]float64, numBuckets)
	for i := range customValues {
		customValues[i] = float64(i + 1)
	}
	// Delta-encoded: first bucket has 1 observation, rest have 0 additional
	// observations, so every bucket's absolute count is 1 and Count sums to
	// numBuckets (native histogram bucket counts are per-bucket, not cumulative).
	positiveBuckets := make([]int64, numBuckets)
	positiveBuckets[0] = 1

	series := make([]storage.Series, numSeries)
	for i := range numSeries {
		lset := labels.FromStrings(
			"__name__", "bench_request_duration_seconds",
			"tenant", fmt.Sprintf("tenant-%d", i%100),
			"handler", fmt.Sprintf("/api/%d", i%50),
			"method", []string{"GET", "POST"}[i%2],
			"status", []string{"200", "400", "500"}[i%3],
			"series", strconv.Itoa(i),
		)

		samples := make([]chunks.Sample, numSamples)
		for j := range numSamples {
			samples[j] = hSample{
				t: int64(j * 60000),
				h: &histogram.Histogram{
					Schema:          histogram.CustomBucketsSchema,
					Count:           uint64(numBuckets),
					Sum:             float64(numBuckets) * 1.5,
					CustomValues:    customValues,
					PositiveSpans:   []histogram.Span{{Offset: 0, Length: uint32(numBuckets)}},
					PositiveBuckets: positiveBuckets,
				},
			}
		}
		series[i] = storage.NewListSeries(lset, samples)
	}
	return series
}

func benchmarkNHCBAsClassicSelect(b *testing.B, suffix string) {
	const (
		numSeries  = 1000
		numBuckets = 30
		numSamples = 6
	)
	nhcbSeries := buildBenchNHCBSeries(numSeries, numBuckets, numSamples)

	mock := &nhcbMockQuerier{
		classicSeries: []storage.Series{},
		nhcbSeries:    nhcbSeries,
	}
	q := NewNHCBAsClassicQuerier(mock)
	matcher := labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "bench_request_duration_seconds"+suffix)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ss := q.Select(context.Background(), false, nil, matcher)
		count := 0
		for ss.Next() {
			count++
			_ = ss.At().Labels()
		}
		if err := ss.Err(); err != nil {
			b.Fatal(err)
		}
		if count == 0 {
			b.Fatal("expected series, got none")
		}
	}
}

func BenchmarkNHCBAsClassicSelect_Bucket(b *testing.B) {
	benchmarkNHCBAsClassicSelect(b, "_bucket")
}

func BenchmarkNHCBAsClassicSelect_Count(b *testing.B) {
	benchmarkNHCBAsClassicSelect(b, "_count")
}

func BenchmarkNHCBAsClassicSelect_Sum(b *testing.B) {
	benchmarkNHCBAsClassicSelect(b, "_sum")
}
