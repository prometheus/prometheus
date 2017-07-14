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
	"math/rand"
	"os"
	"testing"

	"github.com/pkg/errors"
	"github.com/prometheus/tsdb/labels"
	"github.com/stretchr/testify/require"
)

// Convert a SeriesSet into a form useable with reflect.DeepEqual.
func readSeriesSet(ss SeriesSet) (map[string][]sample, error) {
	result := map[string][]sample{}

	for ss.Next() {
		series := ss.At()

		samples := []sample{}
		it := series.Iterator()
		for it.Next() {
			t, v := it.At()
			samples = append(samples, sample{t: t, v: v})
		}

		name := series.Labels().String()
		result[name] = samples
		if err := ss.Err(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func TestDataAvailableOnlyAfterCommit(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpdir)

	db, err := Open(tmpdir, nil, nil, nil)
	require.NoError(t, err)
	defer db.Close()

	app := db.Appender()
	_, err = app.Add(labels.FromStrings("foo", "bar"), 0, 0)
	require.NoError(t, err)

	querier := db.Querier(0, 1)
	seriesSet, err := readSeriesSet(querier.Select(labels.NewEqualMatcher("foo", "bar")))
	require.NoError(t, err)
	require.Equal(t, seriesSet, map[string][]sample{})
	require.NoError(t, querier.Close())

	err = app.Commit()
	require.NoError(t, err)

	querier = db.Querier(0, 1)
	defer querier.Close()

	seriesSet, err = readSeriesSet(querier.Select(labels.NewEqualMatcher("foo", "bar")))
	require.NoError(t, err)
	require.Equal(t, seriesSet, map[string][]sample{`{foo="bar"}`: []sample{{t: 0, v: 0}}})
}

func TestDataNotAvailableAfterRollback(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpdir)

	db, err := Open(tmpdir, nil, nil, nil)
	if err != nil {
		t.Fatalf("Error opening database: %q", err)
	}
	defer db.Close()

	app := db.Appender()
	_, err = app.Add(labels.FromStrings("foo", "bar"), 0, 0)
	require.NoError(t, err)

	err = app.Rollback()
	require.NoError(t, err)

	querier := db.Querier(0, 1)
	defer querier.Close()

	seriesSet, err := readSeriesSet(querier.Select(labels.NewEqualMatcher("foo", "bar")))
	require.NoError(t, err)
	require.Equal(t, seriesSet, map[string][]sample{})
}

func TestDBAppenderAddRef(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpdir)

	db, err := Open(tmpdir, nil, nil, nil)
	require.NoError(t, err)
	defer db.Close()

	app1 := db.Appender()

	ref, err := app1.Add(labels.FromStrings("a", "b"), 0, 0)
	require.NoError(t, err)

	// When a series is first created, refs don't work within that transaction.
	err = app1.AddFast(ref, 1, 1)
	require.EqualError(t, errors.Cause(err), ErrNotFound.Error())

	err = app1.Commit()
	require.NoError(t, err)

	app2 := db.Appender()
	defer app2.Rollback()

	ref, err = app2.Add(labels.FromStrings("a", "b"), 1, 1)
	require.NoError(t, err)

	// Ref must be prefixed with block ULID of the block we wrote to.
	id := db.blocks[len(db.blocks)-1].Meta().ULID
	require.Equal(t, string(id[:]), ref[:16])

	// Reference must be valid to add another sample.
	err = app2.AddFast(ref, 2, 2)
	require.NoError(t, err)

	// AddFast for the same timestamp must fail if the generation in the reference
	// doesn't add up.
	refb := []byte(ref)
	refb[15] ^= refb[15]
	err = app2.AddFast(string(refb), 1, 1)
	require.EqualError(t, errors.Cause(err), ErrNotFound.Error())
}

func TestDeleteSimple(t *testing.T) {
	numSamples := int64(10)

	tmpdir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpdir)

	db, err := Open(tmpdir, nil, nil, nil)
	require.NoError(t, err)
	app := db.Appender()

	smpls := make([]float64, numSamples)
	for i := int64(0); i < numSamples; i++ {
		smpls[i] = rand.Float64()
		app.Add(labels.Labels{{"a", "b"}}, i, smpls[i])
	}

	require.NoError(t, app.Commit())
	cases := []struct {
		intervals intervals
		remaint   []int64
	}{
		{
			intervals: intervals{{1, 3}, {4, 7}},
			remaint:   []int64{0, 8, 9},
		},
	}

Outer:
	for _, c := range cases {
		// TODO(gouthamve): Reset the tombstones somehow.
		// Delete the ranges.
		for _, r := range c.intervals {
			require.NoError(t, db.Delete(r.mint, r.maxt, labels.NewEqualMatcher("a", "b")))
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
