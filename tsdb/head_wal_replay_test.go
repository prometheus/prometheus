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
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/compression"
)

func TestWALReplayPreservesMappedChunksAcrossSeriesRefs(t *testing.T) {
	t.Parallel()
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
			t.Run(fmt.Sprintf("%s/%s", tc.name, name), func(t *testing.T) {
				t.Parallel()
				dir := t.TempDir()
				opts := DefaultOptions()
				opts.StripeSize = 16
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
					app := db.AppenderV2(t.Context())
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

// recordingExemplarStorage records the series of the exemplars added to it.
type recordingExemplarStorage struct {
	ExemplarStorage
	added sync.Map
}

func (s *recordingExemplarStorage) AddExemplar(l labels.Labels, e exemplar.Exemplar) error {
	err := s.ExemplarStorage.AddExemplar(l, e)
	if err == nil {
		s.added.Store(l.String(), struct{}{})
	}
	return err
}

// exemplarCheckingSeriesCallback counts the deleted series whose exemplar has not been added yet.
type exemplarCheckingSeriesCallback struct {
	// Keep missing first so that it is 64-bit aligned on 32-bit platforms.
	missing atomic.Int64
	noopSeriesLifecycleCallback
	exemplars *recordingExemplarStorage
}

func (c *exemplarCheckingSeriesCallback) PostDeletion(deleted map[chunks.HeadSeriesRef]labels.Labels) {
	for _, l := range deleted {
		if _, ok := c.exemplars.added.Load(l.String()); !ok {
			c.missing.Inc()
		}
	}
}

// TestHead_WALReplay_ExemplarsBeforeFullRangeTombstone verifies that a series deleted by
// a full-range tombstone during WAL replay is only deleted after the exemplars preceding
// the tombstone have been replayed, so that they are not counted as unknown references.
func TestHead_WALReplay_ExemplarsBeforeFullRangeTombstone(t *testing.T) {
	// The exemplar of the last series is queued behind the others when its tombstone
	// is replayed.
	const numSeries = 1000
	series := make([]record.RefSeries, 0, numSeries)
	samples := make([]record.RefSample, 0, numSeries)
	exemplars := make([]record.RefExemplar, 0, numSeries)
	for i := range numSeries {
		ref := chunks.HeadSeriesRef(i + 1)
		series = append(series, record.RefSeries{Ref: ref, Labels: labels.FromStrings("a", strconv.Itoa(i))})
		samples = append(samples, record.RefSample{Ref: ref, T: 100, V: 1})
		exemplars = append(exemplars, record.RefExemplar{Ref: ref, T: 100, V: 1, Labels: labels.FromStrings("trace_id", strconv.Itoa(i))})
	}
	stones := []tombstones.Stone{{Ref: numSeries, Intervals: tombstones.Intervals{{Mint: math.MinInt64, Maxt: math.MaxInt64}}}}
	entries := []any{series, samples, exemplars, stones}

	cb := &exemplarCheckingSeriesCallback{}
	opts := newTestHeadDefaultOptions(1000, false)
	opts.SeriesCallback = cb
	head, w := newTestHeadWithOptions(t, compression.None, opts)
	defer func() {
		require.NoError(t, head.Close())
	}()
	populateTestWL(t, w, entries, nil, false)

	cb.exemplars = &recordingExemplarStorage{ExemplarStorage: head.exemplars}
	head.exemplars = cb.exemplars

	require.NoError(t, head.Init(math.MinInt64))

	require.Equal(t, uint64(numSeries-1), head.NumSeries())
	require.Zero(t, cb.missing.Load(), "series must not be deleted before their exemplars are replayed")
	require.Zero(t, prom_testutil.ToFloat64(head.metrics.walReplayUnknownRefsTotal.WithLabelValues("exemplars")))

	q, err := head.ExemplarQuerier(context.Background())
	require.NoError(t, err)
	res, err := q.Select(0, 100, []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "a", ".+")})
	require.NoError(t, err)
	require.Len(t, res, numSeries)
	for _, r := range res {
		require.Len(t, r.Exemplars, 1)
		require.Equal(t, r.SeriesLabels.Get("a"), r.Exemplars[0].Labels.Get("trace_id"))
	}
}
