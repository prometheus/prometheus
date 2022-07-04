// Copyright 2017 The Prometheus Authors
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
	"bufio"
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"hash/crc32"
	"math"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wal"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestMain(m *testing.M) {
	var isolationEnabled bool
	flag.BoolVar(&isolationEnabled, "test.tsdb-isolation", true, "enable isolation")
	flag.Parse()
	defaultIsolationDisabled = !isolationEnabled

	goleak.VerifyTestMain(m, goleak.IgnoreTopFunction("github.com/prometheus/prometheus/tsdb.(*SegmentWAL).cut.func1"), goleak.IgnoreTopFunction("github.com/prometheus/prometheus/tsdb.(*SegmentWAL).cut.func2"))
}

func openTestDB(t testing.TB, opts *Options, rngs []int64) (db *DB) {
	tmpdir := t.TempDir()
	var err error

	if len(rngs) == 0 {
		db, err = Open(tmpdir, nil, nil, opts, nil)
	} else {
		opts, rngs = validateOpts(opts, rngs)
		db, err = open(tmpdir, nil, nil, opts, rngs, nil)
	}
	require.NoError(t, err)

	// Do not Close() the test database by default as it will deadlock on test failures.
	return db
}

// query runs a matcher query against the querier and fully expands its data.
func query(t testing.TB, q storage.Querier, matchers ...*labels.Matcher) map[string][]tsdbutil.Sample {
	ss := q.Select(false, nil, matchers...)
	defer func() {
		require.NoError(t, q.Close())
	}()

	result := map[string][]tsdbutil.Sample{}
	for ss.Next() {
		series := ss.At()

		samples := []tsdbutil.Sample{}
		it := series.Iterator()
		for it.Next() {
			t, v := it.At()
			samples = append(samples, sample{t: t, v: v})
		}
		require.NoError(t, it.Err())

		if len(samples) == 0 {
			continue
		}

		name := series.Labels().String()
		result[name] = samples
	}
	require.NoError(t, ss.Err())
	require.Equal(t, 0, len(ss.Warnings()))

	return result
}

// queryChunks runs a matcher query against the querier and fully expands its data.
func queryChunks(t testing.TB, q storage.ChunkQuerier, matchers ...*labels.Matcher) map[string][]chunks.Meta {
	ss := q.Select(false, nil, matchers...)
	defer func() {
		require.NoError(t, q.Close())
	}()

	result := map[string][]chunks.Meta{}
	for ss.Next() {
		series := ss.At()

		chks := []chunks.Meta{}
		it := series.Iterator()
		for it.Next() {
			chks = append(chks, it.At())
		}
		require.NoError(t, it.Err())

		if len(chks) == 0 {
			continue
		}

		name := series.Labels().String()
		result[name] = chks
	}
	require.NoError(t, ss.Err())
	require.Equal(t, 0, len(ss.Warnings()))
	return result
}

// Ensure that blocks are held in memory in their time order
// and not in ULID order as they are read from the directory.
func TestDB_reloadOrder(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		require.NoError(t, db.Close())
	}()

	metas := []BlockMeta{
		{MinTime: 90, MaxTime: 100},
		{MinTime: 70, MaxTime: 80},
		{MinTime: 100, MaxTime: 110},
	}
	for _, m := range metas {
		createBlock(t, db.Dir(), genSeries(1, 1, m.MinTime, m.MaxTime))
	}

	require.NoError(t, db.reloadBlocks())
	blocks := db.Blocks()
	require.Equal(t, 3, len(blocks))
	require.Equal(t, metas[1].MinTime, blocks[0].Meta().MinTime)
	require.Equal(t, metas[1].MaxTime, blocks[0].Meta().MaxTime)
	require.Equal(t, metas[0].MinTime, blocks[1].Meta().MinTime)
	require.Equal(t, metas[0].MaxTime, blocks[1].Meta().MaxTime)
	require.Equal(t, metas[2].MinTime, blocks[2].Meta().MinTime)
	require.Equal(t, metas[2].MaxTime, blocks[2].Meta().MaxTime)
}

func TestDataAvailableOnlyAfterCommit(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		require.NoError(t, db.Close())
	}()

	ctx := context.Background()
	app := db.Appender(ctx)

	_, err := app.Append(0, labels.FromStrings("foo", "bar"), 0, 0)
	require.NoError(t, err)

	querier, err := db.Querier(context.TODO(), 0, 1)
	require.NoError(t, err)
	seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	require.Equal(t, map[string][]tsdbutil.Sample{}, seriesSet)

	err = app.Commit()
	require.NoError(t, err)

	querier, err = db.Querier(context.TODO(), 0, 1)
	require.NoError(t, err)
	defer querier.Close()

	seriesSet = query(t, querier, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))

	require.Equal(t, map[string][]tsdbutil.Sample{`{foo="bar"}`: {sample{t: 0, v: 0}}}, seriesSet)
}

// TestNoPanicAfterWALCorruption ensures that querying the db after a WAL corruption doesn't cause a panic.
// https://github.com/prometheus/prometheus/issues/7548
func TestNoPanicAfterWALCorruption(t *testing.T) {
	db := openTestDB(t, &Options{WALSegmentSize: 32 * 1024}, nil)

	// Append until the first mmaped head chunk.
	// This is to ensure that all samples can be read from the mmaped chunks when the WAL is corrupted.
	var expSamples []tsdbutil.Sample
	var maxt int64
	ctx := context.Background()
	{
		// Appending 121 samples because on the 121st a new chunk will be created.
		for i := 0; i < 121; i++ {
			app := db.Appender(ctx)
			_, err := app.Append(0, labels.FromStrings("foo", "bar"), maxt, 0)
			expSamples = append(expSamples, sample{t: maxt, v: 0})
			require.NoError(t, err)
			require.NoError(t, app.Commit())
			maxt++
		}
		require.NoError(t, db.Close())
	}

	// Corrupt the WAL after the first sample of the series so that it has at least one sample and
	// it is not garbage collected.
	// The repair deletes all WAL records after the corrupted record and these are read from the mmaped chunk.
	{
		walFiles, err := os.ReadDir(path.Join(db.Dir(), "wal"))
		require.NoError(t, err)
		f, err := os.OpenFile(path.Join(db.Dir(), "wal", walFiles[0].Name()), os.O_RDWR, 0o666)
		require.NoError(t, err)
		r := wal.NewReader(bufio.NewReader(f))
		require.True(t, r.Next(), "reading the series record")
		require.True(t, r.Next(), "reading the first sample record")
		// Write an invalid record header to corrupt everything after the first wal sample.
		_, err = f.WriteAt([]byte{99}, r.Offset())
		require.NoError(t, err)
		f.Close()
	}

	// Query the data.
	{
		db, err := Open(db.Dir(), nil, nil, nil, nil)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, db.Close())
		}()
		require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.walCorruptionsTotal), "WAL corruption count mismatch")

		querier, err := db.Querier(context.TODO(), 0, maxt)
		require.NoError(t, err)
		seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchEqual, "", ""))
		// The last sample should be missing as it was after the WAL segment corruption.
		require.Equal(t, map[string][]tsdbutil.Sample{`{foo="bar"}`: expSamples[0 : len(expSamples)-1]}, seriesSet)
	}
}

func TestDataNotAvailableAfterRollback(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		require.NoError(t, db.Close())
	}()

	app := db.Appender(context.Background())
	_, err := app.Append(0, labels.FromStrings("foo", "bar"), 0, 0)
	require.NoError(t, err)

	err = app.Rollback()
	require.NoError(t, err)

	querier, err := db.Querier(context.TODO(), 0, 1)
	require.NoError(t, err)
	defer querier.Close()

	seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))

	require.Equal(t, map[string][]tsdbutil.Sample{}, seriesSet)
}

func TestDBAppenderAddRef(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		require.NoError(t, db.Close())
	}()

	ctx := context.Background()
	app1 := db.Appender(ctx)

	ref1, err := app1.Append(0, labels.FromStrings("a", "b"), 123, 0)
	require.NoError(t, err)

	// Reference should already work before commit.
	ref2, err := app1.Append(ref1, nil, 124, 1)
	require.NoError(t, err)
	require.Equal(t, ref1, ref2)

	err = app1.Commit()
	require.NoError(t, err)

	app2 := db.Appender(ctx)

	// first ref should already work in next transaction.
	ref3, err := app2.Append(ref1, nil, 125, 0)
	require.NoError(t, err)
	require.Equal(t, ref1, ref3)

	ref4, err := app2.Append(ref1, labels.FromStrings("a", "b"), 133, 1)
	require.NoError(t, err)
	require.Equal(t, ref1, ref4)

	// Reference must be valid to add another sample.
	ref5, err := app2.Append(ref2, nil, 143, 2)
	require.NoError(t, err)
	require.Equal(t, ref1, ref5)

	// Missing labels & invalid refs should fail.
	_, err = app2.Append(9999999, nil, 1, 1)
	require.Equal(t, ErrInvalidSample, errors.Cause(err))

	require.NoError(t, app2.Commit())

	q, err := db.Querier(context.TODO(), 0, 200)
	require.NoError(t, err)

	res := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

	require.Equal(t, map[string][]tsdbutil.Sample{
		labels.FromStrings("a", "b").String(): {
			sample{t: 123, v: 0},
			sample{t: 124, v: 1},
			sample{t: 125, v: 0},
			sample{t: 133, v: 1},
			sample{t: 143, v: 2},
		},
	}, res)
}

func TestAppendEmptyLabelsIgnored(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		require.NoError(t, db.Close())
	}()

	ctx := context.Background()
	app1 := db.Appender(ctx)

	ref1, err := app1.Append(0, labels.FromStrings("a", "b"), 123, 0)
	require.NoError(t, err)

	// Construct labels manually so there is an empty label.
	ref2, err := app1.Append(0, labels.Labels{labels.Label{Name: "a", Value: "b"}, labels.Label{Name: "c", Value: ""}}, 124, 0)
	require.NoError(t, err)

	// Should be the same series.
	require.Equal(t, ref1, ref2)

	err = app1.Commit()
	require.NoError(t, err)
}

func TestDeleteSimple(t *testing.T) {
	numSamples := int64(10)

	cases := []struct {
		Intervals tombstones.Intervals
		remaint   []int64
	}{
		{
			Intervals: tombstones.Intervals{{Mint: 0, Maxt: 3}},
			remaint:   []int64{4, 5, 6, 7, 8, 9},
		},
		{
			Intervals: tombstones.Intervals{{Mint: 1, Maxt: 3}},
			remaint:   []int64{0, 4, 5, 6, 7, 8, 9},
		},
		{
			Intervals: tombstones.Intervals{{Mint: 1, Maxt: 3}, {Mint: 4, Maxt: 7}},
			remaint:   []int64{0, 8, 9},
		},
		{
			Intervals: tombstones.Intervals{{Mint: 1, Maxt: 3}, {Mint: 4, Maxt: 700}},
			remaint:   []int64{0},
		},
		{ // This case is to ensure that labels and symbols are deleted.
			Intervals: tombstones.Intervals{{Mint: 0, Maxt: 9}},
			remaint:   []int64{},
		},
	}

Outer:
	for _, c := range cases {
		db := openTestDB(t, nil, nil)
		defer func() {
			require.NoError(t, db.Close())
		}()

		ctx := context.Background()
		app := db.Appender(ctx)

		smpls := make([]float64, numSamples)
		for i := int64(0); i < numSamples; i++ {
			smpls[i] = rand.Float64()
			app.Append(0, labels.Labels{{Name: "a", Value: "b"}}, i, smpls[i])
		}

		require.NoError(t, app.Commit())

		// TODO(gouthamve): Reset the tombstones somehow.
		// Delete the ranges.
		for _, r := range c.Intervals {
			require.NoError(t, db.Delete(r.Mint, r.Maxt, labels.MustNewMatcher(labels.MatchEqual, "a", "b")))
		}

		// Compare the result.
		q, err := db.Querier(context.TODO(), 0, numSamples)
		require.NoError(t, err)

		res := q.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

		expSamples := make([]tsdbutil.Sample, 0, len(c.remaint))
		for _, ts := range c.remaint {
			expSamples = append(expSamples, sample{ts, smpls[ts]})
		}

		expss := newMockSeriesSet([]storage.Series{
			storage.NewListSeries(labels.FromStrings("a", "b"), expSamples),
		})

		for {
			eok, rok := expss.Next(), res.Next()
			require.Equal(t, eok, rok)

			if !eok {
				require.Equal(t, 0, len(res.Warnings()))
				continue Outer
			}
			sexp := expss.At()
			sres := res.At()

			require.Equal(t, sexp.Labels(), sres.Labels())

			smplExp, errExp := storage.ExpandSamples(sexp.Iterator(), nil)
			smplRes, errRes := storage.ExpandSamples(sres.Iterator(), nil)

			require.Equal(t, errExp, errRes)
			require.Equal(t, smplExp, smplRes)
		}
	}
}

func TestAmendDatapointCausesError(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		require.NoError(t, db.Close())
	}()

	ctx := context.Background()
	app := db.Appender(ctx)
	_, err := app.Append(0, labels.Labels{{Name: "a", Value: "b"}}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	app = db.Appender(ctx)
	_, err = app.Append(0, labels.Labels{{Name: "a", Value: "b"}}, 0, 1)
	require.Equal(t, storage.ErrDuplicateSampleForTimestamp, err)
	require.NoError(t, app.Rollback())
}

func TestDuplicateNaNDatapointNoAmendError(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		require.NoError(t, db.Close())
	}()

	ctx := context.Background()
	app := db.Appender(ctx)
	_, err := app.Append(0, labels.Labels{{Name: "a", Value: "b"}}, 0, math.NaN())
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	app = db.Appender(ctx)
	_, err = app.Append(0, labels.Labels{{Name: "a", Value: "b"}}, 0, math.NaN())
	require.NoError(t, err)
}

func TestNonDuplicateNaNDatapointsCausesAmendError(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		require.NoError(t, db.Close())
	}()

	ctx := context.Background()
	app := db.Appender(ctx)
	_, err := app.Append(0, labels.Labels{{Name: "a", Value: "b"}}, 0, math.Float64frombits(0x7ff0000000000001))
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	app = db.Appender(ctx)
	_, err = app.Append(0, labels.Labels{{Name: "a", Value: "b"}}, 0, math.Float64frombits(0x7ff0000000000002))
	require.Equal(t, storage.ErrDuplicateSampleForTimestamp, err)
}

func TestEmptyLabelsetCausesError(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		require.NoError(t, db.Close())
	}()

	ctx := context.Background()
	app := db.Appender(ctx)
	_, err := app.Append(0, labels.Labels{}, 0, 0)
	require.Error(t, err)
	require.Equal(t, "empty labelset: invalid sample", err.Error())
}

func TestSkippingInvalidValuesInSameTxn(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		require.NoError(t, db.Close())
	}()

	// Append AmendedValue.
	ctx := context.Background()
	app := db.Appender(ctx)
	_, err := app.Append(0, labels.Labels{{Name: "a", Value: "b"}}, 0, 1)
	require.NoError(t, err)
	_, err = app.Append(0, labels.Labels{{Name: "a", Value: "b"}}, 0, 2)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Make sure the right value is stored.
	q, err := db.Querier(context.TODO(), 0, 10)
	require.NoError(t, err)

	ssMap := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

	require.Equal(t, map[string][]tsdbutil.Sample{
		labels.New(labels.Label{Name: "a", Value: "b"}).String(): {sample{0, 1}},
	}, ssMap)

	// Append Out of Order Value.
	app = db.Appender(ctx)
	_, err = app.Append(0, labels.Labels{{Name: "a", Value: "b"}}, 10, 3)
	require.NoError(t, err)
	_, err = app.Append(0, labels.Labels{{Name: "a", Value: "b"}}, 7, 5)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	q, err = db.Querier(context.TODO(), 0, 10)
	require.NoError(t, err)

	ssMap = query(t, q, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

	require.Equal(t, map[string][]tsdbutil.Sample{
		labels.New(labels.Label{Name: "a", Value: "b"}).String(): {sample{0, 1}, sample{10, 3}},
	}, ssMap)
}

func TestDB_Snapshot(t *testing.T) {
	db := openTestDB(t, nil, nil)

	// append data
	ctx := context.Background()
	app := db.Appender(ctx)
	mint := int64(1414141414000)
	for i := 0; i < 1000; i++ {
		_, err := app.Append(0, labels.FromStrings("foo", "bar"), mint+int64(i), 1.0)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	// create snapshot
	snap := t.TempDir()
	require.NoError(t, db.Snapshot(snap, true))
	require.NoError(t, db.Close())

	// reopen DB from snapshot
	db, err := Open(snap, nil, nil, nil, nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()

	querier, err := db.Querier(context.TODO(), mint, mint+1000)
	require.NoError(t, err)
	defer func() { require.NoError(t, querier.Close()) }()

	// sum values
	seriesSet := querier.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	sum := 0.0
	for seriesSet.Next() {
		series := seriesSet.At().Iterator()
		for series.Next() {
			_, v := series.At()
			sum += v
		}
		require.NoError(t, series.Err())
	}
	require.NoError(t, seriesSet.Err())
	require.Equal(t, 0, len(seriesSet.Warnings()))
	require.Equal(t, 1000.0, sum)
}

// TestDB_Snapshot_ChunksOutsideOfCompactedRange ensures that a snapshot removes chunks samples
// that are outside the set block time range.
// See https://github.com/prometheus/prometheus/issues/5105
func TestDB_Snapshot_ChunksOutsideOfCompactedRange(t *testing.T) {
	db := openTestDB(t, nil, nil)

	ctx := context.Background()
	app := db.Appender(ctx)
	mint := int64(1414141414000)
	for i := 0; i < 1000; i++ {
		_, err := app.Append(0, labels.FromStrings("foo", "bar"), mint+int64(i), 1.0)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	snap := t.TempDir()

	// Hackingly introduce "race", by having lower max time then maxTime in last chunk.
	db.head.maxTime.Sub(10)

	require.NoError(t, db.Snapshot(snap, true))
	require.NoError(t, db.Close())

	// Reopen DB from snapshot.
	db, err := Open(snap, nil, nil, nil, nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()

	querier, err := db.Querier(context.TODO(), mint, mint+1000)
	require.NoError(t, err)
	defer func() { require.NoError(t, querier.Close()) }()

	// Sum values.
	seriesSet := querier.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	sum := 0.0
	for seriesSet.Next() {
		series := seriesSet.At().Iterator()
		for series.Next() {
			_, v := series.At()
			sum += v
		}
		require.NoError(t, series.Err())
	}
	require.NoError(t, seriesSet.Err())
	require.Equal(t, 0, len(seriesSet.Warnings()))

	// Since we snapshotted with MaxTime - 10, so expect 10 less samples.
	require.Equal(t, 1000.0-10, sum)
}

func TestDB_SnapshotWithDelete(t *testing.T) {
	numSamples := int64(10)

	db := openTestDB(t, nil, nil)
	defer func() { require.NoError(t, db.Close()) }()

	ctx := context.Background()
	app := db.Appender(ctx)

	smpls := make([]float64, numSamples)
	for i := int64(0); i < numSamples; i++ {
		smpls[i] = rand.Float64()
		app.Append(0, labels.Labels{{Name: "a", Value: "b"}}, i, smpls[i])
	}

	require.NoError(t, app.Commit())
	cases := []struct {
		intervals tombstones.Intervals
		remaint   []int64
	}{
		{
			intervals: tombstones.Intervals{{Mint: 1, Maxt: 3}, {Mint: 4, Maxt: 7}},
			remaint:   []int64{0, 8, 9},
		},
	}

Outer:
	for _, c := range cases {
		// TODO(gouthamve): Reset the tombstones somehow.
		// Delete the ranges.
		for _, r := range c.intervals {
			require.NoError(t, db.Delete(r.Mint, r.Maxt, labels.MustNewMatcher(labels.MatchEqual, "a", "b")))
		}

		// create snapshot
		snap := t.TempDir()

		require.NoError(t, db.Snapshot(snap, true))

		// reopen DB from snapshot
		newDB, err := Open(snap, nil, nil, nil, nil)
		require.NoError(t, err)
		defer func() { require.NoError(t, newDB.Close()) }()

		// Compare the result.
		q, err := newDB.Querier(context.TODO(), 0, numSamples)
		require.NoError(t, err)
		defer func() { require.NoError(t, q.Close()) }()

		res := q.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

		expSamples := make([]tsdbutil.Sample, 0, len(c.remaint))
		for _, ts := range c.remaint {
			expSamples = append(expSamples, sample{ts, smpls[ts]})
		}

		expss := newMockSeriesSet([]storage.Series{
			storage.NewListSeries(labels.FromStrings("a", "b"), expSamples),
		})

		if len(expSamples) == 0 {
			require.False(t, res.Next())
			continue
		}

		for {
			eok, rok := expss.Next(), res.Next()
			require.Equal(t, eok, rok)

			if !eok {
				require.Equal(t, 0, len(res.Warnings()))
				continue Outer
			}
			sexp := expss.At()
			sres := res.At()

			require.Equal(t, sexp.Labels(), sres.Labels())

			smplExp, errExp := storage.ExpandSamples(sexp.Iterator(), nil)
			smplRes, errRes := storage.ExpandSamples(sres.Iterator(), nil)

			require.Equal(t, errExp, errRes)
			require.Equal(t, smplExp, smplRes)
		}
	}
}

func TestDB_e2e(t *testing.T) {
	const (
		numDatapoints = 1000
		numRanges     = 1000
		timeInterval  = int64(3)
	)
	// Create 8 series with 1000 data-points of different ranges and run queries.
	lbls := []labels.Labels{
		{
			{Name: "a", Value: "b"},
			{Name: "instance", Value: "localhost:9090"},
			{Name: "job", Value: "prometheus"},
		},
		{
			{Name: "a", Value: "b"},
			{Name: "instance", Value: "127.0.0.1:9090"},
			{Name: "job", Value: "prometheus"},
		},
		{
			{Name: "a", Value: "b"},
			{Name: "instance", Value: "127.0.0.1:9090"},
			{Name: "job", Value: "prom-k8s"},
		},
		{
			{Name: "a", Value: "b"},
			{Name: "instance", Value: "localhost:9090"},
			{Name: "job", Value: "prom-k8s"},
		},
		{
			{Name: "a", Value: "c"},
			{Name: "instance", Value: "localhost:9090"},
			{Name: "job", Value: "prometheus"},
		},
		{
			{Name: "a", Value: "c"},
			{Name: "instance", Value: "127.0.0.1:9090"},
			{Name: "job", Value: "prometheus"},
		},
		{
			{Name: "a", Value: "c"},
			{Name: "instance", Value: "127.0.0.1:9090"},
			{Name: "job", Value: "prom-k8s"},
		},
		{
			{Name: "a", Value: "c"},
			{Name: "instance", Value: "localhost:9090"},
			{Name: "job", Value: "prom-k8s"},
		},
	}

	seriesMap := map[string][]tsdbutil.Sample{}
	for _, l := range lbls {
		seriesMap[labels.New(l...).String()] = []tsdbutil.Sample{}
	}

	db := openTestDB(t, nil, nil)
	defer func() {
		require.NoError(t, db.Close())
	}()

	ctx := context.Background()
	app := db.Appender(ctx)

	for _, l := range lbls {
		lset := labels.New(l...)
		series := []tsdbutil.Sample{}

		ts := rand.Int63n(300)
		for i := 0; i < numDatapoints; i++ {
			v := rand.Float64()

			series = append(series, sample{ts, v})

			_, err := app.Append(0, lset, ts, v)
			require.NoError(t, err)

			ts += rand.Int63n(timeInterval) + 1
		}

		seriesMap[lset.String()] = series
	}

	require.NoError(t, app.Commit())

	// Query each selector on 1000 random time-ranges.
	queries := []struct {
		ms []*labels.Matcher
	}{
		{
			ms: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "b")},
		},
		{
			ms: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "a", "b"),
				labels.MustNewMatcher(labels.MatchEqual, "job", "prom-k8s"),
			},
		},
		{
			ms: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "a", "c"),
				labels.MustNewMatcher(labels.MatchEqual, "instance", "localhost:9090"),
				labels.MustNewMatcher(labels.MatchEqual, "job", "prometheus"),
			},
		},
		// TODO: Add Regexp Matchers.
	}

	for _, qry := range queries {
		matched := labels.Slice{}
		for _, ls := range lbls {
			s := labels.Selector(qry.ms)
			if s.Matches(ls) {
				matched = append(matched, ls)
			}
		}

		sort.Sort(matched)

		for i := 0; i < numRanges; i++ {
			mint := rand.Int63n(300)
			maxt := mint + rand.Int63n(timeInterval*int64(numDatapoints))

			expected := map[string][]tsdbutil.Sample{}

			// Build the mockSeriesSet.
			for _, m := range matched {
				smpls := boundedSamples(seriesMap[m.String()], mint, maxt)
				if len(smpls) > 0 {
					expected[m.String()] = smpls
				}
			}

			q, err := db.Querier(context.TODO(), mint, maxt)
			require.NoError(t, err)

			ss := q.Select(false, nil, qry.ms...)
			result := map[string][]tsdbutil.Sample{}

			for ss.Next() {
				x := ss.At()

				smpls, err := storage.ExpandSamples(x.Iterator(), newSample)
				require.NoError(t, err)

				if len(smpls) > 0 {
					result[x.Labels().String()] = smpls
				}
			}

			require.NoError(t, ss.Err())
			require.Equal(t, 0, len(ss.Warnings()))
			require.Equal(t, expected, result)

			q.Close()
		}
	}
}

func TestWALFlushedOnDBClose(t *testing.T) {
	db := openTestDB(t, nil, nil)

	dirDb := db.Dir()

	lbls := labels.Labels{labels.Label{Name: "labelname", Value: "labelvalue"}}

	ctx := context.Background()
	app := db.Appender(ctx)
	_, err := app.Append(0, lbls, 0, 1)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	require.NoError(t, db.Close())

	db, err = Open(dirDb, nil, nil, nil, nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()

	q, err := db.Querier(context.TODO(), 0, 1)
	require.NoError(t, err)

	values, ws, err := q.LabelValues("labelname")
	require.NoError(t, err)
	require.Equal(t, 0, len(ws))
	require.Equal(t, []string{"labelvalue"}, values)
}

func TestWALSegmentSizeOptions(t *testing.T) {
	tests := map[int]func(dbdir string, segmentSize int){
		// Default Wal Size.
		0: func(dbDir string, segmentSize int) {
			filesAndDir, err := os.ReadDir(filepath.Join(dbDir, "wal"))
			require.NoError(t, err)
			files := []os.FileInfo{}
			for _, f := range filesAndDir {
				if !f.IsDir() {
					fi, err := f.Info()
					require.NoError(t, err)
					files = append(files, fi)
				}
			}
			// All the full segment files (all but the last) should match the segment size option.
			for _, f := range files[:len(files)-1] {
				require.Equal(t, int64(DefaultOptions().WALSegmentSize), f.Size(), "WAL file size doesn't match WALSegmentSize option, filename: %v", f.Name())
			}
			lastFile := files[len(files)-1]
			require.Greater(t, int64(DefaultOptions().WALSegmentSize), lastFile.Size(), "last WAL file size is not smaller than the WALSegmentSize option, filename: %v", lastFile.Name())
		},
		// Custom Wal Size.
		2 * 32 * 1024: func(dbDir string, segmentSize int) {
			filesAndDir, err := os.ReadDir(filepath.Join(dbDir, "wal"))
			require.NoError(t, err)
			files := []os.FileInfo{}
			for _, f := range filesAndDir {
				if !f.IsDir() {
					fi, err := f.Info()
					require.NoError(t, err)
					files = append(files, fi)
				}
			}
			require.Greater(t, len(files), 1, "current WALSegmentSize should result in more than a single WAL file.")
			// All the full segment files (all but the last) should match the segment size option.
			for _, f := range files[:len(files)-1] {
				require.Equal(t, int64(segmentSize), f.Size(), "WAL file size doesn't match WALSegmentSize option, filename: %v", f.Name())
			}
			lastFile := files[len(files)-1]
			require.Greater(t, int64(segmentSize), lastFile.Size(), "last WAL file size is not smaller than the WALSegmentSize option, filename: %v", lastFile.Name())
		},
		// Wal disabled.
		-1: func(dbDir string, segmentSize int) {
			// Check that WAL dir is not there.
			_, err := os.Stat(filepath.Join(dbDir, "wal"))
			require.Error(t, err)
			// Check that there is chunks dir.
			_, err = os.Stat(mmappedChunksDir(dbDir))
			require.NoError(t, err)
		},
	}
	for segmentSize, testFunc := range tests {
		t.Run(fmt.Sprintf("WALSegmentSize %d test", segmentSize), func(t *testing.T) {
			opts := DefaultOptions()
			opts.WALSegmentSize = segmentSize
			db := openTestDB(t, opts, nil)

			for i := int64(0); i < 155; i++ {
				app := db.Appender(context.Background())
				ref, err := app.Append(0, labels.Labels{labels.Label{Name: "wal" + fmt.Sprintf("%d", i), Value: "size"}}, i, rand.Float64())
				require.NoError(t, err)
				for j := int64(1); j <= 78; j++ {
					_, err := app.Append(ref, nil, i+j, rand.Float64())
					require.NoError(t, err)
				}
				require.NoError(t, app.Commit())
			}

			dbDir := db.Dir()
			require.NoError(t, db.Close())
			testFunc(dbDir, int(opts.WALSegmentSize))
		})
	}
}

// https://github.com/prometheus/prometheus/issues/9846
// https://github.com/prometheus/prometheus/issues/9859
func TestWALReplayRaceOnSamplesLoggedBeforeSeries(t *testing.T) {
	const (
		numRuns                        = 1
		numSamplesBeforeSeriesCreation = 1000
	)

	// We test both with few and many samples appended after series creation. If samples are < 120 then there's no
	// mmap-ed chunk, otherwise there's at least 1 mmap-ed chunk when replaying the WAL.
	for _, numSamplesAfterSeriesCreation := range []int{1, 1000} {
		for run := 1; run <= numRuns; run++ {
			t.Run(fmt.Sprintf("samples after series creation = %d, run = %d", numSamplesAfterSeriesCreation, run), func(t *testing.T) {
				testWALReplayRaceOnSamplesLoggedBeforeSeries(t, numSamplesBeforeSeriesCreation, numSamplesAfterSeriesCreation)
			})
		}
	}
}

func testWALReplayRaceOnSamplesLoggedBeforeSeries(t *testing.T, numSamplesBeforeSeriesCreation, numSamplesAfterSeriesCreation int) {
	const numSeries = 1000

	db := openTestDB(t, nil, nil)
	db.DisableCompactions()

	for seriesRef := 1; seriesRef <= numSeries; seriesRef++ {
		// Log samples before the series is logged to the WAL.
		var enc record.Encoder
		var samples []record.RefSample

		for ts := 0; ts < numSamplesBeforeSeriesCreation; ts++ {
			samples = append(samples, record.RefSample{
				Ref: chunks.HeadSeriesRef(uint64(seriesRef)),
				T:   int64(ts),
				V:   float64(ts),
			})
		}

		err := db.Head().wal.Log(enc.Samples(samples, nil))
		require.NoError(t, err)

		// Add samples via appender so that they're logged after the series in the WAL.
		app := db.Appender(context.Background())
		lbls := labels.FromStrings("series_id", strconv.Itoa(seriesRef))

		for ts := numSamplesBeforeSeriesCreation; ts < numSamplesBeforeSeriesCreation+numSamplesAfterSeriesCreation; ts++ {
			_, err := app.Append(0, lbls, int64(ts), float64(ts))
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	require.NoError(t, db.Close())

	// Reopen the DB, replaying the WAL.
	reopenDB, err := Open(db.Dir(), log.NewLogfmtLogger(os.Stderr), nil, nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, reopenDB.Close())
	})

	// Query back chunks for all series.
	q, err := reopenDB.ChunkQuerier(context.Background(), math.MinInt64, math.MaxInt64)
	require.NoError(t, err)

	set := q.Select(false, nil, labels.MustNewMatcher(labels.MatchRegexp, "series_id", ".+"))
	actualSeries := 0

	for set.Next() {
		actualSeries++
		actualChunks := 0

		chunksIt := set.At().Iterator()
		for chunksIt.Next() {
			actualChunks++
		}
		require.NoError(t, chunksIt.Err())

		// We expect 1 chunk every 120 samples after series creation.
		require.Equalf(t, (numSamplesAfterSeriesCreation/120)+1, actualChunks, "series: %s", set.At().Labels().String())
	}

	require.NoError(t, set.Err())
	require.Equal(t, numSeries, actualSeries)
}

func TestTombstoneClean(t *testing.T) {
	numSamples := int64(10)

	db := openTestDB(t, nil, nil)

	ctx := context.Background()
	app := db.Appender(ctx)

	smpls := make([]float64, numSamples)
	for i := int64(0); i < numSamples; i++ {
		smpls[i] = rand.Float64()
		app.Append(0, labels.Labels{{Name: "a", Value: "b"}}, i, smpls[i])
	}

	require.NoError(t, app.Commit())
	cases := []struct {
		intervals tombstones.Intervals
		remaint   []int64
	}{
		{
			intervals: tombstones.Intervals{{Mint: 1, Maxt: 3}, {Mint: 4, Maxt: 7}},
			remaint:   []int64{0, 8, 9},
		},
	}

	for _, c := range cases {
		// Delete the ranges.

		// Create snapshot.
		snap := t.TempDir()
		require.NoError(t, db.Snapshot(snap, true))
		require.NoError(t, db.Close())

		// Reopen DB from snapshot.
		db, err := Open(snap, nil, nil, nil, nil)
		require.NoError(t, err)
		defer db.Close()

		for _, r := range c.intervals {
			require.NoError(t, db.Delete(r.Mint, r.Maxt, labels.MustNewMatcher(labels.MatchEqual, "a", "b")))
		}

		// All of the setup for THIS line.
		require.NoError(t, db.CleanTombstones())

		// Compare the result.
		q, err := db.Querier(context.TODO(), 0, numSamples)
		require.NoError(t, err)
		defer q.Close()

		res := q.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

		expSamples := make([]tsdbutil.Sample, 0, len(c.remaint))
		for _, ts := range c.remaint {
			expSamples = append(expSamples, sample{ts, smpls[ts]})
		}

		expss := newMockSeriesSet([]storage.Series{
			storage.NewListSeries(labels.FromStrings("a", "b"), expSamples),
		})

		if len(expSamples) == 0 {
			require.False(t, res.Next())
			continue
		}

		for {
			eok, rok := expss.Next(), res.Next()
			require.Equal(t, eok, rok)

			if !eok {
				break
			}
			sexp := expss.At()
			sres := res.At()

			require.Equal(t, sexp.Labels(), sres.Labels())

			smplExp, errExp := storage.ExpandSamples(sexp.Iterator(), nil)
			smplRes, errRes := storage.ExpandSamples(sres.Iterator(), nil)

			require.Equal(t, errExp, errRes)
			require.Equal(t, smplExp, smplRes)
		}
		require.Equal(t, 0, len(res.Warnings()))

		for _, b := range db.Blocks() {
			require.Equal(t, tombstones.NewMemTombstones(), b.tombstones)
		}
	}
}

// TestTombstoneCleanResultEmptyBlock tests that a TombstoneClean that results in empty blocks (no timeseries)
// will also delete the resultant block.
func TestTombstoneCleanResultEmptyBlock(t *testing.T) {
	numSamples := int64(10)

	db := openTestDB(t, nil, nil)

	ctx := context.Background()
	app := db.Appender(ctx)

	smpls := make([]float64, numSamples)
	for i := int64(0); i < numSamples; i++ {
		smpls[i] = rand.Float64()
		app.Append(0, labels.Labels{{Name: "a", Value: "b"}}, i, smpls[i])
	}

	require.NoError(t, app.Commit())
	// Interval should cover the whole block.
	intervals := tombstones.Intervals{{Mint: 0, Maxt: numSamples}}

	// Create snapshot.
	snap := t.TempDir()
	require.NoError(t, db.Snapshot(snap, true))
	require.NoError(t, db.Close())

	// Reopen DB from snapshot.
	db, err := Open(snap, nil, nil, nil, nil)
	require.NoError(t, err)
	defer db.Close()

	// Create tombstones by deleting all samples.
	for _, r := range intervals {
		require.NoError(t, db.Delete(r.Mint, r.Maxt, labels.MustNewMatcher(labels.MatchEqual, "a", "b")))
	}

	require.NoError(t, db.CleanTombstones())

	// After cleaning tombstones that covers the entire block, no blocks should be left behind.
	actualBlockDirs, err := blockDirs(db.dir)
	require.NoError(t, err)
	require.Equal(t, 0, len(actualBlockDirs))
}

// TestTombstoneCleanFail tests that a failing TombstoneClean doesn't leave any blocks behind.
// When TombstoneClean errors the original block that should be rebuilt doesn't get deleted so
// if TombstoneClean leaves any blocks behind these will overlap.
func TestTombstoneCleanFail(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		require.NoError(t, db.Close())
	}()

	var oldBlockDirs []string

	// Create some blocks pending for compaction.
	// totalBlocks should be >=2 so we have enough blocks to trigger compaction failure.
	totalBlocks := 2
	for i := 0; i < totalBlocks; i++ {
		blockDir := createBlock(t, db.Dir(), genSeries(1, 1, int64(i), int64(i)+1))
		block, err := OpenBlock(nil, blockDir, nil)
		require.NoError(t, err)
		// Add some fake tombstones to trigger the compaction.
		tomb := tombstones.NewMemTombstones()
		tomb.AddInterval(0, tombstones.Interval{Mint: int64(i), Maxt: int64(i) + 1})
		block.tombstones = tomb

		db.blocks = append(db.blocks, block)
		oldBlockDirs = append(oldBlockDirs, blockDir)
	}

	// Initialize the mockCompactorFailing with a room for a single compaction iteration.
	// mockCompactorFailing will fail on the second iteration so we can check if the cleanup works as expected.
	db.compactor = &mockCompactorFailing{
		t:      t,
		blocks: db.blocks,
		max:    totalBlocks + 1,
	}

	// The compactor should trigger a failure here.
	require.Error(t, db.CleanTombstones())

	// Now check that the CleanTombstones replaced the old block even after a failure.
	actualBlockDirs, err := blockDirs(db.dir)
	require.NoError(t, err)
	// Only one block should have been replaced by a new block.
	require.Equal(t, len(oldBlockDirs), len(actualBlockDirs))
	require.Equal(t, len(intersection(oldBlockDirs, actualBlockDirs)), len(actualBlockDirs)-1)
}

// TestTombstoneCleanRetentionLimitsRace tests that a CleanTombstones operation
// and retention limit policies, when triggered at the same time,
// won't race against each other.
func TestTombstoneCleanRetentionLimitsRace(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	opts := DefaultOptions()
	var wg sync.WaitGroup

	// We want to make sure that a race doesn't happen when a normal reload and a CleanTombstones()
	// reload try to delete the same block. Without the correct lock placement, it can happen if a
	// block is marked for deletion due to retention limits and also has tombstones to be cleaned at
	// the same time.
	//
	// That is something tricky to trigger, so let's try several times just to make sure.
	for i := 0; i < 20; i++ {
		t.Run(fmt.Sprintf("iteration%d", i), func(t *testing.T) {
			db := openTestDB(t, opts, nil)
			totalBlocks := 20
			dbDir := db.Dir()
			// Generate some blocks with old mint (near epoch).
			for j := 0; j < totalBlocks; j++ {
				blockDir := createBlock(t, dbDir, genSeries(10, 1, int64(j), int64(j)+1))
				block, err := OpenBlock(nil, blockDir, nil)
				require.NoError(t, err)
				// Cover block with tombstones so it can be deleted with CleanTombstones() as well.
				tomb := tombstones.NewMemTombstones()
				tomb.AddInterval(0, tombstones.Interval{Mint: int64(j), Maxt: int64(j) + 1})
				block.tombstones = tomb

				db.blocks = append(db.blocks, block)
			}

			wg.Add(2)
			// Run reload and CleanTombstones together, with a small time window randomization
			go func() {
				defer wg.Done()
				time.Sleep(time.Duration(rand.Float64() * 100 * float64(time.Millisecond)))
				require.NoError(t, db.reloadBlocks())
			}()
			go func() {
				defer wg.Done()
				time.Sleep(time.Duration(rand.Float64() * 100 * float64(time.Millisecond)))
				require.NoError(t, db.CleanTombstones())
			}()

			wg.Wait()

			require.NoError(t, db.Close())
		})
	}
}

func intersection(oldBlocks, actualBlocks []string) (intersection []string) {
	hash := make(map[string]bool)
	for _, e := range oldBlocks {
		hash[e] = true
	}
	for _, e := range actualBlocks {
		// If block present in the hashmap then append intersection list.
		if hash[e] {
			intersection = append(intersection, e)
		}
	}
	return
}

// mockCompactorFailing creates a new empty block on every write and fails when reached the max allowed total.
// For CompactOOO, it always fails.
type mockCompactorFailing struct {
	t      *testing.T
	blocks []*Block
	max    int
}

func (*mockCompactorFailing) Plan(dir string) ([]string, error) {
	return nil, nil
}

func (c *mockCompactorFailing) Write(dest string, b BlockReader, mint, maxt int64, parent *BlockMeta) (ulid.ULID, error) {
	if len(c.blocks) >= c.max {
		return ulid.ULID{}, fmt.Errorf("the compactor already did the maximum allowed blocks so it is time to fail")
	}

	block, err := OpenBlock(nil, createBlock(c.t, dest, genSeries(1, 1, 0, 1)), nil)
	require.NoError(c.t, err)
	require.NoError(c.t, block.Close()) // Close block as we won't be using anywhere.
	c.blocks = append(c.blocks, block)

	// Now check that all expected blocks are actually persisted on disk.
	// This way we make sure that the we have some blocks that are supposed to be removed.
	var expectedBlocks []string
	for _, b := range c.blocks {
		expectedBlocks = append(expectedBlocks, filepath.Join(dest, b.Meta().ULID.String()))
	}
	actualBlockDirs, err := blockDirs(dest)
	require.NoError(c.t, err)

	require.Equal(c.t, expectedBlocks, actualBlockDirs)

	return block.Meta().ULID, nil
}

func (*mockCompactorFailing) Compact(string, []string, []*Block) (ulid.ULID, error) {
	return ulid.ULID{}, nil
}

func (*mockCompactorFailing) CompactOOO(dest string, oooHead *OOOCompactionHead) (result []ulid.ULID, err error) {
	return nil, fmt.Errorf("mock compaction failing CompactOOO")
}

func TestTimeRetention(t *testing.T) {
	db := openTestDB(t, nil, []int64{1000})
	defer func() {
		require.NoError(t, db.Close())
	}()

	blocks := []*BlockMeta{
		{MinTime: 500, MaxTime: 900}, // Oldest block
		{MinTime: 1000, MaxTime: 1500},
		{MinTime: 1500, MaxTime: 2000}, // Newest Block
	}

	for _, m := range blocks {
		createBlock(t, db.Dir(), genSeries(10, 10, m.MinTime, m.MaxTime))
	}

	require.NoError(t, db.reloadBlocks())           // Reload the db to register the new blocks.
	require.Equal(t, len(blocks), len(db.Blocks())) // Ensure all blocks are registered.

	db.opts.RetentionDuration = blocks[2].MaxTime - blocks[1].MinTime
	require.NoError(t, db.reloadBlocks())

	expBlocks := blocks[1:]
	actBlocks := db.Blocks()

	require.Equal(t, 1, int(prom_testutil.ToFloat64(db.metrics.timeRetentionCount)), "metric retention count mismatch")
	require.Equal(t, len(expBlocks), len(actBlocks))
	require.Equal(t, expBlocks[0].MaxTime, actBlocks[0].meta.MaxTime)
	require.Equal(t, expBlocks[len(expBlocks)-1].MaxTime, actBlocks[len(actBlocks)-1].meta.MaxTime)
}

func TestSizeRetention(t *testing.T) {
	opts := DefaultOptions()
	opts.OutOfOrderTimeWindow = 100
	db := openTestDB(t, opts, []int64{100})
	defer func() {
		require.NoError(t, db.Close())
	}()

	blocks := []*BlockMeta{
		{MinTime: 100, MaxTime: 200}, // Oldest block
		{MinTime: 200, MaxTime: 300},
		{MinTime: 300, MaxTime: 400},
		{MinTime: 400, MaxTime: 500},
		{MinTime: 500, MaxTime: 600}, // Newest Block
	}

	for _, m := range blocks {
		createBlock(t, db.Dir(), genSeries(100, 10, m.MinTime, m.MaxTime))
	}

	headBlocks := []*BlockMeta{
		{MinTime: 700, MaxTime: 800},
	}

	// Add some data to the WAL.
	headApp := db.Head().Appender(context.Background())
	var aSeries labels.Labels
	for _, m := range headBlocks {
		series := genSeries(100, 10, m.MinTime, m.MaxTime+1)
		for _, s := range series {
			aSeries = s.Labels()
			it := s.Iterator()
			for it.Next() {
				tim, v := it.At()
				_, err := headApp.Append(0, s.Labels(), tim, v)
				require.NoError(t, err)
			}
			require.NoError(t, it.Err())
		}
	}
	require.NoError(t, headApp.Commit())

	require.Eventually(t, func() bool {
		return db.Head().chunkDiskMapper.IsQueueEmpty()
	}, 2*time.Second, 100*time.Millisecond)

	// Test that registered size matches the actual disk size.
	require.NoError(t, db.reloadBlocks())                               // Reload the db to register the new db size.
	require.Equal(t, len(blocks), len(db.Blocks()))                     // Ensure all blocks are registered.
	blockSize := int64(prom_testutil.ToFloat64(db.metrics.blocksBytes)) // Use the actual internal metrics.
	walSize, err := db.Head().wal.Size()
	require.NoError(t, err)
	cdmSize, err := db.Head().chunkDiskMapper.Size()
	require.NoError(t, err)
	require.NotZero(t, cdmSize)
	// Expected size should take into account block size + WAL size + Head
	// chunks size
	expSize := blockSize + walSize + cdmSize
	actSize, err := fileutil.DirSize(db.Dir())
	require.NoError(t, err)
	require.Equal(t, expSize, actSize, "registered size doesn't match actual disk size")

	// Create a WAL checkpoint, and compare sizes.
	first, last, err := wal.Segments(db.Head().wal.Dir())
	require.NoError(t, err)
	_, err = wal.Checkpoint(log.NewNopLogger(), db.Head().wal, first, last-1, func(x chunks.HeadSeriesRef) bool { return false }, 0)
	require.NoError(t, err)
	blockSize = int64(prom_testutil.ToFloat64(db.metrics.blocksBytes)) // Use the actual internal metrics.
	walSize, err = db.Head().wal.Size()
	require.NoError(t, err)
	cdmSize, err = db.Head().chunkDiskMapper.Size()
	require.NoError(t, err)
	require.NotZero(t, cdmSize)
	expSize = blockSize + walSize + cdmSize
	actSize, err = fileutil.DirSize(db.Dir())
	require.NoError(t, err)
	require.Equal(t, expSize, actSize, "registered size doesn't match actual disk size")

	// Truncate Chunk Disk Mapper and compare sizes.
	require.NoError(t, db.Head().chunkDiskMapper.Truncate(900))
	cdmSize, err = db.Head().chunkDiskMapper.Size()
	require.NoError(t, err)
	require.NotZero(t, cdmSize)
	expSize = blockSize + walSize + cdmSize
	actSize, err = fileutil.DirSize(db.Dir())
	require.NoError(t, err)
	require.Equal(t, expSize, actSize, "registered size doesn't match actual disk size")

	// Add some out of order samples to check the size of WBL.
	headApp = db.Head().Appender(context.Background())
	for ts := int64(750); ts < 800; ts++ {
		_, err := headApp.Append(0, aSeries, ts, float64(ts))
		require.NoError(t, err)
	}
	require.NoError(t, headApp.Commit())

	walSize, err = db.Head().wal.Size()
	require.NoError(t, err)
	wblSize, err := db.Head().wbl.Size()
	require.NoError(t, err)
	require.NotZero(t, wblSize)
	cdmSize, err = db.Head().chunkDiskMapper.Size()
	require.NoError(t, err)
	expSize = blockSize + walSize + wblSize + cdmSize
	actSize, err = fileutil.DirSize(db.Dir())
	require.NoError(t, err)
	require.Equal(t, expSize, actSize, "registered size doesn't match actual disk size")

	// Decrease the max bytes limit so that a delete is triggered.
	// Check total size, total count and check that the oldest block was deleted.
	firstBlockSize := db.Blocks()[0].Size()
	sizeLimit := actSize - firstBlockSize
	db.opts.MaxBytes = sizeLimit          // Set the new db size limit one block smaller that the actual size.
	require.NoError(t, db.reloadBlocks()) // Reload the db to register the new db size.

	expBlocks := blocks[1:]
	actBlocks := db.Blocks()
	blockSize = int64(prom_testutil.ToFloat64(db.metrics.blocksBytes))
	walSize, err = db.Head().wal.Size()
	require.NoError(t, err)
	cdmSize, err = db.Head().chunkDiskMapper.Size()
	require.NoError(t, err)
	require.NotZero(t, cdmSize)
	// Expected size should take into account block size + WAL size + WBL size
	expSize = blockSize + walSize + wblSize + cdmSize
	actRetentionCount := int(prom_testutil.ToFloat64(db.metrics.sizeRetentionCount))
	actSize, err = fileutil.DirSize(db.Dir())
	require.NoError(t, err)

	require.Equal(t, 1, actRetentionCount, "metric retention count mismatch")
	require.Equal(t, actSize, expSize, "metric db size doesn't match actual disk size")
	require.LessOrEqual(t, expSize, sizeLimit, "actual size (%v) is expected to be less than or equal to limit (%v)", expSize, sizeLimit)
	require.Equal(t, len(blocks)-1, len(actBlocks), "new block count should be decreased from:%v to:%v", len(blocks), len(blocks)-1)
	require.Equal(t, expBlocks[0].MaxTime, actBlocks[0].meta.MaxTime, "maxT mismatch of the first block")
	require.Equal(t, expBlocks[len(expBlocks)-1].MaxTime, actBlocks[len(actBlocks)-1].meta.MaxTime, "maxT mismatch of the last block")
}

func TestSizeRetentionMetric(t *testing.T) {
	cases := []struct {
		maxBytes    int64
		expMaxBytes int64
	}{
		{maxBytes: 1000, expMaxBytes: 1000},
		{maxBytes: 0, expMaxBytes: 0},
		{maxBytes: -1000, expMaxBytes: 0},
	}

	for _, c := range cases {
		db := openTestDB(t, &Options{
			MaxBytes: c.maxBytes,
		}, []int64{100})
		defer func() {
			require.NoError(t, db.Close())
		}()

		actMaxBytes := int64(prom_testutil.ToFloat64(db.metrics.maxBytes))
		require.Equal(t, actMaxBytes, c.expMaxBytes, "metric retention limit bytes mismatch")
	}
}

func TestNotMatcherSelectsLabelsUnsetSeries(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		require.NoError(t, db.Close())
	}()

	labelpairs := []labels.Labels{
		labels.FromStrings("a", "abcd", "b", "abcde"),
		labels.FromStrings("labelname", "labelvalue"),
	}

	ctx := context.Background()
	app := db.Appender(ctx)
	for _, lbls := range labelpairs {
		_, err := app.Append(0, lbls, 0, 1)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	cases := []struct {
		selector labels.Selector
		series   []labels.Labels
	}{{
		selector: labels.Selector{
			labels.MustNewMatcher(labels.MatchNotEqual, "lname", "lvalue"),
		},
		series: labelpairs,
	}, {
		selector: labels.Selector{
			labels.MustNewMatcher(labels.MatchEqual, "a", "abcd"),
			labels.MustNewMatcher(labels.MatchNotEqual, "b", "abcde"),
		},
		series: []labels.Labels{},
	}, {
		selector: labels.Selector{
			labels.MustNewMatcher(labels.MatchEqual, "a", "abcd"),
			labels.MustNewMatcher(labels.MatchNotEqual, "b", "abc"),
		},
		series: []labels.Labels{labelpairs[0]},
	}, {
		selector: labels.Selector{
			labels.MustNewMatcher(labels.MatchNotRegexp, "a", "abd.*"),
		},
		series: labelpairs,
	}, {
		selector: labels.Selector{
			labels.MustNewMatcher(labels.MatchNotRegexp, "a", "abc.*"),
		},
		series: labelpairs[1:],
	}, {
		selector: labels.Selector{
			labels.MustNewMatcher(labels.MatchNotRegexp, "c", "abd.*"),
		},
		series: labelpairs,
	}, {
		selector: labels.Selector{
			labels.MustNewMatcher(labels.MatchNotRegexp, "labelname", "labelvalue"),
		},
		series: labelpairs[:1],
	}}

	q, err := db.Querier(context.TODO(), 0, 10)
	require.NoError(t, err)
	defer func() { require.NoError(t, q.Close()) }()

	for _, c := range cases {
		ss := q.Select(false, nil, c.selector...)
		lres, _, ws, err := expandSeriesSet(ss)
		require.NoError(t, err)
		require.Equal(t, 0, len(ws))
		require.Equal(t, c.series, lres)
	}
}

// expandSeriesSet returns the raw labels in the order they are retrieved from
// the series set and the samples keyed by Labels().String().
func expandSeriesSet(ss storage.SeriesSet) ([]labels.Labels, map[string][]sample, storage.Warnings, error) {
	resultLabels := []labels.Labels{}
	resultSamples := map[string][]sample{}
	for ss.Next() {
		series := ss.At()
		samples := []sample{}
		it := series.Iterator()
		for it.Next() {
			t, v := it.At()
			samples = append(samples, sample{t: t, v: v})
		}
		resultLabels = append(resultLabels, series.Labels())
		resultSamples[series.Labels().String()] = samples
	}
	return resultLabels, resultSamples, ss.Warnings(), ss.Err()
}

func TestOverlappingBlocksDetectsAllOverlaps(t *testing.T) {
	// Create 10 blocks that does not overlap (0-10, 10-20, ..., 100-110) but in reverse order to ensure our algorithm
	// will handle that.
	metas := make([]BlockMeta, 11)
	for i := 10; i >= 0; i-- {
		metas[i] = BlockMeta{MinTime: int64(i * 10), MaxTime: int64((i + 1) * 10)}
	}

	require.Equal(t, 0, len(OverlappingBlocks(metas)), "we found unexpected overlaps")

	// Add overlapping blocks. We've to establish order again since we aren't interested
	// in trivial overlaps caused by unorderedness.
	add := func(ms ...BlockMeta) []BlockMeta {
		repl := append(append([]BlockMeta{}, metas...), ms...)
		sort.Slice(repl, func(i, j int) bool {
			return repl[i].MinTime < repl[j].MinTime
		})
		return repl
	}

	// o1 overlaps with 10-20.
	o1 := BlockMeta{MinTime: 15, MaxTime: 17}
	require.Equal(t, Overlaps{
		{Min: 15, Max: 17}: {metas[1], o1},
	}, OverlappingBlocks(add(o1)))

	// o2 overlaps with 20-30 and 30-40.
	o2 := BlockMeta{MinTime: 21, MaxTime: 31}
	require.Equal(t, Overlaps{
		{Min: 21, Max: 30}: {metas[2], o2},
		{Min: 30, Max: 31}: {o2, metas[3]},
	}, OverlappingBlocks(add(o2)))

	// o3a and o3b overlaps with 30-40 and each other.
	o3a := BlockMeta{MinTime: 33, MaxTime: 39}
	o3b := BlockMeta{MinTime: 34, MaxTime: 36}
	require.Equal(t, Overlaps{
		{Min: 34, Max: 36}: {metas[3], o3a, o3b},
	}, OverlappingBlocks(add(o3a, o3b)))

	// o4 is 1:1 overlap with 50-60.
	o4 := BlockMeta{MinTime: 50, MaxTime: 60}
	require.Equal(t, Overlaps{
		{Min: 50, Max: 60}: {metas[5], o4},
	}, OverlappingBlocks(add(o4)))

	// o5 overlaps with 60-70, 70-80 and 80-90.
	o5 := BlockMeta{MinTime: 61, MaxTime: 85}
	require.Equal(t, Overlaps{
		{Min: 61, Max: 70}: {metas[6], o5},
		{Min: 70, Max: 80}: {o5, metas[7]},
		{Min: 80, Max: 85}: {o5, metas[8]},
	}, OverlappingBlocks(add(o5)))

	// o6a overlaps with 90-100, 100-110 and o6b, o6b overlaps with 90-100 and o6a.
	o6a := BlockMeta{MinTime: 92, MaxTime: 105}
	o6b := BlockMeta{MinTime: 94, MaxTime: 99}
	require.Equal(t, Overlaps{
		{Min: 94, Max: 99}:   {metas[9], o6a, o6b},
		{Min: 100, Max: 105}: {o6a, metas[10]},
	}, OverlappingBlocks(add(o6a, o6b)))

	// All together.
	require.Equal(t, Overlaps{
		{Min: 15, Max: 17}: {metas[1], o1},
		{Min: 21, Max: 30}: {metas[2], o2}, {Min: 30, Max: 31}: {o2, metas[3]},
		{Min: 34, Max: 36}: {metas[3], o3a, o3b},
		{Min: 50, Max: 60}: {metas[5], o4},
		{Min: 61, Max: 70}: {metas[6], o5}, {Min: 70, Max: 80}: {o5, metas[7]}, {Min: 80, Max: 85}: {o5, metas[8]},
		{Min: 94, Max: 99}: {metas[9], o6a, o6b}, {Min: 100, Max: 105}: {o6a, metas[10]},
	}, OverlappingBlocks(add(o1, o2, o3a, o3b, o4, o5, o6a, o6b)))

	// Additional case.
	var nc1 []BlockMeta
	nc1 = append(nc1, BlockMeta{MinTime: 1, MaxTime: 5})
	nc1 = append(nc1, BlockMeta{MinTime: 2, MaxTime: 3})
	nc1 = append(nc1, BlockMeta{MinTime: 2, MaxTime: 3})
	nc1 = append(nc1, BlockMeta{MinTime: 2, MaxTime: 3})
	nc1 = append(nc1, BlockMeta{MinTime: 2, MaxTime: 3})
	nc1 = append(nc1, BlockMeta{MinTime: 2, MaxTime: 6})
	nc1 = append(nc1, BlockMeta{MinTime: 3, MaxTime: 5})
	nc1 = append(nc1, BlockMeta{MinTime: 5, MaxTime: 7})
	nc1 = append(nc1, BlockMeta{MinTime: 7, MaxTime: 10})
	nc1 = append(nc1, BlockMeta{MinTime: 8, MaxTime: 9})
	require.Equal(t, Overlaps{
		{Min: 2, Max: 3}: {nc1[0], nc1[1], nc1[2], nc1[3], nc1[4], nc1[5]}, // 1-5, 2-3, 2-3, 2-3, 2-3, 2,6
		{Min: 3, Max: 5}: {nc1[0], nc1[5], nc1[6]},                         // 1-5, 2-6, 3-5
		{Min: 5, Max: 6}: {nc1[5], nc1[7]},                                 // 2-6, 5-7
		{Min: 8, Max: 9}: {nc1[8], nc1[9]},                                 // 7-10, 8-9
	}, OverlappingBlocks(nc1))
}

// Regression test for https://github.com/prometheus/tsdb/issues/347
func TestChunkAtBlockBoundary(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		require.NoError(t, db.Close())
	}()

	ctx := context.Background()
	app := db.Appender(ctx)

	blockRange := db.compactor.(*LeveledCompactor).ranges[0]
	label := labels.FromStrings("foo", "bar")

	for i := int64(0); i < 3; i++ {
		_, err := app.Append(0, label, i*blockRange, 0)
		require.NoError(t, err)
		_, err = app.Append(0, label, i*blockRange+1000, 0)
		require.NoError(t, err)
	}

	err := app.Commit()
	require.NoError(t, err)

	err = db.Compact()
	require.NoError(t, err)

	for _, block := range db.Blocks() {
		r, err := block.Index()
		require.NoError(t, err)
		defer r.Close()

		meta := block.Meta()

		k, v := index.AllPostingsKey()
		p, err := r.Postings(k, v)
		require.NoError(t, err)

		var (
			lset labels.Labels
			chks []chunks.Meta
		)

		chunkCount := 0

		for p.Next() {
			err = r.Series(p.At(), &lset, &chks)
			require.NoError(t, err)
			for _, c := range chks {
				require.True(t, meta.MinTime <= c.MinTime && c.MaxTime <= meta.MaxTime,
					"chunk spans beyond block boundaries: [block.MinTime=%d, block.MaxTime=%d]; [chunk.MinTime=%d, chunk.MaxTime=%d]",
					meta.MinTime, meta.MaxTime, c.MinTime, c.MaxTime)
				chunkCount++
			}
		}
		require.Equal(t, 1, chunkCount, "expected 1 chunk in block %s, got %d", meta.ULID, chunkCount)
	}
}

func TestQuerierWithBoundaryChunks(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		require.NoError(t, db.Close())
	}()

	ctx := context.Background()
	app := db.Appender(ctx)

	blockRange := db.compactor.(*LeveledCompactor).ranges[0]
	label := labels.FromStrings("foo", "bar")

	for i := int64(0); i < 5; i++ {
		_, err := app.Append(0, label, i*blockRange, 0)
		require.NoError(t, err)
		_, err = app.Append(0, labels.FromStrings("blockID", strconv.FormatInt(i, 10)), i*blockRange, 0)
		require.NoError(t, err)
	}

	err := app.Commit()
	require.NoError(t, err)

	err = db.Compact()
	require.NoError(t, err)

	require.GreaterOrEqual(t, len(db.blocks), 3, "invalid test, less than three blocks in DB")

	q, err := db.Querier(context.TODO(), blockRange, 2*blockRange)
	require.NoError(t, err)
	defer q.Close()

	// The requested interval covers 2 blocks, so the querier's label values for blockID should give us 2 values, one from each block.
	b, ws, err := q.LabelValues("blockID")
	require.NoError(t, err)
	require.Equal(t, storage.Warnings(nil), ws)
	require.Equal(t, []string{"1", "2"}, b)
}

// TestInitializeHeadTimestamp ensures that the h.minTime is set properly.
// 	- no blocks no WAL: set to the time of the first  appended sample
// 	- no blocks with WAL: set to the smallest sample from the WAL
//	- with blocks no WAL: set to the last block maxT
// 	- with blocks with WAL: same as above
func TestInitializeHeadTimestamp(t *testing.T) {
	t.Run("clean", func(t *testing.T) {
		dir := t.TempDir()

		db, err := Open(dir, nil, nil, nil, nil)
		require.NoError(t, err)
		defer db.Close()

		// Should be set to init values if no WAL or blocks exist so far.
		require.Equal(t, int64(math.MaxInt64), db.head.MinTime())
		require.Equal(t, int64(math.MinInt64), db.head.MaxTime())

		// First added sample initializes the writable range.
		ctx := context.Background()
		app := db.Appender(ctx)
		_, err = app.Append(0, labels.FromStrings("a", "b"), 1000, 1)
		require.NoError(t, err)

		require.Equal(t, int64(1000), db.head.MinTime())
		require.Equal(t, int64(1000), db.head.MaxTime())
	})
	t.Run("wal-only", func(t *testing.T) {
		dir := t.TempDir()

		require.NoError(t, os.MkdirAll(path.Join(dir, "wal"), 0o777))
		w, err := wal.New(nil, nil, path.Join(dir, "wal"), false)
		require.NoError(t, err)

		var enc record.Encoder
		err = w.Log(
			enc.Series([]record.RefSeries{
				{Ref: 123, Labels: labels.FromStrings("a", "1")},
				{Ref: 124, Labels: labels.FromStrings("a", "2")},
			}, nil),
			enc.Samples([]record.RefSample{
				{Ref: 123, T: 5000, V: 1},
				{Ref: 124, T: 15000, V: 1},
			}, nil),
		)
		require.NoError(t, err)
		require.NoError(t, w.Close())

		db, err := Open(dir, nil, nil, nil, nil)
		require.NoError(t, err)
		defer db.Close()

		require.Equal(t, int64(5000), db.head.MinTime())
		require.Equal(t, int64(15000), db.head.MaxTime())
	})
	t.Run("existing-block", func(t *testing.T) {
		dir := t.TempDir()

		createBlock(t, dir, genSeries(1, 1, 1000, 2000))

		db, err := Open(dir, nil, nil, nil, nil)
		require.NoError(t, err)
		defer db.Close()

		require.Equal(t, int64(2000), db.head.MinTime())
		require.Equal(t, int64(2000), db.head.MaxTime())
	})
	t.Run("existing-block-and-wal", func(t *testing.T) {
		dir := t.TempDir()

		createBlock(t, dir, genSeries(1, 1, 1000, 6000))

		require.NoError(t, os.MkdirAll(path.Join(dir, "wal"), 0o777))
		w, err := wal.New(nil, nil, path.Join(dir, "wal"), false)
		require.NoError(t, err)

		var enc record.Encoder
		err = w.Log(
			enc.Series([]record.RefSeries{
				{Ref: 123, Labels: labels.FromStrings("a", "1")},
				{Ref: 124, Labels: labels.FromStrings("a", "2")},
			}, nil),
			enc.Samples([]record.RefSample{
				{Ref: 123, T: 5000, V: 1},
				{Ref: 124, T: 15000, V: 1},
			}, nil),
		)
		require.NoError(t, err)
		require.NoError(t, w.Close())

		r := prometheus.NewRegistry()

		db, err := Open(dir, nil, r, nil, nil)
		require.NoError(t, err)
		defer db.Close()

		require.Equal(t, int64(6000), db.head.MinTime())
		require.Equal(t, int64(15000), db.head.MaxTime())
		// Check that old series has been GCed.
		require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.series))
	})
}

func TestNoEmptyBlocks(t *testing.T) {
	db := openTestDB(t, nil, []int64{100})
	ctx := context.Background()
	defer func() {
		require.NoError(t, db.Close())
	}()
	db.DisableCompactions()

	rangeToTriggerCompaction := db.compactor.(*LeveledCompactor).ranges[0]/2*3 - 1
	defaultLabel := labels.FromStrings("foo", "bar")
	defaultMatcher := labels.MustNewMatcher(labels.MatchRegexp, "", ".*")

	t.Run("Test no blocks after compact with empty head.", func(t *testing.T) {
		require.NoError(t, db.Compact())
		actBlocks, err := blockDirs(db.Dir())
		require.NoError(t, err)
		require.Equal(t, len(db.Blocks()), len(actBlocks))
		require.Equal(t, 0, len(actBlocks))
		require.Equal(t, 0, int(prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.ran)), "no compaction should be triggered here")
	})

	t.Run("Test no blocks after deleting all samples from head.", func(t *testing.T) {
		app := db.Appender(ctx)
		_, err := app.Append(0, defaultLabel, 1, 0)
		require.NoError(t, err)
		_, err = app.Append(0, defaultLabel, 2, 0)
		require.NoError(t, err)
		_, err = app.Append(0, defaultLabel, 3+rangeToTriggerCompaction, 0)
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		require.NoError(t, db.Delete(math.MinInt64, math.MaxInt64, defaultMatcher))
		require.NoError(t, db.Compact())
		require.Equal(t, 1, int(prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.ran)), "compaction should have been triggered here")

		actBlocks, err := blockDirs(db.Dir())
		require.NoError(t, err)
		require.Equal(t, len(db.Blocks()), len(actBlocks))
		require.Equal(t, 0, len(actBlocks))

		app = db.Appender(ctx)
		_, err = app.Append(0, defaultLabel, 1, 0)
		require.Equal(t, storage.ErrOutOfBounds, err, "the head should be truncated so no samples in the past should be allowed")

		// Adding new blocks.
		currentTime := db.Head().MaxTime()
		_, err = app.Append(0, defaultLabel, currentTime, 0)
		require.NoError(t, err)
		_, err = app.Append(0, defaultLabel, currentTime+1, 0)
		require.NoError(t, err)
		_, err = app.Append(0, defaultLabel, currentTime+rangeToTriggerCompaction, 0)
		require.NoError(t, err)
		require.NoError(t, app.Commit())

		require.NoError(t, db.Compact())
		require.Equal(t, 2, int(prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.ran)), "compaction should have been triggered here")
		actBlocks, err = blockDirs(db.Dir())
		require.NoError(t, err)
		require.Equal(t, len(db.Blocks()), len(actBlocks))
		require.Equal(t, 1, len(actBlocks), "No blocks created when compacting with >0 samples")
	})

	t.Run(`When no new block is created from head, and there are some blocks on disk
	compaction should not run into infinite loop (was seen during development).`, func(t *testing.T) {
		oldBlocks := db.Blocks()
		app := db.Appender(ctx)
		currentTime := db.Head().MaxTime()
		_, err := app.Append(0, defaultLabel, currentTime, 0)
		require.NoError(t, err)
		_, err = app.Append(0, defaultLabel, currentTime+1, 0)
		require.NoError(t, err)
		_, err = app.Append(0, defaultLabel, currentTime+rangeToTriggerCompaction, 0)
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		require.NoError(t, db.head.Delete(math.MinInt64, math.MaxInt64, defaultMatcher))
		require.NoError(t, db.Compact())
		require.Equal(t, 3, int(prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.ran)), "compaction should have been triggered here")
		require.Equal(t, oldBlocks, db.Blocks())
	})

	t.Run("Test no blocks remaining after deleting all samples from disk.", func(t *testing.T) {
		currentTime := db.Head().MaxTime()
		blocks := []*BlockMeta{
			{MinTime: currentTime, MaxTime: currentTime + db.compactor.(*LeveledCompactor).ranges[0]},
			{MinTime: currentTime + 100, MaxTime: currentTime + 100 + db.compactor.(*LeveledCompactor).ranges[0]},
		}
		for _, m := range blocks {
			createBlock(t, db.Dir(), genSeries(2, 2, m.MinTime, m.MaxTime))
		}

		oldBlocks := db.Blocks()
		require.NoError(t, db.reloadBlocks())                          // Reload the db to register the new blocks.
		require.Equal(t, len(blocks)+len(oldBlocks), len(db.Blocks())) // Ensure all blocks are registered.
		require.NoError(t, db.Delete(math.MinInt64, math.MaxInt64, defaultMatcher))
		require.NoError(t, db.Compact())
		require.Equal(t, 5, int(prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.ran)), "compaction should have been triggered here once for each block that have tombstones")

		actBlocks, err := blockDirs(db.Dir())
		require.NoError(t, err)
		require.Equal(t, len(db.Blocks()), len(actBlocks))
		require.Equal(t, 1, len(actBlocks), "All samples are deleted. Only the most recent block should remain after compaction.")
	})
}

func TestDB_LabelNames(t *testing.T) {
	tests := []struct {
		// Add 'sampleLabels1' -> Test Head -> Compact -> Test Disk ->
		// -> Add 'sampleLabels2' -> Test Head+Disk

		sampleLabels1 [][2]string // For checking head and disk separately.
		// To test Head+Disk, sampleLabels2 should have
		// at least 1 unique label name which is not in sampleLabels1.
		sampleLabels2 [][2]string // // For checking head and disk together.
		exp1          []string    // after adding sampleLabels1.
		exp2          []string    // after adding sampleLabels1 and sampleLabels2.
	}{
		{
			sampleLabels1: [][2]string{
				{"name1", "1"},
				{"name3", "3"},
				{"name2", "2"},
			},
			sampleLabels2: [][2]string{
				{"name4", "4"},
				{"name1", "1"},
			},
			exp1: []string{"name1", "name2", "name3"},
			exp2: []string{"name1", "name2", "name3", "name4"},
		},
		{
			sampleLabels1: [][2]string{
				{"name2", "2"},
				{"name1", "1"},
				{"name2", "2"},
			},
			sampleLabels2: [][2]string{
				{"name6", "6"},
				{"name0", "0"},
			},
			exp1: []string{"name1", "name2"},
			exp2: []string{"name0", "name1", "name2", "name6"},
		},
	}

	blockRange := int64(1000)
	// Appends samples into the database.
	appendSamples := func(db *DB, mint, maxt int64, sampleLabels [][2]string) {
		t.Helper()
		ctx := context.Background()
		app := db.Appender(ctx)
		for i := mint; i <= maxt; i++ {
			for _, tuple := range sampleLabels {
				label := labels.FromStrings(tuple[0], tuple[1])
				_, err := app.Append(0, label, i*blockRange, 0)
				require.NoError(t, err)
			}
		}
		err := app.Commit()
		require.NoError(t, err)
	}
	for _, tst := range tests {
		db := openTestDB(t, nil, nil)
		defer func() {
			require.NoError(t, db.Close())
		}()

		appendSamples(db, 0, 4, tst.sampleLabels1)

		// Testing head.
		headIndexr, err := db.head.Index()
		require.NoError(t, err)
		labelNames, err := headIndexr.LabelNames()
		require.NoError(t, err)
		require.Equal(t, tst.exp1, labelNames)
		require.NoError(t, headIndexr.Close())

		// Testing disk.
		err = db.Compact()
		require.NoError(t, err)
		// All blocks have same label names, hence check them individually.
		// No need to aggregate and check.
		for _, b := range db.Blocks() {
			blockIndexr, err := b.Index()
			require.NoError(t, err)
			labelNames, err = blockIndexr.LabelNames()
			require.NoError(t, err)
			require.Equal(t, tst.exp1, labelNames)
			require.NoError(t, blockIndexr.Close())
		}

		// Adding more samples to head with new label names
		// so that we can test (head+disk).LabelNames() (the union).
		appendSamples(db, 5, 9, tst.sampleLabels2)

		// Testing DB (union).
		q, err := db.Querier(context.TODO(), math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		var ws storage.Warnings
		labelNames, ws, err = q.LabelNames()
		require.NoError(t, err)
		require.Equal(t, 0, len(ws))
		require.NoError(t, q.Close())
		require.Equal(t, tst.exp2, labelNames)
	}
}

func TestCorrectNumTombstones(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		require.NoError(t, db.Close())
	}()

	blockRange := db.compactor.(*LeveledCompactor).ranges[0]
	defaultLabel := labels.FromStrings("foo", "bar")
	defaultMatcher := labels.MustNewMatcher(labels.MatchEqual, defaultLabel[0].Name, defaultLabel[0].Value)

	ctx := context.Background()
	app := db.Appender(ctx)
	for i := int64(0); i < 3; i++ {
		for j := int64(0); j < 15; j++ {
			_, err := app.Append(0, defaultLabel, i*blockRange+j, 0)
			require.NoError(t, err)
		}
	}
	require.NoError(t, app.Commit())

	err := db.Compact()
	require.NoError(t, err)
	require.Equal(t, 1, len(db.blocks))

	require.NoError(t, db.Delete(0, 1, defaultMatcher))
	require.Equal(t, uint64(1), db.blocks[0].meta.Stats.NumTombstones)

	// {0, 1} and {2, 3} are merged to form 1 tombstone.
	require.NoError(t, db.Delete(2, 3, defaultMatcher))
	require.Equal(t, uint64(1), db.blocks[0].meta.Stats.NumTombstones)

	require.NoError(t, db.Delete(5, 6, defaultMatcher))
	require.Equal(t, uint64(2), db.blocks[0].meta.Stats.NumTombstones)

	require.NoError(t, db.Delete(9, 11, defaultMatcher))
	require.Equal(t, uint64(3), db.blocks[0].meta.Stats.NumTombstones)
}

// TestBlockRanges checks the following use cases:
//  - No samples can be added with timestamps lower than the last block maxt.
//  - The compactor doesn't create overlapping blocks
// even when the last blocks is not within the default boundaries.
//	- Lower boundary is based on the smallest sample in the head and
// upper boundary is rounded to the configured block range.
//
// This ensures that a snapshot that includes the head and creates a block with a custom time range
// will not overlap with the first block created by the next compaction.
func TestBlockRanges(t *testing.T) {
	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	ctx := context.Background()

	dir := t.TempDir()

	// Test that the compactor doesn't create overlapping blocks
	// when a non standard block already exists.
	firstBlockMaxT := int64(3)
	createBlock(t, dir, genSeries(1, 1, 0, firstBlockMaxT))
	db, err := open(dir, logger, nil, DefaultOptions(), []int64{10000}, nil)
	require.NoError(t, err)

	rangeToTriggerCompaction := db.compactor.(*LeveledCompactor).ranges[0]/2*3 + 1

	app := db.Appender(ctx)
	lbl := labels.Labels{{Name: "a", Value: "b"}}
	_, err = app.Append(0, lbl, firstBlockMaxT-1, rand.Float64())
	if err == nil {
		t.Fatalf("appending a sample with a timestamp covered by a previous block shouldn't be possible")
	}
	_, err = app.Append(0, lbl, firstBlockMaxT+1, rand.Float64())
	require.NoError(t, err)
	_, err = app.Append(0, lbl, firstBlockMaxT+2, rand.Float64())
	require.NoError(t, err)
	secondBlockMaxt := firstBlockMaxT + rangeToTriggerCompaction
	_, err = app.Append(0, lbl, secondBlockMaxt, rand.Float64()) // Add samples to trigger a new compaction

	require.NoError(t, err)
	require.NoError(t, app.Commit())
	for x := 0; x < 100; x++ {
		if len(db.Blocks()) == 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, 2, len(db.Blocks()), "no new block created after the set timeout")

	if db.Blocks()[0].Meta().MaxTime > db.Blocks()[1].Meta().MinTime {
		t.Fatalf("new block overlaps  old:%v,new:%v", db.Blocks()[0].Meta(), db.Blocks()[1].Meta())
	}

	// Test that wal records are skipped when an existing block covers the same time ranges
	// and compaction doesn't create an overlapping block.
	app = db.Appender(ctx)
	db.DisableCompactions()
	_, err = app.Append(0, lbl, secondBlockMaxt+1, rand.Float64())
	require.NoError(t, err)
	_, err = app.Append(0, lbl, secondBlockMaxt+2, rand.Float64())
	require.NoError(t, err)
	_, err = app.Append(0, lbl, secondBlockMaxt+3, rand.Float64())
	require.NoError(t, err)
	_, err = app.Append(0, lbl, secondBlockMaxt+4, rand.Float64())
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.NoError(t, db.Close())

	thirdBlockMaxt := secondBlockMaxt + 2
	createBlock(t, dir, genSeries(1, 1, secondBlockMaxt+1, thirdBlockMaxt))

	db, err = open(dir, logger, nil, DefaultOptions(), []int64{10000}, nil)
	require.NoError(t, err)

	defer db.Close()
	require.Equal(t, 3, len(db.Blocks()), "db doesn't include expected number of blocks")
	require.Equal(t, db.Blocks()[2].Meta().MaxTime, thirdBlockMaxt, "unexpected maxt of the last block")

	app = db.Appender(ctx)
	_, err = app.Append(0, lbl, thirdBlockMaxt+rangeToTriggerCompaction, rand.Float64()) // Trigger a compaction
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	for x := 0; x < 100; x++ {
		if len(db.Blocks()) == 4 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	require.Equal(t, 4, len(db.Blocks()), "no new block created after the set timeout")

	if db.Blocks()[2].Meta().MaxTime > db.Blocks()[3].Meta().MinTime {
		t.Fatalf("new block overlaps  old:%v,new:%v", db.Blocks()[2].Meta(), db.Blocks()[3].Meta())
	}
}

// TestDBReadOnly ensures that opening a DB in readonly mode doesn't modify any files on the disk.
// It also checks that the API calls return equivalent results as a normal db.Open() mode.
func TestDBReadOnly(t *testing.T) {
	var (
		dbDir     string
		logger    = log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
		expBlocks []*Block
		expSeries map[string][]tsdbutil.Sample
		expChunks map[string][]chunks.Meta
		expDBHash []byte
		matchAll  = labels.MustNewMatcher(labels.MatchEqual, "", "")
		err       error
	)

	// Bootstrap the db.
	{
		dbDir = t.TempDir()

		dbBlocks := []*BlockMeta{
			// Create three 2-sample blocks.
			{MinTime: 10, MaxTime: 12},
			{MinTime: 12, MaxTime: 14},
			{MinTime: 14, MaxTime: 16},
		}

		for _, m := range dbBlocks {
			_ = createBlock(t, dbDir, genSeries(1, 1, m.MinTime, m.MaxTime))
		}

		// Add head to test DBReadOnly WAL reading capabilities.
		w, err := wal.New(logger, nil, filepath.Join(dbDir, "wal"), true)
		require.NoError(t, err)
		h := createHead(t, w, genSeries(1, 1, 16, 18), dbDir)
		require.NoError(t, h.Close())
	}

	// Open a normal db to use for a comparison.
	{
		dbWritable, err := Open(dbDir, logger, nil, nil, nil)
		require.NoError(t, err)
		dbWritable.DisableCompactions()

		dbSizeBeforeAppend, err := fileutil.DirSize(dbWritable.Dir())
		require.NoError(t, err)
		app := dbWritable.Appender(context.Background())
		_, err = app.Append(0, labels.FromStrings("foo", "bar"), dbWritable.Head().MaxTime()+1, 0)
		require.NoError(t, err)
		require.NoError(t, app.Commit())

		expBlocks = dbWritable.Blocks()
		expDbSize, err := fileutil.DirSize(dbWritable.Dir())
		require.NoError(t, err)
		require.Greater(t, expDbSize, dbSizeBeforeAppend, "db size didn't increase after an append")

		q, err := dbWritable.Querier(context.TODO(), math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		expSeries = query(t, q, matchAll)
		cq, err := dbWritable.ChunkQuerier(context.TODO(), math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		expChunks = queryChunks(t, cq, matchAll)

		require.NoError(t, dbWritable.Close()) // Close here to allow getting the dir hash for windows.
		expDBHash = testutil.DirHash(t, dbWritable.Dir())
	}

	// Open a read only db and ensure that the API returns the same result as the normal DB.
	dbReadOnly, err := OpenDBReadOnly(dbDir, logger)
	require.NoError(t, err)
	defer func() { require.NoError(t, dbReadOnly.Close()) }()

	t.Run("blocks", func(t *testing.T) {
		blocks, err := dbReadOnly.Blocks()
		require.NoError(t, err)
		require.Equal(t, len(expBlocks), len(blocks))
		for i, expBlock := range expBlocks {
			require.Equal(t, expBlock.Meta(), blocks[i].Meta(), "block meta mismatch")
		}
	})

	t.Run("querier", func(t *testing.T) {
		// Open a read only db and ensure that the API returns the same result as the normal DB.
		q, err := dbReadOnly.Querier(context.TODO(), math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		readOnlySeries := query(t, q, matchAll)
		readOnlyDBHash := testutil.DirHash(t, dbDir)

		require.Equal(t, len(expSeries), len(readOnlySeries), "total series mismatch")
		require.Equal(t, expSeries, readOnlySeries, "series mismatch")
		require.Equal(t, expDBHash, readOnlyDBHash, "after all read operations the db hash should remain the same")
	})
	t.Run("chunk querier", func(t *testing.T) {
		cq, err := dbReadOnly.ChunkQuerier(context.TODO(), math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		readOnlySeries := queryChunks(t, cq, matchAll)
		readOnlyDBHash := testutil.DirHash(t, dbDir)

		require.Equal(t, len(expChunks), len(readOnlySeries), "total series mismatch")
		require.Equal(t, expChunks, readOnlySeries, "series chunks mismatch")
		require.Equal(t, expDBHash, readOnlyDBHash, "after all read operations the db hash should remain the same")
	})
}

// TestDBReadOnlyClosing ensures that after closing the db
// all api methods return an ErrClosed.
func TestDBReadOnlyClosing(t *testing.T) {
	dbDir := t.TempDir()
	db, err := OpenDBReadOnly(dbDir, log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr)))
	require.NoError(t, err)
	require.NoError(t, db.Close())
	require.Equal(t, db.Close(), ErrClosed)
	_, err = db.Blocks()
	require.Equal(t, err, ErrClosed)
	_, err = db.Querier(context.TODO(), 0, 1)
	require.Equal(t, err, ErrClosed)
}

func TestDBReadOnly_FlushWAL(t *testing.T) {
	var (
		dbDir  string
		logger = log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
		err    error
		maxt   int
		ctx    = context.Background()
	)

	// Bootstrap the db.
	{
		dbDir = t.TempDir()

		// Append data to the WAL.
		db, err := Open(dbDir, logger, nil, nil, nil)
		require.NoError(t, err)
		db.DisableCompactions()
		app := db.Appender(ctx)
		maxt = 1000
		for i := 0; i < maxt; i++ {
			_, err := app.Append(0, labels.FromStrings(defaultLabelName, "flush"), int64(i), 1.0)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
		require.NoError(t, db.Close())
	}

	// Flush WAL.
	db, err := OpenDBReadOnly(dbDir, logger)
	require.NoError(t, err)

	flush := t.TempDir()
	require.NoError(t, db.FlushWAL(flush))
	require.NoError(t, db.Close())

	// Reopen the DB from the flushed WAL block.
	db, err = OpenDBReadOnly(flush, logger)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	blocks, err := db.Blocks()
	require.NoError(t, err)
	require.Equal(t, len(blocks), 1)

	querier, err := db.Querier(context.TODO(), 0, int64(maxt)-1)
	require.NoError(t, err)
	defer func() { require.NoError(t, querier.Close()) }()

	// Sum the values.
	seriesSet := querier.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, defaultLabelName, "flush"))

	sum := 0.0
	for seriesSet.Next() {
		series := seriesSet.At().Iterator()
		for series.Next() {
			_, v := series.At()
			sum += v
		}
		require.NoError(t, series.Err())
	}
	require.NoError(t, seriesSet.Err())
	require.Equal(t, 0, len(seriesSet.Warnings()))
	require.Equal(t, 1000.0, sum)
}

func TestDBCannotSeePartialCommits(t *testing.T) {
	if defaultIsolationDisabled {
		t.Skip("skipping test since tsdb isolation is disabled")
	}

	tmpdir := t.TempDir()

	db, err := Open(tmpdir, nil, nil, nil, nil)
	require.NoError(t, err)
	defer db.Close()

	stop := make(chan struct{})
	firstInsert := make(chan struct{})
	ctx := context.Background()

	// Insert data in batches.
	go func() {
		iter := 0
		for {
			app := db.Appender(ctx)

			for j := 0; j < 100; j++ {
				_, err := app.Append(0, labels.FromStrings("foo", "bar", "a", strconv.Itoa(j)), int64(iter), float64(iter))
				require.NoError(t, err)
			}
			err = app.Commit()
			require.NoError(t, err)

			if iter == 0 {
				close(firstInsert)
			}
			iter++

			select {
			case <-stop:
				return
			default:
			}
		}
	}()

	<-firstInsert

	// This is a race condition, so do a few tests to tickle it.
	// Usually most will fail.
	inconsistencies := 0
	for i := 0; i < 10; i++ {
		func() {
			querier, err := db.Querier(context.Background(), 0, 1000000)
			require.NoError(t, err)
			defer querier.Close()

			ss := querier.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
			_, seriesSet, ws, err := expandSeriesSet(ss)
			require.NoError(t, err)
			require.Equal(t, 0, len(ws))

			values := map[float64]struct{}{}
			for _, series := range seriesSet {
				values[series[len(series)-1].v] = struct{}{}
			}
			if len(values) != 1 {
				inconsistencies++
			}
		}()
	}
	stop <- struct{}{}

	require.Equal(t, 0, inconsistencies, "Some queries saw inconsistent results.")
}

func TestDBQueryDoesntSeeAppendsAfterCreation(t *testing.T) {
	if defaultIsolationDisabled {
		t.Skip("skipping test since tsdb isolation is disabled")
	}

	tmpdir := t.TempDir()

	db, err := Open(tmpdir, nil, nil, nil, nil)
	require.NoError(t, err)
	defer db.Close()

	querierBeforeAdd, err := db.Querier(context.Background(), 0, 1000000)
	require.NoError(t, err)
	defer querierBeforeAdd.Close()

	ctx := context.Background()
	app := db.Appender(ctx)
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 0, 0)
	require.NoError(t, err)

	querierAfterAddButBeforeCommit, err := db.Querier(context.Background(), 0, 1000000)
	require.NoError(t, err)
	defer querierAfterAddButBeforeCommit.Close()

	// None of the queriers should return anything after the Add but before the commit.
	ss := querierBeforeAdd.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	_, seriesSet, ws, err := expandSeriesSet(ss)
	require.NoError(t, err)
	require.Equal(t, 0, len(ws))
	require.Equal(t, map[string][]sample{}, seriesSet)

	ss = querierAfterAddButBeforeCommit.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	_, seriesSet, ws, err = expandSeriesSet(ss)
	require.NoError(t, err)
	require.Equal(t, 0, len(ws))
	require.Equal(t, map[string][]sample{}, seriesSet)

	// This commit is after the queriers are created, so should not be returned.
	err = app.Commit()
	require.NoError(t, err)

	// Nothing returned for querier created before the Add.
	ss = querierBeforeAdd.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	_, seriesSet, ws, err = expandSeriesSet(ss)
	require.NoError(t, err)
	require.Equal(t, 0, len(ws))
	require.Equal(t, map[string][]sample{}, seriesSet)

	// Series exists but has no samples for querier created after Add.
	ss = querierAfterAddButBeforeCommit.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	_, seriesSet, ws, err = expandSeriesSet(ss)
	require.NoError(t, err)
	require.Equal(t, 0, len(ws))
	require.Equal(t, map[string][]sample{`{foo="bar"}`: {}}, seriesSet)

	querierAfterCommit, err := db.Querier(context.Background(), 0, 1000000)
	require.NoError(t, err)
	defer querierAfterCommit.Close()

	// Samples are returned for querier created after Commit.
	ss = querierAfterCommit.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	_, seriesSet, ws, err = expandSeriesSet(ss)
	require.NoError(t, err)
	require.Equal(t, 0, len(ws))
	require.Equal(t, map[string][]sample{`{foo="bar"}`: {{t: 0, v: 0}}}, seriesSet)
}

// TestChunkWriter_ReadAfterWrite ensures that chunk segment are cut at the set segment size and
// that the resulted segments includes the expected chunks data.
func TestChunkWriter_ReadAfterWrite(t *testing.T) {
	chk1 := tsdbutil.ChunkFromSamples([]tsdbutil.Sample{sample{1, 1}})
	chk2 := tsdbutil.ChunkFromSamples([]tsdbutil.Sample{sample{1, 2}})
	chk3 := tsdbutil.ChunkFromSamples([]tsdbutil.Sample{sample{1, 3}})
	chk4 := tsdbutil.ChunkFromSamples([]tsdbutil.Sample{sample{1, 4}})
	chk5 := tsdbutil.ChunkFromSamples([]tsdbutil.Sample{sample{1, 5}})
	chunkSize := len(chk1.Chunk.Bytes()) + chunks.MaxChunkLengthFieldSize + chunks.ChunkEncodingSize + crc32.Size

	tests := []struct {
		chks [][]chunks.Meta
		segmentSize,
		expSegmentsCount int
		expSegmentSizes []int
	}{
		// 0:Last chunk ends at the segment boundary so
		// all chunks should fit in a single segment.
		{
			chks: [][]chunks.Meta{
				{
					chk1,
					chk2,
					chk3,
				},
			},
			segmentSize:      3 * chunkSize,
			expSegmentSizes:  []int{3 * chunkSize},
			expSegmentsCount: 1,
		},
		// 1:Two chunks can fit in a single segment so the last one should result in a new segment.
		{
			chks: [][]chunks.Meta{
				{
					chk1,
					chk2,
					chk3,
					chk4,
					chk5,
				},
			},
			segmentSize:      2 * chunkSize,
			expSegmentSizes:  []int{2 * chunkSize, 2 * chunkSize, chunkSize},
			expSegmentsCount: 3,
		},
		// 2:When the segment size is smaller than the size of 2 chunks
		// the last segment should still create a new segment.
		{
			chks: [][]chunks.Meta{
				{
					chk1,
					chk2,
					chk3,
				},
			},
			segmentSize:      2*chunkSize - 1,
			expSegmentSizes:  []int{chunkSize, chunkSize, chunkSize},
			expSegmentsCount: 3,
		},
		// 3:When the segment is smaller than a single chunk
		// it should still be written by ignoring the max segment size.
		{
			chks: [][]chunks.Meta{
				{
					chk1,
				},
			},
			segmentSize:      chunkSize - 1,
			expSegmentSizes:  []int{chunkSize},
			expSegmentsCount: 1,
		},
		// 4:All chunks are bigger than the max segment size, but
		// these should still be written even when this will result in bigger segment than the set size.
		// Each segment will hold a single chunk.
		{
			chks: [][]chunks.Meta{
				{
					chk1,
					chk2,
					chk3,
				},
			},
			segmentSize:      1,
			expSegmentSizes:  []int{chunkSize, chunkSize, chunkSize},
			expSegmentsCount: 3,
		},
		// 5:Adding multiple batches of chunks.
		{
			chks: [][]chunks.Meta{
				{
					chk1,
					chk2,
					chk3,
				},
				{
					chk4,
					chk5,
				},
			},
			segmentSize:      3 * chunkSize,
			expSegmentSizes:  []int{3 * chunkSize, 2 * chunkSize},
			expSegmentsCount: 2,
		},
		// 6:Adding multiple batches of chunks.
		{
			chks: [][]chunks.Meta{
				{
					chk1,
				},
				{
					chk2,
					chk3,
				},
				{
					chk4,
				},
			},
			segmentSize:      2 * chunkSize,
			expSegmentSizes:  []int{2 * chunkSize, 2 * chunkSize},
			expSegmentsCount: 2,
		},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			tempDir := t.TempDir()

			chunkw, err := chunks.NewWriterWithSegSize(tempDir, chunks.SegmentHeaderSize+int64(test.segmentSize))
			require.NoError(t, err)

			for _, chks := range test.chks {
				require.NoError(t, chunkw.WriteChunks(chks...))
			}
			require.NoError(t, chunkw.Close())

			files, err := os.ReadDir(tempDir)
			require.NoError(t, err)
			require.Equal(t, test.expSegmentsCount, len(files), "expected segments count mismatch")

			// Verify that all data is written to the segments.
			sizeExp := 0
			sizeAct := 0

			for _, chks := range test.chks {
				for _, chk := range chks {
					l := make([]byte, binary.MaxVarintLen32)
					sizeExp += binary.PutUvarint(l, uint64(len(chk.Chunk.Bytes()))) // The length field.
					sizeExp += chunks.ChunkEncodingSize
					sizeExp += len(chk.Chunk.Bytes()) // The data itself.
					sizeExp += crc32.Size             // The 4 bytes of crc32
				}
			}
			sizeExp += test.expSegmentsCount * chunks.SegmentHeaderSize // The segment header bytes.

			for i, f := range files {
				fi, err := f.Info()
				require.NoError(t, err)
				size := int(fi.Size())
				// Verify that the segment is the same or smaller than the expected size.
				require.GreaterOrEqual(t, chunks.SegmentHeaderSize+test.expSegmentSizes[i], size, "Segment:%v should NOT be bigger than:%v actual:%v", i, chunks.SegmentHeaderSize+test.expSegmentSizes[i], size)

				sizeAct += size
			}
			require.Equal(t, sizeExp, sizeAct)

			// Check the content of the chunks.
			r, err := chunks.NewDirReader(tempDir, nil)
			require.NoError(t, err)
			defer func() { require.NoError(t, r.Close()) }()

			for _, chks := range test.chks {
				for _, chkExp := range chks {
					chkAct, err := r.Chunk(chkExp)
					require.NoError(t, err)
					require.Equal(t, chkExp.Chunk.Bytes(), chkAct.Bytes())
				}
			}
		})
	}
}

func TestRangeForTimestamp(t *testing.T) {
	type args struct {
		t     int64
		width int64
	}
	tests := []struct {
		args     args
		expected int64
	}{
		{args{0, 5}, 5},
		{args{1, 5}, 5},
		{args{5, 5}, 10},
		{args{6, 5}, 10},
		{args{13, 5}, 15},
		{args{95, 5}, 100},
	}
	for _, tt := range tests {
		got := rangeForTimestamp(tt.args.t, tt.args.width)
		require.Equal(t, tt.expected, got)
	}
}

// TestChunkReader_ConcurrentReads checks that the chunk result can be read concurrently.
// Regression test for https://github.com/prometheus/prometheus/pull/6514.
func TestChunkReader_ConcurrentReads(t *testing.T) {
	chks := []chunks.Meta{
		tsdbutil.ChunkFromSamples([]tsdbutil.Sample{sample{1, 1}}),
		tsdbutil.ChunkFromSamples([]tsdbutil.Sample{sample{1, 2}}),
		tsdbutil.ChunkFromSamples([]tsdbutil.Sample{sample{1, 3}}),
		tsdbutil.ChunkFromSamples([]tsdbutil.Sample{sample{1, 4}}),
		tsdbutil.ChunkFromSamples([]tsdbutil.Sample{sample{1, 5}}),
	}

	tempDir := t.TempDir()

	chunkw, err := chunks.NewWriter(tempDir)
	require.NoError(t, err)

	require.NoError(t, chunkw.WriteChunks(chks...))
	require.NoError(t, chunkw.Close())

	r, err := chunks.NewDirReader(tempDir, nil)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for _, chk := range chks {
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(chunk chunks.Meta) {
				defer wg.Done()

				chkAct, err := r.Chunk(chunk)
				require.NoError(t, err)
				require.Equal(t, chunk.Chunk.Bytes(), chkAct.Bytes())
			}(chk)
		}
		wg.Wait()
	}
	require.NoError(t, r.Close())
}

// TestCompactHead ensures that the head compaction
// creates a block that is ready for loading and
// does not cause data loss.
// This test:
// * opens a storage;
// * appends values;
// * compacts the head; and
// * queries the db to ensure the samples are present from the compacted head.
func TestCompactHead(t *testing.T) {
	dbDir := t.TempDir()

	// Open a DB and append data to the WAL.
	tsdbCfg := &Options{
		RetentionDuration: int64(time.Hour * 24 * 15 / time.Millisecond),
		NoLockfile:        true,
		MinBlockDuration:  int64(time.Hour * 2 / time.Millisecond),
		MaxBlockDuration:  int64(time.Hour * 2 / time.Millisecond),
		WALCompression:    true,
	}

	db, err := Open(dbDir, log.NewNopLogger(), prometheus.NewRegistry(), tsdbCfg, nil)
	require.NoError(t, err)
	ctx := context.Background()
	app := db.Appender(ctx)
	var expSamples []sample
	maxt := 100
	for i := 0; i < maxt; i++ {
		val := rand.Float64()
		_, err := app.Append(0, labels.FromStrings("a", "b"), int64(i), val)
		require.NoError(t, err)
		expSamples = append(expSamples, sample{int64(i), val})
	}
	require.NoError(t, app.Commit())

	// Compact the Head to create a new block.
	require.NoError(t, db.CompactHead(NewRangeHead(db.Head(), 0, int64(maxt)-1)))
	require.NoError(t, db.Close())

	// Delete everything but the new block and
	// reopen the db to query it to ensure it includes the head data.
	require.NoError(t, deleteNonBlocks(db.Dir()))
	db, err = Open(dbDir, log.NewNopLogger(), prometheus.NewRegistry(), tsdbCfg, nil)
	require.NoError(t, err)
	require.Equal(t, 1, len(db.Blocks()))
	require.Equal(t, int64(maxt), db.Head().MinTime())
	defer func() { require.NoError(t, db.Close()) }()
	querier, err := db.Querier(context.Background(), 0, int64(maxt)-1)
	require.NoError(t, err)
	defer func() { require.NoError(t, querier.Close()) }()

	seriesSet := querier.Select(false, nil, &labels.Matcher{Type: labels.MatchEqual, Name: "a", Value: "b"})
	var actSamples []sample

	for seriesSet.Next() {
		series := seriesSet.At().Iterator()
		for series.Next() {
			time, val := series.At()
			actSamples = append(actSamples, sample{int64(time), val})
		}
		require.NoError(t, series.Err())
	}
	require.Equal(t, expSamples, actSamples)
	require.NoError(t, seriesSet.Err())
}

func deleteNonBlocks(dbDir string) error {
	dirs, err := os.ReadDir(dbDir)
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if ok := isBlockDir(dir); !ok {
			if err := os.RemoveAll(filepath.Join(dbDir, dir.Name())); err != nil {
				return err
			}
		}
	}
	dirs, err = os.ReadDir(dbDir)
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if ok := isBlockDir(dir); !ok {
			return errors.Errorf("root folder:%v still hase non block directory:%v", dbDir, dir.Name())
		}
	}
	return nil
}

func TestOpen_VariousBlockStates(t *testing.T) {
	tmpDir := t.TempDir()

	var (
		expectedLoadedDirs  = map[string]struct{}{}
		expectedRemovedDirs = map[string]struct{}{}
		expectedIgnoredDirs = map[string]struct{}{}
	)

	{
		// Ok blocks; should be loaded.
		expectedLoadedDirs[createBlock(t, tmpDir, genSeries(10, 2, 0, 10))] = struct{}{}
		expectedLoadedDirs[createBlock(t, tmpDir, genSeries(10, 2, 10, 20))] = struct{}{}
	}
	{
		// Block to repair; should be repaired & loaded.
		dbDir := filepath.Join("testdata", "repair_index_version", "01BZJ9WJQPWHGNC2W4J9TA62KC")
		outDir := filepath.Join(tmpDir, "01BZJ9WJQPWHGNC2W4J9TA62KC")
		expectedLoadedDirs[outDir] = struct{}{}

		// Touch chunks dir in block.
		require.NoError(t, os.MkdirAll(filepath.Join(dbDir, "chunks"), 0o777))
		defer func() {
			require.NoError(t, os.RemoveAll(filepath.Join(dbDir, "chunks")))
		}()
		require.NoError(t, os.Mkdir(outDir, os.ModePerm))
		require.NoError(t, fileutil.CopyDirs(dbDir, outDir))
	}
	{
		// Missing meta.json; should be ignored and only logged.
		// TODO(bwplotka): Probably add metric.
		dir := createBlock(t, tmpDir, genSeries(10, 2, 20, 30))
		expectedIgnoredDirs[dir] = struct{}{}
		require.NoError(t, os.Remove(filepath.Join(dir, metaFilename)))
	}
	{
		// Tmp blocks during creation; those should be removed on start.
		dir := createBlock(t, tmpDir, genSeries(10, 2, 30, 40))
		require.NoError(t, fileutil.Replace(dir, dir+tmpForCreationBlockDirSuffix))
		expectedRemovedDirs[dir+tmpForCreationBlockDirSuffix] = struct{}{}

		// Tmp blocks during deletion; those should be removed on start.
		dir = createBlock(t, tmpDir, genSeries(10, 2, 40, 50))
		require.NoError(t, fileutil.Replace(dir, dir+tmpForDeletionBlockDirSuffix))
		expectedRemovedDirs[dir+tmpForDeletionBlockDirSuffix] = struct{}{}

		// Pre-2.21 tmp blocks; those should be removed on start.
		dir = createBlock(t, tmpDir, genSeries(10, 2, 50, 60))
		require.NoError(t, fileutil.Replace(dir, dir+tmpLegacy))
		expectedRemovedDirs[dir+tmpLegacy] = struct{}{}
	}
	{
		// One ok block; but two should be replaced.
		dir := createBlock(t, tmpDir, genSeries(10, 2, 50, 60))
		expectedLoadedDirs[dir] = struct{}{}

		m, _, err := readMetaFile(dir)
		require.NoError(t, err)

		compacted := createBlock(t, tmpDir, genSeries(10, 2, 50, 55))
		expectedRemovedDirs[compacted] = struct{}{}

		m.Compaction.Parents = append(m.Compaction.Parents,
			BlockDesc{ULID: ulid.MustParse(filepath.Base(compacted))},
			BlockDesc{ULID: ulid.MustNew(1, nil)},
			BlockDesc{ULID: ulid.MustNew(123, nil)},
		)

		// Regression test: Already removed parent can be still in list, which was causing Open errors.
		m.Compaction.Parents = append(m.Compaction.Parents, BlockDesc{ULID: ulid.MustParse(filepath.Base(compacted))})
		m.Compaction.Parents = append(m.Compaction.Parents, BlockDesc{ULID: ulid.MustParse(filepath.Base(compacted))})
		_, err = writeMetaFile(log.NewLogfmtLogger(os.Stderr), dir, m)
		require.NoError(t, err)
	}
	tmpCheckpointDir := path.Join(tmpDir, "wal/checkpoint.00000001.tmp")
	err := os.MkdirAll(tmpCheckpointDir, 0o777)
	require.NoError(t, err)

	opts := DefaultOptions()
	opts.RetentionDuration = 0
	db, err := Open(tmpDir, log.NewLogfmtLogger(os.Stderr), nil, opts, nil)
	require.NoError(t, err)

	loadedBlocks := db.Blocks()

	var loaded int
	for _, l := range loadedBlocks {
		if _, ok := expectedLoadedDirs[filepath.Join(tmpDir, l.meta.ULID.String())]; !ok {
			t.Fatal("unexpected block", l.meta.ULID, "was loaded")
		}
		loaded++
	}
	require.Equal(t, len(expectedLoadedDirs), loaded)
	require.NoError(t, db.Close())

	files, err := os.ReadDir(tmpDir)
	require.NoError(t, err)

	var ignored int
	for _, f := range files {
		if _, ok := expectedRemovedDirs[filepath.Join(tmpDir, f.Name())]; ok {
			t.Fatal("expected", filepath.Join(tmpDir, f.Name()), "to be removed, but still exists")
		}
		if _, ok := expectedIgnoredDirs[filepath.Join(tmpDir, f.Name())]; ok {
			ignored++
		}
	}
	require.Equal(t, len(expectedIgnoredDirs), ignored)
	_, err = os.Stat(tmpCheckpointDir)
	require.True(t, os.IsNotExist(err))
}

func TestOneCheckpointPerCompactCall(t *testing.T) {
	blockRange := int64(1000)
	tsdbCfg := &Options{
		RetentionDuration: blockRange * 1000,
		NoLockfile:        true,
		MinBlockDuration:  blockRange,
		MaxBlockDuration:  blockRange,
	}

	tmpDir := t.TempDir()

	db, err := Open(tmpDir, log.NewNopLogger(), prometheus.NewRegistry(), tsdbCfg, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	db.DisableCompactions()

	// Case 1: Lot's of uncompacted data in Head.

	lbls := labels.Labels{labels.Label{Name: "foo_d", Value: "choco_bar"}}
	// Append samples spanning 59 block ranges.
	app := db.Appender(context.Background())
	for i := int64(0); i < 60; i++ {
		_, err := app.Append(0, lbls, blockRange*i, rand.Float64())
		require.NoError(t, err)
		_, err = app.Append(0, lbls, (blockRange*i)+blockRange/2, rand.Float64())
		require.NoError(t, err)
		// Rotate the WAL file so that there is >3 files for checkpoint to happen.
		_, err = db.head.wal.NextSegment()
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	// Check the existing WAL files.
	first, last, err := wal.Segments(db.head.wal.Dir())
	require.NoError(t, err)
	require.Equal(t, 0, first)
	require.Equal(t, 60, last)

	require.Equal(t, 0.0, prom_testutil.ToFloat64(db.head.metrics.checkpointCreationTotal))
	require.NoError(t, db.Compact())
	require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.checkpointCreationTotal))

	// As the data spans for 59 blocks, 58 go to disk and 1 remains in Head.
	require.Equal(t, 58, len(db.Blocks()))
	// Though WAL was truncated only once, head should be truncated after each compaction.
	require.Equal(t, 58.0, prom_testutil.ToFloat64(db.head.metrics.headTruncateTotal))

	// The compaction should have only truncated first 2/3 of WAL (while also rotating the files).
	first, last, err = wal.Segments(db.head.wal.Dir())
	require.NoError(t, err)
	require.Equal(t, 40, first)
	require.Equal(t, 61, last)

	// The first checkpoint would be for first 2/3rd of WAL, hence till 39.
	// That should be the last checkpoint.
	_, cno, err := wal.LastCheckpoint(db.head.wal.Dir())
	require.NoError(t, err)
	require.Equal(t, 39, cno)

	// Case 2: Old blocks on disk.
	// The above blocks will act as old blocks.

	// Creating a block to cover the data in the Head so that
	// Head will skip the data during replay and start fresh.
	blocks := db.Blocks()
	newBlockMint := blocks[len(blocks)-1].Meta().MaxTime
	newBlockMaxt := db.Head().MaxTime() + 1
	require.NoError(t, db.Close())

	createBlock(t, db.dir, genSeries(1, 1, newBlockMint, newBlockMaxt))

	db, err = Open(db.dir, log.NewNopLogger(), prometheus.NewRegistry(), tsdbCfg, nil)
	require.NoError(t, err)
	db.DisableCompactions()

	// 1 block more.
	require.Equal(t, 59, len(db.Blocks()))
	// No series in Head because of this new block.
	require.Equal(t, 0, int(db.head.NumSeries()))

	// Adding sample way into the future.
	app = db.Appender(context.Background())
	_, err = app.Append(0, lbls, blockRange*120, rand.Float64())
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// The mint of head is the last block maxt, that means the gap between mint and maxt
	// of Head is too large. This will trigger many compactions.
	require.Equal(t, newBlockMaxt, db.head.MinTime())

	// Another WAL file was rotated.
	first, last, err = wal.Segments(db.head.wal.Dir())
	require.NoError(t, err)
	require.Equal(t, 40, first)
	require.Equal(t, 62, last)

	require.Equal(t, 0.0, prom_testutil.ToFloat64(db.head.metrics.checkpointCreationTotal))
	require.NoError(t, db.Compact())
	require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.checkpointCreationTotal))

	// No new blocks should be created as there was not data in between the new samples and the blocks.
	require.Equal(t, 59, len(db.Blocks()))

	// The compaction should have only truncated first 2/3 of WAL (while also rotating the files).
	first, last, err = wal.Segments(db.head.wal.Dir())
	require.NoError(t, err)
	require.Equal(t, 55, first)
	require.Equal(t, 63, last)

	// The first checkpoint would be for first 2/3rd of WAL, hence till 54.
	// That should be the last checkpoint.
	_, cno, err = wal.LastCheckpoint(db.head.wal.Dir())
	require.NoError(t, err)
	require.Equal(t, 54, cno)
}

func TestNoPanicOnTSDBOpenError(t *testing.T) {
	tmpdir := t.TempDir()

	// Taking the lock will cause a TSDB startup error.
	l, err := tsdbutil.NewDirLocker(tmpdir, "tsdb", log.NewNopLogger(), nil)
	require.NoError(t, err)
	require.NoError(t, l.Lock())

	_, err = Open(tmpdir, nil, nil, DefaultOptions(), nil)
	require.Error(t, err)

	require.NoError(t, l.Release())
}

func TestLockfile(t *testing.T) {
	tsdbutil.TestDirLockerUsage(t, func(t *testing.T, data string, createLock bool) (*tsdbutil.DirLocker, testutil.Closer) {
		opts := DefaultOptions()
		opts.NoLockfile = !createLock

		// Create the DB. This should create lockfile and its metrics.
		db, err := Open(data, nil, nil, opts, nil)
		require.NoError(t, err)

		return db.locker, testutil.NewCallbackCloser(func() {
			require.NoError(t, db.Close())
		})
	})
}

func TestQuerier_ShouldNotPanicIfHeadChunkIsTruncatedWhileReadingQueriedChunks(t *testing.T) {
	t.Skip("TODO: investigate why process crash in CI")

	const numRuns = 5

	for i := 1; i <= numRuns; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			testQuerierShouldNotPanicIfHeadChunkIsTruncatedWhileReadingQueriedChunks(t)
		})
	}
}

func testQuerierShouldNotPanicIfHeadChunkIsTruncatedWhileReadingQueriedChunks(t *testing.T) {
	const (
		numSeries                = 1000
		numStressIterations      = 10000
		minStressAllocationBytes = 128 * 1024
		maxStressAllocationBytes = 512 * 1024
	)

	db := openTestDB(t, nil, nil)
	defer func() {
		require.NoError(t, db.Close())
	}()

	// Disable compactions so we can control it.
	db.DisableCompactions()

	// Generate the metrics we're going to append.
	metrics := make([]labels.Labels, 0, numSeries)
	for i := 0; i < numSeries; i++ {
		metrics = append(metrics, labels.Labels{{Name: labels.MetricName, Value: fmt.Sprintf("test_%d", i)}})
	}

	// Push 1 sample every 15s for 2x the block duration period.
	ctx := context.Background()
	interval := int64(15 * time.Second / time.Millisecond)
	ts := int64(0)

	for ; ts < 2*DefaultBlockDuration; ts += interval {
		app := db.Appender(ctx)

		for _, metric := range metrics {
			_, err := app.Append(0, metric, ts, float64(ts))
			require.NoError(t, err)
		}

		require.NoError(t, app.Commit())
	}

	// Compact the TSDB head for the first time. We expect the head chunks file has been cut.
	require.NoError(t, db.Compact())
	require.Equal(t, float64(1), prom_testutil.ToFloat64(db.Head().metrics.headTruncateTotal))

	// Push more samples for another 1x block duration period.
	for ; ts < 3*DefaultBlockDuration; ts += interval {
		app := db.Appender(ctx)

		for _, metric := range metrics {
			_, err := app.Append(0, metric, ts, float64(ts))
			require.NoError(t, err)
		}

		require.NoError(t, app.Commit())
	}

	// At this point we expect 2 mmap-ed head chunks.

	// Get a querier and make sure it's closed only once the test is over.
	querier, err := db.Querier(ctx, 0, math.MaxInt64)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, querier.Close())
	}()

	// Query back all series.
	hints := &storage.SelectHints{Start: 0, End: math.MaxInt64, Step: interval}
	seriesSet := querier.Select(true, hints, labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, ".+"))

	// Fetch samples iterators from all series.
	var iterators []chunkenc.Iterator
	actualSeries := 0
	for seriesSet.Next() {
		actualSeries++

		// Get the iterator and call Next() so that we're sure the chunk is loaded.
		it := seriesSet.At().Iterator()
		it.Next()
		it.At()

		iterators = append(iterators, it)
	}
	require.NoError(t, seriesSet.Err())
	require.Equal(t, actualSeries, numSeries)

	// Compact the TSDB head again.
	require.NoError(t, db.Compact())
	require.Equal(t, float64(2), prom_testutil.ToFloat64(db.Head().metrics.headTruncateTotal))

	// At this point we expect 1 head chunk has been deleted.

	// Stress the memory and call GC. This is required to increase the chances
	// the chunk memory area is released to the kernel.
	var buf []byte
	for i := 0; i < numStressIterations; i++ {
		//nolint:staticcheck
		buf = append(buf, make([]byte, minStressAllocationBytes+rand.Int31n(maxStressAllocationBytes-minStressAllocationBytes))...)
		if i%1000 == 0 {
			buf = nil
		}
	}

	// Iterate samples. Here we're summing it just to make sure no golang compiler
	// optimization triggers in case we discard the result of it.At().
	var sum float64
	var firstErr error
	for _, it := range iterators {
		for it.Next() {
			_, v := it.At()
			sum += v
		}

		if err := it.Err(); err != nil {
			firstErr = err
		}
	}

	// After having iterated all samples we also want to be sure no error occurred or
	// the "cannot populate chunk XXX: not found" error occurred. This error can occur
	// when the iterator tries to fetch an head chunk which has been offloaded because
	// of the head compaction in the meanwhile.
	if firstErr != nil && !strings.Contains(firstErr.Error(), "cannot populate chunk") {
		t.Fatalf("unexpected error: %s", firstErr.Error())
	}
}

func TestChunkQuerier_ShouldNotPanicIfHeadChunkIsTruncatedWhileReadingQueriedChunks(t *testing.T) {
	t.Skip("TODO: investigate why process crash in CI")

	const numRuns = 5

	for i := 1; i <= numRuns; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			testChunkQuerierShouldNotPanicIfHeadChunkIsTruncatedWhileReadingQueriedChunks(t)
		})
	}
}

func testChunkQuerierShouldNotPanicIfHeadChunkIsTruncatedWhileReadingQueriedChunks(t *testing.T) {
	const (
		numSeries                = 1000
		numStressIterations      = 10000
		minStressAllocationBytes = 128 * 1024
		maxStressAllocationBytes = 512 * 1024
	)

	db := openTestDB(t, nil, nil)
	defer func() {
		require.NoError(t, db.Close())
	}()

	// Disable compactions so we can control it.
	db.DisableCompactions()

	// Generate the metrics we're going to append.
	metrics := make([]labels.Labels, 0, numSeries)
	for i := 0; i < numSeries; i++ {
		metrics = append(metrics, labels.Labels{{Name: labels.MetricName, Value: fmt.Sprintf("test_%d", i)}})
	}

	// Push 1 sample every 15s for 2x the block duration period.
	ctx := context.Background()
	interval := int64(15 * time.Second / time.Millisecond)
	ts := int64(0)

	for ; ts < 2*DefaultBlockDuration; ts += interval {
		app := db.Appender(ctx)

		for _, metric := range metrics {
			_, err := app.Append(0, metric, ts, float64(ts))
			require.NoError(t, err)
		}

		require.NoError(t, app.Commit())
	}

	// Compact the TSDB head for the first time. We expect the head chunks file has been cut.
	require.NoError(t, db.Compact())
	require.Equal(t, float64(1), prom_testutil.ToFloat64(db.Head().metrics.headTruncateTotal))

	// Push more samples for another 1x block duration period.
	for ; ts < 3*DefaultBlockDuration; ts += interval {
		app := db.Appender(ctx)

		for _, metric := range metrics {
			_, err := app.Append(0, metric, ts, float64(ts))
			require.NoError(t, err)
		}

		require.NoError(t, app.Commit())
	}

	// At this point we expect 2 mmap-ed head chunks.

	// Get a querier and make sure it's closed only once the test is over.
	querier, err := db.ChunkQuerier(ctx, 0, math.MaxInt64)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, querier.Close())
	}()

	// Query back all series.
	hints := &storage.SelectHints{Start: 0, End: math.MaxInt64, Step: interval}
	seriesSet := querier.Select(true, hints, labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, ".+"))

	// Iterate all series and get their chunks.
	var chunks []chunkenc.Chunk
	actualSeries := 0
	for seriesSet.Next() {
		actualSeries++
		for it := seriesSet.At().Iterator(); it.Next(); {
			chunks = append(chunks, it.At().Chunk)
		}
	}
	require.NoError(t, seriesSet.Err())
	require.Equal(t, actualSeries, numSeries)

	// Compact the TSDB head again.
	require.NoError(t, db.Compact())
	require.Equal(t, float64(2), prom_testutil.ToFloat64(db.Head().metrics.headTruncateTotal))

	// At this point we expect 1 head chunk has been deleted.

	// Stress the memory and call GC. This is required to increase the chances
	// the chunk memory area is released to the kernel.
	var buf []byte
	for i := 0; i < numStressIterations; i++ {
		//nolint:staticcheck
		buf = append(buf, make([]byte, minStressAllocationBytes+rand.Int31n(maxStressAllocationBytes-minStressAllocationBytes))...)
		if i%1000 == 0 {
			buf = nil
		}
	}

	// Iterate chunks and read their bytes slice. Here we're computing the CRC32
	// just to iterate through the bytes slice. We don't really care the reason why
	// we read this data, we just need to read it to make sure the memory address
	// of the []byte is still valid.
	chkCRC32 := newCRC32()
	for _, chunk := range chunks {
		chkCRC32.Reset()
		_, err := chkCRC32.Write(chunk.Bytes())
		require.NoError(t, err)
	}
}

func newTestDB(t *testing.T) *DB {
	dir := t.TempDir()

	db, err := Open(dir, nil, nil, DefaultOptions(), nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return db
}

func TestOOOWALWrite(t *testing.T) {
	dir := t.TempDir()

	opts := DefaultOptions()
	opts.OutOfOrderCapMin = 2
	opts.OutOfOrderCapMax = 2
	opts.OutOfOrderTimeWindow = 30 * time.Minute.Milliseconds()

	db, err := Open(dir, nil, nil, opts, nil)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	s1, s2 := labels.FromStrings("l", "v1"), labels.FromStrings("l", "v2")
	minutes := func(m int64) int64 { return m * time.Minute.Milliseconds() }

	appendSample := func(app storage.Appender, l labels.Labels, mins int64) {
		_, err = app.Append(0, l, minutes(mins), float64(mins))
		require.NoError(t, err)
	}

	// Ingest sample at 1h.
	app := db.Appender(context.Background())
	appendSample(app, s1, 60)
	appendSample(app, s2, 60)
	require.NoError(t, app.Commit())

	// OOO for s1.
	app = db.Appender(context.Background())
	appendSample(app, s1, 40)
	require.NoError(t, app.Commit())

	// OOO for s2.
	app = db.Appender(context.Background())
	appendSample(app, s2, 42)
	require.NoError(t, app.Commit())

	// OOO for both s1 and s2 in the same commit.
	app = db.Appender(context.Background())
	appendSample(app, s2, 45)
	appendSample(app, s1, 35)
	appendSample(app, s1, 36) // m-maps.
	appendSample(app, s1, 37)
	require.NoError(t, app.Commit())

	// OOO for s1 but not for s2 in the same commit.
	app = db.Appender(context.Background())
	appendSample(app, s1, 50) // m-maps.
	appendSample(app, s2, 65)
	require.NoError(t, app.Commit())

	// Single commit has 2 times m-mapping and more samples after m-map.
	app = db.Appender(context.Background())
	appendSample(app, s2, 50) // m-maps.
	appendSample(app, s2, 51)
	appendSample(app, s2, 52) // m-maps.
	appendSample(app, s2, 53)
	require.NoError(t, app.Commit())

	// The MmapRef in this are not hand calculated, and instead taken from the test run.
	// What is important here is the order of records, and that MmapRef increases for each record.
	oooRecords := []interface{}{
		[]record.RefMmapMarker{
			{Ref: 1},
		},
		[]record.RefSample{
			{Ref: 1, T: minutes(40), V: 40},
		},

		[]record.RefMmapMarker{
			{Ref: 2},
		},
		[]record.RefSample{
			{Ref: 2, T: minutes(42), V: 42},
		},

		[]record.RefSample{
			{Ref: 2, T: minutes(45), V: 45},
			{Ref: 1, T: minutes(35), V: 35},
		},
		[]record.RefMmapMarker{ // 3rd sample, hence m-mapped.
			{Ref: 1, MmapRef: 4294967304},
		},
		[]record.RefSample{
			{Ref: 1, T: minutes(36), V: 36},
			{Ref: 1, T: minutes(37), V: 37},
		},

		[]record.RefMmapMarker{ // 3rd sample, hence m-mapped.
			{Ref: 1, MmapRef: 4294967354},
		},
		[]record.RefSample{ // Does not contain the in-order sample here.
			{Ref: 1, T: minutes(50), V: 50},
		},

		// Single commit but multiple OOO records.
		[]record.RefMmapMarker{
			{Ref: 2, MmapRef: 4294967403},
		},
		[]record.RefSample{
			{Ref: 2, T: minutes(50), V: 50},
			{Ref: 2, T: minutes(51), V: 51},
		},
		[]record.RefMmapMarker{
			{Ref: 2, MmapRef: 4294967452},
		},
		[]record.RefSample{
			{Ref: 2, T: minutes(52), V: 52},
			{Ref: 2, T: minutes(53), V: 53},
		},
	}

	inOrderRecords := []interface{}{
		[]record.RefSeries{
			{Ref: 1, Labels: s1},
			{Ref: 2, Labels: s2},
		},
		[]record.RefSample{
			{Ref: 1, T: minutes(60), V: 60},
			{Ref: 2, T: minutes(60), V: 60},
		},
		[]record.RefSample{
			{Ref: 1, T: minutes(40), V: 40},
		},
		[]record.RefSample{
			{Ref: 2, T: minutes(42), V: 42},
		},
		[]record.RefSample{
			{Ref: 2, T: minutes(45), V: 45},
			{Ref: 1, T: minutes(35), V: 35},
			{Ref: 1, T: minutes(36), V: 36},
			{Ref: 1, T: minutes(37), V: 37},
		},
		[]record.RefSample{ // Contains both in-order and ooo sample.
			{Ref: 1, T: minutes(50), V: 50},
			{Ref: 2, T: minutes(65), V: 65},
		},
		[]record.RefSample{
			{Ref: 2, T: minutes(50), V: 50},
			{Ref: 2, T: minutes(51), V: 51},
			{Ref: 2, T: minutes(52), V: 52},
			{Ref: 2, T: minutes(53), V: 53},
		},
	}

	getRecords := func(walDir string) []interface{} {
		sr, err := wal.NewSegmentsReader(walDir)
		require.NoError(t, err)
		r := wal.NewReader(sr)
		defer func() {
			require.NoError(t, sr.Close())
		}()

		var (
			records []interface{}
			dec     record.Decoder
		)
		for r.Next() {
			rec := r.Record()
			switch typ := dec.Type(rec); typ {
			case record.Series:
				series, err := dec.Series(rec, nil)
				require.NoError(t, err)
				records = append(records, series)
			case record.Samples:
				samples, err := dec.Samples(rec, nil)
				require.NoError(t, err)
				records = append(records, samples)
			case record.MmapMarkers:
				markers, err := dec.MmapMarkers(rec, nil)
				require.NoError(t, err)
				records = append(records, markers)
			default:
				t.Fatalf("got a WAL record that is not series or samples: %v", typ)
			}
		}

		return records
	}

	// The normal WAL.
	actRecs := getRecords(path.Join(dir, "wal"))
	require.Equal(t, inOrderRecords, actRecs)

	// The OOO WAL.
	actRecs = getRecords(path.Join(dir, wal.WblDirName))
	require.Equal(t, oooRecords, actRecs)
}

// Tests https://github.com/prometheus/prometheus/issues/10291#issuecomment-1044373110.
func TestDBPanicOnMmappingHeadChunk(t *testing.T) {
	dir := t.TempDir()

	db, err := Open(dir, nil, nil, DefaultOptions(), nil)
	require.NoError(t, err)
	db.DisableCompactions()

	// Choosing scrape interval of 45s to have chunk larger than 1h.
	itvl := int64(45 * time.Second / time.Millisecond)

	lastTs := int64(0)
	addSamples := func(numSamples int) {
		app := db.Appender(context.Background())
		var ref storage.SeriesRef
		lbls := labels.FromStrings("__name__", "testing", "foo", "bar")
		for i := 0; i < numSamples; i++ {
			ref, err = app.Append(ref, lbls, lastTs, float64(lastTs))
			require.NoError(t, err)
			lastTs += itvl
			if i%10 == 0 {
				require.NoError(t, app.Commit())
				app = db.Appender(context.Background())
			}
		}
		require.NoError(t, app.Commit())
	}

	// Ingest samples upto 2h50m to make the head "about to compact".
	numSamples := int(170*time.Minute/time.Millisecond) / int(itvl)
	addSamples(numSamples)

	require.Len(t, db.Blocks(), 0)
	require.NoError(t, db.Compact())
	require.Len(t, db.Blocks(), 0)

	// Restarting.
	require.NoError(t, db.Close())

	db, err = Open(dir, nil, nil, DefaultOptions(), nil)
	require.NoError(t, err)
	db.DisableCompactions()

	// Ingest samples upto 20m more to make the head compact.
	numSamples = int(20*time.Minute/time.Millisecond) / int(itvl)
	addSamples(numSamples)

	require.Len(t, db.Blocks(), 0)
	require.NoError(t, db.Compact())
	require.Len(t, db.Blocks(), 1)

	// More samples to m-map and panic.
	numSamples = int(120*time.Minute/time.Millisecond) / int(itvl)
	addSamples(numSamples)

	require.NoError(t, db.Close())
}

// TODO(codesome): test more samples incoming once compaction has started. To verify new samples after the start
//   are not included in this compaction.
func TestOOOCompaction(t *testing.T) {
	dir := t.TempDir()

	opts := DefaultOptions()
	opts.OutOfOrderCapMin = 2
	opts.OutOfOrderCapMax = 30
	opts.OutOfOrderTimeWindow = 300 * time.Minute.Milliseconds()
	opts.AllowOverlappingQueries = true
	opts.AllowOverlappingCompaction = true

	db, err := Open(dir, nil, nil, opts, nil)
	require.NoError(t, err)
	db.DisableCompactions() // We want to manually call it.
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	series1 := labels.FromStrings("foo", "bar1")
	series2 := labels.FromStrings("foo", "bar2")

	addSample := func(fromMins, toMins int64) {
		app := db.Appender(context.Background())
		for min := fromMins; min <= toMins; min++ {
			ts := min * time.Minute.Milliseconds()
			_, err := app.Append(0, series1, ts, float64(ts))
			require.NoError(t, err)
			_, err = app.Append(0, series2, ts, float64(2*ts))
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	// Add an in-order samples.
	addSample(250, 350)

	// Verify that the in-memory ooo chunk is empty.
	checkEmptyOOOChunk := func(lbls labels.Labels) {
		ms, created, err := db.head.getOrCreate(lbls.Hash(), lbls)
		require.NoError(t, err)
		require.False(t, created)
		require.Nil(t, ms.oooHeadChunk)
		require.Equal(t, 0, len(ms.oooMmappedChunks))
	}
	checkEmptyOOOChunk(series1)
	checkEmptyOOOChunk(series2)

	// Add ooo samples that creates multiple chunks.
	// 90 to 300 spans across 3 block ranges: [0, 120), [120, 240), [240, 360)
	addSample(90, 310)
	// Adding same samples to create overlapping chunks.
	// Since the active chunk won't start at 90 again, all the new
	// chunks will have different time ranges than the previous chunks.
	addSample(90, 310)

	verifyDBSamples := func() {
		var series1Samples, series2Samples []tsdbutil.Sample
		for _, r := range [][2]int64{{90, 119}, {120, 239}, {240, 350}} {
			fromMins, toMins := r[0], r[1]
			for min := fromMins; min <= toMins; min++ {
				ts := min * time.Minute.Milliseconds()
				series1Samples = append(series1Samples, sample{ts, float64(ts)})
				series2Samples = append(series2Samples, sample{ts, float64(2 * ts)})
			}
		}
		expRes := map[string][]tsdbutil.Sample{
			series1.String(): series1Samples,
			series2.String(): series2Samples,
		}

		q, err := db.Querier(context.Background(), math.MinInt64, math.MaxInt64)
		require.NoError(t, err)

		actRes := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		require.Equal(t, expRes, actRes)
	}

	verifyDBSamples() // Before any compaction.

	// Verify that the in-memory ooo chunk is not empty.
	checkNonEmptyOOOChunk := func(lbls labels.Labels) {
		ms, created, err := db.head.getOrCreate(lbls.Hash(), lbls)
		require.NoError(t, err)
		require.False(t, created)
		require.Greater(t, ms.oooHeadChunk.chunk.NumSamples(), 0)
		require.Equal(t, 14, len(ms.oooMmappedChunks)) // 7 original, 7 duplicate.
	}
	checkNonEmptyOOOChunk(series1)
	checkNonEmptyOOOChunk(series2)

	// No blocks before compaction.
	require.Equal(t, len(db.Blocks()), 0)

	// There is a 0th WBL file.
	files, err := os.ReadDir(db.head.wbl.Dir())
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "00000000", files[0].Name())
	f, err := files[0].Info()
	require.NoError(t, err)
	require.Greater(t, f.Size(), int64(100))

	// OOO compaction happens here.
	require.NoError(t, db.CompactOOOHead())

	// 3 blocks exist now. [0, 120), [120, 240), [240, 360)
	require.Equal(t, len(db.Blocks()), 3)

	verifyDBSamples() // Blocks created out of OOO head now.

	// 0th WBL file will be deleted and 1st will be the only present.
	files, err = os.ReadDir(db.head.wbl.Dir())
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "00000001", files[0].Name())
	f, err = files[0].Info()
	require.NoError(t, err)
	require.Equal(t, int64(0), f.Size())

	// OOO stuff should not be present in the Head now.
	checkEmptyOOOChunk(series1)
	checkEmptyOOOChunk(series2)

	verifySamples := func(block *Block, fromMins, toMins int64) {
		series1Samples := make([]tsdbutil.Sample, 0, toMins-fromMins+1)
		series2Samples := make([]tsdbutil.Sample, 0, toMins-fromMins+1)
		for min := fromMins; min <= toMins; min++ {
			ts := min * time.Minute.Milliseconds()
			series1Samples = append(series1Samples, sample{ts, float64(ts)})
			series2Samples = append(series2Samples, sample{ts, float64(2 * ts)})
		}
		expRes := map[string][]tsdbutil.Sample{
			series1.String(): series1Samples,
			series2.String(): series2Samples,
		}

		q, err := NewBlockQuerier(block, math.MinInt64, math.MaxInt64)
		require.NoError(t, err)

		actRes := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		require.Equal(t, expRes, actRes)
	}

	// Checking for expected data in the blocks.
	verifySamples(db.Blocks()[0], 90, 119)
	verifySamples(db.Blocks()[1], 120, 239)
	verifySamples(db.Blocks()[2], 240, 310)

	// Because of OOO compaction, we already have 2 m-map files.
	// All the chunks are only in the first file.
	mmapDir := mmappedChunksDir(db.head.opts.ChunkDirRoot)
	files, err = os.ReadDir(mmapDir)
	require.NoError(t, err)
	require.Len(t, files, 2)

	// Compact the in-order head and expect another block.
	// Since this is a forced compaction, this block is not aligned with 2h.
	err = db.CompactHead(NewRangeHead(db.head, 250*time.Minute.Milliseconds(), 350*time.Minute.Milliseconds()))
	require.NoError(t, err)
	require.Equal(t, len(db.Blocks()), 4) // [0, 120), [120, 240), [240, 360), [250, 351)
	verifySamples(db.Blocks()[3], 250, 350)

	verifyDBSamples() // Blocks created out of normal and OOO head now. But not merged.

	// The compaction also clears out the old m-map files. Including
	// the file that has ooo chunks.
	files, err = os.ReadDir(mmapDir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "000002", files[0].Name())

	// This will merge overlapping block.
	require.NoError(t, db.Compact())

	require.Equal(t, len(db.Blocks()), 3) // [0, 120), [120, 240), [240, 360)
	verifySamples(db.Blocks()[0], 90, 119)
	verifySamples(db.Blocks()[1], 120, 239)
	verifySamples(db.Blocks()[2], 240, 350) // Merged block.

	verifyDBSamples() // Final state. Blocks from normal and OOO head are merged.
}

// TestOOOCompactionWithNormalCompaction tests if OOO compaction is performed
// when the normal head's compaction is done.
func TestOOOCompactionWithNormalCompaction(t *testing.T) {
	dir := t.TempDir()

	opts := DefaultOptions()
	opts.OutOfOrderCapMin = 2
	opts.OutOfOrderCapMax = 30
	opts.OutOfOrderTimeWindow = 300 * time.Minute.Milliseconds()
	opts.AllowOverlappingQueries = true
	opts.AllowOverlappingCompaction = true

	db, err := Open(dir, nil, nil, opts, nil)
	require.NoError(t, err)
	db.DisableCompactions() // We want to manually call it.
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	series1 := labels.FromStrings("foo", "bar1")
	series2 := labels.FromStrings("foo", "bar2")

	addSamples := func(fromMins, toMins int64) {
		app := db.Appender(context.Background())
		for min := fromMins; min <= toMins; min++ {
			ts := min * time.Minute.Milliseconds()
			_, err := app.Append(0, series1, ts, float64(ts))
			require.NoError(t, err)
			_, err = app.Append(0, series2, ts, float64(2*ts))
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	// Add an in-order samples.
	addSamples(250, 350)

	// Add ooo samples that will result into a single block.
	addSamples(90, 110)

	// Checking that ooo chunk is not empty.
	for _, lbls := range []labels.Labels{series1, series2} {
		ms, created, err := db.head.getOrCreate(lbls.Hash(), lbls)
		require.NoError(t, err)
		require.False(t, created)
		require.Greater(t, ms.oooHeadChunk.chunk.NumSamples(), 0)
	}

	// If the normal Head is not compacted, the OOO head compaction does not take place.
	require.NoError(t, db.Compact())
	require.Equal(t, len(db.Blocks()), 0)

	// Add more in-order samples in future that would trigger the compaction.
	addSamples(400, 450)

	// No blocks before compaction.
	require.Equal(t, len(db.Blocks()), 0)

	// Compacts normal and OOO head.
	require.NoError(t, db.Compact())

	// 2 blocks exist now. [0, 120), [250, 360)
	require.Equal(t, len(db.Blocks()), 2)
	require.Equal(t, int64(0), db.Blocks()[0].MinTime())
	require.Equal(t, 120*time.Minute.Milliseconds(), db.Blocks()[0].MaxTime())
	require.Equal(t, 250*time.Minute.Milliseconds(), db.Blocks()[1].MinTime())
	require.Equal(t, 360*time.Minute.Milliseconds(), db.Blocks()[1].MaxTime())

	// Checking that ooo chunk is empty.
	for _, lbls := range []labels.Labels{series1, series2} {
		ms, created, err := db.head.getOrCreate(lbls.Hash(), lbls)
		require.NoError(t, err)
		require.False(t, created)
		require.Nil(t, ms.oooHeadChunk)
		require.Equal(t, 0, len(ms.oooMmappedChunks))
	}

	verifySamples := func(block *Block, fromMins, toMins int64) {
		series1Samples := make([]tsdbutil.Sample, 0, toMins-fromMins+1)
		series2Samples := make([]tsdbutil.Sample, 0, toMins-fromMins+1)
		for min := fromMins; min <= toMins; min++ {
			ts := min * time.Minute.Milliseconds()
			series1Samples = append(series1Samples, sample{ts, float64(ts)})
			series2Samples = append(series2Samples, sample{ts, float64(2 * ts)})
		}
		expRes := map[string][]tsdbutil.Sample{
			series1.String(): series1Samples,
			series2.String(): series2Samples,
		}

		q, err := NewBlockQuerier(block, math.MinInt64, math.MaxInt64)
		require.NoError(t, err)

		actRes := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		require.Equal(t, expRes, actRes)
	}

	// Checking for expected data in the blocks.
	verifySamples(db.Blocks()[0], 90, 110)
	verifySamples(db.Blocks()[1], 250, 350)
}

func Test_Querier_OOOQuery(t *testing.T) {
	opts := DefaultOptions()
	opts.OutOfOrderCapMin = 2
	opts.OutOfOrderCapMax = 30
	opts.OutOfOrderTimeWindow = 24 * time.Hour.Milliseconds()
	opts.AllowOverlappingQueries = true
	opts.AllowOverlappingCompaction = false

	series1 := labels.FromStrings("foo", "bar1")

	minutes := func(m int64) int64 { return m * time.Minute.Milliseconds() }
	addSample := func(db *DB, fromMins, toMins, queryMinT, queryMaxT int64, expSamples []tsdbutil.Sample) ([]tsdbutil.Sample, int) {
		app := db.Appender(context.Background())
		totalAppended := 0
		for min := fromMins; min <= toMins; min += time.Minute.Milliseconds() {
			_, err := app.Append(0, series1, min, float64(min))
			if min >= queryMinT && min <= queryMaxT {
				expSamples = append(expSamples, sample{t: min, v: float64(min)})
			}
			require.NoError(t, err)
			totalAppended++
		}
		require.NoError(t, app.Commit())
		return expSamples, totalAppended
	}

	tests := []struct {
		name        string
		queryMinT   int64
		queryMaxT   int64
		inOrderMinT int64
		inOrderMaxT int64
		oooMinT     int64
		oooMaxT     int64
	}{
		{
			name:        "query interval covering ooomint and inordermaxt returns all ingested samples",
			queryMinT:   minutes(0),
			queryMaxT:   minutes(200),
			inOrderMinT: minutes(100),
			inOrderMaxT: minutes(200),
			oooMinT:     minutes(0),
			oooMaxT:     minutes(99),
		},
		{
			name:        "partial query interval returns only samples within interval",
			queryMinT:   minutes(20),
			queryMaxT:   minutes(180),
			inOrderMinT: minutes(100),
			inOrderMaxT: minutes(200),
			oooMinT:     minutes(0),
			oooMaxT:     minutes(99),
		},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("name=%s", tc.name), func(t *testing.T) {
			db := openTestDB(t, opts, nil)
			db.DisableCompactions()
			defer func() {
				require.NoError(t, db.Close())
			}()

			var expSamples []tsdbutil.Sample

			// Add in-order samples.
			expSamples, _ = addSample(db, tc.inOrderMinT, tc.inOrderMaxT, tc.queryMinT, tc.queryMaxT, expSamples)

			// Add out-of-order samples.
			expSamples, oooSamples := addSample(db, tc.oooMinT, tc.oooMaxT, tc.queryMinT, tc.queryMaxT, expSamples)

			sort.Slice(expSamples, func(i, j int) bool {
				return expSamples[i].T() < expSamples[j].T()
			})

			querier, err := db.Querier(context.TODO(), tc.queryMinT, tc.queryMaxT)
			require.NoError(t, err)
			defer querier.Close()

			seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar1"))
			require.NotNil(t, seriesSet[series1.String()])
			require.Equal(t, 1, len(seriesSet))
			require.Equal(t, expSamples, seriesSet[series1.String()])
			require.GreaterOrEqual(t, float64(oooSamples), prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamplesAppended), "number of ooo appended samples mismatch")
		})
	}
}

func Test_ChunkQuerier_OOOQuery(t *testing.T) {
	opts := DefaultOptions()
	opts.OutOfOrderCapMin = 2
	opts.OutOfOrderCapMax = 30
	opts.OutOfOrderTimeWindow = 24 * time.Hour.Milliseconds()
	opts.AllowOverlappingQueries = true
	opts.AllowOverlappingCompaction = false

	series1 := labels.FromStrings("foo", "bar1")

	minutes := func(m int64) int64 { return m * time.Minute.Milliseconds() }
	addSample := func(db *DB, fromMins, toMins, queryMinT, queryMaxT int64, expSamples []tsdbutil.Sample) ([]tsdbutil.Sample, int) {
		app := db.Appender(context.Background())
		totalAppended := 0
		for min := fromMins; min <= toMins; min += time.Minute.Milliseconds() {
			_, err := app.Append(0, series1, min, float64(min))
			if min >= queryMinT && min <= queryMaxT {
				expSamples = append(expSamples, sample{t: min, v: float64(min)})
			}
			require.NoError(t, err)
			totalAppended++
		}
		require.NoError(t, app.Commit())
		return expSamples, totalAppended
	}

	tests := []struct {
		name        string
		queryMinT   int64
		queryMaxT   int64
		inOrderMinT int64
		inOrderMaxT int64
		oooMinT     int64
		oooMaxT     int64
	}{
		{
			name:        "query interval covering ooomint and inordermaxt returns all ingested samples",
			queryMinT:   minutes(0),
			queryMaxT:   minutes(200),
			inOrderMinT: minutes(100),
			inOrderMaxT: minutes(200),
			oooMinT:     minutes(0),
			oooMaxT:     minutes(99),
		},
		{
			name:        "partial query interval returns only samples within interval",
			queryMinT:   minutes(20),
			queryMaxT:   minutes(180),
			inOrderMinT: minutes(100),
			inOrderMaxT: minutes(200),
			oooMinT:     minutes(0),
			oooMaxT:     minutes(99),
		},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("name=%s", tc.name), func(t *testing.T) {
			db := openTestDB(t, opts, nil)
			db.DisableCompactions()
			defer func() {
				require.NoError(t, db.Close())
			}()

			var expSamples []tsdbutil.Sample

			// Add in-order samples.
			expSamples, _ = addSample(db, tc.inOrderMinT, tc.inOrderMaxT, tc.queryMinT, tc.queryMaxT, expSamples)

			// Add out-of-order samples.
			expSamples, oooSamples := addSample(db, tc.oooMinT, tc.oooMaxT, tc.queryMinT, tc.queryMaxT, expSamples)

			sort.Slice(expSamples, func(i, j int) bool {
				return expSamples[i].T() < expSamples[j].T()
			})

			querier, err := db.ChunkQuerier(context.TODO(), tc.queryMinT, tc.queryMaxT)
			require.NoError(t, err)
			defer querier.Close()

			chks := queryChunks(t, querier, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar1"))
			require.NotNil(t, chks[series1.String()])
			require.Equal(t, 1, len(chks))
			require.Equal(t, float64(oooSamples), prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamplesAppended), "number of ooo appended samples mismatch")
			var gotSamples []tsdbutil.Sample
			for _, chunk := range chks[series1.String()] {
				it := chunk.Chunk.Iterator(nil)
				for it.Next() {
					ts, v := it.At()
					gotSamples = append(gotSamples, sample{t: ts, v: v})
				}
			}
			require.Equal(t, expSamples, gotSamples)
		})
	}
}

func TestOOOAppendAndQuery(t *testing.T) {
	opts := DefaultOptions()
	opts.OutOfOrderCapMin = 2
	opts.OutOfOrderCapMax = 30
	opts.OutOfOrderTimeWindow = 4 * time.Hour.Milliseconds()
	opts.AllowOverlappingQueries = true

	db := openTestDB(t, opts, nil)
	db.DisableCompactions()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	s1 := labels.FromStrings("foo", "bar1")
	s2 := labels.FromStrings("foo", "bar2")

	minutes := func(m int64) int64 { return m * time.Minute.Milliseconds() }
	expSamples := make(map[string][]tsdbutil.Sample)
	totalSamples := 0
	addSample := func(lbls labels.Labels, fromMins, toMins int64, faceError bool) {
		app := db.Appender(context.Background())
		key := lbls.String()
		from, to := minutes(fromMins), minutes(toMins)
		for min := from; min <= to; min += time.Minute.Milliseconds() {
			val := rand.Float64()
			_, err := app.Append(0, lbls, min, val)
			if faceError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				expSamples[key] = append(expSamples[key], sample{t: min, v: val})
				totalSamples++
			}
		}
		if faceError {
			require.NoError(t, app.Rollback())
		} else {
			require.NoError(t, app.Commit())
		}
	}

	testQuery := func() {
		querier, err := db.Querier(context.TODO(), math.MinInt64, math.MaxInt64)
		require.NoError(t, err)

		seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar."))

		for k, v := range expSamples {
			sort.Slice(v, func(i, j int) bool {
				return v[i].T() < v[j].T()
			})
			expSamples[k] = v
		}
		require.Equal(t, expSamples, seriesSet)
		require.Equal(t, float64(totalSamples-2), prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamplesAppended), "number of ooo appended samples mismatch")
	}

	verifyOOOMinMaxTimes := func(expMin, expMax int64) {
		require.Equal(t, minutes(expMin), db.head.MinOOOTime())
		require.Equal(t, minutes(expMax), db.head.MaxOOOTime())
	}

	// In-order samples.
	addSample(s1, 300, 300, false)
	addSample(s2, 290, 290, false)
	require.Equal(t, float64(2), prom_testutil.ToFloat64(db.head.metrics.chunksCreated))
	testQuery()

	// Some ooo samples.
	addSample(s1, 250, 260, false)
	addSample(s2, 255, 265, false)
	verifyOOOMinMaxTimes(250, 265)
	testQuery()

	// Out of time window.
	addSample(s1, 59, 59, true)
	addSample(s2, 49, 49, true)
	verifyOOOMinMaxTimes(250, 265)
	testQuery()

	// At the edge of time window, also it would be "out of bound" without the ooo support.
	addSample(s1, 60, 65, false)
	addSample(s2, 50, 55, false)
	verifyOOOMinMaxTimes(50, 265)
	testQuery()

	// Out of time window again.
	addSample(s1, 59, 59, true)
	addSample(s2, 49, 49, true)
	testQuery()

	// Generating some m-map chunks. The m-map chunks here are in such a way
	// that when sorted w.r.t. mint, the last chunk's maxt is not the overall maxt
	// of the merged chunk. This tests a bug fixed in https://github.com/grafana/mimir-prometheus/pull/238/.
	require.Equal(t, float64(4), prom_testutil.ToFloat64(db.head.metrics.chunksCreated))
	addSample(s1, 180, 249, false)
	require.Equal(t, float64(6), prom_testutil.ToFloat64(db.head.metrics.chunksCreated))
	verifyOOOMinMaxTimes(50, 265)
	testQuery()
}

func TestOOODisabled(t *testing.T) {
	opts := DefaultOptions()
	opts.OutOfOrderTimeWindow = 0
	db := openTestDB(t, opts, nil)
	db.DisableCompactions()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	s1 := labels.FromStrings("foo", "bar1")
	minutes := func(m int64) int64 { return m * time.Minute.Milliseconds() }
	expSamples := make(map[string][]tsdbutil.Sample)
	totalSamples := 0
	failedSamples := 0
	addSample := func(lbls labels.Labels, fromMins, toMins int64, faceError bool) {
		app := db.Appender(context.Background())
		key := lbls.String()
		from, to := minutes(fromMins), minutes(toMins)
		for min := from; min <= to; min += time.Minute.Milliseconds() {
			val := rand.Float64()
			_, err := app.Append(0, lbls, min, val)
			if faceError {
				require.Error(t, err)
				failedSamples++
			} else {
				require.NoError(t, err)
				expSamples[key] = append(expSamples[key], sample{t: min, v: val})
				totalSamples++
			}
		}
		if faceError {
			require.NoError(t, app.Rollback())
		} else {
			require.NoError(t, app.Commit())
		}
	}

	addSample(s1, 300, 300, false) // In-order samples.
	addSample(s1, 250, 260, true)  // Some ooo samples.
	addSample(s1, 59, 59, true)    // Out of time window.
	addSample(s1, 60, 65, true)    // At the edge of time window, also it would be "out of bound" without the ooo support.
	addSample(s1, 59, 59, true)    // Out of time window again.
	addSample(s1, 301, 310, false) // More in-order samples.

	querier, err := db.Querier(context.TODO(), math.MinInt64, math.MaxInt64)
	require.NoError(t, err)

	seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar."))
	require.Equal(t, expSamples, seriesSet)
	require.Equal(t, float64(0), prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamplesAppended), "number of ooo appended samples mismatch")
	require.Equal(t, float64(failedSamples),
		prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples)+prom_testutil.ToFloat64(db.head.metrics.outOfBoundSamples),
		"number of ooo/oob samples mismatch")

	// Verifying that no OOO artifacts were generated.
	_, err = os.ReadDir(path.Join(db.Dir(), wal.WblDirName))
	require.True(t, os.IsNotExist(err))

	ms, created, err := db.head.getOrCreate(s1.Hash(), s1)
	require.NoError(t, err)
	require.False(t, created)
	require.NotNil(t, ms)
	require.Nil(t, ms.oooHeadChunk)
	require.Len(t, ms.oooMmappedChunks, 0)
}

func TestWBLAndMmapReplay(t *testing.T) {
	opts := DefaultOptions()
	opts.OutOfOrderCapMin = 2
	opts.OutOfOrderCapMax = 30
	opts.OutOfOrderTimeWindow = 4 * time.Hour.Milliseconds()
	opts.AllowOverlappingQueries = true

	db := openTestDB(t, opts, nil)
	db.DisableCompactions()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	s1 := labels.FromStrings("foo", "bar1")

	minutes := func(m int64) int64 { return m * time.Minute.Milliseconds() }
	expSamples := make(map[string][]tsdbutil.Sample)
	totalSamples := 0
	addSample := func(lbls labels.Labels, fromMins, toMins int64) {
		app := db.Appender(context.Background())
		key := lbls.String()
		from, to := minutes(fromMins), minutes(toMins)
		for min := from; min <= to; min += time.Minute.Milliseconds() {
			val := rand.Float64()
			_, err := app.Append(0, lbls, min, val)
			require.NoError(t, err)
			expSamples[key] = append(expSamples[key], sample{t: min, v: val})
			totalSamples++
		}
		require.NoError(t, app.Commit())
	}

	testQuery := func(exp map[string][]tsdbutil.Sample) {
		querier, err := db.Querier(context.TODO(), math.MinInt64, math.MaxInt64)
		require.NoError(t, err)

		seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar."))

		for k, v := range exp {
			sort.Slice(v, func(i, j int) bool {
				return v[i].T() < v[j].T()
			})
			exp[k] = v
		}
		require.Equal(t, exp, seriesSet)
	}

	// In-order samples.
	addSample(s1, 300, 300)
	require.Equal(t, float64(1), prom_testutil.ToFloat64(db.head.metrics.chunksCreated))

	// Some ooo samples.
	addSample(s1, 250, 260)
	addSample(s1, 195, 249) // This creates some m-map chunks.
	require.Equal(t, float64(4), prom_testutil.ToFloat64(db.head.metrics.chunksCreated))
	testQuery(expSamples)
	oooMint, oooMaxt := minutes(195), minutes(260)

	// Collect the samples only present in the ooo m-map chunks.
	ms, created, err := db.head.getOrCreate(s1.Hash(), s1)
	require.False(t, created)
	require.NoError(t, err)
	var s1MmapSamples []tsdbutil.Sample
	for _, mc := range ms.oooMmappedChunks {
		chk, err := db.head.chunkDiskMapper.Chunk(mc.ref)
		require.NoError(t, err)
		it := chk.Iterator(nil)
		for it.Next() {
			ts, val := it.At()
			s1MmapSamples = append(s1MmapSamples, sample{t: ts, v: val})
		}
	}
	require.Greater(t, len(s1MmapSamples), 0)

	require.NoError(t, db.Close())

	// Making a copy of original state of WBL and Mmap files to use it later.
	mmapDir := mmappedChunksDir(db.head.opts.ChunkDirRoot)
	wblDir := db.head.wbl.Dir()
	originalWblDir := filepath.Join(t.TempDir(), "original_wbl")
	originalMmapDir := filepath.Join(t.TempDir(), "original_mmap")
	require.NoError(t, fileutil.CopyDirs(wblDir, originalWblDir))
	require.NoError(t, fileutil.CopyDirs(mmapDir, originalMmapDir))
	resetWBLToOriginal := func() {
		require.NoError(t, os.RemoveAll(wblDir))
		require.NoError(t, fileutil.CopyDirs(originalWblDir, wblDir))
	}
	resetMmapToOriginal := func() {
		require.NoError(t, os.RemoveAll(mmapDir))
		require.NoError(t, fileutil.CopyDirs(originalMmapDir, mmapDir))
	}

	t.Run("Restart DB with both WBL and M-map files for ooo data", func(t *testing.T) {
		db, err = Open(db.dir, nil, nil, opts, nil)
		require.NoError(t, err)
		require.Equal(t, oooMint, db.head.MinOOOTime())
		require.Equal(t, oooMaxt, db.head.MaxOOOTime())
		testQuery(expSamples)
		require.NoError(t, db.Close())
	})

	t.Run("Restart DB with only WBL for ooo data", func(t *testing.T) {
		require.NoError(t, os.RemoveAll(mmapDir))

		db, err = Open(db.dir, nil, nil, opts, nil)
		require.NoError(t, err)
		require.Equal(t, oooMint, db.head.MinOOOTime())
		require.Equal(t, oooMaxt, db.head.MaxOOOTime())
		testQuery(expSamples)
		require.NoError(t, db.Close())
	})

	t.Run("Restart DB with only M-map files for ooo data", func(t *testing.T) {
		require.NoError(t, os.RemoveAll(wblDir))
		resetMmapToOriginal()

		db, err = Open(db.dir, nil, nil, opts, nil)
		require.NoError(t, err)
		require.Equal(t, oooMint, db.head.MinOOOTime())
		require.Equal(t, oooMaxt, db.head.MaxOOOTime())
		inOrderSample := expSamples[s1.String()][len(expSamples[s1.String()])-1]
		testQuery(map[string][]tsdbutil.Sample{
			s1.String(): append(s1MmapSamples, inOrderSample),
		})
		require.NoError(t, db.Close())
	})

	t.Run("Restart DB with WBL+Mmap while increasing the OOOCapMax", func(t *testing.T) {
		resetWBLToOriginal()
		resetMmapToOriginal()

		opts.OutOfOrderCapMax = 60
		db, err = Open(db.dir, nil, nil, opts, nil)
		require.NoError(t, err)
		require.Equal(t, oooMint, db.head.MinOOOTime())
		require.Equal(t, oooMaxt, db.head.MaxOOOTime())
		testQuery(expSamples)
		require.NoError(t, db.Close())
	})

	t.Run("Restart DB with WBL+Mmap while decreasing the OOOCapMax", func(t *testing.T) {
		resetMmapToOriginal() // We need to reset because new duplicate chunks can be written above.

		opts.OutOfOrderCapMax = 10
		db, err = Open(db.dir, nil, nil, opts, nil)
		require.NoError(t, err)
		require.Equal(t, oooMint, db.head.MinOOOTime())
		require.Equal(t, oooMaxt, db.head.MaxOOOTime())
		testQuery(expSamples)
		require.NoError(t, db.Close())
	})

	t.Run("Restart DB with WBL+Mmap while having no m-map markers in WBL", func(t *testing.T) {
		resetMmapToOriginal() // We neet to reset because new duplicate chunks can be written above.

		// Removing m-map markers in WBL by rewriting it.
		newWbl, err := wal.New(log.NewNopLogger(), nil, filepath.Join(t.TempDir(), "new_wbl"), false)
		require.NoError(t, err)
		sr, err := wal.NewSegmentsReader(originalWblDir)
		require.NoError(t, err)
		var dec record.Decoder
		r, markers, addedRecs := wal.NewReader(sr), 0, 0
		for r.Next() {
			rec := r.Record()
			if dec.Type(rec) == record.MmapMarkers {
				markers++
				continue
			}
			addedRecs++
			require.NoError(t, newWbl.Log(rec))
		}
		require.Greater(t, markers, 0)
		require.Greater(t, addedRecs, 0)
		require.NoError(t, newWbl.Close())
		require.NoError(t, os.RemoveAll(wblDir))
		require.NoError(t, os.Rename(newWbl.Dir(), wblDir))

		opts.OutOfOrderCapMax = 30
		db, err = Open(db.dir, nil, nil, opts, nil)
		require.NoError(t, err)
		require.Equal(t, oooMint, db.head.MinOOOTime())
		require.Equal(t, oooMaxt, db.head.MaxOOOTime())
		testQuery(expSamples)
	})
}

func TestOOOCompactionFailure(t *testing.T) {
	dir := t.TempDir()

	opts := DefaultOptions()
	opts.OutOfOrderCapMin = 2
	opts.OutOfOrderCapMax = 30
	opts.OutOfOrderTimeWindow = 300 * time.Minute.Milliseconds()
	opts.AllowOverlappingQueries = true
	opts.AllowOverlappingCompaction = true

	db, err := Open(dir, nil, nil, opts, nil)
	require.NoError(t, err)
	db.DisableCompactions() // We want to manually call it.
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	series1 := labels.FromStrings("foo", "bar1")

	addSample := func(fromMins, toMins int64) {
		app := db.Appender(context.Background())
		for min := fromMins; min <= toMins; min++ {
			ts := min * time.Minute.Milliseconds()
			_, err := app.Append(0, series1, ts, float64(ts))
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	// Add an in-order samples.
	addSample(250, 350)

	// Add ooo samples that creates multiple chunks.
	addSample(90, 310)

	// No blocks before compaction.
	require.Equal(t, len(db.Blocks()), 0)

	// There is a 0th WBL file.
	verifyFirstWBLFileIs0 := func(count int) {
		files, err := os.ReadDir(db.head.wbl.Dir())
		require.NoError(t, err)
		require.Len(t, files, count)
		require.Equal(t, "00000000", files[0].Name())
		f, err := files[0].Info()
		require.NoError(t, err)
		require.Greater(t, f.Size(), int64(100))
	}
	verifyFirstWBLFileIs0(1)

	verifyMmapFiles := func(exp ...string) {
		mmapDir := mmappedChunksDir(db.head.opts.ChunkDirRoot)
		files, err := os.ReadDir(mmapDir)
		require.NoError(t, err)
		require.Len(t, files, len(exp))
		for i, f := range files {
			require.Equal(t, exp[i], f.Name())
		}
	}

	verifyMmapFiles("000001")

	// OOO compaction fails 5 times.
	originalCompactor := db.compactor
	db.compactor = &mockCompactorFailing{t: t}
	for i := 0; i < 5; i++ {
		require.Error(t, db.CompactOOOHead())
	}
	require.Equal(t, len(db.Blocks()), 0)

	// M-map files don't change after failed compaction.
	verifyMmapFiles("000001")

	// Because of 5 compaction attempts, there are 6 files now.
	verifyFirstWBLFileIs0(6)

	db.compactor = originalCompactor
	require.NoError(t, db.CompactOOOHead())
	oldBlocks := db.Blocks()
	require.Equal(t, len(db.Blocks()), 3)

	// Check that the ooo chunks were removed.
	ms, created, err := db.head.getOrCreate(series1.Hash(), series1)
	require.NoError(t, err)
	require.False(t, created)
	require.Nil(t, ms.oooHeadChunk)
	require.Len(t, ms.oooMmappedChunks, 0)

	// The failed compaction should not have left the ooo Head corrupted.
	// Hence, expect no new blocks with another OOO compaction call.
	require.NoError(t, db.CompactOOOHead())
	require.Equal(t, len(db.Blocks()), 3)
	require.Equal(t, oldBlocks, db.Blocks())

	// Because of OOO compaction, we have a second m-map file now
	// All the chunks are only in the first file.
	verifyMmapFiles("000001", "000002")

	// All but last WBL file will be deleted.
	// 8 files in total (starting at 0) because of 7 compaction calls.
	files, err := os.ReadDir(db.head.wbl.Dir())
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "00000007", files[0].Name())
	f, err := files[0].Info()
	require.NoError(t, err)
	require.Equal(t, int64(0), f.Size())

	verifySamples := func(block *Block, fromMins, toMins int64) {
		series1Samples := make([]tsdbutil.Sample, 0, toMins-fromMins+1)
		for min := fromMins; min <= toMins; min++ {
			ts := min * time.Minute.Milliseconds()
			series1Samples = append(series1Samples, sample{ts, float64(ts)})
		}
		expRes := map[string][]tsdbutil.Sample{
			series1.String(): series1Samples,
		}

		q, err := NewBlockQuerier(block, math.MinInt64, math.MaxInt64)
		require.NoError(t, err)

		actRes := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		require.Equal(t, expRes, actRes)
	}

	// Checking for expected data in the blocks.
	verifySamples(db.Blocks()[0], 90, 119)
	verifySamples(db.Blocks()[1], 120, 239)
	verifySamples(db.Blocks()[2], 240, 310)

	// Compact the in-order head and expect another block.
	// Since this is a forced compaction, this block is not aligned with 2h.
	err = db.CompactHead(NewRangeHead(db.head, 250*time.Minute.Milliseconds(), 350*time.Minute.Milliseconds()))
	require.NoError(t, err)
	require.Equal(t, len(db.Blocks()), 4) // [0, 120), [120, 240), [240, 360), [250, 351)
	verifySamples(db.Blocks()[3], 250, 350)

	// The compaction also clears out the old m-map files. Including
	// the file that has ooo chunks.
	verifyMmapFiles("000002")
}

func TestWBLCorruption(t *testing.T) {
	dir := t.TempDir()

	opts := DefaultOptions()
	opts.OutOfOrderCapMin = 2
	opts.OutOfOrderCapMax = 30
	opts.OutOfOrderTimeWindow = 300 * time.Minute.Milliseconds()
	opts.AllowOverlappingQueries = true
	opts.AllowOverlappingCompaction = true

	db, err := Open(dir, nil, nil, opts, nil)
	require.NoError(t, err)
	db.DisableCompactions()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	series1 := labels.FromStrings("foo", "bar1")
	var allSamples, expAfterRestart []tsdbutil.Sample
	addSamples := func(fromMins, toMins int64, afterRestart bool) {
		app := db.Appender(context.Background())
		for min := fromMins; min <= toMins; min++ {
			ts := min * time.Minute.Milliseconds()
			_, err := app.Append(0, series1, ts, float64(ts))
			require.NoError(t, err)
			allSamples = append(allSamples, sample{t: ts, v: float64(ts)})
			if afterRestart {
				expAfterRestart = append(expAfterRestart, sample{t: ts, v: float64(ts)})
			}
		}
		require.NoError(t, app.Commit())
	}

	// Add an in-order samples.
	addSamples(340, 350, true)

	// OOO samples.
	addSamples(90, 99, true)
	addSamples(100, 119, true)
	addSamples(120, 130, true)

	// Moving onto the second file.
	_, err = db.head.wbl.NextSegment()
	require.NoError(t, err)

	// More OOO samples.
	addSamples(200, 230, true)
	addSamples(240, 255, true)

	// We corrupt WBL after the sample at 255. So everything added later
	// should be deleted after replay.

	// Checking where we corrupt it.
	files, err := os.ReadDir(db.head.wbl.Dir())
	require.NoError(t, err)
	require.Len(t, files, 2)
	f1, err := files[1].Info()
	require.NoError(t, err)
	corruptIndex := f1.Size()
	corruptFilePath := path.Join(db.head.wbl.Dir(), files[1].Name())

	// Corrupt the WBL by adding a malformed record.
	require.NoError(t, db.head.wbl.Log([]byte{byte(record.Samples), 99, 9, 99, 9, 99, 9, 99}))

	// More samples after the corruption point.
	addSamples(260, 280, false)
	addSamples(290, 300, false)

	// Another file.
	_, err = db.head.wbl.NextSegment()
	require.NoError(t, err)

	addSamples(310, 320, false)

	// Verifying that we have data after corruption point.
	files, err = os.ReadDir(db.head.wbl.Dir())
	require.NoError(t, err)
	require.Len(t, files, 3)
	f1, err = files[1].Info()
	require.NoError(t, err)
	require.Greater(t, f1.Size(), corruptIndex)
	f0, err := files[0].Info()
	require.NoError(t, err)
	require.Greater(t, f0.Size(), int64(100))
	f2, err := files[2].Info()
	require.NoError(t, err)
	require.Greater(t, f2.Size(), int64(100))

	verifySamples := func(expSamples []tsdbutil.Sample) {
		sort.Slice(expSamples, func(i, j int) bool {
			return expSamples[i].T() < expSamples[j].T()
		})

		expRes := map[string][]tsdbutil.Sample{
			series1.String(): expSamples,
		}

		q, err := db.Querier(context.Background(), math.MinInt64, math.MaxInt64)
		require.NoError(t, err)

		actRes := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		require.Equal(t, expRes, actRes)
	}

	verifySamples(allSamples)

	require.NoError(t, db.Close())

	// We want everything to be replayed from the WBL. So we delete the m-map files.
	require.NoError(t, os.RemoveAll(mmappedChunksDir(db.head.opts.ChunkDirRoot)))

	// Restart does the replay and repair.
	db, err = Open(db.dir, nil, nil, opts, nil)
	require.NoError(t, err)
	require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.walCorruptionsTotal))
	require.Less(t, len(expAfterRestart), len(allSamples))
	verifySamples(expAfterRestart)

	// Verify that it did the repair on disk.
	files, err = os.ReadDir(db.head.wbl.Dir())
	require.NoError(t, err)
	require.Len(t, files, 3)
	f0, err = files[0].Info()
	require.NoError(t, err)
	require.Greater(t, f0.Size(), int64(100))
	f2, err = files[2].Info()
	require.NoError(t, err)
	require.Equal(t, int64(0), f2.Size())
	require.Equal(t, corruptFilePath, path.Join(db.head.wbl.Dir(), files[1].Name()))

	// Verifying that everything after the corruption point is set to 0.
	b, err := os.ReadFile(corruptFilePath)
	require.NoError(t, err)
	sum := 0
	for _, val := range b[corruptIndex:] {
		sum += int(val)
	}
	require.Equal(t, 0, sum)

	// Another restart, everything normal with no repair.
	require.NoError(t, db.Close())
	db, err = Open(db.dir, nil, nil, opts, nil)
	require.NoError(t, err)
	require.Equal(t, 0.0, prom_testutil.ToFloat64(db.head.metrics.walCorruptionsTotal))
	verifySamples(expAfterRestart)
}

func TestOOOMmapCorruption(t *testing.T) {
	dir := t.TempDir()

	opts := DefaultOptions()
	opts.OutOfOrderCapMin = 2
	opts.OutOfOrderCapMax = 10
	opts.OutOfOrderTimeWindow = 300 * time.Minute.Milliseconds()
	opts.AllowOverlappingQueries = true
	opts.AllowOverlappingCompaction = true

	db, err := Open(dir, nil, nil, opts, nil)
	require.NoError(t, err)
	db.DisableCompactions()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	series1 := labels.FromStrings("foo", "bar1")
	var allSamples, expInMmapChunks []tsdbutil.Sample
	addSamples := func(fromMins, toMins int64, inMmapAfterCorruption bool) {
		app := db.Appender(context.Background())
		for min := fromMins; min <= toMins; min++ {
			ts := min * time.Minute.Milliseconds()
			_, err := app.Append(0, series1, ts, float64(ts))
			require.NoError(t, err)
			allSamples = append(allSamples, sample{t: ts, v: float64(ts)})
			if inMmapAfterCorruption {
				expInMmapChunks = append(expInMmapChunks, sample{t: ts, v: float64(ts)})
			}
		}
		require.NoError(t, app.Commit())
	}

	// Add an in-order samples.
	addSamples(340, 350, true)

	// OOO samples.
	addSamples(90, 99, true)
	addSamples(100, 109, true)
	// This sample m-maps a chunk. But 120 goes into a new chunk.
	addSamples(120, 120, false)

	// Second m-map file. We will corrupt this file. Sample 120 goes into this new file.
	require.NoError(t, db.head.chunkDiskMapper.CutNewFile())

	// More OOO samples.
	addSamples(200, 230, false)
	addSamples(240, 255, false)

	require.NoError(t, db.head.chunkDiskMapper.CutNewFile())
	addSamples(260, 290, false)

	verifySamples := func(expSamples []tsdbutil.Sample) {
		sort.Slice(expSamples, func(i, j int) bool {
			return expSamples[i].T() < expSamples[j].T()
		})

		expRes := map[string][]tsdbutil.Sample{
			series1.String(): expSamples,
		}

		q, err := db.Querier(context.Background(), math.MinInt64, math.MaxInt64)
		require.NoError(t, err)

		actRes := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		require.Equal(t, expRes, actRes)
	}

	verifySamples(allSamples)

	// Verifying existing files.
	mmapDir := mmappedChunksDir(db.head.opts.ChunkDirRoot)
	files, err := os.ReadDir(mmapDir)
	require.NoError(t, err)
	require.Len(t, files, 3)

	// Corrupting the 2nd file.
	f, err := os.OpenFile(path.Join(mmapDir, files[1].Name()), os.O_RDWR, 0o666)
	require.NoError(t, err)
	_, err = f.WriteAt([]byte{99, 9, 99, 9, 99}, 20)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	firstFileName := files[0].Name()

	require.NoError(t, db.Close())

	// Moving OOO WBL to use it later.
	wblDir := db.head.wbl.Dir()
	wblDirTmp := path.Join(t.TempDir(), "wbl_tmp")
	require.NoError(t, os.Rename(wblDir, wblDirTmp))

	// Restart does the replay and repair of m-map files.
	db, err = Open(db.dir, nil, nil, opts, nil)
	require.NoError(t, err)
	require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.mmapChunkCorruptionTotal))
	require.Less(t, len(expInMmapChunks), len(allSamples))

	// Since there is no WBL, only samples from m-map chunks comes in the query.
	verifySamples(expInMmapChunks)

	// Verify that it did the repair on disk. All files from the point of corruption
	// should be deleted.
	files, err = os.ReadDir(mmapDir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	f0, err := files[0].Info()
	require.NoError(t, err)
	require.Greater(t, f0.Size(), int64(100))
	require.Equal(t, firstFileName, files[0].Name())

	// Another restart, everything normal with no repair.
	require.NoError(t, db.Close())
	db, err = Open(db.dir, nil, nil, opts, nil)
	require.NoError(t, err)
	require.Equal(t, 0.0, prom_testutil.ToFloat64(db.head.metrics.mmapChunkCorruptionTotal))
	verifySamples(expInMmapChunks)

	// Restart again with the WBL, all samples should be present now.
	require.NoError(t, db.Close())
	require.NoError(t, os.RemoveAll(wblDir))
	require.NoError(t, os.Rename(wblDirTmp, wblDir))
	db, err = Open(db.dir, nil, nil, opts, nil)
	require.NoError(t, err)
	verifySamples(allSamples)
}

func TestOutOfOrderRuntimeConfig(t *testing.T) {
	getDB := func(oooTimeWindow int64) *DB {
		dir := t.TempDir()

		opts := DefaultOptions()
		opts.OutOfOrderTimeWindow = oooTimeWindow

		db, err := Open(dir, nil, nil, opts, nil)
		require.NoError(t, err)
		db.DisableCompactions()
		t.Cleanup(func() {
			require.NoError(t, db.Close())
		})

		return db
	}

	makeConfig := func(oooTimeWindow int) *config.Config {
		return &config.Config{
			StorageConfig: config.StorageConfig{
				TSDBConfig: &config.TSDBConfig{
					OutOfOrderTimeWindow: int64(oooTimeWindow) * time.Minute.Milliseconds(),
				},
			},
		}
	}

	series1 := labels.FromStrings("foo", "bar1")
	addSamples := func(t *testing.T, db *DB, fromMins, toMins int64, success bool, allSamples []tsdbutil.Sample) []tsdbutil.Sample {
		app := db.Appender(context.Background())
		for min := fromMins; min <= toMins; min++ {
			ts := min * time.Minute.Milliseconds()
			_, err := app.Append(0, series1, ts, float64(ts))
			if success {
				require.NoError(t, err)
				allSamples = append(allSamples, sample{t: ts, v: float64(ts)})
			} else {
				require.Error(t, err)
			}
		}
		require.NoError(t, app.Commit())
		return allSamples
	}

	verifySamples := func(t *testing.T, db *DB, expSamples []tsdbutil.Sample) {
		sort.Slice(expSamples, func(i, j int) bool {
			return expSamples[i].T() < expSamples[j].T()
		})

		expRes := map[string][]tsdbutil.Sample{
			series1.String(): expSamples,
		}

		q, err := db.Querier(context.Background(), math.MinInt64, math.MaxInt64)
		require.NoError(t, err)

		actRes := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		require.Equal(t, expRes, actRes)
	}

	doOOOCompaction := func(t *testing.T, db *DB) {
		// WBL is not empty.
		size, err := db.head.wbl.Size()
		require.NoError(t, err)
		require.Greater(t, size, int64(0))

		require.Len(t, db.Blocks(), 0)
		require.NoError(t, db.compactOOOHead())
		require.Greater(t, len(db.Blocks()), 0)

		// WBL is empty.
		size, err = db.head.wbl.Size()
		require.NoError(t, err)
		require.Equal(t, int64(0), size)
	}

	t.Run("increase time window", func(t *testing.T) {
		var allSamples []tsdbutil.Sample
		db := getDB(30 * time.Minute.Milliseconds())

		// In-order.
		allSamples = addSamples(t, db, 300, 310, true, allSamples)

		// OOO upto 30m old is success.
		allSamples = addSamples(t, db, 281, 290, true, allSamples)

		// OOO of 59m old fails.
		s := addSamples(t, db, 251, 260, false, nil)
		require.Len(t, s, 0)
		verifySamples(t, db, allSamples)

		oldWblPtr := fmt.Sprintf("%p", db.head.wbl)

		// Increase time window and try adding again.
		err := db.ApplyConfig(makeConfig(60))
		require.NoError(t, err)
		allSamples = addSamples(t, db, 251, 260, true, allSamples)

		// WBL does not change.
		newWblPtr := fmt.Sprintf("%p", db.head.wbl)
		require.Equal(t, oldWblPtr, newWblPtr)

		doOOOCompaction(t, db)
		verifySamples(t, db, allSamples)
	})

	t.Run("decrease time window and increase again", func(t *testing.T) {
		var allSamples []tsdbutil.Sample
		db := getDB(60 * time.Minute.Milliseconds())

		// In-order.
		allSamples = addSamples(t, db, 300, 310, true, allSamples)

		// OOO upto 59m old is success.
		allSamples = addSamples(t, db, 251, 260, true, allSamples)

		oldWblPtr := fmt.Sprintf("%p", db.head.wbl)
		// Decrease time window.
		err := db.ApplyConfig(makeConfig(30))
		require.NoError(t, err)

		// OOO of 49m old fails.
		s := addSamples(t, db, 261, 270, false, nil)
		require.Len(t, s, 0)

		// WBL does not change.
		newWblPtr := fmt.Sprintf("%p", db.head.wbl)
		require.Equal(t, oldWblPtr, newWblPtr)

		verifySamples(t, db, allSamples)

		// Increase time window again and check
		err = db.ApplyConfig(makeConfig(60))
		require.NoError(t, err)
		allSamples = addSamples(t, db, 261, 270, true, allSamples)
		verifySamples(t, db, allSamples)

		// WBL does not change.
		newWblPtr = fmt.Sprintf("%p", db.head.wbl)
		require.Equal(t, oldWblPtr, newWblPtr)

		doOOOCompaction(t, db)
		verifySamples(t, db, allSamples)
	})

	t.Run("disabled to enabled", func(t *testing.T) {
		var allSamples []tsdbutil.Sample
		db := getDB(0)

		// In-order.
		allSamples = addSamples(t, db, 300, 310, true, allSamples)

		// OOO fails.
		s := addSamples(t, db, 251, 260, false, nil)
		require.Len(t, s, 0)
		verifySamples(t, db, allSamples)

		require.Nil(t, db.head.wbl)

		// Increase time window and try adding again.
		err := db.ApplyConfig(makeConfig(60))
		require.NoError(t, err)
		allSamples = addSamples(t, db, 251, 260, true, allSamples)

		// WBL gets created.
		require.NotNil(t, db.head.wbl)

		verifySamples(t, db, allSamples)

		// OOO compaction works now.
		doOOOCompaction(t, db)
		verifySamples(t, db, allSamples)
	})

	t.Run("enabled to disabled", func(t *testing.T) {
		var allSamples []tsdbutil.Sample
		db := getDB(60 * time.Minute.Milliseconds())

		// In-order.
		allSamples = addSamples(t, db, 300, 310, true, allSamples)

		// OOO upto 59m old is success.
		allSamples = addSamples(t, db, 251, 260, true, allSamples)

		oldWblPtr := fmt.Sprintf("%p", db.head.wbl)
		// Time Window to 0, hence disabled.
		err := db.ApplyConfig(makeConfig(0))
		require.NoError(t, err)

		// OOO within old time window fails.
		s := addSamples(t, db, 290, 309, false, nil)
		require.Len(t, s, 0)

		// WBL does not change and is not removed.
		newWblPtr := fmt.Sprintf("%p", db.head.wbl)
		require.Equal(t, oldWblPtr, newWblPtr)

		verifySamples(t, db, allSamples)

		// Compaction still works after disabling with WBL cleanup.
		doOOOCompaction(t, db)
		verifySamples(t, db, allSamples)
	})

	t.Run("disabled to disabled", func(t *testing.T) {
		var allSamples []tsdbutil.Sample
		db := getDB(0)

		// In-order.
		allSamples = addSamples(t, db, 300, 310, true, allSamples)

		// OOO fails.
		s := addSamples(t, db, 290, 309, false, nil)
		require.Len(t, s, 0)
		verifySamples(t, db, allSamples)
		require.Nil(t, db.head.wbl)

		// Time window to 0.
		err := db.ApplyConfig(makeConfig(0))
		require.NoError(t, err)

		// OOO still fails.
		s = addSamples(t, db, 290, 309, false, nil)
		require.Len(t, s, 0)
		verifySamples(t, db, allSamples)
		require.Nil(t, db.head.wbl)
	})
}

func TestNoGapAfterRestartWithOOO(t *testing.T) {
	series1 := labels.FromStrings("foo", "bar1")
	addSamples := func(t *testing.T, db *DB, fromMins, toMins int64, success bool) {
		app := db.Appender(context.Background())
		for min := fromMins; min <= toMins; min++ {
			ts := min * time.Minute.Milliseconds()
			_, err := app.Append(0, series1, ts, float64(ts))
			if success {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		}
		require.NoError(t, app.Commit())
	}

	verifySamples := func(t *testing.T, db *DB, fromMins, toMins int64) {
		var expSamples []tsdbutil.Sample
		for min := fromMins; min <= toMins; min++ {
			ts := min * time.Minute.Milliseconds()
			expSamples = append(expSamples, sample{t: ts, v: float64(ts)})
		}

		expRes := map[string][]tsdbutil.Sample{
			series1.String(): expSamples,
		}

		q, err := db.Querier(context.Background(), math.MinInt64, math.MaxInt64)
		require.NoError(t, err)

		actRes := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		require.Equal(t, expRes, actRes)
	}

	cases := []struct {
		inOrderMint, inOrderMaxt int64
		oooMint, oooMaxt         int64
		// After compaction.
		blockRanges        [][2]int64
		headMint, headMaxt int64
	}{
		{
			300, 490,
			489, 489,
			[][2]int64{{300, 360}, {480, 600}},
			360, 490,
		},
		{
			300, 490,
			479, 479,
			[][2]int64{{300, 360}, {360, 480}},
			360, 490,
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("case=%d", i), func(t *testing.T) {
			dir := t.TempDir()

			opts := DefaultOptions()
			opts.OutOfOrderTimeWindow = 30 * time.Minute.Milliseconds()

			db, err := Open(dir, nil, nil, opts, nil)
			require.NoError(t, err)
			db.DisableCompactions()
			t.Cleanup(func() {
				require.NoError(t, db.Close())
			})

			// 3h10m=190m worth in-order data.
			addSamples(t, db, c.inOrderMint, c.inOrderMaxt, true)
			verifySamples(t, db, c.inOrderMint, c.inOrderMaxt)

			// One ooo samples.
			addSamples(t, db, c.oooMint, c.oooMaxt, true)
			verifySamples(t, db, c.inOrderMint, c.inOrderMaxt)

			// We get 2 blocks. 1 from OOO, 1 from in-order.
			require.NoError(t, db.Compact())
			verifyBlockRanges := func() {
				blocks := db.Blocks()
				require.Equal(t, len(c.blockRanges), len(blocks))
				for j, br := range c.blockRanges {
					require.Equal(t, br[0]*time.Minute.Milliseconds(), blocks[j].MinTime())
					require.Equal(t, br[1]*time.Minute.Milliseconds(), blocks[j].MaxTime())
				}
			}
			verifyBlockRanges()
			require.Equal(t, c.headMint*time.Minute.Milliseconds(), db.head.MinTime())
			require.Equal(t, c.headMaxt*time.Minute.Milliseconds(), db.head.MaxTime())

			// Restart and expect all samples to be present.
			require.NoError(t, db.Close())

			db, err = Open(dir, nil, nil, opts, nil)
			require.NoError(t, err)
			db.DisableCompactions()

			verifyBlockRanges()
			require.Equal(t, c.headMint*time.Minute.Milliseconds(), db.head.MinTime())
			require.Equal(t, c.headMaxt*time.Minute.Milliseconds(), db.head.MaxTime())
			verifySamples(t, db, c.inOrderMint, c.inOrderMaxt)
		})
	}
}
