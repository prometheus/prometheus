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
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/compression"
)

type metricMetadataBenchmarkMode struct {
	name          string
	legacyEnabled bool
	nativeEnabled bool
}

func (m metricMetadataBenchmarkMode) appendOptions(meta metadata.Metadata) storage.AOptions {
	// The RW2 receiver always supplies NativeMetricMetadata. The Head ignores it
	// when native metadata is disabled.
	opts := storage.AOptions{NativeMetricMetadata: meta}
	if m.legacyEnabled {
		opts.Metadata = meta
	}
	return opts
}

func metricMetadataBenchmarkModes() []metricMetadataBenchmarkMode {
	return []metricMetadataBenchmarkMode{
		{name: "baseline"},
		{name: "legacy", legacyEnabled: true},
		{name: "native", nativeEnabled: true},
	}
}

type metricMetadataBenchmarkFixture struct {
	labels         []labels.Labels
	familyBySeries []int
	options        [][]storage.AOptions
}

func newMetricMetadataBenchmarkFixture(mode metricMetadataBenchmarkMode, numSeries, numFamilies, numVariants int) *metricMetadataBenchmarkFixture {
	metricNames := make([]string, numFamilies)
	for family := range numFamilies {
		metricNames[family] = fmt.Sprintf("metadata_benchmark_%03d_total", family)
	}

	fixture := &metricMetadataBenchmarkFixture{
		labels:         make([]labels.Labels, numSeries),
		familyBySeries: make([]int, numSeries),
		options:        make([][]storage.AOptions, numVariants),
	}
	for i := range numSeries {
		family := i % numFamilies
		fixture.familyBySeries[i] = family
		fixture.labels[i] = labels.FromStrings(
			labels.MetricName, metricNames[family],
			"instance", strconv.Itoa(i),
			"job", "metadata-benchmark",
		)
	}
	for variant := range numVariants {
		fixture.options[variant] = make([]storage.AOptions, numFamilies)
		for family := range numFamilies {
			meta := metadata.Metadata{
				Type: model.MetricTypeCounter,
				Unit: "requests",
				Help: fmt.Sprintf("Total requests processed by benchmark family %03d, metadata version %02d.", family, variant),
			}
			fixture.options[variant][family] = mode.appendOptions(meta)
		}
	}
	return fixture
}

func newMetricMetadataBenchmarkHead(b *testing.B, mode metricMetadataBenchmarkMode, chunkRange int64, withWAL bool) (*Head, *wlog.WL, func()) {
	dir := b.TempDir()
	var wal *wlog.WL
	if withWAL {
		var err error
		wal, err = wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), wlog.DefaultSegmentSize, compression.Snappy)
		if err != nil {
			b.Fatal(err)
		}
	}

	opts := newTestHeadDefaultOptions(chunkRange, false)
	opts.ChunkDirRoot = dir
	opts.EnableExemplarStorage = false
	opts.EnableMetadataWALRecords = mode.legacyEnabled
	opts.EnableNativeMetadata = mode.nativeEnabled
	h, err := NewHead(nil, nil, wal, nil, opts, nil)
	if err != nil {
		if wal != nil {
			_ = wal.Close()
		}
		b.Fatal(err)
	}
	if err := h.chunkDiskMapper.IterateAllChunks(func(chunks.HeadSeriesRef, chunks.ChunkDiskMapperRef, int64, int64, uint16, chunkenc.Encoding, bool) error {
		return nil
	}); err != nil {
		_ = h.Close()
		b.Fatal(err)
	}

	return h, wal, func() {
		if err := h.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

func metricMetadataBenchmarkWALPosition(b *testing.B, wal *wlog.WL) int64 {
	segment, offset, err := wal.LastSegmentAndOffset()
	if err != nil {
		b.Fatal(err)
	}
	return int64(segment)*int64(wlog.DefaultSegmentSize) + int64(offset)
}

func metricMetadataBenchmarkHeapAlloc() uint64 {
	// The second collection clears sync.Pool victim caches left by the commit.
	runtime.GC()
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}

func appendMetricMetadataBenchmarkRound(b *testing.B, h *Head, fixture *metricMetadataBenchmarkFixture, refs []storage.SeriesRef, variant int, timestamp int64) {
	app := h.AppenderV2(b.Context())
	options := fixture.options[variant]
	for i, lset := range fixture.labels {
		ref, err := app.Append(refs[i], lset, 0, timestamp, float64(timestamp), nil, nil, options[fixture.familyBySeries[i]])
		if err != nil {
			b.Fatal(err)
		}
		refs[i] = ref
	}
	if err := app.Commit(); err != nil {
		b.Fatal(err)
	}
}

func validateMetricMetadataBenchmarkState(b *testing.B, h *Head, mode metricMetadataBenchmarkMode, fixture *metricMetadataBenchmarkFixture, refs []storage.SeriesRef, numVersions int) {
	if got, want := h.NumSeries(), uint64(len(refs)); got != want {
		b.Fatalf("unexpected series count: got %d, want %d", got, want)
	}

	switch {
	case mode.nativeEnabled:
		if h.nativeMetricMetadata == nil {
			b.Fatal("native metadata store was not created")
		}
		if got, want := h.nativeMetricMetadata.series.Load(), int64(len(refs)); got != want {
			b.Fatalf("unexpected native metadata series count: got %d, want %d", got, want)
		}
		versionsPerSeries := min(numVersions, maxNativeMetricMetadataVersions)
		if got, want := h.nativeMetricMetadata.versions.Load(), int64(len(refs)*versionsPerSeries); got != want {
			b.Fatalf("unexpected native metadata version count: got %d, want %d", got, want)
		}
		evictionsPerSeries := max(numVersions-maxNativeMetricMetadataVersions, 0)
		if got, want := h.nativeMetricMetadata.evictions.Load(), uint64(len(refs)*evictionsPerSeries); got != want {
			b.Fatalf("unexpected native metadata eviction count: got %d, want %d", got, want)
		}
		for _, i := range []int{0, len(refs) - 1} {
			versions, truncated, ok := h.nativeMetricMetadata.get(chunks.HeadSeriesRef(refs[i]))
			if !ok {
				b.Fatalf("native metadata for series %d was not found", refs[i])
			}
			if len(versions) != versionsPerSeries {
				b.Fatalf("unexpected native metadata history length for series %d: got %d, want %d", refs[i], len(versions), versionsPerSeries)
			}
			if want := numVersions > maxNativeMetricMetadataVersions; truncated != want {
				b.Fatalf("unexpected native metadata truncation for series %d: got %t, want %t", refs[i], truncated, want)
			}
		}
	case mode.legacyEnabled:
		lastVariant := (numVersions - 1) % len(fixture.options)
		for _, i := range []int{0, len(refs) - 1} {
			series := h.series.getByID(chunks.HeadSeriesRef(refs[i]))
			if series == nil {
				b.Fatalf("series %d was not found", refs[i])
			}
			series.Lock()
			got := series.meta
			series.Unlock()
			want := fixture.options[lastVariant][fixture.familyBySeries[i]].Metadata
			if got == nil || !got.Equals(want) {
				b.Fatalf("unexpected legacy metadata for series %d: got %v, want %v", refs[i], got, want)
			}
		}
	default:
		if h.nativeMetricMetadata != nil {
			b.Fatal("baseline unexpectedly created a native metadata store")
		}
		for _, i := range []int{0, len(refs) - 1} {
			series := h.series.getByID(chunks.HeadSeriesRef(refs[i]))
			if series == nil {
				b.Fatalf("series %d was not found", refs[i])
			}
			series.Lock()
			got := series.meta
			series.Unlock()
			if got != nil {
				b.Fatalf("baseline unexpectedly retained metadata for series %d", refs[i])
			}
		}
	}
}

// BenchmarkHeadMetricMetadataRetained compares initial ingestion and retained
// per-series heap. Run each case six times in fresh processes with
// -benchtime=1x -cpu=2 so global interning state cannot cross cases.
func BenchmarkHeadMetricMetadataRetained(b *testing.B) {
	scenarios := []struct {
		name        string
		numSeries   int
		numFamilies int
		numVersions int
		numVariants int
	}{
		{name: "stable", numSeries: 100_000, numFamilies: 100, numVersions: 1, numVariants: 1},
		{name: "stable-unique", numSeries: 25_000, numFamilies: 25_000, numVersions: 1, numVariants: 1},
		{name: "changes=4", numSeries: 25_000, numFamilies: 100, numVersions: 4, numVariants: 4},
		{name: fmt.Sprintf("changes=%d", maxNativeMetricMetadataVersions+1), numSeries: 10_000, numFamilies: 100, numVersions: maxNativeMetricMetadataVersions + 1, numVariants: 2},
	}

	for _, scenario := range scenarios {
		for _, mode := range metricMetadataBenchmarkModes() {
			b.Run(fmt.Sprintf("scenario=%s/mode=%s", scenario.name, mode.name), func(b *testing.B) {
				fixture := newMetricMetadataBenchmarkFixture(mode, scenario.numSeries, scenario.numFamilies, scenario.numVariants)
				refs := make([]storage.SeriesRef, scenario.numSeries)
				var totalHeapBytes uint64
				var totalWALBytes int64

				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					b.StopTimer()
					clear(refs)
					beforeHeap := metricMetadataBenchmarkHeapAlloc()
					h, wal, closeHead := newMetricMetadataBenchmarkHead(b, mode, 1_000_000_000, true)
					beforeWAL := metricMetadataBenchmarkWALPosition(b, wal)
					b.StartTimer()

					for version := range scenario.numVersions {
						appendMetricMetadataBenchmarkRound(b, h, fixture, refs, version%scenario.numVariants, 100+int64(version))
					}

					b.StopTimer()
					afterWAL := metricMetadataBenchmarkWALPosition(b, wal)
					afterHeap := metricMetadataBenchmarkHeapAlloc()
					if afterHeap < beforeHeap {
						b.Fatalf("heap allocation decreased during benchmark: before %d, after %d", beforeHeap, afterHeap)
					}
					totalHeapBytes += afterHeap - beforeHeap
					totalWALBytes += afterWAL - beforeWAL
					validateMetricMetadataBenchmarkState(b, h, mode, fixture, refs, scenario.numVersions)
					runtime.KeepAlive(fixture)
					runtime.KeepAlive(refs)
					runtime.KeepAlive(h)
					closeHead()
				}

				operations := float64(b.N * scenario.numSeries)
				b.ReportMetric(float64(totalHeapBytes)/operations, "heap-B/series")
				b.ReportMetric(float64(totalWALBytes)/operations, "wal-B/series")
			})
		}
	}
}

// BenchmarkHeadMetricMetadataAppend compares steady-state append and commit
// costs after series and metadata have already been established. Use a fixed
// iteration count such as -benchtime=1000x to bound Head and WAL growth.
func BenchmarkHeadMetricMetadataAppend(b *testing.B) {
	benchmarkHeadMetricMetadataAppend(b, true)
}

// BenchmarkHeadMetricMetadataAppendInMemory isolates steady-state Head append
// and commit costs from WAL encoding and I/O.
func BenchmarkHeadMetricMetadataAppendInMemory(b *testing.B) {
	benchmarkHeadMetricMetadataAppend(b, false)
}

func benchmarkHeadMetricMetadataAppend(b *testing.B, withWAL bool) {
	const numSeries = 1_000
	cases := []struct {
		name          string
		numFamilies   int
		setupVersions int
		numVariants   int
	}{
		{name: "stable", numFamilies: 100, setupVersions: 1, numVariants: 1},
		{name: "stable-unique", numFamilies: numSeries, setupVersions: 1, numVariants: 1},
		{name: "changing-at-cap", numFamilies: 100, setupVersions: maxNativeMetricMetadataVersions, numVariants: 2},
	}

	for _, benchmarkCase := range cases {
		for _, mode := range metricMetadataBenchmarkModes() {
			b.Run(fmt.Sprintf("case=%s/mode=%s", benchmarkCase.name, mode.name), func(b *testing.B) {
				fixture := newMetricMetadataBenchmarkFixture(mode, numSeries, benchmarkCase.numFamilies, benchmarkCase.numVariants)
				refs := make([]storage.SeriesRef, numSeries)
				h, wal, closeHead := newMetricMetadataBenchmarkHead(b, mode, 1_000_000_000, withWAL)
				b.Cleanup(closeHead)
				for version := range benchmarkCase.setupVersions {
					appendMetricMetadataBenchmarkRound(b, h, fixture, refs, version%benchmarkCase.numVariants, 100+int64(version))
				}
				var beforeWAL int64
				if wal != nil {
					beforeWAL = metricMetadataBenchmarkWALPosition(b, wal)
				}

				b.ReportAllocs()
				var iteration int64
				for b.Loop() {
					variant := int(iteration) % benchmarkCase.numVariants
					appendMetricMetadataBenchmarkRound(b, h, fixture, refs, variant, 1_000+iteration)
					iteration++
				}

				var afterWAL int64
				if wal != nil {
					afterWAL = metricMetadataBenchmarkWALPosition(b, wal)
				}
				numVersions := benchmarkCase.setupVersions
				if benchmarkCase.numVariants > 1 {
					numVersions += int(iteration)
				}
				validateMetricMetadataBenchmarkState(b, h, mode, fixture, refs, numVersions)
				if wal != nil {
					b.ReportMetric(float64(afterWAL-beforeWAL)/float64(b.N*numSeries), "wal-B/sample")
				}
			})
		}
	}
}

// BenchmarkHeadMetricMetadataChurn measures removal and recreation of series
// carrying unchanged metadata. Use a fixed iteration count such as
// -benchtime=10x to reduce one-shot truncation noise.
func BenchmarkHeadMetricMetadataChurn(b *testing.B) {
	const (
		numSeries   = 10_000
		numFamilies = 100
	)

	for _, mode := range metricMetadataBenchmarkModes() {
		b.Run("mode="+mode.name, func(b *testing.B) {
			fixture := newMetricMetadataBenchmarkFixture(mode, numSeries, numFamilies, 1)
			initialRefs := make([]storage.SeriesRef, numSeries)
			recreatedRefs := make([]storage.SeriesRef, numSeries)
			var totalWALBytes int64

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				b.StopTimer()
				clear(initialRefs)
				clear(recreatedRefs)
				h, wal, closeHead := newMetricMetadataBenchmarkHead(b, mode, 1_000, true)
				appendMetricMetadataBenchmarkRound(b, h, fixture, initialRefs, 0, 100)
				beforeWAL := metricMetadataBenchmarkWALPosition(b, wal)
				b.StartTimer()

				if err := h.truncateMemory(2_000); err != nil {
					b.Fatal(err)
				}
				b.StopTimer()
				if got := h.NumSeries(); got != 0 {
					b.Fatalf("unexpected series count after truncation: got %d, want 0", got)
				}
				if mode.nativeEnabled && h.nativeMetricMetadata.series.Load() != 0 {
					b.Fatalf("native metadata was not removed during truncation: got %d series", h.nativeMetricMetadata.series.Load())
				}
				b.StartTimer()
				appendMetricMetadataBenchmarkRound(b, h, fixture, recreatedRefs, 0, 3_000)

				b.StopTimer()
				afterWAL := metricMetadataBenchmarkWALPosition(b, wal)
				totalWALBytes += afterWAL - beforeWAL
				validateMetricMetadataBenchmarkState(b, h, mode, fixture, recreatedRefs, 1)
				closeHead()
			}

			b.ReportMetric(float64(totalWALBytes)/float64(b.N*numSeries), "wal-B/series")
		})
	}
}
