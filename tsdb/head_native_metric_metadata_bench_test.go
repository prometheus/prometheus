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
	"math"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/prometheus/common/model"
	"go.uber.org/atomic"

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

// metricMetadataBenchmarkModes returns the metadata storage configurations
// under test: "off" stores none, "legacy" writes metadata WAL records,
// and "native" stores versioned metadata, persisting it when a WAL is configured.
//
// All modes receive fixture metadata, matching callers that supply it
// independently of the enabled storage features.
func metricMetadataBenchmarkModes() []metricMetadataBenchmarkMode {
	return []metricMetadataBenchmarkMode{
		{name: "off"},
		{name: "legacy", legacyEnabled: true},
		{name: "native", nativeEnabled: true},
	}
}

type metricMetadataBenchmarkFixture struct {
	labels         []labels.Labels
	familyBySeries []int
	options        [][]storage.AOptions
}

// newMetricMetadataBenchmarkFixture assigns series to families round-robin, as
// a Remote Write request may interleave them.
func newMetricMetadataBenchmarkFixture(numSeries, numFamilies, numVariants int) *metricMetadataBenchmarkFixture {
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
	// Supply metadata and its family name in every mode, independently of
	// whether native storage or legacy metadata WAL recording is enabled.
	for variant := range numVariants {
		fixture.options[variant] = make([]storage.AOptions, numFamilies)
		for family := range numFamilies {
			meta := metadata.Metadata{
				Type: model.MetricTypeCounter,
				Unit: "requests",
				Help: fmt.Sprintf("Total requests processed by benchmark family %03d, metadata version %02d.", family, variant),
			}
			fixture.options[variant][family] = storage.AOptions{
				MetricFamilyName: metricNames[family],
				Metadata:         meta,
			}
		}
	}
	return fixture
}

func newMetricMetadataBenchmarkHead(b *testing.B, mode metricMetadataBenchmarkMode, withWAL bool) (*Head, *wlog.WL, func()) {
	dir := b.TempDir()
	var wal *wlog.WL
	if withWAL {
		var err error
		wal, err = wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), wlog.DefaultSegmentSize, compression.Snappy)
		if err != nil {
			b.Fatal(err)
		}
	}

	opts := newTestHeadDefaultOptions(1_000_000_000, false)
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

// metricMetadataBenchmarkHeap counts live heap bytes and objects.
type metricMetadataBenchmarkHeap struct{ bytes, objects uint64 }

// readMetricMetadataBenchmarkHeap samples the live heap after garbage collection.
func readMetricMetadataBenchmarkHeap() metricMetadataBenchmarkHeap {
	// The second collection clears sync.Pool victim caches left by the commit.
	runtime.GC()
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return metricMetadataBenchmarkHeap{bytes: stats.HeapAlloc, objects: stats.HeapObjects}
}

// addGrowth adds the live heap growth since before, failing if the heap shrank.
func (h *metricMetadataBenchmarkHeap) addGrowth(b *testing.B, before metricMetadataBenchmarkHeap) {
	after := readMetricMetadataBenchmarkHeap()
	if after.bytes < before.bytes || after.objects < before.objects {
		b.Fatalf("retained heap decreased: %+v -> %+v", before, after)
	}
	h.bytes += after.bytes - before.bytes
	h.objects += after.objects - before.objects
}

func (h metricMetadataBenchmarkHeap) report(b *testing.B, series int) {
	b.ReportMetric(float64(h.bytes)/float64(series), "heap-B/series")
	b.ReportMetric(float64(h.objects)/float64(series), "heap-objects/series")
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

	if mode.nativeEnabled {
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
			versions, truncated, ok := h.nativeMetricMetadataForTest(chunks.HeadSeriesRef(refs[i]))
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
	}
	if mode.legacyEnabled {
		lastVariant := (numVersions - 1) % len(fixture.options)
		for _, i := range []int{0, len(refs) - 1} {
			series := h.series.getByID(chunks.HeadSeriesRef(refs[i]))
			if series == nil {
				b.Fatalf("series %d was not found", refs[i])
			}
			got := legacyMetadataForTest(series)
			want := fixture.options[lastVariant][fixture.familyBySeries[i]].Metadata
			if got == nil || !got.Equals(want) {
				b.Fatalf("unexpected legacy metadata for series %d: got %v, want %v", refs[i], got, want)
			}
		}
	}
	if !mode.nativeEnabled && !mode.legacyEnabled {
		if h.nativeMetricMetadata != nil {
			b.Fatal("mode=off unexpectedly created a native metadata store")
		}
		for _, i := range []int{0, len(refs) - 1} {
			series := h.series.getByID(chunks.HeadSeriesRef(refs[i]))
			if series == nil {
				b.Fatalf("series %d was not found", refs[i])
			}
			got := legacyMetadataForTest(series)
			if got != nil {
				b.Fatalf("mode=off unexpectedly retained metadata for series %d", refs[i])
			}
		}
	}
}

// BenchmarkHeadMetricMetadataRetainedHeap compares initial ingestion and
// retained per-series heap. Run each case six times in fresh processes with
// -benchtime=1x -cpu=2 so global interning state cannot cross cases.
func BenchmarkHeadMetricMetadataRetainedHeap(b *testing.B) {
	scenarios := []struct {
		name        string
		numSeries   int
		numFamilies int
		numVersions int
		numVariants int
	}{
		{name: "stable", numSeries: 100_000, numFamilies: 100, numVersions: 1, numVariants: 1},
		{name: "stable-unique", numSeries: 25_000, numFamilies: 25_000, numVersions: 1, numVariants: 1},
		{name: "observed-versions=5", numSeries: 25_000, numFamilies: 100, numVersions: 5, numVariants: 5},
	}

	for _, scenario := range scenarios {
		for _, mode := range metricMetadataBenchmarkModes() {
			b.Run(fmt.Sprintf("scenario=%s/mode=%s", scenario.name, mode.name), func(b *testing.B) {
				fixture := newMetricMetadataBenchmarkFixture(scenario.numSeries, scenario.numFamilies, scenario.numVariants)
				refs := make([]storage.SeriesRef, scenario.numSeries)
				var heap metricMetadataBenchmarkHeap
				var totalWALBytes int64

				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					b.StopTimer()
					clear(refs)
					before := readMetricMetadataBenchmarkHeap()
					h, wal, closeHead := newMetricMetadataBenchmarkHead(b, mode, true)
					beforeWAL := metricMetadataBenchmarkWALPosition(b, wal)
					b.StartTimer()

					for version := range scenario.numVersions {
						appendMetricMetadataBenchmarkRound(b, h, fixture, refs, version%scenario.numVariants, 100+int64(version))
					}

					b.StopTimer()
					afterWAL := metricMetadataBenchmarkWALPosition(b, wal)
					heap.addGrowth(b, before)
					totalWALBytes += afterWAL - beforeWAL
					validateMetricMetadataBenchmarkState(b, h, mode, fixture, refs, scenario.numVersions)
					runtime.KeepAlive(fixture)
					runtime.KeepAlive(refs)
					runtime.KeepAlive(h)
					closeHead()
				}

				heap.report(b, b.N*scenario.numSeries)
				b.ReportMetric(float64(totalWALBytes)/float64(b.N*scenario.numSeries), "wal-B/series")
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
	// Once seeded, the stable cases skip recording unchanged native metadata.
	cases := []struct {
		name          string
		numFamilies   int
		setupVersions int
		numVariants   int
		freshStrings  bool
	}{
		{name: "stable", numFamilies: 100, setupVersions: 1, numVariants: 1},
		{name: "stable-fresh-strings", numFamilies: 100, setupVersions: 1, numVariants: 1, freshStrings: true},
		{name: "stable-unique", numFamilies: numSeries, setupVersions: 1, numVariants: 1},
		{name: "changing-at-cap", numFamilies: 100, setupVersions: maxNativeMetricMetadataVersions, numVariants: 2},
	}
	for _, benchmarkCase := range cases {
		for _, mode := range metricMetadataBenchmarkModes() {
			b.Run(fmt.Sprintf("case=%s/mode=%s", benchmarkCase.name, mode.name), func(b *testing.B) {
				fixture := newMetricMetadataBenchmarkFixture(numSeries, benchmarkCase.numFamilies, benchmarkCase.numVariants)
				refs := make([]storage.SeriesRef, numSeries)
				h, wal, closeHead := newMetricMetadataBenchmarkHead(b, mode, withWAL)
				b.Cleanup(closeHead)
				for version := range benchmarkCase.setupVersions {
					appendMetricMetadataBenchmarkRound(b, h, fixture, refs, version%benchmarkCase.numVariants, 100+int64(version))
				}
				if benchmarkCase.freshStrings {
					for _, options := range fixture.options {
						for i := range options {
							m := &options[i].Metadata
							m.Type = model.MetricType(strings.Clone(string(m.Type)))
							m.Unit = strings.Clone(m.Unit)
							m.Help = strings.Clone(m.Help)
						}
					}
				}
				var beforeWAL int64
				if wal != nil {
					beforeWAL = metricMetadataBenchmarkWALPosition(b, wal)
				}

				b.ReportAllocs()
				var iteration int64
				for b.Loop() {
					// Continue the setup's variant sequence so changing-metadata cases
					// change on their first timed append too. Otherwise an unchanged
					// first append would invalidate their expected version count.
					variant := (benchmarkCase.setupVersions + int(iteration)) % benchmarkCase.numVariants
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
				validateMetricMetadataBenchmarkSamples(b, h, int64(numSeries)*(int64(benchmarkCase.setupVersions)+iteration))
				if wal != nil {
					b.ReportMetric(float64(afterWAL-beforeWAL)/float64(b.N*numSeries), "wal-B/sample")
				}
			})
		}
	}
}

// BenchmarkNativeMetricMetadataPendingHeap isolates the transient transaction
// state retained before commit. Run with -benchtime=1x in fresh processes.
func BenchmarkNativeMetricMetadataPendingHeap(b *testing.B) {
	const sharedFamilies = 100
	sharedMetadata := make([]metadata.Metadata, sharedFamilies)
	for i := range sharedMetadata {
		sharedMetadata[i] = metadata.Metadata{Type: model.MetricTypeGauge, Help: fmt.Sprintf("shared metadata %d", i)}
	}
	uniqueMetadata := make([]metadata.Metadata, 25_000)
	for i := range uniqueMetadata {
		uniqueMetadata[i] = metadata.Metadata{Type: model.MetricTypeGauge, Help: strconv.Itoa(i)}
	}
	// Remote Write 2.0 entries can reference one help symbol with distinct units.
	sharedHelp := strings.Repeat("h", 8_000)
	sharedHelpMetadata := make([]metadata.Metadata, 10_000)
	for i := range sharedHelpMetadata {
		sharedHelpMetadata[i] = metadata.Metadata{Type: model.MetricTypeGauge, Unit: "u" + strconv.Itoa(i), Help: sharedHelp}
	}
	uniqueLongMetadata := make([]metadata.Metadata, 25_000)
	for i := range uniqueLongMetadata {
		uniqueLongMetadata[i] = metadata.Metadata{Type: model.MetricTypeGauge, Help: fmt.Sprintf("%072d", i)}
	}

	identity := func(i int) int { return i }
	for _, benchmarkCase := range []struct {
		name         string
		numSeries    int
		observations int
		metadata     func(int) metadata.Metadata
		series       func(int) int
	}{
		{name: "shared-grouped", numSeries: 100_000, observations: 100_000, metadata: func(i int) metadata.Metadata { return sharedMetadata[i/(100_000/sharedFamilies)] }, series: identity},
		{name: "shared-round-robin", numSeries: 100_000, observations: 100_000, metadata: func(i int) metadata.Metadata { return sharedMetadata[i%sharedFamilies] }, series: identity},
		{name: "single-series", numSeries: 1, observations: 100_000, metadata: func(int) metadata.Metadata { return sharedMetadata[0] }, series: func(int) int { return 0 }},
		{name: "unique", numSeries: 25_000, observations: 25_000, metadata: func(i int) metadata.Metadata { return uniqueMetadata[i] }, series: identity},
		{name: "unique-long", numSeries: 25_000, observations: 25_000, metadata: func(i int) metadata.Metadata { return uniqueLongMetadata[i] }, series: identity},
		{name: "shared-help-distinct-units", numSeries: 10_000, observations: 10_000, metadata: func(i int) metadata.Metadata { return sharedHelpMetadata[i] }, series: identity},
	} {
		b.Run(benchmarkCase.name, func(b *testing.B) {
			series := make([]memSeries, benchmarkCase.numSeries)
			for i := range series {
				series[i].ref = chunks.HeadSeriesRef(i + 1)
			}
			var totalHeap uint64
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				b.StopTimer()
				store := newNativeMetricMetadataStore()
				app := headAppenderBase{head: &Head{nativeMetricMetadata: store}}
				runtime.GC()
				runtime.GC()
				var before runtime.MemStats
				runtime.ReadMemStats(&before)
				b.StartTimer()

				for i := range benchmarkCase.observations {
					s := &series[benchmarkCase.series(i)]
					timestamp := int64(i)
					m := benchmarkCase.metadata(i)
					if app.head.nativeMetricMetadata == nil || m.IsEmpty() {
						continue
					}
					if m.Type == "" {
						m.Type = model.MetricTypeUnknown
					}
					s.Lock()
					observe := app.shouldObserveNativeMetricMetadataLocked(s, timestamp, &m)
					s.Unlock()
					if observe {
						app.recordNativeMetricMetadata(s, timestamp, m)
					}
				}

				b.StopTimer()
				var after runtime.MemStats
				runtime.ReadMemStats(&after)
				if after.HeapAlloc < before.HeapAlloc {
					b.Fatalf("heap allocation decreased during benchmark: before %d, after %d", before.HeapAlloc, after.HeapAlloc)
				}
				totalHeap += after.HeapAlloc - before.HeapAlloc
				app.clearNativeMetricMetadata()
			}
			b.ReportMetric(float64(totalHeap)/float64(b.N*benchmarkCase.observations), "pending-heap-B/observation")
		})
	}
}

func validateMetricMetadataBenchmarkSamples(b *testing.B, h *Head, want int64) {
	b.Helper()
	q, err := NewBlockChunkQuerier(h, math.MinInt64, math.MaxInt64)
	if err != nil {
		b.Fatal(err)
	}
	defer q.Close()
	series := q.Select(b.Context(), false, nil, labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, ".+"))
	var count int64
	var iter chunks.Iterator
	for series.Next() {
		iter = series.At().Iterator(iter)
		for iter.Next() {
			count += int64(iter.At().Chunk.NumSamples())
		}
		if err := iter.Err(); err != nil {
			b.Fatal(err)
		}
	}
	if err := series.Err(); err != nil {
		b.Fatal(err)
	}
	if count != want {
		b.Fatalf("stored %d samples, want %d", count, want)
	}
}

// BenchmarkHeadMetricMetadataAppendFixedConcurrency keeps the series, worker
// count, batch size, and total work fixed while GOMAXPROCS changes.
// CPU counts that do not divide eight are skipped.
func BenchmarkHeadMetricMetadataAppendFixedConcurrency(b *testing.B) {
	const workers, perWorker, families = 8, 1000, 100
	for _, changedPercent := range []int{0, 1, 100} {
		for _, mode := range []metricMetadataBenchmarkMode{{name: "off"}, {name: "native", nativeEnabled: true}} {
			b.Run(fmt.Sprintf("change=%d/mode=%s", changedPercent, mode.name), func(b *testing.B) {
				procs := runtime.GOMAXPROCS(0)
				if procs > workers || workers%procs != 0 {
					b.Skipf("GOMAXPROCS must divide %d, got %d", workers, procs)
				}
				h, _, closeHead := newMetricMetadataBenchmarkHead(b, mode, false)
				b.Cleanup(closeHead)
				fixture := newMetricMetadataBenchmarkFixture(workers*perWorker, families, maxNativeMetricMetadataVersions+2)
				refs := make([]storage.SeriesRef, workers*perWorker)
				for version := range maxNativeMetricMetadataVersions {
					appendMetricMetadataBenchmarkRound(b, h, fixture, refs, version, 100+int64(version))
				}
				var changed [workers * perWorker]bool
				for worker := range workers {
					for i := range perWorker {
						// Ten series per worker, spread across 80 distinct families.
						changed[worker*perWorker+i] = changedPercent == 100 || changedPercent == 1 && i%100 == (i/100+worker*10)%100
					}
				}
				var nextWorker atomic.Uint64
				var completed [workers]int64
				b.ReportAllocs()
				b.SetParallelism(workers / procs)
				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					worker := int(nextWorker.Add(1) - 1)
					if worker >= workers {
						b.Fatal("too many workers")
					}
					var rounds int64
					for pb.Next() {
						app := h.AppenderV2(b.Context())
						for i := worker * perWorker; i < (worker+1)*perWorker; i++ {
							variant := maxNativeMetricMetadataVersions - 1
							if changed[i] {
								variant = maxNativeMetricMetadataVersions + int(rounds%2)
							}
							ref, err := app.Append(refs[i], fixture.labels[i], 0, 1000+rounds, float64(rounds), nil, nil, fixture.options[variant][fixture.familyBySeries[i]])
							if err != nil {
								b.Fatal(err)
							}
							refs[i] = ref
						}
						if err := app.Commit(); err != nil {
							b.Fatal(err)
						}
						rounds++
					}
					completed[worker] = rounds
				})
				b.StopTimer()
				b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*perWorker), "ns/sample")
				var rounds, evictions int64
				for _, n := range completed {
					rounds += n
				}
				if rounds != int64(b.N) || nextWorker.Load() != workers {
					b.Fatalf("unexpected completed work: %d rounds, %d workers", rounds, nextWorker.Load())
				}
				validateMetricMetadataBenchmarkSamples(b, h, workers*perWorker*maxNativeMetricMetadataVersions+rounds*perWorker)
				if !mode.nativeEnabled {
					validateMetricMetadataBenchmarkState(b, h, mode, fixture, refs, maxNativeMetricMetadataVersions)
					return
				}
				for i, ref := range refs {
					n := completed[i/perWorker]
					variant := maxNativeMetricMetadataVersions - 1
					updated := changed[i] && n > 0
					if updated {
						variant = maxNativeMetricMetadataVersions + int((n-1)%2)
						evictions += n
					}
					versions, truncated, ok := h.nativeMetricMetadataForTest(chunks.HeadSeriesRef(ref))
					if !ok || len(versions) != maxNativeMetricMetadataVersions || truncated != updated || !versions[len(versions)-1].Metadata.Equals(fixture.options[variant][fixture.familyBySeries[i]].Metadata) {
						b.Fatalf("unexpected metadata for series %d after %d rounds", i, n)
					}
				}
				if h.NumSeries() != workers*perWorker || h.nativeMetricMetadata.series.Load() != workers*perWorker || h.nativeMetricMetadata.versions.Load() != workers*perWorker*maxNativeMetricMetadataVersions || h.nativeMetricMetadata.evictions.Load() != uint64(evictions) {
					b.Fatal("unexpected series, version, or eviction counters")
				}
			})
		}
	}
}
