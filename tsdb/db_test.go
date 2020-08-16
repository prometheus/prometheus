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
	"fmt"
	"hash/crc32"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wal"
	"github.com/prometheus/prometheus/util/testutil"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func openTestDB(t testing.TB, opts *Options, rngs []int64) (db *DB) {
	tmpdir, err := ioutil.TempDir("", "test")
	testutil.Ok(t, err)

	if len(rngs) == 0 {
		db, err = Open(tmpdir, nil, nil, opts)
	} else {
		opts, rngs = validateOpts(opts, rngs)
		db, err = open(tmpdir, nil, nil, opts, rngs)
	}
	testutil.Ok(t, err)

	// Do not Close() the test database by default as it will deadlock on test failures.
	t.Cleanup(func() {
		testutil.Ok(t, os.RemoveAll(tmpdir))
	})
	return db
}

// query runs a matcher query against the querier and fully expands its data.
func query(t testing.TB, q storage.Querier, matchers ...*labels.Matcher) map[string][]tsdbutil.Sample {
	ss := q.Select(false, nil, matchers...)
	defer func() {
		testutil.Ok(t, q.Close())
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
		testutil.Ok(t, it.Err())

		if len(samples) == 0 {
			continue
		}

		name := series.Labels().String()
		result[name] = samples
	}
	testutil.Ok(t, ss.Err())
	testutil.Equals(t, 0, len(ss.Warnings()))

	return result
}

// queryChunks runs a matcher query against the querier and fully expands its data.
func queryChunks(t testing.TB, q storage.ChunkQuerier, matchers ...*labels.Matcher) map[string][]chunks.Meta {
	ss := q.Select(false, nil, matchers...)
	defer func() {
		testutil.Ok(t, q.Close())
	}()

	result := map[string][]chunks.Meta{}
	for ss.Next() {
		series := ss.At()

		chks := []chunks.Meta{}
		it := series.Iterator()
		for it.Next() {
			chks = append(chks, it.At())
		}
		testutil.Ok(t, it.Err())

		if len(chks) == 0 {
			continue
		}

		name := series.Labels().String()
		result[name] = chks
	}
	testutil.Ok(t, ss.Err())
	testutil.Equals(t, 0, len(ss.Warnings()))
	return result
}

// Ensure that blocks are held in memory in their time order
// and not in ULID order as they are read from the directory.
func TestDB_reloadOrder(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
	}()

	metas := []BlockMeta{
		{MinTime: 90, MaxTime: 100},
		{MinTime: 70, MaxTime: 80},
		{MinTime: 100, MaxTime: 110},
	}
	for _, m := range metas {
		createBlock(t, db.Dir(), genSeries(1, 1, m.MinTime, m.MaxTime))
	}

	testutil.Ok(t, db.reload())
	blocks := db.Blocks()
	testutil.Equals(t, 3, len(blocks))
	testutil.Equals(t, metas[1].MinTime, blocks[0].Meta().MinTime)
	testutil.Equals(t, metas[1].MaxTime, blocks[0].Meta().MaxTime)
	testutil.Equals(t, metas[0].MinTime, blocks[1].Meta().MinTime)
	testutil.Equals(t, metas[0].MaxTime, blocks[1].Meta().MaxTime)
	testutil.Equals(t, metas[2].MinTime, blocks[2].Meta().MinTime)
	testutil.Equals(t, metas[2].MaxTime, blocks[2].Meta().MaxTime)
}

func TestDataAvailableOnlyAfterCommit(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
	}()

	ctx := context.Background()
	app := db.Appender(ctx)

	_, err := app.Add(labels.FromStrings("foo", "bar"), 0, 0)
	testutil.Ok(t, err)

	querier, err := db.Querier(context.TODO(), 0, 1)
	testutil.Ok(t, err)
	seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	testutil.Equals(t, map[string][]tsdbutil.Sample{}, seriesSet)

	err = app.Commit()
	testutil.Ok(t, err)

	querier, err = db.Querier(context.TODO(), 0, 1)
	testutil.Ok(t, err)
	defer querier.Close()

	seriesSet = query(t, querier, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))

	testutil.Equals(t, map[string][]tsdbutil.Sample{`{foo="bar"}`: {sample{t: 0, v: 0}}}, seriesSet)
}

// TestNoPanicAfterWALCorrutpion ensures that querying the db after a WAL corruption doesn't cause a panic.
// https://github.com/prometheus/prometheus/issues/7548
func TestNoPanicAfterWALCorrutpion(t *testing.T) {
	db := openTestDB(t, &Options{WALSegmentSize: 32 * 1024}, nil)

	// Append until the the first mmaped head chunk.
	// This is to ensure that all samples can be read from the mmaped chunks when the WAL is corrupted.
	var expSamples []tsdbutil.Sample
	var maxt int64
	ctx := context.Background()
	{
		for {
			app := db.Appender(ctx)
			_, err := app.Add(labels.FromStrings("foo", "bar"), maxt, 0)
			expSamples = append(expSamples, sample{t: maxt, v: 0})
			testutil.Ok(t, err)
			testutil.Ok(t, app.Commit())
			mmapedChunks, err := ioutil.ReadDir(mmappedChunksDir(db.Dir()))
			testutil.Ok(t, err)
			if len(mmapedChunks) > 0 {
				break
			}
			maxt++
		}
		testutil.Ok(t, db.Close())
	}

	// Corrupt the WAL after the first sample of the series so that it has at least one sample and
	// it is not garbage collected.
	// The repair deletes all WAL records after the corrupted record and these are read from the mmaped chunk.
	{
		walFiles, err := ioutil.ReadDir(path.Join(db.Dir(), "wal"))
		testutil.Ok(t, err)
		f, err := os.OpenFile(path.Join(db.Dir(), "wal", walFiles[0].Name()), os.O_RDWR, 0666)
		testutil.Ok(t, err)
		r := wal.NewReader(bufio.NewReader(f))
		testutil.Assert(t, r.Next(), "reading the series record")
		testutil.Assert(t, r.Next(), "reading the first sample record")
		// Write an invalid record header to corrupt everything after the first wal sample.
		_, err = f.WriteAt([]byte{99}, r.Offset())
		testutil.Ok(t, err)
		f.Close()
	}

	// Query the data.
	{
		db, err := Open(db.Dir(), nil, nil, nil)
		testutil.Ok(t, err)
		defer func() {
			testutil.Ok(t, db.Close())
		}()
		testutil.Equals(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.walCorruptionsTotal), "WAL corruption count mismatch")

		querier, err := db.Querier(context.TODO(), 0, maxt)
		testutil.Ok(t, err)
		seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchEqual, "", ""))
		// The last sample should be missing as it was after the WAL segment corruption.
		testutil.Equals(t, map[string][]tsdbutil.Sample{`{foo="bar"}`: expSamples[0 : len(expSamples)-1]}, seriesSet)
	}
}

func TestDataNotAvailableAfterRollback(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
	}()

	app := db.Appender(context.Background())
	_, err := app.Add(labels.FromStrings("foo", "bar"), 0, 0)
	testutil.Ok(t, err)

	err = app.Rollback()
	testutil.Ok(t, err)

	querier, err := db.Querier(context.TODO(), 0, 1)
	testutil.Ok(t, err)
	defer querier.Close()

	seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))

	testutil.Equals(t, map[string][]tsdbutil.Sample{}, seriesSet)
}

func TestDBAppenderAddRef(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
	}()

	ctx := context.Background()
	app1 := db.Appender(ctx)

	ref1, err := app1.Add(labels.FromStrings("a", "b"), 123, 0)
	testutil.Ok(t, err)

	// Reference should already work before commit.
	err = app1.AddFast(ref1, 124, 1)
	testutil.Ok(t, err)

	err = app1.Commit()
	testutil.Ok(t, err)

	app2 := db.Appender(ctx)

	// first ref should already work in next transaction.
	err = app2.AddFast(ref1, 125, 0)
	testutil.Ok(t, err)

	ref2, err := app2.Add(labels.FromStrings("a", "b"), 133, 1)
	testutil.Ok(t, err)

	testutil.Assert(t, ref1 == ref2, "")

	// Reference must be valid to add another sample.
	err = app2.AddFast(ref2, 143, 2)
	testutil.Ok(t, err)

	err = app2.AddFast(9999999, 1, 1)
	testutil.Equals(t, storage.ErrNotFound, errors.Cause(err))

	testutil.Ok(t, app2.Commit())

	q, err := db.Querier(context.TODO(), 0, 200)
	testutil.Ok(t, err)

	res := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

	testutil.Equals(t, map[string][]tsdbutil.Sample{
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
		testutil.Ok(t, db.Close())
	}()

	ctx := context.Background()
	app1 := db.Appender(ctx)

	ref1, err := app1.Add(labels.FromStrings("a", "b"), 123, 0)
	testutil.Ok(t, err)

	// Construct labels manually so there is an empty label.
	ref2, err := app1.Add(labels.Labels{labels.Label{Name: "a", Value: "b"}, labels.Label{Name: "c", Value: ""}}, 124, 0)
	testutil.Ok(t, err)

	// Should be the same series.
	testutil.Equals(t, ref1, ref2)

	err = app1.Commit()
	testutil.Ok(t, err)
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
			testutil.Ok(t, db.Close())
		}()

		ctx := context.Background()
		app := db.Appender(ctx)

		smpls := make([]float64, numSamples)
		for i := int64(0); i < numSamples; i++ {
			smpls[i] = rand.Float64()
			app.Add(labels.Labels{{Name: "a", Value: "b"}}, i, smpls[i])
		}

		testutil.Ok(t, app.Commit())

		// TODO(gouthamve): Reset the tombstones somehow.
		// Delete the ranges.
		for _, r := range c.Intervals {
			testutil.Ok(t, db.Delete(r.Mint, r.Maxt, labels.MustNewMatcher(labels.MatchEqual, "a", "b")))
		}

		// Compare the result.
		q, err := db.Querier(context.TODO(), 0, numSamples)
		testutil.Ok(t, err)

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
			testutil.Equals(t, eok, rok)

			if !eok {
				testutil.Equals(t, 0, len(res.Warnings()))
				continue Outer
			}
			sexp := expss.At()
			sres := res.At()

			testutil.Equals(t, sexp.Labels(), sres.Labels())

			smplExp, errExp := storage.ExpandSamples(sexp.Iterator(), nil)
			smplRes, errRes := storage.ExpandSamples(sres.Iterator(), nil)

			testutil.Equals(t, errExp, errRes)
			testutil.Equals(t, smplExp, smplRes)
		}
	}
}

func TestAmendDatapointCausesError(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
	}()

	ctx := context.Background()
	app := db.Appender(ctx)
	_, err := app.Add(labels.Labels{{Name: "a", Value: "b"}}, 0, 0)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	app = db.Appender(ctx)
	_, err = app.Add(labels.Labels{{Name: "a", Value: "b"}}, 0, 1)
	testutil.Equals(t, storage.ErrDuplicateSampleForTimestamp, err)
	testutil.Ok(t, app.Rollback())
}

func TestDuplicateNaNDatapointNoAmendError(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
	}()

	ctx := context.Background()
	app := db.Appender(ctx)
	_, err := app.Add(labels.Labels{{Name: "a", Value: "b"}}, 0, math.NaN())
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	app = db.Appender(ctx)
	_, err = app.Add(labels.Labels{{Name: "a", Value: "b"}}, 0, math.NaN())
	testutil.Ok(t, err)
}

func TestNonDuplicateNaNDatapointsCausesAmendError(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
	}()

	ctx := context.Background()
	app := db.Appender(ctx)
	_, err := app.Add(labels.Labels{{Name: "a", Value: "b"}}, 0, math.Float64frombits(0x7ff0000000000001))
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	app = db.Appender(ctx)
	_, err = app.Add(labels.Labels{{Name: "a", Value: "b"}}, 0, math.Float64frombits(0x7ff0000000000002))
	testutil.Equals(t, storage.ErrDuplicateSampleForTimestamp, err)
}

func TestEmptyLabelsetCausesError(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
	}()

	ctx := context.Background()
	app := db.Appender(ctx)
	_, err := app.Add(labels.Labels{}, 0, 0)
	testutil.NotOk(t, err)
	testutil.Equals(t, "empty labelset: invalid sample", err.Error())
}

func TestSkippingInvalidValuesInSameTxn(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
	}()

	// Append AmendedValue.
	ctx := context.Background()
	app := db.Appender(ctx)
	_, err := app.Add(labels.Labels{{Name: "a", Value: "b"}}, 0, 1)
	testutil.Ok(t, err)
	_, err = app.Add(labels.Labels{{Name: "a", Value: "b"}}, 0, 2)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	// Make sure the right value is stored.
	q, err := db.Querier(context.TODO(), 0, 10)
	testutil.Ok(t, err)

	ssMap := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

	testutil.Equals(t, map[string][]tsdbutil.Sample{
		labels.New(labels.Label{Name: "a", Value: "b"}).String(): {sample{0, 1}},
	}, ssMap)

	// Append Out of Order Value.
	app = db.Appender(ctx)
	_, err = app.Add(labels.Labels{{Name: "a", Value: "b"}}, 10, 3)
	testutil.Ok(t, err)
	_, err = app.Add(labels.Labels{{Name: "a", Value: "b"}}, 7, 5)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	q, err = db.Querier(context.TODO(), 0, 10)
	testutil.Ok(t, err)

	ssMap = query(t, q, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

	testutil.Equals(t, map[string][]tsdbutil.Sample{
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
		_, err := app.Add(labels.FromStrings("foo", "bar"), mint+int64(i), 1.0)
		testutil.Ok(t, err)
	}
	testutil.Ok(t, app.Commit())

	// create snapshot
	snap, err := ioutil.TempDir("", "snap")
	testutil.Ok(t, err)

	defer func() {
		testutil.Ok(t, os.RemoveAll(snap))
	}()
	testutil.Ok(t, db.Snapshot(snap, true))
	testutil.Ok(t, db.Close())

	// reopen DB from snapshot
	db, err = Open(snap, nil, nil, nil)
	testutil.Ok(t, err)
	defer func() { testutil.Ok(t, db.Close()) }()

	querier, err := db.Querier(context.TODO(), mint, mint+1000)
	testutil.Ok(t, err)
	defer func() { testutil.Ok(t, querier.Close()) }()

	// sum values
	seriesSet := querier.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	sum := 0.0
	for seriesSet.Next() {
		series := seriesSet.At().Iterator()
		for series.Next() {
			_, v := series.At()
			sum += v
		}
		testutil.Ok(t, series.Err())
	}
	testutil.Ok(t, seriesSet.Err())
	testutil.Equals(t, 0, len(seriesSet.Warnings()))
	testutil.Equals(t, 1000.0, sum)
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
		_, err := app.Add(labels.FromStrings("foo", "bar"), mint+int64(i), 1.0)
		testutil.Ok(t, err)
	}
	testutil.Ok(t, app.Commit())

	snap, err := ioutil.TempDir("", "snap")
	testutil.Ok(t, err)

	// Hackingly introduce "race", by having lower max time then maxTime in last chunk.
	db.head.maxTime.Sub(10)

	defer func() {
		testutil.Ok(t, os.RemoveAll(snap))
	}()
	testutil.Ok(t, db.Snapshot(snap, true))
	testutil.Ok(t, db.Close())

	// Reopen DB from snapshot.
	db, err = Open(snap, nil, nil, nil)
	testutil.Ok(t, err)
	defer func() { testutil.Ok(t, db.Close()) }()

	querier, err := db.Querier(context.TODO(), mint, mint+1000)
	testutil.Ok(t, err)
	defer func() { testutil.Ok(t, querier.Close()) }()

	// Sum values.
	seriesSet := querier.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	sum := 0.0
	for seriesSet.Next() {
		series := seriesSet.At().Iterator()
		for series.Next() {
			_, v := series.At()
			sum += v
		}
		testutil.Ok(t, series.Err())
	}
	testutil.Ok(t, seriesSet.Err())
	testutil.Equals(t, 0, len(seriesSet.Warnings()))

	// Since we snapshotted with MaxTime - 10, so expect 10 less samples.
	testutil.Equals(t, 1000.0-10, sum)
}

func TestDB_SnapshotWithDelete(t *testing.T) {
	numSamples := int64(10)

	db := openTestDB(t, nil, nil)

	ctx := context.Background()
	app := db.Appender(ctx)

	smpls := make([]float64, numSamples)
	for i := int64(0); i < numSamples; i++ {
		smpls[i] = rand.Float64()
		app.Add(labels.Labels{{Name: "a", Value: "b"}}, i, smpls[i])
	}

	testutil.Ok(t, app.Commit())
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
			testutil.Ok(t, db.Delete(r.Mint, r.Maxt, labels.MustNewMatcher(labels.MatchEqual, "a", "b")))
		}

		// create snapshot
		snap, err := ioutil.TempDir("", "snap")
		testutil.Ok(t, err)

		defer func() {
			testutil.Ok(t, os.RemoveAll(snap))
		}()
		testutil.Ok(t, db.Snapshot(snap, true))
		testutil.Ok(t, db.Close())

		// reopen DB from snapshot
		db, err = Open(snap, nil, nil, nil)
		testutil.Ok(t, err)
		defer func() { testutil.Ok(t, db.Close()) }()

		// Compare the result.
		q, err := db.Querier(context.TODO(), 0, numSamples)
		testutil.Ok(t, err)
		defer func() { testutil.Ok(t, q.Close()) }()

		res := q.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

		expSamples := make([]tsdbutil.Sample, 0, len(c.remaint))
		for _, ts := range c.remaint {
			expSamples = append(expSamples, sample{ts, smpls[ts]})
		}

		expss := newMockSeriesSet([]storage.Series{
			storage.NewListSeries(labels.FromStrings("a", "b"), expSamples),
		})

		if len(expSamples) == 0 {
			testutil.Assert(t, res.Next() == false, "")
			continue
		}

		for {
			eok, rok := expss.Next(), res.Next()
			testutil.Equals(t, eok, rok)

			if !eok {
				testutil.Equals(t, 0, len(res.Warnings()))
				continue Outer
			}
			sexp := expss.At()
			sres := res.At()

			testutil.Equals(t, sexp.Labels(), sres.Labels())

			smplExp, errExp := storage.ExpandSamples(sexp.Iterator(), nil)
			smplRes, errRes := storage.ExpandSamples(sres.Iterator(), nil)

			testutil.Equals(t, errExp, errRes)
			testutil.Equals(t, smplExp, smplRes)
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
		testutil.Ok(t, db.Close())
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

			_, err := app.Add(lset, ts, v)
			testutil.Ok(t, err)

			ts += rand.Int63n(timeInterval) + 1
		}

		seriesMap[lset.String()] = series
	}

	testutil.Ok(t, app.Commit())

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
			testutil.Ok(t, err)

			ss := q.Select(false, nil, qry.ms...)
			result := map[string][]tsdbutil.Sample{}

			for ss.Next() {
				x := ss.At()

				smpls, err := storage.ExpandSamples(x.Iterator(), newSample)
				testutil.Ok(t, err)

				if len(smpls) > 0 {
					result[x.Labels().String()] = smpls
				}
			}

			testutil.Ok(t, ss.Err())
			testutil.Equals(t, 0, len(ss.Warnings()))
			testutil.Equals(t, expected, result)

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
	_, err := app.Add(lbls, 0, 1)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	testutil.Ok(t, db.Close())

	db, err = Open(dirDb, nil, nil, nil)
	testutil.Ok(t, err)
	defer func() { testutil.Ok(t, db.Close()) }()

	q, err := db.Querier(context.TODO(), 0, 1)
	testutil.Ok(t, err)

	values, ws, err := q.LabelValues("labelname")
	testutil.Ok(t, err)
	testutil.Equals(t, 0, len(ws))
	testutil.Equals(t, []string{"labelvalue"}, values)
}

func TestWALSegmentSizeOptions(t *testing.T) {
	tests := map[int]func(dbdir string, segmentSize int){
		// Default Wal Size.
		0: func(dbDir string, segmentSize int) {
			filesAndDir, err := ioutil.ReadDir(filepath.Join(dbDir, "wal"))
			testutil.Ok(t, err)
			files := []os.FileInfo{}
			for _, f := range filesAndDir {
				if !f.IsDir() {
					files = append(files, f)
				}
			}
			// All the full segment files (all but the last) should match the segment size option.
			for _, f := range files[:len(files)-1] {
				testutil.Equals(t, int64(DefaultOptions().WALSegmentSize), f.Size(), "WAL file size doesn't match WALSegmentSize option, filename: %v", f.Name())
			}
			lastFile := files[len(files)-1]
			testutil.Assert(t, int64(DefaultOptions().WALSegmentSize) > lastFile.Size(), "last WAL file size is not smaller than the WALSegmentSize option, filename: %v", lastFile.Name())
		},
		// Custom Wal Size.
		2 * 32 * 1024: func(dbDir string, segmentSize int) {
			filesAndDir, err := ioutil.ReadDir(filepath.Join(dbDir, "wal"))
			testutil.Ok(t, err)
			files := []os.FileInfo{}
			for _, f := range filesAndDir {
				if !f.IsDir() {
					files = append(files, f)
				}
			}
			testutil.Assert(t, len(files) > 1, "current WALSegmentSize should result in more than a single WAL file.")
			// All the full segment files (all but the last) should match the segment size option.
			for _, f := range files[:len(files)-1] {
				testutil.Equals(t, int64(segmentSize), f.Size(), "WAL file size doesn't match WALSegmentSize option, filename: %v", f.Name())
			}
			lastFile := files[len(files)-1]
			testutil.Assert(t, int64(segmentSize) > lastFile.Size(), "last WAL file size is not smaller than the WALSegmentSize option, filename: %v", lastFile.Name())
		},
		// Wal disabled.
		-1: func(dbDir string, segmentSize int) {
			// Check that WAL dir is not there.
			_, err := os.Stat(filepath.Join(dbDir, "wal"))
			testutil.NotOk(t, err)
			// Check that there is chunks dir.
			_, err = os.Stat(mmappedChunksDir(dbDir))
			testutil.Ok(t, err)
		},
	}
	for segmentSize, testFunc := range tests {
		t.Run(fmt.Sprintf("WALSegmentSize %d test", segmentSize), func(t *testing.T) {
			opts := DefaultOptions()
			opts.WALSegmentSize = segmentSize
			db := openTestDB(t, opts, nil)

			for i := int64(0); i < 155; i++ {
				app := db.Appender(context.Background())
				ref, err := app.Add(labels.Labels{labels.Label{Name: "wal" + fmt.Sprintf("%d", i), Value: "size"}}, i, rand.Float64())
				testutil.Ok(t, err)
				for j := int64(1); j <= 78; j++ {
					err := app.AddFast(ref, i+j, rand.Float64())
					testutil.Ok(t, err)
				}
				testutil.Ok(t, app.Commit())
			}

			dbDir := db.Dir()
			testutil.Ok(t, db.Close())
			testFunc(dbDir, int(opts.WALSegmentSize))
		})
	}
}

func TestTombstoneClean(t *testing.T) {
	numSamples := int64(10)

	db := openTestDB(t, nil, nil)

	ctx := context.Background()
	app := db.Appender(ctx)

	smpls := make([]float64, numSamples)
	for i := int64(0); i < numSamples; i++ {
		smpls[i] = rand.Float64()
		app.Add(labels.Labels{{Name: "a", Value: "b"}}, i, smpls[i])
	}

	testutil.Ok(t, app.Commit())
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

		// create snapshot
		snap, err := ioutil.TempDir("", "snap")
		testutil.Ok(t, err)

		defer func() {
			testutil.Ok(t, os.RemoveAll(snap))
		}()
		testutil.Ok(t, db.Snapshot(snap, true))
		testutil.Ok(t, db.Close())

		// reopen DB from snapshot
		db, err = Open(snap, nil, nil, nil)
		testutil.Ok(t, err)
		defer db.Close()

		for _, r := range c.intervals {
			testutil.Ok(t, db.Delete(r.Mint, r.Maxt, labels.MustNewMatcher(labels.MatchEqual, "a", "b")))
		}

		// All of the setup for THIS line.
		testutil.Ok(t, db.CleanTombstones())

		// Compare the result.
		q, err := db.Querier(context.TODO(), 0, numSamples)
		testutil.Ok(t, err)
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
			testutil.Assert(t, res.Next() == false, "")
			continue
		}

		for {
			eok, rok := expss.Next(), res.Next()
			testutil.Equals(t, eok, rok)

			if !eok {
				break
			}
			sexp := expss.At()
			sres := res.At()

			testutil.Equals(t, sexp.Labels(), sres.Labels())

			smplExp, errExp := storage.ExpandSamples(sexp.Iterator(), nil)
			smplRes, errRes := storage.ExpandSamples(sres.Iterator(), nil)

			testutil.Equals(t, errExp, errRes)
			testutil.Equals(t, smplExp, smplRes)
		}
		testutil.Equals(t, 0, len(res.Warnings()))

		for _, b := range db.Blocks() {
			testutil.Equals(t, tombstones.NewMemTombstones(), b.tombstones)
		}
	}
}

// TestTombstoneCleanFail tests that a failing TombstoneClean doesn't leave any blocks behind.
// When TombstoneClean errors the original block that should be rebuilt doesn't get deleted so
// if TombstoneClean leaves any blocks behind these will overlap.
func TestTombstoneCleanFail(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
	}()

	var expectedBlockDirs []string

	// Create some empty blocks pending for compaction.
	// totalBlocks should be >=2 so we have enough blocks to trigger compaction failure.
	totalBlocks := 2
	for i := 0; i < totalBlocks; i++ {
		blockDir := createBlock(t, db.Dir(), genSeries(1, 1, 0, 1))
		block, err := OpenBlock(nil, blockDir, nil)
		testutil.Ok(t, err)
		// Add some fake tombstones to trigger the compaction.
		tomb := tombstones.NewMemTombstones()
		tomb.AddInterval(0, tombstones.Interval{Mint: 0, Maxt: 1})
		block.tombstones = tomb

		db.blocks = append(db.blocks, block)
		expectedBlockDirs = append(expectedBlockDirs, blockDir)
	}

	// Initialize the mockCompactorFailing with a room for a single compaction iteration.
	// mockCompactorFailing will fail on the second iteration so we can check if the cleanup works as expected.
	db.compactor = &mockCompactorFailing{
		t:      t,
		blocks: db.blocks,
		max:    totalBlocks + 1,
	}

	// The compactor should trigger a failure here.
	testutil.NotOk(t, db.CleanTombstones())

	// Now check that the CleanTombstones didn't leave any blocks behind after a failure.
	actualBlockDirs, err := blockDirs(db.dir)
	testutil.Ok(t, err)
	testutil.Equals(t, expectedBlockDirs, actualBlockDirs)
}

// mockCompactorFailing creates a new empty block on every write and fails when reached the max allowed total.
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
	testutil.Ok(c.t, err)
	testutil.Ok(c.t, block.Close()) // Close block as we won't be using anywhere.
	c.blocks = append(c.blocks, block)

	// Now check that all expected blocks are actually persisted on disk.
	// This way we make sure that the we have some blocks that are supposed to be removed.
	var expectedBlocks []string
	for _, b := range c.blocks {
		expectedBlocks = append(expectedBlocks, filepath.Join(dest, b.Meta().ULID.String()))
	}
	actualBlockDirs, err := blockDirs(dest)
	testutil.Ok(c.t, err)

	testutil.Equals(c.t, expectedBlocks, actualBlockDirs)

	return block.Meta().ULID, nil
}

func (*mockCompactorFailing) Compact(string, []string, []*Block) (ulid.ULID, error) {
	return ulid.ULID{}, nil
}

func TestTimeRetention(t *testing.T) {
	db := openTestDB(t, nil, []int64{1000})
	defer func() {
		testutil.Ok(t, db.Close())
	}()

	blocks := []*BlockMeta{
		{MinTime: 500, MaxTime: 900}, // Oldest block
		{MinTime: 1000, MaxTime: 1500},
		{MinTime: 1500, MaxTime: 2000}, // Newest Block
	}

	for _, m := range blocks {
		createBlock(t, db.Dir(), genSeries(10, 10, m.MinTime, m.MaxTime))
	}

	testutil.Ok(t, db.reload())                       // Reload the db to register the new blocks.
	testutil.Equals(t, len(blocks), len(db.Blocks())) // Ensure all blocks are registered.

	db.opts.RetentionDuration = blocks[2].MaxTime - blocks[1].MinTime
	testutil.Ok(t, db.reload())

	expBlocks := blocks[1:]
	actBlocks := db.Blocks()

	testutil.Equals(t, 1, int(prom_testutil.ToFloat64(db.metrics.timeRetentionCount)), "metric retention count mismatch")
	testutil.Equals(t, len(expBlocks), len(actBlocks))
	testutil.Equals(t, expBlocks[0].MaxTime, actBlocks[0].meta.MaxTime)
	testutil.Equals(t, expBlocks[len(expBlocks)-1].MaxTime, actBlocks[len(actBlocks)-1].meta.MaxTime)
}

func TestSizeRetention(t *testing.T) {
	db := openTestDB(t, nil, []int64{100})
	defer func() {
		testutil.Ok(t, db.Close())
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
	for _, m := range headBlocks {
		series := genSeries(100, 10, m.MinTime, m.MaxTime)
		for _, s := range series {
			it := s.Iterator()
			for it.Next() {
				tim, v := it.At()
				_, err := headApp.Add(s.Labels(), tim, v)
				testutil.Ok(t, err)
			}
			testutil.Ok(t, it.Err())
		}
	}
	testutil.Ok(t, headApp.Commit())

	// Test that registered size matches the actual disk size.
	testutil.Ok(t, db.reload())                                         // Reload the db to register the new db size.
	testutil.Equals(t, len(blocks), len(db.Blocks()))                   // Ensure all blocks are registered.
	blockSize := int64(prom_testutil.ToFloat64(db.metrics.blocksBytes)) // Use the actual internal metrics.
	walSize, err := db.Head().wal.Size()
	testutil.Ok(t, err)
	// Expected size should take into account block size + WAL size
	expSize := blockSize + walSize
	actSize, err := fileutil.DirSize(db.Dir())
	testutil.Ok(t, err)
	testutil.Equals(t, expSize, actSize, "registered size doesn't match actual disk size")

	// Create a WAL checkpoint, and compare sizes.
	first, last, err := db.Head().wal.Segments()
	testutil.Ok(t, err)
	_, err = wal.Checkpoint(log.NewNopLogger(), db.Head().wal, first, last-1, func(x uint64) bool { return false }, 0)
	testutil.Ok(t, err)
	blockSize = int64(prom_testutil.ToFloat64(db.metrics.blocksBytes)) // Use the actual internal metrics.
	walSize, err = db.Head().wal.Size()
	testutil.Ok(t, err)
	expSize = blockSize + walSize
	actSize, err = fileutil.DirSize(db.Dir())
	testutil.Ok(t, err)
	testutil.Equals(t, expSize, actSize, "registered size doesn't match actual disk size")

	// Decrease the max bytes limit so that a delete is triggered.
	// Check total size, total count and check that the oldest block was deleted.
	firstBlockSize := db.Blocks()[0].Size()
	sizeLimit := actSize - firstBlockSize
	db.opts.MaxBytes = sizeLimit // Set the new db size limit one block smaller that the actual size.
	testutil.Ok(t, db.reload())  // Reload the db to register the new db size.

	expBlocks := blocks[1:]
	actBlocks := db.Blocks()
	blockSize = int64(prom_testutil.ToFloat64(db.metrics.blocksBytes))
	walSize, err = db.Head().wal.Size()
	testutil.Ok(t, err)
	// Expected size should take into account block size + WAL size
	expSize = blockSize + walSize
	actRetentionCount := int(prom_testutil.ToFloat64(db.metrics.sizeRetentionCount))
	actSize, err = fileutil.DirSize(db.Dir())
	testutil.Ok(t, err)

	testutil.Equals(t, 1, actRetentionCount, "metric retention count mismatch")
	testutil.Equals(t, actSize, expSize, "metric db size doesn't match actual disk size")
	testutil.Assert(t, expSize <= sizeLimit, "actual size (%v) is expected to be less than or equal to limit (%v)", expSize, sizeLimit)
	testutil.Equals(t, len(blocks)-1, len(actBlocks), "new block count should be decreased from:%v to:%v", len(blocks), len(blocks)-1)
	testutil.Equals(t, expBlocks[0].MaxTime, actBlocks[0].meta.MaxTime, "maxT mismatch of the first block")
	testutil.Equals(t, expBlocks[len(expBlocks)-1].MaxTime, actBlocks[len(actBlocks)-1].meta.MaxTime, "maxT mismatch of the last block")
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
			testutil.Ok(t, db.Close())
		}()

		actMaxBytes := int64(prom_testutil.ToFloat64(db.metrics.maxBytes))
		testutil.Equals(t, actMaxBytes, c.expMaxBytes, "metric retention limit bytes mismatch")
	}
}

func TestNotMatcherSelectsLabelsUnsetSeries(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
	}()

	labelpairs := []labels.Labels{
		labels.FromStrings("a", "abcd", "b", "abcde"),
		labels.FromStrings("labelname", "labelvalue"),
	}

	ctx := context.Background()
	app := db.Appender(ctx)
	for _, lbls := range labelpairs {
		_, err := app.Add(lbls, 0, 1)
		testutil.Ok(t, err)
	}
	testutil.Ok(t, app.Commit())

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
	testutil.Ok(t, err)
	defer func() { testutil.Ok(t, q.Close()) }()

	for _, c := range cases {
		ss := q.Select(false, nil, c.selector...)
		lres, _, ws, err := expandSeriesSet(ss)
		testutil.Ok(t, err)
		testutil.Equals(t, 0, len(ws))
		testutil.Equals(t, c.series, lres)
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
	var metas = make([]BlockMeta, 11)
	for i := 10; i >= 0; i-- {
		metas[i] = BlockMeta{MinTime: int64(i * 10), MaxTime: int64((i + 1) * 10)}
	}

	testutil.Assert(t, len(OverlappingBlocks(metas)) == 0, "we found unexpected overlaps")

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
	testutil.Equals(t, Overlaps{
		{Min: 15, Max: 17}: {metas[1], o1},
	}, OverlappingBlocks(add(o1)))

	// o2 overlaps with 20-30 and 30-40.
	o2 := BlockMeta{MinTime: 21, MaxTime: 31}
	testutil.Equals(t, Overlaps{
		{Min: 21, Max: 30}: {metas[2], o2},
		{Min: 30, Max: 31}: {o2, metas[3]},
	}, OverlappingBlocks(add(o2)))

	// o3a and o3b overlaps with 30-40 and each other.
	o3a := BlockMeta{MinTime: 33, MaxTime: 39}
	o3b := BlockMeta{MinTime: 34, MaxTime: 36}
	testutil.Equals(t, Overlaps{
		{Min: 34, Max: 36}: {metas[3], o3a, o3b},
	}, OverlappingBlocks(add(o3a, o3b)))

	// o4 is 1:1 overlap with 50-60.
	o4 := BlockMeta{MinTime: 50, MaxTime: 60}
	testutil.Equals(t, Overlaps{
		{Min: 50, Max: 60}: {metas[5], o4},
	}, OverlappingBlocks(add(o4)))

	// o5 overlaps with 60-70, 70-80 and 80-90.
	o5 := BlockMeta{MinTime: 61, MaxTime: 85}
	testutil.Equals(t, Overlaps{
		{Min: 61, Max: 70}: {metas[6], o5},
		{Min: 70, Max: 80}: {o5, metas[7]},
		{Min: 80, Max: 85}: {o5, metas[8]},
	}, OverlappingBlocks(add(o5)))

	// o6a overlaps with 90-100, 100-110 and o6b, o6b overlaps with 90-100 and o6a.
	o6a := BlockMeta{MinTime: 92, MaxTime: 105}
	o6b := BlockMeta{MinTime: 94, MaxTime: 99}
	testutil.Equals(t, Overlaps{
		{Min: 94, Max: 99}:   {metas[9], o6a, o6b},
		{Min: 100, Max: 105}: {o6a, metas[10]},
	}, OverlappingBlocks(add(o6a, o6b)))

	// All together.
	testutil.Equals(t, Overlaps{
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
	testutil.Equals(t, Overlaps{
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
		testutil.Ok(t, db.Close())
	}()

	ctx := context.Background()
	app := db.Appender(ctx)

	blockRange := db.compactor.(*LeveledCompactor).ranges[0]
	label := labels.FromStrings("foo", "bar")

	for i := int64(0); i < 3; i++ {
		_, err := app.Add(label, i*blockRange, 0)
		testutil.Ok(t, err)
		_, err = app.Add(label, i*blockRange+1000, 0)
		testutil.Ok(t, err)
	}

	err := app.Commit()
	testutil.Ok(t, err)

	err = db.Compact()
	testutil.Ok(t, err)

	for _, block := range db.Blocks() {
		r, err := block.Index()
		testutil.Ok(t, err)
		defer r.Close()

		meta := block.Meta()

		k, v := index.AllPostingsKey()
		p, err := r.Postings(k, v)
		testutil.Ok(t, err)

		var (
			lset labels.Labels
			chks []chunks.Meta
		)

		chunkCount := 0

		for p.Next() {
			err = r.Series(p.At(), &lset, &chks)
			testutil.Ok(t, err)
			for _, c := range chks {
				testutil.Assert(t, meta.MinTime <= c.MinTime && c.MaxTime <= meta.MaxTime,
					"chunk spans beyond block boundaries: [block.MinTime=%d, block.MaxTime=%d]; [chunk.MinTime=%d, chunk.MaxTime=%d]",
					meta.MinTime, meta.MaxTime, c.MinTime, c.MaxTime)
				chunkCount++
			}
		}
		testutil.Assert(t, chunkCount == 1, "expected 1 chunk in block %s, got %d", meta.ULID, chunkCount)
	}
}

func TestQuerierWithBoundaryChunks(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
	}()

	ctx := context.Background()
	app := db.Appender(ctx)

	blockRange := db.compactor.(*LeveledCompactor).ranges[0]
	label := labels.FromStrings("foo", "bar")

	for i := int64(0); i < 5; i++ {
		_, err := app.Add(label, i*blockRange, 0)
		testutil.Ok(t, err)
		_, err = app.Add(labels.FromStrings("blockID", strconv.FormatInt(i, 10)), i*blockRange, 0)
		testutil.Ok(t, err)
	}

	err := app.Commit()
	testutil.Ok(t, err)

	err = db.Compact()
	testutil.Ok(t, err)

	testutil.Assert(t, len(db.blocks) >= 3, "invalid test, less than three blocks in DB")

	q, err := db.Querier(context.TODO(), blockRange, 2*blockRange)
	testutil.Ok(t, err)
	defer q.Close()

	// The requested interval covers 2 blocks, so the querier's label values for blockID should give us 2 values, one from each block.
	b, ws, err := q.LabelValues("blockID")
	testutil.Ok(t, err)
	testutil.Equals(t, storage.Warnings(nil), ws)
	testutil.Equals(t, []string{"1", "2"}, b)
}

// TestInitializeHeadTimestamp ensures that the h.minTime is set properly.
// 	- no blocks no WAL: set to the time of the first  appended sample
// 	- no blocks with WAL: set to the smallest sample from the WAL
//	- with blocks no WAL: set to the last block maxT
// 	- with blocks with WAL: same as above
func TestInitializeHeadTimestamp(t *testing.T) {
	t.Run("clean", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "test_head_init")
		testutil.Ok(t, err)
		defer func() {
			testutil.Ok(t, os.RemoveAll(dir))
		}()

		db, err := Open(dir, nil, nil, nil)
		testutil.Ok(t, err)
		defer db.Close()

		// Should be set to init values if no WAL or blocks exist so far.
		testutil.Equals(t, int64(math.MaxInt64), db.head.MinTime())
		testutil.Equals(t, int64(math.MinInt64), db.head.MaxTime())

		// First added sample initializes the writable range.
		ctx := context.Background()
		app := db.Appender(ctx)
		_, err = app.Add(labels.FromStrings("a", "b"), 1000, 1)
		testutil.Ok(t, err)

		testutil.Equals(t, int64(1000), db.head.MinTime())
		testutil.Equals(t, int64(1000), db.head.MaxTime())
	})
	t.Run("wal-only", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "test_head_init")
		testutil.Ok(t, err)
		defer func() {
			testutil.Ok(t, os.RemoveAll(dir))
		}()

		testutil.Ok(t, os.MkdirAll(path.Join(dir, "wal"), 0777))
		w, err := wal.New(nil, nil, path.Join(dir, "wal"), false)
		testutil.Ok(t, err)

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
		testutil.Ok(t, err)
		testutil.Ok(t, w.Close())

		db, err := Open(dir, nil, nil, nil)
		testutil.Ok(t, err)
		defer db.Close()

		testutil.Equals(t, int64(5000), db.head.MinTime())
		testutil.Equals(t, int64(15000), db.head.MaxTime())
	})
	t.Run("existing-block", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "test_head_init")
		testutil.Ok(t, err)
		defer func() {
			testutil.Ok(t, os.RemoveAll(dir))
		}()

		createBlock(t, dir, genSeries(1, 1, 1000, 2000))

		db, err := Open(dir, nil, nil, nil)
		testutil.Ok(t, err)
		defer db.Close()

		testutil.Equals(t, int64(2000), db.head.MinTime())
		testutil.Equals(t, int64(2000), db.head.MaxTime())
	})
	t.Run("existing-block-and-wal", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "test_head_init")
		testutil.Ok(t, err)
		defer func() {
			testutil.Ok(t, os.RemoveAll(dir))
		}()

		createBlock(t, dir, genSeries(1, 1, 1000, 6000))

		testutil.Ok(t, os.MkdirAll(path.Join(dir, "wal"), 0777))
		w, err := wal.New(nil, nil, path.Join(dir, "wal"), false)
		testutil.Ok(t, err)

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
		testutil.Ok(t, err)
		testutil.Ok(t, w.Close())

		r := prometheus.NewRegistry()

		db, err := Open(dir, nil, r, nil)
		testutil.Ok(t, err)
		defer db.Close()

		testutil.Equals(t, int64(6000), db.head.MinTime())
		testutil.Equals(t, int64(15000), db.head.MaxTime())
		// Check that old series has been GCed.
		testutil.Equals(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.series))
	})
}

func TestNoEmptyBlocks(t *testing.T) {
	db := openTestDB(t, nil, []int64{100})
	ctx := context.Background()
	defer func() {
		testutil.Ok(t, db.Close())
	}()
	db.DisableCompactions()

	rangeToTriggerCompaction := db.compactor.(*LeveledCompactor).ranges[0]/2*3 - 1
	defaultLabel := labels.FromStrings("foo", "bar")
	defaultMatcher := labels.MustNewMatcher(labels.MatchRegexp, "", ".*")

	t.Run("Test no blocks after compact with empty head.", func(t *testing.T) {
		testutil.Ok(t, db.Compact())
		actBlocks, err := blockDirs(db.Dir())
		testutil.Ok(t, err)
		testutil.Equals(t, len(db.Blocks()), len(actBlocks))
		testutil.Equals(t, 0, len(actBlocks))
		testutil.Equals(t, 0, int(prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.ran)), "no compaction should be triggered here")
	})

	t.Run("Test no blocks after deleting all samples from head.", func(t *testing.T) {
		app := db.Appender(ctx)
		_, err := app.Add(defaultLabel, 1, 0)
		testutil.Ok(t, err)
		_, err = app.Add(defaultLabel, 2, 0)
		testutil.Ok(t, err)
		_, err = app.Add(defaultLabel, 3+rangeToTriggerCompaction, 0)
		testutil.Ok(t, err)
		testutil.Ok(t, app.Commit())
		testutil.Ok(t, db.Delete(math.MinInt64, math.MaxInt64, defaultMatcher))
		testutil.Ok(t, db.Compact())
		testutil.Equals(t, 1, int(prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.ran)), "compaction should have been triggered here")

		actBlocks, err := blockDirs(db.Dir())
		testutil.Ok(t, err)
		testutil.Equals(t, len(db.Blocks()), len(actBlocks))
		testutil.Equals(t, 0, len(actBlocks))

		app = db.Appender(ctx)
		_, err = app.Add(defaultLabel, 1, 0)
		testutil.Assert(t, err == storage.ErrOutOfBounds, "the head should be truncated so no samples in the past should be allowed")

		// Adding new blocks.
		currentTime := db.Head().MaxTime()
		_, err = app.Add(defaultLabel, currentTime, 0)
		testutil.Ok(t, err)
		_, err = app.Add(defaultLabel, currentTime+1, 0)
		testutil.Ok(t, err)
		_, err = app.Add(defaultLabel, currentTime+rangeToTriggerCompaction, 0)
		testutil.Ok(t, err)
		testutil.Ok(t, app.Commit())

		testutil.Ok(t, db.Compact())
		testutil.Equals(t, 2, int(prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.ran)), "compaction should have been triggered here")
		actBlocks, err = blockDirs(db.Dir())
		testutil.Ok(t, err)
		testutil.Equals(t, len(db.Blocks()), len(actBlocks))
		testutil.Assert(t, len(actBlocks) == 1, "No blocks created when compacting with >0 samples")
	})

	t.Run(`When no new block is created from head, and there are some blocks on disk
	compaction should not run into infinite loop (was seen during development).`, func(t *testing.T) {
		oldBlocks := db.Blocks()
		app := db.Appender(ctx)
		currentTime := db.Head().MaxTime()
		_, err := app.Add(defaultLabel, currentTime, 0)
		testutil.Ok(t, err)
		_, err = app.Add(defaultLabel, currentTime+1, 0)
		testutil.Ok(t, err)
		_, err = app.Add(defaultLabel, currentTime+rangeToTriggerCompaction, 0)
		testutil.Ok(t, err)
		testutil.Ok(t, app.Commit())
		testutil.Ok(t, db.head.Delete(math.MinInt64, math.MaxInt64, defaultMatcher))
		testutil.Ok(t, db.Compact())
		testutil.Equals(t, 3, int(prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.ran)), "compaction should have been triggered here")
		testutil.Equals(t, oldBlocks, db.Blocks())
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
		testutil.Ok(t, db.reload())                                      // Reload the db to register the new blocks.
		testutil.Equals(t, len(blocks)+len(oldBlocks), len(db.Blocks())) // Ensure all blocks are registered.
		testutil.Ok(t, db.Delete(math.MinInt64, math.MaxInt64, defaultMatcher))
		testutil.Ok(t, db.Compact())
		testutil.Equals(t, 5, int(prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.ran)), "compaction should have been triggered here once for each block that have tombstones")

		actBlocks, err := blockDirs(db.Dir())
		testutil.Ok(t, err)
		testutil.Equals(t, len(db.Blocks()), len(actBlocks))
		testutil.Equals(t, 1, len(actBlocks), "All samples are deleted. Only the most recent block should remain after compaction.")
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
				_, err := app.Add(label, i*blockRange, 0)
				testutil.Ok(t, err)
			}
		}
		err := app.Commit()
		testutil.Ok(t, err)
	}
	for _, tst := range tests {
		db := openTestDB(t, nil, nil)
		defer func() {
			testutil.Ok(t, db.Close())
		}()

		appendSamples(db, 0, 4, tst.sampleLabels1)

		// Testing head.
		headIndexr, err := db.head.Index()
		testutil.Ok(t, err)
		labelNames, err := headIndexr.LabelNames()
		testutil.Ok(t, err)
		testutil.Equals(t, tst.exp1, labelNames)
		testutil.Ok(t, headIndexr.Close())

		// Testing disk.
		err = db.Compact()
		testutil.Ok(t, err)
		// All blocks have same label names, hence check them individually.
		// No need to aggregate and check.
		for _, b := range db.Blocks() {
			blockIndexr, err := b.Index()
			testutil.Ok(t, err)
			labelNames, err = blockIndexr.LabelNames()
			testutil.Ok(t, err)
			testutil.Equals(t, tst.exp1, labelNames)
			testutil.Ok(t, blockIndexr.Close())
		}

		// Adding more samples to head with new label names
		// so that we can test (head+disk).LabelNames() (the union).
		appendSamples(db, 5, 9, tst.sampleLabels2)

		// Testing DB (union).
		q, err := db.Querier(context.TODO(), math.MinInt64, math.MaxInt64)
		testutil.Ok(t, err)
		var ws storage.Warnings
		labelNames, ws, err = q.LabelNames()
		testutil.Ok(t, err)
		testutil.Equals(t, 0, len(ws))
		testutil.Ok(t, q.Close())
		testutil.Equals(t, tst.exp2, labelNames)
	}
}

func TestCorrectNumTombstones(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
	}()

	blockRange := db.compactor.(*LeveledCompactor).ranges[0]
	defaultLabel := labels.FromStrings("foo", "bar")
	defaultMatcher := labels.MustNewMatcher(labels.MatchEqual, defaultLabel[0].Name, defaultLabel[0].Value)

	ctx := context.Background()
	app := db.Appender(ctx)
	for i := int64(0); i < 3; i++ {
		for j := int64(0); j < 15; j++ {
			_, err := app.Add(defaultLabel, i*blockRange+j, 0)
			testutil.Ok(t, err)
		}
	}
	testutil.Ok(t, app.Commit())

	err := db.Compact()
	testutil.Ok(t, err)
	testutil.Equals(t, 1, len(db.blocks))

	testutil.Ok(t, db.Delete(0, 1, defaultMatcher))
	testutil.Equals(t, uint64(1), db.blocks[0].meta.Stats.NumTombstones)

	// {0, 1} and {2, 3} are merged to form 1 tombstone.
	testutil.Ok(t, db.Delete(2, 3, defaultMatcher))
	testutil.Equals(t, uint64(1), db.blocks[0].meta.Stats.NumTombstones)

	testutil.Ok(t, db.Delete(5, 6, defaultMatcher))
	testutil.Equals(t, uint64(2), db.blocks[0].meta.Stats.NumTombstones)

	testutil.Ok(t, db.Delete(9, 11, defaultMatcher))
	testutil.Equals(t, uint64(3), db.blocks[0].meta.Stats.NumTombstones)
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

	dir, err := ioutil.TempDir("", "test_storage")
	testutil.Ok(t, err)

	// Test that the compactor doesn't create overlapping blocks
	// when a non standard block already exists.
	firstBlockMaxT := int64(3)
	createBlock(t, dir, genSeries(1, 1, 0, firstBlockMaxT))
	db, err := open(dir, logger, nil, DefaultOptions(), []int64{10000})
	testutil.Ok(t, err)

	rangeToTriggerCompaction := db.compactor.(*LeveledCompactor).ranges[0]/2*3 + 1
	defer func() {
		os.RemoveAll(dir)
	}()
	app := db.Appender(ctx)
	lbl := labels.Labels{{Name: "a", Value: "b"}}
	_, err = app.Add(lbl, firstBlockMaxT-1, rand.Float64())
	if err == nil {
		t.Fatalf("appending a sample with a timestamp covered by a previous block shouldn't be possible")
	}
	_, err = app.Add(lbl, firstBlockMaxT+1, rand.Float64())
	testutil.Ok(t, err)
	_, err = app.Add(lbl, firstBlockMaxT+2, rand.Float64())
	testutil.Ok(t, err)
	secondBlockMaxt := firstBlockMaxT + rangeToTriggerCompaction
	_, err = app.Add(lbl, secondBlockMaxt, rand.Float64()) // Add samples to trigger a new compaction

	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())
	for x := 0; x < 100; x++ {
		if len(db.Blocks()) == 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	testutil.Equals(t, 2, len(db.Blocks()), "no new block created after the set timeout")

	if db.Blocks()[0].Meta().MaxTime > db.Blocks()[1].Meta().MinTime {
		t.Fatalf("new block overlaps  old:%v,new:%v", db.Blocks()[0].Meta(), db.Blocks()[1].Meta())
	}

	// Test that wal records are skipped when an existing block covers the same time ranges
	// and compaction doesn't create an overlapping block.
	app = db.Appender(ctx)
	db.DisableCompactions()
	_, err = app.Add(lbl, secondBlockMaxt+1, rand.Float64())
	testutil.Ok(t, err)
	_, err = app.Add(lbl, secondBlockMaxt+2, rand.Float64())
	testutil.Ok(t, err)
	_, err = app.Add(lbl, secondBlockMaxt+3, rand.Float64())
	testutil.Ok(t, err)
	_, err = app.Add(lbl, secondBlockMaxt+4, rand.Float64())
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())
	testutil.Ok(t, db.Close())

	thirdBlockMaxt := secondBlockMaxt + 2
	createBlock(t, dir, genSeries(1, 1, secondBlockMaxt+1, thirdBlockMaxt))

	db, err = open(dir, logger, nil, DefaultOptions(), []int64{10000})
	testutil.Ok(t, err)

	defer db.Close()
	testutil.Equals(t, 3, len(db.Blocks()), "db doesn't include expected number of blocks")
	testutil.Equals(t, db.Blocks()[2].Meta().MaxTime, thirdBlockMaxt, "unexpected maxt of the last block")

	app = db.Appender(ctx)
	_, err = app.Add(lbl, thirdBlockMaxt+rangeToTriggerCompaction, rand.Float64()) // Trigger a compaction
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())
	for x := 0; x < 100; x++ {
		if len(db.Blocks()) == 4 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	testutil.Equals(t, 4, len(db.Blocks()), "no new block created after the set timeout")

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
		dbDir, err = ioutil.TempDir("", "test")
		testutil.Ok(t, err)

		defer func() {
			testutil.Ok(t, os.RemoveAll(dbDir))
		}()

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
		testutil.Ok(t, err)
		h := createHead(t, w, genSeries(1, 1, 16, 18), dbDir)
		testutil.Ok(t, h.Close())
	}

	// Open a normal db to use for a comparison.
	{
		dbWritable, err := Open(dbDir, logger, nil, nil)
		testutil.Ok(t, err)
		dbWritable.DisableCompactions()

		dbSizeBeforeAppend, err := fileutil.DirSize(dbWritable.Dir())
		testutil.Ok(t, err)
		app := dbWritable.Appender(context.Background())
		_, err = app.Add(labels.FromStrings("foo", "bar"), dbWritable.Head().MaxTime()+1, 0)
		testutil.Ok(t, err)
		testutil.Ok(t, app.Commit())

		expBlocks = dbWritable.Blocks()
		expDbSize, err := fileutil.DirSize(dbWritable.Dir())
		testutil.Ok(t, err)
		testutil.Assert(t, expDbSize > dbSizeBeforeAppend, "db size didn't increase after an append")

		q, err := dbWritable.Querier(context.TODO(), math.MinInt64, math.MaxInt64)
		testutil.Ok(t, err)
		expSeries = query(t, q, matchAll)
		cq, err := dbWritable.ChunkQuerier(context.TODO(), math.MinInt64, math.MaxInt64)
		testutil.Ok(t, err)
		expChunks = queryChunks(t, cq, matchAll)

		testutil.Ok(t, dbWritable.Close()) // Close here to allow getting the dir hash for windows.
		expDBHash = testutil.DirHash(t, dbWritable.Dir())
	}

	// Open a read only db and ensure that the API returns the same result as the normal DB.
	dbReadOnly, err := OpenDBReadOnly(dbDir, logger)
	testutil.Ok(t, err)
	defer func() { testutil.Ok(t, dbReadOnly.Close()) }()

	t.Run("blocks", func(t *testing.T) {
		blocks, err := dbReadOnly.Blocks()
		testutil.Ok(t, err)
		testutil.Equals(t, len(expBlocks), len(blocks))
		for i, expBlock := range expBlocks {
			testutil.Equals(t, expBlock.Meta(), blocks[i].Meta(), "block meta mismatch")
		}
	})

	t.Run("querier", func(t *testing.T) {
		// Open a read only db and ensure that the API returns the same result as the normal DB.
		q, err := dbReadOnly.Querier(context.TODO(), math.MinInt64, math.MaxInt64)
		testutil.Ok(t, err)
		readOnlySeries := query(t, q, matchAll)
		readOnlyDBHash := testutil.DirHash(t, dbDir)

		testutil.Equals(t, len(expSeries), len(readOnlySeries), "total series mismatch")
		testutil.Equals(t, expSeries, readOnlySeries, "series mismatch")
		testutil.Equals(t, expDBHash, readOnlyDBHash, "after all read operations the db hash should remain the same")
	})
	t.Run("chunk querier", func(t *testing.T) {
		cq, err := dbReadOnly.ChunkQuerier(context.TODO(), math.MinInt64, math.MaxInt64)
		testutil.Ok(t, err)
		readOnlySeries := queryChunks(t, cq, matchAll)
		readOnlyDBHash := testutil.DirHash(t, dbDir)

		testutil.Equals(t, len(expChunks), len(readOnlySeries), "total series mismatch")
		testutil.Equals(t, expChunks, readOnlySeries, "series chunks mismatch")
		testutil.Equals(t, expDBHash, readOnlyDBHash, "after all read operations the db hash should remain the same")
	})
}

// TestDBReadOnlyClosing ensures that after closing the db
// all api methods return an ErrClosed.
func TestDBReadOnlyClosing(t *testing.T) {
	dbDir, err := ioutil.TempDir("", "test")
	testutil.Ok(t, err)

	defer func() {
		testutil.Ok(t, os.RemoveAll(dbDir))
	}()
	db, err := OpenDBReadOnly(dbDir, log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr)))
	testutil.Ok(t, err)
	testutil.Ok(t, db.Close())
	testutil.Equals(t, db.Close(), ErrClosed)
	_, err = db.Blocks()
	testutil.Equals(t, err, ErrClosed)
	_, err = db.Querier(context.TODO(), 0, 1)
	testutil.Equals(t, err, ErrClosed)
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
		dbDir, err = ioutil.TempDir("", "test")
		testutil.Ok(t, err)

		defer func() {
			testutil.Ok(t, os.RemoveAll(dbDir))
		}()

		// Append data to the WAL.
		db, err := Open(dbDir, logger, nil, nil)
		testutil.Ok(t, err)
		db.DisableCompactions()
		app := db.Appender(ctx)
		maxt = 1000
		for i := 0; i < maxt; i++ {
			_, err := app.Add(labels.FromStrings(defaultLabelName, "flush"), int64(i), 1.0)
			testutil.Ok(t, err)
		}
		testutil.Ok(t, app.Commit())
		defer func() { testutil.Ok(t, db.Close()) }()
	}

	// Flush WAL.
	db, err := OpenDBReadOnly(dbDir, logger)
	testutil.Ok(t, err)

	flush, err := ioutil.TempDir("", "flush")
	testutil.Ok(t, err)

	defer func() {
		testutil.Ok(t, os.RemoveAll(flush))
	}()
	testutil.Ok(t, db.FlushWAL(flush))
	testutil.Ok(t, db.Close())

	// Reopen the DB from the flushed WAL block.
	db, err = OpenDBReadOnly(flush, logger)
	testutil.Ok(t, err)
	defer func() { testutil.Ok(t, db.Close()) }()
	blocks, err := db.Blocks()
	testutil.Ok(t, err)
	testutil.Equals(t, len(blocks), 1)

	querier, err := db.Querier(context.TODO(), 0, int64(maxt)-1)
	testutil.Ok(t, err)
	defer func() { testutil.Ok(t, querier.Close()) }()

	// Sum the values.
	seriesSet := querier.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, defaultLabelName, "flush"))

	sum := 0.0
	for seriesSet.Next() {
		series := seriesSet.At().Iterator()
		for series.Next() {
			_, v := series.At()
			sum += v
		}
		testutil.Ok(t, series.Err())
	}
	testutil.Ok(t, seriesSet.Err())
	testutil.Equals(t, 0, len(seriesSet.Warnings()))
	testutil.Equals(t, 1000.0, sum)
}

func TestDBCannotSeePartialCommits(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer func() {
		testutil.Ok(t, os.RemoveAll(tmpdir))
	}()

	db, err := Open(tmpdir, nil, nil, nil)
	testutil.Ok(t, err)
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
				_, err := app.Add(labels.FromStrings("foo", "bar", "a", strconv.Itoa(j)), int64(iter), float64(iter))
				testutil.Ok(t, err)
			}
			err = app.Commit()
			testutil.Ok(t, err)

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
			testutil.Ok(t, err)
			defer querier.Close()

			ss := querier.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
			_, seriesSet, ws, err := expandSeriesSet(ss)
			testutil.Ok(t, err)
			testutil.Equals(t, 0, len(ws))

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

	testutil.Equals(t, 0, inconsistencies, "Some queries saw inconsistent results.")
}

func TestDBQueryDoesntSeeAppendsAfterCreation(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer func() {
		testutil.Ok(t, os.RemoveAll(tmpdir))
	}()

	db, err := Open(tmpdir, nil, nil, nil)
	testutil.Ok(t, err)
	defer db.Close()

	querierBeforeAdd, err := db.Querier(context.Background(), 0, 1000000)
	testutil.Ok(t, err)
	defer querierBeforeAdd.Close()

	ctx := context.Background()
	app := db.Appender(ctx)
	_, err = app.Add(labels.FromStrings("foo", "bar"), 0, 0)
	testutil.Ok(t, err)

	querierAfterAddButBeforeCommit, err := db.Querier(context.Background(), 0, 1000000)
	testutil.Ok(t, err)
	defer querierAfterAddButBeforeCommit.Close()

	// None of the queriers should return anything after the Add but before the commit.
	ss := querierBeforeAdd.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	_, seriesSet, ws, err := expandSeriesSet(ss)
	testutil.Ok(t, err)
	testutil.Equals(t, 0, len(ws))
	testutil.Equals(t, map[string][]sample{}, seriesSet)

	ss = querierAfterAddButBeforeCommit.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	_, seriesSet, ws, err = expandSeriesSet(ss)
	testutil.Ok(t, err)
	testutil.Equals(t, 0, len(ws))
	testutil.Equals(t, map[string][]sample{}, seriesSet)

	// This commit is after the queriers are created, so should not be returned.
	err = app.Commit()
	testutil.Ok(t, err)

	// Nothing returned for querier created before the Add.
	ss = querierBeforeAdd.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	_, seriesSet, ws, err = expandSeriesSet(ss)
	testutil.Ok(t, err)
	testutil.Equals(t, 0, len(ws))
	testutil.Equals(t, map[string][]sample{}, seriesSet)

	// Series exists but has no samples for querier created after Add.
	ss = querierAfterAddButBeforeCommit.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	_, seriesSet, ws, err = expandSeriesSet(ss)
	testutil.Ok(t, err)
	testutil.Equals(t, 0, len(ws))
	testutil.Equals(t, map[string][]sample{`{foo="bar"}`: {}}, seriesSet)

	querierAfterCommit, err := db.Querier(context.Background(), 0, 1000000)
	testutil.Ok(t, err)
	defer querierAfterCommit.Close()

	// Samples are returned for querier created after Commit.
	ss = querierAfterCommit.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	_, seriesSet, ws, err = expandSeriesSet(ss)
	testutil.Ok(t, err)
	testutil.Equals(t, 0, len(ws))
	testutil.Equals(t, map[string][]sample{`{foo="bar"}`: {{t: 0, v: 0}}}, seriesSet)
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

			tempDir, err := ioutil.TempDir("", "test_chunk_writer")
			testutil.Ok(t, err)
			defer func() { testutil.Ok(t, os.RemoveAll(tempDir)) }()

			chunkw, err := chunks.NewWriterWithSegSize(tempDir, chunks.SegmentHeaderSize+int64(test.segmentSize))
			testutil.Ok(t, err)

			for _, chks := range test.chks {
				testutil.Ok(t, chunkw.WriteChunks(chks...))
			}
			testutil.Ok(t, chunkw.Close())

			files, err := ioutil.ReadDir(tempDir)
			testutil.Ok(t, err)
			testutil.Equals(t, test.expSegmentsCount, len(files), "expected segments count mismatch")

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
				size := int(f.Size())
				// Verify that the segment is the same or smaller than the expected size.
				testutil.Assert(t, chunks.SegmentHeaderSize+test.expSegmentSizes[i] >= size, "Segment:%v should NOT be bigger than:%v actual:%v", i, chunks.SegmentHeaderSize+test.expSegmentSizes[i], size)

				sizeAct += size
			}
			testutil.Equals(t, sizeExp, sizeAct)

			// Check the content of the chunks.
			r, err := chunks.NewDirReader(tempDir, nil)
			testutil.Ok(t, err)
			defer func() { testutil.Ok(t, r.Close()) }()

			for _, chks := range test.chks {
				for _, chkExp := range chks {
					chkAct, err := r.Chunk(chkExp.Ref)
					testutil.Ok(t, err)
					testutil.Equals(t, chkExp.Chunk.Bytes(), chkAct.Bytes())
				}
			}
		})
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

	tempDir, err := ioutil.TempDir("", "test_chunk_writer")
	testutil.Ok(t, err)
	defer func() { testutil.Ok(t, os.RemoveAll(tempDir)) }()

	chunkw, err := chunks.NewWriter(tempDir)
	testutil.Ok(t, err)

	testutil.Ok(t, chunkw.WriteChunks(chks...))
	testutil.Ok(t, chunkw.Close())

	r, err := chunks.NewDirReader(tempDir, nil)
	testutil.Ok(t, err)

	var wg sync.WaitGroup
	for _, chk := range chks {
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(chunk chunks.Meta) {
				defer wg.Done()

				chkAct, err := r.Chunk(chunk.Ref)
				testutil.Ok(t, err)
				testutil.Equals(t, chunk.Chunk.Bytes(), chkAct.Bytes())
			}(chk)
		}
		wg.Wait()
	}
	testutil.Ok(t, r.Close())
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
	dbDir, err := ioutil.TempDir("", "testFlush")
	testutil.Ok(t, err)
	defer func() { testutil.Ok(t, os.RemoveAll(dbDir)) }()

	// Open a DB and append data to the WAL.
	tsdbCfg := &Options{
		RetentionDuration: int64(time.Hour * 24 * 15 / time.Millisecond),
		NoLockfile:        true,
		MinBlockDuration:  int64(time.Hour * 2 / time.Millisecond),
		MaxBlockDuration:  int64(time.Hour * 2 / time.Millisecond),
		WALCompression:    true,
	}

	db, err := Open(dbDir, log.NewNopLogger(), prometheus.NewRegistry(), tsdbCfg)
	testutil.Ok(t, err)
	ctx := context.Background()
	app := db.Appender(ctx)
	var expSamples []sample
	maxt := 100
	for i := 0; i < maxt; i++ {
		val := rand.Float64()
		_, err := app.Add(labels.FromStrings("a", "b"), int64(i), val)
		testutil.Ok(t, err)
		expSamples = append(expSamples, sample{int64(i), val})
	}
	testutil.Ok(t, app.Commit())

	// Compact the Head to create a new block.
	testutil.Ok(t, db.CompactHead(NewRangeHead(db.Head(), 0, int64(maxt)-1)))
	testutil.Ok(t, db.Close())

	// Delete everything but the new block and
	// reopen the db to query it to ensure it includes the head data.
	testutil.Ok(t, deleteNonBlocks(db.Dir()))
	db, err = Open(dbDir, log.NewNopLogger(), prometheus.NewRegistry(), tsdbCfg)
	testutil.Ok(t, err)
	testutil.Equals(t, 1, len(db.Blocks()))
	testutil.Equals(t, int64(maxt), db.Head().MinTime())
	defer func() { testutil.Ok(t, db.Close()) }()
	querier, err := db.Querier(context.Background(), 0, int64(maxt)-1)
	testutil.Ok(t, err)
	defer func() { testutil.Ok(t, querier.Close()) }()

	seriesSet := querier.Select(false, nil, &labels.Matcher{Type: labels.MatchEqual, Name: "a", Value: "b"})
	var actSamples []sample

	for seriesSet.Next() {
		series := seriesSet.At().Iterator()
		for series.Next() {
			time, val := series.At()
			actSamples = append(actSamples, sample{int64(time), val})
		}
		testutil.Ok(t, series.Err())
	}
	testutil.Equals(t, expSamples, actSamples)
	testutil.Ok(t, seriesSet.Err())
}

func deleteNonBlocks(dbDir string) error {
	dirs, err := ioutil.ReadDir(dbDir)
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
	dirs, err = ioutil.ReadDir(dbDir)
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
	tmpDir, err := ioutil.TempDir("", "test")
	testutil.Ok(t, err)
	t.Cleanup(func() {
		testutil.Ok(t, os.RemoveAll(tmpDir))
	})

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
		testutil.Ok(t, os.MkdirAll(filepath.Join(dbDir, "chunks"), 0777))
		defer func() {
			testutil.Ok(t, os.RemoveAll(filepath.Join(dbDir, "chunks")))
		}()
		testutil.Ok(t, os.Mkdir(outDir, os.ModePerm))
		testutil.Ok(t, fileutil.CopyDirs(dbDir, outDir))
	}
	{
		// Missing meta.json; should be ignored and only logged.
		// TODO(bwplotka): Probably add metric.
		dir := createBlock(t, tmpDir, genSeries(10, 2, 20, 30))
		expectedIgnoredDirs[dir] = struct{}{}
		testutil.Ok(t, os.Remove(filepath.Join(dir, metaFilename)))
	}
	{
		// Tmp blocks during creation & deletion; those should be removed on start.
		dir := createBlock(t, tmpDir, genSeries(10, 2, 30, 40))
		testutil.Ok(t, fileutil.Replace(dir, dir+tmpForCreationBlockDirSuffix))
		expectedRemovedDirs[dir+tmpForCreationBlockDirSuffix] = struct{}{}

		// Tmp blocks during creation & deletion; those should be removed on start.
		dir = createBlock(t, tmpDir, genSeries(10, 2, 40, 50))
		testutil.Ok(t, fileutil.Replace(dir, dir+tmpForDeletionBlockDirSuffix))
		expectedRemovedDirs[dir+tmpForDeletionBlockDirSuffix] = struct{}{}
	}
	{
		// One ok block; but two should be replaced.
		dir := createBlock(t, tmpDir, genSeries(10, 2, 50, 60))
		expectedLoadedDirs[dir] = struct{}{}

		m, _, err := readMetaFile(dir)
		testutil.Ok(t, err)

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
		testutil.Ok(t, err)
	}

	opts := DefaultOptions()
	opts.RetentionDuration = 0
	db, err := Open(tmpDir, log.NewLogfmtLogger(os.Stderr), nil, opts)
	testutil.Ok(t, err)

	loadedBlocks := db.Blocks()

	var loaded int
	for _, l := range loadedBlocks {
		if _, ok := expectedLoadedDirs[filepath.Join(tmpDir, l.meta.ULID.String())]; !ok {
			t.Fatal("unexpected block", l.meta.ULID, "was loaded")
		}
		loaded++
	}
	testutil.Equals(t, len(expectedLoadedDirs), loaded)
	testutil.Ok(t, db.Close())

	files, err := ioutil.ReadDir(tmpDir)
	testutil.Ok(t, err)

	var ignored int
	for _, f := range files {
		if _, ok := expectedRemovedDirs[filepath.Join(tmpDir, f.Name())]; ok {
			t.Fatal("expected", filepath.Join(tmpDir, f.Name()), "to be removed, but still exists")
		}
		if _, ok := expectedIgnoredDirs[filepath.Join(tmpDir, f.Name())]; ok {
			ignored++
		}
	}
	testutil.Equals(t, len(expectedIgnoredDirs), ignored)
}
