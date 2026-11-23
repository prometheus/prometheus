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
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/compression"
)

// TestConcurrentNewSeriesCreationRaceOrphansSamples reproduces a WAL ordering race
// between appenders writing samples for the same newly created series.
//
// A newly created series is visible to other appenders before its creator commits.
// Another appender can therefore commit first, and must log the series record
// before its samples. Otherwise WAL replay can discard those samples as unknown
// references, even though the series record appears later in the WAL.
// This test races appenders and checks that replay reports no unknown references.
func TestConcurrentNewSeriesCreationRaceOrphansSamples(t *testing.T) {
	const numSeries = 500
	const racersPerSeries = 4

	db := newTestDB(t)
	db.DisableCompactions()
	ctx := t.Context()

	type racerErr struct {
		appendErr error
		commitErr error
	}
	errs := make([]racerErr, numSeries*racersPerSeries)

	var wg sync.WaitGroup
	for i := range numSeries {
		lbls := labels.FromStrings("name", "race", "idx", strconv.Itoa(i))
		for r := range racersPerSeries {
			wg.Add(1)
			go func(slot int, lbls labels.Labels, t int64, v float64) {
				defer wg.Done()
				app := db.AppenderV2(ctx)
				_, appendErr := app.Append(0, lbls, 0, t, v, nil, nil, storage.AOptions{})
				var commitErr error
				if appendErr == nil {
					commitErr = app.Commit()
				} else {
					commitErr = app.Rollback()
				}
				errs[slot] = racerErr{appendErr: appendErr, commitErr: commitErr}
			}(i*racersPerSeries+r, lbls, int64(100+r), float64(r))
		}
	}
	wg.Wait()
	for _, e := range errs {
		// Racers for the same series use different timestamps, so they also race the
		// ordinary in-order check (OOO is disabled by default). That check compares
		// against the samples already committed to the series: a racer that calls
		// Append() after another racer with a higher timestamp has committed is
		// correctly rejected as out of order. That is expected and not itself a bug;
		// only an unexpected error type fails the test.
		if e.appendErr != nil {
			require.True(t,
				errors.Is(e.appendErr, storage.ErrOutOfOrderSample) || errors.Is(e.appendErr, storage.ErrOutOfBounds),
				"unexpected append error: %v", e.appendErr)
			require.NoError(t, e.commitErr, "rollback after a tolerated append error must not itself fail")
			continue
		}
		require.NoError(t, e.commitErr)
	}

	require.NoError(t, db.Close())

	// Reopen the DB, with a logger we can inspect, to trigger a WAL replay.
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	opts, rngs, err := validateOpts(DefaultOptions(), nil)
	require.NoError(t, err)
	reopened, err := open(db.Dir(), logger, nil, opts, rngs, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })

	unknownRefs := reopened.Head().metrics.walReplayUnknownRefsTotal
	unknownSamples := prom_testutil.ToFloat64(unknownRefs.WithLabelValues("samples"))
	unknownSeries := prom_testutil.ToFloat64(unknownRefs.WithLabelValues("series"))

	// Every series has a WAL series record, so all of them must exist after replay.
	require.Equal(t, uint64(numSeries), reopened.Head().NumSeries())
	// On replay, each sample must follow the WAL series record for its reference.
	// The head contents are not compared: with different timestamps for the same series,
	// the order in which concurrent commits log to the WAL can differ from the order in which
	// they apply to the head, so the replay can accept samples that the head rejected as out
	// of order, and the other way round. See TestNewSeriesRecordPrecedesSamplesInWAL for
	// a comparison with a fixed commit order.
	require.Zero(t, unknownSamples, "the replay found samples logged before their series record")
	require.Zero(t, unknownSeries, "the replay reported series refs as unknown")
	require.NotContains(t, logBuf.String(), "Unknown series references")
}

// TestNewSeriesRecordPrecedesSamplesInWAL is the deterministic counterpart of
// TestConcurrentNewSeriesCreationRaceOrphansSamples. For each series, the appender that
// creates it has the highest timestamp and commits last, while the appenders that merely
// found the series commit before it, in timestamp order. This fixes the commit order within
// each series, so that the only thing that can make the head differ after a restart is a
// sample logged before its series record.
func TestNewSeriesRecordPrecedesSamplesInWAL(t *testing.T) {
	const numSeries = 50
	const appendersPerSeries = 4

	db := newTestDB(t)
	db.DisableCompactions()
	head := db.Head()
	head.initTime(0) // Skip the lazy time initialization that the first appender of a head would do.

	for i := range numSeries {
		lbls := labels.FromStrings("name", "ordered", "idx", strconv.Itoa(i))
		apps := make([]storage.AppenderV2, appendersPerSeries)
		for r := range apps {
			apps[r] = head.AppenderV2(t.Context())
		}
		// The last appender has the highest timestamp and creates the series.
		for _, r := range []int{3, 0, 1, 2} {
			_, err := apps[r].Append(0, lbls, 0, int64(100+r), float64(r), nil, nil, storage.AOptions{})
			require.NoError(t, err)
		}
		for _, r := range []int{0, 1, 2, 3} {
			require.NoError(t, apps[r].Commit())
		}
	}

	matcher := labels.MustNewMatcher(labels.MatchEqual, "name", "ordered")
	beforeQuerier, err := NewBlockQuerier(head, math.MinInt64, math.MaxInt64)
	require.NoError(t, err)
	before := query(t, beforeQuerier, matcher)
	require.Len(t, before, numSeries)
	for lbls, samples := range before {
		require.Len(t, samples, appendersPerSeries, "series %s", lbls)
	}

	require.NoError(t, db.Close())

	// Read the WAL and check that every sample follows its series record.
	sr, err := wlog.NewSegmentsReader(filepath.Join(db.Dir(), "wal"))
	require.NoError(t, err)
	var (
		dec                = record.NewDecoder(labels.NewSymbolTable(), promslog.NewNopLogger())
		recordedSeriesRefs = map[chunks.HeadSeriesRef]struct{}{}
		samples            int
	)
	r := wlog.NewReader(sr)
	for r.Next() {
		rec := r.Record()
		switch dec.Type(rec) {
		case record.Series:
			series, err := dec.Series(rec, nil)
			require.NoError(t, err)
			for _, s := range series {
				_, duplicate := recordedSeriesRefs[s.Ref]
				require.False(t, duplicate, "duplicate series record for ref %d", s.Ref)
				recordedSeriesRefs[s.Ref] = struct{}{}
			}
		case record.Samples:
			recSamples, err := dec.Samples(rec, nil)
			require.NoError(t, err)
			for _, s := range recSamples {
				require.Contains(t, recordedSeriesRefs, s.Ref, "sample at %d precedes its series record", s.T)
				samples++
			}
		}
	}
	require.NoError(t, r.Err())
	require.NoError(t, sr.Close())
	require.Len(t, recordedSeriesRefs, numSeries, "each series must have exactly one series record")
	require.Equal(t, numSeries*appendersPerSeries, samples)

	// Replay the WAL, and query what the head serves after the restart.
	reopened := newTestDB(t, withDir(db.Dir()))
	afterQuerier, err := NewBlockQuerier(reopened.Head(), math.MinInt64, math.MaxInt64)
	require.NoError(t, err)
	after := query(t, afterQuerier, matcher)

	unknownSamples := prom_testutil.ToFloat64(reopened.Head().metrics.walReplayUnknownRefsTotal.WithLabelValues("samples"))
	require.Equal(t, before, after, "the replay must serve exactly what the head served before the restart")
	require.Zero(t, unknownSamples, "the replay must not find samples without a preceding series record")
}

func TestWALSeriesRecordsBeforePayloads(t *testing.T) {
	for _, version := range []int{1, 2} {
		for _, rollback := range []bool{false, true} {
			for _, tc := range []struct {
				name string
				h    *histogram.Histogram
				fh   *histogram.FloatHistogram
				zero bool
				want any
			}{
				{name: "floats", want: []record.RefSample{}},
				{name: "histograms", h: tsdbutil.GenerateTestHistogram(1), want: []record.RefHistogramSample{}},
				{name: "custom_histograms", h: tsdbutil.GenerateTestCustomBucketsHistogram(1), want: []record.RefHistogramSample{}},
				{name: "float_histograms", fh: tsdbutil.GenerateTestFloatHistogram(1), want: []record.RefFloatHistogramSample{}},
				{name: "custom_float_histograms", fh: tsdbutil.GenerateTestCustomBucketsFloatHistogram(1), want: []record.RefFloatHistogramSample{}},
				{name: "float_st_zero", zero: true, want: []record.RefSample{}},
				{name: "histogram_st_zero", h: tsdbutil.GenerateTestHistogram(1), zero: true, want: []record.RefHistogramSample{}},
				{name: "custom_histogram_st_zero", h: tsdbutil.GenerateTestCustomBucketsHistogram(1), zero: true, want: []record.RefHistogramSample{}},
				{name: "float_histogram_st_zero", fh: tsdbutil.GenerateTestFloatHistogram(1), zero: true, want: []record.RefFloatHistogramSample{}},
				{name: "custom_float_histogram_st_zero", fh: tsdbutil.GenerateTestCustomBucketsFloatHistogram(1), zero: true, want: []record.RefFloatHistogramSample{}},
				{name: "exemplars", want: []record.RefExemplar{}},
				{name: "metadata", want: []record.RefMetadata{}},
			} {
				t.Run(fmt.Sprintf("v%d/%s/creator_rollback=%t", version, tc.name, rollback), func(t *testing.T) {
					opts := newTestHeadDefaultOptions(10000, false)
					opts.EnableMetadataWALRecords = true
					opts.EnableSTAsZeroSample = tc.zero
					h, wal := newTestHeadWithOptions(t, compression.None, opts)
					lbls := labels.FromStrings("name", "series_record")
					creator := h.Appender(t.Context())
					ref, err := creator.Append(0, lbls, 200, 2)
					require.NoError(t, err)
					ex := exemplar.Exemplar{Labels: labels.FromStrings("trace_id", "test"), Value: 1, Ts: 100, HasTs: true}
					meta := metadata.Metadata{Help: "A test series."}
					var txn storage.AppenderTransaction
					if version == 1 {
						app := h.Appender(t.Context())
						txn = app
						switch tc.name {
						case "exemplars":
							_, err = app.AppendExemplar(ref, lbls, ex)
						case "metadata":
							_, err = app.UpdateMetadata(ref, lbls, meta)
						default:
							switch {
							case tc.zero && (tc.h != nil || tc.fh != nil):
								_, err = app.AppendHistogramSTZeroSample(ref, lbls, 100, 99, tc.h, tc.fh)
							case tc.zero:
								_, err = app.AppendSTZeroSample(ref, lbls, 100, 99)
							case tc.h != nil || tc.fh != nil:
								_, err = app.AppendHistogram(ref, lbls, 100, tc.h, tc.fh)
							default:
								_, err = app.Append(ref, lbls, 100, 1)
							}
						}
					} else {
						app := h.AppenderV2(t.Context())
						txn = app
						if tc.name == "exemplars" {
							_, err = app.(storage.ExemplarAppenderV2).AppendExemplars(ref, lbls, []exemplar.Exemplar{ex})
						} else {
							var aopts storage.AOptions
							if tc.name == "metadata" {
								aopts.Metadata = meta
							}
							var st int64
							if tc.zero {
								st = 99
							}
							_, err = app.Append(ref, lbls, st, 100, 1, tc.h, tc.fh, aopts)
						}
					}
					require.NoError(t, err)
					require.NoError(t, txn.Commit())
					if rollback {
						require.NoError(t, creator.Rollback())
					} else {
						require.NoError(t, creator.Commit())
					}
					recs := readTestWAL(t, wal.Dir())
					require.GreaterOrEqual(t, len(recs), 2)
					require.IsType(t, []record.RefSeries{}, recs[0])
					require.IsType(t, tc.want, recs[1])
					seriesRecordCount := 0
					for _, rec := range recs {
						if series, ok := rec.([]record.RefSeries); ok {
							seriesRecordCount += len(series)
						}
					}
					require.Equal(t, 1, seriesRecordCount)
				})
			}
		}
	}
}

func TestWALSeriesRecordRetryAndDeduplication(t *testing.T) {
	for _, failWrite := range []bool{false, true} {
		t.Run(fmt.Sprintf("failed_creator=%t", failWrite), func(t *testing.T) {
			h, wal := newTestHead(t, 10000, compression.None, false)
			creator, other := h.Appender(t.Context()), h.Appender(t.Context())
			refs := make([]storage.SeriesRef, 2)
			for i := range refs {
				ref, err := creator.Append(0, labels.FromStrings("series", strconv.Itoa(i)), 200, 2)
				require.NoError(t, err)
				refs[i] = ref
			}
			for _, timestamp := range []int64{100, 101} {
				for _, ref := range refs {
					_, err := other.Append(ref, labels.EmptyLabels(), timestamp, 1)
					require.NoError(t, err)
				}
			}
			if failWrite {
				require.NoError(t, wal.Close())
				require.Error(t, creator.Commit())
				var err error
				h.wal, err = wlog.New(nil, nil, filepath.Join(t.TempDir(), "wal"), compression.None)
				require.NoError(t, err)
			}
			require.NoError(t, other.Commit())
			if !failWrite {
				require.NoError(t, creator.Rollback())
			}
			recs := readTestWAL(t, h.wal.Dir())
			require.Len(t, recs, 2)
			require.IsType(t, []record.RefSeries{}, recs[0])
			require.Len(t, recs[0], 2)
			require.IsType(t, []record.RefSample{}, recs[1])
			require.Len(t, recs[1], 4)
		})
	}
}

func TestWALSeriesRecordDuringSeriesPublication(t *testing.T) {
	h, wal := newTestHead(t, 10000, compression.None, false)
	h.initTime(0)
	lbls := labels.FromStrings("name", "publishing")
	s := newMemSeries(lbls, chunks.HeadSeriesRef(1), 0, h.opts.IsolationDisabled, true)

	// Reproduce the interval in setUnlessAlreadySet after publishing the series
	// by labels, before adding it to the reference index. A different appender
	// can already find it by labels and commit its samples during this interval.
	hash := lbls.Hash()
	stripe := hash & uint64(h.series.size-1)
	h.series.locks[stripe].Lock()
	h.series.hashes[stripe].set(hash, s)
	h.series.locks[stripe].Unlock()
	app := h.Appender(t.Context())
	ref, err := app.Append(0, lbls, 100, 1)
	require.NoError(t, err)
	require.Equal(t, storage.SeriesRef(s.ref), ref)
	require.NoError(t, app.Commit())

	recs := readTestWAL(t, wal.Dir())
	require.Len(t, recs, 2)
	require.Equal(t, []record.RefSeries{{Ref: s.ref, Labels: lbls}}, recs[0])
	require.IsType(t, []record.RefSample{}, recs[1])
}

func TestWALSeriesRecordAfterSeriesRemoval(t *testing.T) {
	for _, removal := range []string{"eviction", "gc"} {
		t.Run(removal, func(t *testing.T) {
			unlinked, resume := make(chan struct{}), make(chan struct{})
			release := sync.OnceFunc(func() { close(resume) })
			defer release()
			var oldRef storage.SeriesRef
			opts := newTestHeadDefaultOptions(10000, false)
			opts.EnableMetadataWALRecords = true
			if removal == "gc" {
				opts.SeriesCallback = seriesEvictionPostDeletionCallback{postDeletion: func(deleted map[chunks.HeadSeriesRef]labels.Labels) {
					if _, ok := deleted[chunks.HeadSeriesRef(oldRef)]; ok {
						close(unlinked)
						<-resume
					}
				}}
			}
			h, wal := newTestHeadWithOptions(t, compression.None, opts)
			h.initTime(0)
			lbls := labels.FromStrings("name", "removed")
			creator, late := h.Appender(t.Context()), h.Appender(t.Context())
			var err error
			oldRef, err = creator.Append(0, lbls, 200, 1)
			require.NoError(t, err)
			// Metadata tracks a WAL series record without reserving the series against eviction.
			_, err = late.UpdateMetadata(oldRef, lbls, metadata.Metadata{Help: "Old series metadata."})
			require.NoError(t, err)

			// Leave the old series without a WAL series record after both write attempts fail.
			require.NoError(t, wal.Close())
			require.Error(t, creator.Commit())
			walDir := filepath.Join(t.TempDir(), "wal")
			h.wal, err = wlog.New(nil, nil, walDir, compression.None)
			require.NoError(t, err)

			var newRef storage.SeriesRef
			replace := func() {
				app := h.Appender(t.Context())
				newRef, err = app.Append(0, lbls, 300, 3)
				require.NoError(t, err)
				require.NotEqual(t, oldRef, newRef)
				require.NoError(t, app.Commit())
				require.NoError(t, late.Commit())
			}
			if removal == "eviction" {
				h.testBeforeDeletingPostings = replace
				deleted, err := h.gcSeries([]storage.SeriesRef{oldRef}, math.MaxInt64, func(*memSeries) bool { return true })
				require.NoError(t, err)
				require.Len(t, deleted, 1)
			} else {
				// Pause the stripe scan after unlinking, before Head.gc can finish its bookkeeping.
				done := make(chan error, 1)
				go func() { done <- h.Truncate(160) }()
				select {
				case <-unlinked:
				case err := <-done:
					t.Fatalf("truncation returned before unlinking: %v", err)
				case <-time.After(10 * time.Second):
					t.Fatal("truncation did not unlink the old series")
				}
				replace()
				release()
				require.NoError(t, <-done)
			}

			matcher := labels.MustNewMatcher(labels.MatchEqual, "name", "removed")
			q, err := NewBlockQuerier(h, math.MinInt64, math.MaxInt64)
			require.NoError(t, err)
			before := query(t, q, matcher)
			require.Len(t, before[lbls.String()], 1)
			require.NoError(t, h.Close())

			for _, rec := range readTestWAL(t, walDir) {
				if series, ok := rec.([]record.RefSeries); ok {
					for _, s := range series {
						require.Equal(t, chunks.HeadSeriesRef(newRef), s.Ref, "a reference removed from the head index must not get another series record")
					}
				}
			}
			replayWAL, err := wlog.New(nil, nil, walDir, compression.None)
			require.NoError(t, err)
			replayOpts := newTestHeadDefaultOptions(10000, false)
			replayOpts.ChunkDirRoot = t.TempDir()
			replayed, err := NewHead(nil, nil, replayWAL, nil, replayOpts, nil)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, replayed.Close()) })
			require.NoError(t, replayed.Init(math.MinInt64))
			q, err = NewBlockQuerier(replayed, math.MinInt64, math.MaxInt64)
			require.NoError(t, err)
			require.Equal(t, before, query(t, q, matcher))
		})
	}
}

// Use production-sized WAL segments to avoid frequent segment rotation.
func newWALSeriesRecordBenchmarkHead(b *testing.B) *Head {
	b.Helper()
	dir := b.TempDir()
	wal, err := wlog.New(nil, nil, filepath.Join(dir, "wal"), compression.None)
	require.NoError(b, err)
	opts := newTestHeadDefaultOptions(1<<40, false)
	opts.ChunkDirRoot = dir
	h, err := NewHead(nil, nil, wal, nil, opts, nil)
	require.NoError(b, err)
	b.Cleanup(func() {
		require.NoError(b, h.Close())
		// Do not retain a closed Head in the benchmark's cleanup slice.
		h = nil
	})
	require.NoError(b, h.Init(0))
	return h
}

// BenchmarkHeadAppender_WALSeriesRecords measures both appender APIs with existing,
// new, large-label, and shared series. Each operation appends to 100 existing series.
// In the shared case, a creator keeps its new series uncommitted while another
// appender appends two rounds of samples to them and commits first. Both commits
// are included in the timing.
func BenchmarkHeadAppender_WALSeriesRecords(b *testing.B) {
	for _, version := range []int{1, 2} {
		b.Run(fmt.Sprintf("appender=v%d", version), func(b *testing.B) {
			for _, tc := range []struct {
				newSeries       int
				extraLabelBytes int
				shared          bool
			}{
				{newSeries: 0},
				{newSeries: 1},
				{newSeries: 10},
				{newSeries: 100},
				{newSeries: 1, extraLabelBytes: 4096},
				{newSeries: 10, shared: true},
			} {
				b.Run(fmt.Sprintf("new=%d/extra_label_bytes=%d/shared=%t", tc.newSeries, tc.extraLabelBytes, tc.shared), func(b *testing.B) {
					h := newWALSeriesRecordBenchmarkHead(b)
					nworkers := runtime.GOMAXPROCS(0)
					refs := make([][]storage.SeriesRef, nworkers)
					for w := range nworkers {
						if version == 1 {
							app := h.Appender(b.Context())
							for i := range 100 {
								ref, err := app.Append(0, labels.FromStrings("worker", strconv.Itoa(w), "series", strconv.Itoa(i)), 0, 0)
								require.NoError(b, err)
								refs[w] = append(refs[w], ref)
							}
							require.NoError(b, app.Commit())
						} else {
							app := h.AppenderV2(b.Context())
							for i := range 100 {
								ref, err := app.Append(0, labels.FromStrings("worker", strconv.Itoa(w), "series", strconv.Itoa(i)), 0, 0, 0, nil, nil, storage.AOptions{})
								require.NoError(b, err)
								refs[w] = append(refs[w], ref)
							}
							require.NoError(b, app.Commit())
						}
					}
					var workerID atomic.Int64
					b.ReportAllocs()
					b.ResetTimer()
					// Testing helpers take a shared lock even on success, so avoid them in parallel workers.
					b.RunParallel(func(pb *testing.PB) {
						worker := int(workerID.Add(1) - 1)
						workerLabel := strconv.Itoa(worker) + strings.Repeat("x", tc.extraLabelBytes)
						var sharedRefs []storage.SeriesRef
						if tc.shared {
							sharedRefs = make([]storage.SeriesRef, tc.newSeries)
						}
						ts := int64(1)
						if version == 1 {
							for pb.Next() {
								app := h.Appender(b.Context())
								for _, ref := range refs[worker] {
									_, err := app.Append(ref, labels.EmptyLabels(), ts, float64(ts))
									if err != nil {
										b.Fatal(err)
									}
								}
								creator, newTimestamp := app, ts
								if tc.shared {
									creator = h.Appender(b.Context())
									newTimestamp += 2
								}
								for n := range tc.newSeries {
									lbls := labels.FromStrings("worker", workerLabel, "new", strconv.FormatInt(ts*int64(tc.newSeries)+int64(n), 10))
									ref, err := creator.Append(0, lbls, newTimestamp, float64(newTimestamp))
									if err != nil {
										b.Fatal(err)
									}
									if tc.shared {
										sharedRefs[n] = ref
									}
								}
								if tc.shared {
									// Two rounds produce nonconsecutive duplicate refs for WAL record deduplication.
									for round := range int64(2) {
										for _, ref := range sharedRefs {
											_, err := app.Append(ref, labels.EmptyLabels(), ts+round, float64(ts+round))
											if err != nil {
												b.Fatal(err)
											}
										}
									}
								}
								if err := app.Commit(); err != nil {
									b.Fatal(err)
								}
								if tc.shared {
									if err := creator.Commit(); err != nil {
										b.Fatal(err)
									}
								}
								ts++
							}
							return
						}
						for pb.Next() {
							app := h.AppenderV2(b.Context())
							for _, ref := range refs[worker] {
								_, err := app.Append(ref, labels.EmptyLabels(), 0, ts, float64(ts), nil, nil, storage.AOptions{})
								if err != nil {
									b.Fatal(err)
								}
							}
							creator, newTimestamp := app, ts
							if tc.shared {
								creator = h.AppenderV2(b.Context())
								newTimestamp += 2
							}
							for n := range tc.newSeries {
								lbls := labels.FromStrings("worker", workerLabel, "new", strconv.FormatInt(ts*int64(tc.newSeries)+int64(n), 10))
								ref, err := creator.Append(0, lbls, 0, newTimestamp, float64(newTimestamp), nil, nil, storage.AOptions{})
								if err != nil {
									b.Fatal(err)
								}
								if tc.shared {
									sharedRefs[n] = ref
								}
							}
							if tc.shared {
								// Two rounds produce nonconsecutive duplicate refs for WAL record deduplication.
								for round := range int64(2) {
									for _, ref := range sharedRefs {
										_, err := app.Append(ref, labels.EmptyLabels(), 0, ts+round, float64(ts+round), nil, nil, storage.AOptions{})
										if err != nil {
											b.Fatal(err)
										}
									}
								}
							}
							if err := app.Commit(); err != nil {
								b.Fatal(err)
							}
							if tc.shared {
								if err := creator.Commit(); err != nil {
									b.Fatal(err)
								}
							}
							ts++
						}
					})
					b.StopTimer()
					require.Equal(b, uint64(100*nworkers+tc.newSeries*b.N), h.NumSeries())
				})
			}
		})
	}
}

// BenchmarkHeadAppender_ConcurrentNewSeries measures goroutines appending and
// committing to the same fresh series through both appender APIs. Every writer
// buffers its samples before any writer commits. Identical timestamps and values
// make every commit order valid with out-of-order ingestion disabled. One operation
// is a round of concurrent appends and commits, including coordination and labels.
func BenchmarkHeadAppender_ConcurrentNewSeries(b *testing.B) {
	type appendRound struct {
		labels              []labels.Labels
		timestamp           int64
		start, commit       chan struct{}
		appended, committed sync.WaitGroup
		refs                [][]storage.SeriesRef
		errors              []error
	}

	for _, version := range []int{1, 2} {
		for _, writers := range []int{2, 4, 8} {
			for _, series := range []int{1, 10, 100} {
				b.Run(fmt.Sprintf("appender=v%d/writers=%d/series=%d", version, writers, series), func(b *testing.B) {
					h := newWALSeriesRecordBenchmarkHead(b)
					ctx := b.Context()
					jobs := make(chan *appendRound)
					var workers sync.WaitGroup
					workers.Add(writers)
					for worker := range writers {
						go func() {
							defer workers.Done()
							if version == 1 {
								for round := range jobs {
									<-round.start
									app := h.Appender(ctx)
									round.errors[worker] = nil
									for i, lset := range round.labels {
										ref, err := app.Append(0, lset, round.timestamp, 1)
										if err != nil {
											round.errors[worker] = err
											break
										}
										round.refs[worker][i] = ref
									}
									round.appended.Done()
									<-round.commit
									if round.errors[worker] == nil {
										round.errors[worker] = app.Commit()
									} else {
										round.errors[worker] = errors.Join(round.errors[worker], app.Rollback())
									}
									round.committed.Done()
								}
								return
							}
							for round := range jobs {
								<-round.start
								app := h.AppenderV2(ctx)
								round.errors[worker] = nil
								for i, lset := range round.labels {
									ref, err := app.Append(0, lset, 0, round.timestamp, 1, nil, nil, storage.AOptions{})
									if err != nil {
										round.errors[worker] = err
										break
									}
									round.refs[worker][i] = ref
								}
								round.appended.Done()
								<-round.commit
								if round.errors[worker] == nil {
									round.errors[worker] = app.Commit()
								} else {
									round.errors[worker] = errors.Join(round.errors[worker], app.Rollback())
								}
								round.committed.Done()
							}
						}()
					}
					b.Cleanup(func() {
						close(jobs)
						workers.Wait()
					})

					round := appendRound{
						labels: make([]labels.Labels, series),
						refs:   make([][]storage.SeriesRef, writers),
						errors: make([]error, writers),
					}
					for worker := range writers {
						round.refs[worker] = make([]storage.SeriesRef, series)
					}
					seriesLabels := make([]string, series)
					for i := range series {
						seriesLabels[i] = strconv.Itoa(i)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for iteration := range b.N {
						iterationLabel := strconv.Itoa(iteration)
						for i := range series {
							round.labels[i] = labels.FromStrings("round", iterationLabel, "series", seriesLabels[i])
						}
						round.timestamp = int64(iteration + 1)
						round.start, round.commit = make(chan struct{}), make(chan struct{})
						round.appended.Add(writers)
						round.committed.Add(writers)
						for range writers {
							jobs <- &round
						}
						close(round.start)
						round.appended.Wait()
						close(round.commit)
						round.committed.Wait()
						for _, err := range round.errors {
							if err != nil {
								b.Fatal(err)
							}
						}
					}
					b.StopTimer()
					require.Equal(b, uint64(b.N*series), h.NumSeries())
					for _, refs := range round.refs[1:] {
						require.Equal(b, round.refs[0], refs, "writers must append to the same series")
					}
					b.ReportMetric(float64(writers*series), "sample_appends/op")
				})
			}
		}
	}
}
