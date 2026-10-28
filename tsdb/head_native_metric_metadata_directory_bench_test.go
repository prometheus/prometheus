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
	"runtime"
	"testing"

	"github.com/prometheus/prometheus/storage"
)

// BenchmarkHeadMetricMetadataDirectoryLifecycle measures first publication and
// removal through Head appenders and GC. Use a fixed iteration count to bound setup.
func BenchmarkHeadMetricMetadataDirectoryLifecycle(b *testing.B) {
	const numSeries = 4096
	fixture := newMetricMetadataBenchmarkFixture(numSeries, 100, 1)
	for _, every := range []int{1, 256} {
		for _, phase := range []string{"publish", "gc"} {
			for _, enabled := range []bool{false, true} {
				b.Run(fmt.Sprintf("every=%d/phase=%s/native=%t", every, phase, enabled), func(b *testing.B) {
					b.ReportAllocs()
					for range b.N {
						b.StopTimer()
						h, _, closeHead := newMetricMetadataBenchmarkHead(b, metricMetadataBenchmarkMode{nativeEnabled: enabled}, false)
						refs := make([]storage.SeriesRef, numSeries)
						app := h.AppenderV2(b.Context())
						for i, ls := range fixture.labels {
							ref, err := app.Append(0, ls, 0, 100, 1, nil, nil, storage.AOptions{})
							if err != nil {
								b.Fatal(err)
							}
							refs[i] = ref
						}
						if err := app.Commit(); err != nil {
							b.Fatal(err)
						}
						if phase == "publish" {
							b.StartTimer()
						}
						app = h.AppenderV2(b.Context())
						for i := 0; i < numSeries; i += every {
							if _, err := app.Append(refs[i], fixture.labels[i], 0, 200, 2, nil, nil, fixture.options[0][fixture.familyBySeries[i]]); err != nil {
								b.Fatal(err)
							}
						}
						if err := app.Commit(); err != nil {
							b.Fatal(err)
						}
						if phase == "gc" {
							b.StartTimer()
							h.minTime.Store(300)
							h.gc()
						}
						b.StopTimer()
						want := int64((numSeries + every - 1) / every)
						if phase == "gc" {
							want = 0
							if h.NumSeries() != 0 {
								b.Fatal("GC retained expired series")
							}
						}
						if enabled && (h.nativeMetricMetadata.series.Load() != want || h.nativeMetricMetadata.versions.Load() != want) {
							b.Fatal("incorrect native metadata accounting")
						}
						closeHead()
					}
				})
			}
		}
	}
}

// BenchmarkHeadMetricMetadataDirectoryHeap compares dense and singleton pages.
// Run each case in fresh processes with -benchtime=1x -cpu=2.
func BenchmarkHeadMetricMetadataDirectoryHeap(b *testing.B) {
	const numSeries = 16384
	for _, every := range []int{1, 256} {
		for _, enabled := range []bool{false, true} {
			b.Run(fmt.Sprintf("every=%d/native=%t", every, enabled), func(b *testing.B) {
				fixture := newMetricMetadataBenchmarkFixture(numSeries, 100, 1)
				var heap metricMetadataBenchmarkHeap
				for range b.N {
					before := readMetricMetadataBenchmarkHeap()
					h, _, closeHead := newMetricMetadataBenchmarkHead(b, metricMetadataBenchmarkMode{nativeEnabled: enabled}, false)
					app := h.AppenderV2(b.Context())
					for i, ls := range fixture.labels {
						var opts storage.AOptions
						if i%every == 0 {
							opts = fixture.options[0][fixture.familyBySeries[i]]
						}
						if _, err := app.Append(0, ls, 0, 100, 1, nil, nil, opts); err != nil {
							b.Fatal(err)
						}
					}
					if err := app.Commit(); err != nil {
						b.Fatal(err)
					}
					heap.addGrowth(b, before)
					runtime.KeepAlive(h)
					runtime.KeepAlive(fixture)
					closeHead()
				}
				heap.report(b, b.N*numSeries)
			})
		}
	}
}
