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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/compression"
)

// reopenHeadMmapOnly persists the series' only head chunk and reopens the head.
// WAL replay skips samples already covered by mmapped chunks, leaving the series
// with mmapped chunks but no head chunk, as after recovery without a newer WAL tail.
func reopenHeadMmapOnly(t *testing.T, opts *HeadOptions, h *Head, ref chunks.HeadSeriesRef) (*Head, *memSeries) {
	t.Helper()
	ms := h.series.getByID(ref)
	c := ms.headChunks
	// The callback may run on the chunk write queue goroutine, so check its error here.
	writeErr := make(chan error, 1)
	h.chunkDiskMapper.WriteChunk(ms.ref, c.minTime, c.maxTime, c.chunk, false, func(err error) { writeErr <- err })
	require.NoError(t, <-writeErr)
	require.NoError(t, h.Close())
	wal, err := wlog.NewSize(nil, nil, filepath.Join(opts.ChunkDirRoot, "wal"), 32768, compression.None)
	require.NoError(t, err)
	h, err = NewHead(nil, nil, wal, nil, opts, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, h.Close()) })
	require.NoError(t, h.Init(0))
	ms = h.series.getByID(ref)
	require.Nil(t, ms.headChunks)
	require.NotEmpty(t, ms.mmappedChunks)
	return h, ms
}

func TestMemSeries_appendable(t *testing.T) {
	t.Run("recovery", func(t *testing.T) {
		for _, snapshot := range []bool{false, true} {
			for name, scenario := range sampleTypeScenarios {
				for _, tc := range []struct {
					name    string
					window  int64
					wantErr error
				}{
					{name: "ooo disabled", wantErr: storage.ErrOutOfOrderSample},
					{name: "within ooo window", window: 1000},
					{name: "outside ooo window", window: 40, wantErr: storage.ErrTooOldSample},
				} {
					t.Run(fmt.Sprintf("snapshot=%t/%s/%s", snapshot, name, tc.name), func(t *testing.T) {
						opts := newTestHeadDefaultOptions(DefaultBlockDuration, false)
						opts.OutOfOrderTimeWindow.Store(tc.window)
						opts.EnableMemorySnapshotOnShutdown = snapshot
						h, _ := newTestHeadWithOptions(t, compression.None, opts)
						require.NoError(t, h.Init(0))
						ls := labels.FromStrings("metric", "mmap_recovery")
						app := h.AppenderV2(t.Context())
						ref, _, err := scenario.appendFunc(app, ls, 100, 1)
						require.NoError(t, err)
						_, _, err = scenario.appendFunc(app, ls, 200, 2)
						require.NoError(t, err)
						// WAL also contains a conflicting sample that Commit does not store.
						_, _, err = scenario.appendFunc(app, ls, 200, 999)
						require.NoError(t, err)
						require.NoError(t, app.Commit())
						h, ms := reopenHeadMmapOnly(t, opts, h, chunks.HeadSeriesRef(ref))
						newAppender := func() storage.AppenderV2 {
							return h.AppenderV2(t.Context())
						}
						a := newAppender()
						_, _, err = scenario.appendFunc(a, ls, 150, 3)
						if tc.wantErr == nil {
							require.NoError(t, err)
						} else {
							require.ErrorIs(t, err, tc.wantErr)
						}
						require.NoError(t, a.Commit())
						require.Nil(t, ms.headChunks, "an OOO append must not create an in-order head chunk")
						// The last mmapped sample is read back from its chunk, so a
						// conflicting value at its timestamp is rejected as a duplicate,
						// while an exact duplicate is accepted without being stored again.
						a = newAppender()
						_, _, err = scenario.appendFunc(a, ls, 200, 999)
						require.ErrorIs(t, err, storage.ErrDuplicateSampleForTimestamp, "the last mmapped sample must be used for conflict detection")
						require.NoError(t, a.Commit())
						a = newAppender()
						_, _, err = scenario.appendFunc(a, ls, 200, 2)
						require.NoError(t, err, "an exact duplicate of the last mmapped sample is accepted")
						require.NoError(t, a.Commit())
						require.Nil(t, ms.headChunks, "an equal-timestamp append must not create an in-order head chunk")
						if ms.ooo != nil && ms.ooo.oooHeadChunk != nil {
							require.NotContains(t, ms.ooo.oooHeadChunk.chunk.samples, scenario.sampleFunc(200, 999), "a conflicting equal-timestamp append must not reach OOO storage")
						}
						a = newAppender()
						_, _, err = scenario.appendFunc(a, ls, 300, 4)
						require.NoError(t, err)
						require.NoError(t, a.Commit())
						a = newAppender()
						_, _, err = scenario.appendFunc(a, ls, 300, 999)
						require.ErrorIs(t, err, storage.ErrDuplicateSampleForTimestamp, "a new head chunk restores in-order duplicate checks")
						require.NoError(t, a.Rollback())

						want := map[int64]sample{100: scenario.sampleFunc(100, 1), 200: scenario.sampleFunc(200, 2), 300: scenario.sampleFunc(300, 4)}
						if tc.wantErr == nil {
							want[150] = scenario.sampleFunc(150, 3)
						}
						q := NewHeadAndOOOQuerier(0, 0, 400, h, h.oooIso.TrackReadAfter(0), nil, false)
						defer func() { require.NoError(t, q.Close()) }()
						set := q.Select(t.Context(), true, nil, labels.MustNewMatcher(labels.MatchEqual, "metric", "mmap_recovery"))
						require.True(t, set.Next())
						it := set.At().Iterator(nil)
						count := 0
						for typ := it.Next(); typ != chunkenc.ValNone; typ = it.Next() {
							expected, ok := want[it.AtT()]
							require.True(t, ok, "unexpected timestamp %d", it.AtT())
							matches := func(expected sample) bool {
								switch typ {
								case chunkenc.ValFloat:
									_, actual := it.At()
									return expected.f == actual
								case chunkenc.ValHistogram:
									_, actual := it.AtHistogram(nil)
									return expected.h.Equals(actual)
								case chunkenc.ValFloatHistogram:
									_, actual := it.AtFloatHistogram(nil)
									return expected.fh.Equals(actual)
								}
								return false
							}
							require.True(t, matches(expected), "unexpected value at timestamp %d", it.AtT())
							count++
						}
						require.NoError(t, it.Err())
						require.Equal(t, len(want), count, "all accepted samples must be queryable")
						require.False(t, set.Next())
						require.NoError(t, set.Err())
					})
				}
			}
		}
	})

	t.Run("wal repair", func(t *testing.T) {
		for name, scenario := range sampleTypeScenarios {
			for _, corrupt := range []bool{false, true} {
				for _, window := range []int64{0, 1000} {
					t.Run(fmt.Sprintf("%s/corrupt=%t/ooo_window=%d", name, corrupt, window), func(t *testing.T) {
						h, w := newTestHead(t, 100, compression.None, false)
						require.NoError(t, h.Init(0))
						ls := labels.FromStrings("metric", "wal_repair")
						a := h.AppenderV2(t.Context())
						ref, _, err := scenario.appendFunc(a, ls, 100, 1)
						require.NoError(t, err)
						_, _, err = scenario.appendFunc(a, ls, 150, 2)
						require.NoError(t, err)
						require.NoError(t, a.Commit())
						seg, offset, err := w.LastSegmentAndOffset()
						require.NoError(t, err)
						a = h.AppenderV2(t.Context())
						_, _, err = scenario.appendFunc(a, ls, 200, 3)
						require.NoError(t, err)
						require.NoError(t, a.Commit())
						h.mmapHeadChunks()
						ms := h.series.getByID(chunks.HeadSeriesRef(ref))
						require.Len(t, ms.mmappedChunks, 1)
						require.Equal(t, int64(150), ms.mmappedChunks[0].maxTime)
						require.NotNil(t, ms.headChunks)
						require.Equal(t, int64(200), ms.headChunks.maxTime)
						require.NoError(t, h.Close())
						if corrupt {
							// Damage only the newer chunk's WAL record; the older chunk is already mapped.
							f, err := os.OpenFile(wlog.SegmentName(w.Dir(), seg), os.O_WRONLY, 0)
							require.NoError(t, err)
							_, err = f.WriteAt([]byte{255}, int64(offset))
							require.NoError(t, err)
							require.NoError(t, f.Close())
						}
						dbOpts := DefaultOptions()
						dbOpts.OutOfOrderTimeWindow = window
						db, err := Open(h.opts.ChunkDirRoot, nil, nil, dbOpts, nil)
						require.NoError(t, err)
						defer func() { require.NoError(t, db.Close()) }()
						ms = db.Head().series.getByID(chunks.HeadSeriesRef(ref))
						require.NotNil(t, ms)
						ts, val := int64(200), int64(3)
						if corrupt {
							require.Nil(t, ms.headChunks)
							require.NotEmpty(t, ms.mmappedChunks)
							ts, val = 150, 2
						} else {
							require.NotNil(t, ms.headChunks)
						}
						newAppender := func() storage.AppenderV2 {
							return db.AppenderV2(t.Context())
						}
						if corrupt {
							// Repair left the series without a head chunk; the last mmapped
							// sample is read back from its chunk, so a conflicting value at its
							// timestamp is rejected instead of being silently dropped, and an
							// exact duplicate is accepted without being stored again.
							app := newAppender()
							_, _, err = scenario.appendFunc(app, ls, ts, 999)
							require.ErrorIs(t, err, storage.ErrDuplicateSampleForTimestamp)
							require.NoError(t, app.Commit())
							app = newAppender()
							_, _, err = scenario.appendFunc(app, ls, ts, val)
							require.NoError(t, err)
							require.NoError(t, app.Commit())
							require.Nil(t, ms.headChunks)
							require.Nil(t, ms.ooo, "equal-timestamp appends must not create OOO storage")
							return
						}
						app := newAppender()
						_, _, err = scenario.appendFunc(app, ls, ts, val)
						require.NoError(t, err, "the last stored sample is an exact duplicate")
						require.NoError(t, app.Commit())
						app = newAppender()
						_, _, err = scenario.appendFunc(app, ls, ts, 999)
						require.ErrorIs(t, err, storage.ErrDuplicateSampleForTimestamp)
						require.NoError(t, app.Rollback())
					})
				}
			}
		}
	})

	t.Run("unreadable chunk", func(t *testing.T) {
		for _, window := range []int64{0, 1000} {
			t.Run(fmt.Sprintf("ooo_window=%d", window), func(t *testing.T) {
				opts := newTestHeadDefaultOptions(DefaultBlockDuration, false)
				opts.OutOfOrderTimeWindow.Store(window)
				h, _ := newTestHeadWithOptions(t, compression.None, opts)
				require.NoError(t, h.Init(0))
				ls := labels.FromStrings("metric", "mmap_recovery_unreadable_chunk")
				app := h.AppenderV2(t.Context())
				ref, err := app.Append(0, ls, 0, 100, 1, nil, nil, storage.AOptions{})
				require.NoError(t, err)
				_, err = app.Append(0, ls, 0, 200, 2, nil, nil, storage.AOptions{})
				require.NoError(t, err)
				require.NoError(t, app.Commit())
				h, ms := reopenHeadMmapOnly(t, opts, h, chunks.HeadSeriesRef(ref))
				// Point at a chunk file that does not exist, so duplicate detection
				// cannot read the last mmapped sample and must fall back to the
				// out-of-order checks instead of silently accepting the append.
				ms.mmappedChunks[len(ms.mmappedChunks)-1].ref = chunks.ChunkDiskMapperRef(999 << 32)
				newAppender := func() storage.AppenderV2 {
					return h.AppenderV2(t.Context())
				}
				a := newAppender()
				_, err = a.Append(0, ls, 0, 200, 999, nil, nil, storage.AOptions{})
				if window == 0 {
					require.ErrorIs(t, err, storage.ErrOutOfOrderSample)
				} else {
					require.NoError(t, err)
				}
				require.NoError(t, a.Commit())
				if window > 0 {
					require.NotNil(t, ms.ooo)
					require.NotNil(t, ms.ooo.oooHeadChunk)
					require.Contains(t, ms.ooo.oooHeadChunk.chunk.samples, sampleTypeScenarios["float"].sampleFunc(200, 999))
				}
			})
		}
	})

	// Two mmapped chunks prove that equal-timestamp duplicate detection reads the
	// last mmapped chunk, not an earlier one.
	t.Run("multiple mmapped chunks", func(t *testing.T) {
		// A small chunk range makes the two appends cut separate head chunks.
		opts := newTestHeadDefaultOptions(100, false)
		h, _ := newTestHeadWithOptions(t, compression.None, opts)
		require.NoError(t, h.Init(0))
		ls := labels.FromStrings("metric", "mmap_recovery_last_chunk")
		app := h.AppenderV2(t.Context())
		ref, err := app.Append(0, ls, 0, 100, 1, nil, nil, storage.AOptions{})
		require.NoError(t, err)
		_, err = app.Append(0, ls, 0, 200, 2, nil, nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		h.mmapHeadChunks()
		ms := h.series.getByID(chunks.HeadSeriesRef(ref))
		require.Len(t, ms.mmappedChunks, 1)
		require.NotNil(t, ms.headChunks)
		// Persist the newest head chunk too and drop the in-memory copy, leaving the
		// series mmap-only with two chunks, as after recovery without a newer WAL tail.
		c := ms.headChunks
		writeErr := make(chan error, 1)
		r := h.chunkDiskMapper.WriteChunk(ms.ref, c.minTime, c.maxTime, c.chunk, false, func(err error) { writeErr <- err })
		require.NoError(t, <-writeErr)
		ms.mmappedChunks = append(ms.mmappedChunks, &mmappedChunk{
			ref:        r,
			minTime:    c.minTime,
			maxTime:    c.maxTime,
			numSamples: uint16(c.chunk.NumSamples()),
			encoding:   c.chunk.Encoding(),
		})
		ms.setHeadChunks(nil, 0)
		ms.app = nil
		require.Len(t, ms.mmappedChunks, 2)

		app = h.AppenderV2(t.Context())
		_, err = app.Append(0, ls, 0, 200, 2, nil, nil, storage.AOptions{})
		require.NoError(t, err, "the exact duplicate must be detected against the last mmapped chunk")
		require.NoError(t, app.Commit())
		app = h.AppenderV2(t.Context())
		_, err = app.Append(0, ls, 0, 200, 999, nil, nil, storage.AOptions{})
		require.ErrorIs(t, err, storage.ErrDuplicateSampleForTimestamp)
		require.NoError(t, app.Rollback())
		// An older timestamp matches the earlier mmapped chunk, but equal-timestamp
		// duplicate detection only concerns the series' last sample; older samples
		// are rejected by the out-of-order/out-of-bounds checks (OOO is disabled).
		app = h.AppenderV2(t.Context())
		_, err = app.Append(0, ls, 0, 100, 999, nil, nil, storage.AOptions{})
		require.ErrorIs(t, err, storage.ErrOutOfBounds)
		require.NoError(t, app.Rollback())
	})

	t.Run("type conflict", func(t *testing.T) {
		opts := newTestHeadDefaultOptions(DefaultBlockDuration, false)
		h, _ := newTestHeadWithOptions(t, compression.None, opts)
		require.NoError(t, h.Init(0))
		ls := labels.FromStrings("metric", "mmap_recovery_type_conflict")
		app := h.AppenderV2(t.Context())
		ref, err := app.Append(0, ls, 0, 100, 0, tsdbutil.GenerateTestHistogram(1), nil, storage.AOptions{})
		require.NoError(t, err)
		_, err = app.Append(0, ls, 0, 200, 0, tsdbutil.GenerateTestHistogram(2), nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		h, _ = reopenHeadMmapOnly(t, opts, h, chunks.HeadSeriesRef(ref))
		// The last mmapped sample is a histogram, so a float at the same timestamp
		// must be rejected as a type conflict rather than taking the OOO path.
		app = h.AppenderV2(t.Context())
		_, err = app.Append(0, ls, 0, 200, 42, nil, nil, storage.AOptions{})
		require.ErrorIs(t, err, storage.NewDuplicateHistogramToFloatErr(200, 42))
		require.NoError(t, app.Rollback())
	})
}
