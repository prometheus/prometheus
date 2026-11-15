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
	"math/rand"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/compression"
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
// The specified exemplarSeries also receive an exemplar.
func newSeriesEvictionTestDB(t *testing.T, exemplarSeries ...int) *seriesEvictionTestDB {
	t.Helper()

	opts := DefaultOptions()
	opts.MinBlockDuration = 1000
	opts.MaxBlockDuration = 1000
	opts.EnableExemplarStorage = len(exemplarSeries) > 0
	opts.MaxExemplars = int64(len(exemplarSeries))
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
		if slices.Contains(exemplarSeries, i) {
			_, err := app.AppendExemplar(ref, evictionTestLabels(i), exemplar.Exemplar{
				Labels: labels.FromStrings("trace_id", strconv.Itoa(i)), Value: 1, Ts: 200,
			})
			require.NoError(t, err)
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

// requireEvictingSeriesSorted checks, from a hook called by truncateSeries, that the series being evicted are
// published sorted, because evictedSeries.contains does a binary search on them.
func requireEvictingSeriesSorted(t *testing.T, head *Head) {
	t.Helper()

	evicting := head.evictingSeries.Load()
	require.NotNil(t, evicting)
	require.True(t, slices.IsSorted(evicting.refs), "evictedSeries.contains needs the refs to be sorted")
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
	var exemplarCases []evictionTestCase
	for eviction := range evictionTestEvictions() {
		for _, querier := range []string{"Querier", "ChunkQuerier"} {
			for _, timeRange := range []string{"bounded", "unbounded"} {
				exemplarCases = append(exemplarCases, newEvictionTestCase(eviction, querier, timeRange))
			}
		}
	}
	tests := map[string]struct {
		lateSeries          []int
		exemplarSeries      []int
		appendAfterEviction bool
		wrapChunkIDs        bool
		cases               []evictionTestCase
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
		"series with exemplars": {
			exemplarSeries: []int{0},
			cases:          exemplarCases,
		},
		"series with exemplars and a late sample": {
			exemplarSeries: []int{0, 1},
			lateSeries:     []int{0},
			cases:          exemplarCases,
		},
		"series with exemplars and new samples after eviction": {
			exemplarSeries:      []int{0},
			appendAfterEviction: true,
			cases:               exemplarCases,
		},
		"series with exemplars and wrapped chunk IDs": {
			exemplarSeries: []int{0},
			wrapChunkIDs:   true,
			cases:          exemplarCases,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			for _, c := range tt.cases {
				t.Run(c.name, func(t *testing.T) {
					testDB := newSeriesEvictionTestDB(t, tt.exemplarSeries...)
					db := testDB.db
					if len(tt.exemplarSeries) > 0 {
						db.Head().SetMinValidTime(200)
					}
					if tt.wrapChunkIDs {
						// A sample in the next chunk range gives the series a second chunk. It is a stale marker, to
						// keep the series evictable by CompactStaleHead.
						app := db.Appender(context.Background())
						_, err := app.Append(testDB.evictedRefs[0], evictionTestLabels(0), 1000, math.Float64frombits(value.StaleNaN))
						require.NoError(t, err)
						require.NoError(t, app.Commit())

						// The first chunk has the last chunk ID, so the ID of the second chunk wraps to 0, which is
						// lower than the ID of the first chunk.
						s := db.Head().series.getByID(chunks.HeadSeriesRef(testDB.evictedRefs[0]))
						s.Lock()
						require.Equal(t, 2, int(s.headChunkCount.Load()), "the series must have two chunks")
						s.firstChunkID = oooChunkIDMask - 1
						s.Unlock()
					}

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
						requireEvictingSeriesSorted(t, db.Head())
						numSeriesInCallback = db.Head().NumSeries()

						// Open the query now. No one waits for it, and the series are removed after this function returns.
						reading = c.openQuery(t, db, c.mint, c.maxt)
					}

					require.NoError(t, c.evict(db, testDB.evictedRefs))

					removed := evictionTestEvictedSeries - len(tt.lateSeries)
					for _, i := range tt.exemplarSeries {
						s := db.Head().series.getByID(chunks.HeadSeriesRef(testDB.evictedRefs[i]))
						require.NotNil(t, s, "the exemplar keeps the series identity")
						if !slices.Contains(tt.lateSeries, i) {
							removed--
							require.Zero(t, s.headChunkCount.Load())
							require.Empty(t, s.mmappedChunks)
						}
					}
					require.Equal(t, uint64(evictionTestSeries), numSeriesInCallback, "the series must be removed after the callback")
					require.Equal(t, uint64(evictionTestSeries-removed), db.Head().NumSeries())
					if tt.appendAfterEviction {
						app := db.Appender(context.Background())
						_, err := app.Append(testDB.evictedRefs[0], evictionTestLabels(0), 400, 42)
						require.NoError(t, err)
						require.NoError(t, app.Commit())
					}

					// The samples of the evicted series are read after the series were removed from the head,
					// and must be served from the block.
					require.NotNil(t, reading)
					expectedSamples := evictionTestExpectedSamples(tt.lateSeries...)
					expectedSkippedChunks := evictionTestEvictedSeries - len(tt.lateSeries)
					if tt.wrapChunkIDs {
						expectedSamples[evictionTestLabels(0).String()]++ // The sample of the second chunk.
						expectedSkippedChunks++
					}
					require.Equal(t, expectedSamples, reading.mustRead(t))
					require.NoError(t, reading.close())

					// Each persisted series has one chunk, whose read was skipped unless a late sample kept it.
					require.Equal(t, float64(expectedSkippedChunks), prom_testutil.ToFloat64(db.Head().metrics.chunkReadsOfRemovedSeries))
					if tt.appendAfterEviction {
						current := c.openQuery(t, db, c.mint, c.maxt)
						require.Equal(t, evictionTestExpectedSamples(0), current.mustRead(t))
						require.NoError(t, current.close())
					}
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

// TestSeriesEviction_OtherNotFoundErrorsAreReturned verifies that query readers do not skip missing chunks
// unless the series or the particular chunk was removed by the eviction.
func TestSeriesEviction_OtherNotFoundErrorsAreReturned(t *testing.T) {
	for _, truncate := range []bool{false, true} {
		t.Run(fmt.Sprintf("chunks reclaimed=%t", truncate), func(t *testing.T) {
			testDB := newSeriesEvictionTestDB(t, 0)
			head := testDB.db.Head()
			evicting := &evictedSeries{refs: testDB.evictedRefs}
			if truncate {
				head.testAfterWaitingForReadersToEvictSeries = func() { evicting = head.evictingSeries.Load() }
				require.NoError(t, testDB.db.CompactSelectedSeries(testDB.evictedRefs))
			}

			// Both series remain in the head, and neither ever had chunk 1000.
			tests := map[string]storage.SeriesRef{
				"a series being evicted":     testDB.evictedRefs[0],
				"a series not being evicted": testDB.evictedRefs[len(testDB.evictedRefs)-1] + 1,
			}
			for name, ref := range tests {
				t.Run(name, func(t *testing.T) {
					require.NotNil(t, head.series.getByID(chunks.HeadSeriesRef(ref)), "the series must be in the head")
					missingChunk := chunks.Meta{Ref: chunks.ChunkRef(chunks.NewHeadChunkRef(chunks.HeadSeriesRef(ref), 1000))}

					reader := &headChunkReader{head: head, mint: 0, maxt: 1000, evicting: evicting}
					_, _, err := reader.ChunkOrIterable(missingChunk)
					require.ErrorIs(t, err, storage.ErrNotFound)
					_, _, _, err = reader.ChunkOrIterableWithCopy(missingChunk)
					require.ErrorIs(t, err, storage.ErrNotFound)

					oooReader := NewHeadAndOOOChunkReader(head, 0, 1000, reader, nil, 0)
					_, _, err = oooReader.ChunkOrIterable(missingChunk)
					require.ErrorIs(t, err, storage.ErrNotFound)
					_, _, _, err = oooReader.ChunkOrIterableWithCopy(missingChunk)
					require.ErrorIs(t, err, storage.ErrNotFound)

					require.Equal(t, float64(0), prom_testutil.ToFloat64(head.metrics.chunkReadsOfRemovedSeries))
				})
			}
		})
	}
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
//
// The published refs must be sorted, because evictedSeries.contains does a binary search on them. Refs that are not
// sorted can't reach truncateSeries: the block writer rejects them, and the eviction fails before it.
func TestSeriesEviction_SeriesAreOnlyPublishedWhileTheyAreEvicted(t *testing.T) {
	for name, evict := range evictionTestEvictions() {
		t.Run(name, func(t *testing.T) {
			testDB := newSeriesEvictionTestDB(t)
			head := testDB.db.Head()

			require.Nil(t, head.evictingSeries.Load())
			var published *evictedSeries
			head.testAfterWaitingForReadersToEvictSeries = func() { published = head.evictingSeries.Load() }

			require.NoError(t, evict(testDB.db, testDB.evictedRefs))

			require.NotNil(t, published)
			require.Equal(t, testDB.evictedRefs, published.refs)
			require.True(t, slices.IsSorted(published.refs))
			// A reader opened after the eviction loads nothing from here, so it doesn't skip anything.
			require.Nil(t, head.evictingSeries.Load())
		})
	}
}

type seriesEvictionPostDeletionCallback struct {
	noopSeriesLifecycleCallback
	preCreation  func(labels.Labels) error
	postDeletion func(map[chunks.HeadSeriesRef]labels.Labels)
}

func (c seriesEvictionPostDeletionCallback) PreCreation(lset labels.Labels) error {
	if c.preCreation == nil {
		return nil
	}
	return c.preCreation(lset)
}

func (c seriesEvictionPostDeletionCallback) PostDeletion(deleted map[chunks.HeadSeriesRef]labels.Labels) {
	c.postDeletion(deleted)
}

// TestSeriesEviction_RecreationIsDeclaredAfterTombstone verifies that a series with the labels of an evicted series,
// which is created before the tombstone of the latter is written, waits for the tombstone. Its declaration then
// follows the tombstone in the WAL, so replay starts it from scratch instead of attaching it to the evicted series.
func TestSeriesEviction_RecreationIsDeclaredAfterTombstone(t *testing.T) {
	for name, evict := range evictionTestEvictions() {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				testDB := newSeriesEvictionTestDB(t)
				db := testDB.db
				oldRef := testDB.evictedRefs[0]
				lset := evictionTestLabels(0)
				q, err := db.Querier(math.MinInt64, math.MaxInt64)
				require.NoError(t, err)
				expected := query(t, q, evictionTestMatcher)
				expected[lset.String()] = append(expected[lset.String()], sample{t: 400, f: 42}, sample{t: 500, f: 43})

				var newRef storage.SeriesRef
				recreated := make(chan error, 1)
				db.head.series.seriesLifecycleCallback = seriesEvictionPostDeletionCallback{
					postDeletion: func(deleted map[chunks.HeadSeriesRef]labels.Labels) {
						if _, ok := deleted[chunks.HeadSeriesRef(oldRef)]; !ok {
							return
						}
						// The old series is unlinked, but its tombstone is not written. A series with its labels waits
						// for the tombstone, so it can't be created from the goroutine that writes it.
						go func() {
							app := db.Appender(t.Context())
							var err error
							newRef, err = app.Append(oldRef, lset, 400, 42)
							if err == nil {
								err = app.Commit()
							}
							recreated <- err
						}()
						// Wait until the creator is blocked on the pending tombstone, so that the eviction can't write it
						// before the creator looks for it.
						synctest.Wait()
						require.Empty(t, recreated, "the creator must wait for the tombstone")
					},
				}

				require.NoError(t, evict(db, testDB.evictedRefs))
				require.NoError(t, <-recreated)
				require.NotEqual(t, oldRef, newRef)

				app := db.Appender(t.Context())
				laterRef, err := app.Append(newRef, lset, 500, 43)
				require.NoError(t, err)
				require.Equal(t, newRef, laterRef)
				require.NoError(t, app.Commit())

				q, err = db.Querier(math.MinInt64, math.MaxInt64)
				require.NoError(t, err)
				require.Equal(t, expected, query(t, q, evictionTestMatcher))
				require.NoError(t, db.Close())

				stonePosition, seriesPosition := -1, -1
				for i, entry := range readTestWAL(t, filepath.Join(db.Dir(), "wal")) {
					switch records := entry.(type) {
					case []tombstones.Stone:
						for _, stone := range records {
							if stone.Ref == oldRef {
								stonePosition = i
							}
						}
					case []record.RefSeries:
						for _, series := range records {
							if series.Ref == chunks.HeadSeriesRef(newRef) {
								seriesPosition = i
							}
						}
					}
				}
				require.NotEqual(t, -1, stonePosition)
				require.NotEqual(t, -1, seriesPosition)
				require.Greater(t, seriesPosition, stonePosition, "the declaration of the new series must follow the tombstone of the old one")

				reopened, err := Open(db.Dir(), nil, nil, db.opts, nil)
				require.NoError(t, err)
				reopened.DisableCompactions()
				t.Cleanup(func() { require.NoError(t, reopened.Close()) })
				q, err = reopened.Querier(math.MinInt64, math.MaxInt64)
				require.NoError(t, err)
				require.Equal(t, expected, query(t, q, evictionTestMatcher))
				require.Zero(t, prom_testutil.ToFloat64(reopened.head.metrics.walReplayUnknownRefsTotal.WithLabelValues("samples")))

				// The head alone, without the blocks, has only the new series, with the samples written after the eviction.
				// The old one is not restored from its samples in the WAL, because its tombstone removes it.
				require.Nil(t, reopened.head.series.getByID(chunks.HeadSeriesRef(oldRef)))
				newSeries := reopened.head.series.getByID(chunks.HeadSeriesRef(newRef))
				require.NotNil(t, newSeries)
				require.True(t, labels.Equal(lset, newSeries.labels()))
				headQuerier, err := NewBlockQuerier(reopened.Head(), math.MinInt64, math.MaxInt64)
				require.NoError(t, err)
				require.Equal(t, []chunks.Sample{sample{t: 400, f: 42}, sample{t: 500, f: 43}}, query(t, headQuerier, evictionTestMatcher)[lset.String()])
			})
		})
	}
}

// TestHead_GCSeriesDoesNotBlockRecreation verifies that calling gcSeries directly writes the tombstones and releases
// pendingTombstones, and that calling stripeSeries.gcSeries without reserve does not mark hashes as pending.
func TestHead_GCSeriesDoesNotBlockRecreation(t *testing.T) {
	testDB := newSeriesEvictionTestDB(t)
	db := testDB.db
	h := db.Head()
	ref0, ref1 := testDB.evictedRefs[0], testDB.evictedRefs[1]
	lset0, lset1 := evictionTestLabels(0), evictionTestLabels(1)

	deleted, err := h.gcSeries([]storage.SeriesRef{ref0}, math.MaxInt64, func(*memSeries) bool { return true })
	require.NoError(t, err)
	require.Contains(t, deleted, ref0)
	require.Nil(t, h.series.pendingTombstones.pending(lset0.Hash()))

	deleted, _, _, _, _, _ = h.series.gcSeries([]storage.SeriesRef{ref1}, math.MaxInt64, func(*memSeries) bool { return true })
	require.Contains(t, deleted, ref1)
	require.Nil(t, h.series.pendingTombstones.pending(lset1.Hash()))

	recreated := make(chan error, 1)
	go func() {
		app := db.Appender(t.Context())
		_, err := app.Append(0, lset0, 400, 1)
		if err == nil {
			_, err = app.Append(0, lset1, 400, 1)
		}
		if err == nil {
			err = app.Commit()
		}
		recreated <- err
	}()
	select {
	case err := <-recreated:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		require.FailNow(t, "the creation of a series with the labels of a removed series is blocked")
	}
}

// TestSeriesEviction_CreationPausedBeforeInsertionIsDeclaredAfterTombstone verifies that a series that was not found,
// and whose creation is delayed until after a series with the same labels is created and evicted, is not created
// before the tombstone of the latter is written. The check for a pending tombstone is atomic with the insertion, so
// the creation can't pass it before the eviction and insert the series after.
func TestSeriesEviction_CreationPausedBeforeInsertionIsDeclaredAfterTombstone(t *testing.T) {
	for name, evict := range evictionTestEvictions() {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				testDB := newSeriesEvictionTestDB(t)
				db := testDB.db
				ls := evictionTestLabels(20)

				creatorPaused := make(chan struct{})
				resumeCreator := make(chan struct{})
				unlinked := make(chan struct{})
				finishEviction := make(chan struct{})
				var resumeOnce, finishOnce sync.Once
				resume := func() { resumeOnce.Do(func() { close(resumeCreator) }) }
				finish := func() { finishOnce.Do(func() { close(finishEviction) }) }
				var firstCreation atomic.Bool
				var oldRef storage.SeriesRef
				db.head.series.seriesLifecycleCallback = seriesEvictionPostDeletionCallback{
					preCreation: func(lset labels.Labels) error {
						if labels.Equal(ls, lset) && firstCreation.CompareAndSwap(false, true) {
							// The creator did not find the series. Suspend it before it inserts the series.
							close(creatorPaused)
							<-resumeCreator
						}
						return nil
					},
					postDeletion: func(deleted map[chunks.HeadSeriesRef]labels.Labels) {
						if _, ok := deleted[chunks.HeadSeriesRef(oldRef)]; ok {
							// The old series is unlinked, and its tombstone is not written.
							close(unlinked)
							<-finishEviction
						}
					},
				}

				// Allocate the append ID of this transaction before the one of the delayed creator, so that it can be
				// committed below the isolation watermark while the creator is still open.
				oldApp := db.Appender(context.Background())
				type appendResult struct {
					ref storage.SeriesRef
					err error
				}
				created := make(chan appendResult, 1)
				creatorDone := make(chan struct{})
				go func() {
					defer close(creatorDone)
					app := db.Appender(context.Background())
					ref, err := app.Append(0, ls, 400, 42)
					if err == nil {
						err = app.Commit()
					} else {
						_ = app.Rollback()
					}
					created <- appendResult{ref, err}
				}()
				t.Cleanup(func() {
					resume()
					finish()
					select {
					case <-creatorDone:
					case <-time.After(10 * time.Second):
						t.Error("the creator did not finish")
					}
				})
				select {
				case <-creatorPaused:
				case <-time.After(10 * time.Second):
					t.Fatal("the creator did not reach the pause before the insertion")
				}

				for i := range 3 {
					v := float64(i)
					if i == 2 {
						v = math.Float64frombits(value.StaleNaN)
					}
					var err error
					oldRef, err = oldApp.Append(0, ls, int64((i+1)*100), v)
					require.NoError(t, err)
				}
				require.NoError(t, oldApp.Commit())

				evicted := make(chan error, 1)
				evictionDone := make(chan struct{})
				go func() {
					defer close(evictionDone)
					evicted <- evict(db, []storage.SeriesRef{oldRef})
				}()
				t.Cleanup(func() {
					resume()
					finish()
					select {
					case <-evictionDone:
					case <-time.After(10 * time.Second):
						t.Error("the eviction did not finish")
					}
				})
				select {
				case <-unlinked:
				case err := <-evicted:
					t.Fatalf("the eviction returned without unlinking the series: %v", err)
				case <-time.After(10 * time.Second):
					t.Fatal("the eviction did not reach the pause before the tombstone")
				}
				require.True(t, db.head.series.pendingTombstones.active.Load())
				resume()

				// The creator must stay blocked until the tombstone is written.
				synctest.Wait()
				select {
				case <-created:
					require.FailNow(t, "the creator was not blocked while the tombstone of the old series was pending")
				default:
				}
				finish()
				var result appendResult
				select {
				case result = <-created:
				case <-time.After(10 * time.Second):
					t.Fatal("the creator remained blocked after the tombstone was written")
				}
				require.NoError(t, result.err)
				require.NotEqual(t, oldRef, result.ref)
				require.NoError(t, <-evicted)

				app := db.Appender(context.Background())
				_, err := app.Append(result.ref, ls, 500, 43)
				require.NoError(t, err)
				require.NoError(t, app.Commit())
				matcher := labels.MustNewMatcher(labels.MatchEqual, "id", "20")
				q, err := db.Querier(math.MinInt64, math.MaxInt64)
				require.NoError(t, err)
				before := query(t, q, matcher)
				require.Len(t, before[ls.String()], 5)
				require.NoError(t, db.Close())

				oldStonePos, newSeriesPos := -1, -1
				for i, entry := range readTestWAL(t, filepath.Join(db.Dir(), "wal")) {
					switch records := entry.(type) {
					case []tombstones.Stone:
						for _, stone := range records {
							if stone.Ref == oldRef {
								oldStonePos = i
							}
						}
					case []record.RefSeries:
						for _, series := range records {
							if series.Ref == chunks.HeadSeriesRef(result.ref) {
								newSeriesPos = i
							}
						}
					}
				}
				require.NotEqual(t, -1, oldStonePos)
				require.NotEqual(t, -1, newSeriesPos)
				require.Greater(t, newSeriesPos, oldStonePos, "the declaration of the new series must follow the tombstone of the old one")

				reopened, err := Open(db.Dir(), nil, nil, db.opts, nil)
				require.NoError(t, err)
				reopened.DisableCompactions()
				t.Cleanup(func() { require.NoError(t, reopened.Close()) })
				q, err = reopened.Querier(math.MinInt64, math.MaxInt64)
				require.NoError(t, err)
				require.Equal(t, before, query(t, q, matcher), "the committed samples must survive replay")
				require.Zero(t, prom_testutil.ToFloat64(reopened.head.metrics.walReplayUnknownRefsTotal.WithLabelValues("samples")))
			})
		})
	}
}

// TestHead_WALReplay_RecreatedSeriesKeepsSamplesBelowOldMmappedChunks verifies
// that a replacement series keeps its samples after restart, even when their
// timestamps are older than chunks the evicted series had written to disk.
//
// The test starts recreating the series after eviction removes it from Head,
// but before its tombstone is written. Creation must wait for the tombstone,
// so WAL replay restores the replacement as a separate series.
//
// If replay attached the replacement to the old series, it could incorrectly
// skip the replacement's older samples, assuming they were already stored in
// the old series' chunks on disk.
func TestHead_WALReplay_RecreatedSeriesKeepsSamplesBelowOldMmappedChunks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		head, w := newTestHead(t, 10000, compression.None, false)
		require.NoError(t, head.Init(0))
		lset := labels.FromStrings("foo", "bar")
		matcher := labels.MustNewMatcher(labels.MatchEqual, "foo", "bar")
		querySamples := func(h *Head) []int64 {
			q, err := NewBlockQuerier(h, math.MinInt64, math.MaxInt64)
			require.NoError(t, err)
			defer q.Close()
			var timestamps []int64
			for _, samples := range query(t, q, matcher) {
				for _, s := range samples {
					timestamps = append(timestamps, s.T())
				}
			}
			return timestamps
		}

		// The old series: 130 samples at t=0,2,...,258, the last one a stale marker. A chunk
		// holds about 120 samples, so the first ones end up in a chunk that is written to
		// disk, and the last ones stay in the head chunk.
		app := head.Appender(ctx)
		var oldRef storage.SeriesRef
		for i := range 130 {
			v := float64(i)
			if i == 129 {
				v = math.Float64frombits(value.StaleNaN)
			}
			var err error
			oldRef, err = app.Append(0, lset, int64(2*i), v)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
		head.mmapHeadChunks()
		oldSeries := head.series.getByID(chunks.HeadSeriesRef(oldRef))
		require.NotEmpty(t, oldSeries.mmappedChunks)
		mappedMax := oldSeries.mmappedChunks[len(oldSeries.mmappedChunks)-1].maxTime
		require.Greater(t, mappedMax, int64(151), "the replacement's sample must be older than the chunk on disk")
		require.Less(t, mappedMax, int64(258))
		t.Logf("the evicted series has chunks on disk up to t=%d", mappedMax)

		// Evict the old series. Right after it is removed from the head, and before its
		// tombstone is written, ingestion recreates it with a sample older than mappedMax.
		var newRef storage.SeriesRef
		recreated := make(chan error, 1)
		head.series.seriesLifecycleCallback = seriesEvictionPostDeletionCallback{
			postDeletion: func(deleted map[chunks.HeadSeriesRef]labels.Labels) {
				if _, ok := deleted[chunks.HeadSeriesRef(oldRef)]; !ok {
					return
				}
				// A series with the labels of the evicted one waits for its tombstone, so it can't be
				// created from the goroutine that writes it.
				go func() {
					app := head.Appender(ctx)
					var err error
					newRef, err = app.Append(0, lset, 151, 42)
					if err == nil {
						err = app.Commit()
					}
					recreated <- err
				}()
				// Wait until the creator is blocked on the pending tombstone, so that the eviction can't write it before
				// the creator looks for it.
				synctest.Wait()
				require.Empty(t, recreated, "the creator must wait for the tombstone")
			},
		}
		refs := []storage.SeriesRef{oldRef}
		require.NoError(t, head.truncateStaleSeries(refs, 258, head.snapshotFingerprints(refs, math.MaxUint64)))
		require.NoError(t, <-recreated)
		require.NotEqual(t, oldRef, newRef)

		app = head.Appender(ctx)
		_, err := app.Append(newRef, lset, 301, 43)
		require.NoError(t, err)
		require.NoError(t, app.Commit())

		// While running, both samples of the replacement are there.
		require.Equal(t, []int64{151, 301}, querySamples(head))
		require.NoError(t, head.Close())

		// Restart.
		w, err = wlog.New(nil, nil, w.Dir(), compression.None)
		require.NoError(t, err)
		opts := DefaultHeadOptions()
		opts.ChunkRange = 10000
		opts.ChunkDirRoot = head.opts.ChunkDirRoot
		head, err = NewHead(nil, nil, w, nil, opts, nil)
		require.NoError(t, err)
		require.NoError(t, head.Init(0))
		t.Cleanup(func() { require.NoError(t, head.Close()) })

		replayed := head.series.getByHash(lset.Hash(), lset)
		require.NotNil(t, replayed)
		t.Logf("replacement series %d replays as series %d, which skips every sample <= %d", newRef, replayed.ref, replayed.mmMaxTime)

		// Replay must restore only the replacement's samples. The old series'
		// tombstone removes it before the replacement is declared.
		require.Equal(t, []int64{151, 301}, querySamples(head))
	})
}

// TestSeriesEviction_CreatorsAreReleasedBeforePostingsAreDeleted verifies that the creation of a series with the
// labels of an evicted series waits until the tombstones are written, and not until the eviction deletes the series
// from the postings, which takes long because it yields to the readers. The tombstones are in the WAL before the
// declaration of the new series.
func TestSeriesEviction_CreatorsAreReleasedBeforePostingsAreDeleted(t *testing.T) {
	const timeout = 10 * time.Second

	for name, evict := range evictionTestEvictions() {
		t.Run(name, func(t *testing.T) {
			testDB := newSeriesEvictionTestDB(t)
			db := testDB.db
			oldRef := testDB.evictedRefs[0]
			lset := evictionTestLabels(0)

			// Pause the eviction after it unlinked the series, and wrote the tombstones, and before it deletes the
			// postings.
			inPostings, finish := make(chan struct{}), make(chan struct{})
			var pausedOnce, finishOnce sync.Once
			resumeEviction := func() { finishOnce.Do(func() { close(finish) }) }
			db.head.testBeforeDeletingPostings = func() {
				pausedOnce.Do(func() { close(inPostings) })
				<-finish
			}
			evicted := make(chan error, 1)
			evictionDone := make(chan struct{})
			go func() {
				defer close(evictionDone)
				evicted <- evict(db, testDB.evictedRefs)
			}()
			t.Cleanup(func() {
				resumeEviction()
				select {
				case <-evictionDone:
				case <-time.After(timeout):
					t.Error("the eviction did not finish")
				}
			})
			select {
			case <-inPostings:
			case err := <-evicted:
				t.Fatalf("the eviction returned before it deleted the postings: %v", err)
			case <-time.After(timeout):
				t.Fatal("the eviction did not reach the deletion of the postings")
			}
			require.False(t, db.head.series.pendingTombstones.active.Load(), "the creators must be released")

			// The old series is unlinked, so this creates a new one. It must not wait for the eviction to finish.
			var newRef storage.SeriesRef
			created := make(chan error, 1)
			go func() {
				app := db.Appender(t.Context())
				var err error
				newRef, err = app.Append(0, lset, 400, 42)
				if err == nil {
					err = app.Commit()
				}
				created <- err
			}()
			select {
			case err := <-created:
				require.NoError(t, err)
			case <-time.After(timeout):
				require.FailNow(t, "the creator was not released while the eviction deletes the postings")
			}
			require.NotEqual(t, oldRef, newRef)

			// The old series is no longer in the head, but both series are in the postings. The samples of the old one
			// are in the block written by the eviction, and the ones of the new one are in the head.
			requireQueriedTimestamps := func() {
				t.Helper()
				q, err := db.Querier(math.MinInt64, math.MaxInt64)
				require.NoError(t, err)
				var timestamps []int64
				for _, s := range query(t, q, labels.MustNewMatcher(labels.MatchEqual, "id", lset.Get("id")))[lset.String()] {
					timestamps = append(timestamps, s.T())
				}
				require.Equal(t, []int64{100, 200, 300, 400}, timestamps)
			}
			requireQueriedTimestamps()

			resumeEviction()
			require.NoError(t, <-evicted)
			// Deleting the old series from the postings must not remove the new one.
			requireQueriedTimestamps()
			require.NoError(t, db.Close())

			stonePosition, seriesPosition := -1, -1
			for i, entry := range readTestWAL(t, filepath.Join(db.Dir(), "wal")) {
				switch records := entry.(type) {
				case []tombstones.Stone:
					for _, stone := range records {
						if stone.Ref == oldRef {
							stonePosition = i
						}
					}
				case []record.RefSeries:
					for _, series := range records {
						if series.Ref == chunks.HeadSeriesRef(newRef) {
							seriesPosition = i
						}
					}
				}
			}
			require.NotEqual(t, -1, stonePosition)
			require.NotEqual(t, -1, seriesPosition)
			require.Greater(t, seriesPosition, stonePosition, "the declaration of the new series must follow the tombstone of the old one")
		})
	}
}

func TestEvictedSeries_Contains(t *testing.T) {
	refs := []storage.SeriesRef{3, 5, 6, 100, 1 << 40}

	tests := map[string]struct {
		evicted   *evictedSeries
		in, notIn []chunks.HeadSeriesRef
	}{
		"nil": {
			notIn: []chunks.HeadSeriesRef{0, 3},
		},
		"no refs": {
			evicted: &evictedSeries{},
			notIn:   []chunks.HeadSeriesRef{0, 3},
		},
		"refs": {
			evicted: &evictedSeries{refs: refs},
			in:      []chunks.HeadSeriesRef{3, 5, 6, 100, 1 << 40},
			notIn:   []chunks.HeadSeriesRef{0, 2, 4, 7, 99, 101, 1<<40 - 1, 1<<40 + 1},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			for _, ref := range tc.in {
				require.True(t, tc.evicted.contains(ref), "ref %d", ref)
			}
			for _, ref := range tc.notIn {
				require.False(t, tc.evicted.contains(ref), "ref %d", ref)
			}
		})
	}
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
// headChunkReader.evicting. This test fails if that order is changed.
func TestSeriesRemoval_BlockIsAddedBeforeSeriesAreRemoved(t *testing.T) {
	for name, evict := range evictionTestEvictions() {
		t.Run(name, func(t *testing.T) {
			testDB := newSeriesEvictionTestDB(t)
			db := testDB.db

			var before beforeRemoval
			db.Head().testAfterWaitingForReadersToEvictSeries = func() {
				requireEvictingSeriesSorted(t, db.Head())
				before.record(t, db)
			}

			require.NoError(t, evict(db, testDB.evictedRefs))

			before.require(t)
			require.Equal(t, uint64(evictionTestSeries-evictionTestEvictedSeries), db.Head().NumSeries(), "the series should have been removed")
		})
	}
}

// TestSeriesEviction_HeadAndOOOQueriersWithoutSkipEvictedSeriesDoNotSkipRemovedSeries verifies that the head and
// out-of-order queriers, if they are not asked to skip the evicted series, still fail to read the chunks of a series
// that was removed from the head, as they always did.
func TestSeriesEviction_HeadAndOOOQueriersWithoutSkipEvictedSeriesDoNotSkipRemovedSeries(t *testing.T) {
	const mint, maxt = 0, 1000

	tests := map[string]func(t *testing.T, db *DB, isoState *oooIsolationState) *readingSeries{
		"NewHeadAndOOOQuerier": func(t *testing.T, db *DB, iso *oooIsolationState) *readingSeries {
			return selectSamples(t, NewHeadAndOOOQuerier(0, mint, maxt, db.head, iso, nil, false))
		},
		"NewHeadAndOOOChunkQuerier": func(t *testing.T, db *DB, iso *oooIsolationState) *readingSeries {
			return selectChunks(t, NewHeadAndOOOChunkQuerier(0, mint, maxt, db.head, iso, nil, false))
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

// TestSeriesEviction_QueryStartingAtLastTimestampIsWaitedForByNextEviction verifies that an eviction waits for a
// query that starts exactly at the last timestamp it persists.
//
// A series that is selected by an eviction, but gets a late sample, is not evicted. A query opened meanwhile has the
// series in the set being evicted, and does not find the late sample in the blocks. If a following eviction, which
// persists the late sample, didn't wait for the query, it would remove the series, and the query would skip it, and
// silently omit the late sample.
func TestSeriesEviction_QueryStartingAtLastTimestampIsWaitedForByNextEviction(t *testing.T) {
	const lateSampleTs = 400

	queriers := map[string]func(*testing.T, *DB, int64, int64) *readingSeries{
		"Querier":      openSampleQuerier,
		"ChunkQuerier": openChunkQuerier,
	}
	for name, openQuery := range queriers {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				testDB := newSeriesEvictionTestDB(t)
				db := testDB.db

				// The first eviction selects the series to evict, then series 0 gets a late sample, and so it is not
				// evicted. A query for the timestamp of the late sample is opened after the first eviction waited for the
				// readers, so it has the series 0 in the set being evicted, and the late sample is in the head only.
				db.compactHeadViewBeforeEvictTestingCallback = func() {
					app := db.Appender(context.Background())
					_, err := app.Append(0, evictionTestLabels(0), lateSampleTs, 42)
					require.NoError(t, err)
					require.NoError(t, app.Commit())
				}
				var reading *readingSeries
				db.Head().testAfterWaitingForReadersToEvictSeries = func() {
					reading = openQuery(t, db, lateSampleTs, lateSampleTs)
				}
				require.NoError(t, db.CompactSelectedSeries(testDB.evictedRefs))
				require.NotNil(t, reading)
				require.Equal(t, uint64(evictionTestSeries-evictionTestEvictedSeries+1), db.Head().NumSeries())

				// The second eviction persists the late sample, and its last timestamp is the one the query starts at.
				done := make(chan error, 1)
				go func() { done <- db.CompactSelectedSeries(testDB.evictedRefs[:1]) }()
				synctest.Wait()

				// The query is still open, so the eviction must be waiting for it.
				select {
				case err := <-done:
					require.FailNow(t, "the eviction must wait for the query that starts at its last timestamp", "eviction returned: %v", err)
				default:
				}
				samples, err := reading.read()
				require.NoError(t, err)
				require.Equal(t, map[string]int{evictionTestLabels(0).String(): 1}, samples)
				require.NoError(t, reading.close())

				// Advance fake time past the reader-wait polling interval.
				time.Sleep(time.Second)
				synctest.Wait()
				select {
				case err := <-done:
					require.NoError(t, err)
				default:
					t.Fatal("the eviction must complete after the query closes")
				}
			})
		})
	}
}

// TestSeriesEviction_SamplesAndEvictionWhileWaitingForReaders verifies what happens while an eviction waits for the
// open readers, which can take minutes. The series are still in the head and no hash is pending, so a late sample for
// a series to evict, and a new series, are appended without waiting. A second eviction requested meanwhile waits for
// the first one, and then evicts the series that the first one skipped because of their late sample.
func TestSeriesEviction_SamplesAndEvictionWhileWaitingForReaders(t *testing.T) {
	const (
		lateSampleTs = 400
		timeout      = 30 * time.Second
	)

	testDB := newSeriesEvictionTestDB(t)
	db := testDB.db
	head := db.Head()
	newSeries := evictionTestLabels(evictionTestSeries)

	// The query overlaps with the range that the eviction waits for.
	reading := openSampleQuerier(t, db, 0, lateSampleTs)

	first := make(chan error, 1)
	go func() { first <- db.CompactSelectedSeries(testDB.evictedRefs) }()
	require.Eventually(t, func() bool { return head.evictingSeries.Load() != nil }, timeout, 10*time.Millisecond,
		"the first eviction must be waiting for the query")
	require.False(t, head.series.pendingTombstones.active.Load(), "nothing is unlinked while waiting for the readers")

	// If these appends waited for the eviction, the test would time out.
	app := db.Appender(context.Background())
	_, err := app.Append(0, evictionTestLabels(0), lateSampleTs, 42)
	require.NoError(t, err)
	_, err = app.Append(0, newSeries, lateSampleTs, 43)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, uint64(evictionTestSeries+1), head.NumSeries())

	// The first eviction holds the compaction lock, which the second one needs.
	require.False(t, db.cmtx.TryLock(), "the first eviction must hold the compaction lock")
	// The first eviction already passed this hook, so it runs when the second eviction starts, which is after the
	// first one finished.
	var seriesBeforeSecondEviction uint64
	var lateSeriesBeforeSecondEviction *memSeries
	db.compactHeadViewBeforeEvictTestingCallback = func() {
		seriesBeforeSecondEviction = head.NumSeries()
		lateSeriesBeforeSecondEviction = head.series.getByHash(evictionTestLabels(0).Hash(), evictionTestLabels(0))
	}
	second := make(chan error, 1)
	go func() { second <- db.CompactSelectedSeries(testDB.evictedRefs) }()
	require.Equal(t, uint64(evictionTestSeries+1), head.NumSeries(), "nothing may be evicted while the query is open")

	require.NoError(t, reading.close())
	for name, done := range map[string]chan error{"first": first, "second": second} {
		select {
		case err := <-done:
			require.NoError(t, err, name)
		case <-time.After(timeout):
			require.FailNow(t, "the eviction must complete after the query closes", name)
		}
	}

	// The first eviction skipped the series with the late sample, and evicted the others, so the second one is
	// the one that evicts it.
	require.Equal(t, uint64(evictionTestSeries-evictionTestEvictedSeries+2), seriesBeforeSecondEviction)
	require.NotNil(t, lateSeriesBeforeSecondEviction)
	require.Nil(t, head.series.getByHash(evictionTestLabels(0).Hash(), evictionTestLabels(0)))

	require.False(t, head.series.pendingTombstones.active.Load())
	// Only the series that were not selected remain, and the new one.
	require.Equal(t, uint64(evictionTestSeries-evictionTestEvictedSeries+1), head.NumSeries())

	expected := evictionTestExpectedSamples(0)
	expected[newSeries.String()] = 1
	require.Equal(t, expected, openSampleQuerier(t, db, 0, math.MaxInt64).mustRead(t))
}

// BenchmarkTruncateSeries measures the time that truncateSeries takes to evict all the series of a head with a WAL.
func BenchmarkTruncateSeries(b *testing.B) {
	for _, n := range []int{10_000, 100_000, 500_000} {
		b.Run(fmt.Sprintf("series=%d", n), func(b *testing.B) {
			for b.Loop() {
				b.StopTimer()
				h, _ := newTestHead(b, 10000, compression.None, false)
				require.NoError(b, h.Init(0))
				app := h.Appender(context.Background())
				refs := make([]storage.SeriesRef, 0, n)
				for i := range n {
					ref, err := app.Append(0, labels.FromStrings("__name__", "series", "id", fmt.Sprintf("%07d", i)), 10, 1)
					require.NoError(b, err)
					refs = append(refs, ref)
				}
				require.NoError(b, app.Commit())
				b.StartTimer()

				evicted, err := h.truncateSeries(refs, 10, func(*memSeries) bool { return true })
				b.StopTimer()
				require.NoError(b, err)
				require.Equal(b, n, evicted)
				require.NoError(b, h.Close())
				b.StartTimer()
			}
		})
	}
}

// BenchmarkTruncateSeriesCreatorWait measures how long creating a series can take while truncateSeries evicts the
// series of a head with a WAL. The creators call getOrCreate in a loop while the eviction runs:
//   - "matching" creators use the labels of the series being evicted, so the ones that miss because their series
//     was just unlinked wait for the tombstone to be written.
//   - "unrelated" creators use labels that are not evicted, which measures the cost of the check for series that
//     don't wait.
//
// The committers append and commit samples of other series in a loop, which keeps the WAL busy, so that the tombstone
// write has to wait for the WAL lock.
//
// The reported metrics are, averaged over the iterations: maxwait-ms, the longest call to getOrCreate; blocked, the
// number of calls that took at least a millisecond; and creates, the number of calls.
func BenchmarkTruncateSeriesCreatorWait(b *testing.B) {
	const (
		creatorGoroutines = 4
		blockedThreshold  = time.Millisecond
		// unrelatedSeries is the number of distinct series that each creator goroutine creates, one per call, until it
		// has created them all. Then it starts again, and finds them.
		unrelatedSeries = 250_000
	)

	for _, n := range []int{100_000, 500_000} {
		for _, committers := range []int{0, 8} {
			for _, creators := range []string{"matching", "unrelated"} {
				b.Run(fmt.Sprintf("series=%d/committers=%d/creators=%s", n, committers, creators), func(b *testing.B) {
					var (
						iterations int
						maxWaits   time.Duration
						blocked    int
						calls      int
					)
					for b.Loop() {
						b.StopTimer()
						h, _ := newTestHead(b, 10000, compression.None, false)
						require.NoError(b, h.Init(0))
						app := h.Appender(context.Background())
						refs := make([]storage.SeriesRef, 0, n)
						evicted := make([]labels.Labels, 0, n)
						for i := range n {
							lset := labels.FromStrings("__name__", "series", "id", fmt.Sprintf("%07d", i))
							ref, err := app.Append(0, lset, 10, 1)
							require.NoError(b, err)
							refs = append(refs, ref)
							evicted = append(evicted, lset)
						}
						require.NoError(b, app.Commit())

						// The matching creators recreate random evicted series. Each unrelated creator creates its own
						// distinct series one after the other, so that its calls reach the creation of a series, and
						// not only the lookup of an existing one.
						creatorLabels := make([][]labels.Labels, creatorGoroutines)
						creatorHashes := make([][]uint64, creatorGoroutines)
						for g := range creatorGoroutines {
							creatorLabels[g] = evicted
							if creators == "unrelated" {
								creatorLabels[g] = make([]labels.Labels, unrelatedSeries)
								for i := range creatorLabels[g] {
									creatorLabels[g][i] = labels.FromStrings("__name__", "unrelated", "id", fmt.Sprintf("%d_%07d", g, i))
								}
							}
							creatorHashes[g] = make([]uint64, len(creatorLabels[g]))
							for i, lset := range creatorLabels[g] {
								creatorHashes[g][i] = lset.Hash()
							}
						}

						// The goroutines start when the timer does, so that the creation of series does not begin before it.
						stop, start := make(chan struct{}), make(chan struct{})
						var running, ready sync.WaitGroup
						// The committers share the clock, so that none of them falls out of the head's bounds.
						var clock atomic.Int64
						clock.Store(100)
						for c := range committers {
							lset := labels.FromStrings("__name__", "background", "id", strconv.Itoa(c))
							app := h.Appender(context.Background())
							_, err := app.Append(0, lset, 100, 1)
							require.NoError(b, err)
							require.NoError(b, app.Commit())
							ready.Add(1)
							running.Go(func() {
								ready.Done()
								<-start
								for {
									select {
									case <-stop:
										return
									default:
									}
									app := h.Appender(context.Background())
									if _, err := app.Append(0, lset, clock.Add(1), 1); err != nil {
										panic(err)
									}
									if err := app.Commit(); err != nil {
										panic(err)
									}
								}
							})
						}

						type creatorResult struct {
							maxWait time.Duration
							blocked int
							calls   int
						}
						results := make([]creatorResult, creatorGoroutines)
						for g := range creatorGoroutines {
							ready.Add(1)
							running.Go(func() {
								ready.Done()
								<-start
								r := rand.New(rand.NewSource(int64(g)))
								res := &results[g]
								for {
									select {
									case <-stop:
										return
									default:
									}
									i := res.calls % len(creatorLabels[g])
									if creators == "matching" {
										i = r.Intn(len(creatorLabels[g]))
									}
									callStart := time.Now()
									if _, _, err := h.getOrCreate(creatorHashes[g][i], creatorLabels[g][i], false); err != nil {
										panic(err)
									}
									d := time.Since(callStart)
									res.calls++
									res.maxWait = max(res.maxWait, d)
									if d >= blockedThreshold {
										res.blocked++
									}
								}
							})
						}
						ready.Wait()

						b.StartTimer()
						close(start)
						n2, err := h.truncateSeries(refs, 10, func(*memSeries) bool { return true })
						b.StopTimer()
						close(stop)
						running.Wait()
						require.NoError(b, err)
						require.Equal(b, n, n2)
						require.NoError(b, h.Close())

						iterations++
						var iterationMax time.Duration
						for _, res := range results {
							iterationMax = max(iterationMax, res.maxWait)
							blocked += res.blocked
							calls += res.calls
						}
						maxWaits += iterationMax
						b.StartTimer()
					}
					b.ReportMetric(float64(maxWaits)/1e6/float64(iterations), "maxwait-ms")
					b.ReportMetric(float64(blocked)/float64(iterations), "blocked")
					b.ReportMetric(float64(calls)/float64(iterations), "creates")
				})
			}
		}
	}
}
