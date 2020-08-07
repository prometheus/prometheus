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
)

func openTestDB(t testing.TB, opts *Options, rngs []int64) (db *DB, close func()) {
	tmpdir, err := ioutil.TempDir("", "test")
	testutil.Ok(t, err)

	if len(rngs) == 0 {
		db, err = Open(tmpdir, nil, nil, opts)
	} else {
		opts, rngs = validateOpts(opts, rngs)
		db, err = open(tmpdir, nil, nil, opts, rngs)
	}
	testutil.Ok(t, err)

	// Do not close the test database by default as it will deadlock on test failures.
	return db, func() {
		testutil.Ok(t, os.RemoveAll(tmpdir))
	}
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

// Ensure that blocks are held in memory in their time order
// and not in ULID order as they are read from the directory.
func TestDB_reloadOrder(t *testing.T) {
	db, closeFn := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		closeFn()
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
	db, closeFn := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		closeFn()
	}()

	app := db.Appender()

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

func TestDataNotAvailableAfterRollback(t *testing.T) {
	db, closeFn := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		closeFn()
	}()

	app := db.Appender()
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
	db, closeFn := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		closeFn()
	}()

	app1 := db.Appender()

	ref1, err := app1.Add(labels.FromStrings("a", "b"), 123, 0)
	testutil.Ok(t, err)

	// Reference should already work before commit.
	err = app1.AddFast(ref1, 124, 1)
	testutil.Ok(t, err)

	err = app1.Commit()
	testutil.Ok(t, err)

	app2 := db.Appender()

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
	db, closeFn := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		closeFn()
	}()

	app1 := db.Appender()

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
		db, closeFn := openTestDB(t, nil, nil)
		defer func() {
			testutil.Ok(t, db.Close())
			closeFn()
		}()

		app := db.Appender()

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
			newSeries(map[string]string{"a": "b"}, expSamples),
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

			smplExp, errExp := expandSeriesIterator(sexp.Iterator())
			smplRes, errRes := expandSeriesIterator(sres.Iterator())

			testutil.Equals(t, errExp, errRes)
			testutil.Equals(t, smplExp, smplRes)
		}
	}
}

func TestAmendDatapointCausesError(t *testing.T) {
	db, closeFn := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		closeFn()
	}()

	app := db.Appender()
	_, err := app.Add(labels.Labels{{Name: "a", Value: "b"}}, 0, 0)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	app = db.Appender()
	_, err = app.Add(labels.Labels{{Name: "a", Value: "b"}}, 0, 1)
	testutil.Equals(t, storage.ErrDuplicateSampleForTimestamp, err)
	testutil.Ok(t, app.Rollback())
}

func TestDuplicateNaNDatapointNoAmendError(t *testing.T) {
	db, closeFn := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		closeFn()
	}()

	app := db.Appender()
	_, err := app.Add(labels.Labels{{Name: "a", Value: "b"}}, 0, math.NaN())
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	app = db.Appender()
	_, err = app.Add(labels.Labels{{Name: "a", Value: "b"}}, 0, math.NaN())
	testutil.Ok(t, err)
}

func TestNonDuplicateNaNDatapointsCausesAmendError(t *testing.T) {
	db, closeFn := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		closeFn()
	}()

	app := db.Appender()
	_, err := app.Add(labels.Labels{{Name: "a", Value: "b"}}, 0, math.Float64frombits(0x7ff0000000000001))
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	app = db.Appender()
	_, err = app.Add(labels.Labels{{Name: "a", Value: "b"}}, 0, math.Float64frombits(0x7ff0000000000002))
	testutil.Equals(t, storage.ErrDuplicateSampleForTimestamp, err)
}

func TestEmptyLabelsetCausesError(t *testing.T) {
	db, closeFn := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		closeFn()
	}()

	app := db.Appender()
	_, err := app.Add(labels.Labels{}, 0, 0)
	testutil.NotOk(t, err)
	testutil.Equals(t, "empty labelset: invalid sample", err.Error())
}

func TestSkippingInvalidValuesInSameTxn(t *testing.T) {
	db, closeFn := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		closeFn()
	}()

	// Append AmendedValue.
	app := db.Appender()
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
	app = db.Appender()
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
	db, closeFn := openTestDB(t, nil, nil)
	defer closeFn()

	// append data
	app := db.Appender()
	mint := int64(1414141414000)
	for i := 0; i < 1000; i++ {
		_, err := app.Add(labels.FromStrings("foo", "bar"), mint+int64(i), 1.0)
		testutil.Ok(t, err)
	}
	testutil.Ok(t, app.Commit())
	testutil.Ok(t, app.Rollback())

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
	db, closeFn := openTestDB(t, nil, nil)
	defer closeFn()

	app := db.Appender()
	mint := int64(1414141414000)
	for i := 0; i < 1000; i++ {
		_, err := app.Add(labels.FromStrings("foo", "bar"), mint+int64(i), 1.0)
		testutil.Ok(t, err)
	}
	testutil.Ok(t, app.Commit())
	testutil.Ok(t, app.Rollback())

	snap, err := ioutil.TempDir("", "snap")
	testutil.Ok(t, err)

	// Hackingly introduce "race", by having lower max time then maxTime in last chunk.
	db.head.maxTime = db.head.maxTime - 10

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

	db, closeFn := openTestDB(t, nil, nil)
	defer closeFn()

	app := db.Appender()

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
			newSeries(map[string]string{"a": "b"}, expSamples),
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

			smplExp, errExp := expandSeriesIterator(sexp.Iterator())
			smplRes, errRes := expandSeriesIterator(sres.Iterator())

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

	db, closeFn := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		closeFn()
	}()

	app := db.Appender()

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

				smpls, err := expandSeriesIterator(x.Iterator())
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
	db, closeFn := openTestDB(t, nil, nil)
	defer closeFn()

	dirDb := db.Dir()

	lbls := labels.Labels{labels.Label{Name: "labelname", Value: "labelvalue"}}

	app := db.Appender()
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
			db, closeFn := openTestDB(t, opts, nil)
			defer closeFn()

			app := db.Appender()
			for i := int64(0); i < 155; i++ {
				_, err := app.Add(labels.Labels{labels.Label{Name: "wal", Value: "size"}}, i, rand.Float64())
				testutil.Ok(t, err)
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

	db, closeFn := openTestDB(t, nil, nil)
	defer closeFn()

	app := db.Appender()

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
			newSeries(map[string]string{"a": "b"}, expSamples),
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

			smplExp, errExp := expandSeriesIterator(sexp.Iterator())
			smplRes, errRes := expandSeriesIterator(sres.Iterator())

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
	db, closeFn := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		closeFn()
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
	db, closeFn := openTestDB(t, nil, []int64{1000})
	defer func() {
		testutil.Ok(t, db.Close())
		closeFn()
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
	db, closeFn := openTestDB(t, nil, []int64{100})
	defer func() {
		testutil.Ok(t, db.Close())
		closeFn()
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
	headApp := db.Head().Appender()
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
		db, closeFn := openTestDB(t, &Options{
			MaxBytes: c.maxBytes,
		}, []int64{100})
		defer func() {
			testutil.Ok(t, db.Close())
			closeFn()
		}()

		actMaxBytes := int64(prom_testutil.ToFloat64(db.metrics.maxBytes))
		testutil.Equals(t, actMaxBytes, c.expMaxBytes, "metric retention limit bytes mismatch")
	}
}

func TestNotMatcherSelectsLabelsUnsetSeries(t *testing.T) {
	db, closeFn := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		closeFn()
	}()

	labelpairs := []labels.Labels{
		labels.FromStrings("a", "abcd", "b", "abcde"),
		labels.FromStrings("labelname", "labelvalue"),
	}

	app := db.Appender()
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
	db, closeFn := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		closeFn()
	}()

	app := db.Appender()

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
	db, closeFn := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		closeFn()
	}()

	app := db.Appender()

	blockRange := db.compactor.(*LeveledCompactor).ranges[0]
	label := labels.FromStrings("foo", "bar")

	for i := int64(0); i < 5; i++ {
		_, err := app.Add(label, i*blockRange, 0)
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

	// The requested interval covers 2 blocks, so the querier should contain 2 blocks.
	count := len(q.(*querier).blocks)
	testutil.Assert(t, count == 2, "expected 2 blocks in querier, got %d", count)
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
		app := db.Appender()
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
	db, closeFn := openTestDB(t, nil, []int64{100})
	defer func() {
		testutil.Ok(t, db.Close())
		closeFn()
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
		app := db.Appender()
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

		app = db.Appender()
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
		app := db.Appender()
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
		app := db.Appender()
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
		db, closeFn := openTestDB(t, nil, nil)
		defer func() {
			testutil.Ok(t, db.Close())
			closeFn()
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
	db, closeFn := openTestDB(t, nil, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		closeFn()
	}()

	blockRange := db.compactor.(*LeveledCompactor).ranges[0]
	defaultLabel := labels.FromStrings("foo", "bar")
	defaultMatcher := labels.MustNewMatcher(labels.MatchEqual, defaultLabel[0].Name, defaultLabel[0].Value)

	app := db.Appender()
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

func TestVerticalCompaction(t *testing.T) {
	cases := []struct {
		blockSeries          [][]storage.Series
		expSeries            map[string][]tsdbutil.Sample
		expBlockNum          int
		expOverlappingBlocks int
	}{
		// Case 0
		// |--------------|
		//        |----------------|
		{
			blockSeries: [][]storage.Series{
				{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{4, 0},
						sample{5, 0}, sample{7, 0}, sample{8, 0}, sample{9, 0},
					}),
				},
				{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{3, 99}, sample{5, 99}, sample{6, 99}, sample{7, 99},
						sample{8, 99}, sample{9, 99}, sample{10, 99}, sample{11, 99},
						sample{12, 99}, sample{13, 99}, sample{14, 99},
					}),
				},
			},
			expSeries: map[string][]tsdbutil.Sample{`{a="b"}`: {
				sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{3, 99},
				sample{4, 0}, sample{5, 99}, sample{6, 99}, sample{7, 99},
				sample{8, 99}, sample{9, 99}, sample{10, 99}, sample{11, 99},
				sample{12, 99}, sample{13, 99}, sample{14, 99},
			}},
			expBlockNum:          1,
			expOverlappingBlocks: 1,
		},
		// Case 1
		// |-------------------------------|
		//        |----------------|
		{
			blockSeries: [][]storage.Series{
				{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{4, 0},
						sample{5, 0}, sample{7, 0}, sample{8, 0}, sample{9, 0},
						sample{11, 0}, sample{13, 0}, sample{17, 0},
					}),
				},
				{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{3, 99}, sample{5, 99}, sample{6, 99}, sample{7, 99},
						sample{8, 99}, sample{9, 99}, sample{10, 99},
					}),
				},
			},
			expSeries: map[string][]tsdbutil.Sample{`{a="b"}`: {
				sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{3, 99},
				sample{4, 0}, sample{5, 99}, sample{6, 99}, sample{7, 99},
				sample{8, 99}, sample{9, 99}, sample{10, 99}, sample{11, 0},
				sample{13, 0}, sample{17, 0},
			}},
			expBlockNum:          1,
			expOverlappingBlocks: 1,
		},
		// Case 2
		// |-------------------------------|
		//        |------------|
		//                           |--------------------|
		{
			blockSeries: [][]storage.Series{
				{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{4, 0},
						sample{5, 0}, sample{7, 0}, sample{8, 0}, sample{9, 0},
						sample{11, 0}, sample{13, 0}, sample{17, 0},
					}),
				},
				{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{3, 99}, sample{5, 99}, sample{6, 99}, sample{7, 99},
						sample{8, 99}, sample{9, 99},
					}),
				},
				{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{14, 59}, sample{15, 59}, sample{17, 59}, sample{20, 59},
						sample{21, 59}, sample{22, 59},
					}),
				},
			},
			expSeries: map[string][]tsdbutil.Sample{`{a="b"}`: {
				sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{3, 99},
				sample{4, 0}, sample{5, 99}, sample{6, 99}, sample{7, 99},
				sample{8, 99}, sample{9, 99}, sample{11, 0}, sample{13, 0},
				sample{14, 59}, sample{15, 59}, sample{17, 59}, sample{20, 59},
				sample{21, 59}, sample{22, 59},
			}},
			expBlockNum:          1,
			expOverlappingBlocks: 1,
		},
		// Case 3
		// |-------------------|
		//                           |--------------------|
		//               |----------------|
		{
			blockSeries: [][]storage.Series{
				{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{4, 0},
						sample{5, 0}, sample{8, 0}, sample{9, 0},
					}),
				},
				{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{14, 59}, sample{15, 59}, sample{17, 59}, sample{20, 59},
						sample{21, 59}, sample{22, 59},
					}),
				},
				{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{5, 99}, sample{6, 99}, sample{7, 99}, sample{8, 99},
						sample{9, 99}, sample{10, 99}, sample{13, 99}, sample{15, 99},
						sample{16, 99}, sample{17, 99},
					}),
				},
			},
			expSeries: map[string][]tsdbutil.Sample{`{a="b"}`: {
				sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{4, 0},
				sample{5, 99}, sample{6, 99}, sample{7, 99}, sample{8, 99},
				sample{9, 99}, sample{10, 99}, sample{13, 99}, sample{14, 59},
				sample{15, 59}, sample{16, 99}, sample{17, 59}, sample{20, 59},
				sample{21, 59}, sample{22, 59},
			}},
			expBlockNum:          1,
			expOverlappingBlocks: 1,
		},
		// Case 4
		// |-------------------------------------|
		//            |------------|
		//      |-------------------------|
		{
			blockSeries: [][]storage.Series{
				{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{4, 0},
						sample{5, 0}, sample{8, 0}, sample{9, 0}, sample{10, 0},
						sample{13, 0}, sample{15, 0}, sample{16, 0}, sample{17, 0},
						sample{20, 0}, sample{22, 0},
					}),
				},
				{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{7, 59}, sample{8, 59}, sample{9, 59}, sample{10, 59},
						sample{11, 59},
					}),
				},
				{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{3, 99}, sample{5, 99}, sample{6, 99}, sample{8, 99},
						sample{9, 99}, sample{10, 99}, sample{13, 99}, sample{15, 99},
						sample{16, 99}, sample{17, 99},
					}),
				},
			},
			expSeries: map[string][]tsdbutil.Sample{`{a="b"}`: {
				sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{3, 99},
				sample{4, 0}, sample{5, 99}, sample{6, 99}, sample{7, 59},
				sample{8, 59}, sample{9, 59}, sample{10, 59}, sample{11, 59},
				sample{13, 99}, sample{15, 99}, sample{16, 99}, sample{17, 99},
				sample{20, 0}, sample{22, 0},
			}},
			expBlockNum:          1,
			expOverlappingBlocks: 1,
		},
		// Case 5: series are merged properly when there are multiple series.
		// |-------------------------------------|
		//            |------------|
		//      |-------------------------|
		{
			blockSeries: [][]storage.Series{
				{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{4, 0},
						sample{5, 0}, sample{8, 0}, sample{9, 0}, sample{10, 0},
						sample{13, 0}, sample{15, 0}, sample{16, 0}, sample{17, 0},
						sample{20, 0}, sample{22, 0},
					}),
					newSeries(map[string]string{"b": "c"}, []tsdbutil.Sample{
						sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{4, 0},
						sample{5, 0}, sample{8, 0}, sample{9, 0}, sample{10, 0},
						sample{13, 0}, sample{15, 0}, sample{16, 0}, sample{17, 0},
						sample{20, 0}, sample{22, 0},
					}),
					newSeries(map[string]string{"c": "d"}, []tsdbutil.Sample{
						sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{4, 0},
						sample{5, 0}, sample{8, 0}, sample{9, 0}, sample{10, 0},
						sample{13, 0}, sample{15, 0}, sample{16, 0}, sample{17, 0},
						sample{20, 0}, sample{22, 0},
					}),
				},
				{
					newSeries(map[string]string{"__name__": "a"}, []tsdbutil.Sample{
						sample{7, 59}, sample{8, 59}, sample{9, 59}, sample{10, 59},
						sample{11, 59},
					}),
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{7, 59}, sample{8, 59}, sample{9, 59}, sample{10, 59},
						sample{11, 59},
					}),
					newSeries(map[string]string{"aa": "bb"}, []tsdbutil.Sample{
						sample{7, 59}, sample{8, 59}, sample{9, 59}, sample{10, 59},
						sample{11, 59},
					}),
					newSeries(map[string]string{"c": "d"}, []tsdbutil.Sample{
						sample{7, 59}, sample{8, 59}, sample{9, 59}, sample{10, 59},
						sample{11, 59},
					}),
				},
				{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{3, 99}, sample{5, 99}, sample{6, 99}, sample{8, 99},
						sample{9, 99}, sample{10, 99}, sample{13, 99}, sample{15, 99},
						sample{16, 99}, sample{17, 99},
					}),
					newSeries(map[string]string{"aa": "bb"}, []tsdbutil.Sample{
						sample{3, 99}, sample{5, 99}, sample{6, 99}, sample{8, 99},
						sample{9, 99}, sample{10, 99}, sample{13, 99}, sample{15, 99},
						sample{16, 99}, sample{17, 99},
					}),
					newSeries(map[string]string{"c": "d"}, []tsdbutil.Sample{
						sample{3, 99}, sample{5, 99}, sample{6, 99}, sample{8, 99},
						sample{9, 99}, sample{10, 99}, sample{13, 99}, sample{15, 99},
						sample{16, 99}, sample{17, 99},
					}),
				},
			},
			expSeries: map[string][]tsdbutil.Sample{
				`{__name__="a"}`: {
					sample{7, 59}, sample{8, 59}, sample{9, 59}, sample{10, 59},
					sample{11, 59},
				},
				`{a="b"}`: {
					sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{3, 99},
					sample{4, 0}, sample{5, 99}, sample{6, 99}, sample{7, 59},
					sample{8, 59}, sample{9, 59}, sample{10, 59}, sample{11, 59},
					sample{13, 99}, sample{15, 99}, sample{16, 99}, sample{17, 99},
					sample{20, 0}, sample{22, 0},
				},
				`{aa="bb"}`: {
					sample{3, 99}, sample{5, 99}, sample{6, 99}, sample{7, 59},
					sample{8, 59}, sample{9, 59}, sample{10, 59}, sample{11, 59},
					sample{13, 99}, sample{15, 99}, sample{16, 99}, sample{17, 99},
				},
				`{b="c"}`: {
					sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{4, 0},
					sample{5, 0}, sample{8, 0}, sample{9, 0}, sample{10, 0},
					sample{13, 0}, sample{15, 0}, sample{16, 0}, sample{17, 0},
					sample{20, 0}, sample{22, 0},
				},
				`{c="d"}`: {
					sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{3, 99},
					sample{4, 0}, sample{5, 99}, sample{6, 99}, sample{7, 59},
					sample{8, 59}, sample{9, 59}, sample{10, 59}, sample{11, 59},
					sample{13, 99}, sample{15, 99}, sample{16, 99}, sample{17, 99},
					sample{20, 0}, sample{22, 0},
				},
			},
			expBlockNum:          1,
			expOverlappingBlocks: 1,
		},
		// Case 6
		// |--------------|
		//        |----------------|
		//                                         |--------------|
		//                                                  |----------------|
		{
			blockSeries: [][]storage.Series{
				{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{4, 0},
						sample{5, 0}, sample{7, 0}, sample{8, 0}, sample{9, 0},
					}),
				},
				{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{3, 99}, sample{5, 99}, sample{6, 99}, sample{7, 99},
						sample{8, 99}, sample{9, 99}, sample{10, 99}, sample{11, 99},
						sample{12, 99}, sample{13, 99}, sample{14, 99},
					}),
				},
				{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{20, 0}, sample{21, 0}, sample{22, 0}, sample{24, 0},
						sample{25, 0}, sample{27, 0}, sample{28, 0}, sample{29, 0},
					}),
				},
				{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{23, 99}, sample{25, 99}, sample{26, 99}, sample{27, 99},
						sample{28, 99}, sample{29, 99}, sample{30, 99}, sample{31, 99},
					}),
				},
			},
			expSeries: map[string][]tsdbutil.Sample{`{a="b"}`: {
				sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{3, 99},
				sample{4, 0}, sample{5, 99}, sample{6, 99}, sample{7, 99},
				sample{8, 99}, sample{9, 99}, sample{10, 99}, sample{11, 99},
				sample{12, 99}, sample{13, 99}, sample{14, 99},
				sample{20, 0}, sample{21, 0}, sample{22, 0}, sample{23, 99},
				sample{24, 0}, sample{25, 99}, sample{26, 99}, sample{27, 99},
				sample{28, 99}, sample{29, 99}, sample{30, 99}, sample{31, 99},
			}},
			expBlockNum:          2,
			expOverlappingBlocks: 2,
		},
	}

	defaultMatcher := labels.MustNewMatcher(labels.MatchRegexp, "__name__", ".*")
	for _, c := range cases {
		if ok := t.Run("", func(t *testing.T) {

			tmpdir, err := ioutil.TempDir("", "data")
			testutil.Ok(t, err)
			defer func() {
				testutil.Ok(t, os.RemoveAll(tmpdir))
			}()

			for _, series := range c.blockSeries {
				createBlock(t, tmpdir, series)
			}
			opts := DefaultOptions()
			opts.AllowOverlappingBlocks = true
			db, err := Open(tmpdir, nil, nil, opts)
			testutil.Ok(t, err)
			defer func() {
				testutil.Ok(t, db.Close())
			}()
			db.DisableCompactions()
			testutil.Assert(t, len(db.blocks) == len(c.blockSeries), "Wrong number of blocks [before compact].")

			// Vertical Query Merging test.
			querier, err := db.Querier(context.TODO(), 0, 100)
			testutil.Ok(t, err)
			actSeries := query(t, querier, defaultMatcher)
			testutil.Equals(t, c.expSeries, actSeries)

			// Vertical compaction.
			lc := db.compactor.(*LeveledCompactor)
			testutil.Equals(t, 0, int(prom_testutil.ToFloat64(lc.metrics.overlappingBlocks)), "overlapping blocks count should be still 0 here")
			err = db.Compact()
			testutil.Ok(t, err)
			testutil.Equals(t, c.expBlockNum, len(db.Blocks()), "Wrong number of blocks [after compact]")

			testutil.Equals(t, c.expOverlappingBlocks, int(prom_testutil.ToFloat64(lc.metrics.overlappingBlocks)), "overlapping blocks count mismatch")

			// Query test after merging the overlapping blocks.
			querier, err = db.Querier(context.TODO(), 0, 100)
			testutil.Ok(t, err)
			actSeries = query(t, querier, defaultMatcher)
			testutil.Equals(t, c.expSeries, actSeries)
		}); !ok {
			return
		}
	}
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
	app := db.Appender()
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

	app = db.Appender()
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
		dbDir          string
		logger         = log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
		expBlocks      []*Block
		expSeries      map[string][]tsdbutil.Sample
		expSeriesCount int
		expDBHash      []byte
		matchAll       = labels.MustNewMatcher(labels.MatchEqual, "", "")
		err            error
	)

	// Bootstrap the db.
	{
		dbDir, err = ioutil.TempDir("", "test")
		testutil.Ok(t, err)

		defer func() {
			testutil.Ok(t, os.RemoveAll(dbDir))
		}()

		dbBlocks := []*BlockMeta{
			{MinTime: 10, MaxTime: 11},
			{MinTime: 11, MaxTime: 12},
			{MinTime: 12, MaxTime: 13},
		}

		for _, m := range dbBlocks {
			createBlock(t, dbDir, genSeries(1, 1, m.MinTime, m.MaxTime))
		}
		expSeriesCount++
	}

	// Open a normal db to use for a comparison.
	{
		dbWritable, err := Open(dbDir, logger, nil, nil)
		testutil.Ok(t, err)
		dbWritable.DisableCompactions()

		dbSizeBeforeAppend, err := fileutil.DirSize(dbWritable.Dir())
		testutil.Ok(t, err)
		app := dbWritable.Appender()
		_, err = app.Add(labels.FromStrings("foo", "bar"), dbWritable.Head().MaxTime()+1, 0)
		testutil.Ok(t, err)
		testutil.Ok(t, app.Commit())
		expSeriesCount++

		expBlocks = dbWritable.Blocks()
		expDbSize, err := fileutil.DirSize(dbWritable.Dir())
		testutil.Ok(t, err)
		testutil.Assert(t, expDbSize > dbSizeBeforeAppend, "db size didn't increase after an append")

		q, err := dbWritable.Querier(context.TODO(), math.MinInt64, math.MaxInt64)
		testutil.Ok(t, err)
		expSeries = query(t, q, matchAll)

		testutil.Ok(t, dbWritable.Close()) // Close here to allow getting the dir hash for windows.
		expDBHash = testutil.DirHash(t, dbWritable.Dir())
	}

	// Open a read only db and ensure that the API returns the same result as the normal DB.
	{
		dbReadOnly, err := OpenDBReadOnly(dbDir, logger)
		testutil.Ok(t, err)
		defer func() {
			testutil.Ok(t, dbReadOnly.Close())
		}()
		blocks, err := dbReadOnly.Blocks()
		testutil.Ok(t, err)
		testutil.Equals(t, len(expBlocks), len(blocks))

		for i, expBlock := range expBlocks {
			testutil.Equals(t, expBlock.Meta(), blocks[i].Meta(), "block meta mismatch")
		}

		q, err := dbReadOnly.Querier(context.TODO(), math.MinInt64, math.MaxInt64)
		testutil.Ok(t, err)
		readOnlySeries := query(t, q, matchAll)
		readOnlyDBHash := testutil.DirHash(t, dbDir)

		testutil.Equals(t, expSeriesCount, len(readOnlySeries), "total series mismatch")
		testutil.Equals(t, expSeries, readOnlySeries, "series mismatch")
		testutil.Equals(t, expDBHash, readOnlyDBHash, "after all read operations the db hash should remain the same")
	}
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
		app := db.Appender()
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

	// Insert data in batches.
	go func() {
		iter := 0
		for {
			app := db.Appender()

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

	app := db.Appender()
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
	app := db.Appender()
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
