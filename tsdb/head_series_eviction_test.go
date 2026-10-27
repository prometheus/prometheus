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
	"testing"

	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

const (
	evictionTestSeries        = 10
	evictionTestEvictedSeries = 5
	evictionTestSamples       = 3
)

// seriesEvictionTestDB is a DB with evictionTestSeries series. The first evictionTestEvictedSeries
// of them are the ones to evict, and the others remain in the head.
type seriesEvictionTestDB struct {
	db          *DB
	evictedRefs []storage.SeriesRef
}

// newSeriesEvictionTestDB returns a DB where the series to evict have a stale sample as last sample, so that
// they can be evicted by both CompactSelectedSeries and CompactStaleHead.
func newSeriesEvictionTestDB(t *testing.T) *seriesEvictionTestDB {
	t.Helper()

	opts := DefaultOptions()
	opts.MinBlockDuration = 1000
	opts.MaxBlockDuration = 1000
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	res := &seriesEvictionTestDB{db: db}
	app := db.Appender(context.Background())
	for i := range evictionTestSeries {
		evicted := i < evictionTestEvictedSeries

		var ref storage.SeriesRef
		for s := range evictionTestSamples {
			v := float64(i)
			if evicted && s == evictionTestSamples-1 {
				v = math.Float64frombits(value.StaleNaN)
			}
			var err error
			ref, err = app.Append(0, evictionTestLabels(i), int64(100*(s+1)), v)
			require.NoError(t, err)
		}
		if evicted {
			res.evictedRefs = append(res.evictedRefs, ref)
		}
	}
	require.NoError(t, app.Commit())
	require.Equal(t, uint64(evictionTestSeries), db.Head().NumSeries())
	return res
}

func evictionTestLabels(i int) labels.Labels {
	return labels.FromStrings("__name__", "series", "id", fmt.Sprintf("%02d", i))
}

// evictionTestExpectedSamples returns the number of samples of each series. The series that received a late sample
// have one more.
func evictionTestExpectedSamples(lateSeries ...int) map[string]int {
	res := map[string]int{}
	for i := range evictionTestSeries {
		res[evictionTestLabels(i).String()] = evictionTestSamples
	}
	for _, i := range lateSeries {
		res[evictionTestLabels(i).String()]++
	}
	return res
}

// evictionTestEvictions returns the ways of evicting the series to evict.
func evictionTestEvictions() map[string]func(*DB, []storage.SeriesRef) error {
	return map[string]func(*DB, []storage.SeriesRef) error{
		"CompactSelectedSeries": func(db *DB, refs []storage.SeriesRef) error { return db.CompactSelectedSeries(refs) },
		"CompactStaleHead":      func(db *DB, _ []storage.SeriesRef) error { return db.CompactStaleHead() },
	}
}

// readingSeries is a query whose series have been selected, but whose samples have not been read yet, like
// the callers that stream the series to their clients, and then their chunks.
type readingSeries struct {
	close func() error
	// read reads the samples of each selected series, and returns their number by series, or the first error.
	read func() (map[string]int, error)
}

// newReadingSeries returns a readingSeries that is closed when the test ends, if it hasn't been closed. A query
// that is left open blocks closing the DB, which would make a failing test hang instead of failing.
func newReadingSeries(t *testing.T, closeQuery func() error, read func() (map[string]int, error)) *readingSeries {
	closed := false
	r := &readingSeries{read: read, close: func() error {
		if closed {
			return nil
		}
		closed = true
		return closeQuery()
	}}
	t.Cleanup(func() { require.NoError(t, r.close()) })
	return r
}

func (r *readingSeries) mustRead(t *testing.T) map[string]int {
	t.Helper()

	res, err := r.read()
	require.NoError(t, err)
	return res
}

var evictionTestMatcher = labels.MustNewMatcher(labels.MatchEqual, "__name__", "series")

func selectSamples(t *testing.T, q storage.Querier) *readingSeries {
	t.Helper()

	var selected []storage.Series
	ss := q.Select(context.Background(), true, nil, evictionTestMatcher)
	for ss.Next() {
		selected = append(selected, ss.At())
	}
	require.NoError(t, ss.Err())

	return newReadingSeries(t, q.Close, func() (map[string]int, error) {
		res := map[string]int{}
		var it chunkenc.Iterator
		for _, s := range selected {
			it = s.Iterator(it)
			for it.Next() != chunkenc.ValNone {
				res[s.Labels().String()]++
			}
			if err := it.Err(); err != nil {
				return nil, err
			}
		}
		return res, nil
	})
}

func selectChunks(t *testing.T, q storage.ChunkQuerier) *readingSeries {
	t.Helper()

	var selected []storage.ChunkSeries
	ss := q.Select(context.Background(), true, nil, evictionTestMatcher)
	for ss.Next() {
		selected = append(selected, ss.At())
	}
	require.NoError(t, ss.Err())

	return newReadingSeries(t, q.Close, func() (map[string]int, error) {
		res := map[string]int{}
		for _, s := range selected {
			it := s.Iterator(nil)
			for it.Next() {
				res[s.Labels().String()] += it.At().Chunk.NumSamples()
			}
			if err := it.Err(); err != nil {
				return nil, err
			}
		}
		return res, nil
	})
}

func openSampleQuerier(t *testing.T, db *DB, mint, maxt int64) *readingSeries {
	t.Helper()

	q, err := db.Querier(mint, maxt)
	require.NoError(t, err)
	return selectSamples(t, q)
}

func openChunkQuerier(t *testing.T, db *DB, mint, maxt int64) *readingSeries {
	t.Helper()

	q, err := db.ChunkQuerier(mint, maxt)
	require.NoError(t, err)
	return selectChunks(t, q)
}

type evictionTestCase struct {
	name      string
	evict     func(*DB, []storage.SeriesRef) error
	openQuery func(*testing.T, *DB, int64, int64) *readingSeries
	mint      int64
	maxt      int64
}

// newEvictionTestCase returns the test case for a way of evicting series ("CompactSelectedSeries" or
// "CompactStaleHead"), a way of querying ("Querier" or "ChunkQuerier"), and a time range ("bounded" or "unbounded").
func newEvictionTestCase(eviction, querier, timeRange string) evictionTestCase {
	queriers := map[string]func(*testing.T, *DB, int64, int64) *readingSeries{
		"Querier":      openSampleQuerier,
		"ChunkQuerier": openChunkQuerier,
	}
	timeRanges := map[string][2]int64{
		"bounded": {0, 1000},
		// A time range that includes math.MinInt64 overlaps the empty out-of-order range of the head,
		// and then the head is read through the head and out-of-order querier.
		"unbounded": {math.MinInt64, math.MaxInt64},
	}

	tr := timeRanges[timeRange]
	return evictionTestCase{
		name:      fmt.Sprintf("%s/%s/%s", eviction, querier, timeRange),
		evict:     evictionTestEvictions()[eviction],
		openQuery: queriers[querier],
		mint:      tr[0],
		maxt:      tr[1],
	}
}

// TestSeriesEviction_QueryOpenedAfterWaitingForReaders verifies that a query that is opened after truncateSeries
// has waited for the open readers, and before it removes the series from the head, reads all the samples of the
// series, even though they are removed while it is open. Such a query is not waited for.
//
// A series that is selected, but receives a sample before it is evicted, is not evicted. The query must read its
// late sample from the head.
func TestSeriesEviction_QueryOpenedAfterWaitingForReaders(t *testing.T) {
	tests := map[string]struct {
		lateSeries []int
		cases      []evictionTestCase
	}{
		"series to evict": {
			cases: []evictionTestCase{
				newEvictionTestCase("CompactSelectedSeries", "Querier", "bounded"),
				newEvictionTestCase("CompactSelectedSeries", "Querier", "unbounded"),
				newEvictionTestCase("CompactSelectedSeries", "ChunkQuerier", "bounded"),
				newEvictionTestCase("CompactSelectedSeries", "ChunkQuerier", "unbounded"),
				newEvictionTestCase("CompactStaleHead", "ChunkQuerier", "unbounded"),
			},
		},
		"series with a late sample": {
			lateSeries: []int{0},
			cases: []evictionTestCase{
				newEvictionTestCase("CompactSelectedSeries", "ChunkQuerier", "bounded"),
				newEvictionTestCase("CompactSelectedSeries", "Querier", "unbounded"),
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			for _, c := range tt.cases {
				t.Run(c.name, func(t *testing.T) {
					testDB := newSeriesEvictionTestDB(t)
					db := testDB.db

					// The late samples are appended after the series are selected, and before they are evicted.
					db.compactHeadViewBeforeEvictTestingCallback = func() {
						app := db.Appender(context.Background())
						for _, i := range tt.lateSeries {
							_, err := app.Append(0, evictionTestLabels(i), 400, 42)
							require.NoError(t, err)
						}
						require.NoError(t, app.Commit())
					}

					var (
						reading             *readingSeries
						numSeriesInCallback uint64
					)
					db.Head().testAfterWaitingForReadersToEvictSeries = func() {
						numSeriesInCallback = db.Head().NumSeries()

						// Open the query now. No one waits for it, and the series are removed after this function returns.
						reading = c.openQuery(t, db, c.mint, c.maxt)
					}

					require.NoError(t, c.evict(db, testDB.evictedRefs))

					removed := evictionTestEvictedSeries - len(tt.lateSeries)
					require.Equal(t, uint64(evictionTestSeries), numSeriesInCallback, "the series must be removed after the callback")
					require.Equal(t, uint64(evictionTestSeries-removed), db.Head().NumSeries())

					// The samples of the evicted series are read after the series were removed from the head,
					// and must be served from the block.
					require.NotNil(t, reading)
					require.Equal(t, evictionTestExpectedSamples(tt.lateSeries...), reading.mustRead(t))
					require.NoError(t, reading.close())

					// Each evicted series has one chunk, whose read was skipped.
					require.Equal(t, float64(removed), prom_testutil.ToFloat64(db.Head().metrics.chunkReadsOfRemovedSeries))
				})
			}
		})
	}
}

// TestSeriesEviction_OnlyQueriesSkipRemovedSeries verifies that only the readers used by queries skip the
// chunks of the series that were removed from the head. The others, like those that write blocks, must fail,
// instead of silently skipping samples.
func TestSeriesEviction_OnlyQueriesSkipRemovedSeries(t *testing.T) {
	testDB := newSeriesEvictionTestDB(t)
	head := testDB.db.Head()

	// Get the meta of a chunk of a series to evict, and evict the series.
	ir, err := NewRangeHead(head, 0, 1000).Index()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ir.Close()) })
	var (
		builder labels.ScratchBuilder
		metas   []chunks.Meta
	)
	require.NoError(t, ir.Series(testDB.evictedRefs[0], &builder, &metas))
	require.Len(t, metas, 1)
	meta := metas[0]
	require.NoError(t, testDB.db.CompactSelectedSeries(testDB.evictedRefs))

	type chunkReader interface {
		ChunkOrIterable(chunks.Meta) (chunkenc.Chunk, chunkenc.Iterable, error)
		ChunkOrIterableWithCopy(chunks.Meta) (chunkenc.Chunk, chunkenc.Iterable, int64, error)
	}
	evicting := &evictedSeries{refs: testDB.evictedRefs}
	newOOOReader := func(inOrder *headChunkReader) chunkReader {
		return NewHeadAndOOOChunkReader(head, 0, 1000, inOrder, nil, 0)
	}

	tests := map[string]struct {
		reader chunkReader
		skips  bool
	}{
		"reader of a query": {
			reader: &headChunkReader{head: head, mint: 0, maxt: 1000, evicting: evicting},
			skips:  true,
		},
		"reader of a range head that is not used by queries": {
			reader: &headChunkReader{head: head, mint: 0, maxt: 1000},
		},
		"head and out-of-order reader of a query": {
			reader: newOOOReader(&headChunkReader{head: head, mint: 0, maxt: 1000, evicting: evicting}),
			skips:  true,
		},
		"head and out-of-order reader that is not used by queries": {
			reader: newOOOReader(&headChunkReader{head: head, mint: 0, maxt: 1000}),
		},
		"out-of-order reader used to write a block": {
			reader: newOOOReader(nil),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			readsBefore := prom_testutil.ToFloat64(head.metrics.chunkReadsOfRemovedSeries)

			type result struct {
				chunk    chunkenc.Chunk
				iterable chunkenc.Iterable
				err      error
			}
			var results []result
			chk, iterable, err := tc.reader.ChunkOrIterable(meta)
			results = append(results, result{chk, iterable, err})
			chk, iterable, _, err = tc.reader.ChunkOrIterableWithCopy(meta)
			results = append(results, result{chk, iterable, err})

			for _, r := range results {
				if !tc.skips {
					require.ErrorIs(t, r.err, storage.ErrNotFound)
					continue
				}
				require.NoError(t, r.err)
				require.Nil(t, r.chunk)
				require.NotNil(t, r.iterable)
				require.Equal(t, chunkenc.ValNone, r.iterable.Iterator(nil).Next())
			}

			expectedReads := readsBefore // The reads that failed are not counted.
			if tc.skips {
				expectedReads += 2
			}
			require.Equal(t, expectedReads, prom_testutil.ToFloat64(head.metrics.chunkReadsOfRemovedSeries))
		})
	}
}

// TestSeriesEviction_OtherNotFoundErrorsAreReturned verifies that the readers of queries only skip the chunks of the
// series that were removed from the head. If the series is in the head, and its chunk can't be found, the error is
// returned, because the samples of the series are not known to be available from a block.
func TestSeriesEviction_OtherNotFoundErrorsAreReturned(t *testing.T) {
	testDB := newSeriesEvictionTestDB(t)
	head := testDB.db.Head()

	// The series that follows the series to evict is in the head, and doesn't have the chunk 1000.
	keptSeries := testDB.evictedRefs[len(testDB.evictedRefs)-1] + 1
	missingChunk := chunks.Meta{Ref: chunks.ChunkRef(chunks.NewHeadChunkRef(chunks.HeadSeriesRef(keptSeries), 1000))}

	reader := &headChunkReader{head: head, mint: 0, maxt: 1000, evicting: &evictedSeries{refs: testDB.evictedRefs}}
	_, _, err := reader.ChunkOrIterable(missingChunk)
	require.ErrorIs(t, err, storage.ErrNotFound)
	_, _, _, err = reader.ChunkOrIterableWithCopy(missingChunk)
	require.ErrorIs(t, err, storage.ErrNotFound)

	_, _, err = NewHeadAndOOOChunkReader(head, 0, 1000, reader, nil, 0).ChunkOrIterable(missingChunk)
	require.ErrorIs(t, err, storage.ErrNotFound)

	require.Equal(t, float64(0), prom_testutil.ToFloat64(head.metrics.chunkReadsOfRemovedSeries))
}

// TestSeriesEviction_SeriesNotBeingEvictedAreNotSkipped verifies that a reader fails to read the chunk of a series
// that is not in the set being evicted, even if the series is not in the head either, and when nothing is being
// evicted.
func TestSeriesEviction_SeriesNotBeingEvictedAreNotSkipped(t *testing.T) {
	testDB := newSeriesEvictionTestDB(t)
	head := testDB.db.Head()

	ir, err := NewRangeHead(head, 0, 1000).Index()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ir.Close()) })
	var (
		builder labels.ScratchBuilder
		metas   []chunks.Meta
	)
	require.NoError(t, ir.Series(testDB.evictedRefs[0], &builder, &metas))
	require.Len(t, metas, 1)
	meta := metas[0]
	require.NoError(t, testDB.db.CompactSelectedSeries(testDB.evictedRefs))

	tests := map[string]*evictedSeries{
		"nothing is being evicted":                   nil,
		"the series is not in the set being evicted": {refs: testDB.evictedRefs[1:]},
	}
	for name, evicting := range tests {
		t.Run(name, func(t *testing.T) {
			reader := &headChunkReader{head: head, mint: 0, maxt: 1000, evicting: evicting}
			_, _, err := reader.ChunkOrIterable(meta)
			require.ErrorIs(t, err, storage.ErrNotFound)
			_, _, _, err = reader.ChunkOrIterableWithCopy(meta)
			require.ErrorIs(t, err, storage.ErrNotFound)
			require.Equal(t, float64(0), prom_testutil.ToFloat64(head.metrics.chunkReadsOfRemovedSeries))
		})
	}
}

// TestSeriesEviction_SeriesAreOnlyPublishedWhileTheyAreEvicted verifies that the series being evicted are published
// only while truncateSeries runs, so that the queries opened at any other time behave as they always did.
func TestSeriesEviction_SeriesAreOnlyPublishedWhileTheyAreEvicted(t *testing.T) {
	testDB := newSeriesEvictionTestDB(t)
	head := testDB.db.Head()

	require.Nil(t, head.evictingSeries.Load())
	var published *evictedSeries
	head.testAfterWaitingForReadersToEvictSeries = func() { published = head.evictingSeries.Load() }

	require.NoError(t, testDB.db.CompactSelectedSeries(testDB.evictedRefs))

	require.NotNil(t, published)
	require.Equal(t, testDB.evictedRefs, published.refs)
	require.Nil(t, head.evictingSeries.Load())

	// A querier opened after the eviction doesn't skip anything.
	q, err := testDB.db.Querier(0, 1000)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, q.Close()) })
}

// beforeRemoval is what the DB looks like right before the series are removed from the head.
type beforeRemoval struct {
	numSeries uint64
	// inBlocks is the number of samples that the persisted blocks have for each series.
	inBlocks map[string]int
}

func (b *beforeRemoval) record(t *testing.T, db *DB) {
	t.Helper()

	b.numSeries = db.Head().NumSeries()
	b.inBlocks = map[string]int{}
	for _, block := range db.Blocks() {
		q, err := NewBlockQuerier(block, math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		samples, err := selectSamples(t, q).read()
		require.NoError(t, err)
		for lbls, n := range samples {
			b.inBlocks[lbls] += n
		}
	}
}

// require checks that no series had been removed yet, and that the blocks had all the samples of the series to remove.
func (b *beforeRemoval) require(t *testing.T) {
	t.Helper()

	require.Equal(t, uint64(evictionTestSeries), b.numSeries, "no series should have been removed yet")
	for i := range evictionTestEvictedSeries {
		require.Equal(t, evictionTestSamples, b.inBlocks[evictionTestLabels(i).String()], "the samples of series %d must be in the blocks", i)
	}
}

// TestSeriesRemoval_BlockIsAddedBeforeSeriesAreRemoved verifies that, in every way series are evicted from the head
// by truncateSeries, the samples of the series are already in a block of the DB when the series are removed.
//
// The readers of queries skip the chunks of a series that is being evicted, because they read the block too.
// That is correct only if the block is added to the DB before the series are removed. See
// headChunkReader.tolerateRemovedSeries. This test fails if that order is changed.
func TestSeriesRemoval_BlockIsAddedBeforeSeriesAreRemoved(t *testing.T) {
	for name, evict := range evictionTestEvictions() {
		t.Run(name, func(t *testing.T) {
			testDB := newSeriesEvictionTestDB(t)
			db := testDB.db

			var before beforeRemoval
			db.Head().testAfterWaitingForReadersToEvictSeries = func() { before.record(t, db) }

			require.NoError(t, evict(db, testDB.evictedRefs))

			before.require(t)
			require.Equal(t, uint64(evictionTestSeries-evictionTestEvictedSeries), db.Head().NumSeries(), "the series should have been removed")
		})
	}
}

// TestSeriesEviction_ExportedHeadAndOOOQueriersDoNotSkipRemovedSeries verifies that the exported constructors of the
// head and out-of-order queriers still fail to read the chunks of a series that was removed from the head, as they
// always did.
func TestSeriesEviction_ExportedHeadAndOOOQueriersDoNotSkipRemovedSeries(t *testing.T) {
	const mint, maxt = 0, 1000

	tests := map[string]func(t *testing.T, db *DB, isoState *oooIsolationState) *readingSeries{
		"NewHeadAndOOOQuerier": func(t *testing.T, db *DB, iso *oooIsolationState) *readingSeries {
			return selectSamples(t, NewHeadAndOOOQuerier(0, mint, maxt, db.head, iso, nil))
		},
		"NewHeadAndOOOChunkQuerier": func(t *testing.T, db *DB, iso *oooIsolationState) *readingSeries {
			return selectChunks(t, NewHeadAndOOOChunkQuerier(0, mint, maxt, db.head, iso, nil))
		},
	}

	for name, open := range tests {
		t.Run(name, func(t *testing.T) {
			testDB := newSeriesEvictionTestDB(t)
			db := testDB.db

			var reading *readingSeries
			db.Head().testAfterWaitingForReadersToEvictSeries = func() {
				// Open the querier at the point where it is not waited for.
				reading = open(t, db, db.head.oooIso.TrackReadAfter(db.lastGarbageCollectedMmapRef))
			}

			require.NoError(t, db.CompactSelectedSeries(testDB.evictedRefs))
			require.NotNil(t, reading)

			_, err := reading.read()
			require.ErrorIs(t, err, storage.ErrNotFound)
			require.Equal(t, float64(0), prom_testutil.ToFloat64(db.Head().metrics.chunkReadsOfRemovedSeries))
		})
	}
}
