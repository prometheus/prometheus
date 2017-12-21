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
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"sort"
	"testing"

	"github.com/pkg/errors"
	"github.com/prometheus/tsdb/labels"
	"github.com/prometheus/tsdb/testutil"
)

func openTestDB(t testing.TB, opts *Options) (db *DB, close func()) {
	tmpdir, err := ioutil.TempDir("", "test")
	testutil.Ok(t, err)

	db, err = Open(tmpdir, nil, nil, opts)
	testutil.Ok(t, err)

	// Do not close the test database by default as it will deadlock on test failures.
	return db, func() { os.RemoveAll(tmpdir) }
}

// query runs a matcher query against the querier and fully expands its data.
func query(t testing.TB, q Querier, matchers ...labels.Matcher) map[string][]sample {
	ss, err := q.Select(matchers...)
	testutil.Ok(t, err)

	result := map[string][]sample{}

	for ss.Next() {
		series := ss.At()

		samples := []sample{}
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

func TestDataAvailableOnlyAfterCommit(t *testing.T) {
	db, close := openTestDB(t, nil)
	defer close()
	defer db.Close()

	app := db.Appender()

	_, err := app.Add(labels.FromStrings("foo", "bar"), 0, 0)
	testutil.Ok(t, err)

	querier, err := db.Querier(0, 1)
	testutil.Ok(t, err)
	seriesSet := query(t, querier, labels.NewEqualMatcher("foo", "bar"))

	testutil.Equals(t, seriesSet, map[string][]sample{})
	testutil.Ok(t, querier.Close())

	err = app.Commit()
	testutil.Ok(t, err)

	querier, err = db.Querier(0, 1)
	testutil.Ok(t, err)
	defer querier.Close()

	seriesSet = query(t, querier, labels.NewEqualMatcher("foo", "bar"))

	testutil.Equals(t, seriesSet, map[string][]sample{`{foo="bar"}`: []sample{{t: 0, v: 0}}})
}

func TestDataNotAvailableAfterRollback(t *testing.T) {
	db, close := openTestDB(t, nil)
	defer close()
	defer db.Close()

	app := db.Appender()
	_, err := app.Add(labels.FromStrings("foo", "bar"), 0, 0)
	testutil.Ok(t, err)

	err = app.Rollback()
	testutil.Ok(t, err)

	querier, err := db.Querier(0, 1)
	testutil.Ok(t, err)
	defer querier.Close()

	seriesSet := query(t, querier, labels.NewEqualMatcher("foo", "bar"))

	testutil.Equals(t, seriesSet, map[string][]sample{})
}

func TestDBAppenderAddRef(t *testing.T) {
	db, close := openTestDB(t, nil)
	defer close()
	defer db.Close()

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
	testutil.Equals(t, errors.Cause(err), ErrNotFound)

	testutil.Ok(t, app2.Commit())

	q, err := db.Querier(0, 200)
	testutil.Ok(t, err)

	res := query(t, q, labels.NewEqualMatcher("a", "b"))

	testutil.Equals(t, map[string][]sample{
		labels.FromStrings("a", "b").String(): []sample{
			{t: 123, v: 0},
			{t: 124, v: 1},
			{t: 125, v: 0},
			{t: 133, v: 1},
			{t: 143, v: 2},
		},
	}, res)

	testutil.Ok(t, q.Close())
}

func TestDeleteSimple(t *testing.T) {
	numSamples := int64(10)

	db, close := openTestDB(t, nil)
	defer close()
	defer db.Close()

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

		// Compare the result.
		q, err := db.Querier(0, numSamples)
		testutil.Ok(t, err)

		res, err := q.Select(labels.NewEqualMatcher("a", "b"))
		testutil.Ok(t, err)

		expSamples := make([]sample, 0, len(c.remaint))
		for _, ts := range c.remaint {
			expSamples = append(expSamples, sample{ts, smpls[ts]})
		}

		expss := newListSeriesSet([]Series{
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

func TestAmendDatapointCausesError(t *testing.T) {
	db, close := openTestDB(t, nil)
	defer close()
	defer db.Close()

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
	db, close := openTestDB(t, nil)
	defer close()
	defer db.Close()

	app := db.Appender()
	_, err := app.Add(labels.Labels{}, 0, math.NaN())
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	app = db.Appender()
	_, err = app.Add(labels.Labels{}, 0, math.NaN())
	testutil.Ok(t, err)
}

func TestNonDuplicateNaNDatapointsCausesAmendError(t *testing.T) {
	db, close := openTestDB(t, nil)
	defer close()
	defer db.Close()

	app := db.Appender()
	_, err := app.Add(labels.Labels{}, 0, math.Float64frombits(0x7ff0000000000001))
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	app = db.Appender()
	_, err = app.Add(labels.Labels{}, 0, math.Float64frombits(0x7ff0000000000002))
	testutil.Equals(t, ErrAmendSample, err)
}

func TestSkippingInvalidValuesInSameTxn(t *testing.T) {
	db, close := openTestDB(t, nil)
	defer close()
	defer db.Close()

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

	testutil.Equals(t, map[string][]sample{
		labels.New(labels.Label{"a", "b"}).String(): []sample{{0, 1}},
	}, ssMap)

	testutil.Ok(t, q.Close())

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

	testutil.Equals(t, map[string][]sample{
		labels.New(labels.Label{"a", "b"}).String(): []sample{{0, 1}, {10, 3}},
	}, ssMap)
	testutil.Ok(t, q.Close())
}

func TestDB_Snapshot(t *testing.T) {
	db, close := openTestDB(t, nil)
	defer close()

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
	testutil.Ok(t, db.Snapshot(snap))
	testutil.Ok(t, db.Close())

	// reopen DB from snapshot
	db, err = Open(snap, nil, nil, nil)
	testutil.Ok(t, err)
	defer db.Close()

	querier, err := db.Querier(mint, mint+1000)
	testutil.Ok(t, err)
	defer querier.Close()

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
	testutil.Equals(t, sum, 1000.0)
}

func TestDB_SnapshotWithDelete(t *testing.T) {
	numSamples := int64(10)

	db, close := openTestDB(t, nil)
	defer close()

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
		testutil.Ok(t, db.Snapshot(snap))
		testutil.Ok(t, db.Close())

		// reopen DB from snapshot
		db, err = Open(snap, nil, nil, nil)
		testutil.Ok(t, err)
		defer db.Close()

		// Compare the result.
		q, err := db.Querier(0, numSamples)
		testutil.Ok(t, err)
		defer q.Close()

		res, err := q.Select(labels.NewEqualMatcher("a", "b"))
		testutil.Ok(t, err)

		expSamples := make([]sample, 0, len(c.remaint))
		for _, ts := range c.remaint {
			expSamples = append(expSamples, sample{ts, smpls[ts]})
		}

		expss := newListSeriesSet([]Series{
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
		maxTime       = int64(2 * 1000)
		minTime       = int64(200)
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

	seriesMap := map[string][]sample{}
	for _, l := range lbls {
		seriesMap[labels.New(l...).String()] = []sample{}
	}

	db, close := openTestDB(t, nil)
	defer close()
	defer db.Close()

	app := db.Appender()

	for _, l := range lbls {
		lset := labels.New(l...)
		series := []sample{}

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

			expected := map[string][]sample{}

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

			result := map[string][]sample{}

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

	return
}

func TestWALFlushedOnDBClose(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "test")
	testutil.Ok(t, err)
	defer os.RemoveAll(tmpdir)

	db, err := Open(tmpdir, nil, nil, nil)
	testutil.Ok(t, err)

	lbls := labels.Labels{labels.Label{Name: "labelname", Value: "labelvalue"}}

	app := db.Appender()
	_, err = app.Add(lbls, 0, 1)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	testutil.Ok(t, db.Close())

	db, err = Open(tmpdir, nil, nil, nil)
	testutil.Ok(t, err)
	defer db.Close()

	q, err := db.Querier(0, 1)
	testutil.Ok(t, err)

	values, err := q.LabelValues("labelname")
	testutil.Ok(t, err)
	testutil.Equals(t, values, []string{"labelvalue"})
}

func TestTombstoneClean(t *testing.T) {
	numSamples := int64(10)

	db, close := openTestDB(t, nil)
	defer close()

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
		testutil.Ok(t, db.Snapshot(snap))
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

		expSamples := make([]sample, 0, len(c.remaint))
		for _, ts := range c.remaint {
			expSamples = append(expSamples, sample{ts, smpls[ts]})
		}

		expss := newListSeriesSet([]Series{
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

		for _, b := range db.blocks {
			testutil.Equals(t, emptyTombstoneReader, b.tombstones)
		}
	}
}

func TestDB_Retention(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpdir)

	db, err := Open(tmpdir, nil, nil, nil)
	testutil.Ok(t, err)

	lbls := labels.Labels{labels.Label{Name: "labelname", Value: "labelvalue"}}

	app := db.Appender()
	_, err = app.Add(lbls, 0, 1)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	// create snapshot to make it create a block.
	// TODO(gouthamve): Add a method to compact headblock.
	snap, err := ioutil.TempDir("", "snap")
	testutil.Ok(t, err)
	testutil.Ok(t, db.Snapshot(snap))
	testutil.Ok(t, db.Close())
	defer os.RemoveAll(snap)

	// reopen DB from snapshot
	db, err = Open(snap, nil, nil, nil)
	testutil.Ok(t, err)

	testutil.Equals(t, 1, len(db.blocks))

	app = db.Appender()
	_, err = app.Add(lbls, 100, 1)
	testutil.Ok(t, err)
	testutil.Ok(t, app.Commit())

	// Snapshot again to create another block.
	snap, err = ioutil.TempDir("", "snap")
	testutil.Ok(t, err)
	testutil.Ok(t, db.Snapshot(snap))
	testutil.Ok(t, db.Close())
	defer os.RemoveAll(snap)

	// reopen DB from snapshot
	db, err = Open(snap, nil, nil, &Options{
		RetentionDuration: 10,
		BlockRanges:       []int64{50},
	})
	testutil.Ok(t, err)
	defer db.Close()

	testutil.Equals(t, 2, len(db.blocks))

	// Now call rentention.
	changes, err := db.retentionCutoff()
	testutil.Ok(t, err)
	testutil.Assert(t, changes, "there should be changes")
	testutil.Equals(t, 1, len(db.blocks))
	testutil.Equals(t, int64(100), db.blocks[0].meta.MaxTime) // To verify its the right block.
}

func TestNotMatcherSelectsLabelsUnsetSeries(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpdir)

	db, err := Open(tmpdir, nil, nil, nil)
	testutil.Ok(t, err)
	defer db.Close()

	labelpairs := []labels.Labels{
		labels.FromStrings("a", "abcd", "b", "abcde"),
		labels.FromStrings("labelname", "labelvalue"),
	}

	app := db.Appender()
	for _, lbls := range labelpairs {
		_, err = app.Add(lbls, 0, 1)
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
	defer q.Close()

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
