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
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/compression"
)

func TestHeadAppendAfterMmapOnlyRecovery(t *testing.T) {
	for _, useV2 := range []bool{false, true} {
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
					t.Run(fmt.Sprintf("v2=%t/snapshot=%t/%s/%s", useV2, snapshot, name, tc.name), func(t *testing.T) {
						opts := newTestHeadDefaultOptions(DefaultBlockDuration, false)
						opts.OutOfOrderTimeWindow.Store(tc.window)
						opts.EnableMemorySnapshotOnShutdown = snapshot
						h, _ := newTestHeadWithOptions(t, compression.None, opts)
						require.NoError(t, h.Init(0))
						ls := labels.FromStrings("metric", "mmap_recovery")
						app := h.Appender(t.Context())
						ref, _, err := scenario.appendFunc(app, ls, 100, 1)
						require.NoError(t, err)
						_, _, err = scenario.appendFunc(app, ls, 200, 2)
						require.NoError(t, err)
						// WAL also contains a conflicting sample that Commit does not store.
						_, _, err = scenario.appendFunc(app, ls, 200, 999)
						require.NoError(t, err)
						require.NoError(t, app.Commit())
						ms := h.series.getByID(chunks.HeadSeriesRef(ref))
						c := ms.headChunks
						// Persist all samples to reproduce recovery without a newer WAL tail.
						h.chunkDiskMapper.WriteChunk(ms.ref, c.minTime, c.maxTime, c.chunk, false, func(err error) { require.NoError(t, err) })
						require.NoError(t, h.Close())
						wal, err := wlog.NewSize(nil, nil, filepath.Join(opts.ChunkDirRoot, "wal"), 32768, compression.None)
						require.NoError(t, err)
						h, err = NewHead(nil, nil, wal, nil, opts, nil)
						require.NoError(t, err)
						t.Cleanup(func() { require.NoError(t, h.Close()) })
						require.NoError(t, h.Init(0))
						ms = h.series.getByID(chunks.HeadSeriesRef(ref))
						require.NotEmpty(t, ms.mmappedChunks)
						require.Nil(t, ms.headChunks)
						newAppender := func() storage.LimitedAppenderV1 {
							if useV2 {
								return storage.AppenderV2AsLimitedV1(h.AppenderV2(t.Context()))
							}
							return h.Appender(t.Context())
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
						a = newAppender()
						_, _, err = scenario.appendFunc(a, ls, 200, 2)
						require.NoError(t, err, "the latest stored value is an exact duplicate")
						require.NoError(t, a.Commit())
						a = newAppender()
						_, _, err = scenario.appendFunc(a, ls, 200, 999)
						require.ErrorIs(t, err, storage.ErrDuplicateSampleForTimestamp)
						require.NoError(t, a.Rollback())
						a = newAppender()
						_, _, err = scenario.appendFunc(a, ls, 300, 4)
						require.NoError(t, err)
						require.NoError(t, a.Commit())

						want := map[int64]sample{100: scenario.sampleFunc(100, 1), 200: scenario.sampleFunc(200, 2), 300: scenario.sampleFunc(300, 4)}
						if tc.wantErr == nil {
							want[150] = scenario.sampleFunc(150, 3)
						}
						q := NewHeadAndOOOQuerier(0, 0, 400, h, h.oooIso.TrackReadAfter(0), nil)
						defer func() { require.NoError(t, q.Close()) }()
						set := q.Select(t.Context(), true, nil, labels.MustNewMatcher(labels.MatchEqual, "metric", "mmap_recovery"))
						require.True(t, set.Next())
						it := set.At().Iterator(nil)
						count := 0
						for typ := it.Next(); typ != chunkenc.ValNone; typ = it.Next() {
							expected, ok := want[it.AtT()]
							require.True(t, ok, "unexpected timestamp %d", it.AtT())
							switch typ {
							case chunkenc.ValFloat:
								_, actual := it.At()
								require.Equal(t, expected.f, actual)
							case chunkenc.ValHistogram:
								_, actual := it.AtHistogram(nil)
								require.True(t, expected.h.Equals(actual))
							case chunkenc.ValFloatHistogram:
								_, actual := it.AtFloatHistogram(nil)
								require.True(t, expected.fh.Equals(actual))
							}
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
	}
}

func TestHeadAppendAfterMmapOnlyWALRepair(t *testing.T) {
	for _, useV2 := range []bool{false, true} {
		for name, scenario := range sampleTypeScenarios {
			for _, corrupt := range []bool{false, true} {
				t.Run(fmt.Sprintf("v2=%t/%s/corrupt=%t", useV2, name, corrupt), func(t *testing.T) {
					h, w := newTestHead(t, 100, compression.None, false)
					require.NoError(t, h.Init(0))
					ls := labels.FromStrings("metric", "wal_repair")
					a := h.Appender(t.Context())
					ref, _, err := scenario.appendFunc(a, ls, 100, 1)
					require.NoError(t, err)
					_, _, err = scenario.appendFunc(a, ls, 150, 2)
					require.NoError(t, err)
					require.NoError(t, a.Commit())
					seg, offset, err := w.LastSegmentAndOffset()
					require.NoError(t, err)
					a = h.Appender(t.Context())
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
					db, err := Open(h.opts.ChunkDirRoot, nil, nil, DefaultOptions(), nil)
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
					newAppender := func() storage.LimitedAppenderV1 {
						if useV2 {
							return storage.AppenderV2AsLimitedV1(db.AppenderV2(t.Context()))
						}
						return db.Appender(t.Context())
					}
					app := newAppender()
					_, _, err = scenario.appendFunc(app, ls, ts, val)
					require.NoError(t, err, "the last stored sample remains an exact duplicate after repair")
					require.NoError(t, app.Commit())
					app = newAppender()
					_, _, err = scenario.appendFunc(app, ls, ts, 999)
					require.ErrorIs(t, err, storage.ErrDuplicateSampleForTimestamp)
					require.NoError(t, app.Rollback())
				})
			}
		}
	}
}
