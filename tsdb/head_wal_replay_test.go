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
	"os"
	"testing"
	"time"

	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/wlog"
)

func TestWALReplayPreservesMappedChunksAcrossSeriesRefs(t *testing.T) {
	for _, appV2 := range []bool{false, true} {
		for name, scenario := range sampleTypeScenarios {
			for _, tc := range []struct {
				name                                            string
				refs                                            int
				initialOOO, totalOOO                            int
				mappedInOrder, initialMappedInOrder, corruptWAL bool
			}{
				{name: "single reference", refs: 1, initialOOO: 1, totalOOO: 33},
				{name: "unmapped OOO", refs: 2, initialOOO: 1, totalOOO: 32},
				{name: "mapped OOO under earlier reference", refs: 2, initialOOO: 1, totalOOO: 33},
				{name: "mapped OOO under later reference", refs: 2, initialOOO: 33, totalOOO: 33},
				{name: "mapped OOO under both references", refs: 2, initialOOO: 33, totalOOO: 65},
				{name: "three references", refs: 3, initialOOO: 1, totalOOO: 33},
				{name: "mapped in-order", refs: 2, mappedInOrder: true},
				{name: "mapped in-order under both references", refs: 2, mappedInOrder: true, initialMappedInOrder: true},
				{name: "mapped in-order and OOO", refs: 2, mappedInOrder: true, initialOOO: 1, totalOOO: 33},
				{name: "WAL repair with single reference", refs: 1, mappedInOrder: true, corruptWAL: true},
				{name: "WAL repair with two references", refs: 2, mappedInOrder: true, corruptWAL: true},
			} {
				t.Run(fmt.Sprintf("%s/%s/appV2=%v", tc.name, name, appV2), func(t *testing.T) {
					dir := t.TempDir()
					opts := DefaultOptions()
					opts.WALSegmentSize = 32 * 1024
					opts.MaxBlockChunkSegmentSize = 1024 * 1024
					opts.OutOfOrderTimeWindow = 300 * time.Minute.Milliseconds()
					openDB := func() *DB {
						db := newTestDB(t, withDir(dir), withOpts(opts))
						db.DisableCompactions()
						return db
					}
					ls := labels.FromStrings("foo", "bar")
					expected := map[string][]chunks.Sample{ls.String(): {}}
					appendOne := func(db *DB, ts int64) storage.SeriesRef {
						var app storage.LimitedAppenderV1
						if appV2 {
							app = storage.AppenderV2AsLimitedV1(db.AppenderV2(t.Context()))
						} else {
							app = db.Appender(t.Context())
						}
						ref, _, err := scenario.appendFunc(app, ls, ts, ts)
						require.NoError(t, err)
						require.NoError(t, app.Commit())
						return ref
					}
					expectSample := func(ts int64) {
						expected[ls.String()] = append(expected[ls.String()], scenario.sampleFunc(ts, ts))
					}
					checkSamples := func(db *DB) {
						q, err := db.Querier(math.MinInt64, math.MaxInt64)
						require.NoError(t, err)
						requireEqualSeries(t, expected, query(t, q, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar")), true)
						require.Zero(t, prom_testutil.ToFloat64(db.head.metrics.mmapChunkCorruptionTotal))
					}

					db := openDB()
					var firstRef storage.SeriesRef
					// Compact and recreate the series, leaving its earlier definitions in the WAL.
					for i := 1; i < tc.refs; i++ {
						ts := int64(i) * 100 * time.Minute.Milliseconds()
						ref := appendOne(db, ts)
						if i == 1 {
							firstRef = ref
						}
						expectSample(ts)
						require.NoError(t, db.CompactHead(NewRangeHead(db.head, db.head.MinTime(), ts)))
						require.Zero(t, db.head.NumSeries())
					}
					ref := appendOne(db, 300*time.Minute.Milliseconds())
					if tc.refs > 1 {
						require.NotEqual(t, firstRef, ref)
					} else {
						firstRef = ref
					}
					for i := 0; i < tc.initialOOO; i++ {
						appendOne(db, 250*time.Minute.Milliseconds()+int64(i))
					}
					if tc.initialMappedInOrder {
						appendOne(db, 400*time.Minute.Milliseconds())
						db.head.mmapHeadChunks()
					}
					require.NoError(t, db.Close())

					// Replay merges the references. Subsequent chunks belong to the earlier reference.
					db = openDB()
					require.Equal(t, chunks.HeadSeriesRef(firstRef), db.head.series.getByHash(ls.Hash(), ls).ref)
					segment, offset, err := db.head.wal.LastSegmentAndOffset()
					require.NoError(t, err)
					for i := tc.initialOOO; i < tc.totalOOO; i++ {
						appendOne(db, 250*time.Minute.Milliseconds()+int64(i))
					}
					for i := 0; i < tc.totalOOO; i++ {
						expectSample(250*time.Minute.Milliseconds() + int64(i))
					}
					expectSample(300 * time.Minute.Milliseconds())
					if tc.mappedInOrder {
						for _, minute := range []int64{400, 500, 650} {
							ts := minute * time.Minute.Milliseconds()
							if !tc.initialMappedInOrder || minute != 400 {
								appendOne(db, ts)
							}
							expectSample(ts)
						}
						db.head.mmapHeadChunks()
						require.Len(t, db.head.series.getByHash(ls.Hash(), ls).mmappedChunks, 3)
					}
					checkSamples(db)
					require.NoError(t, db.Close())

					if tc.corruptWAL {
						// The chunks at 300, 400 and 500 are safely on disk. Only 650 still
						// depends on the WAL tail which repair will discard.
						f, err := os.OpenFile(wlog.SegmentName(db.head.wal.Dir(), segment), os.O_WRONLY, 0)
						require.NoError(t, err)
						_, err = f.WriteAt([]byte{255}, int64(offset))
						require.NoError(t, err)
						require.NoError(t, f.Close())
						expected[ls.String()] = expected[ls.String()][:len(expected[ls.String()])-1]
					}

					for range 3 {
						db = openDB()
						checkSamples(db)
						require.NoError(t, db.Close())
						if tc.mappedInOrder {
							// Replaying samples already covered by mapped chunks must not write
							// duplicate chunks that the next restart would treat as corruption.
							mapper, err := chunks.NewChunkDiskMapper(nil, mmappedChunksDir(dir), chunkenc.NewPool(), chunks.DefaultWriteBufferSize, 0)
							require.NoError(t, err)
							count := 0
							require.NoError(t, mapper.IterateAllChunks(func(_ chunks.HeadSeriesRef, _ chunks.ChunkDiskMapperRef, _, _ int64, _ uint16, _ chunkenc.Encoding, ooo bool) error {
								if !ooo {
									count++
								}
								return nil
							}))
							require.NoError(t, mapper.Close())
							require.Equal(t, 3, count)
						}
					}
				})
			}
		}
	}
}

func TestMergeReplayMmappedChunks(t *testing.T) {
	a := &mmappedChunk{ref: 1, minTime: 100, maxTime: 150}
	b := &mmappedChunk{ref: 2, minTime: 200, maxTime: 250}
	// Disk order can differ from time order after series references are merged.
	c := &mmappedChunk{ref: 3, minTime: 0, maxTime: 50}
	replayed := &mmappedChunk{ref: 4, minTime: 300, maxTime: 350}
	overlap := &mmappedChunk{ref: 5, minTime: 125, maxTime: 225}
	for _, tc := range []struct {
		name                     string
		existing, incoming, want []*mmappedChunk
		lastRef                  chunks.ChunkDiskMapperRef
		ooo                      bool
	}{
		{name: "first definition", incoming: []*mmappedChunk{a, b}, want: []*mmappedChunk{a, b}, lastRef: 3},
		{name: "empty later definition", existing: []*mmappedChunk{a, b}, want: []*mmappedChunk{a, b}, lastRef: 3},
		{name: "earlier chunks arrive later", existing: []*mmappedChunk{b}, incoming: []*mmappedChunk{c, a}, want: []*mmappedChunk{c, a, b}, lastRef: 3},
		{name: "later chunks arrive later", existing: []*mmappedChunk{c, a}, incoming: []*mmappedChunk{b}, want: []*mmappedChunk{c, a, b}, lastRef: 3},
		{name: "repeated definition", existing: []*mmappedChunk{a, b}, incoming: []*mmappedChunk{a, b}, want: []*mmappedChunk{a, b}, lastRef: 3},
		{name: "discard replayed chunks", existing: []*mmappedChunk{c, a, replayed}, incoming: []*mmappedChunk{b}, want: []*mmappedChunk{c, a, b}, lastRef: 3},
		{name: "discard all replayed chunks", existing: []*mmappedChunk{a, b}, want: []*mmappedChunk{}, lastRef: 0},
		{name: "overlapping in-order chunks", existing: []*mmappedChunk{c, a, b}, incoming: []*mmappedChunk{overlap}, want: []*mmappedChunk{c, overlap}, lastRef: 5},
		{name: "OOO sorted by disk reference", existing: []*mmappedChunk{b}, incoming: []*mmappedChunk{a, c}, want: []*mmappedChunk{a, b, c}, lastRef: 3, ooo: true},
		{name: "repeated OOO definition", existing: []*mmappedChunk{a, b}, incoming: []*mmappedChunk{a, b}, want: []*mmappedChunk{a, b}, lastRef: 3, ooo: true},
		{name: "overlapping OOO chunks", existing: []*mmappedChunk{a, b}, incoming: []*mmappedChunk{overlap}, want: []*mmappedChunk{a, b, overlap}, lastRef: 5, ooo: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			existing := append([]*mmappedChunk(nil), tc.existing...)
			incoming := append([]*mmappedChunk(nil), tc.incoming...)
			require.Equal(t, tc.want, mergeReplayMmappedChunks(tc.existing, tc.incoming, tc.lastRef, tc.ooo))
			require.Equal(t, existing, tc.existing, "do not modify the attached chunk list")
			require.Equal(t, incoming, tc.incoming, "do not modify the replay inventory")
		})
	}
}
