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
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/tsdb/chunks"
	"github.com/prometheus/tsdb/index"
	"github.com/prometheus/tsdb/labels"
	"github.com/prometheus/tsdb/testutil"
	"github.com/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/tsdb/wal"
)

func openTestDB(t testing.TB, opts *Options) (db *DB, close func()) {
	tmpdir, err := ioutil.TempDir("", "test")
	testutil.Ok(t, err)

	db, err = Open(tmpdir, nil, nil, opts)
	testutil.Ok(t, err)

	// Do not close the test database by default as it will deadlock on test failures.
	return db, func() {
		testutil.Ok(t, os.RemoveAll(tmpdir))
	}
}

// query runs a matcher query against the querier and fully expands its data.
func query(t testing.TB, q Querier, matchers ...labels.Matcher) map[string][]tsdbutil.Sample {
	ss, err := q.Select(matchers...)
	defer func() {
		testutil.Ok(t, q.Close())
	}()
	testutil.Ok(t, err)

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

		name := series.Labels().String()
		result[name] = samples
	}
	testutil.Ok(t, ss.Err())

	return result
}

// Ensure that blocks are held in memory in their time order
// and not in ULID order as they are read from the directory.
func TestDB_reloadOrder(t *testing.T) {
	db, delete := openTestDB(t, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		delete()
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
	for _, b := range blocks {
		b.meta.Stats.NumBytes = 0
	}
	testutil.Equals(t, 3, len(blocks))
	testutil.Equals(t, metas[1].MinTime, blocks[0].Meta().MinTime)
	testutil.Equals(t, metas[1].MaxTime, blocks[0].Meta().MaxTime)
	testutil.Equals(t, metas[0].MinTime, blocks[1].Meta().MinTime)
	testutil.Equals(t, metas[0].MaxTime, blocks[1].Meta().MaxTime)
	testutil.Equals(t, metas[2].MinTime, blocks[2].Meta().MinTime)
	testutil.Equals(t, metas[2].MaxTime, blocks[2].Meta().MaxTime)
}

func TestDataAvailableOnlyAfterCommit(t *testing.T) {
	db, delete := openTestDB(t, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		delete()
	}()

	app := db.Appender()

	_, err := app.Add(labels.FromStrings("foo", "bar"), 0, 0)
	testutil.Ok(t, err)

	querier, err := db.Querier(0, 1)
	testutil.Ok(t, err)
	seriesSet := query(t, querier, labels.NewEqualMatcher("foo", "bar"))
	testutil.Equals(t, map[string][]tsdbutil.Sample{}, seriesSet)

	err = app.Commit()
	testutil.Ok(t, err)

	querier, err = db.Querier(0, 1)
	testutil.Ok(t, err)
	defer querier.Close()

	seriesSet = query(t, querier, labels.NewEqualMatcher("foo", "bar"))

	testutil.Equals(t, map[string][]tsdbutil.Sample{`{foo="bar"}`: {sample{t: 0, v: 0}}}, seriesSet)
}

func TestDataNotAvailableAfterRollback(t *testing.T) {
	db, delete := openTestDB(t, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		delete()
	}()

	app := db.Appender()
	_, err := app.Add(labels.FromStrings("foo", "bar"), 0, 0)
	testutil.Ok(t, err)

	err = app.Rollback()
	testutil.Ok(t, err)

	querier, err := db.Querier(0, 1)
	testutil.Ok(t, err)
	defer querier.Close()

	seriesSet := query(t, querier, labels.NewEqualMatcher("foo", "bar"))

	testutil.Equals(t, map[string][]tsdbutil.Sample{}, seriesSet)
}

func TestDBAppenderAddRef(t *testing.T) {
	db, delete := openTestDB(t, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		delete()
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
	testutil.Equals(t, ErrNotFound, errors.Cause(err))

	testutil.Ok(t, app2.Commit())

	q, err := db.Querier(0, 200)
	testutil.Ok(t, err)

	res := query(t, q, labels.NewEqualMatcher("a", "b"))

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

func TestDeleteSimple(t *testing.T) {
	numSamples := int64(10)

	cases := []struct {
		intervals Intervals
		remaint   []int64
	}{
		{
			intervals: Intervals{{0, 3}},
			remaint:   []int64{4, 5, 6, 7, 8, 9},
		},
		{
			intervals: Intervals{{1, 3}},
			remaint:   []int64{0, 4, 5, 6, 7, 8, 9},
		},
		{
			intervals: Intervals{{1, 3}, {4, 7}},
			remaint:   []int64{0, 8, 9},
		},
		{
			intervals: Intervals{{1, 3}, {4, 700}},
			remaint:   []int64{0},
		},
		{ // This case is to ensure that labels and symbols are deleted.
			intervals: Intervals{{0, 9}},
			remaint:   []int64{},
		},
	}

Outer:
	for _, c := range cases {
		db, delete := openTestDB(t, nil)
		defer func() {
			testutil.Ok(t, db.Close())
			delete()
		}()

		app := db.Appender()

		smpls := make([]float64, numSamples)
		for i := int64(0); i < numSamples; i++ {
			smpls[i] = rand.Float64()
			app.Add(labels.Labels{{"a", "b"}}, i, smpls[i])
		}

		testutil.Ok(t, app.Commit())

		// TODO(gouthamve): Reset the tombstones somehow.
		// Delete the ranges.
		for _, r := range c.intervals {
			testutil.Ok(t, db.Delete(r.Mint, r.Maxt, labels.NewEqualMatcher("a", "b")))
		}

		// Compare the result.
		q, err := db.Querier(0, numSamples)
		testutil.Ok(t, err)

		res, err := q.Select(labels.NewEqualMatcher("a", "b"))
		testutil.Ok(t, err)

		expSamples := make([]tsdbutil.Sample, 0, len(c.remaint))
		for _, ts := range c.remaint {
			expSamples = append(expSamples, sample{ts, smpls[ts]})
		}

		expss := newMockSeriesSet([]Series{
			newSeries(map[string]string{"a": "b"}, expSamples),
		})

		lns, err := q.LabelNames()
		testutil.Ok(t, err)
		lvs, err := q.LabelValues("a")
		testutil.Ok(t, err)
		if len(expSamples) == 0 {
			testutil.Equals(t, 0, len(lns))
			testutil.Equals(t, 0, len(lvs))
			testutil.Assert(t, res.Next() == false, "")
			continue
		} else {
			testutil.Equals(t, 1, len(lns))
			testutil.Equals(t, 1, len(lvs))
			testutil.Equals(t, "a", lns[0])
			testutil.Equals(t, "b", lvs[0])
		}

		for {
			eok, rok := expss.Next(), res.Next()
			testutil.Equals(t, eok, rok)

			if !eok {
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
	db, delete := openTestDB(t, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		delete()
	}()

	app := db.Appender()
	_, err := app.Add(labels.Labels{}, 0, 0)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	app = db.Appender()
	_, err = app.Add(labels.Labels{}, 0, 1)
	testutil.Equals(t, ErrAmendSample, err)
	testutil.Ok(t, app.Rollback())
}

func TestDuplicateNaNDatapointNoAmendError(t *testing.T) {
	db, delete := openTestDB(t, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		delete()
	}()

	app := db.Appender()
	_, err := app.Add(labels.Labels{}, 0, math.NaN())
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	app = db.Appender()
	_, err = app.Add(labels.Labels{}, 0, math.NaN())
	testutil.Ok(t, err)
}

func TestNonDuplicateNaNDatapointsCausesAmendError(t *testing.T) {
	db, delete := openTestDB(t, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		delete()
	}()
	app := db.Appender()
	_, err := app.Add(labels.Labels{}, 0, math.Float64frombits(0x7ff0000000000001))
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	app = db.Appender()
	_, err = app.Add(labels.Labels{}, 0, math.Float64frombits(0x7ff0000000000002))
	testutil.Equals(t, ErrAmendSample, err)
}

func TestSkippingInvalidValuesInSameTxn(t *testing.T) {
	db, delete := openTestDB(t, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		delete()
	}()

	// Append AmendedValue.
	app := db.Appender()
	_, err := app.Add(labels.Labels{{"a", "b"}}, 0, 1)
	testutil.Ok(t, err)
	_, err = app.Add(labels.Labels{{"a", "b"}}, 0, 2)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	// Make sure the right value is stored.
	q, err := db.Querier(0, 10)
	testutil.Ok(t, err)

	ssMap := query(t, q, labels.NewEqualMatcher("a", "b"))

	testutil.Equals(t, map[string][]tsdbutil.Sample{
		labels.New(labels.Label{"a", "b"}).String(): {sample{0, 1}},
	}, ssMap)

	// Append Out of Order Value.
	app = db.Appender()
	_, err = app.Add(labels.Labels{{"a", "b"}}, 10, 3)
	testutil.Ok(t, err)
	_, err = app.Add(labels.Labels{{"a", "b"}}, 7, 5)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	q, err = db.Querier(0, 10)
	testutil.Ok(t, err)

	ssMap = query(t, q, labels.NewEqualMatcher("a", "b"))

	testutil.Equals(t, map[string][]tsdbutil.Sample{
		labels.New(labels.Label{"a", "b"}).String(): {sample{0, 1}, sample{10, 3}},
	}, ssMap)
}

func TestDB_Snapshot(t *testing.T) {
	db, delete := openTestDB(t, nil)
	defer delete()

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

	defer os.RemoveAll(snap)
	testutil.Ok(t, db.Snapshot(snap, true))
	testutil.Ok(t, db.Close())

	// reopen DB from snapshot
	db, err = Open(snap, nil, nil, nil)
	testutil.Ok(t, err)
	defer func() { testutil.Ok(t, db.Close()) }()

	querier, err := db.Querier(mint, mint+1000)
	testutil.Ok(t, err)
	defer func() { testutil.Ok(t, querier.Close()) }()

	// sum values
	seriesSet, err := querier.Select(labels.NewEqualMatcher("foo", "bar"))
	testutil.Ok(t, err)

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
	testutil.Equals(t, 1000.0, sum)
}

func TestDB_SnapshotWithDelete(t *testing.T) {
	numSamples := int64(10)

	db, delete := openTestDB(t, nil)
	defer delete()

	app := db.Appender()

	smpls := make([]float64, numSamples)
	for i := int64(0); i < numSamples; i++ {
		smpls[i] = rand.Float64()
		app.Add(labels.Labels{{"a", "b"}}, i, smpls[i])
	}

	testutil.Ok(t, app.Commit())
	cases := []struct {
		intervals Intervals
		remaint   []int64
	}{
		{
			intervals: Intervals{{1, 3}, {4, 7}},
			remaint:   []int64{0, 8, 9},
		},
	}

Outer:
	for _, c := range cases {
		// TODO(gouthamve): Reset the tombstones somehow.
		// Delete the ranges.
		for _, r := range c.intervals {
			testutil.Ok(t, db.Delete(r.Mint, r.Maxt, labels.NewEqualMatcher("a", "b")))
		}

		// create snapshot
		snap, err := ioutil.TempDir("", "snap")
		testutil.Ok(t, err)

		defer os.RemoveAll(snap)
		testutil.Ok(t, db.Snapshot(snap, true))
		testutil.Ok(t, db.Close())

		// reopen DB from snapshot
		db, err = Open(snap, nil, nil, nil)
		testutil.Ok(t, err)
		defer func() { testutil.Ok(t, db.Close()) }()

		// Compare the result.
		q, err := db.Querier(0, numSamples)
		testutil.Ok(t, err)
		defer func() { testutil.Ok(t, q.Close()) }()

		res, err := q.Select(labels.NewEqualMatcher("a", "b"))
		testutil.Ok(t, err)

		expSamples := make([]tsdbutil.Sample, 0, len(c.remaint))
		for _, ts := range c.remaint {
			expSamples = append(expSamples, sample{ts, smpls[ts]})
		}

		expss := newMockSeriesSet([]Series{
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
	lbls := [][]labels.Label{
		{
			{"a", "b"},
			{"instance", "localhost:9090"},
			{"job", "prometheus"},
		},
		{
			{"a", "b"},
			{"instance", "127.0.0.1:9090"},
			{"job", "prometheus"},
		},
		{
			{"a", "b"},
			{"instance", "127.0.0.1:9090"},
			{"job", "prom-k8s"},
		},
		{
			{"a", "b"},
			{"instance", "localhost:9090"},
			{"job", "prom-k8s"},
		},
		{
			{"a", "c"},
			{"instance", "localhost:9090"},
			{"job", "prometheus"},
		},
		{
			{"a", "c"},
			{"instance", "127.0.0.1:9090"},
			{"job", "prometheus"},
		},
		{
			{"a", "c"},
			{"instance", "127.0.0.1:9090"},
			{"job", "prom-k8s"},
		},
		{
			{"a", "c"},
			{"instance", "localhost:9090"},
			{"job", "prom-k8s"},
		},
	}

	seriesMap := map[string][]tsdbutil.Sample{}
	for _, l := range lbls {
		seriesMap[labels.New(l...).String()] = []tsdbutil.Sample{}
	}

	db, delete := openTestDB(t, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		delete()
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
		ms []labels.Matcher
	}{
		{
			ms: []labels.Matcher{labels.NewEqualMatcher("a", "b")},
		},
		{
			ms: []labels.Matcher{
				labels.NewEqualMatcher("a", "b"),
				labels.NewEqualMatcher("job", "prom-k8s"),
			},
		},
		{
			ms: []labels.Matcher{
				labels.NewEqualMatcher("a", "c"),
				labels.NewEqualMatcher("instance", "localhost:9090"),
				labels.NewEqualMatcher("job", "prometheus"),
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

			q, err := db.Querier(mint, maxt)
			testutil.Ok(t, err)

			ss, err := q.Select(qry.ms...)
			testutil.Ok(t, err)

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
			testutil.Equals(t, expected, result)

			q.Close()
		}
	}
}

func TestWALFlushedOnDBClose(t *testing.T) {
	db, delete := openTestDB(t, nil)
	defer delete()

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

	q, err := db.Querier(0, 1)
	testutil.Ok(t, err)

	values, err := q.LabelValues("labelname")
	testutil.Ok(t, err)
	testutil.Equals(t, []string{"labelvalue"}, values)
}

func TestWALSegmentSizeOption(t *testing.T) {
	options := *DefaultOptions
	options.WALSegmentSize = 2 * 32 * 1024
	db, delete := openTestDB(t, &options)
	defer delete()
	app := db.Appender()
	for i := int64(0); i < 155; i++ {
		_, err := app.Add(labels.Labels{labels.Label{Name: "wal", Value: "size"}}, i, rand.Float64())
		testutil.Ok(t, err)
		testutil.Ok(t, app.Commit())
	}

	dbDir := db.Dir()
	db.Close()
	files, err := ioutil.ReadDir(filepath.Join(dbDir, "wal"))
	testutil.Assert(t, len(files) > 1, "current WALSegmentSize should result in more than a single WAL file.")
	testutil.Ok(t, err)
	for i, f := range files {
		if len(files)-1 != i {
			testutil.Equals(t, int64(options.WALSegmentSize), f.Size(), "WAL file size doesn't match WALSegmentSize option, filename: %v", f.Name())
			continue
		}
		testutil.Assert(t, int64(options.WALSegmentSize) > f.Size(), "last WAL file size is not smaller than the WALSegmentSize option, filename: %v", f.Name())
	}
}

func TestTombstoneClean(t *testing.T) {
	numSamples := int64(10)

	db, delete := openTestDB(t, nil)
	defer delete()

	app := db.Appender()

	smpls := make([]float64, numSamples)
	for i := int64(0); i < numSamples; i++ {
		smpls[i] = rand.Float64()
		app.Add(labels.Labels{{"a", "b"}}, i, smpls[i])
	}

	testutil.Ok(t, app.Commit())
	cases := []struct {
		intervals Intervals
		remaint   []int64
	}{
		{
			intervals: Intervals{{1, 3}, {4, 7}},
			remaint:   []int64{0, 8, 9},
		},
	}

	for _, c := range cases {
		// Delete the ranges.

		// create snapshot
		snap, err := ioutil.TempDir("", "snap")
		testutil.Ok(t, err)

		defer os.RemoveAll(snap)
		testutil.Ok(t, db.Snapshot(snap, true))
		testutil.Ok(t, db.Close())

		// reopen DB from snapshot
		db, err = Open(snap, nil, nil, nil)
		testutil.Ok(t, err)
		defer db.Close()

		for _, r := range c.intervals {
			testutil.Ok(t, db.Delete(r.Mint, r.Maxt, labels.NewEqualMatcher("a", "b")))
		}

		// All of the setup for THIS line.
		testutil.Ok(t, db.CleanTombstones())

		// Compare the result.
		q, err := db.Querier(0, numSamples)
		testutil.Ok(t, err)
		defer q.Close()

		res, err := q.Select(labels.NewEqualMatcher("a", "b"))
		testutil.Ok(t, err)

		expSamples := make([]tsdbutil.Sample, 0, len(c.remaint))
		for _, ts := range c.remaint {
			expSamples = append(expSamples, sample{ts, smpls[ts]})
		}

		expss := newMockSeriesSet([]Series{
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

		for _, b := range db.Blocks() {
			testutil.Equals(t, newMemTombstones(), b.tombstones)
		}
	}
}

// TestTombstoneCleanFail tests that a failing TombstoneClean doesn't leave any blocks behind.
// When TombstoneClean errors the original block that should be rebuilt doesn't get deleted so
// if TombstoneClean leaves any blocks behind these will overlap.
func TestTombstoneCleanFail(t *testing.T) {

	db, delete := openTestDB(t, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		delete()
	}()

	var expectedBlockDirs []string

	// Create some empty blocks pending for compaction.
	// totalBlocks should be >=2 so we have enough blocks to trigger compaction failure.
	totalBlocks := 2
	for i := 0; i < totalBlocks; i++ {
		blockDir := createBlock(t, db.Dir(), genSeries(1, 1, 0, 0))
		block, err := OpenBlock(nil, blockDir, nil)
		testutil.Ok(t, err)
		// Add some some fake tombstones to trigger the compaction.
		tomb := newMemTombstones()
		tomb.addInterval(0, Interval{0, 1})
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

	block, err := OpenBlock(nil, createBlock(c.t, dest, genSeries(1, 1, 0, 0)), nil)
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

func (*mockCompactorFailing) Compact(dest string, dirs []string, open []*Block) (ulid.ULID, error) {
	return ulid.ULID{}, nil

}

func TestTimeRetention(t *testing.T) {
	db, delete := openTestDB(t, &Options{
		BlockRanges: []int64{1000},
	})
	defer func() {
		testutil.Ok(t, db.Close())
		delete()
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

	db.opts.RetentionDuration = uint64(blocks[2].MaxTime - blocks[1].MinTime)
	testutil.Ok(t, db.reload())

	expBlocks := blocks[1:]
	actBlocks := db.Blocks()

	testutil.Equals(t, 1, int(prom_testutil.ToFloat64(db.metrics.timeRetentionCount)), "metric retention count mismatch")
	testutil.Equals(t, len(expBlocks), len(actBlocks))
	testutil.Equals(t, expBlocks[0].MaxTime, actBlocks[0].meta.MaxTime)
	testutil.Equals(t, expBlocks[len(expBlocks)-1].MaxTime, actBlocks[len(actBlocks)-1].meta.MaxTime)
}

func TestSizeRetention(t *testing.T) {
	db, delete := openTestDB(t, &Options{
		BlockRanges: []int64{100},
	})
	defer func() {
		testutil.Ok(t, db.Close())
		delete()
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

	// Test that registered size matches the actual disk size.
	testutil.Ok(t, db.reload())                                       // Reload the db to register the new db size.
	testutil.Equals(t, len(blocks), len(db.Blocks()))                 // Ensure all blocks are registered.
	expSize := int64(prom_testutil.ToFloat64(db.metrics.blocksBytes)) // Use the the actual internal metrics.
	actSize := dbDiskSize(db.Dir())
	testutil.Equals(t, expSize, actSize, "registered size doesn't match actual disk size")

	// Decrease the max bytes limit so that a delete is triggered.
	// Check total size, total count and check that the oldest block was deleted.
	firstBlockSize := db.Blocks()[0].Size()
	sizeLimit := actSize - firstBlockSize
	db.opts.MaxBytes = sizeLimit // Set the new db size limit one block smaller that the actual size.
	testutil.Ok(t, db.reload())  // Reload the db to register the new db size.

	expBlocks := blocks[1:]
	actBlocks := db.Blocks()
	expSize = int64(prom_testutil.ToFloat64(db.metrics.blocksBytes))
	actRetentCount := int(prom_testutil.ToFloat64(db.metrics.sizeRetentionCount))
	actSize = dbDiskSize(db.Dir())

	testutil.Equals(t, 1, actRetentCount, "metric retention count mismatch")
	testutil.Equals(t, actSize, expSize, "metric db size doesn't match actual disk size")
	testutil.Assert(t, expSize <= sizeLimit, "actual size (%v) is expected to be less than or equal to limit (%v)", expSize, sizeLimit)
	testutil.Equals(t, len(blocks)-1, len(actBlocks), "new block count should be decreased from:%v to:%v", len(blocks), len(blocks)-1)
	testutil.Equals(t, expBlocks[0].MaxTime, actBlocks[0].meta.MaxTime, "maxT mismatch of the first block")
	testutil.Equals(t, expBlocks[len(expBlocks)-1].MaxTime, actBlocks[len(actBlocks)-1].meta.MaxTime, "maxT mismatch of the last block")

}

func dbDiskSize(dir string) int64 {
	var statSize int64
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		// Include only index,tombstone and chunks.
		if filepath.Dir(path) == chunkDir(filepath.Dir(filepath.Dir(path))) ||
			info.Name() == indexFilename ||
			info.Name() == tombstoneFilename {
			statSize += info.Size()
		}
		return nil
	})
	return statSize
}

func TestNotMatcherSelectsLabelsUnsetSeries(t *testing.T) {
	db, delete := openTestDB(t, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		delete()
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
			labels.Not(labels.NewEqualMatcher("lname", "lvalue")),
		},
		series: labelpairs,
	}, {
		selector: labels.Selector{
			labels.NewEqualMatcher("a", "abcd"),
			labels.Not(labels.NewEqualMatcher("b", "abcde")),
		},
		series: []labels.Labels{},
	}, {
		selector: labels.Selector{
			labels.NewEqualMatcher("a", "abcd"),
			labels.Not(labels.NewEqualMatcher("b", "abc")),
		},
		series: []labels.Labels{labelpairs[0]},
	}, {
		selector: labels.Selector{
			labels.Not(labels.NewMustRegexpMatcher("a", "abd.*")),
		},
		series: labelpairs,
	}, {
		selector: labels.Selector{
			labels.Not(labels.NewMustRegexpMatcher("a", "abc.*")),
		},
		series: labelpairs[1:],
	}, {
		selector: labels.Selector{
			labels.Not(labels.NewMustRegexpMatcher("c", "abd.*")),
		},
		series: labelpairs,
	}, {
		selector: labels.Selector{
			labels.Not(labels.NewMustRegexpMatcher("labelname", "labelvalue")),
		},
		series: labelpairs[:1],
	}}

	q, err := db.Querier(0, 10)
	testutil.Ok(t, err)
	defer func() { testutil.Ok(t, q.Close()) }()

	for _, c := range cases {
		ss, err := q.Select(c.selector...)
		testutil.Ok(t, err)

		lres, err := expandSeriesSet(ss)
		testutil.Ok(t, err)

		testutil.Equals(t, c.series, lres)
	}
}

func expandSeriesSet(ss SeriesSet) ([]labels.Labels, error) {
	result := []labels.Labels{}
	for ss.Next() {
		result = append(result, ss.At().Labels())
	}

	return result, ss.Err()
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
	db, delete := openTestDB(t, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		delete()
	}()

	app := db.Appender()

	blockRange := DefaultOptions.BlockRanges[0]
	label := labels.FromStrings("foo", "bar")

	for i := int64(0); i < 3; i++ {
		_, err := app.Add(label, i*blockRange, 0)
		testutil.Ok(t, err)
		_, err = app.Add(label, i*blockRange+1000, 0)
		testutil.Ok(t, err)
	}

	err := app.Commit()
	testutil.Ok(t, err)

	err = db.compact()
	testutil.Ok(t, err)

	for _, block := range db.Blocks() {
		r, err := block.Index()
		testutil.Ok(t, err)
		defer r.Close()

		meta := block.Meta()

		p, err := r.Postings(index.AllPostingsKey())
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
	db, delete := openTestDB(t, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		delete()
	}()

	app := db.Appender()

	blockRange := DefaultOptions.BlockRanges[0]
	label := labels.FromStrings("foo", "bar")

	for i := int64(0); i < 5; i++ {
		_, err := app.Add(label, i*blockRange, 0)
		testutil.Ok(t, err)
	}

	err := app.Commit()
	testutil.Ok(t, err)

	err = db.compact()
	testutil.Ok(t, err)

	testutil.Assert(t, len(db.blocks) >= 3, "invalid test, less than three blocks in DB")

	q, err := db.Querier(blockRange, 2*blockRange)
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
		defer os.RemoveAll(dir)

		db, err := Open(dir, nil, nil, nil)
		testutil.Ok(t, err)

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
		defer os.RemoveAll(dir)

		testutil.Ok(t, os.MkdirAll(path.Join(dir, "wal"), 0777))
		w, err := wal.New(nil, nil, path.Join(dir, "wal"))
		testutil.Ok(t, err)

		var enc RecordEncoder
		err = w.Log(
			enc.Series([]RefSeries{
				{Ref: 123, Labels: labels.FromStrings("a", "1")},
				{Ref: 124, Labels: labels.FromStrings("a", "2")},
			}, nil),
			enc.Samples([]RefSample{
				{Ref: 123, T: 5000, V: 1},
				{Ref: 124, T: 15000, V: 1},
			}, nil),
		)
		testutil.Ok(t, err)
		testutil.Ok(t, w.Close())

		db, err := Open(dir, nil, nil, nil)
		testutil.Ok(t, err)

		testutil.Equals(t, int64(5000), db.head.MinTime())
		testutil.Equals(t, int64(15000), db.head.MaxTime())
	})
	t.Run("existing-block", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "test_head_init")
		testutil.Ok(t, err)
		defer os.RemoveAll(dir)

		createBlock(t, dir, genSeries(1, 1, 1000, 2000))

		db, err := Open(dir, nil, nil, nil)
		testutil.Ok(t, err)

		testutil.Equals(t, int64(2000), db.head.MinTime())
		testutil.Equals(t, int64(2000), db.head.MaxTime())
	})
	t.Run("existing-block-and-wal", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "test_head_init")
		testutil.Ok(t, err)
		defer os.RemoveAll(dir)

		createBlock(t, dir, genSeries(1, 1, 1000, 6000))

		testutil.Ok(t, os.MkdirAll(path.Join(dir, "wal"), 0777))
		w, err := wal.New(nil, nil, path.Join(dir, "wal"))
		testutil.Ok(t, err)

		var enc RecordEncoder
		err = w.Log(
			enc.Series([]RefSeries{
				{Ref: 123, Labels: labels.FromStrings("a", "1")},
				{Ref: 124, Labels: labels.FromStrings("a", "2")},
			}, nil),
			enc.Samples([]RefSample{
				{Ref: 123, T: 5000, V: 1},
				{Ref: 124, T: 15000, V: 1},
			}, nil),
		)
		testutil.Ok(t, err)
		testutil.Ok(t, w.Close())

		r := prometheus.NewRegistry()

		db, err := Open(dir, nil, r, nil)
		testutil.Ok(t, err)

		testutil.Equals(t, int64(6000), db.head.MinTime())
		testutil.Equals(t, int64(15000), db.head.MaxTime())
		// Check that old series has been GCed.
		testutil.Equals(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.series))
	})
}

func TestNoEmptyBlocks(t *testing.T) {
	db, delete := openTestDB(t, &Options{
		BlockRanges: []int64{100},
	})
	defer func() {
		testutil.Ok(t, db.Close())
		delete()
	}()
	db.DisableCompactions()

	rangeToTriggerCompaction := db.opts.BlockRanges[0]/2*3 - 1
	defaultLabel := labels.FromStrings("foo", "bar")
	defaultMatcher := labels.NewMustRegexpMatcher("", ".*")

	t.Run("Test no blocks after compact with empty head.", func(t *testing.T) {
		testutil.Ok(t, db.compact())
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
		testutil.Ok(t, db.compact())
		testutil.Equals(t, 1, int(prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.ran)), "compaction should have been triggered here")

		actBlocks, err := blockDirs(db.Dir())
		testutil.Ok(t, err)
		testutil.Equals(t, len(db.Blocks()), len(actBlocks))
		testutil.Equals(t, 0, len(actBlocks))

		app = db.Appender()
		_, err = app.Add(defaultLabel, 1, 0)
		testutil.Assert(t, err == ErrOutOfBounds, "the head should be truncated so no samples in the past should be allowed")

		// Adding new blocks.
		currentTime := db.Head().MaxTime()
		_, err = app.Add(defaultLabel, currentTime, 0)
		testutil.Ok(t, err)
		_, err = app.Add(defaultLabel, currentTime+1, 0)
		testutil.Ok(t, err)
		_, err = app.Add(defaultLabel, currentTime+rangeToTriggerCompaction, 0)
		testutil.Ok(t, err)
		testutil.Ok(t, app.Commit())

		testutil.Ok(t, db.compact())
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
		testutil.Ok(t, db.compact())
		testutil.Equals(t, 3, int(prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.ran)), "compaction should have been triggered here")
		testutil.Equals(t, oldBlocks, db.Blocks())
	})

	t.Run("Test no blocks remaining after deleting all samples from disk.", func(t *testing.T) {
		currentTime := db.Head().MaxTime()
		blocks := []*BlockMeta{
			{MinTime: currentTime, MaxTime: currentTime + db.opts.BlockRanges[0]},
			{MinTime: currentTime + 100, MaxTime: currentTime + 100 + db.opts.BlockRanges[0]},
		}
		for _, m := range blocks {
			createBlock(t, db.Dir(), genSeries(2, 2, m.MinTime, m.MaxTime))
		}

		oldBlocks := db.Blocks()
		testutil.Ok(t, db.reload())                                      // Reload the db to register the new blocks.
		testutil.Equals(t, len(blocks)+len(oldBlocks), len(db.Blocks())) // Ensure all blocks are registered.
		testutil.Ok(t, db.Delete(math.MinInt64, math.MaxInt64, defaultMatcher))
		testutil.Ok(t, db.compact())
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
				[2]string{"name1", ""},
				[2]string{"name3", ""},
				[2]string{"name2", ""},
			},
			sampleLabels2: [][2]string{
				[2]string{"name4", ""},
				[2]string{"name1", ""},
			},
			exp1: []string{"name1", "name2", "name3"},
			exp2: []string{"name1", "name2", "name3", "name4"},
		},
		{
			sampleLabels1: [][2]string{
				[2]string{"name2", ""},
				[2]string{"name1", ""},
				[2]string{"name2", ""},
			},
			sampleLabels2: [][2]string{
				[2]string{"name6", ""},
				[2]string{"name0", ""},
			},
			exp1: []string{"name1", "name2"},
			exp2: []string{"name0", "name1", "name2", "name6"},
		},
	}

	blockRange := DefaultOptions.BlockRanges[0]
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
		db, delete := openTestDB(t, nil)
		defer func() {
			testutil.Ok(t, db.Close())
			delete()
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
		err = db.compact()
		testutil.Ok(t, err)
		// All blocks have same label names, hence check them individually.
		// No need to aggregrate and check.
		for _, b := range db.Blocks() {
			blockIndexr, err := b.Index()
			testutil.Ok(t, err)
			labelNames, err = blockIndexr.LabelNames()
			testutil.Ok(t, err)
			testutil.Equals(t, tst.exp1, labelNames)
			testutil.Ok(t, blockIndexr.Close())
		}

		// Addings more samples to head with new label names
		// so that we can test (head+disk).LabelNames() (the union).
		appendSamples(db, 5, 9, tst.sampleLabels2)

		// Testing DB (union).
		q, err := db.Querier(math.MinInt64, math.MaxInt64)
		testutil.Ok(t, err)
		labelNames, err = q.LabelNames()
		testutil.Ok(t, err)
		testutil.Ok(t, q.Close())
		testutil.Equals(t, tst.exp2, labelNames)
	}
}

func TestCorrectNumTombstones(t *testing.T) {
	db, delete := openTestDB(t, nil)
	defer func() {
		testutil.Ok(t, db.Close())
		delete()
	}()

	blockRange := DefaultOptions.BlockRanges[0]
	defaultLabel := labels.FromStrings("foo", "bar")
	defaultMatcher := labels.NewEqualMatcher(defaultLabel[0].Name, defaultLabel[0].Value)

	app := db.Appender()
	for i := int64(0); i < 3; i++ {
		for j := int64(0); j < 15; j++ {
			_, err := app.Add(defaultLabel, i*blockRange+j, 0)
			testutil.Ok(t, err)
		}
	}
	testutil.Ok(t, app.Commit())

	err := db.compact()
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
		blockSeries [][]Series
		expSeries   map[string][]tsdbutil.Sample
	}{
		// Case 0
		// |--------------|
		//        |----------------|
		{
			blockSeries: [][]Series{
				[]Series{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{4, 0},
						sample{5, 0}, sample{7, 0}, sample{8, 0}, sample{9, 0},
					}),
				},
				[]Series{
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
		},
		// Case 1
		// |-------------------------------|
		//        |----------------|
		{
			blockSeries: [][]Series{
				[]Series{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{4, 0},
						sample{5, 0}, sample{7, 0}, sample{8, 0}, sample{9, 0},
						sample{11, 0}, sample{13, 0}, sample{17, 0},
					}),
				},
				[]Series{
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
		},
		// Case 2
		// |-------------------------------|
		//        |------------|
		//                           |--------------------|
		{
			blockSeries: [][]Series{
				[]Series{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{4, 0},
						sample{5, 0}, sample{7, 0}, sample{8, 0}, sample{9, 0},
						sample{11, 0}, sample{13, 0}, sample{17, 0},
					}),
				},
				[]Series{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{3, 99}, sample{5, 99}, sample{6, 99}, sample{7, 99},
						sample{8, 99}, sample{9, 99},
					}),
				},
				[]Series{
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
		},
		// Case 3
		// |-------------------|
		//                           |--------------------|
		//               |----------------|
		{
			blockSeries: [][]Series{
				[]Series{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{4, 0},
						sample{5, 0}, sample{8, 0}, sample{9, 0},
					}),
				},
				[]Series{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{14, 59}, sample{15, 59}, sample{17, 59}, sample{20, 59},
						sample{21, 59}, sample{22, 59},
					}),
				},
				[]Series{
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
		},
		// Case 4
		// |-------------------------------------|
		//            |------------|
		//      |-------------------------|
		{
			blockSeries: [][]Series{
				[]Series{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{0, 0}, sample{1, 0}, sample{2, 0}, sample{4, 0},
						sample{5, 0}, sample{8, 0}, sample{9, 0}, sample{10, 0},
						sample{13, 0}, sample{15, 0}, sample{16, 0}, sample{17, 0},
						sample{20, 0}, sample{22, 0},
					}),
				},
				[]Series{
					newSeries(map[string]string{"a": "b"}, []tsdbutil.Sample{
						sample{7, 59}, sample{8, 59}, sample{9, 59}, sample{10, 59},
						sample{11, 59},
					}),
				},
				[]Series{
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
		},
		// Case 5: series are merged properly when there are multiple series.
		// |-------------------------------------|
		//            |------------|
		//      |-------------------------|
		{
			blockSeries: [][]Series{
				[]Series{
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
				[]Series{
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
				[]Series{
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
		},
	}

	defaultMatcher := labels.NewMustRegexpMatcher("__name__", ".*")
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
			opts := *DefaultOptions
			opts.AllowOverlappingBlocks = true
			db, err := Open(tmpdir, nil, nil, &opts)
			testutil.Ok(t, err)
			defer func() {
				testutil.Ok(t, db.Close())
			}()
			db.DisableCompactions()
			testutil.Assert(t, len(db.blocks) == len(c.blockSeries), "Wrong number of blocks [before compact].")

			// Vertical Query Merging test.
			querier, err := db.Querier(0, 100)
			testutil.Ok(t, err)
			actSeries := query(t, querier, defaultMatcher)
			testutil.Equals(t, c.expSeries, actSeries)

			// Vertical compaction.
			lc := db.compactor.(*LeveledCompactor)
			testutil.Equals(t, 0, int(prom_testutil.ToFloat64(lc.metrics.overlappingBlocks)), "overlapping blocks count should be still 0 here")
			err = db.compact()
			testutil.Ok(t, err)
			testutil.Equals(t, 1, len(db.Blocks()), "Wrong number of blocks [after compact]")

			testutil.Equals(t, 1, int(prom_testutil.ToFloat64(lc.metrics.overlappingBlocks)), "overlapping blocks count mismatch")

			// Query test after merging the overlapping blocks.
			querier, err = db.Querier(0, 100)
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
	if err != nil {
		t.Fatalf("Opening test dir failed: %s", err)
	}

	rangeToTriggercompaction := DefaultOptions.BlockRanges[0]/2*3 + 1

	// Test that the compactor doesn't create overlapping blocks
	// when a non standard block already exists.
	firstBlockMaxT := int64(3)
	createBlock(t, dir, genSeries(1, 1, 0, firstBlockMaxT))
	db, err := Open(dir, logger, nil, DefaultOptions)
	if err != nil {
		t.Fatalf("Opening test storage failed: %s", err)
	}
	defer func() {
		os.RemoveAll(dir)
	}()
	app := db.Appender()
	lbl := labels.Labels{{"a", "b"}}
	_, err = app.Add(lbl, firstBlockMaxT-1, rand.Float64())
	if err == nil {
		t.Fatalf("appending a sample with a timestamp covered by a previous block shouldn't be possible")
	}
	_, err = app.Add(lbl, firstBlockMaxT+1, rand.Float64())
	testutil.Ok(t, err)
	_, err = app.Add(lbl, firstBlockMaxT+2, rand.Float64())
	testutil.Ok(t, err)
	secondBlockMaxt := firstBlockMaxT + rangeToTriggercompaction
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

	db, err = Open(dir, logger, nil, DefaultOptions)
	if err != nil {
		t.Fatalf("Opening test storage failed: %s", err)
	}
	defer db.Close()
	testutil.Equals(t, 3, len(db.Blocks()), "db doesn't include expected number of blocks")
	testutil.Equals(t, db.Blocks()[2].Meta().MaxTime, thirdBlockMaxt, "unexpected maxt of the last block")

	app = db.Appender()
	_, err = app.Add(lbl, thirdBlockMaxt+rangeToTriggercompaction, rand.Float64()) // Trigger a compaction
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
