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

package tsdb

import (
	"fmt"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
)

// BenchmarkHeadMetricMetadataQuery includes sparse metadata and bounded responses.
func BenchmarkHeadMetricMetadataQuery(b *testing.B) {
	benchmarkHeadMetricMetadataQuery(b, 10_000)
}

// BenchmarkHeadMetricMetadataQuerySmall covers small result sets, sparse metadata, and bounded responses.
func BenchmarkHeadMetricMetadataQuerySmall(b *testing.B) {
	for _, size := range []int{1, 64, 256, 257} {
		b.Run(fmt.Sprintf("series=%d", size), func(b *testing.B) {
			benchmarkHeadMetricMetadataQuery(b, size)
		})
	}
}

func benchmarkHeadMetricMetadataQuery(b *testing.B, numSeries int) {
	for _, versions := range []int{1, 5} {
		for _, every := range []int{0, 1, 100} {
			for _, limit := range []int{10, 0} {
				// Empty metadata needs only one history-depth and limit control.
				if every == 0 && (versions != 1 || limit != 10) {
					continue
				}
				// Both densities select the same series in a singleton fixture.
				if numSeries == 1 && every == 100 {
					continue
				}
				b.Run(fmt.Sprintf("versions=%d/every=%d/limit=%d", versions, every, limit), func(b *testing.B) {
					mode := metricMetadataBenchmarkMode{nativeEnabled: true}
					h, _, closeHead := newMetricMetadataBenchmarkHead(b, mode, 1_000_000_000, false)
					b.Cleanup(closeHead)
					fixture := newMetricMetadataBenchmarkFixture(numSeries, min(numSeries, 100), versions)
					refs := make([]storage.SeriesRef, numSeries)
					for version := range versions {
						app := h.AppenderV2(b.Context())
						for i, lset := range fixture.labels {
							var opts storage.AOptions
							if every > 0 && i%every == 0 {
								opts = fixture.options[version][fixture.familyBySeries[i]]
							}
							ref, err := app.Append(refs[i], lset, 0, int64(100+version), float64(version), nil, nil, opts)
							if err != nil {
								b.Fatal(err)
							}
							refs[i] = ref
						}
						if err := app.Commit(); err != nil {
							b.Fatal(err)
						}
					}
					matchers := [][]*labels.Matcher{{labels.MustNewMatcher(labels.MatchEqual, "job", "metadata-benchmark")}}
					var total int
					if every > 0 {
						total = (numSeries + every - 1) / every
					}
					want := total
					if limit > 0 {
						want = min(want, limit)
					}
					b.ReportAllocs()
					for b.Loop() {
						got, truncated, err := h.nativeMetricMetadataForMatchers(b.Context(), matchers, limit)
						if err != nil {
							b.Fatal(err)
						}
						if len(got) != want || truncated != (limit > 0 && limit < total) {
							b.Fatalf("unexpected query result: %d rows, truncated=%t", len(got), truncated)
						}
						if len(got) > 0 && len(got[0].Versions) != versions {
							b.Fatal("unexpected history depth")
						}
					}
				})
			}
		}
	}
}
