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
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
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

func newWALSeriesRecordTestAppender(t *testing.T, h *Head, version int) (storage.AppenderTransaction, func(storage.SeriesRef, labels.Labels, int64, float64) (storage.SeriesRef, error)) {
	t.Helper()
	if version == 1 {
		app := h.Appender(t.Context())
		return app, app.Append
	}
	app := h.AppenderV2(t.Context())
	return app, func(ref storage.SeriesRef, lset labels.Labels, ts int64, value float64) (storage.SeriesRef, error) {
		return app.Append(ref, lset, 0, ts, value, nil, nil, storage.AOptions{})
	}
}

// TestNewSeriesRecordPrecedesSamplesInWAL verifies replay when a borrower commits
// before its creator or after the creator rolls back. Fixed timestamps and commit
// order avoid unrelated out-of-order differences between Head and replay.
func TestNewSeriesRecordPrecedesSamplesInWAL(t *testing.T) {
	for _, version := range []int{1, 2} {
		for _, rollback := range []bool{false, true} {
			t.Run(fmt.Sprintf("v%d/creator_rollback=%t", version, rollback), func(t *testing.T) {
				db := newTestDB(t)
				db.DisableCompactions()
				db.Head().initTime(0)
				creator, create := newWALSeriesRecordTestAppender(t, db.Head(), version)
				borrower, appendSample := newWALSeriesRecordTestAppender(t, db.Head(), version)
				for i := range 2 {
					ref, err := create(0, labels.FromStrings("series", strconv.Itoa(i)), 200, float64(i+2))
					require.NoError(t, err)
					_, err = appendSample(ref, labels.EmptyLabels(), 100, float64(i+1))
					require.NoError(t, err)
				}
				if rollback {
					require.NoError(t, creator.Rollback())
				}
				require.NoError(t, borrower.Commit())
				wantSamples := 1
				if !rollback {
					require.NoError(t, creator.Commit())
					wantSamples++
				}
				matcher := labels.MustNewMatcher(labels.MatchRegexp, "series", ".+")
				q, err := NewBlockQuerier(db.Head(), math.MinInt64, math.MaxInt64)
				require.NoError(t, err)
				before := query(t, q, matcher)
				require.Len(t, before, 2)
				for _, samples := range before {
					require.Len(t, samples, wantSamples)
				}
				require.NoError(t, db.Close())
				replayed := newTestDB(t, withDir(db.Dir()))
				q, err = NewBlockQuerier(replayed.Head(), math.MinInt64, math.MaxInt64)
				require.NoError(t, err)
				require.Equal(t, before, query(t, q, matcher))
			})
		}
	}
}

func TestRollbackDoesNotWriteWAL(t *testing.T) {
	for name, tc := range map[string]struct {
		version      int
		withMetadata bool
	}{
		"v1": {version: 1},
		"v2": {version: 2},
		// Rollback must also release the reservation taken for AppenderV2 metadata.
		"v2_with_metadata": {version: 2, withMetadata: true},
	} {
		for _, compress := range []compression.Type{compression.None, compression.Snappy, compression.Zstd} {
			t.Run(fmt.Sprintf("%s/compress=%s", name, compress), func(t *testing.T) {
				opts := newTestHeadDefaultOptions(1000, false)
				opts.EnableMetadataWALRecords = tc.withMetadata
				h, wal := newTestHeadWithOptions(t, compress, opts)
				app, appendSample := newWALSeriesRecordTestAppender(t, h, tc.version)
				var err error
				if tc.withMetadata {
					_, err = app.(storage.AppenderV2).Append(0, labels.FromStrings("a", "b"), 0, 1, 2, nil, nil, storage.AOptions{
						Metadata: metadata.Metadata{Help: "Test metadata."},
					})
				} else {
					_, err = appendSample(0, labels.FromStrings("a", "b"), 1, 2)
				}
				require.NoError(t, err)
				require.NoError(t, app.Rollback())
				require.Empty(t, readTestWAL(t, wal.Dir()))
				h.gc()
				require.Zero(t, h.NumSeries(), "rollback must release all reservations")
				require.Zero(t, prom_testutil.ToFloat64(h.metrics.pendingCommitUnderflow))
			})
		}
	}
}

func TestCommitLogsOnlyReferencedSeries(t *testing.T) {
	for _, version := range []int{1, 2} {
		for name, tc := range map[string]struct {
			appendValidSample bool
			appendExemplar    bool
		}{
			"rejected_only": {},
			"mixed":         {appendValidSample: true},
			// An accepted exemplar requires the series record even when its sample was rejected.
			"rejected_with_exemplar": {appendExemplar: true},
		} {
			t.Run(fmt.Sprintf("v%d/%s", version, name), func(t *testing.T) {
				opts := newTestHeadDefaultOptions(1000, false)
				opts.OutOfOrderTimeWindow.Store(100)
				h, wal := newTestHeadWithOptions(t, compression.None, opts)
				appender, appendSample := newWALSeriesRecordTestAppender(t, h, version)
				_, err := appendSample(0, labels.FromStrings("series", "existing"), 1000, 1)
				require.NoError(t, err)
				require.NoError(t, appender.Commit())
				before := len(readTestWAL(t, wal.Dir()))
				require.Equal(t, 2, before)

				appender, appendSample = newWALSeriesRecordTestAppender(t, h, version)
				rejectedLabels := labels.FromStrings("series", "rejected")
				_, err = appendSample(0, rejectedLabels, 1, 1)
				require.ErrorIs(t, err, storage.ErrTooOldSample)
				require.Equal(t, uint64(2), h.NumSeries(), "the rejected sample must have created an empty series")
				var ref storage.SeriesRef
				recordLabels := labels.FromStrings("series", "valid")
				if tc.appendValidSample {
					ref, err = appendSample(0, recordLabels, 1000, 1)
					require.NoError(t, err)
				}
				ex := exemplar.Exemplar{Labels: labels.FromStrings("trace_id", "test"), Value: 1, Ts: 1000, HasTs: true}
				if tc.appendExemplar {
					recordLabels = rejectedLabels
					if version == 1 {
						ref, err = appender.(storage.ExemplarAppender).AppendExemplar(0, rejectedLabels, ex)
					} else {
						ref, err = appender.(storage.ExemplarAppenderV2).AppendExemplars(0, rejectedLabels, []exemplar.Exemplar{ex})
					}
					require.NoError(t, err)
				}
				require.NoError(t, appender.Commit())
				recs := readTestWAL(t, wal.Dir())[before:]
				wantSeries := uint64(1)
				if tc.appendValidSample || tc.appendExemplar {
					require.Len(t, recs, 2)
					require.IsType(t, []record.RefSeries{}, recs[0])
					series := recs[0].([]record.RefSeries)
					require.Len(t, series, 1)
					require.Equal(t, chunks.HeadSeriesRef(ref), series[0].Ref)
					require.True(t, labels.Equal(recordLabels, series[0].Labels))
					switch rec := recs[1].(type) {
					case []record.RefSample:
						require.True(t, tc.appendValidSample, "unexpected sample record")
						require.Equal(t, []record.RefSample{{Ref: chunks.HeadSeriesRef(ref), T: 1000, V: 1}}, rec)
					case []record.RefExemplar:
						require.True(t, tc.appendExemplar, "unexpected exemplar record")
						require.Len(t, rec, 1)
						require.Equal(t, chunks.HeadSeriesRef(ref), rec[0].Ref)
						require.Equal(t, ex.Ts, rec[0].T)
						require.Equal(t, ex.Value, rec[0].V)
						require.True(t, labels.Equal(ex.Labels, rec[0].Labels))
					default:
						t.Fatalf("unexpected WAL record type: %T", rec)
					}
					wantSeries++
				} else {
					require.Empty(t, recs)
				}
				h.gc()
				require.Equal(t, wantSeries, h.NumSeries(), "GC must remove series without samples or live exemplars")
				if tc.appendExemplar {
					// Expire the exemplar to verify that its reservation was released too.
					require.NoError(t, h.Truncate(1001))
					require.Zero(t, h.NumSeries())
				}
				require.Zero(t, prom_testutil.ToFloat64(h.metrics.pendingCommitUnderflow))
			})
		}
	}
}

func TestWALSeriesRecordsBeforePayloads(t *testing.T) {
	for _, version := range []int{1, 2} {
		for _, rollback := range []bool{false, true} {
			for _, tc := range []struct {
				name            string
				h               *histogram.Histogram
				fh              *histogram.FloatHistogram
				zero            bool
				extraLabelBytes int
				want            any
			}{
				{name: "floats", want: []record.RefSample{}},
				{name: "large_series_record", extraLabelBytes: 4096, want: []record.RefSample{}},
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
					if tc.extraLabelBytes > 0 {
						lbls = labels.FromStrings("name", strings.Repeat("x", tc.extraLabelBytes))
					}
					creator, create := newWALSeriesRecordTestAppender(t, h, version)
					ref, err := create(0, lbls, 200, 2)
					require.NoError(t, err)
					if rollback {
						require.NoError(t, creator.Rollback())
						require.Empty(t, readTestWAL(t, wal.Dir()))
					}
					ex := exemplar.Exemplar{Labels: labels.FromStrings("trace_id", "test"), Value: 1, Ts: 100, HasTs: true}
					meta := metadata.Metadata{Help: "A test series."}
					var txn storage.AppenderTransaction
					if version == 1 {
						app := h.Appender(t.Context())
						txn = app
						switch {
						case tc.name == "exemplars":
							_, err = app.AppendExemplar(ref, lbls, ex)
						case tc.name == "metadata":
							_, err = app.UpdateMetadata(ref, lbls, meta)
						case tc.zero && (tc.h != nil || tc.fh != nil):
							_, err = app.AppendHistogramSTZeroSample(ref, lbls, 100, 99, tc.h, tc.fh)
						case tc.zero:
							_, err = app.AppendSTZeroSample(ref, lbls, 100, 99)
						case tc.h != nil || tc.fh != nil:
							_, err = app.AppendHistogram(ref, lbls, 100, tc.h, tc.fh)
						default:
							_, err = app.Append(ref, lbls, 100, 1)
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
					if !rollback {
						require.NoError(t, creator.Commit())
					}
					seriesState := h.series.getByID(chunks.HeadSeriesRef(ref))
					seriesState.Lock()
					pending := seriesState.pendingCommitCount()
					seriesState.Unlock()
					require.Zero(t, pending, "all reservations must be released")
					require.Zero(t, prom_testutil.ToFloat64(h.metrics.pendingCommitUnderflow))
					recs := readTestWAL(t, wal.Dir())
					require.GreaterOrEqual(t, len(recs), 2)
					require.IsType(t, []record.RefSeries{}, recs[0])
					series := recs[0].([]record.RefSeries)
					require.Len(t, series, 1)
					require.Equal(t, chunks.HeadSeriesRef(ref), series[0].Ref)
					require.True(t, labels.Equal(lbls, series[0].Labels))
					require.IsType(t, tc.want, recs[1])
					if tc.extraLabelBytes > 0 {
						require.Equal(t, []record.RefSample{{Ref: chunks.HeadSeriesRef(ref), T: 100, V: 1}}, recs[1])
					}
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
	for _, version := range []int{1, 2} {
		t.Run(fmt.Sprintf("v%d", version), func(t *testing.T) {
			h, wal := newTestHead(t, 10000, compression.None, false)
			creator, create := newWALSeriesRecordTestAppender(t, h, version)
			borrower, appendSample := newWALSeriesRecordTestAppender(t, h, version)
			refs := make([]storage.SeriesRef, 2)
			for i := range refs {
				var err error
				refs[i], err = create(0, labels.FromStrings("series", strconv.Itoa(i)), 200, 2)
				require.NoError(t, err)
			}
			// Nonconsecutive duplicates must be deduplicated even after a failed write.
			for _, ts := range []int64{100, 101} {
				for _, ref := range refs {
					_, err := appendSample(ref, labels.EmptyLabels(), ts, 1)
					require.NoError(t, err)
				}
			}
			require.NoError(t, wal.Close())
			require.Error(t, creator.Commit())
			var err error
			h.wal, err = wlog.New(nil, nil, filepath.Join(t.TempDir(), "wal"), compression.None)
			require.NoError(t, err)
			require.NoError(t, borrower.Commit())
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
	for _, version := range []int{1, 2} {
		for name, useGC := range map[string]bool{"eviction": false, "gc": true} {
			t.Run(fmt.Sprintf("v%d/%s", version, name), func(t *testing.T) {
				opts := newTestHeadDefaultOptions(10000, false)
				opts.EnableMetadataWALRecords = true
				h, wal := newTestHeadWithOptions(t, compression.None, opts)
				h.initTime(0)
				lset := labels.FromStrings("name", "removed")
				firstAppender, appendFirstSample := newWALSeriesRecordTestAppender(t, h, version)
				secondAppender, _ := newWALSeriesRecordTestAppender(t, h, version)
				ref, err := appendFirstSample(0, lset, 200, 1)
				require.NoError(t, err)
				meta := metadata.Metadata{Help: "Pending metadata."}
				if version == 1 {
					_, err = secondAppender.(storage.Appender).UpdateMetadata(ref, lset, meta)
				} else {
					// AppenderV2 queues metadata together with a sample.
					_, err = secondAppender.(storage.AppenderV2).Append(ref, lset, 0, 100, 1, nil, nil, storage.AOptions{Metadata: meta})
				}
				require.NoError(t, err)
				require.NoError(t, wal.Close())
				require.Error(t, firstAppender.Commit())
				h.wal, err = wlog.New(nil, nil, filepath.Join(t.TempDir(), "wal"), compression.None)
				require.NoError(t, err)
				remove := func(mint int64) {
					if useGC {
						require.NoError(t, h.Truncate(mint))
						return
					}
					_, err := h.gcSeries([]storage.SeriesRef{ref}, math.MaxInt64, func(*memSeries) bool { return true })
					require.NoError(t, err)
				}
				remove(160)
				require.NotNil(t, h.series.getByID(chunks.HeadSeriesRef(ref)), "pending data must prevent removal")
				require.NoError(t, secondAppender.Rollback())
				remove(161)
				require.Nil(t, h.series.getByID(chunks.HeadSeriesRef(ref)), "rollback must release all reservations")
				replacement, appendReplacement := newWALSeriesRecordTestAppender(t, h, version)
				newRef, err := appendReplacement(0, lset, 300, 3)
				require.NoError(t, err)
				require.NotEqual(t, ref, newRef)
				require.NoError(t, replacement.Commit())
				var refs []chunks.HeadSeriesRef
				for _, rec := range readTestWAL(t, h.wal.Dir()) {
					if series, ok := rec.([]record.RefSeries); ok {
						for _, s := range series {
							refs = append(refs, s.Ref)
						}
					}
				}
				require.Equal(t, []chunks.HeadSeriesRef{chunks.HeadSeriesRef(newRef)}, refs, "only the replacement series record should be logged")
				require.Zero(t, prom_testutil.ToFloat64(h.metrics.pendingCommitUnderflow))
			})
		}
	}
}

// TestWALSeriesRecordsOverlappingBatches checks concurrent commits with shared refs.
func TestWALSeriesRecordsOverlappingBatches(t *testing.T) {
	for _, version := range []int{1, 2} {
		t.Run(fmt.Sprintf("v%d", version), func(t *testing.T) {
			h, wal := newTestHead(t, 10000, compression.None, false)
			h.initTime(0)
			creator := h.Appender(t.Context())
			refs := make([]storage.SeriesRef, 3)
			for i := range refs {
				var err error
				refs[i], err = creator.Append(0, labels.FromStrings("series", strconv.Itoa(i)), 200, 2)
				require.NoError(t, err)
			}
			batches := [][]storage.SeriesRef{
				refs[:2],
				refs[1:],
				// With only the shared ref, this appender can find all its series
				// records already logged after waiting for walSeriesMtx.
				refs[1:2],
			}
			apps := make([]storage.AppenderTransaction, len(batches))
			for i, batch := range batches {
				var appendSample func(storage.SeriesRef, labels.Labels, int64, float64) (storage.SeriesRef, error)
				apps[i], appendSample = newWALSeriesRecordTestAppender(t, h, version)
				for _, ref := range batch {
					_, err := appendSample(ref, labels.EmptyLabels(), 100, 1)
					require.NoError(t, err)
				}
			}
			var wg sync.WaitGroup
			errs := make([]error, len(apps))
			// Hold the series-record lock while starting the committers, yielding
			// to each so they can wait for it before any series record is logged.
			h.walSeriesMtx.Lock()
			for i, app := range apps {
				wg.Go(func() {
					errs[i] = app.Commit()
				})
				runtime.Gosched()
			}
			h.walSeriesMtx.Unlock()
			wg.Wait()
			for _, err := range errs {
				require.NoError(t, err)
			}
			require.NoError(t, creator.Rollback())
			seen := make(map[chunks.HeadSeriesRef]bool)
			var samples int
			for _, rec := range readTestWAL(t, wal.Dir()) {
				switch r := rec.(type) {
				case []record.RefSeries:
					for _, s := range r {
						require.False(t, seen[s.Ref], "overlapping batches must not duplicate series records")
						seen[s.Ref] = true
					}
				case []record.RefSample:
					for _, s := range r {
						require.True(t, seen[s.Ref], "samples must follow their series record")
						samples++
					}
				}
			}
			require.Len(t, seen, 3)
			require.Equal(t, 5, samples)
		})
	}
}

// TestMetadataGCBeforeReservation rejects a series removed between lookup and reservation.
func TestMetadataGCBeforeReservation(t *testing.T) {
	for _, replacement := range []bool{false, true} {
		t.Run(fmt.Sprintf("replacement=%t", replacement), func(t *testing.T) {
			h, wal := newTestHead(t, 10000, compression.None, false)
			h.initTime(0)
			lbls := labels.FromStrings("name", "metadata_gc")
			creator := h.Appender(t.Context())
			oldRef, err := creator.Append(0, lbls, 100, 1)
			require.NoError(t, err)
			require.NoError(t, creator.Rollback())
			h.testAfterSeriesLookup = func(*memSeries) {
				h.testAfterSeriesLookup = nil
				deleted, err := h.gcSeries([]storage.SeriesRef{oldRef}, math.MaxInt64, func(*memSeries) bool { return true })
				require.NoError(t, err)
				require.Len(t, deleted, 1)
				if replacement {
					a := h.Appender(t.Context())
					ref, err := a.Append(0, lbls, 200, 2)
					require.NoError(t, err)
					require.NotEqual(t, oldRef, ref)
					require.NoError(t, a.Commit())
				}
			}
			a := h.Appender(t.Context())
			defer func() { _ = a.Rollback() }()
			_, err = a.UpdateMetadata(oldRef, lbls, metadata.Metadata{Help: "Late metadata."})
			require.ErrorContains(t, err, "unknown series")
			require.NoError(t, a.Rollback())
			wantSeries := uint64(0)
			if replacement {
				wantSeries = 1
			}
			require.Equal(t, wantSeries, h.NumSeries())
			require.Zero(t, prom_testutil.ToFloat64(h.metrics.pendingCommitUnderflow))
			for _, rec := range readTestWAL(t, wal.Dir()) {
				_, isMetadata := rec.([]record.RefMetadata)
				require.False(t, isMetadata, "rejected metadata must not reach the WAL")
			}
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

// BenchmarkHeadAppender_WALSeriesRecords measures existing, mixed, high-churn
// and changing-metadata batches through both appender APIs.
func BenchmarkHeadAppender_WALSeriesRecords(b *testing.B) {
	for _, version := range []int{1, 2} {
		for _, tc := range []struct {
			newSeries int
			metadata  bool
		}{{}, {newSeries: 10}, {newSeries: 100}, {metadata: true}} {
			b.Run(fmt.Sprintf("appender=v%d/new=%d/metadata=%t", version, tc.newSeries, tc.metadata), func(b *testing.B) {
				h := newWALSeriesRecordBenchmarkHead(b)
				h.opts.EnableMetadataWALRecords = tc.metadata
				refs := make([][]storage.SeriesRef, runtime.GOMAXPROCS(0))
				for w := range refs {
					app := h.Appender(b.Context())
					for i := range 100 {
						ref, err := app.Append(0, labels.FromStrings("worker", strconv.Itoa(w), "series", strconv.Itoa(i)), 0, 0)
						require.NoError(b, err)
						refs[w] = append(refs[w], ref)
					}
					require.NoError(b, app.Commit())
				}
				var workerID atomic.Int64
				b.ReportAllocs()
				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					worker := int(workerID.Add(1) - 1)
					workerLabel := strconv.Itoa(worker)
					ts := int64(1)
					for pb.Next() {
						var meta metadata.Metadata
						if tc.metadata {
							meta.Help = strconv.FormatInt(ts, 10)
						}
						if version == 1 {
							app := h.Appender(b.Context())
							for _, ref := range refs[worker] {
								if _, err := app.Append(ref, labels.EmptyLabels(), ts, float64(ts)); err != nil {
									b.Fatal(err)
								}
								if tc.metadata {
									if _, err := app.UpdateMetadata(ref, labels.EmptyLabels(), meta); err != nil {
										b.Fatal(err)
									}
								}
							}
							for n := range tc.newSeries {
								lset := labels.FromStrings("worker", workerLabel, "new", strconv.FormatInt(ts*int64(tc.newSeries)+int64(n), 10))
								if _, err := app.Append(0, lset, ts, float64(ts)); err != nil {
									b.Fatal(err)
								}
							}
							if err := app.Commit(); err != nil {
								b.Fatal(err)
							}
						} else {
							app := h.AppenderV2(b.Context())
							for _, ref := range refs[worker] {
								if _, err := app.Append(ref, labels.EmptyLabels(), 0, ts, float64(ts), nil, nil, storage.AOptions{Metadata: meta}); err != nil {
									b.Fatal(err)
								}
							}
							for n := range tc.newSeries {
								lset := labels.FromStrings("worker", workerLabel, "new", strconv.FormatInt(ts*int64(tc.newSeries)+int64(n), 10))
								if _, err := app.Append(0, lset, 0, ts, float64(ts), nil, nil, storage.AOptions{}); err != nil {
									b.Fatal(err)
								}
							}
							if err := app.Commit(); err != nil {
								b.Fatal(err)
							}
						}
						ts++
					}
				})
				b.StopTimer()
				require.Equal(b, uint64(100*len(refs)+tc.newSeries*b.N), h.NumSeries())
				require.Zero(b, prom_testutil.ToFloat64(h.metrics.pendingCommitUnderflow))
				b.ReportMetric(float64(100+tc.newSeries), "sample_appends/op")
			})
		}
	}
}

// BenchmarkHeadAppender_ConcurrentNewSeries measures full concurrent rounds.
// All writers buffer samples for the same fresh series before any commit;
// equal timestamps and values make every commit order valid with OOO disabled.
func BenchmarkHeadAppender_ConcurrentNewSeries(b *testing.B) {
	type appendRound struct {
		labels              []labels.Labels
		timestamp           int64
		start, commit       chan struct{}
		appended, committed sync.WaitGroup
		errors              []error
	}
	for _, version := range []int{1, 2} {
		for _, writers := range []int{2, 4, 8} {
			for _, series := range []int{1, 100} {
				b.Run(fmt.Sprintf("appender=v%d/writers=%d/series=%d", version, writers, series), func(b *testing.B) {
					h := newWALSeriesRecordBenchmarkHead(b)
					ctx := b.Context()
					jobs := make(chan *appendRound)
					var workers sync.WaitGroup
					for worker := range writers {
						workers.Go(func() {
							for round := range jobs {
								<-round.start
								round.errors[worker] = nil
								var txn storage.AppenderTransaction
								if version == 1 {
									app := h.Appender(ctx)
									txn = app
									for _, lset := range round.labels {
										if _, err := app.Append(0, lset, round.timestamp, 1); err != nil {
											round.errors[worker] = err
											break
										}
									}
								} else {
									app := h.AppenderV2(ctx)
									txn = app
									for _, lset := range round.labels {
										if _, err := app.Append(0, lset, 0, round.timestamp, 1, nil, nil, storage.AOptions{}); err != nil {
											round.errors[worker] = err
											break
										}
									}
								}
								round.appended.Done()
								<-round.commit
								if round.errors[worker] == nil {
									round.errors[worker] = txn.Commit()
								} else {
									round.errors[worker] = errors.Join(round.errors[worker], txn.Rollback())
								}
								round.committed.Done()
							}
						})
					}
					b.Cleanup(func() { close(jobs); workers.Wait() })
					round := appendRound{labels: make([]labels.Labels, series), errors: make([]error, writers)}
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
					b.ReportMetric(float64(writers*series), "sample_appends/op")
				})
			}
		}
	}
}
