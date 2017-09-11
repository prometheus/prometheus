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
	"github.com/stretchr/testify/require"
)

func openTestDB(t testing.TB, opts *Options) (db *DB, close func()) {
	tmpdir, _ := ioutil.TempDir("", "test")

	db, err := Open(tmpdir, nil, nil, opts)
	require.NoError(t, err)

	// Do not close the test database by default as it will deadlock on test failures.
	return db, func() { os.RemoveAll(tmpdir) }
}

// Convert a SeriesSet into a form useable with reflect.DeepEqual.
func readSeriesSet(t testing.TB, ss SeriesSet) map[string][]sample {
	result := map[string][]sample{}

	for ss.Next() {
		series := ss.At()

		samples := []sample{}
		it := series.Iterator()
		for it.Next() {
			t, v := it.At()
			samples = append(samples, sample{t: t, v: v})
		}
		require.NoError(t, it.Err())

		name := series.Labels().String()
		result[name] = samples
	}
	require.NoError(t, ss.Err())

	return result
}

func TestDataAvailableOnlyAfterCommit(t *testing.T) {
	db, close := openTestDB(t, nil)
	defer close()

	app := db.Appender()

	_, err := app.Add(labels.FromStrings("foo", "bar"), 0, 0)
	require.NoError(t, err)

	querier := db.Querier(0, 1)
	seriesSet := readSeriesSet(t, querier.Select(labels.NewEqualMatcher("foo", "bar")))

	require.Equal(t, seriesSet, map[string][]sample{})
	require.NoError(t, querier.Close())

	err = app.Commit()
	require.NoError(t, err)

	querier = db.Querier(0, 1)
	defer querier.Close()

	seriesSet = readSeriesSet(t, querier.Select(labels.NewEqualMatcher("foo", "bar")))

	require.Equal(t, seriesSet, map[string][]sample{`{foo="bar"}`: []sample{{t: 0, v: 0}}})
}

func TestDataNotAvailableAfterRollback(t *testing.T) {
	db, close := openTestDB(t, nil)
	defer close()

	app := db.Appender()
	_, err := app.Add(labels.FromStrings("foo", "bar"), 0, 0)
	require.NoError(t, err)

	err = app.Rollback()
	require.NoError(t, err)

	querier := db.Querier(0, 1)
	defer querier.Close()

	seriesSet := readSeriesSet(t, querier.Select(labels.NewEqualMatcher("foo", "bar")))

	require.Equal(t, seriesSet, map[string][]sample{})
}

func TestDBAppenderAddRef(t *testing.T) {
	db, close := openTestDB(t, nil)
	defer close()

	app1 := db.Appender()

	ref1, err := app1.Add(labels.FromStrings("a", "b"), 123, 0)
	require.NoError(t, err)

	// Reference should already work before commit.
	err = app1.AddFast(ref1, 124, 1)
	require.NoError(t, err)

	err = app1.Commit()
	require.NoError(t, err)

	app2 := db.Appender()

	// first ref should already work in next transaction.
	err = app2.AddFast(ref1, 125, 0)
	require.NoError(t, err)

	ref2, err := app2.Add(labels.FromStrings("a", "b"), 133, 1)
	require.NoError(t, err)

	require.True(t, ref1 == ref2)

	// Reference must be valid to add another sample.
	err = app2.AddFast(ref2, 143, 2)
	require.NoError(t, err)

	err = app2.AddFast(9999999, 1, 1)
	require.EqualError(t, errors.Cause(err), ErrNotFound.Error())

	require.NoError(t, app2.Commit())

	q := db.Querier(0, 200)
	res := readSeriesSet(t, q.Select(labels.NewEqualMatcher("a", "b")))

	require.Equal(t, map[string][]sample{
		labels.FromStrings("a", "b").String(): []sample{
			{t: 123, v: 0},
			{t: 124, v: 1},
			{t: 125, v: 0},
			{t: 133, v: 1},
			{t: 143, v: 2},
		},
	}, res)

	require.NoError(t, q.Close())
}

func TestDeleteSimple(t *testing.T) {
	numSamples := int64(10)

	db, close := openTestDB(t, nil)
	defer close()

	app := db.Appender()

	smpls := make([]float64, numSamples)
	for i := int64(0); i < numSamples; i++ {
		smpls[i] = rand.Float64()
		app.Add(labels.Labels{{"a", "b"}}, i, smpls[i])
	}

	require.NoError(t, app.Commit())
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
			require.NoError(t, db.Delete(r.Mint, r.Maxt, labels.NewEqualMatcher("a", "b")))
		}

		// Compare the result.
		q := db.Querier(0, numSamples)
		res := q.Select(labels.NewEqualMatcher("a", "b"))

		expSamples := make([]sample, 0, len(c.remaint))
		for _, ts := range c.remaint {
			expSamples = append(expSamples, sample{ts, smpls[ts]})
		}

		expss := newListSeriesSet([]Series{
			newSeries(map[string]string{"a": "b"}, expSamples),
		})

		if len(expSamples) == 0 {
			require.False(t, res.Next())
			continue
		}

		for {
			eok, rok := expss.Next(), res.Next()
			require.Equal(t, eok, rok, "next")

			if !eok {
				continue Outer
			}
			sexp := expss.At()
			sres := res.At()

			require.Equal(t, sexp.Labels(), sres.Labels(), "labels")

			smplExp, errExp := expandSeriesIterator(sexp.Iterator())
			smplRes, errRes := expandSeriesIterator(sres.Iterator())

			require.Equal(t, errExp, errRes, "samples error")
			require.Equal(t, smplExp, smplRes, "samples")
		}
	}
}

func TestAmendDatapointCausesError(t *testing.T) {
	db, close := openTestDB(t, nil)
	defer close()

	app := db.Appender()
	_, err := app.Add(labels.Labels{}, 0, 0)
	require.NoError(t, err, "Failed to add sample")
	require.NoError(t, app.Commit(), "Unexpected error committing appender")

	app = db.Appender()
	_, err = app.Add(labels.Labels{}, 0, 1)
	require.Equal(t, ErrAmendSample, err)
	require.NoError(t, app.Rollback(), "Unexpected error rolling back appender")
}

func TestDuplicateNaNDatapointNoAmendError(t *testing.T) {
	db, close := openTestDB(t, nil)
	defer close()

	app := db.Appender()
	_, err := app.Add(labels.Labels{}, 0, math.NaN())
	require.NoError(t, err, "Failed to add sample")
	require.NoError(t, app.Commit(), "Unexpected error committing appender")

	app = db.Appender()
	_, err = app.Add(labels.Labels{}, 0, math.NaN())
	require.NoError(t, err)
}

func TestNonDuplicateNaNDatapointsCausesAmendError(t *testing.T) {
	db, close := openTestDB(t, nil)
	defer close()

	app := db.Appender()
	_, err := app.Add(labels.Labels{}, 0, math.Float64frombits(0x7ff0000000000001))
	require.NoError(t, err, "Failed to add sample")
	require.NoError(t, app.Commit(), "Unexpected error committing appender")

	app = db.Appender()
	_, err = app.Add(labels.Labels{}, 0, math.Float64frombits(0x7ff0000000000002))
	require.Equal(t, ErrAmendSample, err)
}

func TestSkippingInvalidValuesInSameTxn(t *testing.T) {
	db, close := openTestDB(t, nil)
	defer close()

	// Append AmendedValue.
	app := db.Appender()
	_, err := app.Add(labels.Labels{{"a", "b"}}, 0, 1)
	require.NoError(t, err)
	_, err = app.Add(labels.Labels{{"a", "b"}}, 0, 2)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Make sure the right value is stored.
	q := db.Querier(0, 10)
	ss := q.Select(labels.NewEqualMatcher("a", "b"))
	ssMap := readSeriesSet(t, ss)

	require.Equal(t, map[string][]sample{
		labels.New(labels.Label{"a", "b"}).String(): []sample{{0, 1}},
	}, ssMap)

	require.NoError(t, q.Close())

	// Append Out of Order Value.
	app = db.Appender()
	_, err = app.Add(labels.Labels{{"a", "b"}}, 10, 3)
	require.NoError(t, err)
	_, err = app.Add(labels.Labels{{"a", "b"}}, 7, 5)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	q = db.Querier(0, 10)
	ss = q.Select(labels.NewEqualMatcher("a", "b"))
	ssMap = readSeriesSet(t, ss)

	require.Equal(t, map[string][]sample{
		labels.New(labels.Label{"a", "b"}).String(): []sample{{0, 1}, {10, 3}},
	}, ssMap)
	require.NoError(t, q.Close())
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

	app := db.Appender()

	for _, l := range lbls {
		lset := labels.New(l...)
		series := []sample{}

		ts := rand.Int63n(300)
		for i := 0; i < numDatapoints; i++ {
			v := rand.Float64()

			series = append(series, sample{ts, v})

			_, err := app.Add(lset, ts, v)
			require.NoError(t, err)

			ts += rand.Int63n(timeInterval) + 1
		}

		seriesMap[lset.String()] = series
	}

	require.NoError(t, app.Commit())

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

			q := db.Querier(mint, maxt)
			ss := q.Select(qry.ms...)

			result := map[string][]sample{}

			for ss.Next() {
				x := ss.At()

				smpls, err := expandSeriesIterator(x.Iterator())
				require.NoError(t, err)

				if len(smpls) > 0 {
					result[x.Labels().String()] = smpls
				}
			}

			require.NoError(t, ss.Err())
			require.Equal(t, expected, result)

			q.Close()
		}
	}

	return
}
