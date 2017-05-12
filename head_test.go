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
	"unsafe"

	"github.com/pkg/errors"
	"github.com/prometheus/tsdb/labels"

	promlabels "github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/textparse"
	"github.com/stretchr/testify/require"
)

func BenchmarkCreateSeries(b *testing.B) {
	lbls, err := readPrometheusLabels("cmd/tsdb/testdata.1m", 1e6)
	require.NoError(b, err)

	b.Run("", func(b *testing.B) {
		dir, err := ioutil.TempDir("", "create_series_bench")
		require.NoError(b, err)
		defer os.RemoveAll(dir)

		h, err := CreateHeadBlock(dir, 0, nil, 0, 1)
		require.NoError(b, err)

		b.ReportAllocs()
		b.ResetTimer()

		for _, l := range lbls[:b.N] {
			h.create(l.Hash(), l)
		}
	})
}

func readPrometheusLabels(fn string, n int) ([]labels.Labels, error) {
	f, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	p := textparse.New(b)
	i := 0
	var mets []labels.Labels
	hashes := map[uint64]struct{}{}

	for p.Next() && i < n {
		m := make(labels.Labels, 0, 10)
		p.Metric((*promlabels.Labels)(unsafe.Pointer(&m)))

		h := m.Hash()
		if _, ok := hashes[h]; ok {
			continue
		}
		mets = append(mets, m)
		hashes[h] = struct{}{}
		i++
	}
	if err := p.Err(); err != nil {
		return nil, err
	}
	if i != n {
		return mets, errors.Errorf("requested %d metrics but found %d", n, i)
	}
	return mets, nil
}

func TestAmendDatapointCausesError(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpdir)

	hb, err := CreateHeadBlock(tmpdir+"/hb", 0, nil, 0, 1000)
	require.NoError(t, err, "Error creating head block")

	app := hb.Appender()
	_, err = app.Add(labels.Labels{}, 0, 0)
	require.NoError(t, err, "Failed to add sample")
	require.NoError(t, app.Commit(), "Unexpected error committing appender")

	app = hb.Appender()
	_, err = app.Add(labels.Labels{}, 0, 1)
	require.Equal(t, ErrAmendSample, err)
}

func TestDuplicateNaNDatapointNoAmendError(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpdir)

	hb, err := CreateHeadBlock(tmpdir+"/hb", 0, nil, 0, 1000)
	require.NoError(t, err, "Error creating head block")

	app := hb.Appender()
	_, err = app.Add(labels.Labels{}, 0, math.NaN())
	require.NoError(t, err, "Failed to add sample")
	require.NoError(t, app.Commit(), "Unexpected error committing appender")

	app = hb.Appender()
	_, err = app.Add(labels.Labels{}, 0, math.NaN())
	require.NoError(t, err)
}

func TestNonDuplicateNaNDatapointsCausesAmendError(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpdir)

	hb, err := CreateHeadBlock(tmpdir+"/hb", 0, nil, 0, 1000)
	require.NoError(t, err, "Error creating head block")

	app := hb.Appender()
	_, err = app.Add(labels.Labels{}, 0, math.Float64frombits(0x7ff0000000000001))
	require.NoError(t, err, "Failed to add sample")
	require.NoError(t, app.Commit(), "Unexpected error committing appender")

	app = hb.Appender()
	_, err = app.Add(labels.Labels{}, 0, math.Float64frombits(0x7ff0000000000002))
	require.Equal(t, ErrAmendSample, err)
}

func TestSkippingInvalidValuesInSameTxn(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpdir)

	hb, err := CreateHeadBlock(tmpdir+"/hb", 0, nil, 0, 1000)
	require.NoError(t, err)

	// Append AmendedValue.
	app := hb.Appender()
	_, err = app.Add(labels.Labels{{"a", "b"}}, 0, 1)
	require.NoError(t, err)
	_, err = app.Add(labels.Labels{{"a", "b"}}, 0, 2)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, uint64(1), hb.Meta().Stats.NumSamples)

	// Make sure the right value is stored.
	q := hb.Querier(0, 10)
	ss := q.Select(labels.NewEqualMatcher("a", "b"))
	ssMap, err := readSeriesSet(ss)
	require.NoError(t, err)

	require.Equal(t, map[string][]sample{
		labels.New(labels.Label{"a", "b"}).String(): []sample{{0, 1}},
	}, ssMap)

	require.NoError(t, q.Close())

	// Append Out of Order Value.
	app = hb.Appender()
	_, err = app.Add(labels.Labels{{"a", "b"}}, 10, 3)
	require.NoError(t, err)
	_, err = app.Add(labels.Labels{{"a", "b"}}, 7, 5)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, uint64(2), hb.Meta().Stats.NumSamples)

	q = hb.Querier(0, 10)
	ss = q.Select(labels.NewEqualMatcher("a", "b"))
	ssMap, err = readSeriesSet(ss)
	require.NoError(t, err)

	require.Equal(t, map[string][]sample{
		labels.New(labels.Label{"a", "b"}).String(): []sample{{0, 1}, {10, 3}},
	}, ssMap)
	require.NoError(t, q.Close())
}

func TestHeadBlock_e2e(t *testing.T) {
	numDatapoints := 1000
	numRanges := 1000
	timeInterval := int64(3)
	maxTime := int64(2 * 1000)
	minTime := int64(200)
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

	tmpdir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpdir)

	hb, err := CreateHeadBlock(tmpdir+"/hb", 0, nil, minTime, maxTime)
	require.NoError(t, err)
	app := hb.Appender()

	for _, l := range lbls {
		ls := labels.New(l...)
		series := []sample{}

		ts := rand.Int63n(300)
		for i := 0; i < numDatapoints; i++ {
			v := rand.Float64()
			if ts >= minTime && ts <= maxTime {
				series = append(series, sample{ts, v})
			}

			_, err := app.Add(ls, ts, v)
			if ts >= minTime && ts <= maxTime {
				require.NoError(t, err)
			} else {
				require.Error(t, ErrOutOfBounds, err)
			}

			ts += rand.Int63n(timeInterval) + 1
		}

		seriesMap[labels.New(l...).String()] = series
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

			q := hb.Querier(mint, maxt)
			ss := q.Select(qry.ms...)

			// Build the mockSeriesSet.
			matchedSeries := make([]Series, 0, len(matched))
			for _, m := range matched {
				smpls := boundedSamples(seriesMap[m.String()], mint, maxt)

				// Only append those series for which samples exist as mockSeriesSet
				// doesn't skip series with no samples.
				// TODO: But sometimes SeriesSet returns an empty SeriesIterator
				if len(smpls) > 0 {
					matchedSeries = append(matchedSeries, newSeries(
						m.Map(),
						smpls,
					))
				}
			}
			expSs := newListSeriesSet(matchedSeries)

			// Compare both SeriesSets.
			for {
				eok, rok := expSs.Next(), ss.Next()

				// Skip a series if iterator is empty.
				if rok {
					for !ss.At().Iterator().Next() {
						rok = ss.Next()
						if !rok {
							break
						}
					}
				}

				require.Equal(t, eok, rok, "next")

				if !eok {
					break
				}
				sexp := expSs.At()
				sres := ss.At()

				require.Equal(t, sexp.Labels(), sres.Labels(), "labels")

				smplExp, errExp := expandSeriesIterator(sexp.Iterator())
				smplRes, errRes := expandSeriesIterator(sres.Iterator())

				require.Equal(t, errExp, errRes, "samples error")
				require.Equal(t, smplExp, smplRes, "samples")
			}
		}
	}

	return
}

func boundedSamples(full []sample, mint, maxt int64) []sample {
	for len(full) > 0 {
		if full[0].t >= mint {
			break
		}
		full = full[1:]
	}
	for i, s := range full {
		// Terminate on the first sample larger than maxt.
		if s.t > maxt {
			return full[:i]
		}
	}
	// maxt is after highest sample.
	return full
}
